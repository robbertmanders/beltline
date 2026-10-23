// Tests of saved stages: migrations, the stage keys written by stages, and
// the stage and backend parts of belts, with a literal stage.

package core

import (
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// btStageAgent is an agent stage built with NewStage.
func btStageAgent(id string, key *string) Stage {
	return NewStage(StageArgs{ID: id, Type: StageTypeAgent, Agent: &AgentRef{ID: id, Version: 1}, Key: key, Outcomes: []Outcome{}})
}

// TestAStageSavedBeforeTheLibraryUsesTheBuiltInAgentsFirstVersion reads a stage
// whose type names the built-in agent and whose agent key names the CLI.
func TestAStageSavedBeforeTheLibraryUsesTheBuiltInAgentsFirstVersion(t *testing.T) {
	t.Parallel()
	json := `{"id":"build","type":"build","name":"Build","agent":"copilot","model":"","workspace":"worktree",
		"writes":"evidence.md","slots":1,"brief":"","outcomes":[],"fallback":"hold"}`
	stage := btDecode[Stage](t, json)
	if stage.Type != StageTypeAgent {
		t.Errorf("type = %q, want agent", stage.Type)
	}
	if stage.Agent == nil || *stage.Agent != (AgentRef{ID: "build", Version: 1}) {
		t.Errorf("agent = %+v, want build version 1", stage.Agent)
	}
	if stage.Runner == nil || *stage.Runner != BackendCopilot {
		t.Errorf("runner = %v, want copilot", stage.Runner)
	}
	if stage.Thinking != nil {
		t.Errorf("thinking = %v, want none", *stage.Thinking)
	}
	if !stage.ChangesCode() {
		t.Error("changesCode = false, want true")
	}
	if stage.Writes() != "evidence.md" {
		t.Errorf("writes = %q, want evidence.md", stage.Writes())
	}
}

// TestApprovalsAndWatchesKeepTheirType reads an approval and a watch with no agent or runner.
func TestApprovalsAndWatchesKeepTheirType(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"watch","type":"watch","name":"Merge","outcomes":[]}`)
	if stage.Type != StageTypeWatch {
		t.Errorf("type = %q, want watch", stage.Type)
	}
	if stage.Agent != nil || stage.Runner != nil {
		t.Errorf("agent %+v and runner %v, want none", stage.Agent, stage.Runner)
	}
}

// TestStagesSavedBeforeThinkingHaveNoLevel reads an agent stage with no thinking key.
func TestStagesSavedBeforeThinkingHaveNoLevel(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"build","type":"agent","name":"Build","agent":{"id":"build","version":1},"runner":"pi","outcomes":[]}`)
	if stage.Thinking != nil {
		t.Fatalf("thinking = %v, want none", *stage.Thinking)
	}
}

// TestSavedThinkingIsDecoded reads a saved thinking level.
func TestSavedThinkingIsDecoded(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"build","type":"agent","name":"Build","thinking":"max","outcomes":[]}`)
	if stage.Thinking == nil || *stage.Thinking != ThinkingMax {
		t.Fatalf("thinking = %v, want max", stage.Thinking)
	}
}

// TestNewStagesRoundTrip round-trips a literal stage, then the stage DefaultsStageForAgent builds for the checker.
func TestNewStagesRoundTrip(t *testing.T) {
	t.Parallel()
	stage := btStageAgent("checker", new("checker"))
	stage.Outcomes = []Outcome{btOutcome("done")}
	got := btDecode[Stage](t, btEncode(t, stage))
	if d := testkit.Diff(stage, got); d != "" {
		t.Fatalf("round trip differs:\n%s", d)
	}
	// The stage is built with DefaultsStageForAgent.
	defaults := DefaultsStageForAgent(customTestChecker(), nil, nil, nil, TriggerKindIssues)
	if d := testDiffEmpty(defaults, btDecode[Stage](t, btEncode(t, defaults))); d != "" {
		t.Fatalf("a new stage differs after a round trip:\n%s", d)
	}
}

// TestSavedAgentAndStageIdentifiersStayStrings_Belt checks the belt part of saved identifiers.
func TestSavedAgentAndStageIdentifiersStayStrings_Belt(t *testing.T) {
	t.Parallel()
	stage := btStageAgent("triage", nil)
	stage.Model = "claude-opus-5-5-250k"
	belt := btBelt(stage)
	if got := btDecode[Belt](t, btEncode(t, belt)).Stages[0].Model; got != "claude-opus-5-5-250k" {
		t.Fatalf("model = %q, want claude-opus-5-5-250k", got)
	}
}

// TestAStageWritesTheFileItsKeyNames covers the writes getter and setter, and the key setter.
func TestAStageWritesTheFileItsKeyNames(t *testing.T) {
	t.Parallel()
	stage := btStageAgent("custom", new("custom"))
	if stage.Key == nil || *stage.Key != "custom" {
		t.Fatalf("key = %v, want custom", stage.Key)
	}
	if stage.Writes() != "custom.md" {
		t.Fatalf("writes = %q, want custom.md", stage.Writes())
	}
	stage.SetKey(new("notes"))
	if stage.Writes() != "notes.md" {
		t.Fatalf("after key notes, writes = %q", stage.Writes())
	}
	stage.SetKey(nil)
	if stage.Writes() != "output.md" {
		t.Fatalf("after no key, writes = %q, want output.md", stage.Writes())
	}
}

// TestABeltDropsAFileNamedApartFromTheKeyButARunKeepsIt checks that a belt
// drops a file name saved apart from its key, and a stage on its own keeps it.
func TestABeltDropsAFileNamedApartFromTheKeyButARunKeepsIt(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "spec", Type: StageTypeAgent, Name: new("Spec"), Key: new("notes"),
		Writes: new("output.md"), Outcomes: []Outcome{btOutcome("done")}})
	if stage.Writes() != "output.md" {
		t.Fatalf("writes = %q, want output.md", stage.Writes())
	}
	saved := btEncode(t, btBelt(stage))
	if got := btDecode[Belt](t, saved).Stages[0].Writes(); got != "notes.md" {
		t.Fatalf("belt writes = %q, want notes.md", got)
	}
	// A run in progress still finds what its attempts wrote.
	if got := btDecode[Stage](t, btEncode(t, stage)).Writes(); got != "output.md" {
		t.Fatalf("stage writes = %q, want output.md", got)
	}
	stage.SetKey(new("draft"))
	if stage.Writes() != "draft.md" {
		t.Fatalf("after key draft, writes = %q, want draft.md", stage.Writes())
	}
}

// TestABeltSavedBeforeKeysKeepsItsFileUntilConverted checks the decode and writes of a belt saved before keys.
func TestABeltSavedBeforeKeysKeepsItsFileUntilConverted(t *testing.T) {
	t.Parallel()
	json := `{"id":"custom","type":"agent","name":"Custom","agent":{"id":"custom","version":1},"writes":"output.md","outcomes":[]}`
	stage := btDecode[Stage](t, json)
	if stage.Key != nil {
		t.Fatalf("key = %v, want none", *stage.Key)
	}
	legacy := btBelt(stage)
	if got := btDecode[Belt](t, btEncode(t, legacy)).Stages[0].Writes(); got != "output.md" {
		t.Fatalf("writes = %q, want output.md", got)
	}
}

// TestStageMarshalExactBytes checks the keys Stage.encode(to:) writes, and the
// omitted alternates, key-less route and watch.
func TestStageMarshalExactBytes(t *testing.T) {
	t.Parallel()
	runner := BackendPi
	workspace := WorkspaceWorktree
	stage := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Name: new("Build"), Agent: &AgentRef{ID: "build", Version: 1},
		Key: new("build"), Runner: &runner, Model: "m", Workspace: &workspace, Slots: new(1)})
	want := `{"id":"build","type":"agent","name":"Build","agent":{"id":"build","version":1},"key":"build","runner":"pi","model":"m",` +
		`"workspace":"worktree","writes":"build.md","slots":1,"brief":"","outcomes":[],"fallback":"hold"}`
	if got := btEncode(t, stage); got != want {
		t.Fatalf("stage JSON\n got %s\nwant %s", got, want)
	}
}

// TestStageMarshalWritesTheAlternatesOnlyWhenThereAreAny checks the alternates key.
func TestStageMarshalWritesTheAlternatesOnlyWhenThereAreAny(t *testing.T) {
	t.Parallel()
	stage := btStageAgent("build", nil)
	if strings.Contains(btEncode(t, stage), "alternates") {
		t.Fatal("empty alternates were written")
	}
	stage.Alternates = []SessionSettings{{Model: "other"}}
	if !strings.Contains(btEncode(t, stage), `"alternates":[{"model":"other"}]`) {
		t.Fatalf("alternates not written: %s", btEncode(t, stage))
	}
}

// TestStageDefaultsWhenKeysAreMissing covers the defaults Stage.init(from:) applies.
func TestStageDefaultsWhenKeysAreMissing(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"x","type":"agent","name":"X","outcomes":[]}`)
	if stage.Workspace != WorkspaceReadOnly || stage.Slots != 2 || stage.Fallback != FallbackHold {
		t.Errorf("workspace %q, slots %d, fallback %q", stage.Workspace, stage.Slots, stage.Fallback)
	}
	if stage.Model != "" || stage.Brief != "" || stage.Key != nil || stage.Thinking != nil || stage.Route != nil || stage.Watch != nil {
		t.Errorf("model %q brief %q key %v thinking %v route %v watch %v", stage.Model, stage.Brief, stage.Key, stage.Thinking, stage.Route, stage.Watch)
	}
	if stage.Writes() != "output.md" {
		t.Errorf("writes = %q, want output.md", stage.Writes())
	}
	if len(stage.Alternates) != 0 {
		t.Errorf("alternates = %v, want none", stage.Alternates)
	}
}

// TestStageKeyIsIgnoredOnApprovals checks that only agent stages read a key.
func TestStageKeyIsIgnoredOnApprovals(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"a","type":"approve","name":"A","key":"k","runner":"claude","outcomes":[]}`)
	if stage.Key != nil {
		t.Errorf("key = %v on an approval, want none", *stage.Key)
	}
	if stage.Runner == nil || *stage.Runner != BackendClaude {
		t.Errorf("runner = %v, want claude (decoded for any type)", stage.Runner)
	}
	if stage.Writes() != "decision" {
		t.Errorf("writes = %q, want decision", stage.Writes())
	}
}

// TestStageWritesEqualToTheKeyNameIsNotSaved checks that a writes name equal to
// the key's file is not kept apart from the key.
func TestStageWritesEqualToTheKeyNameIsNotSaved(t *testing.T) {
	t.Parallel()
	stage := btDecode[Stage](t, `{"id":"n","type":"agent","name":"N","key":"notes","writes":"notes.md","outcomes":[]}`)
	if stage.Writes() != "notes.md" {
		t.Fatalf("writes = %q, want notes.md", stage.Writes())
	}
	stage.SetKey(new("draft"))
	if stage.Writes() != "draft.md" {
		t.Fatalf("after key draft, writes = %q, want draft.md", stage.Writes())
	}
}

// TestStageRejectsAnUnknownType checks the error Stage.init(from:) throws for a type it does not know.
func TestStageRejectsAnUnknownType(t *testing.T) {
	t.Parallel()
	var stage Stage
	if err := btUnmarshal(`{"id":"x","type":"banana","name":"X","outcomes":[]}`, &stage); err == nil {
		t.Fatal("unknown stage type decoded without error")
	}
}

// TestStageOutcomeLookupNormalisesNames covers Stage.outcome(named:), which normalises names.
func TestStageOutcomeLookupNormalisesNames(t *testing.T) {
	t.Parallel()
	stage := btStageAgent("s", nil)
	stage.Outcomes = []Outcome{btOutcome("Done")}
	if got := stage.Outcome("  done\n"); got == nil || got.Name != "Done" {
		t.Fatalf("Outcome(done) = %+v", got)
	}
	if stage.Outcome("missing") != nil {
		t.Fatal("Outcome(missing) found something")
	}
}

// TestBackendThinkingClampsToTheHighestLevelBelow covers Backend.thinking(_:).
func TestBackendThinkingKeepsTheLevel(t *testing.T) {
	t.Parallel()
	if got := BackendCodex.Thinking(new(ThinkingXHigh)); got == nil || *got != ThinkingXHigh {
		t.Fatalf("codex xhigh = %v", got)
	}
	if BackendPi.Thinking(nil) != nil {
		t.Fatal("nil thinking gave a level")
	}
}

// TestStageWorkerKeyAndFile covers StageFile and WorkerKey.
func TestStageWorkerKeyAndFile(t *testing.T) {
	t.Parallel()
	if got := StageFile(new("spec"), StageTypeAgent); got != "spec.md" {
		t.Errorf("StageFile(spec) = %q", got)
	}
	if got := StageFile(new(""), StageTypeAgent); got != "output.md" {
		t.Errorf("StageFile(empty key) = %q, want output.md", got)
	}
	if got := StageFile(new("x"), StageTypeWatch); got != "pull request" {
		t.Errorf("StageFile(watch) = %q, want the output name", got)
	}
	stage := btStageAgent("triage", nil)
	if got := stage.WorkerKey(); got != "agent:triage:pi" {
		t.Errorf("WorkerKey = %q, want agent:triage:pi", got)
	}
}

// btUnmarshal decodes data into v through jsonx, for tests that expect an error.
func btUnmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

// TestBeltsAndRunningAttemptsSavedBeforeThinkingStillOpen
func TestBeltsAndRunningAttemptsSavedBeforeThinkingStillOpen(t *testing.T) {
	t.Parallel()
	belt := DefaultsBelt([]string{"a/b"}, "agent", DefaultsBuiltInAgents(), true)
	for _, stage := range btDecode[Belt](t, btEncode(t, belt)).Stages {
		if stage.Thinking != nil {
			t.Errorf("belt stage %s thinking = %v, want none", stage.ID, *stage.Thinking)
		}
	}
	run := engineStartRun(belt)
	EngineAdvance(&run, engineSessionStart("s"), testT0)
	reopened := btDecode[Run](t, btEncode(t, run))
	if d := testDiffEmpty(run, reopened); d != "" {
		t.Errorf("reopened run differs:\n%s", d)
	}
	if reopened.State != RunStateRunning {
		t.Errorf("state = %s, want running", reopened.State)
	}
	for _, stage := range reopened.Stages {
		if stage.Thinking != nil {
			t.Errorf("run stage %s thinking = %v, want none", stage.ID, *stage.Thinking)
		}
	}

	// The test starts from DefaultsBelt and a run the engine has started.
	defaults := DefaultsBelt([]string{"a/b"}, "agent", DefaultsBuiltInAgents(), true)
	for _, stage := range btDecode[Belt](t, btEncode(t, defaults)).Stages {
		if stage.Thinking != nil {
			t.Errorf("default belt stage %s thinking = %v, want none", stage.ID, *stage.Thinking)
		}
	}
	started := NewRun(RunArgs{Belt: defaults, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142,
		Title: "Export invoices as CSV", Body: "Add an export button.", IsOpen: true, Blockers: []string{}}, Now: testT0})
	EngineStart(&started, testT0)
	EngineAdvance(&started, Event{Kind: EventKindSessionStarted, Session: "s", SessionID: "id", Folder: "f"}, testT0)
	engineReopened := btDecode[Run](t, btEncode(t, started))
	if d := testDiffEmpty(started, engineReopened); d != "" {
		t.Errorf("reopened engine run differs:\n%s", d)
	}
	if engineReopened.State != RunStateRunning {
		t.Errorf("engine run state = %s, want running", engineReopened.State)
	}
	for _, stage := range engineReopened.Stages {
		if stage.Thinking != nil {
			t.Errorf("engine run stage %s thinking = %v, want none", stage.ID, *stage.Thinking)
		}
	}
}

// TestOldBeltsKeepDerivedKeysUntilSaveAndOldRunsKeepAliases
// Only the model part is here: a belt saved without stage keys loads with none,
// and a run without references keeps the aliases.
func TestOldBeltsKeepDerivedKeysUntilSaveAndOldRunsKeepAliases(t *testing.T) {
	t.Parallel()
	belt := btBelt(btStageAgent("spec", new("spec")), btStageAgent("build", new("build")))
	obj := testJSONMap(t, belt)
	for _, raw := range obj["stages"].([]any) {
		delete(raw.(map[string]any), "key")
	}
	loaded := btDecode[Belt](t, btEncode(t, obj))
	for _, stage := range loaded.Stages {
		if stage.Kind() == StageKindAgent && stage.Key != nil {
			t.Errorf("stage %s key = %q, want none", stage.ID, *stage.Key)
		}
	}
	run := NewRun(RunArgs{Belt: loaded, Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: 1, Title: "Old"}, Now: testT0})
	run.References = nil
	for index := range run.Stages {
		run.Stages[index].Key = nil
	}
	again := btDecode[Run](t, btEncode(t, run))
	if again.UsesStageKeys() {
		t.Error("a run without references uses stage keys")
	}
}
