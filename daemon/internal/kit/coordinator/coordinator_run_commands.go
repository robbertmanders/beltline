package coordinator

import (
	"context"
	"slices"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// The commands the local API shares. Every command checks availability again
// here, so a client can never bypass the table the resource advertised.
// Availability and validation problems are typed; the server maps them to 422
// without a change. Every exported method below is an entry point that takes
// c.mu; the ones that await suspend through c.Suspend. Where a command needs
// an exported coordinator method that takes c.mu itself, it calls the
// lock-held form of that method (park, unpark, pauseRun, resumeRun, startNow,
// adjustStage, overruleAfk), so the whole command is one main-actor turn.
// DecideCommand picks one of the waiting approval's outcomes. A note is required
// when the outcome asks for output. A parked or paused run ignores the decision
// and answers with the run as it is.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) DecideCommand(id, outcome string, note *string, includeSubject *bool) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandDecide)
	if err != nil {
		return core.Run{}, err
	}
	run := command.run()
	// A parked or paused run ignores a decision and answers with the run as it is, as the app did.
	if run.Parking != nil || (run.Pause != nil && run.Pause.BlocksProgress()) {
		return run, nil
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	stage := run.Stage()
	if stage == nil || stage.Kind() != core.StageKindGate {
		return core.Run{}, core.NewValidationFailure("That isn't one of this stage's outcomes.", nil)
	}
	configured := stage.Outcome(outcome)
	if configured == nil {
		return core.Run{}, core.NewValidationFailure("That isn't one of this stage's outcomes.", nil)
	}
	if err := command.refuse(core.RunActionTarget{Stage: &stage.ID, Outcome: &configured.Name}); err != nil {
		return core.Run{}, err
	}
	trimmed := trimmedOrNil(note)
	if configured.Then.WithOutput && trimmed == nil {
		return core.Run{}, core.NewValidationFailure(core.RunAvailabilityNoteRequired, nil)
	}
	prompt := c.approvalFacts(run, *stage).Prompt
	if includeSubject != nil && prompt == nil {
		return core.Run{}, core.NewValidationFailure("Source context can only be chosen at a prompt-entry gate.", nil)
	}
	if prompt != nil {
		includeSubject = new(core.ValueOr(includeSubject, prompt.Include))
	}
	if err := command.apply(core.Event{Kind: core.EventKindDecided, Outcome: configured.Name, Note: trimmed, IncludeSubject: includeSubject}, "Could not save the decision."); err != nil {
		return core.Run{}, err
	}
	c.kick()
	return command.answer("", nil)
}

// ResumeCommand continues a held or parked run at its current stage or a selected one.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) ResumeCommand(id string, stage, note *string, earlier bool, leg *string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandResume)
	if err != nil {
		return core.Run{}, err
	}
	run := command.run()
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	target := core.ValueOr(stage, run.StageID)
	if run.StageIndexOf(target) < 0 {
		return core.Run{}, core.NewValidationFailure("There's no stage with id "+target+".", nil)
	}
	if leg != nil && run.LegByID(leg) == nil {
		return core.Run{}, core.NewValidationFailure("There's no route with id "+*leg+".", nil)
	}
	if err := command.refuse(core.RunActionTarget{Stage: &target}); err != nil {
		return core.Run{}, err
	}
	message := trimmedOrNil(note)
	event := core.EventResume(stage, message, leg)
	event.Earlier = earlier
	// Sending a route to where it already is, with the same note, is a valid move that changes nothing.
	if err := command.apply(event, "Could not save the move."); err != nil {
		return core.Run{}, err
	}
	after, err := command.answer("", nil)
	if err != nil {
		return core.Run{}, err
	}
	if message != nil {
		if to := stageNamed(after, target); to != nil {
			runID := id
			c.book.note("Sent to "+to.Name+": "+*message, &runID)
		}
	}
	c.kick()
	return after, nil
}

// stageNamed is the run's stage with id, or nil.
// Lock: none (pure).
func stageNamed(run core.Run, id string) *core.Stage {
	index := run.StageIndexOf(id)
	if index < 0 {
		return nil
	}
	stage := run.Stages[index]
	return &stage
}

// AnswerCommand continues a held agent stage's session with an answer, or with
// nil after the person answered or continued it interactively. A blank answer is
// the same as nil.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) AnswerCommand(id string, answer *string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandAnswer)
	if err != nil {
		return core.Run{}, err
	}
	run := command.run()
	// A parked or paused run ignores an answer and answers with the run as it is, as the app did.
	if run.Parking != nil || (run.Pause != nil && run.Pause.BlocksProgress()) {
		return run, nil
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	held := run.Answerable()
	if held == nil {
		return core.Run{}, core.NewValidationFailure("The run isn't waiting for an answer.", nil)
	}
	if err := command.refuse(core.RunActionTarget{Stage: &held.StageID, Attempt: &held.ID}); err != nil {
		return core.Run{}, err
	}
	// A blank answer is the same as saying it was answered in the session: nil.
	if err := command.apply(core.Event{Kind: core.EventKindAnswered, Answer: trimmedOrNil(answer)}, "Could not save the answer."); err != nil {
		return core.Run{}, err
	}
	after, err := command.answer("Could not save the answer.", changed)
	if err != nil {
		return core.Run{}, err
	}
	c.kick()
	return after, nil
}

// AbortCommand holds every abortable route, or only the named one. Abort never
// creates a new state: the route waits for you with "stopped by you".
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) AbortCommand(id string, leg *string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandAbort)
	if err != nil {
		return core.Run{}, err
	}
	run := command.run()
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	var legs []string
	if leg != nil {
		canAbort := slices.ContainsFunc(run.Legs, func(candidate core.Leg) bool {
			return candidate.ID == *leg && candidate.CanAbort(run)
		})
		if !canAbort {
			return core.Run{}, core.NewValidationFailure("That route isn't running, queued or waiting to be stopped.", nil)
		}
		if err := command.refuse(core.RunActionTarget{Leg: core.LegFromID(leg)}); err != nil {
			return core.Run{}, err
		}
		legs = []string{*leg}
	} else {
		for _, candidate := range run.Legs {
			if candidate.CanAbort(run) {
				legs = append(legs, candidate.ID)
			}
		}
	}
	for _, at := range legs {
		target := at
		if err := command.apply(core.Event{Kind: core.EventKindAbort, Leg: core.LegFromID(&target)}, "Could not save the stop."); err != nil {
			return core.Run{}, err
		}
	}
	c.kick()
	return command.answer("Could not save the stop.", changed)
}

// DropCommand ends the whole active run, or the named active route.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) DropCommand(id string, leg *string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandDrop)
	if err != nil {
		return core.Run{}, err
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	if leg != nil {
		target := command.run().LegByID(leg)
		if target == nil || !target.State.IsActive() {
			return core.Run{}, core.NewValidationFailure("That route has already finished.", nil)
		}
		if err := command.refuse(core.RunActionTarget{Leg: core.LegFromID(leg)}); err != nil {
			return core.Run{}, err
		}
	}
	if err := command.apply(core.Event{Kind: core.EventKindDrop, Reason: "dropped by you", Leg: leg}, "Could not save the drop."); err != nil {
		return core.Run{}, err
	}
	return command.answer("Could not save the drop.", changed)
}

// ParkCommand parks the whole run from the Run page button. Nothing is posted.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) ParkCommand(id string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandPark)
	if err != nil {
		return core.Run{}, err
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	c.park(id)
	return command.answer("Could not save the parked run.", func(_, after core.Run) bool { return after.IsButtonParked() })
}

// UnparkCommand restores the routes a button parking saved.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) UnparkCommand(id string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandUnpark)
	if err != nil {
		return core.Run{}, err
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	c.unpark(id)
	return command.answer("Could not save the unpark.", func(_, after core.Run) bool {
		return after.Parking == nil || after.Parking.UnparkPending
	})
}

// PauseRunCommand pauses the whole run. A pause already in flight takes this
// call silently, as it did in the app.
// Lock: exported entry point; takes c.mu. Suspends: pauseRun.
func (c *Coordinator) PauseRunCommand(ctx context.Context, id string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandPause)
	if err != nil {
		return core.Run{}, err
	}
	// A pause already in flight takes this call silently, as it did in the app.
	if _, busy := c.runPauseBusy[id]; busy {
		return command.run(), nil
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	c.pauseRun(ctx, id)
	return command.answer("Could not save the pause.", func(_, after core.Run) bool { return after.Pause != nil })
}

// ResumeRunCommand resumes a run paused by you, including a reconciliation retry.
// Lock: exported entry point; takes c.mu. Suspends: resumeRun.
func (c *Coordinator) ResumeRunCommand(ctx context.Context, id string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandResumeRun)
	if err != nil {
		return core.Run{}, err
	}
	if err := command.refuse(core.RunActionTarget{}); err != nil {
		return core.Run{}, err
	}
	c.resumeRun(ctx, id)
	return command.answer("Could not save the resume.", func(_, after core.Run) bool {
		return after.Pause == nil || after.Pause.Phase == core.RunPausePhaseReconcileFailed
	})
}

// RetryContinuationCommand sends a failed Continue again, on the named route or
// the one that stopped.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) RetryContinuationCommand(ctx context.Context, id string, leg *string) (core.Run, error) {
	c.Lock()
	defer c.EndTurn()
	command, err := c.commandOn(id, core.RunCommandRetryContinuation)
	if err != nil {
		return core.Run{}, err
	}
	// A retry is refused by its own rule, which picks the route that stopped when none is named.
	if refusal := core.RunAvailabilityRetryContinuationRefusal(command.context, leg); refusal != nil {
		return core.Run{}, core.NewValidationFailure(*refusal, nil)
	}
	if err := command.apply(core.Event{Kind: core.EventKindRetryContinuation, Leg: core.LegFromID(leg)}, "Could not save the retry."); err != nil {
		return core.Run{}, err
	}
	c.kick()
	return command.answer("", nil)
}
