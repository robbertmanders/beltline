// The library repositories' contract cases: the Library page's reads and the backup,
// restore, refresh, install and share commands, served against the contract demo's local
// repositories. Commit ids differ with the clock of git, so the goldens name them as <COMMIT>.
package servertest

import (
	"regexp"
	"testing"
)

// The library area's contract cases.
var libraryCases = []string{"libraryRepos", "libraryItem", "libraryBrowser", "libraryBelt", "libraryAgent", "libraryRefresh", "libraryBackup", "libraryRestore", "libraryInstall", "libraryShare"}

// libraryShared and libraryBackup name the contract demo's two repositories, as request bodies.
const (
	libraryShared = `{"repository":"example/shared","path":""}`
	libraryBackup = `{"repository":"example/backup","path":""}`
)

// commitIDPattern is a full git commit id, which the goldens do not hold.
var commitIDPattern = regexp.MustCompile(`[0-9a-f]{40}`)

func init() {
	registerArea(libraryCases, exerciseLibrary)
}

// libraryFixture starts a contract fixture whose shared repository has been synced once, so the
// Library page has items to show.
func libraryFixture(t testing.TB) *Fixture {
	t.Helper()
	f := Start(t, Options{})
	if reply := f.Do(Request{Method: "POST", Path: "/api/v1/library/refresh", Token: ContractToken, Body: []byte(libraryShared)}); reply.Status != 200 {
		t.Fatalf("library: refresh answered %d: %s", reply.Status, reply.Body)
	}
	return f
}

// withoutCommitIDs is reply with each commit id replaced by <COMMIT>.
func withoutCommitIDs(reply Reply) Reply {
	reply.Body = commitIDPattern.ReplaceAll(reply.Body, []byte("<COMMIT>"))
	return reply
}

// exerciseLibrary serves one library case and checks its golden; it reports whether the case is the library area's.
func exerciseLibrary(f *Fixture, t testing.TB, contractCase string) bool {
	switch contractCase {
	case "libraryRepos":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/library/repos", Token: ContractToken})
		CheckGolden(t, "library repos", "library-repos.json", withoutCommitIDs(reply), g.Root)
	case "libraryItem":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "GET", Path: libraryItemPath, Token: ContractToken})
		CheckGolden(t, "library item", "library-item.json", reply, g.Root)
	case "libraryBrowser":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/library/browser?kind=belt", Token: ContractToken})
		CheckGolden(t, "library browser", "library-browser.json", reply, g.Root)
	case "libraryBelt":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "GET", Path: libraryBeltPath, Token: ContractToken})
		CheckGolden(t, "library belt", "library-belt.json", reply, g.Root)
	case "libraryAgent":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "GET", Path: libraryAgentPath, Token: ContractToken})
		CheckGolden(t, "library agent", "library-agent.json", reply, g.Root)
	case "libraryRefresh":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "POST", Path: "/api/v1/library/refresh", Token: ContractToken, Body: []byte(libraryShared)})
		CheckGolden(t, "library refresh", "library-refresh.json", withoutCommitIDs(reply), g.Root)
	case "libraryBackup":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "POST", Path: "/api/v1/library/backup", Token: ContractToken, Body: []byte(libraryBackup)})
		CheckGolden(t, "library backup", "library-backup.json", withoutCommitIDs(reply), g.Root)
	case "libraryRestore":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "POST", Path: "/api/v1/library/restore", Token: ContractToken, Body: []byte(libraryBackup)})
		CheckGolden(t, "library restore", "library-restore.json", withoutCommitIDs(reply), g.Root)
	case "libraryInstall":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "POST", Path: "/api/v1/library/install", Token: ContractToken, Body: []byte(libraryInstallBody)})
		CheckGolden(t, "library install", "library-install.json", withoutCommitIDs(reply), g.Root)
		// The shared belt names no repositories, so a belt new to the library lands paused.
		if record := g.Coordinator.ReadBelt("team-nightly"); record == nil || !record.Paused {
			t.Fatalf("library install: the installed belt is %+v, want it paused", record)
		}
	case "libraryShare":
		g := libraryFixture(t)
		reply := g.Do(Request{Method: "POST", Path: "/api/v1/library/share", Token: ContractToken, Body: []byte(libraryShareBody)})
		CheckGolden(t, "library share", "library-share.json", withoutCommitIDs(reply), g.Root)
	default:
		return false
	}
	return true
}

// libraryItemPath asks for the compare files of the team belt in the shared repository.
const libraryItemPath = "/api/v1/library/item?repository=example%2Fshared&path=&kind=belt&id=team-nightly"

// libraryBeltPath asks for the team belt of the shared repository, with its first stage's form.
const libraryBeltPath = "/api/v1/library/belt?repository=example%2Fshared&path=&id=team-nightly&stage=build"

// libraryAgentPath asks for an agent of the shared repository.
const libraryAgentPath = "/api/v1/library/agent?repository=example%2Fshared&path=&id=team-build"

// libraryInstallBody installs the team belt from the shared repository.
const libraryInstallBody = `{"repository":"example/shared","path":"","kind":"belt","id":"team-nightly"}`

// libraryShareBody shares the demo's nightly belt to the shared repository.
const libraryShareBody = `{"repository":"example/shared","path":"","kind":"belt","id":"nightly"}`
