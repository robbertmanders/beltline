// Tests of the demo's belts: the Claude stages, and the demo's watch over several repositories.

package demo

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestDemoIncludesClaudeStages: the tasks belt runs its agents on Claude.
func TestDemoIncludesClaudeStages(t *testing.T) {
	t.Parallel()
	var tasks core.Belt
	for _, entry := range Belts() {
		if entry.Belt.ID == "tasks" {
			tasks = entry.Belt
		}
	}
	if tasks.ID == "" {
		t.Fatal("no tasks belt")
	}
	for _, stage := range tasks.Stages {
		if stage.Kind() != core.StageKindAgent {
			continue
		}
		if stage.Runner == nil || *stage.Runner != core.BackendClaude {
			t.Errorf("stage %s runs on %v, want claude", stage.ID, stage.Runner)
		}
	}
}

// TestDemoSeedsAMultiRepositoryWatch: the Payments ticket PAY-34 waits on a
// watch with a pull request in each of its belt's repositories.
func TestDemoSeedsAMultiRepositoryWatch(t *testing.T) {
	t.Parallel()
	dir := sharedSeed(t)
	runs := store.NewStorage(store.Paths{Root: dir}).LoadRuns()
	var run *core.Run
	for i := range runs {
		if runs[i].Subject.Key == "PAY-34" {
			run = &runs[i]
		}
	}
	if run == nil {
		t.Fatal("no PAY-34 run")
	}
	stage := run.Stage()
	if stage == nil || stage.Kind() != core.StageKindWatch {
		t.Fatalf("PAY-34 is not at a watch: %+v", stage)
	}
	if want := []string{"acme/billing", "acme/infra"}; testkit.Diff(want, run.Repos) != "" {
		t.Errorf("repos differ: %s", testkit.Diff(want, run.Repos))
	}
	var pullRepos []string
	for _, pull := range run.PullRequests {
		pullRepos = append(pullRepos, pull.Repo)
	}
	if testkit.Diff(run.Repos, pullRepos) != "" {
		t.Errorf("pull request repos differ from the run's: %s", testkit.Diff(run.Repos, pullRepos))
	}
}

// TestTheDemoBeltsValidate checks that every demo belt has no validation warnings, with
// stage keys made explicit and checked against the shipped agents.
func TestTheDemoBeltsValidate(t *testing.T) {
	t.Parallel()
	agents := map[core.AgentRef]core.Agent{}
	for _, agent := range core.SampleAgents() {
		agents[agent.Ref()] = agent
	}
	for _, entry := range Belts() {
		converted := core.StageKeysExplicit(entry.Belt, agents)
		var keys []string
		for _, stage := range converted.Stages {
			if stage.Kind() == core.StageKindAgent {
				keys = append(keys, core.StageKeysChosen(stage))
			}
		}
		if warnings := core.ValidationWarnings(converted, keys, agents); len(warnings) != 0 {
			t.Errorf("%s: warnings %v, want none", entry.Belt.ID, warnings)
		}
	}
}
