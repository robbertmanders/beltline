// AfkSettings, AfkOverrule
// and AfkDecision.

package core

import (
	"encoding/json"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// AfkSettings is AFK mode: while you are away, an assistant takes the steps
// that wait for you, within what you allow it, and keeps a log of every decision
// for you to look over when you are back.
//
// Encoding: synthesized. Decoding is custom: a field saved by an older version,
// or missing, takes its default from DefaultAfkSettings, and a backend or
// thinking that is not one of its cases does too.
type AfkSettings struct {
	Enabled bool `json:"enabled"`
	// AnswerQuestions answers an agent that held its stage with questions, by continuing its session.
	AnswerQuestions bool `json:"answerQuestions"`
	// RerunFailures runs a failed stage again, or continues its session with advice.
	RerunFailures bool `json:"rerunFailures"`
	// Approve picks an outcome at an approval stage.
	Approve bool `json:"approve"`
	// ResolveHolds carries on after any other hold, such as a watch that saw
	// something its outcomes don't handle or an outcome that reached its limit:
	// run the stage again or send the run to another stage.
	ResolveHolds bool `json:"resolveHolds"`
	// Drop drops a run, or picks an approval outcome that drops it.
	Drop bool `json:"drop"`
	// Limit is how many decisions it makes at one stage of one run before it leaves
	// the run for you, so it never loops.
	Limit int `json:"limit"`
	// Guidance is what you tell it to keep in mind, such as "never approve a spec that touches billing".
	Guidance string    `json:"guidance"`
	Backend  Backend   `json:"backend"`
	Model    string    `json:"model"`
	Thinking *Thinking `json:"thinking,omitzero"`
}

// DefaultAfkSettings is the default AFK settings:
// off, answering, rerunning and approving on, holds resolved, drop off, a limit
// of 3, and the Pi backend.
func DefaultAfkSettings() AfkSettings {
	return AfkSettings{
		Enabled: false, AnswerQuestions: true, RerunFailures: true, Approve: true,
		ResolveHolds: true, Drop: false, Limit: 3, Guidance: "", Backend: BackendPi, Model: "", Thinking: nil,
	}
}

// UnmarshalJSON reads the settings over DefaultAfkSettings, so a missing field
// keeps its default. The backend falls back to the default, and the thinking to
// none, when it is not readable.
func (s *AfkSettings) UnmarshalJSON(data []byte) error {
	type plain AfkSettings
	var w struct {
		plain
		Backend  json.RawMessage `json:"backend"`
		Thinking json.RawMessage `json:"thinking"`
	}
	w.plain = plain(DefaultAfkSettings())
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	out := AfkSettings(w.plain)
	out.Backend = DefaultAfkSettings().Backend
	if backend, ok := codecTry[Backend](w.Backend); ok && enumKnown(backend, AllBackend) {
		out.Backend = backend
	}
	out.Thinking = nil
	if thinking, ok := codecTry[Thinking](w.Thinking); ok && enumKnown(thinking, AllThinking) {
		out.Thinking = &thinking
	}
	*s = out
	return nil
}

// AfkOverrulePhase is where an overrule's restart is.
type AfkOverrulePhase string

const (
	// AfkOverrulePhaseRestarted means the run was queued at the target stage.
	AfkOverrulePhaseRestarted AfkOverrulePhase = "restarted"
	// AfkOverrulePhaseHeld means the restart could not be finished; the run was held and the comments kept.
	AfkOverrulePhaseHeld AfkOverrulePhase = "held"
)

// AfkOverrule is a person's rejection of an applied AFK decision: their comments
// and the agent stage the run was sent back to. The decision's own status and
// choice stay as the assistant left them.
//
// Encoding writes comments, stageId, stageName, routeName (omitted when nil),
// at, attemptId, phase, detail (omitted when nil).
type AfkOverrule struct {
	Comments  string `json:"comments"`
	StageID   string `json:"stageId"`
	StageName string `json:"stageName"`
	// RouteName is the fork route's name when the stage sits on one, so repeated agents stay distinct.
	RouteName *string    `json:"routeName,omitzero"`
	At        jsonx.Date `json:"at"`
	// AttemptID is the attempt the decision ended.
	AttemptID string           `json:"attemptId"`
	Phase     AfkOverrulePhase `json:"phase"`
	// Detail is why a restart was held, when it was.
	Detail *string `json:"detail,omitzero"`
}

// StageLabel is the stage's name, with its route's name after " · " when it has
// one.
func (o AfkOverrule) StageLabel() string {
	if o.RouteName != nil {
		return o.StageName + " · " + *o.RouteName
	}
	return o.StageName
}

// AfkDecisionStatus is where an AFK decision is.
type AfkDecisionStatus string

const (
	// AfkDecisionStatusDeciding means its session is still deciding.
	AfkDecisionStatusDeciding AfkDecisionStatus = "deciding"
	// AfkDecisionStatusApplied means it decided and Beltline carried the decision out.
	AfkDecisionStatusApplied AfkDecisionStatus = "applied"
	// AfkDecisionStatusLeftForYou means it chose to leave the run for you, or was not allowed to go on.
	AfkDecisionStatusLeftForYou AfkDecisionStatus = "leftForYou"
	// AfkDecisionStatusFailed means its session failed or wrote no decision Beltline could use.
	AfkDecisionStatusFailed AfkDecisionStatus = "failed"
	// AfkDecisionStatusSuperseded means the run moved on before it decided, so nothing was done.
	AfkDecisionStatusSuperseded AfkDecisionStatus = "superseded"
)

// AfkDecision is one decision the AFK assistant made, or is making, for a run that waited for you.
type AfkDecision struct {
	ID       string `json:"id"`
	RunID    string `json:"runId"`
	BeltName string `json:"beltName"`
	// SubjectRef is such as "#142" or "PROJ-7".
	SubjectRef   string `json:"subjectRef"`
	SubjectTitle string `json:"subjectTitle"`
	StageID      string `json:"stageId"`
	StageName    string `json:"stageName"`
	// Situation is what kind of wait the decision is about.
	Situation AfkSituationKind `json:"situation"`
	// Reason is why the run stopped, as the run said when the assistant started.
	Reason *string `json:"reason,omitzero"`
	// Key is the run's stop this decision is about; a new stop is a new decision.
	Key       string            `json:"key"`
	Status    AfkDecisionStatus `json:"status"`
	StartedAt jsonx.Date        `json:"startedAt"`
	DecidedAt *jsonx.Date       `json:"decidedAt,omitzero"`
	// Choice is the option it chose, such as `answer` or `decide approved`.
	Choice *string `json:"choice,omitzero"`
	// Action is what the choice did, in words, such as "Approved: approved".
	Action *string `json:"action,omitzero"`
	// Message is what it told the agent or wrote as the decision's note.
	Message *string `json:"message,omitzero"`
	// Why holds its reasons, for you to check.
	Why *string `json:"why,omitzero"`
	// Detail is why it failed or was left for you, when Beltline says so.
	Detail    *string `json:"detail,omitzero"`
	Session   *string `json:"session,omitzero"`
	SessionID *string `json:"sessionId,omitzero"`
	// Backend, Model and Thinking are what its session ran on, so a restart can find it again.
	Backend  *Backend  `json:"backend,omitzero"`
	Model    *string   `json:"model,omitzero"`
	Thinking *Thinking `json:"thinking,omitzero"`
	// Folder is its folder, relative to the run's folder.
	Folder string `json:"folder"`
	// Reviewed is whether you have looked at it and agree. An overrule is recorded separately.
	Reviewed bool `json:"reviewed"`
	// Overrule is set when you overrule an applied decision. Absent on decisions saved before that.
	Overrule *AfkOverrule `json:"overrule,omitzero"`
}

// AfkDecisionArgs are the parameters of NewAfkDecision.
type AfkDecisionArgs struct {
	ID        string
	Run       Run
	Situation AfkSituation
	StartedAt jsonx.Date
	Folder    string
}

// NewAfkDecision starts a decision for run in situation: the run's id, belt
// name, subject ref and title, stage, reason and the situation's key, with
// status deciding and not reviewed.
func NewAfkDecision(a AfkDecisionArgs) AfkDecision {
	stageName := a.Run.StageID
	if stage := a.Run.Stage(); stage != nil {
		stageName = stage.Name
	}
	return AfkDecision{
		ID: a.ID, RunID: a.Run.ID, BeltName: a.Run.BeltName, SubjectRef: a.Run.Subject.Ref(),
		SubjectTitle: a.Run.Subject.Title, StageID: a.Run.StageID, StageName: stageName,
		Situation: a.Situation.Kind, Reason: CopyPointer(a.Run.Reason), Key: a.Situation.Key,
		Status: AfkDecisionStatusDeciding, StartedAt: a.StartedAt, Folder: a.Folder, Reviewed: false,
	}
}

// NeedsReview reports whether it needs you to look at it: it did something, or
// gave up, and you have not checked it. An overruled decision leaves the review
// list; checking it does not clear the overrule.
func (d AfkDecision) NeedsReview() bool {
	return d.Overrule == nil && !d.Reviewed && d.Status != AfkDecisionStatusDeciding && d.Status != AfkDecisionStatusSuperseded
}
