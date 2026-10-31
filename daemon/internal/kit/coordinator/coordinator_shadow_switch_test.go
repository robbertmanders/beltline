// Coordinator tests of Enable shadow tests: while any belt has a test running the switch stays on,
// whatever the belt's state, and ending the last test unlocks it.

package coordinator

import (
	"reflect"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestEnableShadowTestsStaysOnWhileAnyTestRuns starts a test on a live, a paused and an archived belt.
// Turning the switch off is refused with the same reason until every test has ended, a refused patch
// changes nothing, and a patch that keeps the switch on, or changes other settings, is allowed.
func TestEnableShadowTestsStaysOnWhileAnyTestRuns(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks"), manualBelt("reports"), manualBelt("archive")}})
	c := h.coord
	c.Hold()
	defer func() { c.Release(); c.Idle() }()

	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)}); err != nil {
		t.Fatalf("enable shadow tests: %v", err)
	}
	for _, id := range []string{"tasks", "reports", "archive"} {
		startShadowTest(t, c, id)
	}
	if _, err := c.PauseBeltCommand("reports", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ArchiveBeltCommand("archive", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.StartTaskCommand("tasks", "acme/web", "Kept", "", nil); err != nil {
		t.Fatal(err)
	}

	toggle := c.ShadowTestsToggle()
	if toggle.Enabled || toggle.Reason == nil || *toggle.Reason != core.SettingShadowTestsRunning {
		t.Fatalf("toggle with tests running = %+v", toggle)
	}
	before := c.ReadSettings()
	beforeTests := recordTests(c)
	beforeRuns := c.Runs()
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err == nil || err.Error() != core.SettingShadowTestsRunning {
		t.Fatalf("a false patch with tests running: %v", err)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(true), EnableShadowTests: new(false)}); err == nil || err.Error() != core.SettingShadowTestsRunning {
		t.Fatalf("a mixed false patch with tests running: %v", err)
	}
	if !reflect.DeepEqual(c.ReadSettings(), before) || !reflect.DeepEqual(recordTests(c), beforeTests) || !reflect.DeepEqual(c.Runs(), beforeRuns) {
		t.Fatal("a refused patch changed settings, tests or runs")
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true), Notifications: new(false)}); err != nil {
		t.Fatalf("a true patch with tests running: %v", err)
	}
	if saved, err := c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(true)}); err != nil || !saved.ShowInsights || !saved.EnableShadowTests {
		t.Fatalf("an unrelated patch with tests running: %+v, %v", saved, err)
	}

	// Ending one test leaves the others, so the switch stays locked.
	if _, err := c.AcceptShadowCommand("tasks"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DiscardShadowCommand("reports"); err != nil {
		t.Fatal(err)
	}
	if got := c.ShadowTestsToggle(); got.Enabled {
		t.Fatalf("toggle with a test still running = %+v", got)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err == nil {
		t.Fatal("the switch unlocked while a test on an archived belt runs")
	}

	// Ending the last test unlocks the switch, which stays on until it is turned off.
	if _, err := c.DiscardShadowCommand("archive"); err != nil {
		t.Fatal(err)
	}
	if got := c.ShadowTestsToggle(); !got.Enabled || got.Reason != nil {
		t.Fatalf("toggle with every test ended = %+v", got)
	}
	if !c.ReadSettings().EnableShadowTests {
		t.Fatal("ending the last test turned the switch off")
	}
	saved, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)})
	if err != nil || saved.EnableShadowTests {
		t.Fatalf("turning it off after the last test: %+v, %v", saved, err)
	}
	if got := len(c.Runs()); got != len(beforeRuns) {
		t.Fatalf("turning the switch off changed the runs: %d, want %d", got, len(beforeRuns))
	}
}

// TestEndedTestsAndTheirRunsDoNotBlockDisabling checks a test that has ended, with runs still going on
// it, does not keep the switch on.
func TestEndedTestsAndTheirRunsDoNotBlockDisabling(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
	c := h.coord
	c.Hold()
	defer func() { c.Release(); c.Idle() }()
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)}); err != nil {
		t.Fatal(err)
	}
	startShadowTest(t, c, "tasks")
	if _, err := c.StartTaskCommand("tasks", "acme/web", "Finishing", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AcceptShadowCommand("tasks"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err != nil {
		t.Fatalf("turning it off with only an ended test: %v", err)
	}
}

// startShadowTest saves a changed shadow of the belt, which starts its test.
func startShadowTest(t *testing.T, c *Coordinator, id string) {
	t.Helper()
	draft := c.ReadBelt(id).Belt.Clone()
	draft.Stages[0].Name = "Shadow " + draft.Stages[0].Name
	draft.Triggers = nil
	if record, err := c.SaveShadowCommand(draft); err != nil || record.ShadowTest() == nil {
		t.Fatalf("start a test on %s: %v", id, err)
	}
}

// recordTests is every belt's shadow tests, by belt id.
func recordTests(c *Coordinator) map[string][]core.ShadowTest {
	out := map[string][]core.ShadowTest{}
	for _, record := range c.ReadBelts() {
		out[record.Belt.ID] = record.ShadowTests
	}
	return out
}
