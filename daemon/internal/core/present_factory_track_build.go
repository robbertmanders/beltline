// The factory track's build: the walk that lays each belt's stages into merged stages,
// layers and tracks, and the ordering of each layer so the tracks cross as little
// as they can. The types are in present_factory_track.go; the reads are in
// present_factory_track_queries.go.

package core

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

// NewFactoryTrack builds the floor from belts.
func NewFactoryTrack(belts []FactoryTrackBelt) FactoryTrack {
	f := FactoryTrack{Belts: belts}
	var links []factoryTrackLink
	for _, belt := range belts {
		ends := []FactoryTrackNode{FactoryTrackEntrance(belt.ID)}
		// What the belt has passed on its way here, and the names still ahead;
		// a fork's routes only see their own, including a fork nested in a route.
		_ = f.walkSegments(belt.Layout.Segments(), belt, ends, map[FactoryTrackNode]struct{}{}, map[string]struct{}{}, &links)
	}
	kept, back := factoryTrackSplit(links)
	layer := map[FactoryTrackNode]int{}
	for _, belt := range belts {
		layer[FactoryTrackEntrance(belt.ID)] = 0
	}
	for _, merged := range f.Stages {
		layer[FactoryTrackStageNode(merged.ID)] = 1
	}
	// Each merged stage sits one layer past the furthest node that leads to it.
	for iteration := 0; iteration <= len(f.Stages); iteration++ {
		changed := false
		for _, link := range kept {
			if layer[link.To] < layer[link.From]+1 {
				layer[link.To] = layer[link.From] + 1
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	deepest := 0
	for _, l := range layer {
		deepest = max(deepest, l)
	}
	f.Layers = make([][]FactoryTrackNode, deepest+1)
	for _, belt := range belts {
		f.Layers[0] = append(f.Layers[0], FactoryTrackEntrance(belt.ID))
	}
	for _, merged := range f.Stages {
		l := layer[FactoryTrackStageNode(merged.ID)]
		f.Layers[l] = append(f.Layers[l], FactoryTrackStageNode(merged.ID))
	}

	// A link across several layers becomes one stretch per layer, through waypoints.
	chains := map[FactoryTrackNode]map[FactoryTrackNode][]FactoryTrackNode{}
	waypoints := 0
	for _, link := range kept {
		if _, ok := chains[link.From][link.To]; !ok {
			chain := []FactoryTrackNode{link.From}
			for l := layer[link.From] + 1; l < layer[link.To]; l++ {
				f.Layers[l] = append(f.Layers[l], FactoryTrackWaypoint(waypoints))
				chain = append(chain, FactoryTrackWaypoint(waypoints))
				waypoints++
			}
			if chains[link.From] == nil {
				chains[link.From] = map[FactoryTrackNode][]FactoryTrackNode{}
			}
			chains[link.From][link.To] = append(chain, link.To)
		}
		chain := chains[link.From][link.To]
		for i := 0; i+1 < len(chain); i++ {
			factoryTrackAdd(factoryTrackLink{From: chain[i], To: chain[i+1], Belt: link.Belt}, &f.Tracks)
		}
	}
	for _, link := range back {
		factoryTrackAdd(link, &f.Returns)
	}
	f.order()
	return f
}

// reach is the merged stage for a stage: the one with its name, or the next of that
// name when the belt already passed it. When the belt comes to the name
// again further on, this stage also skips a merged stage that the floor so far
// would lead back from, so its later stage is the one that shares it.
func (f *FactoryTrack) reach(stage Stage, belt FactoryTrackBelt, passed map[FactoryTrackNode]struct{}, ahead map[string]struct{}, links []factoryTrackLink) FactoryTrackNode {
	name := factoryTrackKey(stage.Name)
	taken := func(key string) bool {
		if _, ok := passed[FactoryTrackStageNode(key)]; ok {
			return true
		}
		if _, ok := ahead[name]; !ok || f.stageIndex(key) < 0 {
			return false
		}
		var later []FactoryTrackNode
		for _, merged := range f.Stages {
			if _, ok := ahead[factoryTrackKey(merged.Name)]; ok && merged.ID != key {
				later = append(later, FactoryTrackStageNode(merged.ID))
			}
		}
		here := map[FactoryTrackNode]struct{}{FactoryTrackStageNode(key): {}}
		return factoryTrackReaches([]FactoryTrackNode{FactoryTrackStageNode(key)}, passed, links) ||
			factoryTrackReaches(later, here, links)
	}
	key, count := name, 1
	for taken(key) {
		count++
		key = name + " #" + strconv.Itoa(count)
	}
	member := FactoryTrackMember{BeltID: belt.ID, BeltName: belt.Name, StageID: stage.ID, Stage: cloneValue(stage)}
	if index := f.stageIndex(key); index >= 0 {
		f.Stages[index].Members = append(f.Stages[index].Members, member)
	} else {
		f.Stages = append(f.Stages, FactoryTrackStage{
			ID:      key,
			Name:    strings.TrimSpace(stage.Name),
			Members: []FactoryTrackMember{member},
		})
	}
	return FactoryTrackStageNode(key)
}

// stageIndex is the position of the merged stage with id, or -1.
func (f *FactoryTrack) stageIndex(id string) int {
	for i := range f.Stages {
		if f.Stages[i].ID == id {
			return i
		}
	}
	return -1
}

// walkSegments walks a sequence of stages and forks. Each route of a fork leaves the
// node before the fork and meets the others only at the next stage, so sibling
// routes never link to each other. An empty route skips the fork.
func (f *FactoryTrack) walkSegments(segments []BeltLayoutSegment, belt FactoryTrackBelt, incoming []FactoryTrackNode,
	soFar map[FactoryTrackNode]struct{}, ahead map[string]struct{}, links *[]factoryTrackLink) factoryTrackWalk {
	ends := append([]FactoryTrackNode{}, incoming...)
	passed := CopySet(soFar)
	for index, segment := range segments {
		later := CopySet(ahead)
		for _, following := range segments[index+1:] {
			for _, stage := range following.Stages() {
				later[factoryTrackKey(stage.Name)] = struct{}{}
			}
		}
		switch segment.Kind {
		case BeltLayoutSegmentStage:
			node := f.reach(*segment.Stage, belt, passed, later, *links)
			for _, end := range ends {
				if end != node {
					*links = append(*links, factoryTrackLink{From: end, To: node, Belt: belt.ID})
				}
			}
			ends = []FactoryTrackNode{node}
			passed[node] = struct{}{}
		default:
			var next []FactoryTrackNode
			after := CopySet(passed)
			for _, route := range segment.Routes {
				walked := f.walkSegments(route, belt, ends, passed, later, links)
				for _, end := range walked.Ends {
					if !slices.Contains(next, end) {
						next = append(next, end)
					}
				}
				for node := range walked.Passed {
					after[node] = struct{}{}
				}
			}
			if len(next) > 0 {
				ends = next
			}
			passed = after
		}
	}
	return factoryTrackWalk{Ends: ends, Passed: passed}
}

// factoryTrackReaches reports whether any of starts leads to any of goals along links.
func factoryTrackReaches(starts []FactoryTrackNode, goals map[FactoryTrackNode]struct{}, links []factoryTrackLink) bool {
	seen := map[FactoryTrackNode]struct{}{}
	var stack []FactoryTrackNode
	for _, start := range starts {
		seen[start] = struct{}{}
		stack = append(stack, start)
	}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, link := range links {
			if link.From != node {
				continue
			}
			if _, ok := goals[link.To]; ok {
				return true
			}
			if _, ok := seen[link.To]; !ok {
				seen[link.To] = struct{}{}
				stack = append(stack, link.To)
			}
		}
	}
	return false
}

// factoryTrackAdd adds a link to tracks, or extends the existing track between its ends.
func factoryTrackAdd(link factoryTrackLink, tracks *[]FactoryTrackTrack) {
	for i := range *tracks {
		track := &(*tracks)[i]
		if track.From == link.From && track.To == link.To {
			if !slices.Contains(track.Belts, link.Belt) {
				track.Belts = append(track.Belts, link.Belt)
			}
			return
		}
	}
	*tracks = append(*tracks, FactoryTrackTrack{From: link.From, To: link.To, Belts: []string{link.Belt}})
}

// factoryTrackSplit splits links into those that keep the floor running left to right, in belt
// order, and those that would close a loop with them.
func factoryTrackSplit(links []factoryTrackLink) (kept []factoryTrackLink, back []factoryTrackLink) {
	reaches := func(start, goal FactoryTrackNode) bool {
		seen := map[FactoryTrackNode]struct{}{start: {}}
		stack := []FactoryTrackNode{start}
		for len(stack) > 0 {
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if node == goal {
				return true
			}
			for _, link := range kept {
				if link.From != node {
					continue
				}
				if _, ok := seen[link.To]; !ok {
					seen[link.To] = struct{}{}
					stack = append(stack, link.To)
				}
			}
		}
		return false
	}
	for _, link := range links {
		if reaches(link.To, link.From) {
			back = append(back, link)
		} else {
			kept = append(kept, link)
		}
	}
	return kept, back
}

// order orders each layer so tracks cross as little as they can: a few sweeps
// each way, placing every node at the average place of its neighbours.
func (f *FactoryTrack) order() {
	for sweep := 0; sweep < 6; sweep++ {
		down := sweep%2 == 0
		var indices []int
		if down {
			for l := 1; l < len(f.Layers); l++ {
				indices = append(indices, l)
			}
		} else {
			for l := len(f.Layers) - 2; l >= 0; l-- {
				indices = append(indices, l)
			}
		}
		for _, l := range indices {
			var other []FactoryTrackNode
			if down {
				other = f.Layers[l-1]
			} else {
				other = f.Layers[l+1]
			}
			current := f.Layers[l]
			weights := make([]float64, len(current))
			for index, node := range current {
				var near []float64
				for _, track := range f.Tracks {
					if (down && track.To != node) || (!down && track.From != node) {
						continue
					}
					neighbour := track.To
					if down {
						neighbour = track.From
					}
					position := 0
					for i, candidate := range other {
						if candidate == neighbour {
							position = i
							break
						}
					}
					for range track.Belts {
						near = append(near, float64(position))
					}
				}
				if len(near) == 0 {
					weights[index] = float64(index)
				} else {
					sum := 0.0
					for _, value := range near {
						sum += value
					}
					weights[index] = sum / float64(len(near))
				}
			}
			order := make([]int, len(current))
			for i := range order {
				order[i] = i
			}
			sort.Slice(order, func(i, j int) bool {
				a, b := order[i], order[j]
				if weights[a] == weights[b] {
					return a < b
				}
				return weights[a] < weights[b]
			})
			sorted := make([]FactoryTrackNode, len(current))
			for i, index := range order {
				sorted[i] = current[index]
			}
			f.Layers[l] = sorted
		}
	}
}
