// Coordinator tests of run pauses: pausing one run stops its session and resumes it with one continue
// on the same attempt, repeated and bypass controls do nothing, an incomplete pause keeps its barrier,
// other runs keep working, edits and closed issues are reconciled on resume, a confirmed agent frees
// its slot, a missing checkout or Codex rollout stops only its route, and a launch failure or an
// ambiguous delivery is never sent twice.

package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// coordLongRun is the long-session setup: sessions run for an hour, so a run stays running.
func coordLongRun(t *testing.T, issues []core.Subject, belts []core.Belt) *harness {
	t.Helper()
	opts := harnessOptions{StageDuration: 3600}
	if issues != nil {
		opts.Issues = issues
	}
	if belts != nil {
		opts.Belts = belts
	}
	return newHarness(t, opts)
}

// runningRun is the first run once it is running.
func runningRun(h *harness) {
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
}

// pausePhase is the pause phase of run id, or "" when it is not paused.
func pausePhase(h *harness, id string) core.RunPausePhase {
	run := h.coord.Run(id)
	if run == nil || run.Pause == nil {
		return ""
	}
	return run.Pause.Phase
}

// continueCount counts the continue prompts the simulated runner received.
func continueCount(sim *runner.SimulatedRunner) int {
	n := 0
	for _, prompt := range sim.Prompts() {
		// An interactive turn's Continue carries its new handoff after it.
		if prompt == core.RunPauseContinuePrompt || strings.HasPrefix(prompt, core.RunPauseContinuePrompt+"\n") {
			n++
		}
	}
	return n
}

// noContinue reports whether no continue prompt reached the simulated runner.
func noContinue(sim *runner.SimulatedRunner) bool { return continueCount(sim) == 0 }

// TestPauseStopsAGeneratingRunAndResumeSendsContinueOnceOnTheSameAttempt
func TestPauseStopsAGeneratingRunAndResumeSendsContinueOnceOnTheSameAttempt(t *testing.T) {
	t.Parallel()
	h := coordLongRun(t, nil, nil)
	runningRun(h)
	running := h.Run()
	attempt := running.CurrentAttempt()
	if attempt == nil {
		t.Fatal("no current attempt")
	}
	baseline := attempt.ReadOnlyBaseline
	sim := simOf(h.coord)
	h.coord.PauseRun(context.Background(), running.ID)
	if h.Run().Pause == nil || h.Run().Pause.Phase != core.RunPausePhasePaused {
		t.Fatalf("pause = %+v, want paused", h.Run().Pause)
	}
	if h.Run().State != core.RunStateRunning || h.Run().StageID != running.StageID || len(h.Run().Attempts) != 1 {
		t.Fatalf("run after pausing = %s at %s, attempts %d", h.Run().State, h.Run().StageID, len(h.Run().Attempts))
	}
	if !core.SamePointee(h.Run().CurrentAttempt().ReadOnlyBaseline, baseline) {
		t.Fatal("the read-only baseline changed when pausing")
	}
	promptsBefore := len(sim.Prompts())
	for range 3 {
		h.Tick()
	}
	if h.Run().StageID != running.StageID || h.Run().Attempts[len(h.Run().Attempts)-1].Outcome != nil {
		t.Fatal("the paused run moved on")
	}
	if len(sim.Prompts()) != promptsBefore {
		t.Fatal("a prompt was sent while paused")
	}

	h.coord.Lock()
	h.coord.book.update(running.ID, nil, nil, func(r *core.Run) {
		for i := range r.Stages {
			if r.Stages[i].ID == r.StageID {
				r.Stages[i].Model = "changed/model"
				thinking := core.ThinkingHigh
				r.Stages[i].Thinking = &thinking
			}
		}
	})
	h.coord.EndTurn()
	h.coord.ResumeRun(context.Background(), running.ID)
	h.coord.Idle()
	after := h.Run()
	if after.Pause != nil || len(after.Attempts) != 1 {
		t.Fatalf("after resuming = pause %+v, attempts %d; want none and 1", after.Pause, len(after.Attempts))
	}
	first := after.Attempts[0]
	if first.ID != attempt.ID || first.Folder != attempt.Folder || !core.SamePointee(first.SessionID, attempt.SessionID) {
		t.Fatalf("attempt changed on resume: %+v", first)
	}
	if continueCount(sim) != 1 {
		t.Fatalf("continue prompts = %d, want 1", continueCount(sim))
	}
	turns := filepath.Join(h.coord.Paths().AttemptFolder(*after, first), "turns.log")
	transcript := readText(t, turns)
	if !strings.Contains(transcript, core.RunPauseContinuePrompt) || strings.Index(transcript, core.RunPauseContinuePrompt) <= 0 {
		t.Fatalf("turns.log does not carry the continue after the first turn: %q", transcript)
	}

	h.coord.PauseRun(context.Background(), running.ID)
	if pausePhase(h, running.ID) != core.RunPausePhasePaused {
		t.Fatal("the second pause did not take")
	}
	h.coord.ResumeRun(context.Background(), running.ID)
	h.coord.Idle()
	if continueCount(sim) != 2 {
		t.Fatalf("continue prompts after the second resume = %d, want 2", continueCount(sim))
	}
	if h.Run().Attempts == nil || len(h.Run().Attempts) != 1 {
		t.Fatal("the second resume made an attempt")
	}
	if settings := h.Run().Attempts[0].Settings; settings != nil && settings.Model == "changed/model" {
		t.Fatal("the resumed attempt took the changed model")
	}
}

// TestAnIncompletePauseKeepsTheBarrierAndRetryCanFinishIt
func TestAnIncompletePauseKeepsTheBarrierAndRetryCanFinishIt(t *testing.T) {
	t.Parallel()
	h := coordLongRun(t, nil, nil)
	runningRun(h)
	session := runSession(h.Run())
	sim := simOf(h.coord)
	sim.FailInterrupts(map[string]struct{}{session: {}})
	id := h.Run().ID
	h.coord.PauseRun(context.Background(), id)
	if pausePhase(h, id) != core.RunPausePhaseIncomplete {
		t.Fatalf("phase = %s, want incomplete", pausePhase(h, id))
	}
	if reason := h.Run().Pause.Reason; reason == nil || !strings.Contains(*reason, session) {
		t.Fatalf("reason = %v, want the session %s", reason, session)
	}
	for range 2 {
		h.Tick()
	}
	if h.Run().StageID != "triage" {
		t.Fatalf("stage = %s, want triage", h.Run().StageID)
	}
	sim.FailInterrupts(map[string]struct{}{})
	h.coord.PauseRun(context.Background(), id)
	if pausePhase(h, id) != core.RunPausePhasePaused {
		t.Fatalf("phase after retry = %s, want paused", pausePhase(h, id))
	}
}

// TestAnotherRunKeepsWorking
func TestAnotherRunKeepsWorking(t *testing.T) {
	t.Parallel()
	other := coordSubject(143, "Other")
	h := coordLongRun(t, []core.Subject{coordSubject(142, "Export invoices as CSV"), other}, nil)
	h.Until("runningCount(h.nowRuns()) == 2", func() bool { return runningCount(h.nowRuns()) == 2 })
	var first core.Run
	for _, run := range h.coord.Runs() {
		if run.Subject.Number == 142 {
			first = run
		}
	}
	h.coord.PauseRun(context.Background(), first.ID)
	for range 2 {
		h.Tick()
	}
	var second *core.Run
	for _, run := range h.coord.Runs() {
		if run.Subject.Number == 143 {
			r := run
			second = &r
		}
	}
	if second == nil || second.Pause != nil || second.State != core.RunStateRunning {
		t.Fatalf("second run = %+v, want running, not paused", second)
	}
	if pausePhase(h, first.ID) != core.RunPausePhasePaused {
		t.Fatal("the first run is not paused")
	}
}

// runningCount counts the running runs in runs.
func runningCount(runs []core.Run) int {
	n := 0
	for _, run := range runs {
		if run.State == core.RunStateRunning {
			n++
		}
	}
	return n
}

// continueCountOf counts the continue prompts the harness's simulated runner received.
func continueCountOf(h *harness) int { return continueCount(simOf(h.coord)) }

// TestAnEditDuringPauseWaitsForResumeAndAReadFailureOffersRetry
func TestAnEditDuringPauseWaitsForResumeAndAReadFailureOffersRetry(t *testing.T) {
	t.Parallel()
	h := coordLongRun(t, nil, nil)
	runningRun(h)
	id := h.Run().ID
	h.coord.PauseRun(context.Background(), id)
	edited := coordSubject(142, "Edited title")
	h.github.Put(edited)
	h.Tick()
	if got := h.Run().Subject.Title; got != "Export invoices as CSV" {
		t.Fatalf("title while paused = %q, want the original", got)
	}
	h.github.FailIssueRead("acme/billing", 142, true)
	h.coord.ResumeRun(context.Background(), id)
	if pausePhase(h, id) != core.RunPausePhaseReconcileFailed {
		t.Fatalf("phase = %s, want reconcileFailed", pausePhase(h, id))
	}
	if reason := h.Run().Pause.Reason; reason == nil || !strings.Contains(*reason, "Could not read") {
		t.Fatalf("reason = %v, want a read failure", reason)
	}
	if !noContinue(simOf(h.coord)) {
		t.Fatal("a continue was sent after a read failure")
	}
	h.github.FailIssueRead("acme/billing", 142, false)
	h.coord.ResumeRun(context.Background(), id)
	if h.Run().Pause != nil {
		t.Fatal("the pause did not clear after the retry")
	}
	if got := h.Run().Subject.Title; got != "Edited title" {
		t.Fatalf("title after resume = %q, want Edited title", got)
	}
}

// TestConfirmedAgentsReleaseTheirSlotAndABeltOrGlobalPauseDoesNotClearYours
func TestConfirmedAgentsReleaseTheirSlotAndABeltOrGlobalPauseDoesNotClearYours(t *testing.T) {
	t.Parallel()
	belt := issuesBelt(nil)
	belt.Stages = []core.Stage{core.DefaultsStageForAgent(core.SampleAgent("triage"), nil, nil, nil, core.TriggerKindIssues)}
	belt.Stages[0].Slots = 1
	other := coordSubject(143, "Other")
	h := coordLongRun(t, []core.Subject{coordSubject(142, "Export invoices as CSV"), other}, []core.Belt{belt})
	h.Until("runningCount(h.nowRuns()) == 1 && len(h.nowRuns()) == 2", func() bool { return runningCount(h.nowRuns()) == 1 && len(h.nowRuns()) == 2 })
	var running, waiting core.Run
	for _, run := range h.coord.Runs() {
		switch run.State {
		case core.RunStateRunning:
			running = run
		case core.RunStateQueued:
			waiting = run
		}
	}
	h.coord.PauseRun(context.Background(), running.ID)
	if pausePhase(h, running.ID) != core.RunPausePhasePaused {
		t.Fatal("the running run is not paused")
	}
	h.Until("r := coordNowRun(h, waiting.ID); return r != nil && r.State == core.RunStateRunning", func() bool { r := coordNowRun(h, waiting.ID); return r != nil && r.State == core.RunStateRunning })
	if h.coord.Run(waiting.ID).Pause != nil {
		t.Fatal("the waiting run picked up a pause")
	}

	h.coord.SetGloballyPaused(context.Background(), true)
	h.coord.PauseRun(context.Background(), waiting.ID)
	if h.coord.Run(waiting.ID).Pause != nil {
		t.Fatal("a pause on a run while the app is paused was recorded")
	}
	h.coord.SetGloballyPaused(context.Background(), false)
	if pausePhase(h, running.ID) != core.RunPausePhasePaused {
		t.Fatal("the global pause cleared the run's own pause")
	}
	if err := h.coord.SetPaused(running.BeltID, true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	if err := h.coord.SetPaused(running.BeltID, false); err != nil {
		t.Fatalf("unpause belt: %v", err)
	}
	if pausePhase(h, running.ID) != core.RunPausePhasePaused {
		t.Fatal("the belt pause cleared the run's own pause")
	}
}

// TestAMissingCheckoutOrCodexRolloutStopsThatRouteAndASiblingRetryDoesNotRelaunch
func TestAMissingCheckoutOrCodexRolloutStopsThatRouteAndASiblingRetryDoesNotRelaunch(t *testing.T) {
	t.Parallel()
	h := coordLongRun(t, nil, nil)
	runningRun(h)
	id := h.Run().ID
	h.coord.PauseRun(context.Background(), id)
	directory := h.coord.Spec(*h.Run(), h.Run().Attempts[0], "")
	if directory == nil {
		t.Fatal("no session spec for the attempt")
	}
	if err := os.RemoveAll(directory.Directory); err != nil {
		t.Fatal(err)
	}
	h.coord.ResumeRun(context.Background(), id)
	continuation := h.Run().Attempts[0].Interruption.Continuation
	if continuation == nil || continuation.Status != core.AttemptInterruptionContinuationStatusFailed {
		t.Fatalf("continuation = %+v, want failed", continuation)
	}
	if continuation.Reason == nil || !strings.Contains(*continuation.Reason, "checkout") {
		t.Fatalf("reason = %v, want a checkout reason", continuation.Reason)
	}
	if err := os.MkdirAll(directory.Directory, 0o755); err != nil {
		t.Fatal(err)
	}
	h.coord.RetryContinuation(context.Background(), id, nil)
	h.coord.Idle()
	status := h.Run().Attempts[0].Interruption.Continuation.Status
	if status != core.AttemptInterruptionContinuationStatusLaunched && status != core.AttemptInterruptionContinuationStatusQueued && status != core.AttemptInterruptionContinuationStatusLaunching {
		t.Fatalf("status after the retry = %s, want launched, queued or launching", status)
	}

	again := coordLongRun(t, nil, nil)
	runningRun(again)
	second := again.Run().ID
	again.coord.PauseRun(context.Background(), second)
	again.coord.Lock()
	again.coord.book.update(second, nil, nil, func(r *core.Run) {
		if r.Attempts[0].Settings != nil {
			codex := core.BackendCodex
			r.Attempts[0].Settings.Runner = &codex
		}
		r.Attempts[0].Interruption.Conversation = nil
	})
	again.coord.EndTurn()
	again.coord.ResumeRun(context.Background(), second)
	again.coord.Idle()
	got := again.Run().Attempts[0].Interruption.Continuation
	if got == nil || got.Status != core.AttemptInterruptionContinuationStatusFailed {
		t.Fatalf("codex continuation = %+v, want failed", got)
	}
	if got.Reason == nil || !strings.Contains(*got.Reason, "Codex has no rollout") {
		t.Fatalf("codex reason = %v, want no rollout", got.Reason)
	}
	continues := continueCountOf(again)
	again.coord.RetryContinuation(context.Background(), second, nil)
	again.coord.Idle()
	if continueCountOf(again) != continues {
		t.Fatal("the retry relaunched the continue")
	}
}

// TestALaunchFailureCanBeRetriedAndAnAmbiguousDeliveryIsNotSentTwice
func TestALaunchFailureCanBeRetriedAndAnAmbiguousDeliveryIsNotSentTwice(t *testing.T) {
	t.Parallel()
	h := coordLongRun(t, nil, nil)
	runningRun(h)
	id := h.Run().ID
	h.coord.PauseRun(context.Background(), id)
	refused := core.RunPauseContinuePrompt
	sim := simOf(h.coord)
	sim.RefusePrompt(&refused)
	h.coord.ResumeRun(context.Background(), id)
	if h.Run().Attempts[0].Interruption.Continuation.Status != core.AttemptInterruptionContinuationStatusFailed {
		t.Fatal("the refused continue did not fail")
	}
	sim.RefusePrompt(nil)
	h.coord.RetryContinuation(context.Background(), id, nil)
	h.Until("r := h.nowRun() ...", func() bool {
		r := h.nowRun()
		return r != nil && r.Attempts[0].Interruption != nil && r.Attempts[0].Interruption.Continuation != nil &&
			r.Attempts[0].Interruption.Continuation.Status == core.AttemptInterruptionContinuationStatusLaunched
	})
	if continueCount(sim) != 1 {
		t.Fatalf("continue prompts = %d, want 1", continueCount(sim))
	}

	ambiguous := coordLongRun(t, nil, nil)
	runningRun(ambiguous)
	paused := ambiguous.Run().ID
	ambiguous.coord.PauseRun(context.Background(), paused)
	ambiguous.Turn(func() {
		ambiguous.coord.book.apply(paused, core.Event{Kind: core.EventKindResumePaused, Operation: "r"}, nil, nil, nil)
		ambiguous.coord.book.apply(paused, core.Event{Kind: core.EventKindContinuationsAdmitted}, nil, nil, nil)
		leg := "main"
		ambiguous.coord.book.apply(paused, core.Event{Kind: core.EventKindContinuationLaunching, Operation: "r", Leg: &leg}, nil, nil, nil)
	})
	ambiguous.Tick()
	status := ambiguous.Run().Attempts[0].Interruption.Continuation.Status
	if status != core.AttemptInterruptionContinuationStatusAmbiguous {
		t.Fatalf("status = %s, want ambiguous", status)
	}
	if !noContinue(simOf(ambiguous.coord)) {
		t.Fatal("an ambiguous continue was sent")
	}
	ambiguous.Tick()
	if status := ambiguous.Run().Attempts[0].Interruption.Continuation.Status; status != core.AttemptInterruptionContinuationStatusAmbiguous {
		t.Fatalf("status on the next tick = %s, want ambiguous", status)
	}
}

// TestAReadOnlyChangeSurvivesPauseResumeAndRestart
func TestAReadOnlyChangeSurvivesPauseResumeAndRestart(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	finish := false
	h := newHarness(t, harnessOptions{Runner: func(script *harnessScript) runner.AgentRunner {
		return runner.NewSimulatedRunner(func(runner.SessionSpec) float64 {
			mu.Lock()
			defer mu.Unlock()
			if finish {
				return 0
			}
			return 3600
		}, func() time.Time { return testkit.ContractTime }, script.next)
	}})
	runningRun(h)
	id := h.Run().ID
	spec := h.coord.Spec(*h.Run(), h.Run().Attempts[0], "")
	if spec == nil {
		t.Fatal("no spec")
	}
	baseline := h.Run().Attempts[0].ReadOnlyBaseline
	if err := os.WriteFile(filepath.Join(spec.Directory, "extra.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.coord.PauseRun(context.Background(), id)
	if !core.SamePointee(h.Run().Attempts[0].ReadOnlyBaseline, baseline) {
		t.Fatal("the baseline changed while pausing")
	}
	reopened := h.RestartWith(newSimulated(h.clock))
	if got := reopened.Run(id); got == nil || !core.SamePointee(got.Attempts[0].ReadOnlyBaseline, baseline) {
		t.Fatal("the reopened baseline differs")
	}
	if baseline == nil {
		t.Fatal("the baseline is nil")
	}
	reopened.ResumeRun(context.Background(), id)
	mu.Lock()
	finish = true
	mu.Unlock()
	for range 5 {
		reopened.Tick(context.Background())
		reopened.Idle()
	}
	if got := reopened.Run(id); got == nil || got.Attempts[0].Detail == nil || *got.Attempts[0].Detail != "the session changed its read-only checkout" {
		t.Fatalf("attempt detail = %+v, want the read-only checkout change", got)
	}
}
