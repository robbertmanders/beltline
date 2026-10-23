// RunPause and its nested
// types, and PauseSpend. AttemptInterruption is in model_run_pause_interruption.go.
//
// Dates are jsonx.Date in stored fields and time.Time in parameters.
// Durations are float64 seconds.

package core

import (
	"fmt"
	"time"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// RunPausePhase is where a whole-run pause is.
type RunPausePhase string

const (
	// RunPausePhasePausing is accepted, and a generating session has not confirmed it stopped.
	RunPausePhasePausing RunPausePhase = "pausing"
	// RunPausePhasePaused means every generating session confirmed, or there was nothing to stop.
	RunPausePhasePaused RunPausePhase = "paused"
	// RunPausePhaseIncomplete means at least one session did not confirm. The barrier stays up.
	RunPausePhaseIncomplete RunPausePhase = "incomplete"
	// RunPausePhaseResuming means Resume was accepted. Continuations wait until source reconciliation.
	RunPausePhaseResuming RunPausePhase = "resuming"
	// RunPausePhaseReconcileFailed means the source could not be read. The run stays stopped until Retry.
	RunPausePhaseReconcileFailed RunPausePhase = "reconcileFailed"
)

// AllRunPausePhase lists every RunPausePhase in declaration order (CaseIterable-like).
var AllRunPausePhase = []RunPausePhase{
	RunPausePhasePausing, RunPausePhasePaused, RunPausePhaseIncomplete, RunPausePhaseResuming, RunPausePhaseReconcileFailed,
}

// RunPauseContinuePrompt is the prompt a Continue sends.
const RunPauseContinuePrompt = "Continue"

// RunPauseExplanation is what the pause control says.
// DesktopActionsPausedReason is the reason Ghostty is unavailable while the user
// has paused the run. It lives in core so the
// coordinator and launch share it.
const DesktopActionsPausedReason = "Paused by you"

const RunPauseExplanation = "Paused by you. Resume continues interrupted agent sessions and restores queued work, watches, and decision stops."

// RunPauseGlobalPauseHelp is the help shown while everything is paused.
const RunPauseGlobalPauseHelp = "Everything is paused. Continue from the toolbar first."

// RunPauseSnapshot is a run's state when a pause started.
type RunPauseSnapshot struct {
	ID      string   `json:"id"`
	State   RunState `json:"state"`
	StageID *string  `json:"stageId,omitzero"`
	Reason  *string  `json:"reason,omitzero"`
}

// RunPauseInterval is one closed stretch of pause time.
type RunPauseInterval struct {
	Start jsonx.Date `json:"start"`
	End   jsonx.Date `json:"end"`
}

// RunPauseDeferredKind is the case of a RunPauseDeferred.
type RunPauseDeferredKind string

const (
	// RunPauseDeferredMerged is a merge that finished on disk while the run was paused: Values are the branches.
	RunPauseDeferredMerged RunPauseDeferredKind = "merged"
	// RunPauseDeferredConflict is a merge that conflicted while the run was paused: Values are the branches.
	RunPauseDeferredConflict RunPauseDeferredKind = "conflict"
)

// RunPauseDeferred is a merge that finished on disk while the run was paused,
// applied on Resume. Both cases carry one unlabeled [String] payload, which is
// Values.
//
// The enum form is {"merged":{"_0":[…]}} or {"conflict":{"_0":[…]}}.
type RunPauseDeferred struct {
	Kind   RunPauseDeferredKind
	Values []string
}

// MarshalJSON writes the deferred merge in the enum form.
func (d RunPauseDeferred) MarshalJSON() ([]byte, error) {
	return jsonx.Marshal(map[string]map[string][]string{string(d.Kind): {"_0": d.Values}})
}

// UnmarshalJSON reads the deferred merge from the enum form: one case key, whose
// object holds the payload "_0". Any other key of the payload is ignored.
func (d *RunPauseDeferred) UnmarshalJSON(data []byte) error {
	var outer map[string]struct {
		Values []string `json:"_0"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return fmt.Errorf("core: deferred merge: expected one case object")
	}
	if len(outer) != 1 {
		return fmt.Errorf("core: deferred merge: expected one case, found %d", len(outer))
	}
	for name, payload := range outer {
		kind := RunPauseDeferredKind(name)
		if !enumKnown(kind, runPauseDeferredKinds) {
			return fmt.Errorf("core: deferred merge: %q is not a case", name)
		}
		*d = RunPauseDeferred{Kind: kind, Values: payload.Values}
	}
	return nil
}

// RunPause is a whole-run pause. The legs keep the states they had; this is only
// the scheduling barrier, the interruption records, and the time watches skip.
//
// Encoding writes phase, operation, requestedAt, confirmedAt (omitted
// when nil), reason (omitted when nil), snapshots, deferred. Decoding requires
// every key but confirmedAt and reason.
type RunPause struct {
	Phase       RunPausePhase      `json:"phase"`
	Operation   string             `json:"operation"`
	RequestedAt jsonx.Date         `json:"requestedAt"`
	ConfirmedAt *jsonx.Date        `json:"confirmedAt,omitzero"`
	Reason      *string            `json:"reason,omitzero"`
	Snapshots   []RunPauseSnapshot `json:"snapshots"`
	// Deferred are the merges that finished while the run was paused, applied on Resume.
	Deferred []RunPauseDeferred `json:"deferred"`
}

// BlocksProgress reports whether nothing new starts, and a late result does not
// move the run. Every phase blocks.
func (p RunPause) BlocksProgress() bool {
	switch p.Phase {
	case RunPausePhasePausing, RunPausePhasePaused, RunPausePhaseIncomplete, RunPausePhaseResuming, RunPausePhaseReconcileFailed:
		return true
	}
	return false
}

// RunCanPause reports whether a run can be paused: it is active, and either has
// no pause or its pause is incomplete.
func RunCanPause(run Run) bool {
	if !run.State.IsActive() {
		return false
	}
	if run.Pause == nil {
		return true
	}
	return run.Pause.Phase == RunPausePhaseIncomplete
}

// RunPauseExcluded is the pause time, in seconds, between since and now: the
// closed pauses plus the one still open.
func RunPauseExcluded(run Run, since, now time.Time) float64 {
	var total float64
	for _, interval := range run.PauseTime {
		start := pickBy(interval.Start.Time, since, time.Time.After)
		end := pickBy(interval.End.Time, now, time.Time.Before)
		if end.After(start) {
			total += end.Sub(start).Seconds()
		}
	}
	// Resuming already closed the interval into PauseTime.
	if pause := run.Pause; pause != nil && pause.BlocksProgress() &&
		pause.Phase != RunPausePhaseResuming && pause.Phase != RunPausePhaseReconcileFailed {
		start := pickBy(pause.RequestedAt.Time, since, time.Time.After)
		if now.After(start) {
			total += now.Sub(start).Seconds()
		}
	}
	return total
}

// PauseSpendKind says how a later usage reading joins the one stored for an attempt.
type PauseSpendKind string

const (
	// PauseSpendKindCumulative is a reading that already includes earlier turns, so it replaces them.
	PauseSpendKindCumulative PauseSpendKind = "cumulative"
	// PauseSpendKindDelta is a reading that adds once.
	PauseSpendKindDelta PauseSpendKind = "delta"
)

// PauseSpendMerge joins a later usage reading to the existing one: a cumulative
// reading replaces it, a delta adds to it, and a missing reading leaves the
// existing usage unchanged.
func PauseSpendMerge(existing, reading *Usage, kind PauseSpendKind) *Usage {
	if reading == nil {
		return existing
	}
	if kind == PauseSpendKindDelta && existing != nil {
		sum := UsageAdd(*existing, *reading)
		return &sum
	}
	copied := *reading
	return &copied
}

// runPauseDeferredKinds lists the cases of RunPauseDeferredKind, which is not CaseIterable.
var runPauseDeferredKinds = []RunPauseDeferredKind{RunPauseDeferredMerged, RunPauseDeferredConflict}
