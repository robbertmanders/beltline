// Coordinator tests: the coordinator driven through its ticks, sessions, restarts and commands.

package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// readText reads a file the test wrote or the coordinator wrote, failing the test when it cannot.
func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// writeText writes text to path, creating its folder, failing the test when it cannot.
func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// locked runs f with c.mu held, as a main-actor call in a test does.
func locked(c *Coordinator, f func()) {
	c.Lock()
	defer c.EndTurn()
	f()
}

// finishWith starts and finishes the run's session in the engine, its output ending in an Outcome: line.
func finishWith(run *core.Run, outcome string) {
	core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionStarted, Session: "bl-test", SessionID: "id", Folder: "f"}, sessionSampleT0)
	text := "Some output.\n\nOutcome: " + outcome
	core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionFinished, Output: &text}, sessionSampleT0)
}

// ctxBackground is the context the coordinator tests run their calls with.
var ctxBackground = context.Background()

// TestFinishedSessionPersistsUsageAndStartMark: a finished session keeps its usage and start mark.
func TestFinishedSessionPersistsUsageAndStartMark(t *testing.T) {
	t.Parallel()
	for _, reportsUsage := range []bool{true, false} {
		t.Run(fmt.Sprintf("reportsUsage=%t", reportsUsage), func(t *testing.T) {
			t.Parallel()
			sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, testkitNow, func(runner.SessionSpec) *string { return nil })
			h := newHarness(t, harnessOptions{Runner: func(*harnessScript) runner.AgentRunner {
				return &costRunner{SimulatedRunner: sim, reportsUsage: reportsUsage}
			}})
			c := h.Coord()
			belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
			subject := core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 1, Title: "Test"}
			var run core.Run
			h.Turn(func() { run = c.book.add(core.NewRun(core.RunArgs{Belt: belt, Subject: subject, Now: testkitNow()})) })
			h.Turn(func() { c.sessions.start(ctxBackground, run.ID, nil) })
			if runs := c.Runs(); len(runs) == 0 || runs[0].Attempts[len(runs[0].Attempts)-1].UsageMark == nil ||
				*runs[0].Attempts[len(runs[0].Attempts)-1].UsageMark != 11_000_000_000 {
				t.Fatalf("usage mark = %v, want 11000000000", runs)
			}
			h.Turn(func() { c.sessions.check(ctxBackground) })
			saved := h.storage.LoadRuns()
			if len(saved) == 0 {
				t.Fatal("no saved run")
			}
			last := saved[0].Attempts[len(saved[0].Attempts)-1]
			if reportsUsage {
				if last.Usage == nil || last.Usage.CostNanos == nil || *last.Usage.CostNanos != 230_000_000 {
					t.Fatalf("usage = %+v, want the reported cost", last.Usage)
				}
			} else if last.Usage != nil {
				t.Fatalf("usage = %+v, want none", last.Usage)
			}
			if last.EndedAt == nil {
				t.Fatal("attempt has no end")
			}
		})
	}
}

// testkitNow is the instant the coordinator tests give their runs and runners.
func testkitNow() time.Time { return sessionSampleT0 }

// TestAbortingARunningRunHoldsItAndKeepsItsCheckouts: aborting holds the run and keeps its checkout.
func TestAbortingARunningRunHoldsItAndKeepsItsCheckouts(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Tick()
	running := h.Run()
	if running == nil || running.State != core.RunStateRunning {
		t.Fatalf("run = %+v, want running", running)
	}
	session := *running.CurrentAttempt().Session
	sim := h.services.Runner.(*runner.SimulatedRunner)
	if !sim.IsRunning(session) {
		t.Fatal("session is not running")
	}
	folder := h.Coord().Paths().WorkFolder(*running)
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("work folder: %v", err)
	}
	h.Coord().Abort(running.ID, nil)
	after := h.Run()
	if after.State != core.RunStateHeld || after.StageID != running.StageID {
		t.Fatalf("after abort: state %s stage %s", after.State, after.StageID)
	}
	if after.Reason == nil || *after.Reason != "stopped by you" {
		t.Fatalf("reason = %v, want stopped by you", after.Reason)
	}
	saved := h.storage.LoadRuns()[0]
	if saved.State != core.RunStateHeld || saved.Reason == nil || *saved.Reason != "stopped by you" || saved.StageID != running.StageID {
		t.Fatalf("saved run = %+v", saved)
	}
	last := saved.Attempts[len(saved.Attempts)-1]
	if last.Route == nil || *last.Route != "Stopped: by you" {
		t.Fatalf("route = %v, want Stopped: by you", last.Route)
	}
	if !core.SamePointee(last.SessionID, running.Attempts[len(running.Attempts)-1].SessionID) {
		t.Fatal("saved attempt lost its session id")
	}
	if last.EndedAt == nil {
		t.Fatal("saved attempt has no end")
	}
	// The effect task stops the session; Idle waits for it.
	h.Coord().Idle()
	if sim.IsRunning(session) {
		t.Fatal("session still running after the stop effect")
	}
	h.Tick()
	if got := h.Run(); got == nil || got.State != core.RunStateHeld {
		t.Fatalf("after a tick: %+v, want held", got)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("work folder gone: %v", err)
	}
	if calls := h.github.Calls(); len(calls) != 0 {
		t.Fatalf("github calls = %+v, want none", calls)
	}
}

// TestRerunningAfterAnAbortStartsANewSession: a rerun after an abort starts a new session.
func TestRerunningAfterAnAbortStartsANewSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Tick()
	running := h.Run()
	first := running.CurrentAttempt()
	holdKicks(t, h.Coord())
	h.Coord().Abort(running.ID, nil)
	h.finishStop()
	stage := running.StageID
	h.Coord().Resume(running.ID, &stage, nil, true, nil)
	h.untilHeld("r := h.nowRun(); return r != nil && len(r.Attempts) == 2", func() bool { r := h.nowRun(); return r != nil && len(r.Attempts) == 2 })
	run := h.Run()
	second := run.Attempts[len(run.Attempts)-1]
	spec := h.Coord().Spec(*run, second, "")
	if spec == nil {
		t.Fatal("no spec for the second attempt")
	}
	if second.SessionID == nil || core.SamePointee(second.SessionID, first.SessionID) {
		t.Fatalf("second session id = %v, first = %v", second.SessionID, first.SessionID)
	}
	if second.Continues != nil || second.AgentName != nil {
		t.Fatalf("second continues %v agent %v, want none", second.Continues, second.AgentName)
	}
	if second.Replays == nil || *second.Replays != first.ID {
		t.Fatalf("replays = %v, want %s", second.Replays, first.ID)
	}
	if spec.Resume {
		t.Fatal("spec resumes, want a new session")
	}
	if spec.AgentName != *second.Session {
		t.Fatalf("spec agent name = %q, want %q", spec.AgentName, *second.Session)
	}
	if spec.UsageMark != 0 {
		t.Fatalf("usage mark = %d, want 0", spec.UsageMark)
	}
}

// TestASessionStartsOnTheStageCurrentSettings: a session starts on the stage's current settings,
// whether it resumes the earlier one and whether the run was sent back.
func TestASessionStartsOnTheStageCurrentSettings(t *testing.T) {
	t.Parallel()
	for _, earlier := range []bool{true, false} {
		for _, sendBack := range []bool{true, false} {
			t.Run(fmt.Sprintf("earlier=%t/sendBack=%t", earlier, sendBack), func(t *testing.T) {
				t.Parallel()
				belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
				high := core.ThinkingHigh
				belt.Stages[1].Thinking = &high
				h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}})
				c := h.Coord()
				h.Script.add("triage", "ready to spec")
				h.Until("r := h.nowRun() ...", func() bool {
					r := h.nowRun()
					return r != nil && r.StageID == "spec" && r.State == core.RunStateRunning
				})
				running := h.Run()
				low := core.ThinkingLow
				settings := core.SessionSettings{Model: "test/model", Thinking: &low}
				h.Turn(func() {
					c.book.update(running.ID, nil, nil, func(r *core.Run) {
						r.ChangeCurrentAttempt(func(a *core.Attempt) { a.Settings = &settings })
					})
				})
				first := h.Run().CurrentAttempt()
				directory := ""
				if spec := c.Spec(*h.Run(), *first, ""); spec != nil {
					directory = spec.Directory
				}
				if sendBack {
					approved := "Outcome: complete"
					h.Turn(func() {
						c.book.apply(running.ID, core.Event{Kind: core.EventKindSessionFinished, Output: &approved}, nil, nil, nil)
					})
					c.Decide(running.ID, "approved", nil)
					h.Until("r := h.nowRun() ...", func() bool {
						r := h.nowRun()
						return r != nil && r.StageID == "build" && r.State == core.RunStateRunning
					})
				}
				branchBeforeSend := h.Run().Branch
				c.Abort(running.ID, nil)
				model := "current/model"
				thinking := core.ThinkingHigh
				if _, err := c.AdjustStage(running.ID, "spec", &model, &thinking, nil, nil); err != nil {
					t.Fatalf("adjust stage: %v", err)
				}
				spec := "spec"
				c.Resume(running.ID, &spec, nil, earlier, nil)
				h.Until("r := h.nowRun() ...", func() bool {
					r := h.nowRun()
					return r != nil && lastAttemptNumber(*r, "spec") == 2
				})
				run := h.Run()
				second := run.LastAttempt("spec")
				if second == nil {
					t.Fatal("no second spec attempt")
				}
				got := c.Spec(*run, *second, "")
				if got == nil {
					t.Fatal("no spec for the second attempt")
				}
				if got.Model != "current/model" || got.Thinking == nil || *got.Thinking != core.ThinkingHigh {
					t.Fatalf("spec model %q thinking %v, want current/model high", got.Model, got.Thinking)
				}
				// The runner is recorded with the attempt, so a later resume knows its backend.
				piBackend := core.BackendPi
				wantSettings := core.SessionSettings{Model: "current/model", Thinking: &thinking, Runner: &piBackend}
				if !reflect.DeepEqual(second.Settings, &wantSettings) {
					t.Fatalf("settings = %+v, want %+v", second.Settings, wantSettings)
				}
				if earlier {
					if second.Replays == nil || *second.Replays != first.ID {
						t.Fatalf("replays = %v, want %s", second.Replays, first.ID)
					}
				} else if second.Replays != nil {
					t.Fatalf("replays = %v, want none", second.Replays)
				}
				prompt := readText(t, filepath.Join(got.AttemptFolder, "brief.md"))
				if strings.Contains(prompt, "## What this stage already had") != earlier {
					t.Fatalf("earlier section present = %t, want %t", !earlier, earlier)
				}
				saved := h.storage.LoadRuns()[0]
				if savedFirst := saved.AttemptByID(&first.ID); savedFirst == nil || !reflect.DeepEqual(savedFirst.Settings, &settings) {
					t.Fatalf("saved first settings = %+v, want %+v", savedFirst, settings)
				}
				if savedLast := saved.LastAttempt("spec"); savedLast == nil || !reflect.DeepEqual(savedLast.Settings, second.Settings) {
					t.Fatal("saved second settings differ")
				}
				historical := c.Spec(*run, *first, "")
				if historical == nil || historical.Model != settings.Model || !reflect.DeepEqual(historical.Thinking, settings.Thinking) {
					t.Fatalf("historical spec = %+v, want the first settings", historical)
				}
				if got.Directory != directory {
					t.Fatalf("directory = %q, want %q", got.Directory, directory)
				}
				if !core.SamePointee(run.Branch, branchBeforeSend) {
					t.Fatalf("branch = %v, want %v", run.Branch, branchBeforeSend)
				}
			})
		}
	}
}

// lastAttemptNumber is the number of the run's last attempt at stageID, or 0.
func lastAttemptNumber(run core.Run, stageID string) int {
	if attempt := run.LastAttempt(stageID); attempt != nil {
		return attempt.Number
	}
	return 0
}

// TestASentMessageSurvivesReloadingTheRun: a note sent with a resume is saved and written into the brief.
func TestASentMessageSurvivesReloadingTheRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Script.add("triage", "needs info")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	run := h.Run()
	spec := "spec"
	note := "  Keep the header.\nCheck empty exports. \n"
	// 	// The read of the disk happens before the next await; the kick Resume spawns would move the note into the attempt.
	holdKicks(t, c)
	c.Resume(run.ID, &spec, &note, true, nil)
	const want = "Keep the header.\nCheck empty exports."
	if got := h.Run(); got == nil || !core.SamePointee(got.Note, new(want)) {
		t.Fatalf("note = %v, want %q", got, want)
	}
	// Check the queued note on disk, before the asynchronous kick starts it.
	if saved := h.storage.LoadRuns(); len(saved) == 0 || !core.SamePointee(saved[0].Note, new(want)) {
		t.Fatalf("saved note = %+v", saved)
	}
	h.Until("r := h.nowRun() ...", func() bool {
		r := h.nowRun()
		if r == nil {
			return false
		}
		return slices.ContainsFunc(r.Attempts, func(attempt core.Attempt) bool {
			return attempt.StageID == "spec"
		})
	})
	restored := h.storage.LoadRuns()[0]
	var specAttempt *core.Attempt
	for i := range restored.Attempts {
		if restored.Attempts[i].StageID == "spec" {
			specAttempt = &restored.Attempts[i]
		}
	}
	if specAttempt == nil || !core.SamePointee(specAttempt.Note, new(want)) {
		t.Fatalf("spec attempt note = %+v, want %q", specAttempt, want)
	}
	brief := readText(t, filepath.Join(c.Paths().RunFolder(restored), "02-spec-1", "brief.md"))
	if !strings.Contains(brief, "A note came with this attempt. Act on it before anything else:") {
		t.Fatal("brief lacks the note heading")
	}
	if !strings.Contains(brief, want) {
		t.Fatal("brief lacks the note")
	}
}

// TestALabelledIssueBecomesARunAndStartsTriage: a labelled issue starts a run at triage.
func TestALabelledIssueBecomesARunAndStartsTriage(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Tick()
	runs := h.Coord().Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.State != core.RunStateRunning || run.StageID != "triage" {
		t.Fatalf("run = %s at %s, want running at triage", run.State, run.StageID)
	}
	brief := readText(t, filepath.Join(h.Coord().Paths().RunFolder(run), "01-triage-1", "brief.md"))
	if !strings.Contains(brief, "Export invoices as CSV") {
		t.Fatal("brief lacks the issue title")
	}
}

// TestAnIssueGoesFromLabelToPullRequestWithOneApproval: an issue runs to a merged pull request after one approval.
func TestAnIssueGoesFromLabelToPullRequestWithOneApproval(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting })
	run := h.Run()
	if run.StageID != "approve" {
		t.Fatalf("stage = %s, want approve", run.StageID)
	}
	if got, want := optionalTexts(attemptOutcomes(run.Attempts)), []string{"ready to implement", "complete", "(nil)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %v, want %v", got, want)
	}
	h.Coord().Decide(run.ID, "approved", nil)
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateShipped", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateShipped })
	run = h.Run()
	if run.State != core.RunStateShipped {
		t.Fatalf("state = %s, want shipped", run.State)
	}
	if !hasNoticeContaining(h.Coord().Notices(), run.Subject.Ref()+" shipped") {
		t.Fatalf("notices = %+v, want the run to say it shipped", h.Coord().Notices())
	}
	var outcomes []string
	for _, attempt := range run.Attempts {
		if attempt.Outcome != nil {
			outcomes = append(outcomes, *attempt.Outcome)
		}
	}
	if tail := outcomes[len(outcomes)-3:]; !reflect.DeepEqual(tail, []string{"opened", "merged", "done"}) {
		t.Fatalf("last outcomes = %v, want opened merged done", tail)
	}
	if pull := run.PullRequest(); pull == nil || pull.Number != 201 {
		t.Fatalf("pull request = %+v, want 201", pull)
	}
	if calls := h.github.Calls(); len(calls) != 0 {
		t.Fatalf("github calls = %+v, want none", calls)
	}
}

// TestNeedsInfoCommentsTheQuestionsAndHolds: with the hold comment on, needs info
// comments the questions on the issue and holds.
func TestNeedsInfoCommentsTheQuestionsAndHolds(t *testing.T) {
	t.Parallel()
	belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
	belt.Policies.CommentOnHold = true
	h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}})
	h.Script.add("triage", "needs info")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	// The held run's comment is posted by a tick, so let the whole budget run.
	h.Settle()
	calls := h.github.Calls()
	if len(calls) == 0 || calls[0].Kind != github.FakeCallComment {
		t.Fatalf("calls = %+v, want a comment first", calls)
	}
	if calls[0].Number != 142 {
		t.Fatalf("comment on %d, want 142", calls[0].Number)
	}
	if !strings.Contains(calls[0].Body, "Which page shows the problem?") {
		t.Fatalf("comment body = %q", calls[0].Body)
	}
}

// TestAnAnswerContinuesTheHeldSessionAndTheRunMovesOn: an answer continues the held session and the run moves on.
func TestAnAnswerContinuesTheHeldSessionAndTheRunMovesOn(t *testing.T) {
	t.Parallel()
	belt := core.DefaultsBelt([]string{"acme/billing"}, "agent", core.DefaultsBuiltInAgents(), true)
	high := core.ThinkingHigh
	belt.Stages[0].Thinking = &high
	h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}})
	c := h.Coord()
	h.Script.add("triage", "needs info", "ready to spec")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	held := h.Run().Attempts[0]
	if spec := c.Spec(*h.Run(), held, ""); spec == nil || spec.Thinking == nil || *spec.Thinking != core.ThinkingHigh {
		t.Fatalf("held spec thinking = %+v, want high", spec)
	}
	answer := "  The invoices page.  "
	c.Answer(h.Run().ID, &answer)
	h.Until("r := h.nowRun(); return r != nil && r.StageID == \"spec\"", func() bool { r := h.nowRun(); return r != nil && r.StageID == "spec" })
	run := h.Run()
	if got := optionalTexts(attemptOutcomes(run.Attempts[:2])); !reflect.DeepEqual(got, []string{"needs info", "ready to spec"}) {
		t.Fatalf("outcomes = %v", got)
	}
	second := run.Attempts[1]
	if second.Continues == nil || *second.Continues != held.ID {
		t.Fatalf("continues = %v, want %s", second.Continues, held.ID)
	}
	if !core.SamePointee(second.SessionID, held.SessionID) {
		t.Fatal("continued attempt has another session id")
	}
	if !core.SamePointee(second.AgentName, held.Session) {
		t.Fatalf("agent name = %v, want %v", second.AgentName, held.Session)
	}
	if !core.SamePointee(second.Note, new("The invoices page.")) {
		t.Fatalf("note = %v", second.Note)
	}
	if spec := c.Spec(*run, second, ""); spec == nil || spec.Thinking == nil || *spec.Thinking != core.ThinkingHigh {
		t.Fatalf("continued spec thinking = %+v, want high", spec)
	}
	brief := readText(t, filepath.Join(c.Paths().AttemptFolder(*run, second), "brief.md"))
	if !strings.Contains(brief, "## Their answer\n\nThe invoices page.") {
		t.Fatal("brief lacks their answer")
	}
	// Spec starts its own session.
	h.Until("r := h.nowRun(); return r != nil && len(r.Attempts) == 3", func() bool { r := h.nowRun(); return r != nil && len(r.Attempts) == 3 })
	third := h.Run().Attempts[2]
	if core.SamePointee(third.SessionID, held.SessionID) {
		t.Fatal("spec reused the held session")
	}
	if third.Continues != nil {
		t.Fatalf("spec continues %v, want none", third.Continues)
	}
}

// TestClosingTheIssueDropsTheRun: closing the issue drops its run.
func TestClosingTheIssueDropsTheRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Tick()
	issue, err := h.github.Issue(ctxBackground, "acme/billing", 142)
	if err != nil {
		t.Fatal(err)
	}
	issue.IsOpen = false
	h.github.Put(issue)
	h.Coord().CheckNow(ctxBackground)
	run := h.Run()
	if run.State != core.RunStateDropped {
		t.Fatalf("state = %s, want dropped", run.State)
	}
	if run.Reason == nil || *run.Reason != "the issue was closed" {
		t.Fatalf("reason = %v", run.Reason)
	}
}

// TestARepositoryThatCantBeCheckedLeavesItsRunsAlone: a repository that fails to check leaves its runs be until it works.
func TestARepositoryThatCantBeCheckedLeavesItsRunsAlone(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Tick()
	issue, err := h.github.Issue(ctxBackground, "acme/billing", 142)
	if err != nil {
		t.Fatal(err)
	}
	issue.IsOpen = false
	h.github.Put(issue)
	h.github.SetFailing("acme/billing", true)
	c.CheckNow(ctxBackground)
	if run := h.Run(); run == nil || !run.State.IsActive() {
		t.Fatalf("run = %+v, want still active", run)
	}
	found := slices.ContainsFunc(c.Notices(), func(notice Notice) bool {
		return strings.HasPrefix(notice.Text, "Could not check acme/billing")
	})
	if !found {
		t.Fatal("no notice about the failed check")
	}
	h.github.SetFailing("acme/billing", false)
	c.CheckNow(ctxBackground)
	if run := h.Run(); run.State != core.RunStateDropped {
		t.Fatalf("state = %s, want dropped", run.State)
	}
}

// TestFetchReturnsBeforeSlowAgentStartupFinishes: fetching returns before a slow session starts.
func TestFetchReturnsBeforeSlowAgentStartupFinishes(t *testing.T) {
	t.Parallel()
	gate := &startGate{entered: make(chan struct{}), release: make(chan struct{})}
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 10_000 }, testkitNow, nil)
	h := newHarness(t, harnessOptions{StageDuration: 10_000, Runner: func(script *harnessScript) runner.AgentRunner {
		return gatedRunner{SimulatedRunner: sim, gate: gate}
	}})
	c := h.Coord()
	returned := make(chan struct{})
	go func() {
		c.FetchNow(ctxBackground)
		close(returned)
	}()
	// Hold checkout and session work open after intake has finished.
	<-gate.entered
	<-returned
	if c.Checking() {
		t.Fatal("still checking after fetch returned")
	}
	if c.LastCheck() == nil {
		t.Fatal("no last check")
	}
	close(gate.release)
	c.Idle()
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateRunning", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateRunning })
	if run := h.Run(); run.State != core.RunStateRunning {
		t.Fatalf("state = %s, want running", run.State)
	}
}

// twoIssues is the two labelled issues the build-slot tests use.
func twoIssues() []core.Subject {
	return []core.Subject{harnessIssue(141, "Issue 141", "agent"), harnessIssue(142, "Issue 142", "agent")}
}

// TestBuildRunsOneAtATime: only one run builds at a time; the other queues.
func TestBuildRunsOneAtATime(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: twoIssues()})
	c := h.Coord()
	h.Until("runs := h.nowRuns() ...", func() bool {
		runs := h.nowRuns()
		if len(runs) != 2 {
			return false
		}
		return !slices.ContainsFunc(runs, func(run core.Run) bool {
			return run.State != core.RunStateWaiting
		})
	})
	// 	// The decide tasks run at the await, the tick below; the Go kicks would run between ticks.
	holdKicks(t, c)
	for _, run := range c.Runs() {
		c.Decide(run.ID, "approved", nil)
	}
	c.Tick(ctxBackground)
	var states []string
	for _, run := range c.Runs() {
		if run.StageID == "build" {
			states = append(states, string(run.State))
		}
	}
	if want := []string{"queued", "running"}; !reflect.DeepEqual(sortedCopy(states), want) {
		t.Fatalf("build states = %v, want %v", states, want)
	}
}

// sortedCopy returns a sorted copy of values.
func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestAQueuedRunCanStartPastItsStagesSlots: a queued run starts now, past its stage's slots.
func TestAQueuedRunCanStartPastItsStagesSlots(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: twoIssues()})
	c := h.Coord()
	h.Until("runs := h.nowRuns() ...", func() bool {
		runs := h.nowRuns()
		if len(runs) != 2 {
			return false
		}
		return !slices.ContainsFunc(runs, func(run core.Run) bool {
			return run.State != core.RunStateWaiting
		})
	})
	// As in TestBuildRunsOneAtATime: the decide kicks wait for the one tick.
	holdKicks(t, c)
	for _, run := range c.Runs() {
		c.Decide(run.ID, "approved", nil)
	}
	c.Tick(ctxBackground)
	var queued *core.Run
	for _, run := range c.Runs() {
		if run.StageID == "build" && run.State == core.RunStateQueued {
			r := run
			queued = &r
			break
		}
	}
	if queued == nil {
		t.Fatal("no queued build run")
	}
	c.StartNow(ctxBackground, queued.ID)
	var states []core.RunState
	for _, run := range c.Runs() {
		if run.StageID == "build" {
			states = append(states, run.State)
		}
	}
	if !reflect.DeepEqual(states, []core.RunState{core.RunStateRunning, core.RunStateRunning}) {
		t.Fatalf("build states = %v, want running twice", states)
	}
}

// TestSavingABeltLeavesRunsInProgressOnTheirVersion: a saved belt changes only the runs that start after it.
func TestSavingABeltLeavesRunsInProgressOnTheirVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	h.Tick()
	belt := c.Belts()[0].Belt
	belt.Stages = append(append([]core.Stage(nil), belt.Stages[:2]...), belt.Stages[3:]...)
	belt.Stages[0].Thinking = new(core.ThinkingMax)
	saved, err := c.SaveBelt(belt)
	if err != nil {
		t.Fatalf("save belt: %v", err)
	}
	if saved.Version != 2 {
		t.Fatalf("version = %d, want 2", saved.Version)
	}
	if run := h.Run(); run.BeltVersion != 1 || len(run.Stages) != 10 || run.Stages[0].Thinking != nil {
		t.Fatalf("run kept version %d with %d stages, thinking %v", run.BeltVersion, len(run.Stages), run.Stages[0].Thinking)
	}
	h.github.Put(harnessIssue(150, "New", "agent"))
	c.CheckNow(ctxBackground)
	var newer *core.Run
	for _, run := range c.Runs() {
		if run.Subject.Number == 150 {
			r := run
			newer = &r
		}
	}
	if newer == nil {
		t.Fatal("no run for issue 150")
	}
	if newer.BeltVersion != 2 || len(newer.Stages) != 9 || newer.Stages[0].Thinking == nil || *newer.Stages[0].Thinking != core.ThinkingMax {
		t.Fatalf("new run version %d, stages %d, thinking %v", newer.BeltVersion, len(newer.Stages), newer.Stages[0].Thinking)
	}
	again := h.Restart()
	for _, run := range again.Runs() {
		switch run.Subject.Number {
		case 142:
			if run.Stages[0].Thinking != nil {
				t.Fatalf("issue 142 thinking = %v after restart, want none", run.Stages[0].Thinking)
			}
		case 150:
			if run.Stages[0].Thinking == nil || *run.Stages[0].Thinking != core.ThinkingMax {
				t.Fatalf("issue 150 thinking = %v after restart, want max", run.Stages[0].Thinking)
			}
		}
	}
}

// TestRunsSurviveARestart: runs load again after a restart in the state they were left.
func TestRunsSurviveARestart(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting })
	again := h.Restart()
	runs := again.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].State != core.RunStateWaiting {
		t.Fatalf("state = %s, want waiting", runs[0].State)
	}
	if len(runs[0].Attempts) != 3 {
		t.Fatalf("attempts = %d, want 3", len(runs[0].Attempts))
	}
}

// TestPreviousSessionsAndOutputSurviveARestart: sessions, logs and output of earlier attempts survive a restart.
func TestPreviousSessionsAndOutputSurviveARestart(t *testing.T) {
	t.Parallel()
	// Storage's ISO-8601 dates have second precision.
	h := newHarness(t, harnessOptions{Now: sessionSampleT0})
	c := h.Coord()
	h.Script.add("triage", "needs info", "needs info")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	first := h.Run()
	stage := "triage"
	note := "Try again"
	c.Resume(first.ID, &stage, &note, true, nil)
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld && len(r.Attempts) == 2", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld && len(r.Attempts) == 2 })
	finished := h.Run()
	if len(finished.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(finished.Attempts))
	}
	again := h.Restart()
	restored := again.Run(finished.ID)
	if restored == nil {
		t.Fatal("run lost on restart")
	}
	if diff := testkit.Diff(finished.Attempts, restored.Attempts); diff != "" {
		t.Fatalf("attempts changed on restart:\n%s", diff)
	}
	if sessions := core.AttemptPanelSessions(*restored); testkit.Diff(finished.Attempts, sessions) != "" {
		t.Fatal("attempt sessions changed on restart")
	}
	for _, attempt := range restored.Attempts {
		log := readText(t, filepath.Join(again.Paths().AttemptFolder(*restored, attempt), "session.log"))
		if !strings.Contains(log, "Done.") {
			t.Fatal("session log lost its output")
		}
		got := again.Output(*restored, attempt)
		if got == nil || !core.SamePointee(got, c.Output(*finished, attempt)) {
			t.Fatalf("output = %v, want the original output", got)
		}
	}
}

// TestRunsOnTheSameSubjectKeepSeparateSessions: two runs on one issue keep separate sessions and folders.
func TestRunsOnTheSameSubjectKeepSeparateSessions(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Now: sessionSampleT0})
	c := h.Coord()
	h.Script.add("triage", "needs info", "needs info")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	earlier := h.Run()
	belt := core.DefaultsBelt([]string{earlier.Subject.Repo}, "agent", core.DefaultsBuiltInAgents(), true)
	belt.ID = "another-belt"
	if _, err := c.SaveBelt(belt); err != nil {
		t.Fatalf("save belt: %v", err)
	}
	c.CheckNow(ctxBackground)
	h.Until("runs := h.nowRuns() ...", func() bool {
		runs := h.nowRuns()
		if len(runs) != 2 {
			return false
		}
		return !slices.ContainsFunc(runs, func(run core.Run) bool { return run.State != core.RunStateHeld })
	})
	var later *core.Run
	for _, run := range c.Runs() {
		if run.ID != earlier.ID {
			r := run
			later = &r
		}
	}
	if later == nil {
		t.Fatal("no later run")
	}
	if testkit.Diff(earlier.Subject, later.Subject) != "" {
		t.Fatal("runs have different subjects")
	}
	previous := core.AttemptPanelSessions(*earlier)[0]
	current := core.AttemptPanelSessions(*later)[0]
	if core.SamePointee(previous.Session, current.Session) {
		t.Fatal("both runs use the same session")
	}
	if c.Paths().AttemptFolder(*earlier, previous) == c.Paths().AttemptFolder(*later, current) {
		t.Fatal("both runs use the same attempt folder")
	}
	shown := core.AttemptPanelShown(core.AttemptPanelAttempts(later.StageID, *later), &previous.ID)
	if shown == nil || !core.SamePointee(shown.Session, current.Session) {
		t.Fatalf("shown session = %+v, want %v", shown, current.Session)
	}
	if run := c.Run(earlier.ID); run == nil || testkit.Diff(earlier.Attempts, run.Attempts) != "" {
		t.Fatal("earlier run's attempts changed")
	}
}

// TestASessionThatDisappearsUsesTheFallback: a session that vanished while the app was away is held with the fallback.
func TestASessionThatDisappearsUsesTheFallback(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Tick()
	// A fresh runner has none of the sessions the first coordinator started.
	again := h.RestartWith(h.idleSimulatedRunner())
	again.Tick(ctxBackground)
	again.Idle()
	runs := again.Runs()
	if len(runs) == 0 {
		t.Fatal("no runs after restart")
	}
	run := runs[0]
	if run.State != core.RunStateHeld {
		t.Fatalf("state = %s, want held", run.State)
	}
	if detail := run.Attempts[0].Detail; detail == nil || *detail != "the session ended before the agent finished" {
		t.Fatalf("detail = %v", detail)
	}
}

// TestRejectedCodexEffortFailsTheStageAndKeepsTheLevel: a rejected reasoning effort fails the stage and keeps the level.
func TestRejectedCodexEffortFailsTheStageAndKeepsTheLevel(t *testing.T) {
	t.Parallel()
	for _, thinking := range []core.Thinking{core.ThinkingXHigh, core.ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, testkitNow, nil)
			effort := &rejectingEffort{SimulatedRunner: sim}
			h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Runner: func(*harnessScript) runner.AgentRunner { return effort }})
			c := h.Coord()
			belt := c.Belts()[0].Belt
			var run core.Run
			h.Turn(func() {
				run = c.book.add(core.NewRun(core.RunArgs{
					Belt: belt, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Effort"}, Now: testkitNow(),
				}))
			})
			codex := core.BackendCodex
			brief := "Ship it"
			model := "gpt-test"
			if _, err := c.AdjustStage(run.ID, run.StageID, &model, &thinking, &brief, &codex); err != nil {
				t.Fatalf("adjust stage: %v", err)
			}
			h.Turn(func() { c.sessions.start(ctxBackground, run.ID, nil) })
			h.Turn(func() { c.sessions.check(ctxBackground) })
			for range 3 {
				h.Turn(func() { c.sessions.startQueued(ctxBackground) })
				h.Tick()
			}
			failed := c.Run(run.ID)
			if failed == nil {
				t.Fatal("run gone")
			}
			stage := failed.Stage()
			attempt := failed.Attempts[len(failed.Attempts)-1]
			diagnostic := "ERROR: unsupported model_reasoning_effort " + string(thinking)
			if failed.State != core.RunStateHeld {
				t.Fatalf("state = %s, want held", failed.State)
			}
			if failed.Reason == nil || *failed.Reason != stage.Name+" failed" {
				t.Fatalf("reason = %v", failed.Reason)
			}
			status := core.StatusStage(*failed, *stage, false, core.Pause{}, testkitNow())
			if status.Text != strings.ToUpper(stage.Name+" failed") {
				t.Fatalf("status text = %q", status.Text)
			}
			if attempt.Detail == nil || !strings.Contains(*attempt.Detail, "the agent exited with code 1") || !strings.Contains(*attempt.Detail, diagnostic) {
				t.Fatalf("detail = %q, want the exit code and %q", str(attempt.Detail), diagnostic)
			}
			if attempt.Outcome != nil {
				t.Fatalf("outcome = %v, want none", attempt.Outcome)
			}
			if _, err := os.Stat(filepath.Join(c.Paths().AttemptFolder(*failed, attempt), stage.Writes())); err == nil {
				t.Fatal("the stage wrote its file")
			}
			if got := stageNamed(*failed, stage.ID); got == nil || got.Thinking == nil || *got.Thinking != thinking {
				t.Fatalf("stage thinking = %v, want %s", got, thinking)
			}
			if len(failed.Attempts) != 1 {
				t.Fatalf("attempts = %d, want 1", len(failed.Attempts))
			}
			starts := effort.Starts()
			if len(starts) != 1 || starts[0].Thinking == nil || *starts[0].Thinking != thinking {
				t.Fatalf("starts = %+v, want one at %s", starts, thinking)
			}
			args := runner.CodexAdapter{}.Arguments(starts[0].SessionID, starts[0].Name, starts[0].Model, starts[0].Thinking)
			if !slices.Contains(args, "model_reasoning_effort="+string(thinking)) {
				t.Fatalf("codex arguments = %v, want the effort", args)
			}
			reloaded := h.RestartWith(effort)
			stored := reloaded.Run(run.ID)
			if stored == nil {
				t.Fatal("run lost on restart")
			}
			if got := stageNamed(*stored, stage.ID); got == nil || got.Thinking == nil || *got.Thinking != thinking {
				t.Fatalf("stored stage thinking = %v, want %s", got, thinking)
			}
			if stored.State != core.RunStateHeld {
				t.Fatalf("stored state = %s, want held", stored.State)
			}
			last := stored.Attempts[len(stored.Attempts)-1]
			if last.Detail == nil || !strings.Contains(*last.Detail, diagnostic) {
				t.Fatalf("stored detail = %v", last.Detail)
			}
			if got := len(effort.Starts()); got != 1 {
				t.Fatalf("starts after restart = %d, want 1", got)
			}
		})
	}
}

// str renders an optional string for a failure message.
func str(text *string) string {
	if text == nil {
		return "<nil>"
	}
	return *text
}

// TestABlankAnswerIsTheSameAsAnsweredInTheSession: a blank answer continues the held session without a note.
func TestABlankAnswerIsTheSameAsAnsweredInTheSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Script.add("triage", "needs info", "ready to spec")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	blank := "   "
	if _, err := h.Coord().AnswerCommand(h.Run().ID, &blank); err != nil {
		t.Fatalf("answer: %v", err)
	}
	h.Until("r := h.nowRun(); return r != nil && r.StageID == \"spec\"", func() bool { r := h.nowRun(); return r != nil && r.StageID == "spec" })
	if run := h.Run(); run.Attempts[1].Note != nil {
		t.Fatalf("note = %v, want none", run.Attempts[1].Note)
	}
}

// TestADecisionOnAParkedRunChangesNothingAndIsNotAnError: deciding a parked run changes nothing.
func TestADecisionOnAParkedRunChangesNothingAndIsNotAnError(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.Script.add("triage", "needs info")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateHeld", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateHeld })
	c := h.Coord()
	parked, err := c.ParkCommand(h.Run().ID)
	if err != nil {
		t.Fatalf("park: %v", err)
	}
	after, err := c.DecideCommand(h.Run().ID, "approved", nil)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if diff := testkit.Diff(parked, after); diff != "" {
		t.Fatalf("decided run differs from the parked one:\n%s", diff)
	}
	if len(after.Attempts) != len(parked.Attempts) {
		t.Fatalf("attempts = %d, want %d", len(after.Attempts), len(parked.Attempts))
	}
}
