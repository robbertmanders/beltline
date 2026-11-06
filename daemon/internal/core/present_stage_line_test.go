package core

import (
	"testing"
)

// TestStageLineNamesAFeedingForkByItsStageAndCaptionsItsRoutes checks the fork box of the ask belt: the feeding
// fork's heading names the stage it feeds into, the line route is the one that is empty, and the feeding
// route holds the stage that feeds in.
func TestStageLineNamesAFeedingForkByItsStageAndCaptionsItsRoutes(t *testing.T) {
	t.Parallel()
	belt := nfAskBelt(t)
	line := StageLineOf(belt, nil)

	var box *StageLinePresentationForkBox
	for _, item := range line.Items {
		if item.Fork != nil {
			box = item.Fork
		}
	}
	if box == nil {
		t.Fatal("no fork box on the line")
	}
	if !box.Feeding || box.Heading != "FEEDS INTO "+UppercasedFull(belt.Stages[2].Name) {
		t.Fatalf("heading = %q, feeding = %v", box.Heading, box.Feeding)
	}
	if len(box.Routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(box.Routes))
	}
	main, asking := box.Routes[0], box.Routes[1]
	if main.Caption != "LINE" || main.Help != "Runs on the line go straight on" || main.Removable {
		t.Errorf("line route = %+v", main)
	}
	if main.Empty == nil || *main.Empty != "The line: runs go straight on" {
		t.Errorf("line route empty = %v", main.Empty)
	}
	if asking.Caption != "FEEDS IN" || asking.Empty != nil || !asking.Removable {
		t.Errorf("feeding route = %+v", asking)
	}
	if len(asking.Items) != 1 || asking.Items[0].Stage == nil || asking.Items[0].Stage.ID != "ask" {
		t.Errorf("feeding route items = %+v", asking.Items)
	}
}

// TestStageLineShowsANewerAgentVersionOnlyWhenTheLibraryHasOne checks the Update offer on a stage's block: the
// first library agent with the stage's id is the latest, and only a newer version than the stage's pin counts.
func TestStageLineShowsANewerAgentVersionOnlyWhenTheLibraryHasOne(t *testing.T) {
	t.Parallel()
	belt := nfAskBelt(t)
	pinned := *belt.Stages[0].Agent
	newer := SampleAgent(pinned.ID)
	newer.Version = pinned.Version + 1

	blockUpdate := func(agents []AgentRecord) *int {
		t.Helper()
		for _, item := range StageLineOf(belt, agents).Items {
			if item.Stage != nil && item.Stage.ID == belt.Stages[0].ID {
				return item.Stage.Update
			}
		}
		t.Fatal("no block for the triage stage")
		return nil
	}

	if update := blockUpdate(nil); update != nil {
		t.Errorf("update without a library = %d", *update)
	}
	if update := blockUpdate([]AgentRecord{{Agent: SampleAgent(pinned.ID)}}); update != nil {
		t.Errorf("update for the same version = %d", *update)
	}
	update := blockUpdate([]AgentRecord{{Agent: newer}, {Agent: SampleAgent(pinned.ID), Archived: true}})
	if update == nil || *update != newer.Version {
		t.Errorf("update = %v, want %d", update, newer.Version)
	}
}

// TestStageLineTitlesAMultiTriggerBeltByItsTriggerCount checks the trigger block of a belt with two triggers.
func TestStageLineTitlesAMultiTriggerBeltByItsTriggerCount(t *testing.T) {
	t.Parallel()
	belt := nfAskBelt(t)
	second := belt.Triggers[0]
	second.ID = "2"
	second.Trigger = Trigger{Kind: TriggerKindReviews}
	belt.Triggers = append(belt.Triggers, second)

	line := StageLineOf(belt, nil)
	if line.Trigger.Kind != "Triggers · 2" || line.Trigger.Title != "All of them" {
		t.Errorf("trigger = %q / %q", line.Trigger.Kind, line.Trigger.Title)
	}
	if line.Trigger.Help == nil || *line.Trigger.Help != "Every trigger feeds this belt" {
		t.Errorf("trigger help = %v", line.Trigger.Help)
	}
}

// TestStageLineSaysWhichRoutesOpenAndJoin checks each route row carries the layout's connections: a route that feeds
// in does not open off the line, a route whose last stage only goes back to the watch ends there, so the editor draws
// it without joining it to the line again, and no route of a fork with nothing after it joins.
func TestStageLineSaysWhichRoutesOpenAndJoin(t *testing.T) {
	t.Parallel()
	ask := StageLineOf(nfAskBelt(t), nil)
	for _, item := range ask.Items {
		if item.Fork == nil {
			continue
		}
		main, asking := item.Fork.Routes[0], item.Fork.Routes[1]
		if !main.Opens || !main.Joins || asking.Opens || !asking.Joins {
			t.Errorf("ask belt routes: main opens %v joins %v, asking opens %v joins %v", main.Opens, main.Joins, asking.Opens, asking.Joins)
		}
	}

	after := Fork{ID: "after", Routes: []Route{{ID: "merged", Name: "Merged"}, {ID: "conflicts", Name: "Conflicts"}}}
	belt := draftTestBelt([]Stage{
		draftTestStage("watch", StageTypeWatch, "", "", draftTestOutcome("merged", Then{Action: ThenActionNext})),
		draftTestStage("wrap", StageTypeAgent, "wrap", "merged", draftTestOutcome("done", Then{Action: ThenActionNext})),
		draftTestStage("resolve", StageTypeAgent, "resolve", "conflicts", draftTestOutcome("fixed", Then{Action: ThenActionGoto, To: []string{"watch"}})),
		draftTestStage("ship", StageTypeAgent, "ship", ""),
	}, after)
	line := StageLineOf(belt, nil)
	var box *StageLinePresentationForkBox
	for _, item := range line.Items {
		if item.Fork != nil {
			box = item.Fork
		}
	}
	if box == nil {
		t.Fatal("no fork box on the line")
	}
	merged, conflicts := box.Routes[0], box.Routes[1]
	if !merged.Opens || !merged.Joins {
		t.Errorf("merged opens %v joins %v, want both", merged.Opens, merged.Joins)
	}
	if !conflicts.Opens || conflicts.Joins {
		t.Errorf("conflicts opens %v joins %v, want it to open and end there", conflicts.Opens, conflicts.Joins)
	}
}

// TestStageLineJoinsNoRouteOfAForkAtTheEnd checks the issue belt's fork after the watch, the last thing on the belt:
// Wrap up finishes the run and the other routes go back to the watch, so none of them is drawn joining the line.
func TestStageLineJoinsNoRouteOfAForkAtTheEnd(t *testing.T) {
	t.Parallel()
	after := Fork{ID: "after", Routes: []Route{{ID: "merged", Name: "Merged"}, {ID: "conflicts", Name: "Conflicts"}}}
	belt := draftTestBelt([]Stage{
		draftTestStage("watch", StageTypeWatch, "", "", draftTestOutcome("merged", Then{Action: ThenActionNext})),
		draftTestStage("wrap", StageTypeAgent, "wrap", "merged", draftTestOutcome("done", Then{Action: ThenActionShip})),
		draftTestStage("resolve", StageTypeAgent, "resolve", "conflicts", draftTestOutcome("fixed", Then{Action: ThenActionGoto, To: []string{"watch"}})),
	}, after)
	for _, item := range StageLineOf(belt, nil).Items {
		if item.Fork == nil {
			continue
		}
		for _, route := range item.Fork.Routes {
			if !route.Opens || route.Joins {
				t.Errorf("%s opens %v joins %v, want it to open and end", route.ID, route.Opens, route.Joins)
			}
		}
	}
}
