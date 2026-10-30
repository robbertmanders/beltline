// Fixtures the coordinator tests build on: the sample runs and attempts of the Sessions tests, the belts
// the pause, overrule, parking and erasure tests use (issues, forks, schedules), the AFK world and
// its holding runner, the watch fixtures, and the accessors that read coordinator state under the lock.

package coordinator

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// sessionSampleT0 is the tests' t0 clock start.
var sessionSampleT0 = time.Unix(1_800_000_000, 0)

// sessionSampleRun is a started run of the default belt on acme/billing #142.
func sessionSampleRun() core.Run {
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	run := core.NewRun(core.RunArgs{
		Belt: belt,
		Subject: core.Subject{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142,
			Title: "Export invoices as CSV", Body: "Add an export button.",
		},
		Now: sessionSampleT0,
	})
	core.EngineStart(&run, sessionSampleT0)
	return run
}

// sessionSampleAttempt is an attempt with no leg: Attempt(stageId:number:startedAt:note:folder:).
func sessionSampleAttempt(stageID string, number int, folder string) core.Attempt {
	return core.NewAttempt(core.AttemptArgs{
		StageID: stageID, Number: number, StartedAt: jsonx.Date{Time: sessionSampleT0}, Folder: folder,
	})
}

// Fixtures for the coordinator tests of pauses, parking, overrule, erasure and multi-route belts: the
// belts the builders below make, local dates, and the accessors the tests read under the lock.

// coordLocal is local(month, day, hour, minute) in 2026, in the local time zone.
func coordLocal(month, day, hour int, minute ...int) time.Time {
	m := 0
	if len(minute) > 0 {
		m = minute[0]
	}
	return time.Date(2026, time.Month(month), day, hour, m, 0, 0, time.Local)
}

// issuesBelt is the default issues belt on acme/billing with the given stages and no forks.
func issuesBelt(stages []core.Stage) core.Belt {
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.Stages = stages
	belt.Forks = nil
	return belt
}

// buildStage is a build agent stage with the brief the pause tests use.
func buildStage(existing []core.Stage) core.Stage {
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), nil, nil, existing, core.TriggerKindIssues)
	build.Brief = "Build the change and write {output}."
	return build
}

// coordBuildThenWatch is one agent stage and a watch after it, on the issues belt.
func coordBuildThenWatch() core.Belt {
	build := buildStage(nil)
	watch := core.DefaultsStage(core.StageTypeWatch, []core.Stage{build}, nil, []core.Stage{build})
	return issuesBelt([]core.Stage{build, watch})
}

// coordForkedBelt is a belt whose first outcome splits its run into the
// "complex" and "simple" routes of the "size" fork, with a quick agent on the simple route.
func coordForkedBelt(simple ...string) core.Belt {
	fork := core.Fork{ID: "size", Routes: []core.Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	var stages []core.Stage
	add := func(agent string, route *string, id string) {
		place := core.DefaultsPlace{Before: stages, Forks: []core.Fork{fork}, Route: route}
		stage := core.DefaultsStageForAgentAt(core.SampleAgent(agent), place, stages, core.TriggerKindIssues)
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	complex, simpleRoute := "complex", "simple"
	add("triage", nil, "")
	add("spec", &complex, "")
	add("build", &complex, "")
	for _, agent := range simple {
		add(agent, &simpleRoute, "quick-"+agent)
	}
	add("verify", nil, "")
	add("ship", nil, "")
	stages[0].Outcomes[0].Then = core.Then{Action: core.ThenActionRoute, Routes: []string{"complex", "simple"}}
	belt := issuesBelt(stages)
	belt.Forks = []core.Fork{fork}
	return belt
}

// coordLegBelt is bothRoutes with the simple route's agent given the evidence key.
func coordLegBelt() core.Belt {
	belt := coordForkedBelt("build")
	for i := range belt.Stages {
		if belt.Stages[i].ID == "quick-build" {
			belt.Stages[i].Key = new("quick-evidence")
			belt.Stages[i].Brief = strings.ReplaceAll(core.SampleAgent("build").Brief, "{evidence}", "{output}")
		}
	}
	for i := range belt.Stages {
		if belt.Stages[i].ID == "spec" || belt.Stages[i].ID == "quick-build" {
			belt.Stages[i].Workspace = core.WorkspaceWorktree
		}
	}
	return belt
}

// scheduleBelt is a schedule belt whose build stage runs at hour, then waits for approval.
func scheduleBelt(hour int) core.Belt {
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), nil, nil, nil, core.TriggerKindSchedule)
	build.Brief = "Do the scheduled work and write {output}."
	gate := core.DefaultsStage(core.StageTypeApprove, []core.Stage{build}, nil, []core.Stage{build})
	schedule := core.DefaultSchedule()
	schedule.Hour = hour
	task := core.DefaultScheduledTask()
	task.Schedule = schedule
	task.Title = "Nightly sweep"
	task.Task = "Fix them."
	trigger := core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &task}
	entry := core.TriggerEntry{ID: core.TriggerEntryLegacyID, Trigger: trigger, Repos: []string{"acme/platform"}}
	return core.DefaultBelt("schedule", "Schedule", []core.TriggerEntry{entry}, []core.Stage{build, gate})
}

// coordLastFired is the schedule's last firing saved for the belt with id, or nil.
func coordLastFired(h *harness, id string) *time.Time {
	for _, record := range h.storage.LoadBelts() {
		if record.Belt.ID == id {
			return record.LastFired()
		}
	}
	return nil
}

// coordPaused reports whether the app is paused, through the lock.
func coordPaused(c *Coordinator) bool {
	c.Lock()
	defer c.EndTurn()
	return c.globallyPaused
}

// coordAssistantSession is the configuration assistant's session name, through the lock.
func coordAssistantSession(c *Coordinator) string {
	c.Lock()
	defer c.EndTurn()
	return c.assistant.session
}

// watchChecks copies the watch check times, through the lock.
func watchChecks(c *Coordinator) map[string]time.Time {
	c.Lock()
	defer c.EndTurn()
	out := map[string]time.Time{}
	maps.Copy(out, c.watches.lastCheck)
	return out
}

// runKeys are the subject keys of the runs, in order.
func runKeys(h *harness) []string {
	var keys []string
	for _, run := range h.coord.Runs() {
		keys = append(keys, run.Subject.Key)
	}
	return keys
}

// simOf is the coordinator's simulated runner.
func simOf(c *Coordinator) *runner.SimulatedRunner {
	return c.runner.(*runner.SimulatedRunner)
}

// startedAny reports whether the simulated runner started any session.
func startedAny(sim *runner.SimulatedRunner) bool {
	return slices.ContainsFunc(sim.Trail(), func(entry string) bool {
		return strings.HasPrefix(entry, "start ")
	})
}

// runSession is the current attempt's session of run, or "" when none.
func runSession(run *core.Run) string {
	if run == nil || run.CurrentAttempt() == nil || run.CurrentAttempt().Session == nil {
		return ""
	}
	return *run.CurrentAttempt().Session
}

// coordStatus is the simulated runner's status of the run's current session.
func coordStatus(c *Coordinator, run core.Run) runner.SessionStatus {
	attempt := run.CurrentAttempt()
	if attempt == nil {
		return runner.SessionStatus{Kind: runner.SessionStatusGone}
	}
	spec := c.Spec(run, *attempt, "")
	if spec == nil {
		return runner.SessionStatus{Kind: runner.SessionStatusGone}
	}
	return simOf(c).Status(context.Background(), *spec)
}

// coordNowRun returns the run with id without the lock, for Until predicates (which run with it held), or nil.
func coordNowRun(h *harness, id string) *core.Run {
	for i := range h.coord.book.runs {
		if h.coord.book.runs[i].ID == id {
			run := h.coord.book.runs[i]
			return &run
		}
	}
	return nil
}

// Helpers for the coordinator tests of AFK, pause, parking, overrule and erasure: an AFK world
// and a gated harness, the HoldingRunner that holds an AFK call at a gate, and the run and
// decision builders those suites share.

// coordOptedInBelt is the default belt, opted in to AFK through the global switch.
func coordOptedInBelt() core.Belt {
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.Afk = core.AfkModeGlobal
	return belt
}

// coordSubject is runSubject(number, title): an open agent-labelled issue on acme/billing.
func coordSubject(number int, title string) core.Subject {
	return core.Subject{
		Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number, Title: title,
		Body: "Add an export.", Labels: []string{"agent"}, IsOpen: true,
	}
}

// waitingRun is AfkWorld.waiting(belt, number:): a run that stands at its approval,
// with a snapshot of each built-in agent its stages pin.
func waitingRun(t *testing.T, belt core.Belt, number int) core.Run {
	t.Helper()
	run := core.NewRun(core.RunArgs{Belt: belt, Subject: core.Subject{
		Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number,
		Title: fmt.Sprintf("Issue %d", number), Labels: []string{"agent"},
	}, Now: sessionSampleT0})
	core.EngineStart(&run, sessionSampleT0)
	finishWith(&run, "ready to spec")
	finishWith(&run, "complete")
	if run.State != core.RunStateWaiting {
		t.Fatalf("issue %d is %s, not waiting", number, run.State)
	}
	seen := map[core.AgentRef]bool{}
	for _, stage := range run.Stages {
		if stage.Agent == nil || seen[*stage.Agent] {
			continue
		}
		seen[*stage.Agent] = true
		for _, agent := range core.SampleAgents() {
			if agent.Ref() == *stage.Agent {
				run.Agents = append(run.Agents, core.NewAgentSnapshot(agent))
			}
		}
	}
	return run
}

// gateWith is coordGate over a runner built by make (a holding runner, for the AFK calls the tests hold).
func gateWith(t *testing.T, make func(script *harnessScript) runner.AgentRunner) *harness {
	t.Helper()
	return coordGate(t, harnessOptions{Runner: make})
}

// coordGate is the gate harness: a harness on the opted-in belt, run to its approval.
// Extra options may replace the default belt by setting Belts.
func coordGate(t *testing.T, opts harnessOptions) *harness {
	t.Helper()
	if opts.Belts == nil {
		opts.Belts = []core.Belt{coordOptedInBelt()}
	}
	h := newHarness(t, opts)
	h.Script.add("triage", "ready to spec")
	h.Script.add("spec", "complete")
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateWaiting", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateWaiting })
	if run := h.Run(); run == nil || run.State != core.RunStateWaiting {
		t.Fatalf("run did not reach its approval: %+v", h.Run())
	}
	return h
}

// runOn returns the first run of the belt with beltID, or nil. Takes the lock.
func runOn(h *harness, beltID string) *core.Run {
	for _, run := range h.coord.Runs() {
		if run.BeltID == beltID {
			return &run
		}
	}
	return nil
}

// coordDecisions returns the AFK decision log, oldest first. Takes the lock.
func coordDecisions(h *harness) []core.AfkDecision {
	return h.coord.ReadAfkDecisions()
}

// decisionsNow returns the decision log without the lock, for Until predicates (which run with it held).
func decisionsNow(h *harness) []core.AfkDecision {
	return h.coord.autopilot.decisions
}

// setAfk changes the global AFK settings through the settings command path.
func setAfk(h *harness, change func(afk *core.AfkSettings)) {
	h.Turn(func() {
		h.coord.changeSettings(func(s *store.Settings) { change(&s.Afk) })
	})
}

// globalAfk reports whether global AFK is on, through the lock.
func globalAfk(c *Coordinator) bool {
	c.Lock()
	defer c.EndTurn()
	return c.settings.Afk.Enabled
}

// modeOf is AfkMode(choice:): nil follows the switch, true is always, false never.
func modeOf(choice *bool) core.AfkMode {
	switch {
	case choice == nil:
		return core.AfkModeGlobal
	case *choice:
		return core.AfkModeAlways
	default:
		return core.AfkModeNever
	}
}

// decisionOf returns the decision with id, or nil.
func decisionOf(decisions []core.AfkDecision, id string) *core.AfkDecision {
	for i := range decisions {
		if decisions[i].ID == id {
			return &decisions[i]
		}
	}
	return nil
}

// writeAfkDecision writes the assistant's decision file for decision, as its session would
// .
func writeAfkDecision(t *testing.T, choice string, run core.Run, decision core.AfkDecision, paths store.Paths) {
	t.Helper()
	folder := filepath.Join(paths.RunFolder(run), decision.Folder)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, core.AfkDecisionFile), []byte(runner.SimulatedAFKDecision(choice)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// coordGates are the points of an AFK call a HoldingRunner can hold.
type coordGates string

const (
	gateStart  coordGates = "start"
	gateStatus coordGates = "status"
	gateReply  coordGates = "reply"
)

// coordHolding is a simulated runner whose AFK calls wait at a gate the test
// holds and releases. Stage sessions run as the simulated runner runs them.
type coordHolding struct {
	*runner.SimulatedRunner
	mu sync.Mutex
	// hold is the gate the next AFK call waits at, or "" for none.
	hold coordGates
	// release is closed by release() to let the waiting call go on.
	release chan struct{}
	// entered is closed when an AFK call reaches its gate, for waitUntil.
	entered  map[coordGates]chan struct{}
	sessions map[string]bool
	status   runner.SessionStatus
	reply    *string
	stopped  []string
}

// newH5m4Holding builds a holding runner over a simulated runner that picks outcomes from script.
// Stage sessions run for no time, so the simulated clock is fixed at the contract time.
func newH5m4Holding(script *harnessScript) *coordHolding {
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, func() time.Time { return testkit.ContractTime }, script.next)
	return &coordHolding{
		SimulatedRunner: sim,
		release:         make(chan struct{}),
		entered:         map[coordGates]chan struct{}{},
		sessions:        map[string]bool{},
		status:          runner.SessionStatus{Kind: runner.SessionStatusRunning},
	}
}

// hold makes the next AFK call wait at gate. Nil clears it: pass "" to clear.
func (r *coordHolding) holdAt(gate coordGates) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hold = gate
	if gate != "" {
		r.release = make(chan struct{})
	}
}

// releaseAll clears the gate and lets every call waiting at it go on.
func (r *coordHolding) releaseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hold = ""
	select {
	case <-r.release:
	default:
		close(r.release)
	}
}

// block waits at gate when the test holds it. It records that a call reached the gate.
func (r *coordHolding) block(gate coordGates) {
	r.mu.Lock()
	if r.hold != gate {
		r.mu.Unlock()
		return
	}
	if r.entered[gate] == nil {
		r.entered[gate] = make(chan struct{})
		close(r.entered[gate])
	}
	release := r.release
	r.mu.Unlock()
	<-release
}

// waitUntil reports whether a call reached gate within the wait, without a fixed sleep.
func (r *coordHolding) waitUntil(t *testing.T, gate coordGates) bool {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		r.mu.Lock()
		ch := r.entered[gate]
		r.mu.Unlock()
		if ch != nil {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(time.Millisecond):
		}
	}
}

// hasSeen reports whether a call has reached gate.
func (r *coordHolding) hasSeen(gate coordGates) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entered[gate] != nil
}

// setStatus sets what an AFK session reports from now on.
func (r *coordHolding) setStatus(status runner.SessionStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = status
}

// setReply sets the assistant's reply.
func (r *coordHolding) setReply(text *string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reply = text
}

// stoppedSessions returns the sessions stopped so far, in order.
func (r *coordHolding) stoppedSessions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stopped...)
}

// Start holds AFK sessions at the start gate; stage sessions start as the simulated runner starts them.
func (r *coordHolding) Start(ctx context.Context, spec runner.SessionSpec) error {
	if spec.AgentID == nil || *spec.AgentID != "afk" {
		return r.SimulatedRunner.Start(ctx, spec)
	}
	if err := os.MkdirAll(spec.AttemptFolder, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(spec.AttemptFolder, "brief.md"), []byte(spec.Prompt), 0o644); err != nil {
		return err
	}
	r.mu.Lock()
	r.sessions[spec.Name] = true
	r.mu.Unlock()
	r.block(gateStart)
	return nil
}

// Status holds AFK sessions at the status gate, then reports the status set for them.
func (r *coordHolding) Status(ctx context.Context, spec runner.SessionSpec) runner.SessionStatus {
	if spec.AgentID == nil || *spec.AgentID != "afk" {
		return r.SimulatedRunner.Status(ctx, spec)
	}
	r.block(gateStatus)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.sessions[spec.Name] {
		return runner.SessionStatus{Kind: runner.SessionStatusGone}
	}
	return r.status
}

// Reply holds AFK sessions at the reply gate, then returns the reply set for them.
func (r *coordHolding) Reply(ctx context.Context, spec runner.SessionSpec) *string {
	if spec.AgentID == nil || *spec.AgentID != "afk" {
		return nil
	}
	r.block(gateReply)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reply
}

// Finish ends a stage session; an AFK session has nothing to end.
func (r *coordHolding) Finish(ctx context.Context, spec runner.SessionSpec) {
	if spec.AgentID == nil || *spec.AgentID != "afk" {
		r.SimulatedRunner.Finish(ctx, spec)
	}
}

// Stop records the session and ends it.
func (r *coordHolding) Stop(ctx context.Context, session string) {
	r.mu.Lock()
	r.stopped = append(r.stopped, session)
	r.mu.Unlock()
	r.SimulatedRunner.Stop(ctx, session)
}

// tickUntil ticks the coordinator in a goroutine until the runner's call reaches gate, as
// The tick(_:until:enters:) helper: the returned channel closes when that tick returns.
func tickUntil(h *harness, holding *coordHolding, gate coordGates) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 40 {
			if holding.hasSeen(gate) {
				return
			}
			h.coord.Tick(context.Background())
		}
	}()
	return done
}

// decidingFor returns the decision for run that is still in progress, or nil.
func decidingFor(decisions []core.AfkDecision, runID string) *core.AfkDecision {
	for i := range decisions {
		if decisions[i].RunID == runID && decisions[i].Status == core.AfkDecisionStatusDeciding {
			return &decisions[i]
		}
	}
	return nil
}

// coordCount counts the decisions for which keep is true.
func coordCount(decisions []core.AfkDecision, keep func(core.AfkDecision) bool) int {
	n := 0
	for _, decision := range decisions {
		if keep(decision) {
			n++
		}
	}
	return n
}

// coordWaits polls cond until it holds, failing the test at the deadline. It never fixes a sleep.
func coordWaits(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	testkit.Eventually(t, 10*time.Second, msg, cond)
}

// The fixture for the watch tests, and the helpers they share: a run pinned at its watch stage on a
// coordinator whose GitHub fake opens no pull request on its own, and the turns a test runs the watch in.

// watchOptions are what WatchFixture's init changes about the default watch stage.
type watchOptions struct {
	// events are the watch events the stage keeps. Nil keeps every event.
	events []core.WatchEvent
	// retry makes each kept outcome retry instead of hold.
	retry bool
	// none is an extra none outcome, inserted first when noneFirst and last otherwise.
	none      *core.Outcome
	noneFirst bool
	// repos replaces the run's repositories when set.
	repos []string
}

// coordWatch is a run pinned at its watch stage, over a harness.
type coordWatch struct {
	t  *testing.T
	h  *harness
	gh *github.FakeGitHub
}

// newH5m3Watch builds the fixture: a build stage and a watch stage on
// an issue run, with the build finished and the run's branch set. The run is saved before the
// coordinator loads it, so it is the run book's only run.
func newH5m3Watch(t *testing.T, opts watchOptions) *coordWatch {
	t.Helper()
	run := pinnedRun(opts)
	h := newH5m3Harness(t, &run, nil)
	return &coordWatch{t: t, h: h, gh: h.github}
}

// pinnedRun is the run WatchFixture pins: belt `issues` with a build and a watch stage.
func pinnedRun(opts watchOptions) core.Run {
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), nil, nil, nil, core.TriggerKindIssues)
	watch := core.DefaultsStage(core.StageTypeWatch, []core.Stage{build}, nil, []core.Stage{build})
	events := opts.events
	if events == nil {
		events = core.AllWatchEvent
	}
	keep := map[core.WatchEvent]struct{}{}
	for _, event := range events {
		keep[event] = struct{}{}
	}
	var outcomes []core.Outcome
	for _, outcome := range watch.Outcomes {
		if _, ok := keep[core.WatchEvent(outcome.When)]; ok {
			outcomes = append(outcomes, outcome)
		}
	}
	if !opts.retry {
		for index := range outcomes {
			outcomes[index].Then = core.NewThen(core.ThenArgs{Action: core.ThenActionHold})
		}
	}
	if opts.none != nil {
		none := *opts.none
		if opts.noneFirst {
			outcomes = append([]core.Outcome{none}, outcomes...)
		} else {
			outcomes = append(outcomes, none)
		}
	}
	watch.Outcomes = outcomes

	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.Stages = []core.Stage{build, watch}
	belt.Forks = nil
	run := core.NewRun(core.RunArgs{
		Belt: belt,
		Subject: core.Subject{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export", IsOpen: true,
		},
		Now: sessionSampleT0,
	})
	core.EngineStart(&run, sessionSampleT0)
	finishWith(&run, "done")
	branch := "beltline/issue-142"
	run.Branch = &branch
	if opts.repos != nil {
		run.Repos = opts.repos
	}
	return run
}

// newH5m3Harness is newHarness without the default open pull request: the GitHub fake opens
// nothing on a branch lookup. A run, when given, is saved
// before the coordinator loads the folder.
func newH5m3Harness(t *testing.T, run *core.Run, opensPulls *core.PullRequestState) *harness {
	t.Helper()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	if run != nil {
		if err := storage.SaveRun(*run); err != nil {
			t.Fatalf("save run: %v", err)
		}
	}
	clock := testkit.NewClock(sessionSampleT0)
	h := &harness{
		t: t, root: root, storage: storage, clock: clock,
		github: github.NewFakeGitHub(nil, nil, nil, opensPulls),
		jira:   jira.NewFakeJira(nil, nil, nil),
		Script: &harnessScript{queue: map[string][]string{}},
	}
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, clock.Now, h.Script.next)
	h.services = Services{
		Storage: storage, GitHub: h.github, Jira: h.jira, Runner: sim,
		Workspaces: runner.FolderWorkspaces{}, Clock: clock, IDs: &SequenceIDs{},
		Notifier: func(string, string) {},
	}
	coord, err := New(h.services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.coord = coord
	t.Cleanup(func() { coord.Close() })
	return h
}

// checkWatches runs one forced watch pass as a turn, then waits for
// the work it spawned.
func checkWatches(c *Coordinator, force bool) {
	c.Lock()
	c.watches.check(context.Background(), force)
	c.EndTurn()
	c.Idle()
}

// coordTurn runs change as one main-actor turn, then waits for the work it spawned.
func coordTurn(c *Coordinator, change func()) {
	c.Lock()
	change()
	c.EndTurn()
	c.Idle()
}

// coordApply applies event to the run id as a turn.
func coordApply(c *Coordinator, id string, event core.Event) {
	coordTurn(c, func() { c.book.apply(id, event, nil, nil, nil) })
}

// coordUpdate2 changes the run id as a turn.
func coordUpdate2(c *Coordinator, id string, change func(run *core.Run)) {
	coordTurn(c, func() { c.book.update(id, nil, nil, change) })
}

// coordResume is resume(stage: "watch", note: nil) applied to the run.
func coordResume(c *Coordinator, id string) {
	stage := "watch"
	coordApply(c, id, core.EventResume(&stage, nil, nil))
}

// run returns the fixture's run as a copy.
func (f *coordWatch) run() core.Run {
	f.t.Helper()
	runs := f.h.coord.Runs()
	if len(runs) == 0 {
		f.t.Fatal("no run")
	}
	return runs[0]
}

// check is a forced watch pass.
func (f *coordWatch) check() { checkWatches(f.h.coord, true) }

// checkQuietly is a watch pass that runs only when the last one is a minute old.
func (f *coordWatch) checkQuietly() { checkWatches(f.h.coord, false) }

// open puts pull request 201 on the run's branch, with head "a" and base main.
func (f *coordWatch) open() {
	f.gh.PutPull(coordPull(201, "https://github.com/acme/billing/pull/201", "a", "main"), *f.run().Branch)
}

// resume resumes the fixture's run.
func (f *coordWatch) resume() { coordResume(f.h.coord, f.run().ID) }

// apply applies event to the fixture's run.
func (f *coordWatch) apply(event core.Event) { coordApply(f.h.coord, f.run().ID, event) }

// update changes the fixture's run.
func (f *coordWatch) update(change func(run *core.Run)) { coordUpdate2(f.h.coord, f.run().ID, change) }

// finish finishes the run's current agent stage with outcome.
func (f *coordWatch) finish(outcome string) {
	f.update(func(run *core.Run) { finishWith(run, outcome) })
}

// setOutcome sets the then of the watch stage's outcome for event.
func (f *coordWatch) setOutcome(event core.WatchEvent, then core.Then) {
	f.update(func(run *core.Run) {
		setWatchOutcome(run, event, then)
	})
}

// setNewChanges sets the then of the new-changes outcome.
func (f *coordWatch) setNewChanges(then core.Then) { f.setOutcome(core.WatchEventNewChanges, then) }

// setWatchOutcome sets the then of the watch stage's outcome for event on run.
func setWatchOutcome(run *core.Run, event core.WatchEvent, then core.Then) {
	for index := range run.Stages {
		if run.Stages[index].ID != "watch" {
			continue
		}
		for outcome := range run.Stages[index].Outcomes {
			if run.Stages[index].Outcomes[outcome].When == string(event) {
				run.Stages[index].Outcomes[outcome].Then = then
				return
			}
		}
	}
}

// watchOutcomes is the outcomes of the run's watch attempts, in order.
func coordWatchOutcomes(run core.Run) []string {
	var out []string
	for _, attempt := range run.Attempts {
		if attempt.StageID == "watch" && attempt.Outcome != nil {
			out = append(out, *attempt.Outcome)
		}
	}
	return out
}

// watchOutcomes is the fixture run's watch outcomes.
func (f *coordWatch) watchOutcomes() []string { return coordWatchOutcomes(f.run()) }

// newChanges is the watch outcomes that say "new changes".
func (f *coordWatch) newChanges() []string {
	var out []string
	for _, outcome := range f.watchOutcomes() {
		if outcome == "new changes" {
			out = append(out, outcome)
		}
	}
	return out
}

// seen is the fixture's observation of repo.
func (f *coordWatch) seen(repo string) *core.WatchObservation {
	return coordSeen(f.run(), repo)
}

// coordSeen is the run's observation whose key ends in |repo, or nil.
func coordSeen(run core.Run, repo string) *core.WatchObservation {
	for _, key := range slices.Sorted(maps.Keys(run.WatchObservations)) {
		if strings.HasSuffix(key, "|"+repo) {
			observation := run.WatchObservations[key]
			return &observation
		}
	}
	return nil
}

// restart builds a second coordinator over the fixture's folder, as a restart does
// (a RunBook over the same GitHub and clock).
func (f *coordWatch) restart() *Coordinator { return f.h.Restart() }

// stored is the run as the folder holds it.
func (f *coordWatch) stored() []core.Run { return f.h.storage.LoadRuns() }

// lastDetail is the detail of the stage's last attempt, or "(nil)" when there is none.
func coordLastDetail(run core.Run, stage string) string {
	attempt := run.LastAttempt(stage)
	if attempt == nil || attempt.Detail == nil {
		return "(nil)"
	}
	return *attempt.Detail
}

// lastDetail is the fixture run's last watch detail.
func (f *coordWatch) lastDetail() string { return coordLastDetail(f.run(), "watch") }

// coordPull is an open pull request in repo "acme/billing" at url, with head and base.
func coordPull(number int, url, head, base string) core.PullRequest {
	return core.PullRequest{Number: number, URL: url, State: core.PullRequestStateOpen, HeadSha: new(head), BaseRef: new(base)}
}

// coordEqual fails when got and want differ; nil and empty lists are the same.
func coordEqual[T any](t *testing.T, what string, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for index := range got {
		if fmt.Sprint(got[index]) != fmt.Sprint(want[index]) {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// coordCheck fails unless ok.
func coordCheck(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

// coordQuiet is a none outcome named name with the tone and then given.
func coordQuiet(name string, tone core.Tone, then core.Then) core.Outcome {
	return core.Outcome{ID: core.NewUUID(), Name: name, When: string(core.WatchEventNone), Tone: tone, Then: then}
}

// holdThen is the hold Then.
func holdThen() core.Then { return core.NewThen(core.ThenArgs{Action: core.ThenActionHold}) }

// gotoThen is a goto Then to the stage.
func gotoThen(stage string) core.Then {
	return core.NewThen(core.ThenArgs{Action: core.ThenActionGoto, To: []string{stage}})
}

// retryThen is the retry Then.
func retryThen() core.Then { return core.NewThen(core.ThenArgs{Action: core.ThenActionRetry}) }

// checking starts a forced watch pass and returns once it waits on a pull request read.
// The returned channel closes when the pass ends.
func (f *coordWatch) checking() <-chan struct{} {
	f.gh.HoldPulls()
	done := make(chan struct{})
	go func() {
		defer close(done)
		checkWatches(f.h.coord, true)
	}()
	testkit.Eventually(f.t, 10*time.Second, "the check waits on a pull request", func() bool {
		return f.gh.WaitingPulls() > 0
	})
	return done
}

// coordTwoRepositories: the run watches acme/billing and acme/web,
// each with a pull request on the run's branch at head a.
func coordTwoRepositories(t *testing.T, events []core.WatchEvent) *coordWatch {
	t.Helper()
	if events == nil {
		events = []core.WatchEvent{core.WatchEventNewChanges, core.WatchEventMerged, core.WatchEventClosed, core.WatchEventConflicted, core.WatchEventChangesRequested}
	}
	f := newH5m3Watch(t, watchOptions{events: events})
	f.update(func(run *core.Run) { run.Repos = []string{"acme/billing", "acme/web"} })
	branch := *f.run().Branch
	f.gh.PutPull(coordPull(201, "https://github.com/acme/billing/pull/201", "a", "main"), branch)
	f.gh.PutPull(withRepo(coordPull(88, "https://github.com/acme/web/pull/88", "a", "main"), "acme/web"), branch)
	return f
}

// withRepo sets a pull request's repository.
func withRepo(pull core.PullRequest, repo string) core.PullRequest {
	pull.Repo = repo
	return pull
}
