package core

import "strconv"

// AgentAction is one command the Agents page may offer, with a readable reason when it is
// unavailable. The daemon re-checks eligibility; the flag is presentation,
// never permission.
type AgentAction struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason,omitzero"`
}

// The five agent commands. New and Save are browser-driven drafts; the daemon
// decides Archive, Restore and Reset from the saved record.
const (
	AgentActionsNew  = "new"
	AgentActionsSave = "save"
)

// Copy used by AgentActions. Literal text is exact.
const (
	AgentActionsResetUnchanged  = "This agent already matches the version the app ships."
	AgentActionsResetNotBuiltIn = "Only the agents the app ships can be reset to their default."
	agentActionsLabelNew        = "New agent"
	agentActionsLabelSave       = "Save"
)

// AgentActionsForSaved is the ordered commands for a saved record. Save stays enabled for the
// browser's unsaved draft; the editor shows it only when the draft is dirty.
func AgentActionsForSaved(archived, builtIn, canReset, sharedRepository bool) []AgentAction {
	reset := AgentAction{ID: "reset", Label: "Reset to default", Enabled: canReset}
	if !canReset {
		reason := AgentActionsResetNotBuiltIn
		if builtIn {
			reason = AgentActionsResetUnchanged
		}
		reset.Reason = &reason
	}
	third := AgentAction{ID: "archive", Label: "Archive", Enabled: true}
	if archived {
		third = AgentAction{ID: "unarchive", Label: "Restore", Enabled: true}
	}
	share := AgentAction{ID: "share", Label: "Share", Enabled: sharedRepository && !archived}
	switch {
	case archived:
		reason := "This agent is archived. Restore it first."
		share.Reason = &reason
	case !sharedRepository:
		reason := BeltAvailabilityNoSharedRepository
		share.Reason = &reason
	}
	return []AgentAction{
		{ID: AgentActionsNew, Label: agentActionsLabelNew, Enabled: true},
		{ID: AgentActionsSave, Label: agentActionsLabelSave, Enabled: true},
		third,
		reset,
		share,
	}
}

// AgentActionsForList is the commands the agents list offers: only New, which opens a draft.
func AgentActionsForList() []AgentAction {
	return []AgentAction{{ID: AgentActionsNew, Label: agentActionsLabelNew, Enabled: true}}
}

// AgentActionsForDraft is the commands for an unsaved draft: only New and Save apply.
func AgentActionsForDraft() []AgentAction {
	return []AgentAction{
		{ID: AgentActionsNew, Label: agentActionsLabelNew, Enabled: true},
		{ID: AgentActionsSave, Label: agentActionsLabelSave, Enabled: true},
	}
}

// AgentUsage is a belt pinned to a version of the agent, for the editor's Used by section.
type AgentUsage struct {
	BeltID   string `json:"beltId"`
	BeltName string `json:"beltName"`
	Version  int    `json:"version"`
}

// AgentOutcomeTarget is an agent an outcome may send a run to, for the editor's agent picker.
type AgentOutcomeTarget struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AgentOutcomeTargets lists the library agents an outcome of the agent self may send a run
// to: every agent in the library but self, in the library's order, labelled by name.
// Archived agents stay in the list, as the editor always offered them.
func AgentOutcomeTargets(self string, records []AgentRecord) []AgentOutcomeTarget {
	targets := []AgentOutcomeTarget{}
	for _, record := range records {
		if record.Agent.ID == self {
			continue
		}
		targets = append(targets, AgentOutcomeTarget{ID: record.Agent.ID, Label: record.Agent.Name})
	}
	return targets
}

// AgentPresentation is what the editor shows beside an agent or a draft: the daemon's
// validation warnings, the outcome footer, placeholder help, the belts that pin it,
// the agents its outcomes may send a run to and whether it can be reset.
type AgentPresentation struct {
	IsNew          bool                 `json:"isNew"`
	BuiltIn        bool                 `json:"builtIn"`
	Archived       bool                 `json:"archived"`
	Warnings       []string             `json:"warnings"`
	Footer         string               `json:"footer"`
	Placeholders   PlaceholderHelp      `json:"placeholders"`
	UsedBy         []AgentUsage         `json:"usedBy"`
	OutcomeTargets []AgentOutcomeTarget `json:"outcomeTargets"`
	CanReset       bool                 `json:"canReset"`
	ResetReason    *string              `json:"resetReason,omitzero"`
	// Glyph is the icon a stage box of this agent shows: its own icon, else the one its machine suggests.
	Glyph StageIcon `json:"glyph"`
	// Status is the editor's line about the version: a new agent, unsaved changes that
	// Save would make into the next version, or the latest version.
	Status string `json:"status"`
}

// AgentStatusLine is the editor's status line for an agent, given its latest saved version
// (nil when it is not in the library yet). Only the daemon compares the two.
func AgentStatusLine(agent Agent, latest *Agent) string {
	if agent.Version == 0 {
		return "A new agent. Belts can use it once you save it."
	}
	if latest != nil && latest.SameContent(agent) {
		return "This is the latest version, v" + strconv.Itoa(agent.Version) + "."
	}
	return "Unsaved changes. Saving makes v" + strconv.Itoa(agent.Version+1) + "; belts keep the version they use until you move them."
}

// ThinkingChoice is one labelled thinking level a runner accepts. The browser never invents one.
type ThinkingChoice struct {
	ID    Thinking `json:"id"`
	Label string   `json:"label"`
}
