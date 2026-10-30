// The sample library's agents. The app ships no agents or belts: a new data
// folder starts empty, and the examples are a library repository you install
// from (LibraryExamplesRepository). The sample library is the agents and belts
// earlier versions shipped, kept for three readers: demo mode seeds it, the
// tests build on it, and the data folder upgrade writes the versions an older
// folder pins but never saved (UpgradeSampleAgent). The belts are in
// engine_sample_belts.go and engine_sample_epic_belt.go.

package core

// SampleAgentIDs are the sample library's agents, in the order the demo lists them.
var SampleAgentIDs = []string{
	"triage", "spec", "build", "verify", "ship", "wrap-up", "resolve-conflicts", "fix-checks", "review", "post-review",
	"plan-epic", "open-branches", "file-wave", "assess-epic", "replan-epic", "land-epic", "custom",
}

// SampleAgentFile is the file of a sample agent, or nil.
func SampleAgentFile(id string) *string {
	file, ok := SampleAgentText(id)
	if !ok {
		return nil
	}
	return &file
}

// SampleAgentVersion is the version of each sample agent, the one earlier
// versions of the app shipped last: triage, spec, ship, wrap-up and post-review
// are version 2, the others 1.
func SampleAgentVersion(id string) int {
	switch id {
	case "triage", "spec", "ship", "wrap-up", "post-review":
		return 2
	default:
		return 1
	}
}

// SampleAgentNamed is a sample agent, as its SampleAgentVersion unless version
// is set. Nil when there is no such sample or it does not parse.
func SampleAgentNamed(id string, version *int) *Agent {
	file := SampleAgentFile(id)
	if file == nil {
		return nil
	}
	number := ValueOr(version, SampleAgentVersion(id))
	agent, err := AgentFileParse(*file, id, number)
	if err != nil {
		return nil
	}
	return &agent
}

// SampleAgents are the sample agents that parse, in SampleAgentIDs order. The
// embedded files are read each time it is called.
func SampleAgents() []Agent {
	var agents []Agent
	for _, id := range SampleAgentIDs {
		if agent := SampleAgentNamed(id, nil); agent != nil {
			agents = append(agents, *agent)
		}
	}
	return agents
}

// SampleAgent is a sample agent, for tests and the demo.
// It panics for an id that is not a sample.
func SampleAgent(id string) Agent {
	for _, agent := range SampleAgents() {
		if agent.ID == id {
			return agent
		}
	}
	panic("SampleAgent: no sample agent " + id)
}
