// BeltDraftRequest and
// its Operation, StageEdit and TriggerStash). The functions are in
// model_belt_draft_editing.go.
//
// Optional request fields are pointers (or nil slices: a nil field is absent.
// Fields that don't belong to the selected operation are
// ignored.

package core

// BeltDraftOperation is a draft operation the editor supports. Fork-move,
// fork-update and route-move are deliberately not among them.
type BeltDraftOperation string

const (
	BeltDraftOperationStageAdd         BeltDraftOperation = "stage-add"
	BeltDraftOperationStageRemove      BeltDraftOperation = "stage-remove"
	BeltDraftOperationStageMove        BeltDraftOperation = "stage-move"
	BeltDraftOperationStageUpdate      BeltDraftOperation = "stage-update"
	BeltDraftOperationForkAdd          BeltDraftOperation = "fork-add"
	BeltDraftOperationForkRemove       BeltDraftOperation = "fork-remove"
	BeltDraftOperationRouteAdd         BeltDraftOperation = "route-add"
	BeltDraftOperationRouteRemove      BeltDraftOperation = "route-remove"
	BeltDraftOperationRouteUpdate      BeltDraftOperation = "route-update"
	BeltDraftOperationTriggerAdd       BeltDraftOperation = "trigger-add"
	BeltDraftOperationTriggerRemove    BeltDraftOperation = "trigger-remove"
	BeltDraftOperationTriggerMove      BeltDraftOperation = "trigger-move"
	BeltDraftOperationTriggerUpdate    BeltDraftOperation = "trigger-update"
	BeltDraftOperationTriggerKind      BeltDraftOperation = "trigger-kind"
	BeltDraftOperationRepositoryAdd    BeltDraftOperation = "repository-add"
	BeltDraftOperationRepositoryRemove BeltDraftOperation = "repository-remove"
	BeltDraftOperationFeedInto         BeltDraftOperation = "feed-into"
	BeltDraftOperationPoliciesUpdate   BeltDraftOperation = "policies-update"
	BeltDraftOperationGotoPick         BeltDraftOperation = "goto-pick"
	BeltDraftOperationGotoRoute        BeltDraftOperation = "goto-route"
)

// AllBeltDraftOperation lists every BeltDraftOperation in declaration order (CaseIterable).
var AllBeltDraftOperation = []BeltDraftOperation{
	BeltDraftOperationStageAdd, BeltDraftOperationStageRemove, BeltDraftOperationStageMove, BeltDraftOperationStageUpdate,
	BeltDraftOperationForkAdd, BeltDraftOperationForkRemove, BeltDraftOperationRouteAdd, BeltDraftOperationRouteRemove,
	BeltDraftOperationRouteUpdate, BeltDraftOperationTriggerAdd, BeltDraftOperationTriggerRemove, BeltDraftOperationTriggerMove,
	BeltDraftOperationTriggerUpdate, BeltDraftOperationTriggerKind, BeltDraftOperationRepositoryAdd, BeltDraftOperationRepositoryRemove,
	BeltDraftOperationFeedInto, BeltDraftOperationPoliciesUpdate, BeltDraftOperationGotoPick, BeltDraftOperationGotoRoute,
}

// Known reports whether x is one of the cases of BeltDraftOperation.
func (x BeltDraftOperation) Known() bool { return enumKnown(x, AllBeltDraftOperation) }

// BeltDraftRequest is one structural edit to a belt draft. Every request carries
// the complete draft it was made against, so a client's unsaved input is never
// held by the daemon and two clients cannot overwrite each other's editor state.
// Fields that don't belong to the selected operation are ignored, but a
// structural argument that names something the draft doesn't have is a
// ValidationFailure with an ordered problem list and no mutation.
//
// Encoding: draft, op, and each optional field omitted when nil.
// The JSON keys are the names of the fields.
type BeltDraftRequest struct {
	Draft Belt               `json:"draft" required:"true"`
	Op    BeltDraftOperation `json:"op" required:"true"`
	// Stage is the stage-add stage, fully formed (defaults are the caller's), and the
	// feed-into stage. Also the replacement of stage-update.
	Stage *Stage `json:"stage,omitzero"`
	// StageID is the stage-remove, stage-update, stage-move, fork-add and feed-into stage.
	StageID *string `json:"stageId,omitzero"`
	// Index is where in the list stage-add puts the stage.
	Index *int `json:"index,omitzero"`
	// Route is the route the stage belongs to (stage-add, stage-move).
	Route *string `json:"route,omitzero"`
	// Detach, for stage-move, true takes the stage off its fork onto the line.
	Detach *bool `json:"detach,omitzero"`
	// TargetID, for stage-move, drags the stage onto this stage's position and route.
	TargetID *string `json:"targetId,omitzero"`
	// Step, for stage-move, shifts the stage earlier (-1) or later (1).
	Step *int `json:"step,omitzero"`
	// Update holds the fields stage-update changes.
	Update *StageEdit `json:"update,omitzero"`
	// ForkID is the fork fork-remove removes, or the fork route-add adds a route to.
	ForkID *string `json:"forkId,omitzero"`
	// RouteID is the route route-remove or route-update removes or renames, and goto-route's route.
	RouteID *string `json:"routeId,omitzero"`
	// RouteName is the new name for route-update.
	RouteName *string `json:"routeName,omitzero"`
	// Entry is the entry trigger-add appends; trigger-update's repositories and id.
	Entry *TriggerEntry `json:"entry,omitzero"`
	// EntryID is the entry trigger-remove, trigger-move, trigger-update,
	// trigger-kind, repository-add and repository-remove change.
	EntryID *string `json:"entryId,omitzero"`
	// Offset, for trigger-move, is one step earlier (-1) or later (1).
	Offset *int `json:"offset,omitzero"`
	// Trigger is trigger-update's new kind settings (and the entry's repositories).
	Trigger *Trigger `json:"trigger,omitzero"`
	// Kind is the kind trigger-kind switches to.
	Kind *TriggerKind `json:"kind,omitzero"`
	// Repository is the value repository-add or repository-remove takes.
	Repository *string `json:"repository,omitzero"`
	// Stash is trigger-kind's kinds already entered for this entry during the
	// editing session. Never persisted.
	Stash *TriggerStash `json:"stash,omitzero"`
	// HelpFor is the stage whose placeholder help the response should carry, if any.
	HelpFor *string `json:"helpFor,omitzero"`
	// Policies is policies-update's belt policies, replaced as a whole.
	Policies *Policies `json:"policies,omitzero"`
	// Feeds, for route-add, true adds a feeding route ("+ Feed") rather than a line-route.
	Feeds *bool `json:"feeds,omitzero"`
	// OutcomeID is the outcome whose goto goto-pick and goto-route change.
	OutcomeID *string `json:"outcomeId,omitzero"`
	// GotoTarget is the stage the goto names; empty clears it.
	GotoTarget *string `json:"gotoTarget,omitzero"`
}

// StageEdit holds the fields a stage-update may change. An absent (nil) field is
// left alone; an explicit Watch is a watch-target change with the editor's reset rules.
type StageEdit struct {
	Name      *string      `json:"name,omitzero"`
	Brief     *string      `json:"brief,omitzero"`
	Writes    *string      `json:"writes,omitzero"`
	Key       *string      `json:"key,omitzero"`
	Runner    *Backend     `json:"runner,omitzero"`
	Model     *string      `json:"model,omitzero"`
	Thinking  *Thinking    `json:"thinking,omitzero"`
	Workspace *Workspace   `json:"workspace,omitzero"`
	Slots     *int         `json:"slots,omitzero"`
	Fallback  *Fallback    `json:"fallback,omitzero"`
	Agent     *AgentRef    `json:"agent,omitzero"`
	Watch     *WatchTarget `json:"watch,omitzero"`
	// Outcomes replaces the outcomes as a whole when non-nil.
	Outcomes []Outcome `json:"outcomes,omitzero"`
	// Alternates replaces the other runner, model and thinking settings the stage
	// spreads its sessions across, as a whole, when non-nil.
	Alternates []SessionSettings `json:"alternates,omitzero"`
	// ClearThinking, when true, clears the stage's thinking level, so the runner's default applies.
	ClearThinking *bool `json:"clearThinking,omitzero"`
}

// TriggerStash is what one entry was set to for each kind during an editing
// session. The daemon never stores this: a client sends it with trigger-kind and
// gets the restored or fresh kind back in the updated draft.
type TriggerStash struct {
	Entry string `json:"entry"`
	// Triggers maps a kind raw value to the trigger it was set to.
	Triggers map[string]Trigger `json:"triggers"`
}

// Trigger returns the trigger stored for kind, or nil.
func (s TriggerStash) Trigger(kind TriggerKind) *Trigger {
	t, ok := s.Triggers[string(kind)]
	if !ok {
		return nil
	}
	c := t.Clone()
	return &c
}

// Put stores trigger for kind.
func (s *TriggerStash) Put(kind TriggerKind, trigger Trigger) {
	if s.Triggers == nil {
		s.Triggers = map[string]Trigger{}
	}
	s.Triggers[string(kind)] = trigger
}

// Clone is a deep copy of the stash: its entry and the trigger for each kind.
func (s TriggerStash) Clone() TriggerStash {
	return cloneValue(s)
}
