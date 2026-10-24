// Tests of the status of routes, nested forks, parking and pauses.

package core

import (
	"slices"
	"strings"
	"testing"
)

// legRow is the status leg row whose route is route, or nil when no row has it.
func legRow(rows []StatusLegRow, route string) *StatusLegRow {
	for i := range rows {
		if rows[i].Route != nil && *rows[i].Route == route {
			return &rows[i]
		}
	}
	return nil
}

// TestTheRouteNotTakenIsSkipped
func TestTheRouteNotTakenIsSkipped(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineForkedBelt([]string{"build"}))
	if got := StatusStepOf(run, "spec"); got != StatusStepTodo {
		t.Fatalf("spec before the choice = %s, want todo", got)
	}
	if got := StatusStepOf(run, "quick-build"); got != StatusStepTodo {
		t.Fatalf("quick-build before the choice = %s, want todo", got)
	}
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "done")
	if got := StatusStepOf(run, "spec"); got != StatusStepSkipped {
		t.Fatalf("spec = %s, want skipped", got)
	}
	if got := StatusStepOf(run, "build"); got != StatusStepSkipped {
		t.Fatalf("build = %s, want skipped", got)
	}
	if got := StatusStepOf(run, "quick-build"); got != StatusStepDone {
		t.Fatalf("quick-build = %s, want done", got)
	}
	if got := StatusStage(run, run.Stages[1], false, PauseLive, engineT0).Text; got != "NOT TAKEN" {
		t.Fatalf("spec text = %q, want NOT TAKEN", got)
	}
}

// TestProgressCountsTheRouteTaken
func TestProgressCountsTheRouteTaken(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineForkedBelt([]string{"build"}))
	// Before the choice, along the longest route: Triage, Spec, Build, Verify, Ship.
	if got := StatusProgress(run); got != 0 {
		t.Fatalf("progress before the choice = %v, want 0", got)
	}
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "done")
	// Triage, Build, Verify, Ship: at the third.
	if got := StatusProgress(run); got != 2.0/4 {
		t.Fatalf("progress = %v, want %v", got, 2.0/4)
	}
}

// TestTheRunReadsRunningWhileAnyLegIsAndEachLegHasItsOwnRow
func TestTheRunReadsRunningWhileAnyLegIsAndEachLegHasItsOwnRow(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineBothRoutes())
	engineFinishOutcome(&run, "ready to implement")
	simple := "r:simple"
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "a", SessionID: "a", Folder: "f", Leg: &simple}, engineT0)
	if run.State != RunStateRunning {
		t.Fatalf("state = %s, want running", run.State)
	}
	rows := StatusLegs(run, false, PauseLive, engineT0)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	quick := nestedStageOf(run, "quick-build")
	spec := nestedStageOf(run, "spec")
	if row := legRow(rows, "Simple"); row == nil || row.Stage != quick.Name {
		t.Fatalf("Simple row = %+v, want stage %q", row, quick.Name)
	}
	if row := legRow(rows, "Complex"); row == nil || row.Stage != spec.Name {
		t.Fatalf("Complex row = %+v, want stage %q", row, spec.Name)
	}
	if row := legRow(rows, "Simple"); row == nil || row.State != RunStateRunning {
		t.Fatalf("Simple state = %+v, want running", row)
	}
	EngineAdvance(&run, Event{Kind: EventKindDrop, Reason: "dropped by you", Leg: &simple}, engineT0)
	if run.State != RunStateQueued {
		t.Fatalf("state after drop = %s, want queued", run.State)
	}
	if row := legRow(StatusLegs(run, false, PauseLive, engineT0), "Simple"); row == nil || row.State != RunStateDropped {
		t.Fatalf("Simple after drop = %+v, want dropped", row)
	}
}

// TestAStageOnATakenRouteIsNeverSkipped
func TestAStageOnATakenRouteIsNeverSkipped(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineBothRoutes())
	engineFinishOutcome(&run, "ready to implement")
	if StatusStepOf(run, "spec") == StatusStepSkipped {
		t.Fatal("spec on a taken route is skipped")
	}
	if StatusStepOf(run, "quick-build") == StatusStepSkipped {
		t.Fatal("quick-build on a taken route is skipped")
	}
	// A route the run did not take still reads as not taken.
	single := engineStartRun(engineForkedBelt([]string{"build"}))
	engineFinishOutcome(&single, "ready to implement")
	if got := StatusStepOf(single, "spec"); got != StatusStepSkipped {
		t.Fatalf("untaken spec = %s, want skipped", got)
	}
	if StatusStepOf(single, "quick-build") == StatusStepSkipped {
		t.Fatal("taken quick-build is skipped")
	}
}

// TestTheProgressBarCountsEveryLeg
func TestTheProgressBarCountsEveryLeg(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineBothRoutes())
	engineFinishOutcome(&run, "ready to implement")
	complex := "r:complex"
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "a", SessionID: "a", Folder: "f", Leg: &complex}, engineT0)
	if got := StatusProgress(run); !(got < 0.5) {
		t.Fatalf("progress = %v, want below 0.5", got)
	}
}

// TestNumberingPathsAndSkippedStagesFollowTheNesting
func TestNumberingPathsAndSkippedStagesFollowTheNesting(t *testing.T) {
	t.Parallel()
	layout := nestedFixtureBelt(true, true, true).Layout()
	if got := layout.Number("triage"); got != 1 {
		t.Fatalf("triage number = %d, want 1", got)
	}
	if got := layout.Number("assess"); got != 2 {
		t.Fatalf("assess number = %d, want 2", got)
	}
	if layout.Number("quick-build") != layout.Number("thorough-spec") {
		t.Fatal("quick-build and thorough-spec have different numbers")
	}
	if layout.Number("ship") <= layout.Number("verify") {
		t.Fatal("ship is not numbered after verify")
	}
	run := engineStartRun(nestedFixtureBelt(true, true, true))
	engineFinishOutcome(&run, "go")
	engineFinishOutcome(&run, "go")
	engineFinishOutcome(&run, "done")
	if got := StatusStepOf(run, "thorough-spec"); got != StatusStepSkipped {
		t.Fatalf("thorough-spec = %s, want skipped", got)
	}
	if got := StatusStepOf(run, "skip-build"); got != StatusStepSkipped {
		t.Fatalf("skip-build = %s, want skipped", got)
	}
	if got := StatusStepOf(run, "quick-build"); got != StatusStepDone {
		t.Fatalf("quick-build = %s, want done", got)
	}
	if !(StatusProgress(run) > 0) {
		t.Fatal("progress is not above 0")
	}
}

// TestASplitRunNamesTheFullRoutePath
func TestASplitRunNamesTheFullRoutePath(t *testing.T) {
	t.Parallel()
	belt := nestedFixtureBelt(false, true, true)
	for i := range belt.Stages {
		if belt.Stages[i].ID == "assess" {
			belt.Stages[i].Outcomes = []Outcome{{
				Name: "split", When: "split", Tone: ToneNeutral,
				Then: Then{Action: ThenActionRoute, Routes: []string{"quick", "thorough"}},
			}}
		}
	}
	run := engineStartRun(belt)
	engineFinishOutcome(&run, "go")
	engineFinishOutcome(&run, "split")
	rows := StatusLegs(run, false, PauseLive, engineT0)
	if legRow(rows, "Build / Quick") == nil {
		t.Fatalf("no row named Build / Quick in %+v", rows)
	}
	if legRow(rows, "Build / Thorough") == nil {
		t.Fatalf("no row named Build / Thorough in %+v", rows)
	}
}

// TestACustomApprovalWithoutParkItKeepsItsOpenAttemptStatus checks the status lines of
// a custom approval without park keeping its open attempt.
func TestACustomApprovalWithoutParkItKeepsItsOpenAttemptStatus(t *testing.T) {
	t.Parallel()
	run := engineStartRun(nfApprovalBelt(false))
	engineFinishOutcome(&run, "ready to implement")
	stage := run.Stage()
	for _, outcome := range stage.Outcomes {
		if outcome.Then.Action == ThenActionPark {
			t.Fatal("the approval has a park outcome")
		}
	}
	attempt := run.Attempts[len(run.Attempts)-1].ID
	outcomes := presOutcomeNames(*stage)
	EngineAdvance(&run, Event{Kind: EventKindParkRun}, engineT0)
	if got := StatusStepOf(run, run.StageID); got != StatusStepParked {
		t.Fatalf("step = %s, want parked", got)
	}
	if got := StatusStage(run, *run.Stage(), false, PauseLive, engineT0).Text; got != "PARKED" {
		t.Fatalf("status = %q, want PARKED", got)
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved", Note: new("note")}, engineT0)
	if run.State != RunStateParked {
		t.Fatalf("state after decision = %s, want parked", run.State)
	}
	EngineAdvance(&run, Event{Kind: EventKindUnparkRun}, presAt(1))
	if run.State != RunStateWaiting {
		t.Fatalf("state after unpark = %s, want waiting", run.State)
	}
	last := run.Attempts[len(run.Attempts)-1]
	if last.ID != attempt {
		t.Fatalf("last attempt = %s, want %s", last.ID, attempt)
	}
	if last.Outcome != nil {
		t.Fatalf("last attempt outcome = %q, want none", *last.Outcome)
	}
	if got := presOutcomeNames(*run.Stage()); strings.Join(got, ",") != strings.Join(outcomes, ",") {
		t.Fatalf("outcomes = %v, want %v", got, outcomes)
	}
}

// presOutcomeNames is the names of a stage's outcomes, in order.
func presOutcomeNames(stage Stage) []string {
	names := []string{}
	for _, outcome := range stage.Outcomes {
		names = append(names, outcome.Name)
	}
	return names
}

// statusPausedRunning is a running agent whose pause is confirmed.
func pausedRunning() Run {
	run := engineRunningAgent()
	main := "main"
	conversation := "sid"
	EngineAdvance(&run, Event{Kind: EventKindPause, Operation: "p"}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindPauseConfirmed, Operation: "p", Leg: &main, Conversation: &conversation}, engineT0)
	return run
}

// TestStatusesKeepTheUnderlyingStageAndTheFloorCountsAUserPause
func TestStatusesKeepTheUnderlyingStageAndTheFloorCountsAUserPause(t *testing.T) {
	t.Parallel()
	pausing := engineRunningAgent()
	EngineAdvance(&pausing, Event{Kind: EventKindPause, Operation: "p"}, engineT0)
	if got := StatusStage(pausing, pausing.Stages[0], false, PauseLive, engineT0).Text; got != "PAUSING…" {
		t.Fatalf("pausing = %q", got)
	}
	if got := StatusLocation(pausing, false, PauseLive).Text; got != "Triage · pausing…" {
		t.Fatalf("pausing location = %q", got)
	}
	paused := pausedRunning()
	over := StatusStage(paused, paused.Stages[0], false, PauseLive, engineT0)
	if !strings.HasPrefix(over.Text, "PAUSED ·") || !over.Paused {
		t.Fatalf("paused status = %+v", over)
	}
	if got := StatusLocation(paused, false, PauseLive).Text; got != "Triage · paused by you" {
		t.Fatalf("paused location = %q", got)
	}
	incomplete := pausing.Clone()
	main := "main"
	reason := "did not stop"
	EngineAdvance(&incomplete, Event{Kind: EventKindPauseFailed, Operation: "p", Leg: &main, Reason: reason}, engineT0)
	if got := StatusStage(incomplete, incomplete.Stages[0], false, PauseLive, engineT0).Text; got != "PAUSE INCOMPLETE" {
		t.Fatalf("incomplete = %q", got)
	}
	if got := StatusStage(incomplete, incomplete.Stages[1], false, PauseLive, engineT0).Text; got != "PAUSED · NOT STARTED" {
		t.Fatalf("incomplete next stage = %q", got)
	}
	if got := StatusLocation(incomplete, false, PauseLive).Text; got != "Triage · pause incomplete" {
		t.Fatalf("incomplete location = %q", got)
	}
	queued := paused.Clone()
	EngineAdvance(&queued, Event{Kind: EventKindResumePaused, Operation: "r"}, engineT0)
	EngineAdvance(&queued, Event{Kind: EventKindContinuationsAdmitted}, engineT0)
	if got := StatusStage(queued, queued.Stages[0], false, PauseLive, engineT0).Text; got != "QUEUED TO CONTINUE" {
		t.Fatalf("queued to continue = %q", got)
	}
	if got := StatusLocation(queued, false, PauseLive).Text; got != "Triage · queued to continue" {
		t.Fatalf("queued location = %q", got)
	}
	if got := AttemptPanelCanContinue(paused, paused.Attempts[0], true); got == nil || *got != "Paused by you" {
		t.Fatalf("can continue = %v, want Paused by you", got)
	}
	if !strings.HasPrefix(RunPauseExplanation, "Paused by you.") {
		t.Fatalf("explanation = %q", RunPauseExplanation)
	}
	if RunPauseGlobalPauseHelp != "Everything is paused. Continue from the toolbar first." {
		t.Fatalf("global pause help = %q", RunPauseGlobalPauseHelp)
	}
	if groups := WorkGroupGroupRuns([]Run{paused}, GroupByStatus); len(groups) != 1 || groups[0].Key.Value != "Paused" {
		t.Fatalf("status group of a paused run = %+v, want Paused", groups)
	}
}

// TestASplitRunReadsPausedTooAndNamesItsRouteAgainWhenLive
func TestASplitRunReadsPausedTooAndNamesItsRouteAgainWhenLive(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineBothRoutes())
	engineFinishOutcome(&run, "ready to implement")
	simple := "r:simple"
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "a", SessionID: "a", Folder: "f", Leg: &simple}, engineT0)
	pause := Pause{IsPaused: true, PausedAt: new(engineT0)}
	if run.State != RunStateRunning {
		t.Fatalf("state = %s, want running", run.State)
	}
	// Paused, every leg's row reads paused and so does the run's one line.
	paused := slices.ContainsFunc(StatusLegs(run, false, pause, engineT0), func(row StatusLegRow) bool {
		return row.Paused
	})
	if !paused {
		t.Fatal("no leg row reads paused")
	}
	if got := StatusLocation(run, false, pause).Text; !strings.HasSuffix(got, "· paused") {
		t.Fatalf("paused location = %q", got)
	}
	// Live again, the reason that names the routes the run is working is back.
	if got := StatusLocation(run, false, PauseLive).Text; !strings.Contains(got, "WAITING FOR ROUTES") {
		t.Fatalf("live location = %q, want the routes reason", got)
	}
	var running *StatusLegRow
	live := StatusLegs(run, false, PauseLive, engineT0)
	for i := range live {
		if live[i].State == RunStateRunning {
			running = &live[i]
			break
		}
	}
	if running == nil || running.Look != StageStatusLookRunning || running.Paused {
		t.Fatalf("running leg = %+v, want it running", running)
	}
}
