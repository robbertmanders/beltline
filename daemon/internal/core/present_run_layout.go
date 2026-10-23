// MapPoint, MapPath, MapPlan and MapGeometry).
//
// Pure geometry for the belt strip on a run. Widths and positions are points;
// renderers may convert the paths to their native drawing types.

package core

import "slices"

// RunLayoutCompactWidth is the widest run column that draws the strip compact: the strip follows
// the available run column, not sidebar visibility.
const RunLayoutCompactWidth = 620

// RunLayoutCompact reports whether a run column of width points draws the strip compact.
func RunLayoutCompact(width float64) bool {
	return width <= RunLayoutCompactWidth
}

// Defaults and run sizes of MapMetrics.
const (
	mapMetricsDefaultThickness = 8
	mapMetricsDefaultPad       = 56
	mapMetricsDefaultRowGap    = 18
	mapMetricsDefaultCorner    = 10
	mapMetricsDefaultMinColumn = 104
	mapMetricsRunThickness     = 10
	mapMetricsRunCompactCell   = 134
	mapMetricsRunCell          = 178
	mapMetricsRunCompactTrack  = 95
	mapMetricsRunTrack         = 117
	mapMetricsRunCompactPad    = 40
	mapMetricsRunPad           = 64
)

// MapMetrics is the cell and track dimensions shared by the map plan, app renderer and API.
type MapMetrics struct {
	Cell      float64
	Track     float64
	Thickness float64
	Pad       float64
	RowGap    float64
	Corner    float64
	MinColumn float64
}

// MapMetricsArgs are the parameters of NewMapMetrics. A nil optional takes its default.
type MapMetricsArgs struct {
	Cell      float64
	Track     float64
	Thickness *float64
	Pad       *float64
	RowGap    *float64
	Corner    *float64
	MinColumn *float64
}

// NewMapMetrics is the metrics from their parts, with the defaults for the nil ones.
func NewMapMetrics(a MapMetricsArgs) MapMetrics {
	m := MapMetrics{Cell: a.Cell, Track: a.Track, Thickness: 8, Pad: 56, RowGap: 18, Corner: 10, MinColumn: 104}
	if a.Thickness != nil {
		m.Thickness = *a.Thickness
	}
	if a.Pad != nil {
		m.Pad = *a.Pad
	}
	if a.RowGap != nil {
		m.RowGap = *a.RowGap
	}
	if a.Corner != nil {
		m.Corner = *a.Corner
	}
	if a.MinColumn != nil {
		m.MinColumn = *a.MinColumn
	}
	return m
}

// MapMetricsRun is the run page's belt metrics. Column spacing matches the Floor's minimum;
// cell size, track position and fork bends stay specific to this strip.
func MapMetricsRun(compact bool) MapMetrics {
	cell, track, pad := float64(mapMetricsRunCell), float64(mapMetricsRunTrack), float64(mapMetricsRunPad)
	if compact {
		cell, track, pad = mapMetricsRunCompactCell, mapMetricsRunCompactTrack, mapMetricsRunCompactPad
	}
	thickness := float64(mapMetricsRunThickness)
	return NewMapMetrics(MapMetricsArgs{Cell: cell, Track: track, Thickness: &thickness, Pad: &pad, MinColumn: new(MapWidthFloorMinimumColumn)})
}

// MapPlan is the columns and rows a belt takes before its width is known.
type MapPlan struct {
	Layout   BeltLayout
	Metrics  MapMetrics
	Segments []BeltLayoutSegment
}

// NewMapPlan is the plan of layout drawn with metrics.
func NewMapPlan(layout BeltLayout, metrics MapMetrics) MapPlan {
	return MapPlan{Layout: layout, Metrics: metrics, Segments: layout.Segments()}
}

// Columns is the number of columns the belt takes.
func (p MapPlan) Columns() int {
	total := 0
	for _, segment := range p.Segments {
		total += BeltLayoutColumns(segment)
	}
	return total
}

// Forks is the number of forks in the belt, nested ones included.
func (p MapPlan) Forks() int {
	total := 0
	for _, segment := range p.Segments {
		total += mapPlanForkCount(segment)
	}
	return total
}

// mapPlanForkCount counts a segment's fork and the forks nested in its routes.
func mapPlanForkCount(segment BeltLayoutSegment) int {
	if segment.Kind == BeltLayoutSegmentStage {
		return 0
	}
	count := 1
	for _, route := range segment.Routes {
		for _, nested := range route {
			count += mapPlanForkCount(nested)
		}
	}
	return count
}

// Rows is the height of count rows with their gaps.
func (p MapPlan) Rows(count int) float64 {
	return float64(float64(count)*p.Metrics.Cell) + float64(float64(max(count-1, 0))*p.Metrics.RowGap)
}

// Height is the height of the tallest segment.
func (p MapPlan) Height() float64 {
	return p.tallest(p.Segments)
}

// Width is the width arithmetic of the belt.
func (p MapPlan) Width() MapWidth {
	return MapWidth{Columns: p.Columns(), Forks: p.Forks(), MinimumColumn: p.Metrics.MinColumn, Pad: p.Metrics.Pad}
}

// Geometry is the positions of every belt part once the content width is known. All
// positions use the belt's content width, never the clipped viewport.
func (p MapPlan) Geometry(width float64) MapGeometry {
	column := p.Width().Column(width)
	height := p.Height()
	mainTop := (height - p.Metrics.Cell) / 2
	mainY := mainTop + p.Metrics.Track
	geometry := MapGeometry{Width: width, Height: height, Column: column, MainY: mainY, Layout: p.Layout}
	x := 0.0
	line := []MapPoint{{X: 0, Y: mainY}}
	var lineSegments [][]MapPoint
	for _, segment := range p.Segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			geometry.Cells = append(geometry.Cells, MapGeometryPlaced{
				Stage: *segment.Stage, Number: p.Layout.Number(segment.Stage.ID), X: x + column/2, Top: mainTop,
			})
			x += column
		case BeltLayoutSegmentFork:
			fork := *segment.Fork
			routes := segment.Routes
			line = append(line, MapPoint{X: x, Y: mainY})
			lineSegments = append(lineSegments, line)
			// A fork whose routes hold only stages keeps the flat drawing.
			routeStages, flat := mapPlanFlatStages(routes)
			if flat {
				top := (height - p.Rows(len(routes))) / 2
				start := x
				inside := x + p.Metrics.Pad
				end := inside + float64(float64(BeltLayoutColumns(segment))*column) + p.Metrics.Pad
				for index, route := range mapPlanZip(fork.Routes, len(routeStages)) {
					rowTop := top + float64(float64(index)*(p.Metrics.Cell+p.Metrics.RowGap))
					y := rowTop + p.Metrics.Track
					for stageIndex, stage := range routeStages[index] {
						geometry.Cells = append(geometry.Cells, MapGeometryPlaced{
							Stage: stage, Number: p.Layout.Number(stage.ID),
							X: inside + float64((float64(stageIndex)+0.5)*column), Top: rowTop,
						})
					}
					geometry.Lanes = append(geometry.Lanes, p.lane(fork.ID, route, start, end, mainY, y, inside, column))
				}
				geometry.Forks = append(geometry.Forks, MapGeometrySpan{ID: fork.ID, Start: start, End: end})
				x = end
			} else {
				x = p.place(fork, routes, &geometry, x, mainY, column)
			}
			line = []MapPoint{{X: x, Y: mainY}}
		}
	}
	line = append(line, MapPoint{X: width, Y: mainY})
	lineSegments = append(lineSegments, line)
	geometry.Line = MapPath{Subpaths: lineSegments}
	return geometry
}

// mapPlanFlatStages returns each route's stages when every segment of every route is a stage, and
// false otherwise.
func mapPlanFlatStages(routes [][]BeltLayoutSegment) ([][]Stage, bool) {
	out := make([][]Stage, 0, len(routes))
	for _, route := range routes {
		var stages []Stage
		for _, segment := range route {
			if segment.Kind != BeltLayoutSegmentStage {
				return nil, false
			}
			stages = append(stages, *segment.Stage)
		}
		out = append(out, stages)
	}
	return out, true
}

// mapPlanZip is the routes of a fork that have a sequence as well, up to the shorter of the two.
func mapPlanZip(routes []Route, count int) []Route {
	return routes[:min(len(routes), count)]
}

// lane is a route lane from split to join. It runs along the main line far enough on each
// side for rounded corners to meet cleanly.
func (p MapPlan) lane(fork string, route Route, start, end, mainY, y, inside, column float64) MapGeometryLane {
	lane := NewMapLane(MapLaneArgs{Start: start, End: end, MainY: mainY, Y: y, Thickness: p.Metrics.Thickness})
	points := make([]MapPoint, 0, len(lane.Points)+2)
	for _, point := range lane.Points {
		points = append(points, MapPoint(point))
	}
	if len(points) > 2 {
		points = append([]MapPoint{{X: start - p.Metrics.Corner, Y: mainY}}, points...)
		points = append(points, MapPoint{X: end + p.Metrics.Corner, Y: mainY})
	}
	room := lane.Room(inside, column, p.Metrics.Cell)
	return MapGeometryLane{Fork: fork, Route: route, Y: y, Path: NewMapPathPoints(points, p.Metrics.Corner), LabelX: lane.LabelX, Room: room}
}

// place places a fork, including nested forks, inside its parent route band. It appends
// to geometry and returns the x where the fork ends.
func (p MapPlan) place(fork Fork, routes [][]BeltLayoutSegment, geometry *MapGeometry, x, mainY, column float64) float64 {
	heights := make([]float64, 0, len(routes))
	for _, route := range routes {
		heights = append(heights, p.band(route))
	}
	gap := p.Metrics.RowGap
	total := metroSumF(heights) + float64(float64(max(len(heights)-1, 0))*gap)
	// Center inside the containing route; nested bands use their parent's center.
	top := mainY - p.Metrics.Track + p.Metrics.Cell/2 - total/2
	start := x
	inside := x + p.Metrics.Pad
	end := inside + float64(float64(BeltLayoutColumns(BeltLayoutSegment{Kind: BeltLayoutSegmentFork, Fork: &fork, Routes: routes}))*column) + p.Metrics.Pad
	for index, route := range fork.Routes[:min(len(fork.Routes), len(routes))] {
		segments := routes[index]
		bandHeight := p.band(segments)
		y := top + bandHeight/2 - p.Metrics.Cell/2 + p.Metrics.Track
		cursor := inside
		if len(segments) == 0 {
			cursor = end - p.Metrics.Pad
		}
		for _, segment := range segments {
			switch segment.Kind {
			case BeltLayoutSegmentStage:
				geometry.Cells = append(geometry.Cells, MapGeometryPlaced{
					Stage: *segment.Stage, Number: p.Layout.Number(segment.Stage.ID), X: cursor + column/2,
					Top: top + (bandHeight-p.Metrics.Cell)/2,
				})
				cursor += column
			case BeltLayoutSegmentFork:
				cursor = p.place(*segment.Fork, segment.Routes, geometry, cursor, y, column)
			}
		}
		geometry.Lanes = append(geometry.Lanes, p.lane(fork.ID, route, start, end, mainY, y, inside, column))
		top += bandHeight + gap
	}
	geometry.Forks = append(geometry.Forks, MapGeometrySpan{ID: fork.ID, Start: start, End: end})
	return end
}

// band is the height one route takes, nested forks included.
func (p MapPlan) band(route []BeltLayoutSegment) float64 {
	return max(p.Metrics.Cell, p.tallest(route))
}

// tallest is the height of the tallest of segments, a fork's routes stacked with a row gap between
// them, or a cell when there are none.
func (p MapPlan) tallest(segments []BeltLayoutSegment) float64 {
	height := p.Metrics.Cell
	for i, segment := range segments {
		value := p.Metrics.Cell
		if segment.Kind == BeltLayoutSegmentFork {
			heights := make([]float64, 0, len(segment.Routes))
			for _, route := range segment.Routes {
				heights = append(heights, p.band(route))
			}
			value = metroSumF(heights) + float64(max(len(heights)-1, 0))*p.Metrics.RowGap
		}
		if i == 0 || value > height {
			height = value
		}
	}
	return height
}

// MapGeometryPlaced is a stage's box on the map.
type MapGeometryPlaced struct {
	Stage  Stage
	Number int
	X      float64
	Top    float64
}

// MapGeometryLane is one route's track on the map.
type MapGeometryLane struct {
	Fork   string
	Route  Route
	Y      float64
	Path   MapPath
	LabelX float64
	Room   float64
}

// MapGeometrySpan is the horizontal extent of one fork.
type MapGeometrySpan struct {
	ID    string  `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// MapGeometry is every belt position after the content width is known.
type MapGeometry struct {
	Width  float64
	Height float64
	Column float64
	MainY  float64
	Layout BeltLayout
	Cells  []MapGeometryPlaced
	Lanes  []MapGeometryLane
	Forks  []MapGeometrySpan
	// Line is the track outside forks, including separate stretches before and after each fork.
	Line MapPath
}

// Path is the track the run follows, with undecided forks taking their first route.
func (g MapGeometry) Path(routes map[string][]string) []MapPath {
	var paths []MapPath
	for _, sub := range g.Line.Subpaths {
		paths = append(paths, NewMapPathPoints(sub, 0))
	}
	for _, span := range g.Forks {
		taken, ok := routes[span.ID]
		if !ok {
			taken = nil
			if fork := g.Layout.Fork(span.ID); fork != nil && len(fork.Routes) > 0 {
				taken = []string{fork.Routes[0].ID}
			}
		}
		for _, lane := range g.Lanes {
			if lane.Fork == span.ID && slices.Contains(taken, lane.Route.ID) {
				paths = append(paths, lane.Path)
			}
		}
	}
	return paths
}

// Left is the left edge of a stage's column, where filled track stops.
func (g MapGeometry) Left(stageID string) float64 {
	for _, cell := range g.Cells {
		if cell.Stage.ID == stageID {
			return cell.X - g.Column/2
		}
	}
	return 0
}

// Crate is the crate location for a single run or one unfinished split leg.
func (g MapGeometry) Crate(fill *string, finished bool, routes map[string][]string) MapPoint {
	if finished {
		return MapPoint{X: g.Width - 21, Y: g.MainY}
	}
	var cell *MapGeometryPlaced
	if fill != nil {
		for index := range g.Cells {
			if g.Cells[index].Stage.ID == *fill {
				cell = &g.Cells[index]
				break
			}
		}
	}
	if cell == nil {
		return MapPoint{X: 21, Y: g.MainY}
	}
	y := g.MainY
	if place := g.Layout.Place(*fill); place != nil {
		for _, lane := range g.Lanes {
			if lane.Route.ID == place.Route.ID {
				y = lane.Y
				break
			}
		}
	}
	return MapPoint{X: cell.X, Y: y}
}

// BeltLayoutColumns is how many columns a segment takes when drawn: one per
// stage, and a fork as many as its longest route, counting a nested fork as
// its own width.
func BeltLayoutColumns(segment BeltLayoutSegment) int {
	if segment.Kind == BeltLayoutSegmentStage {
		return 1
	}
	widest := 0
	for _, route := range segment.Routes {
		sum := 0
		for _, seg := range route {
			sum += BeltLayoutColumns(seg)
		}
		widest = max(widest, sum)
	}
	return max(1, widest)
}
