// Contract cases for the agent endpoints: the agent shape checks, the agent commands, the draft
// presentation and the runner switch. Each case runs on a fresh fixture; the command cases replay one
// sequence on one fixture.

package servertest

import (
	"net/url"
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// agentContractCases are the agent area's contract cases, in registry order.
var agentContractCases = []string{
	"agents", "agent", "agentNew", "agentSave", "agentDraftPresentation", "agentDraftSwitchRunner",
	"agentArchive", "agentUnarchive",
}

// agentCommandCases are the cases whose replies come from the command sequence.
var agentCommandCases = map[string]bool{
	"agentNew": true, "agentSave": true, "agentArchive": true, "agentUnarchive": true,
}

func init() {
	registerArea(agentContractCases, exerciseAgentContract)
}

// exerciseAgentContract serves one agent case on a fresh fixture and checks it against its golden.
// It reports false for a case of another area.
func exerciseAgentContract(f *Fixture, t testing.TB, contractCase string) bool {
	if !slices.Contains(agentContractCases, contractCase) {
		return false
	}
	switch {
	case contractCase == "agents":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/agents", Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		checkAgentsShape(t, g, reply.Body)
	case contractCase == "agent":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/agents/" + url.PathEscape("triage"), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		checkAgentShape(t, g, reply.Body)
	case contractCase == "agentDraftPresentation":
		g := Start(t, Options{})
		draft := g.Coordinator.NewAgentDraft()
		reply := g.Do(Request{Method: "POST", Path: contractEndpoint(t, contractCase).Path,
			Body: contractBody(t, api.AgentDraftPresentationRequest{Agent: draft}), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
	case contractCase == "agentDraftSwitchRunner":
		g := Start(t, Options{})
		agent := g.Coordinator.LatestAgent("triage").Agent
		agent.Model = "definitely-not-allowed"
		agent.Thinking = new(core.ThinkingMax)
		reply := g.Do(Request{Method: "POST", Path: contractEndpoint(t, contractCase).Path,
			Body: contractBody(t, api.NewAgentDraftSwitchRunnerRequest(agent, core.BackendCopilot, nil)), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
	case agentCommandCases[contractCase]:
		checkSequenceReply(t, contractSequence(t, "agent-commands", agentCommandReplies), contractCase)
	}
	return true
}

// agentCommandReplies replays the agent command contract (agentCommandsReturnRecordsAndGoldens) on a fresh
// fixture: a new draft, the triage agent saved with a brief, and review archived and unarchived. It
// returns each command's reply by its case.
func agentCommandReplies(t testing.TB) (*Fixture, map[string]Reply) {
	t.Helper()
	g := Start(t, Options{})
	replies := map[string]Reply{}
	post := func(contractCase, path string, body []byte) {
		replies[contractCase] = g.Do(Request{Method: "POST", Path: path, Body: body, Token: ContractToken})
	}
	post("agentNew", "/api/v1/agents/new", nil)
	agent := g.Coordinator.LatestAgent("triage").Agent
	agent.Brief = "A contract save."
	post("agentSave", "/api/v1/agents/save", contractBody(t, api.AgentSaveRequest{Agent: agent}))
	post("agentArchive", "/api/v1/agents/review/archive", nil)
	post("agentUnarchive", "/api/v1/agents/review/unarchive", nil)
	return g, replies
}

// checkAgentsShape checks the `.agents` assertions: the rows are the library's agents, each
// has its belt count, the new action is enabled, and the browser is the daemon's.
func checkAgentsShape(t testing.TB, g *Fixture, body []byte) {
	t.Helper()
	tree := contractTree(t, body)
	rows := contractMember(t, tree, "agents").([]any)
	if len(rows) != len(g.Coordinator.ReadAgents()) {
		t.Fatalf("agents: %d rows, want %d", len(rows), len(g.Coordinator.ReadAgents()))
	}
	contractSameAsValue(t, "agents browser", contractMember(t, tree, "browser"), g.Coordinator.ReadAgentBrowser("", "all"))
	for _, row := range rows {
		if contractMember(t, row, "beltCount") == nil {
			t.Fatal("agents: a row has no belt count")
		}
	}
	newEnabled := false
	for _, action := range contractMember(t, tree, "actions").([]any) {
		if contractMember(t, action, "id") == "new" && contractMember(t, action, "enabled") == true {
			newEnabled = true
		}
	}
	if !newEnabled {
		t.Fatal("agents: the new action is not enabled")
	}
}

// checkAgentShape checks the `.agent` assertions: the triage agent's record and resource
// match the coordinator's, and its version is past the first.
func checkAgentShape(t testing.TB, g *Fixture, body []byte) {
	t.Helper()
	tree := contractTree(t, body)
	var listed *core.AgentRecord
	for _, record := range g.Coordinator.ReadAgents() {
		if record.Agent.ID == "triage" {
			r := record
			listed = &r
		}
	}
	if listed == nil {
		t.Fatal("agent: the library has no triage agent")
	}
	resource := g.Coordinator.ReadAgentResource(*listed)
	contractSameAsValue(t, "agent", contractMember(t, tree, "agent"), listed.Agent)
	if contractMember(t, tree, "archived") != listed.Archived {
		t.Fatal("agent: archived differs")
	}
	contractSameAsValue(t, "agent actions", contractMember(t, tree, "actions"), resource.Actions)
	contractSameAsValue(t, "agent presentation", contractMember(t, tree, "presentation"), resource.Presentation)
	if version, _ := contractMember(t, contractMember(t, tree, "agent"), "version").(float64); version <= 1 {
		t.Fatal("agent: the triage version is not past the first")
	}
}
