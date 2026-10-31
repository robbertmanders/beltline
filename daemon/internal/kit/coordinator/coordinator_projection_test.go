// Coordinator tests of change expectations: each transition the coordinator core owns invalidates the
// keys it affects, and checking and the last check invalidate state without invalidating intake. Each
// transition is one turn; the resources it changes are reported once, sorted, when the turn ends.

package coordinator

import (
	"context"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// sortedResources returns resources sorted, as a turn reports them.
func sortedResources(resources ...Resource) []Resource {
	out := append([]Resource(nil), resources...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestNamedResourceTransitionMatrixInvalidatesEveryAffectedKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	belt := c.Belts()[0].Belt
	run := core.NewRun(core.RunArgs{
		Belt:    belt,
		Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/events", Number: 580, Title: "Event fixture"},
		Now:     testkit.ContractTime,
	})
	runID := run.ID
	beltID := belt.ID
	agentID := h.coord.library.agents[0].Agent.ID
	// Every memo is checked against a fresh encoding on each turn below, so a
	// write that forgot its revision fails here as well as in its own row.
	verifyAllHits.Store(true)
	defer verifyAllHits.Store(false)
	rows := []struct {
		name     string
		expected []Resource
		change   func()
	}{
		{"run add", sortedResources(ResourceRuns, ResourceState), func() { h.coord.book.add(run) }},
		{"run update", sortedResources(ResourceRuns, ResourceState), func() {
			if !h.coord.book.update(runID, nil, nil, func(r *core.Run) { r.Subject.Title += " updated" }) {
				t.Fatal("update refused")
			}
		}},
		{"run erase", sortedResources(ResourceRuns, ResourceState), func() {
			h.coord.book.markErasing(runID)
			h.coord.book.removeErased(runID)
		}},
		{"intake check progress", sortedResources(ResourceState), func() {
			h.coord.intake.setChecking(true)
			h.coord.intake.setLastCheck(testkit.ContractTime.Add(10 * time.Second))
		}},
		{"waiting subjects", sortedResources(ResourceIntake), func() {
			h.coord.intake.setWaiting("matrix", []core.WaitingSubject{{
				BeltID:  belt.ID,
				Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/events", Number: 581, Title: "Waiting"},
				Reason:  "blocked",
			}})
		}},
		{"waiting subjects cleared", sortedResources(ResourceIntake), func() {
			h.coord.intake.setWaiting("matrix", nil)
		}},
		{"notice", sortedResources(ResourceState), func() { h.coord.book.note("Something happened", nil) }},
		{"saved settings", sortedResources(ResourceSettings, ResourceState), func() {
			h.coord.changeSettings(func(s *store.Settings) { s.Login = "event matrix" })
		}},
		{"library repository setting", sortedResources(ResourceLibrary, ResourceSettings), func() {
			h.coord.changeSettings(func(s *store.Settings) {
				s.LibraryRepositories = []core.LibraryRepository{{Repository: "example/shared", Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest}}
			})
		}},
		{"model allowed", sortedResources(ResourceModels, ResourceSettings), func() {
			h.coord.changeSettings(func(s *store.Settings) {
				s.AllowedModels["pi"]["new-model"] = struct{}{}
			})
		}},
		{"model discovery started", sortedResources(ResourceModels), func() {
			h.coord.modelCatalogs.setLoading(core.BackendPi)
		}},
		{"model discovery finished", sortedResources(ResourceModels), func() {
			h.coord.modelCatalogs.clearLoading(core.BackendPi)
		}},
		{"model refresh failure", sortedResources(ResourceModels), func() {
			h.coord.modelCatalogs.setError(core.BackendPi, "discovery failed")
		}},
		{"model refresh failure cleared", sortedResources(ResourceModels), func() {
			h.coord.modelCatalogs.clearError(core.BackendPi)
		}},
		{"belt archived", sortedResources(ResourceLibrary), func() {
			h.coord.library.SetBeltArchived(beltID, true)
		}},
		{"belt restored", sortedResources(ResourceLibrary), func() {
			h.coord.library.SetBeltArchived(beltID, false)
		}},
		{"belt saved as a new version", sortedResources(ResourceLibrary), func() {
			belt := h.coord.library.Belt(beltID).Belt
			belt.Name += " (edited)"
			if _, err := h.coord.library.SaveBelt(belt); err != nil {
				t.Fatalf("save belt: %v", err)
			}
		}},
		{"agent archived", sortedResources(ResourceLibrary), func() {
			if _, err := h.coord.library.ArchivingAgent(agentID, true); err != nil {
				t.Fatalf("archive agent: %v", err)
			}
		}},
		{"agent restored", sortedResources(ResourceLibrary), func() {
			if _, err := h.coord.library.ArchivingAgent(agentID, false); err != nil {
				t.Fatalf("restore agent: %v", err)
			}
		}},
		{"AFK decision logged", sortedResources(ResourceAfk), func() {
			h.coord.autopilot.add(core.AfkDecision{ID: "matrix-decision"})
		}},
		{"AFK decision edited", sortedResources(ResourceAfk), func() {
			h.coord.autopilot.change("matrix-decision", func(d *core.AfkDecision) { d.Reviewed = true })
		}},
		{"assistant problems", sortedResources(ResourceAssistant), func() {
			h.coord.assistant.problems = []string{"belts/invalid-event-fixture/v1.json is not JSON"}
		}},
	}
	// Work a row spawns is held until the row's changes are read, so it cannot report into the next row.
	h.coord.Hold()
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			h.ClearChanges()
			h.Turn(row.change)
			got := h.Changes()
			h.coord.Idle()
			if len(got) != 1 {
				t.Fatalf("%s: %d turn reports, want one flush: %v", row.name, len(got), got)
			}
			if !reflect.DeepEqual(got[0], row.expected) {
				t.Fatalf("%s: changed %v, want %v", row.name, got[0], row.expected)
			}
		})
	}
}

// TestWritesThatLeaveTheValuesAsTheyWereReportNothing pins the other half of the
// memo contract: a write that bumps a revision without changing what a projection
// shows is no change for the browser, as the byte comparison has always said.
func TestWritesThatLeaveTheValuesAsTheyWereReportNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	belt := h.Coord().Belts()[0].Belt
	rows := []struct {
		name   string
		change func()
	}{
		{"setting written with its own value", func() {
			h.coord.changeSettings(func(s *store.Settings) { login := s.Login; s.Login = login })
		}},
		{"discovery finished for a runner not loading", func() {
			h.coord.modelCatalogs.clearLoading(core.BackendPi)
		}},
		{"error cleared for a runner without one", func() {
			h.coord.modelCatalogs.clearError(core.BackendPi)
		}},
		{"belt archived as it is", func() {
			h.coord.library.SetBeltArchived(belt.ID, h.coord.library.Belt(belt.ID).Archived)
		}},
		{"AFK review with nothing to review", func() { h.coord.autopilot.review(nil) }},
		{"run removed that is not there", func() { h.coord.book.removeErased("no-such-run") }},
	}
	h.coord.Hold()
	for _, row := range rows {
		h.ClearChanges()
		h.Turn(row.change)
		got := h.Changes()
		h.coord.Idle()
		if len(got) != 0 {
			t.Fatalf("%s: changes = %v, want none", row.name, got)
		}
	}
}

// TestAnUnchangedProjectionKeepsItsEncoding checks that the memoised projections hand back the
// same bytes while their state is unchanged, so a turn that changes nothing encodes nothing.
func TestAnUnchangedProjectionKeepsItsEncoding(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	for _, r := range []Resource{ResourceRuns, ResourceLibrary, ResourceSettings, ResourceModels, ResourceAfk, ResourceState, ResourceIntake} {
		first := h.coord.Projection(r)
		again := h.coord.Projection(r)
		if len(first) == 0 || &first[0] != &again[0] {
			t.Fatalf("projection of %s was encoded again without a change", r)
		}
	}
}

// TestGlobalPauseInvalidatesSettingsAndState pins the one write the matrix cannot hold in a turn:
// SetGloballyPaused takes the lock itself. It is a turn of its own, and both the saved settings
// and the state it shows change. Work it spawns is held until the turn's changes are read, so
// a tick it kicks cannot report a later change into this check.
func TestGlobalPauseInvalidatesSettingsAndState(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	want := [][]Resource{sortedResources(ResourceSettings, ResourceState)}
	for _, paused := range []bool{true, false} {
		h.ClearChanges()
		h.Coord().Hold()
		h.Coord().SetGloballyPaused(context.Background(), paused)
		got := h.Changes()
		h.Coord().Idle()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("paused=%t changes = %v, want %v", paused, got, want)
		}
	}
}

func TestCheckingAndLastCheckInvalidateStateWithoutInvalidatingIntake(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.ClearChanges()
	h.Turn(func() {
		h.coord.intake.setChecking(true)
		h.coord.intake.setLastCheck(time.Unix(1, 0))
	})
	if got := h.Changes(); !reflect.DeepEqual(got, [][]Resource{{ResourceState}}) {
		t.Fatalf("changes = %v, want state only", got)
	}
}

func TestATurnThatChangesNothingReportsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.ClearChanges()
	h.Turn(func() {})
	h.Turn(func() {
		h.coord.intake.setChecking(true)
		h.coord.intake.setChecking(false)
	})
	if got := h.Changes(); len(got) != 0 {
		t.Fatalf("changes = %v, want none", got)
	}
}

func TestOneTurnWithSeveralChangesReportsOnceSorted(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.ClearChanges()
	h.Turn(func() {
		h.coord.changeSettings(func(s *store.Settings) { s.Login = "first" })
		h.coord.book.note("first note", nil)
		h.coord.assistant.problems = []string{"x"}
	})
	want := [][]Resource{sortedResources(ResourceAssistant, ResourceSettings, ResourceState)}
	if got := h.Changes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestProjectionEncodesEveryResourceAndIsStable(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	for _, resource := range AllResources {
		first := h.coord.Projection(resource)
		if len(first) == 0 {
			t.Fatalf("projection of %s is empty", resource)
		}
		if strings.HasPrefix(string(first), "error: ") {
			t.Fatalf("projection of %s does not encode: %s", resource, first)
		}
		if again := h.coord.Projection(resource); string(again) != string(first) {
			t.Fatalf("projection of %s changed without a change", resource)
		}
	}
}

func TestProjectionOfAStartedCoordinatorMatchesItsBaseline(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.ClearChanges()
	// A read is a turn that changes nothing; the baseline set at New is still current.
	_ = h.coord.Runs()
	_ = h.coord.Pause()
	if got := h.Changes(); len(got) != 0 {
		t.Fatalf("reads reported changes: %v", got)
	}
}

// TestShadowTestsAndTheSwitchChangeTheResourcesTheyGate: starting and ending a test changes the library, on
// which the settings' switch availability depends, and a switch change changes the belt's actions and revision.
func TestShadowTestsAndTheSwitchChangeTheResourcesTheyGate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
	c := h.Coord()
	c.Hold()
	defer func() { c.Release(); c.Idle() }()

	h.ClearChanges()
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)}); err != nil {
		t.Fatal(err)
	}
	if got := h.Changes(); !reflect.DeepEqual(got, [][]Resource{sortedResources(ResourceSettings)}) {
		t.Fatalf("enabling the switch changed %v", got)
	}
	record := *c.ReadBelt("tasks")
	before := c.BeltResourceContext(record, false)
	startShadowTest(t, c, "tasks")
	record = *c.ReadBelt("tasks")
	if got := h.Changes(); !slices.Contains(got[len(got)-1], ResourceLibrary) {
		t.Fatalf("starting a test changed %v, not the library", got)
	}
	started := c.BeltResourceContext(record, false)
	if started.Revision == before.Revision {
		t.Fatal("starting a test kept the belt's revision")
	}
	if _, err := c.AcceptShadowCommand("tasks"); err != nil {
		t.Fatal(err)
	}
	if got := h.Changes(); !slices.Contains(got[len(got)-1], ResourceLibrary) {
		t.Fatalf("ending a test changed %v, not the library", got)
	}

	h.ClearChanges()
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err != nil {
		t.Fatal(err)
	}
	record = *c.ReadBelt("tasks")
	if got := c.BeltResourceContext(record, false); got.Revision == started.Revision {
		t.Fatal("turning the switch off kept the belt's revision")
	}
	if !reflect.DeepEqual(h.Changes(), [][]Resource{sortedResources(ResourceSettings)}) {
		t.Fatalf("turning the switch off changed %v", h.Changes())
	}
}
