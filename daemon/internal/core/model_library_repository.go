// Library repositories are git repositories laid out like the library folder
// (belts/<id>/v<n>.json and agents/<id>/v<n>.md, optionally under a path). A
// backup repository is your own library; a shared one is a team's.

package core

import (
	"fmt"
	"path"
	"strings"
)

// LibraryRole is what a library repository is for.
type LibraryRole string

const (
	// LibraryRoleBackup is your own library, backed up to and restored from the repository.
	LibraryRoleBackup LibraryRole = "backup"
	// LibraryRoleShared is a team's library: items are installed from it and shared to it.
	LibraryRoleShared LibraryRole = "shared"
)

// LibraryPublish is how a share reaches a shared repository.
type LibraryPublish string

const (
	// LibraryPublishPullRequest opens a pull request from a branch.
	LibraryPublishPullRequest LibraryPublish = "pullRequest"
	// LibraryPublishPush commits to the default branch.
	LibraryPublishPush LibraryPublish = "push"
)

// Known reports whether x is one of the cases of LibraryRole.
func (x LibraryRole) Known() bool { return x == LibraryRoleBackup || x == LibraryRoleShared }

// Known reports whether x is one of the cases of LibraryPublish.
func (x LibraryPublish) Known() bool {
	return x == LibraryPublishPullRequest || x == LibraryPublishPush
}

// LibraryRepository is one entry of Settings.libraryRepositories.
type LibraryRepository struct {
	Repository string         `json:"repository"`
	Path       string         `json:"path"`
	Role       LibraryRole    `json:"role"`
	Publish    LibraryPublish `json:"publish"`
}

// LibraryExamplesRepository is the shared library repository of example belts and
// agents. A new data folder lists it, and the upgrade adds it once to an older
// one's settings; nothing is installed from it until you choose to. The app only
// reads it: no share goes to it, whatever its entry says (LibraryShareAccepted).
const LibraryExamplesRepository = "robbertmanders/beltline-examples"

// LibraryExamples is the examples repository's settings entry: shared, at the
// repository's root, and shared to by pull request.
func LibraryExamples() LibraryRepository {
	return LibraryRepository{Repository: LibraryExamplesRepository, Role: LibraryRoleShared, Publish: LibraryPublishPullRequest}
}

// LibraryShareAccepted reports whether a share may go to repo: a shared repository
// other than the examples one, which the app never writes to.
func LibraryShareAccepted(repo LibraryRepository) bool {
	return repo.Role == LibraryRoleShared && repo.Repository != LibraryExamplesRepository
}

// LibraryShareRefusedExamples is why a share to the examples repository is refused.
const LibraryShareRefusedExamples = LibraryExamplesRepository + " only offers examples to install; share to your own shared repository."

// LibraryOrigin is the repository and path an installed belt or agent came from.
// The same value keys a repository in settings.
type LibraryOrigin struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
}

// Origin is the repository's origin.
func (r LibraryRepository) Origin() LibraryOrigin {
	return LibraryOrigin{Repository: r.Repository, Path: r.Path}
}

// LibraryAction is one action a library resource offers, enabled or explained.
type LibraryAction struct {
	ID      string  `json:"id"`
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason,omitzero"`
}

// libraryAction is an action that is enabled, or disabled for reason.
func libraryAction(id string, enabled bool, reason string) LibraryAction {
	action := LibraryAction{ID: id, Enabled: enabled}
	if !enabled {
		action.Reason = &reason
	}
	return action
}

// LibraryRepositoriesProblems are the problems with a settings list: a repository
// that is not owner/repo, a path that leaves the repository, an unknown role or
// publish, or the same repository and path listed twice.
func LibraryRepositoriesProblems(repos []LibraryRepository) []string {
	var problems []string
	seen := map[LibraryOrigin]bool{}
	for _, repo := range repos {
		if !libraryRepositoryNameValid(repo.Repository) {
			problems = append(problems, fmt.Sprintf("%q must be owner/repo.", repo.Repository))
		}
		if !libraryPathValid(repo.Path) {
			problems = append(problems, fmt.Sprintf("%s: the path %q must be a folder inside the repository.", repo.Repository, repo.Path))
		}
		if repo.Role != LibraryRoleBackup && repo.Role != LibraryRoleShared {
			problems = append(problems, fmt.Sprintf("%s: the role %q must be backup or shared.", repo.Repository, repo.Role))
		}
		if repo.Publish != LibraryPublishPullRequest && repo.Publish != LibraryPublishPush {
			problems = append(problems, fmt.Sprintf("%s: the publish %q must be pullRequest or push.", repo.Repository, repo.Publish))
		}
		if seen[repo.Origin()] {
			problems = append(problems, fmt.Sprintf("%s %q is listed twice.", repo.Repository, repo.Path))
		}
		seen[repo.Origin()] = true
	}
	return problems
}

// libraryRepositoryNameValid reports whether name is owner/repo with GitHub's characters.
func libraryRepositoryNameValid(name string) bool {
	owner, repo, found := strings.Cut(name, "/")
	if !found || strings.Contains(repo, "/") {
		return false
	}
	return libraryNamePartValid(owner) && libraryNamePartValid(repo)
}

// libraryNameCharacters are the characters a GitHub owner or repository name may use.
const libraryNameCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_."

// libraryNamePartValid reports whether one half of owner/repo is a name GitHub allows.
func libraryNamePartValid(part string) bool {
	if part == "" || part == "." || part == ".." {
		return false
	}
	for _, c := range part {
		if !strings.ContainsRune(libraryNameCharacters, c) {
			return false
		}
	}
	return true
}

// libraryPathValid reports whether a repository path is empty or a clean relative folder.
func libraryPathValid(p string) bool {
	if p == "" {
		return true
	}
	return path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "../")
}

// LibraryRepoActions are the actions a repository offers. Backup and restore are
// for a backup repository only; restore needs something to restore, and a clone
// to have read it: synced says whether one has been read, holdsItems whether it
// holds any belt or agent.
func LibraryRepoActions(role LibraryRole, synced, holdsItems bool) []LibraryAction {
	actions := []LibraryAction{libraryAction("refresh", true, "")}
	if role != LibraryRoleBackup {
		return actions
	}
	restoreReason := ""
	switch {
	case !synced:
		restoreReason = "Refresh the repository first."
	case !holdsItems:
		restoreReason = "The repository holds no belts or agents to restore."
	}
	return append(actions,
		libraryAction("backup", true, ""),
		libraryAction("restore", restoreReason == "", restoreReason))
}
