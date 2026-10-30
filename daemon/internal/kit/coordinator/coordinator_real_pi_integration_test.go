//go:build integration

// Coordinator tests against real Pi sessions on a tiny local repository (BELTLINE_REAL_PI=1, model
// BELTLINE_REAL_PI_MODEL), with the fake GitHub and no outside calls. The gh and Jira live checks are
// in the github and jira packages (TestLiveGitHub, TestLiveJira).

package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// realPiSessionLog is the Pi session's.jsonl file in its folder, or "" when there is
// none. runner's piSessionLog is not exported, so this finds the same file through runner.PiSessionDirectory.
func realPiSessionLog(directory, sessionID string) string {
	folder := runner.PiSessionDirectory(directory)
	entries, err := os.ReadDir(folder)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".jsonl" && strings.Contains(entry.Name(), sessionID) {
			return filepath.Join(folder, entry.Name())
		}
	}
	return ""
}

// TestRealPiTriagesAnIssueAndReportsAnOutcome: one issue triaged by Pi
// with the belt's first stage, which keeps its own runner, and the session's log, usage and clean checkout.
func TestRealPiTriagesAnIssueAndReportsAnOutcome(t *testing.T) {
	if os.Getenv("BELTLINE_REAL_PI") != "1" {
		t.Skip("set BELTLINE_REAL_PI=1")
	}
	t.Parallel()
	c, paths, run, attempt := realOneIssueTriage(t, nil, os.Getenv("BELTLINE_REAL_PI_MODEL"))
	realLogTriage(t, "Pi", c, run, attempt)
	realExpectTriaged(t, paths, run, attempt)
	exit := strings.TrimSpace(readText(t, filepath.Join(paths.AttemptFolder(run, attempt), "exit")))
	if exit != "0" {
		t.Errorf("exit = %q, want 0", exit)
	}
	spec := c.Spec(run, attempt, "")
	if spec == nil {
		t.Fatal("the attempt has no session spec")
	}
	if attempt.SessionID == nil {
		t.Fatal("the attempt has no session id")
	}
	log := realPiSessionLog(spec.Directory, *attempt.SessionID)
	if log == "" {
		t.Fatal("the Pi session has no log")
	}
	if !fileExistsAt(log) {
		t.Errorf("the session log %s does not exist", log)
	}
	usage := attempt.Usage
	if usage == nil {
		t.Fatal("the attempt has no usage")
	}
	if usage.CostNanos == nil {
		t.Fatal("the usage has no cost")
	}
	if *usage.CostNanos < 0 {
		t.Errorf("cost = %d, want at least 0", *usage.CostNanos)
	}
	if usage.TotalTokens == nil {
		t.Fatal("the usage has no total tokens")
	}
	if *usage.TotalTokens <= 0 {
		t.Errorf("total tokens = %d, want more than 0", *usage.TotalTokens)
	}
}

// TestRealPiTakesAnIssueFromLabelToAVerifiedBranch: the default belt
// with Pi at every agent stage and one approval, up to Verify (the stages before Ship: Ship would push and open a
// real pull request). The run must ship, and its branch must have commits.
func TestRealPiTakesAnIssueFromLabelToAVerifiedBranch(t *testing.T) {
	if os.Getenv("BELTLINE_REAL_PI") != "1" {
		t.Skip("set BELTLINE_REAL_PI=1")
	}
	t.Parallel()
	tm := realTmux(t)
	root := t.TempDir()
	paths := store.Paths{Root: root}
	storage := realStorage(t, paths)
	realOrigin(t, root, paths, "acme/calc", realCalcSetup(`# calc\nA tiny calculator. Run tests with: python3 -m unittest\n`))

	belt := core.SampleBelt([]string{"acme/calc"}, "agent", core.SampleAgents(), true)
	belt.Base = "main"
	var stages []core.Stage
	for _, stage := range belt.Stages {
		if stage.ID == "ship" {
			break
		}
		stages = append(stages, stage)
	}
	belt.Stages = stages
	model := os.Getenv("BELTLINE_REAL_PI_MODEL")
	for i := range belt.Stages {
		if belt.Stages[i].Kind() == core.StageKindAgent {
			backend := core.BackendPi
			belt.Stages[i].Runner = &backend
			belt.Stages[i].Model = model
		}
	}
	saveTmuxBelt(t, storage, belt)

	gh := github.NewFakeGitHub([]core.Subject{coordIssue("acme/calc", 1, "add() subtracts", "calc.add(2, 3) returns -1 instead of 5.")}, nil, nil, nil)
	c := tmuxCoordinator(t, tm, storage, gh, nil, runner.NewGitWorkspaces(paths, coordShell()))
	approved := false
	realDrive(t, c, 1200*time.Second, "the Pi belt ends", func() bool {
		runs := c.Runs()
		if len(runs) == 0 {
			return false
		}
		run := runs[0]
		if run.State == core.RunStateWaiting && !approved {
			approved = true
			c.Decide(run.ID, "approved", nil)
		}
		return run.State == core.RunStateShipped || run.State == core.RunStateHeld ||
			run.State == core.RunStateDropped || run.State == core.RunStateParked
	})
	runs := c.Runs()
	if len(runs) == 0 {
		t.Fatal("the run was not created")
	}
	run := runs[0]
	t.Logf("Pi belt: state=%s reason=%s", run.State, core.ValueOf(run.Reason))
	for _, attempt := range run.Attempts {
		t.Logf("  %s #%d: %s → %s %s", attempt.StageID, attempt.Number, core.ValueOr(attempt.Outcome, "-"), core.ValueOr(attempt.Route, "-"), core.ValueOf(attempt.Detail))
	}
	if run.State != core.RunStateShipped {
		t.Errorf("state = %q, want shipped", run.State)
	}
	clone := paths.Clone("acme/calc")
	log := realGit(clone, "log", "--oneline", "origin/main..HEAD")
	branchLog := realGit(clone, "log", "--oneline", "main..beltline/issue-1")
	t.Logf("Commits on the branch:\n%s%s", branchLog.Stdout, log.Stderr)
	if branchLog.Stdout == "" {
		t.Error("the branch beltline/issue-1 has no commits over main")
	}
}
