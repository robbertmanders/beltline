//
// The placement arithmetic: heights, the layer tops, the lanes and the track paths.

package core

import "math"

// floorLayoutLanePlan is a return lane before it is drawn: its stretch, its horizontal extent and its row.
type floorLayoutLanePlan struct {
	Stretch FactoryTrackTrack
	Right   float64
	Left    float64
	From    float64
	To      float64
	Y       float64
}

// floorLayoutPlan is the placement arithmetic: heights, the layer tops, and the lanes.
type floorLayoutPlan struct {
	Track   FactoryTrack
	Metrics FloorLayoutMetrics
	Heights map[FactoryTrackNode]float64
}

// floorLayoutLaneTolerance is floating-point slack for the spacing between lanes, in points.
const floorLayoutLaneTolerance = 1e-6

// newFloorLayoutPlan measures every node of track: entrances and waypoints by the metrics, stages by cell.
func newFloorLayoutPlan(track FactoryTrack, metrics FloorLayoutMetrics, cell func(merged FactoryTrackStage) float64) floorLayoutPlan {
	heights := map[FactoryTrackNode]float64{}
	for _, layer := range track.Layers {
		for _, node := range layer {
			switch node.Kind {
			case FactoryTrackNodeEntrance:
				heights[node] = metrics.EntranceHeight
			case FactoryTrackNodeStage:
				if merged := track.Stage(node.ID); merged != nil {
					heights[node] = cell(*merged)
				} else {
					heights[node] = metrics.Track
				}
			case FactoryTrackNodeWaypoint:
				heights[node] = metrics.Waypoint
			}
		}
	}
	return floorLayoutPlan{Track: track, Metrics: metrics, Heights: heights}
}

// isStage reports whether a node is a stage.
func isStage(node FactoryTrackNode) bool {
	return node.Kind == FactoryTrackNodeStage
}

// offset is the track's height within a node.
func (p floorLayoutPlan) offset(node FactoryTrackNode) float64 {
	if isStage(node) {
		return p.Metrics.Track
	}
	return p.Heights[node] / 2
}

// tops is every node's top. Each layer keeps its order and spacing, and sits as close as it can
// to where its tracks come from, then to where they go.
func (p floorLayoutPlan) tops() map[FactoryTrackNode]float64 {
	top := map[FactoryTrackNode]float64{}
	for _, layer := range p.Track.Layers {
		y := 0.0
		for _, node := range layer {
			top[node] = y
			y += p.Heights[node] + p.Metrics.Gap
		}
	}
	count := len(p.Track.Layers)
	for pass := 0; pass < 5 && count > 1; pass++ {
		forwards := pass%2 == 0
		var order []int
		if forwards {
			for l := 1; l < count; l++ {
				order = append(order, l)
			}
		} else {
			for l := count - 2; l >= 0; l-- {
				order = append(order, l)
			}
		}
		for _, l := range order {
			layer := p.Track.Layers[l]
			desired := make([]float64, 0, len(layer))
			for _, node := range layer {
				var near []float64
				for _, t := range p.Track.Tracks {
					var other *FactoryTrackNode
					if forwards {
						if t.To == node {
							other = &t.From
						}
					} else if t.From == node {
						other = &t.To
					}
					if other != nil {
						near = append(near, top[*other]+p.offset(*other))
					}
				}
				var want float64
				if len(near) == 0 {
					want = top[node] + p.offset(node)
				} else {
					want = floorLayoutSum(near) / float64(len(near))
				}
				desired = append(desired, want-p.offset(node))
			}
			heights := make([]float64, 0, len(layer))
			for _, node := range layer {
				heights = append(heights, p.Heights[node])
			}
			for index, y := range p.spread(desired, heights) {
				top[layer[index]] = y
			}
		}
	}
	least := 0.0
	first := true
	for _, value := range top {
		if first || value < least {
			least = value
			first = false
		}
	}
	shifted := make(map[FactoryTrackNode]float64, len(top))
	for node, value := range top {
		shifted[node] = value - least
	}
	return shifted
}

// spread positions as near the desired ones as they can be without overlapping: overlapping
// neighbours move together, centred on where they want to be.
func (p floorLayoutPlan) spread(desired []float64, heights []float64) []float64 {
	type block struct {
		first, last int
		top         float64
	}
	blockTop := func(first, last int) float64 {
		offset, sum := 0.0, 0.0
		for i := first; i <= last; i++ {
			sum += desired[i] - offset
			offset += heights[i] + p.Metrics.Gap
		}
		return sum / float64(last-first+1)
	}
	bottom := func(b block) float64 {
		total := 0.0
		for i := b.first; i <= b.last; i++ {
			total += heights[i] + p.Metrics.Gap
		}
		return b.top + total
	}
	var blocks []block
	for i := range desired {
		blocks = append(blocks, block{i, i, desired[i]})
		for len(blocks) > 1 && blocks[len(blocks)-1].top < bottom(blocks[len(blocks)-2]) {
			last := blocks[len(blocks)-1]
			blocks = blocks[:len(blocks)-1]
			first := blocks[len(blocks)-1].first
			blocks = blocks[:len(blocks)-1]
			blocks = append(blocks, block{first, last.last, blockTop(first, last.last)})
		}
	}
	var out []float64
	for _, b := range blocks {
		for i := b.first; i <= b.last; i++ {
			total := 0.0
			for j := b.first; j < i; j++ {
				total += heights[j] + p.Metrics.Gap
			}
			out = append(out, b.top+total)
		}
	}
	return out
}

// lanes places each return lane just below the stages it passes under, in the first gap it fits,
// and below the lanes it overlaps.
func (p floorLayoutPlan) lanes(placed map[FactoryTrackNode]FloorLayoutPlaced, column float64) []floorLayoutLanePlan {
	half := p.Metrics.Thickness / 2
	// The stages are walked in layer order, so the lanes come out the same on every run.
	var boxes []FloorLayoutPlaced
	for _, layer := range p.Track.Layers {
		for _, node := range layer {
			if at, ok := placed[node]; ok && isStage(node) {
				boxes = append(boxes, at)
			}
		}
	}
	var lanes []floorLayoutLanePlan
	for _, stretch := range p.Track.Returns {
		a, okA := placed[stretch.From]
		b, okB := placed[stretch.To]
		if !okA || !okB {
			continue
		}
		right, left := a.X+column/2, b.X-column/2
		var under []FloorLayoutPlaced
		for _, cell := range boxes {
			if cell.X >= b.X && cell.X <= a.X {
				under = append(under, cell)
			}
		}
		var overlapping []floorLayoutLanePlan
		for _, lane := range lanes {
			if lane.Left < right && left < lane.Right {
				overlapping = append(overlapping, lane)
			}
		}
		below := half + p.Metrics.Clearance
		y := max(a.Anchor, b.Anchor) + half
		moved := true
		for moved {
			moved = false
			for _, cell := range under {
				if y > cell.Top-half && y < cell.Top+cell.Height+below {
					y = cell.Top + cell.Height + below
					moved = true
				}
			}
			// The tolerance stops a step that rounds a hair short of metrics.Lane from matching the same lane forever.
			for _, lane := range overlapping {
				if math.Abs(y-lane.Y) < p.Metrics.Lane-floorLayoutLaneTolerance {
					y = lane.Y + p.Metrics.Lane
					moved = true
				}
			}
		}
		lanes = append(lanes, floorLayoutLanePlan{Stretch: stretch, Right: right, Left: left, From: a.Anchor, To: b.Anchor, Y: y})
	}
	return lanes
}

// point is where a stretch's track runs at a node: the entrance's middle, or the node's anchor. ok is
// false when the node is not placed.
func (p floorLayoutPlan) point(node FactoryTrackNode, placed map[FactoryTrackNode]FloorLayoutPlaced) (x, y float64, ok bool) {
	at, found := placed[node]
	if !found {
		return 0, 0, false
	}
	if node.Kind == FactoryTrackNodeEntrance {
		return at.X - 10 + (p.Metrics.EntranceWidth-20)/2, at.Anchor, true
	}
	return at.X, at.Anchor, true
}

// path is straight out of one stage and into the next, turning a rounded corner each way where it
// changes rows: by the stage it splits from, by the one it joins at, or halfway.
func (p floorLayoutPlan) path(stretch FactoryTrackTrack, positions map[FactoryTrackNode]FloorLayoutPlaced, track FactoryTrack) string {
	ax, ay, okA := p.point(stretch.From, positions)
	bx, by, okB := p.point(stretch.To, positions)
	if !okA || !okB {
		return ""
	}
	if !(math.Abs(ay-by) > 0.5) {
		return svgLine([]svgPoint{{X: ax, Y: ay}, {X: bx, Y: by}})
	}
	// A box covers the track at its middle, so the track starts at each box's edge and turns in the gap.
	start := ax + boxHalf(isStage(stretch.From), p.Metrics.Box)
	end := bx - boxHalf(isStage(stretch.To), p.Metrics.Box)
	x := (start + end) / 2
	radius := max(0, min(p.Metrics.Corner, math.Abs(by-ay)/2, x-ax, bx-x))
	return svgRounded([]svgPoint{{X: ax, Y: ay}, {X: x, Y: ay}, {X: x, Y: by}, {X: bx, Y: by}}, radius)
}

// boxHalf is half a box's width where the node is a stage, else zero.
func boxHalf(merged bool, width float64) float64 {
	if merged {
		return width / 2
	}
	return 0
}

// lanePath is a lane's dotted path and the arrow head that marks where it ends.
func (p floorLayoutPlan) lanePath(lane floorLayoutLanePlan) (path string, arrow string) {
	spans := []float64{p.Metrics.Corner * 2, (lane.Y - lane.From) / 2, (lane.Y - lane.To) / 2, (lane.Right - lane.Left) / 4}
	radius := min(spans[0], spans[1], spans[2], spans[3])
	tip := svgPoint{X: lane.Left + radius + 12, Y: lane.To}
	body := svgRounded([]svgPoint{
		{X: lane.Right - radius, Y: lane.From}, {X: lane.Right, Y: lane.From}, {X: lane.Right, Y: lane.Y},
		{X: lane.Left, Y: lane.Y}, {X: lane.Left, Y: lane.To}, tip,
	}, radius)
	path = body + svgSegment(svgPoint{X: tip.X - 6, Y: tip.Y})
	arrow = svgMoveTo + svgNumber(tip.X) + " " + svgNumber(tip.Y) +
		svgLineTo + svgNumber(tip.X-9) + " " + svgNumber(tip.Y-6) +
		svgLineTo + svgNumber(tip.X-9) + " " + svgNumber(tip.Y+6) + " Z"
	return path, arrow
}

// floorLayoutSum is the sum of values, added from zero in order.
func floorLayoutSum(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}
