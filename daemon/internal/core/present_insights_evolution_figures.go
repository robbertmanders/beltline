// The Evolution view's six figures: each one's recent value against before, a statistic per column,
// one dot per run, the ticks of its scale, and how its before and recent runs are spread.

package core

import (
	"math"
	"slices"
	"strconv"
)

// InsightsFigureID names one of the Evolution view's figures.
type InsightsFigureID string

const (
	InsightsFigureLead     InsightsFigureID = "lead"
	InsightsFigureToPR     InsightsFigureID = "toPR"
	InsightsFigureCost     InsightsFigureID = "cost"
	InsightsFigureVisits   InsightsFigureID = "visits"
	InsightsFigureTouches  InsightsFigureID = "touches"
	InsightsFigureShipRate InsightsFigureID = "shipRate"
)

// AllInsightsFigureID lists every InsightsFigureID in declaration order (CaseIterable).
var AllInsightsFigureID = []InsightsFigureID{InsightsFigureLead, InsightsFigureToPR, InsightsFigureCost, InsightsFigureVisits, InsightsFigureTouches, InsightsFigureShipRate}

// InsightsTick is one labelled value on a scale.
type InsightsTick struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
}

// InsightsColumnStat is a figure in one column: the median with the middle half of the runs around
// it, or for a share the share alone. Value is nil when the column has no run with the figure.
type InsightsColumnStat struct {
	Value   *float64 `json:"value"`
	Low     *float64 `json:"low"`
	High    *float64 `json:"high"`
	Display string   `json:"display"`
	Runs    int      `json:"runs"`
}

// InsightsDot is one run's figure: Run indexes the view's runs. Partial marks a cost that left out
// attempts without a dollar figure.
type InsightsDot struct {
	Run     int     `json:"run"`
	Value   float64 `json:"value"`
	Display string  `json:"display"`
	Partial bool    `json:"partial,omitzero"`
}

// InsightsBin is one range of a histogram, with the share of the before and the recent runs in it.
type InsightsBin struct {
	From   float64 `json:"from"`
	To     float64 `json:"to"`
	Label  string  `json:"label"`
	Before float64 `json:"before"`
	Recent float64 `json:"recent"`
}

// InsightsSpreadRow is one group's percentiles, in the order of InsightsSpread.Headers.
type InsightsSpreadRow struct {
	Label string   `json:"label"`
	Runs  int      `json:"runs"`
	Cells []string `json:"cells"`
}

// InsightsSpread is how a figure's before and recent runs are spread: a histogram on the linear and
// on the log scale, each group's sorted values for the share at or under a value, and percentiles.
type InsightsSpread struct {
	Bins    []InsightsBin       `json:"bins"`
	LogBins []InsightsBin       `json:"logBins"`
	Before  []float64           `json:"before"`
	Recent  []float64           `json:"recent"`
	Headers []string            `json:"headers"`
	Rows    []InsightsSpreadRow `json:"rows"`
}

// InsightsFigure is one figure of the Evolution view. Value is the recent runs' median, or share for
// Share; Change compares it with before, such as "↓ −4%". Min and Max bound the linear scale, LogMin
// and Max the log scale that Scalable figures may use instead.
type InsightsFigure struct {
	ID       InsightsFigureID     `json:"id"`
	Label    string               `json:"label"`
	Value    string               `json:"value"`
	Change   string               `json:"change"`
	Caption  string               `json:"caption"`
	Share    bool                 `json:"share"`
	Scalable bool                 `json:"scalable"`
	Columns  []InsightsColumnStat `json:"columns"`
	Dots     []InsightsDot        `json:"dots"`
	Min      float64              `json:"min"`
	LogMin   float64              `json:"logMin"`
	Max      float64              `json:"max"`
	Ticks    []InsightsTick       `json:"ticks"`
	LogTicks []InsightsTick       `json:"logTicks"`
	Spread   *InsightsSpread      `json:"spread,omitzero"`
}

// insightsUnit is how a figure's values read: seconds, nanodollars, counts or shares.
type insightsUnit int

const (
	insightsSeconds insightsUnit = iota
	insightsNanos
	insightsCount
	insightsShare
)

// insightsFigureDef is a figure's definition: its value for a run, false when the run has none.
type insightsFigureDef struct {
	id      InsightsFigureID
	label   string
	caption string
	unit    insightsUnit
	value   func(f *insightsFacts) (float64, bool)
}

var insightsEvolutionFigures = []insightsFigureDef{
	{InsightsFigureLead, "Lead time", "median of shipped runs", insightsSeconds, func(f *insightsFacts) (float64, bool) {
		if !insightsShipped(f) {
			return 0, false
		}
		return f.lead, true
	}},
	{InsightsFigureToPR, "Time to first PR", "median of runs with one", insightsSeconds, func(f *insightsFacts) (float64, bool) {
		if f.toPR == nil {
			return 0, false
		}
		return *f.toPR, true
	}},
	{InsightsFigureCost, "Cost per run", "median of priced shipped runs", insightsNanos, func(f *insightsFacts) (float64, bool) {
		if !insightsShipped(f) || f.cost == nil {
			return 0, false
		}
		return *f.cost, true
	}},
	{InsightsFigureVisits, "Stage visits per run", "median", insightsCount, func(f *insightsFacts) (float64, bool) { return float64(f.visits), true }},
	{InsightsFigureTouches, "You stepped in", "median approvals + holds", insightsCount, func(f *insightsFacts) (float64, bool) { return float64(f.approvals + f.holds), true }},
	{InsightsFigureShipRate, "Ship rate", "shipped", insightsShare, func(f *insightsFacts) (float64, bool) {
		if insightsShipped(f) {
			return 1, true
		}
		return 0, true
	}},
}

// insightsShipped is whether the finished run shipped: the only runs the lead time and cost per run
// count. A Done run is finished, so it counts in the ship rate's denominator and nowhere else here.
func insightsShipped(f *insightsFacts) bool {
	return f.run.State == RunStateShipped
}

// insightsValues is the measure's values over facts, sorted.
func insightsValues(facts []*insightsFacts, m insightsFigureDef) []float64 {
	out := []float64{}
	for _, f := range facts {
		if v, ok := m.value(f); ok {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// insightsQuantile is the p-quantile of sorted values, interpolated; nil when there are none.
func insightsQuantile(sorted []float64, p float64) *float64 {
	if len(sorted) == 0 {
		return nil
	}
	at := float64(len(sorted)-1) * p
	low, high := int(math.Floor(at)), int(math.Ceil(at))
	value := sorted[low] + (sorted[high]-sorted[low])*(at-float64(low))
	return &value
}

// insightsMean is the mean of values, nil when there are none.
func insightsMean(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	mean := total / float64(len(values))
	return &mean
}

// insightsShow labels a value of unit, "–" when unknown.
func insightsShow(unit insightsUnit, value *float64) string {
	if value == nil {
		return "–"
	}
	switch unit {
	case insightsSeconds:
		return StatusDuration(*value)
	case insightsNanos:
		return CostDollars(int(math.Round(*value)))
	case insightsShare:
		return insightsPercent(*value)
	}
	if *value >= 10 || *value == math.Trunc(*value) {
		return strconv.Itoa(int(math.Round(*value)))
	}
	return strconv.FormatFloat(math.Round(*value*10)/10, 'f', -1, 64)
}

// insightsStatOf is the figure's statistic of the given values: the mean for a share, else the median.
func insightsStatOf(unit insightsUnit, sorted []float64) *float64 {
	if unit == insightsShare {
		return insightsMean(sorted)
	}
	return insightsQuantile(sorted, 0.5)
}

// insightsChange is recent against before: "↓ −4%" for an amount, "↑ +33 pts" for a share, "–"
// when either is unknown.
func insightsChange(unit insightsUnit, before, recent *float64) string {
	if before == nil || recent == nil || (unit != insightsShare && *before == 0) {
		return "–"
	}
	var change int
	suffix := "%"
	if unit == insightsShare {
		change, suffix = int(math.Round((*recent-*before)*100)), " pts"
	} else {
		change = int(math.Round((*recent / *before - 1) * 100))
	}
	switch {
	case change > 0:
		return "↑ +" + strconv.Itoa(change) + suffix
	case change < 0:
		return "↓ −" + strconv.Itoa(-change) + suffix
	}
	return "→ 0" + suffix
}

// insightsFigures is the six figures over the columns, comparing recent with before. runIndex finds a
// run's place in the view's runs.
func insightsFigures(groups []*insightsColumnGroup, before, recent []*insightsFacts, runIndex map[string]int) []InsightsFigure {
	out := make([]InsightsFigure, 0, len(insightsEvolutionFigures))
	for _, m := range insightsEvolutionFigures {
		b, r := insightsValues(before, m), insightsValues(recent, m)
		figure := InsightsFigure{
			ID: m.id, Label: m.label, Share: m.unit == insightsShare,
			Scalable: m.unit == insightsSeconds || m.unit == insightsNanos,
			Value:    insightsShow(m.unit, insightsStatOf(m.unit, r)),
			Change:   insightsChange(m.unit, insightsStatOf(m.unit, b), insightsStatOf(m.unit, r)),
			Caption:  strconv.Itoa(len(r)) + " recent " + insightsPlural(len(r), "run", "runs") + " · " + m.caption,
			Columns:  []InsightsColumnStat{}, Dots: []InsightsDot{}, Ticks: []InsightsTick{}, LogTicks: []InsightsTick{},
		}
		var all []float64
		for _, group := range groups {
			values := insightsValues(group.facts, m)
			all = append(all, values...)
			stat := InsightsColumnStat{Value: insightsStatOf(m.unit, values), Runs: len(values)}
			if m.unit == insightsShare {
				shipped := 0
				for _, v := range values {
					shipped += int(v)
				}
				stat.Display = strconv.Itoa(shipped) + " of " + strconv.Itoa(len(values)) + " shipped · " + insightsShow(m.unit, stat.Value)
			} else {
				stat.Display = "Median " + insightsShow(m.unit, stat.Value) + " over " + strconv.Itoa(len(values)) + " " + insightsPlural(len(values), "run", "runs")
				if len(values) >= 3 {
					stat.Low, stat.High = insightsQuantile(values, 0.25), insightsQuantile(values, 0.75)
				}
				for _, f := range group.facts {
					if v, ok := m.value(f); ok {
						figure.Dots = append(figure.Dots, InsightsDot{Run: runIndex[f.run.ID], Value: v, Display: insightsShow(m.unit, &v), Partial: m.unit == insightsNanos && !f.priced})
					}
				}
			}
			figure.Columns = append(figure.Columns, stat)
		}
		slices.Sort(all)
		insightsScaleOf(&figure, m.unit, all)
		if m.unit != insightsShare && len(all) > 0 {
			figure.Spread = insightsSpreadOf(figure, m.unit, b, r)
		}
		out = append(out, figure)
	}
	return out
}

// insightsScaleOf sets the figure's bounds and ticks from its sorted values. The linear scale starts
// at zero; the log scale at the smallest positive value, no lower than a minute or a tenth of a cent.
func insightsScaleOf(figure *InsightsFigure, unit insightsUnit, sorted []float64) {
	if unit == insightsShare {
		figure.Max = 1
		for _, v := range []float64{0, 0.25, 0.5, 0.75, 1} {
			figure.Ticks = append(figure.Ticks, InsightsTick{Value: v, Label: insightsPercent(v)})
		}
		return
	}
	if len(sorted) == 0 {
		figure.Max = 1
		return
	}
	figure.Max = max(sorted[len(sorted)-1], 1)
	floor := 1.0
	switch unit {
	case insightsSeconds:
		floor = 60
	case insightsNanos:
		floor = 1_000_000
	}
	figure.LogMin = floor
	for _, v := range sorted {
		if v > 0 {
			figure.LogMin = max(v, floor)
			break
		}
	}
	if figure.LogMin >= figure.Max {
		figure.Max = figure.LogMin * 10
	}
	figure.Ticks = insightsLinearTicks(unit, figure.Max)
	if figure.Scalable {
		figure.LogTicks = insightsLogTicks(unit, figure.LogMin, figure.Max)
	}
}

// insightsAxisLabel labels a tick: "30m", "2h", "3d", "$0.05", "12".
func insightsAxisLabel(unit insightsUnit, value float64) string {
	switch unit {
	case insightsSeconds:
		switch {
		case value < 3600:
			return strconv.Itoa(int(math.Round(value/60))) + "m"
		case value < 48*3600:
			return strconv.FormatFloat(math.Round(value/360)/10, 'f', -1, 64) + "h"
		}
		return strconv.FormatFloat(math.Round(value/8640)/10, 'f', -1, 64) + "d"
	case insightsNanos:
		return "$" + strconv.FormatFloat(value/1e9, 'f', -1, 64)
	}
	return strconv.FormatFloat(math.Round(value*10)/10, 'f', -1, 64)
}

// insightsLinearTicks is about five evenly spaced ticks from zero past top, on steps that read well
// in the unit.
func insightsLinearTicks(unit insightsUnit, top float64) []InsightsTick {
	var steps []float64
	if unit == insightsSeconds {
		steps = []float64{60, 300, 600, 1800, 3600, 2 * 3600, 5 * 3600, 10 * 3600, 24 * 3600, 2 * 86400, 5 * 86400, 10 * 86400, 30 * 86400}
	} else {
		base := 0.1
		if unit == insightsNanos {
			base = 1e7
		}
		for magnitude := base; len(steps) == 0 || magnitude < top*10; magnitude *= 10 {
			steps = append(steps, magnitude, 2*magnitude, 5*magnitude)
		}
	}
	step := steps[len(steps)-1]
	for _, s := range steps {
		if top/s <= 5 {
			step = s
			break
		}
	}
	out := []InsightsTick{}
	for i := 0; float64(i)*step <= top+step*0.001; i++ {
		v := float64(i) * step
		out = append(out, InsightsTick{Value: v, Label: insightsAxisLabel(unit, v)})
	}
	return out
}

// insightsLogTicks is the ticks between low and high on a log scale: 1, 2 and 5 of each power of ten,
// or for time the minutes, hours and days that read well; only every other one past seven.
func insightsLogTicks(unit insightsUnit, low, high float64) []InsightsTick {
	var candidates []float64
	if unit == insightsSeconds {
		candidates = []float64{60, 120, 300, 600, 1800, 3600, 2 * 3600, 5 * 3600, 10 * 3600, 86400, 2 * 86400, 5 * 86400, 10 * 86400, 30 * 86400}
	} else {
		for magnitude := 1e6; magnitude <= high*10; magnitude *= 10 {
			candidates = append(candidates, magnitude, 2*magnitude, 5*magnitude)
		}
	}
	var values []float64
	for _, v := range candidates {
		if v >= low*0.999 && v <= high*1.001 {
			values = append(values, v)
		}
	}
	// A narrow range between two of those reads from its own ends.
	if len(values) < 2 {
		values = []float64{low, high}
	}
	out := []InsightsTick{}
	for i, v := range values {
		if len(values) > 7 && i%2 == 1 {
			continue
		}
		out = append(out, InsightsTick{Value: v, Label: insightsAxisLabel(unit, v)})
	}
	return out
}

// insightsSpreadOf is how before and recent are spread over the figure's scale.
func insightsSpreadOf(figure InsightsFigure, unit insightsUnit, before, recent []float64) *InsightsSpread {
	spread := &InsightsSpread{
		Before: before, Recent: recent, Bins: []InsightsBin{}, LogBins: []InsightsBin{},
		Headers: []string{"10th", "Median", "75th", "90th", "Slowest", "Mean"},
	}
	low := 0.0
	if len(before)+len(recent) > 0 {
		low = math.Inf(1)
		for _, values := range [][]float64{before, recent} {
			if len(values) > 0 {
				low = min(low, values[0])
			}
		}
	}
	if unit == insightsCount {
		count := int(figure.Max-low) + 1
		width := math.Ceil(float64(count) / 24)
		for from := low; from <= figure.Max; from += width {
			label := insightsAxisLabel(unit, from)
			if width > 1 {
				label += "–" + insightsAxisLabel(unit, from+width-1)
			}
			spread.Bins = append(spread.Bins, InsightsBin{From: from, To: from + width, Label: label})
		}
	} else {
		spread.Bins = insightsEdges(unit, low, figure.Max, false)
		spread.LogBins = insightsEdges(unit, figure.LogMin, figure.Max, true)
	}
	insightsFill(spread.Bins, before, recent)
	insightsFill(spread.LogBins, before, recent)
	row := func(label string, values []float64) InsightsSpreadRow {
		cells := []string{}
		for _, p := range []float64{0.1, 0.5, 0.75, 0.9} {
			cells = append(cells, insightsShow(unit, insightsQuantile(values, p)))
		}
		var slowest *float64
		if len(values) > 0 {
			slowest = &values[len(values)-1]
		}
		cells = append(cells, insightsShow(unit, slowest), insightsShow(unit, insightsMean(values)))
		return InsightsSpreadRow{Label: label, Runs: len(values), Cells: cells}
	}
	spread.Rows = []InsightsSpreadRow{row("Before", before), row("Recent", recent)}
	return spread
}

// insightsSpreadBins is how many ranges a histogram of time or cost has.
const insightsSpreadBins = 18

// insightsEdges is insightsSpreadBins equal ranges from low to high, on a log scale with log.
func insightsEdges(unit insightsUnit, low, high float64, log bool) []InsightsBin {
	if high <= low {
		high = low + 1
	}
	// Each edge is rounded to a thousandth, so the last bits, where arm64 and amd64 differ (a fused
	// multiply-add, math.Pow), never reach the bins or the wire.
	edge := func(i int) float64 {
		t := float64(i) / insightsSpreadBins
		if log {
			return math.Round(math.Pow(10, math.Log10(low)+(math.Log10(high)-math.Log10(low))*t)*1000) / 1000
		}
		return math.Round((low+(high-low)*t)*1000) / 1000
	}
	out := make([]InsightsBin, 0, insightsSpreadBins)
	for i := range insightsSpreadBins {
		from, to := edge(i), edge(i+1)
		out = append(out, InsightsBin{From: from, To: to, Label: insightsShow(unit, &from) + " to " + insightsShow(unit, &to)})
	}
	return out
}

// insightsFill sets each bin's share of before and of recent; a value past either end counts in the
// bin at that end.
func insightsFill(bins []InsightsBin, before, recent []float64) {
	if len(bins) == 0 {
		return
	}
	share := func(values []float64, set func(*InsightsBin, float64)) {
		counts := make([]int, len(bins))
		for _, v := range values {
			at := len(bins) - 1
			for i := range bins {
				if v < bins[i].To {
					at = i
					break
				}
			}
			counts[at]++
		}
		for i := range bins {
			if len(values) > 0 {
				set(&bins[i], float64(counts[i])/float64(len(values)))
			}
		}
	}
	share(before, func(b *InsightsBin, v float64) { b.Before = v })
	share(recent, func(b *InsightsBin, v float64) { b.Recent = v })
}
