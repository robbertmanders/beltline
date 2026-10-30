// SubjectBriefTests, and
// everyKindCanWatchAPullRequest and the trigger-kind parts of the brief tests.

package core

import (
	"strings"
	"testing"
)

// triggerTestBelt is a belt helper:
// a belt of kind with its trigger and repositories. Nil stages are the kind's
// defaults (and its forks); given stages have no forks.
func triggerTestBelt(kind TriggerKind, stages []Stage) Belt {
	trigger := Trigger{Kind: kind}
	var repos []string
	switch kind {
	case TriggerKindIssues:
		trigger.Issues = &IssueFilter{Label: "agent", Assignee: TriggerAssigneeAnyone, Poll: 60}
		repos = []string{"acme/billing"}
	case TriggerKindReviews:
		trigger.Reviews = &ReviewFilter{Reviewers: TriggerReviewersMe, Poll: 60}
		repos = []string{"acme/web"}
	case TriggerKindJira:
		trigger.Jira = &JiraQuery{Jql: "project = PAY", Poll: 60}
		repos = []string{"acme/billing"}
	case TriggerKindSchedule:
		trigger.Schedule = &ScheduledTask{Schedule: Schedule{Days: ScheduleDaysWeekdays, Hour: 9}, Title: "Sweep flaky tests", Task: "Fix or quarantine them."}
		repos = []string{"acme/platform"}
	default:
		repos = []string{"acme/web", "acme/billing"}
	}
	var forks []Fork
	if stages == nil {
		stages = SampleStages(kind, SampleAgents(), true)
		forks = SampleForks(kind)
	}
	return NewBeltFromTrigger(BeltFromTriggerArgs{ID: string(kind), Name: string(kind), Trigger: trigger, Repos: repos, Stages: stages, Forks: forks})
}

// TestEveryKindCanWatchAPullRequest
func TestEveryKindCanWatchAPullRequest(t *testing.T) {
	t.Parallel()
	for _, kind := range AllTriggerKind {
		if !triggerTestBelt(kind, nil).CanHave(StageTypeWatch) {
			t.Fatalf("a %s belt cannot have a watch", kind)
		}
	}
}

// triggerTestBrief renders the first stage of run with the subject brief
// context: /runs/r, the attempt folder /runs/r/01-a-1, base develop.
func triggerTestBrief(run Run) string {
	context := BriefContext{RunFolder: "/runs/r", AttemptFolder: "/runs/r/01-a-1", Base: "develop", Branch: "b", Agents: placeholderTestAgents()}
	return BriefRender(run, run.Stages[0], context)
}

// triggerTestPull is a pull request: acme/web#57 with its head and base.
func triggerTestPull() Subject {
	return Subject{Kind: SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Body: "Uses the new API.",
		URL: "https://github.com/acme/web/pull/57", Base: new("develop"), Occurrence: new("100"), IsOpen: true, Blockers: []string{}}
}

// triggerTestTicket is a Jira ticket: PAY-31 in acme/billing.
func triggerTestTicket() Subject {
	return Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment", Body: "Up to what was paid.",
		URL: "https://acme.atlassian.net/browse/PAY-31", IsOpen: true, Blockers: []string{}}
}

// TestAReviewBriefSaysWhatIsCheckedOut
func TestAReviewBriefSaysWhatIsCheckedOut(t *testing.T) {
	t.Parallel()
	text := triggerTestBrief(NewRun(RunArgs{Belt: triggerTestBelt(TriggerKindReviews, nil), Subject: triggerTestPull(), Now: engineT0}))
	if want := "## The pull request\n\nacme/web#57: Move checkout\nhttps://github.com/acme/web/pull/57\nIts head is checked out in your working folder. It merges into develop."; !strings.Contains(text, want) {
		t.Fatalf("brief lacks %q:\n%s", want, text)
	}
	if !strings.Contains(text, "git diff origin/develop...HEAD") {
		t.Fatalf("brief lacks the diff command:\n%s", text)
	}
}

// TestATicketBriefNamesItsRepository
func TestATicketBriefNamesItsRepository(t *testing.T) {
	t.Parallel()
	text := triggerTestBrief(NewRun(RunArgs{Belt: triggerTestBelt(TriggerKindJira, nil), Subject: triggerTestTicket(), Now: engineT0}))
	if want := "## The Jira ticket\n\nPAY-31: Refund part of a payment\nhttps://acme.atlassian.net/browse/PAY-31\nThe code is in acme/billing.\n\nUp to what was paid."; !strings.Contains(text, want) {
		t.Fatalf("brief lacks %q:\n%s", want, text)
	}
}

// TestAScheduledBriefSaysWhichRunItIs
func TestAScheduledBriefSaysWhichRunItIs(t *testing.T) {
	t.Parallel()
	subject := Subject{Kind: SubjectKindScheduled, Repo: "acme/platform", Key: "2026-09-29", Title: "Sweep", Body: "", IsOpen: true, Blockers: []string{}}
	text := triggerTestBrief(NewRun(RunArgs{Belt: triggerTestBelt(TriggerKindSchedule, nil), Subject: subject, Now: engineT0}))
	if want := "## The scheduled task\n\nSweep, for 2026-09-29\nThe code is in acme/platform.\n\n(The task has no description.)"; !strings.Contains(text, want) {
		t.Fatalf("brief lacks %q:\n%s", want, text)
	}
}
