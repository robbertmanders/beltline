package core

import "math"

// BoxLayoutMap lays out a belt's stages on the grid, with the trigger in front when showsTrigger. look gives each
// stage its icon and colour. trigger is the belt's trigger kind (nil for the issues glyph).
func BoxLayoutMap(layout BeltLayout, showsTrigger bool, trigger *TriggerKind, look func(stage Stage) StageLook) BoxMap {
	builder := newBoxLayoutBuilder(layout, look)
	var tails []boxLayoutTail
	column := 0
	if showsTrigger {
		node := builder.place("trigger", BoxMapKindTrigger, 0, 0, 0, nil, nil,
			StageLook{Icon: StageLookTrigger(trigger), Tint: TintGrey}, false)
		tails = []boxLayoutTail{{Node: node, Route: nil}}
		column = 1
	}
	result := builder.lay(layout.Segments(), column, 0, tails, nil, nil)
	return BoxMap{Columns: max(result.Column, 1), Nodes: builder.nodes, Edges: builder.edges, Routes: builder.routes}
}

// boxLayoutTail is a track still to be joined to what comes next. It leaves node from its right
// edge, turns the corners in via (a route's entry from its fork's spine, along the way), and runs
// on route when that is its track.
type boxLayoutTail struct {
	Node  string
	Route *string
	Via   []BoxMapTurn
}

// boxLayoutResult is where a laid-out run of segments ends: its open tails and the next column.
type boxLayoutResult struct {
	Tails  []boxLayoutTail
	Column int
}

// boxLayoutBuilder accumulates the boxes, edges and route pills of one map as it lays the belt out.
type boxLayoutBuilder struct {
	layout BeltLayout
	look   func(stage Stage) StageLook
	nodes  []BoxMapNode
	edges  []BoxMapEdge
	routes []BoxMapRoutePill
}

// newBoxLayoutBuilder is an empty builder for layout, with look giving each stage's look.
func newBoxLayoutBuilder(layout BeltLayout, look func(stage Stage) StageLook) boxLayoutBuilder {
	return boxLayoutBuilder{layout: layout, look: look, nodes: []BoxMapNode{}, edges: []BoxMapEdge{}, routes: []BoxMapRoutePill{}}
}

// boxLayoutSameRow reports whether two rows are one: rows are halved when a fork splits, so they compare with slack.
func boxLayoutSameRow(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

// band is how many rows a run of segments takes: one, or a fork's routes stacked.
func (b *boxLayoutBuilder) band(segments []BeltLayoutSegment) int {
	widest := 1
	for _, segment := range segments {
		rows := 1
		if segment.Kind == BeltLayoutSegmentFork {
			rows = 0
			for _, route := range segment.Routes {
				rows += b.band(route)
			}
		}
		widest = max(widest, rows)
	}
	return widest
}

// place adds a box in column on the row centred at row, and returns its id. tinted is false for the
// trigger, which is drawn dark.
func (b *boxLayoutBuilder) place(id string, kind BoxMapKind, number, column int, row float64, fork, route *string,
	look StageLook, tinted bool) string {
	var tint *Tint
	if tinted {
		value := look.Tint
		tint = &value
	}
	b.nodes = append(b.nodes, BoxMapNode{
		ID: id, Kind: kind, Number: number, Fork: fork, Route: route,
		Column: column, Row: row, Icon: look.Icon, Tint: tint,
	})
	return id
}

// node is the box with id, or nil.
func (b *boxLayoutBuilder) node(id string) *BoxMapNode {
	for index := range b.nodes {
		if b.nodes[index].ID == id {
			return &b.nodes[index]
		}
	}
	return nil
}

// entering is the track from a tail into a route of the fork whose spine is on the gap in front of spine: along
// the tail's row to the spine, then along the spine to the route's row. A tail that already runs along an outer
// route's row turns off it here.
func (b *boxLayoutBuilder) entering(tail boxLayoutTail, spine int, row float64, route *string) boxLayoutTail {
	from := row
	switch source := b.node(tail.Node); {
	case len(tail.Via) > 0:
		from = tail.Via[len(tail.Via)-1].Row
	case source != nil:
		from = source.Row
	}
	via := make([]BoxMapTurn, 0, len(tail.Via)+2)
	via = append(via, tail.Via...)
	via = append(via, BoxMapTurn{Column: spine, Row: from}, BoxMapTurn{Column: spine, Row: row})
	return boxLayoutTail{Node: tail.Node, Route: route, Via: via}
}

// connect joins each tail to node: from the source's right edge, through the corners the tail has turned, then,
// when the track has to change rows, down or up on the gap in front of node. A track that runs on a route is
// that route's.
func (b *boxLayoutBuilder) connect(tails []boxLayoutTail, node string) {
	target := b.node(node)
	if target == nil {
		return
	}
	for _, tail := range tails {
		source := b.node(tail.Node)
		if source == nil {
			continue
		}
		via := []BoxMapTurn{}
		row := source.Row
		add := func(turn BoxMapTurn) {
			if len(via) > 0 && via[len(via)-1].Column == turn.Column && boxLayoutSameRow(via[len(via)-1].Row, turn.Row) {
				return
			}
			via = append(via, turn)
			row = turn.Row
		}
		for _, turn := range tail.Via {
			add(turn)
		}
		if !boxLayoutSameRow(row, target.Row) {
			add(BoxMapTurn{Column: target.Column, Row: row})
			add(BoxMapTurn{Column: target.Column, Row: target.Row})
		}
		route := tail.Route
		if route == nil {
			route = target.Route
		}
		b.edges = append(b.edges, BoxMapEdge{From: source.ID, To: target.ID, Via: via, Route: route})
	}
}

// lay lays segments from column along the row centred at row, joining the first to tails. A fork
// splits its row: its routes stack evenly above and below it.
func (b *boxLayoutBuilder) lay(segments []BeltLayoutSegment, column int, row float64, tails []boxLayoutTail, fork, route *string) boxLayoutResult {
	for _, segment := range segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			tails = b.layStage(*segment.Stage, column, row, tails, fork, route)
			column++
		case BeltLayoutSegmentFork:
			tails = b.layFork(segment, column, row, tails)
			column += boxMapLead + BeltLayoutColumns(segment)
		}
	}
	return boxLayoutResult{Tails: tails, Column: column}
}

// layStage lays one stage in column on row, joined to tails, and returns the tails the line goes on from.
func (b *boxLayoutBuilder) layStage(stage Stage, column int, row float64, tails []boxLayoutTail, fork, route *string) []boxLayoutTail {
	kind := BoxMapKindAgent
	switch stage.Kind() {
	case StageKindGate:
		kind = BoxMapKindApproval
	case StageKindWatch:
		kind = BoxMapKindWatch
	}
	id := b.place(stage.ID, kind, b.layout.Number(stage.ID), column, row, fork, route, b.look(stage), true)
	b.connect(tails, id)
	return []boxLayoutTail{{Node: id, Route: route}}
}

// layFork lays a fork segment from column on row: each route in its band of rows, its spine on the gap in front
// of column. It returns the tails of the routes that join the line again.
func (b *boxLayoutBuilder) layFork(segment BeltLayoutSegment, column int, row float64, tails []boxLayoutTail) []boxLayoutTail {
	child := *segment.Fork
	routeSegments := segment.Routes
	bands := make([]int, 0, len(routeSegments))
	total := 0
	for _, routeSegment := range routeSegments {
		rows := b.band(routeSegment)
		bands = append(bands, rows)
		total += rows
	}
	top := row - float64(total)/2
	var next []boxLayoutTail
	for index := 0; index < min(len(child.Routes), len(routeSegments)); index++ {
		routeModel := child.Routes[index]
		routeRow := top + float64(bands[index])/2
		top += float64(bands[index])
		laid := b.layForkRoute(child.ID, routeModel, routeSegments[index], column, routeRow, tails)
		if b.layout.Joins(routeModel.ID) {
			next = append(next, laid.Tails...)
		}
	}
	return next
}

// layForkRoute lays one route of the fork forkID on its row: its name pill on the lead of track from the fork's
// spine, its boxes after the lead, and the tracks into it from tails when it opens off the line. An empty route's
// track runs from the spine along its row to where the routes meet. It returns where the route's boxes end.
func (b *boxLayoutBuilder) layForkRoute(forkID string, routeModel Route, segments []BeltLayoutSegment, column int, row float64, tails []boxLayoutTail) boxLayoutResult {
	stageIDs := []string{}
	for _, segment := range segments {
		for _, stage := range segment.Stages() {
			stageIDs = append(stageIDs, stage.ID)
		}
	}
	b.routes = append(b.routes, BoxMapRoutePill{
		Fork: forkID, Route: routeModel.ID, Name: routeModel.Name,
		Column: column, Row: row, StageIDs: stageIDs, Feeds: routeModel.Feeds,
	})
	var entry []boxLayoutTail
	if b.layout.Opens(routeModel.ID) {
		var routeRef *string
		if len(segments) == 0 {
			id := routeModel.ID
			routeRef = &id
		}
		for _, tail := range tails {
			entry = append(entry, b.entering(tail, column, row, routeRef))
		}
	}
	routeID := routeModel.ID
	return b.lay(segments, column+boxMapLead, row, entry, &forkID, &routeID)
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
