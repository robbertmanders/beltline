// Tests of the run book and the coordinator's reads and effect paths it serves. These use the book and
// the reads directly, so they run without a tick.

package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// issueRun is a run on the default issues belt (the harness's belt) for one issue. It does not
// read the coordinator, so it can be called inside a turn.
func issueRun(number int, created time.Time) core.Run {
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	run := core.NewRun(core.RunArgs{
		Belt:    belt,
		Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number, Title: "Run"},
		Now:     created,
	})
	run.CreatedAt = jsonx.Date{Time: created}
	run.UpdatedAt = jsonx.Date{Time: created}
	// A saved run has its model admission; New admits the ones that lack one.
	run.ModelAdmission = &core.ModelAdmission{}
	return run
}

func TestRunsLoadInCreationOrderThenByID(t *testing.T) {
	t.Parallel()
	early := testkit.ContractTime.Add(-2 * time.Hour)
	late := testkit.ContractTime.Add(-time.Hour)
	var first, second, older core.Run
	h := newHarness(t, harnessOptions{Prepare: func(storage store.Storage) {
		first = issueRun(1, late)
		second = issueRun(2, late)
		older = issueRun(3, early)
		for _, run := range []core.Run{first, second, older} {
			if err := storage.SaveRun(run); err != nil {
				t.Fatalf("save run: %v", err)
			}
		}
	}})
	// The coordinator built by newHarness loaded the runs; a restart loads them again.
	c := h.Restart()
	runs := c.Runs()
	if len(runs) != 3 {
		t.Fatalf("runs = %d, want 3", len(runs))
	}
	if runs[0].ID != older.ID {
		t.Fatalf("first run = %s, want the oldest %s", runs[0].ID, older.ID)
	}
	want := []string{first.ID, second.ID}
	if want[0] > want[1] {
		want[0], want[1] = want[1], want[0]
	}
	if runs[1].ID != want[0] || runs[2].ID != want[1] {
		t.Fatalf("order = %s %s, want %s %s", runs[1].ID, runs[2].ID, want[0], want[1])
	}
}

func TestNoticesAreNewestFirstAndKeepTheLastHundred(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Turn(func() {
		for i := range 101 {
			h.coord.book.note(fmt.Sprintf("note %d", i), nil)
		}
	})
	notices := h.Coord().Notices()
	if len(notices) != 100 {
		t.Fatalf("notices = %d, want 100", len(notices))
	}
	if notices[0].Text != "note 100" || notices[99].Text != "note 1" {
		t.Fatalf("notices run from %q to %q, want \"note 100\" to \"note 1\"", notices[0].Text, notices[99].Text)
	}
	if notices[0].ID == notices[1].ID {
		t.Fatal("notices share an id")
	}
}

func TestAddSkipsARunThatIsBeingErased(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	run := issueRun(7, testkit.ContractTime)
	h.Turn(func() {
		h.coord.book.markErasing(run.ID)
		h.coord.book.add(run)
		if len(h.coord.book.runs) != 0 {
			t.Fatalf("erasing run was added: %d runs", len(h.coord.book.runs))
		}
		h.coord.book.clearErasing(run.ID)
		h.coord.book.add(run)
	})
	if got := h.Coord().Runs(); len(got) != 1 || got[0].ID != run.ID {
		t.Fatalf("runs = %v, want the run once it stops erasing", got)
	}
}

func TestApplyRefusesAnotherGeneration(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	var run core.Run
	h.Turn(func() {
		run = h.coord.book.add(issueRun(8, testkit.ContractTime))
	})
	other := run.Generation + 1
	h.Turn(func() {
		if h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindAbort}, &other, nil, nil) {
			t.Fatal("apply accepted another generation")
		}
	})
	if got := h.Coord().Run(run.ID); got == nil || got.Generation != run.Generation {
		t.Fatalf("run changed: %+v", got)
	}
}

func TestUpdateWithoutAChangeReportsSuccessAndLeavesTheRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	var run core.Run
	h.Turn(func() { run = h.coord.book.add(issueRun(9, testkit.ContractTime)) })
	h.ClearChanges()
	h.Turn(func() {
		if !h.coord.book.update(run.ID, nil, nil, func(*core.Run) {}) {
			t.Fatal("an unchanged run reported a failed save")
		}
	})
	if got := h.Changes(); len(got) != 0 {
		t.Fatalf("an unchanged run reported changes: %v", got)
	}
}

func TestASaveFailureNotesAndKeepsTheRunAsItWas(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	var run core.Run
	h.Turn(func() { run = h.coord.book.add(issueRun(10, testkit.ContractTime)) })
	h.storage.WriteFault.Arm("run.json")
	h.Turn(func() {
		if h.coord.book.update(run.ID, nil, nil, func(r *core.Run) { r.Subject.Title = "changed" }) {
			t.Fatal("update reported success after a failed save")
		}
	})
	if got := h.Coord().Run(run.ID); got == nil || got.Subject.Title != "Run" {
		t.Fatalf("run changed after a failed save: %+v", got)
	}
	notices := h.Coord().Notices()
	if len(notices) == 0 || notices[0].RunID != nil {
		t.Fatalf("the failed save was not noted for the coordinator: %+v", notices)
	}
	// A save failure is a notice on the coordinator, not a notification.
	if texts := h.NoticeTexts(); len(texts) == 0 || !strings.Contains(texts[0], "Could not save") {
		t.Fatalf("notice texts = %q, want the save failure", texts)
	}
	if got := h.Notifications(); len(got) != 0 {
		t.Fatalf("a save failure was notified: %v", got)
	}
}

func TestRunsReturnsCopies(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Turn(func() { h.coord.book.add(issueRun(11, testkit.ContractTime)) })
	runs := h.Coord().Runs()
	runs[0].Subject.Title = "mutated by the caller"
	runs[0].Stages = nil
	if got := h.Coord().Runs(); got[0].Subject.Title == "mutated by the caller" || len(got[0].Stages) == 0 {
		t.Fatal("a caller changed the book through its copy")
	}
}

func TestANotifyEffectReachesTheNotifierOnlyWhenNotificationsAreOn(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	run := issueRun(12, testkit.ContractTime)
	effect := []core.Effect{{Kind: core.EffectKindNotify, Title: "Shipped", Body: "#12 shipped"}}
	h.Turn(func() {
		h.coord.changeSettings(func(s *store.Settings) { s.Notifications = false })
		h.coord.perform(effect, run, nil)
	})
	if got := h.Notifications(); len(got) != 0 {
		t.Fatalf("notified with notifications off: %v", got)
	}
	h.Turn(func() {
		h.coord.changeSettings(func(s *store.Settings) { s.Notifications = true })
		h.coord.perform(effect, run, nil)
	})
	if got := h.Notifications(); len(got) != 1 || got[0] != "Shipped: #12 shipped" {
		t.Fatalf("notifications = %v", got)
	}
}

// agentAttempt is a run with one attempt at its first agent stage, and that attempt.
func agentAttempt(t *testing.T) (core.Run, core.Attempt) {
	t.Helper()
	run := issueRun(13, testkit.ContractTime)
	stage := run.Stages[0]
	if stage.Kind() != core.StageKindAgent {
		t.Fatalf("first stage is %s, want an agent", stage.Kind())
	}
	attempt := core.Attempt{ID: "attempt-1", StageID: stage.ID}
	run.Attempts = []core.Attempt{attempt}
	return run, attempt
}

func TestOutputReadsTheStagesFileOfAnAttempt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	run, attempt := agentAttempt(t)
	h.putRun(run)
	if got := h.Coord().Output(run, attempt); got != nil {
		t.Fatalf("output without a file = %q, want nil", *got)
	}
	folder := h.coord.paths.AttemptFolder(run, attempt)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, run.Stages[0].Writes()), []byte("Outcome: complete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := h.Coord().Output(run, attempt)
	if got == nil || *got != "Outcome: complete\n" {
		t.Fatalf("output = %v, want the file's text", got)
	}
}

func TestArtifactExistsOnlyForAFileInTheAttemptsFolder(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	run, attempt := agentAttempt(t)
	h.putRun(run)
	folder := h.coord.paths.ArtifactsFolder(run, attempt)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "report.html"), []byte("<p>ok</p>"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := h.Coord()
	ctx := context.Background()
	if !c.ArtifactExists(ctx, run.ID, attempt.ID, "report.html") {
		t.Fatal("a file in the attempt's folder is not found")
	}
	if c.ArtifactExists(ctx, run.ID, attempt.ID, "missing.html") {
		t.Fatal("a missing file is reported as there")
	}
	if c.ArtifactExists(ctx, run.ID, "no-such-attempt", "report.html") {
		t.Fatal("an unknown attempt is reported as having the file")
	}
}
