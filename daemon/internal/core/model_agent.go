// Agent, AgentRef, AgentSnapshot,
// AgentOutcome, Tint, Thinking and AgentRecord.

package core

import (
	"reflect"
	"time"
)

// Agent is an agent a belt's stages can use: its brief, and the outcomes and
// settings a new stage starts with. Agents live in the library as numbered
// versions of a Markdown file; a saved version never changes, so a belt keeps
// the agent it was saved with.
type Agent struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
	// Key is legacy metadata used to seed stage placeholders and render older
	// runs. New belts name outputs with the key stored on each stage.
	Key       string    `json:"key"`
	Workspace Workspace `json:"workspace"`
	// Slots is how many runs may be at a stage with this agent at once.
	Slots  int     `json:"slots"`
	Runner Backend `json:"runner"`
	// Model is empty for the runner's default.
	Model string `json:"model"`
	// Thinking is nil for the runner's default.
	Thinking *Thinking `json:"thinking,omitzero"`
	// Alternates are other runner, model and thinking settings a new stage
	// spreads its fresh sessions across, besides these.
	Alternates []SessionSettings `json:"alternates"`
	Tint       Tint              `json:"tint"`
	// Icon is the glyph its stage boxes show; nil is the chip.
	Icon     *StageIcon     `json:"icon,omitzero"`
	Outcomes []AgentOutcome `json:"outcomes"`
	Brief    string         `json:"brief"`
	// Origin is the library repository this agent was installed from; nil for your own.
	Origin *LibraryOrigin `json:"origin,omitzero"`
}

// AgentArgs are the parameters of NewAgent. Pointer fields whose default
// is not the Go zero value are nil for that default: Version nil is 1, Key nil
// is the ID, Workspace nil is WorkspaceReadOnly, Slots nil is 2, Runner nil is
// BackendPi and Tint nil is TintGrey.
type AgentArgs struct {
	ID         string
	Version    *int
	Name       string
	Key        *string
	Workspace  *Workspace
	Slots      *int
	Runner     *Backend
	Model      string
	Thinking   *Thinking
	Alternates []SessionSettings
	Tint       *Tint
	Icon       *StageIcon
	Outcomes   []AgentOutcome
	Brief      string
}

// NewAgent creates an agent with the defaults of Agent.init(id:version:name:...).
func NewAgent(a AgentArgs) Agent {
	agent := Agent{
		ID:         a.ID,
		Version:    1,
		Name:       a.Name,
		Key:        a.ID,
		Workspace:  WorkspaceReadOnly,
		Slots:      2,
		Runner:     BackendPi,
		Model:      a.Model,
		Thinking:   a.Thinking,
		Alternates: a.Alternates,
		Tint:       TintGrey,
		Icon:       a.Icon,
		Outcomes:   a.Outcomes,
		Brief:      a.Brief,
	}
	if a.Version != nil {
		agent.Version = *a.Version
	}
	if a.Key != nil {
		agent.Key = *a.Key
	}
	if a.Workspace != nil {
		agent.Workspace = *a.Workspace
	}
	if a.Slots != nil {
		agent.Slots = *a.Slots
	}
	if a.Runner != nil {
		agent.Runner = *a.Runner
	}
	if a.Tint != nil {
		agent.Tint = *a.Tint
	}
	return agent
}

// Clone is a deep copy of the agent: it shares no slice, map or pointer with a.
func (a Agent) Clone() Agent {
	return cloneValue(a)
}

// Ref is the version of the agent a stage pins.
func (a Agent) Ref() AgentRef {
	return AgentRef{ID: a.ID, Version: a.Version}
}

// Glyph is the glyph the agent's stage boxes show: its icon, else the chip.
func (a Agent) Glyph() StageIcon {
	if a.Icon != nil {
		return *a.Icon
	}
	return StageIconChip
}

// Pool is the settings a new stage's fresh sessions can start with: these
// first, then the alternates.
func (a Agent) Pool() []SessionSettings {
	return ModelPoolEntries(a.Runner, a.Model, a.Thinking, a.Alternates)
}

// ChangesCode reports whether a stage with this agent changes the code.
func (a Agent) ChangesCode() bool {
	return a.Workspace == WorkspaceWorktree
}

// SameContent reports whether other is the same agent without its version, to
// tell whether an edit changed anything.
func (a Agent) SameContent(other Agent) bool {
	a.Version = 0
	other.Version = 0
	return agentEqual(a, other)
}

// agentEqual reports whether two model values are equal: every field is
// compared, with a nil slice or map equal to an
// empty one. Used by SameContent and by tests.
func agentEqual(a, b any) bool {
	return valuesEqual(reflect.ValueOf(a), reflect.ValueOf(b))
}

// valuesEqual compares two values field by field: a nil slice or map equals an
// empty one, a nil pointer differs from a non-nil one, and time.Time compares by
// instant. Structs with unexported fields compare through reflect.Value.Equal.
func valuesEqual(x, y reflect.Value) bool {
	if x.Type() == valueTimeType && x.CanInterface() {
		return x.Interface().(time.Time).Equal(y.Interface().(time.Time))
	}
	switch x.Kind() {
	case reflect.Pointer, reflect.Interface:
		if x.IsNil() || y.IsNil() {
			return x.IsNil() == y.IsNil()
		}
		return valuesEqual(x.Elem(), y.Elem())
	case reflect.Slice, reflect.Array:
		if x.Len() != y.Len() {
			return false
		}
		for i := range x.Len() {
			if !valuesEqual(x.Index(i), y.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if x.Len() != y.Len() {
			return false
		}
		for _, key := range x.MapKeys() {
			other := y.MapIndex(key)
			if !other.IsValid() || !valuesEqual(x.MapIndex(key), other) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := range x.NumField() {
			if !valuesEqual(x.Field(i), y.Field(i)) {
				return false
			}
		}
		return true
	}
	if !x.CanInterface() {
		return x.Equal(y)
	}
	return x.Interface() == y.Interface()
}

// valueTimeType is time.Time, which compares with Equal rather than field by field.
var valueTimeType = reflect.TypeFor[time.Time]()

// AgentRef is which version of which agent a stage uses.
type AgentRef struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// AgentSnapshot is an agent version as a run keeps it: its file, copied when the
// run starts, so the library, or the factory that is live, can change without
// changing what the run's stages do.
type AgentSnapshot struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	// File is the agent's Markdown file, as AgentFile writes it.
	File string `json:"file"`
}

// NewAgentSnapshot copies an agent as a snapshot: its id and version, and its
// file as AgentFile writes it.
func NewAgentSnapshot(agent Agent) AgentSnapshot {
	return AgentSnapshot{ID: agent.ID, Version: agent.Version, File: AgentFileWrite(agent)}
}

// Ref is the version of the agent the snapshot keeps.
func (s AgentSnapshot) Ref() AgentRef {
	return AgentRef{ID: s.ID, Version: s.Version}
}

// Agent parses the snapshot's file back into an agent, or nil when it does not
// parse.
func (s AgentSnapshot) Agent() *Agent {
	agent, err := AgentFileParse(s.File, s.ID, s.Version)
	if err != nil {
		return nil
	}
	return &agent
}

// AgentOutcome is an outcome a new stage with this agent starts with. A goto
// names agents, not stages: the nearest earlier stage with one of them is the
// target, or else the nearest later one.
type AgentOutcome struct {
	Name string `json:"name"`
	When string `json:"when"`
	Tone Tone   `json:"tone"`
	Then Then   `json:"then"`
	// To is, for a goto, the agents to go to.
	To []string `json:"to"`
}

// Tint is the colour of an agent's stage boxes.
type Tint string

const (
	TintBlue   Tint = "blue"
	TintSand   Tint = "sand"
	TintGold   Tint = "gold"
	TintGreen  Tint = "green"
	TintViolet Tint = "violet"
	TintRose   Tint = "rose"
	TintTeal   Tint = "teal"
	TintCoral  Tint = "coral"
	TintGrey   Tint = "grey"
)

// AllTint lists every Tint in declaration order (CaseIterable).
var AllTint = []Tint{TintBlue, TintSand, TintGold, TintGreen, TintViolet, TintRose, TintTeal, TintCoral, TintGrey}

// Known reports whether x is one of the cases of Tint.
func (x Tint) Known() bool { return enumKnown(x, AllTint) }

// Label is the name shown for the tint: the raw value with its first letter
// upper-cased.
func (t Tint) Label() string {
	return CapitalizedFirst(string(t))
}

// Thinking is how much an agent should think; an unset level leaves it to the runner.
type Thinking string

const (
	ThinkingLow    Thinking = "low"
	ThinkingMedium Thinking = "medium"
	ThinkingHigh   Thinking = "high"
	ThinkingXHigh  Thinking = "xhigh"
	ThinkingMax    Thinking = "max"
)

// AllThinking lists every Thinking in declaration order (CaseIterable).
var AllThinking = []Thinking{ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax}

// Known reports whether x is one of the cases of Thinking.
func (x Thinking) Known() bool { return enumKnown(x, AllThinking) }

// Label is the name shown for the level: "Extra high" for xhigh, otherwise the
// raw value with its first letter upper-cased.
func (t Thinking) Label() string {
	if t == ThinkingXHigh {
		return "Extra high"
	}
	return CapitalizedFirst(string(t))
}

// AgentRecord is an agent in the library: its latest version, and whether it is
// archived. Archived agents stay on disk for the belts that use them, but new
// stages can't pick them.
//
// Plain struct: Archived defaults to false, the Go zero value, so callers write the literal.
type AgentRecord struct {
	Agent    Agent
	Archived bool
}
