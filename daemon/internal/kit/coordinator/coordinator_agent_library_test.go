// Coordinator tests of the agent library: versions, seeding, pins and archiving through the
// coordinator.

package coordinator

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

func TestSavedVersionOfAnUnknownIdIsRefusedAndWritesNothing(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("agent-library", "Agent library belt", "acme/app"))
	c := h.Coord()
	agent := c.LatestAgent("triage").Agent
	count := len(c.Agents())
	agent.ID = "ghost-agent"
	agent.Version = 2
	_, err := c.SaveAgentCommand(agent)
	failure, ok := err.(core.Failure)
	if !ok {
		t.Fatalf("save error = %#v, want core.Failure", err)
	}
	if !strings.Contains(failure.Message, "ghost-agent") {
		t.Errorf("message %q does not name the agent", failure.Message)
	}
	if len(failure.Problems) != 1 || failure.Problems[0] != failure.Message {
		t.Errorf("problems = %q, want the message alone", failure.Problems)
	}
	if c.LatestAgent("ghost-agent") != nil {
		t.Errorf("the unknown agent was saved")
	}
	if got := len(c.Agents()); got != count {
		t.Errorf("agents = %d, want %d", got, count)
	}
}

func TestANewAgentWithVersionZeroStillSaves(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("agent-library", "Agent library belt", "acme/app"))
	c := h.Coord()
	draft := c.NewAgentDraft()
	draft.Brief = "Write the thing."
	saved, err := c.SaveAgentCommand(draft)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Agent.Version != 1 {
		t.Errorf("saved version = %d, want 1", saved.Agent.Version)
	}
	if record := c.LatestAgent(draft.ID); record == nil || record.Agent.Version != 1 {
		t.Errorf("latest agent = %+v", record)
	}
}

func TestRunsAtOnceAreBoundedToSix(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t, liveTestBelt("agent-library", "Agent library belt", "acme/app"))
	c := h.Coord()
	agent := c.LatestAgent("triage").Agent
	agent.Slots = 7
	if !slices.Contains(core.ValidationAgentWarnings(agent, nil), "Allow at most 6 runs at once.") {
		t.Errorf("seven runs at once is not warned about")
	}
	if _, err := c.SaveAgentCommand(agent); err == nil {
		t.Errorf("saving seven runs at once was accepted")
	}

	agent.Slots = 6
	for _, warning := range core.ValidationAgentWarnings(agent, nil) {
		if strings.Contains(warning, "runs at once") {
			t.Errorf("six runs at once is warned about: %q", warning)
		}
	}
	agent.Slots = 0
	if !slices.Contains(core.ValidationAgentWarnings(agent, nil), "Allow at least one run at once.") {
		t.Errorf("zero runs at once is not warned about")
	}
}

func TestANewInstallHasNoAgentsAndSeedingAddsTheShippedVersions(t *testing.T) {
	t.Parallel()
	storage := unseededStorage(t)
	if got := coordinatorOn(t, storage).Agents(); len(got) != 0 {
		t.Fatalf("a new install has %d agents", len(got))
	}
	storage.SeedAgents()
	c := coordinatorOn(t, storage)
	var ids []string
	for _, record := range c.Agents() {
		ids = append(ids, record.Agent.ID)
		if record.Agent.Version != core.SampleAgentVersion(record.Agent.ID) || record.Archived {
			t.Errorf("agent %s is at version %d, archived %v", record.Agent.ID, record.Agent.Version, record.Archived)
		}
	}
	if want := slices.Sorted(slices.Values(core.SampleAgentIDs)); !reflect.DeepEqual(slices.Sorted(slices.Values(ids)), want) {
		t.Errorf("agent ids = %q, want %q", ids, want)
	}
	for _, id := range []string{"triage", "spec"} {
		record := c.LatestAgent(id)
		if record == nil || record.Agent.Version != 2 {
			t.Errorf("%s latest = %+v, want version 2", id, record)
			continue
		}
		if want := core.SampleAgentNamed(id, nil).Brief; record.Agent.Brief != want {
			t.Errorf("%s brief does not match the shipped one", id)
		}
	}
	file := filepath.Join(storage.Paths.Agents(), "verify", "v1.md")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read verify: %v", err)
	}
	if want := core.SampleAgentFile("verify"); want == nil || string(data) != *want {
		t.Errorf("the verify file does not match the bundled one")
	}
}

func TestSeedingUpgradesOldTemplatesWithoutChangingSavedVersions(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"triage", "spec"} {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			storage := unseededStorage(t)
			if _, ok := storage.Agent(id, 1); ok {
				t.Fatalf("version 1 exists before seeding")
			}
			if _, ok := storage.Agent(id, 2); ok {
				t.Fatalf("version 2 is read before seeding")
			}
			want := core.SampleAgent(id)
			old := core.SampleAgent(id)
			old.Brief = "# Title\n\n## Goal\nOld wording.\n\n## Open questions\n1. (needs info) Which page?"
			saved, err := storage.SaveNewAgentVersion(old)
			if err != nil || saved.Version != 1 {
				t.Fatalf("save old version: %+v, %v", saved, err)
			}
			file := filepath.Join(storage.Paths.Agents(), id, "v1.md")
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read v1: %v", err)
			}
			storage.SeedAgents()
			if got := coordinatorOn(t, storage).LatestAgent(id); got == nil || !got.Agent.SameContent(want) || got.Agent.Version != want.Version {
				t.Errorf("latest %s after seeding = %+v", id, got)
			}
			if got := storage.AgentVersions(id); !reflect.DeepEqual(got, []int{1, 2}) {
				t.Errorf("versions = %v, want [1 2]", got)
			}
			if data, err := os.ReadFile(file); err != nil || string(data) != string(original) {
				t.Errorf("seeding rewrote the saved v1")
			}
			if got, ok := storage.Agent(id, 1); !ok || !strings.Contains(got.Brief, "(needs info)") {
				t.Errorf("v1 lost its brief")
			}
			custom := core.SampleAgent(id)
			custom.Brief = "My custom brief."
			newer, err := storage.SaveNewAgentVersion(custom)
			if err != nil {
				t.Fatalf("save custom: %v", err)
			}
			storage.SeedAgents()
			if got, ok := storage.Agent(id, newer.Version); !ok || got.Brief != custom.Brief {
				t.Errorf("seeding replaced a newer user version")
			}
			if got := storage.AgentVersions(id); !reflect.DeepEqual(got, []int{1, 2, 3}) {
				t.Errorf("versions after the custom save = %v, want [1 2 3]", got)
			}
		})
	}
}

func TestOldBeltPinsOnlyMoveWhenTheBeltIsUpdated(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"triage", "spec"} {
		id := id
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			storage := unseededStorage(t)
			old := core.SampleAgent(id)
			old.Brief = "The old pinned brief."
			saved, err := storage.SaveNewAgentVersion(old)
			if err != nil {
				t.Fatalf("save old: %v", err)
			}
			belt := core.SampleBelt(nil, "agent", core.SampleAgents(), true)
			index := -1
			for i, stage := range belt.Stages {
				if stage.Agent != nil && stage.Agent.ID == id {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatalf("no %s stage", id)
			}
			ref := saved.Ref()
			belt.Stages[index].Agent = &ref
			if _, err := storage.SaveNewVersion(belt); err != nil {
				t.Fatalf("save belt: %v", err)
			}
			storage.SeedAgents()
			if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
				t.Fatalf("save settings: %v", err)
			}
			c := coordinatorOn(t, storage)
			var pinned core.Belt
			for _, record := range c.Belts() {
				if record.Belt.ID == belt.ID {
					pinned = record.Belt
				}
			}
			if pinned.Stages[index].Agent == nil || pinned.Stages[index].Agent.Version != 1 {
				t.Fatalf("pin = %+v, want version 1", pinned.Stages[index].Agent)
			}
			if got, ok := storage.AgentRef(*pinned.Stages[index].Agent); !ok || got.Brief != old.Brief {
				t.Errorf("the pinned version lost its brief")
			}
			if newer := c.NewerAgent(pinned.Stages[index]); newer == nil || newer.Version != 2 {
				t.Errorf("newer agent = %+v, want version 2", newer)
			}
			updated := c.UpdatingAllStages(pinned)
			if updated.Stages[index].Agent == nil || updated.Stages[index].Agent.Version != 2 {
				t.Errorf("updated pin = %+v, want version 2", updated.Stages[index].Agent)
			}
			if !reflect.DeepEqual(updated.Stages[len(updated.Stages)-1], pinned.Stages[len(pinned.Stages)-1]) {
				t.Errorf("an unrelated stage changed")
			}
			for _, record := range storage.LoadBelts() {
				if record.Belt.ID == belt.ID && record.Belt.Stages[index].Agent.Version != 1 {
					t.Errorf("the saved belt moved its pin")
				}
			}
		})
	}
}

func TestSavingAnAgentMakesANewVersionAndLeavesBeltsOnTheirs(t *testing.T) {
	t.Parallel()
	storage := seededLibraryStorage(t)
	c := coordinatorOn(t, storage)
	build := c.LatestAgent("build").Agent
	build.Brief = "Build it well."
	build.Thinking = new(core.ThinkingMedium)
	saved, err := c.SaveAgent(build)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Version != 2 {
		t.Errorf("saved version = %d, want 2", saved.Version)
	}
	if saved.Thinking == nil || *saved.Thinking != core.ThinkingMedium {
		t.Errorf("saved thinking = %v", saved.Thinking)
	}
	if got, ok := storage.Agent("build", 1); !ok || got.Thinking != nil {
		t.Errorf("version 1 changed")
	}
	if got := storage.AgentVersions("build"); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("versions = %v, want [1 2]", got)
	}
	if got, ok := storage.Agent("build", 1); !ok || !strings.HasPrefix(got.Brief, "Build the issue from its spec") {
		t.Errorf("version 1 brief changed")
	}
	if uses := c.BeltsUsing("build"); len(uses) != 1 || uses[0].Version != 1 {
		t.Errorf("belts using build = %+v, want one at version 1", uses)
	}
	again, err := c.SaveAgent(saved)
	if err != nil || again.Version != 2 {
		t.Errorf("saving unchanged = %+v, %v; want version 2", again, err)
	}
	restarted := coordinatorOn(t, storage)
	if got := restarted.LatestAgent("build"); got == nil || got.Agent.Brief != "Build it well." {
		t.Errorf("a restart lost the new version")
	} else if got.Agent.Thinking == nil || *got.Agent.Thinking != core.ThinkingMedium {
		t.Errorf("a restart lost the thinking level")
	}
}

func TestUpdatingABeltMovesOutdatedStagesToTheLatestVersion(t *testing.T) {
	t.Parallel()
	c := coordinatorOn(t, seededLibraryStorage(t))
	build := c.LatestAgent("build").Agent
	build.Slots = 3
	if _, err := c.SaveAgent(build); err != nil {
		t.Fatalf("save: %v", err)
	}
	belt := c.Belts()[0].Belt
	index := -1
	for i, stage := range belt.Stages {
		if stage.ID == "build" {
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("no build stage")
	}
	if newer := c.NewerAgent(belt.Stages[index]); newer == nil || newer.Version != 2 {
		t.Errorf("newer build = %+v, want version 2", newer)
	}
	if c.NewerAgent(belt.Stages[0]) != nil {
		t.Errorf("the first stage has a newer agent")
	}
	updated := c.UpdatingAllStages(belt)
	if updated.Stages[index].Agent == nil || updated.Stages[index].Agent.Version != 2 {
		t.Errorf("updated build pin = %+v", updated.Stages[index].Agent)
	}
	if updated.Stages[index].Slots != 3 {
		t.Errorf("updated slots = %d, want 3", updated.Stages[index].Slots)
	}
	if !reflect.DeepEqual(updated.Stages[0], belt.Stages[0]) {
		t.Errorf("the first stage changed")
	}
	if !reflect.DeepEqual(c.UpdatingAllStages(updated), updated) {
		t.Errorf("updating again changed the belt")
	}
}

func TestArchivedAgentsStayReadableAndSurviveARestart(t *testing.T) {
	t.Parallel()
	storage := seededLibraryStorage(t)
	c := coordinatorOn(t, storage)
	c.SetAgentArchived("review", true)
	if got := coordinatorOn(t, storage).LatestAgent("review"); got == nil || !got.Archived {
		t.Errorf("review is not archived after a restart")
	}
	if _, ok := storage.Agent("review", 1); !ok {
		t.Errorf("the archived agent lost its version")
	}
}
