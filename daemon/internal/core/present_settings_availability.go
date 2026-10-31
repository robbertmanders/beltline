// The settings' availability: whether a setting may change now, and the reason it may not.

package core

// SettingAvailability is whether a setting may be changed to its other value now. Reason is nil when
// it may; otherwise a sentence a person can read. The daemon checks it again when a patch arrives.
type SettingAvailability struct {
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason,omitzero"`
}

// SettingShadowTestsRunning is the explanation the Enable shadow tests switch gives while a test runs.
const SettingShadowTestsRunning = "Accept or discard all running shadow tests before turning this off."

// ShadowTestsToggleAvailability is whether Enable shadow tests may be turned off: not while any belt has
// a shadow test running, whether the belt is live, paused or archived.
func ShadowTestsToggleAvailability(running bool) SettingAvailability {
	if running {
		return SettingAvailability{Enabled: false, Reason: new(SettingShadowTestsRunning)}
	}
	return SettingAvailability{Enabled: true}
}
