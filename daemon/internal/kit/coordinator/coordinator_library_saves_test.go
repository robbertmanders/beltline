// Tests of copying and saving belts through Library.SaveBelt, which is what the coordinator's SaveBelt
// does before it kicks a tick: a copy keeps the definition with no firing history, a warned save pauses
// the belt and a resume waits for warnings to clear, and a failed write saves nothing.

package coordinator

import (
	"bytes"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// savedLibraryBelt saves belt through the library and returns the saved version.
func savedLibraryBelt(t *testing.T, h *harness, belt core.Belt) core.Belt {
	t.Helper()
	var saved core.Belt
	var err error
	h.Turn(func() { saved, err = h.coord.library.SaveBelt(belt) })
	if err != nil {
		t.Fatalf("save %s: %v", belt.ID, err)
	}
	return saved
}

// sameBelt reports whether two belts are the same definition: belts compare
// structurally, and a reloaded belt can differ from its in-memory copy only in nil
// and empty lists, so the wire form is compared.
func sameBelt(a, b core.Belt) bool {
	x, errA := jsonx.Marshal(a)
	y, errB := jsonx.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// beltsOf is the belt definitions of the saved records, in order.
func beltsOf(records []store.BeltRecord) []core.Belt {
	out := make([]core.Belt, 0, len(records))
	for _, record := range records {
		out = append(out, record.Belt)
	}
	return out
}

// matchesSaved reports whether saved is draft, saved as its next version.
func matchesSaved(saved, draft core.Belt) bool {
	expected := draft.Clone()
	expected.Version = saved.Version
	return sameBelt(saved, expected)
}

// libraryRecord is the saved record of belt id, failing the test when there is none.
func libraryRecord(t *testing.T, c *Coordinator, id string) *beltRecordView {
	t.Helper()
	record := c.Belt(id)
	if record == nil {
		t.Fatalf("no belt %q", id)
	}
	return &beltRecordView{Paused: record.Paused, Archived: record.Archived, Created: record.Created, Watch: record.WatchOutcomes, Review: record.ReviewOutcomes, Belt: record.Belt, LastFired: record.LastFired()}
}

// beltRecordView is the part of a saved record the tests compare.
type beltRecordView struct {
	Paused, Archived bool
	Created          time.Time
	Watch, Review    bool
	Belt             core.Belt
	LastFired        *time.Time
}

// defaultLibraryBelt is the default issues belt on a/b, the source most save tests start from.
func defaultLibraryBelt() core.Belt {
	return core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
}

// TestCopiesKeepTheDefinitionAndSaveAsIndependentLiveBelts covers copying a saved belt's draft: the copy
// keeps the definition, its AFK opt-in and its current draft stages, skips taken ids, nothing is saved by
// copying, and saving a copy makes an independent live version 1 that is not paused.
func TestCopiesKeepTheDefinitionAndSaveAsIndependentLiveBelts(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	if c.settings.Afk.Enabled {
		t.Fatalf("the master switch starts on")
	}
	definition := forkedTestBelt()
	definition.Base = "develop"
	definition.Policies.CommentOnHold = false
	definition.Stages[0].Agent.Version = 2
	definition.Afk = core.AfkModeGlobal
	source := savedLibraryBelt(t, h, definition)
	records := c.Belts()

	copied := c.CopyBeltDraft(source)
	if copied.Name != "Copy of "+source.Name || copied.ID == source.ID || copied.Version != 0 || copied.Afk != core.AfkModeGlobal {
		t.Errorf("copy = %q, id %q, version %d, afk %v", copied.Name, copied.ID, copied.Version, copied.Afk)
	}
	if !sameBelt(core.Belt{Stages: copied.Stages, Forks: copied.Forks, Triggers: copied.Triggers, Base: copied.Base, Policies: copied.Policies},
		core.Belt{Stages: source.Stages, Forks: source.Forks, Triggers: source.Triggers, Base: source.Base, Policies: source.Policies}) {
		t.Errorf("the copy's definition differs from the source")
	}
	if c.settings.Afk.Enabled || c.Belt(copied.ID) != nil || !slices.EqualFunc(beltsOf(c.Belts()), beltsOf(records), sameBelt) {
		t.Errorf("copying saved something or switched AFK on")
	}

	// A copy takes the current draft's stages, skips taken ids and copies a copy by its own name.
	edited := source.Clone()
	edited.Name = "Edited name"
	edited.Stages[0].Name = "X"
	edits := c.CopyBeltDraft(edited)
	if edits.Name != "Copy of Edited name" || !sameBelt(core.Belt{Stages: edits.Stages}, core.Belt{Stages: edited.Stages}) {
		t.Errorf("copy of the draft = %q, stages differ = %v", edits.Name, !sameBelt(core.Belt{Stages: edits.Stages}, core.Belt{Stages: edited.Stages}))
	}
	for _, record := range c.Belts() {
		if record.Belt.ID == edits.ID {
			t.Errorf("the copy's id %q is taken", edits.ID)
		}
	}
	again := c.CopyBeltDraft(edits)
	if again.Name != "Copy of Copy of Edited name" || again.ID == edits.ID {
		t.Errorf("second copy = %q, id %q", again.Name, again.ID)
	}

	// Saving a copy makes a live version 1 that does not inherit the source's pause.
	h.Turn(func() {
		if err := c.library.SetPaused(source.ID, true); err != nil {
			t.Fatalf("pause: %v", err)
		}
	})
	count := len(c.Belts())
	copied.Name = "Independent"
	saved := savedLibraryBelt(t, h, copied)
	if saved.Version != 1 || len(c.Belts()) != count+1 || libraryRecord(t, c, saved.ID).Paused {
		t.Errorf("saved copy = version %d, belts %d, paused %v", saved.Version, len(c.Belts()), libraryRecord(t, c, saved.ID).Paused)
	}
	if !libraryRecord(t, c, source.ID).Paused || !sameBelt(libraryRecord(t, c, source.ID).Belt, source) {
		t.Errorf("the source lost its pause or changed")
	}
	reopened := coordinatorOn(t, h.storage)
	if !sameBelt(reopened.Belt(saved.ID).Belt, saved) || !sameBelt(reopened.Belt(source.ID).Belt, source) {
		t.Errorf("a restart lost the saved copy or the source")
	}
	h.Turn(func() {
		if err := c.library.SetPaused(saved.ID, true); err != nil {
			t.Fatalf("pause copy: %v", err)
		}
	})
	copied.Name = "Independent v2"
	if got := savedLibraryBelt(t, h, copied).Version; got != 2 || !libraryRecord(t, c, saved.ID).Paused {
		t.Errorf("second save version = %d, paused kept = %v", got, libraryRecord(t, c, saved.ID).Paused)
	}
}

// TestCopiesOfScheduledBeltsHaveNoFiringHistory checks that a copy of a scheduled belt keeps the trigger
// and has no last firing, while the source keeps its own.
func TestCopiesOfScheduledBeltsHaveNoFiringHistory(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	definition := defaultLibraryBelt()
	definition.Triggers[0] = scheduledEntry(9)
	source := savedLibraryBelt(t, h, definition)
	past := time.Unix(1700000000, 0)
	h.Turn(func() {
		if err := c.library.setLastFired(source.ID, past); err != nil {
			t.Fatalf("set last fired: %v", err)
		}
	})
	saved := savedLibraryBelt(t, h, c.CopyBeltDraft(source))
	if !reflect.DeepEqual(saved.Triggers, source.Triggers) || libraryRecord(t, c, saved.ID).LastFired != nil {
		t.Errorf("the copy's trigger or firing history is wrong")
	}
	if got := libraryRecord(t, c, source.ID).LastFired; got == nil || !got.Equal(past) {
		t.Errorf("the source's firing = %v, want %v", got, past)
	}
}

// warnedEdit is one save that leaves a belt with warnings: the saved belt is paused and the warnings are
// the library's own.
type warnedEdit struct {
	name   string
	change func(belt *core.Belt)
	// warning must appear among the warnings of the saved belt.
	warning string
}

// TestWarnedSavesPauseAndKeepTheirDrafts covers every save that leaves a belt with warnings: the saved
// draft is kept as its next version, the belt is paused, a copy without repositories is warned, and the
// warnings after a reload match the save's.
func TestWarnedSavesPauseAndKeepTheirDrafts(t *testing.T) {
	t.Parallel()
	cases := []warnedEdit{
		{name: "blank name", change: func(b *core.Belt) { b.Name = "" }},
		{name: "copy without repositories", change: func(b *core.Belt) { b.Triggers[0].Repos = nil }, warning: "Add at least one repository to the trigger."},
		{name: "no good outcome", change: func(b *core.Belt) {
			for i := range b.Stages[0].Outcomes {
				b.Stages[0].Outcomes[i].Tone = core.ToneNeutral
			}
		}, warning: "Triage has no good outcome. The run page uses it to know when the stage went well."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newLibraryHarness(t)
			c := h.Coord()
			source := savedLibraryBelt(t, h, defaultLibraryBelt())
			draft := c.CopyBeltDraft(source)
			tc.change(&draft)
			if tc.warning != "" && !slices.Contains(c.Warnings(draft), tc.warning) {
				t.Fatalf("warnings = %q, want %q", c.Warnings(draft), tc.warning)
			}
			saved := savedLibraryBelt(t, h, draft)
			if !matchesSaved(saved, draft) || !libraryRecord(t, c, saved.ID).Paused {
				t.Errorf("the saved draft differs or the warned save did not pause")
			}
			if !reflect.DeepEqual(c.Warnings(saved), c.Warnings(draft)) || len(c.Warnings(saved)) == 0 {
				t.Errorf("warnings after save = %q", c.Warnings(saved))
			}
			if libraryRecord(t, c, source.ID).Paused || !sameBelt(libraryRecord(t, c, source.ID).Belt, source) {
				t.Errorf("the source changed")
			}
		})
	}
}

// TestAnEmptyBeltIsWarnedAndSavedPaused checks the structural warnings of a new empty belt and that the
// save of it is paused, with its warnings unchanged after the save.
func TestAnEmptyBeltIsWarnedAndSavedPaused(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	var empty core.Belt
	h.Turn(func() { empty = c.library.NewBelt() })
	structural := c.Warnings(empty)
	if !slices.Contains(structural, "Add at least one stage.") || !slices.Contains(structural, "Add at least one repository to the trigger.") {
		t.Fatalf("warnings for an empty belt = %q", structural)
	}
	saved := savedLibraryBelt(t, h, empty)
	if !matchesSaved(saved, empty) || !reflect.DeepEqual(c.Warnings(saved), structural) || !libraryRecord(t, c, saved.ID).Paused {
		t.Errorf("the empty belt's save differs, its warnings changed, or it is not paused")
	}
}

// TestAValidSaveKeepsThePauseYouChose checks that a clean save keeps a pause you set, and that a clean
// new belt starts unpaused.
func TestAValidSaveKeepsThePauseYouChose(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	active := savedLibraryBelt(t, h, defaultLibraryBelt())
	renamed := active.Clone()
	renamed.Name = "Still running"
	savedLibraryBelt(t, h, renamed)
	if libraryRecord(t, c, active.ID).Paused {
		t.Fatalf("a valid save paused the belt")
	}
	h.Turn(func() {
		if err := c.library.SetPaused(active.ID, true); err != nil {
			t.Fatalf("pause: %v", err)
		}
	})
	renamed.Name = "Still paused"
	savedLibraryBelt(t, h, renamed)
	if !libraryRecord(t, c, active.ID).Paused {
		t.Errorf("a valid save resumed a paused belt")
	}
	ready := defaultLibraryBelt()
	h.Turn(func() { ready.ID = c.library.NewBelt().ID })
	ready.Version = 0
	ready.Name = "Ready"
	started := savedLibraryBelt(t, h, ready)
	if got := c.Warnings(started); len(got) != 0 || libraryRecord(t, c, started.ID).Paused {
		t.Errorf("the clean new belt has warnings %q or starts paused", got)
	}
}

// TestResumeWaitsForWarningsToClear checks that a warned belt cannot be resumed, that a refused resume
// changes nothing, that a clean save does not resume by itself, and that a restart keeps the pause until
// you resume a clean belt.
func TestResumeWaitsForWarningsToClear(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	belt := defaultLibraryBelt()
	belt.Name = ""
	saved := savedLibraryBelt(t, h, belt)
	if !libraryRecord(t, c, saved.ID).Paused {
		t.Fatalf("a warned save did not pause the belt")
	}
	draft := saved.Clone()
	draft.Name = "Named"
	var resumeErr error
	h.Turn(func() { resumeErr = c.library.SetPaused(saved.ID, false) })
	if failure, ok := resumeErr.(core.Failure); !ok || failure.Message != LibraryResumeBlocked {
		t.Errorf("resume error = %v, want the blocked message", resumeErr)
	}
	if !libraryRecord(t, c, saved.ID).Paused {
		t.Errorf("a refused resume changed the pause")
	}
	corrected := savedLibraryBelt(t, h, draft)
	if len(c.Warnings(corrected)) != 0 || !libraryRecord(t, c, saved.ID).Paused {
		t.Errorf("a clean save resumed the belt by itself")
	}
	reopened := coordinatorOn(t, h.storage)
	if !reopened.Belt(saved.ID).Paused {
		t.Errorf("a restart resumed the belt")
	}
	func() {
		reopened.Lock()
		defer reopened.EndTurn()
		if err := reopened.library.SetPaused(saved.ID, false); err != nil {
			t.Errorf("resume after the fix: %v", err)
		}
	}()
	if reopened.Belt(saved.ID).Paused || coordinatorOn(t, h.storage).Belt(saved.ID).Paused {
		t.Errorf("resuming a clean belt did not resume it, or a restart re-paused it")
	}
}

// TestWarnedSavesKeepEveryOtherBeltsState checks that a warned save leaves a paused belt paused with its
// creation time, keeps an archived belt archived, clears a rescheduled belt's last firing, and that all
// of it survives a restart.
func TestWarnedSavesKeepEveryOtherBeltsState(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	active := savedLibraryBelt(t, h, defaultLibraryBelt())
	running := active.Clone()
	running.Name = ""
	savedRunning := savedLibraryBelt(t, h, running)
	if rec := libraryRecord(t, c, active.ID); !rec.Paused || rec.Archived || !rec.Watch || !rec.Review {
		t.Errorf("active record after a warned save = %+v", rec)
	}

	pausedBelt := defaultLibraryBelt()
	pausedBelt.ID = "paused-belt"
	pausedBelt.Name = "Already paused"
	paused := savedLibraryBelt(t, h, pausedBelt)
	h.Turn(func() {
		if err := c.library.SetPaused(paused.ID, true); err != nil {
			t.Fatalf("pause: %v", err)
		}
	})
	created := libraryRecord(t, c, paused.ID).Created
	pausedEdit := paused.Clone()
	pausedEdit.Stages = nil
	savedPaused := savedLibraryBelt(t, h, pausedEdit)
	if !libraryRecord(t, c, paused.ID).Paused || !libraryRecord(t, c, paused.ID).Created.Equal(created) || !matchesSaved(savedPaused, pausedEdit) {
		t.Errorf("a save resumed the paused belt, changed its creation time or kept a different draft")
	}

	archivedBelt := defaultLibraryBelt()
	archivedBelt.ID = "archived-belt"
	archived := savedLibraryBelt(t, h, archivedBelt)
	h.Turn(func() { c.library.SetBeltArchived(archived.ID, true) })
	archivedEdit := archived.Clone()
	archivedEdit.Name = " "
	savedLibraryBelt(t, h, archivedEdit)
	if rec := libraryRecord(t, c, archived.ID); !rec.Archived || !rec.Paused {
		t.Errorf("archived record = %+v", rec)
	}

	schedule := defaultLibraryBelt()
	schedule.ID = "schedule"
	schedule.Triggers[0] = scheduledEntry(9)
	scheduled := savedLibraryBelt(t, h, schedule)
	fired := time.Unix(1700000000, 0)
	h.Turn(func() {
		if err := c.library.setLastFired(scheduled.ID, fired); err != nil {
			t.Fatalf("set last fired: %v", err)
		}
	})
	rescheduled := scheduled.Clone()
	rescheduled.Name = ""
	rescheduled.Triggers[0] = scheduledEntry(11)
	savedLibraryBelt(t, h, rescheduled)
	if libraryRecord(t, c, scheduled.ID).LastFired != nil || !libraryRecord(t, c, scheduled.ID).Paused {
		t.Errorf("a warned reschedule kept its last firing or did not pause")
	}

	again := coordinatorOn(t, h.storage)
	if !sameBelt(again.Belt(active.ID).Belt, savedRunning) || !again.Belt(active.ID).Paused {
		t.Errorf("a restart lost the warned running belt")
	}
	if !again.Belt(paused.ID).Paused || !sameBelt(again.Belt(paused.ID).Belt, savedPaused) {
		t.Errorf("a restart lost the paused belt's edit")
	}
	if !again.Belt(archived.ID).Archived || !again.Belt(archived.ID).Paused {
		t.Errorf("a restart lost the archived belt")
	}
	if got := again.Belt(scheduled.ID); got.LastFired() != nil || !got.Paused || got.Belt.Name != "" {
		t.Errorf("a restart changed the scheduled belt: %+v", got)
	}
}

// TestFailedWritesSaveNothingAndKeepThePause checks that a failed state, version or first-version write
// is reported and leaves no version, and that a failed resume write does not resume the belt, before and
// after a restart.
func TestFailedWritesSaveNothingAndKeepThePause(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	source := savedLibraryBelt(t, h, defaultLibraryBelt())
	warned := source.Clone()
	warned.Name = ""
	faults := h.storage.WriteFault
	save := func(belt core.Belt) error {
		var err error
		h.Turn(func() { _, err = c.library.SaveBelt(belt) })
		return err
	}
	next := source.Version + 1

	faults.Arm("state.json")
	if err := save(warned); err == nil {
		t.Fatalf("a failed state write was reported as saved")
	}
	if !sameBelt(libraryRecord(t, c, source.ID).Belt, source) || libraryRecord(t, c, source.ID).Paused || h.storage.Belt(source.ID, next) != nil {
		t.Errorf("a failed state write changed the belt or left version %d", next)
	}

	faults.Arm("v" + strconv.Itoa(next) + ".json")
	if err := save(warned); err == nil {
		t.Fatalf("a failed version write was reported as saved")
	}
	if !libraryRecord(t, c, source.ID).Paused && !coordinatorOn(t, h.storage).Belt(source.ID).Paused {
		t.Errorf("the warned save did not leave the belt paused")
	}
	if !sameBelt(libraryRecord(t, c, source.ID).Belt, source) || h.storage.Belt(source.ID, next) != nil {
		t.Errorf("a failed version write changed the saved belt or left version %d", next)
	}

	fresh := c.NewBeltDraft()
	fresh.Name = "Never written"
	faults.Arm("v1.json")
	if err := save(fresh); err == nil {
		t.Fatalf("a failed first version was reported as saved")
	}
	if c.Belt(fresh.ID) != nil || coordinatorOn(t, h.storage).Belt(fresh.ID) != nil {
		t.Errorf("a failed first version was kept")
	}

	h.Turn(func() {
		if err := c.library.SetPaused(source.ID, true); err != nil {
			t.Fatalf("pause: %v", err)
		}
	})
	faults.Arm("state.json")
	var resumeErr error
	h.Turn(func() { resumeErr = c.library.SetPaused(source.ID, false) })
	if resumeErr == nil {
		t.Fatalf("a failed resume write was reported as done")
	}
	if !libraryRecord(t, c, source.ID).Paused || !coordinatorOn(t, h.storage).Belt(source.ID).Paused {
		t.Errorf("a failed resume write resumed the belt")
	}
}

// forkedTestBelt is the forked belt: triage, Spec and Build
// on the Complex route, Build again on the Simple route as quick-build, then Verify
// and Ship. Triage's first outcome routes to the Simple route.
func forkedTestBelt() core.Belt {
	fork := core.Fork{ID: "size", Routes: []core.Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	var stages []core.Stage
	add := func(agentID string, route *string, id string) {
		agent := core.SampleAgent(agentID)
		place := core.DefaultsPlace{Before: append([]core.Stage{}, stages...), Forks: []core.Fork{fork}, Route: route}
		stage := core.DefaultsStageForAgentAt(agent, place, stages, core.TriggerKindIssues)
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	complex, simple := "complex", "simple"
	add("triage", nil, "")
	add("spec", &complex, "")
	add("build", &complex, "")
	add("build", &simple, "quick-build")
	add("verify", nil, "")
	add("ship", nil, "")
	stages[0].Outcomes[0].Then = core.Then{Action: core.ThenActionRoute, Routes: []string{"simple"}}
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "sized", Name: "Sized", Trigger: core.TriggerKindIssues.Fresh(), Repos: []string{"acme/billing"},
		Stages: stages, Forks: []core.Fork{fork},
	})
}

// scheduledEntry is a schedule trigger entry that fires on weekdays at hour.
func scheduledEntry(hour int) core.TriggerEntry {
	schedule := core.DefaultSchedule()
	schedule.Hour = hour
	task := core.DefaultScheduledTask()
	task.Schedule = schedule
	task.Title = "Sweep"
	task.Task = "Fix tests"
	trigger := core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &task}
	return core.TriggerEntry{ID: core.TriggerEntryLegacyID, Trigger: trigger, Repos: []string{"a/b"}}
}

// setLastFired saves when a belt's first schedule last fired.
// Lock: runs with c.mu held.
func (l *Library) setLastFired(id string, date time.Time) error {
	index := l.beltIndex(id)
	if index < 0 {
		return nil
	}
	entry := core.TriggerEntryLegacyID
	for _, existing := range l.belts[index].Belt.Triggers {
		if existing.Trigger.Kind == core.TriggerKindSchedule {
			entry = existing.ID
			break
		}
	}
	return l.setFired(id, entry, date)
}
