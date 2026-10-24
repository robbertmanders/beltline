// Tests of the Work sections, statuses and rows: stage resolution against the Floor,
// the split of main and watching sections, row presentation, the decision stage,
// status grouping order, and repository group headers.

package core

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// wpStageOfKind is the id of the first stage of kind in run, "" when there is none.
func wpStageOfKind(run Run, kind StageKind) string {
	for _, stage := range run.Stages {
		if stage.Kind() == kind {
			return stage.ID
		}
	}
	return ""
}

// wpStageIndexOfKind is the index of the first stage of kind in run.
func wpStageIndexOfKind(run Run, kind StageKind) int {
	return slices.IndexFunc(run.Stages, func(stage Stage) bool { return stage.Kind() == kind })
}

// TestStageResolvesAgainstTheFloorAndUnknownSelectsNothing
func TestStageResolvesAgainstTheFloorAndUnknownSelectsNothing(t *testing.T) {
	t.Parallel()
	belt := DefaultsBelt([]string{"acme/billing"}, "agent", DefaultsBuiltInAgents(), true)
	merged := factoryTrackKey(belt.Stages[0].Name)
	track := NewFactoryTrack([]FactoryTrackBelt{{ID: belt.ID, Name: belt.Name, Layout: belt.Layout()}})
	first := wpMakeRun("first", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	first.StageID = belt.Stages[0].ID
	second := wpMakeRun("second", RunStateQueued, 0, belt.ID, "Issues", "acme/billing", "T", 1, "")
	second.StageID = belt.Stages[0].ID
	runs := []Run{first, second}

	query := wpQuery(WorkFilterAll)
	query.Stage = new(merged)
	if got := wpIDs(WorkPresentationRows(runs, query, &track)); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("selected stage = %v", got)
	}
	query.Stage = new("no-such-stage")
	if got := WorkPresentationRows(runs, query, &track); len(got) != 0 {
		t.Fatalf("unknown stage = %v, want none", wpIDs(got))
	}
	plain := wpQuery(WorkFilterAll)
	if got := WorkPresentationRows(runs, plain, nil); len(got) != 2 {
		t.Fatalf("no track rows = %d, want 2", len(got))
	}
}

// TestSectionsSplitMainAndWatchingAndKeepGroupMembership
func TestSectionsSplitMainAndWatchingAndKeepGroupMembership(t *testing.T) {
	t.Parallel()
	pullWatchRun := wpMakeRun("pull-watch", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	pullWatchRun.StageID = pullWatchRun.Stages[wpStageIndexOfKind(pullWatchRun, StageKindWatch)].ID

	ticketWatchRun := wpMakeRun("ticket-watch", RunStateQueued, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	ticketIndex := wpStageIndexOfKind(ticketWatchRun, StageKindWatch)
	ticket := WatchTargetTicket
	ticketWatchRun.Stages[ticketIndex].Watch = &ticket
	ticketWatchRun.StageID = ticketWatchRun.Stages[ticketIndex].ID

	heldWatch := wpMakeRun("held-watch", RunStateHeld, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	heldWatch.StageID = wpStageOfKind(heldWatch, StageKindWatch)

	namedWatch := wpMakeRun("named-watch", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	agentIndex := wpStageIndexOfKind(namedWatch, StageKindAgent)
	namedWatch.Stages[agentIndex].Name = "Watch"
	namedWatch.StageID = namedWatch.Stages[agentIndex].ID

	parkedWatch := wpMakeRun("parked-watch", RunStateParked, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	parkedWatch.StageID = wpStageOfKind(parkedWatch, StageKindWatch)

	beltPaused := wpMakeRun("belt-paused", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	beltPaused.StageID = wpStageOfKind(beltPaused, StageKindWatch)

	runs := []Run{pullWatchRun, ticketWatchRun, heldWatch, namedWatch, parkedWatch, beltPaused}
	main, watching, _ := WorkPresentationSections(runs, GroupByNone)
	mainIDs := wpIDs(wgRows(main))
	watchingIDs := wpIDs(wgRows(watching))
	if !reflect.DeepEqual(watchingIDs, []string{"belt-paused", "pull-watch", "ticket-watch"}) {
		t.Fatalf("watching = %v", watchingIDs)
	}
	if !reflect.DeepEqual(mainIDs, []string{"held-watch", "named-watch", "parked-watch"}) {
		t.Fatalf("main = %v", mainIDs)
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, mainIDs...), watchingIDs...) {
		if seen[id] {
			t.Fatalf("run %s is in both sections", id)
		}
		seen[id] = true
	}
	if len(seen) != len(runs) {
		t.Fatalf("sections hold %d runs, want %d", len(seen), len(runs))
	}
	// A global or belt pause changes the run's location but never its section.
	location := WorkRowPresentationRow(pullWatchRun, true, Pause{IsPaused: true}, nil, wpBase).Location
	if name := pullWatchRun.Stage().Name; !strings.Contains(location.Text, name) {
		t.Fatalf("location %q does not name stage %q", location.Text, name)
	}
	if !RunSummaryPassivelyWatching(pullWatchRun) {
		t.Fatal("the pull request watch is not passively watching")
	}
}

// TestWorkViewCountsRowsPerSectionAndTogether pins the counts the page shows: the Watching heading
// counts its own rows, and the announced total counts both sections.
func TestWorkViewCountsRowsPerSectionAndTogether(t *testing.T) {
	t.Parallel()
	view := WorkView{
		MainGroups:     []WorkGroupPresentation{{Rows: make([]WorkRowPresentation, 2)}, {Rows: make([]WorkRowPresentation, 1)}},
		WatchingGroups: []WorkGroupPresentation{{Rows: make([]WorkRowPresentation, 3)}},
	}
	if got := view.WatchingRowCount(); got != 3 {
		t.Fatalf("watching rows = %d, want 3", got)
	}
	if got := view.ShownRowCount(); got != 6 {
		t.Fatalf("shown rows = %d, want 6", got)
	}
	if got := (WorkView{}).ShownRowCount(); got != 0 {
		t.Fatalf("empty view shows %d rows", got)
	}
}

// TestRowPresentationCarriesChipsPauseWordsAndProgress
func TestRowPresentationCarriesChipsPauseWordsAndProgress(t *testing.T) {
	t.Parallel()
	held := wpMakeRun("held", RunStateHeld, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	held.StageID = wpStageOfKind(held, StageKindAgent)
	heldRow := WorkRowPresentationRow(held, false, PauseLive, nil, wpBase)
	if heldRow.Chip == nil || *heldRow.Chip != "Answer" {
		t.Fatalf("held chip = %v, want Answer", heldRow.Chip)
	}
	if !heldRow.Held {
		t.Fatal("held row is not held")
	}
	if heldRow.Location.Look != StageStatusLookHeld {
		t.Fatalf("held look = %v", heldRow.Location.Look)
	}

	waiting := wpMakeRun("waiting", RunStateWaiting, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	waiting.StageID = wpStageOfKind(waiting, StageKindAgent)
	if chip := WorkRowPresentationRow(waiting, false, PauseLive, nil, wpBase).Chip; chip == nil || *chip != "Decide" {
		t.Fatalf("waiting chip = %v, want Decide", chip)
	}

	passive := wpMakeRun("passive", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	passive.StageID = wpStageOfKind(passive, StageKindWatch)
	if chip := WorkRowPresentationRow(passive, false, PauseLive, nil, wpBase).Chip; chip != nil {
		t.Fatalf("passive chip = %v, want none", *chip)
	}

	for _, phase := range []RunPausePhase{RunPausePhasePausing, RunPausePhasePaused, RunPausePhaseIncomplete, RunPausePhaseReconcileFailed, RunPausePhaseResuming} {
		paused := wpMakeRun("paused-"+string(phase), RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
		paused.StageID = wpStageOfKind(paused, StageKindAgent)
		paused.Pause = &RunPause{Phase: phase, Operation: "op", RequestedAt: jsonx.Date{Time: wpBase}, Snapshots: []RunPauseSnapshot{}}
		row := WorkRowPresentationRow(paused, false, PauseLive, nil, wpBase)
		if !row.PausedByYou {
			t.Fatalf("%s does not block progress", phase)
		}
		if row.PauseWord == nil {
			t.Fatalf("%s has no location word", phase)
		}
	}

	queued := wpMakeRun("queued-continuation", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	queued.StageID = wpStageOfKind(queued, StageKindAgent)
	attempt := NewAttempt(AttemptArgs{StageID: queued.StageID, Number: 1, StartedAt: jsonx.Date{Time: wpBase}, Folder: "attempt"})
	continuation := NewAttemptInterruptionContinuation("op", AttemptInterruptionContinuationStatusQueued, nil, nil)
	attempt.Interruption = &AttemptInterruption{Operation: "op", LegID: "main", Confirmation: AttemptInterruptionConfirmationConfirmed, Continuation: &continuation}
	queued.Attempts = []Attempt{attempt}
	queuedRow := WorkRowPresentationRow(queued, false, PauseLive, nil, wpBase)
	if queuedRow.PausedByYou {
		t.Fatal("a queued continuation is paused by you")
	}
	if queuedRow.PauseWord == nil || *queuedRow.PauseWord != "queued to continue" {
		t.Fatalf("queued continuation word = %v", queuedRow.PauseWord)
	}

	// A belt pause turns a queued run's location to paused without a run pause.
	beltPaused := wpMakeRun("belt", RunStateQueued, 0, "issues", "Issues", "acme/billing", "T", 1, "")
	beltPaused.StageID = wpStageOfKind(beltPaused, StageKindAgent)
	if text := WorkRowPresentationRow(beltPaused, true, PauseLive, nil, wpBase).Location.Text; !strings.Contains(text, "paused") {
		t.Fatalf("belt-paused location = %q", text)
	}
	if text := WorkRowPresentationRow(beltPaused, false, Pause{IsPaused: true}, nil, wpBase).Location.Text; !strings.Contains(text, "paused") {
		t.Fatalf("pause-paused location = %q", text)
	}
	if progress := WorkRowPresentationRow(held, false, PauseLive, nil, wpBase).Progress; progress < 0 || progress > 1 {
		t.Fatalf("progress = %v, want 0...1", progress)
	}
}

// TestRowsOpenTheDecisionStageOnlyWhenTheyNeedYou
func TestRowsOpenTheDecisionStageOnlyWhenTheyNeedYou(t *testing.T) {
	t.Parallel()
	held := wpRun("held", RunStateHeld)
	running := wpRun("running", RunStateRunning)
	if got := WorkRowPresentationOpenStage(held); got == nil || *got != held.StageID {
		t.Fatalf("held opens %v, want %s", got, held.StageID)
	}
	if got := WorkRowPresentationOpenStage(running); got != nil {
		t.Fatalf("running opens %v, want nothing", *got)
	}
	if ref := WorkRowPresentationRow(held, false, PauseLive, nil, wpBase).Reference; ref != "#1" {
		t.Fatalf("held reference = %q", ref)
	}
	jira := wpMakeRun("jira", RunStateRunning, 0, "issues", "Issues", "acme/billing", "T", 0, "KAN-70")
	if ref := WorkRowPresentationRow(jira, false, PauseLive, nil, wpBase).Reference; ref != "KAN-70" {
		t.Fatalf("jira reference = %q", ref)
	}
}

// TestStatusGroupingOrdersNeedsYouRunningPausedAndTheRest
func TestStatusGroupingOrdersNeedsYouRunningPausedAndTheRest(t *testing.T) {
	t.Parallel()
	paused := wpRun("paused", RunStateRunning)
	paused.Pause = &RunPause{Phase: RunPausePhasePaused, Operation: "op", RequestedAt: jsonx.Date{Time: wpBase}, Snapshots: []RunPauseSnapshot{}}
	runs := []Run{
		paused, wpRun("shipped", RunStateShipped), wpRun("waiting", RunStateWaiting),
		wpRun("dropped", RunStateDropped), wpRun("queued", RunStateQueued), wpRun("parked", RunStateParked),
		wpRun("running", RunStateRunning),
	}
	groups := WorkGroupGroupRuns(runs, GroupByStatus)
	if got := wgValues(groups); !reflect.DeepEqual(got, []string{"Needs you", "Running", "Paused", "Queued", "Parked", "Dropped", "Shipped"}) {
		t.Fatalf("status order = %v", got)
	}
	for _, group := range groups {
		if group.Key.Value == "Paused" {
			if got := wpIDs(group.Runs); !reflect.DeepEqual(got, []string{"paused"}) {
				t.Fatalf("paused group = %v", got)
			}
		}
	}
}

// TestRepositoryGroupsCarryTheDisambiguatedHeaderLabel
func TestRepositoryGroupsCarryTheDisambiguatedHeaderLabel(t *testing.T) {
	t.Parallel()
	web := wpMakeRun("web", RunStateRunning, 0, "issues", "Issues", "acme/web", "T", 1, "")
	other := wpMakeRun("other", RunStateRunning, 0, "issues", "Issues", "other/web", "T", 1, "")
	ambiguous := RepoLabelAmbiguousNames([]Run{web, other})
	var labels []string
	for _, group := range WorkGroupGroupRuns([]Run{web, other}, GroupByRepo) {
		labels = append(labels, WorkGroupPresentationOf(group, nil, PauseLive, ambiguous, time.Now()).Label)
	}
	if !reflect.DeepEqual(labels, []string{"web [acme]", "web [other]"}) {
		t.Fatalf("repository labels = %v", labels)
	}
	var plain []string
	for _, group := range WorkGroupGroupRuns([]Run{web}, GroupByStatus) {
		plain = append(plain, WorkGroupPresentationOf(group, nil, PauseLive, map[string]struct{}{}, time.Now()).Label)
	}
	if !reflect.DeepEqual(plain, []string{"Running"}) {
		t.Fatalf("status labels = %v", plain)
	}
}
