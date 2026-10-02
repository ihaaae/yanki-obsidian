package anki

// NoteInfo is the subset of a note's Anki data that Yanki reads. Field values
// are keyed by field name; the "picture", "sound", and "video" pseudo-fields
// AnkiConnect sometimes adds are ignored.
type NoteInfo struct {
	NoteID    int64                `json:"noteId"`
	ModelName string               `json:"modelName"`
	Tags      []string             `json:"tags"`
	Fields    map[string]FieldInfo `json:"fields"`
	Cards     []int64              `json:"cards"`
}

// FieldInfo carries a field's rendered value and its Anki-defined order.
type FieldInfo struct {
	Value string `json:"value"`
	Order int    `json:"order"`
}

// DeckConfig is the subset of a deck's configuration that Yanki reads. A
// non-zero Dyn means the deck is a filtered (dynamic) deck.
type DeckConfig struct {
	Dyn int `json:"dyn"`
}

// DeckStats is the subset of a deck's statistics that Yanki reads when deciding
// whether a deck is empty and safe to delete.
type DeckStats struct {
	DeckID      int64 `json:"deck_id"`
	NewCount    int   `json:"new_count"`
	LearnCount  int   `json:"learn_count"`
	ReviewCount int   `json:"review_count"`
	TotalInDeck int   `json:"total_in_deck"`
}

// Count returns a best-effort card count. Anki's total_in_deck is occasionally
// unreliable, so the sum of the bucket counts is used when it is larger.
func (s DeckStats) Count() int {
	if s.TotalInDeck > s.NewCount+s.LearnCount+s.ReviewCount {
		return s.TotalInDeck
	}

	return s.NewCount + s.LearnCount + s.ReviewCount
}

// CardTemplate is one card template in a note model.
type CardTemplate struct {
	Name string `json:"Name"`
	Ord  int    `json:"Ord"`
	Qfmt string `json:"Front"`
	Afmt string `json:"Back"`
}

// Model describes a note type. Front/Back in the wire protocol are named Qfmt
// and Afmt in Anki's own data model.
type Model struct {
	CardTemplates []CardTemplate `json:"cardTemplates"`
	InOrderFields []string       `json:"inOrderFields"`
	IsCloze       bool           `json:"isCloze,omitempty"`
	CSS           string         `json:"css,omitempty"`
	ModelName     string         `json:"modelName"`
}

// ModelStyling is the CSS associated with a note model.
type ModelStyling struct {
	CSS string `json:"css"`
}

// Permission is the result of the requestPermission action.
type Permission struct {
	Permission string `json:"permission"`
}
