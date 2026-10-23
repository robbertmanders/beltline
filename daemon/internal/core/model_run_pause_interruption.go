// AttemptInterruption and its
// nested Confirmation and Continuation types.

package core

// AttemptInterruptionConfirmation is whether a paused agent turn was confirmed stopped.
type AttemptInterruptionConfirmation string

const (
	AttemptInterruptionConfirmationPending   AttemptInterruptionConfirmation = "pending"
	AttemptInterruptionConfirmationConfirmed AttemptInterruptionConfirmation = "confirmed"
	AttemptInterruptionConfirmationFailed    AttemptInterruptionConfirmation = "failed"
)

// AttemptInterruptionContinuationStatus is where a Continue is.
type AttemptInterruptionContinuationStatus string

const (
	AttemptInterruptionContinuationStatusQueued      AttemptInterruptionContinuationStatus = "queued"
	AttemptInterruptionContinuationStatusLaunching   AttemptInterruptionContinuationStatus = "launching"
	AttemptInterruptionContinuationStatusLaunched    AttemptInterruptionContinuationStatus = "launched"
	AttemptInterruptionContinuationStatusFailed      AttemptInterruptionContinuationStatus = "failed"
	AttemptInterruptionContinuationStatusAmbiguous   AttemptInterruptionContinuationStatus = "ambiguous"
	AttemptInterruptionContinuationStatusInvalidated AttemptInterruptionContinuationStatus = "invalidated"
)

// AttemptInterruptionContinuation is the Continue that resumes an interrupted turn.
type AttemptInterruptionContinuation struct {
	Operation string                                `json:"operation"`
	Status    AttemptInterruptionContinuationStatus `json:"status"`
	Reason    *string                               `json:"reason,omitzero"`
	Prompt    string                                `json:"prompt"`
}

// NewAttemptInterruptionContinuation creates a continuation. A nil prompt is
// RunPauseContinuePrompt.
func NewAttemptInterruptionContinuation(operation string, status AttemptInterruptionContinuationStatus, reason *string, prompt *string) AttemptInterruptionContinuation {
	c := AttemptInterruptionContinuation{Operation: operation, Status: status, Reason: reason, Prompt: RunPauseContinuePrompt}
	if prompt != nil {
		c.Prompt = *prompt
	}
	return c
}

// AttemptInterruption is one stage-agent turn a pause stopped, and the Continue
// that resumes it.
type AttemptInterruption struct {
	Operation    string                          `json:"operation"`
	LegID        string                          `json:"legId"`
	Confirmation AttemptInterruptionConfirmation `json:"confirmation"`
	Reason       *string                         `json:"reason,omitzero"`
	// Conversation is the native conversation id captured when the turn stopped:
	// Pi session, Copilot name, Claude or Cursor chat, Codex rollout.
	Conversation    *string                          `json:"conversation,omitzero"`
	TranscriptMark  *int                             `json:"transcriptMark,omitzero"`
	UsageMark       *int                             `json:"usageMark,omitzero"`
	UsageCheckpoint *UsageCheckpoint                 `json:"usageCheckpoint,omitzero"`
	Continuation    *AttemptInterruptionContinuation `json:"continuation,omitzero"`
}
