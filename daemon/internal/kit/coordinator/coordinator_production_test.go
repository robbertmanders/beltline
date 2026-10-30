package coordinator

// Tests for the production review fixes. Each one fails on the code as it was
// before its fix.

import (
	"context"
	"sync"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// An assistant change calls library.reload(), which migrates (migrating: true) and
// upgrades belts saved without the watch flags.
func TestAssistantChangeMigratesLibrary(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Turn(func() {
		rec := c.library.belts[0]
		rec.WatchOutcomes = false
		rec.ReviewOutcomes = false
		if err := c.storage.SaveState(rec); err != nil {
			t.Fatal(err)
		}
		c.library.reconcileFiles()
	})
	h.Turn(func() {
		rec := c.library.belts[0]
		if !rec.WatchOutcomes || !rec.ReviewOutcomes {
			t.Fatalf("after an assistant change the belt has WatchOutcomes=%v ReviewOutcomes=%v; reload() migrates it to true/true",
				rec.WatchOutcomes, rec.ReviewOutcomes)
		}
	})
}

// save(agent:) returns a value: the library keeps its own copy, so an edit of the agent a
// caller saved, or of the agent it got back, does not reach the library.
func TestSaveAgentDoesNotShareStateWithTheCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	var agent core.Agent
	var stored func() core.Agent
	h.Turn(func() {
		agent = c.library.agents[0].Agent
		agent.Brief = agent.Brief + " edited"
		if len(agent.Outcomes) == 0 {
			t.Fatal("the built-in agent has no outcomes to share")
		}
		agent.Outcomes = append([]core.AgentOutcome{}, agent.Outcomes...)
		saved, err := c.library.SaveAgent(agent)
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		id := agent.ID
		stored = func() core.Agent { return c.library.agents[c.library.agentIndex(id)].Agent }
		saved.Outcomes[0].Name = "MUTATED BY RESULT"
	})
	h.Turn(func() {
		if got := stored().Outcomes[0].Name; got == "MUTATED BY RESULT" {
			t.Fatal("the library's agent follows the edit of the agent SaveAgent returned")
		}
		before := stored().Outcomes[0].Name
		agent.Outcomes[0].Name = "MUTATED BY CALLER"
		if after := stored().Outcomes[0].Name; after != before {
			t.Fatalf("the library's stored agent follows the caller's later edit: %q -> %q", before, after)
		}
	})
}

// The configuration check reads the agent executables from the environment the coordinator
// was given, and a model's warning depends on the executable its catalog was made for.
func TestValidateConfigurationReadsTheCoordinatorsEnvironment(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	choices := []core.ModelChoice{{ID: "fixture-model", Name: "fixture-model"}}
	snapshot, err := core.NewModelCatalogSnapshot(core.BackendClaude, "/fake/claude", testkit.ContractTime, choices)
	if err != nil {
		t.Fatal(err)
	}
	h.Turn(func() {
		c.changeSettings(func(s *store.Settings) { s.ModelCatalogs[string(core.BackendClaude)] = snapshot })
		configuration := AssistantConfiguration{Backend: core.BackendClaude, Model: "fixture-model"}
		c.env = func(key string) string {
			if key == "BELTLINE_CLAUDE" {
				return "/fake/claude"
			}
			return ""
		}
		if err := c.validateConfiguration(configuration, c.settings, "Assistant"); err != nil {
			t.Fatalf("the coordinator's environment names the executable the catalog was made for, but: %v", err)
		}
		c.env = func(string) string { return "" }
		if err := c.validateConfiguration(configuration, c.settings, "Assistant"); err == nil {
			t.Fatal("with the default executable the catalog is unusable, so the model should be refused")
		}
	})
}

// The next task number is one past the highest existing number, and
// the highest starts from the first number, so negative keys count.
func TestStartTaskNumbersFromTheFirstNumber(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{manualBelt("tasks")}})
	c := h.Coord()
	belt := c.Belts()[0].Belt
	h.Turn(func() {
		for _, key := range []string{"-5", "-9"} {
			c.book.add(core.NewRun(core.RunArgs{
				Belt: belt, Subject: core.Subject{Kind: core.SubjectKindManual, Repo: "acme/web", Key: key, Title: "Old " + key},
				Now: testkitNow(),
			}))
		}
	})
	run, err := c.StartTask("tasks", "acme/web", "New", "", nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.Subject.Key != "-4" {
		t.Fatalf("key = %q, want -4 (one past the highest, -5)", run.Subject.Key)
	}
}

// The runner check accepts the value and the pointer form of the TmuxRunner.
func TestTmuxRunnerCheckAcceptsValueAndPointer(t *testing.T) {
	t.Parallel()
	tmux := runner.NewTmuxRunner(nil)
	var nilPointer *runner.TmuxRunner
	cases := map[string]struct {
		value runner.AgentRunner
		want  bool
	}{
		"value":       {tmux, true},
		"pointer":     {&tmux, true},
		"nil pointer": {nilPointer, false},
		"simulated":   {runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, nil, nil), false},
	}
	for name, tc := range cases {
		if _, got := tmuxRunnerOf(tc.value); got != tc.want {
			t.Errorf("%s: tmuxRunnerOf = %v, want %v", name, got, tc.want)
		}
	}
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Turn(func() {
		c.runner = &tmux
		if !c.isTmuxRunner() {
			t.Fatal("a pointer to the tmux runner is not tmux-backed")
		}
	})
}

// Exported Projection takes the lock: a read from outside a turn never runs alongside a turn's
// writes. Run with -race, a read without the lock is reported.
func TestProjectionTakesTheLockForCallersOutsideATurn(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			_ = c.Projection(ResourceState)
		}
	}()
	for range 50 {
		h.Turn(func() { c.setGloballyPausedFlag(!c.globallyPaused) })
	}
	wg.Wait()
}

// ReviewsSource.login is written on the main actor by beginCheck and read by the
// listing. A check can overlap another (the tick and a check now both poll), and the listing
// reads the login while its GitHub call runs without the lock, so the field is guarded. Run with -race.
func TestReviewsLoginSurvivesOverlappingChecks(t *testing.T) {
	t.Parallel()
	source := newReviewsSource(github.NewFakeGitHub(nil, nil, nil, nil))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if err := source.beginCheck(context.Background()); err != nil {
					t.Errorf("begin check: %v", err)
					return
				}
				// The login is cleared while another check is in its GitHub call, so
				// myLogin may fail here; only the race detector's verdict matters.
				_ = source.adoptingIdentity(core.Subject{Kind: core.SubjectKindPullRequest})
				_, _ = source.myLogin()
			}
		}()
	}
	wg.Wait()
}
