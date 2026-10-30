package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// libraryTestOrigin is the repository the tests install from.
var libraryTestOrigin = LibraryOrigin{Repository: "owner/shared", Path: ""}

// libraryTestRepo is a repository's library files: a belt "team" at version 2 that pins the built-in
// agents, and every built-in agent at its version, all from origin unless origin is nil.
func libraryTestRepo(t *testing.T, version int, origin *LibraryOrigin) map[string][]byte {
	t.Helper()
	belt := SampleBelt([]string{"a/b"}, "agent", SampleAgents(), true)
	belt.ID, belt.Version, belt.Name, belt.Origin = "team", version, "Team", origin
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{libraryVersionPath("belts", "team", version, "json"): data}
	for _, agent := range SampleAgents() {
		agent.Origin = origin
		files[libraryVersionPath("agents", agent.ID, agent.Version, "md")] = []byte(AgentFileWrite(agent))
	}
	return files
}

// libraryTestLocal is this library's files for the repository's content: the same agents, and the team
// belt at version with origin as given (no belt when version is 0).
func libraryTestLocal(t *testing.T, repo map[string][]byte, version int, origin *LibraryOrigin) map[string][]byte {
	t.Helper()
	local := map[string][]byte{}
	for path, data := range repo {
		if strings.HasPrefix(path, "agents/") {
			local[path] = data
		}
	}
	if version > 0 {
		var belt Belt
		if err := jsonx.Unmarshal(repo["belts/team/v2.json"], &belt); err != nil {
			t.Fatal(err)
		}
		belt.Version, belt.Origin = version, origin
		data, err := jsonx.MarshalStorage(belt)
		if err != nil {
			t.Fatal(err)
		}
		local[libraryVersionPath("belts", "team", version, "json")] = data
	}
	return local
}

// libraryTestState is the state of the team belt in items.
func libraryTestState(t *testing.T, items []LibraryItem, kind, id string) LibraryItem {
	t.Helper()
	item, ok := libraryFind(items, kind, id)
	if !ok {
		t.Fatalf("no %s %s in %+v", kind, id, items)
	}
	return item
}

func TestLibraryRepositoriesProblemsNameEachFault(t *testing.T) {
	t.Parallel()
	if problems := LibraryRepositoriesProblems([]LibraryRepository{
		{Repository: "owner/shared", Role: LibraryRoleShared, Publish: LibraryPublishPullRequest},
		{Repository: "owner/backup", Path: "team/library", Role: LibraryRoleBackup, Publish: LibraryPublishPush},
	}); len(problems) != 0 {
		t.Fatalf("valid list has problems %v", problems)
	}
	for _, bad := range []LibraryRepository{
		{Repository: "one-word", Role: LibraryRoleShared, Publish: LibraryPublishPullRequest},
		{Repository: "owner/shared", Path: "../escape", Role: LibraryRoleShared, Publish: LibraryPublishPullRequest},
		{Repository: "owner/shared", Path: "/absolute", Role: LibraryRoleShared, Publish: LibraryPublishPullRequest},
		{Repository: "owner/shared", Role: "mirror", Publish: LibraryPublishPullRequest},
		{Repository: "owner/shared", Role: LibraryRoleShared, Publish: "merge"},
	} {
		if problems := LibraryRepositoriesProblems([]LibraryRepository{bad}); len(problems) != 1 {
			t.Errorf("%+v: problems %v, want one", bad, problems)
		}
	}
	twice := LibraryRepository{Repository: "owner/shared", Role: LibraryRoleShared, Publish: LibraryPublishPush}
	if problems := LibraryRepositoriesProblems([]LibraryRepository{twice, twice}); len(problems) != 1 {
		t.Errorf("a repeated repository: problems %v, want one", problems)
	}
}

func TestLibraryRepoItemsReadTheStateAgainstTheLibrary(t *testing.T) {
	t.Parallel()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	cases := []struct {
		name    string
		local   map[string][]byte
		state   LibraryItemState
		reason  string
		localV  *int
		changed bool
	}{
		{name: "nothing local", local: map[string][]byte{}, state: LibraryItemAvailable},
		{name: "older from the repository", local: libraryTestLocal(t, repo, 1, &libraryTestOrigin), state: LibraryItemUpdate},
		{name: "the same version from the repository", local: libraryTestLocal(t, repo, 2, &libraryTestOrigin), state: LibraryItemInstalled},
		{name: "no origin, the repository's content", local: libraryTestLocal(t, repo, 2, nil), state: LibraryItemInstalled},
		{name: "another origin, the repository's content", local: libraryTestLocal(t, repo, 2, &LibraryOrigin{Repository: "owner/other"}), state: LibraryItemInstalled},
		{name: "no origin, shared from here with its own settings", local: sharedFromHere(t, repo, 2), state: LibraryItemInstalled},
		{name: "no origin, changed content", local: changedTeamBelt(t, repo, 2, nil), state: LibraryItemConflict, reason: "team is already in your library and didn't come from this repository."},
		{name: "changed here at the same version", local: changedTeamBelt(t, repo, 2, &libraryTestOrigin), state: LibraryItemConflict, reason: "You changed team v2 locally; save it under a new id before updating.", changed: true},
	}
	for _, c := range cases {
		items, problems := LibraryRepoItems(libraryTestOrigin, repo, c.local)
		if len(problems) != 0 {
			t.Fatalf("%s: repository problems %v", c.name, problems)
		}
		item := libraryTestState(t, items, "belt", "team")
		if item.State != c.state {
			t.Errorf("%s: state %s, want %s (reason %v)", c.name, item.State, c.state, item.Reason)
		}
		if c.reason != "" && (item.Reason == nil || *item.Reason != c.reason) {
			t.Errorf("%s: reason %v, want %q", c.name, item.Reason, c.reason)
		}
		if installable := item.State == LibraryItemAvailable || item.State == LibraryItemUpdate; installable != item.Actions[0].Enabled {
			t.Errorf("%s: install enabled %v for state %s", c.name, item.Actions[0].Enabled, item.State)
		}
	}
}

// changedTeamBelt is the library's files with the team belt at version, its name changed.
func changedTeamBelt(t *testing.T, repo map[string][]byte, version int, origin *LibraryOrigin) map[string][]byte {
	t.Helper()
	local := libraryTestLocal(t, repo, version, origin)
	path := libraryVersionPath("belts", "team", version, "json")
	var belt Belt
	if err := jsonx.Unmarshal(local[path], &belt); err != nil {
		t.Fatal(err)
	}
	belt.Name = "Edited here"
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		t.Fatal(err)
	}
	local[path] = data
	return local
}

// sharedFromHere is the library's files with the team belt at version as this library holds it after a
// share: no origin, and its own base branch, AFK mode and trigger repositories, which a share leaves out.
func sharedFromHere(t *testing.T, repo map[string][]byte, version int) map[string][]byte {
	t.Helper()
	local := libraryTestLocal(t, repo, version, nil)
	path := libraryVersionPath("belts", "team", version, "json")
	var belt Belt
	if err := jsonx.Unmarshal(local[path], &belt); err != nil {
		t.Fatal(err)
	}
	belt.Base, belt.Afk = "release", AfkModeAlways
	if len(belt.Triggers) > 0 {
		belt.Triggers[0].Repos = []string{"acme/platform"}
	}
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		t.Fatal(err)
	}
	local[path] = data
	return local
}

// TestLibraryRepoItemsTakeAnUnoriginedOlderVersionAsAnUpdate checks that an installed version with no
// origin, whose content is the repository's copy of its number, is updatable, and that the update writes
// the newer version with its origin.
func TestLibraryRepoItemsTakeAnUnoriginedOlderVersionAsAnUpdate(t *testing.T) {
	t.Parallel()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	older := libraryTestLocal(t, repo, 1, &libraryTestOrigin)
	repo[libraryVersionPath("belts", "team", 1, "json")] = older[libraryVersionPath("belts", "team", 1, "json")]
	local := libraryTestLocal(t, repo, 1, nil)

	items, _ := LibraryRepoItems(libraryTestOrigin, repo, local)
	if item := libraryTestState(t, items, "belt", "team"); item.State != LibraryItemUpdate {
		t.Fatalf("an unoriginated v1 with the repository's v1: state %s (reason %v), want update", item.State, item.Reason)
	}
	files, err := LibraryInstallFiles(libraryTestOrigin, "belt", "team", repo, local)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		local[file.Path] = file.Data
		if file.Path == "belts/team/v2.json" {
			var belt Belt
			if err := jsonx.Unmarshal(file.Data, &belt); err != nil || belt.Origin == nil || *belt.Origin != libraryTestOrigin {
				t.Fatalf("the update's belt has origin %v (err %v), want %v", belt.Origin, err, libraryTestOrigin)
			}
		}
	}
	items, _ = LibraryRepoItems(libraryTestOrigin, repo, local)
	if item := libraryTestState(t, items, "belt", "team"); item.State != LibraryItemInstalled || item.LocalVersion == nil || *item.LocalVersion != 2 {
		t.Fatalf("after the update: state %s, local version %v", item.State, item.LocalVersion)
	}
}

// TestLibraryRepoItemsCountAnUnoriginedAgentFromTheRepository checks an agent with no origin: installed
// when its content is the repository's, and a conflict once its content changes here.
func TestLibraryRepoItemsCountAnUnoriginedAgentFromTheRepository(t *testing.T) {
	t.Parallel()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	agent := SampleAgents()[0]
	path := libraryVersionPath("agents", agent.ID, agent.Version, "md")
	standing := func(local Agent) LibraryItem {
		items, _ := LibraryRepoItems(libraryTestOrigin, repo, map[string][]byte{path: []byte(AgentFileWrite(local))})
		return libraryTestState(t, items, "agent", agent.ID)
	}
	if item := standing(agent); item.State != LibraryItemInstalled {
		t.Fatalf("an unoriginated agent with the repository's content: state %s (reason %v), want installed", item.State, item.Reason)
	}
	edited := agent
	edited.Name = "Edited here"
	if item := standing(edited); item.State != LibraryItemConflict || item.Reason == nil || !strings.Contains(*item.Reason, "didn't come from this repository") {
		t.Fatalf("an unoriginated agent changed here: state %s (reason %v), want a conflict", item.State, item.Reason)
	}
}

// TestLibraryShareBranchNamesItsKind checks that a belt and an agent of one id and version never share a
// share branch or a title.
func TestLibraryShareBranchNamesItsKind(t *testing.T) {
	t.Parallel()
	if got := LibraryShareBranch("belt", "review", 1); got != "beltline/share-belt-review-v1" {
		t.Fatalf("belt branch %q", got)
	}
	if belt, agent := LibraryShareBranch("belt", "review", 1), LibraryShareBranch("agent", "review", 1); belt == agent {
		t.Fatalf("a belt and an agent share branch %q", belt)
	}
	if got := LibraryShareTitle("agent", "review", 1); got != "Share agent review v1" {
		t.Fatalf("agent title %q", got)
	}
}

func TestLibraryInstallWritesTheVersionAndTheMissingPins(t *testing.T) {
	t.Parallel()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	files, err := LibraryInstallFiles(libraryTestOrigin, "belt", "team", repo, map[string][]byte{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	slices.Sort(paths)
	if !slices.Contains(paths, "belts/team/v2.json") || len(paths) < 2 {
		t.Fatalf("install wrote %v, want the belt and the agents it pins", paths)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Path, "belts/") && !strings.Contains(string(file.Data), `"origin"`) {
			t.Errorf("the installed belt carries no origin: %s", file.Data)
		}
	}

	// Installed items write nothing; a conflict is refused with its reason.
	installed := libraryTestLocal(t, repo, 2, &libraryTestOrigin)
	if files, err := LibraryInstallFiles(libraryTestOrigin, "belt", "team", repo, installed); err != nil || len(files) != 0 {
		t.Errorf("installed item: files %v, err %v", files, err)
	}
	conflict := changedTeamBelt(t, repo, 2, &libraryTestOrigin)
	if _, err := LibraryInstallFiles(libraryTestOrigin, "belt", "team", repo, conflict); err == nil || !strings.Contains(err.Error(), "You changed team v2 locally") {
		t.Errorf("conflict: err %v", err)
	}
}

func TestLibraryShareStripsTheBeltAndRefusesOtherContent(t *testing.T) {
	t.Parallel()
	own := libraryTestLocal(t, libraryTestRepo(t, 2, nil), 2, nil)
	for path, data := range libraryTestRepo(t, 2, nil) {
		own[path] = data
	}
	var belt Belt
	if err := jsonx.Unmarshal(own["belts/team/v2.json"], &belt); err != nil {
		t.Fatal(err)
	}
	belt.Base = "release"
	belt.Afk = AfkModeAlways
	if len(belt.Triggers) > 0 {
		belt.Triggers[0].Repos = []string{"acme/platform"}
	}
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		t.Fatal(err)
	}
	own["belts/team/v2.json"] = data

	shared := map[string][]byte{}
	files, err := LibraryShareFiles(libraryTestOrigin, "belt", "team", own, shared)
	if err != nil {
		t.Fatal(err)
	}
	var shipped Belt
	for _, file := range files {
		if file.Path == "belts/team/v2.json" {
			if err := jsonx.Unmarshal(file.Data, &shipped); err != nil {
				t.Fatal(err)
			}
		}
	}
	if shipped.Base != "" || shipped.Afk != AfkModeGlobal || shipped.Origin != nil || len(shipped.Triggers[0].Repos) != 0 {
		t.Fatalf("shared belt keeps what belongs to one library: base %q afk %q origin %v repos %v",
			shipped.Base, shipped.Afk, shipped.Origin, shipped.Triggers[0].Repos)
	}

	// The same version with other content in the repository is refused; the same content is not written again.
	other := map[string][]byte{"belts/team/v2.json": []byte(strings.Replace(string(data), `"name":"Team"`, `"name":"Other"`, 1))}
	if _, err := LibraryShareFiles(libraryTestOrigin, "belt", "team", own, other); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("other content: err %v", err)
	}
	mirrored := map[string][]byte{}
	for _, file := range files {
		mirrored[file.Path] = file.Data
	}
	if again, err := LibraryShareFiles(libraryTestOrigin, "belt", "team", own, mirrored); err != nil || len(again) != 0 {
		t.Errorf("a shared version again: files %d, err %v", len(again), err)
	}
}

func TestLibraryBackupFilesLeaveOutTheStateFiles(t *testing.T) {
	t.Parallel()
	backup := LibraryBackupFiles(map[string][]byte{
		"belts/a/v1.json": []byte("{}"), "belts/a/state.json": []byte("{}"),
		"agents/x/v1.md": []byte("---"), "agents/x/state.json": []byte("{}"), "settings.json": []byte("{}"),
	})
	if len(backup) != 2 || backup["belts/a/v1.json"] == nil || backup["agents/x/v1.md"] == nil {
		t.Fatalf("backup holds %v", backup)
	}
}

func TestOnlySharedRepositoriesOtherThanTheExamplesTakeShares(t *testing.T) {
	t.Parallel()
	if !LibraryShareAccepted(LibraryRepository{Repository: "acme/team", Role: LibraryRoleShared}) {
		t.Error("a shared repository refuses shares")
	}
	if LibraryShareAccepted(LibraryRepository{Repository: "acme/backup", Role: LibraryRoleBackup}) {
		t.Error("a backup repository takes shares")
	}
	examples := LibraryExamples()
	examples.Publish = LibraryPublishPush
	if LibraryShareAccepted(examples) {
		t.Error("the examples repository takes shares")
	}
}

func TestLibraryRepoActionsExplainWhatIsUnavailable(t *testing.T) {
	t.Parallel()
	shared := LibraryRepoActions(LibraryRoleShared, true, true)
	if len(shared) != 1 || shared[0].ID != "refresh" || !shared[0].Enabled {
		t.Fatalf("shared actions %+v", shared)
	}
	backup := LibraryRepoActions(LibraryRoleBackup, false, false)
	if restore := backup[2]; restore.ID != "restore" || restore.Enabled || restore.Reason == nil || *restore.Reason != "Refresh the repository first." {
		t.Fatalf("restore before a refresh: %+v", restore)
	}
	if restore := LibraryRepoActions(LibraryRoleBackup, true, false)[2]; restore.Enabled || restore.Reason == nil {
		t.Fatalf("restore of an empty repository: %+v", restore)
	}
}
