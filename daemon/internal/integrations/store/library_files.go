// The library's own files, belts/<id>/… and agents/<id>/…, keyed by that path.

package store

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// libraryKind is one library folder and the extension of its version files.
type libraryKind struct {
	kind   string
	folder string
	ext    string
}

// libraryKinds are the two library folders the configuration assistant reads.
func libraryKinds(paths Paths) []libraryKind {
	return []libraryKind{
		{kind: "belts", folder: paths.Belts(), ext: "json"},
		{kind: "agents", folder: paths.Agents(), ext: "md"},
	}
}

// LibraryFilesRead reads every library file under the data root, keyed by its
// path relative to the root. Unreadable files are absent, and so is anything
// LibraryFilesReadIn leaves out.
func LibraryFilesRead(paths Paths) map[string][]byte {
	root, err := os.OpenRoot(paths.Root)
	if err != nil {
		return map[string][]byte{}
	}
	defer func() { _ = root.Close() }()
	return LibraryFilesReadIn(root)
}

// LibraryRepoFilesRead reads the library files in the folder sub of a library repository's clone, keyed
// by their paths under sub. The clone is someone else's content, so the read stays inside it: a link,
// sub itself included, that leads out of the clone is not followed. A sub that is missing or leads out
// reads as an empty library.
func LibraryRepoFilesRead(clone, sub string) map[string][]byte {
	root, err := os.OpenRoot(clone)
	if err != nil {
		return map[string][]byte{}
	}
	defer func() { _ = root.Close() }()
	folder, err := root.OpenRoot(filepath.Clean(sub))
	if err != nil {
		return map[string][]byte{}
	}
	defer func() { _ = folder.Close() }()
	return LibraryFilesReadIn(folder)
}

// LibraryFilesReadIn reads every library file in root, keyed by its path in root. Only regular files are
// read: a library file that is a link is left out, wherever it points, and a folder link that leads
// out of root is not followed. Unreadable files are absent.
func LibraryFilesReadIn(root *os.Root) map[string][]byte {
	files := map[string][]byte{}
	for _, path := range libraryFilesListIn(root) {
		if data, ok := libraryFileRead(root, path); ok {
			files[path] = data
		}
	}
	return files
}

// libraryFileRead reads the regular file at path in root. A link, a folder or anything else that is not a
// regular file is not read.
func libraryFileRead(root *os.Root, path string) ([]byte, bool) {
	if info, err := root.Lstat(path); err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = file.Close() }()
	// The file could have been swapped for a link since the Lstat; root still keeps it inside, and a
	// folder or device in its place is not read.
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false
	}
	return data, true
}

// WriteFileAtomicIn is WriteFileAtomic at path in root: the temporary file and the rename stay inside
// root, a folder on the way that leads out of it is an error, and a link at path is replaced, not
// written through.
func WriteFileAtomicIn(root *os.Root, path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := root.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	temp := filepath.Join(dir, "."+filepath.Base(path)+"."+core.NewUUID()+".tmp")
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = root.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(temp)
		return err
	}
	if err := root.Rename(temp, path); err != nil {
		_ = root.Remove(temp)
		return err
	}
	return nil
}

// LibraryFilesChanged lists, sorted, the paths that are new or changed in
// after. The returned set is sorted.
func LibraryFilesChanged(before, after map[string]time.Time) []string {
	changed := []string{}
	for path, stamp := range after {
		previous, ok := before[path]
		if !ok || !previous.Equal(stamp) {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

// libraryFilesList is libraryFilesListIn over the data root.
func libraryFilesList(paths Paths) []string {
	root, err := os.OpenRoot(paths.Root)
	if err != nil {
		return []string{}
	}
	defer func() { _ = root.Close() }()
	return libraryFilesListIn(root)
}

// libraryFilesListIn is the library's paths in root, sorted: belts/<id>/state.json and
// belts/<id>/v<n>.json, agents/<id>/state.json and agents/<id>/v<n>.md. Hidden
// folders are skipped, and a folder link that leads out of root lists nothing.
func libraryFilesListIn(root *os.Root) []string {
	result := []string{}
	for _, lib := range libraryKinds(Paths{}) {
		for _, id := range rootFolderNames(root, lib.kind) {
			if strings.HasPrefix(id, ".") {
				continue
			}
			for _, name := range rootFolderNames(root, lib.kind+"/"+id) {
				if name == "state.json" || (strings.HasPrefix(name, "v") && strings.HasSuffix(name, "."+lib.ext)) {
					result = append(result, lib.kind+"/"+id+"/"+name)
				}
			}
		}
	}
	sort.Strings(result)
	return result
}

// rootFolderNames lists the entry names of the folder dir in root, sorted. A missing or unreadable
// folder, or one that leads out of root, is empty.
func rootFolderNames(root *os.Root, dir string) []string {
	folder, err := root.Open(dir)
	if err != nil {
		return nil
	}
	defer func() { _ = folder.Close() }()
	names, err := folder.Readdirnames(-1)
	if err != nil {
		return nil
	}
	sort.Strings(names)
	return names
}

// subfolderNames lists the entry names of dir, sorted. A missing or unreadable directory is empty.
func subfolderNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}
