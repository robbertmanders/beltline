// Tests of loading and saving the library without running a tick: belt modes, reloads that keep the
// saved versions, codex effort, pauses for a belt that needs them, pinned keys, and the agent list.

package coordinator

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

func TestSaveAndReloadKeepTheModeAndLeaveOlderVersionsUntouched(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	belt := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	belt.Afk = core.AfkModeAlways
	first, err := storage.SaveNewVersion(belt)
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: first, Created: upgradeTime, WatchOutcomes: true, ReviewOutcomes: true})); err != nil {
		t.Fatalf("save state: %v", err)
	}
	belt.Afk = core.AfkModeNever
	second, err := storage.SaveNewVersion(belt)
	if err != nil {
		t.Fatalf("save second: %v", err)
	}
	if got := storage.Belt(first.ID, first.Version); got == nil || got.Afk != core.AfkModeAlways {
		t.Errorf("version %d lost its AFK start", first.Version)
	}
	if got := storage.Belt(second.ID, second.Version); got == nil || got.Afk != core.AfkModeNever {
		t.Errorf("version %d lost its AFK start", second.Version)
	}
	reloaded := libraryOver(t, storage).Belt(first.ID)
	if reloaded == nil || reloaded.Belt.Afk != core.AfkModeNever || reloaded.Belt.Version != second.Version {
		t.Errorf("reloaded belt = %+v", reloaded)
	}
}

func TestLoadingAnOldBeltDoesNotRewriteItsSavedVersions(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	saved, err := storage.SaveNewVersion(core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: upgradeTime, WatchOutcomes: true, ReviewOutcomes: true})); err != nil {
		t.Fatalf("save state: %v", err)
	}
	// Rewrite the version file without its afk member, as a belt saved before AFK starts looked.
	url := filepath.Join(storage.Paths.Belts(), saved.ID, fmt.Sprintf("v%d.json", saved.Version))
	data, err := os.ReadFile(url)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	members, err := jsonx.Members(data)
	if err != nil {
		t.Fatalf("read members: %v", err)
	}
	var kept []string
	for _, key := range slices.Sorted(maps.Keys(members)) {
		if key != "afk" {
			kept = append(kept, fmt.Sprintf("%q:%s", key, members[key]))
		}
	}
	rewritten := []byte("{" + joinComma(kept) + "}")
	if err := os.WriteFile(url, rewritten, 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}
	before, err := os.ReadFile(url)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	library := libraryOver(t, storage)
	if after, err := os.ReadFile(url); err != nil || !bytes.Equal(after, before) {
		t.Errorf("loading the old belt rewrote its version")
	}
	if got := library.Belt(saved.ID); got == nil || got.Belt.Afk != core.AfkModeGlobal {
		t.Errorf("an old belt does not read as global")
	}
	if got := storage.Belt(saved.ID, saved.Version+1); got != nil {
		t.Errorf("loading wrote version %d", saved.Version+1)
	}
}

func TestSavesAndReloadsKeepCodexEffort(t *testing.T) {
	t.Parallel()
	for _, thinking := range []core.Thinking{core.ThinkingXHigh, core.ThinkingMax} {
		thinking := thinking
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			// Storage with the model fixtures and nothing seeded.
			storage := unseededStorage(t)
			if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
				t.Fatalf("save settings: %v", err)
			}
			agent := core.SampleAgent("custom")
			belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
				ID: "tasks", Name: "Tasks", Trigger: core.TriggerKindManual.Fresh(), Repos: []string{"acme/app"},
				Stages: []core.Stage{core.DefaultsStageForAgent(agent, nil, nil, nil, core.TriggerKindManual)},
			})
			if _, err := storage.SaveNewAgentVersion(agent); err != nil {
				t.Fatalf("save agent: %v", err)
			}
			if _, err := storage.SaveNewVersion(belt); err != nil {
				t.Fatalf("save belt: %v", err)
			}
			library := libraryOver(t, storage)
			agent.Runner = core.BackendCodex
			agent.Thinking = &thinking
			savedAgent, err := library.SaveAgent(agent)
			if err != nil {
				t.Fatalf("save agent: %v", err)
			}
			if savedAgent.Version != 2 || savedAgent.Thinking == nil || *savedAgent.Thinking != thinking || savedAgent.Runner != core.BackendCodex {
				t.Errorf("saved agent = %+v", savedAgent)
			}
			belt.Stages[0].Runner = new(core.BackendCodex)
			belt.Stages[0].Thinking = &thinking
			explicit, err := library.SaveBelt(belt)
			if err != nil {
				t.Fatalf("save belt: %v", err)
			}
			if explicit.Version != 2 || explicit.Stages[0].Runner == nil || *explicit.Stages[0].Runner != core.BackendCodex || explicit.Stages[0].Thinking == nil || *explicit.Stages[0].Thinking != thinking {
				t.Errorf("explicit belt = %+v", explicit.Stages[0])
			}
			inherited := explicit.Clone()
			inherited.Stages[0].Runner = nil
			inherited.Stages[0].Thinking = &thinking
			savedInherited, err := library.SaveBelt(inherited)
			if err != nil {
				t.Fatalf("save inherited: %v", err)
			}
			if savedInherited.Version != 3 || savedInherited.Stages[0].Runner != nil || savedInherited.Stages[0].Thinking == nil || *savedInherited.Stages[0].Thinking != thinking {
				t.Errorf("inherited belt = %+v", savedInherited.Stages[0])
			}
			reloaded := libraryOver(t, store.NewStorage(store.Paths{Root: storage.Paths.Root}))
			if got := reloaded.LatestAgent(agent.ID); got == nil || got.Agent.Thinking == nil || *got.Agent.Thinking != thinking || got.Agent.Runner != core.BackendCodex {
				t.Errorf("reloaded agent = %+v", got)
			}
		})
	}
}

func TestPinnedBriefKeysCompletionAndMissingVersions(t *testing.T) {
	t.Parallel()
	storage := seededNoBelts(t)
	library := libraryOver(t, storage)
	agent := core.SampleAgent("spec")
	agent.Key = "outline"
	agent.Brief = "Read {draft}."
	pinned, err := library.SaveAgent(agent)
	if err != nil {
		t.Fatalf("save agent: %v", err)
	}
	belt := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	belt.Stages[1].Agent = &core.AgentRef{ID: pinned.ID, Version: pinned.Version}
	belt.Stages[1].Key = nil
	belt.Stages[1].Brief = ""
	for index := range belt.Stages {
		if index != 1 && belt.Stages[index].Kind() == core.StageKindAgent {
			belt.Stages[index].Brief = "Read {triage}."
		}
	}
	if got := library.Warnings(belt); len(got) != 1 {
		t.Errorf("warnings = %q, want one", got)
	}
	agent = pinned
	agent.Key = "new-key"
	agent.Brief = "{another-typo}"
	if _, err := library.SaveAgent(agent); err != nil {
		t.Fatalf("save changed agent: %v", err)
	}
	keys := library.PlaceholderKeys(belt)
	if !slices.Contains(keys, "outline") || slices.Contains(keys, "new-key") {
		t.Errorf("placeholder keys = %q", keys)
	}
}

func TestArchivingTheOpenAgentMovesToTheFirstLiveOne(t *testing.T) {
	t.Parallel()
	c := coordinatorOn(t, seededNoBelts(t))
	open := c.Agents()[0].Agent.ID
	c.SetAgentArchived(open, true)
	if slices.Contains(choiceIDs(core.AgentChoices(c.Agents())), open) {
		t.Errorf("the archived agent is still live")
	}
	if !slices.ContainsFunc(c.Agents(), func(record core.AgentRecord) bool { return record.Agent.ID == open && record.Archived }) {
		t.Errorf("the library lost the archived agent")
	}
	if got := core.AgentChoices(allArchivedRecords()); len(got) != 0 {
		t.Errorf("an all-archived library has live agents")
	}
}

// joinComma joins parts with commas.
func joinComma(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ","
		}
		out += part
	}
	return out
}

// choiceIDs are the ids of the choices, in order.
func choiceIDs(choices []core.AgentChoice) []string {
	ids := make([]string, 0, len(choices))
	for _, choice := range choices {
		ids = append(ids, choice.ID)
	}
	return ids
}

// allArchivedRecords is every built-in agent archived.
func allArchivedRecords() []core.AgentRecord {
	var out []core.AgentRecord
	for _, id := range core.SampleAgentIDs {
		out = append(out, core.AgentRecord{Agent: core.SampleAgent(id), Archived: true})
	}
	return out
}
