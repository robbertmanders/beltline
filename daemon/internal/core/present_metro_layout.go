//
// The factory as a transit map, in points. Every belt is a straight line on
// its own track, coming in at 45° from its name pill on the left; belts that
// share machines run snug side by side, and a shared machine is one
// interchange across their tracks. A fork's other routes run on tracks of
// their own beside the line and come back where they rejoin. Every station
// has an x of its own, and names and run labels sit over or under each band.
// Built from a FactoryTrack and the runs the Floor would show; the app only
// draws it. The API sends these types as they are, its lines and stations with
// what they derive (api.MetroLayoutWire).

package core

import (
	"maps"
	"slices"
)

// Sizes and palette of the metro map.
const (
	// MetroLayoutPaletteSize is the number of belt colours.
	MetroLayoutPaletteSize = 10
	// MetroLayoutLanePitch is the least distance between two tracks, centre to centre.
	MetroLayoutLanePitch = 11.0
	// MetroLayoutCardWidth is the width of a run dot's card.
	MetroLayoutCardWidth = 260.0
	// MetroLayoutCornerRadius is how round a line's corners are.
	MetroLayoutCornerRadius = 10.0
	// MetroLayoutTerminusHalf is half the height of a terminus bar.
	MetroLayoutTerminusHalf = 7.5
	// MetroLayoutLineWidth is the width of a line.
	MetroLayoutLineWidth = 6.0
	// MetroLayoutOverhang is how far an interchange reaches past its outer tracks.
	MetroLayoutOverhang = 7.0
	// MetroLayoutInterchangeWidth is twice the width of a single station, as in the mockup.
	MetroLayoutInterchangeWidth = 30.0
	// MetroLayoutPillHeight is the height of a belt's name pill.
	MetroLayoutPillHeight = 16.0
)

// metroLayoutFNVOffset and metroLayoutFNVPrime are the FNV-1a constants of the colour index.
const (
	metroLayoutFNVOffset uint32 = 2166136261
	metroLayoutFNVPrime  uint32 = 16777619
)

// MetroLayoutColourIndex is a belt's preferred colour, from its id alone (FNV-1a).
func MetroLayoutColourIndex(id string) int {
	hash := metroLayoutFNVOffset
	for i := 0; i < len(id); i++ {
		hash ^= uint32(id[i])
		hash *= metroLayoutFNVPrime
	}
	return int(hash % MetroLayoutPaletteSize)
}

// MetroLayoutColours is a colour per belt: its preferred one, unless a belt whose id sorts first has it,
// then the next free one. Distinct while there are no more belts than colours, and the same whatever
// order the belts come in.
func MetroLayoutColours(ids []string) map[string]int {
	unique := map[string]struct{}{}
	for _, id := range ids {
		unique[id] = struct{}{}
	}
	sorted := slices.Sorted(maps.Keys(unique))
	taken := map[int]struct{}{}
	result := map[string]int{}
	for _, id := range sorted {
		colour := MetroLayoutColourIndex(id)
		if len(taken) < MetroLayoutPaletteSize {
			for {
				if _, ok := taken[colour]; !ok {
					break
				}
				colour = (colour + 1) % MetroLayoutPaletteSize
			}
		}
		taken[colour] = struct{}{}
		result[id] = colour
	}
	return result
}

// MetroLayoutPoint is a point on the map, in points.
type MetroLayoutPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MetroLayoutBox is a rectangle on the map, in points.
type MetroLayoutBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Intersects reports whether two boxes overlap.
func (b MetroLayoutBox) Intersects(other MetroLayoutBox) bool {
	return b.X < other.MaxX() && other.X < b.MaxX() && b.Y < other.MaxY() && other.Y < b.MaxY()
}

// Contains reports whether point lies in the box, grown by margin on every side.
func (b MetroLayoutBox) Contains(point MetroLayoutPoint, margin float64) bool {
	return point.X > b.X-margin && point.X < b.MaxX()+margin && point.Y > b.Y-margin && point.Y < b.MaxY()+margin
}

// MaxX is the right edge of the box.
func (b MetroLayoutBox) MaxX() float64 {
	return b.X + b.Width
}

// MaxY is the bottom edge of the box.
func (b MetroLayoutBox) MaxY() float64 {
	return b.Y + b.Height
}

// MetroLayoutBranch is a fork's branch of a belt's line.
type MetroLayoutBranch struct {
	Points []MetroLayoutPoint `json:"points"`
	From   *string            `json:"from,omitzero"`
	// To is the machine it rejoins; nil when it ends on its own, at End.
	To    *string           `json:"to,omitzero"`
	Stops []string          `json:"stops"`
	End   *MetroLayoutPoint `json:"end,omitzero"`
	// Back is set when it goes back to an earlier stage when done, so it has no terminus.
	Back bool `json:"back"`
}

// MetroLayoutLine is one belt's line on the map: its name pill, its main line, branches and return lanes.
// Its ID is the belt id.
type MetroLayoutLine struct {
	BeltID  string         `json:"beltId"`
	Name    string         `json:"name"`
	Colour  int            `json:"colour"`
	NameBox MetroLayoutBox `json:"nameBox"`
	// Points is the main line, from the name pill through its stations to the terminus.
	Points   []MetroLayoutPoint  `json:"points"`
	Branches []MetroLayoutBranch `json:"branches"`
	// Returns are dotted lanes back to a machine the belt passed further left.
	Returns  [][]MetroLayoutPoint `json:"returns"`
	Terminus MetroLayoutPoint     `json:"terminus"`
	// Pills are the names of the routes the belt takes, each on its track before its first stop.
	Pills []MetroLayoutRoutePill `json:"routes"`
}

// ID is the line's identity: its belt id.
func (l MetroLayoutLine) ID() string {
	return l.BeltID
}

// Strokes are the stretches drawn solid: the main line and its branches.
func (l MetroLayoutLine) Strokes() [][]MetroLayoutPoint {
	out := [][]MetroLayoutPoint{l.Points}
	for _, branch := range l.Branches {
		out = append(out, branch.Points)
	}
	return out
}

// Ends is where the line and each branch that ends on its own stop.
func (l MetroLayoutLine) Ends() []MetroLayoutPoint {
	out := []MetroLayoutPoint{l.Terminus}
	for _, branch := range l.Branches {
		if !branch.Back && branch.End != nil {
			out = append(out, *branch.End)
		}
	}
	return out
}

// metroLayoutSVGPaths is each stretch as SVG path data, its corners rounded to radius.
func metroLayoutSVGPaths(stretches [][]MetroLayoutPoint, radius float64) []string {
	out := make([]string, 0, len(stretches))
	for _, points := range stretches {
		converted := make([]svgPoint, len(points))
		for i, p := range points {
			converted[i] = svgPoint(p)
		}
		out = append(out, svgRounded(converted, radius))
	}
	return out
}

// Paths are the solid stretches as SVG path data, corners rounded.
func (l MetroLayoutLine) Paths() []string {
	return metroLayoutSVGPaths(l.Strokes(), MetroLayoutCornerRadius)
}

// ReturnPaths are the return lanes as SVG path data, corners rounded.
func (l MetroLayoutLine) ReturnPaths() []string {
	return metroLayoutSVGPaths(l.Returns, MetroLayoutCornerRadius)
}

// EndBars is a bar at each end of the line, across its track.
func (l MetroLayoutLine) EndBars() []MetroLayoutBar {
	ends := l.Ends()
	out := make([]MetroLayoutBar, 0, len(ends))
	for _, end := range ends {
		out = append(out, MetroLayoutBar{X: end.X, Top: end.Y - MetroLayoutTerminusHalf, Bottom: end.Y + MetroLayoutTerminusHalf})
	}
	return out
}

// MetroLayoutBar is a terminus bar drawn across a track: its x and the y it runs from and to.
type MetroLayoutBar struct {
	X      float64 `json:"x"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}
