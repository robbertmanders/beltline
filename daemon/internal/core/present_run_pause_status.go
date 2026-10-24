package core

// RunPauseStatusOverlay is how a run paused by you reads, on top of the stage status it
// already had.
func RunPauseStatusOverlay(status StageStatus, run Run, stage Stage) StageStatus {
	step := StatusStepOf(run, stage.ID)
	if step == StatusStepDone || step == StatusStepSkipped {
		return status
	}
	if leg := run.LegAt(&stage.ID); leg != nil && leg.IsFinished() {
		return status
	}
	if attempt := lastOpenAttemptAt(run, stage.ID); attempt != nil && attempt.Interruption != nil &&
		attempt.Interruption.Continuation != nil && attempt.Interruption.Continuation.Status == AttemptInterruptionContinuationStatusQueued && run.Pause == nil {
		return StageStatus{Text: "QUEUED TO CONTINUE", Look: StageStatusLookPlain, Paused: true}
	}
	if run.Pause == nil || !run.Pause.BlocksProgress() {
		return status
	}
	pause := run.Pause
	switch pause.Phase {
	case RunPausePhasePausing:
		return StageStatus{Text: "PAUSING…", Look: status.Look, Paused: true}
	case RunPausePhaseIncomplete:
		for _, attempt := range run.Attempts {
			if attempt.StageID == stage.ID && attempt.IsOpen() && attempt.Interruption != nil &&
				attempt.Interruption.Confirmation == AttemptInterruptionConfirmationFailed {
				return StageStatus{Text: "PAUSE INCOMPLETE", Look: StageStatusLookHeld, Paused: true}
			}
		}
		return StageStatus{Text: "PAUSED · " + status.Text, Look: status.Look, Paused: true}
	case RunPausePhaseReconcileFailed:
		reason := ValueOr(pause.Reason, "COULD NOT READ THE SOURCE")
		return StageStatus{Text: "PAUSED · " + reason, Look: StageStatusLookHeld, Paused: true}
	default:
		return StageStatus{Text: "PAUSED · " + status.Text, Look: status.Look, Paused: true}
	}
}

// lastOpenAttemptAt is the last open attempt at stageID, or nil.
// the last one the overlay reads.
func lastOpenAttemptAt(run Run, stageID string) *Attempt {
	for i := len(run.Attempts) - 1; i >= 0; i-- {
		if run.Attempts[i].StageID == stageID && run.Attempts[i].IsOpen() {
			return &run.Attempts[i]
		}
	}
	return nil
}

// RunPauseStatusLocationWord is the word after the stage name in the work location. Nil
// keeps the ordinary line.
func RunPauseStatusLocationWord(run Run) *string {
	if run.State.IsActive() && run.Pause != nil && run.Pause.BlocksProgress() {
		var word string
		switch run.Pause.Phase {
		case RunPausePhasePausing:
			word = "pausing…"
		case RunPausePhasePaused, RunPausePhaseResuming:
			word = "paused by you"
		case RunPausePhaseIncomplete:
			word = "pause incomplete"
		case RunPausePhaseReconcileFailed:
			word = "paused by you"
		default:
			return nil
		}
		return &word
	}
	for _, attempt := range run.Attempts {
		if attempt.IsOpen() && attempt.Interruption != nil && attempt.Interruption.Continuation != nil &&
			attempt.Interruption.Continuation.Status == AttemptInterruptionContinuationStatusQueued {
			word := "queued to continue"
			return &word
		}
	}
	return nil
}

// RunPauseStatusCounts reports whether a run's pause counts it as paused.
func RunPauseStatusCounts(run Run) bool {
	if !run.State.IsActive() || run.Pause == nil {
		return false
	}
	return run.Pause.BlocksProgress()
}
