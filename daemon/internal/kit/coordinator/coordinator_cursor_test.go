// Coordinator tests of the launch boundary for a Cursor chat: a launch is recorded before it
// starts, and a failure creates only one attempt.

package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
)

// recordedCursor records the saved attempt at the exact
// launch boundary, and its launch is refused. Its allocation fails when allocationFails is set.
type recordedCursor struct {
	*runner.SimulatedRunner
	t              *testing.T
	coord          *Coordinator
	allocationFail bool
	mu             sync.Mutex
	launched       bool
}

// PrepareSessionID returns the chat's id, or fails when the allocation is refused.
func (r *recordedCursor) PrepareSessionID(context.Context, core.Backend, string, string) (string, error) {
	if r.allocationFail {
		return "", core.NewFailure("allocation refused")
	}
	return "12345678-1234-1234-1234-123456789abc", nil
}

// Start checks the attempt saved before the launch, then refuses the launch.
func (r *recordedCursor) Start(_ context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.launched = true
	r.mu.Unlock()
	runs := r.coord.Runs()
	if len(runs) == 0 || len(runs[0].Attempts) == 0 {
		r.t.Errorf("no attempt was saved before the launch")
		return errors.New("launch refused")
	}
	saved := runs[0].Attempts[len(runs[0].Attempts)-1]
	if saved.SessionID == nil || *saved.SessionID != spec.SessionID {
		r.t.Errorf("saved session id = %v, want %s", saved.SessionID, spec.SessionID)
	}
	if saved.Session == nil || *saved.Session != spec.Name {
		r.t.Errorf("saved session = %v, want %s", saved.Session, spec.Name)
	}
	return core.NewFailure("launch refused")
}

// Status reports the session gone.
func (r *recordedCursor) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	return runner.SessionStatus{Kind: runner.SessionStatusGone}
}

// Finish does nothing.
func (r *recordedCursor) Finish(context.Context, runner.SessionSpec) {}

// Launched reports whether Start was reached.
func (r *recordedCursor) Launched() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.launched
}

// TestRecordsBeforeLaunchAndFailureCreatesOnlyOneAttempt: a Cursor run is recorded before its launch, and a failed launch leaves one attempt.
func TestRecordsBeforeLaunchAndFailureCreatesOnlyOneAttempt(t *testing.T) {
	t.Parallel()
	for _, allocationFails := range []bool{false, true} {
		name := "launches"
		if allocationFails {
			name = "allocation fails"
		}
		t.Run(name, func(t *testing.T) {
			belt := core.SampleBelt([]string{"acme/web"}, "agent", core.SampleAgents(), true)
			cursor := core.BackendCursor
			belt.Stages[0].Runner = &cursor
			belt.Policies.CommentOnHold = false
			h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}, Issues: []core.Subject{}})
			rc := &recordedCursor{SimulatedRunner: h.idleSimulatedRunner(), t: t, allocationFail: allocationFails}
			coordinator := h.RestartWith(rc)
			rc.coord = coordinator
			run := coordAdd(coordinator, core.NewRun(core.RunArgs{Belt: belt, Subject: harnessIssue(1, "Work"), Now: sessionSampleT0}))
			locked(coordinator, func() { coordinator.sessions.startQueued(context.Background()) })
			coordinator.Idle()
			held := coordinator.Run(run.ID)
			if held == nil {
				t.Fatal("the run is gone")
			}
			if held.State != core.RunStateHeld {
				t.Errorf("state = %s, want held", held.State)
			}
			if len(held.Attempts) != 1 {
				t.Fatalf("attempts = %d, want 1", len(held.Attempts))
			}
			if rc.Launched() != !allocationFails {
				t.Errorf("launched = %v, want %v", rc.Launched(), !allocationFails)
			}
			// Either way the agent never ran: an answer starts a fresh chat with
			// the whole brief instead of resuming one that holds nothing.
			if held.Attempts[0].SessionID != nil {
				t.Errorf("session id = %v, want none", held.Attempts[0].SessionID)
			}
		})
	}
}
