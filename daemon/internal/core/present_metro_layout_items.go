package core

import (
	"maps"
	"slices"
	"sort"
	"strconv"
)

// MetroLayoutCard is the text a run dot's card shows: a headline, a detail line and a hint, and where
// the card sits beside its dot.
type MetroLayoutCard struct {
	Headline string  `json:"headline"`
	Detail   *string `json:"detail,omitzero"`
	Hint     string  `json:"hint"`
	// Left, Top and Width are where the card's left edge, top and width sit beside its dot; set once the map's width is known.
	Left  float64 `json:"left"`
	Top   float64 `json:"top"`
	Width float64 `json:"width"`
}

// Place puts the card beside its dot: to the right, or to the left when there is no room on the right, and
// never above the map.
func (c *MetroLayoutCard) Place(dot MetroLayoutPoint, mapWidth float64) {
	if dot.X+14+MetroLayoutCardWidth > mapWidth {
		c.Left = dot.X - 14 - MetroLayoutCardWidth
	} else {
		c.Left = dot.X + 14
	}
	c.Top = metroMax(30, dot.Y-34)
	c.Width = MetroLayoutCardWidth
}

// MetroLayoutCardRun is a run's card: its reference and title, its repository and where it stands.
func MetroLayoutCardRun(run Run, paused bool, pause Pause, ambiguous map[string]struct{}) MetroLayoutCard {
	detail := RepoLabelShort(run.Subject.Repo, ambiguous) + " · " + StatusLocation(run, paused, pause).Text
	return MetroLayoutCard{
		Headline: run.Subject.Ref() + " " + run.Subject.Title,
		Detail:   &detail,
		Hint:     "Click to open",
		Width:    MetroLayoutCardWidth,
	}
}

// MetroLayoutCardMore is the card of a dot gathering more runs than it shows.
func MetroLayoutCardMore(count int) MetroLayoutCard {
	return MetroLayoutCard{
		Headline: strconv.Itoa(count) + " more runs here",
		Hint:     "Click to see them",
		Width:    MetroLayoutCardWidth,
	}
}

// MetroLayoutGlyph is the shape a station is drawn as.
type MetroLayoutGlyph string

const (
	MetroLayoutGlyphCircle      MetroLayoutGlyph = "circle"
	MetroLayoutGlyphDiamond     MetroLayoutGlyph = "diamond"
	MetroLayoutGlyphSquare      MetroLayoutGlyph = "square"
	MetroLayoutGlyphInterchange MetroLayoutGlyph = "interchange"
)

// MetroLayoutStation is a stop on the map: a stage, or an interchange of several belts.
type MetroLayoutStation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Lines is the name as it is set, on one line or two.
	Lines []string         `json:"lines"`
	Glyph MetroLayoutGlyph `json:"glyph"`
	// Kind is the stage's own shape, which an interchange keeps in its corners:
	// square for a watch, a little rounded for an agent or an approval.
	Kind MetroLayoutGlyph `json:"kind"`
	X    float64          `json:"x"`
	// Stops is the y each belt stops at. An interchange spans the highest to the lowest.
	Stops   map[string]float64 `json:"stops"`
	NameBox MetroLayoutBox     `json:"nameBox"`
	// Pieces are the stretches of an interchange drawn as one piece; a line that runs between two of them,
	// without stopping, passes over their link.
	Pieces []MetroRange `json:"pieces"`
	// Status is the station's aggregate status chip, above its glyph.
	Status MetroLayoutStatus `json:"status"`
}

// Top is the highest stop.
func (s MetroLayoutStation) Top() float64 {
	if len(s.Stops) == 0 {
		return 0
	}
	first := true
	best := 0.0
	for _, y := range s.Stops {
		if first || y < best {
			best = y
			first = false
		}
	}
	return best
}

// Bottom is the lowest stop.
func (s MetroLayoutStation) Bottom() float64 {
	if len(s.Stops) == 0 {
		return 0
	}
	first := true
	best := 0.0
	for _, y := range s.Stops {
		if first || y > best {
			best = y
			first = false
		}
	}
	return best
}

// Belts are the belts that stop here, ordered by their stop.
func (s MetroLayoutStation) Belts() []string {
	belts := slices.Collect(maps.Keys(s.Stops))
	sort.Slice(belts, func(a, b int) bool {
		if s.Stops[belts[a]] != s.Stops[belts[b]] {
			return s.Stops[belts[a]] < s.Stops[belts[b]]
		}
		return belts[a] < belts[b]
	})
	return belts
}

// Box is the area the glyph covers.
func (s MetroLayoutStation) Box() MetroLayoutBox {
	half := 7.5
	reach := 7.5
	if s.Glyph == MetroLayoutGlyphInterchange {
		half = MetroLayoutInterchangeWidth/2 + 1
		reach = MetroLayoutOverhang
	}
	top, bottom := s.Top(), s.Bottom()
	return MetroLayoutBox{X: s.X - half, Y: top - reach, Width: half * 2, Height: bottom - top + float64(reach*2)}
}

// MetroLayoutLook is how a run dot is drawn. Shipped and dropped runs are absent.
type MetroLayoutLook string

const (
	MetroLayoutLookRunning MetroLayoutLook = "running"
	MetroLayoutLookWaiting MetroLayoutLook = "waiting"
	MetroLayoutLookHeld    MetroLayoutLook = "held"
	MetroLayoutLookQueued  MetroLayoutLook = "queued"
	MetroLayoutLookParked  MetroLayoutLook = "parked"
)

// Halo reports whether the dot has a halo: waiting and held runs.
func (l MetroLayoutLook) Halo() bool {
	return l == MetroLayoutLookWaiting || l == MetroLayoutLookHeld
}

// MetroLayoutLookFor is the look of a run state, or nil for a state that has no dot (shipped and dropped).
func MetroLayoutLookFor(state RunState) *MetroLayoutLook {
	var look MetroLayoutLook
	switch state {
	case RunStateRunning:
		look = MetroLayoutLookRunning
	case RunStateWaiting:
		look = MetroLayoutLookWaiting
	case RunStateHeld:
		look = MetroLayoutLookHeld
	case RunStateQueued:
		look = MetroLayoutLookQueued
	case RunStateParked:
		look = MetroLayoutLookParked
	default:
		return nil
	}
	return &look
}

// MetroLayoutDot is a run dot on a station, or the "+n" mark gathering several.
type MetroLayoutDot struct {
	ID        string          `json:"id"`
	RunIDs    []string        `json:"runIds"`
	BeltID    string          `json:"beltId"`
	StationID string          `json:"stationId"`
	Look      MetroLayoutLook `json:"look"`
	Halo      bool            `json:"halo"`
	Place     int             `json:"place"`
	// Overflow is the runs gathered into the "+n" mark. Zero on a run's own dot.
	Overflow int              `json:"overflow"`
	At       MetroLayoutPoint `json:"at"`
	// Card is what the dot's card says when it is pointed at.
	Card MetroLayoutCard `json:"card"`
}

// MetroLayoutLabel is a run label: the text under or over a station's dots.
type MetroLayoutLabel struct {
	Text     string         `json:"text"`
	Box      MetroLayoutBox `json:"box"`
	NeedsYou bool           `json:"needsYou"`
	// Urgent is Held or Waiting when the label is a pill; nil for plain text.
	Urgent *MetroLayoutLook `json:"urgent,omitzero"`
	RunIDs []string         `json:"runIds"`
	BeltID string           `json:"beltId"`
	Above  bool             `json:"above"`
	// Dot is the first dot it names, which a leader joins it to.
	Dot MetroLayoutPoint `json:"dot"`
	// OpenRunID is the run a click opens: the first that needs you, else the first.
	OpenRunID *string `json:"openRunId,omitzero"`
}

// MetroLayout is the factory as a transit map, in points.
type MetroLayout struct {
	// Lines and Stations go out with what they derive (api.MetroLayoutWire).
	Lines    []MetroLayoutLine    `json:"-"`
	Stations []MetroLayoutStation `json:"-"`
	Dots     []MetroLayoutDot     `json:"dots"`
	Labels   []MetroLayoutLabel   `json:"labels"`
	Width    float64              `json:"width"`
	Height   float64              `json:"height"`
}

// MetroLayoutArgs are the parameters of NewMetroLayout. CharWidth nil is 6.6; Paused and Pause are the
// defaults (none paused, the live pause) when zero.
type MetroLayoutArgs struct {
	Track     FactoryTrack
	Runs      []Run
	CharWidth *float64
	Paused    map[string]struct{}
	Pause     Pause
}

// NewMetroLayout lays the map out for track and the runs the Floor would show. paused are the belts paused by
// their own switch; pause is the global pause. Both shape the cards' locations.
func NewMetroLayout(a MetroLayoutArgs) MetroLayout {
	charWidth := ValueOr(a.CharWidth, 6.6)
	routes := make([]MetroLayoutRoute, 0, len(a.Track.Belts))
	for _, belt := range a.Track.Belts {
		routes = append(routes, metroLayoutRoute(belt, a.Track))
	}
	builder := NewMetroLayoutMap(routes, a.Track, a.Runs, charWidth, a.Paused, a.Pause)
	builder.build()
	layout := MetroLayout{
		Lines:    builder.Lines,
		Stations: builder.Stations,
		Dots:     make([]MetroLayoutDot, 0, len(builder.Dots)),
		Labels:   builder.Labels,
		Width:    builder.Width,
		Height:   builder.Height,
	}
	for _, dot := range builder.Dots {
		dot.Card.Place(dot.At, builder.Width)
		layout.Dots = append(layout.Dots, dot)
	}
	return layout
}

// Line is the line of belt id, or nil.
func (l MetroLayout) Line(id string) *MetroLayoutLine {
	for i := range l.Lines {
		if l.Lines[i].BeltID == id {
			return &l.Lines[i]
		}
	}
	return nil
}

// metroLayoutWrap is a station name on one line, or on two at the space nearest its middle when it is long.
// limit defaults to 15.
func metroLayoutWrap(name string, limit int) []string {
	chars := []rune(name)
	if len(chars) <= limit {
		return []string{name}
	}
	middle := len(chars) / 2
	cut := -1
	for i, c := range chars {
		if c != ' ' {
			continue
		}
		if cut == -1 || absInt(i-middle) < absInt(cut-middle) {
			cut = i
		}
	}
	if cut == -1 {
		return []string{name}
	}
	return []string{string(chars[:cut]), string(chars[cut+1:])}
}

// metroLayoutStateWord is the word a group of runs reads as: held, you, running, queued, else parked.
func metroLayoutStateWord(runs []Run) string {
	has := func(state RunState) bool {
		return slices.ContainsFunc(runs, func(run Run) bool { return run.State == state })
	}
	switch {
	case has(RunStateHeld):
		return "held"
	case has(RunStateWaiting):
		return "you"
	case has(RunStateRunning):
		return "running"
	case has(RunStateQueued):
		return "queued"
	default:
		return "parked"
	}
}
