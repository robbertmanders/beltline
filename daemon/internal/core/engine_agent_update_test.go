// Tests of stage updates to an agent's latest version, and of custom agents in the engine and brief.

package core

import (
	"reflect"
	"strings"
	"testing"
)

// agentUpdateNewer is the newer agent of the update tests: version 2, with a new name,
// slots, workspace, model, thinking and an extra outcome.
func agentUpdateNewer() Agent {
	agent := customTestChecker()
	agent.Version = 2
	agent.Name = "Double checker"
	agent.Slots = 4
	agent.Workspace = WorkspaceReadOnly
	agent.Model = "fast-model"
	agent.Thinking = new(ThinkingHigh)
	agent.Outcomes = append(agent.Outcomes, AgentOutcome{Name: "unsure", When: "It can't tell.", Tone: ToneYou, Then: NewThen(ThenArgs{Action: ThenActionHold})})
	return agent
}

// agentUpdateOutcomeNames are the names of outcomes, in order.
func agentUpdateOutcomeNames(outcomes []Outcome) []string {
	var names []string
	for _, outcome := range outcomes {
		names = append(names, outcome.Name)
	}
	return names
}

// TestSettingsYouDidntChangeFollowTheNewVersion
func TestSettingsYouDidntChangeFollowTheNewVersion(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	checker := customTestChecker()
	newer := agentUpdateNewer()
	updated := DefaultsUpdate(belt.Stages[1], &checker, newer, belt.Stages[:1], nil, TriggerKindIssues)
	if updated.Agent == nil || *updated.Agent != (AgentRef{ID: "checker", Version: 2}) {
		t.Fatalf("agent = %+v, want checker v2", updated.Agent)
	}
	if updated.Name != "Double checker" {
		t.Fatalf("name = %q", updated.Name)
	}
	if updated.Slots != 4 {
		t.Fatalf("slots = %d, want 4", updated.Slots)
	}
	if updated.Workspace != WorkspaceReadOnly {
		t.Fatalf("workspace = %q", updated.Workspace)
	}
	if updated.Model != "fast-model" {
		t.Fatalf("model = %q", updated.Model)
	}
	if updated.Thinking == nil || *updated.Thinking != ThinkingHigh {
		t.Fatalf("thinking = %v, want high", updated.Thinking)
	}
	if want := []string{"fine", "redo", "unsure"}; !reflect.DeepEqual(agentUpdateOutcomeNames(updated.Outcomes), want) {
		t.Fatalf("outcomes = %v, want %v", agentUpdateOutcomeNames(updated.Outcomes), want)
	}
	if got := updated.Outcomes[1].Then.To; !reflect.DeepEqual(got, []string{"writer"}) {
		t.Fatalf("redo goes to %v, want [writer]", got)
	}
	if updated.ID != belt.Stages[1].ID {
		t.Fatalf("id = %q, want %q", updated.ID, belt.Stages[1].ID)
	}
}

// TestWhatYouChangedOnTheStageStays
func TestWhatYouChangedOnTheStageStays(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	checker := customTestChecker()
	stage := belt.Stages[1]
	stage.Name = "Proofread"
	stage.Slots = 1
	stage.Runner = new(BackendCopilot)
	stage.Thinking = new(ThinkingMax)
	stage.Brief = "My own brief."
	stage.Outcomes[0].When = "It reads well."
	stage.Fallback = FallbackDrop
	updated := DefaultsUpdate(stage, &checker, agentUpdateNewer(), belt.Stages[:1], nil, TriggerKindIssues)
	if updated.Agent == nil || updated.Agent.Version != 2 {
		t.Fatalf("agent = %+v, want version 2", updated.Agent)
	}
	if updated.Name != "Proofread" {
		t.Fatalf("name = %q, want Proofread", updated.Name)
	}
	if updated.Slots != 1 {
		t.Fatalf("slots = %d, want 1", updated.Slots)
	}
	if updated.Runner == nil || *updated.Runner != BackendCopilot {
		t.Fatalf("runner = %v, want copilot", updated.Runner)
	}
	if updated.Thinking == nil || *updated.Thinking != ThinkingMax {
		t.Fatalf("thinking = %v, want max", updated.Thinking)
	}
	if updated.Brief != "My own brief." {
		t.Fatalf("brief = %q", updated.Brief)
	}
	if want := []string{"fine", "redo"}; !reflect.DeepEqual(agentUpdateOutcomeNames(updated.Outcomes), want) {
		t.Fatalf("outcomes = %v, want %v", agentUpdateOutcomeNames(updated.Outcomes), want)
	}
	if updated.Fallback != FallbackDrop {
		t.Fatalf("fallback = %q, want drop", updated.Fallback)
	}
	if updated.Model != "fast-model" {
		t.Fatalf("model = %q, want fast-model (an untouched setting still follows)", updated.Model)
	}
	if updated.Key == nil || *updated.Key != "check" {
		t.Fatalf("key = %v, want check", updated.Key)
	}
}

// TestTheStageKeyStaysWhenThePinnedAgentKeyChanges
func TestTheStageKeyStaysWhenThePinnedAgentKeyChanges(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	checker := customTestChecker()
	stage := belt.Stages[1]
	if stage.Key == nil || *stage.Key != checker.Key {
		t.Fatalf("key = %v, want %q", stage.Key, checker.Key)
	}
	stage.SetKey(new("proof"))
	renamed := agentUpdateNewer()
	renamed.Key = "review"
	edited := DefaultsUpdate(stage, &checker, renamed, belt.Stages[:1], nil, TriggerKindIssues)
	if edited.Key == nil || *edited.Key != "proof" {
		t.Fatalf("key = %v, want proof", edited.Key)
	}
	if edited.Agent == nil || edited.Agent.Version != 2 {
		t.Fatalf("agent = %+v, want version 2", edited.Agent)
	}
	stage.SetKey(nil)
	reset := DefaultsUpdate(stage, &checker, renamed, belt.Stages[:1], nil, TriggerKindIssues)
	if reset.Key == nil || *reset.Key != checker.Key {
		t.Fatalf("key without a chosen one = %v, want %q", reset.Key, checker.Key)
	}
}

// TestAnUntouchedThinkingLevelCanChangeOrBeCleared
func TestAnUntouchedThinkingLevelCanChangeOrBeCleared(t *testing.T) {
	t.Parallel()
	old := customTestChecker()
	old.Thinking = new(ThinkingLow)
	stage := DefaultsStageForAgent(old, nil, nil, nil, TriggerKindIssues)
	if got := DefaultsUpdate(stage, &old, agentUpdateNewer(), nil, nil, TriggerKindIssues); got.Thinking == nil || *got.Thinking != ThinkingHigh {
		t.Fatalf("thinking = %v, want high", got.Thinking)
	}
	cleared := agentUpdateNewer()
	cleared.Thinking = nil
	if got := DefaultsUpdate(stage, &old, cleared, nil, nil, TriggerKindIssues); got.Thinking != nil {
		t.Fatalf("thinking = %v, want none", *got.Thinking)
	}
}

// TestChoosingTheRunnersDefaultIsAnOverrideToo
func TestChoosingTheRunnersDefaultIsAnOverrideToo(t *testing.T) {
	t.Parallel()
	old := customTestChecker()
	old.Thinking = new(ThinkingLow)
	stage := DefaultsStageForAgent(old, nil, nil, nil, TriggerKindIssues)
	stage.Thinking = nil
	if got := DefaultsUpdate(stage, &old, agentUpdateNewer(), nil, nil, TriggerKindIssues); got.Thinking != nil {
		t.Fatalf("thinking = %v, want the runner's default", *got.Thinking)
	}
}

// TestAGotoCanSkipAheadToALaterStage
func TestAGotoCanSkipAheadToALaterStage(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	writer := customTestWriter()
	skipping := writer
	skipping.Version = 2
	skipping.Outcomes = append(skipping.Outcomes, AgentOutcome{Name: "trivial", When: "Nothing to write.", Tone: ToneGood,
		Then: NewThen(ThenArgs{Action: ThenActionGoto}), To: []string{"checker"}})
	updated := DefaultsUpdate(belt.Stages[0], &writer, skipping, nil, belt.Stages[1:], TriggerKindIssues)
	want := NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"checker"}})
	if !defaultsThenEqual(updated.Outcomes[1].Then, want) {
		t.Fatalf("updated trivial = %+v, want goto checker", updated.Outcomes[1].Then)
	}
	added := DefaultsStageForAgent(skipping, nil, belt.Stages[1:], belt.Stages, TriggerKindIssues)
	if !defaultsThenEqual(added.Outcomes[1].Then, want) {
		t.Fatalf("added trivial = %+v, want goto checker", added.Outcomes[1].Then)
	}
}

// TestAnEarlierStageWinsOverALaterOne
func TestAnEarlierStageWinsOverALaterOne(t *testing.T) {
	t.Parallel()
	stages := customTestBelt(customTestWriter(), customTestChecker()).Stages
	later := DefaultsStageForAgent(customTestWriter(), nil, nil, stages, TriggerKindIssues)
	outcomes := DefaultsOutcomesForAgentAt(customTestChecker(), DefaultsPlace{Before: []Stage{stages[0]}, Later: []Stage{later}})
	if got := outcomes[1].Then.To; !reflect.DeepEqual(got, []string{"writer"}) {
		t.Fatalf("redo goes to %v, want [writer]", got)
	}
}

// TestWithoutTheOldVersionOnlyThePinMoves
func TestWithoutTheOldVersionOnlyThePinMoves(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	updated := DefaultsUpdate(belt.Stages[1], nil, agentUpdateNewer(), belt.Stages[:1], nil, TriggerKindIssues)
	if updated.Agent == nil || updated.Agent.Version != 2 {
		t.Fatalf("agent = %+v, want version 2", updated.Agent)
	}
	if updated.Name != "Checker" {
		t.Fatalf("name = %q, want Checker", updated.Name)
	}
	if updated.Slots != 2 {
		t.Fatalf("slots = %d, want 2", updated.Slots)
	}
	if updated.Thinking != nil {
		t.Fatalf("thinking = %v, want none", *updated.Thinking)
	}
}

// TestAGotoGoesBackToTheNearestStageWithTheAgent
func TestAGotoGoesBackToTheNearestStageWithTheAgent(t *testing.T) {
	t.Parallel()
	belt := customTestBelt(customTestWriter(), customTestChecker())
	if got := belt.Stages[1].Outcomes[1].Then.To; !reflect.DeepEqual(got, []string{"writer"}) {
		t.Fatalf("redo goes to %v, want [writer]", got)
	}
	if warnings := placeholderTestWarnings(belt); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

// TestABriefReadsAnEarlierAgentsOutputByItsKey
func TestABriefReadsAnEarlierAgentsOutputByItsKey(t *testing.T) {
	t.Parallel()
	writer, checker := customTestWriter(), customTestChecker()
	run := briefTestSampleRun(customTestBelt(writer, checker))
	briefTestFinish(&run, "done")
	run.Attempts[0].Folder = "01-writer-1"
	context := BriefContext{RunFolder: "/runs/r", AttemptFolder: "/runs/r/02-checker-1", Base: "main", Branch: "b",
		Agents: map[AgentRef]Agent{writer.Ref(): writer, checker.Ref(): checker}}
	text := BriefRender(run, *run.Stage(), context)
	if want := "Check /runs/r/01-writer-1/draft.md and write /runs/r/02-checker-1/check.md."; !strings.HasPrefix(text, want) {
		t.Fatalf("brief starts %q, want %q", text, want)
	}
}

// TestAStageUsesThePinnedVersionsBrief
func TestAStageUsesThePinnedVersionsBrief(t *testing.T) {
	t.Parallel()
	writer, checker := customTestWriter(), customTestChecker()
	newer := checker
	newer.Version = 2
	newer.Brief = "Check it again."
	run := briefTestSampleRun(customTestBelt(writer, checker))
	context := BriefContext{RunFolder: "/runs/r", AttemptFolder: "/runs/r/02-checker-1", Base: "main", Branch: "b",
		Agents: map[AgentRef]Agent{writer.Ref(): writer, checker.Ref(): checker, newer.Ref(): newer}}
	text := BriefRender(run, run.Stages[1], context)
	if !strings.HasPrefix(text, "Check ") {
		t.Fatalf("brief starts %q, want the pinned version's brief", text)
	}
	if strings.HasPrefix(text, "Check it again.") {
		t.Fatalf("brief uses the newer version: %q", text)
	}
}

// TestStagesTakeOneValidSuggestionAndDoNotShareTheLibraryNamespace
func TestStagesTakeOneValidSuggestionAndDoNotShareTheLibraryNamespace(t *testing.T) {
	t.Parallel()
	legacy := SampleAgent("build")
	legacy.Key = "Bad Name"
	if warnings := ValidationAgentWarnings(legacy, nil); len(warnings) != 0 {
		t.Fatalf("legacy key warns %q", warnings)
	}
	stage := DefaultsStageForAgent(legacy, nil, nil, nil, TriggerKindIssues)
	if stage.Key == nil || *stage.Key != "build" {
		t.Fatalf("key = %v, want build", stage.Key)
	}

	reserved := legacy
	reserved.ID = "output"
	reserved.Key = "output"
	if key := DefaultsStageForAgent(reserved, nil, nil, nil, TriggerKindIssues).Key; key == nil || *key != "output-stage" {
		t.Fatalf("reserved key = %v, want output-stage", key)
	}

	first := SampleAgent("build")
	first.Key = "shared"
	second := SampleAgent("build")
	second.ID = "second"
	second.Name = "Second"
	second.Key = first.Key
	if warnings := ValidationAgentWarnings(first, nil); len(warnings) != 0 {
		t.Fatalf("first warns %q", warnings)
	}
	if warnings := ValidationAgentWarnings(second, []Agent{first}); len(warnings) != 0 {
		t.Fatalf("second warns %q", warnings)
	}
}

// DefaultsUpdate is stage, between before and later, moved from version old of
// its agent to newAgent (a nil old moves only the pin). Callers pass
// TriggerKindIssues for the default kind.
func DefaultsUpdate(stage Stage, old *Agent, newAgent Agent, before, later []Stage, kind TriggerKind) Stage {
	return DefaultsUpdateAt(stage, old, newAgent, DefaultsPlace{Before: before, Later: later}, kind)
}
