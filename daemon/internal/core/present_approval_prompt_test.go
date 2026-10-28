package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApprovalPromptRequiresTaskInputRatherThanAnOrdinaryDecision(t *testing.T) {
	t.Parallel()
	gate := NewStage(StageArgs{ID: "prompt", Type: StageTypeApprove, Outcomes: []Outcome{
		{ID: "refine", Name: "refine", Then: NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})},
		{ID: "direct", Name: "direct", Then: NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"build"}, WithOutput: true})},
		{ID: "skip", Name: "skip", Then: NewThen(ThenArgs{Action: ThenActionDrop})},
	}})
	run := Run{Subject: Subject{Kind: SubjectKindJira, Key: "TEST-1", Title: "Source title", Body: "Source description"}}
	prompt := ApprovalPromptOf(run, gate, false)
	if prompt == nil || prompt.NoteKey != "refine" || prompt.Include {
		t.Fatalf("prompt = %+v, want shared input with context excluded", prompt)
	}
	if len(prompt.Context) == 0 || !strings.Contains(prompt.Context[len(prompt.Context)-1].Text, "Source description") {
		t.Fatalf("missing source description: %+v", prompt.Context)
	}
	if ApprovalPromptOf(run, gate, true) != nil {
		t.Fatal("an approval with agent output became a prompt editor")
	}
	run.IncludeSubject = new(true)
	if !ApprovalPromptOf(run, gate, false).Include {
		t.Fatal("the saved context choice was lost")
	}
	gate.Outcomes[0].Then.WithOutput = false
	if ApprovalPromptOf(run, gate, false) != nil {
		t.Fatal("an ordinary approval became a prompt editor")
	}
	gate.Outcomes = gate.Outcomes[2:]
	if ApprovalPromptOf(run, gate, false) != nil {
		t.Fatal("a drop-only gate became a prompt editor")
	}
}

func TestPromptContextChoiceSurvivesDecisionAndStorage(t *testing.T) {
	t.Parallel()
	belt := engineIssueBelt()
	belt.Stages = append([]Stage{NewStage(StageArgs{ID: "prompt", Type: StageTypeApprove, Outcomes: []Outcome{
		{ID: "go", Name: "go", Then: NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})},
	}})}, belt.Stages...)
	for _, include := range []bool{false, true} {
		run := engineStartRun(belt)
		EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "invalid", IncludeSubject: new(include)}, engineT0)
		if run.IncludeSubject != nil {
			t.Fatal("invalid decision changed source consent")
		}
		EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "go", Note: new("My task"), IncludeSubject: new(include)}, engineT0)
		encoded, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		var saved Run
		if err := json.Unmarshal(encoded, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.IncludeSubject == nil || *saved.IncludeSubject != include || ValueOf(saved.Note) != "My task" {
			t.Fatalf("saved choice or prompt lost: include=%v, note=%v", saved.IncludeSubject, saved.Note)
		}
	}
}
