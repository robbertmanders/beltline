package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// StateSettings is the settings subset the state resource carries.
type StateSettings struct {
	Login         string `json:"login"`
	Notifications bool   `json:"notifications"`
	AfkEnabled    bool   `json:"afkEnabled"`
}

// StateResponse is the body of GET /api/v1/state. `lastCheck` is written as an
// explicit null when no check has run.
type StateResponse struct {
	GloballyPaused bool                          `json:"globallyPaused"`
	Checking       bool                          `json:"checking"`
	LastCheck      *jsonx.Date                   `json:"lastCheck"`
	Counts         core.RunCounts                `json:"counts"`
	Notices        []coordinator.Notice          `json:"notices"`
	Settings       StateSettings                 `json:"settings"`
	Actions        []core.GlobalAction           `json:"actions"`
	Navigation     coordinator.NavigationSummary `json:"navigation"`
	ClockLabel     string                        `json:"clockLabel"`
	ClockTooltip   string                        `json:"clockTooltip"`
}

// NewStateResponse builds the state response from its arguments.
func NewStateResponse(snapshot coordinator.CoordinatorStateSnapshot) StateResponse {
	var lastCheck *jsonx.Date
	if snapshot.LastCheck != nil {
		lastCheck = &jsonx.Date{Time: *snapshot.LastCheck}
	}
	return StateResponse{
		GloballyPaused: snapshot.GloballyPaused,
		Checking:       snapshot.Checking,
		LastCheck:      lastCheck,
		Counts:         snapshot.Counts,
		Notices:        snapshot.Notices,
		Settings:       StateSettings{Login: snapshot.Login, Notifications: snapshot.Notifications, AfkEnabled: snapshot.AfkEnabled},
		Actions:        snapshot.Actions,
		Navigation:     snapshot.Navigation,
		ClockLabel:     snapshot.ClockLabel,
		ClockTooltip:   snapshot.ClockTooltip,
	}
}
