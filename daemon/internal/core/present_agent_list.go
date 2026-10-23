package core

import "strconv"

// AgentChoice is an agent a new stage may copy, as the belt editor lists it.
type AgentChoice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
}

// AgentChoices are the agents a new stage may copy: the live records in library order, so
// archived agents are not offered. The belt editor's menu and its default come from this list.
func AgentChoices(records []AgentRecord) []AgentChoice {
	choices := make([]AgentChoice, 0, len(records))
	for _, record := range records {
		if !record.Archived {
			choices = append(choices, AgentChoice{ID: record.Agent.ID, Name: record.Agent.Name, Version: record.Agent.Version})
		}
	}
	return choices
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
