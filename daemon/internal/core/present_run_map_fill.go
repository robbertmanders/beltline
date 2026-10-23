//

package core

import "slices"

// MapFill is how far a run's track is filled: the lanes of the track the run follows, the x the
// fill stops at, and whether the run is finished and its stripes move.
type MapFill struct {
	Lanes    []MapGeometryLane
	UntilX   float64
	Finished bool
	Moving   bool
}

// RunFill is the fill of a run's map. A finished run fills to the end of the map, a run with no
// stage fills nothing, and any other run fills to its stage's left edge. The stripes move only
// while the run is working and nothing pauses it; paused says the run or its belt is paused.
func (g MapGeometry) RunFill(run Run, routes map[string][]string, paused bool) MapFill {
	fill := MapFill{
		Lanes:    g.FilledLanes(routes),
		Finished: run.State.Completed(),
		Moving:   run.State == RunStateRunning && !paused,
	}
	switch {
	case fill.Finished:
		fill.UntilX = g.Width
	case run.StageID == "":
		fill.UntilX = 0
	default:
		fill.UntilX = g.Left(run.StageID)
	}
	return fill
}

// FilledLanes is the lanes on the track the run follows, in lane order. A lane is on it when its
// path is one of the selected paths, which is how the map has always matched them.
func (g MapGeometry) FilledLanes(routes map[string][]string) []MapGeometryLane {
	selected := g.Path(routes)
	var lanes []MapGeometryLane
	for _, lane := range g.Lanes {
		for _, path := range selected {
			if path.Equal(lane.Path) {
				lanes = append(lanes, lane)
				break
			}
		}
	}
	return lanes
}

// Equal reports whether two paths have the same subpaths and corner radius.
func (p MapPath) Equal(other MapPath) bool {
	return p.CornerRadius == other.CornerRadius && slices.EqualFunc(p.Subpaths, other.Subpaths, slices.Equal[[]MapPoint])
}
