package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
)

// handoffRunner is a simulated runner whose sessions stay open until the attempt
// folder's handoff record holds the launch's token, as the tmux runner's
// interactive sessions do. The test writes the output itself. Every usage read
// reports the same spend, so a second charge would show.
type handoffRunner struct {
	*runner.SimulatedRunner
	mu     sync.Mutex
	starts []runner.SessionSpec
	usages int
}

// Start records the launch, then starts it.
func (r *handoffRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	r.mu.Lock()
	r.starts = append(r.starts, spec)
	r.mu.Unlock()
	return r.SimulatedRunner.Start(ctx, spec)
}

// Status is finished, with no exit code, once the launch handed back; running before.
func (r *handoffRunner) Status(ctx context.Context, spec runner.SessionSpec) runner.SessionStatus {
	if runner.CompletionHandedBack(spec.AttemptFolder, spec.CompletionToken) {
		return runner.SessionStatus{Kind: runner.SessionStatusFinished}
	}
	return runner.SessionStatus{Kind: runner.SessionStatusRunning}
}

// Usage is 5 nanos each time it is read.
func (r *handoffRunner) Usage(ctx context.Context, spec runner.SessionSpec) *core.Usage {
	r.mu.Lock()
	r.usages++
	r.mu.Unlock()
	usage := core.NewUsage(core.UsageArgs{CostNanos: new(5)})
	return &usage
}

// launches is a copy of every spec started so far.
func (r *handoffRunner) launches() []runner.SessionSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runner.SessionSpec(nil), r.starts...)
}

// interactiveHarness is a harness on a handoffRunner whose first stage runs on backend.
func interactiveHarness(t *testing.T, backend core.Backend, issues []core.Subject) (*harness, *handoffRunner) {
	t.Helper()
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.Stages[0].Runner = &backend
	belt.Stages[0].Slots = 1
	var handoff *handoffRunner
	h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}, Issues: issues, Runner: func(script *harnessScript) runner.AgentRunner {
		handoff = &handoffRunner{SimulatedRunner: runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, nil, script.next)}
		return handoff
	}})
	return h, handoff
}

// interactiveFirstLaunch waits for the first stage's session and returns its spec.
func interactiveFirstLaunch(t *testing.T, h *harness, handoff *handoffRunner) runner.SessionSpec {
	t.Helper()
	h.Until("the first stage launched", func() bool { return len(handoff.launches()) > 0 })
	return handoff.launches()[0]
}

// TestEveryRunnerLaunchesAStageInteractivelyAndCanBeTakenOver checks that a new
// stage launches interactively on each runner with its brief, checkout, model and
// handoff token saved first, and that Take over is offered until the run pauses.
func TestEveryRunnerLaunchesAStageInteractivelyAndCanBeTakenOver(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			h, handoff := interactiveHarness(t, backend, nil)
			spec := interactiveFirstLaunch(t, h, handoff)
			run := h.Run()
			attempt := run.Attempts[len(run.Attempts)-1]
			if !spec.Interactive || spec.Resume || spec.Backend != backend || strings.TrimSpace(spec.Model) == "" {
				t.Fatalf("launch = interactive %v resume %v %s %q, want a fresh interactive %s launch with a model", spec.Interactive, spec.Resume, spec.Backend, spec.Model, backend)
			}
			if attempt.LaunchMode == nil || *attempt.LaunchMode != core.AttemptLaunchInteractive || attempt.CompletionToken == nil || *attempt.CompletionToken != spec.CompletionToken {
				t.Fatalf("attempt = mode %v token %v, want interactive with the launch's token %q", attempt.LaunchMode, attempt.CompletionToken, spec.CompletionToken)
			}
			if !strings.Contains(spec.Prompt, "Outcome:") {
				t.Fatalf("the brief has no outcome footer:\n%s", spec.Prompt)
			}
			// Copilot finishes on its own task completion, so its brief asks for no handoff.
			handsBack := strings.HasSuffix(spec.Prompt, core.BriefHandoff(spec.AttemptFolder, spec.CompletionToken))
			if handsBack != backend.FinishesOnHandoff() || (!handsBack && strings.Contains(spec.Prompt, "## Handing the work back")) {
				t.Fatalf("handoff in the brief = %v, want %v:\n%s", handsBack, backend.FinishesOnHandoff(), spec.Prompt)
			}
			if spec.Directory == "" || attempt.Settings == nil || attempt.Settings.Model == "" {
				t.Fatalf("checkout %q, settings %+v", spec.Directory, attempt.Settings)
			}
			if note := h.coord.HeadlessNote(*run, attempt); note != nil {
				t.Errorf("an interactive attempt has the headless note %q", *note)
			}
			if offer := h.coord.TakeOverOffer(*run, attempt); !offer.Enabled {
				t.Errorf("take over refused: %v", offer.Reason)
			}
			mode, err := h.coord.TerminalModeForSession(*attempt.Session, true, false)
			if err != nil || mode == nil || mode.ReadOnly {
				t.Errorf("take over mode = %+v, %v; want writable", mode, err)
			}
			if mode, _ := h.coord.TerminalModeForSession(*attempt.Session, false, false); mode == nil || !mode.ReadOnly {
				t.Errorf("watching is not read-only: %+v", mode)
			}
			h.coord.PauseRun(context.Background(), run.ID)
			h.coord.Idle()
			paused := h.Run()
			if offer := h.coord.TakeOverOffer(*paused, paused.Attempts[len(paused.Attempts)-1]); offer.Enabled {
				t.Error("a paused attempt can be taken over")
			}
		})
	}
}

// TestAnInteractiveStageFinishesOnlyOnItsOwnHandoffAndRoutesOnce checks that a
// written output, an Outcome line and a stale token leave the stage running, that
// the launch's token routes it once with one charge, and that finishing frees
// the stage's slot for the queued run.
func TestAnInteractiveStageFinishesOnlyOnItsOwnHandoffAndRoutesOnce(t *testing.T) {
	t.Parallel()
	issues := []core.Subject{
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "One", Body: "One.", Labels: []string{"agent"}, IsOpen: true},
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 143, Title: "Two", Body: "Two.", Labels: []string{"agent"}, IsOpen: true},
	}
	h, handoff := interactiveHarness(t, core.BackendClaude, issues)
	spec := interactiveFirstLaunch(t, h, handoff)
	stage := findStage(*h.Run(), h.Run().StageID)
	writeText(t, spec.Output, "# Output\n\nOutcome: "+stage.Outcomes[0].Name+"\n")
	writeText(t, runner.CompletionRecord(spec.AttemptFolder), core.AttemptCompletionToken("someone-else", nil)+"\n")
	h.Settle()
	if got := len(handoff.launches()); got != 1 {
		t.Fatalf("%d launches while the first stage holds its slot, want 1", got)
	}
	runID := ""
	for _, run := range h.coord.Runs() {
		if len(run.Attempts) > 0 {
			runID = run.ID
			if !run.Attempts[0].IsOpen() {
				t.Fatal("the stage finished without its handoff")
			}
		}
	}
	writeText(t, runner.CompletionRecord(spec.AttemptFolder), spec.CompletionToken+"\n")
	h.Until("the queued run takes the freed slot", func() bool { return len(handoff.launches()) >= 2 })
	h.Settle()
	run := h.coord.Run(runID)
	first := run.Attempts[0]
	if first.IsOpen() || first.Outcome == nil || *first.Outcome != stage.Outcomes[0].Name {
		t.Fatalf("first attempt = open %v outcome %v, want %q", first.IsOpen(), first.Outcome, stage.Outcomes[0].Name)
	}
	if run.StageID == stage.ID && run.State == core.RunStateRunning && len(run.Attempts) == 1 {
		t.Fatal("the run did not move on")
	}
	if first.Usage == nil || first.Usage.CostNanos == nil || *first.Usage.CostNanos != 5 {
		t.Fatalf("usage = %+v, want one charge of 5", first.Usage)
	}
	if second := handoff.launches()[1]; second.Name == spec.Name {
		t.Fatal("the first session was launched twice")
	}
}

// TestAHandoffWithoutAKnownOutcomeFailsTheStage checks that handing back with no
// output, or with an outcome the stage does not have, reports the stage failure.
func TestAHandoffWithoutAKnownOutcomeFailsTheStage(t *testing.T) {
	t.Parallel()
	for name, output := range map[string]string{"missing": "", "unknown": "# Output\n\nOutcome: not configured\n"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, handoff := interactiveHarness(t, core.BackendCodex, nil)
			spec := interactiveFirstLaunch(t, h, handoff)
			if output != "" {
				writeText(t, spec.Output, output)
			}
			writeText(t, runner.CompletionRecord(spec.AttemptFolder), spec.CompletionToken+"\n")
			h.Until("the stage failed", func() bool { r := h.nowRun(); return r != nil && !r.Attempts[0].IsOpen() })
			attempt := h.Run().Attempts[0]
			want := map[string]string{"missing": "without writing", "unknown": "no known outcome"}[name]
			if attempt.Outcome != nil || attempt.Detail == nil || !strings.Contains(*attempt.Detail, want) {
				t.Fatalf("attempt = outcome %v detail %v, want a failure about %q", attempt.Outcome, attempt.Detail, want)
			}
		})
	}
}

// TestAFailedInteractiveLaunchReportsAFailure checks that a launch the runner
// refuses fails its stage instead of leaving it running.
func TestAFailedInteractiveLaunchReportsAFailure(t *testing.T) {
	t.Parallel()
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, nil, nil)
	refused := "Handing the work back"
	sim.RefusePrompt(&refused)
	h := newHarness(t, harnessOptions{Belts: []core.Belt{belt}, Runner: func(*harnessScript) runner.AgentRunner { return sim }})
	h.Until("the launch failed", func() bool { r := h.nowRun(); return r != nil && len(r.Attempts) > 0 && !r.Attempts[0].IsOpen() })
	if detail := h.Run().Attempts[0].Detail; detail == nil || !strings.Contains(*detail, "could not start") {
		t.Fatalf("detail = %v, want the launch failure", detail)
	}
}

// TestARestartReconnectsToAnInteractiveSessionWithoutRelaunching checks that a
// restarted daemon keeps waiting on the launched session and its token, finishes
// it on its handoff without a second launch, and leaves older attempts as they ran.
func TestARestartReconnectsToAnInteractiveSessionWithoutRelaunching(t *testing.T) {
	t.Parallel()
	h, handoff := interactiveHarness(t, core.BackendPi, nil)
	spec := interactiveFirstLaunch(t, h, handoff)
	restarted := h.RestartWith(handoff)
	restarted.Tick(context.Background())
	restarted.Idle()
	run := restarted.Run(h.Run().ID)
	attempt := run.Attempts[0]
	restarted.Lock()
	rebuilt := restarted.sessions.spec(*run, attempt, "")
	restarted.EndTurn()
	if rebuilt == nil || !rebuilt.Interactive || rebuilt.CompletionToken != spec.CompletionToken || rebuilt.Name != spec.Name {
		t.Fatalf("rebuilt spec = %+v, want the launched session and its token", rebuilt)
	}
	stage := findStage(*run, run.StageID)
	writeText(t, spec.Output, "# Output\n\nOutcome: "+stage.Outcomes[0].Name+"\n")
	writeText(t, runner.CompletionRecord(spec.AttemptFolder), spec.CompletionToken+"\n")
	for range 5 {
		restarted.Tick(context.Background())
		restarted.Idle()
	}
	if finished := restarted.Run(run.ID).Attempts[0]; finished.IsOpen() || finished.Outcome == nil {
		t.Fatalf("after the handoff the attempt is open %v, outcome %v", finished.IsOpen(), finished.Outcome)
	}
	launched := 0
	for _, start := range handoff.launches() {
		if start.Name == spec.Name {
			launched++
		}
	}
	if launched != 1 {
		t.Fatalf("the session was launched %d times, want once", launched)
	}

	// Attempts saved before stages ran interactively keep how they ran.
	older := attempt.Clone()
	older.LaunchMode, older.CompletionToken = nil, nil
	pi := core.BackendPi
	older.Settings = &core.SessionSettings{Model: "model", Runner: &pi}
	restarted.Lock()
	headless := restarted.sessions.spec(*run, older, "")
	note := restarted.headlessNote(*run, older)
	offer := restarted.takeOverOfferLocked(*run, older)
	copilot := core.BackendCopilot
	flagged := older.Clone()
	flagged.Settings = &core.SessionSettings{Model: "model", Runner: &copilot}
	yes := true
	flagged.CopilotInteractive = &yes
	copilotSpec := restarted.sessions.spec(*run, flagged, "")
	copilotNote := restarted.headlessNote(*run, flagged)
	restarted.EndTurn()
	if headless == nil || headless.Interactive || headless.CompletionToken != "" {
		t.Errorf("an older Pi attempt = %+v, want headless", headless)
	}
	if note == nil || offer.Enabled || offer.Reason == nil || *offer.Reason != *note {
		t.Errorf("an older Pi attempt: note %v, offer %+v; want the headless refusal", note, offer)
	}
	if copilotSpec == nil || !copilotSpec.Interactive || copilotSpec.CompletionToken != "" || copilotNote != nil {
		t.Errorf("an older interactive Copilot attempt = %+v, note %v; want interactive on its task event", copilotSpec, copilotNote)
	}
}

// TestAResumedTurnWaitsForItsOwnHandoff checks that resuming a paused interactive
// turn keeps its attempt, saves a new token and sends it after Continue, so the
// interrupted turn's handoff cannot finish it.
func TestAResumedTurnWaitsForItsOwnHandoff(t *testing.T) {
	t.Parallel()
	h, handoff := interactiveHarness(t, core.BackendClaude, nil)
	spec := interactiveFirstLaunch(t, h, handoff)
	id := h.Run().ID
	h.coord.PauseRun(context.Background(), id)
	h.coord.Idle()
	// The interrupted turn hands back late; the resumed one must not take it.
	writeText(t, runner.CompletionRecord(spec.AttemptFolder), spec.CompletionToken+"\n")
	h.coord.ResumeRun(context.Background(), id)
	h.Until("Continue launched", func() bool { return len(handoff.launches()) >= 2 })
	resumed := handoff.launches()[1]
	run := h.Run()
	attempt := run.Attempts[0]
	if len(run.Attempts) != 1 || !resumed.Resume || !resumed.Interactive || resumed.Name != spec.Name {
		t.Fatalf("resume = %+v with %d attempts, want the same interactive session", resumed, len(run.Attempts))
	}
	if resumed.CompletionToken == spec.CompletionToken || attempt.CompletionToken == nil || *attempt.CompletionToken != resumed.CompletionToken {
		t.Fatalf("resumed token %q, saved %v, first %q; want a new saved token", resumed.CompletionToken, attempt.CompletionToken, spec.CompletionToken)
	}
	if !strings.HasPrefix(resumed.Prompt, core.RunPauseContinuePrompt+"\n") || !strings.Contains(resumed.Prompt, resumed.CompletionToken) {
		t.Fatalf("resume prompt = %q", resumed.Prompt)
	}
	if resumed.Model != spec.Model || resumed.Backend != spec.Backend || !core.SamePointee(resumed.Thinking, spec.Thinking) {
		t.Fatalf("resume settings = %s %s %v, want the attempt's %s %s %v", resumed.Backend, resumed.Model, resumed.Thinking, spec.Backend, spec.Model, spec.Thinking)
	}
	h.Settle()
	if !h.Run().Attempts[0].IsOpen() {
		t.Fatal("the interrupted turn's handoff finished the resumed turn")
	}
	if _, err := os.Stat(filepath.Join(spec.AttemptFolder, "brief.md")); err != nil {
		t.Fatal(err)
	}
}
