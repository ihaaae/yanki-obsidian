package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ihaaae/yanki/internal/anki"
	"github.com/ihaaae/yanki/internal/note"
)

// fakeAnki is a minimal in-memory AnkiConnect server covering the actions a
// sync uses.
type fakeAnki struct {
	mu         sync.Mutex
	notes      map[int64]*fakeNote
	nextNoteID int64
	nextCardID int64
	models     map[string]bool
	decks      map[string]bool
	calls      []string
}

type fakeNote struct {
	id     int64
	model  string
	fields map[string]string
	tags   []string
	deck   string
	cards  []int64
}

type fakeRequest struct {
	Action  string          `json:"action"`
	Version int             `json:"version"`
	Params  json.RawMessage `json:"params"`
	Key     string          `json:"key"`
}

func newFakeAnki() *fakeAnki {
	return &fakeAnki{
		notes:      map[int64]*fakeNote{},
		nextNoteID: 1000,
		nextCardID: 5000,
		models:     map[string]bool{},
		decks:      map[string]bool{"Default": true},
	}
}

func (f *fakeAnki) server(t *testing.T) (*httptest.Server, *anki.Client) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(server.Close)

	return server, anki.NewClient(server.URL, "")
}

func (f *fakeAnki) handle(w http.ResponseWriter, r *http.Request) {
	var request fakeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeFakeError(w, err.Error())
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, request.Action)

	switch request.Action {
	case "requestPermission":
		writeFakeResult(w, map[string]string{"permission": "granted"})
	case "version":
		writeFakeResult(w, 6)
	case "findNotes":
		f.findNotes(w, request.Params)
	case "notesInfo":
		f.notesInfo(w, request.Params)
	case "getDecks":
		f.getDecks(w, request.Params)
	case "addNote":
		f.addNote(w, request.Params)
	case "updateNoteModel":
		f.updateNoteModel(w, request.Params)
	case "changeDeck":
		f.changeDeck(w, request.Params)
	case "deleteNotes":
		f.deleteNotes(w, request.Params)
	case "createModel":
		var model struct {
			ModelName string `json:"modelName"`
		}
		_ = json.Unmarshal(request.Params, &model)
		if f.models[model.ModelName] {
			writeFakeError(w, "Model name already exists")
			return
		}

		f.models[model.ModelName] = true
		writeFakeResult(w, nil)
	case "modelNames":
		names := make([]string, 0, len(f.models))
		for name := range f.models {
			names = append(names, name)
		}

		sort.Strings(names)
		writeFakeResult(w, names)
	case "createDeck":
		var params struct {
			Deck string `json:"deck"`
		}
		_ = json.Unmarshal(request.Params, &params)
		f.decks[params.Deck] = true
		writeFakeResult(w, 1)
	case "getDeckStats":
		f.getDeckStats(w, request.Params)
	case "deleteDecks":
		var params struct {
			Decks []string `json:"decks"`
		}
		_ = json.Unmarshal(request.Params, &params)
		for _, deck := range params.Decks {
			delete(f.decks, deck)
		}

		writeFakeResult(w, nil)
	case "cardsInfo":
		writeFakeResult(w, []any{})
	case "guiCheckDatabase", "reloadCollection", "sync":
		writeFakeResult(w, nil)
	default:
		writeFakeError(w, "unsupported action: "+request.Action)
	}
}

func (f *fakeAnki) findNotes(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(params, &decoded)

	query := strings.Trim(decoded.Query, `"`)
	namespace := strings.TrimPrefix(query, "YankiNamespace:")

	ids := []int64{}
	for id, stored := range f.notes {
		if namespace == "*" || stored.fields["YankiNamespace"] == namespace {
			ids = append(ids, id)
		}
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	writeFakeResult(w, ids)
}

func (f *fakeAnki) notesInfo(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Notes []int64 `json:"notes"`
	}
	_ = json.Unmarshal(params, &decoded)

	infos := []map[string]any{}
	for _, id := range decoded.Notes {
		stored, ok := f.notes[id]
		if !ok {
			continue
		}

		fields := map[string]any{}
		for name, value := range stored.fields {
			fields[name] = map[string]any{"value": value, "order": 0}
		}

		infos = append(infos, map[string]any{
			"noteId":    stored.id,
			"modelName": stored.model,
			"tags":      stored.tags,
			"fields":    fields,
			"cards":     stored.cards,
		})
	}

	writeFakeResult(w, infos)
}

func (f *fakeAnki) getDecks(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Cards []int64 `json:"cards"`
	}
	_ = json.Unmarshal(params, &decoded)

	wanted := map[int64]bool{}
	for _, card := range decoded.Cards {
		wanted[card] = true
	}

	decks := map[string][]int64{}
	for _, stored := range f.notes {
		for _, card := range stored.cards {
			if wanted[card] {
				decks[stored.deck] = append(decks[stored.deck], card)
			}
		}
	}

	writeFakeResult(w, decks)
}

func (f *fakeAnki) addNote(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Note struct {
			DeckName  string            `json:"deckName"`
			ModelName string            `json:"modelName"`
			Fields    map[string]string `json:"fields"`
			Tags      []string          `json:"tags"`
		} `json:"note"`
	}
	_ = json.Unmarshal(params, &decoded)

	if !f.models[decoded.Note.ModelName] {
		writeFakeError(w, "model was not found: "+decoded.Note.ModelName)
		return
	}

	if !f.decks[decoded.Note.DeckName] {
		writeFakeError(w, "deck was not found: "+decoded.Note.DeckName)
		return
	}

	id := f.nextNoteID
	f.nextNoteID++

	cardID := f.nextCardID
	f.nextCardID++

	f.notes[id] = &fakeNote{
		id:     id,
		model:  decoded.Note.ModelName,
		fields: decoded.Note.Fields,
		tags:   decoded.Note.Tags,
		deck:   decoded.Note.DeckName,
		cards:  []int64{cardID},
	}

	writeFakeResult(w, id)
}

func (f *fakeAnki) updateNoteModel(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Note struct {
			ID        int64             `json:"id"`
			ModelName string            `json:"modelName"`
			Fields    map[string]string `json:"fields"`
			Tags      []string          `json:"tags"`
		} `json:"note"`
	}
	_ = json.Unmarshal(params, &decoded)

	stored, ok := f.notes[decoded.Note.ID]
	if !ok {
		writeFakeError(w, "note not found")
		return
	}

	stored.model = decoded.Note.ModelName
	stored.fields = decoded.Note.Fields
	stored.tags = decoded.Note.Tags

	writeFakeResult(w, nil)
}

func (f *fakeAnki) changeDeck(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Cards []int64 `json:"cards"`
		Deck  string  `json:"deck"`
	}
	_ = json.Unmarshal(params, &decoded)

	for _, stored := range f.notes {
		for _, card := range stored.cards {
			for _, wanted := range decoded.Cards {
				if card == wanted {
					stored.deck = decoded.Deck
				}
			}
		}
	}

	writeFakeResult(w, nil)
}

func (f *fakeAnki) deleteNotes(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Notes []int64 `json:"notes"`
	}
	_ = json.Unmarshal(params, &decoded)

	for _, id := range decoded.Notes {
		delete(f.notes, id)
	}

	writeFakeResult(w, nil)
}

func (f *fakeAnki) getDeckStats(w http.ResponseWriter, params json.RawMessage) {
	var decoded struct {
		Decks []string `json:"decks"`
	}
	_ = json.Unmarshal(params, &decoded)

	stats := map[string]map[string]int{}
	for index, deck := range decoded.Decks {
		cardCount := 0
		for _, stored := range f.notes {
			if stored.deck == deck {
				cardCount += len(stored.cards)
			}
		}

		stats[string(rune('0'+index))] = map[string]int{
			"deck_id":       index,
			"new_count":     0,
			"learn_count":   0,
			"review_count":  0,
			"total_in_deck": cardCount,
		}
	}

	writeFakeResult(w, stats)
}

func writeFakeResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
}

func writeFakeError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"result": nil, "error": message})
}

// helpers

func testNote(deck, model, front, back string) note.Note {
	return note.Note{
		DeckName:  deck,
		ModelName: note.ModelName(model),
		Fields: note.Fields{
			Front:          front,
			Back:           back,
			YankiNamespace: "Test",
		},
	}
}

func withNoteID(value note.Note, noteID int64) note.Note {
	value.NoteID = noteID
	return value
}

func countAction(result Result, action Action) int {
	count := 0
	for _, entry := range result.Synced {
		if entry.Action == action {
			count++
		}
	}

	return count
}

func TestSyncCreatesUpdatesAndDeletes(t *testing.T) {
	t.Parallel()

	fake := newFakeAnki()
	_, client := fake.server(t)

	options := Options{
		CheckDatabase: false,
		Namespace:     "Test",
	}

	// First sync creates the note, along with its model and deck.
	created, err := Notes(t.Context(), client, []note.Note{
		testNote("Animals", string(note.ModelBasic), "<p>Horse</p>", "<p>A quadruped</p>"),
	}, options)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}

	if countAction(created, ActionCreated) != 1 {
		t.Fatalf("first sync actions = %v, want one created", created.Synced)
	}

	noteID := created.Synced[0].Note.NoteID
	if noteID == 0 {
		t.Fatal("created note has no ID")
	}

	if !fake.models[string(note.ModelBasic)] {
		t.Error("model was not created")
	}

	if !fake.decks["Animals"] {
		t.Error("deck was not created")
	}

	// A second sync with identical content changes nothing. The note ID would
	// normally come from the file's frontmatter; carry it forward here.
	unchanged, err := Notes(t.Context(), client, []note.Note{
		withNoteID(testNote("Animals", string(note.ModelBasic), "<p>Horse</p>", "<p>A quadruped</p>"), noteID),
	}, options)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if countAction(unchanged, ActionUnchanged) != 1 {
		t.Fatalf("second sync actions = %v, want one unchanged", unchanged.Synced)
	}

	// Changing the content updates the note, preserving its ID.
	updated, err := Notes(t.Context(), client, []note.Note{
		withNoteID(testNote("Animals", string(note.ModelBasic), "<p>Horse</p>", "<p>An odd-toed ungulate</p>"), noteID),
	}, options)
	if err != nil {
		t.Fatalf("third sync: %v", err)
	}

	if countAction(updated, ActionUpdated) != 1 {
		t.Fatalf("third sync actions = %v, want one updated", updated.Synced)
	}

	if updated.Synced[0].Note.NoteID != noteID {
		t.Errorf("note ID changed from %d to %d", noteID, updated.Synced[0].Note.NoteID)
	}

	if fake.notes[noteID].fields["Back"] != "<p>An odd-toed ungulate</p>" {
		t.Errorf("Anki note was not updated: %v", fake.notes[noteID].fields)
	}

	// Syncing no notes deletes the orphan.
	deleted, err := Notes(t.Context(), client, nil, options)
	if err != nil {
		t.Fatalf("fourth sync: %v", err)
	}

	if countAction(deleted, ActionDeleted) != 1 {
		t.Fatalf("fourth sync actions = %v, want one deleted", deleted.Synced)
	}

	if len(fake.notes) != 0 {
		t.Errorf("expected no notes after deletion, got %d", len(fake.notes))
	}
}

func TestSyncDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()

	fake := newFakeAnki()
	_, client := fake.server(t)

	result, err := Notes(t.Context(), client, []note.Note{
		testNote("Animals", string(note.ModelBasic), "<p>Horse</p>", "<p>A quadruped</p>"),
	}, Options{DryRun: true, Namespace: "Test"})
	if err != nil {
		t.Fatalf("dry run sync: %v", err)
	}

	if countAction(result, ActionCreated) != 1 {
		t.Fatalf("dry run actions = %v, want one created", result.Synced)
	}

	if len(fake.notes) != 0 {
		t.Errorf("dry run created %d notes, want 0", len(fake.notes))
	}
}

func TestSyncUnreachableAnki(t *testing.T) {
	t.Parallel()

	// A client pointed at a closed port reports every note as unreachable.
	client := anki.NewClient("http://127.0.0.1:1", "")

	result, err := Notes(t.Context(), client, []note.Note{
		testNote("Animals", string(note.ModelBasic), "<p>Horse</p>", ""),
	}, Options{Namespace: "Test"})
	if err != nil {
		t.Fatalf("sync with unreachable Anki returned error: %v", err)
	}

	if countAction(result, ActionAnkiUnreachable) != 1 {
		t.Fatalf("actions = %v, want one ankiUnreachable", result.Synced)
	}
}
