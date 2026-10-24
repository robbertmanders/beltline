// FloorLayout, Metrics, the
// placed node, strokes and lane routes. The placement arithmetic is in
// present_floor_layout_plan.go and the path data in present_svg_path.go.
//
// Where everything on the factory floor goes, in points, with the track and
// lane paths already drawn as SVG path data. Placement is here so the app and
// the browser draw one layout: each layer keeps
// its order and spacing, sits as near as it can to its tracks, and return
// lanes drop below the stages they pass. The intrinsic drawing uses the
// minimum column, so a narrower screen scrolls the map rather than squeezing it;
// the app passes its width to spread the columns over a wider one.

package core

import "strings"

// FloorLayoutMetrics are the sizes the floor is placed with, in points.
type FloorLayoutMetrics struct {
	// EntranceWidth and EntranceHeight are the belts' entrances on the left.
	EntranceWidth  float64
	EntranceHeight float64
	MinColumn      float64
	// Track is where the track runs, from the top of a stage's cell.
	Track    float64
	Waypoint float64
	Gap      float64
	// Thickness is the width of every track, however many belts share it: a shared stretch is told
	// apart by its colour and its belts, not by a heavier line.
	Thickness float64
	// Corner is how round a track's corners are.
	Corner float64
	// Lane is the space between lanes that go back under the same stages.
	Lane float64
	// Clearance is extra room between a stage's bottom and the return lanes that run under it, beyond the
	// track's half width, so a lane's label clears the node above it.
	Clearance float64
	// Box is the width of the boxes the nodes are drawn as: tracks run from each box's edge and turn half
	// way across the gap.
	Box float64
}

// FloorLayoutPlaced is a node's box, as the drawing places it: its left edge (X is the centre
// of its column), its top, its height, and the Y its track runs at (Anchor).
type FloorLayoutPlaced struct {
	Node   FactoryTrackNode `json:"node"`
	X      float64          `json:"x"`
	Top    float64          `json:"top"`
	Height float64          `json:"height"`
	Anchor float64          `json:"anchor"`
}

// FloorLayoutStroke is a stretch of track between two nodes, drawn as one path.
type FloorLayoutStroke struct {
	From  FactoryTrackNode `json:"from"`
	To    FactoryTrackNode `json:"to"`
	Belts []string         `json:"belts"`
	Path  string           `json:"path"`
	Width float64          `json:"width"`
}

// FloorLayoutLaneRoute is a dotted lane back to an earlier stage, with its arrow head and the names of
// the belts that take it, centred on LabelX, LabelY: down at Right, back along LabelY and up into the
// earlier stage at Left.
type FloorLayoutLaneRoute struct {
	From   FactoryTrackNode `json:"from"`
	To     FactoryTrackNode `json:"to"`
	Belts  []string         `json:"belts"`
	Left   float64          `json:"-"`
	Right  float64          `json:"-"`
	Path   string           `json:"path"`
	Arrow  string           `json:"arrow"`
	Label  string           `json:"label"`
	LabelX float64          `json:"labelX"`
	LabelY float64          `json:"labelY"`
}

// FloorLayout is where everything on the factory floor goes, in points.
type FloorLayout struct {
	Width   float64                `json:"width"`
	Height  float64                `json:"height"`
	Column  float64                `json:"column"`
	Placed  []FloorLayoutPlaced    `json:"placed"`
	Tracks  []FloorLayoutStroke    `json:"tracks"`
	Lanes   []FloorLayoutLaneRoute `json:"lanes"`
	Metrics FloorLayoutMetrics     `json:"-"`
}

// floorLayoutLabelSeparator joins a lane's belt names. Copy from FloorLayout.init.
const floorLayoutLabelSeparator = " \u00b7 "

// NewFloorLayout places track at width, or at the minimum column when width is nil or too narrow.
// cellHeight gives each stage's cell height.
func NewFloorLayout(track FactoryTrack, m FloorLayoutMetrics, width *float64, cellHeight func(merged FactoryTrackStage) float64) FloorLayout {
	plan := newFloorLayoutPlan(track, m, cellHeight)
	column := m.MinColumn
	if width != nil {
		column = max(m.MinColumn, (*width-m.EntranceWidth)/float64(max(len(track.Layers)-1, 1)))
	}
	tops := plan.tops()
	var placed []FloorLayoutPlaced
	positions := map[FactoryTrackNode]FloorLayoutPlaced{}
	for l, layer := range track.Layers {
		x := m.EntranceWidth / 2
		if l != 0 {
			x = m.EntranceWidth + float64((float64(l)-0.5)*column)
		}
		for _, node := range layer {
			box := FloorLayoutPlaced{Node: node, X: x, Top: tops[node], Height: plan.Heights[node], Anchor: tops[node] + plan.offset(node)}
			placed = append(placed, box)
			positions[node] = box
		}
	}
	lanePlans := plan.lanes(positions, column)
	layout := FloorLayout{
		Placed:  placed,
		Column:  column,
		Metrics: m,
		Width:   m.EntranceWidth + float64(float64(max(len(track.Layers)-1, 0))*column),
	}
	// Room for the lowest lane and its belt's tag.
	bottom := 0.0
	for _, box := range placed {
		bottom = max(bottom, box.Top+box.Height)
	}
	lowest := 0.0
	for _, lane := range lanePlans {
		lowest = max(lowest, lane.Y)
	}
	layout.Height = max(bottom, lowest+10)
	for _, stretch := range track.Tracks {
		layout.Tracks = append(layout.Tracks, FloorLayoutStroke{
			From: stretch.From, To: stretch.To, Belts: stretch.Belts,
			Path: plan.path(stretch, positions, track), Width: m.Thickness,
		})
	}
	for _, lane := range lanePlans {
		path, arrow := plan.lanePath(lane)
		var names []string
		for _, id := range lane.Stretch.Belts {
			if belt := track.Belt(id); belt != nil {
				names = append(names, FactoryTrackShortName(belt.Name))
			}
		}
		label := UppercasedFull(strings.Join(names, floorLayoutLabelSeparator))
		layout.Lanes = append(layout.Lanes, FloorLayoutLaneRoute{
			From: lane.Stretch.From, To: lane.Stretch.To, Belts: lane.Stretch.Belts,
			Left: lane.Left, Right: lane.Right, Path: path, Arrow: arrow, Label: label,
			LabelX: (lane.Left + lane.Right) / 2, LabelY: lane.Y,
		})
	}
	return layout
}

// Box is where a node is placed, or nil.
func (l FloorLayout) Box(node FactoryTrackNode) *FloorLayoutPlaced {
	for index := range l.Placed {
		if l.Placed[index].Node == node {
			return &l.Placed[index]
		}
	}
	return nil
}

// FloorLayoutMinimumWidth is the width a layout needs at the minimum column.
func FloorLayoutMinimumWidth(track FactoryTrack, m FloorLayoutMetrics) float64 {
	return m.EntranceWidth + float64(float64(max(len(track.Layers)-1, 0))*m.MinColumn)
}
