//
// The Overview's Summary as values: the four posts and the two lists, with
// the figures, ordering and limits the Summary shows. The browser renders these
// values and computes none of them; the API sends them as they are.

package core

import (
	"sort"
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// OverviewSummarySpend is the Spend post: today's ended-session cost.
type OverviewSummarySpend struct {
	// Amount is `$0.00` when nothing has ended or none of it was priced.
	Amount  string `json:"amount"`
	Caption string `json:"caption"`
	// Note is the partial or unpriced copy for the day's ended sessions, if any.
	Note *string `json:"note,omitzero"`
}

// OverviewSummarySessions is the Sessions post: session counts over the last 14 days.
type OverviewSummarySessions struct {
	Today   int `json:"today"`
	Running int `json:"running"`
	// Buckets are session counts per six-hour block over 14 days, oldest first; 56 cells.
	Buckets []int `json:"-"`
	// Start is the first day's midnight, the axis's start.
	Start jsonx.Date `json:"start"`
	// StartLabel is the first day's axis label, as the app writes it ("SEP 27").
	StartLabel string `json:"startLabel"`
	// Shades is the shade level (0 to 4) of each of the 56 cells, in drawing order: 14
	// day columns of 4 six-hour rows.
	Shades         []int  `json:"shades"`
	RunningCaption string `json:"runningCaption"`
}

// OverviewSummaryRunning is the Running post: how many runs are running, and the first few.
type OverviewSummaryRunning struct {
	Count int `json:"count"`
	// RunIDs are up to four runs in the order the Summary lists them.
	RunIDs    []string `json:"runIds"`
	Remaining int      `json:"remaining"`
	Empty     string   `json:"empty"`
	// Busy is "N AGENTS BUSY", the post's foot caption.
	Busy string `json:"busy"`
}

// OverviewSummaryShipped is the Shipped post: shipped runs per week, the median and the sparkline.
type OverviewSummaryShipped struct {
	// Weeks are shipped runs per week over twelve weeks, oldest first.
	Weeks  []int `json:"weeks"`
	Latest int   `json:"latest"`
	// Median is "—" when nothing has shipped.
	Median string `json:"median"`
	// Chart is the sparkline's drawing in its 300 × 150 space.
	Chart OverviewSummaryChart `json:"chart"`
}

// OverviewSummaryChart is the Shipped post's sparkline in its 300 × 150 space, with its axis labels.
type OverviewSummaryChart struct {
	// Line is the smooth line through every week, as SVG path data.
	Line string `json:"line"`
	// Area is the line closed along the base, as SVG path data.
	Area string `json:"area"`
	// DotX and DotY are the latest week's point.
	DotX        float64 `json:"dotX"`
	DotY        float64 `json:"dotY"`
	StartLabel  string  `json:"startLabel"`
	EndLabel    string  `json:"endLabel"`
	LatestLabel string  `json:"latestLabel"`
}

// OverviewSummaryRow is a row in In progress or Needs you. The glyph, look and chip say what
// the run is doing; OpenStage is the stage the row opens at, if any.
type OverviewSummaryRow struct {
	RunID     string          `json:"runId"`
	Glyph     string          `json:"glyph"`
	Look      StageStatusLook `json:"look"`
	Title     string          `json:"title"`
	Chip      string          `json:"chip"`
	OpenStage *string         `json:"openStage,omitzero"`
}

// OverviewSummaryRows is a list of rows, the count of all of them and the list's empty copy.
type OverviewSummaryRows struct {
	Count int                  `json:"count"`
	Rows  []OverviewSummaryRow `json:"rows"`
	Empty string               `json:"empty"`
}

// OverviewSummary is the Overview's Summary.
type OverviewSummary struct {
	// Caption is the global counts line; the response carries it on its own.
	Caption    string                  `json:"-"`
	Spend      OverviewSummarySpend    `json:"spend"`
	Sessions   OverviewSummarySessions `json:"sessions"`
	Running    OverviewSummaryRunning  `json:"running"`
	Shipped    OverviewSummaryShipped  `json:"shipped"`
	InProgress OverviewSummaryRows     `json:"inProgress"`
	NeedsYou   OverviewSummaryRows     `json:"needsYou"`
}

// Limits and copy of the Summary.
const (
	// OverviewSummaryInProgressLimit is the most rows In progress shows.
	OverviewSummaryInProgressLimit = 6
	// OverviewSummaryRunningLimit is the most running runs the post lists before the remaining count.
	OverviewSummaryRunningLimit = 4
	// OverviewSummaryEmptyRunning is shown when nothing is running.
	OverviewSummaryEmptyRunning = "Nothing is running."
	// OverviewSummaryEmptyInProgress is shown when nothing is in progress.
	OverviewSummaryEmptyInProgress = "Nothing in progress."
	// OverviewSummaryShadeSteps is the shade steps above zero; a cell at zero takes level 0.
	OverviewSummaryShadeSteps = 4
	// OverviewSummaryEmptyNeedsYou is shown when nothing needs you.
	OverviewSummaryEmptyNeedsYou = "Nothing needs you."
)

// Copy used by OverviewSummary. Literal text is exact.
const (
	overviewSummaryMinutesUnit = " M"
	overviewSummaryStartPrefix = "WK "
)

// overviewSummaryBlocks is the number of six-hour blocks in the Sessions post: 14 days of four.
const overviewSummaryBlocks = 56

// overviewSummaryBlockLength is the length of one block in the Sessions post.
const overviewSummaryBlockLength = 6 * time.Hour

// NewOverviewSummary builds the Summary for runs at now, in loc (nil for the local
// calendar), with the belts in pausedBelts paused under pause.
func NewOverviewSummary(runs []Run, now time.Time, loc *time.Location, pausedBelts map[string]struct{}, pause Pause) OverviewSummary {
	if loc == nil {
		loc = time.Local
	}
	counts := RunSummaryCounts(runs, now, loc)
	spend := CostToday(runs, loc, now)
	return OverviewSummary{
		Caption:    overviewSummaryCaption(counts),
		Spend:      OverviewSummarySpend{Amount: CostAmount(spend), Caption: CostTodayCaptionFor(spend), Note: CostRunLine(spend)},
		Sessions:   overviewSummarySessions(runs, counts.Running, now, loc),
		Running:    overviewSummaryRunning(runs),
		Shipped:    overviewSummaryShipped(runs, now),
		InProgress: overviewSummaryInProgress(runs, pausedBelts, pause),
		NeedsYou:   overviewSummaryNeedsYou(runs),
	}
}

// overviewSummaryCaption is the line under the Overview's heading: running, needing you and shipped today.
func overviewSummaryCaption(counts RunCounts) string {
	return strconv.Itoa(counts.Running) + " running · " +
		strconv.Itoa(counts.NeedsYou) + " need you · " +
		strconv.Itoa(counts.ShippedToday) + " shipped today"
}

// overviewSummarySessions is the Sessions post: the 14-day grid of session starts, today's count and the axis.
func overviewSummarySessions(runs []Run, running int, now time.Time, loc *time.Location) OverviewSummarySessions {
	start := overviewSummaryStartOfDay(now, loc).Add(-13 * 24 * time.Hour)
	buckets := make([]int, overviewSummaryBlocks)
	today := 0
	day := localDayOf(now, loc)
	// A session that started a block or more before the grid counts nowhere: its block index is
	// below zero, and today is inside the grid. Most of a run book's are, so they are passed over
	// before the index is worked out.
	earliest := start.Add(-overviewSummaryBlockLength)
	for i := range runs {
		for j := range runs[i].Attempts {
			attempt := &runs[i].Attempts[j]
			if attempt.Session == nil || !attempt.StartedAt.After(earliest) {
				continue
			}
			index := int(attempt.StartedAt.Time.Sub(start).Seconds() / overviewSummaryBlockLength.Seconds())
			if index >= 0 && index < overviewSummaryBlocks {
				buckets[index]++
			}
			if day.holds(attempt.StartedAt.Time) {
				today++
			}
		}
	}
	return OverviewSummarySessions{
		Today:          today,
		Running:        running,
		Buckets:        buckets,
		Start:          jsonx.Date{Time: start},
		StartLabel:     UppercasedFull(overviewSummaryAxisDate(start)),
		Shades:         overviewSummaryShades(buckets),
		RunningCaption: "RUNS TODAY · " + strconv.Itoa(running) + " RUNNING",
	}
}

// overviewSummaryShades gives each of the 56 grid cells its shade level, scaled to the busiest block (at least one).
// Columns are days and rows six-hour blocks, so cell i reads the bucket of its day and block.
func overviewSummaryShades(buckets []int) []int {
	most := 1
	for _, value := range buckets {
		most = max(most, value)
	}
	shades := make([]int, overviewSummaryBlocks)
	for cell := range shades {
		shades[cell] = OverviewSummaryShade(buckets[(cell%14)*4+cell/14], most)
	}
	return shades
}

// overviewSummaryRunning is the Running post: the running runs that are not passively watching, and the first few of them.
func overviewSummaryRunning(runs []Run) OverviewSummaryRunning {
	running := overviewSummaryRunsWhere(runs, func(run Run) bool {
		return run.State == RunStateRunning && !RunSummaryPassivelyWatching(run)
	})
	runIDs := []string{}
	for i, run := range running {
		if i >= OverviewSummaryRunningLimit {
			break
		}
		runIDs = append(runIDs, run.ID)
	}
	remaining := max(len(running)-OverviewSummaryRunningLimit, 0)
	return OverviewSummaryRunning{Count: len(running), RunIDs: runIDs, Remaining: remaining,
		Empty: OverviewSummaryEmptyRunning, Busy: OverviewSummaryBusyCaption(len(running))}
}

// overviewSummaryShipped is the Shipped post: shipped runs per week over twelve weeks, and the median and sparkline.
func overviewSummaryShipped(runs []Run, now time.Time) OverviewSummaryShipped {
	weeks := make([]int, 12)
	// How long each shipped run took, from creation to its last update.
	var durations []float64
	for i := range runs {
		run := &runs[i]
		if run.State != RunStateShipped {
			continue
		}
		durations = append(durations, run.UpdatedAt.Time.Sub(run.CreatedAt.Time).Seconds())
		age := int(now.Sub(run.UpdatedAt.Time).Seconds() / (7 * 86400))
		if age >= 0 && age < 12 {
			weeks[11-age]++
		}
	}
	return OverviewSummaryShipped{
		Weeks:  weeks,
		Latest: weeks[len(weeks)-1],
		Median: overviewSummaryMedian(durations),
		Chart:  OverviewSummaryChartFor(weeks, now),
	}
}

// overviewSummaryInProgress is the In progress list: the queued, running and waiting runs that are not passively
// watching, most recently updated first, with the rows for the first few.
func overviewSummaryInProgress(runs []Run, pausedBelts map[string]struct{}, pause Pause) OverviewSummaryRows {
	progress := overviewSummaryRunsWhere(runs, func(run Run) bool {
		switch run.State {
		case RunStateQueued, RunStateRunning, RunStateWaiting:
			return !RunSummaryPassivelyWatching(run)
		}
		return false
	})
	sort.SliceStable(progress, func(i, j int) bool {
		return progress[i].UpdatedAt.After(progress[j].UpdatedAt.Time)
	})
	rows := OverviewSummaryRows{Count: len(progress), Rows: []OverviewSummaryRow{}, Empty: OverviewSummaryEmptyInProgress}
	for i, run := range progress {
		if i >= OverviewSummaryInProgressLimit {
			break
		}
		_, paused := pausedBelts[run.BeltID]
		rows.Rows = append(rows.Rows, overviewSummaryInProgressRow(run, paused, pause))
	}
	return rows
}

// overviewSummaryNeedsYou is the Needs you list: a row for every run that needs you, in the runs' order.
func overviewSummaryNeedsYou(runs []Run) OverviewSummaryRows {
	needing := overviewSummaryRunsWhere(runs, func(run Run) bool { return run.State.NeedsYou() })
	rows := OverviewSummaryRows{Count: len(needing), Rows: []OverviewSummaryRow{}, Empty: OverviewSummaryEmptyNeedsYou}
	for _, run := range needing {
		rows.Rows = append(rows.Rows, overviewSummaryNeedsYouRow(run))
	}
	return rows
}

// overviewSummaryRunsWhere keeps the runs keep accepts, in their order.
func overviewSummaryRunsWhere(runs []Run, keep func(Run) bool) []Run {
	var kept []Run
	for i := range runs {
		if keep(runs[i]) {
			kept = append(kept, runs[i])
		}
	}
	return kept
}

// overviewSummaryInProgressRow is one In progress row.
func overviewSummaryInProgressRow(run Run, paused bool, pause Pause) OverviewSummaryRow {
	location := StatusLocation(run, paused, pause)
	chip := ""
	if stage := run.stageRef(); stage != nil {
		chip = stage.Name
	}
	return OverviewSummaryRow{
		RunID:     run.ID,
		Glyph:     location.Glyph,
		Look:      location.Look,
		Title:     run.Subject.Title,
		Chip:      UppercasedFull(chip),
		OpenStage: nil,
	}
}

// overviewSummaryNeedsYouRow is one Needs you row.
func overviewSummaryNeedsYouRow(run Run) OverviewSummaryRow {
	waiting := run.State == RunStateWaiting
	glyph := "✕"
	look := StageStatusLookHeld
	chip := "ANSWER"
	var head string
	if waiting {
		glyph = "!"
		look = StageStatusLookWaiting
		chip = "DECIDE"
		if stage := run.stageRef(); stage != nil {
			head = stage.Name
		}
	} else {
		reason := ValueOr(run.Reason, "held")
		head = CapitalizedFirst(reason)
	}
	stageID := run.StageID
	return OverviewSummaryRow{
		RunID:     run.ID,
		Glyph:     glyph,
		Look:      look,
		Title:     head + " · " + run.Subject.Title,
		Chip:      chip,
		OpenStage: &stageID,
	}
}
