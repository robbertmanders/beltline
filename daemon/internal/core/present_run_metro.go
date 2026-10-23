//
// One belt on the run page as a metro line, in points: the belt's name pill at
// the left, the line running through its stations, and every fork route on a
// row of its own. Built by RunMetroLayout from the belt's shape and the
// run's stage statuses; the app only draws it. Stations, lines and labels are
// in the Overview's metro language, so a belt reads the same in both places.

package core

// RunMetroGlyph is what a station is drawn as: an agent's circle, an approval's diamond, a watch's square.
type RunMetroGlyph string

const (
	RunMetroGlyphCircle  RunMetroGlyph = "circle"
	RunMetroGlyphDiamond RunMetroGlyph = "diamond"
	RunMetroGlyphSquare  RunMetroGlyph = "square"
)

// RunMetroTravel is how far the run has got along a stretch of line: done behind the run, todo still
// ahead, skip on a route the run did not take.
type RunMetroTravel string

const (
	RunMetroTravelDone RunMetroTravel = "done"
	RunMetroTravelTodo RunMetroTravel = "todo"
	RunMetroTravelSkip RunMetroTravel = "skip"
)

// RunMetroStationState is a station's place in the run: current is the stage the run is at.
type RunMetroStationState string

const (
	RunMetroStationStateDone    RunMetroStationState = "done"
	RunMetroStationStateCurrent RunMetroStationState = "current"
	RunMetroStationStateTodo    RunMetroStationState = "todo"
	RunMetroStationStateSkip    RunMetroStationState = "skip"
)

// RunMetroBox is a rectangle in the map's points.
type RunMetroBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// RunMetroSegment is one stretch of line with a single travel state. Consecutive stretches that share
// a state are one path, so its corners turn smoothly.
type RunMetroSegment struct {
	ID    string         `json:"id"`
	State RunMetroTravel `json:"state"`
	Path  MapPath        `json:"path"`
}

// RunMetroStation is a stage on the run's metro line.
type RunMetroStation struct {
	// ID is the stage's id.
	ID string `json:"id"`
	// Number is the stage's number as drawn: its column in the belt.
	Number int    `json:"number"`
	Name   string `json:"name"`
	// Lines is the name as it is set, on one line or two.
	Lines []string      `json:"lines"`
	Glyph RunMetroGlyph `json:"glyph"`
	X     float64       `json:"x"`
	Y     float64       `json:"y"`
	// Label is where the name sits, under the station.
	Label RunMetroBox `json:"label"`
	// Hit is the area a click on the station reaches: its glyph and its name.
	Hit   RunMetroBox          `json:"hit"`
	State RunMetroStationState `json:"state"`
	// Status and Look are the status text and look the run page shows for this stage.
	Status string `json:"status"`
	Look   string `json:"look"`
	// Tagged is whether the station shows its status chip: a done or current stage that has a status.
	Tagged bool `json:"tagged"`
	// StatusBox is where the status chip sits, above the station's glyph.
	StatusBox RunMetroBox `json:"statusBox"`
}

// RunMetroRouteTag is a route's name, in a pill on its own track just before the route's first station.
type RunMetroRouteTag struct {
	// ID is "<fork>:<route>".
	ID    string `json:"id"`
	Fork  string `json:"fork"`
	Route string `json:"route"`
	Text  string `json:"text"`
	// X and Y are the top-left corner of the pill, and Width and Height its size. Its centre is on the route's track.
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Taken is whether the run took the route; nil while no choice has been made at its fork.
	Taken *bool `json:"taken,omitzero"`
	Feeds bool  `json:"feeds"`
}

// RunMetroPill is the belt's name at the start of the line.
type RunMetroPill struct {
	Text string      `json:"text"`
	Box  RunMetroBox `json:"box"`
}

// RunMetro is one belt on the run page as a metro line.
type RunMetro struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Colour is the belt's colour index (MetroLayoutColourIndex), into the theme's metro line colours.
	Colour   int                `json:"colour"`
	Trigger  RunMetroPill       `json:"trigger"`
	Segments []RunMetroSegment  `json:"segments"`
	Stations []RunMetroStation  `json:"stations"`
	Routes   []RunMetroRouteTag `json:"routes"`
	// Ends is where the line ends on its own: the terminus and the open end of each route that does not rejoin.
	Ends []MapPoint `json:"ends"`
}
