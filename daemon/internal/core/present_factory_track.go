//
// The whole factory as one floor: every belt's stages merged by name (ignoring
// case and whitespace) into shared merged stages, joined by the tracks the belts
// take between them. A belt that comes to a name it already passed on the
// way gets a second merged stage of that name, so it never seems to go back. Each belt enters on the left at its trigger; where belts
// share a merged stage their tracks meet, and where they go different ways they
// split again. Merged stages sit in layers, left to right, so every track runs
// forwards; a track that would cross a layer without a merged stage passes through
// a waypoint there, so it never runs behind one.

package core

import (
	"slices"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// FactoryTrackBelt is a belt as the floor needs it: identity, name and layout.
type FactoryTrackBelt struct {
	ID     string
	Name   string
	Layout BeltLayout
}

// FactoryTrackMember is one stage of one belt that is part of a merged stage.
type FactoryTrackMember struct {
	BeltID   string
	BeltName string
	StageID  string
	Stage    Stage
}

// FactoryTrackStage is one merged stage on the floor, shared by every belt with a stage of its name.
type FactoryTrackStage struct {
	ID      string
	Name    string
	Members []FactoryTrackMember
}

// Belts are the belts that go through the merged stage, in the order they reach it.
func (m FactoryTrackStage) Belts() []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, member := range m.Members {
		if _, ok := seen[member.BeltID]; ok {
			continue
		}
		seen[member.BeltID] = struct{}{}
		out = append(out, member.BeltID)
	}
	return out
}

// Contains reports whether run is at one of the merged stage's members.
func (m FactoryTrackStage) Contains(run Run) bool {
	return slices.ContainsFunc(m.Members, func(member FactoryTrackMember) bool {
		return member.BeltID == run.BeltID && member.StageID == run.StageID
	})
}

// FactoryTrackNodeKind is the kind of a node placed in a layer.
type FactoryTrackNodeKind string

const (
	// FactoryTrackNodeEntrance is a belt's entrance; ID is the belt id.
	FactoryTrackNodeEntrance FactoryTrackNodeKind = "entrance"
	// FactoryTrackNodeStage is a merged stage; ID is its id.
	FactoryTrackNodeStage FactoryTrackNodeKind = "stage"
	// FactoryTrackNodeWaypoint is a waypoint a longer track passes through; Waypoint is its number.
	FactoryTrackNodeWaypoint FactoryTrackNodeKind = "waypoint"
)

// FactoryTrackNode is something placed in a layer: a belt's entrance, a merged stage, or a waypoint
// a longer track passes through. It is comparable, so it can key a map.
//
// Encoding: {"kind": ..., "value": ...}, the value being the id, or the waypoint's number.
type FactoryTrackNode struct {
	Kind     FactoryTrackNodeKind
	ID       string
	Waypoint int
}

// MarshalJSON writes the node as its kind and value.
func (n FactoryTrackNode) MarshalJSON() ([]byte, error) {
	if n.Kind == FactoryTrackNodeWaypoint {
		return jsonx.Marshal(struct {
			Kind  FactoryTrackNodeKind `json:"kind"`
			Value int                  `json:"value"`
		}{n.Kind, n.Waypoint})
	}
	return jsonx.Marshal(struct {
		Kind  FactoryTrackNodeKind `json:"kind"`
		Value string               `json:"value"`
	}{n.Kind, n.ID})
}

// FactoryTrackEntrance is the node of a belt's entrance.
func FactoryTrackEntrance(beltID string) FactoryTrackNode {
	return FactoryTrackNode{Kind: FactoryTrackNodeEntrance, ID: beltID}
}

// FactoryTrackStageNode is the node of a merged stage.
func FactoryTrackStageNode(stageID string) FactoryTrackNode {
	return FactoryTrackNode{Kind: FactoryTrackNodeStage, ID: stageID}
}

// FactoryTrackWaypoint is the node of a waypoint.
func FactoryTrackWaypoint(number int) FactoryTrackNode {
	return FactoryTrackNode{Kind: FactoryTrackNodeWaypoint, Waypoint: number}
}

// FactoryTrackTrack is a stretch of track between two nodes, and the belts on it.
type FactoryTrackTrack struct {
	From  FactoryTrackNode
	To    FactoryTrackNode
	Belts []string
}

// factoryTrackLink is one stage-to-stage link a belt makes.
type factoryTrackLink struct {
	From FactoryTrackNode
	To   FactoryTrackNode
	Belt string
}

// factoryTrackWalk is where a walk of segments ends: its open ends and the nodes it passed.
type factoryTrackWalk struct {
	Ends   []FactoryTrackNode
	Passed map[FactoryTrackNode]struct{}
}

// FactoryTrack is the whole factory's floor: merged stages, layers and the tracks between
// them, built from every belt.
type FactoryTrack struct {
	Belts  []FactoryTrackBelt
	Stages []FactoryTrackStage
	// Layers are left to right: the entrances, then the merged stages, each layer top to bottom.
	Layers [][]FactoryTrackNode
	// Tracks are stretches between neighbouring layers.
	Tracks []FactoryTrackTrack
	// Returns are where a belt goes back to a merged stage another belt has after this one: drawn as a lane under the floor.
	Returns []FactoryTrackTrack
}

// Copy and word lists used by FactoryTrack. Literal text is exact.
const (
	factoryTrackShortNameLimit = 12
	factoryTrackShortNameMin   = 5
)
