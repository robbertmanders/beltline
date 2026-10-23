//
// Which machines each belt passes, and on which path: its main line or a
// fork's branch. Then which belts share machines, and the order of the
// tracks their paths run on. The types are internal to the metro map, so
// their methods are unexported.

package core

import (
	"maps"
	"slices"
	"sort"
)

// MetroRange is a closed range of layers or rows. Lower and Upper are its ends.
type MetroRange struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// metroMax is max(x, y) for floats: y when y is at least x, else x.
func metroMax(x, y float64) float64 {
	if y >= x {
		return y
	}
	return x
}

// metroMin is min(x, y) for floats: y when y is less than x, else x.
func metroMin(x, y float64) float64 {
	if y < x {
		return y
	}
	return x
}

// metroLayerOf is the layer of a machine in track, or 0 when it is not placed.
func metroLayerOf(track FactoryTrack, machineID string) int {
	if layer := track.LayerOf(FactoryTrackMachineNode(machineID)); layer != nil {
		return *layer
	}
	return 0
}

// contains reports whether x lies in the range.
func (r MetroRange) contains(x float64) bool {
	return r.Lower <= x && x <= r.Upper
}

// overlaps reports whether two ranges share a point.
func (r MetroRange) overlaps(other MetroRange) bool {
	return r.contains(other.Lower) || other.contains(r.Lower)
}

// MetroLayoutPath is one path a belt takes over the map: its main line or a fork's branch.
type MetroLayoutPath struct {
	Belt string
	// Parent is the path this one leaves; nil for the main line.
	Parent *int
	// From is the machine it leaves after; nil when it leaves from the name pill.
	From *string
	// To is the machine it rejoins; nil when it ends on its own.
	To *string
	// Stops are the machines the path stops at, in order.
	Stops []string
	Depth int
	// Back is set when its last stage goes back to an earlier one rather than ending the run.
	Back bool
	// Pills are the names of the routes this path carries, each before the stop it comes before.
	Pills []MetroLayoutPill
}

// MetroLayoutBack is a stop a belt goes back to: drawn as a dotted return lane.
type MetroLayoutBack struct {
	From *string
	To   string
}

// MetroLayoutRoute is every path one belt takes over the map.
type MetroLayoutRoute struct {
	Belt FactoryTrackBelt
	// Paths are the main line first, then each branch.
	Paths []MetroLayoutPath
	// Backs are the stops the belt goes back to.
	Backs []MetroLayoutBack
}

// machineSet is the machines the route stops at, or goes back to.
func (r MetroLayoutRoute) machineSet() map[string]struct{} {
	set := map[string]struct{}{}
	for _, path := range r.Paths {
		for _, stop := range path.Stops {
			set[stop] = struct{}{}
		}
	}
	for _, back := range r.Backs {
		set[back.To] = struct{}{}
	}
	return set
}

// metroRouteWalker is the state a belt's walk over its segments builds the paths and backs in. Its methods
// are the nested functions of MetroLayout.route(_:track:).
type metroRouteWalker struct {
	belt  FactoryTrackBelt
	track FactoryTrack
	paths []MetroLayoutPath
	backs []MetroLayoutBack
}

// layer is a machine's layer, 0 when it is not placed.
func (w *metroRouteWalker) layer(id string) int {
	return metroLayerOf(w.track, id)
}

// machine is the machine a stage is part of on this belt.
func (w *metroRouteWalker) machine(stage Stage) *string {
	m := w.track.MachineOf(stage.ID, w.belt.ID)
	if m == nil {
		return nil
	}
	id := m.ID
	return &id
}

// pill records the name of route k of fork on path. The pill sits before the first stop the route makes there, from
// stop index from on. A route whose first stage is the station it rejoins at makes no stop of its own, so met, the
// station its walk reached first, takes the pill instead. Its name is the route's own.
func (w *metroRouteWalker) pill(path int, fork *Fork, k, count, from int, met *string) {
	if fork == nil || len(fork.Routes) != count || k >= count {
		return
	}
	stops := w.paths[path].Stops
	var stop string
	switch {
	case len(stops) > from:
		stop = stops[from]
	case met != nil:
		stop = *met
	default:
		return
	}
	route := fork.Routes[k]
	w.paths[path].Pills = append(w.paths[path].Pills, MetroLayoutPill{Fork: fork.ID, Route: route.ID, Text: route.Name, Stop: stop})
}

// machines are the machines a sequence of segments stops at, in order, every route of a fork in turn.
func (w *metroRouteWalker) machines(segments []BeltLayoutSegment) []string {
	var out []string
	for _, segment := range segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			if id := w.machine(*segment.Stage); id != nil {
				out = append(out, *id)
			}
		case BeltLayoutSegmentFork:
			for _, route := range segment.Routes {
				out = append(out, w.machines(route)...)
			}
		}
	}
	return out
}

// rejoin is where a fork's routes meet again: the first machine they all reach, or the stage after the
// fork. Among machines all reaching the same layer, the one sorting first wins, since
// a set has no fixed order.
func (w *metroRouteWalker) rejoin(routes [][]BeltLayoutSegment, after []BeltLayoutSegment) *string {
	var common map[string]struct{}
	if len(routes) >= 2 {
		common = map[string]struct{}{}
		for _, id := range w.machines(routes[0]) {
			common[id] = struct{}{}
		}
		for _, route := range routes[1:] {
			next := map[string]struct{}{}
			for _, id := range w.machines(route) {
				if _, ok := common[id]; ok {
					next[id] = struct{}{}
				}
			}
			common = next
		}
	}
	ids := slices.Sorted(maps.Keys(common))
	var best *string
	for _, id := range ids {
		if best == nil || w.layer(id) < w.layer(*best) {
			chosen := id
			best = &chosen
		}
	}
	if best != nil {
		return best
	}
	if first := w.machines(after); len(first) > 0 {
		return &first[0]
	}
	return nil
}

// metroRouteGoesBack reports whether a route's last stage only ever goes back to another stage, rather than
// ending the run.
func metroRouteGoesBack(segments []BeltLayoutSegment) bool {
	if len(segments) == 0 || segments[len(segments)-1].Kind != BeltLayoutSegmentStage {
		return false
	}
	hasGoto := false
	for _, outcome := range segments[len(segments)-1].Stage.Outcomes {
		switch outcome.Then.Action {
		case ThenActionGoto:
			hasGoto = true
		case ThenActionNext, ThenActionDone, ThenActionShip, ThenActionRoute:
			return false
		}
	}
	return hasGoto
}

// metAt is the station a walk stopped at when it met its skip, or nil when it did not.
func metAt(met bool, at *string) *string {
	if met {
		return at
	}
	return nil
}

// walk walks segments on path from prev, having reached cursor, stopping at skip (the machine the fork
// rejoins at) and falling back to outer. It returns where it ended, the cursor and whether it met skip.
func (w *metroRouteWalker) walk(segments []BeltLayoutSegment, path int, prev *string, cursor int, skip, outer *string) (*string, int, bool) {
	for index, segment := range segments {
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			id := w.machine(*segment.Stage)
			if id == nil {
				continue
			}
			if skip != nil && *skip == *id {
				return prev, cursor, true
			}
			if w.layer(*id) < cursor {
				w.backs = append(w.backs, MetroLayoutBack{From: prev, To: *id})
				continue
			}
			w.paths[path].Stops = append(w.paths[path].Stops, *id)
			prev = id
			cursor = w.layer(*id)
		case BeltLayoutSegmentFork:
			routes := segment.Routes
			after := segments[index+1:]
			join := w.rejoin(routes, after)
			if join == nil {
				join = outer
			}
			if len(routes) == 0 {
				continue
			}
			// The main line goes on through the longest route that does not go back.
			counts := make([]int, len(routes))
			for k, route := range routes {
				if metroRouteGoesBack(route) {
					counts[k] = -1
				} else {
					counts[k] = len(w.machines(route))
				}
			}
			less := func(a, b int) bool { return counts[a] < counts[b] || (counts[a] == counts[b] && a > b) }
			best := 0
			for k := 1; k < len(routes); k++ {
				// max(by:) keeps the last of equal maxima.
				if !less(k, best) {
					best = k
				}
			}
			for route := range routes {
				if route == best {
					continue
				}
				back := metroRouteGoesBack(routes[route])
				parent := path
				branch := MetroLayoutPath{Belt: w.belt.ID, Parent: &parent, From: prev, Depth: w.paths[path].Depth + 1, Back: back}
				if !back {
					branch.To = join
				}
				w.paths = append(w.paths, branch)
				branchIndex := len(w.paths) - 1
				_, _, branchMet := w.walk(routes[route], branchIndex, prev, cursor, join, join)
				w.pill(branchIndex, segment.Fork, route, len(routes), 0, metAt(branchMet, join))
			}
			pending := len(w.paths[path].Stops)
			walkedPrev, walkedCursor, met := w.walk(routes[best], path, prev, cursor, skip, join)
			w.pill(path, segment.Fork, best, len(routes), pending, metAt(met, skip))
			prev, cursor = walkedPrev, walkedCursor
			if met {
				return walkedPrev, walkedCursor, true
			}
		}
	}
	return prev, cursor, false
}

// metroLayoutRoute is the paths a belt takes over track.
func metroLayoutRoute(belt FactoryTrackBelt, track FactoryTrack) MetroLayoutRoute {
	w := &metroRouteWalker{belt: belt, track: track, paths: []MetroLayoutPath{{Belt: belt.ID}}}
	w.walk(belt.Layout.Segments(), 0, nil, 0, nil, nil)
	// A route with no stops of its own that rejoins nothing has nothing to draw.
	var kept []int
	for i, path := range w.paths {
		if i == 0 || len(path.Stops) > 0 || path.To != nil {
			kept = append(kept, i)
		}
	}
	index := map[int]int{}
	for newIndex, oldIndex := range kept {
		index[oldIndex] = newIndex
	}
	remapped := make([]MetroLayoutPath, 0, len(kept))
	for _, oldIndex := range kept {
		path := w.paths[oldIndex]
		if path.Parent != nil {
			parent := 0
			if newIndex, ok := index[*path.Parent]; ok {
				parent = newIndex
			}
			path.Parent = &parent
		}
		remapped = append(remapped, path)
	}
	return MetroLayoutRoute{Belt: belt, Paths: remapped, Backs: w.backs}
}

// metroLayoutComponents groups belts that share a machine, directly or through others, in the order of the
// Floor's entrances, larger groups first.
func metroLayoutComponents(routes []MetroLayoutRoute, track FactoryTrack) [][]int {
	parent := make([]int, len(routes))
	for i := range parent {
		parent[i] = i
	}
	var find func(i int) int
	find = func(i int) int {
		if parent[i] == i {
			return i
		}
		return find(parent[i])
	}
	owner := map[string]int{}
	for i, route := range routes {
		machines := slices.Sorted(maps.Keys(route.machineSet()))
		for _, machine := range machines {
			if other, ok := owner[machine]; ok {
				parent[find(i)] = find(other)
			} else {
				owner[machine] = i
			}
		}
	}
	var entrances []FactoryTrackNode
	if len(track.Layers) > 0 {
		entrances = track.Layers[0]
	}
	entrance := func(i int) int {
		want := FactoryTrackEntrance(routes[i].Belt.ID)
		for k, node := range entrances {
			if node == want {
				return k
			}
		}
		return i
	}
	// Groups in the order of their first belt, so the result never depends on map order.
	var roots []int
	groupOf := map[int][]int{}
	for i := range routes {
		root := find(i)
		if _, ok := groupOf[root]; !ok {
			roots = append(roots, root)
		}
		groupOf[root] = append(groupOf[root], i)
	}
	var groups [][]int
	for _, root := range roots {
		members := groupOf[root]
		sort.SliceStable(members, func(a, b int) bool { return entrance(members[a]) < entrance(members[b]) })
		groups = append(groups, members)
	}
	// Larger groups first, so the busiest part of the factory leads.
	sort.SliceStable(groups, func(a, b int) bool {
		ga, gb := groups[a], groups[b]
		if len(ga) != len(gb) {
			return len(ga) > len(gb)
		}
		return -entrance(ga[0]) > -entrance(gb[0])
	})
	return groups
}
