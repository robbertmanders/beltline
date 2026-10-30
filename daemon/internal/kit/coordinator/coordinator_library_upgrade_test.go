// Tests of the library's watch-outcome upgrade when it loads, and of what a save or an import keeps
// of it. The library is built with newLibrary, which loads it with a coordinator that only supplies
// the clock.

package coordinator

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// upgradeTime is the time the library tests give their runs and clocks.
var upgradeTime = testkit.ContractTime

// libraryOver loads a library over storage the way Library(storage:) does.
func libraryOver(t *testing.T, storage store.Storage) *Library {
	t.Helper()
	c := &Coordinator{clock: testkit.NewClock(upgradeTime)}
	library := newLibrary(c, storage)
	return &library
}

// seededNoBelts is the built-in agents and the model fixtures, no belts.
func seededNoBelts(t *testing.T) store.Storage {
	t.Helper()
	storage := unseededStorage(t)
	storage.SeedAgents()
	if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	return storage
}

// watchIndex is the index of the first watch stage of belt.
func watchIndex(t *testing.T, belt core.Belt) int {
	t.Helper()
	for i, stage := range belt.Stages {
		if stage.Kind() == core.StageKindWatch {
			return i
		}
	}
	t.Fatalf("no watch stage")
	return -1
}

func TestLibraryUpgradesOnceWithoutChangingPinnedRunsOrState(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	old := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	old.Stages[6].Outcomes = old.Stages[6].Outcomes[:2]
	old, err := storage.SaveNewVersion(old)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := storage.SetPaused(old.ID, true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	pinned := core.NewRun(core.RunArgs{Belt: old, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "a/b", Number: 1, Title: "Old"}, Now: upgradeTime})
	if err := storage.SaveRun(pinned); err != nil {
		t.Fatalf("save run: %v", err)
	}

	first := libraryOver(t, storage).Belt(old.ID)
	if first == nil {
		t.Fatalf("no belt after the first load")
	}
	if first.Belt.Version != old.Version+1 || !first.Paused {
		t.Errorf("upgraded belt = version %d, paused %v", first.Belt.Version, first.Paused)
	}
	if got := len(first.Belt.Stages[6].Outcomes); got != 8 {
		t.Errorf("outcomes after the upgrade = %d, want 8", got)
	}
	if !slices.EqualFunc(first.Belt.Stages[6].Outcomes[:2], old.Stages[6].Outcomes, sameOutcome) {
		t.Errorf("the authored outcomes changed")
	}
	runs := storage.LoadRuns()
	if len(runs) != 1 || !sameStages(runs[0].Stages, pinned.Stages) {
		t.Errorf("the pinned run's stages changed")
	}
	if got := len(core.NewRun(core.RunArgs{Belt: first.Belt, Subject: pinned.Subject, Now: upgradeTime}).Stages[6].Outcomes); got != 8 {
		t.Errorf("a new run of the upgraded belt has %d outcomes", got)
	}
	if again := libraryOver(t, storage).Belt(old.ID); again == nil || !sameBelt(again.Belt, first.Belt) {
		t.Errorf("a second load changed the upgraded belt")
	}
	trimmed := first.Belt.Clone()
	trimmed.Stages[6].Outcomes = trimmed.Stages[6].Outcomes[:2]
	if _, err := storage.SaveNewVersion(trimmed); err != nil {
		t.Fatalf("save trimmed: %v", err)
	}
	if got := libraryOver(t, storage).Belt(old.ID); got == nil || len(got.Belt.Stages[6].Outcomes) != 2 {
		t.Errorf("taking the added outcomes away did not stick")
	}
}

func TestAStateSavedBeforeWatchOutcomesKeepsPausedAndLastFired(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	old := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	old.Stages[6].Outcomes = old.Stages[6].Outcomes[:2]
	old, err := storage.SaveNewVersion(old)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	fired := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Add(800000000 * time.Second)
	firedText, err := jsonx.Marshal(jsonx.Date{Time: fired})
	if err != nil {
		t.Fatalf("encode date: %v", err)
	}
	state := fmt.Sprintf(`{"paused":true,"lastFired":%s}`, firedText)
	if err := os.WriteFile(filepath.Join(storage.Paths.Belts(), old.ID, "state.json"), []byte(state), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	record := libraryOver(t, storage).Belt(old.ID)
	if record == nil {
		t.Fatalf("no belt")
	}
	if !record.Paused {
		t.Errorf("the pause was lost")
	}
	if last := record.LastFired(); last == nil || !last.Equal(fired) {
		t.Errorf("last fired = %v, want %v", last, fired)
	}
	if got := len(record.Belt.Stages[6].Outcomes); got != 8 {
		t.Errorf("outcomes = %d, want 8", got)
	}
	reloaded := storage.LoadBelts()
	if len(reloaded) != 1 || !reloaded[0].Paused || reloaded[0].LastFired() == nil || !reloaded[0].LastFired().Equal(fired) || !reloaded[0].WatchOutcomes {
		t.Errorf("the stored state lost the pause or the firing")
	}
}

func TestANewBeltKeepsTheWatchOutcomesItWasSavedWith(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	library := libraryOver(t, storage)
	belt := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	belt.ID = library.NewBelt().ID
	belt.Version = 0
	// Without the fork after the watch, whose routes its outcomes take.
	belt.Stages = core.SampleStages(core.TriggerKindManual, core.SampleAgents(), true)
	belt.Forks = nil
	watch := watchIndex(t, belt)
	belt.Stages[watch].Outcomes = belt.Stages[watch].Outcomes[:2]
	saved, err := library.SaveBelt(belt)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded := libraryOver(t, storage).Belt(saved.ID)
	if reloaded == nil || !sameBelt(reloaded.Belt, saved) {
		t.Fatalf("the reloaded belt differs from the saved one")
	}
	if got := len(reloaded.Belt.Stages[watch].Outcomes); got != 2 {
		t.Errorf("outcomes = %d, want 2", got)
	}
}

// sameOutcome compares two outcomes by their wire form.
func sameOutcome(a, b core.Outcome) bool {
	x, errA := jsonx.Marshal(a)
	y, errB := jsonx.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}

// sameStages compares two stage lists by their wire form, in order.
func sameStages(a, b []core.Stage) bool {
	x, errA := jsonx.Marshal(a)
	y, errB := jsonx.Marshal(b)
	return errA == nil && errB == nil && string(x) == string(y)
}
