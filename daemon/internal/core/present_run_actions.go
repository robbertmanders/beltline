// The types and the copy;
// the availability table is in present_run_availability.go.

package core

// RunActionArgs are the parameters of NewRunAction. Optional parameters are pointers;
// nil is the default.
type RunActionArgs struct {
	ID        string
	Enabled   bool
	Reason    *string
	Stage     *string
	Leg       *string
	Outcome   *string
	Attempt   *string
	Decision  *string
	Help      *string
	Previous  *bool
	NeedsNote *bool
	NoteLabel *string
	ThenHelp  *string
}

// RunAction is one command a run resource offers, with whether the daemon would accept it
// now and why not when it would not. Clients must not trust a supplied
// `enabled`: the daemon checks the same table again when a command arrives.
// A contextual record names the concrete target it describes: a stage, a
// route leg, an outcome, a saved attempt or an AFK decision.
type RunAction struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	// Reason is nil when enabled; otherwise a sentence a person can read.
	Reason   *string `json:"reason,omitzero"`
	Stage    *string `json:"stage,omitzero"`
	Leg      *string `json:"leg,omitzero"`
	Outcome  *string `json:"outcome,omitzero"`
	Attempt  *string `json:"attempt,omitzero"`
	Decision *string `json:"decision,omitzero"`
	// Help is what the command does or costs, as the app said beside it: a
	// confirmation's detail, a menu row's help or a sheet's note. Absent when
	// the command has nothing more to say.
	Help *string `json:"help,omitzero"`
	// Previous is true on the overrule destination the applied decision would restart by
	// default, the previous agent. Absent on every other record.
	Previous *bool `json:"previous,omitzero"`
	// NeedsNote is a decide action only: whether the outcome needs a note before it can be sent (its `withOutput`).
	NeedsNote *bool `json:"needsNote,omitzero"`
	// NoteLabel is a decide action only: where the note goes, as "Note for Build".
	NoteLabel *string `json:"noteLabel,omitzero"`
	// ThenHelp is a decide action only: what choosing the outcome does, as the button's help.
	ThenHelp *string `json:"thenHelp,omitzero"`
}

// NewRunAction is a run action from its parameters. Its reason is nil when enabled, and
// otherwise the given reason or "<id> isn't available for this run.".
func NewRunAction(a RunActionArgs) RunAction {
	action := RunAction{
		ID: a.ID, Enabled: a.Enabled, Stage: a.Stage, Leg: a.Leg, Outcome: a.Outcome, Attempt: a.Attempt,
		Decision: a.Decision, Help: a.Help, Previous: a.Previous, NeedsNote: a.NeedsNote, NoteLabel: a.NoteLabel,
		ThenHelp: a.ThenHelp,
	}
	if !a.Enabled {
		reason := ValueOr(a.Reason, a.ID+" isn't available for this run.")
		action.Reason = &reason
	}
	return action
}

// IsUnscoped reports whether the record describes the command itself rather than one
// target.
func (a RunAction) IsUnscoped() bool {
	return a.Stage == nil && a.Leg == nil && a.Outcome == nil && a.Attempt == nil && a.Decision == nil
}

// RunQuickActions is the run page's shortcut buttons beside its menu, in the order they show:
// Start now on a queued run, then Retry when the actions list a retry for a saved attempt. The
// page draws each one the daemon names, enabled from that command's own action.
func RunQuickActions(run Run, actions []RunAction) []RunCommand {
	quick := []RunCommand{}
	if run.State == RunStateQueued {
		quick = append(quick, RunCommandStartNow)
	}
	for _, action := range actions {
		if action.ID == string(RunCommandRetryContinuation) && action.Attempt != nil {
			return append(quick, RunCommandRetryContinuation)
		}
	}
	return quick
}

// RunCommand is a command every run resource describes. The values are the command
// names the API posts to.
type RunCommand string

const (
	RunCommandDecide            RunCommand = "decide"
	RunCommandResume            RunCommand = "resume"
	RunCommandAnswer            RunCommand = "answer"
	RunCommandAbort             RunCommand = "abort"
	RunCommandDrop              RunCommand = "drop"
	RunCommandPark              RunCommand = "park"
	RunCommandUnpark            RunCommand = "unpark"
	RunCommandPause             RunCommand = "pause"
	RunCommandResumeRun         RunCommand = "resume-run"
	RunCommandRetryContinuation RunCommand = "retry-continuation"
	RunCommandStartNow          RunCommand = "start-now"
	RunCommandContinueSession   RunCommand = "continue-session"
	RunCommandAdjustStage       RunCommand = "adjust-stage"
	RunCommandAfk               RunCommand = "afk"
	RunCommandErase             RunCommand = "erase"
	RunCommandOverruleAfk       RunCommand = "overrule-afk"
)

// AllRunCommand lists the cases of RunCommand in declaration order (CaseIterable).
var AllRunCommand = []RunCommand{
	RunCommandDecide, RunCommandResume, RunCommandAnswer, RunCommandAbort, RunCommandDrop,
	RunCommandPark, RunCommandUnpark, RunCommandPause, RunCommandResumeRun, RunCommandRetryContinuation,
	RunCommandStartNow, RunCommandContinueSession, RunCommandAdjustStage, RunCommandAfk, RunCommandErase,
	RunCommandOverruleAfk,
}

// RunOverruleDestination is one agent destination an applied AFK decision can be overruled
// to.
type RunOverruleDestination struct {
	Stage string
	Label string
}

// RunOverruleOffer is one applied AFK decision of a run. A decision with destinations can be
// overruled; one with a refusal cannot, and carries why.
type RunOverruleOffer struct {
	Decision     string
	Attempt      string
	Destinations []RunOverruleDestination
	// Refusal is nil when the decision can be overruled.
	Refusal *string
	// Previous is the stage the decision's previous agent is, when one ran before it: the default restart.
	Previous *string
}

// RunActionContext is what the daemon and the run page know about one run when they
// decide which commands are available. External facts — checkout existence, frozen model
// choices and AFK decisions — are gathered outside the pure core and passed
// in; nothing here reads the filesystem, storage or a runner.
type RunActionContext struct {
	Run Run
	// GlobalPause is the app's one global pause.
	GlobalPause bool
	// Busy is set while a whole-run operation is still in flight, such as an AFK decision
	// or a Continue launch.
	Busy bool
	// Erasing is set while the run is being erased; no command may touch it but Erase, which
	// waits for the erasure to finish.
	Erasing bool
	// ContinueRefusals is per saved attempt: why Continue is unavailable (checkout, historical
	// settings or frozen model admission). Absent means eligible.
	ContinueRefusals map[string]string
	// AdjustRefusals is per agent stage: why Adjust is unavailable (no frozen model choices).
	AdjustRefusals map[string]string
	// Overrules are the run's applied AFK decisions, in stored order.
	Overrules []RunOverruleOffer
	// TmuxRunner is whether the configured runner is tmux-backed. Only those sessions can be
	// continued in a terminal.
	TmuxRunner bool
}

// RunActionContextDefault is the context for run with the defaults: nothing paused,
// busy or erasing, no refusals, no overrules, and a tmux-backed runner.
func RunActionContextDefault(run Run) RunActionContext {
	return RunActionContext{Run: run, TmuxRunner: true}
}

// Actions is the ordered actions: one unscoped record per command in command order,
// then contextual records per command with their targets in model order.
func (c RunActionContext) Actions() []RunAction {
	return RunAvailabilityActions(c)
}

// Static copy of RunAvailability. Literal text is exact.
const (
	RunAvailabilityParked             = "This run is parked. Unpark it first."
	RunAvailabilityErasing            = "This run is being erased."
	RunAvailabilityBusy               = "Beltline is still finishing the last change to this run."
	RunAvailabilitySessionGone        = "This session can't be continued."
	RunAvailabilityNoFailed           = "No Continue needs another try."
	RunAvailabilityContinueNeedsTmux  = "This runner has no terminal to continue a session in."
	RunAvailabilityRetryRouteMissing  = "That route isn't part of this run."
	RunAvailabilityNoteRequired       = "This outcome needs a note before it can be chosen."
	RunAvailabilityPromptRequired     = "Write your prompt before you choose this."
	RunAvailabilitySourceOnlyAtPrompt = "Source context is chosen only at a prompt stage."
	RunAvailabilitySourceUnknown      = "That isn't a source choice there is."
	RunAvailabilityRetryHelp          = "Try Continue again on the route that stopped"
	RunAvailabilityAdjustHelp         = "Uses models allowed when this run started."
	RunAvailabilityOverruleHelp       = "Required. The restarted stage reads this before anything else."
	RunAvailabilityPauseStopping      = "The pause is still stopping sessions."
)
