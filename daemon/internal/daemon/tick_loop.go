// The loop that ticks one lane of the coordinator every two seconds.

package daemon

import (
	"errors"
	"time"
)

// DefaultTickInterval is the wait between one tick's completion and the next.
const DefaultTickInterval = 2 * time.Second

// slowTick is how long a lane's pass runs before the daemon log records it.
const slowTick = 10 * time.Second

// errTickSleepEnded is what the default sleep returns once the loop is cancelled.
var errTickSleepEnded = errors.New("tick loop cancelled")

// TickSleep waits d, or returns an error when done closes first.
type TickSleep func(done <-chan struct{}, d time.Duration) error

// TickLoop ticks the coordinator forever: one tick, then the interval after it
// completes. A slow tick never overlaps the next one. Cancellation waits for the
// tick in progress.
type TickLoop struct {
	Interval time.Duration
	Tick     func()
	Sleep    TickSleep
	// Wake ends the default sleep early. Nil never does.
	Wake <-chan struct{}
}

// Run ticks until done closes. A tick that is running when done closes finishes first.
// Lock: none; the tick takes the coordinator's lock itself.
func (l TickLoop) Run(done <-chan struct{}) {
	sleep := l.Sleep
	if sleep == nil {
		sleep = func(done <-chan struct{}, d time.Duration) error { return wakeableTickSleep(done, l.Wake, d) }
	}
	for !closed(done) {
		l.Tick()
		if closed(done) {
			return
		}
		if err := sleep(done, l.Interval); err != nil {
			return
		}
	}
}

// wakeableTickSleep is the default sleep: it returns early when wake signals, and
// with an error when done closes.
// Lock: none.
func wakeableTickSleep(done, wake <-chan struct{}, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-wake:
		return nil
	case <-done:
		return errTickSleepEnded
	}
}

// closed reports whether done has been closed, without blocking.
// Lock: none.
func closed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// timedTick is tick, reporting how long a pass ran once that reaches slowTick.
// Lock: none.
func timedTick(tick func(), now func() time.Time, report func(time.Duration)) func() {
	return func() {
		start := now()
		tick()
		if took := now().Sub(start); took >= slowTick {
			report(took)
		}
	}
}
