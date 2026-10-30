package core

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// promptTestBelt is the default issues belt with a prompt stage in front.
func promptTestBelt() Belt {
	belt := engineIssueBelt()
	prompt := DefaultsStage(StageTypePrompt, nil, belt.Stages, belt.Stages)
	belt.Stages = append([]Stage{prompt}, belt.Stages...)
	return belt
}

// promptTestDecide starts a run of promptTestBelt and writes task at its prompt stage.
func promptTestDecide(t *testing.T, task string, sharing SourceSharing) Run {
	t.Helper()
	run := engineStartRun(promptTestBelt())
	if run.State != RunStateWaiting || run.Stage().Type != StageTypePrompt {
		t.Fatalf("run = %s at %s, want waiting at the prompt", run.State, run.StageID)
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "started", Note: new(task), Sharing: sharing}, engineT0)
	return run
}

func TestAPromptStageMakesTheRunsTaskAndStartsWithheld(t *testing.T) {
	t.Parallel()
	run := promptTestDecide(t, "  Build only the export button.  ", "")
	if ValueOf(run.Task) != "Build only the export button." || run.Sharing != SourceSharingWithheld {
		t.Fatalf("task = %v, sharing = %q", run.Task, run.Sharing)
	}
	if run.StageID != "triage" || run.Note != nil {
		t.Fatalf("at %s with note %v, want at triage with no note: the prompt is the task", run.StageID, run.Note)
	}
	if detail := run.Attempts[0].Detail; ValueOf(detail) != "  Build only the export button.  " {
		t.Fatalf("the prompt stage's attempt keeps %v", detail)
	}
	shared := promptTestDecide(t, "Do it", SourceSharingShared)
	if shared.Sharing != SourceSharingShared {
		t.Fatalf("sharing = %q, want shared", shared.Sharing)
	}
}

func TestAPromptStageNeedsThePromptToMoveOnButNotToPark(t *testing.T) {
	t.Parallel()
	run := promptTestDecide(t, "   ", "")
	if run.StageID != "prompt" || run.State != RunStateWaiting || run.Task != nil {
		t.Fatalf("a blank prompt moved the run: %s at %s, task %v", run.State, run.StageID, run.Task)
	}
	stage := *run.Stage()
	if !stage.NeedsNote(stage.Outcomes[0]) || stage.NeedsNote(stage.Outcomes[1]) {
		t.Fatalf("needs note: start %v, park %v", stage.NeedsNote(stage.Outcomes[0]), stage.NeedsNote(stage.Outcomes[1]))
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "parked"}, engineT0)
	if run.State != RunStateParked || run.Task != nil {
		t.Fatalf("park: %s, task %v", run.State, run.Task)
	}
}

func TestEveryBriefAfterThePromptCarriesTheTask(t *testing.T) {
	t.Parallel()
	run := promptTestDecide(t, "Build only the export button.", "")
	ctx := briefTestContext()
	for _, stageID := range []string{"triage", "spec"} {
		if run.StageID != stageID {
			t.Fatalf("at %s, want %s", run.StageID, stageID)
		}
		text := BriefRender(run, *run.Stage(), ctx)
		if !strings.Contains(text, "## Your task") || !strings.Contains(text, "Build only the export button.") {
			t.Fatalf("%s's brief lacks the task:\n%s", stageID, text)
		}
		briefTestFinish(&run, "ready to spec")
	}
}

func TestAWithheldSourceKeepsItsReferenceAndWhereTheWorkHappens(t *testing.T) {
	t.Parallel()
	ctx := briefTestContext()
	subject := Subject{Kind: SubjectKindJira, Key: "PAY-12", Repo: "acme/billing", Title: "Secret title", Body: "Secret description",
		URL: "https://jira.example/PAY-12", Labels: []string{"base:release", "secret-label"}, Parent: new("PAY-1")}
	for _, sharing := range []SourceSharing{"", SourceSharingShared, SourceSharingWithheld} {
		run := promptTestDecide(t, "My task", sharing)
		run.Subject = subject
		withheld := sharing != SourceSharingShared
		// The AFK brief never named labels or the parent.
		afk := AfkBriefRender(run, AfkSituation{Options: []AfkOption{{ID: "leave", Action: AfkAction{Kind: AfkActionKindLeave}}}}, AfkBriefContext{})
		for index, text := range []string{BriefRender(run, *run.Stage(), ctx), BriefRerun(run, *run.Stage(), Attempt{}, ctx), afk} {
			details := []string{"Secret title", "Secret description", "https://jira.example/", "secret-label", "PAY-1;"}
			if index == 2 {
				details = details[:3]
			}
			for _, detail := range details {
				if strings.Contains(text, detail) == withheld {
					t.Fatalf("sharing %q: detail %q shared wrongly:\n%s", sharing, detail, text)
				}
			}
			if !strings.Contains(text, "PAY-12") || !strings.Contains(text, "My task") {
				t.Fatalf("sharing %q: the reference or the task is missing:\n%s", sharing, text)
			}
			if withheld && !strings.Contains(text, BriefCopyWithheld(SubjectKindJira)) {
				t.Fatalf("sharing %q: no word on the withheld details:\n%s", sharing, text)
			}
		}
		brief := BriefRender(run, *run.Stage(), ctx)
		for _, kept := range []string{"Its work starts from main", "The code is in acme/billing."} {
			if !strings.Contains(brief, kept) {
				t.Fatalf("sharing %q: brief lacks %q:\n%s", sharing, kept, brief)
			}
		}
		linking := *run.Stage()
		linking.Brief = "Link {issue}."
		if text := BriefRender(run, linking, ctx); !strings.Contains(text, "Link PAY-12 in acme/billing.") {
			t.Fatalf("sharing %q: {issue} is not the reference:\n%s", sharing, text)
		}
	}
}

func TestAPromptStageLooksLikeOne(t *testing.T) {
	t.Parallel()
	prompt := DefaultsStage(StageTypePrompt, nil, nil, nil)
	if chip := StagePanelKindChipOf(prompt, ""); chip != "YOUR PROMPT" {
		t.Fatalf("chip = %q", chip)
	}
	if look := StageLookOf(prompt, nil); look.Icon != StageIconPen {
		t.Fatalf("look = %+v", look)
	}
}

func TestAfkLeavesAPromptStageToYou(t *testing.T) {
	t.Parallel()
	run := engineStartRun(promptTestBelt())
	if situation := AfkSituationOf(run, AfkSettings{Enabled: true, Approve: true, Drop: true}); situation != nil {
		t.Fatalf("AFK may decide a prompt stage: %+v", situation)
	}
}

func TestPromptFactsStartFromTheStageThenTheRunsChoice(t *testing.T) {
	t.Parallel()
	run := engineStartRun(promptTestBelt())
	stage := *run.Stage()
	facts := PromptFactsOf(run, stage)
	if facts == nil || facts.Source != SourceSharingWithheld || facts.Task != nil {
		t.Fatalf("facts = %+v, want withheld with no task", facts)
	}
	var texts []string
	for _, block := range facts.Context {
		texts = append(texts, block.Text)
	}
	if text := strings.Join(texts, "\n"); !strings.Contains(text, "Export invoices as CSV") || !strings.Contains(text, "Add an export button.") {
		t.Fatalf("the context lacks the work item's details: %q", text)
	}
	stage.Source = new(SourceSharingShared)
	if PromptFactsOf(run, stage).Source != SourceSharingShared {
		t.Fatal("the stage's own choice is not the start")
	}
	run.Sharing, run.Task = SourceSharingWithheld, new("Earlier task")
	if facts := PromptFactsOf(run, stage); facts.Source != SourceSharingWithheld || ValueOf(facts.Task) != "Earlier task" {
		t.Fatalf("a visit back = %+v, want the run's choice and task", facts)
	}
	approval := DefaultsStage(StageTypeApprove, nil, nil, nil)
	if PromptFactsOf(run, approval) != nil {
		t.Fatal("an approval has a prompt editor")
	}
}

func TestAPromptStageMayComeFirstAndPassesNothingOn(t *testing.T) {
	t.Parallel()
	belt := promptTestBelt()
	warnings := vtWarnings(belt)
	if slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, "Prompt") }) {
		t.Fatalf("warnings = %v, want none about the prompt stage", warnings)
	}
	belt.Stages[0].Outcomes[0].Then.WithOutput = true
	if !slices.ContainsFunc(vtWarnings(belt), func(w string) bool { return strings.Contains(w, "passes its text on as a note") }) {
		t.Fatalf("no warning for a prompt outcome that passes a note on: %v", vtWarnings(belt))
	}
	approval := engineIssueBelt()
	approval.Stages = append([]Stage{DefaultsStage(StageTypeApprove, nil, approval.Stages, approval.Stages)}, approval.Stages...)
	if !slices.Contains(vtWarnings(approval), "Approval is the first stage, so you would approve issues before any agent has looked at them.") {
		t.Fatalf("an approval first is not warned about: %v", vtWarnings(approval))
	}
}

func TestAPromptStageSavesItsSourceAndOthersDropIt(t *testing.T) {
	t.Parallel()
	prompt := DefaultsStage(StageTypePrompt, nil, nil, nil)
	prompt.Source = new(SourceSharingShared)
	approval := DefaultsStage(StageTypeApprove, nil, nil, nil)
	approval.Source = new(SourceSharingShared)
	for _, stage := range []Stage{prompt, approval} {
		encoded, err := json.Marshal(stage)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Stage
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if (decoded.Source != nil) != (stage.Type == StageTypePrompt) {
			t.Fatalf("%s decodes source %v", stage.Type, decoded.Source)
		}
	}
	var bad Stage
	if err := json.Unmarshal([]byte(`{"id":"p","type":"prompt","name":"Prompt","outcomes":[],"source":"sometimes"}`), &bad); err == nil {
		t.Fatal("an unknown source decoded")
	}
}

// promptTestImplicitGate is an approval as #14 made a prompt editor of: every
// outcome that moves on passes a note on.
func promptTestImplicitGate() Stage {
	return NewStage(StageArgs{ID: "ask", Type: StageTypeApprove, Name: new("Ask"), Outcomes: []Outcome{
		{ID: "refine", Name: "refine", When: "Refine", Then: NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})},
		{ID: "skip", Name: "skip", When: "Skip", Then: NewThen(ThenArgs{Action: ThenActionDrop})},
	}})
}

func TestTheUpgradeMakesAFirstApprovalThatAskedForAPromptAPromptStage(t *testing.T) {
	t.Parallel()
	belt := engineIssueBelt()
	belt.Stages = append([]Stage{promptTestImplicitGate()}, belt.Stages...)
	upgraded := UpgradeBelt(belt, vtPinned(SampleAgents()))
	first := upgraded.Stages[0]
	if first.Type != StageTypePrompt || first.Name != "Ask" || first.Outcomes[0].Then.WithOutput || first.Source != nil {
		t.Fatalf("upgraded = %+v", first)
	}
	if belt.Stages[0].Type != StageTypeApprove {
		t.Fatal("the upgrade changed the belt it was given")
	}
	for name, stages := range map[string][]Stage{
		"an ordinary approval": append([]Stage{DefaultsStage(StageTypeApprove, nil, nil, nil)}, belt.Stages[1:]...),
		"not the first stage":  append(append([]Stage{}, belt.Stages[1:]...), promptTestImplicitGate()),
	} {
		if got := UpgradePromptStage(stages); slices.ContainsFunc(got, func(s Stage) bool { return s.Type == StageTypePrompt }) {
			t.Fatalf("%s became a prompt stage", name)
		}
	}
	run := NewRun(RunArgs{Belt: belt, Subject: engineIssueSubject(), Now: engineT0})
	if UpgradeRun(run, nil, nil).Stages[0].Type != StageTypePrompt {
		t.Fatal("a run's copy of the stage was left an approval")
	}
}

func TestARunDecidedAtAnApprovalThatAskedForAPromptKeepsItsChoice(t *testing.T) {
	t.Parallel()
	belt := engineIssueBelt()
	belt.Stages = append([]Stage{promptTestImplicitGate()}, belt.Stages...)
	for _, include := range []bool{false, true} {
		run := engineStartRun(belt)
		EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "refine", Note: new("My task")}, engineT0)
		encoded, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		fields["includeSubject"] = include
		legacy, _ := json.Marshal(fields)
		var decoded Run
		if err := json.Unmarshal(legacy, &decoded); err != nil {
			t.Fatal(err)
		}
		want := SourceSharingWithheld
		if include {
			want = SourceSharingShared
		}
		if decoded.Sharing != want || ValueOf(decoded.Task) != "My task" {
			t.Fatalf("include %v: sharing %q, task %v", include, decoded.Sharing, decoded.Task)
		}
	}
}
