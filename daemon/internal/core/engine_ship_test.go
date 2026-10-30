// Tests of Done and Ship: how a successful completion ends a run, and how a Ship on one
// route of a fork survives the join and the rest of the trip.

package core

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// shipLineBelt is Triage, then Build, on one line. Build's done outcome takes then.
func shipLineBelt(then Then) Belt {
	var stages []Stage
	for _, agent := range []string{"triage", "build"} {
		stages = append(stages, DefaultsStageForAgentAt(SampleAgent(agent), DefaultsPlace{Before: stages}, stages, TriggerKindIssues))
	}
	stages[1].Outcomes[0].Then = then
	trigger := DefaultIssueFilter()
	trigger.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "line", Name: "Line", Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger},
		Repos: []string{"acme/billing"}, Stages: stages,
	})
}

// shipSetDone sets the done outcome of the stage with id to then.
func shipSetDone(belt *Belt, id string, then Then) {
	for i := range belt.Stages {
		if belt.Stages[i].ID == id {
			belt.Stages[i].Outcomes[0].Then = then
			return
		}
	}
	panic("shipSetDone: no stage " + id)
}

// shipHasOutward reports whether effects include one that changes something outside Beltline.
func shipHasOutward(effects []Effect) bool {
	return slices.ContainsFunc(effects, func(e Effect) bool {
		return e.Kind == EffectKindComment || e.Kind == EffectKindMergeLegs
	})
}

// shipRoundTrip is run as it is saved and read back, as a restart would read it.
func shipRoundTrip(t *testing.T, run Run) Run {
	t.Helper()
	data, err := engineSaved(run)
	if err != nil {
		t.Fatal(err)
	}
	var back Run
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// TestOutcomesFinishTheRunAsDoneOrShipped checks each action's destination, whatever the
// outcome is called and however good its tone is: Ship alone records shipping.
func TestOutcomesFinishTheRunAsDoneOrShipped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		then Then
		want RunState
	}{
		{"done", NewThen(ThenArgs{Action: ThenActionDone}), RunStateDone},
		{"shipped", NewThen(ThenArgs{Action: ThenActionShip}), RunStateShipped},
		{"next past the last stage", NewThen(ThenArgs{Action: ThenActionNext}), RunStateDone},
	}
	for _, c := range cases {
		run := engineStartRun(shipLineBelt(c.then))
		nfFinish(&run, "ready to implement")
		effects := nfFinish(&run, "done")
		if run.State != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, run.State, c.want)
		}
		if run.Shipping != (c.want == RunStateShipped) {
			t.Errorf("%s: shipping = %t", c.name, run.Shipping)
		}
		if !engineHasEffect(effects, EffectKindCleanUp) {
			t.Errorf("%s: effects = %+v, want the workspace cleaned up", c.name, effects)
		}
		if shipHasOutward(effects) {
			t.Errorf("%s: effects = %+v, want no outward change", c.name, effects)
		}
		if run.Attempts[len(run.Attempts)-1].IsOpen() {
			t.Errorf("%s: the last attempt is still open", c.name)
		}
	}
}

// TestCompletionClearsThePauseARunStillCarries checks that Done and Shipped both end a run's pause.
// A paused run cannot reach an outcome, since every pause phase blocks progress, so the completion is
// driven directly, as the pause would be left on it after a restart.
func TestCompletionClearsThePauseARunStillCarries(t *testing.T) {
	t.Parallel()
	for _, shipping := range []bool{false, true} {
		run := engineStartRun(shipLineBelt(NewThen(ThenArgs{Action: ThenActionDone})))
		run.Shipping = shipping
		run.Pause = &RunPause{Phase: RunPausePhasePaused, Operation: "pause", RequestedAt: jsonx.Date{Time: engineT0}}
		effects := engineComplete(&run)
		want := RunStateDone
		if shipping {
			want = RunStateShipped
		}
		if run.State != want || run.Pause != nil {
			t.Errorf("shipping %t: state %q pause %+v, want %q and no pause", shipping, run.State, run.Pause, want)
		}
		if !engineHasEffect(effects, EffectKindCleanUp) {
			t.Errorf("shipping %t: effects = %+v, want the workspace cleaned up", shipping, effects)
		}
	}
}

// TestAGoodDoneOutcomeDoesNotShip checks that a good tone and the name done are not what ships.
func TestAGoodDoneOutcomeDoesNotShip(t *testing.T) {
	t.Parallel()
	run := engineStartRun(shipLineBelt(NewThen(ThenArgs{Action: ThenActionDone})))
	nfFinish(&run, "ready to implement")
	nfFinish(&run, "done")
	if run.State != RunStateDone {
		t.Errorf("state = %q, want done", run.State)
	}
}

// TestAShipOnOneRouteWaitsForTheOtherAndSurvivesARestart runs Quick build on the
// simple route with Ship while Spec and Build still work on the complex route. The
// trip ships once the whole run finishes, and the shipping intent is saved meanwhile.
func TestAShipOnOneRouteWaitsForTheOtherAndSurvivesARestart(t *testing.T) {
	t.Parallel()
	belt := nfBothRoutesOf([]string{"build"})
	shipSetDone(&belt, "quick-build", NewThen(ThenArgs{Action: ThenActionShip}))
	run := engineStartRun(belt)
	nfFinish(&run, "ready to implement")

	nfFinishLeg(&run, "done", "r:simple")
	if !run.Shipping {
		t.Fatalf("shipping = false after the Ship route finished")
	}
	if run.State == RunStateShipped || run.State == RunStateDone {
		t.Fatalf("state = %q while the complex route still works, want it running", run.State)
	}
	run = shipRoundTrip(t, run)
	if !run.Shipping {
		t.Fatalf("shipping intent lost in a save and read back")
	}

	nfFinishLeg(&run, "complete", "r:complex")
	nfFinishLeg(&run, "done", "r:complex")
	if run.StageID != "verify" || run.State != RunStateQueued {
		t.Fatalf("after the join: stage %q state %q, want verify queued", run.StageID, run.State)
	}
	nfFinish(&run, "pass")
	effects := nfFinish(&run, "opened")
	if run.State != RunStateShipped {
		t.Errorf("state = %q, want shipped", run.State)
	}
	if !engineHasEffect(effects, EffectKindCleanUp) {
		t.Errorf("effects = %+v, want the workspace cleaned up", effects)
	}
}

// TestAllDoneRoutesFinishAsDone checks that an ordinary route join is not shipping.
func TestAllDoneRoutesFinishAsDone(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothRoutesOf([]string{"build"}))
	nfFinish(&run, "ready to implement")
	nfFinishLeg(&run, "complete", "r:complex")
	nfFinishLeg(&run, "done", "r:complex")
	nfFinishLeg(&run, "done", "r:simple")
	nfFinish(&run, "pass")
	nfFinish(&run, "opened")
	if run.Shipping || run.State != RunStateDone {
		t.Errorf("shipping %t state %q, want done and not shipping", run.Shipping, run.State)
	}
}

// TestShipPlusDoneIsShipped checks that one Ship on any route ships the trip, and that a
// dropped route does not change it.
func TestShipPlusDoneIsShipped(t *testing.T) {
	t.Parallel()
	for _, drop := range []bool{false, true} {
		belt := nfBothRoutesOf([]string{"build"})
		shipSetDone(&belt, "quick-build", NewThen(ThenArgs{Action: ThenActionShip}))
		run := engineStartRun(belt)
		nfFinish(&run, "ready to implement")
		nfFinishLeg(&run, "done", "r:simple")
		if drop {
			leg := "r:complex"
			EngineAdvance(&run, Event{Kind: EventKindDrop, Leg: &leg, Reason: "not needed"}, engineT0)
		} else {
			nfFinishLeg(&run, "complete", "r:complex")
			nfFinishLeg(&run, "done", "r:complex")
		}
		nfFinish(&run, "pass")
		nfFinish(&run, "opened")
		if run.State != RunStateShipped {
			t.Errorf("dropped %t: state = %q, want shipped", drop, run.State)
		}
	}
}

// TestAWholeRunDropStaysDropped checks that a Ship before a drop does not ship the dropped run.
func TestAWholeRunDropStaysDropped(t *testing.T) {
	t.Parallel()
	belt := nfBothRoutesOf([]string{"build"})
	shipSetDone(&belt, "quick-build", NewThen(ThenArgs{Action: ThenActionShip}))
	run := engineStartRun(belt)
	nfFinish(&run, "ready to implement")
	nfFinishLeg(&run, "done", "r:simple")
	EngineAdvance(&run, Event{Kind: EventKindDrop, Reason: "not needed"}, engineT0)
	if run.State != RunStateDropped {
		t.Errorf("state = %q, want dropped", run.State)
	}
}

// TestAShipOnTheRouteThatFinishesFirstStillShips checks that a Ship which finishes before
// the other route is kept until the run finishes, and that Ship with Done is Shipped.
func TestAShipOnTheRouteThatFinishesFirstStillShips(t *testing.T) {
	t.Parallel()
	belt := nfBothRoutesOf([]string{"build"})
	shipSetDone(&belt, "build", NewThen(ThenArgs{Action: ThenActionShip}))
	run := engineStartRun(belt)
	nfFinish(&run, "ready to implement")
	nfFinishLeg(&run, "complete", "r:complex")
	nfFinishLeg(&run, "done", "r:complex")
	if !run.Shipping {
		t.Fatalf("shipping = false after the Ship route finished")
	}
	nfFinishLeg(&run, "done", "r:simple")
	nfFinish(&run, "pass")
	nfFinish(&run, "opened")
	if run.State != RunStateShipped {
		t.Errorf("state = %q, want shipped", run.State)
	}
}

// TestAShipOnANestedRouteSurvivesTheJoinsAndShipsOnce checks a Ship on the inner route of a nested
// fork: the run waits for the other routes and both joins, the shipped figures stay unchanged until
// the whole run finishes, and then it ships once.
func TestAShipOnANestedRouteSurvivesTheJoinsAndShipsOnce(t *testing.T) {
	t.Parallel()
	belt := nfBothOuter(t, nfBothInner(t, nfBelt(true, true, true)))
	nfSetOutcomes(t, &belt, "quick-build", nfOutcome("done", nfThenOf(ThenActionShip)))
	run := engineStartRun(belt)
	nfFinish(&run, "split")
	nfFinishLeg(&run, "split", "r:build")
	nfFinishLeg(&run, "done", "r:quick")
	if !run.Shipping || run.State.Completed() {
		t.Fatalf("shipping %t state %q after the Ship route, want shipping and still going", run.Shipping, run.State)
	}
	if got := overviewSummaryShipped([]Run{run}, engineT0).Median; got != "—" {
		t.Fatalf("a run still going is in the shipped figures: median %q", got)
	}
	run = shipRoundTrip(t, run)

	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:thorough")
	verify := nfLegsAt(run, "verify")
	if len(verify) != 1 {
		t.Fatalf("verify legs = %d, want the first join to continue once", len(verify))
	}
	nfFinishLeg(&run, "done", verify[0].ID)
	nfFinishLeg(&run, "done", "r:skip")
	if !run.Shipping || run.State.Completed() {
		t.Fatalf("shipping %t state %q at the last join, want shipping and still going", run.Shipping, run.State)
	}
	nfFinish(&run, "opened")
	if run.State != RunStateShipped {
		t.Errorf("state = %q, want shipped", run.State)
	}
	if got := overviewSummaryShipped([]Run{run}, engineT0).Median; got == "—" {
		t.Errorf("the finished run is missing from the shipped figures")
	}
}

// TestDonePlusDroppedIsDone checks that a route dropped beside a Done route, with no Ship anywhere,
// leaves the run Done whichever of the two happens first.
func TestDonePlusDroppedIsDone(t *testing.T) {
	t.Parallel()
	for _, dropFirst := range []bool{false, true} {
		run := engineStartRun(nfBothRoutesOf([]string{"build"}))
		nfFinish(&run, "ready to implement")
		drop := func() {
			leg := "r:complex"
			EngineAdvance(&run, Event{Kind: EventKindDrop, Leg: &leg, Reason: "not needed"}, engineT0)
		}
		if dropFirst {
			drop()
		}
		nfFinishLeg(&run, "done", "r:simple")
		if !dropFirst {
			drop()
		}
		nfFinish(&run, "pass")
		nfFinish(&run, "opened")
		if run.Shipping || run.State != RunStateDone {
			t.Errorf("drop first %t: shipping %t state %q, want done and not shipping", dropFirst, run.Shipping, run.State)
		}
	}
}

// TestRunStateCompletedAndFinished checks the finished groupings of run states.
func TestRunStateCompletedAndFinished(t *testing.T) {
	t.Parallel()
	for _, state := range []RunState{RunStateDone, RunStateShipped} {
		if !state.Completed() || state.IsActive() {
			t.Errorf("%s: completed %t active %t", state, state.Completed(), state.IsActive())
		}
	}
	if RunStateDropped.Completed() || RunStateRunning.Completed() {
		t.Errorf("dropped and running must not count as completed")
	}
}
