// Package library keeps a local clone of each library repository under the data
// folder. Reads and writes go through git; a pull request goes through gh.
package library

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// Repos is the set of clones under Root, run through Runner.
type Repos struct {
	Root   string
	Runner shell.Runner
	// URL, when set, gives a repository's remote, and the clone is made with git
	// rather than gh. Tests use it to clone local bare repositories.
	URL func(repository string) string
}

// Clone is a repository's clone after a sync: its folder, its default branch
// and the commit it is on, which is empty for a repository with no commits yet.
type Clone struct {
	Dir    string
	Branch string
	Commit string
}

// Dir is the clone folder of repository: owner__repo under Root.
func (r Repos) Dir(repository string) string {
	return filepath.Join(r.Root, strings.ReplaceAll(repository, "/", "__"))
}

// Sync brings the clone of repository to its default branch, cloning it first
// when there is none.
func (r Repos) Sync(ctx context.Context, repository string) (Clone, error) {
	dir := r.Dir(repository)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := r.clone(ctx, repository, dir); err != nil {
			return Clone{}, err
		}
	}
	if err := r.git(ctx, dir, "fetch", "-q", "origin"); err != nil {
		return Clone{}, err
	}
	branch, err := r.defaultBranch(ctx, dir)
	if err != nil {
		return Clone{}, err
	}
	if err := r.Reset(ctx, dir, branch); err != nil {
		return Clone{}, err
	}
	clone := Clone{Dir: dir, Branch: branch}
	head, err := r.probe(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return Clone{}, err
	}
	if head.OK() {
		clone.Commit = strings.TrimSpace(head.Stdout)
	}
	return clone, nil
}

// clone makes the clone of repository in dir: with gh, so the user's sign-in is
// used, unless URL names the remote.
func (r Repos) clone(ctx context.Context, repository, dir string) error {
	if err := os.MkdirAll(r.Root, 0o777); err != nil {
		return err
	}
	if r.URL != nil {
		return r.git(ctx, r.Root, "clone", "-q", "-c", noSymlinks, r.URL(repository), dir)
	}
	result := shell.Bounded(ctx, r.Runner, "gh", []string{"repo", "clone", repository, dir, "--", "-c", noSymlinks}, "", nil)
	if !result.OK() {
		return core.NewFailure(fmt.Sprintf("Could not clone %s: %s", repository, result.MessageFor("gh repo clone")))
	}
	return nil
}

// defaultBranch is the branch origin's HEAD names, or main when it names none. A
// read that hit its limit is an error: main would be a guess.
func (r Repos) defaultBranch(ctx context.Context, dir string) (string, error) {
	result, err := r.probe(ctx, dir, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	if name := strings.TrimPrefix(strings.TrimSpace(result.Stdout), "origin/"); result.OK() && name != "" {
		return name, nil
	}
	return "main", nil
}

// Reset puts the clone on branch as origin has it, and leaves no change in the folder: the commits and
// files of a share or backup that never reached origin are taken back, and so are untracked files. A
// share or backup runs it when it ends, so the Library page shows the repository and not the change.
// Lock: none; it runs in Suspend.
func (r Repos) Reset(ctx context.Context, dir, branch string) error {
	verify, err := r.probe(ctx, dir, "rev-parse", "--verify", "-q", "origin/"+branch)
	if err != nil {
		return err
	}
	if verify.OK() {
		if err := r.git(ctx, dir, "checkout", "-q", "-f", "-B", branch, "origin/"+branch); err != nil {
			return err
		}
	} else if err := r.emptyBranch(ctx, dir, branch); err != nil {
		return err
	}
	return r.git(ctx, dir, "clean", "-q", "-f", "-d")
}

// emptyBranch points the clone at branch with no commits, for a remote that has none yet. A branch the
// clone holds already, with commits that never reached origin, is dropped, so HEAD is unborn.
func (r Repos) emptyBranch(ctx context.Context, dir, branch string) error {
	if err := r.git(ctx, dir, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
		return err
	}
	// A fresh clone has no such branch to delete, and the delete has nothing to undo then.
	if _, err := r.probe(ctx, dir, "update-ref", "-d", "refs/heads/"+branch); err != nil {
		return err
	}
	return r.git(ctx, dir, "rm", "-r", "-q", "--cached", "--ignore-unmatch", ".")
}

// Local reports whether the repositories are local remotes (URL names them). Demo mode's are, and they
// have no pull requests, so nothing is opened for them.
func (r Repos) Local() bool { return r.URL != nil }

// Commit stages every change in dir and commits it with message. changed is false,
// and nothing is committed, when the folder matches its commit.
func (r Repos) Commit(ctx context.Context, dir, message string) (commit string, changed bool, err error) {
	if err := r.git(ctx, dir, "add", "-A"); err != nil {
		return "", false, err
	}
	staged := r.run(ctx, dir, "diff", "--cached", "--quiet")
	if staged.Status == 0 {
		return "", false, nil
	}
	if staged.Status != 1 {
		return "", false, core.NewFailure("Could not read the staged changes: " + staged.MessageFor("git diff"))
	}
	identity, err := r.probe(ctx, dir, "config", "user.email")
	if err != nil {
		return "", false, err
	}
	args := []string{"commit", "-q", "-m", message}
	if !identity.OK() {
		args = append([]string{"-c", "user.name=Beltline", "-c", "user.email=beltline@localhost"}, args...)
	}
	if err := r.git(ctx, dir, args...); err != nil {
		return "", false, err
	}
	head, err := r.probe(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", true, err
	}
	return strings.TrimSpace(head.Stdout), true, nil
}

// Branch makes branch at the clone's commit and checks it out.
func (r Repos) Branch(ctx context.Context, dir, branch string) error {
	return r.git(ctx, dir, "checkout", "-q", "-B", branch)
}

// Push sends the clone's commit to branch on origin. It never overwrites a branch.
func (r Repos) Push(ctx context.Context, dir, branch string) error {
	return r.git(ctx, dir, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
}

// PushBranch sends the clone's commit to a share branch on origin, replacing what an earlier share
// pushed there: the branch is Beltline's own, and a newer share of the item supersedes it.
func (r Repos) PushBranch(ctx context.Context, dir, branch string) error {
	return r.git(ctx, dir, "push", "-q", "--force", "-u", "origin", "HEAD:refs/heads/"+branch)
}

// OpenPullRequest opens a pull request from branch into base in repository, and
// returns its URL. When one is already open from branch, it returns that one's URL.
func (r Repos) OpenPullRequest(ctx context.Context, repository, branch, base, title, body string) (string, error) {
	args := []string{"pr", "create", "--repo", repository, "--head", branch, "--base", base, "--title", title, "--body", body}
	result := shell.Bounded(ctx, r.Runner, "gh", args, "", nil)
	// A create that was stopped may have printed "already exists" first; it is not that answer.
	if !result.OK() && !result.IsStopped() && strings.Contains(result.Stderr+result.Stdout, "already exists") {
		view := shell.Bounded(ctx, r.Runner, "gh", []string{"pr", "view", branch, "--repo", repository, "--json", "url", "-q", ".url"}, "", nil)
		// A lookup that was stopped is no answer either, so it fails rather than falling through to create's.
		if view.IsStopped() {
			return "", core.NewFailure(fmt.Sprintf("Could not open a pull request in %s: %s", repository, view.MessageFor("gh pr view")))
		}
		if view.OK() && strings.TrimSpace(view.Stdout) != "" {
			return strings.TrimSpace(view.Stdout), nil
		}
	}
	if !result.OK() {
		return "", core.NewFailure(fmt.Sprintf("Could not open a pull request in %s: %s", repository, result.MessageFor("gh pr create")))
	}
	lines := strings.Fields(result.Stdout)
	if len(lines) == 0 {
		return "", core.NewFailure("gh printed no pull request URL for " + repository)
	}
	return lines[len(lines)-1], nil
}

// noSymlinks is the git setting that checks a link in a clone out as a plain file holding its target, so
// no link in someone else's repository points anywhere on this machine. It is set when a clone is made
// and on every command in one, which covers clones made before it.
const noSymlinks = "core.symlinks=false"

// run runs git with args in dir, bounded as git commands are.
func (r Repos) run(ctx context.Context, dir string, args ...string) shell.Result {
	return shell.Bounded(ctx, r.Runner, "git", append([]string{"-C", dir, "-c", noSymlinks}, args...), "", nil)
}

// probe runs git with args in dir for a read whose failure is an answer, such as a
// missing branch. A run that was stopped gave no answer, so it is an error.
func (r Repos) probe(ctx context.Context, dir string, args ...string) (shell.Result, error) {
	result := r.run(ctx, dir, args...)
	if result.IsStopped() {
		return result, gitFailure(args, result)
	}
	return result, nil
}

// git runs git with args in dir and fails with its message.
func (r Repos) git(ctx context.Context, dir string, args ...string) error {
	result := r.run(ctx, dir, args...)
	if !result.OK() {
		return gitFailure(args, result)
	}
	return nil
}

// gitFailure is the failure for a git command that did not succeed, naming its subcommand.
func gitFailure(args []string, result shell.Result) error {
	return core.NewFailure(fmt.Sprintf("git %s failed: %s", shell.GitSubcommand(args), result.Message()))
}

// Write puts files, by their paths under the folder sub of the clone, into the clone. A clone holds
// someone else's content, so the writes stay inside it: a folder on the way, sub included, that is a
// link leading out of the clone is an error, and a file that is a link is replaced, not written through.
func Write(clone, sub string, files []core.LibraryFile) error {
	folder, err := openLibraryFolder(clone, sub)
	if err != nil {
		return err
	}
	defer func() { _ = folder.Close() }()
	return writeIn(folder, files)
}

// Mirror makes the library files in the folder sub of the clone (its belts and agents, keyed by their
// paths) hold exactly files: it writes each one and removes the others, staying inside the clone as
// Write does. Only regular files are removed; a link is not a library file and is left alone.
func Mirror(clone, sub string, files map[string][]byte) error {
	folder, err := openLibraryFolder(clone, sub)
	if err != nil {
		return err
	}
	defer func() { _ = folder.Close() }()
	var list []core.LibraryFile
	for _, path := range slices.Sorted(maps.Keys(files)) {
		list = append(list, core.LibraryFile{Path: path, Data: files[path]})
	}
	if err := writeIn(folder, list); err != nil {
		return err
	}
	for path := range store.LibraryFilesReadIn(folder) {
		if _, keep := files[path]; !keep {
			if err := folder.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// openLibraryFolder opens the folder sub of the clone as a root of its own, making it when it is
// missing. A sub that leads out of the clone, through a link, is an error.
func openLibraryFolder(clone, sub string) (*os.Root, error) {
	root, err := os.OpenRoot(clone)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	sub = filepath.Clean(sub)
	if err := root.MkdirAll(sub, 0o777); err != nil {
		return nil, err
	}
	return root.OpenRoot(sub)
}

// writeIn writes each file atomically at its path in folder.
func writeIn(folder *os.Root, files []core.LibraryFile) error {
	for _, file := range files {
		if err := store.WriteFileAtomicIn(folder, file.Path, file.Data); err != nil {
			return err
		}
	}
	return nil
}
