// Package vault discovers Markdown notes on disk and maps their locations to
// Anki decks.
package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ignoredDirectoryNames are never walked, even though they are not hidden.
var ignoredDirectoryNames = map[string]bool{"node_modules": true}

// Directory describes a directory of Markdown notes to sync.
type Directory struct {
	// RootPath is the absolute path to the synced directory.
	RootPath string
	// Name is the directory's base name.
	Name string
	// AllFilePaths holds every non-hidden file, used for link resolution.
	AllFilePaths []string
	// StrictLineBreaks treats single newlines as line breaks when true.
	StrictLineBreaks bool
}

// Resolve inspects a directory and returns its note-sync metadata.
func Resolve(directoryPath string) (Directory, error) {
	rootPath, err := filepath.Abs(directoryPath)
	if err != nil {
		return Directory{}, fmt.Errorf("resolve %s: %w", directoryPath, err)
	}

	info, err := os.Stat(rootPath)
	if err != nil {
		return Directory{}, fmt.Errorf("not a directory: %s", rootPath)
	}

	if !info.IsDir() {
		return Directory{}, fmt.Errorf("not a directory: %s", rootPath)
	}

	allFilePaths, err := listFiles(rootPath)
	if err != nil {
		return Directory{}, err
	}

	return Directory{
		RootPath:         rootPath,
		Name:             filepath.Base(rootPath),
		AllFilePaths:     allFilePaths,
		StrictLineBreaks: true,
	}, nil
}

// listFiles walks a directory tree, skipping hidden entries and ignored
// directories.
func listFiles(rootPath string) ([]string, error) {
	var filePaths []string

	err := filepath.WalkDir(rootPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == rootPath {
			return nil
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") || ignoredDirectoryNames[name] {
			if entry.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if entry.IsDir() {
			return nil
		}

		filePaths = append(filePaths, path)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", rootPath, err)
	}

	sort.Strings(filePaths)

	return filePaths, nil
}

// FindNotePaths selects the Markdown notes inside the given folders, relative
// to the root. An empty folder list means the whole directory. Folder notes
// (a note sharing its parent folder's name) are skipped when ignoreFolderNotes
// is set.
func (d Directory) FindNotePaths(folders []string, ignoreFolderNotes bool) []string {
	folderPaths := make([]string, 0, len(folders))
	for _, folder := range folders {
		folderPaths = append(folderPaths, resolveFolderPath(d.RootPath, folder))
	}

	var notePaths []string

	for _, filePath := range d.AllFilePaths {
		if strings.ToLower(filepath.Ext(filePath)) != ".md" {
			continue
		}

		if !underAnyFolder(filePath, folderPaths) {
			continue
		}

		if ignoreFolderNotes && isFolderNote(filePath) {
			continue
		}

		notePaths = append(notePaths, filePath)
	}

	sort.Strings(notePaths)

	return notePaths
}

// underAnyFolder reports whether a file sits inside one of the folder paths.
// An empty folder list matches every file.
func underAnyFolder(filePath string, folderPaths []string) bool {
	if len(folderPaths) == 0 {
		return true
	}

	for _, folderPath := range folderPaths {
		if strings.HasPrefix(filePath, folderPath+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// isFolderNote reports whether a file shares its parent directory's name.
func isFolderNote(filePath string) bool {
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))

	return filepath.Base(filepath.Dir(filePath)) == base
}

// resolveFolderPath turns a settings folder (which may use forward slashes and
// `.`) into an absolute path. Empty, `.`, and `/` all mean the root.
func resolveFolderPath(rootPath, folder string) string {
	normalized := strings.ReplaceAll(folder, "\\", "/")

	var segments []string
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." {
			continue
		}

		segments = append(segments, segment)
	}

	if len(segments) == 0 {
		return rootPath
	}

	return filepath.Join(rootPath, filepath.Join(segments...))
}
