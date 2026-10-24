package core

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
)

// factoryTrackTestBelt is a belt of the issue line's stage kinds, named by names, with the
// id-indexed stage ids and no routes.
func factoryTrackTestBelt(id string, names []string) Belt {
	defaults := DefaultsStages(TriggerKindIssues, nil, true)
	stages := make([]Stage, 0, len(names))
	for index, name := range names {
		stage := defaults[index%len(defaults)]
		stage.ID = id + "-" + strconv.Itoa(index)
		stage.Name = name
		stage.Route = nil
		stages = append(stages, stage)
	}
	return DefaultBelt(id, id, []TriggerEntry{{Trigger: TriggerKindManual.Fresh()}}, stages)
}

// itoaTest is the decimal text of n, for the test ids above.
// factoryTrackTestFloor is the floor of belts, built as a track of them.
func factoryTrackTestFloor(belts ...Belt) FactoryTrack {
	inputs := make([]FactoryTrackBelt, 0, len(belts))
	for _, belt := range belts {
		inputs = append(inputs, FactoryTrackBelt{ID: belt.ID, Name: belt.Name, Layout: belt.Layout()})
	}
	return NewFactoryTrack(inputs)
}

// factoryTrackTestLayer is the layer of node, which must be placed.
func factoryTrackTestLayer(t *testing.T, f FactoryTrack, node FactoryTrackNode) int {
	t.Helper()
	layer := f.LayerOf(node)
	if layer == nil {
		t.Fatalf("node %+v is not placed", node)
	}
	return *layer
}

// factoryTrackTestStageOf is the merged stage a belt's stage is part of, failing when it is none.
func factoryTrackTestStageOf(t *testing.T, f FactoryTrack, stage, belt string) *FactoryTrackStage {
	t.Helper()
	merged := f.StageOf(stage, belt)
	if merged == nil {
		t.Fatalf("stage %s of %s is on no merged stage", stage, belt)
	}
	return merged
}

// TestBeltsEnterOnTheLeftAndMeetAtSharedStages
func TestBeltsEnterOnTheLeftAndMeetAtSharedStages(t *testing.T) {
	t.Parallel()
	f := factoryTrackTestFloor(factoryTrackTestBelt("a", []string{"Triage", "Build", "Ship"}), factoryTrackTestBelt("b", []string{"Build", "Ship"}))
	counts := []int{}
	for _, layer := range f.Layers {
		counts = append(counts, len(layer))
	}
	if !reflect.DeepEqual(counts, []int{2, 2, 1, 1}) {
		t.Fatalf("layer counts = %v", counts)
	}
	if !reflect.DeepEqual(f.Layers[0], []FactoryTrackNode{FactoryTrackEntrance("a"), FactoryTrackEntrance("b")}) {
		t.Fatalf("layer 0 = %+v", f.Layers[0])
	}
	if got := factoryTrackTestLayer(t, f, FactoryTrackStageNode("build")); got != 2 {
		t.Fatalf("build layer = %d, want 2", got)
	}
	// b's way in skips Triage's layer through a waypoint.
	if !factoryTrackHasTrack(f, FactoryTrackEntrance("b"), FactoryTrackWaypoint(0)) {
		t.Fatal("no track from b to waypoint 0")
	}
	var shared *FactoryTrackTrack
	for i := range f.Tracks {
		if f.Tracks[i].From == FactoryTrackStageNode("build") && f.Tracks[i].To == FactoryTrackStageNode("ship") {
			shared = &f.Tracks[i]
		}
	}
	if shared == nil || !reflect.DeepEqual(shared.Belts, []string{"a", "b"}) {
		t.Fatalf("shared track = %+v", shared)
	}
	if len(f.Returns) != 0 {
		t.Fatalf("returns = %+v, want none", f.Returns)
	}
}

// factoryTrackHasTrack reports whether the floor has a track from one node to another.
func factoryTrackHasTrack(f FactoryTrack, from, to FactoryTrackNode) bool {
	return slices.ContainsFunc(f.Tracks, func(track FactoryTrackTrack) bool {
		return track.From == from && track.To == to
	})
}

// TestBeltsInTheOtherOrderLoopBack
func TestBeltsInTheOtherOrderLoopBack(t *testing.T) {
	t.Parallel()
	f := factoryTrackTestFloor(factoryTrackTestBelt("a", []string{"Build", "Verify"}), factoryTrackTestBelt("b", []string{"Verify", "Build"}))
	if factoryTrackTestLayer(t, f, FactoryTrackStageNode("verify")) <= factoryTrackTestLayer(t, f, FactoryTrackStageNode("build")) {
		t.Fatal("verify is not after build")
	}
	if len(f.Returns) != 1 {
		t.Fatalf("returns = %d, want 1", len(f.Returns))
	}
	if f.Returns[0].From != FactoryTrackStageNode("verify") || f.Returns[0].To != FactoryTrackStageNode("build") {
		t.Fatalf("return = %+v", f.Returns[0])
	}
	if !reflect.DeepEqual(f.Returns[0].Belts, []string{"b"}) {
		t.Fatalf("return belts = %v", f.Returns[0].Belts)
	}
}

// TestABeltThatPassesANameAgainGetsASecondStage
func TestABeltThatPassesANameAgainGetsASecondStage(t *testing.T) {
	t.Parallel()
	f := factoryTrackTestFloor(factoryTrackTestBelt("a", []string{"Triage", "Ship", "Verify", "Ship", "Wrap up"}),
		factoryTrackTestBelt("b", []string{"Ship", "Ship"}))
	if len(f.Returns) != 0 {
		t.Fatalf("returns = %+v, want none", f.Returns)
	}
	if got := factoryTrackTestStageOf(t, f, "a-1", "a").ID; got != "ship" {
		t.Fatalf("a-1 merged stage = %q", got)
	}
	if got := factoryTrackTestStageOf(t, f, "a-3", "a").ID; got != "ship #2" {
		t.Fatalf("a-3 merged stage = %q", got)
	}
	if got := f.Stage("ship #2"); got == nil || got.Name != "Ship" {
		t.Fatalf("ship #2 = %+v", got)
	}
	if factoryTrackTestLayer(t, f, FactoryTrackStageNode("ship #2")) <= factoryTrackTestLayer(t, f, FactoryTrackStageNode("verify")) {
		t.Fatal("ship #2 is not after verify")
	}
	// Another belt's second Ship is the same second merged stage.
	if got := factoryTrackTestStageOf(t, f, "b-1", "b").ID; got != "ship #2" {
		t.Fatalf("b-1 merged stage = %q", got)
	}
}

// TestTheEarlierOfTwoStagesWithANameStaysOffTheSharedStageWhenItWouldLeadBack
func TestTheEarlierOfTwoStagesWithANameStaysOffTheSharedStageWhenItWouldLeadBack(t *testing.T) {
	t.Parallel()
	f := factoryTrackTestFloor(factoryTrackTestBelt("a", []string{"Triage", "Verify", "Ship"}),
		factoryTrackTestBelt("b", []string{"Triage", "Ship", "Verify", "Ship"}))
	if len(f.Returns) != 0 {
		t.Fatalf("returns = %+v, want none", f.Returns)
	}
	if got := factoryTrackTestStageOf(t, f, "b-1", "b").ID; got != "ship #2" {
		t.Fatalf("b-1 merged stage = %q", got)
	}
	if got := factoryTrackTestStageOf(t, f, "b-3", "b").ID; got != "ship" {
		t.Fatalf("b-3 merged stage = %q", got)
	}
	if factoryTrackTestLayer(t, f, FactoryTrackStageNode("ship #2")) >= factoryTrackTestLayer(t, f, FactoryTrackStageNode("verify")) {
		t.Fatal("ship #2 is not before verify")
	}
}

// TestABeltInAnotherOrderStillSharesTheStageAndGoesBack
func TestABeltInAnotherOrderStillSharesTheStageAndGoesBack(t *testing.T) {
	t.Parallel()
	f := factoryTrackTestFloor(factoryTrackTestBelt("a", []string{"Verify", "Ship"}), factoryTrackTestBelt("b", []string{"Ship", "Verify"}))
	if len(f.Stages) != 2 {
		t.Fatalf("merged stages = %d, want 2", len(f.Stages))
	}
	if len(f.Returns) != 1 {
		t.Fatalf("returns = %d, want 1", len(f.Returns))
	}
}

// TestARouteOnlyCountsWhatItPassedItself
func TestARouteOnlyCountsWhatItPassedItself(t *testing.T) {
	t.Parallel()
	belt := factoryTrackTestBelt("a", []string{"Triage", "Build", "Build", "Ship", "Triage"})
	belt.Forks = []Fork{{ID: "f", Routes: []Route{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}}}
	one, two := "one", "two"
	belt.Stages[1].Route = &one
	belt.Stages[2].Route = &two
	f := factoryTrackTestFloor(belt)
	// Alternatives share a merged stage; coming back to Triage after the fork does not.
	if got := factoryTrackTestStageOf(t, f, "a-1", "a").ID; got != "build" {
		t.Fatalf("a-1 merged stage = %q", got)
	}
	if got := factoryTrackTestStageOf(t, f, "a-2", "a").ID; got != "build" {
		t.Fatalf("a-2 merged stage = %q", got)
	}
	if got := factoryTrackTestStageOf(t, f, "a-4", "a").ID; got != "triage #2" {
		t.Fatalf("a-4 merged stage = %q", got)
	}
	if len(f.Returns) != 0 {
		t.Fatalf("returns = %+v, want none", f.Returns)
	}
}

// TestShortNames
func TestShortNames(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		" Security fixes ":         "Security",
		"Issues to pull requests":  "Issues",
		"Pull request reviews":     "Pull request",
		"Docs fixes":               "Docs",
		"Tasks":                    "Tasks",
		"Verylongfactoryname belt": "Verylongfact",
	}
	for name, want := range cases {
		if got := FactoryTrackShortName(name); got != want {
			t.Errorf("FactoryTrackShortName(%q) = %q, want %q", name, got, want)
		}
	}
	if got := len([]rune(FactoryTrackShortName("Verylongfactoryname belt"))); got != 12 {
		t.Fatalf("long short name has %d characters, want 12", got)
	}
}
