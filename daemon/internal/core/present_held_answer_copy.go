package core

// heldAnswerButtonCopilot is the button for a Copilot session. Copy from HeldAnswerCopy.buttonTitle.
const heldAnswerButtonCopilot = "I continued in the session"

// heldAnswerButtonDefault is the button for every other runner. Copy from HeldAnswerCopy.buttonTitle.
const heldAnswerButtonDefault = "I answered in the session"

// heldAnswerHelpCopilot is the help for a Copilot session. Copy from HeldAnswerCopy.buttonHelp.
const heldAnswerHelpCopilot = "After Continue, Beltline resumes that Copilot conversation and asks for a complete output in a new attempt. It does not assume the terminal work succeeded."

// heldAnswerHelpDefault is the help for every other runner. Copy from HeldAnswerCopy.buttonHelp.
const heldAnswerHelpDefault = "Continue the session with what you told the agent after pressing Continue in Sessions"

// heldAnswerNoteAgentFallback names the agent when the runner has no label. Copy from HeldAnswerCopy.note.
const heldAnswerNoteAgentFallback = "The agent"

// heldAnswerNoteBody follows the agent's name in the note. Copy from HeldAnswerCopy.note.
const heldAnswerNoteBody = " picks up its session where it stopped and writes its output again. “Run "

// heldAnswerNoteTail follows the stage's name in the note. Copy from HeldAnswerCopy.note.
const heldAnswerNoteTail = " again” starts a new session with the same brief and what this stage's earlier attempts already had."

// HeldAnswerCopy is what a held run's answer box says beside "Send answer": the button
// for an answer given in the session itself, its help, and what happens next. Copilot
// is resumed for a new attempt rather than assumed to have finished.
type HeldAnswerCopy struct {
	Stage       string `json:"stage"`
	ButtonTitle string `json:"buttonTitle"`
	ButtonHelp  string `json:"buttonHelp"`
	Note        string `json:"note"`
}

// HeldAnswerCopyFor is the copy for stage, with the runner of the session the answer
// continues (nil when unknown).
func HeldAnswerCopyFor(stage Stage, runner *Backend) HeldAnswerCopy {
	return HeldAnswerCopy{
		Stage:       stage.ID,
		ButtonTitle: HeldAnswerCopyButtonTitle(runner),
		ButtonHelp:  HeldAnswerCopyButtonHelp(runner),
		Note:        HeldAnswerCopyNote(stage, runner),
	}
}

// HeldAnswerCopyForRun is the copy for a held run's current stage, with the runner of
// the session the answer continues. Nil unless the run is held at a stage.
func HeldAnswerCopyForRun(run Run) *HeldAnswerCopy {
	if run.State != RunStateHeld {
		return nil
	}
	stage := run.stageRef()
	if stage == nil {
		return nil
	}
	runner := stage.Runner
	if attempt := run.LastAttempt(stage.ID); attempt != nil && attempt.Settings != nil && attempt.Settings.Runner != nil {
		runner = attempt.Settings.Runner
	}
	copied := HeldAnswerCopyFor(*stage, runner)
	return &copied
}

// HeldAnswerCopyButtonTitle is the button for runner.
func HeldAnswerCopyButtonTitle(runner *Backend) string {
	if runner != nil && *runner == BackendCopilot {
		return heldAnswerButtonCopilot
	}
	return heldAnswerButtonDefault
}

// HeldAnswerCopyButtonHelp is the help under the button for runner.
func HeldAnswerCopyButtonHelp(runner *Backend) string {
	if runner != nil && *runner == BackendCopilot {
		return heldAnswerHelpCopilot
	}
	return heldAnswerHelpDefault
}

// HeldAnswerCopyNote is what happens next for stage on runner.
func HeldAnswerCopyNote(stage Stage, runner *Backend) string {
	agent := heldAnswerNoteAgentFallback
	if runner != nil {
		agent = runner.Label()
	}
	return agent + heldAnswerNoteBody + stage.Name + heldAnswerNoteTail
}
