// The library repositories' wire types: the Library page's repositories and their
// items, and the answers of the backup, restore, install and share commands.

package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// LibraryRepoRequest names one configured library repository.
type LibraryRepoRequest struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
}

// Origin is the repository the request names.
func (r LibraryRepoRequest) Origin() core.LibraryOrigin {
	return core.LibraryOrigin{Repository: r.Repository, Path: r.Path}
}

// LibraryItemRequest names one belt or agent of a shared repository.
type LibraryItemRequest struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	ID         string `json:"id"`
}

// Origin is the repository the request names.
func (r LibraryItemRequest) Origin() core.LibraryOrigin {
	return core.LibraryOrigin{Repository: r.Repository, Path: r.Path}
}

// LibraryReposResponse is the body of GET /api/v1/library/repos.
type LibraryReposResponse struct {
	Repos []LibraryRepoResponse `json:"repos"`
}

// LibraryRepoResponse is one configured library repository. Items are what a shared
// repository holds (empty for a backup one); changedSinceBackup is set for a backup
// repository only. takesShares says whether Share offers it. syncedAt, commit and problem
// are null until there is something to say.
type LibraryRepoResponse struct {
	Repository         string               `json:"repository"`
	Path               string               `json:"path"`
	Role               core.LibraryRole     `json:"role"`
	Publish            core.LibraryPublish  `json:"publish"`
	SyncedAt           *jsonx.Date          `json:"syncedAt"`
	Commit             *string              `json:"commit"`
	Problem            *string              `json:"problem"`
	ChangedSinceBackup *bool                `json:"changedSinceBackup,omitzero"`
	TakesShares        bool                 `json:"takesShares"`
	Items              []core.LibraryItem   `json:"items"`
	Actions            []core.LibraryAction `json:"actions"`
}

// NewLibraryRepoResponse maps one repository's state from the coordinator.
func NewLibraryRepoResponse(state coordinator.LibraryRepoState) LibraryRepoResponse {
	response := LibraryRepoResponse{
		Repository:         state.Repository,
		Path:               state.Path,
		Role:               state.Role,
		Publish:            state.Publish,
		Problem:            core.CopyPointer(state.Problem),
		ChangedSinceBackup: core.CopyPointer(state.ChangedSinceBackup),
		TakesShares:        state.TakesShares,
		Items:              append([]core.LibraryItem{}, state.Items...),
		Actions:            append([]core.LibraryAction{}, state.Actions...),
	}
	if state.SyncedAt != nil {
		response.SyncedAt = &jsonx.Date{Time: *state.SyncedAt}
	}
	if state.Commit != "" {
		commit := state.Commit
		response.Commit = &commit
	}
	return response
}

// NewLibraryReposResponse maps every configured repository.
func NewLibraryReposResponse(states []coordinator.LibraryRepoState) LibraryReposResponse {
	repos := make([]LibraryRepoResponse, 0, len(states))
	for _, state := range states {
		repos = append(repos, NewLibraryRepoResponse(state))
	}
	return LibraryReposResponse{Repos: repos}
}

// LibraryBackupResponse is what a backup did: its commit, or null when nothing changed.
type LibraryBackupResponse struct {
	Commit  *string `json:"commit"`
	Changed bool    `json:"changed"`
}

// LibraryRestoreResponse names the folder under the data folder the replaced library went to.
type LibraryRestoreResponse struct {
	SavedTo string `json:"savedTo"`
}

// LibraryInstallResponse lists the files an install wrote, relative to the data folder.
type LibraryInstallResponse struct {
	Written []string `json:"written"`
}

// LibraryShareResponse is what a share did: the pull request it opened, or null, and the commit.
type LibraryShareResponse struct {
	PullRequest *string `json:"pullRequest"`
	Commit      *string `json:"commit"`
	Changed     bool    `json:"changed"`
}

// LibraryItemResponse is the item's files beside this library's copies, for the compare sheet.
type LibraryItemResponse struct {
	Files []coordinator.LibraryItemFile `json:"files"`
}

// NewLibraryItemResponse is the item's files.
func NewLibraryItemResponse(files []coordinator.LibraryItemFile) LibraryItemResponse {
	return LibraryItemResponse{Files: files}
}
