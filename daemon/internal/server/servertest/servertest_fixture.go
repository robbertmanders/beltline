// The contract fixture: a private copy of the contract demo per test, with the seeding and the server
// it serves.

package servertest

import (
	"bufio"
	"context"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/kit/demo"
	"github.com/robbertmanders/beltline/daemon/internal/kit/launch"
	"github.com/robbertmanders/beltline/daemon/internal/kit/tools"
	"github.com/robbertmanders/beltline/daemon/internal/server"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// The fixture's constants. ContractToken is the bearer every contract request sends; ContractPort is
// the port the Host and Origin checks expect; ContractNow is the clock the goldens were recorded at.
const (
	ContractToken        = "test-token-0123456789abcdef"
	ContractVersion      = "dev"
	ContractPort         = 40123
	ContractArtifactName = "contract.html"
	ContractArtifactHTML = "<!doctype html><p>Contract artifact</p>\n"
	contractSigningKey   = "contract-artifact-signing-key-32-bytes"
	contractTemplateKey  = "servertest-contract-demo"
	contractStageOutput  = "Contract stage output"
)

// ContractNow is the contract's clock.
var ContractNow = testkit.ContractTime

// ContractSigningKey is the artifact signing key the contract fixture's server uses.
var ContractSigningKey = []byte(contractSigningKey)

// noEnvironment is the process environment the contract reads: none, so the agent executables are the defaults.
func noEnvironment(string) string { return "" }

// Options say what a contract fixture's copy of the demo changes.
type Options struct {
	// Empty starts over an empty data folder with default settings, as a first start has none
	// .
	Empty bool
	// Supplemental applies the fixture's supplemental change: the triage agent saved with high
	// thinking, and the library reloaded (withContractFixture(supplemental: true)).
	Supplemental bool
	// Settings, when set, changes the saved settings before the coordinator loads them. It takes what
	// the tests assign to coordinator.settings directly, which the settings patch validates.
	Settings func(settings *store.Settings)
	// Runner, when set, is the coordinator's agent runner in place of the simulated one
	// (withContractFixture(runner:)).
	Runner runner.AgentRunner
}

// Fixture is one contract coordinator, its router and the values the contract's requests name.
type Fixture struct {
	// Root is the data folder the coordinator reads and writes: a private copy per fixture.
	Root string
	// Coordinator is the coordinator the router serves.
	Coordinator *coordinator.Coordinator
	// Handler is the production router: Host and Origin checks, errors, credentials, every area.
	Handler http.Handler
	// Clock is the artifact links' clock, set by tests to move past an expiry.
	Clock *MutableClock
	// Commands records the desktop commands the routes run.
	Commands *shell.Recorder
	// DesktopApps and DesktopSessions answer whether an application or a tmux session is available.
	DesktopApps, DesktopSessions *MutableLookup
	// ActionRunID and ActionAttemptID name the run and agent attempt the artifact and desktop
	// requests use; their output and artifacts are written into the data folder.
	ActionRunID, ActionAttemptID string
	// VSCodeTargetID is the first worktree target of the action run; LinkTargetID its first link target.
	VSCodeTargetID, LinkTargetID string
	// GitHub, Jira and Runner are the fakes the coordinator runs against, so tests can assert the calls
	// the tests assert on coordinator.github, coordinator.jira and coordinator.runner. Runner is nil when
	// Options.Runner replaces the simulated runner.
	GitHub *github.FakeGitHub
	Jira   *jira.FakeJira
	Runner *runner.SimulatedRunner

	deps        routes.Dependencies
	webRoot     string
	viewsSeeded *sync.Once
}

// contractPulls is the contract demo's pull requests, read from the first copy: every
// copy of the template holds the same runs, and reading them is a third of a start.
var (
	contractPullsOnce sync.Once
	contractPulls     []demo.PullRequestHead
)

func contractPullHeads(root string) []demo.PullRequestHead {
	contractPullsOnce.Do(func() {
		contractPulls = demo.PullHeads(store.NewStorage(store.Paths{Root: root}).LoadRuns())
	})
	// Each fixture's fake GitHub owns its pulls: a deep copy (pointers and reviews).
	pulls := make([]demo.PullRequestHead, len(contractPulls))
	for i, head := range contractPulls {
		data, err := jsonx.Marshal(head.Pull)
		if err == nil {
			err = json.Unmarshal(data, &pulls[i].Pull)
		}
		if err != nil {
			panic("servertest: copy a contract pull: " + err.Error())
		}
		pulls[i].Head = head.Head
	}
	return pulls
}

// Start builds a contract fixture for one test: a private copy of the contract demo (the
// ContractTemplate, built once per test binary), the coordinator over it with the contract's fakes
// and clock, the seeded views state and the action run's files, and the production router.
// The coordinator is closed when the test ends.
func Start(t testing.TB, opts Options) *Fixture {
	t.Helper()
	return startFixture(t, opts, fixtureScope{tempDir: t.TempDir, cleanup: t.Cleanup})
}

// fixtureScope is where a fixture's folders live and when its coordinator closes: the test's own
// (Start), or the package's until TestMain ends (the shared fixtures of servertest_shared.go).
type fixtureScope struct {
	// tempDir is a new empty folder for the fixture's data or web root.
	tempDir func() string
	// cleanup runs close when the scope ends.
	cleanup func(close func())
}

// startFixture builds a contract fixture for opts in scope. See Start.
func startFixture(t testing.TB, opts Options, scope fixtureScope) *Fixture {
	t.Helper()
	f := &Fixture{Clock: NewMutableClock(ContractNow), Commands: &shell.Recorder{}, viewsSeeded: new(sync.Once),
		DesktopApps: NewMutableLookup(true), DesktopSessions: NewMutableLookup(true)}
	services := demo.Setup{}.Services
	var libraryURL func(string) string
	if opts.Empty {
		f.Root = scope.tempDir()
		if err := store.NewStorage(store.Paths{Root: f.Root}).SaveSettings(store.DefaultSettings()); err != nil {
			t.Fatalf("settings: %v", err)
		}
	} else {
		f.Root = testkit.Template(t, contractTemplateKey, buildContractDemo)
		setup := demo.OpenWith(f.Root, contractPullHeads(f.Root))
		services, libraryURL = setup.Services, setup.LibraryURL
	}
	storage := store.NewStorage(store.Paths{Root: f.Root})
	if opts.Settings != nil {
		settings := storage.LoadSettings()
		opts.Settings(&settings)
		if err := storage.SaveSettings(settings); err != nil {
			t.Fatalf("settings: %v", err)
		}
	}
	clock := testkit.NewClock(ContractNow)
	sim := runner.NewSimulatedRunner(nil, clock.Now,
		func(runner.SessionSpec) *string { return nil })
	agentRunner := runner.AgentRunner(sim)
	f.GitHub, f.Jira = services.GitHub(), services.Jira()
	if opts.Runner != nil {
		agentRunner = opts.Runner
	} else {
		f.Runner = sim
	}
	c, err := coordinator.New(coordinator.Services{
		Storage: storage, GitHub: f.GitHub, Jira: f.Jira, Runner: agentRunner,
		Workspaces: runner.FolderWorkspaces{}, Clock: clock, Env: noEnvironment,
		LibraryURL:     libraryURL,
		ModelDiscovery: runner.FixtureModelCatalog{},
		ToolCheck:      func(context.Context) []tools.Result { return tools.Fixtures() },
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}
	scope.cleanup(c.Close)
	c.Idle()
	f.Coordinator = c
	if !opts.Empty {
		c.SeedContractStart(ContractNow)
		if opts.Supplemental {
			if err := c.SeedContractSupplemental(); err != nil {
				t.Fatalf("supplemental: %v", err)
			}
		}
		f.writeActionFiles(t)
	}
	signer := routes.ArtifactLinkSigner{Key: ContractSigningKey, Clock: f.Clock.Now}
	deps := routes.Dependencies{
		DesktopCommandRunner: f.Commands,
		// The goldens and the desktop command expectations are macOS's on every system.
		Platform:          launch.PlatformMacOS,
		ApplicationLookup: f.DesktopApps.IsAllowed,
		SessionLookup:     func(_ context.Context, name string) bool { return f.DesktopSessions.IsAllowed(name) },
	}
	f.deps = deps
	f.webRoot = filepath.Join(scope.tempDir(), "missing-web")
	f.Handler = server.ServerApplicationRouter(ContractPort, ContractToken, ContractVersion, f.webRoot, c, signer, deps, nil)
	return f
}

// RouterWithSigningKey is a second router over the same coordinator and data, whose artifact links are
// signed with key: a server restarted with another key (the replacement server in the
// artifact tests).
func (f *Fixture) RouterWithSigningKey(key []byte) http.Handler {
	signer := routes.ArtifactLinkSigner{Key: key, Clock: f.Clock.Now}
	return server.ServerApplicationRouter(ContractPort, ContractToken, ContractVersion, f.webRoot, f.Coordinator, signer, f.deps, nil)
}

// DoRaw serves a request written as HTTP/1.1 text, parsed as net/http parses a request line and headers
// (for the targets a client library would rewrite). A repeated Host is refused by net/http's parser, so
// those requests are checked over a listener (server's integration tests), not here.
func (f *Fixture) DoRaw(t testing.TB, raw string) Reply {
	t.Helper()
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("raw request: %v", err)
	}
	return serve(f.Handler, req)
}

// buildContractDemo writes the contract demo into dir: Demo.make at the contract clock, then the
// the contract template's changes (SeedContractTemplate).
func buildContractDemo(dir string) error {
	if _, err := demo.Make(dir, ContractNow, noEnvironment); err != nil {
		return err
	}
	return coordinator.SeedContractTemplate(store.NewStorage(store.Paths{Root: dir}), ContractNow)
}

// writeActionFiles picks the action run and its agent attempt (the first run, by id, with a
// session on an agent stage and a link target) and writes the files the fixture writes into
// it: the artifact, the stage's output, and a worktree checkout for each repository. It records the
// ids of the run, the attempt and its first worktree and link targets.
func (f *Fixture) writeActionFiles(t testing.TB) {
	t.Helper()
	c := f.Coordinator
	runs := c.ReadRuns()
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	ambiguous := core.RepoLabelAmbiguousNames(runs)
	for _, run := range runs {
		if len(run.Repos) == 0 || len(core.RunLinksTargets(run.Subject, run.PullRequests, ambiguous)) == 0 {
			continue
		}
		attempt, stage, ok := agentSessionAttempt(run)
		if !ok {
			continue
		}
		paths := c.Paths()
		artifacts := paths.ArtifactsFolder(run, attempt)
		mustWrite(t, artifacts, ContractArtifactName, []byte(ContractArtifactHTML))
		mustWrite(t, paths.AttemptFolder(run, attempt), stage.Writes(), []byte(contractStageOutput))
		worktree := filepath.Join(paths.WorkFolder(run), "worktree")
		for _, repo := range run.Repos {
			checkout := runner.WorkspaceSpecCheckout(repo, run.Repos, worktree)
			if err := mkdirAll(checkout); err != nil {
				t.Fatalf("checkout: %v", err)
			}
		}
		targets := runner.WorktreeTargetsFor(run, paths.WorkFolder(run))
		links := core.RunLinksTargets(run.Subject, run.PullRequests, ambiguous)
		if len(targets) == 0 || len(links) == 0 {
			t.Fatalf("the action run %s has no worktree or link target", run.ID)
		}
		f.ActionRunID = run.ID
		f.ActionAttemptID = attempt.ID
		f.VSCodeTargetID = targets[0].ID
		f.LinkTargetID = links[0].ID
		return
	}
	t.Fatalf("the contract demo has no run with a session, a repository and a link")
}

// agentSessionAttempt is the first attempt of run with a session on one of its agent stages.
func agentSessionAttempt(run core.Run) (core.Attempt, core.Stage, bool) {
	for _, attempt := range run.Attempts {
		if attempt.Session == nil {
			continue
		}
		for _, stage := range run.Stages {
			if stage.ID == attempt.StageID && stage.Kind() == core.StageKindAgent {
				return attempt, stage, true
			}
		}
	}
	return core.Attempt{}, core.Stage{}, false
}

// mustWrite writes data to name inside dir, creating dir first.
func mustWrite(t testing.TB, dir, name string, data []byte) {
	t.Helper()
	if err := mkdirAll(dir); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// mkdirAll creates dir and its parents.
func mkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }

// WriteAttemptOutput writes text as the output file of the action run's agent attempt, the way the
// The run-read tests write an attempt's output before reading it. The run and attempt are the
// fixture's action ones.
func (f *Fixture) WriteAttemptOutput(t testing.TB, text string) {
	t.Helper()
	run := f.Coordinator.ReadRun(f.ActionRunID)
	if run == nil {
		t.Fatalf("no action run %s", f.ActionRunID)
	}
	for _, attempt := range run.Attempts {
		if attempt.ID != f.ActionAttemptID {
			continue
		}
		for _, stage := range run.Stages {
			if stage.ID == attempt.StageID {
				mustWrite(t, f.Coordinator.Paths().AttemptFolder(*run, attempt), stage.Writes(), []byte(text))
				return
			}
		}
	}
	t.Fatalf("the action attempt %s is not on run %s", f.ActionAttemptID, f.ActionRunID)
}

// SeedViews applies the contract's seedViews to the fixture: the waiting subjects and the
// AFK decisions the Overview, Work, waiting and AFK goldens read. The tests call it only where
// they read those goldens, so the other goldens are recorded without it (ContractFixture.seedViews).
//
// It seeds once per fixture: a fresh coordinator is seeded once, and a second
// seed would append the AFK decisions again. Later calls do nothing.
func (f *Fixture) SeedViews() {
	f.viewsSeeded.Do(func() { f.Coordinator.SeedContractViews(ContractNow) })
}
