//
// The Floor's belts, machines or boxes and run chips as values, as the API
// sends them.

package core

import (
	"sort"
	"time"
)

// OverviewFloorBelt is one entrance of the Floor, with its counts.
type OverviewFloorBelt struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Summary is the belt's own one-line summary, from its trigger and repositories.
	Summary string               `json:"summary"`
	Paused  bool                 `json:"paused"`
	Counts  FactorySummaryCounts `json:"counts"`
}

// OverviewFloorChip is a run as a chip below its machine or under its box.
type OverviewFloorChip struct {
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
	// Belt is the belt's short name: always on a box, and on a machine only when more than one belt shares it.
	Belt   *string  `json:"belt,omitzero"`
	State  RunState `json:"state"`
	Paused bool     `json:"paused"`
	// OpenStage is where the chip opens: the run's stage when a person is needed, else the run.
	OpenStage *string `json:"openStage,omitzero"`
}

// OverviewFloorMachine is one machine on the machines Floor, drawn with its MachineLook.
type OverviewFloorMachine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Number is the layer the machine sits in, counted from the entrances.
	Number    int      `json:"number"`
	Belts     []string `json:"belts"`
	BeltNames []string `json:"beltNames"`
	Mixed     bool     `json:"mixed"`
	// Workers is one line per member stage: "belt [id]: worker", for the MIXED tag.
	Workers string      `json:"workers"`
	Status  StageStatus `json:"status"`
	MachineLook
	RunCount int `json:"runCount"`
	// Chips are up to five chips; above five, the first four and More.
	Chips []OverviewFloorChip `json:"chips"`
	More  int                 `json:"more"`
	// Dimmed is true while no active run is at the machine.
	Dimmed bool `json:"dimmed"`
}

// OverviewFloorFrame is a rectangle in the layout's points.
type OverviewFloorFrame struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// OverviewFloorBoxStatus is a box's status: its text and colour look.
type OverviewFloorBoxStatus struct {
	Text string          `json:"text"`
	Look StageStatusLook `json:"look"`
}

// OverviewFloorBox is a merged stage on the boxes Floor: its icon and colour (StageLook), its caption and
// status, its run tags and the cell it sits in on the layout (OverviewFloorFrame: the box, its status
// above and its tags below).
type OverviewFloorBox struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Caption is "Agent", "Approval" or "Watch", with the number of belts when several share it.
	Caption string `json:"caption"`
	StageLook
	Belts    []string               `json:"belts"`
	Status   OverviewFloorBoxStatus `json:"status"`
	RunCount int                    `json:"runCount"`
	// Chips are up to three run tags, then More for the rest.
	Chips []OverviewFloorChip `json:"chips"`
	More  int                 `json:"more"`
	OverviewFloorFrame
}

// OverviewFloor is the Floor for one look: entrances with their counts, and the look's drawing. The
// machines look has its machines and the boxes look its boxes, each with the one drawn layout, in
// points; the metro has neither, only its own drawing.
type OverviewFloor struct {
	Total FactorySummaryCounts `json:"total"`
	Belts []OverviewFloorBelt  `json:"belts"`
	// Machines are for the machines look only; nil for the others.
	Machines []OverviewFloorMachine `json:"machines,omitzero"`
	// Boxes are for the boxes look only; nil for the others.
	Boxes []OverviewFloorBox `json:"boxes,omitzero"`
	// Layout is the machines' or the boxes' drawing; nil for the metro.
	Layout *FloorLayout `json:"layout,omitzero"`
}

// OverviewFloorChipLimit is the most chips a machine shows before it collapses to four and a count.
const OverviewFloorChipLimit = 5

// OverviewFloorArgs are the parameters of NewOverviewFloor. Layout is nil for the metro;
// Agent may be nil, and finds the agent a stage runs. Location nil is the local calendar.
type OverviewFloorArgs struct {
	Track    FactoryTrack
	Belts    []OverviewBelt
	Runs     []Run
	Now      time.Time
	Location *time.Location
	Pause    Pause
	Look     BeltLook
	Layout   *FloorLayout
	Agent    func(stage Stage) *Agent
}

// overviewFloorRank is the sort rank of each run state on the Floor; any other state ranks 9.
var overviewFloorRank = map[RunState]int{RunStateRunning: 0, RunStateWaiting: 1, RunStateHeld: 2, RunStateQueued: 3, RunStateParked: 4, RunStateDropped: 5}

// overviewFloorByState sorts runs by their Floor rank, keeping the order of equal ranks.
func overviewFloorByState(here []Run) []Run {
	sorted := make([]Run, len(here))
	copy(sorted, here)
	rank := func(run Run) int {
		if value, ok := overviewFloorRank[run.State]; ok {
			return value
		}
		return 9
	}
	sort.SliceStable(sorted, func(i, j int) bool { return rank(sorted[i]) < rank(sorted[j]) })
	return sorted
}

// overviewFloorChip is a run as a chip; belt is its short belt name when the look shows one.
func overviewFloorChip(run Run, belt *string, pausedBelts map[string]struct{}) OverviewFloorChip {
	_, paused := pausedBelts[run.BeltID]
	return OverviewFloorChip{
		RunID:     run.ID,
		Ref:       run.Subject.Ref(),
		Belt:      belt,
		State:     run.State,
		Paused:    paused,
		OpenStage: WorkRowPresentationOpenStage(run),
	}
}

// NewOverviewFloor builds the Floor for a.Look. Layout is the placement the look draws
// with, which gives each box its frame.
func NewOverviewFloor(a OverviewFloorArgs) OverviewFloor {
	loc := a.Location
	if loc == nil {
		loc = time.Local
	}
	agent := a.Agent
	if agent == nil {
		agent = func(Stage) *Agent { return nil }
	}
	counted, pausedBelts := overviewFloorCounted(a.Belts)
	summary := NewFactorySummary(counted, a.Runs, a.Now, loc)
	// Past the summary only the runs the Floor draws are read, a machine at a time.
	a.Runs = floorRuns(a.Runs)

	floor := OverviewFloor{
		Belts:  overviewFloorBelts(a.Belts, summary),
		Total:  summary.Total,
		Layout: a.Layout,
	}
	switch a.Look {
	case BeltLookMachines:
		floor.Machines = overviewFloorMachines(a, pausedBelts, agent)
	case BeltLookBoxes:
		floor.Boxes = overviewFloorBoxes(a, pausedBelts, agent)
	case BeltLookMetro:
	}
	return floor
}

// overviewFloorCounted lists the belts for the factory summary, and the IDs of the paused ones the chips read.
func overviewFloorCounted(belts []OverviewBelt) ([]FactorySummaryBeltInput, map[string]struct{}) {
	counted := make([]FactorySummaryBeltInput, 0, len(belts))
	pausedBelts := map[string]struct{}{}
	for _, belt := range belts {
		counted = append(counted, FactorySummaryBeltInput{ID: belt.ID, Name: belt.Name, Paused: belt.Paused})
		if belt.Paused {
			pausedBelts[belt.ID] = struct{}{}
		}
	}
	return counted, pausedBelts
}

// overviewFloorBelts is each belt with its counts from the factory summary, in the order given.
func overviewFloorBelts(belts []OverviewBelt, summary FactorySummary) []OverviewFloorBelt {
	out := make([]OverviewFloorBelt, 0, len(belts))
	for _, belt := range belts {
		var counts FactorySummaryCounts
		for _, summarised := range summary.Belts {
			if summarised.ID == belt.ID {
				counts = summarised.Counts
				break
			}
		}
		out = append(out, OverviewFloorBelt{ID: belt.ID, Name: belt.Name, Summary: belt.Summary, Paused: belt.Paused, Counts: counts})
	}
	return out
}

// overviewFloorMachines is one machine per track machine, in track order, for the machines look.
func overviewFloorMachines(a OverviewFloorArgs, pausedBelts map[string]struct{}, agent func(Stage) *Agent) []OverviewFloorMachine {
	machines := []OverviewFloorMachine{}
	for _, machine := range a.Track.Machines {
		machines = append(machines, overviewFloorMachine(a, machine, pausedBelts, agent))
	}
	return machines
}

// overviewFloorMachine is one machine on the machines Floor: its runs as chips (collapsed past the chip
// limit), its belts, its layer and its status.
func overviewFloorMachine(a OverviewFloorArgs, machine FactoryTrackMachine, pausedBelts map[string]struct{}, agent func(Stage) *Agent) OverviewFloorMachine {
	here := overviewFloorByState(a.Track.RunsAt(machine, a.Runs))
	shared := len(machine.Belts()) > 1
	shown := here
	if len(here) > OverviewFloorChipLimit {
		shown = here[:OverviewFloorChipLimit-1]
	}
	chips := make([]OverviewFloorChip, 0, len(shown))
	for _, run := range shown {
		var belt *string
		if shared {
			name := FactoryTrackShortName(run.BeltName)
			belt = &name
		}
		chips = append(chips, overviewFloorChip(run, belt, pausedBelts))
	}
	more := 0
	if len(here) > OverviewFloorChipLimit {
		more = len(here) - (OverviewFloorChipLimit - 1)
	}
	return OverviewFloorMachine{
		ID:          machine.ID,
		Name:        machine.Name,
		Number:      overviewFloorLayer(a.Track, machine),
		Belts:       machine.Belts(),
		BeltNames:   overviewFloorBeltNames(a.Track, machine),
		Mixed:       machine.Mixed(),
		Workers:     machine.Workers(),
		Status:      StatusColumn(here, pausedBelts, a.Pause),
		MachineLook: MachineLookOf(machine.Members[0].Stage, agent(machine.Members[0].Stage)),
		RunCount:    len(here),
		Chips:       chips,
		More:        more,
		Dimmed:      len(here) == 0,
	}
}

// overviewFloorLayer is the layer a machine sits in, counted from the entrances; 0 when the track has no layer for it.
func overviewFloorLayer(track FactoryTrack, machine FactoryTrackMachine) int {
	if layer := track.LayerOf(FactoryTrackMachineNode(machine.ID)); layer != nil {
		return *layer
	}
	return 0
}

// overviewFloorBeltNames is the name of each belt that goes through the machine, in the machine's belt order.
func overviewFloorBeltNames(track FactoryTrack, machine FactoryTrackMachine) []string {
	names := []string{}
	for _, id := range machine.Belts() {
		if belt := track.Belt(id); belt != nil {
			names = append(names, belt.Name)
		}
	}
	return names
}

// overviewFloorBoxes is one box per track machine, in track order, for the boxes look.
func overviewFloorBoxes(a OverviewFloorArgs, pausedBelts map[string]struct{}, agent func(Stage) *Agent) []OverviewFloorBox {
	boxes := []OverviewFloorBox{}
	for _, machine := range a.Track.Machines {
		boxes = append(boxes, overviewFloorBox(a, machine, pausedBelts, agent))
	}
	return boxes
}

// overviewFloorBox is one merged stage on the boxes Floor: its caption and status, up to three run tags
// (then More), and its frame from the layout.
func overviewFloorBox(a OverviewFloorArgs, machine FactoryTrackMachine, pausedBelts map[string]struct{}, agent func(Stage) *Agent) OverviewFloorBox {
	here := overviewFloorByState(a.Track.RunsAt(machine, a.Runs))
	shown := here
	if len(here) > OverviewBoxesTagLimit {
		shown = here[:OverviewBoxesTagLimit]
	}
	stage := machine.Members[0].Stage
	statusText, statusLook := OverviewBoxesStatus(here)
	frame := OverviewFloorFrame{}
	if a.Layout != nil {
		if placed := a.Layout.Box(FactoryTrackMachineNode(machine.ID)); placed != nil {
			frame = OverviewBoxesFrame(*placed)
		}
	}
	chips := make([]OverviewFloorChip, 0, len(shown))
	for _, run := range shown {
		name := FactoryTrackShortName(run.BeltName)
		chips = append(chips, overviewFloorChip(run, &name, pausedBelts))
	}
	more := 0
	if len(here) > OverviewBoxesTagLimit {
		more = len(here) - OverviewBoxesTagLimit
	}
	return OverviewFloorBox{
		ID:                 machine.ID,
		Name:               machine.Name,
		Caption:            OverviewBoxesCaption(stage, len(machine.Belts())),
		StageLook:          StageLookOf(stage, agent(stage)),
		Belts:              machine.Belts(),
		Status:             OverviewFloorBoxStatus{Text: statusText, Look: statusLook},
		RunCount:           len(here),
		Chips:              chips,
		More:               more,
		OverviewFloorFrame: frame,
	}
}
