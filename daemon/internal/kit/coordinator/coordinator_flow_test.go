// Helpers for the trigger flow tests (coordinator_trigger_flow_test.go and the files beside it): the
// belt and triage approval stage fixtures, the pull request and ticket subjects, t0 and
// local(_:_:_:_:), and a harness that takes reviews, tickets and "opens no pull requests".

package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// flowT0 is the tests' t0: Date(timeIntervalSince1970: 1_800_000_000).
var flowT0 = time.Unix(1_800_000_000, 0)

// flowPull is an open pull request on acme/web, #57.
var flowPull = core.Subject{
	Kind: core.SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
	URL: "https://github.com/acme/web/pull/57", IsOpen: true, Base: new("develop"), Occurrence: new("100"),
}

// flowTicket is an open Jira ticket, PAY-31, with a repository.
var flowTicket = core.Subject{
	Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment",
	Body: "Up to what was paid.", URL: "https://acme.atlassian.net/browse/PAY-31", IsOpen: true,
}

// flowUnassigned is the ticket with no repository.
func flowUnassigned() core.Subject {
	ticket := flowTicket
	ticket.Repo = ""
	return ticket
}

// flowIssue is an open issue: Subject(repo:number:title:labels:).
func flowIssue(repo string, number int, title string, labels ...string) core.Subject {
	return core.Subject{Kind: core.SubjectKindIssue, Repo: repo, Number: number, Title: title, Labels: labels, IsOpen: true}
}

// flowDefaultIssue is the issue the harness lists by default: acme/billing #142 with the agent label.
func flowDefaultIssue() core.Subject {
	return core.Subject{
		Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV",
		Body: "Add an export.", Labels: []string{"agent"}, IsOpen: true,
	}
}

// flowTrigger is the trigger and repositories belt(_:id:stages:) gives each kind.
func flowTrigger(kind core.TriggerKind) (core.Trigger, []string) {
	switch kind {
	case core.TriggerKindIssues:
		return kind.Fresh(), []string{"acme/billing"}
	case core.TriggerKindReviews:
		return kind.Fresh(), []string{"acme/web"}
	case core.TriggerKindJira:
		query := core.DefaultJiraQuery()
		query.Jql = "project = PAY"
		return core.Trigger{Kind: core.TriggerKindJira, Jira: &query}, []string{"acme/billing"}
	case core.TriggerKindSchedule:
		task := core.ScheduledTask{Schedule: core.DefaultSchedule(), Title: "Sweep flaky tests", Task: "Fix or quarantine them."}
		task.Schedule.Days = core.ScheduleDaysWeekdays
		task.Schedule.Hour = 9
		return core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &task}, []string{"acme/platform"}
	default:
		return kind.Fresh(), []string{"acme/web", "acme/billing"}
	}
}

// flowBelt is a belt of kind with its trigger and repositories. A nil
// stages list gives the kind's default stages and forks; given stages come without forks.
func flowBelt(kind core.TriggerKind, id string, stages []core.Stage) core.Belt {
	trigger, repos := flowTrigger(kind)
	if id == "" {
		id = string(kind)
	}
	built, forks := stages, []core.Fork(nil)
	if stages == nil {
		built, forks = core.SampleStages(kind, core.SampleAgents(), true), core.SampleForks(kind)
	}
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: id, Name: kind.Title(), Trigger: trigger, Repos: repos, Stages: built, Forks: forks,
	})
}

// flowTriageApproval is Triage, then an approval gate.
func flowTriageApproval(kind core.TriggerKind) []core.Stage {
	triage := core.DefaultsStageForAgent(core.SampleAgent("triage"), nil, nil, nil, kind)
	approve := core.DefaultsStage(core.StageTypeApprove, []core.Stage{triage}, nil, []core.Stage{triage})
	return []core.Stage{triage, approve}
}

// flowReview is one review request the fake GitHub starts with.
type flowReview struct {
	pull     core.Subject
	reviewer string
	request  string
}

// flowRequest is a review request for the signed-in reviewer ("you").
func flowRequest(pull core.Subject, request string) flowReview {
	return flowReview{pull: pull, reviewer: "you", request: request}
}

// flowOptions are what a flow test changes about the harness.
type flowOptions struct {
	// issues are the GitHub issues (nil: none).
	issues []core.Subject
	// belts are saved before the coordinator loads, in order. Nothing is added when none are named.
	belts []core.Belt
	// reviews are the review requests the fake starts with.
	reviews []flowReview
	// tickets are the Jira tickets the fake starts with.
	tickets []core.Subject
	// now is the fake clock's start. Zero: testkit.ContractTime.
	now time.Time
	// noPulls is the opensPulls setting nil: a branch lookup opens no pull request.
	noPulls bool
	// workspaces wraps the folder workspaces the coordinator prepares checkouts with. Nil: unwrapped.
	workspaces func(inner runner.Workspaces) runner.Workspaces
}

// newFlowHarness builds a coordinator over a copy of the seeded folder with exactly the belts,
// review requests and tickets named. It is closed when the test ends. The shared harness
// (newHarness) always saves a default belt and opens merged pull requests, so the flow tests
// build their own.
func newFlowHarness(t *testing.T, opts flowOptions) *harness {
	t.Helper()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	now := opts.now
	if now.IsZero() {
		now = testkit.ContractTime
	}
	for _, belt := range opts.belts {
		saved, err := storage.SaveNewVersion(belt)
		if err != nil {
			t.Fatalf("save belt: %v", err)
		}
		record := store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: now, WatchOutcomes: true, ReviewOutcomes: true})
		if err := storage.SaveState(record); err != nil {
			t.Fatalf("save belt state: %v", err)
		}
	}
	var opensPulls *core.PullRequestState
	if !opts.noPulls {
		merged := core.PullRequestStateMerged
		opensPulls = &merged
	}
	h := &harness{
		t: t, root: root, storage: storage, clock: testkit.NewClock(now),
		github: github.NewFakeGitHub(opts.issues, nil, nil, opensPulls),
		jira:   jira.NewFakeJira(nil, nil, nil),
		Script: &harnessScript{queue: map[string][]string{}},
	}
	for _, review := range opts.reviews {
		h.github.RequestReview(review.pull, review.reviewer, review.request)
	}
	for _, ticket := range opts.tickets {
		h.jira.Put(ticket)
	}
	sim := runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, h.clock.Now, h.Script.next)
	var workspaces runner.Workspaces = runner.FolderWorkspaces{}
	if opts.workspaces != nil {
		workspaces = opts.workspaces(workspaces)
	}
	h.services = Services{
		Storage: storage, GitHub: h.github, Jira: h.jira, Runner: sim,
		Workspaces: workspaces, Clock: h.clock, IDs: &SequenceIDs{},
		Notifier: func(title, body string) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.notifications = append(h.notifications, title+": "+body)
		},
	}
	coord, err := New(h.services)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.coord = coord
	coord.SetChangeListener(func(resources []Resource) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.changes = append(h.changes, append([]Resource(nil), resources...))
	})
	t.Cleanup(func() { coord.Close() })
	return h
}

// flowLocal is local(month, day, hour, minute): a 2026 time in the Mac's own calendar.
func flowLocal(month, day, hour, minute int) time.Time {
	return time.Date(2026, time.Month(month), day, hour, minute, 0, 0, time.Local)
}

// flowLast is the last of runs, or the zero run when there are none.
func flowLast(runs []core.Run) core.Run {
	if len(runs) == 0 {
		return core.Run{}
	}
	return runs[len(runs)-1]
}

// flowCheck is `await coordinator.checkNow()` followed by the spawned work it started.
func flowCheck(h *harness) {
	h.coord.CheckNow(context.Background())
	h.coord.Idle()
}

// flowCheckOn is flowCheck for another coordinator over the same folder (a restart).
func flowCheckOn(c *Coordinator) {
	c.CheckNow(context.Background())
	c.Idle()
}

// flowUntil ticks until pred holds over the runs, at most 60 times.
// pred runs with the lock held, so it reads the runs without taking it.
func flowUntil(h *harness, pred func(runs []core.Run) bool) {
	h.Until("pred(h.nowRuns())", func() bool { return pred(h.nowRuns()) })
}

// flowIDs maps runs to their ids, in order.
func flowIDs(runs []core.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}

// flowKeys maps runs to their subjects' keys, in order.
func flowKeys(runs []core.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.Subject.Key)
	}
	return out
}

// flowFolder is the run folder of run.
func flowFolder(h *harness, run core.Run) string {
	return h.coord.Paths().RunFolder(run)
}
