// Coordinator tests of parking: parking and unparking post nothing and keep the checkout, a stop that
// is still finishing, unpark waiting for a full stage, a paused belt or the global pause, model
// admission, watch reads after parking, stale completions and AFK decisions, edits and closure,
// sending a split run on, a parked run surviving a reload, a failed save, and the answer box.

package coordinator

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// coordLatch is the stop gate: waiters block until the latch opens; open is safe to call more than once.
type coordLatch struct {
	once sync.Once
	ch   chan struct{}
}

// newH5m4Latch makes a closed-by-default-open latch that is not yet open.
func newH5m4Latch() *coordLatch { return &coordLatch{ch: make(chan struct{})} }

// open lets every waiter go on.
func (l *coordLatch) open() { l.once.Do(func() { close(l.ch) }) }

// wait blocks until the latch opens.
func (l *coordLatch) wait() { <-l.ch }

// approvalBeltStages are a triage agent and an approval gate.
func approvalBeltStages() []core.Stage {
	triage := core.DefaultsStageForAgent(core.SampleAgent("triage"), nil, nil, nil, core.TriggerKindIssues)
	approve := core.DefaultsStage(core.StageTypeApprove, []core.Stage{triage}, nil, []core.Stage{triage})
	return []core.Stage{triage, approve}
}

// TestParkingAndUnparkingPostNothingAndKeepTheCheckout
func TestParkingAndUnparkingPostNothingAndKeepTheCheckout(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	running := h.Run()
	folder := h.coord.Paths().WorkFolder(*running)
	session := runSession(running)
	attempts := len(running.Attempts)
	sim := simOf(h.coord)
	h.coord.Park(running.ID)
	if h.Run().State != core.RunStateParked || len(h.Run().Attempts) != attempts {
		t.Fatalf("after parking = %s with %d attempts, want parked with %d", h.Run().State, len(h.Run().Attempts), attempts)
	}
	if last := h.Run().Attempts[len(h.Run().Attempts)-1]; last.Route == nil || *last.Route != "Parked by you" {
		t.Fatalf("route = %v, want Parked by you", last.Route)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("work folder: %v", err)
	}
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.Parking != nil && len(run.Parking.Stopping) == 0
	})
	if sim.IsRunning(session) {
		t.Fatal("the parked session still runs")
	}
	h.coord.Unpark(running.ID)
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.State == core.RunStateRunning && len(run.Attempts) == attempts+1
	})
	restored := h.Run()
	if restored.StageID != running.StageID {
		t.Fatalf("stage = %s, want %s", restored.StageID, running.StageID)
	}
	last := restored.Attempts[len(restored.Attempts)-1]
	if last.Replays == nil || *last.Replays != running.Attempts[len(running.Attempts)-1].ID || last.Continues != nil {
		t.Fatalf("replay/continue = %v/%v, want replaying the parked attempt and no continue", last.Replays, last.Continues)
	}
	if !core.SamePointee(restored.Branch, running.Branch) {
		t.Fatalf("the branch changed: %v -> %v", running.Branch, restored.Branch)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("work folder after unpark: %v", err)
	}
	saved := h.storage.LoadRuns()
	if len(saved) == 0 || saved[0].Parking != nil {
		t.Fatalf("saved run = %+v, want no parking", saved)
	}
}

// TestAStopStillFinishingDoesNotStartAReplacementAndAFailureStaysParked
func TestAStopStillFinishingDoesNotStartAReplacementAndAFailureStaysParked(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	attempts := len(h.Run().Attempts)
	session := runSession(h.Run())
	sim := simOf(h.coord)
	hold := newH5m4Latch()
	entered := newH5m4Latch()
	sim.BeforeStop = func(string) {
		entered.open()
		hold.wait()
	}
	id := h.Run().ID
	h.coord.Park(id)
	entered.wait()
	h.coord.Unpark(id)
	if run := h.Run(); run.Parking == nil || !run.Parking.UnparkPending || run.State != core.RunStateParked || len(run.Attempts) != attempts {
		t.Fatalf("during the stop = %+v, want parked with unpark pending", run)
	}
	failure := "the session would not stop"
	sim.StopError = &failure
	hold.open()
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.Parking != nil && run.Parking.StopError != nil
	})
	if h.Run().State != core.RunStateParked || len(h.Run().Attempts) != attempts {
		t.Fatal("the run left parking while the stop failed")
	}
	if got := h.Run().Parking.Stopping; len(got) != 1 || got[0] != session {
		t.Fatalf("stopping = %v, want [%s]", got, session)
	}
	if !sim.IsRunning(session) {
		t.Fatal("the session stopped despite the failure")
	}
	if !hasNoticeContaining(h.coord.Notices(), "the session would not stop") {
		t.Fatal("no notice about the stop")
	}
	// Unpark retries the stop. While it still fails, no replacement starts.
	h.coord.Unpark(id)
	if h.Run().State != core.RunStateParked || len(h.Run().Attempts) != attempts {
		t.Fatal("the retry changed the run before it finished")
	}
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.Parking != nil && !run.Parking.UnparkPending && run.Parking.StopError != nil
	})
	if h.Run().State != core.RunStateParked || len(h.Run().Attempts) != attempts {
		t.Fatal("a replacement started while the stop failed")
	}
	if got := h.Run().Parking.Stopping; len(got) != 1 || got[0] != session || !sim.IsRunning(session) {
		t.Fatalf("stopping = %v, running = %v", got, sim.IsRunning(session))
	}
	sim.StopError = nil
	h.coord.Unpark(id)
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	trail := sim.Trail()
	stoppedAt, startedAt := -1, -1
	for i, entry := range trail {
		if entry == "stop "+session {
			stoppedAt = i
		}
		if strings.HasPrefix(entry, "start ") {
			startedAt = i
		}
	}
	if stoppedAt < 0 || startedAt < 0 || stoppedAt >= startedAt {
		t.Fatalf("trail = %v, want the stop before the start", trail)
	}
	if sim.IsRunning(session) {
		t.Fatal("the old session still runs")
	}
	if len(h.Run().Attempts) != attempts+1 || h.Run().Parking != nil {
		t.Fatalf("after the retry = %d attempts, parking %+v", len(h.Run().Attempts), h.Run().Parking)
	}
}

// TestUnparkWaitsForAFullStageAPausedBeltAndAGlobalPause
func TestUnparkWaitsForAFullStageAPausedBeltAndAGlobalPause(t *testing.T) {
	t.Parallel()
	belt := issuesBelt(nil)
	belt.Stages = []core.Stage{core.DefaultsStageForAgent(core.SampleAgent("triage"), nil, nil, nil, core.TriggerKindIssues)}
	belt.Stages[0].Slots = 1
	issues := []core.Subject{coordSubject(1, "One"), coordSubject(2, "Two")}
	h := newHarness(t, harnessOptions{Issues: issues, Belts: []core.Belt{belt}, StageDuration: 3600})
	h.Until("return runningCount(h.nowRuns()) == 1 && len(h.nowRuns()) == 2 ...", func() bool {
		return runningCount(h.nowRuns()) == 1 && len(h.nowRuns()) == 2
	})
	var queued core.Run
	for _, run := range h.coord.Runs() {
		if run.State == core.RunStateQueued {
			queued = run
		}
	}
	h.coord.Park(queued.ID)
	h.coord.Unpark(queued.ID)
	h.Tick()
	if got := h.coord.Run(queued.ID); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("after unpark = %+v, want queued", got)
	}
	if err := h.coord.SetPaused(queued.BeltID, true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	h.Tick()
	if got := h.coord.Run(queued.ID); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("on a paused belt = %+v, want queued", got)
	}
	if err := h.coord.SetPaused(queued.BeltID, false); err != nil {
		t.Fatalf("unpause belt: %v", err)
	}
	h.coord.SetGloballyPaused(context.Background(), true)
	h.Tick()
	if got := h.coord.Run(queued.ID); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("while paused = %+v, want queued", got)
	}
	h.coord.SetGloballyPaused(context.Background(), false)
	// The other run still holds the only slot.
	h.Tick()
	if got := h.coord.Run(queued.ID); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("with the slot taken = %+v, want queued", got)
	}
}

// TestAPullRequestMergedWhileParkedIsReportedAfterUnparkAndGraceDoesNotMove
func TestAPullRequestMergedWhileParkedIsReportedAfterUnparkAndGraceDoesNotMove(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordBuildThenWatch()}, OpensNoPulls: true})
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.Stage() != nil && run.Stage().Kind() == core.StageKindWatch
	})
	started := h.Run().Attempts[len(h.Run().Attempts)-1].StartedAt.Time
	h.clock.Set(started.Add(30 * time.Second))
	id := h.Run().ID
	h.coord.Park(id)
	if h.Run().State != core.RunStateParked {
		t.Fatalf("state = %s, want parked", h.Run().State)
	}
	h.clock.Set(started.Add(30*time.Second + watchesGrace))
	h.coord.CheckNow(context.Background())
	if h.Run().State != core.RunStateParked {
		t.Fatal("a check moved the parked run")
	}
	if last := h.Run().Attempts[len(h.Run().Attempts)-1]; !last.IsOpen() {
		t.Fatal("the watch attempt closed while parked")
	}
	branch := *h.Run().Branch
	h.github.OpenPull("acme/billing", branch, core.PullRequestStateMerged)
	h.coord.CheckNow(context.Background())
	if h.Run().State != core.RunStateParked {
		t.Fatal("a merged pull request moved the parked run")
	}
	h.coord.Unpark(id)
	shifted := h.Run().Attempts[len(h.Run().Attempts)-1].StartedAt.Time
	if got := shifted.Sub(started); got != watchesGrace {
		t.Fatalf("grace shift = %s, want %s", got, watchesGrace)
	}
	h.Until("r := h.nowRun() ...", func() bool {
		r := h.nowRun()
		return slices.ContainsFunc(r.Attempts, func(attempt core.Attempt) bool {
			return attempt.StageID == "watch" && attempt.Outcome != nil && *attempt.Outcome == "merged"
		})
	})
	for _, attempt := range h.Run().Attempts {
		if attempt.StageID == "watch" {
			if attempt.Outcome == nil || *attempt.Outcome != "merged" {
				t.Fatalf("watch outcome = %v, want merged", attempt.Outcome)
			}
			return
		}
	}
	t.Fatal("no watch attempt")
}

// TestAWatchReadThatReturnsAfterParkingDoesNotChangeTheRun
func TestAWatchReadThatReturnsAfterParkingDoesNotChangeTheRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordBuildThenWatch()}, OpensNoPulls: true})
	h.Until("r := h.nowRun() ...", func() bool {
		r := h.nowRun()
		return r != nil && r.Stage() != nil && r.Stage().Kind() == core.StageKindWatch && r.State == core.RunStateRunning
	})
	branch := *h.Run().Branch
	pull := h.github.OpenPull("acme/billing", branch, core.PullRequestStateOpen)
	h.github.HoldPulls()
	checking := make(chan struct{})
	go func() {
		defer close(checking)
		h.coord.Lock()
		h.coord.watches.check(context.Background(), true)
		h.coord.EndTurn()
	}()
	coordWaits(t, "the pull request read waits", func() bool { return h.github.WaitingPulls() > 0 })
	before := h.Run()
	id := h.Run().ID
	h.coord.Park(id)
	h.github.SetPullState(pull.Number, core.PullRequestStateMerged, nil)
	h.coord.Unpark(id)
	h.github.ReleasePulls()
	<-checking
	if h.Run().Parking != nil || h.Run().State != core.RunStateRunning {
		t.Fatalf("run = %s (%v), want running and unparked", h.Run().State, h.Run().Parking)
	}
	if last := h.Run().Attempts[len(h.Run().Attempts)-1]; last.Outcome != nil {
		t.Fatalf("watch outcome = %v, want none", *last.Outcome)
	}
	if h.Run().StageID != before.StageID {
		t.Fatal("the stage moved")
	}
}

// TestAMissedPullRequestReadThatReturnsAfterUnparkDoesNotHoldTheWatch
func TestAMissedPullRequestReadThatReturnsAfterUnparkDoesNotHoldTheWatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordBuildThenWatch()}, OpensNoPulls: true})
	h.Until("r := h.nowRun() ...", func() bool {
		r := h.nowRun()
		return r != nil && r.Stage() != nil && r.Stage().Kind() == core.StageKindWatch && r.State == core.RunStateRunning
	})
	started := h.Run().Attempts[len(h.Run().Attempts)-1].StartedAt.Time
	h.clock.Set(started.Add(watchesGrace + 5*time.Second))
	h.github.HoldPulls()
	checking := make(chan struct{})
	go func() {
		defer close(checking)
		h.coord.Lock()
		h.coord.watches.check(context.Background(), true)
		h.coord.EndTurn()
	}()
	coordWaits(t, "the pull request read waits", func() bool { return h.github.WaitingPulls() > 0 })
	id := h.Run().ID
	h.coord.Park(id)
	h.coord.Unpark(id)
	if h.coord.Run(id).State != core.RunStateRunning {
		t.Fatalf("state after unpark = %s, want running", h.coord.Run(id).State)
	}
	h.github.ReleasePulls()
	<-checking
	h.coord.Tick(context.Background())
	h.coord.Idle()
	run := h.coord.Run(id)
	if run.State != core.RunStateRunning || !run.Attempts[len(run.Attempts)-1].IsOpen() || run.Attempts[len(run.Attempts)-1].Outcome != nil {
		t.Fatalf("run after the late read = %s, attempts %+v; want running with an open watch", run.State, run.Attempts[len(run.Attempts)-1])
	}
}

// TestStaleSessionCompletionAndAfkDoNotMoveAParkedRun
func TestStaleSessionCompletionAndAfkDoNotMoveAParkedRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600, AfkDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	entered := newH5m4Latch()
	hold := newH5m4Latch()
	sim := simOf(h.coord)
	sim.BeforeStatus = func(runner.SessionSpec) {
		entered.open()
		hold.wait()
	}
	checking := make(chan struct{})
	go func() {
		defer close(checking)
		h.coord.Lock()
		h.coord.sessions.check(context.Background())
		h.coord.EndTurn()
	}()
	entered.wait()
	attempts := len(h.Run().Attempts)
	id := h.Run().ID
	h.coord.Park(id)
	h.coord.Unpark(id)
	hold.open()
	<-checking
	h.coord.Tick(context.Background())
	h.coord.Idle()
	if !slices.ContainsFunc(h.Run().Attempts, func(attempt core.Attempt) bool {
		return attempt.Route != nil && *attempt.Route == "Parked by you"
	}) {
		t.Fatal("the park was lost")
	}
	if got := len(h.Run().Attempts); got != attempts+1 && h.Run().State != core.RunStateQueued && h.Run().State != core.RunStateRunning {
		t.Fatalf("attempts = %d, state = %s", got, h.Run().State)
	}
	if h.Run().StageID != "triage" {
		t.Fatalf("stage = %s, want triage", h.Run().StageID)
	}
}

// TestSubjectEditsWhileParkedDoNotRestartAndClosureStillDrops
func TestSubjectEditsWhileParkedDoNotRestartAndClosureStillDrops(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{issuesBelt(approvalBeltStages())}, StageDuration: 0})
	waitingIt(h)
	id := h.Run().ID
	operation := h.Run().Operation
	generation := h.Run().Generation
	h.coord.Park(id)
	edited := h.Run().Subject
	edited.Body = "Edited while parked."
	h.Turn(func() {
		h.coord.intake.tell(id, Fate{Kind: FateChanged, Subject: edited}, generation, operation, false)
	})
	if got := h.coord.Run(id); got == nil || got.Subject.Body != "Edited while parked." {
		t.Fatalf("body = %+v, want the edit", got)
	}
	if got := h.coord.Run(id); got.StageID != h.Run().StageID || !got.IsButtonParked() {
		t.Fatal("the edit moved the run or cleared its park")
	}
	h.coord.Unpark(id)
	if h.coord.Run(id).State != core.RunStateWaiting {
		t.Fatalf("state after unpark = %s, want waiting", h.coord.Run(id).State)
	}
	h.Turn(func() {
		h.coord.intake.tell(id, Fate{Kind: FateChanged, Subject: edited}, generation, operation, false)
	})
	if h.coord.Run(id).State != core.RunStateWaiting {
		t.Fatal("an edit after unpark restarted the run")
	}
	h.coord.Park(id)
	h.Turn(func() {
		h.coord.intake.tell(id, Fate{Kind: FateGone, Subject: edited, Reason: "the issue was closed"}, generation, operation, false)
	})
	if got := h.coord.Run(id); got == nil || got.State != core.RunStateDropped || got.Reason == nil || *got.Reason != "the issue was closed" {
		t.Fatalf("closed run = %+v, want dropped for the closure", got)
	}
}

// TestParkingSurvivesReloadAndAMissedStopIsFinishedBeforeReplacement
func TestParkingSurvivesReloadAndAMissedStopIsFinishedBeforeReplacement(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	belt := h.coord.Belts()[0].Belt
	if err := h.coord.SetPaused(belt.ID, true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	var added core.Run
	h.Turn(func() {
		added = h.coord.book.add(core.NewRun(core.RunArgs{Belt: belt, Subject: coordSubject(142, "Export invoices as CSV"), Now: testkit.ContractTime}))
	})
	id := added.ID
	h.coord.Park(id)
	h.Turn(func() {
		h.coord.book.update(id, nil, nil, func(r *core.Run) { r.Parking.Stopping = []string{"ghost-session"} })
	})
	reopened := h.RestartWith(runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 3600 },
		h.clock.Now, func(runner.SessionSpec) *string { return nil }))
	if run := reopened.Run(id); run == nil || !run.IsButtonParked() {
		t.Fatalf("reopened run = %+v, want button parked", run)
	}
	for range 20 {
		reopened.Tick(context.Background())
		reopened.Idle()
		if run := reopened.Run(id); run != nil && run.Parking != nil && len(run.Parking.Stopping) == 0 {
			break
		}
	}
	run := reopened.Run(id)
	if run == nil || run.Parking == nil || len(run.Parking.Stopping) != 0 || run.State != core.RunStateParked || len(run.Attempts) != 0 {
		t.Fatalf("reopened after the stop = %+v, want parked with nothing stopping and no attempts", run)
	}
	reopened.Unpark(id)
	reopened.Tick(context.Background())
	reopened.Idle()
	if got := reopened.Run(id); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("after unpark = %+v, want queued", got)
	}
}

// TestAFailedSaveLeavesTheRunAndStartsNothing
func TestAFailedSaveLeavesTheRunAndStartsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	id := h.Run().ID
	session := runSession(h.Run())
	before := h.Run()
	folder := h.coord.Paths().RunFolder(*h.Run())
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(folder, 0o755) }()
	h.coord.Park(id)
	if !runsEqual(h.Run(), before) {
		t.Fatal("a failed save changed the run")
	}
	if !hasNoticeContaining(h.coord.Notices(), "Could not save") {
		t.Fatal("no save failure notice")
	}
	if !simOf(h.coord).IsRunning(session) {
		t.Fatal("the session stopped after a failed save")
	}
	if err := os.Chmod(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	h.coord.Park(id)
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && run.Parking != nil && len(run.Parking.Stopping) == 0
	})
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	parked := h.Run()
	h.coord.Unpark(id)
	if !runsEqual(h.Run(), parked) {
		t.Fatal("a failed unpark save changed the run")
	}
	if !hasNoticeContaining(h.coord.Notices(), "Could not save") {
		t.Fatal("no save failure notice for the unpark")
	}
	if h.Run().State != core.RunStateParked {
		t.Fatalf("state = %s, want parked", h.Run().State)
	}
	if err := os.Chmod(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	h.coord.Unpark(id)
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	if h.Run().Parking != nil {
		t.Fatal("the park did not clear")
	}
}

// runsEqual compares two runs by their wire form.
func runsEqual(a, b *core.Run) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, errA := jsonx.Marshal(*a)
	y, errB := jsonx.Marshal(*b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// TestATicketChangeDuringParkingIsReportedAfterUnparkWithoutUsingTheQuietPeriod checks a child that
// finishes while the run is parked: it is not seen until the run is unparked, and then only after the quiet period
// finishes while the run is parked is not seen until it is unparked, and then only after the quiet period
// that starts with the unpark (the change is not counted towards the quiet time spent parked).
func TestATicketChangeDuringParkingIsReportedAfterUnparkWithoutUsingTheQuietPeriod(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindJira, "", []core.Stage{epicTicketWatch(), epicApproval()})},
		tickets: []core.Subject{epicSubject, epicChildOf("PAY-41", false, nil)},
		now:     flowT0,
	})
	h.jira.Unmatch("PAY-41")
	flowCheck(h)
	h.Until("the run is on its watch and running", func() bool {
		r := h.nowRun()
		return r != nil && r.StageID == "watch" && r.State == core.RunStateRunning
	})
	flowCheck(h)
	if _, seen := h.Run().TicketObservations["watch"].Children["PAY-41"]; !seen {
		t.Fatal("the watch has not seen PAY-41")
	}
	seen := h.Run().TicketObservations["watch"]
	h.coord.Park(h.Run().ID)
	h.jira.SetStatus("PAY-41", "Done", true)
	h.clock.Set(flowT0.Add(time.Duration(core.TicketWatchQuiet+50) * time.Second))
	flowCheck(h)
	if h.Run().State != core.RunStateParked {
		t.Fatalf("state = %s, want parked", h.Run().State)
	}
	if !reflect.DeepEqual(h.Run().TicketObservations["watch"], seen) {
		t.Fatal("the watch read the change while the run was parked")
	}
	h.coord.Unpark(h.Run().ID)
	if h.Run().Parking != nil {
		t.Fatal("parking is still set after unpark")
	}
	if h.Run().State != core.RunStateRunning {
		t.Fatalf("state after unpark = %s, want running", h.Run().State)
	}
	pending := h.Run().TicketObservations["watch"].PendingSince
	state, err := h.jira.TicketState(context.Background(), "PAY-40")
	if err != nil || len(state.Children) == 0 || state.Children[0].Status != "Done" {
		t.Fatalf("PAY-40 children = %+v, %v; want PAY-41 done", state.Children, err)
	}
	flowCheck(h)
	if h.Run().StageID != "watch" {
		t.Fatalf("stage = %s, want watch", h.Run().StageID)
	}
	if got := h.Run().TicketObservations["watch"].PendingSince; got == nil && pending != nil {
		t.Fatal("the change is no longer pending")
	}
	since := h.Run().TicketObservations["watch"].PendingSince
	if since == nil {
		t.Fatal("the change is not pending after unpark")
	}
	if h.clock.Now().Sub(since.Time) >= time.Duration(core.TicketWatchQuiet)*time.Second {
		t.Fatalf("the quiet period ran while parked: %v", h.clock.Now().Sub(since.Time))
	}
	h.clock.Set(since.Add(time.Duration(core.TicketWatchQuiet+1) * time.Second))
	flowCheck(h)
	if h.Run().StageID != "approve" {
		t.Fatalf("stage = %s after the quiet period, want approve", h.Run().StageID)
	}
}
