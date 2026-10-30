package coordinator

import (
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// StagePanelFacts is what the run page's stage panel shows beyond the shared
// stage projection, as the app's StagePanel read it: the chips in its head, each
// attempt's runner line, output file, result and AFK note, a gate's decision, a
// watch's pull requests, and an approval's document with what its good outcome
// sets in motion.
type StagePanelFacts struct {
	KindChip string `json:"kindChip"`
	// Working says who is working on the stage, or what a watch waits for.
	Working   string                   `json:"working"`
	State     core.StagePanelChip      `json:"state"`
	CanAdjust bool                     `json:"canAdjust"`
	Attempts  []core.StagePanelAttempt `json:"attempts"`
	// Deciding is the AFK decision the stage is deciding now, when this is the run's stage.
	Deciding *core.StagePanelAfkNote `json:"deciding,omitzero"`
	// WatchCards are a watch's pull requests, when it waits on them.
	WatchCards []core.StagePanelWatchCard `json:"watchCards"`
	// WatchLooking is a watch's line while no pull request is known yet.
	WatchLooking *string                     `json:"watchLooking,omitzero"`
	Approval     *StagePanelFactsApproval    `json:"approval,omitzero"`
	Adjustments  []StagePanelFactsAdjustment `json:"adjustments"`
	// Send is the Send sheet's words for this stage, as the app's SendToSheet put them.
	Send core.StageSendWords `json:"send"`
}

// StagePanelFactsApproval is an approval's document and what its good outcome sets in motion.
type StagePanelFactsApproval struct {
	// Document is the latest agent output leading into this approval's most recent visit.
	Document *string `json:"-"`
	// Blocks is the document as Markdown blocks, stripped of outcome lines and hoisted; nil when there is none.
	Blocks []core.MarkdownBlock `json:"blocks,omitzero"`
	// EmptyDocument is shown in place of the document when no agent has written one.
	EmptyDocument string  `json:"emptyDocument"`
	WillHeading   *string `json:"willHeading,omitzero"`
	// Will is each line of "Beltline will", as inline spans.
	Will [][]core.InlineSpan `json:"will"`
	// Primary is the good outcome: the wide button the others sit under.
	Primary *string `json:"primary,omitzero"`
	// OutcomesHelp is the line under the outcome buttons.
	OutcomesHelp string `json:"outcomesHelp"`
	// Outcomes are the stage's outcomes in its order, each with the words its button shows.
	Outcomes []StagePanelFactsOutcome `json:"outcomes"`
	// Prompt is a prompt stage's editor, in place of the document; nil on an approval.
	Prompt *core.PromptFacts `json:"prompt,omitzero"`
}

// StagePanelFactsOutcome is one outcome button of an approval.
type StagePanelFactsOutcome struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Label string    `json:"label"`
	Tone  core.Tone `json:"tone"`
}

// StagePanelFactsAdjustment is one stage adjustment in the panel's history.
type StagePanelFactsAdjustment struct {
	Date jsonx.Date `json:"date"`
	// Text is "Build adjusted 4 min ago · Model, Thinking".
	Text string `json:"text"`
}

// stagePanelFacts returns the stage panel facts for every stage of a run, by stage id.
// ambiguous is core.RepoLabelAmbiguousNames over the book, which the run page has worked out already.
// Lock: runs with c.mu held.
func (c *Coordinator) stagePanelFacts(run core.Run, now time.Time, beltPaused bool, ambiguous map[string]struct{}) map[string]StagePanelFacts {
	out := make(map[string]StagePanelFacts, len(run.Stages))
	for _, stage := range run.Stages {
		out[stage.ID] = c.stagePanelFactsOne(run, stage, now, beltPaused, ambiguous)
	}
	return out
}

// stagePanelFactsOne returns one stage's panel facts.
// Lock: runs with c.mu held.
func (c *Coordinator) stagePanelFactsOne(run core.Run, stage core.Stage, now time.Time, beltPaused bool, ambiguous map[string]struct{}) StagePanelFacts {
	attempts := core.AttemptPanelAttempts(stage.ID, run)
	step := core.StatusStepOf(run, stage.ID)
	shown := core.AttemptPanelShown(attempts, nil)
	kindChip := core.StagePanelKindChipOf(stage, core.UppercasedFull(core.AttemptPanelRanOn(shown, stage, c.catalogModelLabel)))
	facts := make([]core.StagePanelAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		facts = append(facts, core.StagePanelAttemptOf(attempt, run, stage, c.catalogModelLabel, now, c.autopilot.Decision(attempt.AfkDecision)))
	}
	var approval *StagePanelFactsApproval
	if stage.Kind() == core.StageKindGate {
		value := c.approvalFacts(run, stage)
		approval = &value
	}
	working := core.StagePanelWorkingOf(run, stage, ambiguous)
	var deciding *core.StagePanelAfkNote
	if stage.ID == run.StageID {
		deciding = core.StagePanelDecidingNote(c.autopilot.DecisionsFor(run.ID))
	}
	watchNotTicket := stage.Kind() == core.StageKindWatch && !stage.WatchesTicket()
	cards := []core.StagePanelWatchCard{}
	var looking *string
	if watchNotTicket {
		cards = core.StagePanelWatchCardsOf(run, ambiguous)
		looking = core.RunPresentationLooking(run)
	}
	adjustments := []StagePanelFactsAdjustment{}
	for i := len(run.Adjustments) - 1; i >= 0; i-- {
		adjustment := run.Adjustments[i]
		if adjustment.StageID != stage.ID {
			continue
		}
		adjustments = append(adjustments, StagePanelFactsAdjustment{
			Date: adjustment.Date,
			Text: core.AdjustmentHistoryLine(stage.Name, adjustment, now),
		})
	}
	return StagePanelFacts{
		KindChip:     kindChip,
		Working:      working,
		State:        core.StagePanelStateChipOf(run, stage, step, now, beltPaused, c.currentPause()),
		CanAdjust:    core.StageAdjustCanAdjust(stage, run),
		Attempts:     facts,
		Deciding:     deciding,
		WatchCards:   cards,
		WatchLooking: looking,
		Approval:     approval,
		Adjustments:  adjustments,
		Send:         core.StageSendWordsOf(stage, run),
	}
}

// catalogModelLabel is the catalog's display label of a model on a runner, or nil.
// Lock: runs with c.mu held.
func (c *Coordinator) catalogModelLabel(model string, runner core.Backend) *string {
	snapshot, ok := c.settings.ModelCatalogs[string(runner)]
	if !ok {
		return nil
	}
	for _, choice := range snapshot.Models {
		if choice.ID == model {
			return new(choice.DisplayLabel())
		}
	}
	return nil
}
