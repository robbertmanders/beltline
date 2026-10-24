//

package core

import (
	"reflect"
	"strings"
	"testing"
)

// TestTriggerKindsReadAsTheirSubtitles checks that trigger kinds read as their subtitles.
func TestTriggerKindsReadAsTheirSubtitles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind *TriggerKind
		want string
	}{
		{new(TriggerKindIssues), "github issue"},
		{nil, "github issue"},
		{new(TriggerKindJira), "jira ticket"},
		{new(TriggerKindSchedule), "schedule"},
		{new(TriggerKindManual), "task"},
		{new(TriggerKindReviews), "pull request"},
	}
	for _, c := range cases {
		if got := RunBoxesTriggerSubtitle(c.kind); got != c.want {
			t.Errorf("subtitle = %q, want %q", got, c.want)
		}
	}
}

// rbBoxes is the run's boxes as the Swift tests build them: the run's presentation at 1080 points, then
// RunBoxes.project over its stage panels.
func rbBoxes(run Run) RunBoxes {
	view := RunPresentationProject(run, false, PauseLive, engineT0, nil, nil)
	return RunBoxesProject(run, view.Stages, nil, boxTestLook)
}

// rbNode is the node with id, failing the test when there is none.
func rbNode(t *testing.T, boxes RunBoxes, id string) RunBoxesNode {
	t.Helper()
	node := boxes.Node(id)
	if node == nil {
		t.Fatalf("no node %q", id)
	}
	return *node
}

// rbChip is the chip with text and look, for comparing a node's chip.
func rbChip(text, look string) *RunBoxesChip {
	return &RunBoxesChip{Text: text, Look: look}
}

// rbEdge is the first edge that matches, or nil.
func rbEdge(boxes RunBoxes, match func(edge RunBoxesEdge) bool) *RunBoxesEdge {
	for index := range boxes.Edges {
		if match(boxes.Edges[index]) {
			return &boxes.Edges[index]
		}
	}
	return nil
}

// TestAStartedRunShowsItsTriggerAndTheStageAtWork checks that a started run shows its trigger and the stage at work.
func TestAStartedRunShowsItsTriggerAndTheStageAtWork(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	EngineAdvance(&run, engineSessionStart("f"), engineT0)
	boxes := rbBoxes(run)
	trigger := rbNode(t, boxes, "trigger")
	if trigger.State != RunBoxesStateIdle {
		t.Errorf("trigger state = %v, want idle", trigger.State)
	}
	if trigger.Title != run.Subject.Ref() {
		t.Errorf("trigger title = %q, want %q", trigger.Title, run.Subject.Ref())
	}
	if trigger.Caption != "Trigger" {
		t.Errorf("trigger caption = %q, want Trigger", trigger.Caption)
	}
	if !reflect.DeepEqual(trigger.Details, []string{"github issue"}) {
		t.Errorf("trigger details = %v, want [github issue]", trigger.Details)
	}
	if trigger.Chip != nil {
		t.Errorf("trigger chip = %+v, want none", trigger.Chip)
	}

	first := rbNode(t, boxes, run.StageID)
	if first.State != RunBoxesStateRunning {
		t.Errorf("first state = %v, want running", first.State)
	}
	if !reflect.DeepEqual(first.Chip, rbChip("Running", "running")) {
		t.Errorf("first chip = %+v, want Running/running", first.Chip)
	}
	if !strings.HasSuffix(first.Caption, " · Agent") {
		t.Errorf("first caption = %q, want it to end in · Agent", first.Caption)
	}
	if len(first.Details) != 1 {
		t.Errorf("first details = %v, want one", first.Details)
	}

	next := run.Stages[1]
	nextNode := rbNode(t, boxes, next.ID)
	if nextNode.State != RunBoxesStateTodo {
		t.Errorf("next state = %v, want todo", nextNode.State)
	}
	if !reflect.DeepEqual(nextNode.Chip, rbChip("Not started", "plain")) {
		t.Errorf("next chip = %+v, want Not started/plain", nextNode.Chip)
	}
	// The trigger's track is travelled, and so is the one to the stage at work; the next one is not yet.
	if edge := rbEdge(boxes, func(e RunBoxesEdge) bool { return e.From == "trigger" }); edge == nil || edge.State != RunBoxesTrackDone {
		t.Errorf("the trigger's track = %+v, want done", edge)
	}
	if edge := rbEdge(boxes, func(e RunBoxesEdge) bool { return e.From == run.StageID && e.To == next.ID }); edge == nil || edge.State != RunBoxesTrackTodo {
		t.Errorf("the track to the next stage = %+v, want todo", edge)
	}
	stageName := ""
	if stage := run.Stage(); stage != nil {
		stageName = stage.Name
	}
	if boxes.Caption != "Now at "+stageName {
		t.Errorf("caption = %q, want %q", boxes.Caption, "Now at "+stageName)
	}
}

// TestAQueuedRunHasItsFirstStageQueuedNotRunning checks that a queued run has its first stage queued not running.
func TestAQueuedRunHasItsFirstStageQueuedNotRunning(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	first := rbNode(t, rbBoxes(run), run.StageID)
	if first.State != RunBoxesStateTodo {
		t.Errorf("state = %v, want todo", first.State)
	}
	if !reflect.DeepEqual(first.Chip, rbChip("Queued", "plain")) {
		t.Errorf("chip = %+v, want Queued/plain", first.Chip)
	}
}

// TestBoxesCoverTheMapsNodesOneForOne checks that boxes cover the maps nodes one for one.
func TestBoxesCoverTheMapsNodesOneForOne(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	boxes := rbBoxes(run)
	ids := map[string]bool{}
	for _, node := range boxes.Nodes {
		ids[node.ID] = true
	}
	mapIDs := map[string]bool{}
	for _, node := range boxes.Map.Nodes {
		mapIDs[node.ID] = true
	}
	if !reflect.DeepEqual(ids, mapIDs) {
		t.Errorf("node ids %v differ from the map's %v", ids, mapIDs)
	}
	if len(boxes.Nodes) != len(boxes.Map.Nodes) {
		t.Errorf("nodes = %d, map nodes = %d", len(boxes.Nodes), len(boxes.Map.Nodes))
	}
	if len(boxes.Edges) != len(boxes.Map.Edges) {
		t.Errorf("edges = %d, map edges = %d", len(boxes.Edges), len(boxes.Map.Edges))
	}
}

// TestAWaitingGateAsksForYou checks that a waiting gate asks for you.
func TestAWaitingGateAsksForYou(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	run.StageID = "approve"
	run.State = RunStateWaiting
	approve := "approve"
	run.Legs[0].StageID = &approve
	run.Legs[0].State = RunStateWaiting
	gate := rbNode(t, rbBoxes(run), "approve")
	if gate.State != RunBoxesStateNeeds {
		t.Errorf("state = %v, want needs", gate.State)
	}
	if gate.Dot == nil || *gate.Dot != RunBoxesDotNeeds {
		t.Errorf("dot = %v, want needs", gate.Dot)
	}
	if !reflect.DeepEqual(gate.Chip, rbChip("Waiting for you", "waiting")) {
		t.Errorf("chip = %+v, want Waiting for you/waiting", gate.Chip)
	}
	if !reflect.DeepEqual(gate.Details, []string{"you"}) {
		t.Errorf("details = %v, want [you]", gate.Details)
	}
	if !strings.HasSuffix(gate.Caption, " · Approval") {
		t.Errorf("caption = %q, want it to end in · Approval", gate.Caption)
	}
}

// TestAHeldRunShowsItsReasonOnTheStageItIsHeldAt checks that a held run shows its reason on the stage it is held at.
func TestAHeldRunShowsItsReasonOnTheStageItIsHeldAt(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	run.StageID = "build"
	run.State = RunStateHeld
	reason := "Tests failed"
	run.Reason = &reason
	build := "build"
	run.Legs[0].StageID = &build
	run.Legs[0].State = RunStateHeld
	held := rbNode(t, rbBoxes(run), "build")
	if held.State != RunBoxesStateHeld {
		t.Errorf("state = %v, want held", held.State)
	}
	if held.Dot == nil || *held.Dot != RunBoxesDotHeld {
		t.Errorf("dot = %v, want held", held.Dot)
	}
	if !reflect.DeepEqual(held.Chip, rbChip("Tests failed", "held")) {
		t.Errorf("chip = %+v, want Tests failed/held", held.Chip)
	}
}

// TestAWatchWaitingOnItsPullRequestIsWatching checks that a watch waiting on its pull request is watching.
func TestAWatchWaitingOnItsPullRequestIsWatching(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	var watch *Stage
	for index := range run.Stages {
		if run.Stages[index].Kind() == StageKindWatch {
			watch = &run.Stages[index]
			break
		}
	}
	if watch == nil {
		t.Fatal("the default belt has a watch stage")
	}
	run.StageID = watch.ID
	run.State = RunStateRunning
	id := watch.ID
	run.Legs[0].StageID = &id
	run.Legs[0].State = RunStateRunning
	box := rbNode(t, rbBoxes(run), watch.ID)
	if box.State != RunBoxesStateWatching {
		t.Errorf("state = %v, want watching", box.State)
	}
	if !reflect.DeepEqual(box.Details, []string{"github"}) {
		t.Errorf("details = %v, want [github]", box.Details)
	}
	if box.Chip == nil || box.Chip.Look != "plain" {
		t.Errorf("chip = %+v, want a plain look", box.Chip)
	}
	if !strings.HasSuffix(box.Caption, " · Watch") {
		t.Errorf("caption = %q, want it to end in · Watch", box.Caption)
	}
}

// TestAShippedRunIsDoneEverywhere checks that a shipped or done run is done everywhere, and is
// captioned with its own ending.
func TestAShippedRunIsDoneEverywhere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		state   RunState
		caption string
	}{{RunStateShipped, "Shipped"}, {RunStateDone, "Done"}} {
		run := engineSampleRun()
		run.State = c.state
		boxes := rbBoxes(run)
		for _, node := range boxes.Nodes {
			if node.ID != "trigger" && node.State != RunBoxesStateDone {
				t.Errorf("%s: node %s is %v, want done", c.state, node.ID, node.State)
			}
		}
		for _, edge := range boxes.Edges {
			if edge.State != RunBoxesTrackDone {
				t.Errorf("%s: edge %s to %s is %v, want done", c.state, edge.From, edge.To, edge.State)
			}
		}
		if boxes.Caption != c.caption {
			t.Errorf("%s: caption = %q, want %s", c.state, boxes.Caption, c.caption)
		}
	}
}

// TestRoutesNotTakenAreSkippedAndDrawnOff checks that routes not taken are skipped and drawn off.
func TestRoutesNotTakenAreSkippedAndDrawnOff(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: forkedBeltWith([]string{"build"}), Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Work"}, Now: engineT0})
	run.SetRoutes(map[string][]string{"size": {"simple"}})
	boxes := rbBoxes(run)
	spec := rbNode(t, boxes, "spec")
	if spec.State != RunBoxesStateSkip {
		t.Errorf("spec state = %v, want skip", spec.State)
	}
	if spec.Chip == nil || spec.Chip.Text != "Not taken" {
		t.Errorf("spec chip = %+v, want Not taken", spec.Chip)
	}
	if rbEdge(boxes, func(e RunBoxesEdge) bool { return e.To == "spec" && e.State == RunBoxesTrackSkip }) == nil {
		t.Error("no track to spec is drawn off")
	}
	if got := rbNode(t, boxes, "quick-build").State; got != RunBoxesStateTodo {
		t.Errorf("quick-build state = %v, want todo", got)
	}
	if !reflect.DeepEqual(boxes.TakenRoutes, []string{"size:simple"}) {
		t.Errorf("taken routes = %v, want [size:simple]", boxes.TakenRoutes)
	}
}

// TestTheTravelledTrackFollowsAnEmptyRouteTheRunTook checks that the travelled track follows an empty route the run took.
func TestTheTravelledTrackFollowsAnEmptyRouteTheRunTook(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: boxTestSplitBelt([]boxTestSplitRoute{{ID: "work", Agents: []string{"build"}}, {ID: "skip"}}),
		Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Work"}, Now: engineT0})
	run.SetRoutes(map[string][]string{"split": {"skip"}})
	run.State = RunStateShipped
	boxes := rbBoxes(run)
	if !reflect.DeepEqual(boxes.TakenRoutes, []string{"split:skip"}) {
		t.Fatalf("taken routes = %v, want [split:skip]", boxes.TakenRoutes)
	}
	lane := rbEdge(boxes, func(e RunBoxesEdge) bool { return e.Route != nil && *e.Route == "skip" })
	if lane == nil {
		t.Fatal("the empty route has a track")
	}
	if lane.From != "triage" || lane.To != "ship" {
		t.Errorf("lane runs %s to %s, want triage to ship", lane.From, lane.To)
	}
	if lane.State != RunBoxesTrackDone {
		t.Errorf("lane state = %v, want done", lane.State)
	}
	// The route not taken is drawn off, and the line runs no further than the fork.
	for _, edge := range boxes.Edges {
		if edge.Route != nil && *edge.Route == "work" && edge.State != RunBoxesTrackSkip {
			t.Errorf("track on work is %v, want skip", edge.State)
		}
	}
	if rbEdge(boxes, func(e RunBoxesEdge) bool { return e.From == "triage" && e.To == "ship" && e.Route == nil }) != nil {
		t.Error("the line runs from triage to ship past the fork")
	}
}

// TestAnEmptyRouteNotTakenIsSkippedAndAUndecidedOneIsNot checks that an empty route not taken is skipped and a undecided one is not.
func TestAnEmptyRouteNotTakenIsSkippedAndAUndecidedOneIsNot(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: boxTestSplitBelt([]boxTestSplitRoute{{ID: "work", Agents: []string{"build"}}, {ID: "skip"}}),
		Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Work"}, Now: engineT0})
	skipLane := func(boxes RunBoxes) RunBoxesEdge {
		edge := rbEdge(boxes, func(e RunBoxesEdge) bool { return e.Route != nil && *e.Route == "skip" })
		if edge == nil {
			t.Fatal("the empty route has a track")
		}
		return *edge
	}
	if got := skipLane(rbBoxes(run)).State; got != RunBoxesTrackTodo {
		t.Errorf("undecided empty route is %v, want todo", got)
	}
	run.SetRoutes(map[string][]string{"split": {"work"}})
	if got := skipLane(rbBoxes(run)).State; got != RunBoxesTrackSkip {
		t.Errorf("empty route not taken is %v, want skip", got)
	}
}

// TestARouteStillUndecidedIsNotDrawnAsTaken checks that a route still undecided is not drawn as taken.
func TestARouteStillUndecidedIsNotDrawnAsTaken(t *testing.T) {
	t.Parallel()
	run := NewRun(RunArgs{Belt: forkedBeltWith([]string{"build"}), Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Work"}, Now: engineT0})
	if taken := rbBoxes(run).TakenRoutes; len(taken) != 0 {
		t.Errorf("taken routes = %v, want none", taken)
	}
}
