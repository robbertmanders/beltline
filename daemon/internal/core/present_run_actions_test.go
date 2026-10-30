// Tests of the pure parts of the run actions.

package core

import (
	"reflect"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// gateBelt is Triage, then an approval, so a finished Triage lands on a gate.
func gateBelt() Belt {
	triage := DefaultsStageForAgent(SampleAgent("triage"), nil, nil, nil, TriggerKindIssues)
	gate := DefaultsStage(StageTypeApprove, []Stage{triage}, nil, []Stage{triage})
	return testKindBelt(TriggerKindIssues, triage, gate)
}

// parkableBelt is a belt whose approval can also park, for the outcome-parked case.
func parkableBelt() Belt {
	triage := DefaultsStageForAgent(SampleAgent("triage"), nil, nil, nil, TriggerKindIssues)
	gate := DefaultsStage(StageTypeApprove, []Stage{triage}, nil, []Stage{triage})
	gate.Outcomes = append(gate.Outcomes, Outcome{Name: "parked", When: "Park it", Tone: ToneNeutral, Then: Then{Action: ThenActionPark}})
	return testKindBelt(TriggerKindIssues, triage, gate)
}

// presState is a run in one of the states the table covers.
func presState(name string) Run {
	switch name {
	case "queued":
		return engineSampleRun()
	case "running":
		run := engineSampleRun()
		EngineAdvance(&run, engineSessionStart("01-triage-1"), engineT0)
		return run
	case "held":
		run := engineSampleRun()
		engineFinishOutcome(&run, "needs info")
		return run
	case "aborted":
		run := engineSampleRun()
		EngineAdvance(&run, engineSessionStart("01-triage-1"), engineT0)
		EngineAdvance(&run, Event{Kind: EventKindAbort}, engineT0)
		return run
	case "waiting":
		run := engineStartRun(gateBelt())
		engineFinishOutcome(&run, "ready to implement")
		return run
	case "parked":
		run := engineSampleRun()
		EngineAdvance(&run, Event{Kind: EventKindParkRun}, engineT0)
		return run
	case "outcome-parked":
		run := engineStartRun(parkableBelt())
		engineFinishOutcome(&run, "ready to implement")
		EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "parked"}, engineT0)
		return run
	case "done":
		run := engineSampleRun()
		run.State = RunStateDone
		run.Legs = nil
		return run
	case "shipped":
		run := engineSampleRun()
		run.State = RunStateShipped
		run.Legs = nil
		return run
	case "dropped":
		run := engineSampleRun()
		EngineAdvance(&run, Event{Kind: EventKindDrop, Reason: "dropped by you"}, engineT0)
		return run
	}
	t := "unknown state " + name
	panic(t)
}

// presAction is the unscoped action for command, which must exist.
func presAction(t *testing.T, command RunCommand, context RunActionContext) RunAction {
	t.Helper()
	action := RunAvailabilityActionAt(command, context, RunActionTarget{})
	if action == nil {
		t.Fatalf("no unscoped action for %s", command)
	}
	return *action
}

// presContextual is the contextual (scoped) records of command in context.
func presContextual(context RunActionContext, command RunCommand) []RunAction {
	var list []RunAction
	for _, action := range context.Actions() {
		if action.ID == string(command) && !action.IsUnscoped() {
			list = append(list, action)
		}
	}
	return list
}

// TestEveryCommandHasAnUnscopedRecordInCommandOrder
func TestEveryCommandHasAnUnscopedRecordInCommandOrder(t *testing.T) {
	t.Parallel()
	list := RunActionContextDefault(presState("queued")).Actions()
	for i, command := range AllRunCommand {
		if list[i].ID != string(command) || !list[i].IsUnscoped() {
			t.Fatalf("record %d = %+v, want unscoped %s", i, list[i], command)
		}
	}
	for _, entry := range list {
		if !entry.Enabled && (entry.Reason == nil || *entry.Reason == "") {
			t.Fatalf("%s is disabled without a reason", entry.ID)
		}
	}
	again := RunActionContextDefault(presState("queued")).Actions()
	if !reflect.DeepEqual(list, again) {
		t.Fatal("unchanged inputs must keep the same order and values")
	}
}

// TestStateTableMatchesTheRunStates
func TestStateTableMatchesTheRunStates(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"queued", "running", "held", "aborted", "waiting", "parked", "outcome-parked", "done", "shipped", "dropped"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			run := presState(name)
			context := RunActionContextDefault(run)
			enabled := func(command RunCommand) bool { return presAction(t, command, context).Enabled }
			active := run.State.IsActive()
			stage := run.Stage()
			isAgent := stage != nil && stage.Kind() == StageKindAgent
			isGate := stage != nil && stage.Kind() == StageKindGate
			check := func(command RunCommand, want bool) {
				t.Helper()
				if got := enabled(command); got != want {
					t.Fatalf("%s: %s enabled = %v, want %v", name, command, got, want)
				}
			}
			check(RunCommandPause, active)
			check(RunCommandDrop, active)
			check(RunCommandPark, active && run.Parking == nil && run.CanButtonPark())
			check(RunCommandUnpark, run.Parking != nil)
			check(RunCommandErase, !active)
			check(RunCommandAfk, active)
			check(RunCommandDecide, run.State == RunStateWaiting && isGate)
			check(RunCommandAnswer, run.State == RunStateHeld && run.Answerable() != nil)
			check(RunCommandResume, active)
			check(RunCommandStartNow, run.State == RunStateQueued && isAgent)
			check(RunCommandRetryContinuation, false)
			check(RunCommandResumeRun, false)
			check(RunCommandOverruleAfk, false)
			if !active {
				if presAction(t, RunCommandErase, context).Reason != nil {
					t.Fatal("erase on a finished run has a reason")
				}
				for _, command := range AllRunCommand {
					if command == RunCommandErase {
						continue
					}
					if enabled(command) {
						t.Fatalf("%s must be refused on a finished run", command)
					}
					if presAction(t, command, context).Reason == nil {
						t.Fatalf("%s on a finished run has no reason", command)
					}
				}
			}
		})
	}
}

// TestAbortedWorkHoldsTheLegAndStillOffersAnswerAndResume
func TestAbortedWorkHoldsTheLegAndStillOffersAnswerAndResume(t *testing.T) {
	t.Parallel()
	run := presState("aborted")
	if run.State != RunStateHeld {
		t.Fatalf("state = %s, want held", run.State)
	}
	if run.Reason == nil || *run.Reason != "stopped by you" {
		t.Fatalf("reason = %v, want stopped by you", run.Reason)
	}
	context := RunActionContextDefault(run)
	if !presAction(t, RunCommandAnswer, context).Enabled || !presAction(t, RunCommandResume, context).Enabled {
		t.Fatal("answer and resume should be offered on an aborted run")
	}
	abort := presAction(t, RunCommandAbort, context)
	if abort.Enabled || abort.Reason == nil || *abort.Reason != "Nothing is running or waiting to be stopped." {
		t.Fatalf("abort = %+v", abort)
	}
}

// TestPausePhasesDrivePauseResumeAndTheBlockedCommands
func TestPausePhasesDrivePauseResumeAndTheBlockedCommands(t *testing.T) {
	t.Parallel()
	for _, phase := range []RunPausePhase{RunPausePhasePausing, RunPausePhasePaused, RunPausePhaseIncomplete, RunPausePhaseResuming, RunPausePhaseReconcileFailed} {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()
			run := presState("running")
			pause := RunPause{Phase: phase, Operation: "op", RequestedAt: jsonx.Date{Time: engineT0}}
			if phase == RunPausePhaseIncomplete {
				pause.Reason = new("one session")
			}
			for _, leg := range run.WorkingLegs() {
				pause.Snapshots = append(pause.Snapshots, RunPauseSnapshot{ID: leg.ID, State: leg.State, StageID: leg.StageID, Reason: leg.Reason})
			}
			run.Pause = &pause
			context := RunActionContextDefault(run)
			pauseAction := presAction(t, RunCommandPause, context)
			resumeRun := presAction(t, RunCommandResumeRun, context)
			if pauseAction.Enabled != (phase == RunPausePhaseIncomplete) {
				t.Fatalf("pause enabled = %v", pauseAction.Enabled)
			}
			wantResume := phase == RunPausePhasePaused || phase == RunPausePhaseResuming || phase == RunPausePhaseReconcileFailed
			if resumeRun.Enabled != wantResume {
				t.Fatalf("resume-run enabled = %v, want %v", resumeRun.Enabled, wantResume)
			}
			// Send to is not held back by a pause: it moves the route, and the pause keeps the destination still.
			if !presAction(t, RunCommandResume, context).Enabled || !presAction(t, RunCommandDrop, context).Enabled {
				t.Fatal("send to and drop should survive a run pause")
			}
			for _, command := range []RunCommand{RunCommandDecide, RunCommandAnswer, RunCommandStartNow, RunCommandContinueSession, RunCommandRetryContinuation} {
				action := presAction(t, command, context)
				if action.Enabled {
					t.Fatalf("%s enabled while %s", command, phase)
				}
				if action.Reason == nil || *action.Reason != RunPauseExplanation {
					t.Fatalf("%s reason = %v, want the pause explanation", command, action.Reason)
				}
			}
			if !presAction(t, RunCommandAbort, context).Enabled || !presAction(t, RunCommandDrop, context).Enabled ||
				!presAction(t, RunCommandPark, context).Enabled || !presAction(t, RunCommandAfk, context).Enabled {
				t.Fatal("abort, drop, park and afk should survive a run pause")
			}
		})
	}
}

// TestGlobalPauseDisablesOnlyTheCommandsTheAppRestricts
func TestGlobalPauseDisablesOnlyTheCommandsTheAppRestricts(t *testing.T) {
	t.Parallel()
	run := presState("waiting")
	context := RunActionContextDefault(run)
	context.GlobalPause = true
	for _, command := range []RunCommand{RunCommandPause, RunCommandResumeRun, RunCommandStartNow, RunCommandRetryContinuation} {
		action := presAction(t, command, context)
		if action.Enabled || action.Reason == nil || *action.Reason != RunPauseGlobalPauseHelp {
			t.Fatalf("%s = %+v, want the global pause refusal", command, action)
		}
	}
	if !presAction(t, RunCommandDecide, context).Enabled || !presAction(t, RunCommandDrop, context).Enabled || !presAction(t, RunCommandPark, context).Enabled {
		t.Fatal("decide, drop and park should stay open under a global pause")
	}
	if resume := presAction(t, RunCommandResume, context); !resume.Enabled {
		t.Fatalf("resume = %+v, want send to open under a global pause", resume)
	}
}

// TestGlobalPauseDoesNotResumeAPersonallyPausedRunWhenDisabled
func TestGlobalPauseDoesNotResumeAPersonallyPausedRunWhenDisabled(t *testing.T) {
	t.Parallel()
	run := presState("running")
	pause := RunPause{Phase: RunPausePhasePaused, Operation: "op", RequestedAt: jsonx.Date{Time: engineT0}}
	for _, leg := range run.WorkingLegs() {
		pause.Snapshots = append(pause.Snapshots, RunPauseSnapshot{ID: leg.ID, State: leg.State, StageID: leg.StageID, Reason: leg.Reason})
	}
	run.Pause = &pause
	global := RunActionContextDefault(run)
	global.GlobalPause = true
	if !presAction(t, RunCommandResumeRun, RunActionContextDefault(run)).Enabled {
		t.Fatal("the run's own pause should be offered")
	}
	action := presAction(t, RunCommandResumeRun, global)
	if action.Enabled || action.Reason == nil || *action.Reason != RunPauseGlobalPauseHelp {
		t.Fatalf("resume under a global pause = %+v", action)
	}
}

// TestGatesWatchesAndRunningStagesCannotStartNow
func TestGatesWatchesAndRunningStagesCannotStartNow(t *testing.T) {
	t.Parallel()
	const reason = "Only a queued agent stage can start now."
	waiting := RunActionContextDefault(presState("waiting"))
	if presAction(t, RunCommandStartNow, waiting).Enabled || *presAction(t, RunCommandStartNow, waiting).Reason != reason {
		t.Fatal("a gate can start now")
	}
	if presAction(t, RunCommandStartNow, RunActionContextDefault(presState("running"))).Enabled {
		t.Fatal("a running stage can start now")
	}
	watch := engineSampleRun()
	watch.Stages = []Stage{{ID: "merge", Type: StageTypeWatch, Name: "Merge", Outcomes: []Outcome{}}}
	watch.StageID = "merge"
	watch.State = RunStateQueued
	watch.Legs = []Leg{{ID: "main", StageID: new("merge"), State: RunStateQueued}}
	context := RunActionContextDefault(watch)
	if presAction(t, RunCommandStartNow, context).Enabled {
		t.Fatal("a queued watch can start now")
	}
	if *presAction(t, RunCommandStartNow, context).Reason != reason {
		t.Fatalf("watch reason = %q", *presAction(t, RunCommandStartNow, context).Reason)
	}
}

// TestWatchStagesOfferAbortButNotDecisions
func TestWatchStagesOfferAbortButNotDecisions(t *testing.T) {
	t.Parallel()
	watch := engineSampleRun()
	engineToWatch(&watch)
	if stage := watch.Stage(); stage == nil || stage.Kind() != StageKindWatch {
		t.Fatal("run is not at a watch")
	}
	context := RunActionContextDefault(watch)
	if watch.State != RunStateRunning {
		t.Fatalf("state = %s, want running", watch.State)
	}
	if !presAction(t, RunCommandAbort, context).Enabled {
		t.Fatal("abort is not offered at a watch")
	}
	if presAction(t, RunCommandDecide, context).Enabled || presAction(t, RunCommandStartNow, context).Enabled {
		t.Fatal("decide or start-now offered at a watch")
	}
}

// TestDecideOffersEveryConfiguredOutcomeInStageOrder
func TestDecideOffersEveryConfiguredOutcomeInStageOrder(t *testing.T) {
	t.Parallel()
	run := presState("waiting")
	context := RunActionContextDefault(run)
	stage := run.Stage()
	offered := presContextual(context, RunCommandDecide)
	if len(offered) != len(stage.Outcomes) {
		t.Fatalf("offered %d outcomes, want %d", len(offered), len(stage.Outcomes))
	}
	for i, action := range offered {
		if action.Outcome == nil || *action.Outcome != stage.Outcomes[i].Name || !action.Enabled || action.Reason != nil || action.Stage == nil || *action.Stage != stage.ID {
			t.Fatalf("offer %d = %+v", i, action)
		}
	}
	first := stage.Outcomes[0].Name
	if got := RunAvailabilityActionAt(RunCommandDecide, context, RunActionTarget{Stage: new(stage.ID), Outcome: new(first)}); got == nil || !got.Enabled {
		t.Fatal("the first outcome is not enabled")
	}
	if RunAvailabilityActionAt(RunCommandDecide, context, RunActionTarget{Stage: new(stage.ID), Outcome: new("not-an-outcome")}) != nil {
		t.Fatal("an unknown outcome is offered")
	}
}

// TestResumeOffersEveryStageInBeltOrder
func TestResumeOffersEveryStageInBeltOrder(t *testing.T) {
	t.Parallel()
	run := presState("held")
	offered := presContextual(RunActionContextDefault(run), RunCommandResume)
	if len(offered) != len(run.Stages) {
		t.Fatalf("offered %d stages, want %d", len(offered), len(run.Stages))
	}
	for i, action := range offered {
		if action.Stage == nil || *action.Stage != run.Stages[i].ID || !action.Enabled {
			t.Fatalf("resume %d = %+v", i, action)
		}
	}
}

// splitRun is a split run of two routes, fast running and slow waiting.
func splitRun() Run {
	run := engineSampleRun()
	stage := func(id string) Stage {
		return NewStage(StageArgs{ID: id, Type: StageTypeAgent, Name: new(capitalizeASCII(id)), Agent: &AgentRef{ID: "triage", Version: 2}, Key: new(id), Outcomes: []Outcome{}})
	}
	run.Stages = []Stage{stage("fast"), stage("slow")}
	fastRoute, slowRoute := "fast", "slow"
	fast, slow := "fast", "slow"
	run.Legs = []Leg{
		{ID: "r:fast", Route: &fastRoute, StageID: &fast, State: RunStateRunning},
		{ID: "r:slow", Route: &slowRoute, StageID: &slow, State: RunStateWaiting},
	}
	run.State = RunStateRunning
	return run
}

// capitalizeASCII upper-cases the first ASCII letter of s.
func capitalizeASCII(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// TestAbortAndDropNameTheRoutesOfASplitRun
func TestAbortAndDropNameTheRoutesOfASplitRun(t *testing.T) {
	t.Parallel()
	run := splitRun()
	context := RunActionContextDefault(run)
	aborts := presContextual(context, RunCommandAbort)
	if len(aborts) != 1 || aborts[0].Leg == nil || *aborts[0].Leg != "r:fast" || !aborts[0].Enabled {
		t.Fatalf("aborts = %+v, want one enabled on r:fast", aborts)
	}
	drops := presContextual(context, RunCommandDrop)
	if len(drops) != 2 || *drops[0].Leg != "r:fast" || *drops[1].Leg != "r:slow" {
		t.Fatalf("drops = %+v, want r:fast then r:slow", drops)
	}
	for _, drop := range drops {
		if !drop.Enabled || drop.Stage == nil {
			t.Fatalf("drop = %+v", drop)
		}
	}
	if got := RunAvailabilityActionAt(RunCommandAbort, context, RunActionTarget{Leg: new("r:fast")}); got == nil || !got.Enabled {
		t.Fatal("abort at r:fast is not enabled")
	}
	if RunAvailabilityActionAt(RunCommandAbort, context, RunActionTarget{Leg: new("r:slow")}) != nil {
		t.Fatal("abort at r:slow is offered")
	}
}

// TestRunQuickActionsFollowTheStateAndTheListedRetry checks the run page's shortcut buttons: Start
// now only on a queued run, and Retry only when the actions list a retry for a saved attempt.
func TestRunQuickActionsFollowTheStateAndTheListedRetry(t *testing.T) {
	t.Parallel()
	retry := NewRunAction(RunActionArgs{ID: string(RunCommandRetryContinuation), Enabled: true, Attempt: new("01-triage-1")})
	cases := []struct {
		name    string
		state   string
		actions []RunAction
		want    []RunCommand
	}{
		{name: "queued with no retry", state: "queued", want: []RunCommand{RunCommandStartNow}},
		{name: "queued with a retry", state: "queued", actions: []RunAction{retry}, want: []RunCommand{RunCommandStartNow, RunCommandRetryContinuation}},
		{name: "running with a retry", state: "running", actions: []RunAction{retry}, want: []RunCommand{RunCommandRetryContinuation}},
		{name: "running with an unscoped retry", state: "running", actions: []RunAction{NewRunAction(RunActionArgs{ID: string(RunCommandRetryContinuation)})}, want: []RunCommand{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RunQuickActions(presState(tc.state), tc.actions)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("RunQuickActions = %v, want %v", got, tc.want)
			}
		})
	}
}
