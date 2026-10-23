// The Ghostty offer of the Sessions sidebar: whether
// the Sessions sidebar may open the selected session in Ghostty, and the reason when it may
// not. The coordinator reads the selection and whether a terminal is up, and passes them in.

package core

// SessionOffer says whether a sidebar control may be used now, and when it may not, why.
// Reason is the sentence a person reads; nil when enabled.
type SessionOffer struct {
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason,omitzero"`
}

// SessionGhosttyNoSessionOffer is the Ghostty offer when no attempt is selected, or the
// selected one has no session to open.
func SessionGhosttyNoSessionOffer() SessionOffer {
	reason := "This run has no session to open in Ghostty."
	return SessionOffer{Enabled: false, Reason: &reason}
}

// SessionGhosttyOffer is the Ghostty offer for a selected session: unavailable when no
// terminal is running for it, or when the user has paused the run, else enabled.
func SessionGhosttyOffer(run Run, hasTerminal bool) SessionOffer {
	switch {
	case !hasTerminal:
		reason := "No terminal is running for this session."
		return SessionOffer{Enabled: false, Reason: &reason}
	case run.Pause != nil && run.Pause.BlocksProgress():
		reason := DesktopActionsPausedReason
		return SessionOffer{Enabled: false, Reason: &reason}
	default:
		return SessionOffer{Enabled: true}
	}
}
