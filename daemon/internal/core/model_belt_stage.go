// StageType, Backend and Stage;
// the rest of the file is in model_belt.go.

package core

// StageType is the type of a stage: an agent, an approval, a prompt or a watch.
type StageType string

const (
	// StageTypeAgent is an agent stage.
	StageTypeAgent StageType = "agent"
	// StageTypeApprove is an approval stage.
	StageTypeApprove StageType = "approve"
	// StageTypePrompt is a prompt stage: you write the run's task before any agent works on it.
	StageTypePrompt StageType = "prompt"
	// StageTypeWatch is a watch stage.
	StageTypeWatch StageType = "watch"
)

// AllStageType lists every StageType in declaration order (CaseIterable).
var AllStageType = []StageType{StageTypeAgent, StageTypeApprove, StageTypePrompt, StageTypeWatch}

// Known reports whether x is one of the cases of StageType.
func (x StageType) Known() bool { return enumKnown(x, AllStageType) }

// StageTypeLegacyAgents are the stage types saved before agents had their own
// files. Each named the built-in agent of the same id, which is now that
// agent's version 1.
var StageTypeLegacyAgents = map[string]struct{}{
	"triage": {}, "spec": {}, "build": {}, "verify": {}, "review": {}, "custom": {},
}

// Kind is the kind of stage this type makes.
func (t StageType) Kind() StageKind {
	switch t {
	case StageTypeAgent:
		return StageKindAgent
	case StageTypeApprove, StageTypePrompt:
		return StageKindGate
	case StageTypeWatch:
		return StageKindWatch
	}
	return ""
}

// Title is the name a new stage of this type gets.
func (t StageType) Title() string {
	switch t {
	case StageTypeAgent:
		return "Agent"
	case StageTypeApprove:
		return "Approval"
	case StageTypePrompt:
		return "Prompt"
	case StageTypeWatch:
		return "Watcher"
	}
	return ""
}

// Output is what a stage of this type produces; an agent stage's is its own
// file.
func (t StageType) Output() string {
	switch t {
	case StageTypeAgent:
		return "output.md"
	case StageTypeApprove:
		return "decision"
	case StageTypePrompt:
		return "task"
	case StageTypeWatch:
		return "pull request"
	}
	return ""
}

// Backend is the agent CLI an agent stage runs.
type Backend string

const (
	// BackendCopilot is the Copilot CLI.
	BackendCopilot Backend = "copilot"
	// BackendPi is Pi.
	BackendPi Backend = "pi"
	// BackendClaude is Claude Code.
	BackendClaude Backend = "claude"
	// BackendCodex is the Codex CLI.
	BackendCodex Backend = "codex"
	// BackendCursor is the Cursor CLI.
	BackendCursor Backend = "cursor"
)

// AllBackend lists every Backend in declaration order (CaseIterable).
var AllBackend = []Backend{BackendCopilot, BackendPi, BackendClaude, BackendCodex, BackendCursor}

// Known reports whether x is one of the cases of Backend.
func (x Backend) Known() bool { return enumKnown(x, AllBackend) }

// Label is the name shown for the backend.
func (b Backend) Label() string {
	switch b {
	case BackendCopilot:
		return "Copilot CLI"
	case BackendPi:
		return "Pi"
	case BackendClaude:
		return "Claude Code"
	case BackendCodex:
		return "Codex CLI"
	case BackendCursor:
		return "Cursor CLI"
	}
	return ""
}

// FinishesOnHandoff reports whether an interactive launch on the backend ends
// when the agent writes its handoff record. Copilot's autopilot keeps working
// over several turns until it records its own task completion, so a Copilot
// launch finishes on that instead and its brief has no handoff.
func (b Backend) FinishesOnHandoff() bool { return b != BackendCopilot }

// SupportedThinking is the thinking levels the CLI takes, shared by validation
// and adapters. An empty set means the runner has no thinking setting and
// ignores it. Cursor takes a level through the model variant its catalog lists
// for it (ModelChoice.identifier(thinking:)), not a flag.
func (b Backend) SupportedThinking() map[Thinking]struct{} {
	set := make(map[Thinking]struct{}, len(AllThinking))
	switch b {
	case BackendCopilot, BackendPi, BackendClaude, BackendCodex, BackendCursor:
		for _, t := range AllThinking {
			set[t] = struct{}{}
		}
	}
	return set
}

// ThinkingNote is why Thinking is off for a runner that has no thinking
// setting, or nil when it has one.
func (b Backend) ThinkingNote() *string {
	if len(b.SupportedThinking()) != 0 {
		return nil
	}
	note := b.Label() + " uses the model's reasoning settings; Thinking is not passed."
	return &note
}

// Thinking is the level passed for thinking: the highest the CLI takes at or
// below it, for a setting that reached a run without passing Save. Nil when
// thinking is nil or unsupported.
func (b Backend) Thinking(thinking *Thinking) *Thinking {
	if thinking == nil {
		return nil
	}
	index := -1
	for i, t := range AllThinking {
		if t == *thinking {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	supported := b.SupportedThinking()
	for i := index; i >= 0; i-- {
		if _, ok := supported[AllThinking[i]]; ok {
			t := AllThinking[i]
			return &t
		}
	}
	return nil
}

// Executable is the command to run, unless BELTLINE_<BACKEND> names another
// path. The environment is a parameter: core never reads it.
func (b Backend) Executable(env func(string) string) string {
	key := "BELTLINE_" + UppercasedFull(string(b))
	if path := env(key); path != "" {
		return path
	}
	if b == BackendCursor {
		return "cursor-agent"
	}
	return string(b)
}

// Stage is one step of a belt: an agent session, an approval or a watch, with
// the outcomes that decide what comes next.
//
// The encoded keys are id, type, name, agent, key, runner, model, thinking,
// alternates (only when non-empty), workspace, writes, slots, brief, outcomes,
// fallback, route and watch. `writes` is not a stored field: it is Writes(), which
// MarshalJSON writes. Decoding also accepts the legacy stage shapes (see
// UnmarshalJSON).
type Stage struct {
	ID   string    `json:"id"`
	Type StageType `json:"type"`
	Name string    `json:"name"`
	// Agent is the library agent an agent stage uses, at the version the belt
	// was saved with.
	Agent *AgentRef `json:"agent,omitzero"`
	// Key is the placeholder briefs use for this stage's output. Nil on a belt
	// saved before stages had their own key; an explicit conversion uses the
	// originally pinned agent's legacy key before the next save or start. A pin
	// update does not replace a chosen key. Choosing one also names the stage's
	// file (Writes).
	//
	// Changing the key clears savedWrites, so code that changes
	// Key must call SetKey, which does the same.
	Key *string `json:"key,omitzero"`
	// Runner is the agent CLI, for agent stages.
	Runner *Backend `json:"runner,omitzero"`
	// Model is the model the runner is given.
	Model string `json:"model"`
	// Thinking is nil for the runner's default.
	Thinking *Thinking `json:"thinking,omitzero"`
	// Alternates are other runner, model and thinking settings this stage
	// spreads its fresh sessions across, besides its own, to compare them
	// (ModelPool). Encoded only when non-empty.
	Alternates []SessionSettings `json:"alternates,omitzero"`
	// Workspace says where an agent stage works.
	Workspace Workspace `json:"workspace"`
	// savedWrites is a file name saved apart from the key, from before it
	// followed the key. Not encoded itself: Writes() is encoded as `writes`.
	savedWrites *string
	// Slots is how many runs may be at this stage's agent at once.
	Slots int `json:"slots"`
	// Brief is the brief; empty means the agent's own.
	Brief string `json:"brief"`
	// Outcomes are the outcomes the stage can end with.
	Outcomes []Outcome `json:"outcomes"`
	// Fallback is what happens when the session fails or ends without a known
	// outcome. Approvals and watches have no fallback.
	Fallback Fallback `json:"fallback"`
	// Route is the fork route this stage is on, if any.
	Route *string `json:"route,omitzero"`
	// Watch is what a watch stage waits on; nil is the pull request, as before
	// watches could wait on anything else.
	Watch *WatchTarget `json:"watch,omitzero"`
	// Source is what a prompt stage's source checkbox starts as on each run; nil
	// is withheld. Only prompt stages keep it.
	Source *SourceSharing `json:"source,omitzero"`
}

// Kind is the kind of the stage's type.
func (s Stage) Kind() StageKind { return s.Type.Kind() }

// SourceSharing is what a prompt stage's source checkbox starts as: its Source,
// or withheld. Nil when the stage is not a prompt stage.
func (s Stage) SourceSharing() *SourceSharing {
	if s.Type != StageTypePrompt {
		return nil
	}
	return new(ValueOr(s.Source, SourceSharingWithheld))
}

// NeedsNote reports whether picking outcome at this gate needs written text: a
// prompt stage's outcomes that move the run on take the prompt, and any other
// outcome needs a note when it passes it on (withOutput).
func (s Stage) NeedsNote(outcome Outcome) bool {
	if s.Type == StageTypePrompt {
		return outcome.Then.Action.MovesOn()
	}
	return outcome.Then.WithOutput
}

// WatchesTicket reports whether a watch stage waits on the run's own ticket.
func (s Stage) WatchesTicket() bool {
	target := s.WatchTarget()
	return target != nil && *target == WatchTargetTicket
}

// WatchTarget is what a watch stage waits on, or nil when the stage is not a
// watch. A watch without a target waits on the pull
// request.
func (s Stage) WatchTarget() *WatchTarget {
	if s.Kind() != StageKindWatch {
		return nil
	}
	if s.Watch != nil {
		return CopyPointer(s.Watch)
	}
	pr := WatchTargetPullRequest
	return &pr
}

// StageFile is the file a stage with key writes, given its type.
func StageFile(key *string, typ StageType) string {
	if typ.Kind() != StageKindAgent || key == nil || *key == "" {
		return typ.Output()
	}
	return *key + ".md"
}

// Writes is the file the agent writes: its key with `.md`, unless the stage
// was saved with a file of its own.
func (s Stage) Writes() string {
	if s.savedWrites != nil {
		return *s.savedWrites
	}
	return StageFile(s.Key, s.Type)
}

// SetWrites names the stage's file; a name equal to the one the key gives
// clears the saved name.
func (s *Stage) SetWrites(name string) {
	if name == StageFile(s.Key, s.Type) {
		s.savedWrites = nil
		return
	}
	saved := name
	s.savedWrites = &saved
}

// SetKey changes the key and clears savedWrites, as Stage.key's didSet does.
func (s *Stage) SetKey(key *string) {
	s.Key = key
	s.savedWrites = nil
}

// Pool is the settings a fresh session here can start with: its own first,
// then its alternates.
func (s Stage) Pool() []SessionSettings {
	runner := ValueOr(s.Runner, BackendPi)
	return ModelPoolEntries(runner, s.Model, s.Thinking, s.Alternates)
}

// ChangesCode reports whether this stage changes the code: an agent working in
// the worktree.
func (s Stage) ChangesCode() bool {
	return s.Kind() == StageKindAgent && s.Workspace == WorkspaceWorktree
}

// followingKey returns the stage as a belt defines it: a stage with a key
// writes the file named after it, so its saved file name is dropped.
func (s Stage) followingKey() Stage {
	if s.Key != nil {
		s.savedWrites = nil
	}
	return s
}

// Outcome finds the outcome whose normalised name matches name, or nil.
func (s Stage) Outcome(name string) *Outcome {
	wanted := outcomeNormalize(name)
	for i := range s.Outcomes {
		if outcomeNormalize(s.Outcomes[i].Name) == wanted {
			o := s.Outcomes[i].Clone()
			return &o
		}
	}
	return nil
}

// StageArgs are the parameters of NewStage. Pointer fields whose default
// is not the Go zero value are nil for that default: Name nil derives the name
// from the type or watch target, Workspace nil is WorkspaceReadOnly, Slots nil
// is 2, Fallback nil is FallbackHold, Runner nil is BackendPi for agent stages.
type StageArgs struct {
	ID         string
	Type       StageType
	Name       *string
	Agent      *AgentRef
	Key        *string
	Runner     *Backend
	Model      string
	Thinking   *Thinking
	Alternates []SessionSettings
	Workspace  *Workspace
	Writes     *string
	Slots      *int
	Brief      string
	Outcomes   []Outcome
	Fallback   *Fallback
	Route      *string
	Watch      *WatchTarget
	Source     *SourceSharing
}

// NewStage creates a stage with the defaults and derivations of Stage.init(id:type:...):
// the name, the agent, key and runner only on agent stages, the watch only on
// watch stages other than the pull request, and Writes applied when given.
func NewStage(a StageArgs) Stage {
	s := Stage{ID: a.ID, Type: a.Type, Model: a.Model, Thinking: CopyPointer(a.Thinking), Brief: a.Brief, Route: CopyPointer(a.Route)}
	isAgent := a.Type.Kind() == StageKindAgent
	switch {
	case a.Name != nil:
		s.Name = *a.Name
	case a.Type == StageTypeWatch:
		target := ValueOr(a.Watch, WatchTargetPullRequest)
		s.Name = target.Title()
	default:
		s.Name = a.Type.Title()
	}
	if isAgent {
		s.Agent = CopyPointer(a.Agent)
		s.Key = CopyPointer(a.Key)
		runner := ValueOr(a.Runner, BackendPi)
		s.Runner = &runner
		if len(a.Alternates) > 0 {
			s.Alternates = cloneEach(a.Alternates, cloneValue[SessionSettings])
		}
	}
	s.Workspace = WorkspaceReadOnly
	if a.Workspace != nil {
		s.Workspace = *a.Workspace
	}
	s.Slots = 2
	if a.Slots != nil {
		s.Slots = *a.Slots
	}
	s.Outcomes = append([]Outcome{}, cloneEach(a.Outcomes, Outcome.Clone)...)
	s.Fallback = FallbackHold
	if a.Fallback != nil {
		s.Fallback = *a.Fallback
	}
	if a.Type.Kind() == StageKindWatch && a.Watch != nil && *a.Watch != WatchTargetPullRequest {
		target := *a.Watch
		s.Watch = &target
	}
	if a.Type == StageTypePrompt && a.Source != nil {
		s.Source = CopyPointer(a.Source)
	}
	if a.Writes != nil {
		s.SetWrites(*a.Writes)
	}
	return s
}

// Clone is a deep copy of the stage: its outcomes, alternates, agent pin, key
// and pointers share nothing with s. Clone also copies
// the unexported savedWrites name.
func (s Stage) Clone() Stage {
	c := cloneValue(s)
	c.savedWrites = CopyPointer(s.savedWrites)
	return c
}
