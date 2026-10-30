// The situations an AFK run waits in and the options the assistant may choose.
// The decision file and the overrule
// extension are in engine_afk_decision.go and engine_overrule.go.

package core

import (
	"fmt"
	"strings"
)

// AfkSituationKind is why a run waits for you.
type AfkSituationKind string

const (
	// AfkSituationKindQuestion: an agent held its stage with questions.
	AfkSituationKindQuestion AfkSituationKind = "question"
	// AfkSituationKindFailure: an agent stage failed, the session ended without a known outcome.
	AfkSituationKindFailure AfkSituationKind = "failure"
	// AfkSituationKindApproval: an approval stage waits for a decision.
	AfkSituationKindApproval AfkSituationKind = "approval"
	// AfkSituationKindHold: any other hold: a watch saw something its outcomes
	// don't handle, had no pull request to watch, or an outcome reached its limit.
	AfkSituationKindHold AfkSituationKind = "hold"
)

// AfkSituation is why a run waits for you, and what the AFK assistant may do about it.
//
// Encoding: not encoded (the AFK decision stores the kind and key).
type AfkSituation struct {
	Kind AfkSituationKind
	// Key is the run's stop: the same run stopping again later is a new key.
	Key string
	// Options is what the assistant may choose, always ending with leaving it for you.
	Options []AfkOption
}

// Label is the name shown for the kind: "Open questions", "Failed stage", "Approval"
// or "Held".
func (k AfkSituationKind) Label() string {
	switch k {
	case AfkSituationKindQuestion:
		return "Open questions"
	case AfkSituationKindFailure:
		return "Failed stage"
	case AfkSituationKindApproval:
		return "Approval"
	default:
		return "Held"
	}
}

// AfkOption is one thing the AFK assistant may choose.
type AfkOption struct {
	// ID is the word the assistant writes after `Decision:`, such as `answer` or `decide approved`.
	ID     string
	Action AfkAction
	// Label is what it does, for the brief and the log.
	Label string
}

// AfkActionKind is which case of AfkAction a value is.
type AfkActionKind string

const (
	// AfkActionKindAnswer continues the held agent's session with the message.
	AfkActionKindAnswer AfkActionKind = "answer"
	// AfkActionKindRerun runs the current stage again, with the message as its note.
	AfkActionKindRerun AfkActionKind = "rerun"
	// AfkActionKindDecide picks an approval stage's outcome, with the message as its note.
	AfkActionKindDecide AfkActionKind = "decide"
	// AfkActionKindSend continues at another stage, with the message as its note.
	AfkActionKindSend AfkActionKind = "send"
	// AfkActionKindDrop drops the run.
	AfkActionKindDrop AfkActionKind = "drop"
	// AfkActionKindLeave leaves the run for you.
	AfkActionKindLeave AfkActionKind = "leave"
)

// AfkAction is what an option does. Outcome is
// set for decide, Stage for send.
type AfkAction struct {
	Kind    AfkActionKind
	Outcome string
	Stage   string
}

// AfkDecisionFile is the file a decision session writes.
const AfkDecisionFile = "decision.md"

// AfkName is who the log and the run say decided.
const AfkName = "AFK assistant"

// AfkSituationOf is what the assistant may do for a run that waits for you, given
// what you allow; nil when the run doesn't wait for you or you allow nothing
// there.
func AfkSituationOf(run Run, allowing AfkSettings) *AfkSituation {
	stage := run.Stage()
	if stage == nil {
		return nil
	}
	key := fmt.Sprintf("%s|%s|%s|%d", run.ID, run.StageID, run.State, len(run.Attempts))
	var options []AfkOption
	var kind AfkSituationKind
	switch {
	case run.State == RunStateWaiting && stage.Kind() == StageKindGate:
		// A prompt stage asks for what only you can write.
		if !allowing.Approve || stage.Type == StageTypePrompt {
			return nil
		}
		kind = AfkSituationKindApproval
		for _, outcome := range stage.Outcomes {
			if !allowing.Drop && outcome.Then.Action == ThenActionDrop {
				continue
			}
			label := "Choose “" + outcome.Name + "” (" + outcome.When + "): " +
				EngineDescribe(outcome.Then, run.Layout(), stage.ID, run.Routes())
			options = append(options, AfkOption{
				ID:     "decide " + afkNormalize(outcome.Name),
				Action: AfkAction{Kind: AfkActionKindDecide, Outcome: outcome.Name},
				Label:  label,
			})
		}
	case run.State == RunStateHeld:
		// You stopped it yourself, or it was held before its stage started.
		if run.Reason != nil && *run.Reason == "stopped by you" {
			return nil
		}
		last := run.LastAttempt(run.StageID)
		if last == nil || last.IsOpen() {
			return nil
		}
		heldByOutcome := false
		if last.Outcome != nil {
			if outcome := stage.Outcome(*last.Outcome); outcome != nil {
				heldByOutcome = outcome.Then.Action == ThenActionHold
			}
		}
		route := ValueOf(last.Route)
		switch {
		case stage.Kind() == StageKindAgent && last.Outcome == nil:
			if !allowing.RerunFailures {
				return nil
			}
			kind = AfkSituationKindFailure
			options = append(options, afkRerun(*stage))
			if run.Answerable() != nil {
				options = append(options, afkAnswer(*stage, true))
			}
		case stage.Kind() == StageKindAgent && heldByOutcome && !strings.HasPrefix(route, "Held: “"):
			if !allowing.AnswerQuestions || run.Answerable() == nil {
				return nil
			}
			kind = AfkSituationKindQuestion
			options = append(options, afkAnswer(*stage, false))
		default:
			if !allowing.ResolveHolds {
				return nil
			}
			kind = AfkSituationKindHold
			options = append(options, afkRerun(*stage))
			for _, other := range run.Layout().Path(run.Routes(), false) {
				if other.ID == stage.ID {
					continue
				}
				options = append(options, AfkOption{
					ID:     "send " + other.ID,
					Action: AfkAction{Kind: AfkActionKindSend, Stage: other.ID},
					Label:  "Send the run to " + other.Name,
				})
			}
		}
	default:
		return nil
	}
	if allowing.Drop {
		options = append(options, AfkOption{ID: "drop", Action: AfkAction{Kind: AfkActionKindDrop}, Label: "Drop the run"})
	}
	options = append(options, AfkOption{ID: "leave", Action: AfkAction{Kind: AfkActionKindLeave}, Label: "Leave it for the person to decide"})
	return &AfkSituation{Kind: kind, Key: key, Options: options}
}

// afkRerun is the option that runs the stage again (or waits at it again, or
// goes back to watching).
func afkRerun(stage Stage) AfkOption {
	var label string
	switch stage.Kind() {
	case StageKindAgent:
		label = "Run " + stage.Name + " again in a fresh session, with your message as its note"
	case StageKindGate:
		label = "Wait at " + stage.Name + " again for a decision"
		if stage.Type == StageTypePrompt {
			label = "Wait at " + stage.Name + " again for a prompt"
		}
	case StageKindWatch:
		label = "Go back to watching the pull request"
	default:
		panic("afkRerun: unknown stage kind")
	}
	return AfkOption{ID: "rerun", Action: AfkAction{Kind: AfkActionKindRerun}, Label: label}
}

// afkAnswer is the option that answers the stage's held session; failed when the
// stage failed rather than held with questions.
func afkAnswer(stage Stage, failed bool) AfkOption {
	label := "Answer " + stage.Name + "'s questions: its session continues with your message"
	if failed {
		label = "Continue " + stage.Name + "'s session with your message, telling it how to finish"
	}
	return AfkOption{ID: "answer", Action: AfkAction{Kind: AfkActionKindAnswer}, Label: label}
}
