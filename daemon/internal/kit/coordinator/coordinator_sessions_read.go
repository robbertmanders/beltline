package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
)

// SessionsView is the Run page's Sessions read: the rows and the chosen attempt,
// its saved log, the terminal the daemon can attach now and the offers it permits.
type SessionsView struct {
	RunID string                        `json:"runId"`
	Rows  []SessionSidebarProjectionRow `json:"rows"`
	// Selection is the attempt the terminal shows.
	Selection SessionSidebarProjectionSelection `json:"selection"`
	// EmptyMessage is the copy an empty list shows.
	EmptyMessage *string `json:"emptyMessage,omitzero"`
	// Terminal is the session the daemon can attach now, nil when none.
	Terminal *SessionsViewTerminal `json:"terminal,omitzero"`
	// Log is the selected attempt's saved transcript; nil when no attempt is selected.
	Log *string `json:"log,omitzero"`
	// Usage is the selected attempt's token and estimate detail.
	Usage *SessionUsage `json:"usage,omitzero"`
	// TakeOver is nil when Take over is not offered for the selection.
	TakeOver *SessionOffer `json:"takeOver,omitzero"`
	// Ghostty is the offer to open the session in Ghostty.
	Ghostty SessionOffer `json:"ghostty"`
	// ContinueSession is nil for an open attempt or one the run does not offer Continue for.
	ContinueSession *SessionOffer `json:"continueSession,omitzero"`
}

// SessionsViewTerminal is a terminal the daemon can attach: the attempt's own
// session while it runs, or its Continue conversation once it has ended.
type SessionsViewTerminal struct {
	Target    string `json:"target"`
	ReadOnly  bool   `json:"readOnly"`
	Continued bool   `json:"continued"`
}

// ReadSessionsView is the Sessions read gathered on the main actor. It reads only
// the selected attempt's logs and asks the runner whether a terminal is up; no tmux
// command is built from anything the browser sent.
// Lock: exported entry point; takes c.mu. Suspends: attachableTerminal.
func (c *Coordinator) ReadSessionsView(ctx context.Context, runID string, stage, attempt *string, now *time.Time) *SessionsView {
	c.Lock()
	defer c.EndTurn()
	run := c.book.Run(runID)
	if run == nil {
		return nil
	}
	current := core.ValueOr(now, c.clock.Now())
	selection := SessionSidebarProjectionSelectionOf(*run, stage, attempt)
	rows := SessionSidebarProjectionRows(*run, selection.AttemptID, current)
	var emptyMessage *string
	if len(rows) == 0 {
		message := core.SessionEmptyCopy(*run)
		emptyMessage = &message
	}
	noSession := func() *SessionsView {
		return &SessionsView{
			RunID: run.ID, Rows: rows, Selection: selection, EmptyMessage: emptyMessage,
			Ghostty: core.SessionGhosttyNoSessionOffer(),
		}
	}
	if selection.AttemptID == nil {
		return noSession()
	}
	var selected *core.Attempt
	for index := range run.Attempts {
		if run.Attempts[index].ID == *selection.AttemptID {
			selected = &run.Attempts[index]
			break
		}
	}
	if selected == nil || selected.Session == nil {
		return noSession()
	}
	session := *selected.Session

	terminal := c.attachableTerminal(ctx, *run, *selected, session)
	folder := c.paths.AttemptFolder(*run, *selected)
	original := readOptionalText(filepath.Join(folder, "session.log"))
	continuation := readOptionalText(filepath.Join(folder, "continue.log"))
	_, simulated := c.runner.(*runner.SimulatedRunner)
	log := core.SessionLogText(original, continuation, selected.IsOpen(), simulated)

	var takeOver *SessionOffer
	if selected.IsOpen() && terminal != nil {
		offer := c.takeOverOfferLocked(*run, *selected)
		takeOver = &offer
	}
	ghostty := core.SessionGhosttyOffer(*run, terminal != nil)
	var continueOffer *SessionOffer
	if !selected.IsOpen() {
		continueOffer = c.continueOfferLocked(*run, *selected)
	}
	return &SessionsView{
		RunID: run.ID, Rows: rows, Selection: selection, EmptyMessage: emptyMessage,
		Terminal: terminal, Log: &log, Usage: SessionSidebarProjectionUsage(*selected, *run),
		TakeOver: takeOver, Ghostty: ghostty, ContinueSession: continueOffer,
	}
}

// continueOfferLocked is the Continue control of an attempt: the contextual action the run
// page would show for it, read from the run's action context (
// `runActionContext(run).actions.first { … continueSession … }`). Nil when the run offers none.
// Lock: runs with c.mu held.
func (c *Coordinator) continueOfferLocked(run core.Run, attempt core.Attempt) *SessionOffer {
	for _, action := range c.runActionContext(run).Actions() {
		if action.ID == string(core.RunCommandContinueSession) && action.Attempt != nil && *action.Attempt == attempt.ID {
			return &SessionOffer{Enabled: action.Enabled, Reason: action.Reason}
		}
	}
	return nil
}

// attachableTerminal returns the session the daemon can attach now, read-only
// unless the policy makes it a conversation the browser may type into; nil when
// nothing is running.
// Lock: runs with c.mu held. Suspends: runner.attachCommand.
func (c *Coordinator) attachableTerminal(ctx context.Context, run core.Run, attempt core.Attempt, session string) *SessionsViewTerminal {
	name := session
	if !attempt.IsOpen() {
		name = runner.TmuxContinueName(session)
	}
	mode, err := c.terminalModeLocked(name, false, false)
	if err != nil || mode == nil {
		return nil
	}
	var command []string
	c.Suspend(func() { command = c.runner.AttachCommand(ctx, name, mode.ReadOnly) })
	if command == nil {
		return nil
	}
	return &SessionsViewTerminal{Target: name, ReadOnly: mode.ReadOnly, Continued: !attempt.IsOpen()}
}

// readOptionalText is a file's text, or nil when it cannot be read or is not
// UTF-8.
// Lock: none (reads the filesystem).
func readOptionalText(path string) *string {
	data, err := os.ReadFile(path)
	if err != nil || !utf8.Valid(data) {
		return nil
	}
	text := string(data)
	return &text
}
