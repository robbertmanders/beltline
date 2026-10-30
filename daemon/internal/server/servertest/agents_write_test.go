// The agent commands,
// the draft presentation and runner switch, and the invalid saves that write nothing.
package servertest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// TestAgentCommandsReturnRecordsAndGoldens is WriteContractTests.agentCommandsReturnRecordsAndGoldens:
// the steps in their order (the unchanged save comes before the archive).
func TestAgentCommandsReturnRecordsAndGoldens(t *testing.T) {
	g := Start(t, Options{})
	var draft core.Agent
	newReply := postJSON(g, "/api/v1/agents/new", nil)
	requireStatus(t, newReply, 200)
	CheckGolden(t, "agentNew", contractEndpoint(t, "agentNew").GoldenName, newReply, g.Root)
	mustDecode(t, newReply, &draft)
	if draft.Version != 0 || !strings.HasPrefix(draft.ID, "agent-") {
		t.Fatalf("the new draft is %q version %d", draft.ID, draft.Version)
	}

	agent := g.Coordinator.LatestAgent("triage").Agent
	agent.Brief = "A contract save."
	saveReply := postJSON(g, "/api/v1/agents/save", contractBody(t, api.AgentSaveRequest{Agent: agent}))
	requireStatus(t, saveReply, 200)
	CheckGolden(t, "agentSave", contractEndpoint(t, "agentSave").GoldenName, saveReply, g.Root)
	var saved api.AgentRecordResponse
	mustDecode(t, saveReply, &saved)
	if saved.Agent.Brief != "A contract save." || saved.Agent.Version <= agent.Version {
		t.Fatal("the save did not write the brief as a newer version")
	}
	unchanged := postJSON(g, "/api/v1/agents/save", contractBody(t, api.AgentSaveRequest{Agent: saved.Agent}))
	requireStatus(t, unchanged, 200)
	var again api.AgentRecordResponse
	mustDecode(t, unchanged, &again)
	if again.Agent.Version != saved.Agent.Version {
		t.Fatal("an unchanged save wrote a version")
	}

	archive := postJSON(g, "/api/v1/agents/review/archive", nil)
	requireStatus(t, archive, 200)
	CheckGolden(t, "agentArchive", contractEndpoint(t, "agentArchive").GoldenName, archive, g.Root)
	var archived api.AgentRecordResponse
	mustDecode(t, archive, &archived)
	if !archived.Archived {
		t.Fatal("archive did not archive the agent")
	}
	unarchive := postJSON(g, "/api/v1/agents/review/unarchive", nil)
	requireStatus(t, unarchive, 200)
	CheckGolden(t, "agentUnarchive", contractEndpoint(t, "agentUnarchive").GoldenName, unarchive, g.Root)
	var unarchived api.AgentRecordResponse
	mustDecode(t, unarchive, &unarchived)
	if unarchived.Archived {
		t.Fatal("unarchive left the agent archived")
	}
}

// TestAgentDraftPresentationAndRunnerSwitchReturnGoldens is WriteContractTests.agentDraftPresentationAndRunnerSwitchReturnGoldens.
func TestAgentDraftPresentationAndRunnerSwitchReturnGoldens(t *testing.T) {
	g := Start(t, Options{})
	draft := g.Coordinator.NewAgentDraft()
	presentation := postJSON(g, contractEndpoint(t, "agentDraftPresentation").Path,
		contractBody(t, api.AgentDraftPresentationRequest{Agent: draft}))
	requireStatus(t, presentation, 200)
	CheckGolden(t, "agentDraftPresentation", contractEndpoint(t, "agentDraftPresentation").GoldenName, presentation, g.Root)
	var response api.AgentDraftResponse
	mustDecode(t, presentation, &response)
	if response.Agent.Version != 0 {
		t.Fatal("the draft presentation's agent is not version 0")
	}
	if ids := agentActionIDs(t, presentation.Body); !reflect.DeepEqual(ids, []string{"new", "save"}) {
		t.Fatalf("draft actions %v, want new and save", ids)
	}
	if response.Presentation.Footer == "" {
		t.Fatal("the draft presentation has no footer")
	}

	agent := g.Coordinator.LatestAgent("triage").Agent
	agent.Model = "definitely-not-allowed"
	agent.Thinking = new(core.ThinkingMax)
	switched := postJSON(g, contractEndpoint(t, "agentDraftSwitchRunner").Path,
		contractBody(t, api.NewAgentDraftSwitchRunnerRequest(agent, core.BackendCopilot, nil)))
	requireStatus(t, switched, 200)
	CheckGolden(t, "agentDraftSwitchRunner", contractEndpoint(t, "agentDraftSwitchRunner").GoldenName, switched, g.Root)
	var normalized api.AgentDraftResponse
	mustDecode(t, switched, &normalized)
	if normalized.Agent.Runner != core.BackendCopilot {
		t.Fatal("the switch did not move the runner to copilot")
	}
	// The app's runner picker kept the model and let the save's warnings report it.
	if normalized.Agent.Model != agent.Model {
		t.Fatal("the switch changed the model")
	}
}

// TestInvalidAgentSavesReturn422AndWriteNothing checks a rejected save, and that the reset command is gone.
func TestInvalidAgentSavesReturn422AndWriteNothing(t *testing.T) {
	g := Start(t, Options{})
	before := contractInventory(t, g.Root)
	agent := g.Coordinator.LatestAgent("triage").Agent
	version := agent.Version
	agent.Name = " "
	save := postJSON(g, contractEndpoint(t, "agentSave").Path, contractBody(t, api.AgentSaveRequest{Agent: agent}))
	requireStatus(t, save, 422)
	tree := contractTree(t, save.Body)
	problems, _ := contractMember(t, tree, "problems").([]any)
	if len(problems) == 0 || contractMember(t, tree, "message") != joinProblems(problems) {
		t.Fatal("the save's message is not its problems joined")
	}
	if g.Coordinator.LatestAgent("triage").Agent.Version != version {
		t.Fatal("the rejected save changed the agent")
	}
	reset := postJSON(g, "/api/v1/agents/triage/reset", []byte("{}"))
	requireStatus(t, reset, 404)
	requireSameInventory(t, before, g.Root)
}

// agentActionIDs is the ids of the actions in an agent draft response body.
func agentActionIDs(t testing.TB, body []byte) []string {
	t.Helper()
	var ids []string
	for _, action := range contractMember(t, contractTree(t, body), "actions").([]any) {
		ids = append(ids, contractMember(t, action, "id").(string))
	}
	return ids
}
