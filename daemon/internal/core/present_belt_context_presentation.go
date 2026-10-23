package core

import (
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// BeltContextPresentationRouteItem is a route in the editor's route menu.
type BeltContextPresentationRouteItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Fork is the fork the route belongs to.
	Fork string `json:"fork"`
}

// BeltContextPresentationStageItem is a stage in the editor's stage menu.
type BeltContextPresentationStageItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BeltContextPresentationOutcomePreview is one outcome with what it does.
type BeltContextPresentationOutcomePreview struct {
	Stage   string `json:"stage"`
	Outcome string `json:"outcome"`
	Label   string `json:"label"`
}

// SchedulePreviewMissedTimes is what happens to a missed time and to a time that comes while
// the last run is still going.
const SchedulePreviewMissedTimes = "A time missed while the Mac sleeps runs once when it wakes; a time that comes while this schedule's last run is still going is skipped."

// BeltContextPresentationSchedulePreview is a schedule's summary and when it next fires. A
// value built as a literal takes SchedulePreviewMissedTimes as its note, which is the default.
type BeltContextPresentationSchedulePreview struct {
	Entry   string `json:"entry"`
	Summary string `json:"summary"`
	// Next is when the schedule next fires after the context was read, for the page to format.
	Next *jsonx.Date `json:"next,omitzero"`
	// Note is what happens to a missed time and to a time that comes while the last run is still going.
	Note string `json:"note"`
}

// BeltContextPresentationPolicyOption is one policy the belt's triggers can use.
type BeltContextPresentationPolicyOption struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Detail  string `json:"detail"`
	Enabled bool   `json:"enabled"`
}

// BeltContextPresentationPolicySection is the policies the belt's triggers can use, with the section's
// heading and note. Empty when no trigger watches a subject that can change under it
// (schedules and manual tasks).
type BeltContextPresentationPolicySection struct {
	Heading string                                `json:"heading"`
	Options []BeltContextPresentationPolicyOption `json:"options"`
	Note    *string                               `json:"note,omitzero"`
}

// Copy used by BeltContextPresentation. Literal text is exact.
const (
	beltContextJoin           = ", "
	beltContextRetriageDetail = "A stage that changes code starts the run again at the first stage."
	beltContextStopDetail     = "A closed, finished or no longer requested issue stops the run, unless it has reached a watch stage."
	beltContextCommentDetail  = "The issue, pull request or ticket gets a comment when a stage holds the run."
	beltContextBlockerDetail  = "An issue or ticket that an open issue or ticket blocks is left alone until it is unblocked."
)

// BeltContextPresentationRoutes is the editor's route menu for belt.
func BeltContextPresentationRoutes(belt Belt) []BeltContextPresentationRouteItem {
	items := []BeltContextPresentationRouteItem{}
	for _, fork := range belt.Forks {
		for _, route := range fork.Routes {
			items = append(items, BeltContextPresentationRouteItem{ID: route.ID, Name: route.Name, Fork: fork.ID})
		}
	}
	return items
}

// BeltContextPresentationStages is the editor's stage menu for belt.
func BeltContextPresentationStages(belt Belt) []BeltContextPresentationStageItem {
	items := []BeltContextPresentationStageItem{}
	for _, stage := range belt.Stages {
		items = append(items, BeltContextPresentationStageItem{ID: stage.ID, Name: stage.Name})
	}
	return items
}

// BeltContextPresentationOutcomes is every outcome of belt with its label.
func BeltContextPresentationOutcomes(belt Belt) []BeltContextPresentationOutcomePreview {
	items := []BeltContextPresentationOutcomePreview{}
	for _, stage := range belt.Stages {
		for _, outcome := range stage.Outcomes {
			items = append(items, BeltContextPresentationOutcomePreview{
				Stage: stage.ID, Outcome: outcome.ID, Label: BeltContextPresentationLabel(outcome.Then, belt),
			})
		}
	}
	return items
}

// BeltContextPresentationLabel is what an outcome does, naming the stages and routes it reaches.
func BeltContextPresentationLabel(then Then, belt Belt) string {
	switch then.Action {
	case ThenActionGoto:
		if len(then.To) == 0 {
			return ThenActionGoto.Label()
		}
		names := make([]string, 0, len(then.To))
		for _, id := range then.To {
			names = append(names, beltContextStageName(id, belt))
		}
		return "Go to " + strings.Join(names, beltContextJoin)
	case ThenActionRoute:
		if len(then.Routes) == 0 {
			return ThenActionRoute.Label()
		}
		names := make([]string, 0, len(then.Routes))
		for _, id := range then.Routes {
			names = append(names, beltContextRouteName(id, belt))
		}
		return "Take " + strings.Join(names, beltContextJoin)
	}
	return then.Action.Label()
}

// BeltContextPresentationSchedules are the schedule summaries of belt; next is set when now is
// given, in loc (nil for the local calendar).
func BeltContextPresentationSchedules(belt Belt, now *time.Time, loc *time.Location) []BeltContextPresentationSchedulePreview {
	items := []BeltContextPresentationSchedulePreview{}
	for _, entry := range belt.Triggers {
		if entry.Trigger.Kind != TriggerKindSchedule {
			continue
		}
		task := entry.Trigger.scheduleSettings()
		preview := BeltContextPresentationSchedulePreview{Entry: entry.ID, Summary: task.Schedule.Summary(), Note: SchedulePreviewMissedTimes}
		if now != nil {
			if next := task.Schedule.Next(*now, loc); next != nil {
				preview.Next = &jsonx.Date{Time: *next}
			}
		}
		items = append(items, preview)
	}
	return items
}

// beltContextPolicyKinds are the kinds whose subjects the policies are about, in the order the
// belt first names them.
func beltContextPolicyKinds(belt Belt) []TriggerKind {
	seen := []TriggerKind{}
	for _, entry := range belt.Triggers {
		kind := entry.Trigger.Kind
		if kind != TriggerKindIssues && kind != TriggerKindJira && kind != TriggerKindReviews {
			continue
		}
		known := false
		for _, s := range seen {
			known = known || s == kind
		}
		if !known {
			seen = append(seen, kind)
		}
	}
	return seen
}

// BeltContextPresentationPolicySectionOf is the editor's policy section: shown once a trigger
// watches issues, review requests or tickets, worded for those kinds, and offering Wait for
// blockers only when one of them can have blockers. Nil when there are none.
func BeltContextPresentationPolicySectionOf(belt Belt) *BeltContextPresentationPolicySection {
	kinds := beltContextPolicyKinds(belt)
	if len(kinds) == 0 {
		return nil
	}
	kind := kinds[0]
	noun := kind.Subject().Noun()
	blockers := false
	for _, k := range kinds {
		blockers = blockers || k != TriggerKindReviews
	}
	policies := belt.Policies
	retriage := "Edited before Build → triage it again"
	if len(kinds) == 1 && kind == TriggerKindReviews {
		retriage = "Pull request edited → review it again"
	}
	options := []BeltContextPresentationPolicyOption{
		{Key: "retriageOnEdit", Label: retriage, Detail: beltContextRetriageDetail, Enabled: policies.RetriageOnEdit},
		{Key: "stopOnClose", Label: beltContextStopLabel(kinds), Detail: beltContextStopDetail, Enabled: policies.StopOnClose},
		{Key: "commentOnHold", Label: "Comment when a stage holds it", Detail: beltContextCommentDetail, Enabled: policies.CommentOnHold},
	}
	if blockers {
		options = append(options, BeltContextPresentationPolicyOption{
			Key: "waitForBlockers", Label: "Wait while a blocker is still open", Detail: beltContextBlockerDetail, Enabled: policies.WaitForBlockers,
		})
	}
	article := "a"
	if noun == "issue" {
		article = "an"
	}
	subject := "subject"
	if len(kinds) == 1 {
		subject = noun
	}
	section := &BeltContextPresentationPolicySection{
		Heading: "While " + article + " " + subject + " is on the belt",
		Options: options,
	}
	if blockers {
		section.Note = new("A “base:” label on an issue or ticket names the branch its work starts from and merges into, such as an epic's feature branch.")
	}
	return section
}

// beltContextStopLabel is the stop policy's label for the kinds the belt watches.
func beltContextStopLabel(kinds []TriggerKind) string {
	if len(kinds) == 1 {
		switch kinds[0] {
		case TriggerKindReviews:
			return "Pull request closed or no longer asks for your review → stop its work, unless it reached a watch"
		case TriggerKindJira:
			return "Ticket done → stop its work, unless it reached a watch"
		case TriggerKindIssues:
			return "Issue closed or label removed → stop its work, unless it reached a watch"
		}
	}
	return "Closed, done, or no longer matching → stop its work, unless it reached a watch"
}

// BeltContextPresentationPolicies are the four policy options of belt, in order.
func BeltContextPresentationPolicies(belt Belt) []BeltContextPresentationPolicyOption {
	policies := belt.Policies
	return []BeltContextPresentationPolicyOption{
		{Key: "retriageOnEdit", Label: "Start again when the belt is edited", Detail: beltContextRetriageDetail, Enabled: policies.RetriageOnEdit},
		{Key: "stopOnClose", Label: "Stop when the work is closed", Detail: beltContextStopDetail, Enabled: policies.StopOnClose},
		{Key: "commentOnHold", Label: "Comment when a stage holds the run", Detail: beltContextCommentDetail, Enabled: policies.CommentOnHold},
		{Key: "waitForBlockers", Label: "Wait for blocking work", Detail: beltContextBlockerDetail, Enabled: policies.WaitForBlockers},
	}
}

// beltContextStageName is a stage's name, or its id when the belt has no such stage.
func beltContextStageName(id string, belt Belt) string {
	for _, stage := range belt.Stages {
		if stage.ID == id {
			return stage.Name
		}
	}
	return id
}

// beltContextRouteName is a route's name, or its id when the belt has no such route.
func beltContextRouteName(id string, belt Belt) string {
	for _, fork := range belt.Forks {
		for _, route := range fork.Routes {
			if route.ID == id {
				return route.Name
			}
		}
	}
	return id
}
