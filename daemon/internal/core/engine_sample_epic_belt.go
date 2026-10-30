package core

// SampleEpicAgents are the agents an epic belt uses, in its order.
var SampleEpicAgents = []string{"plan-epic", "open-branches", "file-wave", "assess-epic", "replan-epic", "land-epic", "wrap-up"}

// SampleAfterAssess is the fork after Assess: plan the next wave, or land the
// epic.
func SampleAfterAssess() Fork {
	return Fork{ID: "after-assess", Routes: []Route{{ID: "next-wave", Name: "Next wave"}, {ID: "land", Name: "Land"}}}
}

// SampleEpicBelt is the epics-to-tickets belt: Plan, Approve the plan, Open the feature
// branch, File the wave, then Watch the ticket, which wakes Assess when a ticket
// under the epic is added or changes status, or someone comments. Assess waits
// for more, or takes a route: Next wave (Replan, Approve the wave, which files it)
// or Land (a pull request per repository from the feature branch, Merge, then Wrap
// up, which closes the epic). The tickets it files start on another belt, the one
// that builds tickets, by their labels; `base:` puts their work on the feature
// branch. Callers pass the defaults explicitly: the trigger is a Jira query
// for issuetype = Epic AND labels = beltline, repos nil, agents
// SampleAgents().
func SampleEpicBelt(trigger Trigger, repos []string, agents []Agent) Belt {
	forks := []Fork{SampleAfterAssess()}
	var stages []Stage
	place := func(route *string) DefaultsPlace {
		return DefaultsPlace{Before: append([]Stage{}, stages...), Forks: forks, Route: route}
	}
	add := func(id string, route *string) {
		var agent *Agent
		for index := range agents {
			if agents[index].ID == id {
				agent = &agents[index]
				break
			}
		}
		if agent == nil {
			agent = SampleAgentNamed(id, nil)
		}
		if agent == nil {
			return
		}
		stages = append(stages, DefaultsStageForAgentAt(*agent, place(route), stages, trigger.Kind))
	}
	gate := func(id, name string, approved Then, back string, route *string) {
		gateStage := DefaultsStageAt(StageTypeApprove, place(route), stages)
		gateStage.ID = id
		gateStage.Name = name
		gateStage.Outcomes = []Outcome{
			epicOutcome("approved", "Approve", ToneGood, approved),
			epicOutcome("sent back", "Send back with a note", ToneProblem, NewThen(ThenArgs{Action: ThenActionGoto, To: []string{back}, WithOutput: true})),
			epicOutcome("parked", "Park it", ToneNeutral, NewThen(ThenArgs{Action: ThenActionPark})),
		}
		stages = append(stages, gateStage)
	}
	add("plan-epic", nil)
	gate("approve-plan", "Approve the plan", NewThen(ThenArgs{Action: ThenActionNext}), "plan-epic", nil)
	add("open-branches", nil)
	add("file-wave", nil)
	ticket := WatchTargetTicket
	watchName := WatchTargetTicket.Title()
	stages = append(stages, NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Name: &watchName, Outcomes: []Outcome{
		epicOutcome("child tickets changed", string(TicketEventChildrenChanged), ToneNeutral, NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})),
		epicOutcome("commented", string(TicketEventNewComment), ToneNeutral, NewThen(ThenArgs{Action: ThenActionNext, WithOutput: true})),
		epicOutcome("closed", string(TicketEventTicketClosed), ToneProblem, NewThen(ThenArgs{Action: ThenActionDrop})),
	}, Watch: &ticket}))
	add("assess-epic", nil)
	nextWave := "next-wave"
	land := "land"
	add("replan-epic", &nextWave)
	gate("approve-wave", "Approve the wave", NewThen(ThenArgs{Action: ThenActionGoto, To: []string{"file-wave"}}), "replan-epic", &nextWave)
	add("land-epic", &land)
	merge := DefaultsStageAt(StageTypeWatch, place(&land), stages)
	merge.ID = "merge"
	merge.Name = "Merge"
	merge.Outcomes = []Outcome{
		epicOutcome("merged", string(WatchEventMerged), ToneGood, NewThen(ThenArgs{Action: ThenActionNext})),
		epicOutcome("closed", string(WatchEventClosed), ToneProblem, NewThen(ThenArgs{Action: ThenActionHold})),
		epicOutcome("checks failing", string(WatchEventChecksFailed), ToneProblem, NewThen(ThenArgs{Action: ThenActionHold})),
		epicOutcome("has conflicts", string(WatchEventConflicted), ToneProblem, NewThen(ThenArgs{Action: ThenActionHold})),
	}
	stages = append(stages, merge)
	add("wrap-up", &land)
	// A goto to a later stage can't be found while the belt is still being
	// laid out; Assess's to a stage on the fork after it takes that route.
	for index := range stages {
		if stages[index].Kind() != StageKindAgent {
			continue
		}
		agent := epicAgentOf(stages[index], agents)
		if agent == nil {
			continue
		}
		for at, outcome := range agent.Outcomes {
			if outcome.Then.Action != ThenActionGoto || at >= len(stages[index].Outcomes) || len(outcome.To) == 0 {
				continue
			}
			name := outcome.To[0]
			target := epicStageNamed(stages, name)
			if target == nil {
				continue
			}
			then := outcome.Then.Clone()
			if stages[index].ID == "assess-epic" && target.Route != nil {
				then.Action = ThenActionRoute
				route := *target.Route
				then.SetRoute(&route)
				then.To = []string{}
			} else {
				then.To = []string{target.ID}
			}
			stages[index].Outcomes[at].Then = then
		}
	}
	// Planning writes the design into the epic, which mustn't start it over.
	policies := DefaultPolicies()
	policies.RetriageOnEdit = false
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "epics", Name: "Epics to tickets", Trigger: trigger, Repos: repos, Policies: &policies, Stages: stages, Forks: forks,
	})
}

// epicOutcome is an outcome of the epic belt's stages. Its id is a fresh UUID, as
// defaultsOutcome's is (see defaultsOutcomeID).
func epicOutcome(name, when string, tone Tone, then Then) Outcome {
	return Outcome{ID: defaultsOutcomeID(name), Name: name, When: when, Tone: tone, Then: then}
}

// epicAgentOf is the agent a stage was built from: the given agent with the stage's
// pin, else the bundled agent of that id. Nil when neither exists.
func epicAgentOf(stage Stage, agents []Agent) *Agent {
	if stage.Agent == nil {
		return nil
	}
	for index := range agents {
		if agents[index].Ref() == *stage.Agent {
			return &agents[index]
		}
	}
	return SampleAgentNamed(stage.Agent.ID, nil)
}

// epicStageNamed is the first stage whose agent or id is name, if there is one.
func epicStageNamed(stages []Stage, name string) *Stage {
	for index := range stages {
		if (stages[index].Agent != nil && stages[index].Agent.ID == name) || stages[index].ID == name {
			return &stages[index]
		}
	}
	return nil
}
