package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// OverviewResponse is `GET /api/v1/views/overview`: the Summary, the Floor and
// the global counts. Every figure, order and connection is computed by Core;
// the browser lays the Floor's track out in points and draws it.
type OverviewResponse struct {
	Summary core.RunCounts `json:"summary"`
	// Caption is the global counts line the Overview's greeting carries.
	Caption string `json:"caption"`
	// Greeting is "Good morning." and its kin, for the daemon's clock and calendar.
	Greeting string               `json:"greeting"`
	Today    core.OverviewSummary `json:"today"`
	Floor    core.OverviewFloor   `json:"floor"`
	// Runs is every active run the Overview names, ordered by id.
	Runs []core.OverviewRun `json:"runs"`
}

// NewOverviewResponse builds the overview response from its arguments.
func NewOverviewResponse(view core.OverviewView) OverviewResponse {
	return OverviewResponse{
		Summary:  view.Summary,
		Caption:  view.Today.Caption,
		Greeting: view.Greeting,
		Today:    view.Today,
		Floor:    view.Floor,
		Runs:     view.OrderedRuns(),
	}
}
