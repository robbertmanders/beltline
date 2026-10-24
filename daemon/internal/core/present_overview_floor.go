//
// The Floor's belts, boxes and run chips as values, as the API sends them.

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

// OverviewFloorChip is a run as a tag under its box.
type OverviewFloorChip struct {
	RunID string `json:"runId"`
	Ref   string `json:"ref"`
	// Belt is the belt's short name.
	Belt   *string  `json:"belt,omitzero"`
	State  RunState `json:"state"`
	Paused bool     `json:"paused"`
	// OpenStage is where the chip opens: the run's stage when a person is needed, else the run.
	OpenStage *string `json:"openStage,omitzero"`
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

// OverviewFloorBox is a merged stage on the Floor: its icon and colour (StageLook), its caption and
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

// OverviewFloor is the Floor: entrances with their counts, and its boxes with the layout they are
// drawn on, in points.
type OverviewFloor struct {
	Total  FactorySummaryCounts `json:"total"`
	Belts  []OverviewFloorBelt  `json:"belts"`
	Boxes  []OverviewFloorBox   `json:"boxes"`
	Layout FloorLayout          `json:"layout"`
}

// OverviewFloorArgs are the parameters of NewOverviewFloor. Agent may be nil, and finds the agent a
// stage runs. Location nil is the local calendar.
type OverviewFloorArgs struct {
	Track    FactoryTrack
	Belts    []OverviewBelt
	Runs     []Run
	Now      time.Time
	Location *time.Location
	Layout   FloorLayout
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

// overviewFloorChip is a run as a chip; belt is its short belt name.
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

// NewOverviewFloor builds the Floor. Layout is the placement it draws with, which gives each box
// its frame.
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
	// Past the summary only the runs the Floor draws are read, a box at a time.
	a.Runs = floorRuns(a.Runs)

	return OverviewFloor{
		Belts:  overviewFloorBelts(a.Belts, summary),
		Total:  summary.Total,
		Boxes:  overviewFloorBoxes(a, pausedBelts, agent),
		Layout: a.Layout,
	}
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

// overviewFloorBoxes is one box per track stage, in track order.
func overviewFloorBoxes(a OverviewFloorArgs, pausedBelts map[string]struct{}, agent func(Stage) *Agent) []OverviewFloorBox {
	boxes := []OverviewFloorBox{}
	for _, merged := range a.Track.Stages {
		boxes = append(boxes, overviewFloorBox(a, merged, pausedBelts, agent))
	}
	return boxes
}

// overviewFloorBox is one merged stage on the Floor: its caption and status, up to three run tags
// (then More), and its frame from the layout.
func overviewFloorBox(a OverviewFloorArgs, merged FactoryTrackStage, pausedBelts map[string]struct{}, agent func(Stage) *Agent) OverviewFloorBox {
	here := overviewFloorByState(a.Track.RunsAt(merged, a.Runs))
	shown := here
	if len(here) > OverviewBoxesTagLimit {
		shown = here[:OverviewBoxesTagLimit]
	}
	stage := merged.Members[0].Stage
	statusText, statusLook := OverviewBoxesStatus(here)
	frame := OverviewFloorFrame{}
	if placed := a.Layout.Box(FactoryTrackStageNode(merged.ID)); placed != nil {
		frame = OverviewBoxesFrame(*placed)
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
		ID:                 merged.ID,
		Name:               merged.Name,
		Caption:            OverviewBoxesCaption(stage, len(merged.Belts())),
		StageLook:          StageLookOf(stage, agent(stage)),
		Belts:              merged.Belts(),
		Status:             OverviewFloorBoxStatus{Text: statusText, Look: statusLook},
		RunCount:           len(here),
		Chips:              chips,
		More:               more,
		OverviewFloorFrame: frame,
	}
}
