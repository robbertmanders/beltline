// Engine and layout-end tests. The validation tests are in engine_validation_fork_test.go.
// The route lane tests are presentation tests.

package core

import (
	"slices"
	"testing"
)

// nfAskBelt is triage, then a fork before Spec with an empty
// Main route, the line, and a Needs details route that feeds into Spec ("ask").
// Triage's "needs info" jumps to ask; then Spec, Build and Ship.
func nfAskBelt(t *testing.T) Belt {
	t.Helper()
	fork := Fork{ID: "details", Routes: []Route{{ID: "main", Name: "Main"}, {ID: "asking", Name: "Needs details", Feeds: true}}}
	var stages []Stage
	add := func(agent string, route *string, id string) {
		stage := DefaultsStageForAgentAt(SampleAgent(agent), DefaultsPlace{Before: stages, Forks: []Fork{fork}, Route: route}, stages, TriggerKindIssues)
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	add("triage", nil, "")
	add("spec", new("asking"), "ask")
	add("spec", nil, "")
	add("build", nil, "")
	add("ship", nil, "")
	for i, outcome := range stages[0].Outcomes {
		if outcome.Name == "needs info" {
			stages[0].Outcomes[i].Then = nfGotoThen("ask")
		}
		if outcome.Name == "ready to implement" || outcome.Name == "ready to spec" {
			stages[0].Outcomes[i].Then = nfThenOf(ThenActionNext)
		}
	}
	trigger := DefaultIssueFilter()
	trigger.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "asking", Name: "Asking",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger},
		Repos:   []string{"acme/billing"}, Stages: stages, Forks: []Fork{fork},
	})
}

// TestRoutesAfterTheWatchThatGoBackDoNotJoin
func TestRoutesAfterTheWatchThatGoBackDoNotJoin(t *testing.T) {
	t.Parallel()
	layout := SampleBelt([]string{"acme/billing"}, "agent", SampleAgents(), true).Layout()
	if !layout.Joins("merged") {
		t.Errorf("merged does not join")
	}
	if layout.Joins("conflicts") {
		t.Errorf("conflicts joins")
	}
	if layout.Joins("checks") {
		t.Errorf("checks joins")
	}
	// The watch takes each of them, so every one opens off the line.
	for _, route := range []string{"merged", "conflicts", "checks"} {
		if !layout.Opens(route) {
			t.Errorf("%s does not open", route)
		}
	}
}

// TestAJumpTakesTheRunOntoTheRouteAndItJoinsAtTheStageAfter
func TestAJumpTakesTheRunOntoTheRouteAndItJoinsAtTheStageAfter(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfAskBelt(t))
	nfFinish(&run, "needs info")
	if run.StageID != "ask" {
		t.Errorf("stage = %q, want ask", run.StageID)
	}
	if got := run.Routes()["details"]; !slices.Equal(got, []string{"asking"}) {
		t.Errorf("details = %v, want [asking]", got)
	}
	nfFinish(&run, "complete")
	if run.StageID != "spec" {
		t.Errorf("stage = %q, want spec", run.StageID)
	}
}

// TestMovingOnPassesThroughTheEmptyRoute
func TestMovingOnPassesThroughTheEmptyRoute(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfAskBelt(t))
	nfFinish(&run, "ready to implement")
	if run.StageID != "spec" {
		t.Errorf("stage = %q, want spec", run.StageID)
	}
	if got := run.Routes()["details"]; !slices.Equal(got, []string{"main"}) {
		t.Errorf("details = %v, want [main]", got)
	}
}

// TestAJumpBackOntoTheRouteNeedsNoOtherRoute
func TestAJumpBackOntoTheRouteNeedsNoOtherRoute(t *testing.T) {
	t.Parallel()
	belt := nfAskBelt(t)
	spec := nfStageIndex(t, belt, "spec")
	incomplete := -1
	for i, outcome := range belt.Stages[spec].Outcomes {
		if outcome.Name == "incomplete" {
			incomplete = i
		}
	}
	belt.Stages[spec].Outcomes[incomplete].Then = nfGotoThen("ask")
	if nfContains(nfWarnings(belt), "goes back into a fork") {
		t.Errorf("warnings = %v, want no goes back into a fork warning", nfWarnings(belt))
	}
	run := engineStartRun(belt)
	nfFinish(&run, "ready to implement")
	nfFinish(&run, "incomplete")
	if run.StageID != "ask" {
		t.Errorf("stage = %q, want ask", run.StageID)
	}
	if got := run.Routes()["details"]; !slices.Equal(got, []string{"asking"}) {
		t.Errorf("details = %v, want [asking]", got)
	}
}

// TestAForkAtTheStartLeavesARouteThatFeedsIn
func TestAForkAtTheStartLeavesARouteThatFeedsIn(t *testing.T) {
	t.Parallel()
	belt := nfAskBelt(t)
	belt.Stages = belt.Stages[1:]
	triage := DefaultsStageForAgentAt(SampleAgent("triage"), DefaultsPlace{Forks: belt.Forks, Route: new("main")}, belt.Stages, TriggerKindIssues)
	nfInsertStage(&belt, 0, triage)
	belt.Tidy()
	run := engineStartRun(belt)
	if got := run.Routes()["details"]; !slices.Equal(got, []string{"main"}) {
		t.Errorf("details = %v, want [main]", got)
	}
	if run.StageID != "triage" {
		t.Errorf("stage = %q, want triage", run.StageID)
	}
	if StageKeysTakenTogether(belt.Forks[0], "main", "asking", belt.Layout()) {
		t.Errorf("main and asking are taken together")
	}
}

// TestDoneAfterAForkAtTheEndOfTheBeltJoins
func TestDoneAfterAForkAtTheEndOfTheBeltJoins(t *testing.T) {
	t.Parallel()
	belt := SampleBelt([]string{"acme/billing"}, "agent", SampleAgents(), true)
	wrap := nfStageIndex(t, belt, "wrap-up")
	for i := range belt.Stages[wrap].Outcomes {
		if belt.Stages[wrap].Outcomes[i].Then.Action == ThenActionNext {
			belt.Stages[wrap].Outcomes[i].Then = nfThenOf(ThenActionDone)
		}
	}
	if !belt.Layout().Joins("merged") {
		t.Errorf("merged does not join")
	}
}
