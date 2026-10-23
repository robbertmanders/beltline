// Tests of the trigger model: the kinds, their settings and the decoding of the
// stored triggers.

package core

import (
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// btIssuesOf is the issues trigger with the given filter.
func btIssuesOf(f IssueFilter) Trigger { return Trigger{Kind: TriggerKindIssues, Issues: &f} }

// btReviewsOf is the reviews trigger with the given filter.
func btReviewsOf(f ReviewFilter) Trigger { return Trigger{Kind: TriggerKindReviews, Reviews: &f} }

// btJiraOf is the Jira trigger with the given query.
func btJiraOf(q JiraQuery) Trigger { return Trigger{Kind: TriggerKindJira, Jira: &q} }

// btManual is the manual trigger.
func btManual() Trigger { return Trigger{Kind: TriggerKindManual} }

// btInputs lists a set of inputs as a sorted slice, for comparing with a want list.
func btInputs(set map[Input]struct{}) []Input {
	var out []Input
	for _, in := range AllInput {
		if _, ok := set[in]; ok {
			out = append(out, in)
		}
	}
	return out
}

// TestABeltSavedBeforeOtherKindsWatchesIssues: a trigger object with no kind is an issues trigger.
func TestABeltSavedBeforeOtherKindsWatchesIssues(t *testing.T) {
	t.Parallel()
	json := `{"repos":["a/b"],"label":"agent","assignee":"me","poll":300,"base":"","retriageOnEdit":false,"stopOnClose":true,"commentOnHold":true}`
	trigger := btDecode[Trigger](t, json)
	want := btIssuesOf(IssueFilter{Label: "agent", Assignee: TriggerAssigneeMe, Poll: 300})
	if d := testkit.Diff(want, trigger); d != "" {
		t.Fatalf("trigger differs:\n%s", d)
	}
}

// TestABeltSavedWithEverythingOnItsTriggerMovesItToTheBelt reads a belt whose
// repositories, base and policies sit on its trigger, and saves it again.
func TestABeltSavedWithEverythingOnItsTriggerMovesItToTheBelt(t *testing.T) {
	t.Parallel()
	json := `{"id":"b","name":"B","version":3,"stages":[],"trigger":{"kind":"jira","repos":["a/b"],"jql":"project = PAY","poll":30,"base":"develop","retriageOnEdit":false,"stopOnClose":true,"commentOnHold":false}}`
	belt := btDecode[Belt](t, json)
	if d := testkit.Diff(btJiraOf(JiraQuery{Jql: "project = PAY", Poll: 30}), belt.Trigger()); d != "" {
		t.Fatalf("trigger differs:\n%s", d)
	}
	if d := testkit.Diff([]string{"a/b"}, belt.Repos()); d != "" {
		t.Fatalf("repos differ:\n%s", d)
	}
	if belt.Base != "develop" {
		t.Fatalf("base = %q, want develop", belt.Base)
	}
	if d := testkit.Diff(Policies{RetriageOnEdit: false, StopOnClose: true, CommentOnHold: false, WaitForBlockers: true}, belt.Policies); d != "" {
		t.Fatalf("policies differ:\n%s", d)
	}
	again := btDecode[Belt](t, btEncode(t, belt))
	if d := testkit.Diff(belt, again); d != "" {
		t.Fatalf("saved belt does not read back the same:\n%s", d)
	}
}

// TestATriggerSavesOnlyItsKindsFields checks the keys a Jira trigger writes.
func TestATriggerSavesOnlyItsKindsFields(t *testing.T) {
	t.Parallel()
	trigger := btJiraOf(JiraQuery{Jql: "project = PAY", Poll: 60})
	json := btEncode(t, trigger)
	if !strings.Contains(json, "jql") {
		t.Errorf("no jql in %s", json)
	}
	for _, key := range []string{"label", "schedule", "repos"} {
		if strings.Contains(json, key) {
			t.Errorf("%q written for a Jira trigger: %s", key, json)
		}
	}
	if d := testkit.Diff(trigger, btDecode[Trigger](t, json)); d != "" {
		t.Fatalf("round trip differs:\n%s", d)
	}
}

// TestTriggerMarshalExactBytes checks the wire form of each kind: its kind,
// then its own settings, sorted.
func TestTriggerMarshalExactBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		trigger Trigger
		want    string
	}{
		{"issues", btIssuesOf(DefaultIssueFilter()), `{"kind":"issues","label":"agent","assignee":"anyone","poll":60}`},
		{"reviews", btReviewsOf(DefaultReviewFilter()), `{"kind":"reviews","reviewers":"me","poll":60}`},
		{"jira", btJiraOf(DefaultJiraQuery()), `{"kind":"jira","jql":"","poll":60}`},
		{"schedule", vtScheduleTrigger(ScheduledTask{Schedule: DefaultSchedule(), Title: "t", Task: "x"}),
			`{"kind":"schedule","schedule":{"days":"weekdays","weekday":2,"hour":9,"minute":0},"title":"t","task":"x"}`},
		{"manual", btManual(), `{"kind":"manual"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := btEncode(t, tc.trigger); got != tc.want {
				t.Fatalf("got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestTriggerKindDefaultsToIssues covers the missing-kind default of Trigger.init(from:).
func TestTriggerKindDefaultsToIssues(t *testing.T) {
	t.Parallel()
	got := btDecode[Trigger](t, `{}`)
	if d := testkit.Diff(btIssuesOf(IssueFilter{Assignee: TriggerAssigneeAnyone, Poll: 60}), got); d != "" {
		t.Fatalf("empty trigger:\n%s", d)
	}
	if got.Kind != TriggerKindIssues {
		t.Fatalf("kind = %q", got.Kind)
	}
}

// TestTriggerRejectsAnUnknownKind checks the error for a kind it does not know.
func TestTriggerRejectsAnUnknownKind(t *testing.T) {
	t.Parallel()
	var trigger Trigger
	if err := btUnmarshal(`{"kind":"webhook"}`, &trigger); err == nil {
		t.Fatal("unknown kind decoded without error")
	}
}

// TestFiltersDecodeTheirDefaults covers the defaults of each filter's init(from:).
func TestFiltersDecodeTheirDefaults(t *testing.T) {
	t.Parallel()
	if d := testkit.Diff(IssueFilter{Assignee: TriggerAssigneeAnyone, Poll: 60}, btDecode[IssueFilter](t, `{}`)); d != "" {
		t.Errorf("issue filter: %s", d)
	}
	if d := testkit.Diff(ReviewFilter{Reviewers: TriggerReviewersMe, Poll: 60}, btDecode[ReviewFilter](t, `{}`)); d != "" {
		t.Errorf("review filter: %s", d)
	}
	if d := testkit.Diff(JiraQuery{Poll: 60}, btDecode[JiraQuery](t, `{}`)); d != "" {
		t.Errorf("jira query: %s", d)
	}
	task := btDecode[ScheduledTask](t, `{}`)
	if d := testkit.Diff(ScheduledTask{Schedule: DefaultSchedule()}, task); d != "" {
		t.Errorf("scheduled task: %s", d)
	}
	if d := testkit.Diff(DefaultSchedule(), btDecode[Schedule](t, `{"days":"weekdays","weekday":2,"hour":9,"minute":0}`)); d != "" {
		t.Errorf("schedule: %s", d)
	}
}

// TestTriggerPollAndSummary covers the poll accessors and the one-line summaries.
func TestTriggerPollAndSummary(t *testing.T) {
	t.Parallel()
	trigger := btIssuesOf(IssueFilter{Label: "agent", Assignee: TriggerAssigneeAnyone, Poll: 60})
	if p := trigger.Poll(); p == nil || *p != 60 {
		t.Fatalf("poll = %v, want 60", p)
	}
	ninety := 90
	trigger.SetPoll(&ninety)
	if p := trigger.Poll(); p == nil || *p != 90 {
		t.Fatalf("after SetPoll, poll = %v, want 90", p)
	}
	trigger.SetPoll(nil)
	if p := trigger.Poll(); p == nil || *p != 90 {
		t.Fatal("SetPoll(nil) changed the poll")
	}
	if btManual().Poll() != nil {
		t.Fatal("manual trigger has a poll")
	}
	if got := btIssuesOf(IssueFilter{Label: "bug"}).Summary(); got != "label: bug" {
		t.Errorf("issues summary = %q", got)
	}
	if got := btReviewsOf(ReviewFilter{Reviewers: TriggerReviewersMeAndTeams}).Summary(); got != "your and your teams' reviews" {
		t.Errorf("reviews summary = %q", got)
	}
	if got := btJiraOf(JiraQuery{Jql: " "}).Summary(); got != " " {
		t.Errorf("jira summary = %q", got)
	}
	if got := btJiraOf(JiraQuery{}).Summary(); got != "no query" {
		t.Errorf("empty jira summary = %q", got)
	}
	if got := btManual().Summary(); got != "started by you" {
		t.Errorf("manual summary = %q", got)
	}
}

// TestTriggerEntryLineAndScheduledKey covers TriggerEntry.line and scheduledKey.
func TestTriggerEntryLineAndScheduledKey(t *testing.T) {
	t.Parallel()
	one := TriggerEntry{ID: "1", Trigger: btJiraOf(JiraQuery{Jql: "project = PAY"}), Repos: []string{"a/b"}}
	if got := one.Line(); got != "project = PAY → a/b" {
		t.Errorf("jira line = %q", got)
	}
	several := TriggerEntry{ID: "2", Trigger: btIssuesOf(IssueFilter{Label: "agent"}), Repos: []string{"a/b", "c/d"}}
	if got := several.Line(); got != "label: agent · 2 repos" {
		t.Errorf("issues line = %q", got)
	}
	slot := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	if got := one.ScheduledKey(slot, time.UTC); got != "2026-09-29" {
		t.Errorf("legacy entry key = %q, want the date", got)
	}
	other := TriggerEntry{ID: "4", Trigger: btManual()}
	if got := other.ScheduledKey(slot, time.UTC); got != "4@2026-09-29" {
		t.Errorf("entry key = %q, want 4@2026-09-29", got)
	}
}

// TestAStageNeedsWhatItsWorkspaceAndTypeNeed is ported with literal stages in
// place of Defaults.stage.
func TestAStageNeedsWhatItsWorkspaceAndTypeNeed(t *testing.T) {
	t.Parallel()
	worktree := WorkspaceWorktree
	readOnly := WorkspaceWorktreeReadOnly
	build := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Workspace: &worktree, Outcomes: []Outcome{}})
	verify := NewStage(StageArgs{ID: "verify", Type: StageTypeAgent, Workspace: &readOnly, Outcomes: []Outcome{}})
	watch := NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Outcomes: []Outcome{}})
	if d := testkit.Diff([]Input{InputBaseBranch}, btInputs(build.Needs())); d != "" {
		t.Errorf("build needs:\n%s", d)
	}
	if d := testkit.Diff([]Input{InputBranch, InputPullRequest}, btInputs(build.Produces())); d != "" {
		t.Errorf("build produces:\n%s", d)
	}
	if d := testkit.Diff([]Input{InputBranch}, btInputs(verify.Needs())); d != "" {
		t.Errorf("verify needs:\n%s", d)
	}
	if len(verify.Produces()) != 0 {
		t.Errorf("verify produces %v, want none", btInputs(verify.Produces()))
	}
	if d := testkit.Diff([]Input{InputPullRequest}, btInputs(watch.Needs())); d != "" {
		t.Errorf("watch needs:\n%s", d)
	}
}

// TestStageNeedsATicketOnATicketWatch covers the ticket branch of Stage.needs.
func TestStageNeedsATicketOnATicketWatch(t *testing.T) {
	t.Parallel()
	ticket := WatchTargetTicket
	stage := NewStage(StageArgs{ID: "w", Type: StageTypeWatch, Watch: &ticket, Outcomes: []Outcome{}})
	if d := testkit.Diff([]Input{InputTicket}, btInputs(stage.Needs())); d != "" {
		t.Fatalf("ticket watch needs:\n%s", d)
	}
}

// TestRunHasTheInputsItsSubjectGivesIt covers Run.has.
func TestRunHasTheInputsItsSubjectGivesIt(t *testing.T) {
	t.Parallel()
	branch := "beltline/issue-1"
	issue := Run{Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: 1}}
	pr := Run{Subject: Subject{Kind: SubjectKindPullRequest, Repo: "a/b", Number: 2}}
	task := Run{Subject: Subject{Kind: SubjectKindManual, Key: "3"}}
	withBranch := Run{Subject: Subject{Kind: SubjectKindManual, Key: "4"}, Branch: &branch}
	if !issue.Has(InputBaseBranch) || !issue.Has(InputBranch) {
		t.Error("base branch and code to read are always there")
	}
	if !issue.Has(InputTicket) || !pr.Has(InputPullRequest) {
		t.Error("an issue has a ticket to watch and a pull request has a pull request")
	}
	if pr.Has(InputTicket) {
		t.Error("a pull request run has no ticket")
	}
	if task.Has(InputPullRequest) || task.Has(InputTicket) {
		t.Error("a task has no pull request or ticket")
	}
	if !withBranch.Has(InputPullRequest) {
		t.Error("a run with a branch has a pull request")
	}
}

// TestWeekdaysFireOnWorkingDaysOnly is ported from ScheduleTests (the model part).
func TestWeekdaysFireOnWorkingDaysOnly(t *testing.T) {
	t.Parallel()
	schedule := Schedule{Days: ScheduleDaysWeekdays, Weekday: 2, Hour: 9, Minute: 0}
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 9, day, hour, minute, 0, 0, time.UTC)
	}
	// 29 September 2026 is a Tuesday.
	btAssertTime(t, "latest 29 10:00", schedule.Latest(at(29, 10, 0), time.UTC), at(29, 9, 0))
	btAssertTime(t, "latest 29 08:00", schedule.Latest(at(29, 8, 0), time.UTC), at(28, 9, 0))
	btAssertTime(t, "latest 28 08:00", schedule.Latest(at(28, 8, 0), time.UTC), at(25, 9, 0))
	btAssertTime(t, "next 25 10:00", schedule.Next(at(25, 10, 0), time.UTC), at(28, 9, 0))
}

// TestWeeklyFiresOnItsDay is ported from ScheduleTests (the model part).
func TestWeeklyFiresOnItsDay(t *testing.T) {
	t.Parallel()
	schedule := Schedule{Days: ScheduleDaysWeekly, Weekday: 4, Hour: 6, Minute: 30}
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 9, day, hour, minute, 0, 0, time.UTC)
	}
	btAssertTime(t, "latest 29 10:00", schedule.Latest(at(29, 10, 0), time.UTC), at(23, 6, 30))
	btAssertTime(t, "next 29 10:00", schedule.Next(at(29, 10, 0), time.UTC), at(30, 6, 30))
	if got := schedule.Summary(); got != "Wednesdays at 06:30" {
		t.Errorf("summary = %q, want Wednesdays at 06:30", got)
	}
}

// TestAFiringIsKeyedByItsDate is ported from ScheduleTests (the model part).
func TestAFiringIsKeyedByItsDate(t *testing.T) {
	t.Parallel()
	slot := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	if got := ScheduleKey(slot, time.UTC); got != "2026-09-29" {
		t.Fatalf("key = %q, want 2026-09-29", got)
	}
}

// TestScheduleLatestAndNextAreNilWhenNothingFires checks that Latest and Next return nil when nothing fires.
func TestScheduleLatestAndNextAreNilWhenNothingFires(t *testing.T) {
	t.Parallel()
	never := Schedule{Days: ScheduleDays("never"), Hour: 9}
	if never.Latest(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC), time.UTC) != nil {
		t.Error("Latest found a firing for a schedule that never fires")
	}
	if never.Next(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC), time.UTC) != nil {
		t.Error("Next found a firing for a schedule that never fires")
	}
}

// TestScheduleSummaries covers each schedule's one-line summary, including the
// clamping of the weekday to 1 through 7.
func TestScheduleSummaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		want     string
		schedule Schedule
	}{
		{"every day at 07:05", Schedule{Days: ScheduleDaysEveryDay, Hour: 7, Minute: 5}},
		{"weekdays at 09:00", Schedule{Days: ScheduleDaysWeekdays, Hour: 9}},
		{"Sundays at 00:00", Schedule{Days: ScheduleDaysWeekly, Weekday: 1}},
		{"Saturdays at 00:00", Schedule{Days: ScheduleDaysWeekly, Weekday: 9}},
		{"Sundays at 00:00", Schedule{Days: ScheduleDaysWeekly, Weekday: 0}},
	}
	for _, tc := range cases {
		if got := tc.schedule.Summary(); got != tc.want {
			t.Errorf("summary = %q, want %q", got, tc.want)
		}
	}
}

// btAssertTime fails the test unless got is a time at the same instant as want.
func btAssertTime(t *testing.T, what string, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: no time, want %s", what, want)
	}
	if !got.Equal(want) {
		t.Fatalf("%s: got %s, want %s", what, got, want)
	}
}

// TestARunSavedBeforeOtherKindsIsAboutAnIssue checks that a run saved before other kinds is about an issue.
// The run is testModelRun, without an engine start.
func TestARunSavedBeforeOtherKindsIsAboutAnIssue(t *testing.T) {
	t.Parallel()
	run := testModelRun()
	obj := testJSONMap(t, run)
	issue, ok := obj["issue"].(map[string]any)
	if !ok {
		t.Fatalf("issue = %v", obj["issue"])
	}
	delete(issue, "kind")
	delete(issue, "key")
	decoded := testRunFrom(t, obj)
	if d := testDiffEmpty(run, decoded); d != "" {
		t.Errorf("decoded run differs:\n%s", d)
	}
	if decoded.ID != "issues:acme/billing#142" {
		t.Errorf("id = %q", decoded.ID)
	}
	if decoded.Folder() != "issues/acme/billing/142" {
		t.Errorf("folder = %q", decoded.Folder())
	}
}

// TestARunSavedWithItsTriggerReadsItsPoliciesAndBase
// A run saved with a trigger copy and no policies or base reads them from it, and
// saves them itself without the trigger.
func TestARunSavedWithItsTriggerReadsItsPoliciesAndBase(t *testing.T) {
	t.Parallel()
	obj := testJSONMap(t, testModelRun())
	delete(obj, "policies")
	delete(obj, "base")
	obj["trigger"] = map[string]any{
		"kind": "issues", "repos": []string{"acme/billing"}, "label": "agent", "base": "develop",
		"retriageOnEdit": false, "stopOnClose": true, "commentOnHold": false,
	}
	decoded := testRunFrom(t, obj)
	// Policies(retriageOnEdit:stopOnClose:commentOnHold:) leaves waitForBlockers on.
	want := DefaultPolicies()
	want.RetriageOnEdit, want.CommentOnHold = false, false
	if decoded.Policies != want {
		t.Errorf("policies = %+v, want %+v", decoded.Policies, want)
	}
	if decoded.Base != "develop" {
		t.Errorf("base = %q, want develop", decoded.Base)
	}
	saved := testJSONMap(t, decoded)
	if _, ok := saved["trigger"]; ok {
		t.Error("saved run still has a trigger")
	}
	if saved["base"] != "develop" {
		t.Errorf("saved base = %v, want develop", saved["base"])
	}
	if _, ok := saved["policies"]; !ok {
		t.Error("saved run has no policies")
	}
}

// TestTriggerEntryDecodesStrictKeys checks a trigger entry's required keys and
// that its trigger is decoded with the same rules.
// TestRunSourceDecodesStrictKeys checks a run source's required keys.
// TestScheduleDecodesStrictKeys checks that every schedule key is required and
// that its days must be a case.
