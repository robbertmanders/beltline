package core

import (
	"fmt"
	"slices"
	"strings"
)

// StageLinePresentation is the belt editor's stage line as the app drew it
// (StageLine): the trigger block, a block per stage, a box per fork with a row per
// route, and where each "+" between them inserts a stage. Names come from the
// browser's draft; the structure, numbers, positions and wording come from here.
type StageLinePresentation struct {
	Trigger StageLinePresentationTrigger `json:"trigger"`
	Items   []StageLinePresentationItem  `json:"items"`
	// AddEnd is where the "+" after the last item inserts a stage.
	AddEnd int `json:"addEnd"`
	// Agents are the agents a "+" offers: active ones whose needs this belt's triggers provide.
	Agents []StageLinePresentationAgentChoice `json:"agents"`
	// WatchAllowed reports whether a watch stage may be added to this belt.
	WatchAllowed bool `json:"watchAllowed"`
}

// StageLinePresentationTrigger is the trigger block at the head of the line.
type StageLinePresentationTrigger struct {
	Kind  string   `json:"kind"`
	Title string   `json:"title"`
	Lines []string `json:"lines"`
	Help  *string  `json:"help,omitzero"`
}

// StageLinePresentationBlock is one stage's block on the line.
type StageLinePresentationBlock struct {
	ID string `json:"id"`
	// Kind is `01 · Agent`.
	Kind     string   `json:"kind"`
	Subtitle []string `json:"subtitle"`
	// Update is the newer version of the stage's agent, when the library has one.
	Update *int `json:"update,omitzero"`
}

// StageLinePresentationItem is one entry on a line or a route: a stage or a fork box.
type StageLinePresentationItem struct {
	// AddAt is where the "+" in front of this item inserts a stage.
	AddAt int                           `json:"addAt"`
	Stage *StageLinePresentationBlock   `json:"stage,omitzero"`
	Fork  *StageLinePresentationForkBox `json:"fork,omitzero"`
}

// StageLinePresentationForkBox is a fork drawn as a box with a row per route.
type StageLinePresentationForkBox struct {
	ID      string `json:"id"`
	Heading string `json:"heading"`
	// Feeding is set for a fork for branches that feed in, with the line as its first route.
	Feeding bool                            `json:"feeding"`
	Routes  []StageLinePresentationRouteRow `json:"routes"`
}

// StageLinePresentationRouteRow is one route of a fork box.
type StageLinePresentationRouteRow struct {
	ID        string                      `json:"id"`
	Caption   string                      `json:"caption"`
	Help      string                      `json:"help"`
	Removable bool                        `json:"removable"`
	Items     []StageLinePresentationItem `json:"items"`
	// AddEnd is where the "+" at the route's end inserts a stage.
	AddEnd int `json:"addEnd"`
	// Empty is what an empty route says.
	Empty *string `json:"empty,omitzero"`
	// Opens is whether the line splits into the route (BeltLayout.Opens): a route that feeds in starts on its own.
	Opens bool `json:"opens"`
	// Joins is whether a run that finishes the route carries on to a stage where the routes meet (BeltLayout.Joins),
	// so the drawing joins it to the line again. A route that only goes back, holds or drops ends there, and so does
	// every route of a fork with no stage after it, where finishing the route finishes the run.
	Joins bool `json:"joins"`
}

// StageLinePresentationAgentChoice is an agent a "+" offers, by id, version and name.
type StageLinePresentationAgentChoice struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
	// Detail tells this agent from another of the same name: its repository, or its id.
	Detail string `json:"detail,omitzero"`
}

// StageLineOf is the stage line of a belt's draft: its blocks, fork boxes and where
// each "+" inserts a stage. agents is the whole library in
// its order, archived agents included: it decides which agents a "+" offers and
// which stage has a newer version of its agent (Library.NewerAgent).
func StageLineOf(belt Belt, agents []AgentRecord) StageLinePresentation {
	build := stageLineBuild{belt: belt, layout: belt.Layout(), agents: agents}
	return StageLinePresentation{
		Trigger: stageLineTrigger(belt), Items: build.items(build.layout.Segments()), AddEnd: len(belt.Stages),
		Agents: stageLineAgentChoices(belt, agents), WatchAllowed: belt.CanHave(StageTypeWatch),
	}
}

// stageLineBuild is what one StageLineOf call reads while it walks the belt's layout.
type stageLineBuild struct {
	belt   Belt
	layout BeltLayout
	agents []AgentRecord
}

// index is the position of the stage with id in the belt, or the stage count when it is not there.
func (b stageLineBuild) index(id string) int {
	for i, stage := range b.belt.Stages {
		if stage.ID == id {
			return i
		}
	}
	return len(b.belt.Stages)
}

// items is a run of layout segments as the line's items: a block per stage and a box per fork.
func (b stageLineBuild) items(segments []BeltLayoutSegment) []StageLinePresentationItem {
	out := make([]StageLinePresentationItem, 0, len(segments))
	for _, segment := range segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			stage := *segment.Stage
			out = append(out, StageLinePresentationItem{AddAt: b.index(stage.ID), Stage: stageLineBlock(b.layout, stage, b.agents)})
		case BeltLayoutSegmentFork:
			box := b.forkBox(*segment.Fork, segment.Routes)
			out = append(out, StageLinePresentationItem{AddAt: b.index(firstStageID(segment.Routes)), Fork: &box})
		}
	}
	return out
}

// forkBox is a fork's box with a row per route, each route's items drawn from routes.
func (b stageLineBuild) forkBox(fork Fork, routes [][]BeltLayoutSegment) StageLinePresentationForkBox {
	feeds := stageLineFeeding(fork)
	// The routes meet at a stage only when one follows the fork, here or past the forks around it.
	meets := b.layout.Resume(fork).Stage != nil
	rows := make([]StageLinePresentationRouteRow, 0, len(fork.Routes))
	for i, route := range fork.Routes {
		segments := []BeltLayoutSegment{}
		if i < len(routes) {
			segments = routes[i]
		}
		line := fork.Routes[0].ID == route.ID
		caption, help := stageLineRouteCaption(route, line, feeds)
		rows = append(rows, StageLinePresentationRouteRow{
			ID: route.ID, Caption: caption, Help: help, Removable: !feeds || !line,
			Items: b.items(segments), AddEnd: b.belt.End(route.ID), Empty: stageLineRouteEmpty(segments, line, feeds),
			Opens: b.layout.Opens(route.ID), Joins: meets && b.layout.Joins(route.ID),
		})
	}
	return StageLinePresentationForkBox{ID: fork.ID, Heading: b.forkHeading(fork, feeds), Feeding: feeds, Routes: rows}
}

// forkHeading is a fork box's heading: its route count, or where the fork feeds in.
func (b stageLineBuild) forkHeading(fork Fork, feeds bool) string {
	if !feeds {
		return fmt.Sprintf("FORK · %d ROUTES", len(fork.Routes))
	}
	heading := "FEEDS IN"
	if into := b.layout.Resume(fork).Stage; into != nil {
		for _, stage := range b.belt.Stages {
			if stage.ID == *into {
				heading = "FEEDS INTO " + UppercasedFull(stage.Name)
				break
			}
		}
	}
	return heading
}

// firstStageID is the first stage on the first route of routes that has one, or "" when none does.
func firstStageID(routes [][]BeltLayoutSegment) string {
	first := ""
	for _, route := range routes {
		for _, inner := range route {
			if stages := inner.Stages(); len(stages) > 0 {
				first = stages[0].ID
				break
			}
		}
		if first != "" {
			break
		}
	}
	return first
}

// stageLineFeeding is whether a fork feeds into the stage after it: one of its routes does.
func stageLineFeeding(fork Fork) bool {
	return slices.ContainsFunc(fork.Routes, func(route Route) bool { return route.Feeds })
}

// stageLineRouteCaption is a route row's caption and help: what the route is in its fork.
func stageLineRouteCaption(route Route, line, feeds bool) (string, string) {
	switch {
	case route.Feeds:
		return "FEEDS IN", "Nothing on the line goes here: a run gets here with Go to a stage or Send to"
	case line && feeds:
		return "LINE", "Runs on the line go straight on"
	case line:
		return "ROUTE · FIRST", "Runs take this route unless an outcome before the fork takes another"
	default:
		return "ROUTE", ""
	}
}

// stageLineRouteEmpty is what an empty route says, or nil when it has stages.
func stageLineRouteEmpty(segments []BeltLayoutSegment, line, feeds bool) *string {
	if len(segments) > 0 {
		return nil
	}
	text := "No stages: runs skip ahead"
	if feeds && line {
		text = "The line: runs go straight on"
	}
	return &text
}

// stageLineTrigger is the trigger block: the one trigger's kind and lines, or all of them together.
func stageLineTrigger(belt Belt) StageLinePresentationTrigger {
	if len(belt.Triggers) > 1 {
		kinds := make([]string, 0, len(belt.Triggers))
		for _, entry := range belt.Triggers {
			kinds = append(kinds, entry.Trigger.Kind.Title())
		}
		help := "Every trigger feeds this belt"
		return StageLinePresentationTrigger{
			Kind: fmt.Sprintf("Triggers · %d", len(belt.Triggers)), Title: "All of them",
			Lines: []string{fmt.Sprintf("%d triggers", len(belt.Triggers)), strings.Join(kinds, ", ")}, Help: &help,
		}
	}
	repos := belt.Repos()
	repoLine := fmt.Sprintf("%d repositories", len(repos))
	if len(repos) == 1 {
		repoLine = repos[0]
	}
	return StageLinePresentationTrigger{Kind: "Trigger", Title: belt.Trigger().Kind.Title(),
		Lines: stageLineTriggerLines(belt.Trigger(), repoLine, len(repos))}
}

// stageLineAgentChoices are the active agents whose needs the belt's triggers provide, in library order.
func stageLineAgentChoices(belt Belt, agents []AgentRecord) []StageLinePresentationAgentChoice {
	inputs := belt.MenuInputs()
	choices := []StageLinePresentationAgentChoice{}
	details := agentChoiceDetails(agents)
	for _, record := range agents {
		if record.Archived || !InputsSubset(record.Agent.Workspace.Needs(), inputs) {
			continue
		}
		choices = append(choices, StageLinePresentationAgentChoice{ID: record.Agent.ID, Version: record.Agent.Version, Name: record.Agent.Name, Detail: details[record.Agent.ID]})
	}
	return choices
}

// stageLineBlock is one stage's block on the line, with the newer version of its agent when the library has one.
func stageLineBlock(layout BeltLayout, stage Stage, agents []AgentRecord) *StageLinePresentationBlock {
	kind := fmt.Sprintf("%02d · %s", layout.Number(stage.ID), stage.Kind().Label())
	var subtitle []string
	switch stage.Kind() {
	case StageKindGate:
		subtitle = []string{"Decided by you"}
	case StageKindWatch:
		if stage.WatchesTicket() {
			subtitle = []string{"Watches the ticket"}
		} else {
			subtitle = []string{"Waits for the merge"}
		}
	default:
		agent := "Agent"
		if stage.Runner != nil {
			agent = stage.Runner.Label()
		}
		subtitle = []string{agent, "→ " + stage.Writes()}
	}
	block := StageLinePresentationBlock{ID: stage.ID, Kind: kind, Subtitle: subtitle}
	block.Update = stageLineNewerVersion(agents, stage)
	return &block
}

// stageLineNewerVersion is the version of a stage's agent in the library when it
// is newer than the one the stage pins, or nil. The library's first agent with the
// stage's id is the latest one, as Library.LatestAgent reads it.
func stageLineNewerVersion(agents []AgentRecord, stage Stage) *int {
	if stage.Agent == nil {
		return nil
	}
	for _, record := range agents {
		if record.Agent.ID != stage.Agent.ID {
			continue
		}
		if record.Agent.Version <= stage.Agent.Version {
			return nil
		}
		version := record.Agent.Version
		return &version
	}
	return nil
}

// stageLineTriggerLines are the lines of a single trigger's block.
func stageLineTriggerLines(trigger Trigger, repos string, count int) []string {
	switch trigger.Kind {
	case TriggerKindIssues:
		label := ""
		if trigger.Issues != nil {
			label = trigger.Issues.Label
		}
		return []string{repos, "label: " + label}
	case TriggerKindReviews:
		reviewers := ""
		if trigger.Reviews != nil {
			reviewers = LowercasedFull(trigger.Reviews.Reviewers.Label())
		}
		return []string{repos, "asked of " + reviewers}
	case TriggerKindJira:
		jql := ""
		if trigger.Jira != nil {
			jql = trigger.Jira.Jql
		}
		return []string{repos, jql}
	case TriggerKindSchedule:
		summary := ""
		if trigger.Schedule != nil {
			summary = trigger.Schedule.Schedule.Summary()
		}
		return []string{summary, repos}
	default:
		return []string{repos, "started by you"}
	}
}
