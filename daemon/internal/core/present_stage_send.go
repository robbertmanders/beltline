package core

import "strconv"

// StageSendWords is the Send sheet's words for a stage.
type StageSendWords struct {
	// Earlier is "Include Build's 2 earlier attempts", for an agent stage.
	Earlier *string `json:"earlier,omitzero"`
	// Note is what sending here starts, for an agent stage.
	Note *string `json:"note,omitzero"`
	// Again is what running the held stage again does: "Run this stage again." or "Unpark at this stage."
	Again string `json:"again"`
}

// StageSendWordsOf is the Send sheet's words for stage in run.
func StageSendWordsOf(stage Stage, run Run) StageSendWords {
	again := "Run this stage again."
	if run.State == RunStateParked {
		again = "Unpark at this stage."
	}
	if stage.Kind() != StageKindAgent {
		return StageSendWords{Again: again}
	}
	count := len(run.ClosedAttempts(stage.ID, nil))
	noun := "attempts"
	if count == 1 {
		noun = "attempt"
	}
	earlier := "Include " + stage.Name + "'s " + strconv.Itoa(count) + " earlier " + noun
	note := stage.Name + " starts a new session with its brief and current model and thinking level."
	return StageSendWords{Earlier: &earlier, Note: &note, Again: again}
}
