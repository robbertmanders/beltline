// The belt editor's draft commands: applying a structural draft edit, the placeholder help and
// stage templates a draft needs, the draft actions, the new, copied and stage-updated drafts, and
// the choice checks. The lock rules for these commands are in coordinator_belt_commands.go.

package coordinator

import (
	"fmt"
	"slices"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// ApplyDraft applies one structural draft edit after re-checking the choices the
// editor restricts (archived agents and unsupported watch targets).
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ApplyDraft(request core.BeltDraftRequest) (core.Belt, error) {
	c.Lock()
	defer c.EndTurn()
	draft, _, err := c.applyDraftResultNow(request)
	return draft, err
}

// ApplyDraftResult is ApplyDraft with the trigger stash a `trigger-kind` leaves
// for the client. A stage brief the agent already has is stored as nil (the stage
// follows the agent), and alternates whose runner changed drop the model and
// effort that runner doesn't take.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ApplyDraftResult(request core.BeltDraftRequest) (core.Belt, *core.TriggerStash, error) {
	c.Lock()
	defer c.EndTurn()
	return c.applyDraftResultNow(request)
}

// applyDraftResultNow is ApplyDraftResult with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) applyDraftResultNow(request core.BeltDraftRequest) (core.Belt, *core.TriggerStash, error) {
	normalized := c.normalizedDraftRequest(request)
	draft, stash, err := core.BeltDraftEditingApplyResult(normalized)
	if err != nil {
		return core.Belt{}, nil, err
	}
	if err := CoordinatorCheckDraftChoices(normalized, draft, &c.library); err != nil {
		return core.Belt{}, nil, err
	}
	return draft, stash, nil
}

// normalizedDraftRequest makes a draft request's fields the form the edit reads.
// Lock: runs with c.mu held.
func (c *Coordinator) normalizedDraftRequest(request core.BeltDraftRequest) core.BeltDraftRequest {
	if request.StageID == nil {
		return request
	}
	var stage *core.Stage
	for i := range request.Draft.Stages {
		if request.Draft.Stages[i].ID == *request.StageID {
			stage = &request.Draft.Stages[i]
			break
		}
	}
	if stage == nil {
		return request
	}
	stageID := *request.StageID
	if request.Update != nil {
		update := *request.Update
		agent := c.library.Agent(stage.Agent)
		if update.Brief != nil {
			brief := core.StageAdjustOwnBrief(update.Brief, agent)
			update.Brief = &brief
		}
		if update.Alternates != nil {
			alternates := make([]core.SessionSettings, 0, len(update.Alternates))
			for index, alternate := range update.Alternates {
				if index >= len(stage.Alternates) || alternateRunner(stage.Alternates[index]) == alternateRunner(alternate) {
					alternates = append(alternates, alternate)
					continue
				}
				runner := alternateRunner(alternate)
				allowed := c.allowedModelIDs(runner)
				draft := core.StageAdjustDraft{Runner: runner, Model: alternate.Model, Thinking: alternate.Thinking}
				switched := core.StageAdjustSwitching(draft, runner, allowed)
				next := alternate
				next.Model = switched.Model
				next.Thinking = switched.Thinking
				alternates = append(alternates, next)
			}
			update.Alternates = alternates
		}
		request.Update = &update
	}
	if request.Stage != nil && request.Stage.ID == stageID {
		replacement := *request.Stage
		brief := replacement.Brief
		replacement.Brief = core.StageAdjustOwnBrief(&brief, c.library.Agent(replacement.Agent))
		request.Stage = &replacement
	}
	return request
}

// alternateRunner is the runner a session setting names, Pi when it names none.
// Lock: none (pure).
func alternateRunner(settings core.SessionSettings) core.Backend {
	if settings.Runner == nil {
		return core.BackendPi
	}
	return *settings.Runner
}

// allowedModelIDs are the identifiers of the models the settings allow on runner.
// Lock: runs with c.mu held.
func (c *Coordinator) allowedModelIDs(runner core.Backend) []string {
	choices := c.settings.AllowedChoices(runner, "", c.env)
	ids := make([]string, 0, len(choices))
	for _, choice := range choices {
		ids = append(ids, choice.ID)
	}
	return ids
}

// PlaceholderHelp returns the placeholder help for a draft or a saved belt, using
// pinned agent versions and the legacy-key conversion.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) PlaceholderHelp(belt core.Belt, stage *string) core.PlaceholderHelp {
	c.Lock()
	defer c.EndTurn()
	return c.library.PlaceholderHelp(belt, stage)
}

// PlaceholderHelpByID is PlaceholderHelp for a saved belt looked up by id; nil
// when there is no such belt.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) PlaceholderHelpByID(beltID string, stage *string) *core.PlaceholderHelp {
	c.Lock()
	defer c.EndTurn()
	record := c.library.Belt(beltID)
	if record == nil {
		return nil
	}
	help := c.library.PlaceholderHelp(record.Belt, stage)
	return &help
}

// StageTemplate builds a new stage for a draft, so the editor never invents
// defaults: an agent stage with a library agent's settings, or an approval or
// watch stage, placed after the draft's last stage on route.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) StageTemplate(draft core.Belt, stageType core.StageType, agent *core.AgentRef, route *string, index *int) (core.Stage, error) {
	c.Lock()
	defer c.EndTurn()
	at := core.ValueOr(index, len(draft.Stages))
	at = min(max(at, 0), len(draft.Stages))
	place := core.DefaultsPlace{
		Before: append([]core.Stage{}, draft.Stages[:at]...),
		Later:  append([]core.Stage{}, draft.Stages[at:]...),
		Forks:  draft.Forks, Route: route,
	}
	kind := draft.StageKind()
	switch stageType {
	case core.StageTypeAgent:
		libraryAgent := c.library.Agent(agent)
		if libraryAgent == nil {
			return core.Stage{}, core.NewValidationFailure("stage-template needs a library agent.", nil)
		}
		return core.DefaultsStageForAgentAt(*libraryAgent, place, draft.Stages, kind), nil
	case core.StageTypeApprove, core.StageTypePrompt, core.StageTypeWatch:
		return core.DefaultsStageAt(stageType, place, draft.Stages), nil
	}
	return core.Stage{}, core.NewValidationFailure(fmt.Sprintf("Stage type %s has no template.", stageType), nil)
}

// DraftActions returns the actions an editor's draft may take now: a draft of a
// saved belt takes its archived and paused state from the library, and every
// draft is dirty by definition.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) DraftActions(draft core.Belt) []core.BeltAction {
	c.Lock()
	defer c.EndTurn()
	return c.draftActionsNow(draft)
}

// draftActionsNow is DraftActions with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) draftActionsNow(draft core.Belt) []core.BeltAction {
	archived, paused, shadow := false, false, false
	if record := c.library.Belt(draft.ID); record != nil {
		archived, paused, shadow = record.Archived, record.Paused, record.ShadowTest() != nil
	}
	return core.BeltAvailabilityActions(core.BeltActionContext{
		Saved:              draft.Version > 0,
		Archived:           archived,
		Paused:             paused,
		Warnings:           c.library.Warnings(draft),
		OutdatedStages:     c.outdatedStageCount(draft),
		HasManualTrigger:   draft.HasManualTrigger(),
		Dirty:              true,
		SharedRepository:   c.hasSharedLibraryRepository(),
		Shadow:             shadow,
		ShadowTestsEnabled: c.settings.EnableShadowTests,
	})
}

// NewBeltDraft returns a new unsaved belt from the existing library defaults and an unused id.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) NewBeltDraft() core.Belt {
	c.Lock()
	defer c.EndTurn()
	return c.library.NewBelt()
}

// CopyBeltDraft returns a copy of the supplied draft at a different unused id, with version 0 and a "Copy of" name.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) CopyBeltDraft(draft core.Belt) core.Belt {
	c.Lock()
	defer c.EndTurn()
	return c.library.CopyBelt(draft)
}

// UpdatingStageDraft returns the draft with one stage moved to its agent's latest version.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) UpdatingStageDraft(draft core.Belt, stageID *string, index *int) (core.Belt, error) {
	c.Lock()
	defer c.EndTurn()
	at := -1
	switch {
	case stageID != nil:
		for i, stage := range draft.Stages {
			if stage.ID == *stageID {
				at = i
				break
			}
		}
		if at < 0 {
			return core.Belt{}, core.NewValidationFailure("No stage with id "+*stageID+" is on the belt.", nil)
		}
	case index != nil:
		if *index < 0 || *index >= len(draft.Stages) {
			return core.Belt{}, core.NewValidationFailure(fmt.Sprintf("Stage index %d is outside the stage list.", *index), nil)
		}
		at = *index
	default:
		return core.Belt{}, core.NewValidationFailure("update-stage needs a stageId or index.", nil)
	}
	if c.library.NewerAgent(draft.Stages[at]) == nil {
		return core.Belt{}, core.NewValidationFailure(draft.Stages[at].Name+" already uses its agent's latest version.", nil)
	}
	return c.library.UpdatingStage(at, draft), nil
}

// UpdatingAllStagesDraft returns the draft with every stage moved to its agent's latest version.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) UpdatingAllStagesDraft(draft core.Belt) (core.Belt, error) {
	c.Lock()
	defer c.EndTurn()
	outdated := slices.ContainsFunc(draft.Stages, func(stage core.Stage) bool {
		return c.library.NewerAgent(stage) != nil
	})
	if !outdated {
		return core.Belt{}, core.NewValidationFailure("Every stage already uses its agent's latest version.", nil)
	}
	return c.library.UpdatingAllStages(draft), nil
}

// CoordinatorCheckDraftChoices re-checks the choices the editor offers before a
// draft request is applied: a new stage or a watch target.
// It reads the library and writes nothing.
// Lock: none; it reads the library passed to it, which the caller holds.
func CoordinatorCheckDraftChoices(request core.BeltDraftRequest, draft core.Belt, library *Library) error {
	var stages []core.Stage
	switch request.Op {
	case core.BeltDraftOperationStageAdd, core.BeltDraftOperationFeedInto:
		if request.Stage != nil {
			stages = []core.Stage{*request.Stage}
		}
	case core.BeltDraftOperationStageUpdate:
		var stage *core.Stage
		if request.Stage != nil {
			stage = request.Stage
		} else if request.StageID != nil {
			for i := range draft.Stages {
				if draft.Stages[i].ID == *request.StageID {
					stage = &draft.Stages[i]
					break
				}
			}
		}
		if stage != nil && (request.Update != nil && request.Update.Agent != nil || request.Stage != nil || request.Update != nil && request.Update.Watch != nil) {
			stages = []core.Stage{*stage}
		}
	}
	for _, stage := range stages {
		if stage.Kind() == core.StageKindAgent && stage.Agent != nil {
			record := library.LatestAgent(stage.Agent.ID)
			if record == nil {
				return core.NewValidationFailure(stage.Name+" uses an agent that isn't in the library.", nil)
			}
			if record.Archived {
				return core.NewValidationFailure(stage.Name+" uses "+record.Agent.Name+", which is archived. Restore it or pick another agent.", nil)
			}
			menu := draft
			if request.Op == core.BeltDraftOperationStageUpdate && request.StageID != nil {
				for i := range draft.Stages {
					if draft.Stages[i].ID == *request.StageID {
						menu.Stages = append(append([]core.Stage{}, draft.Stages[:i]...), draft.Stages[i+1:]...)
						break
					}
				}
			}
			if !core.InputsSubset(record.Agent.Workspace.Needs(), menu.MenuInputs()) {
				return core.NewValidationFailure(record.Agent.Name+" needs an input this belt's triggers don't provide.", nil)
			}
		}
		if stage.Kind() == core.StageKindWatch {
			target := core.WatchTargetPullRequest
			if stored := stage.WatchTarget(); stored != nil {
				target = *stored
			}
			switch target {
			case core.WatchTargetTicket:
				if _, ok := draft.MenuInputs()[core.InputTicket]; !ok {
					return core.NewValidationFailure(stage.Name+" watches the issue or ticket, which this belt's triggers don't provide.", nil)
				}
			case core.WatchTargetPullRequest:
				if _, ok := draft.MenuInputs()[core.InputPullRequest]; !ok {
					return core.NewValidationFailure(stage.Name+" watches a pull request, which this belt's triggers don't provide.", nil)
				}
			}
		}
	}
	return nil
}
