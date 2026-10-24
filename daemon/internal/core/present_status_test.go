// Tests of the status, the pause bar and the pause status.

package core

import (
	"fmt"
	"testing"
	"time"
)

// presAt is the instant `seconds` after the tests' clock start.
func presAt(seconds float64) time.Time {
	return engineT0.Add(time.Duration(seconds * float64(time.Second)))
}

// presRun is a default issues run for issue number,
// taken through outcomes (a gate takes them as decisions), and started when start is set.
func presRun(number int, outcomes []string, start bool) Run {
	run := NewRun(RunArgs{
		Belt:    placeholderTestBelt("a/b"),
		Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: number, Title: fmt.Sprintf("T%d", number), IsOpen: true},
		Now:     engineT0,
	})
	EngineStart(&run, engineT0)
	for _, outcome := range outcomes {
		if stage := run.Stage(); stage != nil && stage.Kind() == StageKindGate {
			EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: outcome}, engineT0)
		} else {
			engineFinishOutcome(&run, outcome)
		}
	}
	if start {
		EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "s", SessionID: "i", Folder: "f"}, engineT0)
	}
	return run
}

// presSessionStarted is the sessionStarted event of the status tests' session.
func presSessionStarted() Event {
	return Event{Kind: EventKindSessionStarted, Session: "s", SessionID: "i", Folder: "f"}
}

// TestWatchStatusCountsPullRequests
func TestWatchStatusCountsPullRequests(t *testing.T) {
	t.Parallel()
	watch := DefaultsStage(StageTypeWatch, nil, nil, nil)
	r := NewRun(RunArgs{Belt: testKindBelt(TriggerKindReviews, watch), Subject: pullSubject(), Now: engineT0})
	EngineStart(&r, engineT0)
	seven := PullRequest{Number: 7, State: PullRequestStateOpen}
	seven.Repo = "acme/billing"
	r.PullRequests = []PullRequest{seven}
	if got := StatusStage(r, watch, false, PauseLive, engineT0).Text; got != "WATCHING #7" {
		t.Fatalf("one pull request: %q", got)
	}
	nine := PullRequest{Number: 9, State: PullRequestStateOpen}
	nine.Repo = "acme/infra"
	r.PullRequests = append(r.PullRequests, nine)
	if got := StatusStage(r, watch, false, PauseLive, engineT0).Text; got != "WATCHING 2 PRS" {
		t.Fatalf("two pull requests: %q", got)
	}
}

// TestStagesBeforeTheCurrentOneAreDone
func TestStagesBeforeTheCurrentOneAreDone(t *testing.T) {
	t.Parallel()
	r := presRun(1, []string{"ready to spec"}, true)
	if got := StatusStepOf(r, "triage"); got != StatusStepDone {
		t.Fatalf("triage = %s, want done", got)
	}
	if got := StatusStepOf(r, "spec"); got != StatusStepRunning {
		t.Fatalf("spec = %s, want running", got)
	}
	if got := StatusStepOf(r, "approve"); got != StatusStepTodo {
		t.Fatalf("approve = %s, want todo", got)
	}
	spec := StatusStage(r, r.Stages[1], false, PauseLive, presAt(75))
	if spec.Text != "RUNNING 01:15" || spec.Look != StageStatusLookRunning {
		t.Fatalf("spec = %+v, want RUNNING 01:15, running", spec)
	}
	if got := StatusStage(r, r.Stages[0], false, PauseLive, engineT0).Text; got != "✓ 0s" {
		t.Fatalf("triage text = %q, want ✓ 0s", got)
	}
}

// TestAStageThatWillRunAgainSaysSo
func TestAStageThatWillRunAgainSaysSo(t *testing.T) {
	t.Parallel()
	r := presRun(1, []string{"ready to spec", "complete"}, false)
	EngineAdvance(&r, Event{Kind: EventKindDecided, Outcome: "sent back", Note: new("x")}, engineT0)
	if got := StatusStepOf(r, "approve"); got != StatusStepRedo {
		t.Fatalf("approve = %s, want redo", got)
	}
}

// TestQueuedStagesShowWhetherTheBeltIsPaused
func TestQueuedStagesShowWhetherTheBeltIsPaused(t *testing.T) {
	t.Parallel()
	r := presRun(1, nil, false)
	if got := StatusStage(r, r.Stages[0], true, PauseLive, engineT0).Text; got != "PAUSED" {
		t.Fatalf("belt paused: %q, want PAUSED", got)
	}
	if got := StatusLocation(r, false, PauseLive).Text; got != "Triage · queued" {
		t.Fatalf("location = %q, want Triage · queued", got)
	}
}

// TestProgressGrowsAlongTheBelt
func TestProgressGrowsAlongTheBelt(t *testing.T) {
	t.Parallel()
	if got := StatusProgress(presRun(1, nil, false)); got != 0 {
		t.Fatalf("queued progress = %v, want 0", got)
	}
	if got := StatusProgress(presRun(1, []string{"ready to spec"}, true)); got != 1.5/8 {
		t.Fatalf("progress = %v, want %v", got, 1.5/8)
	}
}

// TestTheControlSaysWhatPressingItDoes
func TestTheControlSaysWhatPressingItDoes(t *testing.T) {
	t.Parallel()
	if got := PauseLabelFor(false); got != "Pause everything" {
		t.Fatalf("live label = %q", got)
	}
	if got := PauseLabelFor(true); got != "Paused · Continue" {
		t.Fatalf("paused label = %q", got)
	}
}

// TestTheSidebarLampSaysPausedOnce
func TestTheSidebarLampSaysPausedOnce(t *testing.T) {
	t.Parallel()
	if got := PauseClockLabel(true, "22:41"); got != "PAUSED · 22:41" {
		t.Fatalf("paused lamp = %q", got)
	}
	if got := PauseClockLabel(false, "22:41"); got != "LIVE · 22:41" {
		t.Fatalf("live lamp = %q", got)
	}
}

// TestARunningAgentSaysPausedAndItsTimerStops
func TestARunningAgentSaysPausedAndItsTimerStops(t *testing.T) {
	t.Parallel()
	r := presRun(1, []string{"ready to spec"}, false)
	EngineAdvance(&r, presSessionStarted(), engineT0)
	stage := r.Stages[1]
	pause := Pause{IsPaused: true, PausedAt: new(presAt(60))}
	if got := StatusStage(r, stage, false, pause, presAt(600)).Text; got != "PAUSED" {
		t.Fatalf("paused agent = %q, want PAUSED", got)
	}
	// Ten minutes later the paused hour still counts as one minute.
	if got := StatusStage(r, stage, false, Pause{}, presAt(120)).Text; got != "RUNNING 02:00" {
		t.Fatalf("live agent = %q, want RUNNING 02:00", got)
	}
	if got := StatusStage(r, stage, false, pause, presAt(660)).Text; got != "PAUSED" {
		t.Fatalf("paused again = %q, want PAUSED", got)
	}
}

// TestAQueuedStageReadsPausedOnEveryBelt
func TestAQueuedStageReadsPausedOnEveryBelt(t *testing.T) {
	t.Parallel()
	r := presRun(1, nil, false)
	pause := Pause{IsPaused: true, PausedAt: new(engineT0)}
	if got := StatusStage(r, r.Stages[0], false, pause, engineT0).Text; got != "PAUSED" {
		t.Fatalf("global pause = %q", got)
	}
	if got := StatusStage(r, r.Stages[0], true, PauseLive, engineT0).Text; got != "PAUSED" {
		t.Fatalf("belt pause = %q", got)
	}
	if got := StatusStage(r, r.Stages[0], false, PauseLive, engineT0).Text; got != "QUEUED" {
		t.Fatalf("live = %q, want QUEUED", got)
	}
}

// TestLocationAgreesWithTheStage
func TestLocationAgreesWithTheStage(t *testing.T) {
	t.Parallel()
	queued := presRun(1, nil, false)
	running := presRun(2, []string{"ready to spec"}, true)
	pause := Pause{IsPaused: true, PausedAt: new(engineT0)}
	if got := StatusLocation(queued, false, pause).Text; got != "Triage · paused" {
		t.Fatalf("queued location = %q", got)
	}
	if got := StatusLocation(running, false, pause).Text; got != "Spec · paused" {
		t.Fatalf("running location = %q", got)
	}
	if got := StatusLocation(running, false, PauseLive).Text; got != "Spec · running" {
		t.Fatalf("live location = %q", got)
	}
	// A watch has no agent to suspend, so it keeps saying it is running.
	watch := DefaultsStage(StageTypeWatch, nil, nil, nil)
	watching := NewRun(RunArgs{
		Belt:    testKindBelt(TriggerKindManual, watch),
		Subject: Subject{Kind: SubjectKindManual, Repo: "acme/web", Key: "1", Title: "Watched", IsOpen: true},
		Now:     engineT0,
	})
	EngineStart(&watching, engineT0)
	EngineAdvance(&watching, presSessionStarted(), engineT0)
	if watching.State != RunStateRunning {
		t.Fatalf("watching state = %s, want running", watching.State)
	}
	if got := StatusLocation(watching, false, pause).Text; got != "Watch the pull request · running" {
		t.Fatalf("watch location = %q", got)
	}
}

// TestPausedTimeDoesNotCountAgainstARun
func TestPausedTimeDoesNotCountAgainstARun(t *testing.T) {
	t.Parallel()
	// Ten minutes of real time, nine of them paused, is one minute of work.
	since := engineT0
	at := presAt(600)
	first := DateInterval{Start: presAt(30), End: presAt(570)}
	check := func(what string, got, want float64) {
		t.Helper()
		if got != want {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
	check("unpaused", PauseElapsed(since, nil, nil, at), 600)
	check("one paused stretch", PauseElapsed(since, []DateInterval{first}, nil, at), 60)
	// Still paused: the number stands where it stopped.
	check("still paused", PauseElapsed(since, nil, new(at), presAt(3600)), 600)
	// Continued, then paused again later: the real work in between counts,
	// the time after does not.
	again := presAt(720)
	check("paused again", PauseElapsed(since, []DateInterval{first}, new(again), again), 180)
	check("paused again, later", PauseElapsed(since, []DateInterval{first}, new(again), presAt(1020)), 180)
	second := DateInterval{Start: again, End: presAt(1020)}
	check("two stretches", PauseElapsed(since, []DateInterval{first, second}, nil, presAt(1220)), 380)
}

// TestAPauseBeforeAnAttemptStartedDoesNotCountAgainstIt
func TestAPauseBeforeAnAttemptStartedDoesNotCountAgainstIt(t *testing.T) {
	t.Parallel()
	// A night's pause that ended before the attempt began takes nothing off it.
	night := DateInterval{Start: presAt(-36000), End: presAt(-36000 + 28800)}
	if got := PauseElapsed(engineT0, []DateInterval{night}, nil, presAt(120)); got != 120 {
		t.Fatalf("night pause elapsed = %v, want 120", got)
	}
	// One that started before the attempt and ended during it takes off only its overlap.
	overlap := DateInterval{Start: presAt(-60), End: presAt(30)}
	if got := PauseElapsed(engineT0, []DateInterval{overlap}, nil, presAt(120)); got != 90 {
		t.Fatalf("overlapping pause elapsed = %v, want 90", got)
	}
}

// pullSubject is the review of acme/web#57.
func pullSubject() Subject {
	base := "develop"
	occurrence := "100"
	return Subject{Kind: SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
		URL: "https://github.com/acme/web/pull/57", Base: &base, Occurrence: &occurrence, IsOpen: true}
}

// TestAHeldRunWithoutAReasonSaysHeld pins the status text of a held run without a reason
func TestAHeldRunWithoutAReasonSaysHeld(t *testing.T) {
	t.Parallel()
	r := presRun(1, nil, false)
	r.State = RunStateHeld
	r.Reason = nil
	if got := StatusLocation(r, false, PauseLive).Text; got != "Triage · held" {
		t.Fatalf("location = %q, want Triage · held", got)
	}
}

// TestStatusDurationBoundaries is the boundary table of Status.duration: the steps fall at
// 60 s, 60 min and 24 h, a value rounds to the nearest second, and a negative span reads as 0s.
func TestStatusDurationBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		seconds float64
		want    string
	}{
		{-5, "0s"},
		{-0.4, "0s"},
		{59, "59s"},
		{59.5, "1m 0s"},
		{60, "1m 0s"},
		{3599, "59m 59s"},
		{3599.6, "1h 0m"},
		{3600, "1h 0m"},
		{86399, "23h 59m"},
		{86400, "24h 0m"},
	}
	for _, c := range cases {
		if got := StatusDuration(c.seconds); got != c.want {
			t.Errorf("StatusDuration(%v) = %q, want %q", c.seconds, got, c.want)
		}
	}
}
