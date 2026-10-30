// Coordinator tests of whole runs over a watch stage, driven through Until and CheckNow. The GitHub
// fake opens no pull request on its own, so a watch finds only the pull requests the test opens. Also
// the GitHub-backed integration case that needs no external service (its fixture is in
// coordinator_watch_fixture_test.go).

package coordinator

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
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

// coordFlow is what a flow test's harness is built from.
type coordFlow struct {
	belt core.Belt
	// issues are the GitHub issues. Nil: one issue, acme/billing #142, labelled agent.
	issues []core.Subject
	// tickets are the Jira tickets.
	tickets []core.Subject
	// reviews are the review requests the fake holds.
	reviews []github.FakeReviewRequest
}

// newH5m3Flow builds a coordinator over a copy of the seeded folder with the flow's belt saved,
// GitHub that opens no pull request, and the simulated runner.
func newH5m3Flow(t *testing.T, flow coordFlow) *harness {
	t.Helper()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	now := sessionSampleT0
	saved, err := storage.SaveNewVersion(flow.belt)
	if err != nil {
		t.Fatalf("save belt: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: now, WatchOutcomes: true, ReviewOutcomes: true})); err != nil {
		t.Fatalf("save belt state: %v", err)
	}
	issues := flow.issues
	if issues == nil {
		issues = []core.Subject{{
			Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV",
			Body: "Add an export.", Labels: []string{"agent"}, IsOpen: true,
		}}
	}
	clock := testkit.NewClock(now)
	h := &harness{
		t: t, root: root, storage: storage, clock: clock,
		github: github.NewFakeGitHub(issues, flow.reviews, nil, nil),
		jira:   jira.NewFakeJira(flow.tickets, nil, nil),
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

// coordBuildWatchWrapUp is build, then a watch, then Wrap up.
func coordBuildWatchWrapUp() core.Belt {
	agents := core.SampleAgents()
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), nil, nil, nil, core.TriggerKindIssues)
	build.Brief = "Build the requested change and write the result to {output}."
	watch := core.DefaultsStage(core.StageTypeWatch, []core.Stage{build}, nil, []core.Stage{build})
	wrap := core.DefaultsStageForAgent(core.SampleAgent("wrap-up"), []core.Stage{build, watch}, nil, []core.Stage{build, watch}, core.TriggerKindIssues)
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", agents, true)
	belt.Stages = []core.Stage{build, watch, wrap}
	belt.Forks = nil
	return belt
}

// coordWithNone drops the watch outcomes for drop and adds a none outcome.
func coordWithNone(belt core.Belt, drop []core.WatchEvent, name string, then core.Then) core.Belt {
	index := -1
	for i, stage := range belt.Stages {
		if stage.Type == core.StageTypeWatch {
			index = i
			break
		}
	}
	dropped := map[string]struct{}{}
	for _, event := range drop {
		dropped[string(event)] = struct{}{}
	}
	var outcomes []core.Outcome
	for _, outcome := range belt.Stages[index].Outcomes {
		if _, ok := dropped[outcome.When]; !ok {
			outcomes = append(outcomes, outcome)
		}
	}
	outcomes = append(outcomes, core.Outcome{ID: core.NewUUID(), Name: name, When: string(core.WatchEventNone), Tone: core.ToneNeutral, Then: then})
	belt.Stages[index].Outcomes = outcomes
	return belt
}

// multiRepo is the belt on a Jira query over acme/billing and
// acme/infra, with the Jira ticket and no hold comments.
func multiRepo(belt core.Belt) coordFlow {
	trigger := core.TriggerKindJira.Fresh()
	query := core.DefaultJiraQuery()
	query.Jql = "project = PAY"
	trigger.Jira = &query
	belt.Triggers = []core.TriggerEntry{{ID: "jira", Trigger: trigger, Repos: []string{"acme/billing", "acme/infra"}}}
	belt.Policies.CommentOnHold = false
	ticket := basesTestTicket
	ticket.IsOpen = true
	return coordFlow{belt: belt, issues: []core.Subject{}, tickets: []core.Subject{ticket}}
}

// reviewPull is a pull request under review in acme/web.
func reviewPull() core.Subject {
	base := "develop"
	occurrence := "100"
	return core.Subject{
		Kind: core.SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
		URL: "https://github.com/acme/web/pull/57", Base: &base, Occurrence: &occurrence, IsOpen: true,
	}
}

// untilStage ticks until the run is at its watch stage.
func untilStage(h *harness, stage string) {
	h.Until("r := h.nowRun(); return r != nil && r.StageID == stage", func() bool { r := h.nowRun(); return r != nil && r.StageID == stage })
}

// coordCheckNow checks every trigger, pull request and part, then ticks.
func coordCheckNow(h *harness) { h.coord.CheckNow(context.Background()) }

// watchOutcome is the last watch attempt with an outcome, or "(nil)".
func watchOutcome(run *core.Run) string {
	if run == nil {
		return "(nil)"
	}
	for index := len(run.Attempts) - 1; index >= 0; index-- {
		attempt := run.Attempts[index]
		if attempt.StageID == "watch" && attempt.Outcome != nil {
			return *attempt.Outcome
		}
	}
	return "(nil)"
}

// runCopy is the harness's first run as a copy, failing the test when there is none.
func runCopy(t *testing.T, h *harness) core.Run {
	t.Helper()
	run := h.Run()
	if run == nil {
		t.Fatal("no run")
	}
	return *run
}

// reloadRun round-trips a run through its JSON, as a restart reads it.
func reloadRun(t *testing.T, run core.Run) core.Run {
	t.Helper()
	data, err := jsonx.Marshal(run)
	if err != nil {
		t.Fatalf("encode run: %v", err)
	}
	var loaded core.Run
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	return loaded
}

// TestAWatchFindsThePullRequestAnAgentOpenedAndMovesOnWhenItMerges
func TestAWatchFindsThePullRequestAnAgentOpenedAndMovesOnWhenItMerges(t *testing.T) {
	t.Parallel()
	h := newH5m3Flow(t, coordFlow{belt: coordBuildWatchWrapUp()})
	untilStage(h, "watch")
	run := runCopy(t, h)
	coordCheck(t, run.State == core.RunStateRunning, "state = %v, want running", run.State)
	coordCheck(t, run.Branch != nil && *run.Branch == "beltline/issue-142", "branch = %v", run.Branch)

	opened := h.github.OpenPull("acme/billing", *run.Branch, core.PullRequestStateOpen)
	coordCheckNow(h)
	observed := opened
	observed.Behind = new(0)
	run = runCopy(t, h)
	sameJSON(t, "pull request", run.PullRequest(), &observed)
	coordCheck(t, run.State == core.RunStateRunning, "state after watch = %v, want running", run.State)

	h.github.SetPullState(opened.Number, core.PullRequestStateMerged, nil)
	coordCheckNow(h)
	coordCheck(t, watchOutcome(h.Run()) == "merged", "outcome = %s, want merged", watchOutcome(h.Run()))
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateShipped", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateShipped })
	run = runCopy(t, h)
	coordCheck(t, run.PullRequest() != nil && run.PullRequest().State == core.PullRequestStateMerged, "pull request is not merged")
	brief := h.storage.Paths.RunFolder(run) + "/03-wrap-up-1/brief.md"
	text := readText(t, brief)
	coordCheck(t, strings.Contains(text, "Its pull request: #"+strconv.Itoa(opened.Number)+", "+opened.URL+" (merged)"), "brief does not name the merged pull request")
}

// TestAPullRequestThatNeverShowsUpHoldsTheRun
func TestAPullRequestThatNeverShowsUpHoldsTheRun(t *testing.T) {
	t.Parallel()
	h := newH5m3Flow(t, coordFlow{belt: coordBuildWatchWrapUp()})
	untilStage(h, "watch")
	coordCheckNow(h)
	coordCheck(t, runCopy(t, h).State == core.RunStateRunning, "state = %v, want running", runCopy(t, h).State)
	h.clock.Set(sessionSampleT0.Add(watchesGrace + time.Second))
	coordCheckNow(h)
	run := runCopy(t, h)
	coordCheck(t, run.State == core.RunStateHeld, "state = %v, want held", run.State)
	coordCheck(t, run.Reason != nil && *run.Reason == "no pull request from beltline/issue-142", "reason = %v", run.Reason)
}

// TestAllRepositoriesMustFinishBeforeReportingMerged
func TestAllRepositoriesMustFinishBeforeReportingMerged(t *testing.T) {
	t.Parallel()
	h := newH5m3Flow(t, multiRepo(coordBuildWatchWrapUp()))
	untilStage(h, "watch")
	branch := *runCopy(t, h).Branch
	// Identical PR numbers in different repositories are distinct.
	for _, repo := range []string{"acme/billing", "acme/infra"} {
		h.github.PutPull(core.PullRequest{Number: 7, URL: "https://github.com/" + repo + "/pull/7", Repo: repo, State: core.PullRequestStateOpen}, branch)
	}
	coordCheckNow(h)
	coordCheck(t, len(runCopy(t, h).PullRequests) == 2, "pull requests = %d, want 2", len(runCopy(t, h).PullRequests))
	h.github.SetPullState(7, core.PullRequestStateMerged, new("acme/billing"))
	coordCheckNow(h)
	coordCheck(t, runCopy(t, h).State == core.RunStateRunning, "state after one merge = %v, want running", runCopy(t, h).State)
	h.github.SetPullState(7, core.PullRequestStateMerged, new("acme/infra"))
	coordCheckNow(h)
	run := runCopy(t, h)
	attempt := run.LastAttempt("watch")
	coordCheck(t, attempt != nil && attempt.Outcome != nil && *attempt.Outcome == "merged", "outcome = %v, want merged", attempt)
	coordCheck(t, attempt.Detail != nil && *attempt.Detail == "#7 in acme/billing, #7 in acme/infra", "detail = %v", attempt.Detail)
}

// mixedReason is the hold reason when acme/billing's pull request merged and acme/infra's closed.
func mixedReason(merged, closed core.PullRequest) string {
	return "#" + strconv.Itoa(merged.Number) + " in acme/billing merged, but #" + strconv.Itoa(closed.Number) + " in acme/infra closed without merging"
}

// TestMissingRepositoryGraceStartsWhenFirstPullAppearsAndSurvivesReload
func TestMissingRepositoryGraceStartsWhenFirstPullAppearsAndSurvivesReload(t *testing.T) {
	t.Parallel()
	h := newH5m3Flow(t, multiRepo(coordBuildWatchWrapUp()))
	untilStage(h, "watch")
	h.clock.Set(sessionSampleT0.Add(watchesGrace - time.Second))
	opened := h.github.OpenPull("acme/billing", *runCopy(t, h).Branch, core.PullRequestStateMerged)
	coordCheckNow(h)
	run := runCopy(t, h)
	coordCheck(t, run.FirstPullRequestAt != nil, "first pull request time is not set")
	first := *run.FirstPullRequestAt
	loaded := reloadRun(t, run)
	coordCheck(t, loaded.FirstPullRequestAt != nil && loaded.FirstPullRequestAt.Equal(first.Time), "first pull request time did not survive a reload")
	sameJSON(t, "pull requests after reload", loaded.PullRequests, []core.PullRequest{opened})
	h.clock.Set(sessionSampleT0.Add(watchesGrace + time.Second))
	coordCheckNow(h)
	coordCheck(t, runCopy(t, h).State == core.RunStateRunning, "state inside the grace = %v, want running", runCopy(t, h).State)
	h.clock.Set(first.Add(watchesGrace + time.Second))
	coordCheckNow(h)
	run = runCopy(t, h)
	coordCheck(t, run.State == core.RunStateHeld, "state after the grace = %v, want held", run.State)
	coordCheck(t, run.Reason != nil && strings.Contains(*run.Reason, "acme/infra"), "reason = %v, want acme/infra", run.Reason)
}

// TestALatePullRequestJoinsTheWatch
func TestALatePullRequestJoinsTheWatch(t *testing.T) {
	t.Parallel()
	h := newH5m3Flow(t, multiRepo(coordBuildWatchWrapUp()))
	untilStage(h, "watch")
	branch := *runCopy(t, h).Branch
	h.github.OpenPull("acme/billing", branch, core.PullRequestStateMerged)
	coordCheckNow(h)
	coordCheck(t, runCopy(t, h).State == core.RunStateRunning, "state = %v, want running", runCopy(t, h).State)
	h.github.OpenPull("acme/infra", branch, core.PullRequestStateOpen)
	coordCheckNow(h)
	run := runCopy(t, h)
	coordCheck(t, len(run.PullRequests) == 2, "pull requests = %d, want 2", len(run.PullRequests))
	coordCheck(t, run.State == core.RunStateRunning, "state = %v, want running", run.State)
}

// TestAReviewBeltWatchesThePullRequestUnderReview
func TestAReviewBeltWatchesThePullRequestUnderReview(t *testing.T) {
	t.Parallel()
	watch := core.DefaultsStage(core.StageTypeWatch, nil, nil, nil)
	trigger := core.TriggerKindReviews.Fresh()
	belt := core.SampleBelt([]string{"acme/web"}, "agent", core.SampleAgents(), true)
	belt.Triggers = []core.TriggerEntry{{ID: "reviews", Trigger: trigger, Repos: []string{"acme/web"}}}
	belt.Stages = []core.Stage{watch}
	belt.Forks = nil
	pull := reviewPull()
	h := newH5m3Flow(t, coordFlow{
		belt: belt, issues: []core.Subject{}, reviews: []github.FakeReviewRequest{{Pull: pull, Request: "100"}},
	})
	h.github.PutPull(core.PullRequest{Number: 57, URL: pull.URL, State: core.PullRequestStateMerged}, "someone/checkout")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateDone", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateDone })
	run := runCopy(t, h)
	coordCheck(t, run.PullRequest() != nil && run.PullRequest().Number == 57, "pull request = %v, want 57", run.PullRequest())
}

// hasOutcome reports whether the run has an attempt with outcome.
func hasOutcome(run core.Run, outcome string) bool {
	return slices.ContainsFunc(run.Attempts, func(attempt core.Attempt) bool {
		return attempt.Outcome != nil && *attempt.Outcome == outcome
	})
}

// TestClosureBeatsOpenSignalsAndMergeWaitsForEveryPull checks that a closure beats open signals, and a merge waits for every pull request,
// with closed, like merged, waiting for every pull request.
func TestClosureBeatsOpenSignalsAndMergeWaitsForEveryPull(t *testing.T) {
	t.Parallel()
	quiet := func() core.Belt { return coordWithNone(coordBuildWatchWrapUp(), nil, "quiet", holdThen()) }
	mixed := newH5m3Flow(t, multiRepo(quiet()))
	untilStage(mixed, "watch")
	branch := *runCopy(t, mixed).Branch
	mergedPull := mixed.github.OpenPull("acme/billing", branch, core.PullRequestStateMerged)
	closedPull := mixed.github.OpenPull("acme/infra", branch, core.PullRequestStateClosed)
	coordCheckNow(mixed)
	run := runCopy(t, mixed)
	coordCheck(t, run.State == core.RunStateHeld, "mixed state = %v, want held", run.State)
	coordCheck(t, run.Reason != nil && *run.Reason == mixedReason(mergedPull, closedPull), "mixed reason = %v", run.Reason)
	coordCheck(t, !hasOutcome(run, "quiet"), "quiet was reported for a mixed merge and close")

	beside := newH5m3Flow(t, multiRepo(quiet()))
	untilStage(beside, "watch")
	sideBranch := *runCopy(t, beside).Branch
	open := beside.github.OpenPull("acme/billing", sideBranch, core.PullRequestStateOpen)
	beside.github.OpenPull("acme/infra", sideBranch, core.PullRequestStateMerged)
	beside.github.SetChecks(open.Number, 1, 4, false)
	coordCheckNow(beside)
	run = runCopy(t, beside)
	coordCheck(t, watchOutcome(&run) == "checks failing", "beside outcome = %s, want checks failing", watchOutcome(&run))
	coordCheck(t, !hasOutcome(run, "quiet"), "quiet was reported beside checks failing")

	quietPulls := newH5m3Flow(t, multiRepo(quiet()))
	untilStage(quietPulls, "watch")
	quietBranch := *runCopy(t, quietPulls).Branch
	quietPulls.github.OpenPull("acme/billing", quietBranch, core.PullRequestStateMerged)
	stillOpen := quietPulls.github.OpenPull("acme/infra", quietBranch, core.PullRequestStateOpen)
	coordCheckNow(quietPulls)
	run = runCopy(t, quietPulls)
	coordCheck(t, watchOutcome(&run) == "quiet", "quiet outcome = %s, want quiet", watchOutcome(&run))
	coordCheck(t, run.Reason != nil && *run.Reason == "quiet", "quiet reason = %v", run.Reason)
	coordCheck(t, strings.Contains(coordLastDetail(run, "watch"), "#"+strconv.Itoa(stillOpen.Number)), "quiet detail %q does not name #%d", coordLastDetail(run, "watch"), stillOpen.Number)

	closedOpen := newH5m3Flow(t, multiRepo(quiet()))
	untilStage(closedOpen, "watch")
	both := *runCopy(t, closedOpen).Branch
	closedOpen.github.OpenPull("acme/billing", both, core.PullRequestStateClosed)
	other := closedOpen.github.OpenPull("acme/infra", both, core.PullRequestStateOpen)
	closedOpen.github.SetChecks(other.Number, 2, 3, false)
	coordCheckNow(closedOpen)
	run = runCopy(t, closedOpen)
	// One closed beside an open one is not closed: the open one's failing checks are reported.
	coordCheck(t, watchOutcome(&run) == "checks failing", "closed-open outcome = %s, want checks failing", watchOutcome(&run))

	missing := newH5m3Flow(t, multiRepo(quiet()))
	untilStage(missing, "watch")
	missing.github.OpenPull("acme/billing", *runCopy(t, missing).Branch, core.PullRequestStateClosed)
	coordCheckNow(missing)
	run = runCopy(t, missing)
	coordCheck(t, watchOutcome(&run) == "closed", "missing outcome = %s, want closed", watchOutcome(&run))
	coordCheck(t, run.Reason != nil && *run.Reason == "closed", "missing reason = %v", run.Reason)

	early := newH5m3Flow(t, multiRepo(coordWithNone(coordBuildWatchWrapUp(), []core.WatchEvent{core.WatchEventMerged}, "quiet", holdThen())))
	untilStage(early, "watch")
	early.github.OpenPull("acme/billing", *runCopy(t, early).Branch, core.PullRequestStateMerged)
	coordCheckNow(early)
	run = runCopy(t, early)
	coordCheck(t, run.State == core.RunStateRunning, "early state = %v, want running", run.State)
	coordCheck(t, watchOutcome(&run) == "(nil)", "early outcome = %s, want none", watchOutcome(&run))
}

// TestAFailedFetchLeavesTheRunAndALaterQuietFetchTakesNone checks that a failed fetch leaves the run, and a later quiet fetch takes none
// (its coordinator half: the watch is the fixture's, configured with a quiet outcome only).
func TestAFailedFetchLeavesTheRunAndALaterQuietFetchTakesNone(t *testing.T) {
	t.Parallel()
	f := newH5m3Watch(t, watchOptions{events: []core.WatchEvent{}, none: new(coordQuiet("quiet", core.ToneNeutral, holdThen()))})
	f.gh.SetFailing("acme/billing", true)
	before := f.run()
	f.check()
	sameJSON(t, "run after a failed fetch", f.run(), before)
	coordCheck(t, countOutcome(f.watchOutcomes(), "quiet") == 0, "quiet was taken on a failed fetch")
	f.gh.SetFailing("acme/billing", false)
	f.open()
	f.check()
	run := f.run()
	coordCheck(t, run.State == core.RunStateHeld, "state = %v, want held", run.State)
	coordCheck(t, lastOutcome(run, "watch") == "quiet", "outcome = %s, want quiet", lastOutcome(run, "watch"))
	coordCheck(t, coordLastDetail(run, "watch") == watchesNoneDetail("#201"), "detail = %q", coordLastDetail(run, "watch"))
	stored := f.stored()
	coordCheck(t, len(stored) == 1 && lastOutcome(stored[0], "watch") == "quiet", "saved outcome is not quiet")
}
