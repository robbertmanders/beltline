package core

import (
	"fmt"
	"slices"
)

// validationStageWarnings are the save-time checks on one stage of a belt, at its
// place index: its name, outcomes, watch, gate, agent settings and slots. The
// checks keep the order the belt's warnings list them in.
func validationStageWarnings(stage Stage, index int, layout BeltLayout, agents map[AgentRef]Agent) []string {
	var warnings []string
	if validationBlank(stage.Name, false) {
		warnings = append(warnings, fmt.Sprintf("Stage %d has no name.", index+1))
	}
	warnings = append(warnings, validationOutcomeWarnings(stage, index, layout)...)
	if stage.Kind() == StageKindWatch {
		warnings = append(warnings, validationWatchWarnings(stage)...)
	}
	if stage.Kind() == StageKindGate {
		warnings = append(warnings, validationGateLabelWarnings(stage)...)
	}
	warnings = append(warnings, validationFlowWarnings(stage)...)
	if stage.Kind() == StageKindAgent {
		warnings = append(warnings, validationAgentStageWarnings(stage, agents)...)
	}
	if stage.Kind() == StageKindGate && index == 0 {
		warnings = append(warnings, stage.Name+" is the first stage, so you would approve issues before any agent has looked at them.")
	}
	warnings = append(warnings, validationSlotWarnings(stage)...)
	return warnings
}

// validationOutcomeWarnings are a stage's outcome checks: it has some, each has a
// unique name, and each goto or route points somewhere real.
func validationOutcomeWarnings(stage Stage, index int, layout BeltLayout) []string {
	var warnings []string
	if len(stage.Outcomes) == 0 {
		warnings = append(warnings, stage.Name+" has no outcomes, so a run can't leave it.")
	}
	names := make([]string, len(stage.Outcomes))
	for i, o := range stage.Outcomes {
		names[i] = outcomeNormalize(o.Name)
	}
	if slices.Contains(names, "") {
		warnings = append(warnings, stage.Name+" has an outcome without a name.")
	}
	if validationHasDuplicate(names) {
		warnings = append(warnings, stage.Name+" has two outcomes with the same name.")
	}
	for _, outcome := range stage.Outcomes {
		switch outcome.Then.Action {
		case ThenActionGoto:
			warnings = append(warnings, validationGoto(outcome, stage, index, layout)...)
		case ThenActionRoute:
			warnings = append(warnings, validationRoute(outcome, stage, layout)...)
		}
	}
	return warnings
}

// validationWatchWarnings are a watch stage's checks: each outcome is tied to an
// event its target reports, and at most one is for no event at all.
func validationWatchWarnings(stage Stage) []string {
	var warnings []string
	target := WatchTargetPullRequest
	if t := stage.WatchTarget(); t != nil {
		target = *t
	}
	allowed := map[string]struct{}{}
	for _, e := range target.Events() {
		allowed[e.When] = struct{}{}
	}
	for _, outcome := range stage.Outcomes {
		if _, ok := allowed[outcome.When]; !ok {
			warnings = append(warnings, fmt.Sprintf("%s: “%s” isn't tied to an event.", stage.Name, outcome.Name))
		}
	}
	noneCount := 0
	for _, outcome := range stage.Outcomes {
		if outcome.When == string(WatchEventNone) {
			noneCount++
		}
	}
	if noneCount > 1 {
		warnings = append(warnings, stage.Name+" can have only one outcome for when none of the others applies.")
	}
	return warnings
}

// validationGateLabelWarnings is the check that every button on a gate has a label.
func validationGateLabelWarnings(stage Stage) []string {
	for _, outcome := range stage.Outcomes {
		if validationBlank(outcome.When, false) {
			return []string{stage.Name + " has an outcome without a button label."}
		}
	}
	return nil
}

// validationFlowWarnings are the checks that a stage with outcomes can move a run
// on, and that an agent stage has a good outcome for the run page.
func validationFlowWarnings(stage Stage) []string {
	var warnings []string
	if len(stage.Outcomes) > 0 && !validationMovesOn(stage.Outcomes) {
		warnings = append(warnings, stage.Name+" has no outcome that lets a run move on.")
	}
	if stage.Kind() == StageKindAgent && len(stage.Outcomes) > 0 && !validationHasGood(stage.Outcomes) {
		warnings = append(warnings, stage.Name+" has no good outcome. The run page uses it to know when the stage went well.")
	}
	return warnings
}

// validationAgentStageWarnings are an agent stage's checks: its runner's thinking
// levels for the stage and its alternates, and that it names an agent.
func validationAgentStageWarnings(stage Stage, agents map[AgentRef]Agent) []string {
	runner := BackendPi
	if stage.Runner != nil {
		runner = *stage.Runner
	} else if stage.Agent != nil {
		if a, ok := agents[*stage.Agent]; ok {
			runner = a.Runner
		}
	}
	var warnings []string
	warnings = append(warnings, validationThinkingWarnings(stage.Name, runner, stage.Thinking, nil)...)
	warnings = append(warnings, validationAlternateWarnings(stage.Name, stage.Alternates)...)
	if stage.Agent == nil {
		warnings = append(warnings, stage.Name+" has no agent.")
	}
	return warnings
}

// validationSlotWarnings are the checks on how many runs a stage allows at once,
// and how many times each outcome may repeat.
func validationSlotWarnings(stage Stage) []string {
	var warnings []string
	if stage.Slots < BeltSlotRange.Lower {
		warnings = append(warnings, stage.Name+" must allow at least one run at once.")
	}
	if stage.Slots > BeltSlotRange.Upper {
		warnings = append(warnings, fmt.Sprintf("%s allows at most %d runs at once.", stage.Name, BeltSlotRange.Upper))
	}
	for _, outcome := range stage.Outcomes {
		if outcome.Then.Max != nil && *outcome.Then.Max > BeltMaxRepeatRange.Upper {
			warnings = append(warnings, fmt.Sprintf("%s: “%s” repeats at most %d times.", stage.Name, outcome.Name, BeltMaxRepeatRange.Upper))
		}
	}
	return warnings
}
