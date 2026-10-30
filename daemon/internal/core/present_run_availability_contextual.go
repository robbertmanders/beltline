// RunAvailability: the contextual
// records, one per stage, leg, attempt, route and AFK offer. The entry points are in
// present_run_availability.go.

package core

// runAvailabilityContextual is the contextual records: targets in model order.
func runAvailabilityContextual(context RunActionContext) []RunAction {
	var list []RunAction
	list = append(list, runAvailabilityDecisions(context)...)
	list = append(list, runAvailabilityResumes(context)...)
	list = append(list, runAvailabilityAnswers(context)...)
	list = append(list, runAvailabilityLegs(context)...)
	list = append(list, runAvailabilitySessions(context)...)
	list = append(list, runAvailabilityRetries(context)...)
	list = append(list, runAvailabilityAdjustments(context)...)
	list = append(list, runAvailabilityOverrules(context)...)
	return list
}

// runAvailabilityDecisions is one decide record per outcome of the gate the run waits at.
func runAvailabilityDecisions(context RunActionContext) []RunAction {
	run := context.Run
	if !run.State.IsActive() || run.State != RunStateWaiting {
		return nil
	}
	stage := run.stageRef()
	if stage == nil || stage.Kind() != StageKindGate {
		return nil
	}
	var list []RunAction
	why := RunAvailabilityBaseReason(RunCommandDecide, context)
	for _, outcome := range stage.Outcomes {
		needsNote := stage.NeedsNote(outcome)
		noteLabel := StageCopyNoteLabel(StageCopyNoteTarget(outcome, run))
		thenHelp := StageCopyThenHelp(outcome, run, *stage)
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandDecide), Enabled: why == nil, Reason: why, Stage: &stage.ID,
			Outcome: &outcome.Name, NeedsNote: &needsNote, NoteLabel: &noteLabel, ThenHelp: &thenHelp}))
	}
	return list
}

// runAvailabilityResumes is one Send to record per stage. Send to ends a button park, so every
// stage, the current one included, is open.
func runAvailabilityResumes(context RunActionContext) []RunAction {
	run := context.Run
	if !run.State.IsActive() {
		return nil
	}
	var list []RunAction
	for _, stage := range run.Stages {
		why := RunAvailabilityBaseReason(RunCommandResume, context)
		stageID := stage.ID
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandResume), Enabled: why == nil, Reason: why, Stage: &stageID,
			Help: RunAvailabilityHelp(RunCommandResume, context)}))
	}
	return list
}

// runAvailabilityAnswers is the answer record for the held attempt, when there is one.
func runAvailabilityAnswers(context RunActionContext) []RunAction {
	run := context.Run
	if !run.State.IsActive() {
		return nil
	}
	held := run.Answerable()
	if held == nil {
		return nil
	}
	why := RunAvailabilityBaseReason(RunCommandAnswer, context)
	stageID := held.StageID
	attemptID := held.ID
	return []RunAction{NewRunAction(RunActionArgs{ID: string(RunCommandAnswer), Enabled: why == nil, Reason: why, Stage: &stageID, Attempt: &attemptID})}
}

// runAvailabilityLegs is the abort and drop records for each leg that has an ID.
func runAvailabilityLegs(context RunActionContext) []RunAction {
	var list []RunAction
	for _, leg := range context.Run.Legs {
		legID := LegFromID(&leg.ID)
		if legID == nil {
			continue
		}
		if leg.CanAbort(context.Run) {
			why := RunAvailabilityBaseReason(RunCommandAbort, context)
			list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandAbort), Enabled: why == nil, Reason: why, Stage: leg.StageID,
				Leg: legID, Help: RunAvailabilityHelp(RunCommandAbort, context)}))
		}
		if leg.State.IsActive() {
			why := RunAvailabilityBaseReason(RunCommandDrop, context)
			list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandDrop), Enabled: why == nil, Reason: why, Stage: leg.StageID,
				Leg: legID, Help: RunAvailabilityHelp(RunCommandDrop, context)}))
		}
	}
	return list
}

// runAvailabilitySessions is one Continue session record per attempt that has a session.
func runAvailabilitySessions(context RunActionContext) []RunAction {
	var list []RunAction
	for _, attempt := range context.Run.Attempts {
		if attempt.Session == nil {
			continue
		}
		why := RunAvailabilityBaseReason(RunCommandContinueSession, context)
		if why == nil && !context.TmuxRunner {
			why = new(RunAvailabilityContinueNeedsTmux)
		}
		if why == nil {
			if refusal, ok := context.ContinueRefusals[attempt.ID]; ok {
				why = new(refusal)
			}
		}
		stageID := attempt.StageID
		attemptID := attempt.ID
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandContinueSession), Enabled: why == nil, Reason: why, Stage: &stageID,
			Leg: attempt.Leg, Attempt: &attemptID}))
	}
	return list
}

// runAvailabilityRetries is one retry record per open attempt whose continuation failed.
func runAvailabilityRetries(context RunActionContext) []RunAction {
	var list []RunAction
	for _, attempt := range context.Run.Attempts {
		if !attempt.IsOpen() || !runAvailabilityContinuationFailed(attempt) {
			continue
		}
		why := RunAvailabilityRetryContinuationRefusal(context, attempt.Leg)
		stageID := attempt.StageID
		attemptID := attempt.ID
		leg := runAvailabilityAttemptLeg(attempt)
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandRetryContinuation), Enabled: why == nil, Reason: why, Stage: &stageID,
			Leg: &leg, Attempt: &attemptID, Help: new(RunAvailabilityRetryHelp)}))
	}
	return list
}

// runAvailabilityAdjustments is one adjust record per agent stage.
func runAvailabilityAdjustments(context RunActionContext) []RunAction {
	var list []RunAction
	for _, stage := range context.Run.Stages {
		if stage.Kind() != StageKindAgent {
			continue
		}
		why := RunAvailabilityBaseReason(RunCommandAdjustStage, context)
		if why == nil {
			if refusal, ok := context.AdjustRefusals[stage.ID]; ok {
				why = new(refusal)
			}
		}
		stageID := stage.ID
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandAdjustStage), Enabled: why == nil, Reason: why, Stage: &stageID,
			Help: new(RunAvailabilityAdjustHelp)}))
	}
	return list
}

// runAvailabilityOverrules is the AFK overrule offer for each decision, then one record per destination.
func runAvailabilityOverrules(context RunActionContext) []RunAction {
	var list []RunAction
	for _, offer := range context.Overrules {
		why := RunAvailabilityBaseReason(RunCommandOverruleAfk, context)
		decision := offer.Decision
		attemptID := offer.Attempt
		reason := why
		if offer.Refusal != nil {
			reason = offer.Refusal
		}
		list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandOverruleAfk), Enabled: why == nil && offer.Refusal == nil, Reason: reason,
			Attempt: &attemptID, Decision: &decision, Help: new(RunAvailabilityOverruleHelp)}))
		for _, destination := range offer.Destinations {
			stageID := destination.Stage
			var previous *bool
			if offer.Previous != nil && *offer.Previous == destination.Stage {
				previous = new(true)
			}
			list = append(list, NewRunAction(RunActionArgs{ID: string(RunCommandOverruleAfk), Enabled: why == nil, Reason: why, Stage: &stageID,
				Attempt: &attemptID, Decision: &decision, Help: new(RunAvailabilityOverruleHelp), Previous: previous}))
		}
	}
	return list
}
