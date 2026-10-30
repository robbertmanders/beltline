// Coordinator tests of the trigger flows: the review request, issue, Jira, schedule and manual flows,
// one belt each.

package coordinator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// readBytes reads path, failing the test when it cannot.
func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// writeBytes writes data to path, failing the test when it cannot.
func writeBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// flowReviewBelt is a review belt with the default stages.
func flowReviewBelt() core.Belt { return flowBelt(core.TriggerKindReviews, "", nil) }

// TestEachRequestIsOneRunWhoseAgentPostsTheReview
func TestEachRequestIsOneRunWhoseAgentPostsTheReview(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{flowReviewBelt()}, reviews: []flowReview{flowRequest(flowPull, "100")}})
	h.Script.add("review", "blocking findings")
	h.Until("r := h.nowRun(); return r != nil && r.State == core.RunStateDone", func() bool { r := h.nowRun(); return r != nil && r.State == core.RunStateDone })
	run := h.Run()
	if !hasNoticeContaining(h.Coord().Notices(), run.Subject.Ref()+" done") || hasNoticeContaining(h.Coord().Notices(), " shipped") {
		t.Fatalf("notices = %+v, want the review to say it is done, not shipped", h.Coord().Notices())
	}
	if run.ID != "reviews:acme/web#57@100" {
		t.Fatalf("run id = %s, want reviews:acme/web#57@100", run.ID)
	}
	if got, want := optionalTexts(attemptOutcomes(run.Attempts)), []string{"blocking findings", "submitted"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %v, want %v", got, want)
	}
	folder := flowFolder(h, *run)
	brief := readText(t, filepath.Join(folder, "02-post-review-1", "brief.md"))
	want := "Submit the review written in " + filepath.Join(folder, "01-review-1", "review.md") + " on #57 in acme/web"
	if !strings.Contains(brief, want) {
		t.Fatalf("brief lacks %q", want)
	}
	for _, call := range h.github.Calls() {
		if call.Kind != github.FakeCallLatestReviewRequest {
			t.Fatalf("github call %s, want only latest review request reads", call.Kind)
		}
	}

	// Answered: no new run until someone asks again.
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d after a check, want 1", n)
	}
	h.github.RequestReview(flowPull, "you", "101")
	flowCheck(h)
	flowUntil(h, func(runs []core.Run) bool {
		last := flowLast(runs)
		return len(runs) > 0 && last.State == core.RunStateDone
	})
	if got, want := flowIDs(h.Coord().Runs()), []string{"reviews:acme/web#57@100", "reviews:acme/web#57@101"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("run ids = %v, want %v", got, want)
	}
}

// TestTheReviewReadsThePullRequestsHead
func TestTheReviewReadsThePullRequestsHead(t *testing.T) {
	t.Parallel()
	// The belt overrides the reviewed repository's base; the review still reads the pull request's head.
	belt := flowReviewBelt()
	belt.RepositoryBases = map[string]string{"acme/web": "release/web"}
	rec := &basesRecordingWorkspaces{}
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{belt}, reviews: []flowReview{flowRequest(flowPull, "100")}, workspaces: recordingWorkspacesFor(rec)})
	h.Tick()
	run := h.Run()
	if run == nil {
		t.Fatal("no run")
	}
	brief := readText(t, filepath.Join(flowFolder(h, *run), "01-review-1", "brief.md"))
	if !strings.Contains(brief, "It merges into develop.") {
		t.Fatal("brief lacks the pull request's base")
	}
	headChecked := false
	for _, spec := range rec.prepared() {
		if spec.Checkout.Kind != runner.WorkspaceCheckoutKindPullRequest {
			continue
		}
		headChecked = true
		if spec.Checkout.Number != 57 || spec.Base != "develop" {
			t.Fatalf("head checkout = %+v from %q, want pull request 57 from its base develop", spec.Checkout, spec.Base)
		}
	}
	if !headChecked {
		t.Fatal("no checkout of the pull request's head was prepared")
	}
	reads := run.Stages[0]
	reads.Workspace = core.WorkspaceWorktreeReadOnly
	want := runner.WorkspaceCheckout{Kind: runner.WorkspaceCheckoutKindPullRequest, Number: 57}
	if got := sessionsCheckout(*run, reads); got != want {
		t.Fatalf("checkout = %+v, want %+v", got, want)
	}
	dir := runner.WorkspaceSpecDirectory(want, h.Coord().Paths().WorkFolder(*run))
	if filepath.Base(dir) != "readonly" {
		t.Fatalf("checkout folder = %s, want readonly", dir)
	}
}

// TestSwitchingAccountsKeepsOneRunForAPullRequest
func TestSwitchingAccountsKeepsOneRunForAPullRequest(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))},
		reviews: []flowReview{flowRequest(flowPull, "100")}, now: flowT0,
	})
	h.Tick()
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	before := *h.Run()
	folder := flowFolder(h, before)
	h.github.SetLogin("other")
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	if after := *h.Run(); !reflect.DeepEqual(after, before) {
		t.Fatal("the run changed when the signed-in account did")
	}
	last := h.github.Calls()[len(h.github.Calls())-1]
	if last.Kind != github.FakeCallLatestReviewRequest || last.Repo != "acme/web" || last.Number != 57 || last.Reviewer != "you" {
		t.Fatalf("last call = %+v, want the latest request for you on acme/web #57", last)
	}
	h.github.SetLogin("you")
	calls := h.github.Calls()
	flowCheck(h)
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	after := *h.Run()
	if after.State != core.RunStateWaiting || after.ID != before.ID || after.StageID != before.StageID {
		t.Fatalf("run = %s at %s, want %s waiting at %s", after.State, after.StageID, before.ID, before.StageID)
	}
	if !reflect.DeepEqual(attemptIDs(after.Attempts), attemptIDs(before.Attempts)) {
		t.Fatal("attempts changed")
	}
	if flowFolder(h, after) != folder {
		t.Fatal("run folder changed")
	}
	if !reflect.DeepEqual(h.github.Calls(), calls) {
		t.Fatal("checks as you made calls that the earlier checks did not")
	}
	if n := h.github.LoginCount(); n != 4 {
		t.Fatalf("login count = %d, want 4", n)
	}
	h.github.WithdrawReview("acme/web", 57, "you")
	flowCheck(h)
	if state := h.Run().State; state != core.RunStateDropped {
		t.Fatalf("state = %s after the request was withdrawn, want dropped", state)
	}
}

// attemptIDs maps attempts to their ids, in order.
func attemptIDs(attempts []core.Attempt) []string {
	out := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, attempt.ID)
	}
	return out
}

// TestANewRunRecordsTheNewAccountAndAnsweredRequestsStayAnswered
func TestANewRunRecordsTheNewAccountAndAnsweredRequestsStayAnswered(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{flowReviewBelt()}})
	h.github.RequestReview(flowPull, "other", "200")
	h.github.SetLogin("other")
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateDone })
	before := *h.Run()
	if before.Subject.Reviewer == nil || *before.Subject.Reviewer != "other" {
		t.Fatalf("reviewer = %v, want other", before.Subject.Reviewer)
	}
	saved := h.storage.LoadRuns()
	if len(saved) == 0 || saved[0].Subject.Reviewer == nil || *saved[0].Subject.Reviewer != "other" {
		t.Fatal("the saved run does not record the reviewer")
	}
	if !containsCall(h.github.Calls(), github.FakeCall{Kind: github.FakeCallLatestReviewRequest, Repo: "acme/web", Number: 57, Reviewer: "other"}) {
		t.Fatal("no latest review request read for other")
	}
	count := h.github.LoginCount()
	h.github.SetLogin("you")
	flowCheck(h)
	h.github.SetLogin("other")
	flowCheck(h)
	flowCheck(h)
	if n := h.github.LoginCount(); n != count+3 {
		t.Fatalf("login count = %d, want %d", n, count+3)
	}
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	if after := *h.Run(); !reflect.DeepEqual(after, before) {
		t.Fatal("the run changed")
	}
}

// containsCall reports whether calls holds want.
func containsCall(calls []github.FakeCall, want github.FakeCall) bool {
	return slices.ContainsFunc(calls, func(call github.FakeCall) bool {
		return reflect.DeepEqual(call, want)
	})
}

// TestALegacySavedRunAdoptsTheFirstChecksAccount
func TestALegacySavedRunAdoptsTheFirstChecksAccount(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))},
		reviews: []flowReview{flowRequest(flowPull, "100")}, now: flowT0,
	})
	h.Tick()
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	before := *h.Run()
	stripReviewerFromRunFile(t, filepath.Join(flowFolder(h, before), "run.json"))
	if got := h.storage.LoadRuns()[0].Subject.Reviewer; got != nil {
		t.Fatalf("reviewer = %v before the first check, want none", *got)
	}
	restored := h.RestartWith(h.idleSimulatedRunner())
	flowCheckOn(restored)
	adopted := restored.Runs()
	if len(adopted) == 0 {
		t.Fatal("no run after the restart")
	}
	got := adopted[0]
	if got.ID != before.ID {
		t.Fatalf("id = %s, want %s", got.ID, before.ID)
	}
	if flowFolder(h, got) != flowFolder(h, before) {
		t.Fatal("run folder changed")
	}
	if !sameEncoding(t, got.Attempts, before.Attempts) || got.StageID != before.StageID || got.State != before.State {
		t.Fatalf("the adopted run differs from the saved one (state %s/%s, stage %s/%s)", got.State, before.State, got.StageID, before.StageID)
	}
	if saved := h.storage.LoadRuns()[0]; saved.Subject.Reviewer == nil || *saved.Subject.Reviewer != "you" {
		rv := core.ValueOr(saved.Subject.Reviewer, "nil")
		t.Fatalf("the saved run did not adopt the reviewer you: saved=%s memory=%v", rv, restored.Runs()[0].Subject.Reviewer)
	}
	h.github.SetLogin("other")
	flowCheckOn(restored)
	if state := restored.Runs()[0].State; state != core.RunStateWaiting {
		t.Fatalf("state = %s after another account checked, want waiting", state)
	}
	calls := h.github.Calls()
	last := calls[len(calls)-1]
	if last.Kind != github.FakeCallLatestReviewRequest || last.Reviewer != "you" {
		t.Fatalf("last call = %+v, want the latest request for you", last)
	}
}

// sameEncoding reports whether a and b encode to the same JSON. Values read back from disk carry
// another time zone, so a comparison by encoding (instants) stands in for comparing times.
func sameEncoding(t *testing.T, a, b any) bool {
	t.Helper()
	left, err := jsonx.Marshal(a)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	right, err := jsonx.Marshal(b)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(left) == string(right)
}

// stripReviewerFromRunFile removes the reviewer from a saved run's subject, as a run saved before reviewers were recorded has it.
func stripReviewerFromRunFile(t *testing.T, path string) {
	t.Helper()
	members, err := jsonx.Members(readBytes(t, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(readBytes(t, path), &doc); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	subject, ok := doc["issue"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no issue object (%d members)", path, len(members))
	}
	delete(subject, "reviewer")
	data, err := jsonx.Marshal(doc)
	if err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	writeBytes(t, path, data)
}

// TestAFailedRequestLookupLeavesTheRunAloneAndNotesTheFailure
func TestAFailedRequestLookupLeavesTheRunAloneAndNotesTheFailure(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))},
		reviews: []flowReview{flowRequest(flowPull, "100")},
	})
	h.Tick()
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	before := *h.Run()
	h.github.SetLogin("other")
	h.github.SetReviewRequestFailing("acme/web", true)
	flowCheck(h)
	if after := *h.Run(); !reflect.DeepEqual(after, before) {
		t.Fatal("the run changed when the lookup failed")
	}
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	if !hasNoticeFor(h.Coord().Notices(), before.ID, "Could not read the review request") {
		t.Fatal("no notice about the failed review request lookup")
	}
}

// hasNoticeFor reports whether a notice about run id contains text.
func hasNoticeFor(notices []Notice, id, text string) bool {
	return slices.ContainsFunc(notices, func(notice Notice) bool {
		return notice.RunID != nil && *notice.RunID == id && strings.Contains(notice.Text, text)
	})
}

// TestTheLoginIsReadOncePerBeltCheckNotPerRepository
func TestTheLoginIsReadOncePerBeltCheckNotPerRepository(t *testing.T) {
	t.Parallel()
	reviews := flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))
	reviews.SetRepos(append(reviews.Repos(), "acme/platform"))
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{reviews}, reviews: []flowReview{flowRequest(flowPull, "100")}, now: flowT0})
	flowCheck(h)
	if n := h.github.LoginCount(); n != 1 {
		t.Fatalf("login count = %d after the first check, want 1", n)
	}
	h.github.SetLogin("other")
	flowCheck(h)
	if n := h.github.LoginCount(); n != 2 {
		t.Fatalf("login count = %d after the second check, want 2", n)
	}
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
}

// TestAFailedNewRequestLookupDoesNotStartAFallbackRun
func TestAFailedNewRequestLookupDoesNotStartAFallbackRun(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))},
		reviews: []flowReview{flowRequest(flowPull, "100")},
	})
	h.github.SetReviewRequestFailing("acme/web", true)
	flowCheck(h)
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d, want none", n)
	}
	if !hasNotice(h.Coord().Notices(), "Could not read the review request") {
		t.Fatal("no notice about the failed review request lookup")
	}
	h.github.SetReviewRequestFailing("acme/web", false)
	flowCheck(h)
	if run := h.Run(); run == nil || run.ID != "reviews:acme/web#57@100" {
		t.Fatalf("run = %v, want reviews:acme/web#57@100", run)
	}
}

// hasNotice reports whether any notice contains text.
func hasNotice(notices []Notice, text string) bool {
	return slices.ContainsFunc(notices, func(notice Notice) bool {
		return strings.Contains(notice.Text, text)
	})
}

// TestAWithdrawnRequestStopsItsRun
func TestAWithdrawnRequestStopsItsRun(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindReviews, "", flowTriageApproval(core.TriggerKindReviews))},
		reviews: []flowReview{flowRequest(flowPull, "100")},
	})
	h.Tick()
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	if state := h.Run().State; state != core.RunStateWaiting {
		t.Fatalf("state = %s, want waiting", state)
	}
	h.github.WithdrawReview("acme/web", 57, "you")
	flowCheck(h)
	run := h.Run()
	if run.State != core.RunStateDropped {
		t.Fatalf("state = %s, want dropped", run.State)
	}
	if run.Reason == nil || *run.Reason != "the pull request was closed or no longer asks for your review" {
		t.Fatalf("reason = %v", run.Reason)
	}
}

// TestATicketStaysOnItsBeltUntilDone
func TestATicketStaysOnItsBeltUntilDone(t *testing.T) {
	t.Parallel()
	triage := core.DefaultsStageForAgent(core.SampleAgent("triage"), nil, nil, nil, core.TriggerKindJira)
	approve := core.DefaultsStage(core.StageTypeApprove, []core.Stage{triage}, nil, []core.Stage{triage})
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{flowBelt(core.TriggerKindJira, "", []core.Stage{triage, approve})},
		tickets: []core.Subject{flowUnassigned()},
	})
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	if repo := h.Run().Subject.Repo; repo != "acme/billing" {
		t.Fatalf("repo = %s, want acme/billing", repo)
	}

	// Moving it out of the query keeps the run; done stops it.
	h.jira.Unmatch("PAY-31")
	flowCheck(h)
	if state := h.Run().State; state != core.RunStateWaiting {
		t.Fatalf("state = %s after the ticket left the query, want waiting", state)
	}
	done := flowUnassigned()
	done.IsOpen = false
	h.jira.Put(done)
	h.jira.Unmatch("PAY-31")
	flowCheck(h)
	run := h.Run()
	if run.State != core.RunStateDropped {
		t.Fatalf("state = %s after the ticket is done, want dropped", run.State)
	}
	if run.Reason == nil || *run.Reason != "the ticket is done" {
		t.Fatalf("reason = %v, want the ticket is done", run.Reason)
	}
}

// TestAHeldTicketGetsTheQuestions: with the hold comment on, a held ticket gets
// the questions as a comment.
func TestAHeldTicketGetsTheQuestions(t *testing.T) {
	t.Parallel()
	belt := flowDefaultJiraBelt()
	belt.Policies.CommentOnHold = true
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{belt},
		tickets: []core.Subject{flowUnassigned()},
	})
	h.Script.add("triage", "needs info")
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateHeld })
	h.Coord().Idle()
	calls := h.jira.Calls()
	if len(calls) == 0 {
		t.Fatal("no comment was posted to the ticket")
	}
	if calls[0].Key != "PAY-31" {
		t.Fatalf("comment key = %s, want PAY-31", calls[0].Key)
	}
	if !strings.HasPrefix(calls[0].Body, "Beltline paused this ticket at **Triage**: needs info.") {
		t.Fatalf("comment body = %q", calls[0].Body)
	}
}

// flowDefaultJiraBelt is the default Jira belt.
func flowDefaultJiraBelt() core.Belt { return flowBelt(core.TriggerKindJira, "", nil) }

// TestAScheduleFiresOnceATimeAndSkipsWhileBusy
func TestAScheduleFiresOnceATimeAndSkipsWhileBusy(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts: []core.Belt{flowBelt(core.TriggerKindSchedule, "", flowTriageApproval(core.TriggerKindSchedule))},
		now:   flowLocal(9, 29, 8, 0),
	})
	h.Tick()
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d before the time, want none", n)
	}

	h.clock.Set(flowLocal(9, 29, 9, 30))
	h.Tick()
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	runs := h.Coord().Runs()
	if got := flowKeys(runs); !reflect.DeepEqual(got, []string{"2026-09-29"}) {
		t.Fatalf("keys = %v, want [2026-09-29]", got)
	}
	if title := runs[0].Subject.Title; title != "Sweep flaky tests" {
		t.Fatalf("title = %s", title)
	}
	if state := runs[0].State; state != core.RunStateWaiting {
		t.Fatalf("state = %s, want waiting", state)
	}

	// The next morning the first run is still waiting, so that time is skipped.
	h.clock.Set(flowLocal(9, 30, 9, 30))
	h.Tick()
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	notices := h.Coord().Notices()
	if len(notices) == 0 || notices[0].Text != "Schedule skipped its 2026-09-30 run: 09-29 is still on the belt" {
		t.Fatalf("first notice = %v, want the skipped run", notices)
	}

	// Away over the weekend: one run for the latest time, not one per day.
	h.Coord().Drop(h.Run().ID, nil)
	h.clock.Set(flowLocal(10, 5, 12, 0))
	h.Tick()
	if got := flowKeys(h.Coord().Runs()); !reflect.DeepEqual(got, []string{"2026-09-29", "2026-10-05"}) {
		t.Fatalf("keys = %v, want [2026-09-29 2026-10-05]", got)
	}
	if last := h.storage.LoadBelts()[0].Fired[core.TriggerEntryLegacyID]; !last.Equal(flowLocal(10, 5, 9, 0)) {
		t.Fatalf("last fired = %v, want %v", last, flowLocal(10, 5, 9, 0))
	}
}

// TestAChangedScheduleCountsFromWhenItIsSaved
func TestAChangedScheduleCountsFromWhenItIsSaved(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{flowBelt(core.TriggerKindSchedule, "", nil)}, now: flowLocal(9, 29, 8, 0)})
	h.Tick()
	h.clock.Set(flowLocal(9, 29, 10, 0))
	h.Tick()
	h.Coord().Drop(h.Run().ID, nil)
	// 09:45 has passed today, but only since the change was saved.
	changed := h.Coord().Belts()[0].Belt
	task := core.ScheduledTask{Schedule: core.DefaultSchedule(), Title: "Sweep flaky tests", Task: "Fix or quarantine them."}
	task.Schedule.Days = core.ScheduleDaysWeekdays
	task.Schedule.Hour = 9
	task.Schedule.Minute = 45
	changed.SetTrigger(core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &task})
	if _, err := h.Coord().SaveBelt(changed); err != nil {
		t.Fatalf("save: %v", err)
	}
	h.Tick()
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d after the save, want 1", n)
	}
	h.clock.Set(flowLocal(9, 30, 10, 0))
	h.Tick()
	if got := flowKeys(h.Coord().Runs()); !reflect.DeepEqual(got, []string{"2026-09-29", "2026-09-30"}) {
		t.Fatalf("keys = %v, want [2026-09-29 2026-09-30]", got)
	}
}

// TestATaskYouStartGoesToAPullRequest
func TestATaskYouStartGoesToAPullRequest(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{flowBelt(core.TriggerKindManual, "tasks", nil)}, noPulls: true})
	run, err := h.Coord().StartTask("tasks", "acme/web", " Tidy the settings copy ", "Sentence case.", nil)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if ref := run.Subject.Ref(); ref != "T1" {
		t.Fatalf("ref = %s, want T1", ref)
	}
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].State == core.RunStateWaiting })
	h.Coord().Decide(run.ID, "approved", nil)
	flowUntil(h, func(runs []core.Run) bool { return len(runs) > 0 && runs[0].StageID == "watch" })
	if branch := h.Run().Branch; branch == nil || *branch != "beltline/tasks-task-1" {
		t.Fatalf("branch = %v, want beltline/tasks-task-1", branch)
	}
	opened := h.github.OpenPull("acme/web", "beltline/tasks-task-1", core.PullRequestStateMerged)
	flowCheck(h)
	after := h.Run()
	if after.State != core.RunStateShipped {
		t.Fatalf("state = %s, want shipped", after.State)
	}
	if pull := after.PullRequest(); pull == nil || !reflect.DeepEqual(*pull, opened) {
		t.Fatalf("pull request = %+v, want %+v", pull, opened)
	}

	second, err := h.Coord().StartTask("tasks", "acme/billing", "Another", "", nil)
	if err != nil {
		t.Fatalf("second task: %v", err)
	}
	if second.Subject.Key != "2" {
		t.Fatalf("second key = %s, want 2", second.Subject.Key)
	}
	if _, err := h.Coord().StartTask("tasks", "acme/other", "No", "", nil); err == nil {
		t.Fatal("a repository the belt does not have was accepted")
	}
	if _, err := h.Coord().StartTask("tasks", "acme/web", "  ", "", nil); err == nil {
		t.Fatal("a task with no title was accepted")
	}
}

// TestOnlyAManualBeltTakesTasks
func TestOnlyAManualBeltTakesTasks(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{issues: []core.Subject{flowDefaultIssue()}, belts: []core.Belt{defaultBelt()}})
	if _, err := h.Coord().StartTask("issues", "acme/billing", "No", "", nil); err == nil {
		t.Fatal("a task was accepted on a belt that takes none")
	}
}

// TestATaskYouStartIsOpen checks the task's subject is open, as a new subject defaults it
// (the tasks golden records isOpen true).
func TestATaskYouStartIsOpen(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{flowBelt(core.TriggerKindManual, "tasks", nil)}, noPulls: true})
	run, err := h.Coord().StartTask("tasks", "acme/web", "Open task", "", nil)
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if !run.Subject.IsOpen {
		t.Fatal("a task you start is closed, want open")
	}
}
