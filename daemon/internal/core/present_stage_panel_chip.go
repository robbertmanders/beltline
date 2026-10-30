// The chips and the working line of a stage panel
// (stateChip and the kind and working lines).

package core

import (
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// StagePanelChip is one chip in a stage panel's head.
type StagePanelChip struct {
	Text string `json:"text"`
	// Tone is one of good, running, you, problem or grey.
	Tone string `json:"tone"`
	// ClockSince is, while an agent stage runs and nothing pauses it, when its clock started, less any paused time.
	ClockSince *jsonx.Date `json:"clockSince,omitzero"`
}

// StagePanelStateChipOf is the chip in a stage panel's head for the stage's status step, given the
// global pause as it stands.
func StagePanelStateChipOf(run Run, stage Stage, step StatusStep, now time.Time, beltPaused bool, pause Pause) StagePanelChip {
	chip := func(text, tone string) StagePanelChip { return StagePanelChip{Text: text, Tone: tone} }
	switch step {
	case StatusStepDone:
		return chip("DONE", "good")
	case StatusStepRunning:
		// A watch has no session to suspend; an agent stage's clock waits while everything is paused.
		if stage.Kind() == StageKindWatch {
			return chip("WATCHING", "running")
		}
		if pause.IsPaused {
			return chip("PAUSED", "running")
		}
		started := now
		if attempt := run.CurrentAttempt(); attempt != nil {
			started = attempt.StartedAt.Time
		}
		elapsed := pause.Elapsed(started, now)
		since := now.Add(-time.Duration(elapsed * float64(time.Second)))
		return StagePanelChip{Text: "RUNNING", Tone: "running", ClockSince: &jsonx.Date{Time: since}}
	case StatusStepWaiting:
		return chip("WAITING FOR YOU", "you")
	case StatusStepHeld:
		reason := ValueOr(run.Reason, "held")
		return chip(UppercasedFull(reason), "problem")
	case StatusStepParked:
		return chip("PARKED", "grey")
	case StatusStepDropped:
		return chip("DROPPED", "grey")
	case StatusStepQueued:
		if beltPaused || pause.IsPaused {
			return chip("PAUSED", "grey")
		}
		return chip("QUEUED", "grey")
	case StatusStepRedo:
		return chip("WILL RUN AGAIN", "running")
	case StatusStepSkipped:
		return chip("NOT TAKEN", "grey")
	default:
		return chip("NOT STARTED", "grey")
	}
}

// StagePanelKindChipOf is the chip that names what a stage is in a stage panel's head: "YOUR PROMPT"
// for a prompt stage, "YOUR DECISION" for an approval, "WATCH" for a watch, and otherwise ranOn, the
// runner and model the stage ran on.
func StagePanelKindChipOf(stage Stage, ranOn string) string {
	switch stage.Kind() {
	case StageKindGate:
		if stage.Type == StageTypePrompt {
			return "YOUR PROMPT"
		}
		return "YOUR DECISION"
	case StageKindWatch:
		return "WATCH"
	}
	return ranOn
}

// StagePanelWorkingOf says who is working on a stage, or what a watch waits for: the runner this run
// took on an agent stage, or the watch's summary. ambiguous is the repository labels that need a
// name in full.
func StagePanelWorkingOf(run Run, stage Stage, ambiguous map[string]struct{}) string {
	if stage.Kind() == StageKindAgent {
		attempts := AttemptPanelAttempts(stage.ID, run)
		// A stage that spreads its sessions names the runner this run took there.
		runnerOf := BackendPi
		last := len(attempts) - 1
		if last >= 0 && attempts[last].Settings != nil && attempts[last].Settings.Runner != nil {
			runnerOf = *attempts[last].Settings.Runner
		} else if current := ModelPoolCurrent(run, stage); current.Runner != nil {
			runnerOf = *current.Runner
		}
		return runnerOf.Label() + " is working on it."
	}
	return RunPresentationWatchingSummary(run, stage, ambiguous)
}
