package store

import (
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestNewInstallHasNoAgentsAndSeedingAddsTheShippedVersions checks that a new install has no agents
// and that seeding adds the shipped versions.
func TestNewInstallHasNoAgentsAndSeedingAddsTheShippedVersions(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	if got := s.LoadAgents(); len(got) != 0 {
		t.Fatalf("a new install has %d agents", len(got))
	}
	s.SeedAgents()
	agents := s.LoadAgents()
	var ids []string
	for _, record := range agents {
		ids = append(ids, record.Agent.ID)
		if record.Agent.Version != core.SampleAgentVersion(record.Agent.ID) || record.Archived {
			t.Fatalf("%s at version %d, archived %v", record.Agent.ID, record.Agent.Version, record.Archived)
		}
	}
	if len(ids) != len(core.SampleAgentIDs) {
		t.Fatalf("agents %v, want %v", ids, core.SampleAgentIDs)
	}
	for i := 1; i < len(agents); i++ {
		if core.LocalizedStandardCompare(agents[i-1].Agent.Name, agents[i].Agent.Name) > 0 {
			t.Fatalf("agents are not in name order: %q before %q", agents[i-1].Agent.Name, agents[i].Agent.Name)
		}
	}
	for _, id := range []string{"triage", "spec"} {
		agent, ok := s.Agent(id, 2)
		builtIn := core.SampleAgentNamed(id, nil)
		if !ok || builtIn == nil || agent.Brief != builtIn.Brief {
			t.Fatalf("%s v2 is not the shipped one", id)
		}
	}
	file, _ := core.SampleAgentText("verify")
	if fileText(t, s.agentFolder("verify")+"/v1.md") != file {
		t.Fatal("the seeded file is not the bundled text")
	}
}

// TestSeedingUpgradesOldTemplatesWithoutChangingSavedVersions checks that seeding upgrades old templates
// without changing saved versions.
func TestSeedingUpgradesOldTemplatesWithoutChangingSavedVersions(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"triage", "spec"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			s := newTestStorage(t)
			if _, ok := s.Agent(id, 1); ok {
				t.Fatal("version 1 exists before any save")
			}
			if _, ok := s.Agent(id, 2); ok {
				t.Fatal("the sample version is read before it is saved")
			}
			shipped := core.SampleAgent(id)
			old := shipped
			old.Brief = "# Title\n\n## Goal\nOld wording.\n\n## Open questions\n1. (needs info) Which page?"
			saved, err := s.SaveNewAgentVersion(old)
			if err != nil || saved.Version != 1 {
				t.Fatalf("save old: %d, %v", saved.Version, err)
			}
			file := s.agentFolder(id) + "/v1.md"
			original := fileText(t, file)
			s.SeedAgents()
			if got := s.AgentVersions(id); len(got) != 2 || got[0] != 1 || got[1] != 2 {
				t.Fatalf("versions %v", got)
			}
			if fileText(t, file) != original {
				t.Fatal("seeding changed the saved version 1")
			}
			if v1, _ := s.Agent(id, 1); !strings.Contains(v1.Brief, "(needs info)") {
				t.Fatal("version 1 lost its wording")
			}
			custom := shipped
			custom.Brief = "My custom brief."
			newer, err := s.SaveNewAgentVersion(custom)
			if err != nil || newer.Version != 3 {
				t.Fatalf("save custom: %d, %v", newer.Version, err)
			}
			s.SeedAgents()
			if got, _ := s.Agent(id, newer.Version); got.Brief != custom.Brief {
				t.Fatal("seeding replaced a newer user version")
			}
			if got := s.AgentVersions(id); len(got) != 3 {
				t.Fatalf("versions after the custom save %v", got)
			}
		})
	}
}

// TestSavingAnAgentMakesANewVersionAndKeepsTheOldOne checks that saving an agent makes a new version
// and leaves belts on their pinned versions.
func TestSavingAnAgentMakesANewVersionAndKeepsTheOldOne(t *testing.T) {
	t.Parallel()
	s := seededStorage(t)
	build, ok := s.Agent("build", 1)
	if !ok {
		t.Fatal("build v1 missing")
	}
	build.Brief = "Build it well."
	thinking := core.ThinkingMedium
	build.Thinking = &thinking
	saved, err := s.SaveNewAgentVersion(build)
	if err != nil || saved.Version != 2 {
		t.Fatalf("save: %d, %v", saved.Version, err)
	}
	if v1, _ := s.Agent("build", 1); v1.Thinking != nil || !strings.HasPrefix(v1.Brief, "Build the issue from its spec") {
		t.Fatalf("version 1 changed: %+v", v1)
	}
	if got := s.AgentVersions("build"); len(got) != 2 {
		t.Fatalf("versions %v", got)
	}
	if v2, _ := s.Agent("build", 2); v2.Thinking == nil || *v2.Thinking != core.ThinkingMedium {
		t.Fatalf("thinking not saved: %+v", v2.Thinking)
	}
	s.SeedAgents()
	if v2, _ := s.Agent("build", 2); v2.Brief != "Build it well." {
		t.Fatal("seeding overwrote the saved version")
	}
}

// TestArchivedAgentsStayReadableAndSurviveARestart checks an archived agent
// is listed as archived after a reload, and its versions still read.
func TestArchivedAgentsStayReadableAndSurviveARestart(t *testing.T) {
	t.Parallel()
	s := seededStorage(t)
	if err := s.SetAgentArchived("review", true); err != nil {
		t.Fatal(err)
	}
	reopened := NewStorage(s.Paths)
	for _, record := range reopened.LoadAgents() {
		if record.Agent.ID == "review" && !record.Archived {
			t.Fatal("review lost its archive after a restart")
		}
	}
	if _, ok := reopened.Agent("review", 1); !ok {
		t.Fatal("an archived agent's version does not read")
	}
}

// TestAgentRefReadsTheNamedVersion checks a stage pin reads the version it names.
func TestAgentRefReadsTheNamedVersion(t *testing.T) {
	t.Parallel()
	s := seededStorage(t)
	got, ok := s.AgentRef(core.AgentRef{ID: "triage", Version: 2})
	if !ok || got.Version != 2 {
		t.Fatalf("pin v2: %+v, %v", got, ok)
	}
	if _, ok := s.AgentRef(core.AgentRef{ID: "triage", Version: 9}); ok {
		t.Fatal("a version that does not exist was served")
	}
	if diff := testkit.Diff(got, mustAgent(t, s, "triage", 2)); diff != "" {
		t.Fatalf("pins differ:\n%s", diff)
	}
}

// mustAgent reads an agent version, failing the test when it is missing.
func mustAgent(t *testing.T, s Storage, id string, version int) core.Agent {
	t.Helper()
	agent, ok := s.Agent(id, version)
	if !ok {
		t.Fatalf("%s v%d missing", id, version)
	}
	return agent
}
