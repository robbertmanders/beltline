// Tests of the watch target in belt draft editing.

package core

import "testing"

// draftWatchStage is the watch stage of belt, or nil.
func draftWatchStage(belt Belt) *Stage {
	for index := range belt.Stages {
		if belt.Stages[index].Kind() == StageKindWatch {
			return &belt.Stages[index]
		}
	}
	return nil
}

// TestChangingAWatchTargetResetsItsDefaultOutcomesAndDefaultName
func TestChangingAWatchTargetResetsItsDefaultOutcomesAndDefaultName(t *testing.T) {
	t.Parallel()
	base := SampleBelt([]string{"acme/app"}, "agent", SampleAgents(), true)
	watch := draftWatchStage(base)
	if watch == nil {
		t.Fatal("no watch stage")
	}
	watchID := watch.ID
	for index := range base.Stages {
		if base.Stages[index].ID == watchID {
			base.Stages[index].Name = WatchTargetPullRequest.Title()
		}
	}
	target := WatchTargetTicket
	update := StageEdit{Watch: &target}
	base = draftRequestApply(t, BeltDraftRequest{Draft: base, Op: BeltDraftOperationStageUpdate, StageID: new(watchID), Update: &update})
	changed := draftStage(base, watchID)
	if changed == nil {
		t.Fatal("stage lost")
	}
	if got := changed.WatchTarget(); got == nil || *got != WatchTargetTicket {
		t.Fatalf("watch target = %v, want ticket", got)
	}
	if changed.Name != WatchTargetTicket.Title() {
		t.Fatalf("name = %q, want %q", changed.Name, WatchTargetTicket.Title())
	}
	if !defaultsSame(changed.Outcomes, DefaultsTicketWatchOutcomes()) {
		t.Fatalf("outcomes = %+v, want the ticket watch's defaults", changed.Outcomes)
	}
}

// TestACustomWatchNameIsKeptWhenTheTargetChanges
func TestACustomWatchNameIsKeptWhenTheTargetChanges(t *testing.T) {
	t.Parallel()
	base := SampleBelt([]string{"acme/app"}, "agent", SampleAgents(), true)
	watch := draftWatchStage(base)
	if watch == nil {
		t.Fatal("no watch stage")
	}
	watchID := watch.ID
	for index := range base.Stages {
		if base.Stages[index].ID == watchID {
			base.Stages[index].Name = "My watch"
		}
	}
	target := WatchTargetTicket
	update := StageEdit{Watch: &target}
	changed := draftRequestApply(t, BeltDraftRequest{Draft: base, Op: BeltDraftOperationStageUpdate, StageID: new(watchID), Update: &update})
	if stage := draftStage(changed, watchID); stage == nil || stage.Name != "My watch" {
		t.Fatalf("name = %v, want My watch", stage)
	}
}
