package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeckNamesFromFilePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  []string
	}{
		{
			name: "folder with its own notes keeps the common folder",
			paths: []string{
				"/vault/Anki/Animals/Biped/Plato.md",
				"/vault/Anki/Animals/Quadruped/Horse.md",
				"/vault/Anki/Animals/ignore.md",
				"/vault/Anki/Non-living things/Rock.md",
			},
			want: []string{
				"Animals::Biped",
				"Animals::Quadruped",
				"Animals",
				"Non-living things",
			},
		},
		{
			name: "common folder without its own notes is dropped",
			paths: []string{
				"/vault/Anki/Animals/Biped/Plato.md",
				"/vault/Anki/Animals/Quadruped/Horse.md",
			},
			want: []string{"Biped", "Quadruped"},
		},
		{
			name:  "single note in the base path uses the base directory name",
			paths: []string{"/vault/Note.md"},
			want:  []string{"vault"},
		},
		{
			name:  "empty input",
			paths: nil,
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := DeckNamesFromFilePaths(test.paths)
			if len(got) != len(test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}

			for index := range got {
				if got[index] != test.want[index] {
					t.Errorf("deck[%d] = %q, want %q", index, got[index], test.want[index])
				}
			}
		})
	}
}

func TestResolveAndFindNotePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	write := func(relative, contents string) {
		t.Helper()

		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("Anki/Animals/Plato.md", "Front\n\n---\n\nBack")
	write("Anki/Animals/Animals.md", "Folder note")
	write("Anki/Animals/nested/Horse.md", "Front\n\n---\n\nBack")
	write("Anki/notes.txt", "not markdown")
	write("Outside.md", "Front\n\n---\n\nBack")
	write(".hidden/Secret.md", "Front\n\n---\n\nBack")

	directory, err := Resolve(root)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	paths := directory.FindNotePaths([]string{"Anki"}, true)
	relative := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}

		relative = append(relative, filepath.ToSlash(rel))
	}

	want := []string{"Anki/Animals/Plato.md", "Anki/Animals/nested/Horse.md"}
	if len(relative) != len(want) {
		t.Fatalf("FindNotePaths = %v, want %v", relative, want)
	}

	for index := range want {
		if relative[index] != want[index] {
			t.Errorf("note[%d] = %q, want %q", index, relative[index], want[index])
		}
	}

	// Without the folder-note filter, the folder note is included.
	all := directory.FindNotePaths([]string{"Anki"}, false)
	if len(all) != 3 {
		t.Errorf("FindNotePaths without the filter = %d notes, want 3", len(all))
	}

	// An empty folder list means the whole directory.
	everything := directory.FindNotePaths(nil, true)
	if len(everything) != 3 {
		t.Errorf("FindNotePaths with no folders = %d notes, want 3", len(everything))
	}
}
