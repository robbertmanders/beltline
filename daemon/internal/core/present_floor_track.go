// The Floor's structure as the browser lays it out: the layers of nodes left to right, the stretches of track
// between neighbouring layers with the belts on each, and the return lanes back to an earlier stage with the names
// of the belts that take them. The browser places them all in points.

package core

import "strings"

// FloorTrack is the factory track as the Floor draws it.
type FloorTrack struct {
	// Layers are left to right: the entrances, then the merged stages, each layer top to bottom.
	Layers [][]FactoryTrackNode `json:"layers"`
	// Tracks are stretches between neighbouring layers.
	Tracks []FactoryTrackTrack `json:"tracks"`
	// Returns are where a belt goes back to a merged stage another belt has after this one: drawn as a lane under the floor.
	Returns []FloorTrackReturn `json:"returns"`
}

// FloorTrackReturn is a lane back to an earlier stage, and the belts that take it.
type FloorTrackReturn struct {
	From  FactoryTrackNode `json:"from"`
	To    FactoryTrackNode `json:"to"`
	Belts []string         `json:"belts"`
	// Label is the short names of the belts that take it, in capitals.
	Label string `json:"label"`
}

// floorTrackLabelSeparator joins a lane's belt names.
const floorTrackLabelSeparator = " · "

// NewFloorTrack is track as the Floor draws it.
func NewFloorTrack(track FactoryTrack) FloorTrack {
	out := FloorTrack{Layers: [][]FactoryTrackNode{}, Tracks: []FactoryTrackTrack{}, Returns: []FloorTrackReturn{}}
	for _, layer := range track.Layers {
		out.Layers = append(out.Layers, append([]FactoryTrackNode{}, layer...))
	}
	out.Tracks = append(out.Tracks, track.Tracks...)
	for _, stretch := range track.Returns {
		var names []string
		for _, id := range stretch.Belts {
			if belt := track.Belt(id); belt != nil {
				names = append(names, FactoryTrackShortName(belt.Name))
			}
		}
		out.Returns = append(out.Returns, FloorTrackReturn{
			From: stretch.From, To: stretch.To, Belts: stretch.Belts,
			Label: UppercasedFull(strings.Join(names, floorTrackLabelSeparator)),
		})
	}
	return out
}
