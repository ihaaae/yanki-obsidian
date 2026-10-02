package anki

import "context"

// RequestPermission asks AnkiConnect whether this client is allowed to connect.
// It is the cheapest way to tell a running Anki apart from a closed one.
func (c *Client) RequestPermission(ctx context.Context) (Permission, error) {
	var permission Permission
	err := c.invoke(ctx, "requestPermission", nil, &permission)
	return permission, err
}

// Version returns the AnkiConnect add-on's version.
func (c *Client) Version(ctx context.Context) (int, error) {
	var version int
	err := c.invoke(ctx, "version", nil, &version)
	return version, err
}

// DeckNames returns the names of every deck in the collection.
func (c *Client) DeckNames(ctx context.Context) ([]string, error) {
	var names []string
	err := c.invoke(ctx, "deckNames", nil, &names)
	return names, err
}

// CreateDeck creates a deck, returning its ID. Creating an existing deck is not
// an error.
func (c *Client) CreateDeck(ctx context.Context, deck string) (int64, error) {
	var id int64
	err := c.invoke(ctx, "createDeck", map[string]any{"deck": deck}, &id)
	return id, err
}

// ChangeDeck moves the given cards into a deck.
func (c *Client) ChangeDeck(ctx context.Context, cards []int64, deck string) error {
	return c.invoke(ctx, "changeDeck", map[string]any{"cards": cards, "deck": deck}, nil)
}

// GetDecks maps deck names to the cards they contain, for the given cards.
func (c *Client) GetDecks(ctx context.Context, cards []int64) (map[string][]int64, error) {
	var decks map[string][]int64
	err := c.invoke(ctx, "getDecks", map[string]any{"cards": cards}, &decks)
	return decks, err
}

// GetDeckConfig returns a deck's configuration.
func (c *Client) GetDeckConfig(ctx context.Context, deck string) (DeckConfig, error) {
	var config DeckConfig
	err := c.invoke(ctx, "getDeckConfig", map[string]any{"deck": deck}, &config)
	return config, err
}

// GetDeckStats returns statistics for the given decks, keyed by deck ID as a
// string (as AnkiConnect returns them).
func (c *Client) GetDeckStats(ctx context.Context, decks []string) (map[string]DeckStats, error) {
	var stats map[string]DeckStats
	err := c.invoke(ctx, "getDeckStats", map[string]any{"decks": decks}, &stats)
	return stats, err
}

// DeleteDecks deletes decks, optionally deleting their cards too.
func (c *Client) DeleteDecks(ctx context.Context, decks []string, cardsToo bool) error {
	return c.invoke(ctx, "deleteDecks", map[string]any{"decks": decks, "cardsToo": cardsToo}, nil)
}

// ModelNames returns the names of every note model in the collection.
func (c *Client) ModelNames(ctx context.Context) ([]string, error) {
	var names []string
	err := c.invoke(ctx, "modelNames", nil, &names)
	return names, err
}

// CreateModel creates a note model.
func (c *Client) CreateModel(ctx context.Context, model Model) error {
	return c.invoke(ctx, "createModel", model, nil)
}

// ModelStyling returns a model's CSS.
func (c *Client) ModelStyling(ctx context.Context, modelName string) (ModelStyling, error) {
	var styling ModelStyling
	err := c.invoke(ctx, "modelStyling", map[string]any{"modelName": modelName}, &styling)
	return styling, err
}

// UpdateModelStyling replaces a model's CSS.
func (c *Client) UpdateModelStyling(ctx context.Context, modelName, css string) error {
	return c.invoke(ctx, "updateModelStyling", map[string]any{
		"model": map[string]any{"name": modelName, "css": css},
	}, nil)
}

// FindNotes returns the IDs of every note matching an Anki search query.
func (c *Client) FindNotes(ctx context.Context, query string) ([]int64, error) {
	var ids []int64
	err := c.invoke(ctx, "findNotes", map[string]any{"query": query}, &ids)
	return ids, err
}

// NotesInfo returns detailed information for the given note IDs.
func (c *Client) NotesInfo(ctx context.Context, notes []int64) ([]NoteInfo, error) {
	var info []NoteInfo
	err := c.invoke(ctx, "notesInfo", map[string]any{"notes": notes}, &info)
	return info, err
}

// AddNoteParams describes a note to create.
type AddNoteParams struct {
	DeckName       string            `json:"deckName"`
	ModelName      string            `json:"modelName"`
	Fields         map[string]string `json:"fields"`
	Tags           []string          `json:"tags,omitempty"`
	AllowDuplicate bool              `json:"-"`
}

// AddNote creates a note, returning its ID.
func (c *Client) AddNote(ctx context.Context, note AddNoteParams) (int64, error) {
	params := map[string]any{
		"note": map[string]any{
			"deckName":  note.DeckName,
			"modelName": note.ModelName,
			"fields":    note.Fields,
			"tags":      note.Tags,
			"options":   map[string]any{"allowDuplicate": note.AllowDuplicate},
		},
	}

	var id int64
	err := c.invoke(ctx, "addNote", params, &id)
	return id, err
}

// UpdateNoteModelParams describes the fields, model, and tags to set on an
// existing note. This is AnkiConnect's `updateNoteModel` action, which updates
// the model assignment, fields, and tags in one call.
type UpdateNoteModelParams struct {
	ID        int64             `json:"id"`
	ModelName string            `json:"modelName"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
}

// UpdateNoteModel updates an existing note's model, fields, and tags.
func (c *Client) UpdateNoteModel(ctx context.Context, note UpdateNoteModelParams) error {
	return c.invoke(ctx, "updateNoteModel", map[string]any{"note": note}, nil)
}

// DeleteNotes deletes the given notes.
func (c *Client) DeleteNotes(ctx context.Context, notes []int64) error {
	return c.invoke(ctx, "deleteNotes", map[string]any{"notes": notes}, nil)
}

// CardsInfo returns information for the given cards.
func (c *Client) CardsInfo(ctx context.Context, cards []int64) ([]map[string]any, error) {
	var info []map[string]any
	err := c.invoke(ctx, "cardsInfo", map[string]any{"cards": cards}, &info)
	return info, err
}

// AddTags adds tags to the given notes.
func (c *Client) AddTags(ctx context.Context, tags []string, notes []int64) error {
	return c.invoke(ctx, "addTags", map[string]any{"tags": tags, "notes": notes}, nil)
}

// RemoveTags removes tags from the given notes.
func (c *Client) RemoveTags(ctx context.Context, tags []string, notes []int64) error {
	return c.invoke(ctx, "removeTags", map[string]any{"tags": tags, "notes": notes}, nil)
}

// Sync triggers a full AnkiWeb sync, the equivalent of pressing Sync in Anki.
func (c *Client) Sync(ctx context.Context) error {
	return c.invoke(ctx, "sync", nil, nil)
}

// GuiCheckDatabase asks Anki to check and repair the collection database.
func (c *Client) GuiCheckDatabase(ctx context.Context) error {
	return c.invoke(ctx, "guiCheckDatabase", nil, nil)
}

// ReloadCollection reloads the collection from disk.
func (c *Client) ReloadCollection(ctx context.Context) error {
	return c.invoke(ctx, "reloadCollection", nil, nil)
}

// GetMediaFilesNames returns the names of media files matching a glob pattern.
func (c *Client) GetMediaFilesNames(ctx context.Context, pattern string) ([]string, error) {
	var names []string
	err := c.invoke(ctx, "getMediaFilesNames", map[string]any{"pattern": pattern}, &names)
	return names, err
}

// StoreMediaFileByPath stores a local file as an Anki media asset.
func (c *Client) StoreMediaFileByPath(ctx context.Context, filename, path string, deleteExisting bool) (string, error) {
	var stored string
	err := c.invoke(ctx, "storeMediaFile", map[string]any{
		"filename":       filename,
		"path":           path,
		"deleteExisting": deleteExisting,
	}, &stored)
	return stored, err
}

// StoreMediaFileByURL stores a remote URL as an Anki media asset.
func (c *Client) StoreMediaFileByURL(ctx context.Context, filename, url string, deleteExisting bool) (string, error) {
	var stored string
	err := c.invoke(ctx, "storeMediaFile", map[string]any{
		"filename":       filename,
		"url":            url,
		"deleteExisting": deleteExisting,
	}, &stored)
	return stored, err
}

// DeleteMediaFile deletes an Anki media asset.
func (c *Client) DeleteMediaFile(ctx context.Context, filename string) error {
	return c.invoke(ctx, "deleteMediaFile", map[string]any{"filename": filename}, nil)
}
