package api

import (
	"errors"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// AfkDecisionsResponse is `GET /api/v1/afk/decisions?filter=review|all`: the
// decision records in the AFK log's newest-first order for the chosen filter
// (All by default), with the derived presentation and the number each filter
// would show.
type AfkDecisionsResponse struct {
	Filter    string                `json:"filter"`
	Counts    AfkDecisionCountsWire `json:"counts"`
	Decisions []AfkDecisionWire     `json:"decisions"`
}

// NewAfkDecisionsResponse builds the afk decisions response from its arguments.
// Runs are the daemon's current runs: navigation availability and whether a
// decision can be overruled are facts about those runs. FoldersPresent holds
// the ids of decisions whose session folder is on disk.
func NewAfkDecisionsResponse(decisions []core.AfkDecision, runs []core.Run, foldersPresent map[string]struct{}, filter core.AfkLogFilter) AfkDecisionsResponse {
	// The first run with an id wins.
	byID := map[string]*core.Run{}
	for i := range runs {
		if _, seen := byID[runs[i].ID]; !seen {
			byID[runs[i].ID] = &runs[i]
		}
	}
	rows := core.AfkLogRows(decisions, filter)
	wires := make([]AfkDecisionWire, 0, len(rows))
	for _, decision := range rows {
		run := byID[decision.RunID]
		stageExists := core.AfkLogStageExists(decision, run)
		_, folderExists := foldersPresent[decision.ID]
		var offer *AfkOverruleOfferWire
		offerValue, ok := afkOverruleOfferFor(decision, run)
		if ok {
			offer = &offerValue
		}
		wires = append(wires, NewAfkDecisionWire(decision, run != nil, stageExists, folderExists, offer))
	}
	return AfkDecisionsResponse{
		Filter: string(filter),
		Counts: AfkDecisionCountsWire{
			Review: len(core.AfkLogRows(decisions, core.AfkLogFilterReview)),
			All:    len(core.AfkLogRows(decisions, core.AfkLogFilterAll)),
		},
		Decisions: wires,
	}
}

// afkOverruleOfferFor builds the wire form of the overrule offer for a run:
// the wire offer for a decision, or false when the decision offers no overrule.
func afkOverruleOfferFor(decision core.AfkDecision, run *core.Run) (AfkOverruleOfferWire, bool) {
	offer, err := core.AfkOfferFor(decision, run)
	if err == nil {
		return NewAfkOverruleOfferWire(&offer, nil)
	}
	var refusal core.AfkRefusal
	if !errors.As(err, &refusal) {
		return AfkOverruleOfferWire{}, false
	}
	return NewAfkOverruleOfferWire(nil, &refusal)
}

// AfkDecisionCountsWire says how many decisions each filter shows, so the page
// can label both without a second request.
type AfkDecisionCountsWire struct {
	Review int `json:"review"`
	All    int `json:"all"`
}

// AfkTargetWire is an agent stage an overrule can restart.
type AfkTargetWire struct {
	StageID string `json:"stageId"`
	Label   string `json:"label"`
}

// NewAfkTargetWire builds the afk target wire from its arguments.
func NewAfkTargetWire(target core.AfkTarget) AfkTargetWire {
	return AfkTargetWire{StageID: target.StageID, Label: target.Label()}
}

// AfkOverruleOfferWire says whether an applied decision can be overruled, as the
// app's Overrule button and sheet showed it: available with where it would
// restart, or unavailable with the reason. Decisions that offer no overrule
// leave it out.
type AfkOverruleOfferWire struct {
	Available bool    `json:"available"`
	Reason    *string `json:"reason,omitzero"`
	// Previous is the agent to restart by default; absent when none ran before
	// the decision.
	Previous      *AfkTargetWire  `json:"previous,omitzero"`
	Choices       []AfkTargetWire `json:"choices"`
	DecisionStage *string         `json:"decisionStage,omitzero"`
	CurrentStage  *string         `json:"currentStage,omitzero"`
	Warning       string          `json:"warning"`
	Notice        string          `json:"notice"`
	// CommentsHelp is what the restarted stage does with the comments.
	CommentsHelp string `json:"commentsHelp"`
}

// NewAfkOverruleOfferWire builds the afk overrule offer wire from its arguments.
// Exactly one of offer and refusal is non-nil. It returns false (the
// failable init's nil) for an ineligible refusal.
func NewAfkOverruleOfferWire(offer *core.AfkOffer, refusal *core.AfkRefusal) (AfkOverruleOfferWire, bool) {
	wire := AfkOverruleOfferWire{
		Warning:      core.AfkOverruleWarning,
		Notice:       core.AfkOverruleAfkNotice,
		CommentsHelp: core.RunAvailabilityOverruleHelp,
	}
	switch {
	case offer != nil:
		wire.Available = true
		if offer.Previous != nil {
			previous := NewAfkTargetWire(*offer.Previous)
			wire.Previous = &previous
		}
		wire.Choices = make([]AfkTargetWire, 0, len(offer.Choices))
		for _, choice := range offer.Choices {
			wire.Choices = append(wire.Choices, NewAfkTargetWire(choice))
		}
		decisionStage := offer.DecisionStage
		wire.DecisionStage = &decisionStage
		currentStage := offer.CurrentStage
		wire.CurrentStage = &currentStage
	case refusal != nil && refusal.Kind == core.AfkRefusalKindUnavailable:
		wire.Available = false
		reason := refusal.Message
		wire.Reason = &reason
		wire.Choices = []AfkTargetWire{}
	default:
		return AfkOverruleOfferWire{}, false
	}
	return wire, true
}

// AfkOverruleWire is the overrule an applied decision carries, with its stage's label.
type AfkOverruleWire struct {
	core.AfkOverrule
	StageLabel string `json:"stageLabel"`
}

// AfkDecisionWire is one decision: its stored record, plus the fields the AFK
// log derives so a client shows the same status without recomputing them.
type AfkDecisionWire struct {
	core.AfkDecision
	SituationLabel string           `json:"situationLabel"`
	Overrule       *AfkOverruleWire `json:"overrule,omitzero"`
	NeedsReview    bool             `json:"needsReview"`
	Label          string           `json:"label"`
	Tone           core.Tone        `json:"tone"`
	Headline       string           `json:"headline"`
	OverruleLine   *string          `json:"overruleLine,omitzero"`
	RunExists      bool             `json:"runExists"`
	StageExists    bool             `json:"stageExists"`
	// FolderExists says whether the decision's session folder is on disk; the
	// reveal action is unavailable without it.
	FolderExists bool `json:"folderExists"`
	// RevealUnavailable says why "Show session files" is unavailable, when it is.
	RevealUnavailable *string               `json:"revealUnavailable,omitzero"`
	OverruleOffer     *AfkOverruleOfferWire `json:"overruleOffer,omitzero"`
}

// NewAfkDecisionWire is the decision with what the AFK log derives from it.
func NewAfkDecisionWire(decision core.AfkDecision, runExists, stageExists, folderExists bool, overruleOffer *AfkOverruleOfferWire) AfkDecisionWire {
	wire := AfkDecisionWire{
		AfkDecision:       decision,
		SituationLabel:    decision.Situation.Label(),
		NeedsReview:       decision.NeedsReview(),
		Label:             core.AfkLogLabel(decision),
		Tone:              core.AfkLogTone(decision),
		Headline:          core.AfkLogHeadline(decision),
		OverruleLine:      core.AfkLogOverruleLine(decision),
		RunExists:         runExists,
		StageExists:       stageExists,
		FolderExists:      folderExists,
		RevealUnavailable: core.AfkLogRevealUnavailable(decision, folderExists),
		OverruleOffer:     overruleOffer,
	}
	if decision.Overrule != nil {
		wire.Overrule = &AfkOverruleWire{AfkOverrule: *decision.Overrule, StageLabel: decision.Overrule.StageLabel()}
	}
	return wire
}
