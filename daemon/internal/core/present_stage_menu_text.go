// The stage menu's labels, intro, preview and comparison helpers: the goto labels with their route
// path, the watch note, the intro text, the preview and the set comparisons the menu uses.
// The menu types and StageMenuMake are in present_stage_menu.go.

package core

import "strings"

// stageMenuLabel is a stage's name with its route path, for a goto.
func stageMenuLabel(stage Stage, layout BeltLayout) string {
	path := layout.PathOf(stage.ID)
	if len(path) == 0 {
		return stage.Name
	}
	return stage.Name + " · " + strings.Join(path, stageMenuPathSeparator)
}

// stageMenuRoutePath is a route's name with the names of the routes it is nested in,
// outermost first.
func stageMenuRoutePath(id string, layout BeltLayout) string {
	fork := layout.ForkWithRoute(id)
	if fork == nil {
		return id
	}
	var route *Route
	for i := range fork.Routes {
		if fork.Routes[i].ID == id {
			route = &fork.Routes[i]
			break
		}
	}
	if route == nil {
		return id
	}
	names := []string{route.Name}
	parent := fork.Parent
	seen := map[string]struct{}{fork.ID: {}}
	for parent != nil {
		outer := layout.ForkWithRoute(*parent)
		if outer == nil {
			break
		}
		if _, dup := seen[outer.ID]; dup {
			break
		}
		seen[outer.ID] = struct{}{}
		name := ""
		found := false
		for _, outerRoute := range outer.Routes {
			if outerRoute.ID == *parent {
				name = outerRoute.Name
				found = true
				break
			}
		}
		if !found {
			break
		}
		names = append([]string{name}, names...)
		parent = outer.Parent
	}
	return strings.Join(names, stageMenuPathSeparator)
}

// stageMenuWatchNote is the explanation of a watch target.
func stageMenuWatchNote(target WatchTarget) string {
	if target == WatchTargetTicket {
		return "Watches the issue or ticket the run is about: the tickets under it (sub-issues, or an epic's tickets) being added or changing status, comments on it, and it closing."
	}
	return "Watches the run's pull request for a merge, closure, failing checks, merge conflicts, a branch behind its base, approvals, comment reviews or changes requested (only submitted reviews, not plain pull request comments), or for new changes: the one under review, or the one an agent opened from the run's branch. Put it after the agent that opens it. A None outcome is optional. On a check where none of the other outcomes applies, including the first quiet check after the run arrives, Beltline takes it and does not wait for a merge."
}

// stageMenuIntro is the lead of the outcomes section for stage.
func stageMenuIntro(stage Stage) string {
	var lead string
	switch stage.Kind() {
	case StageKindAgent:
		lead = "How this stage can end. The agent picks one and writes it as the last line of its output, so describe when each one applies."
	case StageKindGate:
		lead = "The choices you get when a run reaches this stage. Each outcome is a button."
		if stage.Type == StageTypePrompt {
			lead = "The choices you get after writing the run's task. Each outcome is a button, and one that moves the run on needs the prompt."
		}
	case StageKindWatch:
		if target := stage.WatchTarget(); target != nil && *target == WatchTargetTicket {
			lead = "Beltline picks the outcome from what happened to the issue or ticket and the tickets under it. Changes that can wait are gathered for five minutes and reported together."
		} else {
			lead = "Beltline picks the outcome from what happened to the pull request."
		}
	}
	return lead + " The tone decides how the run looks: Needs you and held runs are highlighted in Work."
}

// stageMenuPreview is the preview under the outcomes for stage.
func stageMenuPreview(stage Stage) StageMenuPreview {
	switch stage.Kind() {
	case StageKindAgent:
		return StageMenuPreview{Title: "Added to the end of the brief", Lines: strings.Split(BriefFooter(stage, stage.Writes()), "\n")}
	case StageKindGate:
		lines := []string{}
		for _, outcome := range stage.Outcomes {
			if outcome.When == "" {
				lines = append(lines, outcome.Name)
			} else {
				lines = append(lines, outcome.When)
			}
		}
		return StageMenuPreview{Title: "What you see on the run", Lines: lines}
	default:
		target := stage.WatchTarget()
		isTicket := target != nil && *target == WatchTargetTicket
		lines := []string{}
		for _, outcome := range stage.Outcomes {
			when := outcome.When
			if isTicket {
				when = WatchTargetTicket.LabelOf(outcome.When)
			} else if event := WatchEvent(outcome.When); enumKnown(event, AllWatchEvent) {
				when = event.WhenClause()
			}
			lines = append(lines, "When "+when+" → "+outcome.Name)
		}
		return StageMenuPreview{Title: "How Beltline decides", Lines: lines}
	}
}
