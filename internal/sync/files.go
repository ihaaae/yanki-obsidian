package sync

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/ihaaae/yanki/internal/anki"
	"github.com/ihaaae/yanki/internal/markdown"
	"github.com/ihaaae/yanki/internal/note"
	"github.com/ihaaae/yanki/internal/vault"
)

// FileOptions configures a file-backed sync.
type FileOptions struct {
	Namespace        string
	StrictLineBreaks bool
	DryRun           bool
	CheckDatabase    bool
	PushToAnkiWeb    bool
	StrictMatching   bool
}

// SyncedFile pairs a sync action with the file it came from. Deleted notes
// have no file.
type SyncedFile struct {
	Action           Action
	FilePath         string
	FilePathOriginal string
	Note             note.Note
}

// FilesResult summarizes a file-backed sync.
type FilesResult struct {
	AnkiWeb         bool
	DeletedDecks    []string
	DeletedMedia    []string
	DryRun          bool
	Duration        time.Duration
	FixedDatabase   bool
	Namespace       string
	ReuploadedMedia []string
	Synced          []SyncedFile
}

// Files syncs the given Markdown files to Anki, inferring decks from their
// locations and recording new note IDs in each file's frontmatter.
func Files(ctx context.Context, client *anki.Client, filePaths []string, options FileOptions) (FilesResult, error) {
	start := time.Now()

	sorted := append([]string{}, filePaths...)
	sort.Strings(sorted)

	deckNames := vault.DeckNamesFromFilePaths(sorted)

	locals := make([]note.Note, 0, len(sorted))
	markdowns := make([]string, 0, len(sorted))

	for index, filePath := range sorted {
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return FilesResult{}, fmt.Errorf("read %s: %w", filePath, err)
		}

		parsed, err := markdown.Parse(string(contents), markdown.Options{
			Namespace:        options.Namespace,
			StrictLineBreaks: options.StrictLineBreaks,
		})
		if err != nil {
			return FilesResult{}, fmt.Errorf("parse %s: %w", filePath, err)
		}

		if parsed.DeckName == "" && index < len(deckNames) {
			parsed.DeckName = deckNames[index]
		}

		locals = append(locals, parsed)
		markdowns = append(markdowns, string(contents))
	}

	result, err := Notes(ctx, client, locals, Options{
		CheckDatabase:  options.CheckDatabase,
		DryRun:         options.DryRun,
		Namespace:      options.Namespace,
		PushToAnkiWeb:  options.PushToAnkiWeb,
		StrictMatching: options.StrictMatching,
	})
	if err != nil {
		return FilesResult{}, err
	}

	syncedFiles := make([]SyncedFile, 0, len(result.Synced))
	liveIndex := 0

	for _, entry := range result.Synced {
		if entry.Action == ActionDeleted {
			syncedFiles = append(syncedFiles, SyncedFile{Action: ActionDeleted, Note: entry.Note})
			continue
		}

		if liveIndex >= len(sorted) {
			break
		}

		filePath := sorted[liveIndex]
		original := locals[liveIndex]
		liveIndex++

		if entry.Action != ActionAnkiUnreachable && !options.DryRun && entry.Note.NoteID != 0 && entry.Note.NoteID != original.NoteID {
			noteID := entry.Note.NoteID

			updated, err := note.SetNoteID(markdowns[liveIndex-1], &noteID)
			if err != nil {
				return FilesResult{}, fmt.Errorf("update frontmatter for %s: %w", filePath, err)
			}

			if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
				return FilesResult{}, fmt.Errorf("write %s: %w", filePath, err)
			}
		}

		syncedFiles = append(syncedFiles, SyncedFile{
			Action:           entry.Action,
			FilePath:         filePath,
			FilePathOriginal: filePath,
			Note:             entry.Note,
		})
	}

	sort.SliceStable(syncedFiles, func(i, j int) bool {
		return syncedFiles[i].FilePath < syncedFiles[j].FilePath
	})

	return FilesResult{
		AnkiWeb:         result.AnkiWeb,
		DeletedDecks:    result.DeletedDecks,
		DeletedMedia:    result.DeletedMedia,
		DryRun:          result.DryRun,
		Duration:        time.Since(start),
		FixedDatabase:   result.FixedDatabase,
		Namespace:       result.Namespace,
		ReuploadedMedia: result.ReuploadedMedia,
		Synced:          syncedFiles,
	}, nil
}
