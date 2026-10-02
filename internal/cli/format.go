package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ihaaae/yanki/internal/sync"
)

// formatSyncResult renders a sync result for humans.
func formatSyncResult(result sync.FilesResult, verbose bool) string {
	counts := make(map[sync.Action]int)
	totalSynced := 0
	totalRenamed := 0

	for _, entry := range result.Synced {
		counts[entry.Action]++

		if entry.Action != sync.ActionDeleted {
			totalSynced++
		}

		if entry.FilePath != entry.FilePathOriginal {
			totalRenamed++
		}
	}

	lines := []string{
		fmt.Sprintf(
			"%s %d %s to Anki%s.",
			headlineVerb(result),
			totalSynced,
			pluralize("note", totalSynced),
			durationSuffix(result),
		),
	}

	if !verbose {
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "", summaryTitle(result))

	for _, action := range sortedActions(counts) {
		lines = append(lines, fmt.Sprintf("  %s: %d", capitalize(string(action)), counts[action]))
	}

	if totalRenamed > 0 {
		lines = append(lines, "", fmt.Sprintf("Local notes renamed: %d", totalRenamed))
	}

	if len(result.DeletedDecks) > 0 {
		lines = append(lines, "", fmt.Sprintf("Decks pruned: %d", len(result.DeletedDecks)))
	}

	lines = append(lines, "", detailTitle(result))

	for _, entry := range result.Synced {
		if entry.FilePath == "" {
			lines = append(lines, fmt.Sprintf("  Note ID %d %s (From Anki)", entry.Note.NoteID, capitalize(string(entry.Action))))
			continue
		}

		lines = append(lines, fmt.Sprintf("  Note ID %d %s %s", entry.Note.NoteID, capitalize(string(entry.Action)), entry.FilePath))
	}

	return strings.Join(lines, "\n")
}

func headlineVerb(result sync.FilesResult) string {
	switch {
	case result.DryRun:
		return "Will sync"
	case hasAction(result, sync.ActionAnkiUnreachable):
		return "Failed to sync"
	default:
		return "Successfully synced"
	}
}

func durationSuffix(result sync.FilesResult) string {
	if result.DryRun {
		return ""
	}

	return " in " + result.Duration.Round(time.Millisecond).String()
}

func summaryTitle(result sync.FilesResult) string {
	if result.DryRun {
		return "Sync Plan Summary:"
	}

	return "Sync Summary:"
}

func detailTitle(result sync.FilesResult) string {
	if result.DryRun {
		return "Sync Plan Details:"
	}

	return "Sync Details:"
}

func hasAction(result sync.FilesResult, action sync.Action) bool {
	for _, entry := range result.Synced {
		if entry.Action == action {
			return true
		}
	}

	return false
}

func sortedActions(counts map[sync.Action]int) []sync.Action {
	actions := make([]sync.Action, 0, len(counts))
	for action := range counts {
		actions = append(actions, action)
	}

	sort.Slice(actions, func(i, j int) bool {
		return actions[i] < actions[j]
	})

	return actions
}

func capitalize(value string) string {
	if value == "" {
		return value
	}

	return strings.ToUpper(value[:1]) + value[1:]
}

func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}

	return word + "s"
}
