package coordinator

import (
	"fmt"
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// SessionOffer says whether a control may be used now, and when it may not, why
// (core.SessionOffer).
type SessionOffer = core.SessionOffer

// SessionUsage is the token and estimate facts of a finished agent attempt.
type SessionUsage struct {
	Tokens   *string `json:"tokens,omitzero"`
	Estimate *string `json:"estimate,omitzero"`
}

// SessionSidebarProjectionStatus is a sidebar row's state:
// a live session, one that ended in a problem, or one that finished.
type SessionSidebarProjectionStatus string

const (
	SessionSidebarProjectionLive    SessionSidebarProjectionStatus = "live"
	SessionSidebarProjectionProblem SessionSidebarProjectionStatus = "problem"
	SessionSidebarProjectionDone    SessionSidebarProjectionStatus = "done"
)

// SessionSidebarProjectionRow is one row of the Sessions sidebar.
type SessionSidebarProjectionRow struct {
	AttemptID string                         `json:"attemptId"`
	StageID   string                         `json:"stageId"`
	Title     string                         `json:"title"`
	Caption   string                         `json:"caption"`
	Status    SessionSidebarProjectionStatus `json:"status"`
	Session   string                         `json:"session"`
	Selected  bool                           `json:"selected"`
}

// SessionSidebarProjectionSelection is the attempt the terminal shows. AttemptID
// is nil only when the run has no session at all. Fallback explains why the
// chosen stage's own session is not the one shown.
type SessionSidebarProjectionSelection struct {
	StageID   string  `json:"stageId"`
	AttemptID *string `json:"attemptId,omitzero"`
	Fallback  *string `json:"fallback,omitzero"`
}

// SessionSidebarProjectionRows returns every session-bearing attempt of run, in
// stage order.
// Lock: none (pure).
func SessionSidebarProjectionRows(run core.Run, selectedAttemptID *string, now time.Time) []SessionSidebarProjectionRow {
	rows := []SessionSidebarProjectionRow{}
	for _, attempt := range core.AttemptPanelSessions(run) {
		if attempt.Session == nil {
			continue
		}
		stage := findStage(run, attempt.StageID)
		if stage == nil {
			continue
		}
		rows = append(rows, SessionSidebarProjectionRow{
			AttemptID: attempt.ID,
			StageID:   attempt.StageID,
			Title:     SessionSidebarProjectionTitle(attempt, *stage, run),
			Caption:   core.CostSession(attempt, core.CostIsAgent(attempt, run), now),
			Status:    SessionSidebarProjectionStatusOf(attempt),
			Session:   *attempt.Session,
			Selected:  selectedAttemptID != nil && *selectedAttemptID == attempt.ID,
		})
	}
	return rows
}

// SessionSidebarProjectionTitle is the row title, such as "03 Build · attempt 2".
// Lock: none (pure).
func SessionSidebarProjectionTitle(attempt core.Attempt, stage core.Stage, run core.Run) string {
	return fmt.Sprintf("%02d ", run.Layout().Number(stage.ID)) + stage.Name + " · attempt " + strconv.Itoa(attempt.Number)
}

// SessionSidebarProjectionStatusOf is the row status of an attempt.
// Lock: none (pure).
func SessionSidebarProjectionStatusOf(attempt core.Attempt) SessionSidebarProjectionStatus {
	if attempt.IsOpen() {
		return SessionSidebarProjectionLive
	}
	if attempt.Tone != nil && *attempt.Tone == core.ToneProblem {
		return SessionSidebarProjectionProblem
	}
	return SessionSidebarProjectionDone
}

// SessionSidebarProjectionSelectionOf chooses the attempt the terminal shows: the
// stage's own session when it has one, chosen by attempt or its latest. Otherwise
// the run's latest session, with the reason.
// Lock: none (pure).
func SessionSidebarProjectionSelectionOf(run core.Run, stage *string, attempt *string) SessionSidebarProjectionSelection {
	stageID := run.StageID
	if stage != nil && findStage(run, *stage) != nil {
		stageID = *stage
	}
	var stageSessions []core.Attempt
	for _, candidate := range core.AttemptPanelAttempts(stageID, run) {
		if candidate.Session != nil {
			stageSessions = append(stageSessions, candidate)
		}
	}
	if own := core.AttemptPanelShown(stageSessions, attempt); own != nil {
		id := own.ID
		return SessionSidebarProjectionSelection{StageID: stageID, AttemptID: &id}
	}
	sessions := core.AttemptPanelSessions(run)
	if len(sessions) == 0 {
		return SessionSidebarProjectionSelection{StageID: stageID}
	}
	latest := sessions[len(sessions)-1].ID
	fallback := core.SessionFallbackCopy(run, stageID)
	return SessionSidebarProjectionSelection{StageID: stageID, AttemptID: &latest, Fallback: &fallback}
}

// SessionSidebarProjectionUsage is the token and estimate detail of a finished
// agent attempt that reported usage, or nil.
// Lock: none (pure).
func SessionSidebarProjectionUsage(attempt core.Attempt, run core.Run) *SessionUsage {
	if !core.CostIsAgent(attempt, run) || attempt.IsOpen() || attempt.Usage == nil {
		return nil
	}
	usage := SessionUsage{Tokens: core.CostTokenDetail(*attempt.Usage)}
	if core.CostExplainsEstimate(attempt.Usage) {
		explanation := core.CostEstimateExplanation
		usage.Estimate = &explanation
	}
	return &usage
}

// SessionSidebarProjectionAttachTarget is the session a tmux attach command names:
// the word after its last `-t`.
// Lock: none (pure).
func SessionSidebarProjectionAttachTarget(command []string) *string {
	index := -1
	for i, word := range command {
		if word == "-t" {
			index = i
		}
	}
	if index < 0 || index+1 >= len(command) {
		return nil
	}
	target := command[index+1]
	return &target
}
