package coordinator

import (
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

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
