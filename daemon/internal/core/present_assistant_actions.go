// The action rules of the configuration assistant:
// which commands the assistant offers in its current state, and why
// a command is unavailable. The coordinator reads its state and passes it in.

package core

import "strings"

// AssistantAction is a daemon-owned command the configuration assistant can offer
// in its current state. Reason says why it is unavailable; nil when it is enabled.
type AssistantAction struct {
	ID      string  `json:"id"`
	Enabled bool    `json:"enabled"`
	Label   string  `json:"label"`
	Reason  *string `json:"reason"`
}

// Copy used by the configuration assistant's commands. Literal text is exact.
const (
	assistantReasonBusy = "The assistant is busy."
)

// AssistantActionsOf lists the assistant's three commands, in order: change its
// configuration, start it and stop it. model is the configured model, and
// modelWarning gives the reason that model cannot start a session, nil when it
// can; it is only asked when the assistant is neither busy nor running and a model
// is set, so a caller's lookup is never made for a command that is unavailable anyway.
func AssistantActionsOf(busy, running bool, model string, modelWarning func() *string) []AssistantAction {
	var configurationReason, stateReason, stopReason *string
	switch {
	case busy:
		configurationReason = new(assistantReasonBusy)
	case running:
		configurationReason = new("Stop the assistant before changing its configuration.")
	}
	switch {
	case busy:
		stateReason = new(assistantReasonBusy)
	case running:
		stateReason = new("The assistant is already running.")
	}
	startReason := stateReason
	if startReason == nil {
		if strings.TrimSpace(model) == "" {
			startReason = new("Assistant: choose an allowed model.")
		} else {
			startReason = modelWarning()
		}
	}
	switch {
	case busy:
		stopReason = new(assistantReasonBusy)
	case !running:
		stopReason = new("The assistant is not running.")
	}
	return []AssistantAction{
		{ID: "configuration", Enabled: configurationReason == nil, Label: "Change configuration", Reason: configurationReason},
		{ID: "start", Enabled: startReason == nil, Label: "Start", Reason: startReason},
		{ID: "stop", Enabled: stopReason == nil, Label: "Stop", Reason: stopReason},
	}
}
