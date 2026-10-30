// The belt, its forks and its
// stage lists. The agents, outcomes and single stages are in engine_defaults.go.

package core

import "slices"
import "strings"

// SampleBelt is the issues-to-pull-requests belt: Triage, Spec, Approve, Build,
// Verify, Ship, Merge, then Wrap up once merged, with Resolve conflicts and Fix
// checks going back to Merge, and the latest of agents where it has them.
// Callers pass the defaults explicitly: repos nil, label "agent",
// agents SampleAgents(), bundled true.
func SampleBelt(repos []string, label string, agents []Agent, bundled bool) Belt {
	filter := DefaultIssueFilter()
	filter.Label = label
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID:      "issues",
		Name:    "Issues to pull requests",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &filter},
		Repos:   repos,
		Stages:  SampleStages(TriggerKindIssues, agents, bundled),
		Forks:   SampleForks(TriggerKindIssues),
	})
}

// SampleAfterMerge is the fork after the watch on a belt that opens its own pull
// requests: a merged pull request is wrapped up, one with conflicts or behind its
// base is brought up to date, and one with failing checks is fixed.
// Each call returns a fresh value.
func SampleAfterMerge() Fork {
	return Fork{ID: "after-merge", Routes: []Route{{ID: "merged", Name: "Merged"}, {ID: "conflicts", Name: "Conflicts"}, {ID: "checks", Name: "Failing checks"}}}
}

// SampleForks are the forks a new belt of kind starts with.
func SampleForks(kind TriggerKind) []Fork {
	switch kind {
	case TriggerKindIssues, TriggerKindJira:
		return []Fork{SampleAfterMerge()}
	default:
		return []Fork{}
	}
}

// SampleStages are the stages a new belt of kind starts with. Issues and
// tickets get the whole line: once their pull request is merged they are tidied
// up, and while it waits, conflicts and failing checks are fixed and watched
// again. A task you wrote yourself skips Triage; a scheduled task goes straight to
// Build; a review reads the pull request and submits what it found. Without
// bundled, a step whose agent isn't in agents is left out.
func SampleStages(kind TriggerKind, agents []Agent, bundled bool) []Stage {
	type step struct {
		ID    string
		Route *string
	}
	var steps []step
	add := func(ids ...string) {
		for _, id := range ids {
			steps = append(steps, step{ID: id})
		}
	}
	switch kind {
	case TriggerKindIssues, TriggerKindJira:
		add("triage", "spec", "approve", "build", "verify", "ship", "watch")
		for _, pair := range [][2]string{{"wrap-up", "merged"}, {"resolve-conflicts", "conflicts"}, {"fix-checks", "checks"}} {
			route := pair[1]
			steps = append(steps, step{ID: pair[0], Route: &route})
		}
	case TriggerKindManual:
		add("spec", "approve", "build", "verify", "ship", "watch")
	case TriggerKindSchedule:
		add("build", "verify", "ship", "watch")
	case TriggerKindReviews:
		add("review", "post-review")
	}
	forks := SampleForks(kind)
	var stages []Stage
	for _, s := range steps {
		place := DefaultsPlace{Before: append([]Stage{}, stages...), Forks: forks, Route: s.Route}
		if typ, ok := defaultsStageType(s.ID); ok && typ.Kind() != StageKindAgent {
			stage := DefaultsStageAt(typ, place, stages)
			switch typ {
			case StageTypeApprove:
				stage.Name = "Approve"
			case StageTypeWatch:
				stage.Name = "Merge"
				if kind == TriggerKindManual || kind == TriggerKindSchedule {
					defaultsShipMerged(&stage)
				}
			default:
				stage.Name = typ.Title()
			}
			stages = append(stages, stage)
			continue
		}
		var agent *Agent
		for index := range agents {
			if agents[index].ID == s.ID {
				agent = &agents[index]
				break
			}
		}
		if agent == nil && bundled {
			agent = SampleAgentNamed(s.ID, nil)
		}
		if agent != nil {
			stages = append(stages, DefaultsStageForAgentAt(*agent, place, stages, kind))
		}
	}
	return SampleTaskBriefs(SampleSendingFixes(stages, forks), agents)
}

// defaultsShipMerged makes a merged pull request ship the run, on a watch that ends
// its belt: a manual or scheduled delivery has no stage after the merge to tidy up.
func defaultsShipMerged(stage *Stage) {
	for index := range stage.Outcomes {
		if stage.Outcomes[index].Name == "merged" {
			stage.Outcomes[index].Then = NewThen(ThenArgs{Action: ThenActionShip})
		}
	}
}

// defaultsStageType is the stage type whose raw value is id, if there is one.
func defaultsStageType(id string) (StageType, bool) {
	for _, typ := range AllStageType {
		if string(typ) == id {
			return typ, true
		}
	}
	return "", false
}

// SampleTaskBriefs adapts the briefs of generated task belts: their bundled
// briefs read the task description instead of referring to an output that cannot
// exist (Triage and Spec may be missing). Only generated stages are adapted; saved
// belts and custom agents stay intact.
// Callers pass SampleAgents() for the default agents.
func SampleTaskBriefs(stages []Stage, agents []Agent) []Stage {
	ids := map[string]struct{}{}
	for _, stage := range stages {
		if stage.Agent != nil {
			ids[stage.Agent.ID] = struct{}{}
		}
	}
	out := make([]Stage, 0, len(stages))
	for _, stage := range stages {
		out = append(out, defaultsTaskBrief(stage, agents, ids))
	}
	return out
}

// defaultsTaskBrief is one stage of SampleTaskBriefs.
func defaultsTaskBrief(stage Stage, agents []Agent, ids map[string]struct{}) Stage {
	if stage.Kind() != StageKindAgent || stage.Agent == nil {
		return stage
	}
	ref := *stage.Agent
	bundled := SampleAgentNamed(ref.ID, nil)
	if bundled == nil || bundled.Ref() != ref {
		return stage
	}
	agent := *bundled
	for _, candidate := range agents {
		if candidate.Ref() == ref {
			agent = candidate
			break
		}
	}
	if agent.Brief != bundled.Brief {
		return stage
	}
	original := StageAdjustEffectiveBrief(stage, &agent)
	brief := original
	for _, missing := range []string{"triage", "spec"} {
		if _, ok := ids[missing]; !ok {
			brief = strings.ReplaceAll(brief, "{"+missing+"}", "{issue}")
		}
	}
	adapted := stage.Clone()
	if brief != original {
		adapted.Brief = brief
	}
	return adapted
}

// SampleSendingFixes makes the watch before SampleAfterMerge send conflicts and
// a branch behind its base to Resolve conflicts, and failing checks to Fix checks,
// each at most twice.
func SampleSendingFixes(stages []Stage, forks []Fork) []Stage {
	afterMerge := SampleAfterMerge()
	hasAfterMerge := slices.ContainsFunc(forks, func(fork Fork) bool { return fork.ID == afterMerge.ID })
	index := -1
	for i, stage := range stages {
		if stage.Kind() == StageKindWatch {
			index = i
			break
		}
	}
	if !hasAfterMerge || index < 0 {
		return stages
	}
	routes := map[string]string{
		string(WatchEventConflicted):   "conflicts",
		string(WatchEventBehind):       "conflicts",
		string(WatchEventChecksFailed): "checks",
	}
	out := make([]Stage, len(stages))
	for i := range stages {
		out[i] = stages[i].Clone()
	}
	for at, outcome := range out[index].Outcomes {
		if route, ok := routes[outcome.When]; ok {
			target := route
			out[index].Outcomes[at].Then = NewThen(ThenArgs{Action: ThenActionRoute, Route: &target, Max: new(2)})
		}
	}
	return out
}
