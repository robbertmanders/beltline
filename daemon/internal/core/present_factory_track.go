//
// The whole factory as one floor: every belt's stages merged by name (ignoring
// case and whitespace) into shared machines, joined by the tracks the belts
// take between them. A belt that comes to a name it already passed on the
// way gets a second machine of that name, so it never seems to go back. Each belt enters on the left at its trigger; where belts
// share a machine their tracks meet, and where they go different ways they
// split again. Machines sit in layers, left to right, so every track runs
// forwards; a track that would cross a layer without a machine passes through
// a waypoint there, so it never runs behind one.

package core

import (
	"slices"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// FactoryTrackBelt is a belt as the floor needs it: identity, name and layout.
type FactoryTrackBelt struct {
	ID     string
	Name   string
	Layout BeltLayout
}

// FactoryTrackMember is one stage of one belt that is part of a machine.
type FactoryTrackMember struct {
	BeltID   string
	BeltName string
	StageID  string
	Stage    Stage
}

// Worker is the member's kind, agent and runner, joined with " · ".
func (m FactoryTrackMember) Worker() string {
	var parts []string
	parts = append(parts, string(m.Stage.Kind()))
	if m.Stage.Agent != nil {
		parts = append(parts, m.Stage.Agent.ID)
	}
	if m.Stage.Runner != nil {
		parts = append(parts, m.Stage.Runner.Label())
	}
	return strings.Join(parts, " · ")
}

// FactoryTrackMachine is one machine on the floor, shared by every belt with a stage of its name.
type FactoryTrackMachine struct {
	ID      string
	Name    string
	Members []FactoryTrackMember
}

// Belts are the belts that go through the machine, in the order they reach it.
func (m FactoryTrackMachine) Belts() []string {
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

// Mixed reports whether the members do not all share one worker key.
func (m FactoryTrackMachine) Mixed() bool {
	keys := map[string]struct{}{}
	for _, member := range m.Members {
		keys[member.Stage.WorkerKey()] = struct{}{}
	}
	return len(keys) > 1
}

// Workers is one line per member: "belt [id]: worker".
func (m FactoryTrackMachine) Workers() string {
	lines := make([]string, 0, len(m.Members))
	for _, member := range m.Members {
		lines = append(lines, member.BeltName+" ["+member.BeltID+"]: "+member.Worker())
	}
	return strings.Join(lines, "\n")
}

// Contains reports whether run is at one of the machine's stages.
func (m FactoryTrackMachine) Contains(run Run) bool {
	return slices.ContainsFunc(m.Members, func(member FactoryTrackMember) bool {
		return member.BeltID == run.BeltID && member.StageID == run.StageID
	})
}

// FactoryTrackNodeKind is the kind of a node placed in a layer.
type FactoryTrackNodeKind string

const (
	// FactoryTrackNodeEntrance is a belt's entrance; ID is the belt id.
	FactoryTrackNodeEntrance FactoryTrackNodeKind = "entrance"
	// FactoryTrackNodeMachine is a machine; ID is the machine id.
	FactoryTrackNodeMachine FactoryTrackNodeKind = "machine"
	// FactoryTrackNodeWaypoint is a waypoint a longer track passes through; Waypoint is its number.
	FactoryTrackNodeWaypoint FactoryTrackNodeKind = "waypoint"
)

// FactoryTrackNode is something placed in a layer: a belt's entrance, a machine, or a waypoint
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

// FactoryTrackMachineNode is the node of a machine.
func FactoryTrackMachineNode(machineID string) FactoryTrackNode {
	return FactoryTrackNode{Kind: FactoryTrackNodeMachine, ID: machineID}
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

// FactoryTrack is the whole factory's floor: machines, layers and the tracks between
// them, built from every belt.
type FactoryTrack struct {
	Belts    []FactoryTrackBelt
	Machines []FactoryTrackMachine
	// Layers are left to right: the entrances, then the machines, each layer top to bottom.
	Layers [][]FactoryTrackNode
	// Tracks are stretches between neighbouring layers.
	Tracks []FactoryTrackTrack
	// Returns are where a belt goes back to a machine another belt has after this one: drawn as a lane under the floor.
	Returns []FactoryTrackTrack
}

// Copy and word lists used by FactoryTrack. Literal text is exact.
const (
	factoryTrackShortNameLimit = 12
	factoryTrackShortNameMin   = 5
)
