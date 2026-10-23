package core

import (
	"fmt"
	"strconv"
)

// TriggerFormPresentationKind is one kind of trigger as the form shows it: its heading, whether it
// polls and how many repositories it takes, and its note.
type TriggerFormPresentationKind struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Heading is what the belt does with this kind, as the entry's heading.
	Heading           string  `json:"heading"`
	Polls             bool    `json:"polls"`
	OneRepo           bool    `json:"oneRepo"`
	RepositoriesTitle string  `json:"repositoriesTitle"`
	Note              *string `json:"note,omitzero"`
}

// TriggerFormPresentationChoice is one value and its label in a form menu.
type TriggerFormPresentationChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TriggerFormPresentation is the choices and help the trigger editor shows, as the native form
// shows them, so the web editor renders them and makes up none: each kind with its
// heading, whether it polls and how many repositories it takes, and its note; the poll
// intervals, assignees, reviewers and the schedule's days, weekdays and times.
type TriggerFormPresentation struct {
	Kinds []TriggerFormPresentationKind `json:"kinds"`
	// PollChoices are the poll intervals in seconds.
	PollChoices  []TriggerFormPresentationChoice `json:"pollChoices"`
	Assignees    []TriggerFormPresentationChoice `json:"assignees"`
	Reviewers    []TriggerFormPresentationChoice `json:"reviewers"`
	ScheduleDays []TriggerFormPresentationChoice `json:"scheduleDays"`
	// Weekdays are numbered from 1 for Sunday.
	Weekdays []TriggerFormPresentationChoice `json:"weekdays"`
	Hours    []TriggerFormPresentationChoice `json:"hours"`
	Minutes  []TriggerFormPresentationChoice `json:"minutes"`
	// Heading is the form's heading over its entries.
	Heading string `json:"heading"`
	// Empty is shown when the belt has no trigger.
	Empty string `json:"empty"`
	// ShowsBase is whether a trigger supplies a base branch, so the shared card asks for one.
	ShowsBase bool `json:"showsBase"`
	// RepositoryBases are the per-repository base branch fields the shared card
	// shows. Absent unless ShowsBase and some trigger lists a repository.
	RepositoryBases *TriggerFormRepositoryBases `json:"repositoryBases,omitzero"`
}

// TriggerFormRepositoryBases is the group of repository base branch fields: one
// field per distinct repository the belt lists, in first-appearance order.
type TriggerFormRepositoryBases struct {
	Heading     string                           `json:"heading"`
	Help        string                           `json:"help"`
	Placeholder string                           `json:"placeholder"`
	Fields      []TriggerFormRepositoryBaseField `json:"fields"`
}

// TriggerFormRepositoryBaseField is one repository's base branch field. Its value
// is the belt's override in the draft; Label is the repository it is for.
type TriggerFormRepositoryBaseField struct {
	Repository string `json:"repository"`
	Label      string `json:"label"`
}

// TriggerFormPresentationMake is the form for belt.
func TriggerFormPresentationMake(belt Belt) TriggerFormPresentation {
	several := len(belt.Triggers) > 1
	kinds := []TriggerFormPresentationKind{}
	for _, kind := range AllTriggerKind {
		repositoriesTitle := "Repositories"
		switch {
		case kind == TriggerKindJira:
			repositoriesTitle = "Repositories agents may work in"
		case kind.OneRepo():
			repositoriesTitle = "Repository"
		}
		kinds = append(kinds, TriggerFormPresentationKind{
			ID: string(kind), Title: kind.Title(), Heading: TriggerFormPresentationHeading(kind),
			Polls: kind.Polls(), OneRepo: kind.OneRepo(), RepositoriesTitle: repositoriesTitle,
			Note: TriggerFormPresentationNote(kind),
		})
	}
	pollLabels := []string{"30 seconds", "1 minute", "5 minutes", "15 minutes"}
	pollChoices := []TriggerFormPresentationChoice{}
	for i, seconds := range TriggerPollChoices {
		pollChoices = append(pollChoices, TriggerFormPresentationChoice{Value: strconv.Itoa(seconds), Label: pollLabels[i]})
	}
	assignees := []TriggerFormPresentationChoice{}
	for _, assignee := range AllTriggerAssignee {
		label := "Anyone"
		if assignee == TriggerAssigneeMe {
			label = "Me"
		}
		assignees = append(assignees, TriggerFormPresentationChoice{Value: string(assignee), Label: label})
	}
	reviewers := []TriggerFormPresentationChoice{}
	for _, reviewer := range AllTriggerReviewers {
		reviewers = append(reviewers, TriggerFormPresentationChoice{Value: string(reviewer), Label: reviewer.Label()})
	}
	days := []TriggerFormPresentationChoice{}
	for _, day := range AllScheduleDays {
		days = append(days, TriggerFormPresentationChoice{Value: string(day), Label: day.Label()})
	}
	weekdays := []TriggerFormPresentationChoice{}
	for day := 1; day <= 7; day++ {
		weekdays = append(weekdays, TriggerFormPresentationChoice{Value: strconv.Itoa(day), Label: ScheduleWeekdayNames[day-1]})
	}
	hours := []TriggerFormPresentationChoice{}
	for hour := 0; hour <= 23; hour++ {
		hours = append(hours, TriggerFormPresentationChoice{Value: strconv.Itoa(hour), Label: fmt.Sprintf("%02d", hour)})
	}
	minutes := []TriggerFormPresentationChoice{}
	for minute := 0; minute < 60; minute += 5 {
		minutes = append(minutes, TriggerFormPresentationChoice{Value: strconv.Itoa(minute), Label: fmt.Sprintf(":%02d", minute)})
	}
	heading := "Trigger"
	if several {
		heading = "Every one feeds this belt"
	} else if len(belt.Triggers) > 0 {
		heading = TriggerFormPresentationHeading(belt.Triggers[0].Trigger.Kind)
	}
	showsBase := false
	for _, entry := range belt.Triggers {
		_, provides := entry.Trigger.Kind.Provides()[InputBaseBranch]
		showsBase = showsBase || provides
	}
	form := TriggerFormPresentation{
		Kinds: kinds, PollChoices: pollChoices, Assignees: assignees, Reviewers: reviewers,
		ScheduleDays: days, Weekdays: weekdays, Hours: hours, Minutes: minutes,
		Heading: heading, Empty: "Add at least one trigger", ShowsBase: showsBase,
	}
	if showsBase {
		form.RepositoryBases = triggerFormRepositoryBasesOf(belt)
	}
	return form
}

// triggerFormRepositoryBasesOf is the repository base branch group for belt, or
// nil when no trigger lists a repository.
func triggerFormRepositoryBasesOf(belt Belt) *TriggerFormRepositoryBases {
	fields := []TriggerFormRepositoryBaseField{}
	for _, repo := range belt.ListedRepositories() {
		fields = append(fields, TriggerFormRepositoryBaseField{Repository: repo, Label: repo})
	}
	if len(fields) == 0 {
		return nil
	}
	return &TriggerFormRepositoryBases{
		Heading:     "Repository base branches",
		Help:        "An empty field starts that repository from the shared base branch, or from its default branch when the shared base is empty.",
		Placeholder: "Use shared base branch",
		Fields:      fields,
	}
}

// TriggerFormPresentationHeading is the heading of one kind of trigger.
func TriggerFormPresentationHeading(kind TriggerKind) string {
	switch kind {
	case TriggerKindIssues:
		return "When a GitHub issue matches"
	case TriggerKindReviews:
		return "When a pull request asks for your review"
	case TriggerKindJira:
		return "When a Jira ticket matches"
	case TriggerKindSchedule:
		return "On a schedule"
	case TriggerKindManual:
		return "When you start a task"
	}
	return ""
}

// TriggerFormPresentationNote is the note under one kind of trigger, or nil.
func TriggerFormPresentationNote(kind TriggerKind) *string {
	switch kind {
	case TriggerKindReviews:
		return new("Each request is one run on a read-only checkout of the pull request. When someone asks for your review again, a new run starts.")
	case TriggerKindJira:
		return new("Beltline reads Jira through the Atlassian CLI and its sign-in: acli jira auth login. A ticket that leaves the query keeps its run, so a belt can move tickets along. List the project's repositories; the agents decide which ones the work needs, and a repository without the base branch works from its default. The first repository anchors the ticket.")
	case TriggerKindManual:
		return new("Start a task from the belt's header: pick a repository, give it a title and say what to do.")
	}
	return nil
}
