// A fork's track, expressed as straight segments at its reserved edges.
// Shape depends only on row geometry, not route order or stage count.

package core

// MapLanePoint is a corner of a lane, in points.
type MapLanePoint struct {
	X float64
	Y float64
}

// MapLane is a fork's track as straight segments.
type MapLane struct {
	Points []MapLanePoint
	LabelX float64
	// Opens is whether the lane leaves the line at its start.
	Opens bool
	// Joins is whether the lane comes back to the line at its end.
	Joins bool
}

// MapLaneArgs are the parameters of NewMapLane. Opens nil is true.
type MapLaneArgs struct {
	Start     float64
	End       float64
	MainY     float64
	Y         float64
	Thickness float64
	Opens     *bool
	// Stop, when set, ends the lane on its row instead of joining the line.
	Stop *float64
}

// NewMapLane is a fork's lane. A lane that doesn't open starts on its own row where the
// split would be; one with a stop ends there, on its row, instead of joining.
func NewMapLane(a MapLaneArgs) MapLane {
	opens := ValueOr(a.Opens, true)
	var begin []MapLanePoint
	if opens {
		begin = []MapLanePoint{{X: a.Start, Y: a.MainY}, {X: a.Start, Y: a.Y}}
	} else {
		begin = []MapLanePoint{{X: a.Start + a.Thickness/2, Y: a.Y}}
	}
	var finish []MapLanePoint
	if a.Stop != nil {
		finish = []MapLanePoint{{X: *a.Stop, Y: a.Y}}
	} else {
		finish = []MapLanePoint{{X: a.End, Y: a.Y}, {X: a.End, Y: a.MainY}}
	}
	var points []MapLanePoint
	// On the line itself the corners fall away.
	if a.Y == a.MainY {
		points = []MapLanePoint{begin[0], finish[0]}
	} else {
		points = append(append([]MapLanePoint{}, begin...), finish...)
	}
	return MapLane{
		Points: points,
		LabelX: a.Start + a.Thickness/2 + 4,
		Opens:  opens,
		Joins:  a.Stop == nil,
	}
}

// Room is the space before the first machine, excluding the corner's stroke.
func (l MapLane) Room(inside, column, cell float64) float64 {
	return inside + column/2 - float64(cell*0.28) - l.LabelX
}
