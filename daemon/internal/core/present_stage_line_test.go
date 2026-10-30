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
