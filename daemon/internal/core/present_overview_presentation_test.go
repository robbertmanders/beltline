// Tests of the Overview presentation: the empty floor, the empty runs, and the summary.

package core

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ovFloorBelt is an overview belt with the given identity and archive and pause state.
func ovFloorBelt(id string, archived, paused bool) OverviewBelt {
	return OverviewBelt{ID: id, Name: id, Archived: archived, Paused: paused}
}

// ovRun is a run on beltID in state, with the fixture subject.
func ovRun(id, beltID string, state RunState) Run {
	source := DefaultsBelt([]string{"acme/billing"}, "agent", DefaultsBuiltInAgents(), true)
	run := NewRun(RunArgs{Belt: source, Subject: DefaultSubject("acme/billing", id), Now: ovNow})
	run.ID = id
	run.BeltID = beltID
	run.BeltName = source.Name
	run.State = state
	run.UpdatedAt.Time = ovNow
	return run
}

// ovNow is the fixture clock of the Overview tests: 1_791_356_400 seconds after the epoch.
var ovNow = time.Unix(1_791_356_400, 0)

// TestFloorKeepsLiveBeltsAndArchivedBeltsWithActiveRuns
func TestFloorKeepsLiveBeltsAndArchivedBeltsWithActiveRuns(t *testing.T) {
	t.Parallel()
	belts := []OverviewBelt{
		ovFloorBelt("live", false, false),
		ovFloorBelt("archived-active", true, false),
		ovFloorBelt("archived-inactive", true, false),
	}
	runs := []Run{
		ovRun("one", "archived-active", RunStateParked),
		ovRun("two", "archived-inactive", RunStateShipped),
		ovRun("three", "archived-inactive", RunStateDropped),
	}
	floor := OverviewPresentationFloorBelts(belts, runs)
	ids := []string{}
	for _, belt := range floor {
		ids = append(ids, belt.ID)
	}
	if len(ids) != 2 || ids[0] != "live" || ids[1] != "archived-active" {
		t.Fatalf("floor ids = %v, want [live archived-active]", ids)
	}
	if !OverviewPresentationShowsOnFloor(false, "anything", nil) {
		t.Fatal("a live belt is not on the floor")
	}
	if !OverviewPresentationShowsOnFloor(true, "x", []Run{ovRun("x", "x", RunStateRunning)}) {
		t.Fatal("an archived belt with an active run is not on the floor")
	}
	if OverviewPresentationShowsOnFloor(true, "x", []Run{ovRun("x", "x", RunStateShipped)}) {
		t.Fatal("an archived belt with only a shipped run is on the floor")
	}
	// A dropped run is not active; a parked one is, as RunState.isActive says.
	if !OverviewPresentationShowsOnFloor(true, "y", []Run{ovRun("y", "y", RunStateParked)}) {
		t.Fatal("an archived belt with a parked run is not on the floor")
	}
}

// TestSessionShadesFollowTheBusiestBlockAndGridOrder
func TestSessionShadesFollowTheBusiestBlockAndGridOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ value, most, want int }{{0, 8, 0}, {1, 8, 1}, {5, 8, 3}, {8, 8, 4}} {
		if got := OverviewSummaryShade(tc.value, tc.most); got != tc.want {
			t.Fatalf("shade(%d, most: %d) = %d, want %d", tc.value, tc.most, got, tc.want)
		}
	}
	if got := OverviewSummaryBusyCaption(3); got != "3 AGENTS BUSY" {
		t.Fatalf("busy caption = %q", got)
	}
	utc := time.UTC
	nineAM := time.Date(2026, 10, 9, 9, 0, 0, 0, utc)
	sixPM := time.Date(2026, 10, 9, 18, 0, 0, 0, utc)
	if got := OverviewSummaryGreeting(nineAM, utc); got != "Good morning." {
		t.Fatalf("9 AM greeting = %q", got)
	}
	if got := OverviewSummaryGreeting(sixPM, utc); got != "Good evening." {
		t.Fatalf("6 PM greeting = %q", got)
	}
}

// TestShippedChartSendsItsCurveAndWeekLabels
func TestShippedChartSendsItsCurveAndWeekLabels(t *testing.T) {
	t.Parallel()
	weeks := make([]int, 12)
	weeks[11] = 7
	chart := OverviewSummaryChartFor(weeks, ovNow)
	if !strings.HasPrefix(chart.Line, "M10 120 C") {
		t.Fatalf("line = %q", chart.Line)
	}
	if !strings.HasSuffix(chart.Area, "L290 120 L10 120 Z") {
		t.Fatalf("area = %q", chart.Area)
	}
	if chart.DotX != 290 {
		t.Fatalf("dotX = %v, want 290", chart.DotX)
	}
	// The last week is the peak, so its point sits at the top of the curve's range: 120 - 100 * 7 / (7 * 8 / 7).
	if chart.DotY != 32.5 {
		t.Fatalf("dotY = %v, want 32.5", chart.DotY)
	}
	if chart.LatestLabel != "[ 7 ]" {
		t.Fatalf("latest label = %q", chart.LatestLabel)
	}
	if !strings.HasPrefix(chart.StartLabel, "WK ") {
		t.Fatalf("start label = %q", chart.StartLabel)
	}
}

// ovRunAt is a run on beltID in state, last updated at updated.
func ovRunAt(id, beltID string, state RunState, updated time.Time) Run {
	run := ovRun(id, beltID, state)
	run.UpdatedAt = jsonx.Date{Time: updated}
	return run
}

// ovAttempt is an ended attempt of stage at date, a minute long, with a session unless session is false.
func ovAttempt(stage string, at time.Time, session bool, usage *Usage) Attempt {
	attempt := NewAttempt(AttemptArgs{StageID: stage, Number: 1, StartedAt: jsonx.Date{Time: at}, Folder: stage})
	end := jsonx.Date{Time: at.Add(60 * time.Second)}
	attempt.EndedAt = &end
	if session {
		name := "bl-" + stage
		attempt.Session = &name
	}
	attempt.Usage = usage
	return attempt
}

// ovUTC is the UTC calendar the Overview tests use.
var ovUTC = time.UTC

// TestSummarySpendUsesTheSharedFiguresAndCopy
func TestSummarySpendUsesTheSharedFiguresAndCopy(t *testing.T) {
	t.Parallel()
	priced := ovRun("priced", "issues", RunStateShipped)
	nanos := 340_000_000
	spent := NewUsage(UsageArgs{CostNanos: &nanos})
	priced.Attempts = []Attempt{ovAttempt("build", ovNow, true, &spent)}
	unpriced := ovRun("unpriced", "issues", RunStateRunning)
	unpriced.Attempts = []Attempt{ovAttempt("spec", ovNow, true, nil)}
	summary := NewOverviewSummary([]Run{priced, unpriced}, ovNow, ovUTC, nil, PauseLive)
	spend := CostToday([]Run{priced, unpriced}, ovUTC, ovNow)
	if summary.Spend.Amount != CostAmount(spend) {
		t.Fatalf("amount = %q, want %q", summary.Spend.Amount, CostAmount(spend))
	}
	if summary.Spend.Amount != "$0.34" {
		t.Fatalf("amount = %q, want $0.34", summary.Spend.Amount)
	}
	if summary.Spend.Caption != CostTodayCaptionFor(spend) {
		t.Fatalf("caption = %q, want %q", summary.Spend.Caption, CostTodayCaptionFor(spend))
	}
	if !reflect.DeepEqual(summary.Spend.Note, CostRunLine(spend)) {
		t.Fatalf("note = %v, want %v", summary.Spend.Note, CostRunLine(spend))
	}
}

// TestSummarySpendIsZeroWithNoEndedSessions
func TestSummarySpendIsZeroWithNoEndedSessions(t *testing.T) {
	t.Parallel()
	summary := NewOverviewSummary(nil, ovNow, ovUTC, nil, PauseLive)
	if summary.Spend.Amount != "$0.00" {
		t.Fatalf("amount = %q, want $0.00", summary.Spend.Amount)
	}
	if summary.Spend.Note != nil {
		t.Fatalf("note = %q, want nil", *summary.Spend.Note)
	}
}

// TestSessionBucketsCountRecordedSessionsInSixHourBlocksOverFourteenDays
func TestSessionBucketsCountRecordedSessionsInSixHourBlocksOverFourteenDays(t *testing.T) {
	t.Parallel()
	start := ovStartOfDayUTC(ovNow).Add(-13 * 86400 * time.Second)
	session := ovRun("sessions", "issues", RunStateRunning)
	session.Attempts = []Attempt{
		ovAttempt("build", start.Add(25*time.Hour), true, nil),
		ovAttempt("spec", ovNow, true, nil),
		ovAttempt("triage", ovNow, false, nil),
	}
	summary := NewOverviewSummary([]Run{session}, ovNow, ovUTC, nil, PauseLive)
	if len(summary.Sessions.Buckets) != 56 {
		t.Fatalf("buckets = %d, want 56", len(summary.Sessions.Buckets))
	}
	if summary.Sessions.Buckets[4] != 1 {
		t.Fatalf("bucket 4 = %d, want 1: 25 hours in falls in the fifth six-hour block", summary.Sessions.Buckets[4])
	}
	sum := 0
	for _, count := range summary.Sessions.Buckets {
		sum += count
	}
	if sum != 2 {
		t.Fatalf("bucket sum = %d, want 2: an attempt without a session is not counted", sum)
	}
	if summary.Sessions.Today != 1 {
		t.Fatalf("today = %d, want 1", summary.Sessions.Today)
	}
	if summary.Sessions.RunningCaption != "RUNS TODAY · 1 RUNNING" {
		t.Fatalf("running caption = %q", summary.Sessions.RunningCaption)
	}
}

// ovStartOfDayUTC is the midnight UTC of t's day.
func ovStartOfDayUTC(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestShippedBucketsAndMedianFollowTheSharedRules
func TestShippedBucketsAndMedianFollowTheSharedRules(t *testing.T) {
	t.Parallel()
	var weekly []Run
	for week := 0; week < 12; week++ {
		updated := ovNow.Add(-time.Duration(float64(week)*7*86400+3600) * time.Second)
		weekly = append(weekly, ovRunAt("shipped-"+strconv.Itoa(week), "issues", RunStateShipped, updated))
	}
	summary := NewOverviewSummary(weekly, ovNow, ovUTC, nil, PauseLive)
	if len(summary.Shipped.Weeks) != 12 {
		t.Fatalf("weeks = %d, want 12", len(summary.Shipped.Weeks))
	}
	total := 0
	for _, count := range summary.Shipped.Weeks {
		total += count
	}
	if total != 12 {
		t.Fatalf("week total = %d, want 12", total)
	}
	if summary.Shipped.Weeks[11] != 1 {
		t.Fatalf("newest week = %d, want 1: the newest week is the last column", summary.Shipped.Weeks[11])
	}
	if summary.Shipped.Latest != 1 {
		t.Fatalf("latest = %d, want 1", summary.Shipped.Latest)
	}
	none := NewOverviewSummary(nil, ovNow, ovUTC, nil, PauseLive)
	if none.Shipped.Median != "—" {
		t.Fatalf("median with none = %q, want —", none.Shipped.Median)
	}
}

// TestRowsKeepTheirOrderAndLimits
func TestRowsKeepTheirOrderAndLimits(t *testing.T) {
	t.Parallel()
	var runs []Run
	for index := 0; index < 8; index++ {
		runs = append(runs, ovRunAt("progress-"+strconv.Itoa(index), "issues", RunStateRunning, ovNow.Add(time.Duration(index)*60*time.Second)))
	}
	waiting := ovRun("decide", "issues", RunStateWaiting)
	held := ovRun("answer", "issues", RunStateHeld)
	runs = append(runs, waiting, held)
	for index := 0; index < 5; index++ {
		runs = append(runs, ovRun("running-"+strconv.Itoa(index), "issues", RunStateRunning))
	}
	summary := NewOverviewSummary(runs, ovNow, ovUTC, nil, PauseLive)
	// Eight running, five running and the one waiting; the held run is a need, not progress.
	if summary.InProgress.Count != 14 {
		t.Fatalf("in progress count = %d, want 14", summary.InProgress.Count)
	}
	if len(summary.InProgress.Rows) != OverviewSummaryInProgressLimit {
		t.Fatalf("in progress rows = %d, want %d", len(summary.InProgress.Rows), OverviewSummaryInProgressLimit)
	}
	if summary.InProgress.Rows[0].RunID != "progress-7" {
		t.Fatalf("first in-progress row = %q, want progress-7: the most recently updated run leads", summary.InProgress.Rows[0].RunID)
	}
	if summary.Running.Count != 13 {
		t.Fatalf("running count = %d, want 13", summary.Running.Count)
	}
	if len(summary.Running.RunIDs) != OverviewSummaryRunningLimit {
		t.Fatalf("running ids = %d, want %d", len(summary.Running.RunIDs), OverviewSummaryRunningLimit)
	}
	if summary.Running.Remaining != 9 {
		t.Fatalf("running remaining = %d, want 9", summary.Running.Remaining)
	}
	if summary.NeedsYou.Count != 2 {
		t.Fatalf("needs you count = %d, want 2", summary.NeedsYou.Count)
	}
	chips := []string{summary.NeedsYou.Rows[0].Chip, summary.NeedsYou.Rows[1].Chip}
	if !reflect.DeepEqual(chips, []string{"DECIDE", "ANSWER"}) {
		t.Fatalf("needs you chips = %v", chips)
	}
	if summary.NeedsYou.Rows[0].OpenStage == nil || *summary.NeedsYou.Rows[0].OpenStage != waiting.StageID {
		t.Fatalf("decide opens %v, want %s", summary.NeedsYou.Rows[0].OpenStage, waiting.StageID)
	}
	if summary.NeedsYou.Rows[1].OpenStage == nil || *summary.NeedsYou.Rows[1].OpenStage != held.StageID {
		t.Fatalf("answer opens %v, want %s", summary.NeedsYou.Rows[1].OpenStage, held.StageID)
	}
}

// TestSummaryUsesTheSuppliedClockAndCalendarAndMatchesRunSummary
func TestSummaryUsesTheSuppliedClockAndCalendarAndMatchesRunSummary(t *testing.T) {
	t.Parallel()
	amsterdam, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	local := ovNow.In(amsterdam)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, amsterdam)
	shippedBefore := ovRunAt("before", "issues", RunStateShipped, midnight.Add(-time.Second))
	shippedToday := ovRunAt("today", "issues", RunStateShipped, midnight)
	doneToday := ovRunAt("done-today", "issues", RunStateDone, midnight.Add(time.Hour))
	passive := ovRun("watch", "issues", RunStateRunning)
	passive.StageID = wpStageOfKind(passive, StageKindWatch)
	runs := []Run{shippedBefore, shippedToday, doneToday, passive}
	view := NewOverviewView(OverviewViewArgs{Belts: []OverviewBelt{ovFloorBelt("issues", false, false)}, Runs: runs, Now: ovNow, Location: amsterdam})
	if want := RunSummaryCounts(runs, ovNow, amsterdam); view.Summary != want {
		t.Fatalf("summary = %+v, want RunSummary's %+v", view.Summary, want)
	}
	if view.Summary.ShippedToday != 1 {
		t.Fatalf("shipped today = %d, want 1: a Done run finished today is not shipped", view.Summary.ShippedToday)
	}
	if view.Summary.Active != 0 {
		t.Fatalf("active = %d, want 0: a passive watch is not active", view.Summary.Active)
	}
	if view.Summary.Watching != 1 {
		t.Fatalf("watching = %d, want 1", view.Summary.Watching)
	}
}

// TestOverviewFloorWidthSpreadsColumnsAndNeverGoesBelowMinimum
func TestOverviewFloorWidthSpreadsColumnsAndNeverGoesBelowMinimum(t *testing.T) {
	t.Parallel()
	belt := DefaultsBelt([]string{"a/b"}, "agent", DefaultsBuiltInAgents(), true)
	track := OverviewPresentationTrack([]OverviewBelt{OverviewBeltFromBelt(belt, false, false)})
	narrowWidth := 100.0
	narrow := NewFloorLayout(track, OverviewBoxesMetrics(), &narrowWidth, func(FactoryTrackStage) float64 { return 96 })
	if narrow.Column != OverviewBoxesMetrics().MinColumn {
		t.Errorf("narrow column = %v, want the minimum %v", narrow.Column, OverviewBoxesMetrics().MinColumn)
	}
	wideWidth := 2000.0
	wide := NewFloorLayout(track, OverviewBoxesMetrics(), &wideWidth, func(FactoryTrackStage) float64 { return 96 })
	if wide.Column <= narrow.Column {
		t.Errorf("wide column = %v, want more than the narrow %v", wide.Column, narrow.Column)
	}
}

// TestOverviewBuildsTheSharedTrack checks the Overview lays its Floor out on the track of its belts.
func TestOverviewBuildsTheSharedTrack(t *testing.T) {
	t.Parallel()
	source := DefaultsBelt([]string{"acme/billing"}, "agent", DefaultsBuiltInAgents(), true)
	docs := source
	docs.ID = "docs"
	docs.Name = "Docs"
	belts := []OverviewBelt{OverviewBeltFromBelt(source, false, false), OverviewBeltFromBelt(docs, false, false)}
	runs := []Run{ovRun("one", "issues", RunStateRunning)}
	view := NewOverviewView(OverviewViewArgs{Belts: belts, Runs: runs, Now: ovNow})
	if got := []string{view.FactoryTrack.Belts[0].ID, view.FactoryTrack.Belts[1].ID}; !reflect.DeepEqual(got, []string{"issues", "docs"}) {
		t.Fatalf("track belts = %v, want [issues docs]", got)
	}
	if len(view.Floor.Boxes) != len(view.FactoryTrack.Stages) {
		t.Errorf("boxes = %d, want one per merged stage (%d)", len(view.Floor.Boxes), len(view.FactoryTrack.Stages))
	}
}

// TestEmptyFloorAndEmptyRunsProduceEmptyGeometry
func TestEmptyFloorAndEmptyRunsProduceEmptyGeometry(t *testing.T) {
	t.Parallel()
	view := NewOverviewView(OverviewViewArgs{Now: ovNow})
	if view.Summary.Total != 0 {
		t.Errorf("summary total = %d, want 0", view.Summary.Total)
	}
	if len(view.FactoryTrack.Belts) != 0 || len(view.FactoryTrack.Stages) != 0 {
		t.Errorf("track has belts or merged stages: %+v", view.FactoryTrack)
	}
	if len(view.Floor.Boxes) != 0 || len(view.Floor.Layout.Tracks) != 0 || len(view.Floor.Layout.Lanes) != 0 {
		t.Errorf("empty floor has boxes, tracks or lanes: %d %d %d", len(view.Floor.Boxes), len(view.Floor.Layout.Tracks), len(view.Floor.Layout.Lanes))
	}
}

// TestOverviewOrderedRunsSortsByID pins the order the Overview lists its runs in: by id,
// whatever order the runs map was filled in.
func TestOverviewOrderedRunsSortsByID(t *testing.T) {
	t.Parallel()
	view := OverviewView{Runs: map[string]OverviewRun{
		"run-c": {ID: "run-c"},
		"run-a": {ID: "run-a"},
		"run-b": {ID: "run-b"},
	}}
	var ids []string
	for _, run := range view.OrderedRuns() {
		ids = append(ids, run.ID)
	}
	if want := []string{"run-a", "run-b", "run-c"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ordered run ids = %v, want %v", ids, want)
	}
	if got := (OverviewView{}).OrderedRuns(); len(got) != 0 {
		t.Errorf("empty view ordered runs = %v, want none", got)
	}
}
