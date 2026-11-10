// Tests that a library repository's links never lead Beltline outside its clone: Write and Mirror
// neither write nor remove through them, the reads leave them out, and a clone checks them out as
// plain files.

package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// outsideFile is a file outside any clone, holding text, and its path.
func outsideFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v1.md")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// symlink makes a link at path, under clone, pointing at target.
func symlink(t *testing.T, clone, path, target string) {
	t.Helper()
	link := filepath.Join(clone, path)
	if err := os.MkdirAll(filepath.Dir(link), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// textOf is the text of the file at path.
func textOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWriteReplacesALinkedFileRatherThanWritingThroughIt(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	symlink(t, clone, "agents/probe/v1.md", outside)
	if err := Write(clone, "", []core.LibraryFile{{Path: "agents/probe/v1.md", Data: []byte("shared")}}); err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, outside); got != "private" {
		t.Errorf("outside file = %q, want it untouched", got)
	}
	info, err := os.Lstat(filepath.Join(clone, "agents/probe/v1.md"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the link was not replaced by a file: %v %v", info, err)
	}
}

func TestWriteRefusesAFolderLinkThatLeadsOutOfTheClone(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	symlink(t, clone, "agents/probe", filepath.Dir(outside))
	err := Write(clone, "", []core.LibraryFile{{Path: "agents/probe/v1.md", Data: []byte("shared")}})
	if err == nil {
		t.Fatal("Write through a folder link out of the clone succeeded")
	}
	if got := textOf(t, outside); got != "private" {
		t.Errorf("outside file = %q, want it untouched", got)
	}
}

func TestWriteRefusesASubfolderThatLeadsOutOfTheClone(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	symlink(t, clone, "library", filepath.Dir(outside))
	if err := Write(clone, "library", []core.LibraryFile{{Path: "agents/probe/v1.md", Data: []byte("shared")}}); err == nil {
		t.Fatal("Write into a subfolder that is a link out of the clone succeeded")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(outside), "agents")); !os.IsNotExist(err) {
		t.Errorf("Write made folders outside the clone: %v", err)
	}
}

func TestMirrorNeverRemovesThroughALink(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	symlink(t, clone, "agents/probe/v1.md", outside)
	writeAll(t, clone, map[string]string{"agents/old/v1.md": "old"})
	if err := Mirror(clone, "", map[string][]byte{"belts/b/v1.json": []byte("{}")}); err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, outside); got != "private" {
		t.Errorf("outside file = %q, want it untouched", got)
	}
	if _, err := os.Stat(filepath.Join(clone, "agents/old/v1.md")); !os.IsNotExist(err) {
		t.Errorf("the stale file was kept: %v", err)
	}
	if got := textOf(t, filepath.Join(clone, "belts/b/v1.json")); got != "{}" {
		t.Errorf("mirrored file = %q", got)
	}
}

func TestMirrorRefusesAFolderLinkThatLeadsOutOfTheClone(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	symlink(t, clone, "agents/probe", filepath.Dir(outside))
	// The link is not followed for the listing, so the file outside is neither read nor removed.
	if err := Mirror(clone, "", map[string][]byte{}); err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, outside); got != "private" {
		t.Errorf("outside file = %q, want it untouched", got)
	}
}

func TestRepoFilesReadLeavesLinksOut(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	clone := t.TempDir()
	writeAll(t, clone, map[string]string{"library/agents/kept/v1.md": "kept"})
	symlink(t, clone, "library/agents/file/v1.md", outside)
	symlink(t, clone, "library/agents/folder", filepath.Dir(outside))
	symlink(t, clone, "elsewhere", filepath.Dir(outside))
	files := store.LibraryRepoFilesRead(clone, "library")
	if len(files) != 1 || string(files["agents/kept/v1.md"]) != "kept" {
		t.Errorf("read %v, want only agents/kept/v1.md", files)
	}
	if files := store.LibraryRepoFilesRead(clone, "elsewhere"); len(files) != 0 {
		t.Errorf("a subfolder link out of the clone read %v", files)
	}
}

func TestAClonedLinkIsAPlainFile(t *testing.T) {
	t.Parallel()
	outside := outsideFile(t, "private")
	remote := newRemote(t, map[string]string{"agents/kept/v1.md": "kept"})
	seed := filepath.Join(filepath.Dir(remote), "seed")
	symlink(t, seed, "agents/probe/v1.md", outside)
	shellIn(t, seed, "add", "-A")
	shellIn(t, seed, "commit", "-q", "-m", "link")
	shellIn(t, seed, "push", "-q", "origin", "HEAD:main")
	repos := reposFor(t, "acme/library", remote, shell.ProcessRunner{})
	clone, err := repos.Sync(context.Background(), "acme/library")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(clone.Dir, "agents/probe/v1.md"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the link was checked out as %v (%v), want a plain file", info, err)
	}
	if got := textOf(t, filepath.Join(clone.Dir, "agents/probe/v1.md")); got != outside {
		t.Errorf("the plain file holds %q, want the link's target", got)
	}
}
