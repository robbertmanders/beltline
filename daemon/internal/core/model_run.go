// RunState, and Run's stored
// fields, its initialisers, its id and folder, and its AFK and routes.
// The Codable conformance is in model_run_codec.go and the accessors in
// model_run_access.go.
//
// Dates are jsonx.Date. The subject is saved under the key "issue".

package core

import (
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// RunState is where a run is.
type RunState string

const (
	RunStateQueued  RunState = "queued"
	RunStateRunning RunState = "running"
	RunStateWaiting RunState = "waiting"
	RunStateHeld    RunState = "held"
	RunStateParked  RunState = "parked"
	// RunStateDone is a run that finished successfully without shipping: a PR review, say.
	RunStateDone RunState = "done"
	// RunStateShipped is a run that finished successfully through a Ship outcome.
	RunStateShipped RunState = "shipped"
	RunStateDropped RunState = "dropped"
)

// AllRunState lists every RunState in declaration order (CaseIterable).
var AllRunState = []RunState{RunStateQueued, RunStateRunning, RunStateWaiting, RunStateHeld, RunStateParked, RunStateDone, RunStateShipped, RunStateDropped}

// IsActive reports whether the run is still going: not done, shipped or dropped.
func (s RunState) IsActive() bool {
	return s != RunStateDone && s != RunStateShipped && s != RunStateDropped
}

// Completed reports whether the run finished successfully, as Done or Shipped.
func (s RunState) Completed() bool {
	return s == RunStateDone || s == RunStateShipped
}

// NeedsYou reports whether the run waits for a person: waiting or held.
func (s RunState) NeedsYou() bool {
	return s == RunStateWaiting || s == RunStateHeld
}

// Run is one subject's trip through one belt version.
//
// The belt version's stages and policies are copied in, so saving the belt
// never changes this run.
type Run struct {
	ID          string `json:"id"`
	BeltID      string `json:"beltId"`
	BeltName    string `json:"beltName"`
	BeltVersion int    `json:"beltVersion"`
	// Shadow is set on a run that started during a shadow test of its belt, on
	// either version. BeltVersion is the version it started on and keeps.
	Shadow *RunShadow `json:"shadow,omitzero"`
	// Stages and Forks are the belt version's stages and forks, copied when the
	// run starts.
	Stages []Stage `json:"stages"`
	Forks  []Fork  `json:"forks"`
	// Adjustments are the stage adjustments made on this run.
	Adjustments []Adjustment `json:"adjustments"`
	Policies    Policies     `json:"policies"`
	// Base is the branch work starts from when the run started: the subject's
	// base: label when it had one, otherwise the belt's shared base branch; empty
	// means the repository's default. It is kept as it was before overrides
	// existed, so runs saved without them keep their base.
	Base string `json:"base"`
	// BaseFromLabel is true when Base is the subject's base: label, which outranks
	// the overrides below; when absent, Base is the shared base and an override wins.
	BaseFromLabel *bool `json:"baseFromLabel,omitzero"`
	// RepositoryBases are the belt's per-repository base branches when the run
	// started, copied so later belt edits leave them alone. Nil when there are none.
	RepositoryBases map[string]string `json:"repositoryBases,omitzero"`
	// Repos are the repositories frozen when the run starts.
	Repos []string `json:"repos"`
	// Source is the trigger entry that admitted the run, with the settings it had
	// then. Nil on a run saved before a belt could have more than one trigger.
	Source  *RunSource `json:"source,omitzero"`
	Subject Subject    `json:"issue"`
	// Task is the prompt you wrote at a prompt stage: what the run is for. Every
	// brief after it carries it. Nil when the run has had no prompt stage.
	Task *string `json:"task,omitzero"`
	// Sharing is whether agents see the details of the work item the run came
	// from. Only withheld withholds them: empty is a run that never chose, as
	// every run before prompt stages.
	Sharing SourceSharing `json:"sourceSharing,omitzero"`
	State   RunState      `json:"state"`
	StageID string        `json:"stageId"`
	// Reason is why the run is held or parked, or why it was dropped.
	Reason *string `json:"reason,omitzero"`
	// Shipping is set when an outcome of this trip shipped the work: a Ship outcome
	// finished a route. A successful completion is Shipped while it is set, and Done
	// otherwise. It is cleared when the run starts work again after it finished.
	Shipping bool `json:"shipping,omitzero"`
	// Note is the note waiting for the next attempt at the current stage.
	Note *string `json:"note,omitzero"`
	// Continues is the attempt whose session the next attempt at the current
	// stage continues, instead of starting a new one.
	Continues *string `json:"continues,omitzero"`
	// Replays is the attempt whose context the next fresh attempt at this stage starts from.
	Replays *string `json:"replays,omitzero"`
	// Uses are the uses of each outcome that has a max, keyed "<stage>:<outcome>".
	Uses map[string]int `json:"uses"`
	// Taken holds the routes the run took at each fork it has passed, saved under
	// "routes"; a fork with one route saves as a bare string.
	Taken RouteSet `json:"routes"`
	// Legs are the routes the run works at the same time. One leg per taken
	// route after a fork, one leg on the line before one; empty on older runs,
	// which read as a single leg where they are.
	Legs []Leg `json:"legs"`
	// MergeBranches are the leg branches to merge into the run's branch before
	// the stage where the routes meet, and that has not happened yet.
	MergeBranches []string `json:"mergeBranches"`
	// MergeInto is the branch an inner join merges into. Nil merges into the run's own branch.
	MergeInto *string `json:"mergeInto,omitzero"`
	// MergeCheckout is the leg whose checkout already has MergeInto.
	MergeCheckout *string `json:"mergeCheckout,omitzero"`
	// MergeLeg is the leg that is waiting on the merge and is held if it conflicts.
	MergeLeg *string `json:"mergeLeg,omitzero"`
	// MergeContext names which routes are being merged, and into what, when a conflict is reported.
	MergeContext *string `json:"mergeContext,omitzero"`
	// MergesWaiting are the joins whose merge waits for the one in progress, oldest first.
	MergesWaiting []Merge `json:"mergesWaiting"`
	// MergeNote is what Beltline merged at the join, which the join stage's attempt shows.
	MergeNote *string `json:"mergeNote,omitzero"`
	// MergeNoteLeg is the leg whose next session shows MergeNote. Nil is whichever starts first.
	MergeNoteLeg *string `json:"mergeNoteLeg,omitzero"`
	// Joins are the splits still open, including a fork nested inside a route. Empty on a flat run.
	Joins    []Join    `json:"joins"`
	Attempts []Attempt `json:"attempts"`
	Branch   *string   `json:"branch,omitzero"`
	// PullRequests are the pull requests found by a watch, with their repository and latest state.
	PullRequests []PullRequest `json:"pullRequests"`
	// FirstPullRequestAt is persisted so the missing-repository grace survives a restart.
	FirstPullRequestAt *jsonx.Date `json:"firstPullRequestAt,omitzero"`
	// Agents are the agent versions the stages use, copied when the run starts,
	// as its stages are. Runs saved before this read their agents from the library.
	Agents []AgentSnapshot `json:"agents"`
	// References says how briefs name stage output. Nil, on a run saved before
	// stage keys, keeps agent-key and stage-id aliases.
	References *int `json:"references,omitzero"`
	// ModelAdmission is nil only for pre-feature files. Invalid present metadata fails closed.
	ModelAdmission *ModelAdmission `json:"modelAdmission,omitzero"`
	// WatchObservations are what each watch stage last saw, by stage.
	WatchObservations map[string]WatchObservation `json:"watchObservations"`
	// TicketObservations are what each ticket watch stage last reported, by stage.
	// Kept when the run leaves the watch, so a change while it was away still counts.
	TicketObservations map[string]TicketObservation `json:"ticketObservations"`
	// AfkEnabled is nil to follow the global AFK switch on a belt that allows AFK
	// decisions. True or false is this run's own choice until the next global AFK change.
	AfkEnabled *bool `json:"afkEnabled,omitzero"`
	// Generation distinguishes a run from an erased one that reused this id. Late
	// work from the erased run carries the generation it started with.
	Generation int `json:"generation"`
	// Operation is bumped when an overrule restarts the run, so work that started
	// before the restart cannot move it.
	Operation int `json:"operation"`
	// Restart is the overrule restart in progress or last applied. Absent on older runs.
	Restart *RestartIntent `json:"restart,omitzero"`
	// Pause is the whole-run pause. Absent on runs saved before pausing, and after
	// Resume admits the run.
	Pause *RunPause `json:"pause,omitzero"`
	// PauseTime are the pauses that already ended, so a watch can leave that time out of its grace.
	PauseTime []RunPauseInterval `json:"pauseTime"`
	// Parking is set while the Run page button has parked the whole run. Absent on older runs.
	Parking   *Parking   `json:"parking,omitzero"`
	CreatedAt jsonx.Date `json:"createdAt"`
	UpdatedAt jsonx.Date `json:"updatedAt"`
}

// RunArgs are the parameters of NewRun.
type RunArgs struct {
	Belt    Belt
	Subject Subject
	Now     time.Time
	// Entry is nil for the belt's only entry, when it has one.
	Entry *TriggerEntry
}

// NewRun starts a run of belt for subject at now, copying the belt's stages,
// forks, policies and base, its repositories (all of them for a Jira subject,
// otherwise the subject's own), the trigger entry, the first stage and leg, and
// the AFK choice the belt's mode gives.
func NewRun(a RunArgs) Run {
	belt := a.Belt
	subject := a.Subject.Clone()
	entry := a.Entry
	if entry == nil && len(belt.Triggers) == 1 {
		only := belt.Triggers[0].Clone()
		entry = &only
	}
	// A `base:` label sets the branch work starts from and merges into; it is
	// copied here as it always was, so later label edits leave the run alone.
	base := belt.Base
	var fromLabel *bool
	if hint := subject.BaseHint(); hint != nil {
		base = *hint
		fromLabel = new(true)
	}
	repositories := belt.Repos()
	if entry != nil {
		repositories = entry.Repos
	}
	repos := []string{subject.Repo}
	if subject.Kind == SubjectKindJira {
		repos = repositories
	}
	first := belt.Layout().First()
	run := Run{
		ID:              RunID(belt.ID, subject),
		BeltID:          belt.ID,
		BeltName:        belt.Name,
		BeltVersion:     belt.Version,
		Stages:          cloneEach(belt.Stages, Stage.Clone),
		Forks:           cloneEach(belt.Forks, Fork.Clone),
		Policies:        belt.Policies,
		Base:            base,
		BaseFromLabel:   fromLabel,
		RepositoryBases: RepositoryBasesClean(belt.RepositoryBases),
		Repos:           cloneStrings(repos),
		Subject:         subject,
		State:           RunStateQueued,
		Legs:            []Leg{{ID: "main", StageID: CopyPointer(first.Stage), State: RunStateQueued}},
		Attempts:        []Attempt{},
		References:      new(StageKeysVersion),
		// A belt that always or never starts runs in AFK mode decides, whatever the global switch.
		AfkEnabled: belt.Afk.RunChoice(),
		CreatedAt:  jsonx.Date{Time: a.Now},
		UpdatedAt:  jsonx.Date{Time: a.Now},
	}
	run.fillEmptyMaps()
	if first.Stage != nil {
		run.StageID = *first.Stage
	}
	if entry != nil {
		run.Source = &RunSource{Entry: entry.ID, Trigger: entry.Trigger.Clone(), Repos: cloneStrings(entry.Repos)}
	}
	return run
}

// fillEmptyMaps gives each of the run's dictionaries an empty map where it has
// none: a nil map here would encode as
// null and read as absent. Used by NewRun and by decoding.
func (r *Run) fillEmptyMaps() {
	if r.Taken.Routes == nil {
		r.Taken.Routes = map[string][]string{}
	}
	if r.Uses == nil {
		r.Uses = map[string]int{}
	}
	if r.WatchObservations == nil {
		r.WatchObservations = map[string]WatchObservation{}
	}
	if r.TicketObservations == nil {
		r.TicketObservations = map[string]TicketObservation{}
	}
}

// RunID is the id of a run of the belt with beltID for subject. Issues keep the
// id they had before there were other triggers.
func RunID(beltID string, subject Subject) string {
	switch subject.Kind {
	case SubjectKindIssue:
		return beltID + ":" + subject.Repo + "#" + strconv.Itoa(subject.Number)
	case SubjectKindPullRequest:
		return beltID + ":" + subject.Repo + "#" + strconv.Itoa(subject.Number) + "@" + ValueOf(subject.Occurrence)
	case SubjectKindJira, SubjectKindManual, SubjectKindScheduled:
		return beltID + ":" + string(subject.Kind) + ":" + subject.Key
	}
	return ""
}

// Layout is the run's stages and forks, read as a belt layout.
func (r Run) Layout() BeltLayout {
	return BeltLayout{Stages: r.Stages, Forks: r.Forks}
}

// Routes are the routes the run took at each fork it has passed.
func (r Run) Routes() map[string][]string {
	return r.Taken.Clone().Routes
}

// SetRoutes replaces the routes taken.
func (r *Run) SetRoutes(routes map[string][]string) {
	r.Taken = RouteSet{Routes: cloneValue(routes)}
}

// PullRequest is the first pull request, for callers that show one.
func (r Run) PullRequest() *PullRequest {
	if len(r.PullRequests) == 0 {
		return nil
	}
	first := r.PullRequests[0]
	return &first
}

// SetPullRequest replaces the pull requests with one (or none for nil), filling
// in its repository from the subject when it has none.
func (r *Run) SetPullRequest(pull *PullRequest) {
	if pull == nil {
		r.PullRequests = []PullRequest{}
		return
	}
	copied := *pull
	if copied.Repo == "" {
		copied.Repo = r.Subject.Repo
	}
	r.PullRequests = []PullRequest{copied}
}

// AfkEffective reports whether the AFK assistant may decide for this run. An
// explicit choice wins; otherwise the global switch.
func (r Run) AfkEffective(global bool) bool {
	if r.AfkEnabled != nil {
		return *r.AfkEnabled
	}
	return global
}

// Afk is the run's AFK choice as a mode: always, never, or following the global
// switch.
func (r Run) Afk() AfkMode {
	return AfkModeFromChoice(r.AfkEnabled)
}

// SetAfk saves the mode as the run's choice.
func (r *Run) SetAfk(mode AfkMode) {
	r.AfkEnabled = mode.RunChoice()
}

// UsesStageKeys reports whether the run was started with stage keys. Older saved
// runs keep their aliases.
func (r Run) UsesStageKeys() bool {
	return r.References != nil && *r.References == StageKeysVersion
}

// IsSplit reports whether several routes are being worked at once.
func (r Run) IsSplit() bool {
	return len(r.Legs) > 1
}

// IsButtonParked reports whether the Run page button parked this run, so Unpark
// restores it directly.
func (r Run) IsButtonParked() bool {
	return r.Parking != nil
}

// Clone is a deep copy of the run: its stages, forks, legs, attempts, pull
// requests, observations, pause and every other list or map, and the routes it
// took, so changing the copy never changes the run.
// WithholdsSource reports whether the run's agents see only the reference of the
// work item it came from, not its details.
func (r Run) WithholdsSource() bool { return r.Sharing == SourceSharingWithheld }

func (r Run) Clone() Run {
	c := cloneValue(r)
	c.Taken = r.Taken.Clone()
	return c
}
