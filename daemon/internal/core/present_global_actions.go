// The global controls: pause, AFK and check now, with their labels and help. The
// coordinator reads the pause, AFK and decision state and passes it in.

package core

// GlobalAction is one of the app's global controls. Reason says why it is unavailable;
// nil when it is enabled. Help is what the control does, as the app's tooltip said it.
type GlobalAction struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason"`
	Help    *string `json:"help,omitzero"`
}

// Copy used by the global controls. Literal text is exact.
const ()

// GlobalActionsOf lists the global controls in pause, AFK, check-now order. Every
// control is enabled: Fetch stays available while paused, as in the app. globallyPaused
// is the global pause switch, afkEnabled the global AFK switch, and deciding whether
// an AFK decision is in progress.
func GlobalActionsOf(globallyPaused, afkEnabled, deciding bool) []GlobalAction {
	pauseHelp := "Stop every agent session, so nothing fails while the laptop is closed"
	if globallyPaused {
		pauseHelp = "Continue every agent session where it stopped"
	}
	afkLabel, afkState := "GLOBAL AFK OFF", "off"
	if afkEnabled {
		afkLabel, afkState = "GLOBAL AFK ON", "on"
		if deciding {
			afkLabel = "GLOBAL AFK · DECIDING"
		}
	}
	afkHelp := "Global AFK is " + afkState + " for runs set to Global. Runs set to Always or Never keep their own."
	return []GlobalAction{
		{ID: "pause", Label: PauseLabelFor(globallyPaused), Enabled: true, Help: new(pauseHelp)},
		{ID: "afk", Label: afkLabel, Enabled: true, Help: new(afkHelp)},
		{ID: "check-now", Label: "Fetch", Enabled: true},
	}
}
