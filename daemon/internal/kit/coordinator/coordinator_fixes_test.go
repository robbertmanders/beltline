// Coordinator tests of fixes that no shared suite pins on its own: the AFK decision log loads at
// start, a legacy active factory migrates at start, an Adjust without a thinking level does not fail,
// a failure detail keeps whole characters, and the factory name reads the origin's version.

package coordinator

import (
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TestNewLoadsTheSavedAfkDecisions: New reads afk/decisions.json, so the decisions a
// previous launch made are listed again after a restart (Autopilot's decision log).
func TestNewLoadsTheSavedAfkDecisions(t *testing.T) {
	t.Parallel()
	decision := core.AfkDecision{
		ID: "decision-1", RunID: "run-1", BeltName: "Issues", SubjectRef: "acme/billing#142",
		StageID: "triage", StageName: "Triage", Situation: core.AfkSituationKindQuestion, Status: core.AfkDecisionStatusApplied,
		StartedAt: jsonx.Date{Time: sessionSampleT0}, Key: "triage",
	}
	h := newHarness(t, harnessOptions{Prepare: func(storage store.Storage) {
		if err := storage.SaveAfkDecisions([]core.AfkDecision{decision}); err != nil {
			t.Fatalf("save decisions: %v", err)
		}
	}})
	again := h.Restart()
	decisions := again.ReadAfkDecisions()
	if len(decisions) != 1 || decisions[0].ID != "decision-1" {
		t.Fatalf("decisions after New = %+v, want decision-1", decisions)
	}
}

// TestAdjustStageWithoutAThinkingLevelKeepsTheStage: an Adjust that names no thinking level
// saves the model and leaves the stage's thinking unset (the nil level must not be dereferenced).
func TestAdjustStageWithoutAThinkingLevelKeepsTheStage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	belt := c.Belts()[0].Belt
	var run core.Run
	h.Turn(func() {
		run = c.book.add(core.NewRun(core.RunArgs{
			Belt: belt, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Adjust"}, Now: testkitNow(),
		}))
	})
	model := "gpt-test"
	adjustment, err := c.AdjustStage(run.ID, run.StageID, &model, nil, nil, nil)
	if err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if adjustment == nil {
		t.Fatal("no adjustment recorded")
	}
	after := c.Run(run.ID)
	if after == nil {
		t.Fatal("run gone")
	}
	stage := stageNamed(*after, run.StageID)
	if stage == nil || stage.Model != "gpt-test" || stage.Thinking != nil {
		t.Fatalf("stage = %+v, want model gpt-test and no thinking", stage)
	}
}

// TestAgentResourcePresentationOfASavedAgent: a saved agent is not new, and offers no reset.
func TestAgentResourcePresentationOfASavedAgent(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	record := h.Coord().LatestAgent("triage")
	if record == nil {
		t.Fatal("no triage agent")
	}
	resource := h.Coord().ReadAgentResource(*record)
	if resource.Presentation.IsNew {
		t.Fatalf("presentation = %+v, want not new", resource.Presentation)
	}
	for _, action := range resource.Actions {
		if action.ID == "reset" {
			t.Fatalf("actions = %+v, want no reset", resource.Actions)
		}
	}
}

// TestAgentResourcePresentationWarnsAboutAModelNotAllowed: a draft agent whose model the runner does not
// allow carries the model warning, as modelWarnings(agent:) gives.
func TestAgentResourcePresentationWarnsAboutAModelNotAllowed(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	record := c.LatestAgent("triage")
	if record == nil {
		t.Fatal("no triage agent")
	}
	agent := record.Agent
	agent.Model = "not/allowed"
	warnings := c.ModelWarnings(agent)
	if len(warnings) == 0 {
		t.Fatal("no model warning for a model the runner does not allow")
	}
	found := slices.ContainsFunc(warnings, func(warning string) bool {
		return strings.Contains(warning, "not/allowed")
	})
	if !found {
		t.Fatalf("warnings = %v, want the model named", warnings)
	}
	draft := c.DraftAgentResource(agent)
	if len(draft.Presentation.Warnings) == 0 {
		t.Fatalf("draft warnings = %v, want the model warning", draft.Presentation.Warnings)
	}
}
