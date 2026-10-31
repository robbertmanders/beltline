// Coordinator tests of shadow tests: new runs alternate between live and the
// shadow and keep their version, and accept and discard end the test.

package coordinator

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestShadowTestSplitsNewRunsAndEndsOnAcceptOrDiscard walks one belt through a
// test that is accepted and one that is discarded.
func TestShadowTestSplitsNewRunsAndEndsOnAcceptOrDiscard(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
	c := h.coord
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)}); err != nil {
		t.Fatalf("enable shadow tests: %v", err)
	}
	c.Hold()
	defer func() { c.Release(); c.Idle() }()

	live := c.ReadBelt("tasks").Belt
	if refusal := c.BeltCommandRefusal(core.BeltCommandShadowAccept, "tasks", false); refusal == nil || *refusal != core.BeltAvailabilityNoShadow {
		t.Fatalf("accept without a shadow: %v", refusal)
	}
	draft := live.Clone()
	draft.Stages[0].Name = "Shadow triage"
	draft.Triggers = nil
	record, err := c.SaveShadowCommand(draft)
	if err != nil {
		t.Fatal(err)
	}
	test := record.ShadowTest()
	if test == nil || test.ID != "t1" || test.Live != live.Version || record.Shadow == nil || record.Shadow.Version != live.Version+1 {
		t.Fatalf("after the first shadow save %+v", record.ShadowTests)
	}
	if len(record.Shadow.Triggers) != len(live.Triggers) || record.Belt.Version != live.Version {
		t.Fatalf("the shadow lost live's triggers or live moved: %+v", record.Shadow.Triggers)
	}
	if refusal := c.BeltCommandRefusal(core.BeltCommandShadowCreate, "tasks", false); refusal == nil {
		t.Fatal("a second shadow could start while one runs")
	}

	arms := map[core.ShadowArm]int{}
	for _, title := range []string{"One", "Two", "Three", "Four"} {
		run, err := c.StartTaskCommand("tasks", "acme/web", title, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if run.Shadow == nil || run.Shadow.Test != "t1" {
			t.Fatalf("run %s has no mark: %+v", title, run.Shadow)
		}
		arms[run.Shadow.Arm]++
		wantVersion, wantName := live.Version, live.Stages[0].Name
		if run.Shadow.Arm == core.ShadowArmShadow {
			wantVersion, wantName = record.Shadow.Version, "Shadow triage"
		}
		if run.BeltVersion != wantVersion || run.Stages[0].Name != wantName {
			t.Fatalf("run on %s started on v%d with %q", run.Shadow.Arm, run.BeltVersion, run.Stages[0].Name)
		}
	}
	if arms[core.ShadowArmLive] != 2 || arms[core.ShadowArmShadow] != 2 {
		t.Fatalf("arms %v", arms)
	}

	draft.Stages[0].Name = "Shadow triage, again"
	record, err = c.SaveShadowCommand(draft)
	if err != nil || record.ShadowTest().ID != "t1" || record.Shadow.Version != live.Version+2 {
		t.Fatalf("second shadow save %+v, %v", record.ShadowTests, err)
	}
	record, err = c.AcceptShadowCommand("tasks")
	if err != nil {
		t.Fatal(err)
	}
	if record.ShadowTest() != nil || record.Shadow != nil || record.Belt.Version != live.Version+2 || record.Belt.Stages[0].Name != "Shadow triage, again" {
		t.Fatalf("after accept v%d %+v", record.Belt.Version, record.ShadowTests)
	}
	if record.ShadowTests[0].Result != core.ShadowResultAccepted {
		t.Fatalf("test result %+v", record.ShadowTests[0])
	}
	for _, run := range c.Runs() {
		if run.BeltVersion != live.Version && run.BeltVersion != live.Version+1 {
			t.Fatalf("a run moved to v%d", run.BeltVersion)
		}
	}
	if run, err := c.StartTaskCommand("tasks", "acme/web", "After", "", nil); err != nil || run.Shadow != nil || run.BeltVersion != live.Version+2 {
		t.Fatalf("a run after the test %+v, %v", run.Shadow, err)
	}

	if _, err := c.SaveShadowCommand(draft); err != nil {
		t.Fatal(err)
	}
	record, err = c.DiscardShadowCommand("tasks")
	if err != nil {
		t.Fatal(err)
	}
	if record.ShadowTest() != nil || record.Belt.Version != live.Version+2 || len(record.ShadowTests) != 2 || record.ShadowTests[1].Result != core.ShadowResultDiscarded {
		t.Fatalf("after discard v%d %+v", record.Belt.Version, record.ShadowTests)
	}
}

// TestShadowTestsStartOnlyWhileSettingsEnableThem checks a shadow test cannot start while Settings keeps
// shadow tests off, whether asked directly, by a stale read or from an open draft, and that enabling
// them restores creation. A refused start stores neither a test nor a shadow version.
func TestShadowTestsStartOnlyWhileSettingsEnableThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
	c := h.coord
	c.Hold()
	defer func() { c.Release(); c.Idle() }()

	live := c.ReadBelt("tasks").Belt
	draft := live.Clone()
	draft.Stages[0].Name = "Shadow triage"
	draft.Triggers = nil
	const reason = core.BeltAvailabilityShadowTestsOff
	if refusal := c.BeltCommandRefusal(core.BeltCommandShadowCreate, "tasks", false); refusal == nil || *refusal != reason {
		t.Fatalf("create while off: %v", refusal)
	}
	if actions := c.DraftActions(draft); !beltActionDisabled(actions, core.BeltCommandShadowCreate, reason) {
		t.Fatalf("an open draft may start a test while off: %+v", actions)
	}
	if _, err := c.SaveShadowCommand(draft); err == nil || err.Error() != reason {
		t.Fatalf("save a shadow while off: %v", err)
	}
	if record := c.ReadBelt("tasks"); record.ShadowTest() != nil || record.Shadow != nil || len(record.ShadowTests) != 0 {
		t.Fatalf("a refused start stored a test: %+v", record.ShadowTests)
	}

	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)}); err != nil {
		t.Fatalf("enable shadow tests: %v", err)
	}
	if refusal := c.BeltCommandRefusal(core.BeltCommandShadowCreate, "tasks", false); refusal != nil {
		t.Fatalf("create after enabling: %v", *refusal)
	}
	if record, err := c.SaveShadowCommand(draft); err != nil || record.ShadowTest() == nil {
		t.Fatalf("start after enabling: %v", err)
	}
	if _, err := c.DiscardShadowCommand("tasks"); err != nil {
		t.Fatal(err)
	}

	// A create read while enabled is refused once the switch is off again, with the same reason.
	enabled := c.DraftActions(draft)
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err != nil {
		t.Fatalf("turn shadow tests off with none running: %v", err)
	}
	if !beltActionDisabled(c.DraftActions(draft), core.BeltCommandShadowCreate, reason) || beltActionDisabled(enabled, core.BeltCommandShadowCreate, reason) {
		t.Fatal("the create action did not follow the switch")
	}
	if _, err := c.SaveShadowCommand(draft); err == nil || err.Error() != reason {
		t.Fatalf("a stale start after turning it off: %v", err)
	}
	if got := len(c.ReadBelt("tasks").ShadowTests); got != 1 {
		t.Fatalf("a refused start stored tests: %d", got)
	}
}

// beltActionDisabled reports whether actions has command disabled with reason.
func beltActionDisabled(actions []core.BeltAction, command core.BeltCommand, reason string) bool {
	for _, action := range actions {
		if action.ID == string(command) {
			return !action.Enabled && action.Reason != nil && *action.Reason == reason
		}
	}
	return false
}
