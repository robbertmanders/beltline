// Engine tests of parking: the first part of the tests.
// The rest are in engine_park_routes_test.go.

package core

import (
	"slices"
	"testing"
	"time"
)

// nfParkWatchGrace is the watch grace period: 600 seconds. The
// Go coordinator owns the constant; the engine test only needs its value.
const nfParkWatchGrace = 600

// nfDefaultRun is a run on the default issues belt.
func nfDefaultRun() Run {
	return engineStartRun(SampleBelt([]string{"acme/billing"}, "agent", SampleAgents(), true))
}

// nfToWatch moves the run to its watch stage, the Ship
// agent having opened the pull request.
func nfToWatch(run *Run) {
	nfFinish(run, "ready to implement")
	nfFinish(run, "complete")
	EngineAdvance(run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	nfFinish(run, "done")
	nfFinish(run, "pass")
	nfFinish(run, "opened")
}

// nfParked is what the Run page does when it parks the run.
func nfParked(run *Run, at time.Time) []Effect {
	return EngineAdvance(run, Event{Kind: EventKindParkRun}, at)
}

// nfUnpark is the unparkRun event.
func nfUnpark(run *Run, at time.Time) []Effect {
	return EngineAdvance(run, Event{Kind: EventKindUnparkRun}, at)
}

// nfParkStopped is the parkStopped event for session, with its error (nil when it worked).
func nfParkStopped(run *Run, session string, err *string, at time.Time) []Effect {
	return EngineAdvance(run, Event{Kind: EventKindParkStopped, Session: session, Error: err}, at)
}

// nfTicketWatch is the watch on the
// run's own ticket, with the default ticket outcomes.
func nfTicketWatch() Stage {
	watch := WatchTargetTicket
	return NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Name: new("Watch the ticket"), Outcomes: DefaultsTicketWatchOutcomes(), Watch: &watch})
}

// nfJiraBelt is a Jira belt on
// acme/billing with the given stages and no forks.
func nfJiraBelt(stages []Stage) Belt {
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "jira", Name: "Jira",
		Trigger: Trigger{Kind: TriggerKindJira, Jira: &JiraQuery{Jql: "project = PAY"}},
		Repos:   []string{"acme/billing"}, Stages: stages,
	})
}

// nfEpicSubject is a Jira epic PAY-40.
func nfEpicSubject() Subject {
	return Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-40", Title: "Split invoicing", Labels: []string{"epic"}}
}

// nfWaitingApproval is triage finished "ready to
// implement" on the approval belt, so the approval is waiting.
func nfWaitingApproval() Run {
	run := engineStartRun(nfApprovalBelt(false))
	nfFinish(&run, "ready to implement")
	return run
}

// TestEveryUnfinishedStageCanBeParkedAndUnparked
func TestEveryUnfinishedStageCanBeParkedAndUnparked(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"queued", "running", "held", "admission", "failed", "waiting", "watch", "ticket"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			runEveryUnfinishedStageParkCase(t, kind)
		})
	}
}

// runEveryUnfinishedStageParkCase is one row of everyUnfinishedStageCanBeParkedAndUnparked.
func runEveryUnfinishedStageParkCase(t *testing.T, kind string) {
	var run Run
	switch kind {
	case "queued":
		run = engineStartRun(nfApprovalBelt(false))
	case "running":
		run = engineStartRun(nfApprovalBelt(false))
		EngineAdvance(&run, nfStarted("bl-test", "01-triage-1", nil), engineT0)
	case "held":
		run = engineStartRun(nfApprovalBelt(false))
		nfFinish(&run, "needs info")
	case "admission":
		run = engineStartRun(nfApprovalBelt(false))
		EngineAdvance(&run, Event{Kind: EventKindAdmissionBlocked, Reason: "Choose an allowed model"}, engineT0)
	case "failed":
		run = engineStartRun(nfApprovalBelt(false))
		engineFinishWithoutOutput(&run)
	case "waiting":
		run = engineStartRun(nfApprovalBelt(false))
		nfFinish(&run, "ready to implement")
	case "watch":
		run = nfDefaultRun()
		nfToWatch(&run)
	case "ticket":
		stages := []Stage{nfTicketWatch(), DefaultsStage(StageTypeApprove, nil, nil, nil)}
		run = NewRun(RunArgs{Belt: nfJiraBelt(stages), Subject: nfEpicSubject(), Now: engineT0})
		EngineStart(&run, engineT0)
	default:
		t.Fatalf("unknown %s", kind)
	}
	stage := run.StageID
	attempts := make([]string, 0, len(run.Attempts))
	for _, attempt := range run.Attempts {
		attempts = append(attempts, attempt.ID)
	}
	branch := run.Branch
	if !run.CanButtonPark() {
		t.Fatalf("canButtonPark = false")
	}
	if waitingPark := nfStageHasPark(run, stage); waitingPark && kind == "waiting" {
		t.Errorf("the waiting approval has a park outcome")
	}
	effects := nfParked(&run, engineT0)
	if run.State != RunStateParked {
		t.Fatalf("state = %q, want parked", run.State)
	}
	if run.StageID != stage {
		t.Errorf("stage = %q, want %q", run.StageID, stage)
	}
	if !run.IsButtonParked() {
		t.Errorf("not button parked")
	}
	if !SamePointee(run.Branch, branch) {
		t.Errorf("branch = %v, want %v", ValueOf(run.Branch), ValueOf(branch))
	}
	if engineHasEffect(effects, EffectKindComment) || engineHasEffect(effects, EffectKindCleanUp) {
		t.Errorf("parking effects = %+v, want no comment or cleanup", effects)
	}
	if again := nfParked(&run, engineT0.Add(5*time.Second)); len(again) != 0 {
		t.Errorf("second park effects = %+v, want none", again)
	}
	if kind == "running" && !slices.Equal(nfAttemptIDs(run), attempts) {
		t.Errorf("attempts changed while parking: %v, want %v", nfAttemptIDs(run), attempts)
	}
	beforeUnpark := run.Clone()
	if run.Parking != nil && len(run.Parking.Stopping) > 0 {
		nfParkStopped(&run, run.Parking.Stopping[0], nil, engineT0.Add(9*time.Second))
	}
	nfUnpark(&run, engineT0.Add(10*time.Second))
	if run.Parking != nil {
		t.Errorf("parking = %+v, want none", run.Parking)
	}
	if run.StageID != beforeUnpark.StageID {
		t.Errorf("stage = %q, want %q", run.StageID, beforeUnpark.StageID)
	}
	if !SamePointee(run.Branch, branch) {
		t.Errorf("branch = %v, want %v", ValueOf(run.Branch), ValueOf(branch))
	}
	nfCheckAfterUnpark(t, kind, run, effects, attempts)
	if repeat := nfUnpark(&run, engineT0.Add(11*time.Second)); len(repeat) != 0 {
		t.Errorf("repeat unpark effects = %+v, want none", repeat)
	}
}

// nfCheckAfterUnpark is the per-kind half of everyUnfinishedStageCanBeParkedAndUnparked.
func nfCheckAfterUnpark(t *testing.T, kind string, run Run, effects []Effect, attempts []string) {
	t.Helper()
	switch kind {
	case "queued":
		if run.State != RunStateQueued {
			t.Errorf("state = %q, want queued", run.State)
		}
		if !slices.Equal(nfAttemptIDs(run), attempts) {
			t.Errorf("attempts = %v, want %v", nfAttemptIDs(run), attempts)
		}
	case "running":
		if run.State != RunStateQueued {
			t.Errorf("state = %q, want queued", run.State)
		}
		if run.Replays == nil || len(attempts) == 0 || *run.Replays != attempts[len(attempts)-1] {
			t.Errorf("replays = %v, want the last attempt %v", ValueOf(run.Replays), attempts)
		}
		if got := nfLastRoute(run); got != "Parked by you" {
			t.Errorf("last attempt route = %q, want Parked by you", got)
		}
		if !nfContainsEffect(effects, Effect{Kind: EffectKindStopForPark, Session: "bl-test"}) {
			t.Errorf("effects = %+v, want stopForPark bl-test", effects)
		}
	case "held":
		if run.State != RunStateHeld {
			t.Errorf("state = %q, want held", run.State)
		}
		if !nfReasonIs(run, "needs info") {
			t.Errorf("reason = %v, want needs info", run.Reason)
		}
		answerable := run.Answerable()
		if answerable == nil || len(attempts) == 0 || answerable.ID != attempts[len(attempts)-1] {
			t.Errorf("answerable = %+v, want the last attempt", answerable)
		}
	case "admission":
		if run.State != RunStateHeld {
			t.Errorf("state = %q, want held", run.State)
		}
		if !nfReasonIs(run, "Choose an allowed model") {
			t.Errorf("reason = %v, want Choose an allowed model", run.Reason)
		}
		if len(run.Attempts) != 0 {
			t.Errorf("attempts = %d, want none", len(run.Attempts))
		}
	case "failed":
		if run.State != RunStateHeld {
			t.Errorf("state = %q, want held", run.State)
		}
		if run.Reason == nil || !nfContains([]string{*run.Reason}, "failed") {
			t.Errorf("reason = %v, want it to say failed", run.Reason)
		}
	case "waiting":
		if run.State != RunStateWaiting {
			t.Errorf("state = %q, want waiting", run.State)
		}
		if !slices.Equal(nfAttemptIDs(run), attempts) {
			t.Errorf("attempts = %v, want %v", nfAttemptIDs(run), attempts)
		}
		last := run.Attempts[len(run.Attempts)-1]
		if last.Outcome != nil || !last.IsOpen() {
			t.Errorf("last attempt = %+v, want open with no outcome", last)
		}
	case "watch", "ticket":
		if run.State != RunStateRunning {
			t.Errorf("state = %q, want running", run.State)
		}
		if !slices.Equal(nfAttemptIDs(run), attempts) {
			t.Errorf("attempts = %v, want %v", nfAttemptIDs(run), attempts)
		}
		if last := run.Attempts[len(run.Attempts)-1]; !last.IsOpen() {
			t.Errorf("last attempt = %+v, want open", last)
		}
	}
}

// nfAttemptIDs is the ids of the run's attempts, in order.
func nfAttemptIDs(run Run) []string {
	ids := make([]string, 0, len(run.Attempts))
	for _, attempt := range run.Attempts {
		ids = append(ids, attempt.ID)
	}
	return ids
}

// nfStageHasPark reports whether the stage with id on run has a park outcome.
func nfStageHasPark(run Run, id string) bool {
	for _, stage := range run.Stages {
		if stage.ID != id {
			continue
		}
		for _, outcome := range stage.Outcomes {
			if outcome.Then.Action == ThenActionPark {
				return true
			}
		}
	}
	return false
}

// TestTerminalRunsIgnoreParkAndUnpark
func TestTerminalRunsIgnoreParkAndUnpark(t *testing.T) {
	t.Parallel()
	for _, state := range []RunState{RunStateShipped, RunStateDropped} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			run := nfDefaultRun()
			run.State = state
			run.Legs = nil
			before := run.Clone()
			if effects := nfParked(&run, engineT0); len(effects) != 0 {
				t.Errorf("park effects = %+v, want none", effects)
			}
			if effects := nfUnpark(&run, engineT0); len(effects) != 0 {
				t.Errorf("unpark effects = %+v, want none", effects)
			}
			if !nfSameJSON(t, run, before) {
				t.Errorf("run changed")
			}
			if run.CanButtonPark() {
				t.Errorf("canButtonPark = true, want false")
			}
		})
	}
}

// TestUnparkingAHeldQuestionThenAnsweringContinuesTheSession
func TestUnparkingAHeldQuestionThenAnsweringContinuesTheSession(t *testing.T) {
	t.Parallel()
	run := nfDefaultRun()
	nfFinish(&run, "needs info")
	held := run.Answerable()
	if held == nil {
		t.Fatalf("answerable = nil, want the held question")
	}
	nfParked(&run, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("The export page.")}, engineT0.Add(time.Second))
	if !run.IsButtonParked() {
		t.Errorf("not button parked")
	}
	if run.Continues != nil {
		t.Errorf("continues = %v, want none", *run.Continues)
	}
	nfUnpark(&run, engineT0.Add(2*time.Second))
	if run.State != RunStateHeld {
		t.Errorf("state = %q, want held", run.State)
	}
	if answerable := run.Answerable(); answerable == nil || answerable.ID != held.ID {
		t.Errorf("answerable = %+v, want the held question", answerable)
	}
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("The export page.")}, engineT0.Add(3*time.Second))
	if run.State != RunStateQueued {
		t.Errorf("state = %q, want queued", run.State)
	}
	if run.Continues == nil || *run.Continues != held.ID {
		t.Errorf("continues = %v, want %q", ValueOf(run.Continues), held.ID)
	}
	if run.StageID != held.StageID {
		t.Errorf("stage = %q, want %q", run.StageID, held.StageID)
	}
}

// TestACustomApprovalWithoutParkItKeepsItsOpenAttempt checks that a custom approval without park keeps its open attempt.
func TestACustomApprovalWithoutParkItKeepsItsOpenAttempt(t *testing.T) {
	t.Parallel()
	run := nfWaitingApproval()
	if nfStageHasPark(run, run.StageID) {
		t.Fatalf("the approval has a park outcome")
	}
	attempt := nfAttemptIDs(run)
	last := attempt[len(attempt)-1]
	var outcomes []string
	for _, outcome := range run.Stage().Outcomes {
		outcomes = append(outcomes, outcome.Name)
	}
	nfParked(&run, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved", Note: new("note")}, engineT0)
	if run.State != RunStateParked {
		t.Errorf("state = %q, want parked", run.State)
	}
	nfUnpark(&run, engineT0.Add(time.Second))
	if run.State != RunStateWaiting {
		t.Errorf("state = %q, want waiting", run.State)
	}
	if got := run.Attempts[len(run.Attempts)-1].ID; got != last {
		t.Errorf("last attempt = %q, want %q", got, last)
	}
	if run.Attempts[len(run.Attempts)-1].Outcome != nil {
		t.Errorf("last attempt outcome = %v, want none", *run.Attempts[len(run.Attempts)-1].Outcome)
	}
	var now []string
	for _, outcome := range run.Stage().Outcomes {
		now = append(now, outcome.Name)
	}
	if !slices.Equal(now, outcomes) {
		t.Errorf("outcomes = %v, want %v", now, outcomes)
	}
}
