package coordinator

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// AgentResource is an agent record with the commands the daemon would accept now,
// plus the presentation the editor shows: warnings, the outcome footer,
// placeholder help and the belts that pin it.
type AgentResource struct {
	Record       core.AgentRecord
	Actions      []core.AgentAction
	Presentation core.AgentPresentation
}

// Agent is the record's agent version.
// Lock: none (pure).
func (r AgentResource) Agent() core.Agent { return r.Record.Agent }

// Archived reports whether the record is archived.
// Lock: none (pure).
func (r AgentResource) Archived() bool { return r.Record.Archived }

// AgentPresentation is the editor context for a saved agent or an unsaved draft.
// It is pure: it reads the library and settings and writes nothing.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) AgentPresentation(agent core.Agent, archived bool) core.AgentPresentation {
	c.Lock()
	defer c.EndTurn()
	return c.agentPresentation(agent, archived)
}

// agentPresentation is AgentPresentation with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) agentPresentation(agent core.Agent, archived bool) core.AgentPresentation {
	isNew := agent.Version == 0
	warnings := append([]string{}, c.library.SavedVersionWarnings(agent)...)
	warnings = append(warnings, core.ValidationAgentWarnings(agent, c.library.OtherAgents(agent))...)
	warnings = append(warnings, c.modelWarnings(agent)...)

	stage := core.DefaultsStageForAgent(agent, nil, nil, nil, core.TriggerKindIssues)
	usedBy := c.library.AgentUsages(agent.ID)
	var latest *core.Agent
	if record := c.library.LatestAgent(agent.ID); record != nil {
		value := record.Agent
		latest = &value
	}
	return core.AgentPresentation{
		IsNew:          isNew,
		Archived:       archived,
		Warnings:       warnings,
		Footer:         core.BriefFooter(stage, stage.Writes()),
		Placeholders:   core.PlaceholderHelpAgent(),
		UsedBy:         usedBy,
		OutcomeTargets: core.AgentOutcomeTargets(agent.ID, c.library.agents),
		Glyph:          agent.Glyph(),
		Status:         core.AgentStatusLine(agent, latest),
	}
}

// ReadAgentResource returns the saved record with its re-checked commands and presentation.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAgentResource(record core.AgentRecord) AgentResource {
	c.Lock()
	defer c.EndTurn()
	return c.readAgentResourceNow(record)
}

// readAgentResourceNow is ReadAgentResource with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) readAgentResourceNow(record core.AgentRecord) AgentResource {
	record.Agent = cloneAgent(record.Agent)
	presentation := c.agentPresentation(record.Agent, record.Archived)
	return AgentResource{
		Record:       record,
		Actions:      core.AgentActionsForSaved(record.Archived, c.hasSharedLibraryRepository()),
		Presentation: presentation,
	}
}

// DraftAgentResource returns a draft's context: the commands New and Save, and the
// presentation of the supplied draft. It writes nothing and invalidates no read.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) DraftAgentResource(agent core.Agent) AgentResource {
	c.Lock()
	defer c.EndTurn()
	agent = cloneAgent(agent)
	return AgentResource{
		Record:       core.AgentRecord{Agent: agent},
		Actions:      core.AgentActionsForDraft(),
		Presentation: c.agentPresentation(agent, false),
	}
}

// SwitchAgentRunnerDraft changes a draft's runner and nothing else. The model and
// thinking it had stay as they are; a mismatch is reported by the draft's warnings
// when it is saved. An alternate row changes in place.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SwitchAgentRunnerDraft(agent core.Agent, runner core.Backend, alternateIndex *int) core.Agent {
	c.Lock()
	defer c.EndTurn()
	next := cloneAgent(agent)
	if alternateIndex != nil && *alternateIndex >= 0 && *alternateIndex < len(next.Alternates) {
		backend := runner
		next.Alternates[*alternateIndex].Runner = &backend
	} else {
		next.Runner = runner
	}
	return next
}
