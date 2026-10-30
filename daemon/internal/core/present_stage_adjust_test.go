// Tests of stage adjustment: the pure parts (eligibility,
// backends and effort, switching, the agent help, and a run saved before the
// history).

package core

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// stageAdjustTestBelt is a belt of one agent stage, an approval and a watch, the
// stage kinds the eligibility test covers.
func stageAdjustTestBelt() Belt {
	approve := NewStage(StageArgs{ID: "approve", Type: StageTypeApprove, Outcomes: []Outcome{}})
	watch := NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Outcomes: []Outcome{}})
	agent := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Outcomes: []Outcome{}})
	return NewBeltFromTrigger(BeltFromTriggerArgs{ID: "test", Name: "Test", Trigger: Trigger{Kind: TriggerKindIssues}, Repos: []string{"acme/billing"}, Stages: []Stage{agent, approve, watch}})
}

// TestEligibilityCoversEveryStateAndKind
func TestEligibilityCoversEveryStateAndKind(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: stageAdjustTestBelt(), Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 1, Title: "Test"}, Now: testT0})
	for _, state := range AllRunState {
		run.State = state
		for _, stage := range run.Stages {
			want := state.IsActive() && stage.Kind() == StageKindAgent
			if got := StageAdjustCanAdjust(stage, run); got != want {
				t.Fatalf("state %s, stage %s: canAdjust = %v, want %v", state, stage.ID, got, want)
			}
		}
	}
	absent := Stage{ID: "absent", Type: StageTypeAgent, Outcomes: []Outcome{}}
	run.State = RunStateQueued
	if StageAdjustCanAdjust(absent, run) {
		t.Fatal("a stage the run does not have can be adjusted")
	}
	var gatesAndWatches []Stage
	for _, stage := range run.Stages {
		if stage.Kind() == StageKindGate || stage.Kind() == StageKindWatch {
			gatesAndWatches = append(gatesAndWatches, stage)
		}
	}
	if len(gatesAndWatches) == 0 {
		t.Fatal("the fixture has no approval or watch")
	}
	for _, state := range []RunState{RunStateShipped, RunStateDropped, RunStateRunning, RunStateHeld, RunStateQueued} {
		run.State = state
		for _, stage := range gatesAndWatches {
			if StageAdjustCanAdjust(stage, run) {
				t.Fatalf("state %s: %s can be adjusted", state, stage.ID)
			}
		}
		for _, stage := range run.Stages {
			if stage.Kind() == StageKindAgent && StageAdjustCanAdjust(stage, run) != state.IsActive() {
				t.Fatalf("state %s: agent stage canAdjust disagrees with the state", state)
			}
		}
	}
}

// TestRunsOnStartsAtTheStageBackendAndOffersEveryBackend
// The stage is built without a runner, as a stage saved before runners were recorded.
func TestRunsOnStartsAtTheStageBackendAndOffersEveryBackend(t *testing.T) {
	t.Parallel()
	var labels []string
	for _, backend := range AllBackend {
		labels = append(labels, backend.Label())
	}
	if want := []string{"Copilot CLI", "Pi", "Claude Code", "Codex CLI", "Cursor CLI"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	stage := NewStage(StageArgs{ID: "triage", Type: StageTypeAgent, Outcomes: []Outcome{{Name: "done", When: "", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})}}})
	// The stage is encoded, its runner key removed and decoded again.
	data, err := jsonx.Marshal(stage)
	if err != nil {
		t.Fatal(err)
	}
	members, err := jsonx.Members(data)
	if err != nil {
		t.Fatal(err)
	}
	delete(members, "runner")
	data, err = jsonx.Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	stage = Stage{}
	if err := json.Unmarshal(data, &stage); err != nil {
		t.Fatalf("decode without a runner: %v", err)
	}
	if stage.Runner != nil {
		t.Fatal("a stage without a runner has one")
	}
	if got := StageAdjustBackend(stage); got != BackendPi {
		t.Fatalf("backend = %s, want pi", got)
	}
	codex := BackendCodex
	stage.Runner = &codex
	if got := StageAdjustBackend(stage); got != BackendCodex {
		t.Fatalf("backend = %s, want codex", got)
	}
	if got, want := StageAdjustSupportedEffort(BackendCodex), []Thinking{ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax}; !reflect.DeepEqual(got, want) {
		t.Fatalf("codex effort = %v, want %v", got, want)
	}
	if got := StageAdjustSupportedEffort(BackendPi); !reflect.DeepEqual(got, AllThinking) {
		t.Fatalf("pi effort = %v, want every level", got)
	}
	want := "Changes apply to the next fresh session. Answering or continuing an existing session keeps its original backend, model, and effort."
	if StageAdjustFreshSessionNote != want {
		t.Fatalf("fresh session note = %q", StageAdjustFreshSessionNote)
	}
}

// TestSwitchingBackendKeepsOnlyAnExactAllowedModelAndSupportedEffort
func TestSwitchingBackendKeepsOnlyAnExactAllowedModelAndSupportedEffort(t *testing.T) {
	t.Parallel()
	for _, thinking := range []Thinking{ThinkingXHigh, ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			level := thinking
			brief := "Keep this brief"
			pi := StageAdjustDraft{Runner: BackendPi, Model: "shared", Thinking: &level, Brief: brief}
			disjoint := StageAdjustSwitching(pi, BackendCodex, []string{"codex-only"})
			if want := (StageAdjustDraft{Runner: BackendCodex, Model: "", Thinking: &level, Brief: brief}); !reflect.DeepEqual(disjoint, want) {
				t.Fatalf("disjoint = %+v, want %+v", disjoint, want)
			}
			// Switching back follows the same rule and does not restore the cleared model.
			back := StageAdjustSwitching(disjoint, BackendPi, []string{"shared", "pi-only"})
			if want := (StageAdjustDraft{Runner: BackendPi, Model: "", Thinking: &level, Brief: brief}); !reflect.DeepEqual(back, want) {
				t.Fatalf("back = %+v, want %+v", back, want)
			}
			overlap := StageAdjustSwitching(pi, BackendCodex, []string{"other", "shared"})
			if want := (StageAdjustDraft{Runner: BackendCodex, Model: "shared", Thinking: &level, Brief: brief}); !reflect.DeepEqual(overlap, want) {
				t.Fatalf("overlap = %+v, want %+v", overlap, want)
			}
			if got := StageAdjustSwitching(pi, BackendCodex, []string{"shared-extra"}); got.Model != "" {
				t.Fatalf("model = %q, want cleared", got.Model)
			}
			high := ThinkingHigh
			kept := StageAdjustSwitching(StageAdjustDraft{Runner: BackendPi, Model: "shared", Thinking: &high, Brief: brief}, BackendCodex, []string{"shared"})
			if kept.Thinking == nil || *kept.Thinking != ThinkingHigh || kept.Model != "shared" || kept.Brief != brief {
				t.Fatalf("kept = %+v, want high, shared and the brief", kept)
			}
		})
	}
}

// TestEditorBriefAndPlaceholdersMatchPinnedAgent checks the editor brief of a pinned agent.
// The stage is the first stage of
// SampleBelt, which is the same pinned Triage stage.
func TestEditorBriefAndPlaceholdersMatchPinnedAgent(t *testing.T) {
	t.Parallel()
	agents := SampleAgents()
	stage := SampleBelt([]string{"a/b"}, "agent", agents, true).Stages[0]
	var agent *Agent
	for i := range agents {
		if stage.Agent != nil && agents[i].Ref() == *stage.Agent {
			agent = &agents[i]
		}
	}
	if agent == nil {
		t.Fatal("the first stage has no pinned agent")
	}
	if got := StageAdjustEffectiveBrief(stage, agent); got != agent.Brief {
		t.Fatalf("effective brief %q, want the agent's", got)
	}
	stage.Brief = "Own brief"
	if got := StageAdjustEffectiveBrief(stage, agent); got != "Own brief" {
		t.Fatalf("own brief not used: %q", got)
	}
	own := agent.Brief
	stage.Brief = StageAdjustOwnBrief(&own, agent)
	if stage.Brief != "" {
		t.Fatalf("own brief of the agent's text = %q, want empty", stage.Brief)
	}
	if got := StageAdjustEffectiveBrief(stage, agent); got != agent.Brief {
		t.Fatalf("effective brief after the round trip %q, want the agent's", got)
	}
	want := append([]string{"triage", "spec", "draft"}, BriefFixedPlaceholders...)
	if got := StageAdjustPlaceholders([]string{"triage", "spec", "triage"}, []string{"draft", "spec"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("placeholders %v, want %v", got, want)
	}
}

// TestAgentHelpOffersOnlyFixedNames
func TestAgentHelpOffersOnlyFixedNames(t *testing.T) {
	t.Parallel()
	help := PlaceholderHelpAgent()
	var names []string
	for _, entry := range help.Entries {
		names = append(names, entry.Name)
	}
	if !reflect.DeepEqual(names, BriefFixedPlaceholders) {
		t.Fatalf("names = %v, want %v", names, BriefFixedPlaceholders)
	}
	for _, entry := range help.Entries {
		if entry.Name == "output" && !strings.Contains(entry.Description, "current result") {
			t.Fatalf("output description = %q", entry.Description)
		}
	}
}

// adjustmentsKeyPattern matches the adjustments member of a saved run, with the
// comma that joins it to the member before it or, when it comes first, after it.
var adjustmentsKeyPattern = regexp.MustCompile(`(,"adjustments":\[[^\]]*\])|("adjustments":\[[^\]]*\],)`)

// TestOldRunsWithoutHistoryDecode
// The history member is cut out of the saved text, as a run saved before it has none.
func TestOldRunsWithoutHistoryDecode(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: stageAdjustTestBelt(), Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 1, Title: "Test"}, Now: testT0})
	data, err := jsonx.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if !adjustmentsKeyPattern.Match(data) {
		t.Fatalf("a saved run has no adjustments member: %s", data)
	}
	old := adjustmentsKeyPattern.ReplaceAll(data, nil)
	if adjustmentsKeyPattern.Match(old) {
		t.Fatal("the adjustments member is still there")
	}
	var decoded Run
	if err := json.Unmarshal(old, &decoded); err != nil {
		t.Fatalf("a run without adjustments does not decode: %v", err)
	}
	if len(decoded.Adjustments) != 0 {
		t.Fatalf("adjustments = %+v, want none", decoded.Adjustments)
	}
}

// TestStageAdjustChangedFieldsFollowTheCurrentSettings pins which settings an
// adjustment changes, the backend it saves and the choice it takes, against what
// the stage's session runs with now.
func TestStageAdjustChangedFieldsFollowTheCurrentSettings(t *testing.T) {
	t.Parallel()
	agent := Stage{ID: "build", Type: StageTypeAgent, Outcomes: []Outcome{}, Brief: "own"}
	high := ThinkingHigh
	low := ThinkingLow
	codex := BackendCodex
	current := SessionSettings{Model: "m1", Thinking: &high}

	if got := StageAdjustChangedFields(current, agent, BackendPi, "m1", &high, "own"); got != nil {
		t.Fatalf("unchanged fields = %v, want nil", got)
	}
	if got := StageAdjustChangedFields(current, agent, BackendPi, "m2", &high, "own"); !reflect.DeepEqual(got, []AdjustmentField{AdjustmentFieldModel}) {
		t.Fatalf("model change = %v, want model", got)
	}
	if got := StageAdjustChangedFields(current, agent, BackendCodex, "m1", &high, "own"); !reflect.DeepEqual(got, []AdjustmentField{AdjustmentFieldBackend}) {
		t.Fatalf("backend change = %v, want backend", got)
	}
	if got := StageAdjustChangedFields(current, agent, BackendPi, "m1", &low, "own"); !reflect.DeepEqual(got, []AdjustmentField{AdjustmentFieldThinking}) {
		t.Fatalf("effort change = %v, want thinking", got)
	}
	if got := StageAdjustChangedFields(current, agent, BackendPi, "m1", nil, "own"); !reflect.DeepEqual(got, []AdjustmentField{AdjustmentFieldThinking}) {
		t.Fatalf("cleared effort = %v, want thinking", got)
	}
	if got := StageAdjustChangedFields(current, agent, BackendPi, "m1", &high, "other"); !reflect.DeepEqual(got, []AdjustmentField{AdjustmentFieldBrief}) {
		t.Fatalf("brief change = %v, want brief", got)
	}
	all := StageAdjustChangedFields(current, agent, BackendCodex, "m2", &low, "other")
	if want := []AdjustmentField{AdjustmentFieldBackend, AdjustmentFieldModel, AdjustmentFieldThinking, AdjustmentFieldBrief}; !reflect.DeepEqual(all, want) {
		t.Fatalf("all changed = %v, want %v", all, want)
	}

	// A session that already runs on codex compares against codex, not the stage's pi.
	onCodex := SessionSettings{Runner: &codex, Model: "m1", Thinking: &high}
	if got := StageAdjustChangedFields(onCodex, agent, BackendCodex, "m1", &high, "own"); got != nil {
		t.Fatalf("runner-held backend = %v, want nil", got)
	}

	if StageAdjustChangesRunner([]AdjustmentField{AdjustmentFieldBrief}) {
		t.Fatal("a brief-only change reports a runner change")
	}
	if !StageAdjustChangesRunner([]AdjustmentField{AdjustmentFieldBrief, AdjustmentFieldModel}) {
		t.Fatal("a model change does not report a runner change")
	}

	if got := StageAdjustTarget(&codex, SessionSettings{}, agent); got != BackendCodex {
		t.Fatalf("chosen backend = %s, want codex", got)
	}
	if got := StageAdjustTarget(nil, SessionSettings{Runner: &codex}, agent); got != BackendCodex {
		t.Fatalf("current runner = %s, want codex", got)
	}
	if got := StageAdjustTarget(nil, SessionSettings{}, agent); got != BackendPi {
		t.Fatalf("stage backend = %s, want pi", got)
	}

	choices := []ModelChoice{{ID: "a"}, {ID: "b"}}
	if got := StageAdjustChoice(choices, "b"); got == nil || got.ID != "b" {
		t.Fatalf("choice b = %+v, want b", got)
	}
	for _, id := range []string{"", "  ", "c"} {
		if got := StageAdjustChoice(choices, id); got != nil {
			t.Fatalf("choice %q = %+v, want nil", id, got)
		}
	}
}
