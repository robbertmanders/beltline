// RouteSet, SessionSettings and
// Attempt.

package core

import (
	"strconv"
	"strings"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// RouteSet is the routes a run took at each fork. One route per fork saves as a
// bare string, the way runs saved before several routes could be taken at once
// did, so an older run reads back unchanged.
//
// Encoding: single value. Each fork with exactly one route is written as a
// string; otherwise the map of arrays. An empty set is written as {} (the
// goldens show it so). Decoding accepts both shapes, and any other value reads
// as no routes. MarshalJSON and UnmarshalJSON are the source of truth.
type RouteSet struct {
	Routes map[string][]string
}

// MarshalJSON writes the route set in its single-value form.
func (r RouteSet) MarshalJSON() ([]byte, error) {
	if len(r.Routes) == 0 {
		return []byte("{}"), nil
	}
	single := make(map[string]string, len(r.Routes))
	for fork, routes := range r.Routes {
		if len(routes) != 1 {
			return jsonx.Marshal(r.Routes)
		}
		single[fork] = routes[0]
	}
	return jsonx.Marshal(single)
}

// UnmarshalJSON reads a route set in either form: a string per fork, or an
// array. Any other value reads as no routes.
func (r *RouteSet) UnmarshalJSON(data []byte) error {
	var one map[string]string
	if json.Unmarshal(data, &one) == nil && one != nil {
		r.Routes = make(map[string][]string, len(one))
		for fork, route := range one {
			r.Routes[fork] = []string{route}
		}
		return nil
	}
	var many map[string][]string
	if json.Unmarshal(data, &many) == nil && many != nil {
		r.Routes = many
		return nil
	}
	r.Routes = map[string][]string{}
	return nil
}

// SessionSettings are the runner, model and thinking one session starts with.
type SessionSettings struct {
	Runner   *Backend  `json:"runner,omitzero"`
	Model    string    `json:"model"`
	Thinking *Thinking `json:"thinking,omitzero"`
}

// Attempt is one attempt at one stage.
//
// Encoding: synthesized: every key listed below, the optional ones omitted when
// nil. Decoding is synthesized too: id, stageId, number, startedAt and folder
// are required, and a tone that is not one of its cases fails.
type Attempt struct {
	ID      string `json:"id"`
	StageID string `json:"stageId"`
	// Number is 1 for the first attempt at this stage in the run.
	Number    int         `json:"number"`
	StartedAt jsonx.Date  `json:"startedAt"`
	EndedAt   *jsonx.Date `json:"endedAt,omitzero"`
	Outcome   *string     `json:"outcome,omitzero"`
	Tone      *Tone       `json:"tone,omitzero"`
	// Route is what the outcome did, such as "Next: Spec" or "Held for you".
	Route *string `json:"route,omitzero"`
	// Note is the note this attempt started with.
	Note *string `json:"note,omitzero"`
	// Session is the tmux session, for agent stages.
	Session *string `json:"session,omitzero"`
	// SessionID is the agent's own session id, for its log and for continuing it.
	SessionID *string `json:"sessionId,omitzero"`
	// Settings are frozen at start, so inspecting or continuing an old session
	// never substitutes a later stage adjustment. Absent on older attempts.
	Settings *SessionSettings `json:"settings,omitzero"`
	// Continues is the attempt whose agent session this one continues, after you answered it.
	Continues *string `json:"continues,omitzero"`
	// Replays is the earlier attempt whose context this fresh session starts from.
	Replays *string `json:"replays,omitzero"`
	// AgentName is the name the agent knows its session by, when it isn't Session:
	// a continued attempt keeps the first attempt's.
	AgentName *string `json:"agentName,omitzero"`
	// LogMark is how many assistant messages the agent's log held when this
	// attempt started, so a continued session's earlier turns don't count.
	LogMark *int `json:"logMark,omitzero"`
	// Usage is the agent-reported spend, captured when this attempt ends.
	Usage *Usage `json:"usage,omitzero"`
	// UsageMark is Copilot's cumulative nano-AIU before this attempt started.
	UsageMark *int `json:"usageMark,omitzero"`
	// CopilotInteractive is set when this Copilot attempt was launched
	// interactively. Absent on a legacy headless attempt, which still finishes
	// when its process exits.
	CopilotInteractive *bool `json:"copilotInteractive,omitzero"`
	// LaunchMode is how this attempt's session was launched. Absent on attempts
	// saved before every stage ran interactively: see Interactive.
	LaunchMode *AttemptLaunchMode `json:"launchMode,omitzero"`
	// CompletionToken is the current launch's handoff token. An interactive
	// session finishes only once the attempt folder's handoff record holds it,
	// so an earlier launch's record cannot end this one.
	CompletionToken *string `json:"completionToken,omitzero"`
	// UsageCheckpoint is the Claude line count, Codex counters, or the native
	// rollout id for this attempt's boundary.
	UsageCheckpoint *UsageCheckpoint `json:"usageCheckpoint,omitzero"`
	// Folder is the attempt's folder, relative to the run's folder.
	Folder string `json:"folder"`
	// Leg is the leg the attempt is one of, after a fork took more than one route.
	Leg *string `json:"leg,omitzero"`
	// Detail is a line about what happened, such as a failure reason or the decision's note.
	Detail *string `json:"detail,omitzero"`
	// AfkDecision is the AFK decision that ended this attempt's wait for you, by id.
	AfkDecision *string `json:"afkDecision,omitzero"`
	// Interruption is the pause that interrupted this open turn, and the Continue that resumes it.
	Interruption *AttemptInterruption `json:"interruption,omitzero"`
	// ReadOnlyBaseline is the read-only checkout fingerprint taken when the
	// attempt started. Pause never replaces it.
	ReadOnlyBaseline *string `json:"readOnlyBaseline,omitzero"`
}

// AttemptLaunchMode is how an agent attempt's session was launched.
type AttemptLaunchMode string

const (
	// AttemptLaunchInteractive runs the runner's own interface: the brief starts
	// on its own, a person can take over, and the agent hands the work back.
	AttemptLaunchInteractive AttemptLaunchMode = "interactive"
	// AttemptLaunchHeadless runs one headless turn that finishes when its process exits.
	AttemptLaunchHeadless AttemptLaunchMode = "headless"
)

// Interactive reports whether the attempt's session runs interactively on
// backend: its saved launch mode, else the Copilot flag older attempts saved.
// Older attempts on other runners ran headless.
func (a Attempt) Interactive(backend Backend) bool {
	if a.LaunchMode != nil {
		return *a.LaunchMode == AttemptLaunchInteractive
	}
	return backend == BackendCopilot && a.CopilotInteractive != nil && *a.CopilotInteractive
}

// AttemptCompletionToken is the handoff token of a session's next launch after
// previous, the token of its last one: the session id and the launch's number,
// so each launch of an attempt, a turn resumed after a pause included, waits for
// a handoff of its own.
func AttemptCompletionToken(sessionID string, previous *string) string {
	launch := 1
	if previous != nil {
		if count, found := strings.CutPrefix(*previous, sessionID+"#"); found {
			if n, err := strconv.Atoi(count); err == nil {
				launch = n + 1
			}
		}
	}
	return sessionID + "#" + strconv.Itoa(launch)
}

// AttemptArgs are the parameters of NewAttempt.
type AttemptArgs struct {
	StageID   string
	Number    int
	StartedAt jsonx.Date
	Note      *string
	Folder    string
	Leg       *string
}

// NewAttempt starts an attempt: its id is AttemptID of its stage, number and
// leg, and its leg is LegFromID(leg).
func NewAttempt(a AttemptArgs) Attempt {
	return Attempt{
		ID: AttemptID(a.StageID, a.Number, a.Leg), StageID: a.StageID, Number: a.Number, StartedAt: a.StartedAt,
		Note: a.Note, Folder: a.Folder, Leg: LegFromID(a.Leg),
	}
}

// AttemptID is an attempt's id, which tells one leg's attempt at a stage from
// another's.
func AttemptID(stageID string, number int, leg *string) string {
	if id := LegFromID(leg); id != nil {
		return stageID + "-" + *id + "-" + strconv.Itoa(number)
	}
	return stageID + "-" + strconv.Itoa(number)
}

// AttemptKey is the key that tells one attempt at a stage from another, with the
// leg after a fork. The line before a fork has no leg of its own.
func AttemptKey(stageID string, leg *string) string {
	if id := LegFromID(leg); id != nil {
		return stageID + "|" + *id
	}
	return stageID
}

// IsOpen reports whether the attempt has not ended.
func (a Attempt) IsOpen() bool {
	return a.EndedAt == nil
}

// AgentSessionName is the name the agent knows the session by: AgentName, else
// Session.
func (a Attempt) AgentSessionName() *string {
	if a.AgentName != nil {
		return CopyPointer(a.AgentName)
	}
	return CopyPointer(a.Session)
}

// Clone is a deep copy of the route set: its map and each route list.
func (r RouteSet) Clone() RouteSet {
	return cloneValue(r)
}

// Clone is a deep copy of the attempt, with its usage, settings, and interruption.
func (a Attempt) Clone() Attempt {
	return cloneValue(a)
}
