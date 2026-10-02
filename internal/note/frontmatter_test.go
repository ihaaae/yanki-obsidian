package note

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		markdown   string
		wantNoteID int64
		wantTags   []string
		wantDeck   string
	}{
		{
			name:     "no frontmatter",
			markdown: "Front\n\n---\n\nBack",
		},
		{
			name:       "note id and tags",
			markdown:   "---\nnoteId: 123\ntags:\n  - one\n  - two\n---\n\nBody",
			wantNoteID: 123,
			wantTags:   []string{"one", "two"},
		},
		{
			name:     "single string tag",
			markdown: "---\ntags: solo\n---\n\nBody",
			wantTags: []string{"solo"},
		},
		{
			name:     "deck name",
			markdown: "---\ndeckName: My Deck\n---\n\nBody",
			wantDeck: "My Deck",
		},
		{
			name:     "unterminated frontmatter is not frontmatter",
			markdown: "---\nnoteId: 1\n\nBody",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			frontmatter, err := ParseFrontmatter(test.markdown)
			if err != nil {
				t.Fatalf("ParseFrontmatter returned error: %v", err)
			}

			if frontmatter.NoteID != test.wantNoteID {
				t.Errorf("note ID = %d, want %d", frontmatter.NoteID, test.wantNoteID)
			}

			if frontmatter.DeckName != test.wantDeck {
				t.Errorf("deck name = %q, want %q", frontmatter.DeckName, test.wantDeck)
			}

			if strings.Join(frontmatter.Tags, ",") != strings.Join(test.wantTags, ",") {
				t.Errorf("tags = %v, want %v", frontmatter.Tags, test.wantTags)
			}
		})
	}
}

func TestSetNoteID(t *testing.T) {
	t.Parallel()

	noteID := int64(42)
	other := int64(99)

	tests := []struct {
		name     string
		markdown string
		noteID   *int64
		want     string
	}{
		{
			name:     "adds frontmatter when absent",
			markdown: "Front\n\n---\n\nBack",
			noteID:   &noteID,
			want:     "---\nnoteId: 42\n---\n\nFront\n\n---\n\nBack",
		},
		{
			name:     "adds note id to existing frontmatter",
			markdown: "---\ntags: [one]\n---\n\nBody",
			noteID:   &noteID,
			want:     "---\ntags: [one]\nnoteId: 42\n---\n\nBody",
		},
		{
			name:     "replaces note id",
			markdown: "---\nnoteId: 42\ntags: [one]\n---\n\nBody",
			noteID:   &other,
			want:     "---\nnoteId: 99\ntags: [one]\n---\n\nBody",
		},
		{
			name:     "removes note id and keeps other frontmatter",
			markdown: "---\nnoteId: 42\ntags: [one]\n---\n\nBody",
			noteID:   nil,
			want:     "---\ntags: [one]\n---\n\nBody",
		},
		{
			name:     "removes frontmatter when it becomes empty",
			markdown: "---\nnoteId: 42\n---\n\nBody",
			noteID:   nil,
			want:     "Body",
		},
		{
			name:     "no-op when removing absent note id",
			markdown: "---\ntags: [one]\n---\n\nBody",
			noteID:   nil,
			want:     "---\ntags: [one]\n---\n\nBody",
		},
		{
			name:     "preserves crlf-free body verbatim",
			markdown: "---\nnoteId: 42\n---\n\n# Heading\n\nParagraph with | pipes.",
			noteID:   &other,
			want:     "---\nnoteId: 99\n---\n\n# Heading\n\nParagraph with | pipes.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := SetNoteID(test.markdown, test.noteID)
			if err != nil {
				t.Fatalf("SetNoteID returned error: %v", err)
			}

			if got != test.want {
				t.Errorf("SetNoteID mismatch\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

func TestStripFrontmatter(t *testing.T) {
	t.Parallel()

	got := StripFrontmatter("---\nnoteId: 1\n---\n\nBody text")
	if got != "\nBody text" {
		t.Errorf("StripFrontmatter = %q, want %q", got, "\nBody text")
	}

	unchanged := "Body only"
	if StripFrontmatter(unchanged) != unchanged {
		t.Errorf("StripFrontmatter changed a document without frontmatter")
	}
}
