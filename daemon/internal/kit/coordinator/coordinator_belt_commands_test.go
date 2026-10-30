// Coordinator tests of the belt commands.

package coordinator

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// actionByID returns the action with id, failing the test when there is none.
func actionByID(t *testing.T, actions []core.BeltAction, id string) core.BeltAction {
	t.Helper()
	for _, action := range actions {
		if action.ID == id {
			return action
		}
	}
	t.Fatalf("no action %q", id)
	return core.BeltAction{}
}

// beltRecordOf returns the saved belt record with id, failing the test when there is none.
func beltRecordOf(t *testing.T, c *Coordinator, id string) store.BeltRecord {
	t.Helper()
	record := c.Belt(id)
	if record == nil {
		t.Fatalf("no belt %q", id)
	}
	return *record
}

// actionsOf are the belt actions the saved belt with id offers now.
func actionsOf(t *testing.T, c *Coordinator, id string) []core.BeltAction {
	t.Helper()
	return core.BeltAvailabilityActions(c.ActionContext(beltRecordOf(t, c, id), false))
}

func TestAvailabilityTableCoversTheEditorStates(t *testing.T) {
	t.Parallel()
	newActions := core.BeltAvailabilityActions(core.BeltActionContext{Saved: false})
	if !actionByID(t, newActions, string(core.BeltCommandSave)).Enabled {
		t.Errorf("save is enabled for a new belt")
	}
	if actionByID(t, newActions, string(core.BeltCommandPause)).Enabled {
		t.Errorf("pause is disabled for a new belt")
	}
	if reason := actionByID(t, newActions, string(core.BeltCommandCopy)).Reason; reason == nil || *reason != core.BeltAvailabilityUnsavedCopy {
		t.Errorf("copy reason = %v, want the unsaved copy reason", reason)
	}

	live := core.BeltActionContext{Saved: true, OutdatedStages: 2, HasManualTrigger: true}
	if action := core.BeltAvailabilityAction(core.BeltCommandPause, live); action == nil || !action.Enabled {
		t.Errorf("pause on a live belt: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandResume, live); action == nil || action.Enabled {
		t.Errorf("resume on a live belt: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandUpdateAllStages, live); action == nil || !action.Enabled {
		t.Errorf("update-all-stages with outdated stages: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandArchive, live); action == nil || !action.Enabled {
		t.Errorf("archive on a live belt: %+v", action)
	}

	paused := core.BeltActionContext{Saved: true, Paused: true}
	if action := core.BeltAvailabilityAction(core.BeltCommandPause, paused); action == nil || action.Enabled {
		t.Errorf("pause on a paused belt: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandResume, paused); action == nil || !action.Enabled {
		t.Errorf("resume on a paused belt: %+v", action)
	}

	warned := core.BeltActionContext{Saved: true, Paused: true, Warnings: []string{"Add at least one stage."}}
	if action := core.BeltAvailabilityAction(core.BeltCommandResume, warned); action == nil || action.Enabled {
		t.Errorf("resume on a warned belt: %+v", action)
	} else if action.Reason == nil || *action.Reason != core.BeltAvailabilityResumeBlocked {
		t.Errorf("resume reason = %v, want the blocked reason", action.Reason)
	}

	archived := core.BeltActionContext{Saved: true, Archived: true}
	if action := core.BeltAvailabilityAction(core.BeltCommandPause, archived); action == nil || action.Enabled {
		t.Errorf("pause on an archived belt: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandUnarchive, archived); action == nil || !action.Enabled {
		t.Errorf("unarchive on an archived belt: %+v", action)
	}
	if action := core.BeltAvailabilityAction(core.BeltCommandUpdateAllStages, archived); action == nil || action.Enabled {
		t.Errorf("update-all-stages on an archived belt: %+v", action)
	}

	fresh := core.BeltActionContext{Saved: true}
	if action := core.BeltAvailabilityAction(core.BeltCommandUpdateAllStages, fresh); action == nil || action.Enabled {
		t.Errorf("update-all-stages with nothing outdated: %+v", action)
	}
	if core.BeltAvailabilityRefusal(core.BeltCommandUpdateAllStages, fresh) == nil {
		t.Errorf("update-all-stages with nothing outdated is refused")
	}
	if core.BeltAvailabilityRefusal(core.BeltCommandUpdateAllStages, live) != nil {
		t.Errorf("update-all-stages with outdated stages is not refused")
	}
}

func TestRevisionIsStableForUnchangedReadsAndChangesWithState(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	record := beltRecordOf(t, c, "command")
	first := c.BeltResourceContext(record, false)
	second := c.BeltResourceContext(record, false)
	if first.Revision != second.Revision {
		t.Fatalf("unchanged reads have different revisions")
	}
	if !reflect.DeepEqual(first.Actions, second.Actions) {
		t.Fatalf("unchanged reads have different actions")
	}

	h.Turn(func() {
		if err := c.library.SetPaused("command", true); err != nil {
			t.Fatalf("pause: %v", err)
		}
	})
	paused := c.BeltResourceContext(beltRecordOf(t, c, "command"), false)
	if paused.Revision == first.Revision {
		t.Errorf("pausing did not change the revision")
	}
	if reflect.DeepEqual(paused.Actions, first.Actions) {
		t.Errorf("pausing did not change the actions")
	}

	h.Turn(func() {
		if err := c.library.SetPaused("command", false); err != nil {
			t.Fatalf("resume: %v", err)
		}
	})
	resumed := c.BeltResourceContext(beltRecordOf(t, c, "command"), false)
	if resumed.Revision != first.Revision {
		t.Errorf("resuming did not restore the revision")
	}
}

func TestRevisionChangesWhenANewAgentVersionMakesAStageOutdated(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	before := c.BeltResourceContext(beltRecordOf(t, c, "command"), false)
	if action := actionByID(t, actionsOf(t, c, "command"), string(core.BeltCommandUpdateAllStages)); action.Enabled {
		t.Fatalf("nothing is outdated yet")
	}

	agent := c.LatestAgent("triage").Agent
	agent.Model = "fixture-model"
	if _, err := c.SaveAgent(agent); err != nil {
		t.Fatalf("save agent: %v", err)
	}
	after := c.BeltResourceContext(beltRecordOf(t, c, "command"), false)
	if after.Revision == before.Revision {
		t.Errorf("a new agent version did not change the revision")
	}
	if action := actionByID(t, actionsOf(t, c, "command"), string(core.BeltCommandUpdateAllStages)); !action.Enabled {
		t.Errorf("update-all-stages is not enabled after a new agent version")
	}
}

func TestNewAndCopyKeepVersionZeroAndUnusedIds(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	draft := c.NewBeltDraft()
	if draft.Version != 0 {
		t.Errorf("new draft version = %d", draft.Version)
	}
	if c.Belt(draft.ID) != nil {
		t.Errorf("new draft %q is already saved", draft.ID)
	}
	if len(draft.Stages) != 0 {
		t.Errorf("new draft has %d stages", len(draft.Stages))
	}

	source := liveTestBelt("command", "Source", "acme/app")
	source.Stages[0].Brief = "unsaved"
	copied := c.CopyBeltDraft(source)
	if copied.Version != 0 {
		t.Errorf("copy version = %d", copied.Version)
	}
	if copied.ID == source.ID {
		t.Errorf("copy kept the id %q", copied.ID)
	}
	if copied.Name != "Copy of Source" {
		t.Errorf("copy name = %q", copied.Name)
	}
	if copied.Stages[0].Brief != "unsaved" || source.Stages[0].Brief != "unsaved" {
		t.Errorf("copy did not keep the unsaved brief")
	}
	if c.Belt(copied.ID) != nil {
		t.Errorf("copy %q is saved", copied.ID)
	}
}

func TestAnUnavailableCommandIsRefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	if _, err := c.ArchiveBeltCommand("command", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	statePath := filepath.Join(c.Paths().Belts(), "command", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if _, err := c.PauseBeltCommand("command", true); err == nil {
		t.Errorf("pausing an archived belt was accepted")
	}
	if _, err := c.ArchiveBeltCommand("command", true); err == nil {
		t.Errorf("archiving an archived belt was accepted")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("a refused command changed state.json")
	}
}

func TestRejectedAgentSavesCarryOrderedProblemsAndWriteNothing(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	agent := c.LatestAgent("triage").Agent
	version := agent.Version
	agent.Name = " "
	_, err := c.SaveAgentCommand(agent)
	failure, ok := err.(core.Failure)
	if !ok {
		t.Fatalf("save error = %#v, want core.Failure", err)
	}
	named := slices.ContainsFunc(failure.Problems, func(problem string) bool {
		return strings.Contains(strings.ToLower(problem), "name")
	})
	if !named {
		t.Errorf("problems %q do not name the field", failure.Problems)
	}
	if want := strings.Join(failure.Problems, " "); failure.Message != want {
		t.Errorf("message = %q, want the problems joined: %q", failure.Message, want)
	}
	if got := c.LatestAgent("triage").Agent.Version; got != version {
		t.Errorf("a rejected save changed the version to %d", got)
	}
}

func TestArchivedAgentsAreRefusedForNewStages(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	if _, err := c.ArchiveAgentCommand("review", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	review := c.LatestAgent("review").Agent
	ref := review.Ref()
	index := 0
	request := core.BeltDraftRequest{
		Draft: liveTestBelt("command", "Command belt", "acme/app"), Op: core.BeltDraftOperationStageAdd,
		Stage: &core.Stage{
			ID: review.ID, Type: core.StageTypeAgent, Name: review.Name, Agent: &ref, Key: &review.Key,
			Runner: &review.Runner, Model: review.Model, Thinking: review.Thinking, Workspace: review.Workspace,
			Slots: review.Slots, Outcomes: []core.Outcome{},
		},
		Index: &index,
	}
	if _, err := c.ApplyDraft(request); err == nil {
		t.Errorf("a stage using an archived agent was accepted")
	}
}

// stageNamed returns the stage of belt with id, failing the test when there is none.
func beltStageNamedForTest(belt core.Belt, id string) core.Stage {
	for _, stage := range belt.Stages {
		if stage.ID == id {
			return stage
		}
	}
	return core.Stage{}
}

// taskTestBelt is a belt with the given trigger entries and no stages.
func taskTestBelt(entries []core.TriggerEntry) core.Belt {
	return core.DefaultBelt("tasks", "Tasks", entries, nil)
}

// manualEntry is a manual trigger entry with repos.
func manualEntry(id string, repos ...string) core.TriggerEntry {
	return core.TriggerEntry{ID: id, Trigger: core.TriggerKindManual.Fresh(), Repos: repos}
}

func TestRetainsARepositoryOnlyWhileTheEntryHasIt(t *testing.T) {
	t.Parallel()
	belt := taskTestBelt([]core.TriggerEntry{manualEntry("2", "acme/two", "acme/three")})
	two, none := "2", "none"
	if got := core.TaskDraftRepo("acme/three", &two, belt); got != "acme/three" {
		t.Errorf("repo kept = %q", got)
	}
	if got := core.TaskDraftRepo("acme/other", &two, belt); got != "acme/two" {
		t.Errorf("repo replaced = %q, want acme/two", got)
	}
	if got := core.TaskDraftRepo("acme/two", &none, belt); got != "" {
		t.Errorf("repo for a missing entry = %q", got)
	}
}

func TestStartNeedsANonBlankTitleAndAValidRepository(t *testing.T) {
	t.Parallel()
	belt := taskTestBelt([]core.TriggerEntry{manualEntry("2", "acme/two")})
	entry := "2"
	selection := core.TaskDraftSelectionOf(belt, &entry, "acme/two", "Do it")
	if !selection.CanStart {
		t.Errorf("a titled task in the entry's repository cannot start")
	}
	selection = core.TaskDraftSelectionOf(belt, &entry, "acme/two", "   ")
	if selection.CanStart || selection.Problem == nil {
		t.Errorf("a blank title can start: %+v", selection)
	}
	selection = core.TaskDraftSelectionOf(belt, &entry, "acme/other", "Do it")
	if !selection.CanStart || selection.Repo != "acme/two" {
		t.Errorf("a repository the entry lacks = %+v", selection)
	}
	if core.TaskDraftCanStart("Do it", "acme/other", []string{"acme/two"}) {
		t.Errorf("TaskDraftCanStart accepts a repository the entry lacks")
	}
}
func TestNewAgentIsVersionZeroAndSaveMakesTheNextVersion(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("command", "Command belt", "acme/app"))
	c := h.Coord()
	draft := c.NewAgentDraft()
	if draft.Version != 0 {
		t.Errorf("new agent version = %d", draft.Version)
	}
	if draft.ID == "" {
		t.Errorf("new agent has no id")
	}
	if c.LatestAgent(draft.ID) != nil {
		t.Errorf("new agent %q is already saved", draft.ID)
	}

	agent := c.LatestAgent("triage").Agent
	agent.Brief = "changed"
	saved, err := c.SaveAgentCommand(agent)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Agent.Brief != "changed" || saved.Agent.Version <= agent.Version {
		t.Errorf("saved agent = %+v", saved.Agent)
	}
}
