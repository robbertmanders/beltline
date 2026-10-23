// The run page's projection: its header facts, cost, links, watch notes and
// stages. The stage facts and inline spans are in api_run_views_facts.go, the
// map, Markdown blocks and attempt output and artifacts in api_run_views_map.go.
package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
)

// RunViewResponse is the complete daemon-owned run page projection. Navigation
// and selected attempts are intentionally left to each client.
//
// Map, Boxes and Metro are sent only for their look (omitted otherwise);
// HeldAnswer, FirstVerdict and PauseCopy are omitted when nil.
type RunViewResponse struct {
	Run core.Run `json:"run"`
	// AvailableWidth is the width the page has for the belt's drawing.
	AvailableWidth float64 `json:"availableWidth"`
	// Look is how the belt is drawn: `boxes`, `machines` or `metro` (BeltLook).
	Look string `json:"look"`
	// Map is the belt as machines on a track. Sent only for the `machines` look.
	Map *core.RunMap `json:"map,omitzero"`
	// Boxes is the belt as stage boxes. Sent only for the `boxes` look.
	Boxes *core.RunBoxes `json:"boxes,omitzero"`
	// Metro is the belt as a metro line. Sent only for the `metro` look.
	Metro    *core.RunMetro `json:"metro,omitzero"`
	Metadata string         `json:"metadata"`
	// Heading is the tag over the title: the belt's name and the run's repository, short.
	Heading string `json:"heading"`
	// Reference is the subject's reference: `#139`, or a ticket key.
	Reference    string  `json:"reference"`
	FirstVerdict *string `json:"firstVerdict,omitzero"`
	// Shadow is the chip of a run on a shadow version, such as "Shadow v2".
	Shadow      *string     `json:"shadow,omitzero"`
	PauseCopy   *string     `json:"pauseCopy,omitzero"`
	ErasureCopy string      `json:"erasureCopy"`
	Cost        RunCostWire `json:"cost"`
	// SessionCount is how many attempts ran an agent session: the Sessions button's count.
	SessionCount        int                             `json:"sessionCount"`
	Links               []RunLinkWire                   `json:"links"`
	Worktrees           []RunWorktreeWire               `json:"worktrees"`
	WorktreeAction      string                          `json:"worktreeAction"`
	WorktreeUnavailable string                          `json:"worktreeUnavailable"`
	WatchNotes          []core.RunPresentationWatchNote `json:"watchNotes"`
	Stages              []RunStageWire                  `json:"stages"`
	// HeldAnswer is a held run's in-session answer button, its help and what happens next.
	HeldAnswer *core.HeldAnswerCopy `json:"heldAnswer,omitzero"`
	Legs       []core.StatusLegRow  `json:"legs"`
	Actions    []core.RunAction     `json:"actions"`
	// Afk is the run's AFK button: its mode, whether that means AFK mode now,
	// the mode a click moves to, and its help.
	Afk RunAfkWire `json:"afk"`
	// Active is false once the run has shipped or been dropped: the page then
	// hides its live controls (AFK, Pause, Send to, Abort and Drop).
	Active bool `json:"active"`
	// QuickActions are the shortcut buttons the page shows beside its menu, in order:
	// Start now on a queued run and Retry when a saved attempt can be retried.
	QuickActions []core.RunCommand `json:"quickActions"`
	// Menu is the Run actions menu's rows, in order, each with the record it is enabled from.
	Menu []core.RunMenuRow `json:"menu"`
}

// NewRunViewResponse is the run page for the snapshot.
func NewRunViewResponse(snapshot coordinator.CoordinatorRunViewSnapshot) RunViewResponse {
	out := RunViewResponse{
		Run:            snapshot.Run,
		AvailableWidth: snapshot.AvailableWidth,
		Look:           string(snapshot.Look),
		Boxes:          snapshot.Boxes,
		Metadata:       snapshot.Presentation.Metadata,
		Reference:      snapshot.Run.Subject.Ref(),
		FirstVerdict:   snapshot.Presentation.FirstVerdict,
		Shadow:         core.ShadowChip(snapshot.Run),
		PauseCopy:      snapshot.Presentation.PauseCopy,
		ErasureCopy:    snapshot.Presentation.ErasureCopy,
		Cost:           NewRunCostWire(snapshot.Presentation.Spend, snapshot.Presentation.CostLine),
		SessionCount:   snapshot.Presentation.SessionCount,
		Links:          []RunLinkWire{},
		Worktrees:      []RunWorktreeWire{},
		WorktreeAction: runner.WorktreeTargetsAction,
		// WorktreeUnavailable is the reason when a run has no worktree to open.
		WorktreeUnavailable: runner.WorktreeTargetsUnavailable,
		WatchNotes:          snapshot.Presentation.WatchNotes,
		Stages:              []RunStageWire{},
		HeldAnswer:          core.HeldAnswerCopyForRun(snapshot.Run),
		Legs:                snapshot.Presentation.Legs,
		Actions:             snapshot.Presentation.Actions,
		Afk:                 NewRunAfkWire(snapshot.Run, snapshot.AfkGlobal),
		Active:              snapshot.Run.State.IsActive(),
		QuickActions:        core.RunQuickActions(snapshot.Run, snapshot.Presentation.Actions),
		Menu:                core.RunMenu(snapshot.Run, snapshot.Presentation.Actions),
	}
	if snapshot.Look == core.BeltLookMachines {
		runMap := core.NewRunMap(core.RunMapArgs{
			Run: snapshot.Run, Geometry: snapshot.Map, Compact: snapshot.Presentation.Compact,
			AvailableWidth: snapshot.AvailableWidth, ContentWidth: snapshot.MapContentWidth,
			Panels: snapshot.Presentation.Stages, Paused: snapshot.Pause.IsPaused || snapshot.BeltPaused,
			Ambiguous: snapshot.AmbiguousRepositories,
		})
		out.Map = &runMap
	}
	if snapshot.Look == core.BeltLookMetro {
		metro := core.RunMetroLayoutMap(snapshot.Run, snapshot.Presentation.Stages)
		out.Metro = &metro
	}
	out.Heading = core.RunPresentationHeading(snapshot.Run, snapshot.AmbiguousRepositories)
	for _, link := range snapshot.Presentation.Links {
		out.Links = append(out.Links, NewRunLinkWire(link))
	}
	for _, target := range snapshot.Worktrees {
		out.Worktrees = append(out.Worktrees, NewRunWorktreeWire(target))
	}
	for _, panel := range snapshot.Presentation.Stages {
		var adjust *RunStageAdjustWire
		if help, ok := snapshot.AdjustHelp[panel.ID]; ok {
			wire := NewRunStageAdjustWire(help)
			adjust = &wire
		}
		var facts *coordinator.StagePanelFacts
		if panelFacts, ok := snapshot.PanelFacts[panel.ID]; ok {
			facts = &panelFacts
		}
		out.Stages = append(out.Stages, NewRunStageWire(panel, adjust, facts))
	}
	return out
}

// RunAfkWire is the run's AFK button.
type RunAfkWire struct {
	Mode  string `json:"mode"`
	Label string `json:"label"`
	On    bool   `json:"on"`
	Next  string `json:"next"`
	Help  string `json:"help"`
}

// NewRunAfkWire is the run's AFK button; core.RunAfkButtonHelp writes the help.
func NewRunAfkWire(run core.Run, global bool) RunAfkWire {
	mode := run.Afk()
	on := run.AfkEffective(global)
	return RunAfkWire{
		Mode:  string(mode),
		Label: mode.Label(),
		On:    on,
		Next:  string(mode.Next()),
		Help:  core.RunAfkButtonHelp(mode, on),
	}
}

// RunWorktreeWire is one existing worktree the daemon can open in an editor.
// Detail is its full path.
type RunWorktreeWire struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// NewRunWorktreeWire is a worktree target as the page offers it.
func NewRunWorktreeWire(target runner.WorktreeTarget) RunWorktreeWire {
	return RunWorktreeWire{ID: target.ID, Title: target.Title, Detail: target.Detail()}
}

// RunCostWire is the run's cost. Nanos and Line are omitted when nil.
type RunCostWire struct {
	Nanos               *int64  `json:"nanos,omitzero"`
	Estimated           bool    `json:"estimated"`
	Partial             bool    `json:"partial"`
	EndedAgents         int     `json:"endedAgents"`
	LiveAgents          bool    `json:"liveAgents"`
	Line                *string `json:"line,omitzero"`
	EstimateExplanation string  `json:"estimateExplanation"`
}

// NewRunCostWire is the run's cost from its spend and cost line.
func NewRunCostWire(spend core.Spend, line *string) RunCostWire {
	out := RunCostWire{
		Estimated:           spend.Estimated,
		Partial:             spend.Partial,
		EndedAgents:         spend.EndedAgents,
		LiveAgents:          spend.LiveAgents,
		Line:                line,
		EstimateExplanation: core.CostEstimateExplanation,
	}
	if spend.Nanos != nil {
		nanos := int64(*spend.Nanos)
		out.Nanos = &nanos
	}
	return out
}

// RunLinkWire is one link the run page offers.
type RunLinkWire struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Reference string `json:"reference"`
	Detail    string `json:"detail"`
	URL       string `json:"url"`
}

// NewRunLinkWire is one link target as the page offers it.
func NewRunLinkWire(target core.RunLinksTarget) RunLinkWire {
	return RunLinkWire{ID: target.ID, Title: target.Title, Reference: target.Reference, Detail: target.Detail(), URL: target.URL}
}

// RunStageWire is one stage panel of the run page, with its step word and the
// snapshot's per-stage help: Adjust for an agent stage, and the panel's Facts.
type RunStageWire struct {
	core.RunPresentationStagePanel
	// Step is the panel's step word: done, running, waiting, queued, held,
	// parked, dropped, redo, notStarted, skipped.
	Step string `json:"step"`
	// Adjust is an agent stage's Adjust stage help; absent for approvals and watches.
	Adjust *RunStageAdjustWire `json:"adjust,omitzero"`
	// Facts are the panel's head chips, attempt lines and an approval's document.
	Facts *coordinator.StagePanelFacts `json:"facts,omitzero"`
}

// NewRunStageWire is the panel with its step word, adjust help and facts.
func NewRunStageWire(panel core.RunPresentationStagePanel, adjust *RunStageAdjustWire, facts *coordinator.StagePanelFacts) RunStageWire {
	step := string(panel.Step)
	if panel.Step == core.StatusStepTodo {
		step = "notStarted"
	}
	return RunStageWire{RunPresentationStagePanel: panel, Step: step, Adjust: adjust, Facts: facts}
}

// RunStageAdjustWire is a stage's Adjust sheet: its help, the current settings
// and the runners the sheet offers. AgentBrief, Thinking and RepairNote are
// omitted when nil.
type RunStageAdjustWire struct {
	Placeholders core.PlaceholderHelp `json:"placeholders"`
	Note         string               `json:"note"`
	AgentBrief   *string              `json:"agentBrief,omitzero"`
	Runner       string               `json:"runner"`
	Model        string               `json:"model"`
	Thinking     *string              `json:"thinking,omitzero"`
	Brief        string               `json:"brief"`
	// Runners is every runner the sheet offers, with the models and effort levels each takes in this run.
	Runners []RunAdjustRunnerWire `json:"runners"`
	// RepairNote is shown under the model list of a legacy blank stage.
	RepairNote *string `json:"repairNote,omitzero"`
	// FreshSessionNote is shown under the sheet's fields: the changes apply to the next fresh session.
	FreshSessionNote string `json:"freshSessionNote"`
}

// NewRunStageAdjustWire is the Adjust sheet for a stage's help.
func NewRunStageAdjustWire(help coordinator.StageAdjustHelp) RunStageAdjustWire {
	out := RunStageAdjustWire{
		Placeholders:     help.Placeholders,
		Note:             help.Note,
		AgentBrief:       help.AgentBrief,
		Runner:           string(help.Runner),
		Model:            help.Model,
		Brief:            help.Brief,
		Runners:          []RunAdjustRunnerWire{},
		RepairNote:       help.RepairNote,
		FreshSessionNote: core.StageAdjustFreshSessionNote,
	}
	if help.Thinking != nil {
		thinking := string(*help.Thinking)
		out.Thinking = &thinking
	}
	for _, rn := range help.Runners {
		out.Runners = append(out.Runners, NewRunAdjustRunnerWire(rn))
	}
	return out
}

// RunAdjustRunnerWire is one runner row of the Adjust sheet. EmptyNote is shown
// instead of the model list when this run may use no model on the runner.
type RunAdjustRunnerWire struct {
	Runner       string             `json:"runner"`
	Label        string             `json:"label"`
	Models       []core.ModelChoice `json:"models"`
	Efforts      []string           `json:"efforts"`
	ThinkingNote *string            `json:"thinkingNote,omitzero"`
	EmptyNote    *string            `json:"emptyNote,omitzero"`
}

// NewRunAdjustRunnerWire is one runner row of the Adjust sheet.
func NewRunAdjustRunnerWire(rn coordinator.StageAdjustRunner) RunAdjustRunnerWire {
	out := RunAdjustRunnerWire{
		Runner:       string(rn.Backend),
		Label:        rn.Backend.Label(),
		Models:       rn.Models,
		Efforts:      []string{},
		ThinkingNote: rn.ThinkingNote,
	}
	for _, effort := range rn.Efforts {
		out.Efforts = append(out.Efforts, string(effort))
	}
	if len(rn.Models) == 0 {
		empty := core.StageAdjustNoModelsAllowed
		out.EmptyNote = &empty
	}
	return out
}
