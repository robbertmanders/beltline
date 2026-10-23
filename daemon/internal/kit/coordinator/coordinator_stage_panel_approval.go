// An approval's document and the
// helpers the panel's facts use. The watch cards and the state chip live in core.

package coordinator

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// approvalFacts returns an approval's document and outcome copy.
// Lock: runs with c.mu held.
func (c *Coordinator) approvalFacts(run core.Run, stage core.Stage) StagePanelFactsApproval {
	primary := core.ApprovalPrimary(stage.Outcomes)
	document := c.approvalDocument(run, stage)
	var blocks []core.MarkdownBlock
	if document != nil {
		text := core.DocumentHoisting(core.OutcomeParserStrip(*document), core.DocumentHoistingDefaultSection)
		blocks = append([]core.MarkdownBlock{}, core.MarkdownBlockParse(text)...)
	}
	buttons := make([]StagePanelFactsOutcome, 0, len(stage.Outcomes))
	for _, outcome := range stage.Outcomes {
		buttons = append(buttons, StagePanelFactsOutcome{ID: outcome.ID, Name: outcome.Name, Label: outcome.ButtonLabel(), Tone: outcome.Tone})
	}

	if primary == nil {
		return StagePanelFactsApproval{Document: document, Blocks: blocks, EmptyDocument: core.StageCopyNoAgentWroteYet,
			Will: [][]core.InlineSpan{}, OutcomesHelp: core.StageCopyOutcomeButtonsHelp, Outcomes: buttons}
	}
	primaryID := primary.ID
	will := core.ApprovalWillOf(run, stage, *primary)
	return StagePanelFactsApproval{Document: document, Blocks: blocks, EmptyDocument: core.StageCopyNoAgentWroteYet, WillHeading: &will.Heading,
		Will: will.Lines, Primary: &primaryID, OutcomesHelp: core.StageCopyOutcomeButtonsHelp, Outcomes: buttons}
}

// approvalDocument reads the latest agent output leading into this approval's
// most recent visit, not an output written after it or on another active leg.
// Lock: runs with c.mu held.
func (c *Coordinator) approvalDocument(run core.Run, stage core.Stage) *string {
	gate := run.LastAttempt(stage.ID)
	if gate == nil {
		return nil
	}
	before := len(run.Attempts)
	for i := range run.Attempts {
		if run.Attempts[i].ID == gate.ID {
			before = i
			break
		}
	}
	for i := before - 1; i >= 0; i-- {
		attempt := run.Attempts[i]
		if gate.Leg != nil && attempt.Leg != nil && !core.SamePointee(gate.Leg, attempt.Leg) {
			continue
		}
		if text := c.outputs.output(run, attempt); text != nil {
			return text
		}
	}
	return nil
}
