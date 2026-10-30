//go:build integration

// Helpers shared by the opt-in real-agent tests of this package (coordinator_real_*_integration_test.go
// and coordinator_native_pause_integration_test.go). Unlike the tmux flows, these tests run no stand-in
// agent: the real CLI on the user's PATH is the agent, so each test is skipped unless its
// BELTLINE_REAL_* variable is set.

package coordinator

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// realTmuxSocketSeq numbers the real-agent tests' tmux servers within the process.
var realTmuxSocketSeq atomic.Int64

// realTmux gives one real-agent test a private tmux server: its own socket, and a pane shell that is the user's
// own, so the agent CLIs the coordinator starts are the real ones (a TmuxRunner on a socket of the test's
// own). The server is killed, and its socket file removed, when the test ends.
func realTmux(t *testing.T) coordTmux {
	t.Helper()
	tm := coordTmux{socket: fmt.Sprintf("beltline-test-%d-real-%d", os.Getpid(), realTmuxSocketSeq.Add(1))}
	if result := tm.tmux("-f", "/dev/null", "start-server", ";", "set-option", "-g", "exit-empty", "off"); !result.OK() {
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

// realStorage is the storage the real-agent tests start from: the built-in agents and the
// model fixtures, and no belts or runs.
func realStorage(t *testing.T, paths store.Paths) store.Storage {
	t.Helper()
	storage := store.NewStorage(paths)
	storage.SeedAgents()
	if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
		t.Fatalf("settings: %v", err)
	}
	return storage
}

// realCalcSetup is the shell script that makes the tiny calculator origin (git init -q --bare -b main
// origin.git && git clone … && printf …). readme is the README.md text as a printf format.
func realCalcSetup(readme string) string {
	return "git init -q --bare -b main origin.git && git clone -q origin.git seed && cd seed " +
		"&& printf 'def add(a, b):\\n    return a - b\\n' > calc.py " +
		"&& printf '" + readme + "' > README.md " +
		"&& git add . && git -c user.name=t -c user.email=t@t commit -qm init && git push -q origin main"
}

// realOrigin runs setup in root, which makes the bare origin, and clones the origin to the app's clone of repo.
func realOrigin(t *testing.T, root string, paths store.Paths, repo, setup string) {
	t.Helper()
	if !coordRun(root, setup).OK() {
		t.Fatal("could not make the origin repository")
	}
	if err := os.MkdirAll(filepath.Dir(paths.Clone(repo)), 0o755); err != nil {
		t.Fatalf("mkdir clone: %v", err)
	}
	if !coordRun(root, fmt.Sprintf("git clone -q '%s' '%s'", filepath.Join(root, "origin.git"), paths.Clone(repo))).OK() {
		t.Fatalf("could not clone %s", repo)
	}
}

// realGit runs git with arguments in dir.
func realGit(dir string, arguments ...string) shell.Result {
	return coordShell().Run(context.Background(), "git", arguments, dir, nil)
}

// realPoll checks cond once a second until it holds, and fails the test when timeout passes first.
// It uses testkit.Eventually, which polls every 20 ms,
// and cond is only evaluated once a second.
func realPoll(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	var last time.Time
	testkit.Eventually(t, timeout, what, func() bool {
		if !last.IsZero() && time.Since(last) < time.Second {
			return false
		}
		last = time.Now()
		return cond()
	})
}

// realDrive ticks c once a second, as the loops do, until done holds or timeout passes.
func realDrive(t *testing.T, c *Coordinator, timeout time.Duration, what string, done func() bool) {
	t.Helper()
	realPoll(t, timeout, what, func() bool {
		c.Tick(context.Background())
		c.Idle()
		return done()
	})
}

// realOneIssueTriage runs one issue of acme/calc through a belt with only its first stage, the agent stage that
// triages. backend, when not nil, is the stage's runner; nil keeps the stage's
// own runner. The stage's model is model. It drives the coordinator until the run leaves queued and running
//
// and returns the coordinator, the paths, the run and its first attempt.
func realOneIssueTriage(t *testing.T, backend *core.Backend, model string) (*Coordinator, store.Paths, core.Run, core.Attempt) {
	t.Helper()
	tm := realTmux(t)
	root := t.TempDir()
	paths := store.Paths{Root: root}
	storage := realStorage(t, paths)
	realOrigin(t, root, paths, "acme/calc", realCalcSetup(`# calc\nA tiny calculator.\n`))

	belt := core.SampleBelt([]string{"acme/calc"}, "agent", core.SampleAgents(), true)
	belt.Base = "main"
	belt.Stages = belt.Stages[:1]
	if backend != nil {
		belt.Stages[0].Runner = backend
	}
	belt.Stages[0].Model = model
	saveTmuxBelt(t, storage, belt)

	gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/calc", 1, "add() subtracts", "calc.add(2, 3) returns -1 instead of 5.")}, nil, nil, nil)
	c := tmuxCoordinator(t, tm, storage, gh, nil, runner.NewGitWorkspaces(paths, coordShell()))
	realDrive(t, c, 240*time.Second, "the triage ends", func() bool {
		runs := c.Runs()
		return len(runs) > 0 && runs[0].State != core.RunStateQueued && runs[0].State != core.RunStateRunning
	})
	runs := c.Runs()
	if len(runs) == 0 {
		t.Fatal("the run was not created")
	}
	run := runs[0]
	if len(run.Attempts) == 0 {
		t.Fatal("the run has no attempt")
	}
	return c, paths, run, run.Attempts[0]
}

// realLogTriage logs the triage's state, outcome and detail and the attempt's output.
func realLogTriage(t *testing.T, label string, c *Coordinator, run core.Run, attempt core.Attempt) {
	t.Helper()
	output := "(none)"
	if text := c.Output(run, attempt); text != nil {
		output = *text
	}
	t.Logf("%s triage: state=%s outcome=%s detail=%s\n%s", label, run.State, core.ValueOr(attempt.Outcome, "none"), core.ValueOf(attempt.Detail), output)
}

// realExpectTriaged asserts what every triage test asserts: the attempt has an outcome, the run is shipped, held or
// parked, and the base checkout is clean.
func realExpectTriaged(t *testing.T, paths store.Paths, run core.Run, attempt core.Attempt) {
	t.Helper()
	if attempt.Outcome == nil {
		t.Error("the attempt has no outcome")
	}
	if run.State != core.RunStateShipped && run.State != core.RunStateHeld && run.State != core.RunStateParked {
		t.Errorf("state = %q, want shipped, held or parked", run.State)
	}
	if status := realGit(checkoutDir(paths, run, runner.WorkspaceCheckoutKindBase), "status", "--porcelain"); status.Stdout != "" {
		t.Errorf("the base checkout is not clean: %q", status.Stdout)
	}
}

// realFinishedZero reports whether a session finished with exit code 0.
func realFinishedZero(status runner.SessionStatus) bool {
	return status.Kind == runner.SessionStatusFinished && status.ExitCode != nil && *status.ExitCode == 0
}

// realPrefix is the first n bytes of s, or s when it is shorter.
func realPrefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// realUUID is a random UUID in upper case text form.
func realUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}
