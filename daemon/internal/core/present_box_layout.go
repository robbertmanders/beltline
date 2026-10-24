package core

import "slices"
import "math"

// BoxLayoutMap lays out a belt's stages, with the trigger in front when showsTrigger. look gives each
// stage its icon and colour. trigger is the belt's trigger kind (nil for the issues glyph).
func BoxLayoutMap(layout BeltLayout, metrics BoxMapMetrics, showsTrigger bool, trigger *TriggerKind, look func(stage Stage) StageLook) BoxMap {
	builder := newBoxLayoutBuilder(layout, metrics, look)
	var tails []boxLayoutTail
	column := 0.0
	if showsTrigger {
		node := builder.place("trigger", BoxMapKindTrigger, 0, 0, 0, nil, nil,
			StageLook{Icon: StageLookTrigger(trigger), Tint: TintGrey}, false)
		tails = []boxLayoutTail{{Node: node, Route: nil}}
		column = 1
	}
	result := builder.lay(layout.Segments(), column, 0, tails, nil, nil)
	// The + past the last box of the line: where the next box would go.
	var end *string
	if len(result.Tails) > 0 {
		node := result.Tails[0].Node
		end = &node
	}
	insertX := builder.x(0) + metrics.NodeWidth/2
	if result.Column != 0 {
		insertX = builder.x(result.Column) - metrics.Gap/2
	}
	builder.addInsert(insertX, 0, end, nil, nil, len(layout.Stages))
	// The line runs through the middle; move everything down so the highest row sits at the top padding. An empty
	// route has no box to measure by, so its row is measured as a box's would be, or it would fall off the map.
	type span struct{ top, bottom float64 }
	var spans []span
	for _, node := range builder.nodes {
		spans = append(spans, span{node.Y, node.Y + node.Height})
	}
	for _, row := range builder.emptyRows {
		spans = append(spans, span{row - metrics.NodeHeight/2, row + metrics.NodeHeight/2})
	}
	top := -metrics.NodeHeight / 2
	bottom := metrics.NodeHeight / 2
	for index, s := range spans {
		if index == 0 || s.top < top {
			top = s.top
		}
		if index == 0 || s.bottom > bottom {
			bottom = s.bottom
		}
	}
	if len(spans) == 0 {
		top = -metrics.NodeHeight / 2
		bottom = metrics.NodeHeight / 2
	}
	builder.shift(metrics.PadTop - top)
	width := float64(metrics.PadX*2) + float64(max(result.Column, 1)*(metrics.NodeWidth+metrics.Gap))
	height := bottom - top + metrics.PadTop + metrics.PadBottom
	return BoxMap{
		Width: width, Height: height, Nodes: builder.nodes, Edges: builder.edges, Forks: builder.forks,
		Routes: builder.routes, Inserts: builder.inserts,
	}
}

// boxLayoutTail is a track still to be joined to what comes next. It leaves node from its right
// edge, turns the corners in via (a route's entry from its fork's spine, along the way), and runs
// on route when that is its track.
type boxLayoutTail struct {
	Node  string
	Route *string
	Via   []MapPoint
}

// boxLayoutResult is where a laid-out run of segments ends: its open tails and the next column.
type boxLayoutResult struct {
	Tails  []boxLayoutTail
	Column float64
}

// boxLayoutBuilder accumulates the boxes, edges, fork areas, route pills and inserts of one
// map as it lays the belt out.
type boxLayoutBuilder struct {
	layout  BeltLayout
	metrics BoxMapMetrics
	look    func(stage Stage) StageLook
	nodes   []BoxMapNode
	edges   []BoxMapEdge
	forks   []BoxMapForkArea
	routes  []BoxMapRoutePill
	inserts []BoxMapInsert
	// emptyRows are the centres of each empty route's row, before the map is shifted: no node marks these rows.
	emptyRows []float64
}

// newBoxLayoutBuilder is an empty builder for layout at metrics, with look giving each stage's look.
func newBoxLayoutBuilder(layout BeltLayout, metrics BoxMapMetrics, look func(stage Stage) StageLook) boxLayoutBuilder {
	return boxLayoutBuilder{
		layout:    layout,
		metrics:   metrics,
		look:      look,
		nodes:     []BoxMapNode{},
		edges:     []BoxMapEdge{},
		forks:     []BoxMapForkArea{},
		routes:    []BoxMapRoutePill{},
		inserts:   []BoxMapInsert{},
		emptyRows: []float64{},
	}
}

// x is the left edge of a column.
func (b *boxLayoutBuilder) x(column float64) float64 {
	return b.metrics.PadX + float64(column*(b.metrics.NodeWidth+b.metrics.Gap))
}

// band is how many rows a run of segments takes: one, or a fork's routes stacked.
func (b *boxLayoutBuilder) band(segments []BeltLayoutSegment) int {
	widest := 0
	for index, segment := range segments {
		rows := 1
		if segment.Kind == BeltLayoutSegmentFork {
			rows = 0
			for _, route := range segment.Routes {
				rows += b.band(route)
			}
		}
		if index == 0 || rows > widest {
			widest = rows
		}
	}
	if len(segments) == 0 {
		return 1
	}
	return max(widest, 1)
}

// shift moves every drawn thing down by dy.
func (b *boxLayoutBuilder) shift(dy float64) {
	for index := range b.nodes {
		b.nodes[index].Y += dy
	}
	for index := range b.edges {
		subpaths := make([][]MapPoint, 0, len(b.edges[index].Path.Subpaths))
		for _, subpath := range b.edges[index].Path.Subpaths {
			moved := make([]MapPoint, 0, len(subpath))
			for _, point := range subpath {
				moved = append(moved, MapPoint{X: point.X, Y: point.Y + dy})
			}
			subpaths = append(subpaths, moved)
		}
		b.edges[index].Path.Subpaths = subpaths
	}
	for index := range b.forks {
		b.forks[index].Y += dy
	}
	for index := range b.routes {
		b.routes[index].Y += dy
	}
	for index := range b.inserts {
		b.inserts[index].Y += dy
	}
}

// place adds a box centred on centre in column, and returns its id. tinted is false for the
// trigger, which is drawn dark.
func (b *boxLayoutBuilder) place(id string, kind BoxMapKind, number int, column, centre float64, fork, route *string,
	look StageLook, tinted bool) string {
	var tint *Tint
	if tinted {
		value := look.Tint
		tint = &value
	}
	b.nodes = append(b.nodes, BoxMapNode{
		ID: id, Kind: kind, Number: number, Fork: fork, Route: route,
		X: b.x(column), Y: centre - b.metrics.NodeHeight/2,
		Width: b.metrics.NodeWidth, Height: b.metrics.NodeHeight, Icon: look.Icon, Tint: tint,
	})
	return id
}

// index is where a stage added at this point of the stage list goes: the index of a stage, or the
// list's length when stageID is nil or unknown.
func (b *boxLayoutBuilder) index(stageID *string) int {
	if stageID != nil {
		for index, stage := range b.layout.Stages {
			if stage.ID == *stageID {
				return index
			}
		}
	}
	return len(b.layout.Stages)
}

// addInsert adds a + at a point. Two can fall on one spot: a route's end and the box after its fork, when the
// route fills the fork and runs straight into it. The + in front of the box keeps the spot (the line's stage goes
// there); a route's end gives way to it, and any other + already there stays.
func (b *boxLayoutBuilder) addInsert(x, y float64, after, before, route *string, addAt int) {
	insert := BoxMapInsert{X: x, Y: y, After: after, Before: before, Route: route, AddAt: addAt}
	at := -1
	for index, existing := range b.inserts {
		if math.Abs(existing.X-x) < 0.5 && math.Abs(existing.Y-y) < 0.5 {
			at = index
			break
		}
	}
	if at < 0 {
		b.inserts = append(b.inserts, insert)
		return
	}
	if before != nil && b.inserts[at].Before == nil && b.inserts[at].Route != nil {
		b.inserts[at] = insert
	}
}

// entering is the track from a tail into a route of the fork whose spine is at spine: along the tail's row
// to the spine, then along the spine to the route's row. A tail that already runs along an outer route's row turns off it here.
func (b *boxLayoutBuilder) entering(tail boxLayoutTail, spine, row float64, route *string) boxLayoutTail {
	var source *BoxMapNode
	for index := range b.nodes {
		if b.nodes[index].ID == tail.Node {
			source = &b.nodes[index]
			break
		}
	}
	var y float64
	switch {
	case len(tail.Via) > 0:
		y = tail.Via[len(tail.Via)-1].Y
	case source != nil:
		y = source.Y + source.Height/2
	default:
		y = row
	}
	via := make([]MapPoint, 0, len(tail.Via)+2)
	via = append(via, tail.Via...)
	via = append(via, MapPoint{X: spine, Y: y}, MapPoint{X: spine, Y: row})
	return boxLayoutTail{Node: tail.Node, Route: route, Via: via}
}

// connect joins each tail to node: from the node's right edge, through the corners the tail has turned, then, when the
// track has to change rows, down or up on the gap before node. A track that runs on a route is that route's.
func (b *boxLayoutBuilder) connect(tails []boxLayoutTail, node string) {
	var target *BoxMapNode
	for index := range b.nodes {
		if b.nodes[index].ID == node {
			target = &b.nodes[index]
			break
		}
	}
	if target == nil {
		return
	}
	x2 := target.X
	y2 := target.Y + target.Height/2
	for _, tail := range tails {
		var source *BoxMapNode
		for index := range b.nodes {
			if b.nodes[index].ID == tail.Node {
				source = &b.nodes[index]
				break
			}
		}
		if source == nil {
			continue
		}
		start := MapPoint{X: source.X + source.Width, Y: source.Y + source.Height/2}
		points := []MapPoint{start}
		add := func(point MapPoint) {
			last := points[len(points)-1]
			if math.Abs(last.X-point.X) >= 0.5 || math.Abs(last.Y-point.Y) >= 0.5 {
				points = append(points, point)
			}
		}
		for _, via := range tail.Via {
			add(via)
		}
		if last := points[len(points)-1]; math.Abs(last.Y-y2) >= 0.5 {
			turn := x2 - b.metrics.Gap/2
			add(MapPoint{X: turn, Y: last.Y})
			add(MapPoint{X: turn, Y: y2})
		}
		add(MapPoint{X: x2, Y: y2})
		straight := !slices.ContainsFunc(points, func(point MapPoint) bool {
			return !(math.Abs(point.Y-start.Y) < 0.5)
		})
		route := tail.Route
		if route == nil {
			route = target.Route
		}
		b.edges = append(b.edges, BoxMapEdge{
			From: source.ID, To: target.ID, Path: NewMapPathPoints(points, b.metrics.Corner),
			Straight: straight, Route: route,
		})
	}
}

// lay lays segments from column along the row whose middle is centre, joining the first to tails. A fork
// splits its row: its routes stack evenly above and below it. Every box gets the +
// in front of it, and every route its + at the end (or in its empty slot), so each place a stage can go is drawn.
func (b *boxLayoutBuilder) lay(segments []BeltLayoutSegment, column, centre float64, tails []boxLayoutTail, fork, route *string) boxLayoutResult {
	for _, segment := range segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			tails = b.layStage(*segment.Stage, column, centre, tails, fork, route)
			column++
		case BeltLayoutSegmentFork:
			width := b.metrics.Lead + float64(BeltLayoutColumns(segment))
			tails = b.layFork(segment, width, column, centre, tails, route)
			column += width
		}
	}
	return boxLayoutResult{Tails: tails, Column: column}
}

// layStage lays one stage at column on the row centre, joined to tails, and returns the tails the line goes on from.
func (b *boxLayoutBuilder) layStage(stage Stage, column, centre float64, tails []boxLayoutTail, fork, route *string) []boxLayoutTail {
	stageID := stage.ID
	b.addInsert(b.x(column)-b.metrics.Gap/2, centre, lastTailNode(tails), &stageID, route, b.index(&stageID))
	kind := BoxMapKindAgent
	switch stage.Kind() {
	case StageKindGate:
		kind = BoxMapKindApproval
	case StageKindWatch:
		kind = BoxMapKindWatch
	}
	id := b.place(stage.ID, kind, b.layout.Number(stage.ID), column, centre, fork, route, b.look(stage), true)
	b.connect(tails, id)
	return []boxLayoutTail{{Node: id, Route: route}}
}

// layFork lays a fork segment of the given width at column on the row centre: the line's + in front of its
// first box, then each route in its band of rows. It returns the tails of the routes that join the line again.
func (b *boxLayoutBuilder) layFork(segment BeltLayoutSegment, width, column, centre float64, tails []boxLayoutTail, route *string) []boxLayoutTail {
	child := *segment.Fork
	routeSegments := segment.Routes
	spine := b.x(column) - b.metrics.Gap/2
	first := firstForkStageID(routeSegments)
	// The + before the fork is the line's (or the enclosing route's) own, in front of its first box.
	b.addInsert(spine, centre, lastTailNode(tails), first, route, b.index(first))
	bands, totalRows := b.forkBands(routeSegments)
	total := float64(totalRows) * b.metrics.RowHeight
	top := centre - total/2
	firstBand := 1
	if len(bands) > 0 {
		firstBand = bands[0]
	}
	firstY := top + float64(float64(firstBand)*b.metrics.RowHeight)/2
	var next []boxLayoutTail
	for index := 0; index < min(len(child.Routes), len(routeSegments)); index++ {
		routeModel := child.Routes[index]
		rows := bands[index]
		routeY := top + float64(float64(rows)*b.metrics.RowHeight)/2
		top += float64(rows) * b.metrics.RowHeight
		laid := b.layForkRoute(child.ID, routeModel, routeSegments[index], spine, column, routeY, tails)
		if b.layout.Joins(routeModel.ID) {
			next = append(next, laid.Tails...)
		}
	}
	used := float64(totalRows)
	b.forks = append(b.forks, BoxMapForkArea{
		ID: child.ID, Routes: len(child.Routes),
		X: spine + 3, Y: firstY - b.metrics.NodeHeight/2 - 58,
		Width:  float64(width*(b.metrics.NodeWidth+b.metrics.Gap)) - 6,
		Height: float64((used-1)*b.metrics.RowHeight) + b.metrics.NodeHeight + 72,
	})
	return next
}

// layForkRoute lays one route of the fork forkID on its row routeY: its name pill, its boxes after the lead of
// track from the fork's column, the tracks into it from tails when it opens off the line, and its + at the end.
// It returns where the route's boxes end.
func (b *boxLayoutBuilder) layForkRoute(forkID string, routeModel Route, segments []BeltLayoutSegment, spine, column, routeY float64, tails []boxLayoutTail) boxLayoutResult {
	// A route's first box comes after its lead of track, where its name sits; an empty route's
	// track runs from the spine along its row to where the routes meet.
	start := column + b.metrics.Lead
	namesAt := spine
	if b.metrics.Lead > 0 {
		namesAt = (spine + b.x(start)) / 2
	}
	stageIDs := []string{}
	for _, stage := range boxLayoutStages(segments) {
		stageIDs = append(stageIDs, stage.ID)
	}
	b.routes = append(b.routes, BoxMapRoutePill{
		Fork: forkID, Route: routeModel.ID, Name: routeModel.Name,
		X: namesAt, Y: routeY, StageIDs: stageIDs, Feeds: routeModel.Feeds,
	})
	var entry []boxLayoutTail
	if b.layout.Opens(routeModel.ID) {
		var routeRef *string
		if len(segments) == 0 {
			id := routeModel.ID
			routeRef = &id
		}
		for _, tail := range tails {
			entry = append(entry, b.entering(tail, spine, routeY, routeRef))
		}
	}
	routeID := routeModel.ID
	laid := b.lay(segments, start, routeY, entry, &forkID, &routeID)
	if len(segments) == 0 {
		b.emptyRows = append(b.emptyRows, routeY)
	}
	b.addRouteEnd(routeModel.ID, segments, start, laid.Column, routeY)
	return laid
}

// addRouteEnd adds the + at the end of a route on row routeY: in the middle of its slot when it is empty, else
// after its last box, which ends at laidColumn.
func (b *boxLayoutBuilder) addRouteEnd(route string, segments []BeltLayoutSegment, start, laidColumn, routeY float64) {
	end := b.layout.End(route)
	if len(segments) == 0 {
		// An empty route's + sits in the middle of its slot.
		b.addInsert(b.x(start)+b.metrics.NodeWidth/2, routeY, nil, nil, &route, end)
		return
	}
	var lastID *string
	if lastStages := segments[len(segments)-1].Stages(); len(lastStages) > 0 {
		id := lastStages[len(lastStages)-1].ID
		lastID = &id
	}
	b.addInsert(b.x(laidColumn)-b.metrics.Gap/2, routeY, lastID, nil, &route, end)
}

// forkBands is how many rows each route of a fork takes, and their total.
func (b *boxLayoutBuilder) forkBands(routeSegments [][]BeltLayoutSegment) ([]int, int) {
	bands := make([]int, 0, len(routeSegments))
	total := 0
	for _, routeSegment := range routeSegments {
		rows := b.band(routeSegment)
		bands = append(bands, rows)
		total += rows
	}
	return bands, total
}

// firstForkStageID is the id of the first stage in a fork's routes, or nil when they hold none.
func firstForkStageID(routeSegments [][]BeltLayoutSegment) *string {
	for _, route := range routeSegments {
		for _, nested := range route {
			if stages := nested.Stages(); len(stages) > 0 {
				id := stages[0].ID
				return &id
			}
		}
	}
	return nil
}

// lastTailNode is the node the last of tails leaves from, or nil when there are none.
func lastTailNode(tails []boxLayoutTail) *string {
	if len(tails) == 0 {
		return nil
	}
	last := tails[len(tails)-1].Node
	return &last
}

// boxLayoutStages is the stages of a route's segments in order, nested forks included.
func boxLayoutStages(segments []BeltLayoutSegment) []Stage {
	var out []Stage
	for _, segment := range segments {
		out = append(out, segment.Stages()...)
	}
	return out
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
