package core

import (
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// libraryIDsAgent is an agent's path and file, with name as what tells its content apart.
func libraryIDsAgent(id string, version int, name string, change func(*Agent)) (string, []byte) {
	agent := NewAgent(AgentArgs{ID: id, Version: &version, Name: name, Brief: "Brief."})
	if change != nil {
		change(&agent)
	}
	return libraryVersionPath("agents", id, version, "md"), []byte(AgentFileWrite(agent))
}

// libraryIDsFiles adds each agent to a new set of files.
func libraryIDsFiles(agents ...func() (string, []byte)) map[string][]byte {
	files := map[string][]byte{}
	for _, agent := range agents {
		path, data := agent()
		files[path] = data
	}
	return files
}

// libraryIDsOf is an agent as a function of no arguments, for libraryIDsFiles.
func libraryIDsOf(id string, version int, name string, change func(*Agent)) func() (string, []byte) {
	return func() (string, []byte) { return libraryIDsAgent(id, version, name, change) }
}

// libraryIDsFrom gives an agent an origin, and an origin id when it isn't empty.
func libraryIDsFrom(origin LibraryOrigin, originID string) func(*Agent) {
	return func(agent *Agent) { agent.Origin, agent.OriginID = &origin, originID }
}

func TestLibraryKeepsAnUnoriginedItemTheRepositorysAfterALocalSave(t *testing.T) {
	t.Parallel()
	repo := libraryIDsFiles(libraryIDsOf("build", 1, "Build", nil))
	local := libraryIDsFiles(libraryIDsOf("build", 1, "Build", nil), libraryIDsOf("build", 2, "Build, changed", nil))
	items, _ := LibraryRepoItems(libraryTestOrigin, repo, local)
	item := libraryTestState(t, items, "agent", "build")
	if item.LocalID == nil || *item.LocalID != "build" || item.State != LibraryItemInstalled {
		t.Fatalf("a shared agent with a newer local version: %+v", item)
	}
	if _, files, err := LibraryInstallFiles(libraryTestOrigin, "agent", "build", repo, local); err != nil || len(files) != 0 {
		t.Fatalf("install: files %v, err %v", files, err)
	}

	repo = libraryIDsFiles(libraryIDsOf("build", 1, "Build", nil), libraryIDsOf("build", 2, "Build, theirs", nil))
	items, _ = LibraryRepoItems(libraryTestOrigin, repo, local)
	if item := libraryTestState(t, items, "agent", "build"); item.State != LibraryItemConflict {
		t.Fatalf("a locally changed version the repository also has: %+v", item)
	}
}

func TestLibraryDoesNotTakeAnotherRepositorysItemByItsContent(t *testing.T) {
	t.Parallel()
	other := LibraryOrigin{Repository: "owner/other"}
	repo := libraryIDsFiles(libraryIDsOf("build", 1, "Build", nil), libraryIDsOf("build", 2, "Build two", nil))
	local := libraryIDsFiles(libraryIDsOf("build", 1, "Build", libraryIDsFrom(other, "")))
	items, _ := LibraryRepoItems(libraryTestOrigin, repo, local)
	if item := libraryTestState(t, items, "agent", "build"); item.State != LibraryItemAvailable || item.LocalID != nil {
		t.Fatalf("another repository's agent with the same text: %+v", item)
	}
	localID, files, err := LibraryInstallFiles(libraryTestOrigin, "agent", "build", repo, local)
	if err != nil || localID == "build" || len(files) != 1 || !strings.HasPrefix(files[0].Path, "agents/"+localID+"/") {
		t.Fatalf("install: id %q, files %v, err %v", localID, files, err)
	}
}

func TestLibraryShareRefusesAnIDTheRepositoryUsesForSomethingElse(t *testing.T) {
	t.Parallel()
	repo := libraryIDsFiles(libraryIDsOf("build", 1, "Theirs", nil), libraryIDsOf("build", 2, "Theirs too", nil))
	local := libraryIDsFiles(libraryIDsOf("build", 3, "Mine", nil))
	if files, err := LibraryShareFiles(libraryTestOrigin, "agent", "build", local, repo); err == nil || !strings.Contains(err.Error(), "already uses the id build") {
		t.Fatalf("an unrelated agent: files %v, err %v", files, err)
	}
}

func TestLibraryShareRefusesABeltPinningTwoAgentsOfOneRepositoryID(t *testing.T) {
	t.Parallel()
	belt := SampleBelt([]string{"a/b"}, "agent", SampleAgents(), true)
	belt.ID, belt.Version = "team", 1
	var stages []int
	for index, stage := range belt.Stages {
		if stage.Agent != nil {
			stages = append(stages, index)
		}
	}
	belt.Stages[stages[0]].Agent = &AgentRef{ID: "build-7k2m", Version: 1}
	belt.Stages[stages[1]].Agent = &AgentRef{ID: "build-9x3p", Version: 1}
	belt.Stages = []Stage{belt.Stages[stages[0]], belt.Stages[stages[1]]}
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		t.Fatal(err)
	}
	local := libraryIDsFiles(
		libraryIDsOf("build-7k2m", 1, "One", libraryIDsFrom(libraryTestOrigin, "build")),
		libraryIDsOf("build-9x3p", 1, "Two", libraryIDsFrom(libraryTestOrigin, "build")))
	local["belts/team/v1.json"] = data
	if _, err := LibraryShareFiles(libraryTestOrigin, "belt", "team", local, map[string][]byte{}); err == nil || !strings.Contains(err.Error(), "the same id, build") {
		t.Fatalf("err %v", err)
	}
}

func TestLibraryKeepsAnIDAnotherRepositorysAgentGoesTo(t *testing.T) {
	t.Parallel()
	other := LibraryOrigin{Repository: "owner/other"}
	local := libraryIDsFiles(libraryIDsOf("a", 1, "A", func(agent *Agent) {
		libraryIDsFrom(other, "")(agent)
		agent.Outcomes = []AgentOutcome{{Name: "back", When: "Go back.", Tone: ToneNeutral, Then: Then{Action: ThenActionGoto}, To: []string{"b"}}}
	}))
	if _, agents, problems := libraryCheckRead(local); len(problems) != 0 || agents["a"][1].Outcomes[0].To[0] != "b" {
		t.Fatalf("the agent doesn't read back: %v %+v", problems, agents["a"][1])
	}
	repo := libraryIDsFiles(libraryIDsOf("b", 1, "B", nil))
	localID, _, err := LibraryInstallFiles(libraryTestOrigin, "agent", "b", repo, local)
	if err != nil || localID == "b" || !strings.HasPrefix(localID, "b-") {
		t.Fatalf("id %q, err %v", localID, err)
	}
	if localID, _, err := LibraryInstallFiles(other, "agent", "b", repo, local); err != nil || localID != "b" {
		t.Fatalf("from the repository the target means: id %q, err %v", localID, err)
	}
}

func TestLibraryLocalPathsFollowTheMappedIDs(t *testing.T) {
	t.Parallel()
	repo := libraryIDsFiles(libraryIDsOf("build", 1, "Theirs", nil))
	local := libraryIDsFiles(
		libraryIDsOf("build", 5, "Mine", nil),
		libraryIDsOf("build-7k2m", 1, "Theirs", libraryIDsFrom(libraryTestOrigin, "build")))
	paths := LibraryLocalPaths(libraryTestOrigin, []string{"agents/build/v1.md"}, repo, local)
	if got := paths["agents/build/v1.md"]; got != "agents/build-7k2m/v1.md" {
		t.Fatalf("local path %q", got)
	}
}

func TestLibraryRepoIDIsTheOriginIDOnlyForThatOrigin(t *testing.T) {
	t.Parallel()
	if got := LibraryRepoID(libraryTestOrigin, "build-7k2m", &libraryTestOrigin, "build"); got != "build" {
		t.Errorf("its own repository: %q", got)
	}
	if got := LibraryRepoID(libraryTestOrigin, "build-7k2m", &LibraryOrigin{Repository: "owner/other"}, "build"); got != "build-7k2m" {
		t.Errorf("another repository: %q", got)
	}
	if got := LibraryRepoID(libraryTestOrigin, "build", nil, ""); got != "build" {
		t.Errorf("no origin: %q", got)
	}
}

func TestAgentChoiceDetailsUseTheIDWithinOneRepository(t *testing.T) {
	t.Parallel()
	acme, other := LibraryOrigin{Repository: "acme/lib"}, LibraryOrigin{Repository: "acme/other"}
	record := func(id string, origin *LibraryOrigin) AgentRecord {
		agent := NewAgent(AgentArgs{ID: id, Name: "Build", Brief: "Brief."})
		agent.Origin = origin
		return AgentRecord{Agent: agent}
	}
	want := map[string]string{"one": "one", "two": "two", "three": "acme/other", "mine": "mine"}
	for _, choice := range AgentChoices([]AgentRecord{record("one", &acme), record("two", &acme), record("three", &other), record("mine", nil)}) {
		if choice.Detail != want[choice.ID] {
			t.Errorf("%s: detail %q, want %q", choice.ID, choice.Detail, want[choice.ID])
		}
	}
}
