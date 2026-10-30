// The outcomes a new stage starts with, by its type and place. The single
// stages, their updates and their comparisons are in engine_defaults_stage.go.
//
// Go has no default arguments. Where a parameter has a default (a Place's
// later, a kind of .issues, the events of a migration), the Go caller passes
// that default explicitly; the doc comment names it.

package core

import "slices"

// DefaultsModel is the model new agent stages run, through Pi and OpenRouter.
const DefaultsModel = "openrouter/stealth/space-bunny-alpha"

// defaultsTemplate is the outcome a stage type starts with, before its place is
// known. To is the agents a goto jumps to (empty when none), or `watch` for a
// watch stage: the nearest earlier stage with one wins, then the nearest later one.
type defaultsTemplate struct {
	Name string
	When string
	Tone Tone
	Then Then
	To   []string
}

// defaultsTemplates is the template table for a stage type.
func defaultsTemplates(typ StageType) []defaultsTemplate {
	switch typ {
	case StageTypeApprove:
		return []defaultsTemplate{
			{Name: "approved", When: "Approve", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})},
			{Name: "sent back", When: "Send back with a note", Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionGoto, WithOutput: true}), To: []string{"spec", "build"}},
			{Name: "parked", When: "Park it", Tone: ToneNeutral, Then: NewThen(ThenArgs{Action: ThenActionPark})},
		}
	case StageTypeWatch:
		// Held, not moved on: the stages after a watch expect a merged pull request.
		// New changes is held too: a new head is not one.
		return []defaultsTemplate{
			{Name: "merged", When: string(WatchEventMerged), Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})},
			{Name: "closed", When: string(WatchEventClosed), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionHold})},
			{Name: "checks failing", When: string(WatchEventChecksFailed), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionGoto, Max: new(2)})},
			{Name: "has conflicts", When: string(WatchEventConflicted), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionHold})},
			{Name: "is behind", When: string(WatchEventBehind), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionGoto, Max: new(2)})},
			{Name: "is approved", When: string(WatchEventApproved), Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionHold})},
			{Name: "a reviewer commented", When: string(WatchEventCommented), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionHold})},
			{Name: "changes requested", When: string(WatchEventChangesRequested), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionHold})},
			{Name: "new changes", When: string(WatchEventNewChanges), Tone: ToneYou, Then: NewThen(ThenArgs{Action: ThenActionHold})},
		}
	default:
		return nil
	}
}

// DefaultsMigratedWatchEvents are the events a belt saved before a watch could
// see them may gain: every WatchEvent but none and new changes, which stays
// opt-in.
func DefaultsMigratedWatchEvents() map[WatchEvent]struct{} {
	events := map[WatchEvent]struct{}{}
	for _, event := range AllWatchEvent {
		if event != WatchEventNone && event != WatchEventNewChanges {
			events[event] = struct{}{}
		}
	}
	return events
}

// DefaultsUpgradingWatchOutcomes adds missing watch events to belt without changing
// any authored outcomes or pinned runs. None and new changes are never added.
// Callers pass DefaultsMigratedWatchEvents() for the default events.
func DefaultsUpgradingWatchOutcomes(belt Belt, events map[WatchEvent]struct{}) Belt {
	eligible := map[WatchEvent]struct{}{}
	for event := range events {
		if event != WatchEventNone && event != WatchEventNewChanges {
			eligible[event] = struct{}{}
		}
	}
	upgraded := belt.Clone()
	for index, stage := range belt.Stages {
		if stage.Kind() != StageKindWatch {
			continue
		}
		place := DefaultsPlace{Before: append([]Stage{}, belt.Stages[:index]...), Forks: belt.Forks, Route: stage.Route}
		existing := map[string]struct{}{}
		for _, outcome := range stage.Outcomes {
			existing[outcome.When] = struct{}{}
		}
		for _, outcome := range DefaultsOutcomesForAt(StageTypeWatch, place) {
			if _, ok := existing[outcome.When]; ok {
				continue
			}
			if _, ok := eligible[WatchEvent(outcome.When)]; !ok {
				continue
			}
			upgraded.Stages[index].Outcomes = append(upgraded.Stages[index].Outcomes, outcome)
		}
	}
	return upgraded
}

// DefaultsPlace is where a new stage goes: between Before and Later, on Route of
// one of Forks or on none. Its zero value is the default place.
type DefaultsPlace struct {
	Before []Stage
	Later  []Stage
	Forks  []Fork
	Route  *string
}

// DefaultsOutcomesForAt is the default outcomes for a stage of typ at place.
func DefaultsOutcomesForAt(typ StageType, place DefaultsPlace) []Outcome {
	var outcomes []Outcome
	for _, template := range defaultsTemplates(typ) {
		outcomes = append(outcomes, defaultsOutcome(template.Name, template.When, template.Tone, template.Then, template.To, place))
	}
	return outcomes
}

// DefaultsOutcomesForAgentAt is the outcomes a stage with agent starts with at
// place.
func DefaultsOutcomesForAgentAt(agent Agent, place DefaultsPlace) []Outcome {
	var outcomes []Outcome
	for _, outcome := range agent.Outcomes {
		outcomes = append(outcomes, defaultsOutcome(outcome.Name, outcome.When, outcome.Tone, outcome.Then, outcome.To, place))
	}
	return outcomes
}

// defaultsOutcome builds one outcome for a template. A goto with no target
// (empty To) is resolved against place: back to the nearest earlier stage with one
// of the agents (or, for watch, a watch stage), or else ahead to the nearest later
// one, and held when there is none. A stage on a route only goes to stages on its
// own route or off the fork; one that lands on a fork's route from outside it also
// goes to the matching stage on each other route that has one.
//
// The id is a fresh UUID (defaultsOutcomeID).
func defaultsOutcome(name, when string, tone Tone, then Then, to []string, at DefaultsPlace) Outcome {
	then = then.Clone()
	if then.Action == ThenActionGoto && len(then.To) == 0 {
		layout := BeltLayout{Stages: append(append([]Stage{}, at.Before...), at.Later...), Forks: at.Forks}
		var own *Fork
		if at.Route != nil {
			own = layout.ForkWithRoute(*at.Route)
		}
		uses := func(stage Stage) bool {
			var named bool
			if stage.Agent != nil {
				named = slices.Contains(to, stage.Agent.ID)
			} else {
				named = stage.Kind() == StageKindWatch && slices.Contains(to, string(stage.Type))
			}
			eligible := named
			if len(to) == 0 {
				eligible = stage.Kind() == StageKindAgent
			}
			if !eligible {
				return false
			}
			if own == nil || stage.Route == nil || !forkHasRoute(*own, *stage.Route) {
				return true
			}
			return *stage.Route == *at.Route
		}
		var target *Stage
		for index := len(at.Before) - 1; index >= 0 && target == nil; index-- {
			if uses(at.Before[index]) {
				target = &at.Before[index]
			}
		}
		if target == nil && len(to) > 0 {
			for index := range at.Later {
				if uses(at.Later[index]) {
					target = &at.Later[index]
					break
				}
			}
		}
		if target != nil {
			then.To = []string{target.ID}
			if way := layout.Place(target.ID); way != nil && (own == nil || way.Fork.ID != own.ID) {
				for _, route := range way.Fork.Routes {
					if target.Route != nil && route.ID == *target.Route {
						continue
					}
					stages := layout.StagesOn(route.ID)
					for index := len(stages) - 1; index >= 0; index-- {
						if uses(stages[index]) {
							then.To = append(then.To, stages[index].ID)
							break
						}
					}
				}
			}
		} else {
			then = NewThen(ThenArgs{Action: ThenActionHold})
		}
	}
	return Outcome{ID: defaultsOutcomeID(name), Name: name, When: when, Tone: tone, Then: then}
}

// defaultsOutcomeID is the id of a default outcome: a fresh UUID.
// The name is not used for the id.
func defaultsOutcomeID(name string) string {
	return NewUUID()
}

// DefaultsTicketWatchOutcomes are the outcomes a watch on the run's issue or
// ticket starts with: the next stage hears what changed on its child tickets, and
// you hear of a comment or the ticket closing.
func DefaultsTicketWatchOutcomes() []Outcome {
	return []Outcome{
		{ID: defaultsOutcomeID("child tickets changed"), Name: "child tickets changed", When: string(TicketEventChildrenChanged), Tone: ToneNeutral, Then: NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})},
		{ID: defaultsOutcomeID("commented"), Name: "commented", When: string(TicketEventNewComment), Tone: ToneNeutral, Then: NewThen(ThenArgs{Action: ThenActionHold})},
		{ID: defaultsOutcomeID("closed"), Name: "closed", When: string(TicketEventTicketClosed), Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionHold})},
	}
}
