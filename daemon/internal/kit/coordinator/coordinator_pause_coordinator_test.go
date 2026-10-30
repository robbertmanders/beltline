// Coordinator tests of pauses: the app-wide pause suspends the assistant and every session and leaves
// runs and schedules where they were, while your own decisions and answers still work; the belts' own
// pauses; and the pause survives a restart with the schedule's missed slots.

package coordinator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestPausingSuspendsTheAssistantAndEveryRunningSessionAndContinuingWakesThem
func TestPausingSuspendsTheAssistantAndEveryRunningSessionAndContinuingWakesThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordBuildThenWatch()}, StageDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	if err := h.coord.StartAssistant(context.Background(), "Add a belt for docs fixes"); err != nil {
		t.Fatalf("start assistant: %v", err)
	}
	sim := simOf(h.coord)
	session := runSession(h.Run())
	if session == "" || !sim.IsRunning(session) {
		t.Fatal("the stage session is not running")
	}
	h.coord.SetGloballyPaused(context.Background(), true)
	if !coordPaused(h.coord) {
		t.Fatal("the app is not paused")
	}
	pause := h.coord.Pause()
	if !pause.IsPaused || pause.PausedAt == nil || !pause.PausedAt.Equal(h.clock.Now()) {
		t.Fatalf("pause = %+v, want paused at the clock", pause)
	}
	if !sim.IsSuspended(session) || !sim.IsSuspended(coordAssistantSession(h.coord)) {
		t.Fatal("a session is not suspended while paused")
	}
	// A stopped process is still a running one: same attempt, same state.
	live := h.Run()
	if status := coordStatus(h.coord, *live); status.Kind != runner.SessionStatusRunning {
		t.Fatalf("status while paused = %v, want running", status.Kind)
	}
	if h.Run().State != core.RunStateRunning {
		t.Fatalf("state while paused = %s, want running", h.Run().State)
	}

	h.clock.Set(h.clock.Now().Add(time.Hour))
	h.coord.SetGloballyPaused(context.Background(), false)
	if coordPaused(h.coord) {
		t.Fatal("the app is still paused after continuing")
	}
	if sim.IsSuspended(session) || sim.IsSuspended(coordAssistantSession(h.coord)) {
		t.Fatal("a session stayed suspended after continuing")
	}
	after := h.coord.Pause()
	if after.PausedFor() != 3600 || after.PausedAt != nil {
		t.Fatalf("pause after continuing = %+v, want 3600 seconds and no start", after)
	}
}

// TestPausingSuspendsEveryLegOfASplitRun
func TestPausingSuspendsEveryLegOfASplitRun(t *testing.T) {
	t.Parallel()
	var sim *runner.SimulatedRunner
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordLegBelt()}, Runner: func(script *harnessScript) runner.AgentRunner {
		// Only Triage finishes; both legs' sessions keep going.
		sim = runner.NewSimulatedRunner(func(spec runner.SessionSpec) float64 {
			if spec.AgentID != nil && *spec.AgentID == "triage" {
				return 0
			}
			return 10_000
		}, func() time.Time { return testkit.ContractTime }, script.next)
		return sim
	}})
	h.Script.add("triage", "ready to implement")
	h.Until("run := h.nowRun() ...", func() bool {
		run := h.nowRun()
		return run != nil && len(h.nowRunningLegs(run)) == 2
	})
	run := h.Run()
	if !run.IsSplit() {
		t.Fatal("the run is not split")
	}
	var sessions []string
	for _, leg := range run.WorkingLegs() {
		stage := leg.Stage(*run)
		if leg.State != core.RunStateRunning || stage == nil || stage.Kind() != core.StageKindAgent {
			continue
		}
		stageID := core.ValueOr(leg.StageID, run.StageID)
		key := core.AttemptKey(stageID, &leg.ID)
		for i := len(run.Attempts) - 1; i >= 0; i-- {
			attempt := run.Attempts[i]
			if attempt.IsOpen() && core.AttemptKey(attempt.StageID, attempt.Leg) == key && attempt.Session != nil {
				sessions = append(sessions, *attempt.Session)
				break
			}
		}
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %v, want 2", sessions)
	}
	for _, session := range sessions {
		if !sim.IsRunning(session) {
			t.Fatalf("session %s is not running", session)
		}
	}
	h.coord.SetGloballyPaused(context.Background(), true)
	if !coordPaused(h.coord) {
		t.Fatal("the app is not paused")
	}
	for _, session := range sessions {
		if !sim.IsSuspended(session) {
			t.Fatalf("session %s is not suspended", session)
		}
	}
	for range 3 {
		h.Tick()
	}
	for _, leg := range h.Run().WorkingLegs() {
		if leg.State != core.RunStateRunning {
			t.Fatalf("leg %s = %s while paused, want running", leg.ID, leg.State)
		}
	}
	h.coord.SetGloballyPaused(context.Background(), false)
	for _, session := range sessions {
		if sim.IsSuspended(session) {
			t.Fatalf("session %s stayed suspended after continuing", session)
		}
	}
}

// nowRunningLegs returns the legs of run that are running, without the lock (for Until predicates).
func (h *harness) nowRunningLegs(run *core.Run) []core.Leg {
	var out []core.Leg
	for _, leg := range run.WorkingLegs() {
		if leg.State == core.RunStateRunning {
			out = append(out, leg)
		}
	}
	return out
}

// TestAConversationStartedWhilePausedIsSuspendedByTheNextPause
func TestAConversationStartedWhilePausedIsSuspendedByTheNextPause(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	if err := h.coord.StartAssistant(context.Background(), "Add a belt for docs fixes"); err != nil {
		t.Fatalf("start assistant: %v", err)
	}
	h.coord.SetGloballyPaused(context.Background(), true)
	h.coord.SetGloballyPaused(context.Background(), false)
	// Started while paused, so the next pause has to catch it too.
	session := coordAssistantSession(h.coord)
	sim := simOf(h.coord)
	if sim.IsSuspended(session) {
		t.Fatal("the assistant is suspended after continuing")
	}
	h.coord.SetGloballyPaused(context.Background(), true)
	if !sim.IsSuspended(session) {
		t.Fatal("the assistant started while paused was not suspended by the next pause")
	}
}

// TestWhilePausedNothingIsPickedUpFiredOrStarted
func TestWhilePausedNothingIsPickedUpFiredOrStarted(t *testing.T) {
	t.Parallel()
	pull := core.Subject{Kind: core.SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
		URL: "https://github.com/acme/web/pull/57", Base: new("develop"), Occurrence: new("100"), IsOpen: true}
	ticket := core.Subject{Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment",
		Body: "Up to what was paid.", URL: "https://acme.atlassian.net/browse/PAY-31", IsOpen: true}
	jiraQuery := core.DefaultJiraQuery()
	jiraQuery.Jql = "project = PAY"
	h := newHarness(t, harnessOptions{
		Reviews: []github.FakeReviewRequest{{Pull: pull, Request: "100"}},
		Tickets: []core.Subject{ticket},
		Belts: []core.Belt{
			core.DefaultBelt("reviews", "Reviews", []core.TriggerEntry{{ID: "reviews", Trigger: core.Trigger{Kind: core.TriggerKindReviews, Reviews: new(core.DefaultReviewFilter())}, Repos: []string{"acme/web"}}}, nil),
			core.DefaultBelt("jira", "Jira", []core.TriggerEntry{{ID: "jira", Trigger: core.Trigger{Kind: core.TriggerKindJira, Jira: new(jiraQuery)}, Repos: []string{"acme/billing"}}}, nil),
			core.DefaultBelt("tasks", "Tasks", []core.TriggerEntry{manualEntry("tasks", "acme/web", "acme/billing")}, nil),
		},
		StageDuration: 3600,
	})
	h.coord.SetGloballyPaused(context.Background(), true)
	for range 5 {
		h.Tick()
	}
	h.coord.CheckNow(context.Background())
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs while paused = %d, want 0", n)
	}
	if n := len(watchChecks(h.coord)); n != 0 {
		t.Fatalf("watch checks while paused = %d, want 0", n)
	}
	if startedAny(simOf(h.coord)) {
		t.Fatal("a session started while paused")
	}
	if last := h.coord.LastCheck(); last != nil {
		t.Fatalf("last check = %v, want none", last)
	}
	if _, err := h.coord.StartTask("tasks", "acme/web", "Tidy", "", nil); err == nil {
		t.Fatal("a task started while paused")
	}
}

// TestTheTwoPausesAreIndependent
func TestTheTwoPausesAreIndependent(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{StageDuration: 3600})
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
	session := runSession(h.Run())
	before := h.Run()
	h.coord.SetGloballyPaused(context.Background(), true)

	if err := h.coord.SetPaused("issues", true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	if err := h.coord.SetPaused("issues", false); err != nil {
		t.Fatalf("unpause belt: %v", err)
	}
	h.coord.SetBeltArchived("issues", false)
	for range 2 {
		h.Tick()
	}
	if got := h.Run(); got.State != before.State || got.StageID != before.StageID {
		t.Fatalf("run = %s at %s, want %s at %s", got.State, got.StageID, before.State, before.StageID)
	}
	if !simOf(h.coord).IsRunning(session) {
		t.Fatal("the session stopped")
	}
	// Continuing does not unpause any belt.
	if record := h.coord.Belt("issues"); record == nil || record.EffectivelyPaused() {
		t.Fatalf("belt = %+v, want not paused", record)
	}
	h.coord.SetGloballyPaused(context.Background(), false)
	if record := h.coord.Belt("issues"); record == nil || record.EffectivelyPaused() {
		t.Fatalf("belt after continuing = %+v, want not paused", record)
	}
	if h.Run().State != core.RunStateRunning {
		t.Fatalf("state after continuing = %s, want running", h.Run().State)
	}
	// And the belt's own pause still holds while the app is live.
	if err := h.coord.SetPaused("issues", true); err != nil {
		t.Fatalf("pause belt: %v", err)
	}
	h.Tick()
	if record := h.coord.Belt("issues"); record == nil || !record.EffectivelyPaused() {
		t.Fatalf("belt = %+v, want paused", record)
	}
}

// TestYourOwnDecisionsKeepWorkingWhilePaused
func TestYourOwnDecisionsKeepWorkingWhilePaused(t *testing.T) {
	t.Parallel()
	spec := core.DefaultsStageForAgent(core.SampleAgent("spec"), nil, nil, nil, core.TriggerKindManual)
	spec.Brief = "Write {output}."
	gate := core.DefaultsStage(core.StageTypeApprove, []core.Stage{spec}, nil, []core.Stage{spec})
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), []core.Stage{spec, gate}, nil, []core.Stage{spec, gate}, core.TriggerKindManual)
	belt := core.DefaultBelt("tasks", "Tasks", []core.TriggerEntry{manualEntry("tasks", "acme/web", "acme/billing")}, []core.Stage{spec, gate, build})
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{belt}, Runner: func(script *harnessScript) runner.AgentRunner {
		return runner.NewSimulatedRunner(func(spec runner.SessionSpec) float64 {
			if spec.AgentID != nil && *spec.AgentID == "build" {
				return 3600
			}
			return 0
		}, func() time.Time { return testkit.ContractTime }, script.next)
	}})
	h.Script.add("spec", "complete")
	run, err := h.coord.StartTask("tasks", "acme/web", "Tidy the copy", "", nil)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateWaiting })
	h.coord.SetGloballyPaused(context.Background(), true)

	// Decide, which queues the agent stage rather than starting it.
	h.coord.Decide(run.ID, "approved", nil)
	if got := h.Run(); got.StageID != "build" || got.State != core.RunStateQueued {
		t.Fatalf("after deciding = %s at %s, want queued at build", got.State, got.StageID)
	}
	// Even Start now does not slip past.
	h.coord.StartNow(context.Background(), run.ID)
	for range 2 {
		h.Tick()
	}
	if got := h.Run(); got.State != core.RunStateQueued {
		t.Fatalf("after starting = %s, want queued", got.State)
	}

	// Adjusting a stage saves. The model has to be one this run was admitted with, as it does when the app is live.
	adjusted, err := h.coord.AdjustStage(run.ID, "build", new("custom/model"), nil, new("Do it."), nil)
	if err != nil || adjusted == nil {
		t.Fatalf("adjust = %+v, %v", adjusted, err)
	}
	if fields := adjusted.Fields; len(fields) != 2 || fields[0] != core.AdjustmentFieldModel || fields[1] != core.AdjustmentFieldBrief {
		t.Fatalf("adjusted fields = %v, want model and brief", fields)
	}
	if stage := h.coord.Run(run.ID); stage == nil || stageNamed(*stage, "build") == nil || stageNamed(*stage, "build").Model != "custom/model" {
		t.Fatal("the build stage did not take the model")
	}

	// The assistant starts.
	if err := h.coord.StartAssistant(context.Background(), "Add a belt for docs fixes"); err != nil {
		t.Fatalf("start assistant: %v", err)
	}
	if !h.coord.ReadAssistant().Running {
		t.Fatal("the assistant is not running")
	}

	// Abort and drop still work once the app is live again.
	h.coord.SetGloballyPaused(context.Background(), false)
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateRunning", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateRunning })
	h.coord.Abort(run.ID, nil)
	if got := h.Run(); got.State != core.RunStateHeld || got.Reason == nil || *got.Reason != "stopped by you" {
		t.Fatalf("after abort = %s (%v), want held, stopped by you", got.State, got.Reason)
	}
	h.coord.Resume(run.ID, nil, nil, false, nil)
	h.coord.Drop(run.ID, nil)
	if got := h.Run(); got.State != core.RunStateDropped {
		t.Fatalf("after drop = %s, want dropped", got.State)
	}
}

// TestContinuingStartsQueuedSessionsOnTheNextTick
func TestContinuingStartsQueuedSessionsOnTheNextTick(t *testing.T) {
	t.Parallel()
	build := buildStage(nil)
	build.Slots = 1
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{core.DefaultBelt("tasks", "Tasks", []core.TriggerEntry{manualEntry("tasks", "acme/web", "acme/billing")}, []core.Stage{build})}, StageDuration: 3600})
	first, err := h.coord.StartTask("tasks", "acme/web", "One", "", nil)
	if err != nil {
		t.Fatalf("start one: %v", err)
	}
	second, err := h.coord.StartTask("tasks", "acme/web", "Two", "", nil)
	if err != nil {
		t.Fatalf("start two: %v", err)
	}
	// StartTask spawns a tick that may start the first task's session. Let that finish before
	// pausing, so the check below sees only what starts while paused.
	h.coord.Idle()
	startsBefore := len(simOf(h.coord).Trail())
	h.coord.SetGloballyPaused(context.Background(), true)
	// The test waits on the second run's session here (`until`), which a paused belt never starts: the
	// whole budget passes, and the checks below show nothing started.
	h.Settle()
	if got := h.coord.Run(second.ID); got == nil || got.State != core.RunStateQueued {
		t.Fatalf("second = %+v, want queued", got)
	}
	if n := len(simOf(h.coord).Trail()); n != startsBefore {
		t.Fatalf("sessions started while paused: trail grew from %d to %d", startsBefore, n)
	}

	h.coord.SetGloballyPaused(context.Background(), false)
	// The next tick fills the one slot the stage has, and no more.
	h.Until("r := coordNowRun(h, first.ID); return r != nil && r.CurrentAttempt() != nil", func() bool { r := coordNowRun(h, first.ID); return r != nil && r.CurrentAttempt() != nil })
	got := h.coord.Run(first.ID)
	if got == nil || got.State != core.RunStateRunning {
		t.Fatalf("first = %+v, want running", got)
	}
	session := runSession(got)
	if !simOf(h.coord).IsRunning(session) {
		t.Fatal("the first session is not running")
	}
	if g := h.coord.Run(second.ID); g == nil || g.State != core.RunStateQueued {
		t.Fatalf("second = %+v, want still queued", g)
	}
}

// TestContinuingMakesWatchesCheckAgain
func TestContinuingMakesWatchesCheckAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordBuildThenWatch()}, OpensNoPulls: true})
	h.Until("run := h.nowRun(); return run != nil && run.StageID == \"watch\"", func() bool { run := h.nowRun(); return run != nil && run.StageID == "watch" })
	run := h.Run()
	if run.Branch == nil {
		t.Fatal("the run has no branch")
	}
	h.github.OpenPull("acme/billing", *run.Branch, core.PullRequestStateOpen)
	h.coord.SetGloballyPaused(context.Background(), true)
	h.clock.Set(h.clock.Now().Add(time.Hour))
	for range 3 {
		h.Tick()
	}
	// The only check is the one the run's watch had before the pause. Each leg is checked under its own key.
	key := run.ID + "#main"
	first := h.clock.Now().Add(-time.Hour)
	if checks := watchChecks(h.coord); len(checks) != 1 || !checks[key].Equal(first) {
		t.Fatalf("checks while paused = %v, want only %s at %s", checks, key, first)
	}
	h.coord.SetGloballyPaused(context.Background(), false)
	h.Until("when, ok := h.coord.watches.lastCheck[key] ...", func() bool {
		when, ok := h.coord.watches.lastCheck[key]
		return ok && !when.Equal(first)
	})
	if got := watchChecks(h.coord)[key]; !got.Equal(h.clock.Now()) {
		t.Fatalf("check after continuing = %s, want %s", got, h.clock.Now())
	}
}

// TestASessionThatDiesWhilePausedIsHeldTheSameWayItWouldBeOtherwise
func TestASessionThatDiesWhilePausedIsHeldTheSameWayItWouldBeOtherwise(t *testing.T) {
	t.Parallel()
	const detail = "the session ended before the agent finished"
	const reason = "Build failed"
	// Triage retries itself once before it holds, so give it a stage that holds.
	build := buildStage(nil)
	build.Fallback = core.FallbackHold
	scenario := func(paused bool) (core.Run, *string) {
		h := newHarness(t, harnessOptions{Belts: []core.Belt{issuesBelt([]core.Stage{build})}, StageDuration: 3600})
		h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateRunning", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateRunning })
		if paused {
			h.coord.SetGloballyPaused(context.Background(), true)
		}
		// The session dies under the app, as a killed agent would.
		live := h.Run()
		attempt := live.CurrentAttempt()
		spec := h.coord.Spec(*live, *attempt, "")
		simOf(h.coord).Finish(context.Background(), *spec)
		for range 2 {
			h.Tick()
		}
		if paused {
			h.coord.SetGloballyPaused(context.Background(), false)
		}
		h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateHeld", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateHeld })
		after := h.Run()
		return *after, after.Reason
	}
	without, withoutReason := scenario(false)
	with, withReason := scenario(true)
	if without.State != core.RunStateHeld {
		t.Fatalf("without the pause = %s, want held", without.State)
	}
	if with.State != without.State {
		t.Fatalf("with the pause = %s, want %s", with.State, without.State)
	}
	if !core.SamePointee(withReason, withoutReason) || withReason == nil || *withReason != reason {
		t.Fatalf("reason with the pause = %v, without = %v, want %s", withReason, withoutReason, reason)
	}
	lastWith := with.LastAttempt(with.StageID)
	lastWithout := without.LastAttempt(without.StageID)
	if lastWith == nil || lastWithout == nil || !core.SamePointee(lastWith.Detail, lastWithout.Detail) || lastWith.Detail == nil || *lastWith.Detail != detail {
		t.Fatalf("attempt details = %v / %v, want %s", lastWith, lastWithout, detail)
	}
}

// TestOneRunForTheNewestMissedSlotAndNothingForTheRest
func TestOneRunForTheNewestMissedSlotAndNothingForTheRest(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{scheduleBelt(2)}, Now: coordLocal(9, 29, 23)})
	h.Tick()
	// Nothing before the pause: the schedule starts counting from now.
	if got := coordLastFired(h, "schedule"); got == nil || !got.Equal(coordLocal(9, 29, 23)) {
		t.Fatalf("last fired = %v, want 09-29 23:00", got)
	}
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs = %d, want 0", n)
	}

	h.coord.SetGloballyPaused(context.Background(), true)
	h.clock.Set(coordLocal(9, 30, 9))
	for range 3 {
		h.Tick()
	}
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs while paused = %d, want 0", n)
	}
	if got := coordLastFired(h, "schedule"); got == nil || !got.Equal(coordLocal(9, 29, 23)) {
		t.Fatalf("last fired while paused = %v, want 09-29 23:00", got)
	}

	h.coord.SetGloballyPaused(context.Background(), false)
	h.Tick()
	if keys := runKeys(h); len(keys) != 1 || keys[0] != "2026-09-30" {
		t.Fatalf("run keys = %v, want [2026-09-30]", keys)
	}
	if got := coordLastFired(h, "schedule"); got == nil || !got.Equal(coordLocal(9, 30, 2)) {
		t.Fatalf("last fired = %v, want 09-30 02:00", got)
	}
}

// TestAScheduleThatNeverFiredCountsFromTheContinueNotThePause
func TestAScheduleThatNeverFiredCountsFromTheContinueNotThePause(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{scheduleBelt(9)}, Now: coordLocal(9, 29, 8)})
	h.coord.SetGloballyPaused(context.Background(), true)
	h.clock.Set(coordLocal(9, 30, 10))
	for range 3 {
		h.Tick()
	}
	if got := coordLastFired(h, "schedule"); got != nil {
		t.Fatalf("last fired while paused = %v, want none", got)
	}
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs = %d, want 0", n)
	}

	h.coord.SetGloballyPaused(context.Background(), false)
	h.Tick()
	if got := coordLastFired(h, "schedule"); got == nil || !got.Equal(coordLocal(9, 30, 10)) {
		t.Fatalf("last fired after continuing = %v, want 09-30 10:00", got)
	}
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs after continuing = %d, want 0", n)
	}
	// 09-30's 09:00 is past now, so it is gone rather than replayed.
	h.clock.Set(coordLocal(9, 30, 11))
	h.Tick()
	if n := len(h.coord.Runs()); n != 0 {
		t.Fatalf("runs an hour later = %d, want 0", n)
	}
}

// TestAPausedOrArchivedBeltStillSkipsItsSlotOnContinue
func TestAPausedOrArchivedBeltStillSkipsItsSlotOnContinue(t *testing.T) {
	t.Parallel()
	for _, archive := range []bool{false, true} {
		name := "paused"
		if archive {
			name = "archived"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{scheduleBelt(2)}, Now: coordLocal(9, 29, 23)})
			h.Tick()
			if archive {
				h.coord.SetBeltArchived("schedule", true)
			} else if err := h.coord.SetPaused("schedule", true); err != nil {
				t.Fatalf("pause belt: %v", err)
			}
			h.coord.SetGloballyPaused(context.Background(), true)
			h.clock.Set(coordLocal(9, 30, 9))
			h.Tick()
			h.coord.SetGloballyPaused(context.Background(), false)
			h.Tick()
			if n := len(h.coord.Runs()); n != 0 {
				t.Fatalf("runs = %d, want 0", n)
			}
			// The slot is consumed exactly as it is today.
			if got := coordLastFired(h, "schedule"); got == nil || !got.Equal(coordLocal(9, 30, 2)) {
				t.Fatalf("last fired = %v, want 09-30 02:00", got)
			}
		})
	}
}

// newSimulated is a simulated runner that runs nothing long.
func newSimulated(clock *testkit.Clock) runner.AgentRunner {
	return runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, clock.Now, func(runner.SessionSpec) *string { return nil })
}

// TestSettingsWrittenBeforeThePauseDecodeAsLive
func TestSettingsWrittenBeforeThePauseDecodeAsLive(t *testing.T) {
	t.Parallel()
	var settings store.Settings
	if err := json.Unmarshal([]byte(`{"login":"you","notifications":true,"factoryRepository":""}`), &settings); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if settings.Paused || settings.PausedAt != nil || len(settings.Pauses) != 0 {
		t.Fatalf("settings = %+v, want live", settings)
	}
}

// TestASlotMissedWhileTheAppWasClosedFiresOnContinue
func TestASlotMissedWhileTheAppWasClosedFiresOnContinue(t *testing.T) {
	t.Parallel()
	root := harnessFolder(t)
	paths := store.Paths{Root: root}
	storage := store.NewStorage(paths)
	definition := scheduleBelt(2)
	definition.Stages = flowTriageApproval(core.TriggerKindSchedule)
	definition.Triggers[0].Trigger.Schedule.Schedule.Hour = 2
	if _, err := storage.SaveNewVersion(definition); err != nil {
		t.Fatalf("save belt: %v", err)
	}
	clock := testkit.NewClock(coordLocal(9, 29, 23))
	newCoordinator := func() *Coordinator {
		coord, err := New(Services{
			Storage: store.NewStorage(paths), GitHub: github.NewFakeGitHub(nil, nil, nil, nil), Jira: jira.NewFakeJira(nil, nil, nil),
			Runner:     runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 3600 }, clock.Now, func(runner.SessionSpec) *string { return nil }),
			Workspaces: runner.FolderWorkspaces{}, Clock: clock, IDs: &SequenceIDs{}, Notifier: func(string, string) {},
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		t.Cleanup(func() { coord.Close() })
		return coord
	}
	coord := newCoordinator()
	coord.Tick(context.Background())
	coord.Idle()
	coord.SetGloballyPaused(context.Background(), true)

	// Quit, sleep, and open the app again after the 02:00 slot.
	reopened := newCoordinator()
	if !coordPaused(reopened) {
		t.Fatal("the reopened app is not paused")
	}
	clock.Set(coordLocal(9, 30, 9))
	reopened.Tick(context.Background())
	reopened.Idle()
	if n := len(reopened.Runs()); n != 0 {
		t.Fatalf("runs while paused = %d, want 0", n)
	}
	reopened.SetGloballyPaused(context.Background(), false)
	reopened.Tick(context.Background())
	reopened.Idle()
	runs := reopened.Runs()
	if len(runs) != 1 || runs[0].Subject.Key != "2026-09-30" {
		t.Fatalf("runs after continuing = %+v, want one for 2026-09-30", runs)
	}
}
