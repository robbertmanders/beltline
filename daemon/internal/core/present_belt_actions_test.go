// Tests of the belt command availability table, which covers the editor states.

package core

import "testing"

// beltActionsTestEnabled is the enabled flag of command in actions, and whether it was found.
func beltActionsTestEnabled(actions []BeltAction, command BeltCommand) (bool, bool) {
	for _, action := range actions {
		if action.ID == string(command) {
			return action.Enabled, true
		}
	}
	return false, false
}

// beltActionsTestReason is the reason of the action for command, or "" when it has none.
func beltActionsTestReason(actions []BeltAction, command BeltCommand) string {
	for _, action := range actions {
		if action.ID == string(command) && action.Reason != nil {
			return *action.Reason
		}
	}
	return ""
}

func TestAvailabilityTableCoversTheEditorStates(t *testing.T) {
	t.Parallel()
	newBelt := BeltActionContext{Saved: false}
	newActions := BeltAvailabilityActions(newBelt)
	if enabled, _ := beltActionsTestEnabled(newActions, BeltCommandSave); !enabled {
		t.Errorf("save on a new belt disabled, want enabled")
	}
	if enabled, _ := beltActionsTestEnabled(newActions, BeltCommandPause); enabled {
		t.Errorf("pause on a new belt enabled, want disabled")
	}
	if reason := beltActionsTestReason(newActions, BeltCommandCopy); reason != BeltAvailabilityUnsavedCopy {
		t.Errorf("copy reason = %q, want %q", reason, BeltAvailabilityUnsavedCopy)
	}

	live := BeltActionContext{Saved: true, OutdatedStages: 2, HasManualTrigger: true}
	if action := BeltAvailabilityAction(BeltCommandPause, live); action == nil || !action.Enabled {
		t.Errorf("pause on a live belt = %+v, want enabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandResume, live); action == nil || action.Enabled {
		t.Errorf("resume on a live belt = %+v, want disabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandUpdateAllStages, live); action == nil || !action.Enabled {
		t.Errorf("update-all-stages with outdated stages = %+v, want enabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandArchive, live); action == nil || !action.Enabled {
		t.Errorf("archive on a live belt = %+v, want enabled", action)
	}

	paused := BeltActionContext{Saved: true, Paused: true}
	if action := BeltAvailabilityAction(BeltCommandPause, paused); action == nil || action.Enabled {
		t.Errorf("pause on a paused belt = %+v, want disabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandResume, paused); action == nil || !action.Enabled {
		t.Errorf("resume on a paused belt = %+v, want enabled", action)
	}

	warned := BeltActionContext{Saved: true, Paused: true, Warnings: []string{"Add at least one stage."}}
	if action := BeltAvailabilityAction(BeltCommandResume, warned); action == nil || action.Enabled {
		t.Errorf("resume on a warned belt = %+v, want disabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandResume, warned); action == nil || action.Reason == nil || *action.Reason != BeltAvailabilityResumeBlocked {
		t.Errorf("resume reason on a warned belt = %+v, want %q", action, BeltAvailabilityResumeBlocked)
	}

	archived := BeltActionContext{Saved: true, Archived: true}
	if action := BeltAvailabilityAction(BeltCommandPause, archived); action == nil || action.Enabled {
		t.Errorf("pause on an archived belt = %+v, want disabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandUnarchive, archived); action == nil || !action.Enabled {
		t.Errorf("unarchive on an archived belt = %+v, want enabled", action)
	}
	if action := BeltAvailabilityAction(BeltCommandUpdateAllStages, archived); action == nil || action.Enabled {
		t.Errorf("update-all-stages on an archived belt = %+v, want disabled", action)
	}

	fresh := BeltActionContext{Saved: true}
	if action := BeltAvailabilityAction(BeltCommandUpdateAllStages, fresh); action == nil || action.Enabled {
		t.Errorf("update-all-stages with nothing outdated = %+v, want disabled", action)
	}
	if refusal := BeltAvailabilityRefusal(BeltCommandUpdateAllStages, fresh); refusal == nil {
		t.Errorf("refusal of update-all-stages with nothing outdated = nil, want a reason")
	}
	if refusal := BeltAvailabilityRefusal(BeltCommandUpdateAllStages, live); refusal != nil {
		t.Errorf("refusal of update-all-stages when outdated = %q, want nil", *refusal)
	}
}

// TestShadowCreateNeedsTheExperimentalSwitch checks Create shadow is refused with the switch off, with the
// reason the editor shows, and that the running-test and archive reasons come first.
func TestShadowCreateNeedsTheExperimentalSwitch(t *testing.T) {
	t.Parallel()
	off := BeltActionContext{Saved: true, HasManualTrigger: true}
	if action := BeltAvailabilityAction(BeltCommandShadowCreate, off); action == nil || action.Enabled || action.Reason == nil || *action.Reason != BeltAvailabilityShadowTestsOff {
		t.Errorf("create shadow with the switch off = %+v, want disabled with the switch reason", action)
	}
	if refusal := BeltAvailabilityRefusal(BeltCommandShadowCreate, off); refusal == nil || *refusal != "Enable shadow tests in Settings → Experimental to start a new test." {
		t.Errorf("refusal with the switch off = %v", refusal)
	}

	on := off
	on.ShadowTestsEnabled = true
	if action := BeltAvailabilityAction(BeltCommandShadowCreate, on); action == nil || !action.Enabled || action.Reason != nil {
		t.Errorf("create shadow with the switch on = %+v, want enabled", action)
	}
	if refusal := BeltAvailabilityRefusal(BeltCommandShadowCreate, on); refusal != nil {
		t.Errorf("refusal with the switch on = %q, want nil", *refusal)
	}

	running := on
	running.Shadow = true
	if action := BeltAvailabilityAction(BeltCommandShadowCreate, running); action == nil || action.Enabled || *action.Reason != BeltAvailabilityShadowRunning {
		t.Errorf("create shadow with a test running = %+v", action)
	}
	archived := off
	archived.Archived = true
	if action := BeltAvailabilityAction(BeltCommandShadowCreate, archived); action == nil || *action.Reason != BeltAvailabilityArchivedNote {
		t.Errorf("create shadow on an archived belt = %+v", action)
	}
	unsaved := BeltActionContext{}
	if action := BeltAvailabilityAction(BeltCommandShadowCreate, unsaved); action == nil || *action.Reason != BeltAvailabilityUnsavedCopy {
		t.Errorf("create shadow on an unsaved belt = %+v", action)
	}

	// The switch gates creation only: a running test keeps its save, accept and discard.
	running = off
	running.Shadow = true
	for _, command := range []BeltCommand{BeltCommandShadowSave, BeltCommandShadowAccept, BeltCommandShadowDiscard} {
		if action := BeltAvailabilityAction(command, running); action == nil || !action.Enabled {
			t.Errorf("%s with a running test and the switch off = %+v, want enabled", command, action)
		}
	}
}

// TestResourceRevisionTokenMatchesFixedValues pins ResourceRevision.token: FNV-1a 64 over the
// UTF-8 bytes of the parts joined by U+001F, as 16 lower-case hex digits. The expected values
// were computed by an independent script that follows the function, so they
// are not checked against any other implementation.
func TestResourceRevisionTokenMatchesFixedValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		parts []string
		want  string
	}{
		{[]string{}, "cbf29ce484222325"},
		{[]string{""}, "cbf29ce484222325"},
		{[]string{"a"}, "af63dc4c8601ec8c"},
		{[]string{"a", "b"}, "e5eaa319043b0991"},
		{[]string{"ab"}, "089c4407b545986a"},
		{[]string{"paused", "true"}, "f91d0139859719bc"},
		{[]string{"Émile", "Zed"}, "959193778a807880"},
		{[]string{"acme/billing", "3"}, "c3f8d0fa33e82b45"},
	}
	for _, c := range cases {
		if got := ResourceRevisionToken(c.parts); got != c.want {
			t.Errorf("ResourceRevisionToken(%q) = %s, want %s", c.parts, got, c.want)
		}
	}
	if ResourceRevisionToken([]string{"a", "b"}) == ResourceRevisionToken([]string{"ab"}) {
		t.Error("the separator does not change the token: [a b] and [ab] collide")
	}
}
