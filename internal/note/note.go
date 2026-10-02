package note

// Fields holds the rendered HTML for a note's fields. Extra is only meaningful
// for the reversed model.
type Fields struct {
	Front          string
	Back           string
	Extra          string
	YankiNamespace string
}

// Note is a single Anki note as Yanki sees it, whether it came from a Markdown
// file or from Anki. A NoteID of zero means the note does not exist in Anki
// yet.
type Note struct {
	NoteID    int64
	DeckName  string
	ModelName ModelName
	Fields    Fields
	Tags      []string
	Cards     []int64
}

// AnkiFields converts the note's fields into the map AnkiConnect expects,
// omitting Extra when the model does not define it.
func (n Note) AnkiFields() map[string]string {
	fields := map[string]string{
		"Front":          n.Fields.Front,
		"Back":           n.Fields.Back,
		"YankiNamespace": n.Fields.YankiNamespace,
	}

	if n.ModelName == ModelBasicReversed {
		fields["Extra"] = n.Fields.Extra
	}

	return fields
}
