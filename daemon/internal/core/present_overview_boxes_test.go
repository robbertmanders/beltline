// Tests of the boxes Overview: the pure tests, and the tests that build an
// OverviewView (the belts, runs and layouts of the Floor).

package core

import (
	"strconv"
	"strings"
	"testing"
)

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
	stage := SampleStages(TriggerKindIssues, SampleAgents(), true)[0]
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

// floorTestCapitalized capitalizes the one-word ids of these tests.
func floorTestCapitalized(id string) string {
	if id == "" {
		return id
	}
	return strings.ToUpper(id[:1]) + strings.ToLower(id[1:])
}

// obBelt is the default belt on acme/<id>, named by id.
func obBelt(id string) Belt {
	source := SampleBelt([]string{"acme/" + id}, "agent", SampleAgents(), true)
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

// TestEveryBoxHasItsNodeOnTheFloorsTrack checks that the browser finds every box's node in the track's layers,
// each once, and that every stretch joins nodes in neighbouring layers.
func TestEveryBoxHasItsNodeOnTheFloorsTrack(t *testing.T) {
	t.Parallel()
	belts := []OverviewBelt{obOverviewBelt("issues"), obOverviewBelt("docs")}
	view := obView(belts, []Run{obRun("one", "issues", RunStateHeld)})
	if len(view.Floor.Boxes) == 0 {
		t.Fatal("the Floor has no boxes")
	}
	layerOf := map[FactoryTrackNode]int{}
	for index, layer := range view.Floor.Track.Layers {
		for _, node := range layer {
			if _, again := layerOf[node]; again {
				t.Errorf("node %v is in two layers", node)
			}
			layerOf[node] = index
		}
	}
	for _, box := range view.Floor.Boxes {
		if _, ok := layerOf[FactoryTrackStageNode(box.ID)]; !ok {
			t.Errorf("box %s has no node on the track", box.ID)
		}
	}
	for _, belt := range belts {
		if layerOf[FactoryTrackEntrance(belt.ID)] != 0 {
			t.Errorf("belt %s does not enter in the first layer", belt.ID)
		}
	}
	for _, stretch := range view.Floor.Track.Tracks {
		if layerOf[stretch.To] != layerOf[stretch.From]+1 {
			t.Errorf("stretch %v>%v skips a layer", stretch.From, stretch.To)
		}
	}
}

// TestAReturnLaneIsLabelledWithTheBeltsThatTakeIt checks that each return lane names its belts, short and in capitals.
func TestAReturnLaneIsLabelledWithTheBeltsThatTakeIt(t *testing.T) {
	t.Parallel()
	// Belts that reach a shared stage in another order return to it, which is where the lanes come from.
	specs := []struct {
		id    string
		names []string
	}{
		{"alpha", []string{"Build", "Test", "Verify"}},
		{"beta", []string{"Verify", "Build"}},
		{"gamma", []string{"Verify", "Test"}},
	}
	var tracked []FactoryTrackBelt
	for _, spec := range specs {
		base := SampleStages(TriggerKindIssues, SampleAgents(), true)
		stages := make([]Stage, 0, len(spec.names))
		for index, name := range spec.names {
			stage := base[index]
			stage.ID = spec.id + "-" + strconv.Itoa(index)
			stage.Name = name
			stage.Route = nil
			stages = append(stages, stage)
		}
		belt := NewBeltFromTrigger(BeltFromTriggerArgs{ID: spec.id, Name: floorTestCapitalized(spec.id), Trigger: Trigger{Kind: TriggerKindManual}, Stages: stages})
		tracked = append(tracked, FactoryTrackBelt{ID: belt.ID, Name: belt.Name, Layout: belt.Layout()})
	}
	floor := NewFloorTrack(NewFactoryTrack(tracked))
	if len(floor.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(floor.Returns))
	}
	for _, lane := range floor.Returns {
		if len(lane.Belts) == 0 {
			t.Fatal("a lane carries no belts")
		}
		var names []string
		for _, id := range lane.Belts {
			names = append(names, strings.ToUpper(id))
		}
		if want := strings.Join(names, " · "); lane.Label != want {
			t.Errorf("lane label = %q, want %q", lane.Label, want)
		}
	}
}
