// Tests of the defaults: the watch outcomes, the built-in ship agent and the stage and belt builders.

package core

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// defaultsTestBuildStage is the agent stage the watch-default test places a
// watch after: a plain agent stage with the id build.
func defaultsTestBuildStage() Stage {
	return NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Outcomes: []Outcome{}})
}

// TestWatchDefaultsCoverEveryEventAndBoundRetries
func TestWatchDefaultsCoverEveryEventAndBoundRetries(t *testing.T) {
	t.Parallel()
	build := defaultsTestBuildStage()
	outcomes := DefaultsOutcomesForAt(StageTypeWatch, DefaultsPlace{Before: []Stage{build}})
	var names, whens []string
	var tones []Tone
	var thens []Then
	for _, outcome := range outcomes {
		names = append(names, outcome.Name)
		whens = append(whens, outcome.When)
		tones = append(tones, outcome.Tone)
		thens = append(thens, outcome.Then)
	}
	wantNames := []string{"merged", "closed", "checks failing", "has conflicts", "is behind", "is approved", "a reviewer commented", "changes requested", "new changes"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("names = %v, want %v", names, wantNames)
	}
	var wantWhens []string
	for _, event := range AllWatchEvent {
		if event != WatchEventNone {
			wantWhens = append(wantWhens, string(event))
		}
	}
	if !reflect.DeepEqual(whens, wantWhens) {
		t.Fatalf("whens = %v, want %v", whens, wantWhens)
	}
	wantTones := []Tone{ToneGood, ToneProblem, ToneProblem, ToneProblem, ToneProblem, ToneGood, ToneProblem, ToneProblem, ToneYou}
	if !reflect.DeepEqual(tones, wantTones) {
		t.Fatalf("tones = %v, want %v", tones, wantTones)
	}
	retry := func(action ThenAction, max int) Then {
		return NewThen(ThenArgs{Action: action, To: []string{build.ID}, Max: new(max)})
	}
	wantThens := []Then{
		NewThen(ThenArgs{Action: ThenActionNext}), NewThen(ThenArgs{Action: ThenActionHold}), retry(ThenActionGoto, 2),
		NewThen(ThenArgs{Action: ThenActionHold}), retry(ThenActionGoto, 2), NewThen(ThenArgs{Action: ThenActionHold}),
		NewThen(ThenArgs{Action: ThenActionHold}), NewThen(ThenArgs{Action: ThenActionHold}), NewThen(ThenArgs{Action: ThenActionHold}),
	}
	if !slices.EqualFunc(thens, wantThens, defaultsThenEqual) {
		t.Fatalf("thens = %+v, want %+v", thens, wantThens)
	}
	if got := DefaultsOutcomesForAt(StageTypeWatch, DefaultsPlace{Later: []Stage{build}}); !defaultsThenEqual(got[2].Then, NewThen(ThenArgs{Action: ThenActionHold})) {
		t.Fatalf("a retry with nothing before it = %+v, want hold", got[2].Then)
	}
}

// TestAGotoWithNothingBeforeItHolds
func TestAGotoWithNothingBeforeItHolds(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsOutcomesForAgentAt(SampleAgent("verify"), DefaultsPlace{})
	if !defaultsThenEqual(outcomes[1].Then, NewThen(ThenArgs{Action: ThenActionHold})) {
		t.Fatalf("goto with no stage before it = %+v, want hold", outcomes[1].Then)
	}
}

// TestShipOpensOnePullRequestPerChangedRepositoryAndCanShipNothing
func TestShipOpensOnePullRequestPerChangedRepositoryAndCanShipNothing(t *testing.T) {
	t.Parallel()
	ship := SampleAgentNamed("ship", nil)
	if ship == nil {
		t.Fatal("no bundled ship agent")
	}
	if ship.Version != 2 {
		t.Fatalf("ship version = %d, want 2", ship.Version)
	}
	var names []string
	for _, outcome := range ship.Outcomes {
		names = append(names, outcome.Name)
	}
	if want := []string{"opened", "nothing to ship", "failed"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("outcomes = %v, want %v", names, want)
	}
	if ship.Outcomes[1].Then.Action != ThenActionHold {
		t.Fatalf("nothing to ship = %+v, want hold", ship.Outcomes[1].Then)
	}
	if !strings.Contains(ship.Brief, "in each repository where it has commits to merge") {
		t.Fatal("the ship brief does not say it opens one pull request per changed repository")
	}
}

// defaultsTestDefaultBelt is Defaults.belt(repos: ["a/b"]) in Go's arguments.
func defaultsTestDefaultBelt() Belt {
	return SampleBelt([]string{"a/b"}, "agent", SampleAgents(), true)
}

// TestWatchUpgradeIsAdditiveAndIdempotent
func TestWatchUpgradeIsAdditiveAndIdempotent(t *testing.T) {
	t.Parallel()
	old := defaultsTestDefaultBelt()
	old.Stages[6].Outcomes = append([]Outcome{}, old.Stages[6].Outcomes[:2]...)
	old.Stages[6].Outcomes[0].Then = NewThen(ThenArgs{Action: ThenActionDone})
	custom := Outcome{ID: NewUUID(), Name: "fix CI", When: "checksFailed", Tone: ToneYou, Then: NewThen(ThenArgs{Action: ThenActionPark})}
	old.Stages[6].Outcomes = append(old.Stages[6].Outcomes, custom)
	data, err := jsonx.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Belt
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	upgraded := DefaultsUpgradingWatchOutcomes(decoded, DefaultsMigratedWatchEvents())
	if !reflect.DeepEqual(upgraded.Stages[6].Outcomes[:3], old.Stages[6].Outcomes) {
		t.Fatalf("the first three outcomes changed: %+v", upgraded.Stages[6].Outcomes[:3])
	}
	if got := len(upgraded.Stages[6].Outcomes); got != 8 {
		t.Fatalf("outcomes = %d, want 8", got)
	}
	for _, outcome := range upgraded.Stages[6].Outcomes {
		if outcome.When == string(WatchEventNewChanges) || outcome.When == string(WatchEventNone) {
			t.Fatalf("an outcome for %q was added, want none", outcome.When)
		}
	}
	all := map[WatchEvent]struct{}{}
	for _, event := range AllWatchEvent {
		all[event] = struct{}{}
	}
	if got := len(DefaultsUpgradingWatchOutcomes(decoded, all).Stages[6].Outcomes); got != 8 {
		t.Fatalf("upgrading every event gives %d outcomes, want 8", got)
	}
	if again := DefaultsUpgradingWatchOutcomes(upgraded, all); !reflect.DeepEqual(again, upgraded) {
		t.Fatal("upgrading an upgraded belt changed it")
	}
	data, err = jsonx.Marshal(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded Belt
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if again := DefaultsUpgradingWatchOutcomes(reloaded, DefaultsMigratedWatchEvents()); !reflect.DeepEqual(again, upgraded) {
		t.Fatal("upgrading a saved and reloaded upgraded belt changed it")
	}
}

// TestGotoTargetsTheNearestEarlierStage
func TestGotoTargetsTheNearestEarlierStage(t *testing.T) {
	t.Parallel()
	belt := defaultsTestDefaultBelt()
	if got := belt.Stages[2].Outcomes[1].Then; !defaultsThenEqual(got, NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"spec"}, WithOutput: true})) {
		t.Fatalf("approve's send back = %+v, want goto spec with output", got)
	}
	if got := belt.Stages[4].Outcomes[1].Then; !defaultsThenEqual(got, NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"build"}, WithOutput: true, Max: new(1)})) {
		t.Fatalf("verify's code mismatch = %+v, want goto build with output, max 1", got)
	}
}

// TestNewStagesGetUniqueIds
func TestNewStagesGetUniqueIds(t *testing.T) {
	t.Parallel()
	belt := defaultsTestDefaultBelt()
	stage := DefaultsStage(StageTypeApprove, belt.Stages, nil, belt.Stages)
	if stage.ID != "approve-2" {
		t.Fatalf("id = %q, want approve-2", stage.ID)
	}
	if stage.Agent != nil {
		t.Fatalf("agent = %+v, want none", stage.Agent)
	}
	if stage.Runner != nil {
		t.Fatalf("runner = %v, want none", *stage.Runner)
	}
}

// TestAgentsDefaultToPiWithSpaceBunny
func TestAgentsDefaultToPiWithSpaceBunny(t *testing.T) {
	t.Parallel()
	belt := defaultsTestDefaultBelt()
	for _, stage := range belt.Stages {
		if stage.Kind() != StageKindAgent {
			continue
		}
		if stage.Runner == nil || *stage.Runner != BackendPi {
			t.Fatalf("%s runner = %v, want pi", stage.ID, stage.Runner)
		}
		if stage.Agent == nil || stage.Agent.Version != SampleAgentVersion(stage.Agent.ID) {
			t.Fatalf("%s agent = %+v, want the shipped version", stage.ID, stage.Agent)
		}
		if stage.Model != "openrouter/stealth/space-bunny-alpha" {
			t.Fatalf("%s model = %q", stage.ID, stage.Model)
		}
		if stage.Thinking != nil {
			t.Fatalf("%s thinking = %v, want the runner's default", stage.ID, *stage.Thinking)
		}
	}
	if belt.Stages[2].Model != "" {
		t.Fatalf("approve model = %q, want empty", belt.Stages[2].Model)
	}
	if belt.Stages[3].Slots != 1 {
		t.Fatalf("build slots = %d, want 1", belt.Stages[3].Slots)
	}
	if belt.Stages[3].Workspace != WorkspaceWorktree {
		t.Fatalf("build workspace = %q, want worktree", belt.Stages[3].Workspace)
	}
	if belt.Stages[4].Workspace != WorkspaceWorktreeReadOnly {
		t.Fatalf("verify workspace = %q, want worktreeReadOnly", belt.Stages[4].Workspace)
	}
}

// TestNewStagesStartWithTheirAgentsThinking
func TestNewStagesStartWithTheirAgentsThinking(t *testing.T) {
	t.Parallel()
	agent := SampleAgent("build")
	agent.Thinking = new(ThinkingHigh)
	stage := DefaultsStageForAgent(agent, nil, nil, nil, TriggerKindIssues)
	if stage.Thinking == nil || *stage.Thinking != ThinkingHigh {
		t.Fatalf("thinking = %v, want high", stage.Thinking)
	}
}
