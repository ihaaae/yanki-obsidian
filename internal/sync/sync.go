// Package sync reconciles local Markdown notes with the notes in Anki.
package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ihaaae/yanki/internal/anki"
	"github.com/ihaaae/yanki/internal/note"
	"golang.org/x/text/unicode/norm"
)

// normalizeNFC makes string comparisons tolerant of Unicode normalization
// differences between platforms.
func normalizeNFC(value string) string {
	return norm.NFC.String(value)
}

// Action describes what happened to a note during a sync.
type Action string

const (
	// ActionCreated means the note was added to Anki.
	ActionCreated Action = "created"
	// ActionUpdated means an existing Anki note was changed.
	ActionUpdated Action = "updated"
	// ActionUnchanged means an existing Anki note already matched.
	ActionUnchanged Action = "unchanged"
	// ActionMatched means a local note was matched to an existing Anki note by
	// content.
	ActionMatched Action = "matched"
	// ActionDeleted means an Anki note was removed because its file is gone.
	ActionDeleted Action = "deleted"
	// ActionAnkiUnreachable means Anki could not be contacted.
	ActionAnkiUnreachable Action = "ankiUnreachable"
)

// SyncedNote pairs a note with the action taken on it.
type SyncedNote struct {
	Action Action
	Note   note.Note
}

// Result summarizes a sync.
type Result struct {
	Synced          []SyncedNote
	DeletedDecks    []string
	DeletedMedia    []string
	ReuploadedMedia []string
	FixedDatabase   bool
	Duration        time.Duration
	// AnkiWeb reports whether an AnkiWeb push was requested.
	AnkiWeb bool
	// DryRun reports whether changes were simulated rather than applied.
	DryRun bool
	// Namespace is the namespace the sync ran under.
	Namespace string
}

// Options configures a sync.
type Options struct {
	// Namespace scopes the sync. Only notes with this namespace are managed.
	Namespace string
	// DryRun simulates the sync without touching Anki.
	DryRun bool
	// CheckDatabase asks Anki to repair the collection after model changes.
	CheckDatabase bool
	// PushToAnkiWeb triggers an AnkiWeb sync after a local sync.
	PushToAnkiWeb bool
	// StrictMatching only matches local notes to remote notes by note ID, not
	// by content.
	StrictMatching bool
}

// Notes syncs local notes to Anki.
func Notes(ctx context.Context, client *anki.Client, localNotes []note.Note, options Options) (Result, error) {
	start := time.Now()

	namespace, err := note.ValidateAndSanitizeNamespace(options.Namespace)
	if err != nil {
		return Result{}, err
	}

	permission, err := client.RequestPermission(ctx)
	if err != nil {
		if anki.Unreachable(err) {
			return unreachableResult(localNotes, options, namespace, start), nil
		}

		return Result{}, err
	}

	if permission.Permission == "denied" {
		return Result{}, errors.New("permission denied; add this origin to the AnkiConnect add-on's webCorsOriginList configuration")
	}

	// Copy the notes so the caller's slice is not mutated.
	locals := make([]note.Note, len(localNotes))
	copy(locals, localNotes)

	for index := range locals {
		if locals[index].DeckName == "" {
			locals[index].DeckName = note.DefaultDeckName
		}
	}

	allRemote, err := getRemoteNotes(ctx, client, "*")
	if err != nil {
		return Result{}, err
	}

	var remote []note.Note
	for _, remoteNote := range allRemote {
		if remoteNote.Fields.YankiNamespace == namespace {
			remote = append(remote, remoteNote)
		}
	}

	clearDuplicateNoteIDs(locals, remote)

	matchedIDs := make(map[int64]bool)
	for _, local := range locals {
		if local.NoteID == 0 {
			continue
		}

		for _, remoteNote := range remote {
			if remoteNote.NoteID == local.NoteID {
				matchedIDs[local.NoteID] = true
				break
			}
		}
	}

	synced := make([]SyncedNote, 0, len(locals))

	for index := range locals {
		local := &locals[index]

		remoteNote := findRemoteByID(allRemote, local.NoteID)

		// A note ID that belongs to another namespace is stale: recreate.
		if remoteNote != nil && remoteNote.Fields.YankiNamespace != namespace {
			local.NoteID = 0
			remoteNote = nil
		}

		if remoteNote == nil {
			if !options.StrictMatching {
				local.NoteID = findContentMatchID(*local, remote, matchedIDs)
			}

			if local.NoteID == 0 {
				newID, err := addNote(ctx, client, *local, options.DryRun)
				if err != nil {
					return Result{}, err
				}

				local.NoteID = newID
				synced = append(synced, SyncedNote{Action: ActionCreated, Note: *local})
			} else {
				synced = append(synced, SyncedNote{Action: ActionMatched, Note: *local})
			}
		} else {
			updated, err := updateNote(ctx, client, *local, *remoteNote, options.DryRun)
			if err != nil {
				return Result{}, err
			}

			action := ActionUnchanged
			if updated {
				action = ActionUpdated
			}

			synced = append(synced, SyncedNote{Action: action, Note: *local})
		}

		matchedIDs[local.NoteID] = true
	}

	// Delete remote notes that no local note claims.
	var orphaned []note.Note
	for _, remoteNote := range remote {
		claimed := false
		for _, local := range locals {
			if local.NoteID == remoteNote.NoteID {
				claimed = true
				break
			}
		}

		if !claimed {
			orphaned = append(orphaned, remoteNote)
		}
	}

	if !options.DryRun && len(orphaned) > 0 {
		ids := make([]int64, 0, len(orphaned))
		for _, orphan := range orphaned {
			ids = append(ids, orphan.NoteID)
		}

		if err := client.DeleteNotes(ctx, ids); err != nil {
			return Result{}, fmt.Errorf("delete orphaned notes: %w", err)
		}
	}

	for _, orphan := range orphaned {
		synced = append(synced, SyncedNote{Action: ActionDeleted, Note: orphan})
	}

	live := make([]note.Note, 0, len(synced))
	for _, entry := range synced {
		if entry.Action != ActionDeleted {
			live = append(live, entry.Note)
		}
	}

	deletedDecks, err := deleteOrphanedDecks(ctx, client, live, remote, options.DryRun)
	if err != nil {
		return Result{}, err
	}

	fixedDatabase := false
	if options.CheckDatabase {
		fixedDatabase, err = checkDatabase(ctx, client, synced, remote)
		if err != nil {
			return Result{}, err
		}
	}

	if !options.DryRun && options.PushToAnkiWeb {
		// A failure here is non-fatal: the local sync already succeeded.
		_ = client.Sync(ctx)
	}

	return Result{
		AnkiWeb:       options.PushToAnkiWeb,
		DeletedDecks:  deletedDecks,
		Duration:      time.Since(start),
		DryRun:        options.DryRun,
		FixedDatabase: fixedDatabase,
		Namespace:     namespace,
		Synced:        synced,
	}, nil
}

func unreachableResult(localNotes []note.Note, options Options, namespace string, start time.Time) Result {
	synced := make([]SyncedNote, 0, len(localNotes))
	for _, local := range localNotes {
		synced = append(synced, SyncedNote{Action: ActionAnkiUnreachable, Note: local})
	}

	return Result{
		AnkiWeb:   options.PushToAnkiWeb,
		Duration:  time.Since(start),
		DryRun:    options.DryRun,
		Namespace: namespace,
		Synced:    synced,
	}
}

// getRemoteNotes returns every Yanki-managed note in Anki whose namespace
// matches the given search value ("*" for all).
func getRemoteNotes(ctx context.Context, client *anki.Client, namespace string) ([]note.Note, error) {
	query := fmt.Sprintf("%q", "YankiNamespace:"+namespace)

	ids, err := client.FindNotes(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("find notes: %w", err)
	}

	if len(ids) == 0 {
		return nil, nil
	}

	infos, err := client.NotesInfo(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("notes info: %w", err)
	}

	var cardIDs []int64
	for _, info := range infos {
		cardIDs = append(cardIDs, info.Cards...)
	}

	deckByCard := make(map[int64]string)
	if len(cardIDs) > 0 {
		decks, err := client.GetDecks(ctx, cardIDs)
		if err != nil {
			return nil, fmt.Errorf("get decks: %w", err)
		}

		for deckName, cards := range decks {
			for _, cardID := range cards {
				deckByCard[cardID] = deckName
			}
		}
	}

	notes := make([]note.Note, 0, len(infos))
	for _, info := range infos {
		if info.NoteID == 0 {
			// findNotes can return phantom IDs that notesInfo no longer
			// resolves. Skip them.
			continue
		}

		modelName := note.ModelName(info.ModelName)
		if !isKnownModel(modelName) {
			return nil, fmt.Errorf("unknown model name %q for note %d", info.ModelName, info.NoteID)
		}

		deckName := ""
		for _, cardID := range info.Cards {
			if name, ok := deckByCard[cardID]; ok {
				deckName = name
				break
			}
		}

		if deckName == "" {
			return nil, fmt.Errorf("no deck found for cards in note %d", info.NoteID)
		}

		notes = append(notes, note.Note{
			NoteID:    info.NoteID,
			DeckName:  deckName,
			ModelName: modelName,
			Fields: note.Fields{
				Front:          fieldValue(info, "Front"),
				Back:           fieldValue(info, "Back"),
				Extra:          fieldValue(info, "Extra"),
				YankiNamespace: fieldValue(info, "YankiNamespace"),
			},
			Tags:  info.Tags,
			Cards: info.Cards,
		})
	}

	return notes, nil
}

func fieldValue(info anki.NoteInfo, name string) string {
	if field, ok := info.Fields[name]; ok {
		return field.Value
	}

	return ""
}

func isKnownModel(modelName note.ModelName) bool {
	for _, known := range note.KnownModelNames {
		if known == modelName {
			return true
		}
	}

	return false
}

func findRemoteByID(remote []note.Note, noteID int64) *note.Note {
	if noteID == 0 {
		return nil
	}

	for index := range remote {
		if remote[index].NoteID == noteID {
			return &remote[index]
		}
	}

	return nil
}

// clearDuplicateNoteIDs handles several local notes claiming the same Anki
// note, which happens when a synced note is duplicated as a shortcut. The copy
// whose content matches Anki keeps the ID; the rest become new notes.
func clearDuplicateNoteIDs(locals []note.Note, remote []note.Note) {
	for index := range locals {
		if locals[index].NoteID == 0 {
			continue
		}

		var duplicates []*note.Note
		for other := range locals {
			if locals[other].NoteID == locals[index].NoteID {
				duplicates = append(duplicates, &locals[other])
			}
		}

		if len(duplicates) <= 1 {
			continue
		}

		remoteNote := findRemoteByID(remote, locals[index].NoteID)

		keep := duplicates[0]
		for _, duplicate := range duplicates {
			if remoteNote != nil && fieldsEqual(duplicate.Fields, remoteNote.Fields) {
				keep = duplicate
				break
			}
		}

		for _, duplicate := range duplicates {
			if duplicate != keep {
				duplicate.NoteID = 0
			}
		}
	}
}

func findContentMatchID(local note.Note, remote []note.Note, matchedIDs map[int64]bool) int64 {
	for _, remoteNote := range remote {
		if remoteNote.NoteID == 0 || matchedIDs[remoteNote.NoteID] {
			continue
		}

		if notesEqual(local, remoteNote, false) {
			return remoteNote.NoteID
		}
	}

	return 0
}

// addNote creates a note, creating its model or deck on demand.
func addNote(ctx context.Context, client *anki.Client, local note.Note, dryRun bool) (int64, error) {
	if local.NoteID != 0 {
		return 0, errors.New("note already has an ID")
	}

	if dryRun {
		return 0, nil
	}

	id, err := client.AddNote(ctx, anki.AddNoteParams{
		DeckName:       local.DeckName,
		ModelName:      string(local.ModelName),
		Fields:         local.AnkiFields(),
		Tags:           local.Tags,
		AllowDuplicate: true,
	})

	if err != nil {
		var apiErr *anki.APIError
		if !errors.As(err, &apiErr) {
			return 0, err
		}

		switch {
		case apiErr.Message == "model was not found: "+string(local.ModelName):
			if err := ensureModelExists(ctx, client, local.ModelName); err != nil {
				return 0, err
			}

			return addNote(ctx, client, local, dryRun)
		case apiErr.Message == "deck was not found: "+local.DeckName:
			if local.DeckName == "" {
				return 0, errors.New("deck name is empty")
			}

			if _, err := client.CreateDeck(ctx, local.DeckName); err != nil {
				return 0, err
			}

			return addNote(ctx, client, local, dryRun)
		default:
			return 0, err
		}
	}

	return id, nil
}

// updateNote applies deck, field, tag, and model changes to an existing note,
// reporting whether anything changed.
func updateNote(ctx context.Context, client *anki.Client, local, remote note.Note, dryRun bool) (bool, error) {
	if local.NoteID == 0 {
		return false, errors.New("local note ID is undefined")
	}

	if len(remote.Cards) == 0 {
		return false, errors.New("remote note cards are undefined")
	}

	updated := false

	if local.DeckName != remote.DeckName {
		if local.DeckName == "" {
			return false, errors.New("local deck name is empty")
		}

		if !dryRun {
			if err := client.ChangeDeck(ctx, remote.Cards, local.DeckName); err != nil {
				return false, err
			}
		}

		updated = true
	}

	if !tagsEqual(local.Tags, remote.Tags) || !fieldsEqual(local.Fields, remote.Fields) || local.ModelName != remote.ModelName {
		if !dryRun {
			err := client.UpdateNoteModel(ctx, anki.UpdateNoteModelParams{
				ID:        local.NoteID,
				ModelName: string(local.ModelName),
				Fields:    local.AnkiFields(),
				Tags:      local.Tags,
			})
			if err != nil {
				var apiErr *anki.APIError
				if errors.As(err, &apiErr) && apiErr.Message == fmt.Sprintf("Model '%s' not found", local.ModelName) {
					if ensureErr := ensureModelExists(ctx, client, local.ModelName); ensureErr != nil {
						return false, ensureErr
					}

					return updateNote(ctx, client, local, remote, dryRun)
				}

				return false, err
			}
		}

		updated = true
	}

	return updated, nil
}

// ensureModelExists creates a Yanki model if Anki does not have it yet.
func ensureModelExists(ctx context.Context, client *anki.Client, modelName note.ModelName) error {
	definition, ok := note.ModelDefinition(modelName)
	if !ok {
		return fmt.Errorf("model not found: %s", modelName)
	}

	if err := client.CreateModel(ctx, definition); err != nil {
		var apiErr *anki.APIError
		if errors.As(err, &apiErr) && apiErr.Message == "Model name already exists" {
			return nil
		}

		return err
	}

	return nil
}

// fieldsEqual compares Front, Back, and Extra with presence semantics: a field
// present on only one side counts as different.
func fieldsEqual(local, remote note.Fields) bool {
	type pair struct {
		local  string
		remote string
	}

	values := []pair{
		{local.Front, remote.Front},
		{local.Back, remote.Back},
	}

	// Extra only participates when either side defines it, which the model
	// determines. Empty on both sides is equal.
	if local.Extra != "" || remote.Extra != "" {
		values = append(values, pair{local.Extra, remote.Extra})
	}

	for _, value := range values {
		if normalizeNFC(value.local) != normalizeNFC(value.remote) {
			return false
		}
	}

	return true
}

func tagsEqual(local, remote []string) bool {
	union := make(map[string]bool)
	for _, tag := range local {
		union[strings.ToLower(normalizeNFC(tag))] = true
	}

	remoteSet := make(map[string]bool)
	for _, tag := range remote {
		remoteSet[strings.ToLower(normalizeNFC(tag))] = true
		union[strings.ToLower(normalizeNFC(tag))] = true
	}

	return len(union) == len(remoteSet)
}

func notesEqual(a, b note.Note, includeID bool) bool {
	if includeID && a.NoteID != b.NoteID {
		return false
	}

	if a.DeckName != b.DeckName || a.ModelName != b.ModelName {
		return false
	}

	return fieldsEqual(a.Fields, b.Fields) && tagsEqual(a.Tags, b.Tags)
}

// deleteOrphanedDecks removes decks that no longer hold any live note.
func deleteOrphanedDecks(ctx context.Context, client *anki.Client, active, original []note.Note, dryRun bool) ([]string, error) {
	activeDecks := make(map[string]bool)
	for _, note := range active {
		if note.DeckName != "" {
			activeDecks[note.DeckName] = true
		}
	}

	candidates := make(map[string]bool)
	for _, note := range original {
		if note.DeckName == "" || activeDecks[note.DeckName] {
			continue
		}

		candidates[note.DeckName] = true
		for _, parent := range orphanedParentDeckNames(note.DeckName, activeDecks) {
			candidates[parent] = true
		}
	}

	var toDelete []string
	for deckName := range candidates {
		stats, err := client.GetDeckStats(ctx, []string{deckName})
		if err != nil {
			return nil, err
		}

		if len(stats) != 1 {
			continue
		}

		for _, deckStats := range stats {
			if deckStats.Count() == 0 {
				toDelete = append(toDelete, deckName)
			}
		}
	}

	if !dryRun && len(toDelete) > 0 {
		if err := client.DeleteDecks(ctx, toDelete, true); err != nil {
			return nil, err
		}
	}

	return toDelete, nil
}

// orphanedParentDeckNames walks up a deck's ancestry, stopping at the first
// ancestor that still contains an active note.
func orphanedParentDeckNames(deckName string, activeDecks map[string]bool) []string {
	var parents []string
	parts := strings.Split(deckName, "::")

	for len(parts) > 1 {
		parts = parts[:len(parts)-1]
		parent := strings.Join(parts, "::")

		hasActiveChild := false
		for active := range activeDecks {
			if strings.HasPrefix(active, parent+"::") {
				hasActiveChild = true
				break
			}
		}

		if hasActiveChild {
			break
		}

		parents = append(parents, parent)
	}

	return parents
}

// checkDatabase verifies that model changes did not leave stale cards behind,
// repairing the collection if they did.
func checkDatabase(ctx context.Context, client *anki.Client, synced []SyncedNote, remote []note.Note) (bool, error) {
	var cardIDs []int64

	for _, entry := range synced {
		if entry.Action != ActionUpdated {
			continue
		}

		for _, remoteNote := range remote {
			if remoteNote.NoteID == entry.Note.NoteID && remoteNote.ModelName != entry.Note.ModelName {
				cardIDs = append(cardIDs, remoteNote.Cards...)
			}
		}
	}

	if len(cardIDs) == 0 {
		return false, nil
	}

	if _, err := client.CardsInfo(ctx, cardIDs); err != nil {
		if checkErr := client.GuiCheckDatabase(ctx); checkErr != nil {
			return false, checkErr
		}

		if reloadErr := client.ReloadCollection(ctx); reloadErr != nil {
			return false, reloadErr
		}

		return true, nil
	}

	return false, nil
}

// ListRemote returns the Yanki notes in Anki for a namespace, or for every
// namespace when namespace is "*".
func ListRemote(ctx context.Context, client *anki.Client, namespace string) ([]note.Note, error) {
	return getRemoteNotes(ctx, client, namespace)
}

// CleanNamespace deletes every Yanki note in Anki for a namespace, returning
// the deleted note IDs. With dryRun it reports the IDs without deleting.
func CleanNamespace(ctx context.Context, client *anki.Client, namespace string, dryRun bool) ([]int64, error) {
	notes, err := getRemoteNotes(ctx, client, namespace)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(notes))
	for _, remoteNote := range notes {
		ids = append(ids, remoteNote.NoteID)
	}

	if dryRun || len(ids) == 0 {
		return ids, nil
	}

	if err := client.DeleteNotes(ctx, ids); err != nil {
		return nil, fmt.Errorf("delete notes: %w", err)
	}

	return ids, nil
}
