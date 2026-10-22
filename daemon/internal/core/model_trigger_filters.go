// IssueFilter, ReviewFilter,
// JiraQuery, ScheduledTask and legacyTrigger.

package core

import (
	"encoding/json"
)

// IssueFilter decides which GitHub issues start runs: open ones with a label.
//
// Encoding: synthesized: label, assignee, poll. Decoding starts from the defaults:
// label "", assignee anyone, poll 60); see UnmarshalJSON.
type IssueFilter struct {
	Label    string          `json:"label"`
	Assignee TriggerAssignee `json:"assignee"`
	// Poll is seconds between checks.
	Poll int `json:"poll"`
}

// DefaultIssueFilter is the default issue filter:
// label "agent", anyone, every 60 seconds.
func DefaultIssueFilter() IssueFilter {
	return IssueFilter{Label: "agent", Assignee: TriggerAssigneeAnyone, Poll: 60}
}

// UnmarshalJSON reads an issue filter over its defaults, so absent keys keep them.
func (f *IssueFilter) UnmarshalJSON(data []byte) error {
	type plain IssueFilter
	*f = IssueFilter{Assignee: TriggerAssigneeAnyone, Poll: 60}
	return json.Unmarshal(data, (*plain)(f))
}

// ReviewFilter decides which review requests start runs.
//
// Encoding: synthesized: reviewers, poll. Decoding defaults reviewers to me and
// poll to 60; see UnmarshalJSON.
type ReviewFilter struct {
	Reviewers TriggerReviewers `json:"reviewers"`
	// Poll is seconds between checks.
	Poll int `json:"poll"`
}

// DefaultReviewFilter is the default review filter:
// me, every 60 seconds.
func DefaultReviewFilter() ReviewFilter {
	return ReviewFilter{Reviewers: TriggerReviewersMe, Poll: 60}
}

// UnmarshalJSON reads a review filter over its defaults, so absent keys keep them.
func (f *ReviewFilter) UnmarshalJSON(data []byte) error {
	type plain ReviewFilter
	*f = DefaultReviewFilter()
	return json.Unmarshal(data, (*plain)(f))
}

// JiraQuery decides which Jira tickets start runs: the ones a JQL query finds.
//
// Encoding: synthesized: jql, poll. Decoding defaults jql to "" and poll to 60.
type JiraQuery struct {
	Jql string `json:"jql"`
	// Poll is seconds between checks.
	Poll int `json:"poll"`
}

// DefaultJiraQuery is the default Jira query: an
// empty query, every 60 seconds.
func DefaultJiraQuery() JiraQuery {
	return JiraQuery{Poll: 60}
}

// UnmarshalJSON reads a Jira query over its defaults, so absent keys keep them.
func (q *JiraQuery) UnmarshalJSON(data []byte) error {
	type plain JiraQuery
	*q = DefaultJiraQuery()
	return json.Unmarshal(data, (*plain)(q))
}

// ScheduledTask is a task that starts on a timetable, with its title and
// description as an issue would have them.
//
// Encoding: synthesized: schedule, title, task. Decoding defaults each to
// DefaultSchedule() and "".
type ScheduledTask struct {
	Schedule Schedule `json:"schedule"`
	Title    string   `json:"title"`
	Task     string   `json:"task"`
}

// DefaultScheduledTask is the default scheduled task.
func DefaultScheduledTask() ScheduledTask {
	return ScheduledTask{Schedule: DefaultSchedule()}
}

// UnmarshalJSON reads a scheduled task over its defaults, so absent keys keep them.
func (t *ScheduledTask) UnmarshalJSON(data []byte) error {
	type plain ScheduledTask
	*t = DefaultScheduledTask()
	return json.Unmarshal(data, (*plain)(t))
}

// legacyTrigger holds the fields belts and runs kept on the trigger before the
// belt had them. Decode only. A nil pointer or slice means the key was absent.
// The type is unexported. Its fields stay exported because
// encoding/json decodes only exported fields.
type legacyTrigger struct {
	Repos          []string `json:"repos,omitzero"`
	Base           *string  `json:"base,omitzero"`
	RetriageOnEdit *bool    `json:"retriageOnEdit,omitzero"`
	StopOnClose    *bool    `json:"stopOnClose,omitzero"`
	CommentOnHold  *bool    `json:"commentOnHold,omitzero"`
}

// Policies are the policies the legacy trigger keys give: each missing one is
// true, and waitForBlockers is always true.
func (l legacyTrigger) Policies() Policies {
	p := legacyPolicies()
	if l.RetriageOnEdit != nil {
		p.RetriageOnEdit = *l.RetriageOnEdit
	}
	if l.StopOnClose != nil {
		p.StopOnClose = *l.StopOnClose
	}
	if l.CommentOnHold != nil {
		p.CommentOnHold = *l.CommentOnHold
	}
	return p
}
