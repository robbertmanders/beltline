// The route names and station status chips of the Floor's metro map. The Floor's metro
// map gets them from the run page's metro, so the daemon places them and the app only draws them.

package core

// Sizes of the route name pills and status chips on the Floor's metro map, in points.
const (
	// metroMapPillHeight is a route name pill's height; its centre is on the route's track.
	metroMapPillHeight = 14.0
	// metroMapPillPad is the room a pill takes beyond its text: its border and padding.
	metroMapPillPad = 12.0
	// metroMapPillInset is how far a pill sits past the stretch where its route leaves its station.
	metroMapPillInset = 4.0
	// metroMapPillGap is the track kept between a pill and the station it comes before.
	metroMapPillGap = 8.0
	// metroMapChipHeight is a status chip's height.
	metroMapChipHeight = 16.0
	// metroMapChipPad is the room a chip takes beyond its text.
	metroMapChipPad = 16.0
	// metroMapChipAbove is the gap between a status chip and its station's glyph.
	metroMapChipAbove = 3.0
)

// MetroLayoutPill is a route's name on its track, before the route's first stop. Stop is the machine it
// comes before. Fork and Route are the route's ids; Text is its name.
type MetroLayoutPill struct {
	Fork  string
	Route string
	Text  string
	Stop  string
}

// MetroLayoutRoutePill is a route's name pill as placed on the map: the belt's line it sits on, and its box.
type MetroLayoutRoutePill struct {
	// ID is "<fork>:<route>".
	ID    string         `json:"id"`
	Fork  string         `json:"fork"`
	Route string         `json:"route"`
	Text  string         `json:"text"`
	Box   MetroLayoutBox `json:"box"`
}

// MetroLayoutStatus is a station's aggregate status chip: its text and look, and the box it sits in above the glyph.
type MetroLayoutStatus struct {
	Text string          `json:"text"`
	Look StageStatusLook `json:"look"`
	Box  MetroLayoutBox  `json:"box"`
}

// metroMapPillWidth is the width of a route name pill with text.
func (m MetroLayoutMap) metroMapPillWidth(text string) float64 {
	return m.textWidth(text) + metroMapPillPad
}

// metroMapChipWidth is the width of a status chip with text.
func (m MetroLayoutMap) metroMapChipWidth(text string) float64 {
	return m.textWidth(text) + metroMapChipPad
}

// pillsAt are the name pills of lane that come before column c, outermost route first. A route that starts with a
// fork shares its first stop with that fork's route, and the walk records the inner route's pill before the outer's.
func (m MetroLayoutMap) pillsAt(g, lane, c int) []MetroLayoutPill {
	path := m.Groups[g].Plan.Lanes[lane].Path
	var pills []MetroLayoutPill
	for i := len(path.Pills) - 1; i >= 0; i-- {
		if s := m.spot(&path.Pills[i].Stop); s != nil && s.Column == c {
			pills = append(pills, path.Pills[i])
		}
	}
	return pills
}

// lastStop is the column of the last stop a lane makes before column c, and its machine, when there is one.
func (m MetroLayoutMap) lastStop(g, lane, c int) (int, string, bool) {
	path := m.Groups[g].Plan.Lanes[lane].Path
	column, machine, found := -1, "", false
	for i := range path.Stops {
		if s := m.spot(&path.Stops[i]); s != nil && s.Column < c && s.Column > column {
			column, machine, found = s.Column, path.Stops[i], true
		}
	}
	return column, machine, found
}

// stationReach is how far past a station the furthest dot of any belt there reaches. A name pill that starts
// at the station keeps clear of every belt's dots, since the belts share the station's column.
func (m MetroLayoutMap) stationReach(machine string) float64 {
	spot := m.spot(&machine)
	if spot == nil {
		return 0
	}
	reach := 0.0
	for belt := range spot.Members {
		if r := m.dotReach(machine, belt); r > reach {
			reach = r
		}
	}
	return reach
}

// pillRoom is the track a lane needs before column c for its routes' name pills, one after another: zero when no
// pill comes there.
func (m MetroLayoutMap) pillRoom(g, lane, c int) float64 {
	room := 0.0
	for _, pill := range m.pillsAt(g, lane, c) {
		room += m.metroMapPillWidth(pill.Text) + metroMapPillGap
	}
	return room
}

// spotStatus is a station's aggregate status: the runs at its machine across every belt, read the way the
// boxes Floor reads them, so held and waiting come first, then running, else idle.
func (m MetroLayoutMap) spotStatus(spot MetroLayoutSpot) (string, StageStatusLook) {
	machine := m.Track.Machine(spot.ID)
	if machine == nil {
		return OverviewBoxesStatus(nil)
	}
	return OverviewBoxesStatus(m.Track.RunsAt(*machine, m.Runs))
}

// statusBox is the box a station's status chip sits in, above its glyph: its foot clears the glyph by a little.
func (m MetroLayoutMap) statusBox(station MetroLayoutStation, text string) MetroLayoutBox {
	width := m.metroMapChipWidth(text)
	return MetroLayoutBox{X: station.X - width/2, Y: station.Box().Y - metroMapChipAbove - metroMapChipHeight,
		Width: width, Height: metroMapChipHeight}
}
