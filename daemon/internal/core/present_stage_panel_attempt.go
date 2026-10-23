// The attempt lines and AFK notes of a stage panel
// (attemptFacts and afkNote, and the deciding note).

package core

import (
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// StagePanelAttempt is one attempt's line in a stage panel (StagePanelFacts.Attempt).
type StagePanelAttempt struct {
	ID string `json:"id"`
	// RanOn is the runner and model the attempt ran on.
	RanOn string `json:"ranOn"`
	// File is the output file, relative to the data folder.
	File     *string `json:"file,omitzero"`
	Duration *string `json:"duration,omitzero"`
	// Decision is a gate's decision: what was chosen, or "Not decided yet".
	Decision  *string     `json:"decision,omitzero"`
	DecidedBy *string     `json:"decidedBy,omitzero"`
	DecidedAt *jsonx.Date `json:"decidedAt,omitzero"`
	// DecisionLine is a gate's line under the decision: "You decided 4 min ago.", or "This stage waits for you.".
	DecisionLine *string                  `json:"decisionLine,omitzero"`
	Result       *StagePanelAttemptResult `json:"result,omitzero"`
	// Afk is the AFK decision that ended this attempt's wait for you.
	Afk *StagePanelAfkNote `json:"afk,omitzero"`
}

// StagePanelAttemptResult is what an attempt ended with: the outcome row, the route it took, and the note or detail that went with it.
type StagePanelAttemptResult struct {
	// Outcome is the outcome's chip text; "failed" for a failure without one. Nil shows no outcome row.
	Outcome *string `json:"outcome,omitzero"`
	// Tone is the outcome's tone, as Tone names it.
	Tone  *string `json:"tone,omitzero"`
	Route *string `json:"route,omitzero"`
	// Note is "Your message: …", for an agent's note.
	Note *string `json:"note,omitzero"`
	// Detail is the detail, as the stage words it ("Your note: …" on a gate).
	Detail  *string `json:"detail,omitzero"`
	Problem bool    `json:"problem"`
}

// StagePanelAfkNote is an AFK decision as the AFK log presents it, for the panel's note.
type StagePanelAfkNote struct {
	// Title is the headline, such as "AFK assistant · Chose “approved” at Approve".
	Title        string  `json:"title"`
	OverruleLine *string `json:"overruleLine,omitzero"`
	Comments     *string `json:"comments,omitzero"`
	Why          *string `json:"why,omitzero"`
	// Deciding is set while the decision is still being made: the note offers a working bar instead of See on AFK.
	Deciding bool `json:"deciding"`
}

// StagePanelAttemptOf is one attempt's line in a stage panel. label is the catalog's display label of a model
// on a runner, or nil. afk is the AFK decision that ended the attempt's wait for you, when it is known.
func StagePanelAttemptOf(attempt Attempt, run Run, stage Stage, label func(model string, runner Backend) *string,
	now time.Time, afk *AfkDecision) StagePanelAttempt {
	isGate := stage.Kind() == StageKindGate
	var decidedBy *string
	if isGate && attempt.EndedAt != nil {
		by := "You"
		if attempt.AfkDecision != nil {
			by = "The " + AfkName
		}
		decidedBy = &by
	}
	problem := attempt.Tone != nil && *attempt.Tone == ToneProblem
	var outcome *string
	switch {
	case attempt.Outcome != nil:
		outcome = attempt.Outcome
	case problem:
		failed := StageCopyFailedOutcome
		outcome = &failed
	}
	var note *string
	if attempt.Note != nil {
		text := StageCopyMessageLine(*attempt.Note)
		note = &text
	}
	var detail *string
	if attempt.Detail != nil {
		text := StageCopyDetailLine(*attempt.Detail, isGate)
		detail = &text
	}
	var result *StagePanelAttemptResult
	if outcome != nil || problem || note != nil || detail != nil {
		result = &StagePanelAttemptResult{Outcome: outcome, Route: attempt.Route, Note: note, Detail: detail, Problem: problem}
		if attempt.Tone != nil {
			tone := string(*attempt.Tone)
			result.Tone = &tone
		}
	}
	var decisionLine *string
	var decidedAt *jsonx.Date
	var decision *string
	if isGate {
		line := StageCopyWaitsForYou
		if attempt.EndedAt != nil {
			by := ValueOr(decidedBy, "You")
			line = StageCopyDecidedLine(by, attempt.EndedAt.Time, now)
			at := *attempt.EndedAt
			decidedAt = &at
		}
		decisionLine = &line
		text := StageCopyNotDecidedYet
		if attempt.Outcome != nil {
			text = CapitalizedFirst(*attempt.Outcome)
		}
		decision = &text
	}
	var file *string
	if stage.Kind() == StageKindAgent {
		path := "runs/" + run.Folder() + "/" + attempt.Folder + "/" + stage.Writes()
		file = &path
	}
	var duration *string
	if attempt.EndedAt != nil {
		text := StatusDuration(attempt.EndedAt.Time.Sub(attempt.StartedAt.Time).Seconds())
		duration = &text
	}
	var afkNote *StagePanelAfkNote
	if afk != nil {
		note := StagePanelAfkNoteOf(*afk)
		afkNote = &note
	}
	return StagePanelAttempt{
		ID:           attempt.ID,
		RanOn:        AttemptPanelRanOn(&attempt, stage, label),
		File:         file,
		Duration:     duration,
		Decision:     decision,
		DecidedBy:    decidedBy,
		DecidedAt:    decidedAt,
		DecisionLine: decisionLine,
		Result:       result,
		Afk:          afkNote,
	}
}

// StagePanelAfkNoteOf is the AFK log's words for one decision: its headline, overrule line, comments and why.
func StagePanelAfkNoteOf(decision AfkDecision) StagePanelAfkNote {
	var comments *string
	if decision.Overrule != nil && decision.Overrule.Comments != "" {
		text := decision.Overrule.Comments
		comments = &text
	}
	var why *string
	if decision.Why != nil && *decision.Why != "" {
		text := *decision.Why
		why = &text
	}
	return StagePanelAfkNote{
		Title:        AfkName + " · " + AfkLogHeadline(decision),
		OverruleLine: AfkLogOverruleLine(decision),
		Comments:     comments,
		Why:          why,
		Deciding:     decision.Status == AfkDecisionStatusDeciding,
	}
}

// StagePanelDecidingNote is the AFK note of the decision being made now, the last one of decisions in
// the run's order that is still deciding, or nil.
func StagePanelDecidingNote(decisions []AfkDecision) *StagePanelAfkNote {
	for i := len(decisions) - 1; i >= 0; i-- {
		if decisions[i].Status == AfkDecisionStatusDeciding {
			note := StagePanelAfkNoteOf(decisions[i])
			return &note
		}
	}
	return nil
}
