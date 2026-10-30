// The writer
// and checker agents are the fixtures of the custom agent tests.

package core

import (
	"slices"
	"testing"
)

// vtWriterAgent is the writer: it changes code, names its output
// {draft}, and its one outcome moves on.
func vtWriterAgent() Agent {
	workspace := WorkspaceWorktree
	return NewAgent(AgentArgs{
		ID: "writer", Name: "Writer", Key: new("draft"), Workspace: &workspace,
		Outcomes: []AgentOutcome{{Name: "done", When: "It's written.", Tone: ToneGood, Then: nfThenOf(ThenActionNext)}},
		Brief:    "Write {draft}.",
	})
}

// vtCheckerAgent is the checker: it reads the writer's output
// and writes {check}; its redo outcome goes back to the writer once.
func vtCheckerAgent() Agent {
	workspace := WorkspaceWorktreeReadOnly
	return NewAgent(AgentArgs{
		ID: "checker", Name: "Checker", Key: new("check"), Workspace: &workspace,
		Outcomes: []AgentOutcome{
			{Name: "fine", When: "It holds up.", Tone: ToneGood, Then: nfThenOf(ThenActionNext)},
			{Name: "redo", When: "It doesn't.", Tone: ToneProblem, Then: NewThen(ThenArgs{Action: ThenActionGoto, WithOutput: true, Max: new(1)}), To: []string{"writer"}},
		},
		Brief: "Check {draft} and write {check}.",
	})
}

// TestAgentsAreChecked
func TestAgentsAreChecked(t *testing.T) {
	t.Parallel()
	agent := vtWriterAgent()
	agent.Key = "spec"
	if got := ValidationAgentWarnings(agent, SampleAgents()); len(got) != 0 {
		t.Fatalf("shared key warnings %v", got)
	}
	agent.Key = "output"
	if got := ValidationAgentWarnings(agent, nil); len(got) != 0 {
		t.Fatalf("output key warnings %v", got)
	}

	checker := vtCheckerAgent()
	checker.Outcomes[1].To = nil
	if got := ValidationAgentWarnings(checker, nil); !slices.Contains(got, "“redo” goes to a stage, but names no agent.") {
		t.Fatalf("missing no-agent warning in %v", got)
	}

	writer := vtWriterAgent()
	writer.ID = "build"
	if got := ValidationAgentWarnings(writer, SampleAgents()); !slices.Contains(got, "Another agent already has the id “build”.") {
		t.Fatalf("missing id warning in %v", got)
	}
}
