// RestartIntent, Join, Merge and
// Leg.

package core

import (
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// RestartIntentPhase is where an overrule restart is.
type RestartIntentPhase string

const (
	// RestartIntentPhaseIntent is saved, and the replacement session has not been queued yet.
	RestartIntentPhaseIntent RestartIntentPhase = "intent"
	// RestartIntentPhaseApplied means the run was queued at StageID.
	RestartIntentPhaseApplied RestartIntentPhase = "applied"
	// RestartIntentPhaseHeld means the restart could not be finished. The comments stay on the run.
	RestartIntentPhaseHeld RestartIntentPhase = "held"
)

// AllRestartIntentPhase lists every RestartIntentPhase in declaration order (CaseIterable).
var AllRestartIntentPhase = []RestartIntentPhase{RestartIntentPhaseIntent, RestartIntentPhaseApplied, RestartIntentPhaseHeld}

// RestartIntent is a durable request to restart a run at an agent stage after an
// AFK overrule. The run is the authority; the AFK log is a projection of it.
//
// Encoding writes decisionId, attemptId, stageId, comments, at, phase,
// and detail (omitted when nil).
type RestartIntent struct {
	DecisionID string `json:"decisionId"`
	// AttemptID is the attempt the decision ended.
	AttemptID string             `json:"attemptId"`
	StageID   string             `json:"stageId"`
	Comments  string             `json:"comments"`
	At        jsonx.Date         `json:"at"`
	Phase     RestartIntentPhase `json:"phase"`
	Detail    *string            `json:"detail,omitzero"`
}

// Join is one split a run is still inside. A fork nested in a route has its own
// join, whose parent is the split that contains it. When every leg has finished,
// the join merges their branches and continues once, unless a leg jumped past
// it, in which case the merge and the continuation are skipped.
type Join struct {
	ID      string   `json:"id"`
	ForkID  string   `json:"forkId"`
	Routes  []string `json:"routes"`
	Parent  *string  `json:"parent,omitzero"`
	Legs    []string `json:"legs"`
	Escaped bool     `json:"escaped"`
	// Into is the branch the routes merge into. Nil is the run's own branch.
	Into *string `json:"into,omitzero"`
	// Checkout is the leg whose checkout is already on Into.
	Checkout *string `json:"checkout,omitzero"`
	Joined   bool    `json:"joined"`
	// AfterStage is the stage after the fork, when the route has one. Nil ends the route.
	AfterStage  *string             `json:"afterStage,omitzero"`
	AfterRoutes map[string][]string `json:"afterRoutes"`
}

// Merge is a join's merge that waits while another join's merge is still under
// way. It holds what Run.mergeBranches, mergeInto, mergeCheckout, mergeLeg and
// mergeContext hold for the merge in progress.
type Merge struct {
	Branches []string `json:"branches"`
	Into     *string  `json:"into,omitzero"`
	Checkout *string  `json:"checkout,omitzero"`
	Leg      *string  `json:"leg,omitzero"`
	Context  *string  `json:"context,omitzero"`
}

// Leg is one of the routes a run works at the same time after a fork. The run
// is still one run over one subject: each leg has its own stage, checkout,
// sessions and branch, and the run's own stage, state and note speak for the
// leg in front. A leg is finished when it reaches Done, Ship or Drop.
//
// The optional keys are omitted when nil, and departJoins when empty.
type Leg struct {
	// ID is `main` on the line before a fork, and `r:<route>` for a route leg.
	ID string `json:"id"`
	// Route is the route this leg took at the fork it is in; nil on the line.
	Route *string `json:"route,omitzero"`
	// StageID is the stage the leg is at, or nil once it has finished.
	StageID *string  `json:"stageId,omitzero"`
	State   RunState `json:"state"`
	// Reason is why this leg is held or parked, or was dropped.
	Reason    *string `json:"reason,omitzero"`
	Note      *string `json:"note,omitzero"`
	Continues *string `json:"continues,omitzero"`
	Replays   *string `json:"replays,omitzero"`
	// Branch is the branch this leg's code work goes on.
	Branch *string `json:"branch,omitzero"`
	// Join is the split this leg belongs to, when several routes of a nested fork are in progress.
	Join *string `json:"join,omitzero"`
	// Depart is the stage this leg jumped to and has not started, while the routes it left are still finishing.
	Depart *string `json:"depart,omitzero"`
	// DepartJoins are the splits that jump is waiting on.
	DepartJoins []string `json:"departJoins,omitzero"`
}

// IsFinished reports whether the leg has finished: its state is no longer active.
func (l Leg) IsFinished() bool {
	return !l.State.IsActive()
}

// CanAbort reports whether Abort holds this leg where it is: queued or running
// agent work, a running watch, or an approval waiting for a decision.
func (l Leg) CanAbort(run Run) bool {
	stage := l.Stage(run)
	if stage == nil {
		return false
	}
	switch stage.Kind() {
	case StageKindAgent:
		return l.State == RunStateRunning || l.State == RunStateQueued
	case StageKindWatch:
		return l.State == RunStateRunning
	case StageKindGate:
		return l.State == RunStateWaiting
	}
	return false
}

// LegFromID is the leg id to save: the line before a fork has none, so `main`
// is nil.
func LegFromID(id *string) *string {
	if id == nil || *id == "main" {
		return nil
	}
	return id
}

// Stage is the stage the leg is at in run, or nil.
func (l Leg) Stage(run Run) *Stage {
	if l.StageID == nil {
		return nil
	}
	for i := range run.Stages {
		if run.Stages[i].ID == *l.StageID {
			stage := run.Stages[i].Clone()
			return &stage
		}
	}
	return nil
}

// RouteName is the route's name on the belt, for a leg that took one, or nil.
func (l Leg) RouteName(layout BeltLayout) *string {
	if l.Route == nil {
		return nil
	}
	route := layout.Route(*l.Route)
	if route == nil {
		return nil
	}
	name := route.Name
	return &name
}
