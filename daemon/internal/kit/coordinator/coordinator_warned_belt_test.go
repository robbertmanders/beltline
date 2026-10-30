// WarnedBeltIntakeTests: a belt saved with
// warnings is paused, takes no new work, and resumes only once it is clean. The shared harness has
// no review requests or tickets, so warnedHarness rebuilds the fakes over the harness's folder.

package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
)

// kindBelt is a belt of kind with its starting stages and a trigger that passes validation.
func kindBelt(kind core.TriggerKind, id string) core.Belt {
	agents := core.SampleAgents()
	var trigger core.Trigger
	var repos []string
	switch kind {
	case core.TriggerKindIssues:
		filter := core.DefaultIssueFilter()
		filter.Label = "agent"
		trigger = core.Trigger{Kind: kind, Issues: &filter}
		repos = []string{"acme/billing"}
	case core.TriggerKindReviews:
		filter := core.DefaultReviewFilter()
		trigger = core.Trigger{Kind: kind, Reviews: &filter}
		repos = []string{"acme/web"}
	case core.TriggerKindJira:
		query := core.DefaultJiraQuery()
		query.Jql = "project = PAY"
		trigger = core.Trigger{Kind: kind, Jira: &query}
		repos = []string{"acme/billing"}
	case core.TriggerKindSchedule:
		task := core.DefaultScheduledTask()
		schedule := core.DefaultSchedule()
		schedule.Hour = 9
		task.Schedule = schedule
		task.Title = "Sweep flaky tests"
		task.Task = "Fix or quarantine them."
		trigger = core.Trigger{Kind: kind, Schedule: &task}
		repos = []string{"acme/platform"}
	default:
		trigger = kind.Fresh()
		repos = []string{"acme/web", "acme/billing"}
	}
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: id, Name: id, Trigger: trigger, Repos: repos,
		Stages: core.SampleStages(kind, agents, true), Forks: core.SampleForks(kind),
	})
}

// coordWarn is the belt with its name cleared, which is a warning.
func coordWarn(belt core.Belt) core.Belt {
	belt.Name = ""
	return belt
}

// warnedHarness is newHarness with review requests and tickets. It
// rebuilds the GitHub and Jira fakes over the harness's folder and restarts the coordinator with them.
func warnedHarness(t *testing.T, opts harnessOptions, reviews []github.FakeReviewRequest, tickets []core.Subject) *harness {
	t.Helper()
	h := newHarness(t, opts)
	h.coord.Close()
	issues := opts.Issues
	if issues == nil {
		issues = []core.Subject{{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV",
			Body: "Add an export.", Labels: []string{"agent"}, IsOpen: true,
		}}
	}
	opensPulls := core.ValueOr(opts.OpensPulls, core.PullRequestStateMerged)
	h.github = github.NewFakeGitHub(issues, reviews, nil, &opensPulls)
	h.jira = jira.NewFakeJira(tickets, nil, nil)
	h.services.GitHub = h.github
	h.services.Jira = h.jira
	coord, err := New(h.services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.coord = coord
	t.Cleanup(func() { coord.Close() })
	return h
}

// coordSaveBelt saves belt as a new version, failing the test on error.
func coordSaveBelt(t *testing.T, c *Coordinator, belt core.Belt) core.Belt {
	t.Helper()
	saved, err := c.SaveBelt(belt)
	if err != nil {
		t.Fatalf("save %q: %v", belt.ID, err)
	}
	return saved
}

// coordHold holds a source's preparation until the test releases it.
type coordHold struct {
	entered  chan struct{}
	released chan struct{}
	once     sync.Once
}

// newH5m6Hold is a hold nothing has entered yet.
func newH5m6Hold() *coordHold {
	return &coordHold{entered: make(chan struct{}), released: make(chan struct{})}
}

// enter marks the hold entered and waits for its release.
func (h *coordHold) enter() {
	h.once.Do(func() { close(h.entered) })
	<-h.released
}

// release lets the held preparation finish.
func (h *coordHold) release() { close(h.released) }

// holdSource is one waiting issue per repository, whose preparation waits at the hold.
type holdSource struct {
	sourceDefaults
	gate *coordHold
}

// scopes lists the belt's repositories.
func (s holdSource) scopes(belt core.Belt) []string { return belt.Repos() }

// place names a scope as itself.
func (s holdSource) place(scope string, _ core.Belt) string { return scope }

// list returns one open agent issue in the scope.
func (s holdSource) list(_ context.Context, scope string, _ core.Belt) ([]core.Subject, error) {
	return []core.Subject{{Kind: core.SubjectKindIssue, Repo: scope, Number: 7, Title: "Waiting", Labels: []string{"agent"}, IsOpen: true}}, nil
}

// watches reports that no run is watched here.
func (s holdSource) watches(core.Run, string) bool { return false }

// prepare waits at the hold before the subject is returned.
func (s holdSource) prepare(_ context.Context, subject core.Subject, _ core.Belt) (core.Subject, error) {
	s.gate.enter()
	return subject, nil
}

// missing says the subject is still there.
func (s holdSource) missing(_ context.Context, run core.Run, _ core.Belt) (Fate, error) {
	return Fate{Kind: FateChanged, Subject: run.Subject}, nil
}

// TestAWarnedSaveTakesNoIssueReviewJiraScheduleOrManualWork: a warned belt of each trigger kind starts no work.
func TestAWarnedSaveTakesNoIssueReviewJiraScheduleOrManualWork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	issues := newHarness(t, harnessOptions{})
	ic := issues.Coord()
	coordSaveBelt(t, ic, coordWarn(ic.Belts()[0].Belt))
	ic.CheckNow(ctx)
	ic.Idle()
	if runs := ic.Runs(); len(runs) != 0 {
		t.Errorf("issues: a warned belt started %d runs", len(runs))
	}

	reviews := warnedHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{kindBelt(core.TriggerKindReviews, "reviews")}},
		[]github.FakeReviewRequest{{Pull: pullFixture(), Request: "100"}}, nil)
	rc := reviews.Coord()
	coordSaveBelt(t, rc, coordWarn(rc.Belts()[0].Belt))
	rc.CheckNow(ctx)
	rc.Idle()
	if runs := rc.Runs(); len(runs) != 0 {
		t.Errorf("reviews: a warned belt started %d runs", len(runs))
	}

	jiraHarness := warnedHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{kindBelt(core.TriggerKindJira, "jira")}},
		nil, []core.Subject{ticketFixture()})
	jc := jiraHarness.Coord()
	coordSaveBelt(t, jc, coordWarn(jc.Belts()[0].Belt))
	jc.CheckNow(ctx)
	jc.Idle()
	if runs := jc.Runs(); len(runs) != 0 {
		t.Errorf("jira: a warned belt started %d runs", len(runs))
	}

	schedule := time.Date(2026, time.September, 29, 8, 0, 0, 0, time.Local)
	sh := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{kindBelt(core.TriggerKindSchedule, "schedule")}, Now: schedule})
	sc := sh.Coord()
	sh.Tick()
	if runs := sc.Runs(); len(runs) != 0 {
		t.Errorf("schedule: runs before the warned save = %d", len(runs))
	}
	coordSaveBelt(t, sc, coordWarn(sc.Belts()[0].Belt))
	sh.clock.Set(time.Date(2026, time.September, 29, 10, 0, 0, 0, time.Local))
	sh.Tick()
	if runs := sc.Runs(); len(runs) != 0 {
		t.Errorf("schedule: a warned belt fired %d runs", len(runs))
	}
	if belt := sc.Belt("schedule"); belt == nil || !belt.Paused {
		t.Errorf("schedule: the warned belt is not paused")
	}

	manual := newHarness(t, harnessOptions{Issues: []core.Subject{}, Belts: []core.Belt{kindBelt(core.TriggerKindManual, "tasks")}})
	mc := manual.Coord()
	coordSaveBelt(t, mc, coordWarn(beltRecordOf(t, mc, "tasks").Belt))
	if _, err := mc.StartTask("tasks", "acme/web", "Tidy", "", nil); err == nil {
		t.Errorf("manual: a warned belt started a task")
	}
	if runs := mc.Runs(); len(runs) != 0 {
		t.Errorf("manual: runs = %d", len(runs))
	}
}

// TestPreparationThatFinishesAfterAWarnedSaveDoesNotJoin: a preparation that was under way when the belt was
// warned does not start its run when it finishes.
func TestPreparationThatFinishesAfterAWarnedSaveDoesNotJoin(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}})
	c := h.Coord()
	record := c.Belts()[0]
	gate := newH5m6Hold()
	source := holdSource{gate: gate}
	entry := record.Belt.Triggers[0]
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Lock()
		defer c.EndTurn()
		c.intake.poll(context.Background(), record, entry, source, "acme/billing")
	}()
	<-gate.entered
	coordSaveBelt(t, c, coordWarn(record.Belt))
	gate.release()
	<-done
	if runs := c.Runs(); len(runs) != 0 {
		t.Errorf("a preparation that finished after the warned save started %d runs", len(runs))
	}
	if belt := c.Belt(record.Belt.ID); belt == nil || !belt.Paused {
		t.Errorf("the belt is not paused")
	}
}

// TestAWarnedSaveLeavesTheRunningSessionAndStillAllowsStartNow: a warned save keeps the running session and its
// saved run, and a queued run can still be started by hand.
func TestAWarnedSaveLeavesTheRunningSessionAndStillAllowsStartNow(t *testing.T) {
	t.Parallel()
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.Stages[0].Slots = 1
	issues := []core.Subject{
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Issue 142", Labels: []string{"agent"}, IsOpen: true},
		{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 143, Title: "Issue 143", Labels: []string{"agent"}, IsOpen: true},
	}
	h := newHarness(t, harnessOptions{Issues: issues, Belts: []core.Belt{belt}, StageDuration: 3600})
	c := h.Coord()
	h.Until("var running, queued bool ...", func() bool {
		var running, queued bool
		for _, run := range h.nowRuns() {
			running = running || run.State == core.RunStateRunning
			queued = queued || run.State == core.RunStateQueued
		}
		return running && queued
	})
	var running, waiting core.Run
	for _, run := range c.Runs() {
		if run.State == core.RunStateRunning && running.ID == "" {
			running = run
		}
		if run.State == core.RunStateQueued && waiting.ID == "" {
			waiting = run
		}
	}
	if running.ID == "" || waiting.ID == "" {
		t.Fatalf("no running and queued run")
	}
	attempt := running.CurrentAttempt()
	if attempt == nil || attempt.Session == nil {
		t.Fatalf("the running run has no session")
	}
	sim := h.services.Runner.(*runner.SimulatedRunner)
	if !sim.IsRunning(*attempt.Session) {
		t.Fatalf("the session is not running")
	}
	savedRun := findRunOnDisk(h, running.ID)

	coordSaveBelt(t, c, coordWarn(beltRecordOf(t, c, belt.ID).Belt))
	h.Tick()
	kept := c.Run(running.ID)
	if kept == nil || kept.State != core.RunStateRunning {
		t.Fatalf("the warned save changed the running run: %+v", kept)
	}
	if coordJSON(t, kept.Stages) != coordJSON(t, running.Stages) || coordJSON(t, kept.Policies) != coordJSON(t, running.Policies) ||
		coordJSON(t, kept.Agents) != coordJSON(t, running.Agents) || kept.BeltVersion != running.BeltVersion {
		t.Errorf("the running run's stages, policies, agents or belt version changed")
	}
	if coordJSON(t, findRunOnDisk(h, running.ID)) != coordJSON(t, savedRun) {
		t.Errorf("the saved running run changed")
	}
	if !sim.IsRunning(*attempt.Session) {
		t.Errorf("the session stopped")
	}
	if got := c.Run(waiting.ID); got == nil || got.State != core.RunStateQueued {
		t.Errorf("the waiting run is not queued")
	}
	c.StartNow(context.Background(), waiting.ID)
	started := c.Run(waiting.ID)
	if started == nil || started.State != core.RunStateRunning {
		t.Fatalf("start now did not start the waiting run: %+v", started)
	}
	if started.BeltVersion != waiting.BeltVersion || coordJSON(t, started.Stages) != coordJSON(t, waiting.Stages) {
		t.Errorf("the started run changed its belt version or stages")
	}
}

// findRunOnDisk is the saved run with id, read from the folder.
func findRunOnDisk(h *harness, id string) core.Run {
	for _, run := range h.storage.LoadRuns() {
		if run.ID == id {
			return run
		}
	}
	return core.Run{}
}

// TestResumingACorrectedBeltAllowsIntakeAndAutomaticSessions: once the warnings are fixed the belt is still paused
// until resumed, and then a restarted coordinator takes its work.
func TestResumingACorrectedBeltAllowsIntakeAndAutomaticSessions(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	ctx := context.Background()
	warned := coordSaveBelt(t, c, coordWarn(c.Belts()[0].Belt))
	c.CheckNow(ctx)
	c.Idle()
	if runs := c.Runs(); len(runs) != 0 {
		t.Fatalf("a warned belt started %d runs", len(runs))
	}
	fixed := warned.Clone()
	fixed.Name = "Issues to pull requests"
	corrected := coordSaveBelt(t, c, fixed)
	if w := c.Warnings(corrected); len(w) != 0 {
		t.Errorf("warnings after the fix = %v", w)
	}
	if belt := c.Belt(corrected.ID); belt == nil || !belt.Paused {
		t.Errorf("the corrected belt is not paused")
	}
	c.CheckNow(ctx)
	c.Idle()
	if runs := c.Runs(); len(runs) != 0 {
		t.Errorf("a paused corrected belt started %d runs", len(runs))
	}
	if err := c.SetPaused(corrected.ID, false); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	c.Idle()

	again := h.Restart()
	again.Idle()
	if belt := again.Belt(corrected.ID); belt == nil || belt.Paused {
		t.Fatalf("the resumed belt is paused after a restart")
	}
	again.CheckNow(ctx)
	again.Idle()
	for i := 0; i < 40; i++ {
		runs := again.Runs()
		if len(runs) > 0 && (runs[0].State == core.RunStateRunning || runs[0].State == core.RunStateHeld || runs[0].State == core.RunStateWaiting) {
			break
		}
		again.Tick(ctx)
		again.Idle()
	}
	runs := again.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].BeltVersion != corrected.Version {
		t.Errorf("run belt version = %d, want %d", runs[0].BeltVersion, corrected.Version)
	}
}

// pullFixture is pull request 57 on acme/web, from develop, at occurrence 100.
func pullFixture() core.Subject {
	return core.Subject{Kind: core.SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
		URL: "https://github.com/acme/web/pull/57", Base: new("develop"), Occurrence: new("100")}
}

// ticketFixture is PAY-31 on acme/billing.
func ticketFixture() core.Subject {
	return core.Subject{Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment", Body: "Up to what was paid.",
		URL: "https://acme.atlassian.net/browse/PAY-31", IsOpen: true}
}
