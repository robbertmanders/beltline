// The run's Sessions read (the terminal, the
// saved log, the selection and the empty copy) through the production router.
package servertest

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// attachableRunner is a simulated runner (one that never finishes a session)
// whose attach command names a terminal for every session, so the attachable path runs without tmux.
type attachableRunner struct {
	runner.AgentRunner
}

// AttachCommand answers the tmux attach command for any session, read-only or not.
func (attachableRunner) AttachCommand(_ context.Context, session string, _ bool) []string {
	return []string{"tmux", "-L", "beltline-test", "attach-session", "-t", session}
}

// finishedAttempt is the first finished agent attempt that has a session, with its run
// SessionsReadTests.finishedAttempt).
func finishedAttempt(t *testing.T, f *Fixture) (core.Run, core.Attempt) {
	t.Helper()
	for _, run := range f.Coordinator.ReadRuns() {
		for _, attempt := range run.Attempts {
			if attempt.Session != nil && !attempt.IsOpen() {
				return run, attempt
			}
		}
	}
	t.Fatal("no finished attempt with a session")
	return core.Run{}, core.Attempt{}
}

// sessionsPath is the Sessions read of run for the attempt's stage and id.
func sessionsPath(run core.Run, attempt core.Attempt) string {
	return "/api/v1/views/run/" + EncodeSegment(run.ID) + "/sessions?stage=" + EncodeSegment(attempt.StageID) +
		"&attempt=" + EncodeSegment(attempt.ID)
}

// decodeSessions decodes a Sessions read's reply body.
func decodeSessions(t *testing.T, reply Reply) coordinator.SessionsView {
	t.Helper()
	var response coordinator.SessionsView
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	return response
}

// TestMissingRunIsNotFound
func TestMissingRunIsNotFound(t *testing.T) {
	t.Parallel()
	f := ReadOnlyFixture(t)
	reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/missing-run/sessions", Token: ContractToken})
	if reply.Status != http.StatusNotFound {
		t.Fatalf("status %d, want 404", reply.Status)
	}
}

// TestAbsentAttachmentShowsNoTerminalAndTheSavedLog
func TestAbsentAttachmentShowsNoTerminalAndTheSavedLog(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	run, attempt := finishedAttempt(t, f)
	folder := f.Coordinator.Paths().AttemptFolder(run, attempt)
	mustWrite(t, folder, "session.log", []byte("original transcript"))
	mustWrite(t, folder, "continue.log", []byte("continued transcript"))

	reply := f.Do(Request{Method: "GET", Path: sessionsPath(run, attempt), Token: ContractToken})
	if reply.Status != http.StatusOK {
		t.Fatalf("status %d, want 200", reply.Status)
	}
	response := decodeSessions(t, reply)
	if response.Terminal != nil {
		t.Fatalf("terminal %+v, want none", *response.Terminal)
	}
	if response.Log == nil || *response.Log != "original transcript\n─────\ncontinued transcript" {
		t.Fatalf("log %v", response.Log)
	}
	if response.Selection.AttemptID == nil || *response.Selection.AttemptID != attempt.ID {
		t.Fatalf("selection %+v, want attempt %s", response.Selection, attempt.ID)
	}
	if response.Selection.Fallback != nil {
		t.Fatalf("fallback %q, want none", *response.Selection.Fallback)
	}
	if response.TakeOver != nil {
		t.Fatalf("take over %+v, want none", *response.TakeOver)
	}
	if response.Ghostty.Enabled {
		t.Fatal("ghostty is enabled for an absent attachment")
	}
	if response.ContinueSession == nil {
		t.Fatal("no continue session offer")
	}
}

// TestOneSavedLogAloneIsShownAndNoLogSaysSo
func TestOneSavedLogAloneIsShownAndNoLogSaysSo(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	run, attempt := finishedAttempt(t, f)
	folder := f.Coordinator.Paths().AttemptFolder(run, attempt)
	mustWrite(t, folder, "session.log", []byte("only the original"))
	_ = os.Remove(filepath.Join(folder, "continue.log"))

	response := decodeSessions(t, f.Do(Request{Method: "GET", Path: sessionsPath(run, attempt), Token: ContractToken}))
	if response.Log == nil || *response.Log != "only the original" {
		t.Fatalf("log %v, want only the original", response.Log)
	}

	_ = os.Remove(filepath.Join(folder, "session.log"))
	response = decodeSessions(t, f.Do(Request{Method: "GET", Path: sessionsPath(run, attempt), Token: ContractToken}))
	if response.Log == nil || *response.Log != "No log was saved for this session." {
		t.Fatalf("log %v, want the no-log copy", response.Log)
	}
}

// TestAttachableContinuationReportsItsTargetAsAWritableConversation
func TestAttachableContinuationReportsItsTargetAsAWritableConversation(t *testing.T) {
	t.Parallel()
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 3600 }, nil, nil)
	f := Start(t, Options{Runner: attachableRunner{AgentRunner: sim}})
	run, attempt := finishedAttempt(t, f)
	reply := f.Do(Request{Method: "GET", Path: sessionsPath(run, attempt), Token: ContractToken})
	if reply.Status != http.StatusOK {
		t.Fatalf("status %d, want 200", reply.Status)
	}
	response := decodeSessions(t, reply)
	if attempt.Session == nil {
		t.Fatal("the attempt has no session")
	}
	want := coordinator.SessionsViewTerminal{Target: runner.TmuxContinueName(*attempt.Session), ReadOnly: false, Continued: true}
	if response.Terminal == nil || *response.Terminal != want {
		t.Fatalf("terminal %+v, want %+v", response.Terminal, want)
	}
	if response.TakeOver != nil {
		t.Fatal("a finished attempt offers no take over")
	}
	if !response.Ghostty.Enabled {
		t.Fatal("an attached terminal may open in Ghostty when the run is not paused")
	}
	found := slices.ContainsFunc(response.Rows, func(row coordinator.SessionSidebarProjectionRow) bool {
		return row.AttemptID == attempt.ID && row.Selected
	})
	if !found {
		t.Fatal("no selected row for the attempt")
	}
}

// TestARunWithoutSessionsHasNoRowsAndTheEmptyCopy
func TestARunWithoutSessionsHasNoRowsAndTheEmptyCopy(t *testing.T) {
	t.Parallel()
	f := ReadOnlyFixture(t)
	var runID string
	for _, run := range f.Coordinator.ReadRuns() {
		empty := !slices.ContainsFunc(run.Attempts, func(attempt core.Attempt) bool {
			return attempt.Session != nil
		})
		if empty {
			runID = run.ID
			break
		}
	}
	if runID == "" {
		t.Fatal("no run without sessions")
	}
	reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/" + EncodeSegment(runID) + "/sessions", Token: ContractToken})
	if reply.Status != http.StatusOK {
		t.Fatalf("status %d, want 200", reply.Status)
	}
	response := decodeSessions(t, reply)
	if len(response.Rows) != 0 {
		t.Fatalf("rows %d, want none", len(response.Rows))
	}
	if response.Selection.AttemptID != nil {
		t.Fatalf("selection attempt %q, want none", *response.Selection.AttemptID)
	}
	if response.Terminal != nil || response.Log != nil {
		t.Fatal("terminal or log present for a run without sessions")
	}
	if response.EmptyMessage == nil || !strings.HasPrefix(*response.EmptyMessage, "No sessions yet.") {
		t.Fatalf("empty message %v", response.EmptyMessage)
	}
}
