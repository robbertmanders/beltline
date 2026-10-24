// The coordinator's read of the run page, and the help for the Adjust sheet.

package coordinator

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
)

// ReadRunViewSnapshot is the run page's pure shared projection plus the facts that
// require the coordinator: clock, pause states, repository ambiguity and actions.
// stage is the stage the page shows: the Adjust help is built for that stage alone (see runFocusStage).
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadRunViewSnapshot(id string, stage *string) *CoordinatorRunViewSnapshot {
	c.Lock()
	defer c.EndTurn()
	run := c.book.Run(id)
	if run == nil {
		return nil
	}
	now := c.clock.Now()
	pause := c.currentPause()
	beltPaused := false
	for _, record := range c.library.belts {
		if record.Belt.ID == run.BeltID {
			beltPaused = record.EffectivelyPaused()
			break
		}
	}
	ambiguous := core.RepoLabelAmbiguousNames(c.book.runs)
	actions := c.runActionContextNow(*run).Actions()
	presentation := core.RunPresentationProject(*run, beltPaused, pause, now, ambiguous, actions)
	boxes := core.RunBoxesProject(*run, presentation.Stages, ambiguous, func(stage core.Stage) core.StageLook {
		return core.StageLookOf(stage, c.library.AgentFor(stage.Agent, *run))
	})
	return &CoordinatorRunViewSnapshot{
		Run:                   *run,
		BeltPaused:            beltPaused,
		Pause:                 pause,
		Now:                   now,
		AmbiguousRepositories: ambiguous,
		Actions:               actions,
		Presentation:          presentation,
		Worktrees:             runner.WorktreeTargetsFor(*run, c.paths.WorkFolder(*run)),
		AdjustHelp:            c.adjustHelpFor(*run, runFocusStage(*run, stage)),
		PanelFacts:            c.stagePanelFacts(*run, now, beltPaused, ambiguous),
		// afkDefault(for:): the global switch, which a run in the global mode follows.
		AfkGlobal: c.settings.Afk.Enabled,
		Boxes:     boxes,
	}
}

// AdjustHelp is the help the Adjust stage sheet shows: keys resolve against this
// run's own stages and the route each one is on (adjustHelp(for:)).
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) AdjustHelp(run core.Run) map[string]StageAdjustHelp {
	c.Lock()
	defer c.EndTurn()
	return c.adjustHelpFor(run, nil)
}

// runFocusStage is the stage a run page's Adjust help is built for: the requested stage when the run
// has it, else the run's current stage, which is the stage the page opens on.
// Lock: none (pure).
func runFocusStage(run core.Run, requested *string) *string {
	if requested != nil {
		for _, stage := range run.Stages {
			if stage.ID == *requested {
				return requested
			}
		}
	}
	current := run.StageID
	return &current
}

// adjustHelpFor is AdjustHelp with c.mu held, for the stage only when only is set (every agent
// stage when it is nil). The same help the Adjust stage sheet showed in the app: keys resolve
// against this run's own stages and the route each one is on.
// Lock: runs with c.mu held.
func (c *Coordinator) adjustHelpFor(run core.Run, only *string) map[string]StageAdjustHelp {
	legacy := !run.UsesStageKeys()
	note := core.StageAdjustResolutionNote
	if legacy {
		note = core.StageAdjustLegacyResolutionNote
	}
	help := map[string]StageAdjustHelp{}
	for _, stage := range run.Stages {
		if stage.Kind() != core.StageKindAgent || (only != nil && stage.ID != *only) {
			continue
		}
		currentID := stage.ID
		placeholders := core.PlaceholderHelpStages(run.Stages, &currentID,
			func(s core.Stage) *string { return c.library.KeyIn(s, run) },
			func(s core.Stage) string { return core.StageKeysPathOf(s, run.Layout()) }, legacy)
		agent := c.library.AgentFor(stage.Agent, run)
		// A stage that spreads its sessions opens on the settings this run took there.
		current := core.ModelPoolCurrent(run, stage)
		runners := make([]StageAdjustRunner, 0, len(core.AllBackend))
		for _, backend := range core.AllBackend {
			runners = append(runners, StageAdjustRunner{
				Backend: backend, Models: c.adjustmentModels(run, stage, &backend),
				Efforts: core.StageAdjustSupportedEffort(backend), ThinkingNote: backend.ThinkingNote(),
			})
		}
		runnerOf := core.ValueOr(current.Runner, core.StageAdjustBackend(stage))
		var agentBrief *string
		if agent != nil {
			agentBrief = new(agent.Brief)
		}
		var repairNote *string
		if run.ModelAdmission != nil {
			if _, repairable := run.ModelAdmission.LegacyRepairs[stage.ID]; repairable {
				repairNote = new(core.StageAdjustLegacyRepairNote)
			}
		}
		help[stage.ID] = StageAdjustHelp{
			Placeholders: placeholders,
			Note:         note,
			AgentBrief:   agentBrief,
			Runner:       runnerOf,
			Model:        current.Model,
			Thinking:     current.Thinking,
			Brief:        core.StageAdjustEffectiveBrief(stage, agent),
			Runners:      runners,
			RepairNote:   repairNote,
		}
	}
	return help
}

// runActionContextNow is the lock-free form of RunActionContext, the context the run
// page's actions are decided with (runActionContext(_:)).
// Lock: runs with c.mu held. It is the lock-held runActionContext.
func (c *Coordinator) runActionContextNow(run core.Run) core.RunActionContext {
	return c.runActionContext(run)
}
