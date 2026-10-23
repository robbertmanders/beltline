// TriggerEntry, RunSource,
// Trigger with its Kind, Assignee and Reviewers; the filters are in
// model_trigger_filters.go, Schedule in model_trigger_schedule.go and the Belt
// extensions in model_trigger_belt.go).
//
// Date parameters are time.Time and the calendar is a *time.Location (nil
// for the local zone, since core never reads the clock or the environment.

package core

import (
	"fmt"
	"time"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TriggerEntryLegacyID is the id a single-trigger belt gets when it is read, so
// the same file decodes to the same entry every time.
const TriggerEntryLegacyID = "1"

// TriggerEntry is one configured source on a belt: its kind settings and its
// own repositories. The id stays put when the entry is edited or reordered. A
// belt saved with a single trigger reads back as the entry `1`.
type TriggerEntry struct {
	ID      string   `json:"id"`
	Trigger Trigger  `json:"trigger"`
	Repos   []string `json:"repos"`
}

// Line is one line about the entry, such as "label: agent · 3 repos".
func (e TriggerEntry) Line() string {
	where := fmt.Sprintf("%d repos", len(e.Repos))
	if len(e.Repos) == 1 {
		where = e.Repos[0]
	}
	if e.Trigger.Kind == TriggerKindJira {
		return e.Trigger.Summary() + " → " + where
	}
	return e.Trigger.Summary() + " · " + where
}

// ScheduledKey is the task key a firing of this entry gets. The original single
// schedule keeps the date key its runs already use; every other schedule names
// itself.
func (e TriggerEntry) ScheduledKey(slot time.Time, loc *time.Location) string {
	day := ScheduleKey(slot, loc)
	if e.ID == TriggerEntryLegacyID {
		return day
	}
	return e.ID + "@" + day
}

// RunSource is the trigger entry that admitted a run, frozen so a later edit of
// the belt cannot retarget it. Absent on runs saved before entries existed.
type RunSource struct {
	Entry   string   `json:"entry"`
	Trigger Trigger  `json:"trigger"`
	Repos   []string `json:"repos"`
}

// Trigger is where a belt's work comes from. Each kind carries only its own
// settings; the repositories belong to its trigger entry, and the base branch
// and policies belong to the belt. It saves as one object with a `kind`.
//
// The cases are the payload fields: Kind says which one is set, and the
// others are nil. The `kind` is the Kind field itself. Encoding is
// custom (the Trigger decoder and encoder): the kind's own settings
// are written beside `kind`. A kind missing from the input is issues.
type Trigger struct {
	Kind     TriggerKind    `json:"kind"`
	Issues   *IssueFilter   `json:"-"`
	Reviews  *ReviewFilter  `json:"-"`
	Jira     *JiraQuery     `json:"-"`
	Schedule *ScheduledTask `json:"-"`
}

// TriggerKind is the kind of a trigger.
type TriggerKind string

const (
	// TriggerKindIssues is GitHub issues with a label.
	TriggerKindIssues TriggerKind = "issues"
	// TriggerKindReviews is pull requests that ask for your review.
	TriggerKindReviews TriggerKind = "reviews"
	// TriggerKindJira is Jira tickets that match a JQL query.
	TriggerKindJira TriggerKind = "jira"
	// TriggerKindSchedule is a task on a timetable.
	TriggerKindSchedule TriggerKind = "schedule"
	// TriggerKindManual is a task you start yourself.
	TriggerKindManual TriggerKind = "manual"
)

// AllTriggerKind lists every TriggerKind in declaration order (CaseIterable).
var AllTriggerKind = []TriggerKind{TriggerKindIssues, TriggerKindReviews, TriggerKindJira, TriggerKindSchedule, TriggerKindManual}

// Known reports whether x is one of the cases of TriggerKind.
func (x TriggerKind) Known() bool { return enumKnown(x, AllTriggerKind) }

// Title is the name shown for the kind.
func (k TriggerKind) Title() string {
	switch k {
	case TriggerKindIssues:
		return "GitHub issues"
	case TriggerKindReviews:
		return "Review requests"
	case TriggerKindJira:
		return "Jira tickets"
	case TriggerKindSchedule:
		return "Schedule"
	case TriggerKindManual:
		return "Manual"
	}
	return ""
}

// Subject is the kind of subject a run of this trigger kind works on.
func (k TriggerKind) Subject() SubjectKind {
	switch k {
	case TriggerKindIssues:
		return SubjectKindIssue
	case TriggerKindReviews:
		return SubjectKindPullRequest
	case TriggerKindJira:
		return SubjectKindJira
	case TriggerKindSchedule:
		return SubjectKindScheduled
	case TriggerKindManual:
		return SubjectKindManual
	}
	return ""
}

// Polls reports whether the kind checks somewhere every so often.
func (k TriggerKind) Polls() bool {
	return k == TriggerKindIssues || k == TriggerKindReviews || k == TriggerKindJira
}

// OneRepo reports whether the kind's code work happens in one repository rather
// than several.
func (k TriggerKind) OneRepo() bool { return k == TriggerKindSchedule }

// Fresh is a trigger of this kind with its starting settings.
func (k TriggerKind) Fresh() Trigger {
	switch k {
	case TriggerKindIssues:
		f := DefaultIssueFilter()
		return Trigger{Kind: TriggerKindIssues, Issues: &f}
	case TriggerKindReviews:
		f := DefaultReviewFilter()
		return Trigger{Kind: TriggerKindReviews, Reviews: &f}
	case TriggerKindJira:
		q := DefaultJiraQuery()
		return Trigger{Kind: TriggerKindJira, Jira: &q}
	case TriggerKindSchedule:
		t := DefaultScheduledTask()
		return Trigger{Kind: TriggerKindSchedule, Schedule: &t}
	case TriggerKindManual:
		return Trigger{Kind: TriggerKindManual}
	}
	return Trigger{Kind: k}
}

// TriggerAssignee is who an issue filter's assignee must be.
type TriggerAssignee string

const (
	// TriggerAssigneeAnyone takes issues with any or no assignee.
	TriggerAssigneeAnyone TriggerAssignee = "anyone"
	// TriggerAssigneeMe takes issues assigned to you.
	TriggerAssigneeMe TriggerAssignee = "me"
)

// AllTriggerAssignee lists every TriggerAssignee in declaration order (CaseIterable).
var AllTriggerAssignee = []TriggerAssignee{TriggerAssigneeAnyone, TriggerAssigneeMe}

// Known reports whether x is one of the cases of TriggerAssignee.
func (x TriggerAssignee) Known() bool { return enumKnown(x, AllTriggerAssignee) }

// TriggerReviewers is whose review requests a review filter takes.
type TriggerReviewers string

const (
	// TriggerReviewersMe takes requests to you by name.
	TriggerReviewersMe TriggerReviewers = "me"
	// TriggerReviewersMeAndTeams also takes requests to a team you are on.
	TriggerReviewersMeAndTeams TriggerReviewers = "meAndTeams"
)

// AllTriggerReviewers lists every TriggerReviewers in declaration order (CaseIterable).
var AllTriggerReviewers = []TriggerReviewers{TriggerReviewersMe, TriggerReviewersMeAndTeams}

// Known reports whether x is one of the cases of TriggerReviewers.
func (x TriggerReviewers) Known() bool { return enumKnown(x, AllTriggerReviewers) }

// Label is the name shown for the reviewers setting.
func (r TriggerReviewers) Label() string {
	switch r {
	case TriggerReviewersMe:
		return "Me"
	case TriggerReviewersMeAndTeams:
		return "Me or my teams"
	}
	return ""
}

// TriggerPollChoices are the check intervals the trigger editor offers, in
// seconds.
var TriggerPollChoices = []int{30, 60, 300, 900}

// Kind-agnostic accessors below work for every kind.

// Poll is the seconds between checks, for the kinds that check somewhere, or nil.
func (t Trigger) Poll() *int {
	var poll int
	switch t.Kind {
	case TriggerKindIssues:
		poll = t.issueSettings().Poll
	case TriggerKindReviews:
		poll = t.reviewSettings().Poll
	case TriggerKindJira:
		poll = t.jiraSettings().Poll
	default:
		return nil
	}
	return &poll
}

// SetPoll sets the seconds between checks for the kinds that check somewhere.
// A nil value changes nothing.
func (t *Trigger) SetPoll(poll *int) {
	if poll == nil {
		return
	}
	switch t.Kind {
	case TriggerKindIssues:
		f := t.issueSettings()
		f.Poll = *poll
		t.Issues = &f
	case TriggerKindReviews:
		f := t.reviewSettings()
		f.Poll = *poll
		t.Reviews = &f
	case TriggerKindJira:
		q := t.jiraSettings()
		q.Poll = *poll
		t.Jira = &q
	}
}

// Summary is one line about what the trigger watches, such as "label: agent".
func (t Trigger) Summary() string {
	switch t.Kind {
	case TriggerKindIssues:
		return "label: " + t.issueSettings().Label
	case TriggerKindReviews:
		if t.reviewSettings().Reviewers == TriggerReviewersMe {
			return "your reviews"
		}
		return "your and your teams' reviews"
	case TriggerKindJira:
		if q := t.jiraSettings(); q.Jql != "" {
			return q.Jql
		}
		return "no query"
	case TriggerKindSchedule:
		return t.scheduleSettings().Schedule.Summary()
	case TriggerKindManual:
		return "started by you"
	}
	return ""
}

// UnmarshalJSON reads a trigger: the kind (issues when missing), then that
// kind's own settings from the same object.
func (t *Trigger) UnmarshalJSON(data []byte) error {
	var head struct {
		Kind *TriggerKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	kind := TriggerKindIssues
	if head.Kind != nil {
		kind = *head.Kind
	}
	*t = Trigger{Kind: kind}
	var err error
	switch kind {
	case TriggerKindIssues:
		t.Issues = new(IssueFilter)
		err = json.Unmarshal(data, t.Issues)
	case TriggerKindReviews:
		t.Reviews = new(ReviewFilter)
		err = json.Unmarshal(data, t.Reviews)
	case TriggerKindJira:
		t.Jira = new(JiraQuery)
		err = json.Unmarshal(data, t.Jira)
	case TriggerKindSchedule:
		t.Schedule = new(ScheduledTask)
		err = json.Unmarshal(data, t.Schedule)
	case TriggerKindManual:
	default:
		return fmt.Errorf("unknown trigger kind %q", kind)
	}
	return err
}

// MarshalJSON writes a trigger: its kind, then the settings of that kind beside it.
func (t Trigger) MarshalJSON() ([]byte, error) {
	return jsonx.Marshal(t.wire())
}

// WireShape is the value a trigger's keys are written by: its kind beside the settings of
// that kind. A request body's trigger is checked against it (routes).
func (t Trigger) WireShape() any {
	return t.wire()
}

// wire is the trigger's encoded form: its kind, then the settings of that kind.
func (t Trigger) wire() any {
	switch t.Kind {
	case TriggerKindIssues:
		return triggerIssuesWire{Kind: t.Kind, IssueFilter: t.issueSettings()}
	case TriggerKindReviews:
		return triggerReviewsWire{Kind: t.Kind, ReviewFilter: t.reviewSettings()}
	case TriggerKindJira:
		return triggerJiraWire{Kind: t.Kind, JiraQuery: t.jiraSettings()}
	case TriggerKindSchedule:
		return triggerScheduleWire{Kind: t.Kind, ScheduledTask: t.scheduleSettings()}
	}
	return triggerKindWire{Kind: t.Kind}
}

// The wire forms of a Trigger: its kind, then the settings of that kind beside
// it. Each embeds only the settings of its own kind, so no two fields collide.
type triggerKindWire struct {
	Kind TriggerKind `json:"kind"`
}

type triggerIssuesWire struct {
	Kind TriggerKind `json:"kind"`
	IssueFilter
}

type triggerReviewsWire struct {
	Kind TriggerKind `json:"kind"`
	ReviewFilter
}

type triggerJiraWire struct {
	Kind TriggerKind `json:"kind"`
	JiraQuery
}

type triggerScheduleWire struct {
	Kind TriggerKind `json:"kind"`
	ScheduledTask
}

// issueSettings is the issue filter, or the kind's fresh one when the trigger holds none.
func (t Trigger) issueSettings() IssueFilter {
	if t.Issues != nil {
		return *t.Issues
	}
	return DefaultIssueFilter()
}

// reviewSettings is the review filter, or the kind's fresh one when the trigger holds none.
func (t Trigger) reviewSettings() ReviewFilter {
	if t.Reviews != nil {
		return *t.Reviews
	}
	return DefaultReviewFilter()
}

// jiraSettings is the Jira query, or the kind's fresh one when the trigger holds none.
func (t Trigger) jiraSettings() JiraQuery {
	if t.Jira != nil {
		return *t.Jira
	}
	return DefaultJiraQuery()
}

// scheduleSettings is the scheduled task, or the kind's fresh one when the trigger holds none.
func (t Trigger) scheduleSettings() ScheduledTask {
	if t.Schedule != nil {
		return *t.Schedule
	}
	return DefaultScheduledTask()
}

// Clone is a deep copy of the trigger, including the settings its kind payloads point to.
func (t Trigger) Clone() Trigger {
	return cloneValue(t)
}

// Clone is a deep copy of the entry, its repositories and its trigger.
func (e TriggerEntry) Clone() TriggerEntry {
	return cloneValue(e)
}

// Clone is a deep copy of the run source, with its trigger and repositories.
func (s RunSource) Clone() RunSource {
	return cloneValue(s)
}
