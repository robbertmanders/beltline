package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// laneRunning runs one pass of lane in the background; the channel closes when it ends.
func laneRunning(h *harness, lane Lane) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.coord.TickLane(context.Background(), lane)
	}()
	return done
}

// TestASlowTriggerCheckDoesNotHoldUpSessions
func TestASlowTriggerCheckDoesNotHoldUpSessions(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 10})
	h.Tick()
	// The agent has finished and the trigger is due.
	h.clock.Advance(2 * time.Minute)
	gate := newMultiGate()
	h.github.BeforeIssueList = gate.wait
	defer gate.releaseNow()
	polling := laneRunning(h, LaneIntake)
	gate.awaitEntered(t)
	if !h.coord.Checking() {
		t.Fatal("the held check is not reported as checking")
	}

	h.coord.TickLane(context.Background(), LaneSessions)
	h.coord.TickLane(context.Background(), LaneStarts)
	if run := h.Run(); run.StageID != "spec" || run.State != core.RunStateRunning {
		t.Fatalf("stage %s state %s during the check, want spec running", run.StageID, run.State)
	}
	gate.releaseNow()
	<-polling
	if h.coord.Checking() {
		t.Fatal("still checking after the check ended")
	}
}

// TestASlowWatchDoesNotHoldUpSessions
func TestASlowWatchDoesNotHoldUpSessions(t *testing.T) {
	t.Parallel()
	f := coordTwoRepositories(t, nil)
	f.gh.HoldPulls()
	defer f.gh.ReleasePulls()
	watching := laneRunning(f.h, LaneWatches)
	testkit.Eventually(t, 10*time.Second, "the watch waits on a pull request", func() bool {
		return f.gh.WaitingPulls() > 0
	})
	h := f.h
	h.coord.TickLane(context.Background(), LaneSessions)
	h.coord.TickLane(context.Background(), LaneStarts)
	h.coord.TickLane(context.Background(), LaneIntake)
	f.gh.ReleasePulls()
	<-watching
}

// gatedStartRunner holds Start at a gate once one is set.
type gatedStartRunner struct {
	*runner.SimulatedRunner
	gate *multiGate
}

func (r *gatedStartRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	if r.gate != nil {
		r.gate.wait(ctx)
	}
	return r.SimulatedRunner.Start(ctx, spec)
}

// TestASlowStartDoesNotHoldUpIntakeOrSessions
func TestASlowStartDoesNotHoldUpIntakeOrSessions(t *testing.T) {
	t.Parallel()
	var h *harness
	var gated *gatedStartRunner
	h = newHarness(t, harnessOptions{StageDuration: 10, Runner: func(script *harnessScript) runner.AgentRunner {
		gated = &gatedStartRunner{SimulatedRunner: runner.NewSimulatedRunner(
			func(runner.SessionSpec) float64 { return 10 },
			func() time.Time { return h.clock.Now() },
			script.next,
		)}
		return gated
	}})
	h.Tick()
	h.clock.Advance(2 * time.Minute)
	h.coord.TickLane(context.Background(), LaneSessions)
	gate := newMultiGate()
	gated.gate = gate
	defer gate.releaseNow()
	starting := laneRunning(h, LaneStarts)
	gate.awaitEntered(t)

	h.github.Put(flowIssue("acme/billing", 200, "New", "agent"))
	h.coord.TickLane(context.Background(), LaneSessions)
	h.coord.TickLane(context.Background(), LaneIntake)
	if got := len(h.coord.Runs()); got != 2 {
		t.Fatalf("runs = %d during the start, want 2", got)
	}
	gate.releaseNow()
	<-starting
}

// TestLanesWakeOnAKickAndCoalesce
func TestLanesWakeOnAKickAndCoalesce(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	wakes := h.coord.StartLanes()
	h.Turn(func() {
		h.coord.kick()
		h.coord.kick()
	})
	for _, lane := range AllLanes {
		select {
		case <-wakes[lane]:
		default:
			t.Fatalf("lane %s was not woken", lane)
		}
		select {
		case <-wakes[lane]:
			t.Fatalf("lane %s was woken twice", lane)
		default:
		}
	}
}

// TestALaneAndAFullTickExcludeEachOther
func TestALaneAndAFullTickExcludeEachOther(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 10})
	h.Tick()
	h.clock.Advance(2 * time.Minute)
	gate := newMultiGate()
	h.github.BeforeIssueList = gate.wait
	defer gate.releaseNow()
	polling := laneRunning(h, LaneIntake)
	gate.awaitEntered(t)
	h.coord.Tick(context.Background())
	if run := h.Run(); run.StageID != "triage" {
		t.Fatalf("a tick ran during a lane: stage %s", run.StageID)
	}
	gate.releaseNow()
	<-polling
}

// TestOverlappingChecksStayCheckingUntilTheLastEnds
func TestOverlappingChecksStayCheckingUntilTheLastEnds(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Turn(func() {
		h.coord.intake.setChecking(true)
		h.coord.intake.setChecking(true)
		h.coord.intake.setChecking(false)
		if !h.coord.intake.isChecking() {
			t.Error("the first check to end cleared checking")
		}
		h.coord.intake.setChecking(false)
		if h.coord.intake.isChecking() {
			t.Error("still checking after both ended")
		}
	})
}
