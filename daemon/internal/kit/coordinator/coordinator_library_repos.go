// The Library page's repositories: what each configured library repository holds,
// and the commands that back up, restore, install and share through them. Git
// runs in Suspend, so the lock is not held while it does; the library is reloaded
// and saved as each change lands.

package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/library"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// librarySync is a repository's last sync here: when, the commit it reached and what went wrong.
type librarySync struct {
	at     time.Time
	commit string
	err    string
}

// LibraryRepoState is one configured library repository as the Library page shows it.
type LibraryRepoState struct {
	Repository string
	Path       string
	Role       core.LibraryRole
	Publish    core.LibraryPublish
	// SyncedAt and Commit are from the last sync here; both are empty before one.
	SyncedAt *time.Time
	Commit   string
	// Problem is what is wrong with the last sync or the repository's files, or nil.
	Problem *string
	// ChangedSinceBackup is set for a backup repository: whether the library
	// differs from what the last backup or restore left.
	ChangedSinceBackup *bool
	// TakesShares is whether Share may send items to it: a shared repository other than the examples one.
	TakesShares bool
	Items       []core.LibraryItem
	Actions     []core.LibraryAction
}

// LibraryBackupResult is what a backup did: the commit it made, or nil when the repository already held the library.
type LibraryBackupResult struct {
	Commit  *string
	Changed bool
}

// LibraryShareResult is what a share did: the pull request it opened, or the commit it pushed.
type LibraryShareResult struct {
	PullRequest *string
	Commit      *string
	Changed     bool
}

// LibraryItemFile is one file an item is made of, with the repository's and this library's text (nil when absent).
type LibraryItemFile struct {
	Path  string  `json:"path"`
	Repo  *string `json:"repo"`
	Local *string `json:"local"`
}

// LibraryRepos reads every configured library repository from the clones on disk,
// with no network. Lock: exported entry point; takes c.mu.
func (c *Coordinator) LibraryRepos() []LibraryRepoState {
	c.Lock()
	defer c.EndTurn()
	return c.libraryRepoStates()
}

// libraryRepoStates is LibraryRepos, for callers with the lock.
// Lock: runs with c.mu held.
func (c *Coordinator) libraryRepoStates() []LibraryRepoState {
	states := make([]LibraryRepoState, 0, len(c.settings.LibraryRepositories))
	local := store.LibraryFilesRead(c.paths)
	for _, repo := range c.settings.LibraryRepositories {
		origin := repo.Origin()
		state := LibraryRepoState{Repository: repo.Repository, Path: repo.Path, Role: repo.Role, Publish: repo.Publish,
			TakesShares: core.LibraryShareAccepted(repo), Items: []core.LibraryItem{}}
		if sync, ok := c.libraryRepoSyncs[origin]; ok {
			at := sync.at
			state.SyncedAt = &at
			state.Commit = sync.commit
			if sync.err != "" {
				state.Problem = &sync.err
			}
		}
		clone := c.libraryRepos.Dir(repo.Repository)
		synced := libraryCloned(clone)
		repoFiles := store.LibraryFilesRead(store.Paths{Root: filepath.Join(clone, repo.Path)})
		if repo.Role == core.LibraryRoleBackup {
			changed := c.changedSinceBackup(repo)
			state.ChangedSinceBackup = &changed
		} else {
			var problems []string
			state.Items, problems = core.LibraryRepoItems(origin, repoFiles, local)
			if state.Problem == nil && len(problems) > 0 {
				text := strings.Join(problems, "\n")
				state.Problem = &text
			}
		}
		state.Actions = core.LibraryRepoActions(repo.Role, synced, len(repoFiles) > 0)
		if _, busy := c.libraryBusy[repo.Repository]; busy {
			state.Actions = libraryBusyActions(state.Actions, repo.Repository)
			for index := range state.Items {
				state.Items[index].Actions = libraryBusyActions(state.Items[index].Actions, repo.Repository)
			}
		}
		states = append(states, state)
	}
	return states
}

// changedSinceBackup reports whether the library differs from what the last backup or restore of repo
// left; with none, it always does. Lock: runs with c.mu held.
func (c *Coordinator) changedSinceBackup(repo core.LibraryRepository) bool {
	state := c.storage.LoadLibraryBackupState()
	return state == nil || state.Repository != repo.Repository || state.Path != repo.Path ||
		!assistantStringsEqual(state.Signature, c.library.Signature())
}

// hasSharedLibraryRepository is whether a library repository that takes shares is configured: a
// shared one other than the examples repository. Lock: runs with c.mu held.
func (c *Coordinator) hasSharedLibraryRepository() bool {
	for _, repo := range c.settings.LibraryRepositories {
		if core.LibraryShareAccepted(repo) {
			return true
		}
	}
	return false
}

// libraryNavigation is the navigation's library summary: whether any repository is configured, and
// whether the library changed since the backup repository. Lock: runs with c.mu held.
func (c *Coordinator) libraryNavigation() LibraryNavigation {
	nav := LibraryNavigation{Offered: len(c.settings.LibraryRepositories) > 0}
	for _, repo := range c.settings.LibraryRepositories {
		if repo.Role == core.LibraryRoleBackup {
			changed := c.changedSinceBackup(repo)
			nav.ChangedSinceBackup = &changed
			break
		}
	}
	return nav
}

// libraryCloned reports whether a clone folder exists.
func libraryCloned(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// configuredRepo is the settings entry for origin. When role is set, the entry must have that role.
// Lock: runs with c.mu held.
func (c *Coordinator) configuredRepo(origin core.LibraryOrigin, role core.LibraryRole) (core.LibraryRepository, error) {
	for _, repo := range c.settings.LibraryRepositories {
		if repo.Origin() != origin {
			continue
		}
		if role != "" && repo.Role != role {
			return repo, core.NewFailure(fmt.Sprintf("%s is not a %s library repository.", origin.Repository, role))
		}
		return repo, nil
	}
	return core.LibraryRepository{}, core.NewFailure(fmt.Sprintf("%s is not a library repository in your settings.", origin.Repository))
}

// RefreshLibraryRepo syncs one configured repository with its remote and returns how it stands. Lock:
// exported entry point; takes c.mu. Suspends: the fetch runs without the lock.
func (c *Coordinator) RefreshLibraryRepo(ctx context.Context, origin core.LibraryOrigin) (LibraryRepoState, error) {
	c.Lock()
	defer c.EndTurn()
	if err := c.refreshLibraryRepo(ctx, origin); err != nil {
		return LibraryRepoState{}, err
	}
	// Read after the command has cleared its mark, so the answer shows the repository as it now stands.
	for _, state := range c.libraryRepoStates() {
		if state.Repository == origin.Repository && state.Path == origin.Path {
			return state, nil
		}
	}
	return LibraryRepoState{}, nil
}

// refreshLibraryRepo is RefreshLibraryRepo's sync, with the repository's mark held for its duration.
// Lock: runs with c.mu held.
func (c *Coordinator) refreshLibraryRepo(ctx context.Context, origin core.LibraryOrigin) error {
	if _, err := c.configuredRepo(origin, ""); err != nil {
		return err
	}
	release, err := c.beginLibraryCommand(origin.Repository)
	if err != nil {
		return err
	}
	defer release()
	_, err = c.syncLibraryRepo(ctx, origin)
	return err
}

// beginLibraryCommand marks repository as having a library command in flight and returns the function
// that clears the mark, which the command defers. A second command on the same repository is refused
// rather than made to wait: the waiting command would need the lock that the first one holds only
// between its Suspends, and the wait could not end. Lock: runs with c.mu held.
func (c *Coordinator) beginLibraryCommand(repository string) (func(), error) {
	if _, busy := c.libraryBusy[repository]; busy {
		return nil, core.NewFailure(libraryBusyReason(repository))
	}
	c.libraryBusy[repository] = struct{}{}
	c.libraryRepoRevision++
	return func() {
		delete(c.libraryBusy, repository)
		c.libraryRepoRevision++
	}, nil
}

// libraryBusyReason is why a repository's actions are unavailable while one of its commands runs.
func libraryBusyReason(repository string) string {
	return "A library command on " + repository + " is still running."
}

// libraryBusyActions disables each action with the reason a command on the repository is running.
func libraryBusyActions(actions []core.LibraryAction, repository string) []core.LibraryAction {
	out := make([]core.LibraryAction, 0, len(actions))
	for _, action := range actions {
		reason := libraryBusyReason(repository)
		out = append(out, core.LibraryAction{ID: action.ID, Enabled: false, Reason: &reason})
	}
	return out
}

// syncLibraryRepo brings a repository's clone to its default branch and records the sync. Suspends.
// Lock: runs with c.mu held; the fetch runs without it.
func (c *Coordinator) syncLibraryRepo(ctx context.Context, origin core.LibraryOrigin) (library.Clone, error) {
	var clone library.Clone
	var err error
	c.Suspend(func() { clone, err = c.libraryRepos.Sync(ctx, origin.Repository) })
	sync := librarySync{at: c.clock.Now(), commit: clone.Commit}
	if err != nil {
		sync.err = err.Error()
	}
	c.libraryRepoSyncs[origin] = sync
	c.libraryRepoRevision++
	return clone, err
}

// BackupLibrary writes the library's belts and agents into a backup repository and pushes them to its
// default branch, then records the backup. Lock: exported entry point; takes c.mu. Suspends: git runs
// without the lock.
func (c *Coordinator) BackupLibrary(ctx context.Context, origin core.LibraryOrigin) (LibraryBackupResult, error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleBackup); err != nil {
		return LibraryBackupResult{}, err
	}
	release, err := c.beginLibraryCommand(origin.Repository)
	if err != nil {
		return LibraryBackupResult{}, err
	}
	defer release()
	clone, err := c.syncLibraryRepo(ctx, origin)
	if err != nil {
		return LibraryBackupResult{}, err
	}
	backup := core.LibraryBackupFiles(store.LibraryFilesRead(c.paths))
	var commit string
	var changed bool
	c.Suspend(func() {
		err = library.Mirror(filepath.Join(clone.Dir, origin.Path), backup)
		if err == nil {
			commit, changed, err = c.libraryRepos.Commit(ctx, clone.Dir, "Back up Beltline library")
		}
		if err == nil && changed {
			err = c.libraryRepos.Push(ctx, clone.Dir, clone.Branch)
		}
		// Whatever did not reach origin is taken back, so the Library page reads the repository as it stands.
		if resetErr := c.libraryRepos.Reset(ctx, clone.Dir, clone.Branch); err == nil {
			err = resetErr
		}
	})
	if err != nil {
		return LibraryBackupResult{}, err
	}
	if !changed {
		commit = clone.Commit
	}
	c.recordBackup(origin, commit)
	result := LibraryBackupResult{Changed: changed}
	if changed {
		result.Commit = &commit
	}
	return result, nil
}

// recordBackup saves what the library is now as the last backup or restore of origin.
// Lock: runs with c.mu held.
func (c *Coordinator) recordBackup(origin core.LibraryOrigin, commit string) {
	_ = c.storage.SaveLibraryBackupState(store.LibraryBackupState{
		Repository: origin.Repository, Path: origin.Path, Commit: commit, Signature: c.library.Signature(),
	})
	c.libraryRepoRevision++
}

// RestoreLibrary moves the library's belts and agents into library-backups/<time>/ and writes the backup
// repository's in their place, then reloads the library. Runs keep their own copies. It returns the
// backup folder, relative to the data folder. Lock: exported entry point; takes c.mu. Suspends: git runs
// without the lock.
func (c *Coordinator) RestoreLibrary(ctx context.Context, origin core.LibraryOrigin) (string, error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleBackup); err != nil {
		return "", err
	}
	release, err := c.beginLibraryCommand(origin.Repository)
	if err != nil {
		return "", err
	}
	defer release()
	clone, err := c.syncLibraryRepo(ctx, origin)
	if err != nil {
		return "", err
	}
	backup := core.LibraryBackupFiles(store.LibraryFilesRead(store.Paths{Root: filepath.Join(clone.Dir, origin.Path)}))
	if len(backup) == 0 {
		return "", core.NewFailure(fmt.Sprintf("%s holds no belts or agents to restore.", origin.Repository))
	}
	var files []core.LibraryFile
	for path, data := range backup {
		files = append(files, core.LibraryFile{Path: path, Data: data})
	}
	c.library.reconcileFiles()
	// The library is reloaded whatever the restore did, so the Library page reads what is on disk now.
	defer c.reloadLibraryFromDisk()
	saved, err := c.storage.LibraryRestore(c.clock.Now().Format("2006-01-02T15-04-05"), files)
	if err != nil {
		return "", err
	}
	c.recordBackup(origin, clone.Commit)
	return saved, nil
}

// reloadLibraryFromDisk reads the belts and agents again after a write of the library's own files,
// and reconsiders the triggers the change touched. Lock: runs with c.mu held.
func (c *Coordinator) reloadLibraryFromDisk() {
	c.library.reload(false)
	c.intake.lastPoll = map[string]time.Time{}
}

// InstallLibraryItem writes an item of a shared repository into the library: its latest version, with
// the agent versions it pins that the library lacks. A belt new to the library is installed paused: a
// shared belt names no repositories, so you choose them before you resume it. An update keeps the belt's
// state. It returns the paths written. Lock: exported entry point; takes c.mu. Suspends: the fetch runs
// without the lock.
func (c *Coordinator) InstallLibraryItem(ctx context.Context, origin core.LibraryOrigin, kind, id string) ([]string, error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleShared); err != nil {
		return nil, err
	}
	release, err := c.beginLibraryCommand(origin.Repository)
	if err != nil {
		return nil, err
	}
	defer release()
	clone, err := c.syncLibraryRepo(ctx, origin)
	if err != nil {
		return nil, err
	}
	repoFiles := store.LibraryFilesRead(store.Paths{Root: filepath.Join(clone.Dir, origin.Path)})
	c.library.reconcileFiles()
	files, err := core.LibraryInstallFiles(origin, kind, id, repoFiles, store.LibraryFilesRead(c.paths))
	if err != nil {
		return nil, err
	}
	isNewBelt := kind == "belt" && c.library.Belt(id) == nil
	if err := c.storage.LibraryWriteFiles(files); err != nil {
		return nil, err
	}
	if isNewBelt && len(files) > 0 {
		if err := c.storage.SetPaused(id, true); err != nil {
			return nil, err
		}
	}
	c.reloadLibraryFromDisk()
	c.libraryRepoRevision++
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths, nil
}

// ShareLibraryItem sends an item of this library to a shared repository: on a pull request from its own
// branch, or straight to the default branch, as the repository's publish setting says. Lock: exported
// entry point; takes c.mu. Suspends: git and gh run without the lock.
func (c *Coordinator) ShareLibraryItem(ctx context.Context, origin core.LibraryOrigin, kind, id string) (LibraryShareResult, error) {
	c.Lock()
	defer c.EndTurn()
	repo, err := c.configuredRepo(origin, core.LibraryRoleShared)
	if err != nil {
		return LibraryShareResult{}, err
	}
	// A local remote has no pull requests, and gh must not be asked about one (demo mode never calls GitHub).
	if !core.LibraryShareAccepted(repo) {
		return LibraryShareResult{}, core.NewFailure(core.LibraryShareRefusedExamples)
	}
	if repo.Publish == core.LibraryPublishPullRequest && c.libraryRepos.Local() {
		return LibraryShareResult{}, core.NewFailure("Pull requests aren't available for a local repository; set it to push.")
	}
	release, err := c.beginLibraryCommand(origin.Repository)
	if err != nil {
		return LibraryShareResult{}, err
	}
	defer release()
	clone, err := c.syncLibraryRepo(ctx, origin)
	if err != nil {
		return LibraryShareResult{}, err
	}
	files, err := core.LibraryShareFiles(origin, kind, id, store.LibraryFilesRead(c.paths), store.LibraryFilesRead(store.Paths{Root: filepath.Join(clone.Dir, origin.Path)}))
	if err != nil {
		return LibraryShareResult{}, err
	}
	if len(files) == 0 {
		return LibraryShareResult{}, nil
	}
	version := c.libraryItemVersion(kind, id)
	title := core.LibraryShareTitle(kind, id, version)
	branch := core.LibraryShareBranch(kind, id, version)
	var result LibraryShareResult
	c.Suspend(func() { result, err = c.publishShare(ctx, clone, origin, repo, files, title, branch) })
	if err != nil {
		return LibraryShareResult{}, err
	}
	c.libraryRepoRevision++
	return result, nil
}

// publishShare writes files into the clone and commits them, on branch when the repository takes pull
// requests, then pushes them: to branch with a pull request, or to the default branch. However it ends,
// the clone goes back to the default branch as origin has it, so the Library page reads the repository
// and not the share. Lock: none; it runs in Suspend.
func (c *Coordinator) publishShare(ctx context.Context, clone library.Clone, origin core.LibraryOrigin, repo core.LibraryRepository, files []core.LibraryFile, title, branch string) (result LibraryShareResult, err error) {
	defer func() {
		if resetErr := c.libraryRepos.Reset(ctx, clone.Dir, clone.Branch); err == nil && resetErr != nil {
			result, err = LibraryShareResult{}, resetErr
		}
	}()
	dir := filepath.Join(clone.Dir, origin.Path)
	pullRequest := repo.Publish == core.LibraryPublishPullRequest
	if pullRequest {
		if err := c.libraryRepos.Branch(ctx, clone.Dir, branch); err != nil {
			return LibraryShareResult{}, err
		}
	}
	if err := library.Write(dir, files); err != nil {
		return LibraryShareResult{}, err
	}
	commit, changed, err := c.libraryRepos.Commit(ctx, clone.Dir, title)
	if err != nil || !changed {
		return LibraryShareResult{}, err
	}
	result = LibraryShareResult{Commit: &commit, Changed: true}
	if !pullRequest {
		if err := c.libraryRepos.Push(ctx, clone.Dir, clone.Branch); err != nil {
			return LibraryShareResult{}, err
		}
		return result, nil
	}
	if err := c.libraryRepos.PushBranch(ctx, clone.Dir, branch); err != nil {
		return LibraryShareResult{}, err
	}
	url, err := c.libraryRepos.OpenPullRequest(ctx, origin.Repository, branch, clone.Branch, title, "Shared from Beltline.")
	if err != nil {
		return LibraryShareResult{}, err
	}
	result.PullRequest = &url
	return result, nil
}

// libraryItemVersion is the saved version of an item: the belt's or the agent's latest.
// Lock: runs with c.mu held.
func (c *Coordinator) libraryItemVersion(kind, id string) int {
	if kind == "agent" {
		if record := c.library.LatestAgent(id); record != nil {
			return record.Agent.Version
		}
		return 0
	}
	if record := c.library.Belt(id); record != nil {
		return record.Belt.Version
	}
	return 0
}

// LibraryItemCompare is the files of an item in a shared repository beside this library's copies of
// them, for the compare sheet. Lock: exported entry point; takes c.mu.
func (c *Coordinator) LibraryItemCompare(origin core.LibraryOrigin, kind, id string) ([]LibraryItemFile, error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleShared); err != nil {
		return nil, err
	}
	repoFiles := store.LibraryFilesRead(store.Paths{Root: filepath.Join(c.libraryRepos.Dir(origin.Repository), origin.Path)})
	local := store.LibraryFilesRead(c.paths)
	var files []LibraryItemFile
	for _, path := range core.LibraryItemPaths(kind, id, repoFiles) {
		file := LibraryItemFile{Path: path}
		if text, ok := repoFiles[path]; ok {
			s := string(text)
			file.Repo = &s
		}
		if text, ok := local[path]; ok {
			s := string(text)
			file.Local = &s
		}
		files = append(files, file)
	}
	return files, nil
}
