// Tests of the Insights view: visits, first tries, tries and cost per visit, models, the belt's
// figures and their lines, periods and the overview of every belt.

package core

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

var insightsNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// insightsTestRun is a run of the default belt, numbered n, created at created.
func insightsTestRun(t *testing.T, n int, created time.Time) Run {
	t.Helper()
	subject := DefaultSubject("a/b", "Test")
	subject.Number = n
	return NewRun(RunArgs{Belt: SampleBelt(nil, "agent", SampleAgents(), true), Subject: subject, Now: created})
}

// insightsStageOf is the id of the run's index-th stage of kind.
func insightsStageOf(t *testing.T, run Run, kind StageKind, index int) string {
	t.Helper()
	seen := 0
	for _, stage := range run.Stages {
		if stage.Kind() == kind {
			if seen == index {
				return stage.ID
			}
			seen++
		}
	}
	t.Fatalf("the default belt has no %s stage number %d", kind, index)
	return ""
}

// insightsAttemptArgs describe one ended attempt.
type insightsAttemptArgs struct {
	stage   string
	number  int
	ended   time.Time
	length  time.Duration
	tone    Tone
	stopped bool
	cost    *int
	model   string
}

func insightsAdd(run *Run, args insightsAttemptArgs) {
	length := args.length
	if length == 0 {
		length = time.Minute
	}
	attempt := NewAttempt(AttemptArgs{StageID: args.stage, Number: args.number, StartedAt: jsonx.Date{Time: args.ended.Add(-length)}, Folder: args.stage})
	end := jsonx.Date{Time: args.ended}
	attempt.EndedAt = &end
	if args.stopped {
		attempt.Route = new("Stopped: by you")
	} else {
		attempt.Tone = &args.tone
	}
	if args.cost != nil {
		attempt.Usage = new(NewUsage(UsageArgs{CostNanos: args.cost}))
	}
	if args.model != "" {
		attempt.Settings = &SessionSettings{Model: args.model}
	}
	run.Attempts = append(run.Attempts, attempt)
}

// insightsPrice gives the run's last attempt a dollar cost of nanos.
func insightsPrice(run *Run, nanos int) {
	run.Attempts[len(run.Attempts)-1].Usage = new(NewUsage(UsageArgs{CostNanos: &nanos}))
}

func insightsStage(t *testing.T, view InsightsView, key string) InsightsStage {
	t.Helper()
	for _, stage := range view.Stages {
		if stage.Key == key {
			return stage
		}
	}
	t.Fatalf("no stage %q in %v", key, view.Stages)
	return InsightsStage{}
}

// TestInsightsStageCountsVisitsFirstTriesTriesAndCost checks one stage's figures: a stopped
// attempt counts nowhere, the first counted attempt is the first try, and cost is per priced visit.
func TestInsightsStageCountsVisitsFirstTriesTriesAndCost(t *testing.T) {
	t.Parallel()
	one := insightsTestRun(t, 1, insightsNow.Add(-48*time.Hour))
	build := insightsStageOf(t, one, StageKindAgent, 1)
	at := insightsNow.Add(-24 * time.Hour)
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 1, ended: at, stopped: true})
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 2, ended: at.Add(time.Hour), tone: ToneProblem, cost: new(100_000_000), length: 2 * time.Minute})
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 3, ended: at.Add(2 * time.Hour), tone: ToneGood, cost: new(200_000_000), length: 4 * time.Minute})
	two := insightsTestRun(t, 2, insightsNow.Add(-48*time.Hour))
	insightsAdd(&two, insightsAttemptArgs{stage: build, number: 1, ended: at, tone: ToneGood, length: 6 * time.Minute})

	view := Insights([]Run{one, two}, nil, InsightsQuery{Belt: one.BeltID, Period: InsightsPeriodAll}, insightsNow, time.UTC)
	got := insightsStage(t, view, build).Figures
	if got.Visits != 2 || got.FirstTry != (InsightsRate{Count: 1, Of: 2, Percent: "50%"}) || got.Tries != "1.50" {
		t.Fatalf("visits %d, first try %+v, tries %q", got.Visits, got.FirstTry, got.Tries)
	}
	if got.MedianTime != "4m 0s" {
		t.Fatalf("median time = %q, want the middle of 2, 4 and 6 minutes", got.MedianTime)
	}
	if got.Cost != "$0.30" || got.CostCaption != "1 of 2 visits priced" {
		t.Fatalf("cost = %q (%q)", got.Cost, got.CostCaption)
	}
	if !got.LowSample {
		t.Fatal("two visits are a small sample")
	}
}

// TestInsightsVisitIsOneEntryIntoTheStage checks that a run sent back to a stage visits it again:
// visits count entries, visits per run divides them by the runs, and each entry has a first try.
// A stopped attempt at another stage still ends the visit before it.
func TestInsightsVisitIsOneEntryIntoTheStage(t *testing.T) {
	t.Parallel()
	one := insightsTestRun(t, 1, insightsNow.Add(-48*time.Hour))
	build, verify := insightsStageOf(t, one, StageKindAgent, 1), insightsStageOf(t, one, StageKindAgent, 2)
	at := insightsNow.Add(-24 * time.Hour)
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 1, ended: at, tone: ToneGood})
	insightsAdd(&one, insightsAttemptArgs{stage: verify, number: 1, ended: at.Add(time.Hour), tone: ToneProblem})
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 2, ended: at.Add(2 * time.Hour), tone: ToneProblem})
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 3, ended: at.Add(3 * time.Hour), tone: ToneGood})
	insightsAdd(&one, insightsAttemptArgs{stage: verify, number: 2, ended: at.Add(4 * time.Hour), stopped: true})
	insightsAdd(&one, insightsAttemptArgs{stage: build, number: 4, ended: at.Add(5 * time.Hour), tone: ToneGood})
	two := insightsTestRun(t, 2, insightsNow.Add(-48*time.Hour))
	insightsAdd(&two, insightsAttemptArgs{stage: build, number: 1, ended: at, tone: ToneGood})

	view := Insights([]Run{one, two}, nil, InsightsQuery{Belt: one.BeltID, Period: InsightsPeriodAll}, insightsNow, time.UTC)
	got := insightsStage(t, view, build).Figures
	if got.Visits != 4 || got.VisitsPerRun != "2.00" || got.Tries != "1.25" {
		t.Fatalf("visits %d, per run %q, tries %q; want 4 entries over 2 runs, 5 tries", got.Visits, got.VisitsPerRun, got.Tries)
	}
	if got.FirstTry != (InsightsRate{Count: 3, Of: 4, Percent: "75%"}) {
		t.Fatalf("first try = %+v, want one per entry", got.FirstTry)
	}
	if got := insightsStage(t, view, verify).Figures; got.Visits != 1 || got.VisitsPerRun != "1.00" {
		t.Fatalf("verify: visits %d, per run %q; the stopped attempt counts nowhere", got.Visits, got.VisitsPerRun)
	}
}

// TestInsightsModelsRankByFirstTryAndMarkTheBest checks the models of a stage: those with enough
// visits first, by first try, the lower cost breaking a tie, and the best marked.
func TestInsightsModelsRankByFirstTryAndMarkTheBest(t *testing.T) {
	t.Parallel()
	var runs []Run
	add := func(model string, good bool, cost int) {
		run := insightsTestRun(t, len(runs)+1, insightsNow.Add(-72*time.Hour))
		tone := ToneProblem
		if good {
			tone = ToneGood
		}
		insightsAdd(&run, insightsAttemptArgs{stage: insightsStageOf(t, run, StageKindAgent, 0), number: 1, ended: insightsNow.Add(-time.Hour), tone: tone, cost: new(cost), model: model})
		runs = append(runs, run)
	}
	for i := range 5 {
		add("fast", i < 4, 10)
		add("cheap", i < 4, 5)
		add("weak", i < 2, 1)
	}
	add("rare", true, 1)
	view := Insights(runs, nil, InsightsQuery{Belt: runs[0].BeltID}, insightsNow, time.UTC)
	models := insightsStage(t, view, insightsStageOf(t, runs[0], StageKindAgent, 0)).Models
	var order []string
	for _, model := range models {
		order = append(order, model.Model)
	}
	if want := []string{"cheap", "fast", "weak", "rare"}; !equalStrings(order, want) {
		t.Fatalf("model order = %v, want %v", order, want)
	}
	if !models[0].Best || models[1].Best || !models[3].Figures.LowSample {
		t.Fatalf("best and small samples: %+v", models)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestInsightsStagesFollowBeltOrderAndNameApprovals checks the stages: every stage of the belt in its
// order, one nobody visited listed empty, and the approval label.
func TestInsightsStagesFollowBeltOrderAndNameApprovals(t *testing.T) {
	t.Parallel()
	run := insightsTestRun(t, 1, insightsNow.Add(-48*time.Hour))
	gate := insightsStageOf(t, run, StageKindGate, 0)
	first := insightsStageOf(t, run, StageKindAgent, 0)
	insightsAdd(&run, insightsAttemptArgs{stage: gate, number: 1, ended: insightsNow.Add(-2 * time.Hour), tone: ToneGood})
	insightsAdd(&run, insightsAttemptArgs{stage: first, number: 1, ended: insightsNow.Add(-3 * time.Hour), tone: ToneGood})
	view := Insights([]Run{run}, nil, InsightsQuery{Belt: run.BeltID}, insightsNow, time.UTC)
	var keys []string
	for _, stage := range view.Stages {
		keys = append(keys, stage.Key)
	}
	var want []string
	for _, stage := range run.Stages {
		want = append(want, stage.ID)
	}
	if !equalStrings(keys, want) {
		t.Fatalf("stages = %v, want the belt's %v", keys, want)
	}
	approval := insightsStage(t, view, gate)
	if approval.KindLabel != "APPROVAL" || insightsStage(t, view, first).KindLabel != "" || len(approval.Models) != 0 {
		t.Fatalf("kind labels and models: %+v", view.Stages)
	}
	unvisited := insightsStage(t, view, insightsStageOf(t, run, StageKindAgent, 1)).Figures
	if unvisited.Visits != 0 || unvisited.FirstTry.Percent != "–" || unvisited.CostCaption != "No visits in this period" {
		t.Fatalf("an unvisited stage = %+v", unvisited)
	}
}

// TestInsightsBeltFiguresSubtractPauseAndKeepToThePeriod checks the ship rate, the lead time less
// paused time, and that the week leaves out what ended before it.
func TestInsightsBeltFiguresSubtractPauseAndKeepToThePeriod(t *testing.T) {
	t.Parallel()
	shipped := insightsTestRun(t, 1, insightsNow.Add(-5*time.Hour))
	stage := insightsStageOf(t, shipped, StageKindAgent, 0)
	insightsAdd(&shipped, insightsAttemptArgs{stage: stage, number: 1, ended: insightsNow.Add(-time.Hour), tone: ToneGood})
	shipped.State = RunStateShipped
	shipped.PauseTime = []RunPauseInterval{{Start: jsonx.Date{Time: insightsNow.Add(-4 * time.Hour)}, End: jsonx.Date{Time: insightsNow.Add(-3 * time.Hour)}}}
	dropped := insightsTestRun(t, 2, insightsNow.Add(-3*time.Hour))
	insightsAdd(&dropped, insightsAttemptArgs{stage: stage, number: 1, ended: insightsNow.Add(-2 * time.Hour), tone: ToneProblem})
	dropped.State = RunStateDropped
	old := insightsTestRun(t, 3, insightsNow.Add(-30*24*time.Hour))
	insightsAdd(&old, insightsAttemptArgs{stage: stage, number: 1, ended: insightsNow.Add(-29 * 24 * time.Hour), tone: ToneGood})
	old.State = RunStateShipped

	overview := Insights([]Run{shipped, dropped, old}, nil, InsightsQuery{Period: InsightsPeriodWeek}, insightsNow, time.UTC)
	row := overview.BeltRows[0]
	if row.ShipRate.Percent != "50%" || row.FirstTry.Percent != "50%" || row.LeadTime != "3h 0m" {
		t.Fatalf("figures = %+v", row)
	}
	if len(row.Trend.Points) != 8 {
		t.Fatalf("the line has %d points, want one a day for the week and today", len(row.Trend.Points))
	}
	view := Insights([]Run{shipped, dropped, old}, nil, InsightsQuery{Belt: shipped.BeltID, Period: InsightsPeriodWeek}, insightsNow, time.UTC)
	if view.Caption != "2 runs · 2 attempts" {
		t.Fatalf("caption = %q", view.Caption)
	}
}

// TestInsightsDeltaComparesTheLastTwoWeeks checks the change: points for a share, flat under five,
// and none without five on each side.
func TestInsightsDeltaComparesTheLastTwoWeeks(t *testing.T) {
	t.Parallel()
	var runs []Run
	add := func(daysAgo int, good bool) {
		run := insightsTestRun(t, len(runs)+1, insightsNow.Add(-time.Duration(daysAgo+1)*24*time.Hour))
		tone := ToneProblem
		if good {
			tone = ToneGood
		}
		insightsAdd(&run, insightsAttemptArgs{stage: insightsStageOf(t, run, StageKindAgent, 0), number: 1, ended: insightsNow.Add(-time.Duration(daysAgo)*24*time.Hour - time.Hour), tone: tone})
		runs = append(runs, run)
	}
	for i := range 5 {
		add(10, i < 2) // 40% the week before
		add(2, i < 4)  // 80% this week
	}
	view := Insights(runs, nil, InsightsQuery{}, insightsNow, time.UTC)
	delta := view.BeltRows[0].Trend.Delta
	if delta.Label != "+40 pts" || delta.Tone != InsightsDeltaBetter {
		t.Fatalf("delta = %+v", delta)
	}
}

// TestInsightsEveryBeltListsBeltsWithRuns checks the overview: library belts with runs, and a
// deleted belt by its runs' name, most runs first.
func TestInsightsEveryBeltListsBeltsWithRuns(t *testing.T) {
	t.Parallel()
	a := insightsTestRun(t, 1, insightsNow.Add(-time.Hour))
	insightsAdd(&a, insightsAttemptArgs{stage: insightsStageOf(t, a, StageKindAgent, 0), number: 1, ended: insightsNow.Add(-time.Minute), tone: ToneGood})
	b := a.Clone()
	b.ID, b.BeltID, b.BeltName = "gone-1", "gone", "Gone belt"
	c := b.Clone()
	c.ID = "gone-2"
	belts := []InsightsLibraryBelt{{ID: a.BeltID, Name: "Issues", Version: 1}, {ID: "idle", Name: "Idle", Version: 3}}
	view := Insights([]Run{a, b, c}, belts, InsightsQuery{}, insightsNow, time.UTC)
	if len(view.BeltRows) != 2 || view.BeltRows[0].Name != "Gone belt" || view.BeltRows[0].Runs != 2 || view.BeltRows[1].Name != "Issues" {
		t.Fatalf("belt rows = %+v", view.BeltRows)
	}
	if len(view.Stages) != 0 || view.Caption != "2 belts with runs" || len(view.Belts) != 3 {
		t.Fatalf("overview: %d stages, caption %q, %d belts", len(view.Stages), view.Caption, len(view.Belts))
	}
	has := func(belt string, version int, runs ...Run) bool {
		return InsightsHasBelt(InsightsQuery{Belt: belt, Version: version}, belts, runs)
	}
	if !has("gone", 0, b) || !has("idle", 0) || !has("idle", 3) || has("idle", 2) || has("nope", 0, b) || !has("gone", b.BeltVersion, b) || has("gone", b.BeltVersion+1, b) {
		t.Fatal("InsightsHasBelt")
	}
}

// TestInsightsLinesReadEveryRunWhateverThePeriod checks the lines of a short period: the first day's
// 7-day window still holds the runs before the period, the change compares the week before, a line
// starts at the first day with runs rather than the period's start, and a thin window leaves a gap.
func TestInsightsLinesReadEveryRunWhateverThePeriod(t *testing.T) {
	t.Parallel()
	var runs []Run
	add := func(daysAgo int, good bool) {
		run := insightsTestRun(t, len(runs)+1, insightsNow.Add(-time.Duration(daysAgo+1)*24*time.Hour))
		tone := ToneProblem
		if good {
			tone = ToneGood
		}
		insightsAdd(&run, insightsAttemptArgs{stage: insightsStageOf(t, run, StageKindAgent, 0), number: 1, ended: insightsNow.Add(-time.Duration(daysAgo)*24*time.Hour - time.Hour), tone: tone})
		runs = append(runs, run)
	}
	for i := range 5 {
		add(9, i < 1) // 20%, the week before this one
		add(1, i < 4) // 80%, yesterday
	}
	week := Insights(runs, nil, InsightsQuery{Period: InsightsPeriodWeek}, insightsNow, time.UTC).BeltRows[0]
	if week.FirstTry.Percent != "80%" {
		t.Fatalf("the week's first try = %q, want only this week's runs", week.FirstTry.Percent)
	}
	if got := week.Trend.Points[0]; got.Label != "Oct 9" || got.Count != 5 || got.Display != "20%" {
		t.Fatalf("the week's first point = %+v, want the 7 days before it, with the older runs", got)
	}
	if week.Trend.Delta.Label != "+60 pts" {
		t.Fatalf("the week's change = %+v, want it measured against the week before", week.Trend.Delta)
	}
	quarter := Insights(runs, nil, InsightsQuery{Period: InsightsPeriodQuarter}, insightsNow, time.UTC).BeltRows[0]
	if points := quarter.Trend.Points; points[0].Label != "Oct 7" || len(points) != 10 {
		t.Fatalf("the quarter's line starts %q with %d points, want the first day with runs", points[0].Label, len(points))
	}

	thin := insightsTestRun(t, 99, insightsNow.Add(-48*time.Hour))
	insightsAdd(&thin, insightsAttemptArgs{stage: insightsStageOf(t, thin, StageKindAgent, 0), number: 1, ended: insightsNow.Add(-time.Hour), tone: ToneGood})
	lone := Insights([]Run{thin}, nil, InsightsQuery{}, insightsNow, time.UTC).BeltRows[0]
	for _, point := range lone.Trend.Points {
		if point.Value != nil {
			t.Fatalf("a window of one run drew a point: %+v", point)
		}
	}
}

// TestInsightsVersionsChooseRunsAndStages checks the versions: every version lists the current
// version's stages, leaving out one it removed; a chosen version keeps to its runs and its own
// stages; and the picker names each version with its runs, newest first.
func TestInsightsVersionsChooseRunsAndStages(t *testing.T) {
	t.Parallel()
	old := insightsTestRun(t, 1, insightsNow.Add(-48*time.Hour))
	old.BeltVersion = 1
	removed := insightsStageOf(t, old, StageKindAgent, 1)
	kept := insightsStageOf(t, old, StageKindAgent, 0)
	insightsAdd(&old, insightsAttemptArgs{stage: kept, number: 1, ended: insightsNow.Add(-40 * time.Hour), tone: ToneProblem})
	insightsAdd(&old, insightsAttemptArgs{stage: removed, number: 1, ended: insightsNow.Add(-39 * time.Hour), tone: ToneGood})
	current := insightsTestRun(t, 2, insightsNow.Add(-24*time.Hour))
	current.BeltVersion = 2
	current.Stages = slices.DeleteFunc(slices.Clone(current.Stages), func(stage Stage) bool { return stage.ID == removed })
	insightsAdd(&current, insightsAttemptArgs{stage: kept, number: 1, ended: insightsNow.Add(-20 * time.Hour), tone: ToneGood})
	library := []InsightsLibraryBelt{{ID: current.BeltID, Name: "Issues", Version: 2, Stages: current.Stages}}
	runs := []Run{old, current}

	every := Insights(runs, library, InsightsQuery{Belt: current.BeltID}, insightsNow, time.UTC)
	if slices.ContainsFunc(every.Stages, func(stage InsightsStage) bool { return stage.Key == removed }) {
		t.Fatalf("every version lists %q, which the current version removed", removed)
	}
	if got := insightsStage(t, every, kept).Figures; got.Visits != 2 || got.FirstTry.Percent != "50%" {
		t.Fatalf("every version's %s = %+v, want both runs", kept, got)
	}
	first := Insights(runs, library, InsightsQuery{Belt: current.BeltID, Version: 1}, insightsNow, time.UTC)
	if got := insightsStage(t, first, kept).Figures; got.Visits != 1 || got.FirstTry.Percent != "0%" {
		t.Fatalf("v1's %s = %+v, want only the v1 run", kept, got)
	}
	if insightsStage(t, first, removed).Figures.Visits != 1 || first.Version != 1 {
		t.Fatalf("v1 does not list its own stage %q", removed)
	}
	var labels []string
	for _, version := range every.Versions {
		labels = append(labels, version.Label)
	}
	if want := []string{"All versions · 2 runs", "v2 (current) · 1 run", "v1 · 1 run"}; !equalStrings(labels, want) {
		t.Fatalf("versions = %v, want %v", labels, want)
	}
}

// insightsBeltRun is a run of the belt beltID, named name, numbered n and created at created, with one
// agent attempt that ended at created, so the run counts in the periods that reach it.
func insightsBeltRun(t *testing.T, n int, beltID, name string, created time.Time) Run {
	t.Helper()
	run := insightsTestRun(t, n, created)
	run.ID = beltID + "-" + strconv.Itoa(n)
	run.BeltID, run.BeltName = beltID, name
	insightsAdd(&run, insightsAttemptArgs{stage: insightsStageOf(t, run, StageKindAgent, 0), number: 1, ended: created, tone: ToneGood})
	return run
}

// insightsBeltIDs and insightsRowIDs list the belts a picker offers and the overview's rows, in order.
func insightsBeltIDs(belts []InsightsBelt) []string {
	ids := []string{}
	for _, belt := range belts {
		ids = append(ids, belt.ID)
	}
	return ids
}

func insightsRowIDs(rows []InsightsBeltRow) []string {
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

// insightsArchiveFixture is two active belts and four archived ones. The active issues belt has two
// runs, the archived old belt three, and a belt only its runs name, gone, one. The idle belt and the
// archived retired belt have no runs, and retired's library record is the only one for it. The archived
// faded belt has one run, from 30 days before now, so it has runs only outside a week.
func insightsArchiveFixture(t *testing.T) ([]Run, []InsightsLibraryBelt) {
	t.Helper()
	recent := insightsNow.Add(-time.Hour)
	runs := []Run{
		insightsBeltRun(t, 1, "issues", "Issues", recent), insightsBeltRun(t, 2, "issues", "Issues", recent),
		insightsBeltRun(t, 3, "old", "Old", recent), insightsBeltRun(t, 4, "old", "Old", recent), insightsBeltRun(t, 5, "old", "Old", recent),
		insightsBeltRun(t, 6, "gone", "Gone belt", recent),
		insightsBeltRun(t, 7, "faded", "Faded", insightsNow.AddDate(0, 0, -30)),
	}
	stages := insightsTestRun(t, 0, recent).Stages
	library := []InsightsLibraryBelt{
		{ID: "issues", Name: "Issues", Version: 1, Stages: stages},
		{ID: "idle", Name: "Idle", Version: 1, Stages: stages},
		{ID: "old", Name: "Old", Version: 1, Stages: stages, Archived: true},
		{ID: "retired", Name: "Retired", Version: 1, Stages: stages, Archived: true},
		{ID: "faded", Name: "Faded", Version: 1, Stages: stages, Archived: true},
	}
	return runs, library
}

// TestInsightsArchivedBeltsAreHiddenByDefault checks the default lists: an archived belt with runs and
// one without leave the picker and the overview, the active belts keep their order and figures, and a
// belt only its runs name stays.
func TestInsightsArchivedBeltsAreHiddenByDefault(t *testing.T) {
	t.Parallel()
	runs, library := insightsArchiveFixture(t)
	view := Insights(runs, library, InsightsQuery{Period: InsightsPeriodAll}, insightsNow, time.UTC)
	if got, want := insightsBeltIDs(view.Belts), []string{"issues", "gone", "idle"}; !slices.Equal(got, want) {
		t.Fatalf("picker = %v, want %v", got, want)
	}
	if got, want := insightsRowIDs(view.BeltRows), []string{"issues", "gone"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if view.BeltRows[0].Runs != 2 || view.BeltRows[1].Runs != 1 || view.BeltRows[1].Name != "Gone belt" {
		t.Fatalf("rows = %+v", view.BeltRows)
	}
	if view.Caption != "2 belts with runs" {
		t.Fatalf("caption = %q", view.Caption)
	}
}

// TestInsightsShowArchivedOffersArchivedBeltsWithRunsInThePeriod checks the opt-in: archived belts join
// the picker, and the overview gets those with runs in the period. An archived belt whose runs all fall
// before the period is in the picker but has no row for it. An archived belt with no runs at all, retired,
// is in the picker and never in the overview.
func TestInsightsShowArchivedOffersArchivedBeltsWithRunsInThePeriod(t *testing.T) {
	t.Parallel()
	runs, library := insightsArchiveFixture(t)
	week := Insights(runs, library, InsightsQuery{Period: InsightsPeriodWeek, ShowArchived: true}, insightsNow, time.UTC)
	if got, want := insightsBeltIDs(week.Belts), []string{"old", "issues", "faded", "gone", "idle", "retired"}; !slices.Equal(got, want) {
		t.Fatalf("picker = %v, want %v", got, want)
	}
	if got, want := insightsRowIDs(week.BeltRows), []string{"old", "issues", "gone"}; !slices.Equal(got, want) {
		t.Fatalf("7-day rows = %v, want %v", got, want)
	}
	if week.BeltRows[0].Runs != 3 || week.Caption != "3 belts with runs" {
		t.Fatalf("7-day rows = %+v, caption %q", week.BeltRows, week.Caption)
	}
	all := Insights(runs, library, InsightsQuery{Period: InsightsPeriodAll, ShowArchived: true}, insightsNow, time.UTC)
	if got, want := insightsRowIDs(all.BeltRows), []string{"old", "issues", "faded", "gone"}; !slices.Equal(got, want) {
		t.Fatalf("all-time rows = %v, want %v", got, want)
	}
	if slices.Contains(insightsRowIDs(all.BeltRows), "retired") || !slices.Contains(insightsBeltIDs(all.Belts), "retired") {
		t.Fatalf("archived belt without runs: picker %v, rows %v", insightsBeltIDs(all.Belts), insightsRowIDs(all.BeltRows))
	}
	hidden := Insights(runs, library, InsightsQuery{Period: InsightsPeriodAll}, insightsNow, time.UTC)
	if got, want := insightsBeltIDs(hidden.Belts), []string{"issues", "gone", "idle"}; !slices.Equal(got, want) {
		t.Fatalf("picker without the opt-in = %v, want %v", got, want)
	}
}

// TestInsightsArchivedBeltChosenKeepsItsDetail checks an archived belt named by the query: it stays in
// the picker and its detail matches the detail with the opt-in on. Once the query is "All belts" again,
// it leaves the picker, and an archived belt with no runs is an empty detail.
func TestInsightsArchivedBeltChosenKeepsItsDetail(t *testing.T) {
	t.Parallel()
	runs, library := insightsArchiveFixture(t)
	off := Insights(runs, library, InsightsQuery{Belt: "old", Period: InsightsPeriodAll}, insightsNow, time.UTC)
	on := Insights(runs, library, InsightsQuery{Belt: "old", Period: InsightsPeriodAll, ShowArchived: true}, insightsNow, time.UTC)
	if off.Title != "Old" || !slices.Contains(insightsBeltIDs(off.Belts), "old") {
		t.Fatalf("title %q, picker %v", off.Title, insightsBeltIDs(off.Belts))
	}
	if off.Caption != on.Caption || off.Title != on.Title ||
		!reflect.DeepEqual(off.Stages, on.Stages) || !reflect.DeepEqual(off.Versions, on.Versions) {
		t.Fatalf("the chosen archived belt's detail differs from its opt-in view")
	}
	if len(off.Stages) == 0 || off.Caption != "3 runs · 3 attempts" {
		t.Fatalf("stages %d, caption %q", len(off.Stages), off.Caption)
	}
	all := Insights(runs, library, InsightsQuery{Period: InsightsPeriodAll}, insightsNow, time.UTC)
	if slices.Contains(insightsBeltIDs(all.Belts), "old") || len(all.BeltRows) != 2 {
		t.Fatalf("all belts after the choice: picker %v, rows %v", insightsBeltIDs(all.Belts), insightsRowIDs(all.BeltRows))
	}
	empty := Insights(runs, library, InsightsQuery{Belt: "retired", Period: InsightsPeriodAll}, insightsNow, time.UTC)
	if empty.Title != "Retired" || empty.Caption != "0 runs · 0 attempts" || !slices.Contains(insightsBeltIDs(empty.Belts), "retired") {
		t.Fatalf("empty archived detail: title %q, caption %q", empty.Title, empty.Caption)
	}
}

// TestInsightsAllArchivedBeltsLeaveAnEmptyOverview checks a library whose every belt is archived: the
// picker is empty, and the overview keeps its table with no rows and the zero caption.
func TestInsightsAllArchivedBeltsLeaveAnEmptyOverview(t *testing.T) {
	t.Parallel()
	runs := []Run{insightsBeltRun(t, 1, "old", "Old", insightsNow.Add(-time.Hour))}
	library := []InsightsLibraryBelt{{ID: "old", Name: "Old", Version: 1, Archived: true}}
	view := Insights(runs, library, InsightsQuery{Period: InsightsPeriodAll}, insightsNow, time.UTC)
	if len(view.Belts) != 0 || view.BeltRows == nil || len(view.BeltRows) != 0 || view.Caption != "0 belts with runs" {
		t.Fatalf("picker %v, rows %v, caption %q", view.Belts, view.BeltRows, view.Caption)
	}
}
