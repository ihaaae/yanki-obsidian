package vault

import (
	"path/filepath"
	"strings"
)

// DeckNamesFromFilePaths infers an Anki deck name for each file from its
// location. Deck names are `::`-delimited and relative to the longest common
// ancestor of the files, so a folder hierarchy becomes a nested deck
// hierarchy.
//
// The common ancestor itself becomes part of the deck names only when a file
// lives directly inside it. This keeps a top-level folder from being dropped
// when it contains notes of its own, while not prefixing every deck with a
// directory that holds only subdirectories.
func DeckNamesFromFilePaths(absoluteFilePaths []string) []string {
	if len(absoluteFilePaths) == 0 {
		return nil
	}

	segmentLists := make([][]string, len(absoluteFilePaths))
	for index, filePath := range absoluteFilePaths {
		segmentLists[index] = strings.Split(filepath.ToSlash(filepath.Dir(filePath)), "/")
	}

	common := segmentLists[0]
	for _, segments := range segmentLists[1:] {
		limit := len(common)
		if len(segments) < limit {
			limit = len(segments)
		}

		shared := 0
		for shared < limit && common[shared] == segments[shared] {
			shared++
		}

		common = common[:shared]
	}

	offset := 0
	if len(common) > 0 {
		lastSegment := common[len(common)-1]
		for _, segments := range segmentLists {
			if len(segments) > 0 && segments[len(segments)-1] == lastSegment {
				offset = 1
				break
			}
		}
	}

	names := make([]string, len(segmentLists))
	for index, segments := range segmentLists {
		start := len(common) - offset
		if start < 0 {
			start = 0
		}

		if start > len(segments) {
			start = len(segments)
		}

		names[index] = strings.Join(segments[start:], "::")
	}

	return names
}
