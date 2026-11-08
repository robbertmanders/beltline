package core

import "strconv"

// AgentChoice is an agent a new stage may copy, as the belt editor lists it.
type AgentChoice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	// Detail tells this agent from another of the same name: its repository, or its id.
	Detail string `json:"detail,omitzero"`
}

// AgentChoices are the agents a new stage may copy: the live records in library order, so
// archived agents are not offered. The belt editor's menu and its default come from this list.
func AgentChoices(records []AgentRecord) []AgentChoice {
	choices := make([]AgentChoice, 0, len(records))
	details := agentChoiceDetails(records)
	for _, record := range records {
		if !record.Archived {
			choices = append(choices, AgentChoice{ID: record.Agent.ID, Name: record.Agent.Name, Version: record.Agent.Version, Detail: details[record.Agent.ID]})
		}
	}
	return choices
}

// agentChoiceDetails tells apart the live agents that share a name, by id: each one's repository,
// or its id when it is your own or two come from one repository.
func agentChoiceDetails(records []AgentRecord) map[string]string {
	named := map[string][]Agent{}
	for _, record := range records {
		if !record.Archived {
			named[record.Agent.Name] = append(named[record.Agent.Name], record.Agent)
		}
	}
	details := map[string]string{}
	for _, agents := range named {
		if len(agents) < 2 {
			continue
		}
		for _, agent := range agents {
			details[agent.ID] = agent.ID
			if agent.Origin != nil {
				details[agent.ID] = agent.Origin.Repository
			}
		}
	}
	return details
}

// AgentListDetail is the row's secondary label: the version and belt usage. An agent's
// legacy key is not a placeholder offered by the library.
func AgentListDetail(agent Agent, uses int) string {
	usage := "unused"
	if uses != 0 {
		noun := "belts"
		if uses == 1 {
			noun = "belt"
		}
		usage = strconv.Itoa(uses) + " " + noun
	}
	return "v" + strconv.Itoa(agent.Version) + " · " + usage
}
