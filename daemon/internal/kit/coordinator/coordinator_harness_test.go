// The coordinator test harness: a seeded folder cloned per test, and the Harness, Script and TestClock
// types that the coordinator tests drive. Helpers, in one line each:
// harnessFolder(t) a fresh copy of the seeded folder (agents, model fixtures, no belts or runs)
// modelFixtureSettings() the settings whose every runner allows the same fixture models
// newHarness(t, opts) a coordinator over a copy of the seeded folder, with fakes and a scripted runner
// h.Turn(func()) runs func as one main-actor turn (c.Lock, change, c.EndTurn)
// h.Tick() one c.Tick, then waits for spawned work (c.Idle)
// h.Until(what, done) Tick until done() holds, at most 60 times, else fail with what
// h.Settle() Tick the whole budget with no predicate
// h.Run() the first run, as a copy, or nil
// h.Restart(t) a second coordinator over the same folder (a restart)
// h.Changes() / h.ClearChanges the resource changes reported by each turn, in order
// h.Script.add(agent, outs...) the outcomes the simulated agent picks, in order
// h.Notifications() the notifications delivered so far
// h.NoticeTexts() the coordinator's notices, newest first
// The test binary builds the seeded folder once (testkit.Template) and copies it per test, so no test
// writes to another test's folder. Tests never sleep: they use the fake clock and c.Idle.

package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// standInCleanup removes the folder the tmux flows put the stand-in agent in. Only the integration
// build sets it; TestMain calls it after the tests.
var standInCleanup func()

// TestMain removes the seeded folders this binary built, and the stand-in agent's folder.
func TestMain(m *testing.M) {
	verifyProjectionMemos = true
	code := m.Run()
	testkit.Cleanup()
	if standInCleanup != nil {
		standInCleanup()
	}
	os.Exit(code)
}

// harnessFolderKey names the seeded template every harness folder is copied from.
const harnessFolderKey = "coordinator-harness"

// harnessFolder returns a fresh copy of the seeded data folder for one test. The seed has
// the built-in agents and the model fixtures, and no belts, runs or state.
func harnessFolder(t *testing.T) string {
	t.Helper()
	return testkit.Template(t, harnessFolderKey, func(dir string) error {
		storage := store.NewStorage(store.Paths{Root: dir})
		storage.SeedAgents()
		return storage.SaveSettings(modelFixtureSettings())
	})
}

// modelFixtureSettings are the explicit model fixtures: every runner
// allows the same fixture models and has a catalog that lists them.
func modelFixtureSettings() store.Settings {
	settings := store.DefaultSettings()
	identifiers := []string{
		"openrouter/stealth/space-bunny-alpha", "custom/model", "another", "old", "new", "newest",
		"adjusted", "test/model", "current/model", "gpt-test", "fast-model", "bigger", "fixture-model",
	}
	for _, backend := range core.AllBackend {
		allowed := map[string]struct{}{}
		choices := make([]core.ModelChoice, 0, len(identifiers))
		for _, id := range identifiers {
			allowed[id] = struct{}{}
			choices = append(choices, core.ModelChoice{ID: id, Name: id})
		}
		settings.AllowedModels[string(backend)] = allowed
		snapshot, err := core.NewModelCatalogSnapshot(backend, backend.Executable(func(string) string { return "" }), testkit.ContractTime, choices)
		if err != nil {
			panic(err)
		}
		settings.ModelCatalogs[string(backend)] = snapshot
	}
	settings.ConfigurationModel = "gpt-test"
	settings.Afk.Model = "fixture-model"
	return settings
}

// harnessScript holds the outcomes simulated agents pick, per agent, in order.
type harnessScript struct {
	mu    sync.Mutex
	queue map[string][]string
}

// add queues outcomes for the agent with id agent.
func (s *harnessScript) add(agent string, outcomes ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue[agent] = append(s.queue[agent], outcomes...)
}

// next returns the next outcome for the spec's agent, or nil when none is queued.
func (s *harnessScript) next(spec runner.SessionSpec) *string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.AgentID == nil {
		return nil
	}
	list := s.queue[*spec.AgentID]
	if len(list) == 0 {
		return nil
	}
	first := list[0]
	s.queue[*spec.AgentID] = list[1:]
	return &first
}

// harnessOptions are what a test changes about the default harness.
type harnessOptions struct {
	// Issues are the GitHub issues. Nil: one issue, acme/billing #142.
	Issues []core.Subject
	// Belts are saved before the coordinator loads. Empty: the default issues belt on acme/billing.
	Belts []core.Belt
	// Now is the fake clock's start. Zero: testkit.ContractTime.
	Now time.Time
	// OpensPulls is the state a pull request the fake opens starts in. Nil: merged.
	OpensPulls *core.PullRequestState
	// OpensNoPulls makes the fake open no pull request for a branch.
	OpensNoPulls bool
	// StageDuration and AfkDuration are how long simulated agent and AFK sessions run, in seconds.
	StageDuration float64
	AfkDuration   float64
	// Prepare runs on the storage before the coordinator loads it (for runs written in advance).
	Prepare func(storage store.Storage)
	// Runner replaces the simulated runner. It receives the harness's script.
	Runner func(script *harnessScript) runner.AgentRunner
	// Reviews are the review requests GitHub has. Nil: none.
	Reviews []github.FakeReviewRequest
	// Tickets are the Jira tickets. Nil: none.
	Tickets []core.Subject
	// LibraryURL gives a library repository's git remote, as demo mode's local remotes do. Nil: gh.
	LibraryURL func(repository string) string
	// WorkspacesFor builds the workspaces from the harness folder (real clones and worktrees, for the
	// integration flows). Nil: plain folders.
	WorkspacesFor func(root string) runner.Workspaces
}

// harness is a coordinator over its own copy of the seeded folder, with fakes for GitHub and
// Jira, a simulated runner, and a fake clock. Field Script queues the agents' outcomes.
type harness struct {
	t       *testing.T
	root    string
	storage store.Storage
	github  *github.FakeGitHub
	jira    *jira.FakeJira
	clock   *testkit.Clock
	Script  *harnessScript
	coord   *Coordinator
	// services is what the coordinator was built with, for Restart.
	services Services
	// notifications are the notifications delivered, in order.
	notifications []string

	mu      sync.Mutex
	changes [][]Resource
}

// newHarness builds a coordinator over a copy of the seeded folder. The coordinator is closed when the
// test ends.
func newHarness(t *testing.T, opts harnessOptions) *harness {
	t.Helper()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	now := opts.Now
	if now.IsZero() {
		now = testkit.ContractTime
	}
	belts := opts.Belts
	if len(belts) == 0 {
		belts = []core.Belt{core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)}
	}
	for _, belt := range belts {
		saved, err := storage.SaveNewVersion(belt)
		if err != nil {
			t.Fatalf("save belt: %v", err)
		}
		record := store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: now, WatchOutcomes: true, ReviewOutcomes: true})
		if err := storage.SaveState(record); err != nil {
			t.Fatalf("save belt state: %v", err)
		}
	}
	if opts.Prepare != nil {
		opts.Prepare(storage)
	}
	issues := opts.Issues
	if issues == nil {
		issues = []core.Subject{{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV",
			Body: "Add an export.", Labels: []string{"agent"}, IsOpen: true,
		}}
	}
	opensPulls := core.ValueOr(opts.OpensPulls, core.PullRequestStateMerged)
	var opened *core.PullRequestState
	if !opts.OpensNoPulls {
		opened = &opensPulls
	}
	h := &harness{
		t: t, root: root, storage: storage, clock: testkit.NewClock(now),
		github: github.NewFakeGitHub(issues, opts.Reviews, nil, opened),
		jira:   jira.NewFakeJira(opts.Tickets, nil, nil),
		Script: &harnessScript{queue: map[string][]string{}},
	}
	var sim runner.AgentRunner
	if opts.Runner != nil {
		sim = opts.Runner(h.Script)
	} else {
		sim = runner.NewSimulatedRunner(func(spec runner.SessionSpec) float64 {
			if spec.AgentID != nil && *spec.AgentID == "afk" {
				return opts.AfkDuration
			}
			return opts.StageDuration
		}, h.clock.Now, h.Script.next)
	}
	h.services = Services{
		Storage: storage, GitHub: h.github, Jira: h.jira, Runner: sim,
		Workspaces: runner.FolderWorkspaces{}, Clock: h.clock, IDs: &SequenceIDs{},
		LibraryURL: opts.LibraryURL,
		Notifier: func(title, body string) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.notifications = append(h.notifications, title+": "+body)
		},
	}
	if opts.WorkspacesFor != nil {
		h.services.Workspaces = opts.WorkspacesFor(root)
	}
	coord, err := New(h.services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.coord = coord
	coord.SetChangeListener(func(resources []Resource) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.changes = append(h.changes, append([]Resource(nil), resources...))
	})
	t.Cleanup(func() { coord.Close() })
	return h
}

// Coord is the harness's coordinator.
func (h *harness) Coord() *Coordinator { return h.coord }

// Turn runs change as one main-actor turn: the lock is held for it, and the changed resources are
// reported when it ends.
func (h *harness) Turn(change func()) {
	h.coord.Lock()
	defer h.coord.EndTurn()
	change()
}

// Tick runs one tick and waits for the work it spawned.
func (h *harness) Tick() {
	h.coord.Tick(context.Background())
	h.coord.Idle()
}

// Until ticks until done holds, at most 60 times. It never sleeps: spawned work is
// waited for with Idle. If done has not held after the 60th tick, the test fails with what, so a
// predicate that never holds cannot pass silently.
func (h *harness) Until(what string, done func() bool) {
	h.t.Helper()
	for range 60 {
		// Work a command kicked (a tick in a goroutine) finishes before each check, so
		// done never sees a half-done turn (an attempt recorded, its brief not yet written).
		h.coord.Idle()
		if h.evaluate(done) {
			return
		}
		h.Tick()
	}
	h.coord.Idle()
	if h.evaluate(done) {
		return
	}
	h.t.Fatalf("Until: not reached after 60 ticks: %s", what)
}

// untilHeld is Until for a test that holds the work its commands kick (holdKicks). It ticks the
// coordinator directly, so the held work does not run between the ticks, and it checks done before
// each tick, as until does. The kicked task runs only at the until's await; with the
// work held here, the ticks are the same ones counted.
func (h *harness) untilHeld(what string, done func() bool) {
	h.t.Helper()
	for range 60 {
		if h.evaluate(done) {
			return
		}
		h.coord.Tick(context.Background())
	}
	if h.evaluate(done) {
		return
	}
	h.t.Fatalf("untilHeld: not reached after 60 ticks: %s", what)
}

// finishStop lets the session stop that Abort started finish before the test's next command. A start
// waits for a pending stop, and untilHeld never runs held work, so without this the stop would never end.
// Idle also releases the kicked ticks, so the hold is taken again straight after.
func (h *harness) finishStop() {
	h.coord.Idle()
	h.coord.Hold()
}

// Settle ticks the whole 60-tick budget without a predicate. Use it where a
// check needs time to pass, such as a comment posted by a tick, or nothing starting while paused.
func (h *harness) Settle() {
	for range 60 {
		h.Tick()
	}
}

// evaluate reports done() under the lock, so it reads the coordinator the way a turn does.
func (h *harness) evaluate(done func() bool) bool {
	h.coord.Lock()
	defer h.coord.EndTurn()
	return done()
}

// Run returns the first run as a copy, or nil.
func (h *harness) Run() *core.Run {
	runs := h.coord.Runs()
	if len(runs) == 0 {
		return nil
	}
	return &runs[0]
}

// Restart builds a second coordinator over the same folder and fakes, as a restart does. It is
// closed when the test ends.
func (h *harness) Restart() *Coordinator {
	h.t.Helper()
	coord, err := New(h.services)
	if err != nil {
		h.t.Fatalf("restart: %v", err)
	}
	h.t.Cleanup(func() { coord.Close() })
	return coord
}

// Changes returns the resource changes reported so far, one slice per turn, in order.
func (h *harness) Changes() [][]Resource {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([][]Resource, len(h.changes))
	copy(out, h.changes)
	return out
}

// ClearChanges forgets the changes reported so far.
func (h *harness) ClearChanges() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.changes = nil
}

// Notifications returns the notifications delivered so far, as "title: body".
func (h *harness) Notifications() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.notifications...)
}

// NoticeTexts returns the texts of the coordinator's notices, newest first. A refusal
// or a note the coordinator keeps is a notice; a notification is only what the notifier delivers.
func (h *harness) NoticeTexts() []string {
	notices := h.coord.Notices()
	texts := make([]string, 0, len(notices))
	for _, notice := range notices {
		texts = append(texts, notice.Text)
	}
	return texts
}

// putRun puts run straight into the run book without saving it, for read tests.
// It bumps the book's revision as every run-book write must (projection memo).
func (h *harness) putRun(run core.Run) {
	h.Turn(func() {
		h.coord.book.appendRunForContract(run)
	})
}

// Helpers the coordinator tests use beyond the harness in coordinator_harness_test.go: an open issue,
// the run reads a tick's predicate may make without the lock, a restart with another runner, the
// runners the tests wrap, and the held-at-build run.

// harnessIssue is an open issue on acme/billing, as Subject(repo:number:title:labels:) makes one.
func harnessIssue(number int, title string, labels ...string) core.Subject {
	return core.Subject{
		Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number, Title: title,
		Labels: labels, IsOpen: true,
	}
}

// nowRuns returns the run book's runs without taking the lock. Only predicates of
// Until use it, since Until calls them with the lock held.
func (h *harness) nowRuns() []core.Run { return h.coord.book.runs }

// nowRun returns the first run without taking the lock, or nil.
func (h *harness) nowRun() *core.Run {
	if len(h.coord.book.runs) == 0 {
		return nil
	}
	run := h.coord.book.runs[0]
	return &run
}

// RestartWith builds a coordinator over the same folder and fakes with runner in place of the
// harness's runner, as the tests build `Coordinator(storage:…, runner: SimulatedRunner())`.
// It is closed when the test ends.
func (h *harness) RestartWith(sim runner.AgentRunner) *Coordinator {
	h.t.Helper()
	services := h.services
	services.Runner = sim
	coord, err := New(services)
	if err != nil {
		h.t.Fatalf("restart: %v", err)
	}
	h.t.Cleanup(func() { coord.Close() })
	return coord
}

// idleSimulatedRunner is a simulated runner that runs no session long and picks no outcome,
// as the default `SimulatedRunner()` does.
func (h *harness) idleSimulatedRunner() *runner.SimulatedRunner {
	return runner.NewSimulatedRunner(
		func(runner.SessionSpec) float64 { return 0 },
		h.clock.Now,
		func(runner.SessionSpec) *string { return nil },
	)
}

// costRunner is a simulated runner whose usage is only reported after the
// session finished, with the mark it started with.
type costRunner struct {
	*runner.SimulatedRunner
	reportsUsage bool
	mu           sync.Mutex
	finished     bool
	mark         int
}

// Start records the usage mark the session starts with.
func (r *costRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.mark = spec.UsageMark
	r.mu.Unlock()
	return r.SimulatedRunner.Start(ctx, spec)
}

// Status reports the session finished with exit code 0.
func (r *costRunner) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	zero := int32(0)
	return runner.SessionStatus{Kind: runner.SessionStatusFinished, ExitCode: &zero}
}

// Finish marks the session finished, so its usage can be read.
func (r *costRunner) Finish(context.Context, runner.SessionSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished = true
}

// UsageMark is the mark a fresh session starts at.
func (r *costRunner) UsageMark(context.Context, runner.SessionSpec) int { return 11_000_000_000 }

// Usage reports the usage once the session finished, with the mark it started with.
// Usage must be collected after finish and with the persisted mark.
func (r *costRunner) Usage(_ context.Context, spec runner.SessionSpec) *core.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.reportsUsage || !r.finished || spec.UsageMark != r.mark {
		return nil
	}
	cost, input, total := 230_000_000, 100, 100
	usage := core.NewUsage(core.UsageArgs{CostNanos: &cost, Input: &input, TotalTokens: &total})
	return &usage
}

// startGate holds a session's start until the test releases it (OverruleGate
// around setBeforeStart).
type startGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

// gatedRunner is a simulated runner whose sessions wait at the gate before they start.
type gatedRunner struct {
	*runner.SimulatedRunner
	gate *startGate
}

// Start enters the gate, waits for its release, then starts the session.
func (r gatedRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	r.gate.once.Do(func() { close(r.gate.entered) })
	<-r.gate.release
	return r.SimulatedRunner.Start(ctx, spec)
}

// rejectingEffort is a session that writes the diagnostic of an
// unsupported reasoning effort and exits with code 1.
type rejectingEffort struct {
	*runner.SimulatedRunner
	mu     sync.Mutex
	starts []runner.SessionSpec
}

// Start records the session and writes the diagnostic into its log.
func (r *rejectingEffort) Start(_ context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.starts = append(r.starts, spec)
	r.mu.Unlock()
	if err := os.MkdirAll(spec.AttemptFolder, 0o755); err != nil {
		return err
	}
	level := "none"
	if spec.Thinking != nil {
		level = string(*spec.Thinking)
	}
	diagnostic := "Reading the repository.\nERROR: unsupported model_reasoning_effort " + level
	return os.WriteFile(filepath.Join(spec.AttemptFolder, "session.log"), []byte(diagnostic), 0o644)
}

// Status reports the session finished with exit code 1.
func (r *rejectingEffort) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	one := int32(1)
	return runner.SessionStatus{Kind: runner.SessionStatusFinished, ExitCode: &one}
}

// Finish does nothing: the diagnostic stays in the log, as rejectingEffort leaves it.
func (r *rejectingEffort) Finish(context.Context, runner.SessionSpec) {}

// Starts returns the specs the session was started with, in order.
func (r *rejectingEffort) Starts() []runner.SessionSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runner.SessionSpec(nil), r.starts...)
}

// optionalTexts renders optional texts for comparison: a nil one reads "(nil)".
func optionalTexts(values []*string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == nil {
			out = append(out, "(nil)")
			continue
		}
		out = append(out, *value)
	}
	return out
}

// attemptOutcomes is each attempt's outcome, as optionalTexts renders it.
func attemptOutcomes(attempts []core.Attempt) []*string {
	out := make([]*string, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, attempt.Outcome)
	}
	return out
}

// Test seams for the model-catalog tests: a recording discovery fake and a harness coordinator built
// with changed services.

// fakeDiscovery is a runner.ModelCatalogDiscovery that answers with a function (
// ModelCatalogLifecycleTests' discovery).
type fakeDiscovery struct {
	answer func(ctx context.Context, backend core.Backend, executable string) ([]core.ModelChoice, error)
}

// Discover answers with the fake's function.
func (d fakeDiscovery) Discover(ctx context.Context, backend core.Backend, executable string) ([]core.ModelChoice, error) {
	return d.answer(ctx, backend, executable)
}

// failingDiscovery answers every discovery with an error, so nothing is ever saved.
func failingDiscovery() runner.ModelCatalogDiscovery {
	return fakeDiscovery{answer: func(context.Context, core.Backend, string) ([]core.ModelChoice, error) {
		return nil, core.NewFailure("discovery is unavailable")
	}}
}

// withServices builds a coordinator over the harness's folder, with the services change makes
// applied to the harness's own. The coordinator is closed when the test ends. The harness's own
// coordinator stays as the harness built it (its fakes are the defaults).
func (h *harness) withServices(t *testing.T, change func(services *Services)) *Coordinator {
	t.Helper()
	services := h.services
	change(&services)
	coord, err := New(services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { coord.Close() })
	return coord
}

// Helpers for the library and command tests. They extend the shared harness without changing it: a
// harness over the seeded folder with only the belts a test names (the shared harness saves a default
// belt when none is named), and a coordinator over a folder that has no agents (a first install).

// newLibraryHarness is newHarness over the seeded folder with exactly belts saved
// (none when no belt is named). Each belt gets its own state with both watch
// outcome flags set, as the shared harness saves them.
func newLibraryHarness(t *testing.T, belts ...core.Belt) *harness {
	t.Helper()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	now := testkit.ContractTime
	for _, belt := range belts {
		saved, err := storage.SaveNewVersion(belt)
		if err != nil {
			t.Fatalf("save belt: %v", err)
		}
		record := store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: now, WatchOutcomes: true, ReviewOutcomes: true})
		if err := storage.SaveState(record); err != nil {
			t.Fatalf("save belt state: %v", err)
		}
	}
	h := &harness{t: t, root: root, storage: storage, clock: testkit.NewClock(now), Script: &harnessScript{queue: map[string][]string{}}}
	h.github = github.NewFakeGitHub(nil, nil, nil, nil)
	h.jira = jira.NewFakeJira(nil, nil, nil)
	h.services = Services{
		Storage: storage, GitHub: h.github, Jira: h.jira,
		Runner:     runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, h.clock.Now, h.Script.next),
		Workspaces: runner.FolderWorkspaces{}, Clock: h.clock, IDs: &SequenceIDs{},
	}
	coord, err := New(h.services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.coord = coord
	t.Cleanup(func() { coord.Close() })
	return h
}

// coordinatorOn builds a coordinator over storage with fakes, as a restart does
// when called again on the same folder. It is closed when the test ends.
func coordinatorOn(t *testing.T, storage store.Storage) *Coordinator {
	t.Helper()
	clock := testkit.NewClock(testkit.ContractTime)
	coord, err := New(Services{
		Storage: storage, GitHub: github.NewFakeGitHub(nil, nil, nil, nil), Jira: jira.NewFakeJira(nil, nil, nil),
		Runner:     runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, clock.Now, func(runner.SessionSpec) *string { return nil }),
		Workspaces: runner.FolderWorkspaces{}, Clock: clock, IDs: &SequenceIDs{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { coord.Close() })
	return coord
}

// unseededStorage is an empty data folder: a first install, with nothing seeded.
func unseededStorage(t *testing.T) store.Storage {
	t.Helper()
	return store.NewStorage(store.Paths{Root: t.TempDir()})
}

// seededLibraryStorage is the library the app has before factories: the built-in
// agents and the default belt, which has no repositories.
func seededLibraryStorage(t *testing.T) store.Storage {
	t.Helper()
	storage := unseededStorage(t)
	storage.SeedAgents()
	if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	saved, err := storage.SaveNewVersion(core.SampleBelt(nil, "agent", core.SampleAgents(), true))
	if err != nil {
		t.Fatalf("save default belt: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: testkit.ContractTime, WatchOutcomes: true, ReviewOutcomes: true})); err != nil {
		t.Fatalf("save default belt state: %v", err)
	}
	return storage
}

// liveTestBelt is SampleBelt(repos:) with an id and a name.
func liveTestBelt(id, name string, repos ...string) core.Belt {
	belt := core.SampleBelt(repos, "agent", core.SampleAgents(), true)
	belt.ID = id
	belt.Name = name
	return belt
}

// Helpers the run tests share: a sample run and its finish helper, the main-actor calls the tests make
// on the coordinator's parts (taken here under c.mu), and the runner stubs.

// defaultBelt is SampleBelt(repos: ["acme/billing"]).
func defaultBelt() core.Belt {
	return core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
}

// coordSampleRun is sampleRun(belt): a started run of belt on acme/billing #142.
func coordSampleRun(belt core.Belt) core.Run {
	run := core.NewRun(core.RunArgs{
		Belt: belt,
		Subject: core.Subject{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142,
			Title: "Export invoices as CSV", Body: "Add an export button.",
		},
		Now: sessionSampleT0,
	})
	core.EngineStart(&run, sessionSampleT0)
	return run
}

// coordFinish starts the current agent stage's
// session and finishes it with outcome (nil: finishes without an outcome line).
func coordFinish(run *core.Run, outcome *string) {
	core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionStarted, Session: "bl-test", SessionID: "id", Folder: "f"}, sessionSampleT0)
	var output *string
	if outcome != nil {
		text := "Some output.\n\nOutcome: " + *outcome
		output = &text
	}
	core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionFinished, Output: output}, sessionSampleT0)
}

// coordSame reports whether two values are equal after a
// save and reload: instants compare by time, a nil collection equals an empty one (Set and Array
// have no nil state). reflect.DeepEqual would tell a reloaded time or nil map apart.
func coordSame(a, b any) bool {
	return cmp.Diff(a, b, cmp.Exporter(func(reflect.Type) bool { return true }), cmpopts.EquateEmpty()) == ""
}

// coordStart is c.sessions.start(id): the lock is taken for the call.
func coordStart(c *Coordinator, id string) {
	locked(c, func() { c.sessions.start(context.Background(), id, nil) })
}

// coordJoin is c.intake.join(record, subject).
func coordJoin(c *Coordinator, record store.BeltRecord, subject core.Subject) (core.Run, error) {
	var run core.Run
	var err error
	locked(c, func() { run, err = c.intake.join(record, subject, nil) })
	return run, err
}

// coordAdd is c.book.add(run).
func coordAdd(c *Coordinator, run core.Run) core.Run {
	var saved core.Run
	locked(c, func() { saved = c.book.add(run) })
	return saved
}

// coordUpdate is c.book.update(id) { … }.
func coordUpdate(c *Coordinator, id string, change func(run *core.Run)) {
	locked(c, func() { c.book.update(id, nil, nil, change) })
}

// newBoundaryRunner builds the stub over storage; its counters start at zero.
func newBoundaryRunner(h *harness, t *testing.T, reports bool) *boundaryRunner {
	return &boundaryRunner{SimulatedRunner: h.idleSimulatedRunner(), t: t, storage: h.storage, reports: reports, counters: map[string]int{}}
}

// startsRunner is a simulated runner that records each start
// and runs the session for an hour of the fake clock.
type startsRunner struct {
	*runner.SimulatedRunner
	mu     sync.Mutex
	starts []runner.SessionSpec
}

// newH5m2StartsRunner builds the stub on the harness's clock.
func newH5m2StartsRunner(h *harness) *startsRunner {
	return &startsRunner{SimulatedRunner: runner.NewSimulatedRunner(
		func(runner.SessionSpec) float64 { return 3600 }, h.clock.Now, nil)}
}

// Start records the spec and starts the simulated session.
func (r *startsRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.starts = append(r.starts, spec)
	r.mu.Unlock()
	return r.SimulatedRunner.Start(ctx, spec)
}

// Starts returns the specs the starts were given, in order.
func (r *startsRunner) Starts() []runner.SessionSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runner.SessionSpec(nil), r.starts...)
}

// admissionRunner counts the session ids it allocates
// and records the specs it is started with, and its sessions never run.
type admissionRunner struct {
	*runner.SimulatedRunner
	mu          sync.Mutex
	allocations int
	specs       []runner.SessionSpec
}

// newH5m2AdmissionRunner builds the stub with an idle simulated runner underneath.
func newH5m2AdmissionRunner(h *harness) *admissionRunner {
	return &admissionRunner{SimulatedRunner: h.idleSimulatedRunner()}
}

// PrepareSessionID counts the allocation and returns the proposed id.
func (r *admissionRunner) PrepareSessionID(_ context.Context, _ core.Backend, proposed string, _ string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.allocations++
	return proposed, nil
}

// Start records the spec; the session does not run.
func (r *admissionRunner) Start(_ context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs = append(r.specs, spec)
	return nil
}

// Status reports every session running.
func (r *admissionRunner) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	return runner.SessionStatus{Kind: runner.SessionStatusRunning}
}

// Finish does nothing: the stub's sessions never finish.
func (r *admissionRunner) Finish(context.Context, runner.SessionSpec) {}

// Allocations is the number of session ids allocated so far.
func (r *admissionRunner) Allocations() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.allocations
}

// Specs returns the specs the starts were given, in order.
func (r *admissionRunner) Specs() []runner.SessionSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runner.SessionSpec(nil), r.specs...)
}

// boundaryRunner is a simulated runner whose counters move with bump and set, its
// usage is the counter's movement since the spec's checkpoint, and it reports none when
// reports is false.
type boundaryRunner struct {
	*runner.SimulatedRunner
	t       *testing.T
	storage store.Storage
	reports bool
	mu      sync.Mutex
	// counters are the totals per session identity; known is the last started identity.
	counters map[string]int
	known    string
}

// Start records the identity the session counts under and checks the checkpoint it saved.
func (r *boundaryRunner) Start(_ context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.known = checkpointID(spec)
	r.mu.Unlock()
	var saved *core.Attempt
	var matched []core.Attempt
	for _, run := range r.storage.LoadRuns() {
		for index := range run.Attempts {
			attempt := run.Attempts[index]
			if attempt.SessionID != nil && spec.SessionID == *attempt.SessionID {
				saved = &run.Attempts[index]
				matched = append(matched, attempt)
			}
		}
	}
	if saved == nil {
		r.t.Errorf("the started session was not saved before its launch")
		return nil
	}
	if coordShow(totalOf(saved.UsageCheckpoint)) != coordShow(totalOf(spec.UsageCheckpoint)) {
		r.t.Errorf("the saved checkpoint differs from the launch's: saved %s (%d attempts matched), launch %s, session %s", coordShow(totalOf(saved.UsageCheckpoint)), len(matched), coordShow(totalOf(spec.UsageCheckpoint)), spec.SessionID)
	}
	return nil
}

// Status reports every session finished with exit code 0.
func (r *boundaryRunner) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	zero := int32(0)
	return runner.SessionStatus{Kind: runner.SessionStatusFinished, ExitCode: &zero}
}

// Finish does nothing.
func (r *boundaryRunner) Finish(context.Context, runner.SessionSpec) {}

// UsageCheckpoint reports the session's counter as its totals.
func (r *boundaryRunner) UsageCheckpoint(_ context.Context, spec runner.SessionSpec) *core.UsageCheckpoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := checkpointID(spec)
	total := r.counters[id]
	return &core.UsageCheckpoint{SessionID: &id, Totals: &core.TokenTotals{Input: total, Total: total}}
}

// Usage reports the counter's movement since the spec's checkpoint, or nil when the runner
// reports none or the counter went backwards.
func (r *boundaryRunner) Usage(_ context.Context, spec runner.SessionSpec) *core.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.reports || spec.UsageCheckpoint == nil || spec.UsageCheckpoint.Totals == nil {
		return nil
	}
	start := spec.UsageCheckpoint.Totals.Total
	end := r.counters[checkpointID(spec)]
	if end < start {
		return nil
	}
	cost := end - start
	return coordUsage(cost)
}

// bump adds by to the last started session's counter.
func (r *boundaryRunner) bump(by int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.known != "" {
		r.counters[r.known] += by
	}
}

// set sets a session identity's counter.
func (r *boundaryRunner) set(id string, value int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[id] = value
}

// checkpointID is the identity a session counts under: its checkpoint's, else its own.
func checkpointID(spec runner.SessionSpec) string {
	if spec.UsageCheckpoint != nil && spec.UsageCheckpoint.SessionID != nil {
		return *spec.UsageCheckpoint.SessionID
	}
	return spec.SessionID
}

// totalOf is the checkpoint's total, or nil.
func totalOf(checkpoint *core.UsageCheckpoint) *int {
	if checkpoint == nil || checkpoint.Totals == nil {
		return nil
	}
	total := checkpoint.Totals.Total
	return &total
}

// coordShow renders an optional count for a message.
func coordShow(n *int) string {
	if n == nil {
		return "(nil)"
	}
	return fmt.Sprint(*n)
}

// coordUsage is a usage whose cost and total tokens are both n.
func coordUsage(n int) *core.Usage {
	usage := core.NewUsage(core.UsageArgs{CostNanos: &n, TotalTokens: &n})
	return &usage
}
