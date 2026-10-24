//
// The Floor: every merged stage as one stage box, each belt's entrance card on
// the left, and up to three run tags under each box. The layout is
// FloorLayout's, with these metrics; the browser draws the boxes where they say.

package core

import "strconv"

// Sizes of the Floor, in points.
const (
	// OverviewBoxesBoxWidth is a box's width: the run page's size.
	OverviewBoxesBoxWidth = 148.0
	// OverviewBoxesBoxHeight is a box's height: the run page's size.
	OverviewBoxesBoxHeight = 52.0
	// OverviewBoxesStatusHeight is the room above a box for its status.
	OverviewBoxesStatusHeight = 22.0
	// OverviewBoxesTagHeight is one run tag's row.
	OverviewBoxesTagHeight = 23.0
	// OverviewBoxesTagsGap is the gap above the tags.
	OverviewBoxesTagsGap = 6.0
	// OverviewBoxesTagLimit is the most run tags a box shows before one count of the rest.
	OverviewBoxesTagLimit = 3
)

// OverviewBoxesMetrics are the Floor's layout metrics for boxes: room between neighbours, so boxes and their
// run tags need air above and below, each return lane keeps its label clear of the boxes above it and of the
// next lane, and columns keep the turns apart.
func OverviewBoxesMetrics() FloorLayoutMetrics {
	return FloorLayoutMetrics{
		EntranceWidth:  220,
		EntranceHeight: 112,
		MinColumn:      216,
		Track:          OverviewBoxesStatusHeight + OverviewBoxesBoxHeight/2,
		Waypoint:       14,
		Gap:            22,
		Thickness:      8,
		Corner:         10,
		Lane:           26,
		Clearance:      12,
		Box:            OverviewBoxesBoxWidth,
	}
}

// Copy used by OverviewBoxes. Literal text is exact.
const (
	overviewBoxesRunning = " running"
)

// OverviewBoxesHeight is a box's cell height: its status, its box and its run tags.
func OverviewBoxesHeight(count int) float64 {
	rows := OverviewBoxesTagRows(count)
	tags := 0.0
	if rows > 0 {
		tags = OverviewBoxesTagsGap + float64(float64(rows)*OverviewBoxesTagHeight)
	}
	return OverviewBoxesStatusHeight + OverviewBoxesBoxHeight + tags
}

// OverviewBoxesTagRows is the rows of run tags a box has: its shown tags, and one more when some are left over.
func OverviewBoxesTagRows(count int) int {
	more := 0
	if count > OverviewBoxesTagLimit {
		more = 1
	}
	return min(count, OverviewBoxesTagLimit) + more
}

// OverviewBoxesFrame is the box a placed cell takes: its column's centre is the box's middle, and its top is
// the cell's.
func OverviewBoxesFrame(placed FloorLayoutPlaced) OverviewFloorFrame {
	return OverviewFloorFrame{X: placed.X - OverviewBoxesBoxWidth/2, Y: placed.Top, Width: OverviewBoxesBoxWidth, Height: placed.Height}
}

// OverviewBoxesCaption is what a stage's box is captioned: its kind, and how many belts share it.
func OverviewBoxesCaption(stage Stage, belts int) string {
	var kind string
	switch stage.Kind() {
	case StageKindAgent:
		kind = "Agent"
	case StageKindGate:
		kind = "Approval"
	default:
		kind = "Watch"
	}
	if belts > 1 {
		return kind + " · " + strconv.Itoa(belts) + " belts"
	}
	return kind
}

// OverviewBoxesStatus is the status above a box, from the runs at it: held and waiting first,
// then running, else idle.
func OverviewBoxesStatus(here []Run) (text string, look StageStatusLook) {
	held, waiting, running := 0, 0, 0
	for _, run := range here {
		switch run.State {
		case RunStateHeld:
			held++
		case RunStateWaiting:
			waiting++
		case RunStateRunning:
			running++
		}
	}
	if held > 0 {
		if running > 0 {
			return strconv.Itoa(held) + " held · " + strconv.Itoa(running) + overviewBoxesRunning, StageStatusLookHeld
		}
		return strconv.Itoa(held) + " held", StageStatusLookHeld
	}
	if waiting > 0 {
		return strconv.Itoa(waiting) + " waiting for you", StageStatusLookWaiting
	}
	if running > 0 {
		return strconv.Itoa(running) + overviewBoxesRunning, StageStatusLookRunning
	}
	return "Idle", StageStatusLookPlain
}
