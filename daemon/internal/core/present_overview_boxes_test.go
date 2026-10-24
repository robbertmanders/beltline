// Tests of the boxes Overview: the pure tests, and the tests that build an
// OverviewView (the belts, runs and layouts of the Floor).

package core

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestACellHoldsItsStatusAndUpToThreeTagsThenACount
func TestACellHoldsItsStatusAndUpToThreeTagsThenACount(t *testing.T) {
	t.Parallel()
	if got := OverviewBoxesHeight(0); got != OverviewBoxesStatusHeight+OverviewBoxesBoxHeight {
		t.Errorf("height(0) = %v", got)
	}
	if got := OverviewBoxesTagRows(0); got != 0 {
		t.Errorf("tagRows(0) = %d", got)
	}
	if got := OverviewBoxesTagRows(3); got != 3 {
		t.Errorf("tagRows(3) = %d", got)
	}
	if got := OverviewBoxesTagRows(4); got != 4 {
		t.Errorf("tagRows(4) = %d", got)
	}
	if got := OverviewBoxesTagRows(9); got != 4 {
		t.Errorf("tagRows(9) = %d", got)
	}
	threeTags := OverviewBoxesStatusHeight + OverviewBoxesBoxHeight + OverviewBoxesTagsGap + 3*OverviewBoxesTagHeight
	if got := OverviewBoxesHeight(3); got != threeTags {
		t.Errorf("height(3) = %v, want %v", got, threeTags)
	}
	if OverviewBoxesHeight(9) != OverviewBoxesHeight(4) {
		t.Errorf("height(9) = %v, height(4) = %v", OverviewBoxesHeight(9), OverviewBoxesHeight(4))
	}
}

// TestTheStatusReadsHeldThenWaitingThenRunningThenIdle
func TestTheStatusReadsHeldThenWaitingThenRunningThenIdle(t *testing.T) {
	t.Parallel()
	held := Run{State: RunStateHeld}
	running := Run{State: RunStateRunning}
	waiting := Run{State: RunStateWaiting}
	cases := []struct {
		name string
		runs []Run
		text string
		look StageStatusLook
	}{
		{"held and running", []Run{held, running}, "1 held · 1 running", StageStatusLookHeld},
		{"held", []Run{held}, "1 held", StageStatusLookHeld},
		{"waiting and running", []Run{waiting, running}, "1 waiting for you", StageStatusLookWaiting},
		{"running", []Run{running, running}, "2 running", StageStatusLookRunning},
		{"idle", nil, "Idle", StageStatusLookPlain},
	}
	for _, c := range cases {
		text, look := OverviewBoxesStatus(c.runs)
		if text != c.text || look != c.look {
			t.Errorf("%s: status = (%q, %q), want (%q, %q)", c.name, text, look, c.text, c.look)
		}
	}
}

// TestASharedStageIsCaptionedWithItsBeltsAndAnOwnStageWithItsKind
func TestASharedStageIsCaptionedWithItsBeltsAndAnOwnStageWithItsKind(t *testing.T) {
	t.Parallel()
	stage := DefaultsStages(TriggerKindIssues, DefaultsBuiltInAgents(), true)[0]
	if got := OverviewBoxesCaption(stage, 1); got != "Agent" {
		t.Errorf("caption(1) = %q", got)
	}
	if got := OverviewBoxesCaption(stage, 3); got != "Agent · 3 belts" {
		t.Errorf("caption(3) = %q", got)
	}
	gate := Stage{ID: "approve", Type: StageTypeApprove}
	if got := OverviewBoxesCaption(gate, 1); got != "Approval" {
		t.Errorf("gate caption = %q", got)
	}
	watch := Stage{ID: "watch", Type: StageTypeWatch}
	if got := OverviewBoxesCaption(watch, 1); got != "Watch" {
		t.Errorf("watch caption = %q", got)
	}
}

// TestTheBoxMetricsKeepTheTrackAtTheBoxCentreAndTheColumnPitch
func TestTheBoxMetricsKeepTheTrackAtTheBoxCentreAndTheColumnPitch(t *testing.T) {
	t.Parallel()
	metrics := OverviewBoxesMetrics()
	if metrics.Track != OverviewBoxesStatusHeight+OverviewBoxesBoxHeight/2 {
		t.Errorf("track = %v", metrics.Track)
	}
	if metrics.MinColumn != 216 {
		t.Errorf("minColumn = %v", metrics.MinColumn)
	}
	if metrics.Gap != 22 {
		t.Errorf("gap = %v", metrics.Gap)
	}
	if metrics.Box != 148 {
		t.Errorf("box = %v, want 148", metrics.Box)
	}
}

// obBelt is the default belt on acme/<id>, named by id.
func obBelt(id string) Belt {
	source := DefaultsBelt([]string{"acme/" + id}, "agent", DefaultsBuiltInAgents(), true)
	source.ID = id
	source.Name = floorTestCapitalized(id)
	return source
}

// obOverviewBelt is obBelt as an overview belt, not archived or paused.
func obOverviewBelt(id string) OverviewBelt {
	return OverviewBeltFromBelt(obBelt(id), false, false)
}

// obRun is a run of belt beltID at its first stage, in state.
func obRun(name, beltID string, state RunState) Run {
	source := obBelt(beltID)
	run := NewRun(RunArgs{Belt: source, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/" + beltID, Number: 1, Title: name}, Now: ovNow})
	run.ID = name
	run.BeltID = beltID
	run.BeltName = source.Name
	run.StageID = source.Stages[0].ID
	run.State = state
	return run
}

// obView is the Overview of belts and runs.
func obView(belts []OverviewBelt, runs []Run) OverviewView {
	return NewOverviewView(OverviewViewArgs{Belts: belts, Runs: runs, Now: ovNow})
}

// TestTheFloorSendsBoxesWithTheirTags
func TestTheFloorSendsBoxesWithTheirTags(t *testing.T) {
	t.Parallel()
	belts := []OverviewBelt{obOverviewBelt("issues"), obOverviewBelt("docs")}
	var runs []Run
	for index := range 5 {
		runs = append(runs, obRun("chip-"+strconv.Itoa(index), "issues", RunStateRunning))
	}
	view := obView(belts, runs)
	var box, shared *OverviewFloorBox
	for index := range view.Floor.Boxes {
		if view.Floor.Boxes[index].RunCount == 5 && box == nil {
			box = &view.Floor.Boxes[index]
		}
		if len(view.Floor.Boxes[index].Belts) > 1 && shared == nil {
			shared = &view.Floor.Boxes[index]
		}
	}
	if box == nil {
		t.Fatal("no box holds the five runs")
	}
	if len(box.Chips) != OverviewBoxesTagLimit {
		t.Errorf("chips = %d, want %d", len(box.Chips), OverviewBoxesTagLimit)
	}
	if box.More != 5-OverviewBoxesTagLimit {
		t.Errorf("more = %d, want %d", box.More, 5-OverviewBoxesTagLimit)
	}
	for _, chip := range box.Chips {
		// A box always names each run's belt.
		if chip.Belt == nil || *chip.Belt != "Issues" {
			t.Errorf("chip belt = %v, want Issues", chip.Belt)
		}
	}
	if shared == nil || !strings.HasSuffix(shared.Caption, "2 belts") {
		t.Errorf("a shared stage's caption = %v, want it to end in 2 belts", shared)
	}
}

// TestBoxFramesSitInTheirCellsAndInsideTheLayout
func TestBoxFramesSitInTheirCellsAndInsideTheLayout(t *testing.T) {
	t.Parallel()
	belts := []OverviewBelt{obOverviewBelt("issues"), obOverviewBelt("docs")}
	runs := []Run{obRun("one", "issues", RunStateHeld), obRun("two", "docs", RunStateWaiting)}
	view := obView(belts, runs)
	layout := view.Floor.Layout
	if layout.Metrics.Box != OverviewBoxesBoxWidth {
		t.Errorf("box metric = %v, want %v", layout.Metrics.Box, OverviewBoxesBoxWidth)
	}
	if len(view.Floor.Boxes) == 0 {
		t.Fatal("the Floor has no boxes")
	}
	for _, box := range view.Floor.Boxes {
		placed := layout.Box(FactoryTrackStageNode(box.ID))
		if placed == nil {
			t.Fatalf("box %s is not placed", box.ID)
		}
		if box.X != placed.X-OverviewBoxesBoxWidth/2 {
			t.Errorf("box %s x = %v, want %v", box.ID, box.X, placed.X-OverviewBoxesBoxWidth/2)
		}
		if box.Y != placed.Top {
			t.Errorf("box %s y = %v, want %v", box.ID, box.Y, placed.Top)
		}
		if box.Width != OverviewBoxesBoxWidth {
			t.Errorf("box %s width = %v, want %v", box.ID, box.Width, OverviewBoxesBoxWidth)
		}
		if box.Height != placed.Height {
			t.Errorf("box %s height = %v, want %v", box.ID, box.Height, placed.Height)
		}
		// The box's centre is the track's anchor: the status sits above it, then the box.
		if placed.Anchor != placed.Top+OverviewBoxesStatusHeight+OverviewBoxesBoxHeight/2 {
			t.Errorf("box %s anchor = %v, want the box's centre", box.ID, placed.Anchor)
		}
		if box.X+box.Width > layout.Width || box.Y+box.Height > layout.Height {
			t.Errorf("box %s lies outside the layout", box.ID)
		}
	}
	// Boxes in one column never overlap.
	columns := map[float64][]OverviewFloorBox{}
	for _, box := range view.Floor.Boxes {
		columns[box.X] = append(columns[box.X], box)
	}
	for _, column := range columns {
		sort.SliceStable(column, func(i, j int) bool { return column[i].Y < column[j].Y })
		for index := 1; index < len(column); index++ {
			above, below := column[index-1], column[index]
			if above.Y+above.Height > below.Y {
				t.Errorf("box %s overlaps box %s in its column", above.ID, below.ID)
			}
		}
	}
}

// TestAReturnLaneLabelClearsTheBoxesAboveItAndTheLanesBesideIt
func TestAReturnLaneLabelClearsTheBoxesAboveItAndTheLanesBesideIt(t *testing.T) {
	t.Parallel()
	// Belts that reach a shared stage in another order return to it, which is where the lanes come from.
	specs := []struct {
		id    string
		names []string
	}{
		{"a", []string{"Build", "Test", "Verify"}},
		{"b", []string{"Verify", "Build"}},
		{"c", []string{"Verify", "Test"}},
	}
	var tracked []FactoryTrackBelt
	for _, spec := range specs {
		base := DefaultsStages(TriggerKindIssues, DefaultsBuiltInAgents(), true)
		stages := make([]Stage, 0, len(spec.names))
		for index, name := range spec.names {
			stage := base[index]
			stage.ID = spec.id + "-" + strconv.Itoa(index)
			stage.Name = name
			stage.Route = nil
			stages = append(stages, stage)
		}
		belt := NewBeltFromTrigger(BeltFromTriggerArgs{ID: spec.id, Name: spec.id, Trigger: Trigger{Kind: TriggerKindManual}, Stages: stages})
		tracked = append(tracked, FactoryTrackBelt{ID: belt.ID, Name: belt.Name, Layout: belt.Layout()})
	}
	track := NewFactoryTrack(tracked)
	metrics := OverviewBoxesMetrics()
	layout := NewFloorLayout(track, metrics, nil, func(FactoryTrackStage) float64 { return OverviewBoxesHeight(0) })
	// A lane label is a 16 point pill centred on its lane, so it needs 8 points of air above and below.
	const labelHalf = 8.0
	if len(layout.Lanes) == 0 {
		t.Fatal("the fixture has return lanes")
	}
	for _, lane := range layout.Lanes {
		for _, node := range layout.Placed {
			if node.X < lane.Left || node.X > lane.Right || !isStage(node.Node) || node.Top+node.Height > lane.LabelY {
				continue
			}
			if lane.LabelY-labelHalf < node.Top+node.Height+labelHalf {
				t.Errorf("a lane label at %v touches the box above it (bottom %v)", lane.LabelY, node.Top+node.Height)
			}
		}
	}
	for index, lane := range layout.Lanes {
		for _, other := range layout.Lanes[index+1:] {
			if lane.Left < other.Right && other.Left < lane.Right {
				if math.Abs(lane.LabelY-other.LabelY) < metrics.Lane-floorLayoutLaneTolerance {
					t.Errorf("overlapping lanes at %v and %v keep less than the lane spacing %v", lane.LabelY, other.LabelY, metrics.Lane)
				}
			}
		}
	}
}
