// Engine tests of the nested fork flow.

package core

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

// TestAnOuterRouteLeavesTheOtherRoutesUnselected
func TestAnOuterRouteLeavesTheOtherRoutesUnselected(t *testing.T) {
	t.Parallel()
	belt := nfBelt(true, true, true)
	belt.Stages[0].Outcomes[0].Then = nfRouteThen("skip")
	run := engineStartRun(belt)
	nfFinish(&run, belt.Stages[0].Outcomes[0].Name)
	routes := run.Routes()
	if !slices.Equal(routes["wave"], []string{"skip"}) {
		t.Errorf("wave = %v, want [skip]", routes["wave"])
	}
	if routes["pace"] != nil {
		t.Errorf("pace = %v, want none", routes["pace"])
	}
	quick := belt.Stages[nfStageIndex(t, belt, "quick-build")]
	glance := belt.Stages[nfStageIndex(t, belt, "skip-build")]
	if !StageKeysExclusive(quick, glance, belt.Layout()) {
		t.Errorf("quick-build and skip-build are not exclusive")
	}
}

// TestNextTakesTheFirstRouteAtEveryDepth
func TestNextTakesTheFirstRouteAtEveryDepth(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBelt(false, true, true))
	nfFinish(&run, "go")
	if run.StageID != "assess" {
		t.Errorf("stage = %q, want assess", run.StageID)
	}
	if !slices.Equal(run.Routes()["wave"], []string{"build"}) {
		t.Errorf("wave = %v, want [build]", run.Routes()["wave"])
	}
	if len(run.Legs) != 1 || run.Legs[0].ID != "main" {
		t.Errorf("legs = %+v, want the main line", run.Legs)
	}
	nfFinish(&run, "go")
	if run.StageID != "quick-build" {
		t.Errorf("stage = %q, want quick-build", run.StageID)
	}
	if !slices.Equal(run.Routes()["pace"], []string{"quick"}) {
		t.Errorf("pace = %v, want [quick]", run.Routes()["pace"])
	}
	for _, attempt := range run.Attempts {
		if attempt.StageID == "thorough-spec" || attempt.StageID == "skip-build" {
			t.Errorf("attempt at %q, want none", attempt.StageID)
		}
	}
}

// TestAnExplicitSelectionRunsOnlyThoseRoutes
func TestAnExplicitSelectionRunsOnlyThoseRoutes(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothInner(t, nfBelt(false, true, true)))
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	if got := nfStageOf(nfLegOn(run, "quick")); got != "quick-build" {
		t.Errorf("quick leg at %q, want quick-build", got)
	}
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-spec" {
		t.Errorf("thorough leg at %q, want thorough-spec", got)
	}
	for _, leg := range run.Legs {
		if leg.Route == nil || (*leg.Route != "quick" && *leg.Route != "thorough") {
			t.Errorf("leg %q on route %v, want quick or thorough", leg.ID, leg.Route)
		}
	}
}

// TestAnEmptyRouteFinishesAtItsOwnJoin
func TestAnEmptyRouteFinishesAtItsOwnJoin(t *testing.T) {
	t.Parallel()
	belt := nfBothInner(t, nfBelt(false, true, false))
	run := engineStartRun(belt)
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	if quick := nfLegOn(run, "quick"); quick == nil || !quick.IsFinished() {
		t.Errorf("quick leg = %+v, want finished", quick)
	}
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-spec" {
		t.Errorf("thorough leg at %q, want thorough-spec", got)
	}
	if run.StageID == "verify" {
		t.Errorf("run stage = verify, want it to wait")
	}
	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:thorough")
	if n := len(nfLegsAt(run, "verify")); n != 1 {
		t.Errorf("verify legs = %d, want 1", n)
	}
}

// TestAnInnerForkAtTheEndOfItsRouteJoinsTheOuterRoute
func TestAnInnerForkAtTheEndOfItsRouteJoinsTheOuterRoute(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothInner(t, nfBelt(false, false, true)))
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	nfFinishLeg(&run, "done", "r:quick")
	if run.StageID == "ship" {
		t.Errorf("run stage = ship, want it to wait for the other route")
	}
	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:thorough")
	if n := len(nfLegsAt(run, "ship")); n != 1 {
		t.Errorf("ship legs = %d, want 1", n)
	}
	if n := len(nfAttemptsAt(run, "verify")); n != 0 {
		t.Errorf("verify attempts = %d, want 0", n)
	}
}

// TestEachJoinContinuesOnceAndAHeldSiblingHoldsOnlyItsJoin
func TestEachJoinContinuesOnceAndAHeldSiblingHoldsOnlyItsJoin(t *testing.T) {
	t.Parallel()
	belt := nfBothOuter(t, nfBothInner(t, nfBelt(true, true, true)))
	run := engineStartRun(belt)
	nfFinish(&run, "split")
	if got := nfStageOf(nfLegOn(run, "build")); got != "assess" {
		t.Errorf("build leg at %q, want assess", got)
	}
	if got := nfStageOf(nfLegOn(run, "skip")); got != "skip-build" {
		t.Errorf("skip leg at %q, want skip-build", got)
	}
	nfFinishLeg(&run, "split", "r:build")
	if got := nfRouteSet(run.Legs); !reflect.DeepEqual(got, map[string]struct{}{"quick": {}, "thorough": {}, "skip": {}}) {
		t.Errorf("routes = %v, want quick, thorough and skip", got)
	}
	nfFinishLeg(&run, "done", "r:quick")
	if n := len(nfAttemptsAt(run, "verify")); n != 0 {
		t.Errorf("verify attempts = %d, want 0", n)
	}
	if n := len(nfUnfinishedLegsAt(run, "verify")); n != 0 {
		t.Errorf("unfinished verify legs = %d, want 0", n)
	}
	nfFinishLeg(&run, "done", "r:thorough")
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-build" {
		t.Errorf("thorough leg at %q, want thorough-build", got)
	}
	nfFinishLeg(&run, "done", "r:thorough")
	if n := len(nfLegsAt(run, "verify")); n != 1 {
		t.Errorf("verify legs = %d, want 1", n)
	}
	if got := nfStageOf(nfLegOn(run, "skip")); got != "skip-build" {
		t.Errorf("skip leg at %q, want skip-build", got)
	}
	verify := nfLegsAt(run, "verify")[0]
	nfFinishLeg(&run, "done", verify.ID)
	if run.StageID == "ship" {
		t.Errorf("run stage = ship, want it to wait for skip")
	}
	nfFinishLeg(&run, "done", "r:skip")
	if n := len(nfLegsAt(run, "ship")); n != 1 {
		t.Errorf("ship legs = %d, want 1", n)
	}
}

// TestOneOuterRouteStillJoinsItsInnerForkOnce
func TestOneOuterRouteStillJoinsItsInnerForkOnce(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothInner(t, nfBelt(false, true, true)))
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:quick")
	if n := len(nfLegsAt(run, "verify")); n != 1 {
		t.Errorf("verify legs = %d, want 1", n)
	}
	if run.IsSplit() {
		t.Errorf("run is still split")
	}
}

// TestHeldParkedRetryAnswerApprovalAbortAndDropStayOnThatWork
func TestHeldParkedRetryAnswerApprovalAbortAndDropStayOnThatWork(t *testing.T) {
	t.Parallel()
	belt := nfBothInner(t, nfBelt(false, true, true))
	nfSetOutcomes(t, &belt, "quick-build",
		nfOutcome("ask", nfThenOf(ThenActionHold)),
		nfOutcome("again", nfThenOf(ThenActionRetry)),
		nfOutcome("wait", nfThenOf(ThenActionPark)),
	)
	approve := DefaultsStageAt(StageTypeApprove, DefaultsPlace{Route: new("thorough")}, nil)
	approve.ID = "approve"
	approve.Route = new("thorough")
	approve.Outcomes = []Outcome{nfOutcome("approved", nfThenOf(ThenActionNext))}
	nfInsertStage(&belt, nfStageIndex(t, belt, "thorough-build"), approve)
	run := engineStartRun(belt)
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	thorough := nfLegOn(run, "thorough").ID
	nfFinishLeg(&run, "ask", "r:quick")
	if state := nfStateOf(nfLegOn(run, "quick")); state != RunStateHeld {
		t.Errorf("quick state = %q, want held", state)
	}
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-spec" {
		t.Errorf("thorough leg at %q, want thorough-spec", got)
	}
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("because"), Leg: new("r:quick")}, engineT0)
	if state := nfStateOf(nfLegOn(run, "quick")); state != RunStateQueued {
		t.Errorf("quick state after answer = %q, want queued", state)
	}
	if note := nfLegOn(run, "quick").Note; note == nil || *note != "because" {
		t.Errorf("quick note = %v, want because", note)
	}
	nfFinishLeg(&run, "again", "r:quick")
	if got := nfStageOf(nfLegOn(run, "quick")); got != "quick-build" {
		t.Errorf("quick leg at %q, want quick-build", got)
	}
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-spec" {
		t.Errorf("thorough leg at %q, want thorough-spec", got)
	}
	EngineAdvance(&run, nfStarted("s", "f", new("r:quick")), engineT0)
	EngineAdvance(&run, Event{Kind: EventKindAbort, Leg: new("r:quick")}, engineT0)
	if state := nfStateOf(nfLegOn(run, "quick")); state != RunStateHeld {
		t.Errorf("quick state after abort = %q, want held", state)
	}
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-spec" {
		t.Errorf("thorough leg at %q, want thorough-spec", got)
	}
	nfFinishLeg(&run, "done", thorough)
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "approve" {
		t.Errorf("thorough leg at %q, want approve", got)
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved", Leg: new(thorough)}, engineT0)
	if got := nfStageOf(nfLegOn(run, "thorough")); got != "thorough-build" {
		t.Errorf("thorough leg at %q, want thorough-build", got)
	}
	EngineAdvance(&run, Event{Kind: EventKindDrop, Reason: "skip quick", Leg: new("r:quick")}, engineT0)
	if state := nfStateOf(nfLegOn(run, "quick")); state != RunStateDropped {
		t.Errorf("quick state = %q, want dropped", state)
	}
	if state := nfStateOf(nfLegOn(run, "thorough")); state == RunStateDropped {
		t.Errorf("thorough state = dropped, want it kept")
	}
	nfFinishLeg(&run, "done", thorough)
	if n := len(nfLegsAt(run, "verify")); n != 1 {
		t.Errorf("verify legs = %d, want 1", n)
	}
}

// TestDroppingTheRunStopsEveryDescendant
func TestDroppingTheRunStopsEveryDescendant(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothInner(t, nfBelt(false, true, true)))
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	EngineAdvance(&run, Event{Kind: EventKindDrop, Reason: "stop"}, engineT0)
	if run.State != RunStateDropped {
		t.Errorf("state = %q, want dropped", run.State)
	}
	if len(run.Legs) != 0 {
		t.Errorf("legs = %+v, want none", run.Legs)
	}
}

// TestAParkedDescendantKeepsTheJoinWaiting
func TestAParkedDescendantKeepsTheJoinWaiting(t *testing.T) {
	t.Parallel()
	belt := nfBothInner(t, nfBelt(false, true, true))
	nfSetOutcomes(t, &belt, "quick-build", nfOutcome("wait", nfThenOf(ThenActionPark)))
	run := engineStartRun(belt)
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	nfFinishLeg(&run, "wait", "r:quick")
	nfFinishLeg(&run, "done", "r:thorough")
	nfFinishLeg(&run, "done", "r:thorough")
	if state := nfStateOf(nfLegOn(run, "quick")); state != RunStateParked {
		t.Errorf("quick state = %q, want parked", state)
	}
	if n := len(nfUnfinishedLegsAt(run, "verify")); n != 0 {
		t.Errorf("unfinished verify legs = %d, want 0", n)
	}
	waiting := slices.ContainsFunc(run.Joins, func(join Join) bool {
		return join.ForkID == "pace" && !join.Joined
	})
	if !waiting {
		t.Errorf("joins = %+v, want the pace join waiting", run.Joins)
	}
}

// TestThreeLevelsJoinFromTheInsideOut
func TestThreeLevelsJoinFromTheInsideOut(t *testing.T) {
	t.Parallel()
	belt := nfBothInner(t, nfBelt(false, true, true))
	belt.ForkAround("thorough-build")
	var deep Fork
	for _, fork := range belt.Forks {
		if fork.Parent != nil && *fork.Parent == "thorough" {
			deep = fork
		}
	}
	first := deep.Routes[0].ID
	second := deep.Routes[1].ID
	nfSetOutcomes(t, &belt, "thorough-spec", nfOutcome("split", nfRoutesThen(first, second)))
	side := DefaultsStageForAgentAt(SampleAgent("build"), DefaultsPlace{Before: belt.Stages, Route: new(second)}, belt.Stages, TriggerKindIssues)
	side.ID = "side"
	side.Route = new(second)
	side.Outcomes = []Outcome{nfOutcome("done", nfThenOf(ThenActionNext))}
	belt.Insert(side, belt.End(second))
	run := engineStartRun(belt)
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	nfFinishLeg(&run, "done", "r:quick")
	nfFinishLeg(&run, "split", "r:thorough")
	var deepLegs []Leg
	for _, leg := range run.Legs {
		if leg.Route != nil && (*leg.Route == first || *leg.Route == second) {
			deepLegs = append(deepLegs, leg)
		}
	}
	if len(deepLegs) != 2 {
		t.Fatalf("deep legs = %d, want 2", len(deepLegs))
	}
	for _, leg := range deepLegs {
		nfFinishLeg(&run, "done", leg.ID)
	}
	if n := len(nfLegsAt(run, "verify")); n != 1 {
		t.Errorf("verify legs = %d, want 1", n)
	}
}

// TestANormalJoinMergesIntoTheEnclosingBranch
func TestANormalJoinMergesIntoTheEnclosingBranch(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothOuter(t, nfBothInner(t, nfBelt(true, true, true))))
	run.Branch = new("run-branch")
	nfFinish(&run, "split")
	for i := range run.Legs {
		if run.Legs[i].Route != nil && *run.Legs[i].Route == "build" {
			run.Legs[i].Branch = new("build-branch")
		} else {
			run.Legs[i].Branch = new("skip-branch")
		}
	}
	EngineAdvance(&run, nfStarted("a", "f", new("r:build")), engineT0)
	EngineAdvance(&run, nfEnded("Outcome: split", new("r:build")), engineT0)
	for i := range run.Legs {
		if run.Legs[i].Branch == nil {
			name := run.Legs[i].ID
			if run.Legs[i].Route != nil {
				name = *run.Legs[i].Route
			}
			run.Legs[i].Branch = new("inner-" + name)
		}
	}
	nfFinishLeg(&run, "done", "r:quick")
	EngineAdvance(&run, nfStarted("b", "f", new("r:thorough")), engineT0)
	EngineAdvance(&run, nfEnded("Outcome: done", new("r:thorough")), engineT0)
	if !nfIsString(run.MergeInto, "build-branch") && len(nfLegsAt(run, "thorough-build")) == 0 {
		t.Errorf("merge into = %v, legs = %+v", run.MergeInto, run.Legs)
	}
	nfFinishLeg(&run, "done", "r:thorough")
	if !nfIsString(run.MergeInto, "build-branch") {
		t.Errorf("merge into = %v, want build-branch", run.MergeInto)
	}
	for _, branch := range run.MergeBranches {
		if len(branch) < len("inner-") || branch[:len("inner-")] != "inner-" {
			t.Errorf("merge branch %q, want an inner- branch", branch)
		}
	}
}

// nfIsString reports whether p is non-nil and holds s.
func nfIsString(p *string, s string) bool {
	return p != nil && *p == s
}

// TestSendToMovesOnlyTheNamedRunningRouteAndKeepsItsSibling: a running route is sent to
// the stage it is at. Its session stops, and the other route keeps its work untouched.
func TestSendToMovesOnlyTheNamedRunningRouteAndKeepsItsSibling(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfBothInner(t, nfBelt(false, true, true)))
	nfFinish(&run, "go")
	nfFinish(&run, "split")
	EngineAdvance(&run, nfStarted("q-s", "f", new("r:quick")), engineT0)
	sibling := nfLegByID(run, "r:thorough")
	if sibling == nil {
		t.Fatal("no thorough route")
	}
	before := *sibling
	effects := EngineAdvance(&run, EventResume(new("quick-build"), new("again"), new("r:quick")), engineT0.Add(time.Second))
	if !reflect.DeepEqual(effects, []Effect{{Kind: EffectKindStop, Session: "q-s"}}) {
		t.Errorf("effects = %+v, want stop q-s", effects)
	}
	quick := nfLegByID(run, "r:quick")
	if quick == nil || quick.State != RunStateQueued || nfStageOf(quick) != "quick-build" || engineStr(quick.Note) != "again" {
		t.Errorf("quick route = %+v, want queued at quick-build with the note", quick)
	}
	if got := nfLegByID(run, "r:thorough"); got == nil || !reflect.DeepEqual(*got, before) {
		t.Errorf("thorough route changed: %+v, want %+v", got, before)
	}
	if last := nfAttemptsAt(run, "quick-build"); len(last) != 1 || last[0].IsOpen() {
		t.Errorf("quick-build attempts = %+v, want the one stopped attempt", last)
	}
}
