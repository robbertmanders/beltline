// Coordinator tests of the decisions a view makes on a run. The drafts themselves are the web
// app's unsent input, so only the coordinator's side is checked. Each decision holds the tick it
// kicks until the assertions have run: holdKicks holds the coordinator's spawned work, and the
// test's cleanup releases it.

package coordinator

import (
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// draftsCoordinator is a coordinator with an empty library, with its setup work finished.
func draftsCoordinator(t *testing.T) (*harness, *Coordinator) {
	t.Helper()
	h := newLibraryHarness(t)
	c := h.Coord()
	c.Idle()
	return h, c
}

// holdKicks holds the work a decision spawns until the test ends, so the assertions see the turn
// the decision made and nothing after it; the cleanup releases the held work (Turns.Hold).
func holdKicks(t *testing.T, c *Coordinator) {
	t.Helper()
	c.Hold()
	t.Cleanup(c.Release)
}

// coordHeldRun is a held run of the default belt on acme/billing at its spec stage, with one
// finished attempt, put straight into the run book.
func coordHeldRun(t *testing.T, h *harness, number int) core.Run {
	t.Helper()
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	run := core.NewRun(core.RunArgs{
		Belt:    belt,
		Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number, Title: "Draft test"},
		Now:     sessionSampleT0,
	})
	spec := coordStageNamed(run.Stages, "Spec")
	// 	// run.stageId and run.state set the main leg as well, which the engine reads.
	run.StageID = spec.ID
	run.Legs[0].StageID = new(spec.ID)
	run.State = core.RunStateHeld
	run.Legs[0].State = core.RunStateHeld
	attempt := sessionSampleAttempt(spec.ID, 1, "01-spec-1")
	session := uuid(number)
	attempt.SessionID = &session
	ended := jsonx.Date{Time: sessionSampleT0}
	attempt.EndedAt = &ended
	run.Attempts = []core.Attempt{attempt}
	h.putRun(run)
	return run
}

// uuid is a stable session identifier for a test run.
func uuid(number int) string {
	return "00000000-0000-4000-8000-0000000000" + string(rune('0'+number%10))
}

// coordStageNamed is the stage with the given name, or the zero stage.
func coordStageNamed(stages []core.Stage, name string) core.Stage {
	for _, stage := range stages {
		if stage.Name == name {
			return stage
		}
	}
	return core.Stage{}
}

// lastAttemptID is the id of a run's last attempt, or "".
func lastAttemptID(run core.Run) string {
	if len(run.Attempts) == 0 {
		return ""
	}
	return run.Attempts[len(run.Attempts)-1].ID
}

// TestSendingAnAnswerQueuesTheRunWithItsNote: sending an answer to a held run queues it with the answer as its note.
func TestSendingAnAnswerQueuesTheRunWithItsNote(t *testing.T) {
	t.Parallel()
	h, c := draftsCoordinator(t)
	run := coordHeldRun(t, h, 1)
	holdKicks(t, c)
	c.Answer(run.ID, new("Use CSV"))
	got := c.Run(run.ID)
	if got == nil || got.State != core.RunStateQueued {
		t.Fatalf("answered run = %+v, want queued", got)
	}
	if got.Note == nil || *got.Note != "Use CSV" {
		t.Errorf("note = %v, want Use CSV", got.Note)
	}
	if got.Continues == nil || *got.Continues != lastAttemptID(run) {
		t.Errorf("continues = %v, want the last attempt", got.Continues)
	}
}

// TestAnsweringInTheSessionQueuesTheRunWithoutANote: answering with nothing sent queues the run with no note.
func TestAnsweringInTheSessionQueuesTheRunWithoutANote(t *testing.T) {
	t.Parallel()
	h, c := draftsCoordinator(t)
	run := coordHeldRun(t, h, 1)
	holdKicks(t, c)
	c.Answer(run.ID, nil)
	got := c.Run(run.ID)
	if got == nil || got.State != core.RunStateQueued {
		t.Fatalf("answered run = %+v, want queued", got)
	}
	if got.Note != nil {
		t.Errorf("note = %q, want none", *got.Note)
	}
	if got.Continues == nil || *got.Continues != lastAttemptID(run) {
		t.Errorf("continues = %v, want the last attempt", got.Continues)
	}
}

// TestAbortingARunKeepsTheAttemptNote: aborting a run holds it and keeps the attempt's saved note.
func TestAbortingARunKeepsTheAttemptNote(t *testing.T) {
	t.Parallel()
	h, c := draftsCoordinator(t)
	run := coordHeldRun(t, h, 1)
	h.Turn(func() {
		c.book.update(run.ID, nil, nil, func(r *core.Run) {
			r.State = core.RunStateRunning
			r.Attempts[0].EndedAt = nil
			r.Attempts[0].Note = new("Saved message")
		})
	})
	holdKicks(t, c)
	c.Abort(run.ID, nil)
	got := c.Run(run.ID)
	if got == nil || got.State != core.RunStateHeld {
		t.Errorf("aborted run = %+v, want held", got)
	}
	if got != nil && (got.Attempts[0].Note == nil || *got.Attempts[0].Note != "Saved message") {
		t.Errorf("attempt note = %v", got.Attempts[0].Note)
	}
	saved := findRunOnDisk(h, run.ID)
	if len(saved.Attempts) == 0 || saved.Attempts[0].Note == nil || *saved.Attempts[0].Note != "Saved message" {
		t.Errorf("the saved attempt note is not kept")
	}
}

// TestDroppingARunLeavesTheOtherRuns: dropping a run drops only that run.
func TestDroppingARunLeavesTheOtherRuns(t *testing.T) {
	t.Parallel()
	h, c := draftsCoordinator(t)
	run := coordHeldRun(t, h, 1)
	other := coordHeldRun(t, h, 2)
	holdKicks(t, c)
	c.Drop(run.ID, nil)
	if got := c.Run(run.ID); got == nil || got.State != core.RunStateDropped {
		t.Errorf("dropped run = %+v, want dropped", got)
	}
	if got := c.Run(other.ID); got == nil || got.State != core.RunStateHeld {
		t.Errorf("other run = %+v, want held", got)
	}
}

// TestDecidingWithANoteSendsIt: a gate decision sends its note with the decision.
func TestDecidingWithANoteSendsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Until("for _, run := range h.nowRuns() { ...", func() bool {
		return slices.ContainsFunc(h.nowRuns(), func(run core.Run) bool {
			return run.State == core.RunStateWaiting
		})
	})
	run := c.Runs()[0]
	stage := run.Stage()
	if stage == nil || stage.ID != "approve" {
		t.Fatalf("the waiting stage = %+v, want approve", stage)
	}
	// Keep the decision's kicked tick from starting the next stage before the assertions.
	holdKicks(t, c)
	c.Decide(run.ID, "sent back", new("Split the writer out first."))
	got := c.Run(run.ID)
	if got == nil || got.StageID != "spec" {
		t.Fatalf("the decided run = %+v, want stage spec", got)
	}
	if got.Note == nil || *got.Note != "Split the writer out first." {
		t.Errorf("note = %v", got.Note)
	}
	for _, attempt := range got.Attempts {
		if attempt.StageID == stage.ID && (attempt.Detail == nil || *attempt.Detail != "Split the writer out first.") {
			t.Errorf("the decision's attempt detail = %v", attempt.Detail)
		}
	}
}
