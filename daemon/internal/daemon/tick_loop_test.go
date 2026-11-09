package daemon

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// errTickDone ends a test's tick loop from its sleep.
var errTickDone = errors.New("done")

// TestTicksImmediatelyAndWaitsTwoSecondsAfterCompletion
func TestTicksImmediatelyAndWaitsTwoSecondsAfterCompletion(t *testing.T) {
	t.Parallel()
	ticks := 0
	var waits []time.Duration
	loop := TickLoop{
		Interval: 2 * time.Second,
		Tick:     func() { ticks++ },
		Sleep: func(_ <-chan struct{}, d time.Duration) error {
			waits = append(waits, d)
			return errTickDone
		},
	}
	loop.Run(make(chan struct{}))
	if ticks != 1 {
		t.Fatalf("ticks %d, want 1", ticks)
	}
	if len(waits) != 1 || waits[0] != 2*time.Second {
		t.Fatalf("waits %v, want [2s]", waits)
	}
}

// TestASlowTickNeverOverlapsAndShutdownWaitsForIt
// The loop runs
// each tick in its own call, so a tick never starts before the previous one
// returns; the test keeps the active-count assertions. The
// 20 ms tick body is dropped: nothing in it is timed.
func TestASlowTickNeverOverlapsAndShutdownWaitsForIt(t *testing.T) {
	t.Parallel()
	active := 0
	maximumActive := 0
	ticks := 0
	loop := TickLoop{
		Interval: time.Millisecond,
		Tick: func() {
			active++
			maximumActive = max(maximumActive, active)
			ticks++
			active--
		},
		Sleep: func(_ <-chan struct{}, _ time.Duration) error {
			if ticks == 2 {
				return errTickDone
			}
			return nil
		},
	}
	loop.Run(make(chan struct{}))
	if ticks != 2 {
		t.Fatalf("ticks %d, want 2", ticks)
	}
	if maximumActive != 1 {
		t.Fatalf("maximum active %d, want 1", maximumActive)
	}
	if active != 0 {
		t.Fatalf("active %d after shutdown, want 0", active)
	}
}

// TestCancellationWaitsForTheActiveTickToFinish
func TestCancellationWaitsForTheActiveTickToFinish(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Bool
	loop := TickLoop{
		Interval: time.Second,
		Tick: func() {
			active.Store(true)
			close(started)
			<-release
			active.Store(false)
		},
		Sleep: func(_ <-chan struct{}, _ time.Duration) error { return errTickDone },
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		loop.Run(done)
	}()
	<-started
	close(done)
	if !active.Load() {
		t.Fatal("cancelling ended the active tick")
	}
	close(release)
	<-finished
	if active.Load() {
		t.Fatal("loop returned while its tick was still active")
	}
}

// TestAWakeEndsTheSleepAndAWakeDuringATickIsKept
func TestAWakeEndsTheSleepAndAWakeDuringATickIsKept(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	wake := make(chan struct{}, 1)
	ticks := 0
	loop := TickLoop{
		Interval: time.Hour,
		Wake:     wake,
		Tick: func() {
			ticks++
			if ticks == 1 {
				wake <- struct{}{}
				return
			}
			close(done)
		},
	}
	loop.Run(done)
	if ticks != 2 {
		t.Fatalf("ticks %d, want 2", ticks)
	}
}

// TestTimedTickReportsOnlySlowPasses
func TestTimedTickReportsOnlySlowPasses(t *testing.T) {
	t.Parallel()
	clock := time.Unix(1_000, 0)
	took := time.Second
	var reports []time.Duration
	tick := timedTick(func() { clock = clock.Add(took) }, func() time.Time { return clock }, func(d time.Duration) { reports = append(reports, d) })
	tick()
	took = slowTick
	tick()
	if len(reports) != 1 || reports[0] != slowTick {
		t.Fatalf("reports %v, want [%v]", reports, slowTick)
	}
}
