// Coordinator tests of belt archiving: it keeps a belt's versions, runs and pause, stops its
// intake and queued sessions, leaves an open session running, skips a schedule's elapsed times,
// and round-trips through a factory export.

package coordinator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// coordJSON is a value as JSON, so two values compare as the disk would hold them.
func coordJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := jsonx.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(data)
}

// manualBelt is a manual belt on two repositories with the manual kind's stages.
func manualBelt(id string) core.Belt {
	agents := core.SampleAgents()
	entry := core.TriggerEntry{ID: "manual", Trigger: core.TriggerKindManual.Fresh(), Repos: []string{"acme/web", "acme/billing"}}
	belt := core.DefaultBelt(id, id, []core.TriggerEntry{entry}, core.SampleStages(core.TriggerKindManual, agents, true))
	belt.Forks = core.SampleForks(core.TriggerKindManual)
	return belt
}

// runWithoutLock returns the run with id without taking the lock, for Until predicates (which run with it held).
func runWithoutLock(h *harness, id string) *core.Run {
	for _, run := range h.nowRuns() {
		if run.ID == id {
			return &run
		}
	}
	return nil
}

// TestArchivePreservesVersionsRunsAndPause: archiving a belt keeps its version, its runs and its pause, and
// blocks new work until it is restored.
func TestArchivePreservesVersionsRunsAndPause(t *testing.T) {
	t.Parallel()
	for _, paused := range []bool{false, true} {
		t.Run(map[bool]string{false: "running", true: "paused"}[paused], func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
			c := h.Coord()
			// 			// The test is synchronous, so the tick a start kicks never runs before its
			// assertions. These steps run in one turn, which keeps the spawned tick waiting too.
			var run core.Run
			var setupErr error
			h.Turn(func() {
				run, setupErr = c.intake.startTask("tasks", "acme/web", "Task", "", nil)
				if setupErr == nil {
					setupErr = c.library.SetPaused("tasks", paused)
				}
			})
			if setupErr != nil {
				t.Fatalf("setup: %v", setupErr)
			}
			diskRuns := coordJSON(t, h.storage.LoadRuns())
			before := beltRecordOf(t, c, "tasks")
			file := filepath.Join(h.root, "belts", "tasks", "v1.json")
			bytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read belt file: %v", err)
			}

			c.SetBeltArchived("tasks", true)
			if got := coordJSON(t, c.Runs()); got != coordJSON(t, []core.Run{run}) {
				t.Errorf("archiving changed the runs")
			}
			archived := beltRecordOf(t, c, "tasks")
			if coordJSON(t, archived.Belt) != coordJSON(t, before.Belt) {
				t.Errorf("archiving changed the belt")
			}
			if !archived.EffectivelyPaused() {
				t.Error("an archived belt is not effectively paused")
			}
			if got, want := core.StatusLocation(run, archived.EffectivelyPaused(), core.Pause{}), core.StatusLocation(run, true, core.Pause{}); got != want {
				t.Errorf("location = %+v, want %+v", got, want)
			}
			if records := h.storage.LoadBelts(); len(records) == 0 || !records[0].Archived {
				t.Error("the stored belt is not archived")
			}
			if got := coordJSON(t, h.storage.LoadRuns()); got != diskRuns {
				t.Errorf("archiving rewrote the runs")
			}
			if after, err := os.ReadFile(file); err != nil || string(after) != string(bytes) {
				t.Errorf("archiving rewrote the belt's version: %v", err)
			}
			if _, err := c.StartTask("tasks", "acme/web", "No", "", nil); err == nil {
				t.Error("an archived belt started a task")
			}

			edited := before.Belt.Clone()
			edited.Name = "Edited while archived"
			saved, err := c.SaveBelt(edited)
			if err != nil {
				t.Fatalf("save while archived: %v", err)
			}
			if saved.Version != before.Belt.Version+1 {
				t.Errorf("saved version = %d, want %d", saved.Version, before.Belt.Version+1)
			}
			if !beltRecordOf(t, c, "tasks").Archived {
				t.Error("saving un-archived the belt")
			}
			if live := beltsArchived(c.Belts(), false); live != 0 {
				t.Errorf("live belts while archived = %d", live)
			}

			copied, err := c.SaveBelt(c.CopyBeltDraft(saved))
			if err != nil {
				t.Fatalf("copy: %v", err)
			}
			if beltRecordOf(t, c, copied.ID).Archived || beltRecordOf(t, c, copied.ID).Paused {
				t.Error("a copy starts archived or paused")
			}
			if copied.ID == "tasks" {
				t.Error("the copy reused the belt's id")
			}
			c.SetBeltArchived(copied.ID, true)
			if n := len(c.Belts()); n != 2 {
				t.Errorf("belts = %d, want 2", n)
			}
			if c.NewBeltDraft().ID == "tasks" {
				t.Error("a new belt reused the archived id")
			}

			c.SetBeltArchived("tasks", false)
			if got := beltRecordOf(t, c, "tasks").Paused; got != paused {
				t.Errorf("paused after restore = %v, want %v", got, paused)
			}
			if coordJSON(t, beltRecordOf(t, c, "tasks").Belt) != coordJSON(t, saved) {
				t.Error("restoring changed the saved version")
			}
			if n := beltsArchived(c.Belts(), true); n != 1 {
				t.Errorf("archived belts = %d, want 1", n)
			}
			c.SetBeltArchived(copied.ID, false)
			if n := beltsArchived(c.Belts(), true); n != 0 {
				t.Errorf("archived belts after restoring the copy = %d", n)
			}
		})
	}
}

// TestArchivingDoesNotStopAnOpenSession: archiving leaves a session that is already open running.
func TestArchivingDoesNotStopAnOpenSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}, StageDuration: 3600})
	c := h.Coord()
	run, err := c.StartTask("tasks", "acme/web", "Open", "", nil)
	if err != nil {
		t.Fatalf("StartTask: %v", err)
	}
	h.Until("got := runWithoutLock(h, run.ID) ...", func() bool {
		got := runWithoutLock(h, run.ID)
		return got != nil && len(got.Attempts) > 0 && got.Attempts[len(got.Attempts)-1].IsOpen()
	})
	open := c.Run(run.ID)
	if open == nil || len(open.Attempts) == 0 || !open.Attempts[len(open.Attempts)-1].IsOpen() {
		t.Fatal("no open attempt")
	}
	attempt := open.Attempts[len(open.Attempts)-1]

	c.SetBeltArchived("tasks", true)
	h.Tick()
	after := c.Run(run.ID)
	if after == nil || len(after.Attempts) == 0 {
		t.Fatal("the run lost its attempts")
	}
	if coordJSON(t, after.Attempts[len(after.Attempts)-1]) != coordJSON(t, attempt) {
		t.Error("archiving changed the open attempt")
	}
	if after.State != core.RunStateRunning {
		t.Errorf("state = %q, want running", after.State)
	}
}

// beltsArchived counts the records whose archived flag is archived.
func beltsArchived(records []store.BeltRecord, archived bool) int {
	count := 0
	for _, record := range records {
		if record.Archived == archived {
			count++
		}
	}
	return count
}
