//go:build integration

// Coordinator tests of a run's legs: each with its own checkout and branch, the join that merges the
// leg branches, and a nested fork whose legs inherit the parent's branch. Real git clones, pushes and
// merges go through GitWorkspaces; the agents are simulated.

package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// mrLegBelt is bothRoutes with the simple route's build given the evidence key, the spec and
// the simple build on worktrees with one slot each, and the base branch main.
func mrLegBelt() core.Belt {
	belt := coordLegBelt()
	for i := range belt.Stages {
		if id := belt.Stages[i].ID; id == "spec" || id == "quick-build" {
			belt.Stages[i].Slots = 1
		}
	}
	belt.Base = "main"
	return belt
}

// mrNestedStage is a nested fixture stage: an agent on route, with one outcome.
func mrNestedStage(id, agent string, route *string, name string, outcome core.Outcome) core.Stage {
	stage := core.DefaultsStageForAgentAt(core.SampleAgent(agent), core.DefaultsPlace{Route: route}, nil, core.TriggerKindIssues)
	stage.ID = id
	stage.Name = name
	stage.Route = route
	stage.Outcomes = []core.Outcome{outcome}
	return stage
}

// mrNestedBelt is a wave fork (build, skip) whose build route holds
// a pace fork (quick, thorough).
func mrNestedBelt(skipStage bool) core.Belt {
	wave := core.Fork{ID: "wave", Routes: []core.Route{{ID: "build", Name: "Build"}, {ID: "skip", Name: "Skip"}}}
	pace := core.Fork{ID: "pace", Parent: new("build"), Routes: []core.Route{{ID: "quick", Name: "Quick"}, {ID: "thorough", Name: "Thorough"}}}
	goOn := core.Outcome{ID: "go", Name: "go", When: "go", Tone: core.ToneNeutral, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionNext})}
	done := core.Outcome{ID: "done", Name: "done", When: "done", Tone: core.ToneNeutral, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionNext})}
	opened := core.Outcome{ID: "opened", Name: "opened", When: "opened", Tone: core.ToneNeutral, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionDone})}
	build, quick, complexRoute := new("build"), new("quick"), new("thorough")
	stages := []core.Stage{
		mrNestedStage("triage", "triage", nil, "Triage", goOn),
		mrNestedStage("assess", "spec", build, "Assess", goOn),
		mrNestedStage("quick-build", "build", quick, "Build", done),
		mrNestedStage("thorough-spec", "spec", complexRoute, "Spec", done),
		mrNestedStage("thorough-build", "build", complexRoute, "Build", done),
		mrNestedStage("verify", "verify", build, "Verify", done),
	}
	if skipStage {
		stages = append(stages, mrNestedStage("skip-build", "build", new("skip"), "Build", done))
	}
	stages = append(stages, mrNestedStage("ship", "ship", nil, "Ship", opened))
	trigger := core.DefaultIssueFilter()
	return core.Belt{
		ID: "nested", Name: "Nested", Version: 1,
		Triggers: []core.TriggerEntry{{ID: core.TriggerEntryLegacyID, Trigger: core.Trigger{Kind: core.TriggerKindIssues, Issues: &trigger}, Repos: []string{"acme/billing"}}},
		Policies: core.DefaultPolicies(), Stages: stages, Forks: []core.Fork{wave, pace}, Afk: core.AfkModeGlobal,
	}
}

// mrNestedWorkBelt is the nested belt whose first stage splits into build and skip,
// whose assess splits into quick and thorough, on worktrees, with the outcomes that move on marked good.
func mrNestedWorkBelt() core.Belt {
	belt := mrNestedBelt(true)
	for i := range belt.Stages {
		switch belt.Stages[i].ID {
		case "triage":
			belt.Stages[i].Outcomes = []core.Outcome{{ID: "split", Name: "split", When: "split", Tone: core.ToneGood,
				Then: core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Routes: []string{"build", "skip"}})}}
		case "assess":
			belt.Stages[i].Workspace = core.WorkspaceWorktree
			belt.Stages[i].Outcomes = []core.Outcome{
				{ID: "pause", Name: "pause", When: "pause", Tone: core.ToneNeutral, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionHold})},
				{ID: "split", Name: "split", When: "split", Tone: core.ToneGood,
					Then: core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Routes: []string{"quick", "thorough"}})},
			}
		}
	}
	keys := map[string][3]string{ // stage id: key, brief placeholder, agent whose brief it takes
		"thorough-spec":  {"thorough-spec", "spec", "spec"},
		"thorough-build": {"thorough-evidence", "build", "evidence"},
		"skip-build":     {"skip-evidence", "build", "evidence"},
	}
	for i := range belt.Stages {
		id := belt.Stages[i].ID
		if spec, ok := keys[id]; ok {
			belt.Stages[i].Key = new(spec[0])
			belt.Stages[i].Brief = strings.ReplaceAll(core.SampleAgent(spec[1]).Brief, "{"+spec[2]+"}", "{output}")
		}
		if id == "quick-build" || id == "thorough-spec" || id == "thorough-build" || id == "skip-build" {
			belt.Stages[i].Workspace = core.WorkspaceWorktree
		}
	}
	for i := range belt.Stages {
		if belt.Stages[i].Kind() != core.StageKindAgent {
			continue
		}
		hasGood := false
		for _, outcome := range belt.Stages[i].Outcomes {
			hasGood = hasGood || outcome.Tone == core.ToneGood
		}
		if hasGood {
			continue
		}
		for j := range belt.Stages[i].Outcomes {
			if belt.Stages[i].Outcomes[j].Then.Action.MovesOn() {
				belt.Stages[i].Outcomes[j].Tone = core.ToneGood
				break
			}
		}
	}
	return belt
}

// mrHarness builds a harness over a real-git workspace on the belt, with the issues and the simulated agents'
// durations (a session runs for duration seconds).
func mrHarness(t *testing.T, belt core.Belt, issues []core.Subject, duration func(runner.SessionSpec) float64) *harness {
	t.Helper()
	return newHarness(t, harnessOptions{
		Issues: issues, Belts: []core.Belt{belt},
		WorkspacesFor: func(root string) runner.Workspaces {
			return runner.NewGitWorkspaces(store.Paths{Root: root}, coordShell())
		},
		Runner: func(script *harnessScript) runner.AgentRunner {
			return runner.NewSimulatedRunner(duration, func() time.Time { return testkit.ContractTime }, script.next)
		},
	})
}

// mrNoDuration is every session finishing at once.
func mrNoDuration(runner.SessionSpec) float64 { return 0 }

// mrGit runs git with arguments in dir and returns its trimmed standard output.
func mrGit(dir string, arguments ...string) string {
	result := coordShell().Run(context.Background(), "git", arguments, dir, nil)
	return strings.TrimSpace(result.Stdout)
}

// mrCommit stages every change in dir and commits it with a test identity.
func mrCommit(dir, message string) {
	mrGit(dir, "add", ".")
	mrGit(dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", message)
}

// mrCheckout is the folder a leg's checkout is in.
func mrCheckout(paths store.Paths, run core.Run, stage core.Stage, leg core.Leg) string {
	return runner.WorkspaceCheckoutPath(paths, run, sessionsCheckout(run, stage), &leg.ID)
}

// mrSeed makes the origin and the app's clone of acme/billing (the flows' origin).
func mrSeed(t *testing.T, h *harness) store.Paths {
	t.Helper()
	paths := store.Paths{Root: h.root}
	coordOrigin(t, h.root, paths, "acme/billing")
	return paths
}

// mrTick ticks the harness times times.
func mrTick(h *harness, times int) {
	for range times {
		h.Tick()
	}
}

// mrLegOn is the run's leg on route, or fails the test.
func mrLegOn(t *testing.T, run core.Run, route string) core.Leg {
	t.Helper()
	leg := run.LegOn(&route)
	if leg == nil {
		t.Fatalf("no leg on %s in %+v", route, run.Legs)
	}
	return *leg
}

// mrSplit ticks until the run has split and its complex leg is held, so the test can push onto each leg's branch
// before the join.
func mrSplit(t *testing.T, h *harness) core.Run {
	t.Helper()
	h.Script.add("triage", "ready to implement", "ready to implement")
	h.Script.add("spec", "open questions")
	h.Script.add("build", "done")
	h.Script.add("verify", "spec mismatch")
	mrTick(h, 12)
	run := h.Run()
	if run == nil || !run.IsSplit() {
		t.Fatalf("run = %+v, want a split run", run)
	}
	if complex := mrLegOn(t, *run, "complex"); complex.State != core.RunStateHeld {
		t.Fatalf("complex leg = %s, want held", complex.State)
	}
	return *run
}

// mrPushOnBothLegs commits a file in each leg's own worktree and pushes its branch, so the join has something to
// merge.
func mrPushOnBothLegs(h *harness, paths store.Paths, run core.Run, sameFile bool) {
	for _, leg := range run.Legs {
		if leg.Branch == nil || leg.StageID == nil {
			continue
		}
		stage := leg.Stage(run)
		if stage == nil {
			continue
		}
		folder := mrCheckout(paths, run, *stage, leg)
		file := strings.ReplaceAll(leg.ID, ":", "-") + ".txt"
		if sameFile {
			file = "README.md"
		}
		mrGit(folder, "checkout", "-q", "-B", *leg.Branch)
		_ = os.WriteFile(filepath.Join(folder, file), []byte("from "+leg.ID+"\n"), 0o644)
		mrCommit(folder, "leg "+leg.ID)
		mrGit(folder, "push", "-q", "origin", "HEAD:"+*leg.Branch)
	}
}

// mrFinishHeldLeg resumes the run and finishes its held complex leg, so the join runs.
func mrFinishHeldLeg(h *harness, run core.Run) {
	h.coord.Resume(run.ID, nil, nil, false, nil)
	h.Coord().Idle()
	mrTick(h, 12)
	h.Turn(func() {
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: complete"), Leg: new("r:complex")}, nil, nil, nil)
	})
	mrTick(h, 6)
}

// mrMergeLanded reports whether the join has recorded its merge: the branches are merged and the note says so, or
// an attempt shows it.
func mrMergeLanded(run *core.Run) bool {
	if run == nil || len(run.MergeBranches) != 0 {
		return false
	}
	if run.MergeNote != nil && strings.Contains(*run.MergeNote, "Merged") {
		return true
	}
	return slices.ContainsFunc(run.Attempts, func(attempt core.Attempt) bool {
		return attempt.Detail != nil && strings.Contains(*attempt.Detail, "Merged")
	})
}

// mrMergeMentions reports whether the merge the join recorded names branch: in the note, or in the attempt
// the meeting stage copied it onto.
func mrMergeMentions(run core.Run, branch string) bool {
	if run.MergeNote != nil && strings.Contains(*run.MergeNote, branch) {
		return true
	}
	return slices.ContainsFunc(run.Attempts, func(attempt core.Attempt) bool {
		return attempt.Detail != nil && strings.Contains(*attempt.Detail, "Merged") && strings.Contains(*attempt.Detail, branch)
	})
}

// TestEachLegGetsItsOwnWorktreeAndBranch
func TestEachLegGetsItsOwnWorktreeAndBranch(t *testing.T) {
	t.Parallel()
	h := mrHarness(t, mrLegBelt(), []core.Subject{{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Two routes", Labels: []string{"agent"}, IsOpen: true}}, mrNoDuration)
	paths := mrSeed(t, h)
	h.Script.add("triage", "ready to implement", "ready to implement")
	h.Script.add("spec", "open questions")
	h.Script.add("build", "done")
	mrTick(h, 10)
	run := h.Run()
	if run == nil || !run.IsSplit() {
		t.Fatalf("run = %+v, want a split run", run)
	}
	for _, leg := range run.Legs {
		stage := leg.Stage(*run)
		if stage == nil {
			t.Fatalf("leg %s has no stage", leg.ID)
		}
		directory := mrCheckout(paths, *run, *stage, leg)
		if _, err := os.Stat(filepath.Join(directory, ".git")); err != nil {
			t.Fatalf("leg %s has no checkout at %s", leg.ID, directory)
		}
		branch := mrGit(directory, "rev-parse", "--abbrev-ref", "HEAD")
		if branch != sessionsBranch(*run, &leg) {
			t.Fatalf("leg %s is on %q, want %q", leg.ID, branch, sessionsBranch(*run, &leg))
		}
		if branch == sessionsBranch(*run, nil) {
			t.Fatalf("leg %s is on the run's own branch %q", leg.ID, branch)
		}
	}
	for _, route := range []string{"complex", "simple"} {
		leg := mrLegOn(t, *run, route)
		if leg.Branch == nil || *leg.Branch != sessionsBranch(*run, &leg) {
			t.Fatalf("%s leg branch = %v, want %q", route, leg.Branch, sessionsBranch(*run, &leg))
		}
	}
}

// TestAStageWithOneSlotQueuesTheSecondRunsLeg
func TestAStageWithOneSlotQueuesTheSecondRunsLeg(t *testing.T) {
	t.Parallel()
	issues := []core.Subject{
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "One", Labels: []string{"agent"}, IsOpen: true},
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 10, Title: "Two", Labels: []string{"agent"}, IsOpen: true},
	}
	// Only triage finishes; the legs keep their sessions running.
	h := mrHarness(t, mrLegBelt(), issues, runningDuration)
	mrSeed(t, h)
	h.Script.add("triage", "ready to implement", "ready to implement")
	h.Until("two split runs", func() bool {
		runs := h.nowRuns()
		if len(runs) != 2 {
			return false
		}
		return !slices.ContainsFunc(runs, func(run core.Run) bool { return !run.IsSplit() })
	})
	runs := h.Coord().Runs()
	var atSpec, running []core.Leg
	for _, run := range runs {
		for _, leg := range run.WorkingLegs() {
			if leg.StageID != nil && *leg.StageID == "spec" {
				atSpec = append(atSpec, leg)
				if leg.State == core.RunStateRunning {
					running = append(running, leg)
				}
			}
		}
	}
	if len(running) > 1 {
		t.Fatalf("%d legs run at spec, want at most one", len(running))
	}
	queued := false
	for _, leg := range atSpec {
		queued = queued || leg.State == core.RunStateQueued
	}
	if !queued {
		t.Fatal("no leg is queued at spec")
	}
	for _, run := range runs {
		var folders []string
		for _, attempt := range run.Attempts {
			folders = append(folders, attempt.Folder)
		}
		seen := map[string]bool{}
		for _, folder := range folders {
			if seen[folder] {
				t.Fatalf("attempt folder %s is shared", folder)
			}
			seen[folder] = true
		}
	}
}

// TestTheJoinMergesBothLegBranchesIntoTheRunsBranch
func TestTheJoinMergesBothLegBranchesIntoTheRunsBranch(t *testing.T) {
	t.Parallel()
	h := mrHarness(t, mrLegBelt(), []core.Subject{{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Two routes", Labels: []string{"agent"}, IsOpen: true}}, mrNoDuration)
	paths := mrSeed(t, h)
	run := mrSplit(t, h)
	mrPushOnBothLegs(h, paths, run, false)
	mrFinishHeldLeg(h, run)
	h.Until("the run is at verify with the merge landed", func() bool {
		r := h.nowRun()
		return r != nil && r.StageID == "verify" && mrMergeLanded(r)
	})
	after := h.Run()
	if after == nil || after.StageID != "verify" || len(after.MergeBranches) != 0 || !mrMergeLanded(after) {
		t.Fatalf("after the join = %+v, want verify with the merge landed", after)
	}
	head := mrGit(runner.WorkspaceCheckoutPath(paths, *after, runner.WorkspaceCheckout{Kind: runner.WorkspaceCheckoutKindWorktree}, nil), "log", "--oneline")
	for _, leg := range []string{"leg r:complex", "leg r:simple"} {
		if !strings.Contains(head, leg) {
			t.Fatalf("the run's branch has no %q: %s", leg, head)
		}
	}
}

// TestAConflictAtTheJoinHoldsTheRunWithBothFilesNamed
func TestAConflictAtTheJoinHoldsTheRunWithBothFilesNamed(t *testing.T) {
	t.Parallel()
	h := mrHarness(t, mrLegBelt(), []core.Subject{{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Two routes", Labels: []string{"agent"}, IsOpen: true}}, mrNoDuration)
	paths := mrSeed(t, h)
	run := mrSplit(t, h)
	mrPushOnBothLegs(h, paths, run, true)
	mrFinishHeldLeg(h, run)
	h.Until("the run is held at verify, naming README.md", func() bool {
		r := h.nowRun()
		return r != nil && r.State == core.RunStateHeld && r.Reason != nil && strings.Contains(*r.Reason, "README.md")
	})
	after := h.Run()
	if after.State != core.RunStateHeld || after.StageID != "verify" || after.Reason == nil || !strings.Contains(*after.Reason, "README.md") {
		t.Fatalf("after the conflict = %+v, want held at verify naming README.md", after)
	}
}

// mrNestedSplit runs a nested work run to its build leg's hold and returns it (the start of the nested tests).
func mrNestedSplit(t *testing.T, h *harness) {
	t.Helper()
	h.Script.add("triage", "split")
	h.Script.add("spec", "pause", "split")
	h.Until("the build leg is held", func() bool {
		r := h.nowRun()
		return r != nil && r.LegOn(new("build")) != nil && r.LegOn(new("build")).State == core.RunStateHeld
	})
}

// TestNestedLegsInheritTheParentBranchAndMergeThere
func TestNestedLegsInheritTheParentBranchAndMergeThere(t *testing.T) {
	t.Parallel()
	h := mrHarness(t, mrNestedWorkBelt(), []core.Subject{{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Nested", Labels: []string{"agent"}, IsOpen: true}}, func(spec runner.SessionSpec) float64 {
		if strings.Contains(spec.Name, "quick-build") || strings.Contains(spec.Name, "thorough") {
			return 10_000
		}
		return 0
	})
	paths := mrSeed(t, h)
	mrNestedSplit(t, h)
	h.Script.add("review", "done")
	run := *h.Run()
	parent := mrLegOn(t, run, "build")
	parentBranch := *parent.Branch
	parentFolder := mrCheckout(paths, run, core.Stage{Workspace: core.WorkspaceWorktree}, parent)
	_ = os.WriteFile(filepath.Join(parentFolder, "parent.txt"), []byte("parent\n"), 0o644)
	mrGit(parentFolder, "add", "parent.txt")
	mrGit(parentFolder, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "parent")
	h.coord.Resume(run.ID, nil, nil, false, nil)
	h.Until("the quick and thorough legs run", func() bool {
		r := h.nowRun()
		return r != nil && r.LegOn(new("quick")) != nil && r.LegOn(new("quick")).State == core.RunStateRunning &&
			r.LegOn(new("thorough")) != nil && r.LegOn(new("thorough")).State == core.RunStateRunning
	})
	run = *h.Run()
	if skip := run.LegOn(new("skip")); skip == nil || !skip.IsFinished() {
		t.Fatalf("skip leg = %+v, want finished", skip)
	}
	var branches []string
	for _, leg := range run.Legs {
		if leg.Route == nil || (*leg.Route != "quick" && *leg.Route != "thorough") {
			continue
		}
		directory := mrCheckout(paths, run, core.Stage{Workspace: core.WorkspaceWorktree}, leg)
		text, err := os.ReadFile(filepath.Join(directory, "parent.txt"))
		if err != nil || string(text) != "parent\n" {
			t.Fatalf("leg %s has the parent's file %q, %v", leg.ID, text, err)
		}
		branch := mrGit(directory, "rev-parse", "--abbrev-ref", "HEAD")
		if branch == parentBranch {
			t.Fatalf("leg %s is on the parent branch %q", leg.ID, branch)
		}
		if slices.Contains(branches, branch) {
			t.Fatalf("leg %s shares branch %q", leg.ID, branch)
		}
		branches = append(branches, branch)
		_ = os.WriteFile(filepath.Join(directory, *leg.Route+".txt"), []byte("from "+leg.ID+"\n"), 0o644)
		mrCommit(directory, "leg "+leg.ID)
		mrGit(directory, "push", "-q", "origin", "HEAD:"+branch)
	}
	if len(branches) != 2 {
		t.Fatalf("branches = %v, want two", branches)
	}
	h.Turn(func() {
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:quick")}, nil, nil, nil)
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:thorough")}, nil, nil, nil)
	})
	h.Until("the thorough leg is at its build", func() bool {
		r := h.nowRun()
		leg := r.LegOn(new("thorough"))
		return r != nil && leg != nil && leg.StageID != nil && *leg.StageID == "thorough-build" && leg.State == core.RunStateRunning
	})
	h.Turn(func() {
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:thorough")}, nil, nil, nil)
	})
	// The merge note is seen between two ticks. Here the join's merge and the
	// meeting stage's start land in one tick, which copies the note onto that attempt and clears it, so the
	// merge is read where it landed (mergeLanded, as the join tests do).
	h.Until("the join has merged the legs", func() bool { return mrMergeLanded(h.nowRun()) })
	after := h.Run()
	if !mrMergeLanded(after) || !mrMergeMentions(*after, parentBranch) {
		t.Fatalf("the join merge = %+v, want it into the parent branch %q", after, parentBranch)
	}
	merged := mrGit(parentFolder, "log", "--oneline")
	if !strings.Contains(merged, "leg r:quick") || !strings.Contains(merged, "parent") {
		t.Fatalf("the parent's branch log = %s, want the quick leg and the parent commit", merged)
	}
}

// TestANestedMergeConflictNamesTheRouteAndLeavesTheSibling
func TestANestedMergeConflictNamesTheRouteAndLeavesTheSibling(t *testing.T) {
	t.Parallel()
	h := mrHarness(t, mrNestedWorkBelt(), []core.Subject{{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Nested", Labels: []string{"agent"}, IsOpen: true}}, func(spec runner.SessionSpec) float64 {
		if strings.Contains(spec.Name, "quick-build") || strings.Contains(spec.Name, "thorough") {
			return 10_000
		}
		return 0
	})
	paths := mrSeed(t, h)
	mrNestedSplit(t, h)
	run := *h.Run()
	h.coord.Resume(run.ID, nil, nil, false, nil)
	h.Until("the quick and thorough legs run", func() bool {
		r := h.nowRun()
		return r != nil && r.LegOn(new("quick")) != nil && r.LegOn(new("quick")).State == core.RunStateRunning &&
			r.LegOn(new("thorough")) != nil && r.LegOn(new("thorough")).State == core.RunStateRunning
	})
	run = *h.Run()
	skip := run.LegOn(new("skip"))
	var skipStage *string
	if skip != nil {
		skipStage = skip.StageID
	}
	for _, leg := range run.Legs {
		if leg.Route == nil || (*leg.Route != "quick" && *leg.Route != "thorough") {
			continue
		}
		directory := mrCheckout(paths, run, core.Stage{Workspace: core.WorkspaceWorktree}, leg)
		branch := mrGit(directory, "rev-parse", "--abbrev-ref", "HEAD")
		_ = os.WriteFile(filepath.Join(directory, "README.md"), []byte("from "+leg.ID+"\n"), 0o644)
		mrCommit(directory, "leg "+leg.ID)
		mrGit(directory, "push", "-q", "origin", "HEAD:"+branch)
	}
	h.Turn(func() {
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:quick")}, nil, nil, nil)
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:thorough")}, nil, nil, nil)
	})
	h.Until("the thorough leg is at its build", func() bool {
		r := h.nowRun()
		leg := r.LegOn(new("thorough"))
		return r != nil && leg != nil && leg.StageID != nil && *leg.StageID == "thorough-build" && leg.State == core.RunStateRunning
	})
	h.Turn(func() {
		h.coord.book.apply(run.ID, core.Event{Kind: core.EventKindSessionFinished, Output: new("Outcome: done"), Leg: new("r:thorough")}, nil, nil, nil)
	})
	h.Until("the run is held, naming README.md", func() bool {
		r := h.nowRun()
		return r != nil && r.State == core.RunStateHeld && r.Reason != nil && strings.Contains(*r.Reason, "README.md")
	})
	after := h.Run()
	if after.Reason == nil || !strings.Contains(*after.Reason, "README.md") {
		t.Fatalf("reason = %v, want README.md", after.Reason)
	}
	if !strings.Contains(*after.Reason, "Quick") && !strings.Contains(*after.Reason, "Thorough") {
		t.Fatalf("reason = %v, want the route named", *after.Reason)
	}
	leg := after.LegOn(new("skip"))
	var now *string
	if leg != nil {
		now = leg.StageID
	}
	if !reflect.DeepEqual(now, skipStage) {
		t.Fatal("the sibling skip leg moved")
	}
}
