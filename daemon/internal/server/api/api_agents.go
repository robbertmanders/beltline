package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
)

// AgentRecordResponse is one agent row of the library as the API returns it.
// BeltCount is omitted when no count was given.
type AgentRecordResponse struct {
	Agent     core.Agent `json:"agent"`
	Archived  bool       `json:"archived"`
	BuiltIn   bool       `json:"builtIn"`
	BeltCount *int       `json:"beltCount,omitzero"`
	// Glyph is the icon a stage box of this agent shows: its own, else the one
	// its machine suggests.
	Glyph core.StageIcon `json:"glyph"`
	// Detail is the library row's secondary line, such as "v3 · 2 belts".
	Detail string `json:"detail"`
}

// NewAgentRecordResponse builds the agent record response from its arguments.
func NewAgentRecordResponse(record core.AgentRecord, beltCount *int) AgentRecordResponse {
	uses := core.ValueOf(beltCount)
	return AgentRecordResponse{
		Agent:     record.Agent,
		Archived:  record.Archived,
		BuiltIn:   record.BuiltIn(),
		BeltCount: core.CopyPointer(beltCount),
		Glyph:     record.Agent.Glyph(),
		Detail:    core.AgentListDetail(record.Agent, uses),
	}
}

// AgentsResponse is the body of GET /api/v1/agents.
type AgentsResponse struct {
	Agents  []AgentRecordResponse `json:"agents"`
	Choices []core.AgentChoice    `json:"choices"`
	Actions []core.AgentAction    `json:"actions"`
	// Browser is omitted when the library browser was not read.
	Browser *core.LibraryBrowser `json:"browser,omitzero"`
}

// NewAgentsResponse builds the agents response from its arguments:
// one row per record with its belt count (zero when unknown), the agents a new
// stage may copy, and the New agent action.
func NewAgentsResponse(agents []core.AgentRecord, beltCounts map[string]int, browser *core.LibraryBrowser) AgentsResponse {
	rows := make([]AgentRecordResponse, 0, len(agents))
	for _, record := range agents {
		count := beltCounts[record.Agent.ID]
		rows = append(rows, NewAgentRecordResponse(record, &count))
	}
	return AgentsResponse{
		Agents:  rows,
		Choices: core.AgentChoices(agents),
		Actions: core.AgentActionsForList(),
		Browser: core.CopyPointer(browser),
	}
}

// NewAgentsResponseFromCoordinator builds the agents response from coordinator from its arguments,
// which reads the records, their belt counts and the browser in one turn.
// Lock: none held by the caller; takes c.mu through ReadAgentsReads.
func NewAgentsResponseFromCoordinator(c *coordinator.Coordinator, search, filter string) AgentsResponse {
	reads := c.ReadAgentsReads(search, filter)
	return NewAgentsResponse(reads.Records, reads.BeltCounts, &reads.Browser)
}

// AgentDetailResponse is the selected agent's editor context: the saved record,
// the commands the daemon would accept now, and the presentation the browser
// renders.
type AgentDetailResponse struct {
	Agent        core.Agent             `json:"agent"`
	Archived     bool                   `json:"archived"`
	BuiltIn      bool                   `json:"builtIn"`
	Actions      []core.AgentAction     `json:"actions"`
	Presentation core.AgentPresentation `json:"presentation"`
	// Folder is the agent's folder on this Mac, where its version files live.
	Folder string `json:"folder"`
	// Origin is the library repository this agent was installed from, when it was.
	Origin *core.LibraryOrigin `json:"origin,omitzero"`
}

// NewAgentDetailResponse builds the agent detail response from its arguments.
func NewAgentDetailResponse(resource coordinator.AgentResource, folder string) AgentDetailResponse {
	return AgentDetailResponse{
		Agent:        resource.Record.Agent,
		Archived:     resource.Record.Archived,
		BuiltIn:      resource.Record.BuiltIn(),
		Actions:      append([]core.AgentAction{}, resource.Actions...),
		Presentation: resource.Presentation,
		Folder:       folder,
		Origin:       core.CopyPointer(resource.Record.Agent.Origin),
	}
}

// AgentDraftResponse is a draft's editor context. The agent is echoed so a
// switch-runner response carries the daemon's normalized draft.
type AgentDraftResponse struct {
	Agent        core.Agent             `json:"agent"`
	Actions      []core.AgentAction     `json:"actions"`
	Presentation core.AgentPresentation `json:"presentation"`
}

// NewAgentDraftResponse builds the agent draft response from its arguments.
func NewAgentDraftResponse(resource coordinator.AgentResource) AgentDraftResponse {
	return AgentDraftResponse{
		Agent:        resource.Record.Agent,
		Actions:      append([]core.AgentAction{}, resource.Actions...),
		Presentation: resource.Presentation,
	}
}

// AgentSaveRequest is a full agent definition for save.
type AgentSaveRequest struct {
	Agent core.Agent `json:"agent" required:"true"`
}

// AgentDraftPresentationRequest is a draft whose daemon-owned presentation the
// editor asks for.
type AgentDraftPresentationRequest struct {
	Agent core.Agent `json:"agent" required:"true"`
}

// AgentDraftSwitchRunnerRequest is a draft whose runner changed. The daemon
// normalizes the primary settings, or the alternate row at AlternateIndex.
type AgentDraftSwitchRunnerRequest struct {
	Agent          core.Agent   `json:"agent" required:"true"`
	Runner         core.Backend `json:"runner" required:"true"`
	AlternateIndex *int         `json:"alternateIndex,omitzero"`
}

// NewAgentDraftSwitchRunnerRequest builds the agent draft switch runner request from its arguments.
func NewAgentDraftSwitchRunnerRequest(agent core.Agent, runner core.Backend, alternateIndex *int) AgentDraftSwitchRunnerRequest {
	return AgentDraftSwitchRunnerRequest{Agent: agent, Runner: runner, AlternateIndex: core.CopyPointer(alternateIndex)}
}
