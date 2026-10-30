package coordinator

import (
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

func TestAPromptStageTakesItsSourceFromTheDecisionOrTheStage(t *testing.T) {
	t.Parallel()
	for _, source := range []*core.SourceSharing{nil, new(core.SourceSharingWithheld), new(core.SourceSharingShared)} {
		t.Run(string(core.ValueOr(source, "stage default")), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, harnessOptions{})
			c := h.Coord()
			belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
			prompt := core.DefaultsStage(core.StageTypePrompt, nil, belt.Stages, belt.Stages)
			belt.Stages = append([]core.Stage{prompt}, belt.Stages...)
			run := core.NewRun(core.RunArgs{Belt: belt, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 1, Title: "Source title"}, Now: testkitNow()})
			core.EngineStart(&run, testkitNow())
			h.putRun(run)
			facts := c.approvalFacts(run, prompt)
			if facts.Prompt == nil || facts.Document != nil || facts.Prompt.Source != core.SourceSharingWithheld {
				t.Fatalf("facts = %+v, want the prompt editor starting withheld", facts)
			}
			if _, err := c.DecideCommand(run.ID, "started", new("  "), source); err == nil {
				t.Fatal("a blank prompt was accepted")
			}
			after, err := c.DecideCommand(run.ID, "started", new("Do my task"), source)
			if err != nil {
				t.Fatal(err)
			}
			want := core.ValueOr(source, core.SourceSharingWithheld)
			saved := h.storage.LoadRuns()
			if after.Sharing != want || len(saved) != 1 || saved[0].Sharing != want || core.ValueOf(saved[0].Task) != "Do my task" {
				t.Fatalf("sharing %q, saved %+v, want %q with the task", after.Sharing, saved, want)
			}
		})
	}
}

func TestOnlyAPromptStageTakesASourceChoice(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
	approval := core.DefaultsStage(core.StageTypeApprove, nil, belt.Stages, belt.Stages)
	belt.Stages = append([]core.Stage{approval}, belt.Stages...)
	run := core.NewRun(core.RunArgs{Belt: belt, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 1}, Now: testkitNow()})
	core.EngineStart(&run, testkitNow())
	h.putRun(run)
	if c.approvalFacts(run, approval).Prompt != nil {
		t.Fatal("an approval has a prompt editor")
	}
	if _, err := c.DecideCommand(run.ID, "approved", nil, new(core.SourceSharingShared)); err == nil {
		t.Fatal("an approval took a source choice")
	}
	if after, err := c.DecideCommand(run.ID, "approved", nil, nil); err != nil || after.Sharing != "" {
		t.Fatalf("approve: %v, sharing %q", err, after.Sharing)
	}
}

func TestApprovalDocumentFollowsTheGateVisit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	agent := func(id string) core.Stage {
		return core.NewStage(core.StageArgs{ID: id, Type: core.StageTypeAgent, Key: new(id), Writes: new(id + ".md")})
	}
	gate := core.NewStage(core.StageArgs{ID: "pick-fixes", Type: core.StageTypeApprove})
	run := core.Run{
		ID: "test:jira:TEST-1", BeltID: "test",
		Subject: core.Subject{Kind: core.SubjectKindJira, Key: "TEST-1"},
		Stages:  []core.Stage{agent("spec"), agent("feedback"), gate, agent("fixes")},
		Attempts: []core.Attempt{
			{ID: "spec-1", StageID: "spec", Folder: "spec-1"},
			{ID: "feedback-1", StageID: "feedback", Folder: "feedback-1", Leg: new("review")},
			{ID: "other-leg-1", StageID: "fixes", Folder: "other-leg-1", Leg: new("other")},
			{ID: "pick-fixes-1", StageID: "pick-fixes", Folder: "pick-fixes-1", Leg: new("review")},
			{ID: "fixes-1", StageID: "fixes", Folder: "fixes-1", Leg: new("review")},
		},
	}
	for _, attempt := range run.Attempts {
		if run.HasAgentStage(attempt.StageID) {
			stage := run.Stages[run.StageIndexOf(attempt.StageID)]
			writeText(t, filepath.Join(c.Paths().AttemptFolder(run, attempt), stage.Writes()), attempt.ID)
		}
	}
	check := func(want *string) {
		t.Helper()
		h.Turn(func() {
			got := c.approvalFacts(run, gate).Document
			if !core.SamePointee(got, want) {
				t.Errorf("approval document = %v, want %v", core.ValueOf(got), core.ValueOf(want))
			}
		})
	}
	check(new("feedback-1"))
	run.Attempts = append(run.Attempts, core.Attempt{ID: "feedback-2", StageID: "feedback", Folder: "feedback-2", Leg: new("review")})
	writeText(t, filepath.Join(c.Paths().AttemptFolder(run, run.Attempts[len(run.Attempts)-1]), "feedback.md"), "revised feedback")
	run.Attempts = append(run.Attempts, core.Attempt{ID: "pick-fixes-2", StageID: gate.ID, Folder: "pick-fixes-2", Leg: new("review")})
	check(new("revised feedback"))
	run.Attempts = run.Attempts[:1]
	check(nil)
	run.Attempts = append(run.Attempts, core.Attempt{ID: "pick-fixes-1", StageID: gate.ID})
	check(new("spec-1"))
	run.Attempts = run.Attempts[1:]
	check(nil)
}
