package api

import (
	"reflect"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// BeltAfkControl is the belt's AFK start mode as its button shows it: the mode,
// the one a click moves to, whether AFK is on for new runs (always, or global
// while the global switch is on) and the button's label.
type BeltAfkControl struct {
	Mode  core.AfkMode `json:"mode"`
	Next  core.AfkMode `json:"next"`
	On    bool         `json:"on"`
	Label string       `json:"label"`
}

// NewBeltAfkControl builds the belt afk control from its arguments.
func NewBeltAfkControl(mode core.AfkMode, globalOn bool) BeltAfkControl {
	return BeltAfkControl{
		Mode:  mode,
		Next:  mode.Next(),
		On:    mode == core.AfkModeAlways || (mode == core.AfkModeGlobal && globalOn),
		Label: mode.Label(),
	}
}

// BeltProblem is a structural problem with the field it is about, for the save
// response. Warnings stay strings in the lists.
type BeltProblem struct {
	Message  string                `json:"message"`
	Location *core.ProblemLocation `json:"location,omitzero"`
}

// BeltSnapshot is one belt resource as the API returns it: the state, the commands the
// daemon would accept now, and an opaque revision token. The definition is Draft, the one
// the editor opens. Belt is the stored definition, sent only when it differs from the draft
// (when legacy stage keys were filled in); the stored definition is the draft otherwise.
// LastFired and Origin are written as explicit nulls when absent.
type BeltSnapshot struct {
	Belt              *core.Belt  `json:"belt,omitzero"`
	Paused            bool        `json:"paused"`
	Archived          bool        `json:"archived"`
	EffectivelyPaused bool        `json:"effectivelyPaused"`
	Created           jsonx.Date  `json:"created"`
	LastFired         *jsonx.Date `json:"lastFired"`
	// Actions are ordered, with an enabled flag and an explanation when disabled.
	Actions []core.BeltAction `json:"actions"`
	// Warnings are the saved version's warnings, in the order the daemon reports them.
	Warnings []string `json:"warnings"`
	// Revision is opaque, stable while the resource and its available actions
	// are unchanged; distinct from the belt's stored version.
	Revision string `json:"revision"`
	// Detail is the library row's detail line: the trigger's summary and the stage count.
	Detail string `json:"detail"`
	// Afk is the AFK button for the belt's new runs.
	Afk BeltAfkControl `json:"afk"`
	// Draft is the definition the editor opens: the stored belt with its legacy
	// stage keys filled from the pinned agents.
	Draft core.Belt `json:"draft"`
	// Origin is the library repository the belt was installed from, if any.
	Origin *core.LibraryOrigin `json:"origin"`
	// Shadow is the belt's running shadow test, omitted when none runs.
	Shadow *core.ShadowSummary `json:"shadow,omitzero"`
}

// NewBeltSnapshot builds the belt snapshot from its arguments.
// Draft is the stored belt and Origin is nil; the coordinator constructor below
// fills both from the daemon.
func NewBeltSnapshot(record store.BeltRecord, actions []core.BeltAction, revision string, warnings []string, globalAfkOn bool) BeltSnapshot {
	var lastFired *jsonx.Date
	if at := record.LastFired(); at != nil {
		lastFired = &jsonx.Date{Time: *at}
	}
	return BeltSnapshot{
		Paused:            record.Paused,
		Archived:          record.Archived,
		EffectivelyPaused: record.EffectivelyPaused(),
		Created:           jsonx.Date{Time: record.Created},
		LastFired:         lastFired,
		Actions:           append([]core.BeltAction{}, actions...),
		Warnings:          append([]string{}, warnings...),
		Revision:          revision,
		Detail:            core.BeltLibraryDetail(record.Belt),
		Afk:               NewBeltAfkControl(record.Belt.Afk, globalAfkOn),
		Draft:             record.Belt,
		Origin:            core.CopyPointer(record.Belt.Origin),
	}
}

// NewBeltSnapshotFromCoordinator builds the belt snapshot from coordinator from its arguments:
// the actions and revision the daemon computes, and the legacy-key-converted draft. The reads are one coordinator turn.
// Lock: none held by the caller; takes c.mu through ReadBeltSnapshot.
func NewBeltSnapshotFromCoordinator(record store.BeltRecord, c *coordinator.Coordinator) BeltSnapshot {
	return beltSnapshotFromReads(record, c.ReadBeltSnapshot(record))
}

// beltSnapshotFromReads builds a belt snapshot from the reads one turn returned.
func beltSnapshotFromReads(record store.BeltRecord, reads coordinator.BeltSnapshotReads) BeltSnapshot {
	context := reads.Context
	snapshot := NewBeltSnapshot(record, context.Actions, context.Revision, context.Warnings, reads.GlobalAfkOn)
	snapshot.Draft = reads.Draft
	snapshot.Shadow = reads.Shadow
	if !sameBeltWire(record.Belt, reads.Draft) {
		stored := record.Belt
		snapshot.Belt = &stored
	}
	return snapshot
}

// BeltsResponse is the body of GET /api/v1/belts. LiveIDs are the belts not archived,
// in list order. Browser is omitted when the library browser was not read.
type BeltsResponse struct {
	Belts   []BeltSnapshot       `json:"belts"`
	LiveIDs []string             `json:"liveIds"`
	Browser *core.LibraryBrowser `json:"browser,omitzero"`
}

// NewBeltsResponse builds the belts response from its arguments.
func NewBeltsResponse(belts []BeltSnapshot, liveIDs []string, browser *core.LibraryBrowser) BeltsResponse {
	return BeltsResponse{Belts: append([]BeltSnapshot{}, belts...), LiveIDs: append([]string{}, liveIDs...), Browser: core.CopyPointer(browser)}
}

// NewBeltsResponseFromCoordinator builds the belts response from coordinator from its arguments:
// the records, their snapshots and the browser are read in one coordinator turn.
// Lock: none held by the caller; takes c.mu through ReadBeltsReads.
func NewBeltsResponseFromCoordinator(c *coordinator.Coordinator, search, filter string) BeltsResponse {
	reads := c.ReadBeltsReads(search, filter)
	belts := make([]BeltSnapshot, 0, len(reads.Records))
	for i, record := range reads.Records {
		belts = append(belts, beltSnapshotFromReads(record, reads.Snapshots[i]))
	}
	return NewBeltsResponse(belts, reads.LiveIDs(), &reads.Browser)
}

// BeltDraftResponse is a streaming draft response: the complete updated draft,
// its ordered warnings, and placeholder help when the request supplied stage
// context.
type BeltDraftResponse struct {
	Draft    core.Belt `json:"draft"`
	Warnings []string  `json:"warnings"`
	// Actions are the actions the draft's editor may take, in the daemon's order
	// and with its reasons.
	Actions      []core.BeltAction     `json:"actions"`
	Placeholders *core.PlaceholderHelp `json:"placeholders,omitzero"`
	// Stash is set after `trigger-kind`: the stash the client keeps for this
	// entry, with the outgoing kind in it.
	Stash *core.TriggerStash `json:"stash,omitzero"`
}

// NewBeltDraftResponse builds the belt draft response from its arguments.
func NewBeltDraftResponse(draft core.Belt, warnings []string, actions []core.BeltAction, placeholders *core.PlaceholderHelp, stash *core.TriggerStash) BeltDraftResponse {
	return BeltDraftResponse{
		Draft:        draft,
		Warnings:     append([]string{}, warnings...),
		Actions:      append([]core.BeltAction{}, actions...),
		Placeholders: core.CopyPointer(placeholders),
		Stash:        core.CopyPointer(stash),
	}
}

// BeltResourceResponse is a saved belt command's response: the resulting
// resource and warnings, and the structural problems of the saved belt with the
// field each is about.
type BeltResourceResponse struct {
	Resource BeltSnapshot  `json:"resource"`
	Warnings []string      `json:"warnings"`
	Problems []BeltProblem `json:"problems"`
}

// NewBeltResourceResponse builds the belt resource response from its arguments.
func NewBeltResourceResponse(resource BeltSnapshot, warnings []string, problems []BeltProblem) BeltResourceResponse {
	return BeltResourceResponse{
		Resource: resource,
		Warnings: append([]string{}, warnings...),
		Problems: append([]BeltProblem{}, problems...),
	}
}

// NewBeltResourceResponseFromCoordinator builds the belt resource response from coordinator from its arguments.
// Problems come from the belt's structure, in the order the structure reports them.
// Lock: none held by the caller; each read takes c.mu in turn.
func NewBeltResourceResponseFromCoordinator(record store.BeltRecord, c *coordinator.Coordinator) BeltResourceResponse {
	context := c.BeltResourceContext(record, false)
	resource := NewBeltSnapshot(record, context.Actions, context.Revision, context.Warnings, false)
	resource.Shadow = c.ReadShadowSummary(record)
	located := core.BeltStructureLocated(record.Belt)
	problems := make([]BeltProblem, 0, len(located))
	for _, problem := range located {
		problems = append(problems, BeltProblem{Message: problem.Message, Location: core.CopyPointer(problem.Location)})
	}
	return NewBeltResourceResponse(resource, context.Warnings, problems)
}

// BeltContextRequest asks for the editor's context for a draft, with the stage
// whose placeholder help it wants, if any.
//
// Shadow says the draft is the belt's shadow, so the context names how its stages differ from live.
type BeltContextRequest struct {
	Draft   core.Belt `json:"draft" required:"true"`
	StageID *string   `json:"stageId,omitzero"`
	Shadow  bool      `json:"shadow,omitzero"`
}

// NewBeltContextRequest builds the belt context request from its arguments.
func NewBeltContextRequest(draft core.Belt, stageID *string) BeltContextRequest {
	return BeltContextRequest{Draft: draft, StageID: core.CopyPointer(stageID)}
}

// BeltStageTemplateRequest asks for a stage the daemon builds for a draft: Type
// is `agent`, `approve` or `watch`; Agent names the library agent an agent stage
// copies, and Route the route it joins. Index is where in the stage list the
// stage will go, so its defaults follow the stages before it; the end when absent.
type BeltStageTemplateRequest struct {
	Draft core.Belt      `json:"draft" required:"true"`
	Type  core.StageType `json:"type" required:"true"`
	Agent *core.AgentRef `json:"agent,omitzero"`
	Route *string        `json:"route,omitzero"`
	Index *int           `json:"index,omitzero"`
}

// NewBeltStageTemplateRequest builds the belt stage template request from its arguments.
func NewBeltStageTemplateRequest(draft core.Belt, stageType core.StageType, agent *core.AgentRef, route *string, index *int) BeltStageTemplateRequest {
	return BeltStageTemplateRequest{
		Draft: draft,
		Type:  stageType,
		Agent: core.CopyPointer(agent),
		Route: core.CopyPointer(route),
		Index: core.CopyPointer(index),
	}
}

// BeltStageTemplateResponse is the stage a template request built, ready to
// send as a `stage-add` or `feed-into` operand.
type BeltStageTemplateResponse struct {
	Stage core.Stage `json:"stage"`
}

// BeltDraftOnlyRequest is the full draft for copy and the update commands.
type BeltDraftOnlyRequest struct {
	Draft core.Belt `json:"draft" required:"true"`
}

// BeltUpdateStageRequest names one stage by its stable id, or by its list
// position for a client that only has the draft.
type BeltUpdateStageRequest struct {
	Draft   core.Belt `json:"draft" required:"true"`
	StageID *string   `json:"stageId,omitzero"`
	Index   *int      `json:"index,omitzero"`
}

// NewBeltUpdateStageRequest builds the belt update stage request from its arguments.
func NewBeltUpdateStageRequest(draft core.Belt, stageID *string, index *int) BeltUpdateStageRequest {
	return BeltUpdateStageRequest{Draft: draft, StageID: core.CopyPointer(stageID), Index: core.CopyPointer(index)}
}

// BeltSaveRequest is a full belt definition for save.
type BeltSaveRequest struct {
	Belt core.Belt `json:"belt" required:"true"`
}

// BeltPauseRequest takes an explicit value, including false for resume.
type BeltPauseRequest struct {
	Paused bool `json:"paused" required:"true"`
}

// BeltAfkRequest takes an explicit AFK mode.
type BeltAfkRequest struct {
	Afk core.AfkMode `json:"afk" required:"true"`
}

// PlaceholdersResponse is placeholder help for a saved belt, or a supplied draft.
type PlaceholdersResponse struct {
	Placeholders core.PlaceholderHelp `json:"placeholders"`
}

// BeltEmptyRequest is a command that needs no operands. An absent body, `{}`
// and an object with unknown keys are all accepted; malformed JSON is not.
type BeltEmptyRequest struct{}

// UnmarshalJSON accepts any JSON object and ignores its keys: the body must be an
// object (or absent, which the route handles), but unknown keys are not rejected.
func (r *BeltEmptyRequest) UnmarshalJSON(data []byte) error {
	var members map[string]json.RawMessage
	return json.Unmarshal(data, &members)
}

// sameBeltWire reports whether two belts have the same wire form, which is what a client compares:
// the stored definition and the draft are the same belt exactly when they encode alike.
// Lock: none (pure).
func sameBeltWire(a, b core.Belt) bool {
	// Equal values encode alike, and a stored belt's draft mostly equals it, so the two
	// encodings are compared only for belts that differ, perhaps only in a nil and empty list.
	if reflect.DeepEqual(a, b) {
		return true
	}
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}
