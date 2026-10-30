//go:build integration

// Coordinator flows over real tmux sessions, git clones and worktrees, with the stand-in agent in
// daemon/testdata/fixtures instead of each agent CLI. Each test has its own private tmux server (see
// tmuxSetup), so the flows run in parallel; each test kills its server and removes its socket file
// when it ends.

package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// coordStandIn is the stand-in agent's shell, once per test process: the folder the stand-in is linked into
// under each backend's executable name, and the shell the tmux servers run their panes in. The runner runs
// each backend's executable by name, and a pane's login shell resets PATH, so the shell puts that folder
// first on PATH itself. Only the tmux servers of these tests see it: no process environment changes, so the
// other tests of the package run as they always did.
var (
	standInOnce sync.Once
	standInErr  error
	standInSh   string
	standInDir  string
)

// socketSeq numbers the tests' tmux servers within the process.
var socketSeq atomic.Int64

// standInInstall links the stand-in agent under each backend's executable name in a folder, and writes
// the pane shell that puts the folder first on PATH and runs the command as zsh -c would. The folder is
// removed when the test binary ends (standInCleanup).
func standInInstall() {
	stand, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "fixtures", "stand-in-agent.sh"))
	if err != nil {
		standInErr = fmt.Errorf("stand-in agent: %w", err)
		return
	}
	if _, err := os.Stat(stand); err != nil {
		standInErr = fmt.Errorf("stand-in agent: %w", err)
		return
	}
	dir, err := os.MkdirTemp("", "beltline-stand-in-")
	if err != nil {
		standInErr = fmt.Errorf("stand-in folder: %w", err)
		return
	}
	standInCleanup = func() { _ = os.RemoveAll(dir) }
	standInDir = dir
	for _, backend := range core.AllBackend {
		if err := os.Symlink(stand, filepath.Join(dir, backend.Executable(func(string) string { return "" }))); err != nil {
			standInErr = fmt.Errorf("link stand-in as %s: %w", backend, err)
			return
		}
	}
	standInSh = filepath.Join(dir, "pane-shell")
	script := "#!/bin/sh\nPATH=\"" + dir + ":$PATH\"\nexport PATH\nif [ \"$1\" = \"-c\" ]; then shift; exec /bin/zsh -c \"$1\"; fi\nexec /bin/zsh \"$@\"\n"
	if err := os.WriteFile(standInSh, []byte(script), 0o755); err != nil {
		standInErr = fmt.Errorf("pane shell: %w", err)
	}
}

// coordTmux is one test's private tmux server: the socket it listens on, which the test's runner uses too.
type coordTmux struct {
	socket string
}

// tmuxSetup prepares the stand-in agent and gives one test its own private tmux server. The server is
// killed, and its socket file removed, when the test ends.
func tmuxSetup(t *testing.T) coordTmux {
	t.Helper()
	standInOnce.Do(standInInstall)
	if standInErr != nil {
		t.Fatal(standInErr)
	}
	tm := coordTmux{socket: fmt.Sprintf("beltline-test-%d-coordinator-%d", os.Getpid(), socketSeq.Add(1))}
	// The server starts with no sessions and stays up (exit-empty off) for the runner's sessions; its panes
	// run in the stand-in shell.
	if result := tm.tmux("-f", "/dev/null", "start-server", ";", "set-option", "-g", "exit-empty", "off", ";",
		"set-option", "-g", "default-shell", standInSh); !result.OK() {
		t.Fatalf("start the test's tmux server: %s", result.Stderr)
	}
	t.Cleanup(func() {
		_ = coordShell().Run(context.Background(), "tmux", []string{"-L", tm.socket, "kill-server"}, "", nil)
		dir := os.Getenv("TMUX_TMPDIR")
		if dir == "" {
			dir = "/tmp"
		}
		_ = os.Remove(fmt.Sprintf("%s/tmux-%d/%s", dir, os.Getuid(), tm.socket))
	})
	return tm
}

// runner is the tmux runner on the test's server. The commands it runs outside a pane (Cursor's
// create-chat) reach the stand-in too, never an agent CLI installed on this machine.
func (tm coordTmux) runner() runner.TmuxRunner {
	return runner.NewTmuxRunnerOnSocket(standInRunner{}, tm.socket)
}

// standInRunner is coordShell with each backend's executable name taken from the stand-in folder.
type standInRunner struct{}

// Run runs executable, the stand-in when it names an agent CLI.
func (standInRunner) Run(ctx context.Context, executable string, args []string, dir string, input *string) shell.Result {
	for _, backend := range core.AllBackend {
		if executable == backend.Executable(func(string) string { return "" }) {
			executable = filepath.Join(standInDir, executable)
			break
		}
	}
	return coordShell().Run(ctx, executable, args, dir, input)
}

// tmux runs tmux on the test's server with arguments and returns the result.
func (tm coordTmux) tmux(arguments ...string) shell.Result {
	return coordShell().Run(context.Background(), "tmux", append([]string{"-L", tm.socket}, arguments...), "", nil)
}

// has reports whether the test's server has the session.
func (tm coordTmux) has(session string) bool {
	return tm.tmux("has-session", "-t", session).OK()
}

// tmuxSettings are the fixture settings with each catalog stamped with the executable the environment
// names, since the stand-in agent replaces every backend's executable (modelFixtureSettings stamps the default).
func tmuxSettings() store.Settings {
	settings := modelFixtureSettings()
	for _, backend := range core.AllBackend {
		snapshot, err := core.NewModelCatalogSnapshot(backend, backend.Executable(os.Getenv), testkit.ContractTime, settingsChoices(settings, backend))
		if err != nil {
			panic(err)
		}
		settings.ModelCatalogs[string(backend)] = snapshot
	}
	return settings
}

// settingsChoices lists the models a catalog of the settings holds for backend, sorted by id.
func settingsChoices(settings store.Settings, backend core.Backend) []core.ModelChoice {
	var choices []core.ModelChoice
	for id := range settings.AllowedModels[string(backend)] {
		choices = append(choices, core.ModelChoice{ID: id, Name: id})
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].ID < choices[j].ID })
	return choices
}

// coordShell is the process runner the flows use for git, tmux and the test's own commands.
func coordShell() shell.ProcessRunner { return shell.ProcessRunner{} }

// coordRun runs command through the shell in dir and returns the result.
func coordRun(dir, command string) shell.Result {
	return coordShell().Run(context.Background(), "/bin/zsh", []string{"-c", command}, dir, nil)
}

// coordOrigin makes a bare origin with one commit on main and clones it to the app's clone of repo
// (the same git commands the flows run). It returns the origin's path.
func coordOrigin(t *testing.T, root string, paths store.Paths, repos ...string) string {
	t.Helper()
	if !coordRun(root, "git init -q --bare -b main origin.git && git clone -q origin.git seed && cd seed && echo hi > README.md && git add . && git -c user.name=t -c user.email=t@t commit -qm init && git push -q origin main").OK() {
		t.Fatal("could not make the origin repository")
	}
	origin := filepath.Join(root, "origin.git")
	for _, repo := range repos {
		if err := os.MkdirAll(filepath.Dir(paths.Clone(repo)), 0o755); err != nil {
			t.Fatalf("mkdir clone: %v", err)
		}
		if !coordRun(root, fmt.Sprintf("git clone -q '%s' '%s'", origin, paths.Clone(repo))).OK() {
			t.Fatalf("could not clone %s", repo)
		}
	}
	return origin
}

// saveTmuxBelt saves belt as its first version with its state, as the flows' storage.saveNewVersion does.
func saveTmuxBelt(t *testing.T, storage store.Storage, belt core.Belt) core.Belt {
	t.Helper()
	saved, err := storage.SaveNewVersion(belt)
	if err != nil {
		t.Fatalf("save belt: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: testkit.ContractTime, WatchOutcomes: true, ReviewOutcomes: true})); err != nil {
		t.Fatalf("save belt state: %v", err)
	}
	return saved
}

// tmuxCoordinator builds a coordinator over storage with the real tmux runner on the test's server and
// the given workspaces, the fake GitHub and Jira tickets. It is closed when the test ends.
func tmuxCoordinator(t *testing.T, tm coordTmux, storage store.Storage, gh *github.FakeGitHub, tickets []core.Subject, workspaces runner.Workspaces) *Coordinator {
	t.Helper()
	coord, err := New(Services{
		Storage: storage, GitHub: gh, Jira: jira.NewFakeJira(tickets, nil, nil),
		Runner: tm.runner(), Workspaces: workspaces,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { coord.Close() })
	return coord
}

// coordDrive ticks c until done holds, polling every 20 ms for up to timeout.
func coordDrive(t *testing.T, c *Coordinator, timeout time.Duration, what string, done func() bool) {
	t.Helper()
	testkit.Eventually(t, timeout, what, func() bool {
		c.Tick(context.Background())
		c.Idle()
		return done()
	})
}

// agentBelt is Defaults.belt(repos:) with an id and base, each agent stage on the given backend, and the
// stages' workspace set to the given one when it is not nil.
func agentBelt(repos []string, backend core.Backend, base string) core.Belt {
	belt := core.SampleBelt(repos, "agent", core.SampleAgents(), true)
	for i := range belt.Stages {
		if belt.Stages[i].Type == core.StageTypeAgent {
			runnerBackend := backend
			belt.Stages[i].Runner = &runnerBackend
		}
	}
	belt.ID = fmt.Sprintf("tmux-%d", time.Now().UnixNano())
	belt.Base = base
	return belt
}

// firstStageBelt is Defaults.belt(repos:) with an id and base, and only its first stage on the given backend
// .
func firstStageBelt(repos []string, backend core.Backend, base string) core.Belt {
	belt := core.SampleBelt(repos, "agent", core.SampleAgents(), true)
	runnerBackend := backend
	belt.Stages[0].Runner = &runnerBackend
	belt.ID = fmt.Sprintf("tmux-%d", time.Now().UnixNano())
	belt.Base = base
	return belt
}

// coordSavedRun is the run with id as the folder saved it.
func coordSavedRun(storage store.Storage, id string) core.Run {
	for _, run := range storage.LoadRuns() {
		if run.ID == id {
			return run
		}
	}
	return core.Run{}
}

// coordIssue is an open agent issue on repo, as Subject(repo:number:title:body:labels:) makes one.
func coordIssue(repo string, number int, title, body string) core.Subject {
	return core.Subject{Kind: core.SubjectKindIssue, Repo: repo, Number: number, Title: title, Body: body, Labels: []string{"agent"}, IsOpen: true}
}

// checkoutDir is the directory of a run's checkout of the given kind.
func checkoutDir(paths store.Paths, run core.Run, kind runner.WorkspaceCheckoutKind) string {
	return runner.WorkspaceSpecDirectory(runner.WorkspaceCheckout{Kind: kind}, paths.WorkFolder(run))
}

// TestASessionThatChangesItsReadOnlyCheckoutFails: a session that edits its read-only checkout is held, and the
// checkout is left clean.
func TestASessionThatChangesItsReadOnlyCheckoutFails(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			coordOrigin(t, root, paths, "acme/web")
			saveTmuxBelt(t, storage, firstStageBelt([]string{"acme/web"}, backend, "main"))
			// The stand-in agent edits README.md when the issue asks it to.
			issue := coordIssue("acme/web", 8, "Vandal", "EDIT-THE-CHECKOUT")
			c := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub([]core.Subject{issue}, nil, nil, nil), nil, runner.NewGitWorkspaces(paths, coordShell()))
			coordDrive(t, c, 30*time.Second, "the run is held", func() bool {
				runs := c.Runs()
				return len(runs) > 0 && runs[0].State == core.RunStateHeld
			})
			run := c.Runs()[0]
			if run.State != core.RunStateHeld {
				t.Fatalf("state = %q, want held", run.State)
			}
			if detail := run.Attempts[0].Detail; detail == nil || *detail != "the session changed its read-only checkout" {
				t.Errorf("detail = %v", detail)
			}
			if status := coordRun(checkoutDir(paths, run, runner.WorkspaceCheckoutKindBase), "git status --porcelain"); status.Stdout != "" {
				t.Errorf("the read-only checkout changed: %q", status.Stdout)
			}
			c.Drop(run.ID, nil)
			c.Idle()
		})
	}
}

// coordAbortAndRestart: a running session is stopped by hand, the stop is saved, and
// the run is then answered (the session resumes) or rerun (a fresh session starts with the same context).
func coordAbortAndRestart(t *testing.T, backend core.Backend, answer bool) {
	t.Parallel()
	tm := tmuxSetup(t)
	root := t.TempDir()
	paths := store.Paths{Root: root}
	storage := store.NewStorage(paths)
	storage.SeedAgents()
	if err := storage.SaveSettings(tmuxSettings()); err != nil {
		t.Fatalf("settings: %v", err)
	}
	coordOrigin(t, root, paths, "acme/abort")
	belt := firstStageBelt([]string{"acme/abort"}, backend, "main")
	belt.Stages[0].Workspace = core.WorkspaceWorktree
	saveTmuxBelt(t, storage, belt)
	gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/abort", 10, "Stop me", "WAIT-FOR-ABORT")}, nil, nil, nil)
	workspaces := runner.NewGitWorkspaces(paths, coordShell())
	c := tmuxCoordinator(t, tm, storage, gh, nil, workspaces)
	coordDrive(t, c, 30*time.Second, "the session is ready to abort", func() bool {
		for _, run := range c.Runs() {
			attempt := run.CurrentAttempt()
			if attempt != nil && fileExistsAt(filepath.Join(paths.AttemptFolder(run, *attempt), "ready-to-abort")) {
				return true
			}
		}
		return false
	})
	running := c.Runs()[0]
	attempt := running.CurrentAttempt()
	if running.State != core.RunStateRunning || attempt == nil || attempt.Session == nil {
		t.Fatalf("running run = state %q, attempt %v", running.State, attempt)
	}
	session := *attempt.Session
	if !tm.has(session) {
		t.Fatalf("the session %s is not running", session)
	}
	c.Abort(running.ID, nil)
	testkit.Eventually(t, 15*time.Second, "the session stops", func() bool { return !tm.has(session) })
	c.Tick(context.Background())
	c.Idle()
	held := c.Run(running.ID)
	if held == nil || held.State != core.RunStateHeld {
		t.Fatalf("stopped run = %+v, want held", held)
	}
	if held.Reason == nil || *held.Reason != "stopped by you" {
		t.Errorf("reason = %v", held.Reason)
	}
	if held.StageID != running.StageID {
		t.Errorf("stage = %q, want %q", held.StageID, running.StageID)
	}
	if str(held.Branch) != str(running.Branch) {
		t.Errorf("branch = %v, want %v", held.Branch, running.Branch)
	}
	if route := held.Attempts[0].Route; route == nil || *route != "Stopped: by you" {
		t.Errorf("route = %v", route)
	}
	if !fileExistsAt(filepath.Join(checkoutDir(paths, *held, runner.WorkspaceCheckoutKindWorktree), "README.md")) {
		t.Errorf("the worktree has no README.md")
	}
	saved := coordSavedRun(storage, running.ID)
	if saved.State != core.RunStateHeld || str(saved.Reason) != str(held.Reason) || saved.StageID != held.StageID || str(saved.Branch) != str(held.Branch) {
		t.Errorf("the saved run does not match the stopped one: %+v", saved)
	}
	if route := saved.Attempts[0].Route; route == nil || *route != "Stopped: by you" || saved.Attempts[0].EndedAt == nil {
		t.Errorf("the saved attempt = route %v, ended %v", route, saved.Attempts[0].EndedAt)
	}
	if id := saved.Attempts[0].SessionID; id == nil || attempt.SessionID == nil || *id != *attempt.SessionID {
		t.Errorf("the saved session id = %v, want %v", id, attempt.SessionID)
	}
	if calls := gh.Calls(); len(calls) != 0 {
		t.Errorf("GitHub was called: %v", calls)
	}

	// Reload from disk before the answer: the chat ID must survive restart.
	restarted := tmuxCoordinator(t, tm, storage, gh, nil, workspaces)
	if answer {
		message := "Continue with the export page."
		restarted.Answer(held.ID, &message)
	} else {
		stage := held.StageID
		restarted.Resume(held.ID, &stage, nil, true, nil)
	}
	if answer && backend == core.BackendCodex {
		// The stand-in leaves no Codex rollout: the answer is refused rather than
		// starting a fresh conversation.
		coordDrive(t, restarted, 30*time.Second, "the answer is refused", func() bool {
			run := restarted.Run(held.ID)
			return run != nil && len(run.Attempts) > 1 && !run.Attempts[1].IsOpen()
		})
		refused := restarted.Run(held.ID).Attempts[1]
		if refused.Detail == nil || !strings.Contains(*refused.Detail, "no saved conversation to resume") || refused.Outcome != nil {
			t.Errorf("the answered Codex attempt = detail %v outcome %v, want refused", refused.Detail, refused.Outcome)
		}
		restarted.Drop(held.ID, nil)
		restarted.Idle()
		return
	}
	coordDrive(t, restarted, 30*time.Second, "the run reaches spec", func() bool {
		run := restarted.Run(held.ID)
		return run != nil && run.StageID == "spec"
	})
	continued := restarted.Run(held.ID)
	second := continued.Attempts[1]
	if answer {
		if second.Continues == nil || *second.Continues != attempt.ID {
			t.Errorf("continues = %v, want %s", second.Continues, attempt.ID)
		}
		if second.SessionID == nil || attempt.SessionID == nil || *second.SessionID != *attempt.SessionID {
			t.Errorf("the answered session id = %v, want %v", second.SessionID, attempt.SessionID)
		}
	} else {
		if second.Continues != nil {
			t.Errorf("a rerun continues %v", second.Continues)
		}
		if second.SessionID == nil || attempt.SessionID == nil || *second.SessionID == *attempt.SessionID {
			t.Errorf("a rerun reused the session id %v", second.SessionID)
		}
		if second.Replays == nil || *second.Replays != attempt.ID {
			t.Errorf("replays = %v, want %s", second.Replays, attempt.ID)
		}
	}
	if second.Outcome == nil || *second.Outcome != "ready to spec" {
		t.Errorf("outcome = %v, want ready to spec", second.Outcome)
	}
	folder := paths.AttemptFolder(*continued, second)
	log := readText(t, filepath.Join(folder, "session.log"))
	if answer {
		if !strings.Contains(log, "resumed with:") {
			t.Errorf("the answered session's log lacks 'resumed with:'")
		}
		sessionID := *attempt.SessionID
		switch backend {
		case core.BackendPi:
			coordHas(t, log, "--tui-mode fullscreen --approve --session "+sessionID)
			coordLacks(t, log, "--print")
		case core.BackendClaude, core.BackendCursor:
			coordHas(t, log, "--resume "+sessionID)
		case core.BackendCopilot:
			coordHas(t, log, "--resume=")
			coordHas(t, log, "--interactive")
			coordLacks(t, log, "--prompt")
		}
	} else {
		if strings.Contains(log, "resumed with:") {
			t.Errorf("a rerun resumed the aborted session")
		}
		script := readText(t, filepath.Join(folder, "run.sh"))
		if strings.Contains(script, *attempt.SessionID) {
			t.Errorf("a rerun's script names the aborted session")
		}
		// Cursor names the chat it just created, so only it keeps --resume.
		if backend != core.BackendCursor {
			coordLacks(t, script, "--resume")
		}
		if output := readText(t, filepath.Join(folder, "triage.md")); !strings.Contains(output, "Outcome: ready to spec") {
			t.Errorf("the rerun's output lacks its outcome")
		}
		if second.Folder == attempt.Folder {
			t.Errorf("the rerun reused the attempt folder %s", second.Folder)
		}
	}
	c.Drop(continued.ID, nil)
	c.Idle()
}

// coordHas fails the test when text lacks want.
func coordHas(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Errorf("the text lacks %q", want)
	}
}

// coordLacks fails the test when text contains unwanted.
func coordLacks(t *testing.T, text, unwanted string) {
	t.Helper()
	if strings.Contains(text, unwanted) {
		t.Errorf("the text contains %q", unwanted)
	}
}

// TestAbortingStopsTheSessionAndAnsweringContinuesIt: an abort stops the tmux session and saves the stop; an
// answer then resumes the same session.
func TestAbortingStopsTheSessionAndAnsweringContinuesIt(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			coordAbortAndRestart(t, backend, true)
		})
	}
}

// TestRerunningAfterAnAbortStartsAFreshTmuxSession: a rerun after an abort starts a new session with the same context.
func TestRerunningAfterAnAbortStartsAFreshTmuxSession(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			coordAbortAndRestart(t, backend, false)
		})
	}
}

// coordCheckJiraRepositories: a Jira ticket on two repositories runs its
// stages in tmux in one checkout holding every repository, and the checkout is removed when the run is dropped.
func coordCheckJiraRepositories(t *testing.T, workspace core.Workspace) {
	t.Parallel()
	tm := tmuxSetup(t)
	root := t.TempDir()
	paths := store.Paths{Root: root}
	storage := store.NewStorage(paths)
	storage.SeedAgents()
	if err := storage.SaveSettings(tmuxSettings()); err != nil {
		t.Fatalf("settings: %v", err)
	}
	repos := []string{"acme/billing", "acme/infra"}
	coordOrigin(t, root, paths, repos...)
	build := *core.SampleAgentNamed("build", nil)
	triage := *core.SampleAgentNamed("triage", nil)
	builder := core.DefaultsStageForAgent(build, nil, nil, nil, core.TriggerKindJira)
	builder.Brief = "Make a small code change and record what changed in {output}."
	stage := core.DefaultsStageForAgent(triage, []core.Stage{builder}, nil, []core.Stage{builder}, core.TriggerKindJira)
	stage.Workspace = workspace
	gate := core.DefaultsStage(core.StageTypeApprove, []core.Stage{builder, stage}, nil, []core.Stage{builder, stage})
	base := ""
	if workspace.IsReadOnly() {
		base = "main"
	}
	query := core.DefaultJiraQuery()
	query.Jql = "project = PAY"
	belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: fmt.Sprintf("tmux-%d", time.Now().UnixNano()), Name: "Jira tmux",
		Trigger: core.Trigger{Kind: core.TriggerKindJira, Jira: &query}, Repos: repos, Base: base,
		Stages: []core.Stage{builder, stage, gate},
	})
	saveTmuxBelt(t, storage, belt)
	c := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), []core.Subject{ticketFixture()}, runner.NewGitWorkspaces(paths, coordShell()))
	coordDrive(t, c, 60*time.Second, "the run waits at its gate", func() bool {
		runs := c.Runs()
		return len(runs) > 0 && runs[0].State == core.RunStateWaiting
	})
	run := c.Runs()[0]
	if run.State != core.RunStateWaiting {
		t.Fatalf("state = %q, want waiting", run.State)
	}
	checkout := sessionsCheckout(run, stage)
	container := runner.WorkspaceSpecDirectory(checkout, paths.WorkFolder(run))
	var attempt *core.Attempt
	for i := range run.Attempts {
		if run.Attempts[i].StageID == stage.ID {
			attempt = &run.Attempts[i]
		}
	}
	if attempt == nil {
		t.Fatal("the stage has no attempt")
	}
	script := readText(t, filepath.Join(paths.AttemptFolder(run, *attempt), "run.sh"))
	coordHas(t, script, container)
	for _, repo := range repos {
		folder := runner.WorkspaceSpecCheckout(repo, repos, container)
		if !fileExistsAt(filepath.Join(folder, ".git")) {
			t.Errorf("%s has no checkout at %s", repo, folder)
		}
		if filepath.Base(folder) != repo[strings.LastIndex(repo, "/")+1:] {
			t.Errorf("the checkout folder %s is not named for %s", filepath.Base(folder), repo)
		}
		if workspace.IsReadOnly() {
			head := strings.TrimSpace(coordRun(folder, "git rev-parse HEAD").Stdout)
			base := strings.TrimSpace(coordRun(paths.Clone(repo), "git rev-parse origin/main").Stdout)
			if head != base {
				t.Errorf("%s is at %s, want %s", repo, head, base)
			}
		} else {
			branch := strings.TrimSpace(coordRun(folder, "git rev-parse --abbrev-ref HEAD").Stdout)
			if branch != core.RunBranch(run) {
				t.Errorf("%s is on %q, want %s", repo, branch, core.RunBranch(run))
			}
		}
	}
	if workspace.IsReadOnly() {
		workspaces := runner.NewGitWorkspaces(paths, coordShell())
		ctx := context.Background()
		before := workspaces.Fingerprint(ctx, container)
		if !coordRun(filepath.Join(container, "infra"), "echo changed >> README.md").OK() {
			t.Fatal("could not change the read-only checkout")
		}
		if workspaces.Fingerprint(ctx, container) == before {
			t.Error("the change did not show in the fingerprint")
		}
		workspaces.Restore(ctx, container)
		if workspaces.Fingerprint(ctx, container) != before {
			t.Error("restoring did not return the checkout to its fingerprint")
		}
	}
	c.Drop(run.ID, nil)
	testkit.Eventually(t, 10*time.Second, "the work folder is removed", func() bool {
		return !fileExistsAt(paths.WorkFolder(run))
	})
	if fileExistsAt(paths.WorkFolder(run)) {
		t.Error("the work folder is still there")
	}
	for _, repo := range repos {
		if worktrees := coordRun(paths.Clone(repo), "git worktree list --porcelain").Stdout; strings.Contains(worktrees, container) {
			t.Errorf("%s still lists the removed checkout", repo)
		}
	}
}

// TestJiraReadOnlySessionHasEveryRepository: a read-only Jira session has every repository checked out at base.
func TestJiraReadOnlySessionHasEveryRepository(t *testing.T) {
	coordCheckJiraRepositories(t, core.WorkspaceReadOnly)
}

// TestJiraCodeSessionHasTheSameBranchInEveryRepository: a code session has the run's branch in every repository.
func TestJiraCodeSessionHasTheSameBranchInEveryRepository(t *testing.T) {
	coordCheckJiraRepositories(t, core.WorkspaceWorktree)
}

// TestJiraWorktreeReadOnlySessionHasEveryRepository: a read-only worktree session has every repository.
func TestJiraWorktreeReadOnlySessionHasEveryRepository(t *testing.T) {
	coordCheckJiraRepositories(t, core.WorkspaceWorktreeReadOnly)
}

// TestAnAnsweredStageResumesItsSessionInTmux: a held stage gets its reasoning effort on the command line, and an
// answer resumes the same session with the effort kept.
func TestAnAnsweredStageResumesItsSessionInTmux(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			coordOrigin(t, root, paths, "acme/app")
			belt := firstStageBelt([]string{"acme/app"}, backend, "main")
			belt.Policies.CommentOnHold = false
			high := core.ThinkingHigh
			belt.Stages[0].Thinking = &high
			saveTmuxBelt(t, storage, belt)
			// The stand-in agent holds when the issue asks it to.
			gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/app", 9, "Unclear", "HOLD-FOR-ME")}, nil, nil, nil)
			workspaces := runner.NewGitWorkspaces(paths, coordShell())
			c := tmuxCoordinator(t, tm, storage, gh, nil, workspaces)
			coordDrive(t, c, 30*time.Second, "the run is held", func() bool {
				runs := c.Runs()
				return len(runs) > 0 && runs[0].State == core.RunStateHeld
			})
			held := c.Runs()[0]
			if held.Reason == nil || *held.Reason != "needs info" {
				t.Errorf("reason = %v, want needs info", held.Reason)
			}
			script := readText(t, filepath.Join(paths.AttemptFolder(held, held.Attempts[0]), "run.sh"))
			switch backend {
			case core.BackendCodex:
				coordHas(t, script, "'-c' 'model_reasoning_effort=high'")
			case core.BackendPi:
				coordHas(t, script, "'--thinking' 'high'")
			case core.BackendClaude:
				coordHas(t, script, "'--effort' 'high'")
			case core.BackendCopilot:
				coordHas(t, script, "'--reasoning-effort' 'high'")
			case core.BackendCursor:
				coordLacks(t, script, "high")
			}
			if backend == core.BackendCopilot {
				coordHas(t, script, "'--interactive'")
				coordHas(t, script, "'--autopilot'")
				coordLacks(t, script, "--prompt")
			}
			if !held.Attempts[0].Interactive(backend) || held.Attempts[0].CompletionToken == nil {
				t.Errorf("%s: the first attempt was not launched interactively with a token", backend)
			}

			restarted := tmuxCoordinator(t, tm, storage, gh, nil, workspaces)
			restarted.Answer(held.ID, new("The export page."))
			if backend == core.BackendCodex {
				// The stand-in leaves no Codex rollout: the answer is refused rather
				// than starting a fresh conversation.
				coordDrive(t, restarted, 30*time.Second, "the answer is refused", func() bool {
					run := restarted.Run(held.ID)
					return run != nil && len(run.Attempts) > 1 && !run.Attempts[1].IsOpen()
				})
				refused := restarted.Run(held.ID).Attempts[1]
				if refused.Detail == nil || !strings.Contains(*refused.Detail, "no saved conversation to resume") || refused.Outcome != nil {
					t.Errorf("the answered Codex attempt = detail %v outcome %v, want refused", refused.Detail, refused.Outcome)
				}
				restarted.Drop(held.ID, nil)
				restarted.Idle()
				return
			}
			coordDrive(t, restarted, 30*time.Second, "the run reaches spec", func() bool {
				run := restarted.Run(held.ID)
				return run != nil && run.StageID == "spec"
			})
			run := restarted.Run(held.ID)
			if len(run.Attempts) < 2 {
				t.Fatalf("attempts = %d, want the answered one", len(run.Attempts))
			}
			if o := run.Attempts[1].Outcome; o == nil || *o != "ready to spec" {
				t.Errorf("outcome = %v, want ready to spec", o)
			}
			log := readText(t, filepath.Join(paths.AttemptFolder(*run, run.Attempts[1]), "session.log"))
			coordHas(t, log, "resumed with:")
			sessionID := *held.Attempts[0].SessionID
			switch backend {
			case core.BackendPi:
				coordHas(t, log, "--tui-mode fullscreen --approve --session "+sessionID)
				coordLacks(t, log, "--print")
				coordHas(t, log, "--thinking high")
			case core.BackendClaude:
				coordHas(t, log, "--resume "+sessionID)
				coordHas(t, log, "--effort high")
			case core.BackendCopilot:
				coordHas(t, log, "--resume=")
				coordHas(t, log, "--reasoning-effort high")
				coordHas(t, log, "--interactive")
				coordLacks(t, log, "--prompt")
			case core.BackendCursor:
				coordHas(t, log, "--resume "+sessionID)
				coordLacks(t, log, "high")
			}
			restarted.Drop(run.ID, nil)
			restarted.Idle()
		})
	}
}

// TestParkingALiveSessionStopsItAndUnparkStartsANewOneWithTheEarlierWork: parking stops the live session and keeps
// the checkout; unparking starts a new session that replays the earlier work.
func TestParkingALiveSessionStopsItAndUnparkStartsANewOneWithTheEarlierWork(t *testing.T) {
	t.Parallel()
	tm := tmuxSetup(t)
	root := t.TempDir()
	paths := store.Paths{Root: root}
	storage := store.NewStorage(paths)
	storage.SeedAgents()
	if err := storage.SaveSettings(tmuxSettings()); err != nil {
		t.Fatalf("settings: %v", err)
	}
	coordOrigin(t, root, paths, "acme/park")
	belt := firstStageBelt([]string{"acme/park"}, core.BackendPi, "main")
	belt.Stages[0].Workspace = core.WorkspaceWorktree
	saveTmuxBelt(t, storage, belt)
	gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/park", 11, "Park me", "WAIT-FOR-ABORT")}, nil, nil, nil)
	c := tmuxCoordinator(t, tm, storage, gh, nil, runner.NewGitWorkspaces(paths, coordShell()))
	coordDrive(t, c, 30*time.Second, "the session is ready to abort", func() bool {
		for _, run := range c.Runs() {
			attempt := run.CurrentAttempt()
			if attempt != nil && fileExistsAt(filepath.Join(paths.AttemptFolder(run, *attempt), "ready-to-abort")) {
				return true
			}
		}
		return false
	})
	running := c.Runs()[0]
	attempt := running.CurrentAttempt()
	if running.State != core.RunStateRunning || attempt == nil || attempt.Session == nil {
		t.Fatalf("running run = state %q, attempt %v", running.State, attempt)
	}
	session := *attempt.Session
	c.Park(running.ID)
	if got := c.Run(running.ID); got == nil || got.State != core.RunStateParked {
		t.Fatalf("parked run = %+v, want parked", got)
	}
	if route := c.Run(running.ID).Attempts[len(c.Run(running.ID).Attempts)-1].Route; route == nil || *route != "Parked by you" {
		t.Errorf("route = %v, want Parked by you", route)
	}
	testkit.Eventually(t, 15*time.Second, "the session stops", func() bool { return !tm.has(session) })
	if calls := gh.Calls(); len(calls) != 0 {
		t.Errorf("GitHub was called: %v", calls)
	}
	if !fileExistsAt(filepath.Join(checkoutDir(paths, running, runner.WorkspaceCheckoutKindWorktree), "README.md")) {
		t.Errorf("the parked checkout has no README.md")
	}
	c.Unpark(running.ID)
	coordDrive(t, c, 30*time.Second, "the run reaches spec", func() bool {
		run := c.Run(running.ID)
		return run != nil && run.StageID == "spec"
	})
	continued := c.Run(running.ID)
	if len(continued.Attempts) < 2 {
		t.Fatalf("attempts = %d, want the restarted one", len(continued.Attempts))
	}
	second := continued.Attempts[1]
	if second.Continues != nil {
		t.Errorf("continues = %v, want none", second.Continues)
	}
	if second.Replays == nil || *second.Replays != attempt.ID {
		t.Errorf("replays = %v, want %s", second.Replays, attempt.ID)
	}
	if second.SessionID == nil || attempt.SessionID == nil || *second.SessionID == *attempt.SessionID {
		t.Errorf("the restarted session reused the parked id")
	}
	log := readText(t, filepath.Join(paths.AttemptFolder(*continued, second), "session.log"))
	if !strings.Contains(log, "What this stage already had") && !strings.Contains(log, "stand-in agent") {
		t.Errorf("the restarted session's log shows neither the earlier work nor the stand-in")
	}
	c.Drop(continued.ID, nil)
	c.Idle()
}

// TestANonzeroExitHoldsWithTheRunnersLastLines: a session that exits non-zero holds its run with the runner's last
// lines, and its exit code is kept in the attempt folder.
func TestANonzeroExitHoldsWithTheRunnersLastLines(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			run := sessionSampleRun()
			run.State = core.RunStateQueued
			run.Attempts = nil
			run.BeltID = fmt.Sprintf("runner-failure-%d", time.Now().UnixNano())
			run.Stages[0].Runner = new(backend)
			run.Stages[0].Brief = "EXIT-WITHOUT-OUTPUT. Write to {output}."
			run.Stages[0].Workspace = core.WorkspaceWorktree
			if err := storage.SaveRun(run); err != nil {
				t.Fatalf("save run: %v", err)
			}
			c := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			coordDrive(t, c, 30*time.Second, "the run is held", func() bool {
				got := c.Run(run.ID)
				return got != nil && got.State == core.RunStateHeld
			})
			held := c.Run(run.ID)
			attempt := held.Attempts[len(held.Attempts)-1]
			if attempt.Detail == nil || !strings.Contains(*attempt.Detail, "code 42") {
				t.Errorf("detail = %v, want the exit code", attempt.Detail)
			} else if !strings.Contains(*attempt.Detail, "stand-in: test provider refused the request") {
				t.Errorf("detail = %v, want the runner's last line", *attempt.Detail)
			}
			folder := paths.AttemptFolder(*held, attempt)
			if exit := strings.TrimSpace(readText(t, filepath.Join(folder, "exit"))); exit != "42" {
				t.Errorf("exit file = %q, want 42", exit)
			}
			if !fileExistsAt(filepath.Join(folder, "session.log")) {
				t.Errorf("the attempt has no session.log")
			}
			c.Drop(held.ID, nil)
			c.Idle()
		})
	}
}

// TestPausingInterruptsGenerationAndResumeSendsContinue: a pause interrupts the live session and keeps its partial
// output; a resume sends Continue to the same session (Codex has no rollout to continue, so it fails and waits).
func TestPausingInterruptsGenerationAndResumeSendsContinue(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			belt := core.SampleBelt([]string{"acme/pause"}, "agent", core.SampleAgents(), true)
			belt.Stages[0].Runner = new(backend)
			belt.ID = fmt.Sprintf("pause-%s-%d", backend, time.Now().UnixNano()%1_000_000)
			saveTmuxBelt(t, storage, belt)
			gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/pause", 7, "Pause me", "WAIT-FOR-PAUSE PARTIAL-OUTCOME")}, nil, nil, nil)
			c := tmuxCoordinator(t, tm, storage, gh, nil, runner.FolderWorkspaces{})
			id := belt.ID + ":acme/pause#7"
			testkit.Eventually(t, 30*time.Second, "the session is ready to pause", func() bool {
				c.Tick(context.Background())
				c.Idle()
				run := c.Run(id)
				if run == nil {
					return false
				}
				attempt := run.CurrentAttempt()
				return attempt != nil && fileExistsAt(filepath.Join(paths.AttemptFolder(*run, *attempt), "ready-to-pause"))
			})
			running := c.Run(id)
			attempt := running.CurrentAttempt()
			c.PauseRun(context.Background(), id)
			paused := c.Run(id)
			if paused.Pause == nil || paused.Pause.Phase != core.RunPausePhasePaused {
				t.Fatalf("pause = %+v, want paused", paused.Pause)
			}
			if paused.StageID != "triage" || len(paused.Attempts) != 1 || paused.Attempts[0].Outcome != nil {
				t.Fatalf("paused run: stage %q, attempts %d", paused.StageID, len(paused.Attempts))
			}
			partial := readText(t, filepath.Join(paths.AttemptFolder(*paused, paused.Attempts[0]), "triage.md"))
			coordHas(t, partial, "Outcome:")
			for i := 0; i < 3; i++ {
				c.Tick(context.Background())
				c.Idle()
			}
			if got := c.Run(id); got.StageID != "triage" || got.Attempts[0].Outcome != nil {
				t.Errorf("a paused run moved: stage %q", got.StageID)
			}
			c.ResumeRun(context.Background(), id)
			if backend == core.BackendCodex {
				first := c.Run(id).Attempts[0]
				if first.Interruption == nil || first.Interruption.Continuation == nil ||
					first.Interruption.Continuation.Status != core.AttemptInterruptionContinuationStatusFailed ||
					first.Interruption.Continuation.Reason == nil || !strings.Contains(*first.Interruption.Continuation.Reason, "Codex has no rollout") {
					t.Errorf("the Codex continuation = %+v, want failed for want of a rollout", first.Interruption)
				}
				if got := c.Run(id); got.StageID != "triage" {
					t.Errorf("the Codex run moved to %q", got.StageID)
				}
				c.Drop(id, nil)
				c.Idle()
				return
			}
			coordDrive(t, c, 40*time.Second, "the run reaches spec", func() bool {
				run := c.Run(id)
				return run != nil && run.StageID == "spec"
			})
			continued := c.Run(id)
			first := continued.Attempts[0]
			if first.ID != attempt.ID || first.Folder != attempt.Folder || str(first.SessionID) != str(attempt.SessionID) {
				t.Errorf("the continued attempt changed identity: %s %s %s", first.ID, first.Folder, str(first.SessionID))
			}
			if o := first.Outcome; o == nil || *o != "ready to spec" {
				t.Errorf("outcome = %v, want ready to spec", o)
			}
			// The resumed turn ran interactively and finished on a handoff of its own,
			// not the interrupted turn's partial output.
			if !first.Interactive(backend) || first.CompletionToken == nil || attempt.CompletionToken == nil || *first.CompletionToken == *attempt.CompletionToken {
				t.Errorf("tokens: first launch %v, resumed %v; want a new one on the interactive attempt", attempt.CompletionToken, first.CompletionToken)
			}
			resumedBrief := readText(t, filepath.Join(paths.AttemptFolder(*continued, first), "brief.md"))
			// Copilot finishes on its own task completion, so it is asked for no handoff.
			if !backend.FinishesOnHandoff() {
				if resumedBrief != core.RunPauseContinuePrompt {
					t.Errorf("the resumed brief = %q, want Continue alone", resumedBrief)
				}
			} else if !strings.HasPrefix(resumedBrief, core.RunPauseContinuePrompt+"\n") || first.CompletionToken == nil || !strings.Contains(resumedBrief, *first.CompletionToken) {
				t.Errorf("the resumed brief = %q, want Continue with its own handoff", resumedBrief)
			}
			// The stand-in records its prompt under $TMPDIR.
			standIn := filepath.Join(os.TempDir(), "beltline-stand-in")
			remembered := ""
			for _, name := range []*string{attempt.SessionID, attempt.Session} {
				if name != nil {
					remembered += readTextOrEmpty(filepath.Join(standIn, *name, "transcript"))
				}
			}
			coordHas(t, remembered, "WAIT-FOR-PAUSE")
			c.Drop(id, nil)
			c.Idle()
		})
	}
}

// readTextOrEmpty is a file's text, or "" when it cannot be read.
func readTextOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// TestContinueEndsWhenTheRunLeavesAndKeepsItsTranscript: Continue opens a conversation in the attempt's
// checkout beside the attempt's own transcript; leaving the run ends it and keeps its own transcript.
func TestContinueEndsWhenTheRunLeavesAndKeepsItsTranscript(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			run := sessionSampleRun()
			run.Stages[0].Runner = new(backend)
			finishWith(&run, "needs info")
			session := "bl-continue-test-" + fmt.Sprintf("%d", time.Now().UnixNano())
			run.Attempts[0].Session = &session
			if err := storage.SaveRun(run); err != nil {
				t.Fatalf("save run: %v", err)
			}
			tmux := tm.runner()
			c := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			attempt := run.Attempts[0]
			spec := c.Spec(run, attempt, "")
			if spec == nil {
				t.Fatal("no spec for the attempt")
			}
			if err := os.MkdirAll(spec.Directory, 0o755); err != nil {
				t.Fatalf("mkdir directory: %v", err)
			}
			if err := os.MkdirAll(spec.AttemptFolder, 0o755); err != nil {
				t.Fatalf("mkdir attempt folder: %v", err)
			}
			original := filepath.Join(spec.AttemptFolder, "session.log")
			if err := os.WriteFile(original, []byte("original transcript\n"), 0o644); err != nil {
				t.Fatalf("write transcript: %v", err)
			}
			if _, err := c.ContinueSession(context.Background(), run.ID, attempt); err != nil {
				t.Fatalf("ContinueSession: %v", err)
			}
			name := runner.TmuxContinueName(spec.Name)
			testkit.Eventually(t, 15*time.Second, "Continue is ready", func() bool {
				return strings.Contains(tm.tmux("capture-pane", "-p", "-t", name).Stdout, "stand-in Continue")
			})
			if !tmux.ConversationAlive(context.Background(), name) {
				t.Fatalf("the Continue conversation %s is not alive", name)
			}
			stage := "spec"
			c.Resume(run.ID, &stage, nil, true, nil)
			c.Tick(context.Background())
			c.Idle()
			testkit.Eventually(t, 15*time.Second, "Continue ends with the run", func() bool { return !tm.has(name) })
			transcript := readText(t, filepath.Join(spec.AttemptFolder, "continue.log"))
			coordHas(t, transcript, "stand-in Continue in "+spec.Directory)
			if got := readText(t, original); got != "original transcript\n" {
				t.Errorf("the attempt's transcript changed: %q", got)
			}
			tmux.EndContinuing(context.Background(), *spec)
			if got := readText(t, filepath.Join(spec.AttemptFolder, "continue.log")); got != transcript {
				t.Errorf("the Continue transcript changed after it ended")
			}
			c.Drop(run.ID, nil)
			c.Tick(context.Background())
			c.Idle()
		})
	}
}

// TestTheAssistantTakesSeveralTurnsChangesTheLibraryAndSurvivesARestart: the configuration assistant keeps one
// tmux conversation across several turns, what it writes is the live library, and a restarted coordinator finds
// the conversation again.
func TestTheAssistantTakesSeveralTurnsChangesTheLibraryAndSurvivesARestart(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			first := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			if _, err := first.ConfigureAssistant(AssistantConfiguration{Backend: backend, Model: "gpt-test"}); err != nil {
				t.Fatalf("configure: %v", err)
			}
			if err := first.StartAssistant(context.Background(), "Create a helper"); err != nil {
				t.Fatalf("start: %v", err)
			}
			var session string
			locked(first, func() { session = first.assistant.session })
			testkit.Eventually(t, 15*time.Second, "the conversation directory is written", func() bool {
				return fileExistsAt(filepath.Join(root, "conversation-directory.txt"))
			})
			tmux := tm.runner()
			for n := 1; n <= 3; n++ {
				message := fmt.Sprintf("message-%d", n)
				if !tm.tmux("send-keys", "-t", session, message, "Enter").OK() {
					t.Fatalf("send-keys %s failed", message)
				}
				testkit.Eventually(t, 15*time.Second, message+" is taken", func() bool {
					return strings.Contains(readTextOrEmpty(filepath.Join(root, "turns.txt")), message)
				})
				if !tmux.ConversationAlive(context.Background(), session) {
					t.Fatalf("the conversation ended after %s", message)
				}
			}
			turns := coordLines(readTextOrEmpty(filepath.Join(root, "turns.txt")))
			if len(turns) != 3 {
				t.Errorf("turns = %d, want 3", len(turns))
			}
			// What it wrote is the live library, with nothing wrong in it.
			coordReconcile(first)
			if ids := beltIDs(first); len(ids) != 1 || ids[0] != "tasks" {
				t.Errorf("belts = %v, want [tasks]", ids)
			}
			if agents := agentIDsOf(first); len(agents) != 1 || agents[0] != "helper" {
				t.Errorf("agents = %v, want [helper]", agents)
			}
			locked(first, func() {
				if len(first.assistant.problems) != 0 {
					t.Errorf("problems = %v", first.assistant.problems)
				}
			})
			if fileExistsAt(filepath.Join(root, "prompt.md")) {
				t.Errorf("the prompt file sits in the data folder")
			}
			restarted := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			coordReconcile(restarted)
			locked(restarted, func() {
				if !restarted.assistant.running || restarted.assistant.command == nil {
					t.Errorf("restart: running %v, command %v", restarted.assistant.running, restarted.assistant.command)
				}
			})
			stopAssistant(restarted)
			locked(restarted, func() {
				if restarted.assistant.running {
					t.Errorf("the restarted assistant still runs after stop")
				}
			})
		})
	}
}

// coordLines is the non-empty lines of text.
func coordLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// agentIDsOf lists the ids of the agents the coordinator loaded.
func agentIDsOf(c *Coordinator) []string {
	var ids []string
	for _, record := range c.Agents() {
		ids = append(ids, record.Agent.ID)
	}
	return ids
}

// TestTheAfkAssistantApprovesInTmux: the AFK assistant decides an approval in a real tmux session, and a restart
// in the middle finds that decision again and applies it.
func TestTheAfkAssistantApprovesInTmux(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
			belt.Afk = core.AfkModeGlobal
			saveTmuxBelt(t, storage, belt)
			run := sessionSampleRun()
			finishWith(&run, "ready to spec")
			finishWith(&run, "complete")
			if err := storage.SaveRun(run); err != nil {
				t.Fatalf("save run: %v", err)
			}
			settings := tmuxSettings()
			settings.Afk = core.DefaultAfkSettings()
			settings.Afk.Enabled = true
			settings.Afk.Backend = backend
			settings.Afk.Model = "fixture-model"
			if err := storage.SaveSettings(settings); err != nil {
				t.Fatalf("settings: %v", err)
			}
			ctx := context.Background()
			first := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			locked(first, func() { first.autopilot.check(ctx) })
			locked(first, func() {
				if n := len(first.autopilot.Deciding()); n != 1 {
					t.Errorf("deciding = %d, want 1", n)
				}
			})
			restarted := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub(nil, nil, nil, nil), nil, runner.FolderWorkspaces{})
			testkit.Eventually(t, 60*time.Second, "the approval is applied", func() bool {
				locked(restarted, func() { restarted.autopilot.check(ctx) })
				got := restarted.Run(run.ID)
				return got != nil && got.StageID != "approve"
			})
			after := restarted.Run(run.ID)
			if after.StageID != "build" {
				t.Fatalf("stage = %q, want build", after.StageID)
			}
			var decision core.AfkDecision
			locked(restarted, func() {
				if len(restarted.autopilot.decisions) > 0 {
					decision = restarted.autopilot.decisions[0]
				}
			})
			const message = "Approved by the stand-in."
			if decision.Status != core.AfkDecisionStatusApplied {
				t.Errorf("decision status = %q, want applied", decision.Status)
			}
			if decision.Message == nil || *decision.Message != message {
				t.Errorf("decision message = %v", decision.Message)
			}
			if approve := after.LastAttempt("approve"); approve == nil || approve.Detail == nil || *approve.Detail != message {
				t.Errorf("the approval's detail = %+v", approve)
			}
			folder := filepath.Join(paths.RunFolder(*after), decision.Folder)
			coordHas(t, readText(t, filepath.Join(folder, "brief.md")), "deciding for someone who is away")
			if !fileExistsAt(filepath.Join(folder, "session.log")) {
				t.Errorf("the decision has no session.log")
			}
			restarted.Drop(after.ID, nil)
			restarted.Idle()
		})
	}
}

// TestTriageAndSpecRunInTmuxOnAReadOnlyCheckout: an issue runs triage and spec in tmux on a read-only checkout,
// then waits for the approval gate; the approval starts the build on its own branch.
func TestTriageAndSpecRunInTmuxOnAReadOnlyCheckout(t *testing.T) {
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			tm := tmuxSetup(t)
			root := t.TempDir()
			paths := store.Paths{Root: root}
			storage := store.NewStorage(paths)
			storage.SeedAgents()
			if err := storage.SaveSettings(tmuxSettings()); err != nil {
				t.Fatalf("settings: %v", err)
			}
			coordOrigin(t, root, paths, "acme/billing")
			belt := saveTmuxBelt(t, storage, agentBelt([]string{"acme/billing"}, backend, "main"))
			issue := coordIssue("acme/billing", 7, "Export", "Add an export.")
			c := tmuxCoordinator(t, tm, storage, github.NewFakeGitHub([]core.Subject{issue}, nil, nil, nil), nil, runner.NewGitWorkspaces(paths, coordShell()))

			coordDrive(t, c, 30*time.Second, "the run waits at its gate", func() bool {
				runs := c.Runs()
				return len(runs) > 0 && runs[0].State == core.RunStateWaiting
			})
			run := c.Runs()[0]
			if run.State != core.RunStateWaiting {
				t.Fatalf("state = %q, want waiting", run.State)
			}
			var outcomes []string
			for _, attempt := range run.Attempts {
				if attempt.Outcome == nil {
					outcomes = append(outcomes, "<nil>")
				} else {
					outcomes = append(outcomes, *attempt.Outcome)
				}
			}
			if want := []string{"ready to spec", "complete", "<nil>"}; fmt.Sprint(outcomes) != fmt.Sprint(want) {
				t.Errorf("outcomes = %v, want %v", outcomes, want)
			}
			log := readText(t, filepath.Join(paths.AttemptFolder(run, run.Attempts[0]), "session.log"))
			if !strings.Contains(log, "stand-in agent") {
				t.Errorf("the first session's log lacks the stand-in agent")
			}
			if _, err := os.Stat(filepath.Join(checkoutDir(paths, run, runner.WorkspaceCheckoutKindBase), "README.md")); err != nil {
				t.Errorf("the read-only checkout has no README.md: %v", err)
			}
			if run.Attempts[0].Session != nil && tm.has(*run.Attempts[0].Session) {
				t.Errorf("the first session is still running after it finished")
			}

			c.Decide(run.ID, "approved", nil)
			coordDrive(t, c, 30*time.Second, "the build finishes", func() bool {
				return slices.ContainsFunc(c.Run(run.ID).Attempts, func(attempt core.Attempt) bool {
					return attempt.StageID == "build" && attempt.Outcome != nil
				})
			})
			built := c.Run(run.ID)
			want := fmt.Sprintf("beltline/%s-issue-7", belt.ID)
			if built.Branch == nil || *built.Branch != want {
				t.Errorf("branch = %v, want %s", built.Branch, want)
			}
			branch := coordRun(checkoutDir(paths, *built, runner.WorkspaceCheckoutKindWorktree), "git rev-parse --abbrev-ref HEAD")
			if strings.TrimSpace(branch.Stdout) != want {
				t.Errorf("the worktree is on %q, want %s", strings.TrimSpace(branch.Stdout), want)
			}
			c.Drop(built.ID, nil)
			c.Idle()
		})
	}
}
