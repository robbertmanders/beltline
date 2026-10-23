package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// WorkResponse is `GET /api/v1/views/work`: the global summary and the selected
// groups. All rows travel unfolded; folding stays a client preference.
type WorkResponse struct {
	Summary core.RunCounts `json:"summary"`
	// Caption is the global counts line, which no query changes.
	Caption string          `json:"caption"`
	Stage   *core.WorkStage `json:"stage,omitzero"`
	Belts   []core.WorkBelt `json:"belts"`
	// MainEmptyMessage is the main table's empty copy, present only when it has no row.
	MainEmptyMessage *string                      `json:"mainEmptyMessage,omitzero"`
	MainGroups       []core.WorkGroupPresentation `json:"mainGroups"`
	WatchingGroups   []core.WorkGroupPresentation `json:"watchingGroups"`
	// ShadowGroups are the runs on a shadow version, a section of their own.
	ShadowGroups []core.WorkGroupPresentation `json:"shadowGroups"`
	// ShownRows counts the rows of every section; WatchingRows and ShadowRows count one section's alone.
	ShownRows    int                  `json:"shownRows"`
	WatchingRows int                  `json:"watchingRows"`
	ShadowRows   int                  `json:"shadowRows"`
	Waiting      []WaitingSubjectWire `json:"waiting"`
}

// NewWorkResponse builds the work response from its arguments.
func NewWorkResponse(view core.WorkView) WorkResponse {
	out := WorkResponse{
		Summary:          view.Summary,
		Caption:          view.Caption(),
		Stage:            view.Stage,
		Belts:            view.Belts,
		MainEmptyMessage: view.MainEmptyMessage,
		MainGroups:       view.MainGroups,
		WatchingGroups:   view.WatchingGroups,
		ShadowGroups:     view.ShadowGroups,
		ShownRows:        view.ShownRowCount(),
		WatchingRows:     view.WatchingRowCount(),
		ShadowRows:       view.ShadowRowCount(),
		Waiting:          []WaitingSubjectWire{},
	}
	for _, waiting := range view.Waiting {
		out.Waiting = append(out.Waiting, NewWaitingSubjectWire(waiting))
	}
	return out
}
