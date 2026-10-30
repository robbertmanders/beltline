// Shared fixtures for the validation tests (engine_validation_*_test.go). They
// build belts with the real builders (SampleBelt, DefaultsStage…) and check
// them with the same warnings check the engine runs.
// Names carry the vt prefix so they do not collide with the other suites here.

package core

import "slices"

// vtAgent is a shipped built-in agent by id.
func vtAgent(id string) Agent {
	agent := SampleAgentNamed(id, nil)
	if agent == nil {
		panic("no built-in agent " + id)
	}
	return *agent
}

// vtPinned maps agents by their pinned refs.
func vtPinned(agents []Agent) map[AgentRef]Agent {
	pinned := make(map[AgentRef]Agent, len(agents))
	for _, agent := range agents {
		pinned[agent.Ref()] = agent
	}
	return pinned
}

// vtWarnings is the warnings of a belt:
// the belt with its stage keys converted, checked against the shipped agents.
func vtWarnings(belt Belt) []string {
	agents := vtPinned(SampleAgents())
	converted := StageKeysExplicit(belt, agents)
	var keys []string
	for _, stage := range converted.Stages {
		if stage.Kind() == StageKindAgent {
			keys = append(keys, StageKeysChosen(stage))
		}
	}
	return ValidationWarnings(converted, keys, agents)
}

// vtThenGoto is Then(.goto, to: targets).
func vtThenGoto(targets ...string) Then {
	return NewThen(ThenArgs{Action: ThenActionGoto, To: targets})
}

// vtThenRoute is Then(.route, route: route).
func vtThenRoute(route string) Then {
	return NewThen(ThenArgs{Action: ThenActionRoute, Route: &route})
}

// vtOutcome is an outcome with a fresh id.
func vtOutcome(name, when string, tone Tone, then Then) Outcome {
	return Outcome{ID: NewUUID(), Name: name, When: when, Tone: tone, Then: then}
}

// vtStageFor is the stage for agent on an issues belt, placed before the given stages.
func vtStageFor(agent Agent, before, existing []Stage) Stage {
	return DefaultsStageForAgent(agent, before, nil, existing, TriggerKindIssues)
}

// vtBeltOf is a manual-trigger belt over repos.
func vtBeltOf(id string, repos []string, stages []Stage, forks []Fork) Belt {
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: id, Name: "Tasks", Trigger: Trigger{Kind: TriggerKindManual}, Repos: repos, Stages: stages, Forks: forks,
	})
}

// vtForkOf is a fork with the given route ids and names, in order.
func vtForkOf(id string, routes ...Route) Fork {
	return Fork{ID: id, Routes: routes}
}

// vtRoute is a route with id and name.
func vtRoute(id, name string) Route {
	return Route{ID: id, Name: name}
}

// vtForkedBelt is the forked belt for simple agents: triage, then
// Spec and Build on the Complex route or the simple agents on the Simple route
// (quick-<agent>), then Verify and Ship; triage's first outcome takes the Simple route.
func vtForkedBelt(simple []string) Belt {
	fork := vtForkOf("size", vtRoute("complex", "Complex"), vtRoute("simple", "Simple"))
	var stages []Stage
	add := func(agent string, route *string, id string) {
		stage := DefaultsStageForAgentAt(vtAgent(agent), DefaultsPlace{Before: stages, Forks: []Fork{fork}, Route: route}, stages, TriggerKindIssues)
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	add("triage", nil, "")
	add("spec", new("complex"), "")
	add("build", new("complex"), "")
	for _, agent := range simple {
		add(agent, new("simple"), "quick-"+agent)
	}
	add("verify", nil, "")
	add("ship", nil, "")
	stages[0].Outcomes[0].Then = vtThenRoute("simple")
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "sized", Name: "Sized", Trigger: Trigger{Kind: TriggerKindIssues, Issues: new(DefaultIssueFilter())},
		Repos: []string{"acme/billing"}, Stages: stages, Forks: []Fork{fork},
	})
}

// vtDefaultBelt is the default belt over repos, with its default label and agents.
func vtDefaultBelt(repos ...string) Belt {
	if len(repos) == 0 {
		repos = nil
	}
	return SampleBelt(repos, "agent", SampleAgents(), true)
}

// vtStageIndex is the index of the first stage of kind, or -1.
func vtStageIndex(stages []Stage, kind StageKind) int {
	return slices.IndexFunc(stages, func(stage Stage) bool { return stage.Kind() == kind })
}
