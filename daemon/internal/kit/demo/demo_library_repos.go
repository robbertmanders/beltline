// The demo's library repositories: two local git repositories under the demo folder,
// so the Library page works offline. The shared one holds a team copy of the nightly
// belt with the agents it pins, under new ids, so nothing in it is in the library
// yet; the backup one holds the demo's own library.

package demo

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/library"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// The demo's two library repositories, by owner/repo.
const (
	demoSharedRepository = "example/shared"
	demoBackupRepository = "example/backup"
)

// demoTeamPrefix is what the shared repository's ids start with, so they are new to the library.
const demoTeamPrefix = "team-"

// libraryRepoURL is the local bare repository a demo library repository is.
func libraryRepoURL(root, repository string) string {
	return filepath.Join(root, "demo-library", strings.ReplaceAll(repository, "/", "__")+".git")
}

// seedLibraryRepos makes the demo's two repositories under root. saved is the demo's belts as saved.
func seedLibraryRepos(root string, saved map[string]core.Belt) error {
	paths := store.Paths{Root: root}
	shared, err := demoSharedFiles(paths, saved)
	if err != nil {
		return err
	}
	if err := demoRepoCommit(root, demoSharedRepository, shared); err != nil {
		return err
	}
	return demoRepoCommit(root, demoBackupRepository, core.LibraryBackupFiles(store.LibraryFilesRead(paths)))
}

// demoSharedFiles are the team copy: the nightly belt under a new id, and each agent version it
// pins, under new ids too.
func demoSharedFiles(paths store.Paths, saved map[string]core.Belt) (map[string][]byte, error) {
	// A copy through its encoding, so the pins changed below leave the saved belt alone.
	encoded, err := jsonx.MarshalStorage(saved["nightly"])
	if err != nil {
		return nil, err
	}
	var belt core.Belt
	if err := jsonx.Unmarshal(encoded, &belt); err != nil {
		return nil, err
	}
	belt.ID = demoTeamPrefix + belt.ID
	belt.Name = "Nightly (team)"
	files := map[string][]byte{}
	for i := range belt.Stages {
		ref := belt.Stages[i].Agent
		if ref == nil {
			continue
		}
		text, err := os.ReadFile(filepath.Join(paths.Agents(), ref.ID, "v"+strconv.Itoa(ref.Version)+".md"))
		if err != nil {
			return nil, err
		}
		agent, err := core.AgentFileParse(string(text), ref.ID, ref.Version)
		if err != nil {
			return nil, err
		}
		agent.ID = demoTeamPrefix + ref.ID
		ref.ID = agent.ID
		files["agents/"+agent.ID+"/v"+strconv.Itoa(agent.Version)+".md"] = []byte(core.AgentFileWrite(agent))
	}
	belt.Version = 1
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		return nil, err
	}
	files["belts/"+belt.ID+"/v1.json"] = data
	return files, nil
}

// demoGitDates fixes the dates of the demo's commits, so seeding the same demo writes the same repositories.
var demoGitDates = []string{"GIT_AUTHOR_DATE=2026-10-15T10:00:00Z", "GIT_COMMITTER_DATE=2026-10-15T10:00:00Z"}

// demoRepoCommit makes the bare repository of repository holding files as its one commit. The
// working copy it is built in is removed after the push.
func demoRepoCommit(root, repository string, files map[string][]byte) error {
	ctx := context.Background()
	sh := shell.ProcessRunner{}
	remote := libraryRepoURL(root, repository)
	work := filepath.Join(root, "demo-library", "work", strings.ReplaceAll(repository, "/", "__"))
	// The working copy is scratch: a leftover one is harmless, so its removal error is dropped.
	defer func() { _ = os.RemoveAll(work) }()
	if err := os.MkdirAll(work, 0o777); err != nil {
		return err
	}
	// git runs through env, which sets the dates before it starts git.
	git := func(args ...string) error {
		result := sh.Run(ctx, "env", append(append([]string{}, demoGitDates...), append([]string{"git"}, args...)...), "", nil)
		if !result.OK() {
			return core.NewFailure(result.Message())
		}
		return nil
	}
	if err := git("init", "-q", "--bare", "-b", "main", remote); err != nil {
		return err
	}
	if err := git("init", "-q", "-b", "main", work); err != nil {
		return err
	}
	if err := library.Mirror(work, "", files); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"-C", work, "add", "-A"},
		{"-C", work, "-c", "user.name=Beltline", "-c", "user.email=demo@beltline.local", "commit", "-q", "-m", "Demo library"},
		{"-C", work, "remote", "add", "origin", remote},
		{"-C", work, "push", "-q", "origin", "HEAD:refs/heads/main"},
	} {
		if err := git(args...); err != nil {
			return err
		}
	}
	return nil
}
