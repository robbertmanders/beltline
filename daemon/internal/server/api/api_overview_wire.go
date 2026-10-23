package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// OverviewResponse is `GET /api/v1/views/overview`: the Summary, the Floor in
// the belt look Settings picks, and the global counts. Every figure, order and
// coordinate is computed by Core; the browser draws what arrives and computes
// none of it.
type OverviewResponse struct {
	Summary core.RunCounts `json:"summary"`
	// Caption is the global counts line the Overview's greeting carries.
	Caption string `json:"caption"`
	// Greeting is "Good morning." and its kin, for the daemon's clock and calendar.
	Greeting string `json:"greeting"`
	// Look is how the Floor is drawn: the belt look Settings holds.
	Look  core.BeltLook        `json:"look"`
	Today core.OverviewSummary `json:"today"`
	Floor core.OverviewFloor   `json:"floor"`
	// MetroLayout is the metro's drawing, sent only when the Floor is the metro.
	MetroLayout *MetroLayoutWire `json:"metroLayout,omitzero"`
	// Runs is every active run the Overview names, ordered by id.
	Runs []core.OverviewRun `json:"runs"`
}

// NewOverviewResponse builds the overview response from its arguments.
func NewOverviewResponse(view core.OverviewView) OverviewResponse {
	out := OverviewResponse{
		Summary:  view.Summary,
		Caption:  view.Today.Caption,
		Greeting: view.Greeting,
		Look:     view.Look,
		Today:    view.Today,
		Floor:    view.Floor,
		Runs:     view.OrderedRuns(),
	}
	if view.MetroLayout != nil {
		metro := NewMetroLayoutWire(*view.MetroLayout)
		out.MetroLayout = &metro
	}
	return out
}
