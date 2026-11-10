// The data folder's side of library repositories: the record of the last backup
// or restore, and the belt and agent files a restore moves aside and writes.

package store

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// LibraryBackupState is what the last backup or restore left: the repository, its
// path, the commit it reached and the library's signature at that point.
type LibraryBackupState struct {
	Repository string   `json:"repository"`
	Path       string   `json:"path"`
	Commit     string   `json:"commit"`
	Signature  []string `json:"signature"`
}

// LoadLibraryBackupState reads backup-state.json, or nil when there is none.
func (s Storage) LoadLibraryBackupState() *LibraryBackupState {
	var state LibraryBackupState
	if s.Read(s.libraryBackupStatePath(), &state) != nil {
		return nil
	}
	return &state
}

// SaveLibraryBackupState writes backup-state.json.
func (s Storage) SaveLibraryBackupState(state LibraryBackupState) error {
	return s.Write(state, s.libraryBackupStatePath())
}

func (s Storage) libraryBackupStatePath() string {
	return filepath.Join(s.Paths.LibraryRepos(), "backup-state.json")
}

// LibraryRestore replaces the belt and agent version files under the data folder with files. The files
// it replaces move into library-backups/<stamp>/, keeping their paths, and that folder is returned
// relative to the data folder. A restore that fails puts the library back as it was (see
// libraryPutBack). State files stay where they are.
func (s Storage) LibraryRestore(stamp string, files []core.LibraryFile) (string, error) {
	saved := filepath.Join("library-backups", stamp)
	var moved []string
	for _, path := range slices.Sorted(maps.Keys(core.LibraryBackupFiles(LibraryFilesRead(s.Paths)))) {
		target := filepath.Join(s.Paths.Root, saved, path)
		err := os.MkdirAll(filepath.Dir(target), 0o777)
		if err == nil {
			err = os.Rename(filepath.Join(s.Paths.Root, path), target)
		}
		if err != nil {
			return "", s.libraryPutBack(saved, moved, nil, err)
		}
		moved = append(moved, path)
	}
	if err := s.LibraryWriteFiles(files); err != nil {
		return "", s.libraryPutBack(saved, moved, files, err)
	}
	return saved, nil
}

// libraryPutBack undoes a restore that failed with cause: the files it wrote are removed, the files it
// moved aside return to their places, and the backup folder goes when nothing is left in it. It returns
// cause, joined with anything that stopped the undo.
func (s Storage) libraryPutBack(saved string, moved []string, written []core.LibraryFile, cause error) error {
	errs := []error{cause}
	for _, file := range written {
		target := filepath.Join(s.Paths.Root, file.Path)
		if _, err := os.Lstat(target); err != nil {
			// Never written, so there is nothing to remove.
			continue
		}
		if err := os.Remove(target); err != nil {
			errs = append(errs, err)
		}
	}
	for _, path := range moved {
		target := filepath.Join(s.Paths.Root, path)
		err := os.MkdirAll(filepath.Dir(target), 0o777)
		if err == nil {
			err = os.Rename(filepath.Join(s.Paths.Root, saved, path), target)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	for _, path := range moved {
		for dir := filepath.Dir(path); dir != "."; dir = filepath.Dir(dir) {
			// A folder that still holds a file is kept, and so is every folder above it.
			_ = os.Remove(filepath.Join(s.Paths.Root, saved, dir))
		}
	}
	_ = os.Remove(filepath.Join(s.Paths.Root, saved))
	return errors.Join(errs...)
}

// LibraryWriteFiles writes library files, by their paths under the data folder, atomically, and only
// inside it (WriteFileAtomicIn).
func (s Storage) LibraryWriteFiles(files []core.LibraryFile) error {
	root, err := os.OpenRoot(s.Paths.Root)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, file := range files {
		if err := WriteFileAtomicIn(root, file.Path, file.Data); err != nil {
			return err
		}
	}
	return nil
}
