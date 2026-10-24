// The per-stage parts of the run page projection (RunPresentationProject): one stage panel,
// its attempt entries, and the run-wide first verdict and watch notes.

package core

import "time"

// runPresentationProjection holds what one RunPresentationProject call shares across its stages.
type runPresentationProjection struct {
	run                   Run
	layout                BeltLayout
	pausedBelt            bool
	pause                 Pause
	now                   time.Time
	ambiguousRepositories map[string]struct{}
}

// stagePanel is one stage's panel on the run page.
func (in runPresentationProjection) stagePanel(stage Stage) RunPresentationStagePanel {
	run := in.run
	attempts := AttemptPanelAttempts(stage.ID, run)
	step := StatusStepOf(run, stage.ID)
	var defaultAttempt *string
	if shown := AttemptPanelShown(attempts, nil); shown != nil {
		id := shown.ID
		defaultAttempt = &id
	}
	status := StatusStage(run, stage, in.pausedBelt, in.pause, in.now)
	var liveWatch *string
	if stage.Kind() == StageKindWatch && step == StatusStepRunning {
		summary := RunPresentationWatchingSummary(run, stage, in.ambiguousRepositories)
		liveWatch = &summary
	}
	var emptyMessage *string
	if defaultAttempt == nil {
		message := RunPresentationEmptyMessage(stage, run, step)
		emptyMessage = &message
	}
	worker := RunPresentationWorker(stage, run)
	return RunPresentationStagePanel{
		ID:               stage.ID,
		Name:             stage.Name,
		Number:           in.layout.Number(stage.ID),
		Kind:             stage.Kind(),
		Step:             step,
		Status:           status,
		DefaultAttempt:   defaultAttempt,
		Attempts:         runPresentationAttemptEntries(attempts, step),
		EmptyMessage:     emptyMessage,
		LiveWatchSummary: liveWatch,
		Worker:           &worker,
	}
}

// runPresentationAttemptEntries is the attempt list a stage panel shows.
func runPresentationAttemptEntries(attempts []Attempt, step StatusStep) []RunPresentationAttemptEntry {
	presented := make([]RunPresentationAttemptEntry, 0, len(attempts))
	for _, attempt := range attempts {
		picked := attempt.ID
		state := AttemptPanelContentFor(attempts, &picked, step)
		entry := RunPresentationAttemptEntry{
			ID:      attempt.ID,
			Number:  attempt.Number,
			Caption: AttemptPanelCaption(attempt, attempts),
			Outcome: attempt.Outcome,
			Tone:    attempt.Tone,
			Route:   attempt.Route,
			Note:    attempt.Note,
			Detail:  attempt.Detail,
			IsOpen:  attempt.IsOpen(),
		}
		switch state.Kind {
		case AttemptPanelContentRunning:
			entry.Content = RunPresentationAttemptEntryContentRunning
		case AttemptPanelContentDecision:
			entry.Content = RunPresentationAttemptEntryContentDecision
		case AttemptPanelContentAttempt:
			entry.Content = RunPresentationAttemptEntryContentHistoricalOutput
			entry.RunningAttempt = state.More
		default:
			entry.Content = RunPresentationAttemptEntryContentEmpty
		}
		presented = append(presented, entry)
	}
	return presented
}

// runPresentationFirstVerdict is the outcome of the newest attempt of the run's first stage, when that stage is an agent.
func runPresentationFirstVerdict(run Run) *string {
	if len(run.Stages) == 0 || run.Stages[0].Kind() != StageKindAgent {
		return nil
	}
	first := run.Stages[0]
	for i := len(run.Attempts) - 1; i >= 0; i-- {
		if run.Attempts[i].StageID == first.ID {
			return run.Attempts[i].Outcome
		}
	}
	return nil
}

// watchNotes is the live watch notes for the run, taken only when a watch stage that is not a ticket watch is running.
func (in runPresentationProjection) watchNotes() []RunPresentationWatchNote {
	for _, stage := range in.run.Stages {
		if stage.Kind() == StageKindWatch && !statusIsTicketWatch(stage) && StatusStepOf(in.run, stage.ID) == StatusStepRunning {
			return RunPresentationWatchNotes(in.run, in.ambiguousRepositories)
		}
	}
	return []RunPresentationWatchNote{}
}
