// EpicFlowTests: a run on a
// ticket that waits for its blockers, an issue that waits for the issue that
// blocks it, the ticket watch that moves on once a child changes, and a run
// over two repositories whose branch only changed one of them.

package coordinator

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// epicSubject is the epic ticket PAY-40.
var epicSubject = core.Subject{
	Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-40", Title: "Split invoicing",
	Labels: []string{"epic"}, IsOpen: true,
}

// epicChildOf is child(_:done:labels:blockers:): a ticket under PAY-40.
func epicChildOf(key string, done bool, labels []string, blockers ...string) core.Subject {
	return core.Subject{
		Kind: core.SubjectKindJira, Repo: "acme/billing", Key: key, Title: "Part " + key, Labels: labels,
		IsOpen: !done, Parent: new("PAY-40"), Blockers: blockers,
	}
}

// epicTicketWatch is ticketWatch(_:): a watch on the run's own ticket, with the default outcomes.
func epicTicketWatch() core.Stage {
	target := core.WatchTargetTicket
	return core.NewStage(core.StageArgs{
		ID: "watch", Type: core.StageTypeWatch, Name: new("Watch the ticket"),
		Outcomes: core.DefaultsTicketWatchOutcomes(), Watch: &target,
	})
}

// epicApproval is the approval stage of the trigger flow tests.
func epicApproval() core.Stage { return core.DefaultsStage(core.StageTypeApprove, nil, nil, nil) }

// TestABlockedTicketWaitsUntilItsBlockerIsDone
func TestABlockedTicketWaitsUntilItsBlockerIsDone(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindJira, "", flowTriageApproval(core.TriggerKindJira))},
		tickets: []core.Subject{epicChildOf("PAY-43", false, nil, "PAY-42")},
	})
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d, want none", n)
	}
	if got := waitingReasons(h.Coord().ReadWaitingSubjects()); !reflect.DeepEqual(got, []string{"waiting on PAY-42"}) {
		t.Fatalf("waiting = %v, want [waiting on PAY-42]", got)
	}
	h.jira.Put(epicChildOf("PAY-43", false, nil))
	flowCheck(h)
	if got := flowKeys(h.Coord().Runs()); !reflect.DeepEqual(got, []string{"PAY-43"}) {
		t.Fatalf("keys = %v, want [PAY-43]", got)
	}
	if n := len(h.Coord().ReadWaitingSubjects()); n != 0 {
		t.Fatalf("waiting = %d, want none", n)
	}
}

// waitingReasons maps waiting rows to their reasons, in order.
func waitingReasons(rows []core.WaitingSubject) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Reason)
	}
	return out
}

// TestAnIssueWaitsForTheIssueThatBlocksIt
func TestAnIssueWaitsForTheIssueThatBlocksIt(t *testing.T) {
	t.Parallel()
	blocker := flowIssue("acme/billing", 5, "First")
	blocked := flowIssue("acme/billing", 6, "Second", "agent")
	blocked.Blockers = []string{"#5"}
	h := newFlowHarness(t, flowOptions{
		issues: []core.Subject{blocked},
		belts:  []core.Belt{flowBelt(core.TriggerKindIssues, "", flowTriageApproval(core.TriggerKindIssues))},
	})
	h.github.Put(blocker)
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d, want none", n)
	}
	closed := blocker
	closed.IsOpen = false
	h.github.Put(closed)
	flowCheck(h)
	if got := sortedNumbers(h.Coord().Runs()); !reflect.DeepEqual(got, []int{6}) {
		t.Fatalf("numbers = %v, want [6]", got)
	}
}

// TestTheTicketWatchMovesOnOnceAChildChanged
func TestTheTicketWatchMovesOnOnceAChildChanged(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindJira, "", []core.Stage{epicTicketWatch(), epicApproval()})},
		tickets: []core.Subject{epicSubject, epicChildOf("PAY-41", false, nil)},
		now:     flowT0,
	})
	h.jira.Unmatch("PAY-41")
	flowCheck(h)
	run := h.Run()
	if run == nil {
		t.Fatal("no run on the epic")
	}
	if run.Subject.Key != "PAY-40" {
		t.Fatalf("key = %s, want PAY-40", run.Subject.Key)
	}
	if run.State != core.RunStateRunning {
		t.Fatalf("state = %s, want running", run.State)
	}
	// The first look only learns what is there.
	flowCheck(h)
	obs, ok := h.Run().TicketObservations["watch"]
	if !ok {
		t.Fatal("no ticket observation for the watch")
	}
	if got := slices.Sorted(maps.Keys(obs.Children)); !reflect.DeepEqual(got, []string{"PAY-41"}) {
		t.Fatalf("children seen = %v, want [PAY-41]", got)
	}
	if h.Run().StageID != "watch" {
		t.Fatalf("stage = %s, want watch", h.Run().StageID)
	}
	h.jira.SetStatus("PAY-41", "Done", true)
	flowCheck(h)
	if h.Run().StageID != "watch" {
		t.Fatalf("stage = %s after a change that is still quiet, want watch", h.Run().StageID)
	}
	h.clock.Set(flowT0.Add(time.Duration(core.TicketWatchQuiet+1) * time.Second))
	flowCheck(h)
	after := h.Run()
	if after.StageID != "approve" {
		t.Fatalf("stage = %s, want approve", after.StageID)
	}
	if after.State != core.RunStateWaiting {
		t.Fatalf("state = %s, want waiting", after.State)
	}
	if d := after.Attempts[0].Detail; d == nil || !containsText(*d, "PAY-41 finished") {
		t.Fatalf("detail = %v, want it to name PAY-41 finished", d)
	}
}

// containsText reports whether text contains part.
func containsText(text, part string) bool { return strings.Contains(text, part) }

// epicOneOfTwo is oneOfTwo(): a Jira run on two repositories, with a build agent and a
// watch on its pull requests, opening none of them itself.
func epicOneOfTwo(t *testing.T) *harness {
	t.Helper()
	build := core.DefaultsStageForAgent(core.SampleAgent("build"), nil, nil, nil, core.TriggerKindIssues)
	build.Brief = "Build the requested change and write the result to {output}."
	watch := core.DefaultsStage(core.StageTypeWatch, []core.Stage{build}, nil, []core.Stage{build})
	belt := flowBelt(core.TriggerKindJira, "", []core.Stage{build, watch, epicApproval()})
	belt.SetRepos([]string{"acme/billing", "acme/infra"})
	return newFlowHarness(t, flowOptions{belts: []core.Belt{belt}, tickets: []core.Subject{flowTicket}, now: flowT0, noPulls: true})
}

// TestARepositoryWithCommitsStillNeedsItsPullRequest
func TestARepositoryWithCommitsStillNeedsItsPullRequest(t *testing.T) {
	t.Parallel()
	h := epicOneOfTwo(t)
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].StageID == "watch" })
	branch := h.Run().Branch
	if branch == nil {
		t.Fatal("the run has no branch")
	}
	h.github.OpenPull("acme/billing", *branch, core.PullRequestStateOpen)
	h.github.SetAhead("acme/infra", *branch, 2)
	flowCheck(h)
	h.clock.Set(flowT0.Add(watchesGrace + time.Second))
	flowCheck(h)
	run := h.Run()
	if run.State != core.RunStateHeld {
		t.Fatalf("state = %s, want held", run.State)
	}
	want := "no pull request from " + *branch + " in acme/infra"
	if run.Reason == nil || *run.Reason != want {
		t.Fatalf("reason = %v, want %q", run.Reason, want)
	}
}
