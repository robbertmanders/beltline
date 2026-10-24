//
// The Overview's floor eligibility and layout inputs, shared by the API.
// Handlers only hand these functions their coordinator snapshot, and the API sends
// what they return as it is.

package core

import (
	"sort"
	"time"
)

// OverviewBelt is a belt as the Overview needs it: its identity and layout, plus the record
// state the Floor's eligibility and summary read.
type OverviewBelt struct {
	ID       string
	Name     string
	Summary  string
	Layout   BeltLayout
	Archived bool
	Paused   bool
}

// OverviewBeltFromBelt is an Overview belt for belt with the given archive and pause state.
func OverviewBeltFromBelt(belt Belt, archived, paused bool) OverviewBelt {
	return OverviewBelt{ID: belt.ID, Name: belt.Name, Summary: belt.Summary(), Layout: belt.Layout(), Archived: archived, Paused: paused}
}

// OverviewPresentationShowsOnFloor reports whether a belt is on the Floor:
// every live belt, plus an archived belt while it still has an active run.
func OverviewPresentationShowsOnFloor(archived bool, beltID string, runs []Run) bool {
	if !archived {
		return true
	}
	for i := range runs {
		if runs[i].BeltID == beltID && runs[i].State.IsActive() {
			return true
		}
	}
	return false
}

// OverviewPresentationFloorBelts is the belts the Floor shows, in the library's order.
func OverviewPresentationFloorBelts(belts []OverviewBelt, runs []Run) []OverviewBelt {
	floor := []OverviewBelt{}
	for _, belt := range belts {
		if OverviewPresentationShowsOnFloor(belt.Archived, belt.ID, runs) {
			floor = append(floor, belt)
		}
	}
	return floor
}

// OverviewPresentationTrack is the factory track the Floor draws, from the eligible belts.
func OverviewPresentationTrack(belts []OverviewBelt) FactoryTrack {
	tracked := make([]FactoryTrackBelt, 0, len(belts))
	for _, belt := range belts {
		tracked = append(tracked, FactoryTrackBelt{ID: belt.ID, Name: belt.Name, Layout: belt.Layout})
	}
	return NewFactoryTrack(tracked)
}

// OverviewRun is an active run as the Overview's cards name it: its subject, repository,
// where it is, and the stage a click opens it at. Floor chips and the Summary rows
// look their runs up here.
type OverviewRun struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Title string `json:"title"`
	// StageName is the stage the run is at, by name, as the Running post names it.
	StageName  string      `json:"stageName"`
	Repository string      `json:"repository"`
	BeltID     string      `json:"beltId"`
	BeltName   string      `json:"beltName"`
	Location   StatusWhere `json:"location"`
	NeedsYou   bool        `json:"needsYou"`
	OpenStage  *string     `json:"openStage,omitzero"`
}

// OverviewView is everything the Overview shows for one read: the global counts, the Summary,
// and the Floor.
type OverviewView struct {
	Summary RunCounts
	Today   OverviewSummary
	Floor   OverviewFloor
	// FactoryTrack is the track the Floor was built from.
	FactoryTrack FactoryTrack
	// Runs is every active run the Overview names, by id.
	Runs map[string]OverviewRun
	// Greeting is the greeting for the clock the view was read at.
	Greeting string
}

// OverviewViewArgs are the parameters of NewOverviewView. Nil pointers and a nil Location
// are the defaults: the local calendar, and the Floor drawn at its minimum column.
type OverviewViewArgs struct {
	Belts    []OverviewBelt
	Runs     []Run
	Now      time.Time
	Location *time.Location
	Pause    Pause
	Width    *float64
	Agent    func(stage Stage) *Agent
	// Track is OverviewPresentationTrack of the floor belts when the caller keeps it; nil builds it.
	Track *FactoryTrack
}

// NewOverviewView builds the Overview for one read.
func NewOverviewView(a OverviewViewArgs) OverviewView {
	loc := a.Location
	if loc == nil {
		loc = time.Local
	}
	agent := a.Agent
	if agent == nil {
		agent = func(Stage) *Agent { return nil }
	}
	floorBelts := OverviewPresentationFloorBelts(a.Belts, a.Runs)
	var track FactoryTrack
	if a.Track != nil {
		track = *a.Track
	} else {
		track = OverviewPresentationTrack(floorBelts)
	}
	// The layout looks runs up per box, and only active runs sit at one.
	active := floorRuns(a.Runs)
	paused := map[string]struct{}{}
	for _, belt := range a.Belts {
		if belt.Paused {
			paused[belt.ID] = struct{}{}
		}
	}
	layout := NewFloorLayout(track, OverviewBoxesMetrics(), a.Width, func(merged FactoryTrackStage) float64 {
		return OverviewBoxesHeight(len(track.RunsAt(merged, active)))
	})

	view := OverviewView{
		Summary:      RunSummaryCounts(a.Runs, a.Now, loc),
		Today:        NewOverviewSummary(a.Runs, a.Now, loc, paused, a.Pause),
		FactoryTrack: track,
	}
	view.Floor = NewOverviewFloor(OverviewFloorArgs{
		Track:    track,
		Belts:    floorBelts,
		Runs:     a.Runs,
		Now:      a.Now,
		Location: loc,
		Layout:   layout,
		Agent:    agent,
	})
	view.Greeting = OverviewSummaryGreeting(a.Now, loc)
	ambiguous := RepoLabelAmbiguousNames(a.Runs)
	view.Runs = map[string]OverviewRun{}
	for i := range a.Runs {
		if !a.Runs[i].State.IsActive() {
			continue
		}
		run := a.Runs[i]
		_, isPaused := paused[run.BeltID]
		stageName := run.StageID
		if stage := run.stageRef(); stage != nil {
			stageName = stage.Name
		}
		view.Runs[run.ID] = OverviewRun{
			ID:         run.ID,
			Ref:        run.Subject.Ref(),
			Title:      run.Subject.Title,
			StageName:  stageName,
			Repository: RepoLabelShort(run.Subject.Repo, ambiguous),
			BeltID:     run.BeltID,
			BeltName:   run.BeltName,
			Location:   StatusLocation(run, isPaused, a.Pause),
			NeedsYou:   run.State.NeedsYou(),
			OpenStage:  WorkRowPresentationOpenStage(run),
		}
	}
	return view
}

// floorRuns is the runs the Floor draws, in order: the active ones, apart from
// runs on a shadow version, whose stages the Floor's live belts may not have.
func floorRuns(runs []Run) []Run {
	out := []Run{}
	for i := range runs {
		if runs[i].State.IsActive() && !runs[i].OnShadow() {
			out = append(out, runs[i])
		}
	}
	return out
}

// OrderedRuns returns the Overview's runs sorted by id, the order the Overview lists
// them in.
func (v OverviewView) OrderedRuns() []OverviewRun {
	runs := make([]OverviewRun, 0, len(v.Runs))
	for _, run := range v.Runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs
}
