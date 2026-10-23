// The run's belt as machines on a track, as the run page draws it: the
// machine map's cells, tracks, fork spans, route labels and crates, in points.

package core

import (
	"slices"
	"strconv"
)

// RunMap is the renderer-independent machine map of a run, in points. `y` in a
// placement is the top of its cell; route spans identify where each fork leaves
// and rejoins.
type RunMap struct {
	AvailableWidth      float64            `json:"availableWidth"`
	ContentWidth        float64            `json:"contentWidth"`
	MinimumContentWidth float64            `json:"minimumContentWidth"`
	Compact             bool               `json:"compact"`
	Column              float64            `json:"column"`
	Height              float64            `json:"height"`
	MainY               float64            `json:"mainY"`
	Placements          []RunMapPlacement  `json:"placements"`
	Tracks              []RunMapTrackPath  `json:"tracks"`
	FilledTrackPathIDs  []string           `json:"filledTrackPathIds"`
	FillStageID         *string            `json:"fillStageId,omitzero"`
	FillUntilX          float64            `json:"fillUntilX"`
	Finished            bool               `json:"finished"`
	Forks               []MapGeometrySpan  `json:"forks"`
	RouteLabels         []RunMapRouteLabel `json:"routeLabels"`
	Crates              []MapCrate         `json:"crates"`
	// Caption is the caption over the map: where the run is now, or what it shipped as.
	Caption string `json:"caption"`
	// Moving says whether the filled track's stripes move: the run is working and nothing pauses it.
	Moving bool `json:"moving"`
}

// RunMapPlacement is one stage cell of the map. Dimmed is set for a stage
// the run has not reached or a route skipped.
type RunMapPlacement struct {
	StageID string  `json:"stageId"`
	Number  int     `json:"number"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	Dimmed  bool    `json:"dimmed"`
}

// RunMapTrackPath is one drawn track: the main line or a fork route.
// ForkID, RouteID and Taken are set for route tracks only.
type RunMapTrackPath struct {
	ID           string       `json:"id"`
	Kind         string       `json:"kind"`
	ForkID       *string      `json:"forkId,omitzero"`
	RouteID      *string      `json:"routeId,omitzero"`
	Taken        *bool        `json:"taken,omitzero"`
	Subpaths     [][]MapPoint `json:"subpaths"`
	CornerRadius float64      `json:"cornerRadius"`
}

// RunMapRouteLabel is the name of a fork route, drawn beside its track.
type RunMapRouteLabel struct {
	ID      string  `json:"id"`
	ForkID  string  `json:"forkId"`
	RouteID string  `json:"routeId"`
	Text    string  `json:"text"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Room    float64 `json:"room"`
	Taken   *bool   `json:"taken,omitzero"`
}

// RunMapArgs are what NewRunMap draws from: the run, its laid-out geometry, the
// page's widths, its stage panels and whether a pause holds the run's belt.
type RunMapArgs struct {
	Run            Run
	Geometry       MapGeometry
	Compact        bool
	AvailableWidth float64
	ContentWidth   float64
	Panels         []RunPresentationStagePanel
	Paused         bool
	Ambiguous      map[string]struct{}
}

// NewRunMap is the run's machine map. Track paths are the main line's subpaths
// then one per lane; the filled track ids are the main subpaths plus the lanes
// on the run's selected path.
func NewRunMap(a RunMapArgs) RunMap {
	run, geometry := a.Run, a.Geometry
	metrics := MapMetricsRun(a.Compact)
	plan := NewMapPlan(run.Layout(), metrics)
	routes := run.Routes()
	fill := geometry.RunFill(run, routes, a.Paused)
	routeTracks, routeLabels := runMapRoutes(plan, geometry, routes)
	stageID := run.StageID
	return RunMap{
		AvailableWidth: a.AvailableWidth,
		ContentWidth:   a.ContentWidth,
		MinimumContentWidth: MapWidth{
			Columns:       plan.Columns(),
			Forks:         plan.Forks(),
			MinimumColumn: metrics.MinColumn,
			Pad:           metrics.Pad,
		}.MinimumWidth(),
		Compact:            a.Compact,
		Column:             geometry.Column,
		Height:             geometry.Height,
		MainY:              geometry.MainY,
		Placements:         runMapPlacements(geometry, metrics.Cell, a.Panels),
		Tracks:             append(runMapMainTracks(geometry), routeTracks...),
		FilledTrackPathIDs: runMapFilledIDs(geometry, fill),
		FillStageID:        &stageID,
		FillUntilX:         fill.UntilX,
		Finished:           fill.Finished,
		Forks:              geometry.Forks,
		RouteLabels:        routeLabels,
		Crates:             geometry.RunCrates(run, routes, metrics.Thickness),
		Caption:            RunPresentationCaption(run, a.Ambiguous),
		Moving:             fill.Moving,
	}
}

// runMapPlacements is one cell per stage, centred on its column. A cell is
// dimmed when its stage panel's step is one the map dims; a stage with no panel is not.
func runMapPlacements(geometry MapGeometry, height float64, panels []RunPresentationStagePanel) []RunMapPlacement {
	dimmed := map[string]bool{}
	for _, panel := range panels {
		dimmed[panel.ID] = panel.Step.Dimmed()
	}
	out := []RunMapPlacement{}
	for _, cell := range geometry.Cells {
		out = append(out, RunMapPlacement{
			StageID: cell.Stage.ID,
			Number:  cell.Number,
			X:       cell.X - geometry.Column/2,
			Y:       cell.Top,
			Width:   geometry.Column,
			Height:  height,
			Dimmed:  dimmed[cell.Stage.ID],
		})
	}
	return out
}

// runMapMainTracks is the main line, one track per subpath.
func runMapMainTracks(geometry MapGeometry) []RunMapTrackPath {
	out := []RunMapTrackPath{}
	for index, subpath := range geometry.Line.Subpaths {
		out = append(out, RunMapTrackPath{
			ID:           "main-" + strconv.Itoa(index),
			Kind:         "main",
			Subpaths:     [][]MapPoint{subpath},
			CornerRadius: geometry.Line.CornerRadius,
		})
	}
	return out
}

// runMapRoutes is one track per fork route, and a route label beside each
// track except the line's own, which a feeding fork's first route carries.
func runMapRoutes(plan MapPlan, geometry MapGeometry, routes map[string][]string) ([]RunMapTrackPath, []RunMapRouteLabel) {
	tracks := []RunMapTrackPath{}
	labels := []RunMapRouteLabel{}
	for _, lane := range geometry.Lanes {
		forkID := lane.Fork
		routeID := lane.Route.ID
		var taken *bool
		if routeIDs, ok := routes[lane.Fork]; ok {
			value := slices.Contains(routeIDs, lane.Route.ID)
			taken = &value
		}
		tracks = append(tracks, RunMapTrackPath{
			ID:           "route:" + lane.Fork + ":" + lane.Route.ID,
			Kind:         "route",
			ForkID:       &forkID,
			RouteID:      &routeID,
			Taken:        taken,
			Subpaths:     lane.Path.Subpaths,
			CornerRadius: lane.Path.CornerRadius,
		})
		if plan.IsLine(lane, geometry) {
			continue
		}
		labels = append(labels, RunMapRouteLabel{
			ID:      lane.Fork + ":" + lane.Route.ID,
			ForkID:  lane.Fork,
			RouteID: lane.Route.ID,
			Text:    lane.Route.Name,
			X:       lane.LabelX,
			Y:       lane.Y - 10,
			Room:    lane.Room,
			Taken:   taken,
		})
	}
	return tracks, labels
}

// runMapFilledIDs is the main subpaths plus the lanes on the run's selected path.
func runMapFilledIDs(geometry MapGeometry, fill MapFill) []string {
	var ids []string
	for index := range geometry.Line.Subpaths {
		ids = append(ids, "main-"+strconv.Itoa(index))
	}
	for _, lane := range fill.Lanes {
		ids = append(ids, "route:"+lane.Fork+":"+lane.Route.ID)
	}
	return ids
}
