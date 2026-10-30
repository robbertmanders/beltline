// The Evolution view of Insights: one belt's finished runs over its versions or weeks, so you can see
// whether it gets faster, cheaper, ships more and needs you less. Runs fall into columns, one per
// version (small versions merged until a column has InsightsEvolutionMinRuns) or one per week they
// finished in; the columns from From on are "recent" and the ones before it "before". A run on a
// shadow version counts towards the live version its test shadowed. Like the rest of Insights it
// reads only the runs.

package core

import (
	"slices"
	"strconv"
	"time"
)

// InsightsEvolutionAxis is what the columns are: belt versions or weeks.
type InsightsEvolutionAxis string

const (
	InsightsEvolutionByVersion InsightsEvolutionAxis = "version"
	InsightsEvolutionByWeek    InsightsEvolutionAxis = "week"
)

// AllInsightsEvolutionAxis lists every InsightsEvolutionAxis in declaration order (CaseIterable).
var AllInsightsEvolutionAxis = []InsightsEvolutionAxis{InsightsEvolutionByVersion, InsightsEvolutionByWeek}

// InsightsEvolutionMinRuns is how many runs a merged version column gathers before it closes.
const InsightsEvolutionMinRuns = 5

// InsightsEvolutionQuery is what the Evolution view asks for: a belt, its columns, and From, the
// first recent column; zero picks the column where the last third of the runs starts.
type InsightsEvolutionQuery struct {
	Belt string
	Axis InsightsEvolutionAxis
	// Separate keeps every version in its own column instead of merging small ones.
	Separate     bool
	From         int
	ShowArchived bool
}

// InsightsColumn is one column: a version, a range of merged versions, or a week. Changes counts the
// stage changes since the column before, which the axis marks; ChangeCaption says what they were
// counted against.
type InsightsColumn struct {
	Label         string `json:"label"`
	Runs          int    `json:"runs"`
	Changes       int    `json:"changes"`
	ChangeCaption string `json:"changeCaption"`
}

// InsightsEvolutionOutcome is how a finished run ended: Done, Shipped or Dropped.
type InsightsEvolutionOutcome string

const (
	InsightsEvolutionOutcomeDone    InsightsEvolutionOutcome = "done"
	InsightsEvolutionOutcomeShipped InsightsEvolutionOutcome = "shipped"
	InsightsEvolutionOutcomeDropped InsightsEvolutionOutcome = "dropped"
)

// AllInsightsEvolutionOutcome lists every InsightsEvolutionOutcome in declaration order (CaseIterable).
var AllInsightsEvolutionOutcome = []InsightsEvolutionOutcome{InsightsEvolutionOutcomeDone, InsightsEvolutionOutcomeShipped, InsightsEvolutionOutcomeDropped}

// InsightsEvolutionRun is a run the charts draw as a dot, with what its tooltip says.
type InsightsEvolutionRun struct {
	ID       string                   `json:"id"`
	Ref      string                   `json:"ref"`
	Title    string                   `json:"title"`
	Version  int                      `json:"version"`
	Outcome  InsightsEvolutionOutcome `json:"outcome"`
	Finished string                   `json:"finished"`
	Column   int                      `json:"column"`
}

// insightsEvolutionOutcomeOf is how run ended, for the Evolution view.
func insightsEvolutionOutcomeOf(run *Run) InsightsEvolutionOutcome {
	switch run.State {
	case RunStateShipped:
		return InsightsEvolutionOutcomeShipped
	case RunStateDone:
		return InsightsEvolutionOutcomeDone
	}
	return InsightsEvolutionOutcomeDropped
}

// InsightsEvolutionView is the Evolution view of one belt.
type InsightsEvolutionView struct {
	Belt     string                `json:"belt"`
	Belts    []InsightsBelt        `json:"belts"`
	Title    string                `json:"title"`
	Caption  string                `json:"caption"`
	Axis     InsightsEvolutionAxis `json:"axis"`
	Separate bool                  `json:"separate"`
	Columns  []InsightsColumn      `json:"columns"`
	// From is the first recent column; zero when there are too few columns to compare.
	From int `json:"from"`
	// Split says which columns and how many runs are before and recent.
	Split     string                 `json:"split"`
	Runs      []InsightsEvolutionRun `json:"runs"`
	Figures   []InsightsFigure       `json:"figures"`
	Time      InsightsStack          `json:"time"`
	Touches   InsightsStack          `json:"touches"`
	SendsBack InsightsGrid           `json:"sendsBack"`
	Cost      InsightsStack          `json:"cost"`
	Endings   InsightsEndings        `json:"endings"`
}

// insightsFacts is what the view needs from one finished run.
type insightsFacts struct {
	run     *Run
	version int
	ended   time.Time
	lead    float64
	toPR    *float64
	// cost is nil when no agent attempt reported a dollar figure; priced is whether every one did.
	cost      *float64
	priced    bool
	visits    int
	approvals int
	holds     int
	afk       int
	agent     float64
	approve   float64
	hold      float64
	watch     float64
	sends     []string
	retries   int
	costBy    map[string]float64
	dropAt    string
	dropWhy   string
}

// insightsHeld reports whether the attempt ended by holding or failing the run for you.
func insightsHeld(attempt *Attempt) bool {
	route := ValueOf(attempt.Route)
	return len(route) >= 4 && (route[:4] == "Held" || (len(route) >= 6 && route[:6] == "Failed"))
}

// insightsStageLabel is the stage's name, with its route when another stage shares the name.
func insightsStageLabel(run *Run, index int) string {
	stage := &run.Stages[index]
	shared := false
	for i := range run.Stages {
		if i != index && run.Stages[i].Name == stage.Name {
			shared = true
		}
	}
	if !shared || stage.Route == nil {
		return stage.Name
	}
	for _, fork := range run.Forks {
		for _, route := range fork.Routes {
			if route.ID == *stage.Route {
				return stage.Name + " (" + route.Name + ")"
			}
		}
	}
	return stage.Name
}

// insightsFactsOf is run's facts, or false when it has not finished or never started a stage.
// version is the version it counts towards.
func insightsFactsOf(run *Run, version int) (insightsFacts, bool) {
	if run.State.IsActive() || len(run.Attempts) == 0 {
		return insightsFacts{}, false
	}
	f := insightsFacts{run: run, version: version, priced: true, costBy: map[string]float64{}}
	f.ended = run.UpdatedAt.Time
	endedSet := false
	order := make([]int, len(run.Attempts))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return run.Attempts[a].StartedAt.Compare(run.Attempts[b].StartedAt.Time) })
	type legAt struct {
		index int
		end   time.Time
		held  bool
	}
	previous := map[string]legAt{}
	anyAgent := false
	var cost float64
	for _, i := range order {
		attempt := &run.Attempts[i]
		start := attempt.StartedAt.Time
		end := start
		if attempt.EndedAt != nil {
			end = attempt.EndedAt.Time
			if !endedSet || end.After(f.ended) {
				f.ended, endedSet = end, true
			}
		}
		index := run.StageIndexOf(attempt.StageID)
		leg := ValueOr(attempt.Leg, "")
		prev, seen := previous[leg]
		if seen && prev.held && start.After(prev.end) {
			f.hold += insightsActive(run, prev.end, start)
		}
		if index >= 0 {
			label := insightsStageLabel(run, index)
			switch {
			case !seen || prev.index != index:
				f.visits++
				if seen && prev.index >= 0 && index < prev.index {
					f.sends = append(f.sends, insightsStageLabel(run, prev.index)+" → "+label)
				}
			default:
				f.retries++
			}
			seconds := insightsActive(run, start, end)
			switch run.Stages[index].Kind() {
			case StageKindAgent:
				f.agent += seconds
			case StageKindGate:
				f.approve += seconds
				// Writing a prompt is time you give, not an approval.
				if run.Stages[index].Type != StageTypePrompt && attempt.AfkDecision == nil && attempt.EndedAt != nil && !insightsStopped(attempt) {
					f.approvals++
				}
			case StageKindWatch:
				f.watch += seconds
			}
			if costIsAgent(attempt, run) {
				anyAgent = true
				if attempt.Usage != nil && attempt.Usage.CostNanos != nil {
					value := float64(*attempt.Usage.CostNanos)
					cost += value
					f.costBy[label] += value
				} else {
					f.priced = false
				}
			}
		}
		if attempt.AfkDecision != nil {
			f.afk++
		}
		held := insightsHeld(attempt)
		if held && attempt.AfkDecision == nil {
			f.holds++
		}
		previous[leg] = legAt{index: index, end: end, held: held}
	}
	f.lead = insightsActive(run, run.CreatedAt.Time, f.ended)
	if run.FirstPullRequestAt != nil {
		seconds := insightsActive(run, run.CreatedAt.Time, run.FirstPullRequestAt.Time)
		f.toPR = &seconds
	}
	if anyAgent && len(f.costBy) > 0 {
		f.cost = &cost
	}
	f.priced = f.priced && anyAgent
	if run.State == RunStateDropped {
		if index := run.StageIndexOf(run.StageID); index >= 0 {
			f.dropAt = insightsStageLabel(run, index)
		}
		f.dropWhy = ValueOr(run.Reason, "")
	}
	return f, true
}

// insightsLiveVersion is the version run counts towards: its own, or for a run on a shadow version
// the live version its test shadowed, when the test is known.
func insightsLiveVersion(run *Run, tests []ShadowTest) int {
	if run.OnShadow() {
		for _, test := range tests {
			if test.ID == run.Shadow.Test {
				return test.Live
			}
		}
	}
	return run.BeltVersion
}

// insightsColumnGroup is one column while it is built.
type insightsColumnGroup struct {
	label    string
	from, to int
	facts    []*insightsFacts
}

// InsightsEvolution builds the Evolution view for query from runs and the library's belts; weeks are
// counted in loc.
func InsightsEvolution(runs []Run, library []InsightsLibraryBelt, query InsightsEvolutionQuery, loc *time.Location) InsightsEvolutionView {
	if loc == nil {
		loc = time.Local
	}
	axis := query.Axis
	if !slices.Contains(AllInsightsEvolutionAxis, axis) {
		axis = InsightsEvolutionByVersion
	}
	view := InsightsEvolutionView{
		Belt: query.Belt, Belts: insightsPickerBelts(runs, library, InsightsQuery{Belt: query.Belt, ShowArchived: query.ShowArchived}),
		Title: query.Belt, Axis: axis, Separate: query.Separate, Columns: []InsightsColumn{}, Runs: []InsightsEvolutionRun{}, Figures: []InsightsFigure{},
	}
	for _, belt := range view.Belts {
		if belt.ID == query.Belt {
			view.Title = belt.Label
		}
	}
	var tests []ShadowTest
	for _, belt := range library {
		if belt.ID == query.Belt {
			tests = belt.ShadowTests
		}
	}
	var facts []*insightsFacts
	for i := range runs {
		run := &runs[i]
		if run.BeltID != query.Belt {
			continue
		}
		if f, ok := insightsFactsOf(run, insightsLiveVersion(run, tests)); ok {
			facts = append(facts, &f)
		}
	}
	slices.SortStableFunc(facts, func(a, b *insightsFacts) int { return a.ended.Compare(b.ended) })
	var groups []*insightsColumnGroup
	if axis == InsightsEvolutionByWeek {
		groups = insightsWeekColumns(facts, loc)
	} else {
		groups = insightsVersionColumns(facts, !query.Separate)
	}
	view.From = insightsFrom(groups, len(facts), query.From)
	versions := map[int]int{}
	for c, group := range groups {
		for _, f := range group.facts {
			versions[f.version]++
			view.Runs = append(view.Runs, InsightsEvolutionRun{
				ID: f.run.ID, Ref: insightsRef(f.run), Title: f.run.Subject.Title, Version: f.version,
				Outcome: insightsEvolutionOutcomeOf(f.run), Finished: f.ended.In(loc).Format("Jan 2"), Column: c,
			})
		}
	}
	view.Columns = insightsColumnsOf(groups, facts)
	if len(facts) > 0 {
		low, high := facts[0].version, facts[0].version
		for v := range versions {
			low, high = min(low, v), max(high, v)
		}
		span := "v" + strconv.Itoa(low)
		if high != low {
			span += "–v" + strconv.Itoa(high)
		}
		view.Caption = costGrouped(len(facts)) + " finished " + insightsPlural(len(facts), "run", "runs") + " · " + span
	} else {
		view.Caption = "No finished runs"
	}
	var before, recent []*insightsFacts
	for c, group := range groups {
		if view.From > 0 && c < view.From {
			before = append(before, group.facts...)
		} else {
			recent = append(recent, group.facts...)
		}
	}
	if view.From > 0 {
		view.Split = "Before is " + insightsSpan(groups[:view.From]) + " (" + strconv.Itoa(len(before)) + " " + insightsPlural(len(before), "run", "runs") +
			"); recent is " + insightsSpan(groups[view.From:]) + " (" + strconv.Itoa(len(recent)) + " " + insightsPlural(len(recent), "run", "runs") + ")."
	} else {
		view.Split = "Too few runs on this belt to compare."
	}
	runIndex := map[string]int{}
	for i, r := range view.Runs {
		runIndex[r.ID] = i
	}
	view.Figures = insightsFigures(groups, before, recent, runIndex)
	view.Time = insightsTimeStack(groups)
	view.Touches = insightsTouchStack(groups)
	view.SendsBack = insightsSendsBack(groups)
	view.Cost = insightsCostStack(groups)
	view.Endings = insightsEndingsOf(before, recent)
	return view
}

// insightsRef is how a run's subject is referred to: "#399", "KAN-12", or nothing.
func insightsRef(run *Run) string {
	if run.Subject.Key != "" {
		return run.Subject.Key
	}
	if run.Subject.Number > 0 {
		return "#" + strconv.Itoa(run.Subject.Number)
	}
	return ""
}

// insightsVersionColumns is one column per version, or with merge, consecutive versions merged until
// a column has InsightsEvolutionMinRuns runs; a short last column joins the one before it.
func insightsVersionColumns(facts []*insightsFacts, merge bool) []*insightsColumnGroup {
	byVersion := map[int][]*insightsFacts{}
	for _, f := range facts {
		byVersion[f.version] = append(byVersion[f.version], f)
	}
	versions := make([]int, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
	}
	slices.Sort(versions)
	var out []*insightsColumnGroup
	var open *insightsColumnGroup
	for _, v := range versions {
		if open == nil {
			open = &insightsColumnGroup{from: v}
		}
		open.to = v
		open.facts = append(open.facts, byVersion[v]...)
		if !merge || len(open.facts) >= InsightsEvolutionMinRuns {
			out, open = append(out, open), nil
		}
	}
	if open != nil {
		if len(out) > 0 {
			last := out[len(out)-1]
			last.to = open.to
			last.facts = append(last.facts, open.facts...)
		} else {
			out = append(out, open)
		}
	}
	for _, group := range out {
		group.label = "v" + strconv.Itoa(group.from)
		if group.to != group.from {
			group.label += "–" + strconv.Itoa(group.to)
		}
	}
	return out
}

// insightsWeekColumns is one column per week, Monday to Sunday in loc, that a run finished in.
func insightsWeekColumns(facts []*insightsFacts, loc *time.Location) []*insightsColumnGroup {
	var out []*insightsColumnGroup
	var current time.Time
	for _, f := range facts {
		day := f.ended.In(loc)
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		start = start.AddDate(0, 0, -((int(start.Weekday()) + 6) % 7))
		if len(out) == 0 || !start.Equal(current) {
			current = start
			out = append(out, &insightsColumnGroup{label: start.Format("Jan 2")})
		}
		out[len(out)-1].facts = append(out[len(out)-1].facts, f)
	}
	return out
}

// insightsFrom is the first recent column: asked, when it is a column after the first, or the
// column where the last third of the runs starts. Zero when there are fewer than two columns.
func insightsFrom(groups []*insightsColumnGroup, total, asked int) int {
	if len(groups) < 2 {
		return 0
	}
	if asked >= 1 && asked < len(groups) {
		return asked
	}
	seen := 0
	for c, group := range groups {
		seen += len(group.facts)
		if 3*seen >= 2*total {
			return min(max(1, c+1), len(groups)-1)
		}
	}
	return len(groups) - 1
}

// insightsSpan names a range of columns: "v2 to v62", or "Sep 29 to Oct 13".
func insightsSpan(groups []*insightsColumnGroup) string {
	first, last := groups[0], groups[len(groups)-1]
	start, end := first.label, last.label
	if first.to != 0 {
		start, end = "v"+strconv.Itoa(first.from), "v"+strconv.Itoa(last.to)
	}
	if start == end {
		return start
	}
	return start + " to " + end
}

// insightsColumnsOf is the columns with their run counts and the stage changes since the column
// before, compared on the newest live run of each.
func insightsColumnsOf(groups []*insightsColumnGroup, facts []*insightsFacts) []InsightsColumn {
	out := make([]InsightsColumn, 0, len(groups))
	var previous *Run
	for _, group := range groups {
		column := InsightsColumn{Label: group.label, Runs: len(group.facts)}
		var newest *Run
		for _, f := range group.facts {
			if f.run.OnShadow() {
				continue
			}
			if newest == nil || f.run.BeltVersion > newest.BeltVersion || (f.run.BeltVersion == newest.BeltVersion && f.run.CreatedAt.After(newest.CreatedAt.Time)) {
				newest = f.run
			}
		}
		if newest != nil && previous != nil && newest.BeltVersion != previous.BeltVersion {
			column.Changes = insightsStageChanges(previous.Stages, newest.Stages)
			if column.Changes > 0 {
				column.ChangeCaption = strconv.Itoa(column.Changes) + " stage " + insightsPlural(column.Changes, "change", "changes") +
					" from v" + strconv.Itoa(previous.BeltVersion) + " to v" + strconv.Itoa(newest.BeltVersion)
			}
		}
		if newest != nil {
			previous = newest
		}
		out = append(out, column)
	}
	return out
}

// insightsStageChanges counts the stages added or removed between two versions, and each change to
// a kept stage's model, thinking, agent version, brief or outcomes.
func insightsStageChanges(before, after []Stage) int {
	byID := map[string]*Stage{}
	for i := range before {
		byID[before[i].ID] = &before[i]
	}
	changes := 0
	kept := map[string]bool{}
	for i := range after {
		stage := &after[i]
		old, ok := byID[stage.ID]
		if !ok {
			changes++
			continue
		}
		kept[stage.ID] = true
		if old.Model != stage.Model {
			changes++
		}
		if ValueOr(old.Thinking, "") != ValueOr(stage.Thinking, "") {
			changes++
		}
		if (old.Agent == nil) != (stage.Agent == nil) || (old.Agent != nil && *old.Agent != *stage.Agent) {
			changes++
		}
		if old.Brief != stage.Brief {
			changes++
		}
		if !slices.EqualFunc(old.Outcomes, stage.Outcomes, insightsSameOutcome) {
			changes++
		}
	}
	for id := range byID {
		if !kept[id] {
			changes++
		}
	}
	return changes
}

// insightsSameOutcome reports whether two outcomes read the same: name, when, tone and what follows.
func insightsSameOutcome(a, b Outcome) bool {
	return a.Name == b.Name && a.When == b.When && a.Tone == b.Tone && a.Then.Action == b.Then.Action &&
		slices.Equal(a.Then.To, b.Then.To) && slices.Equal(a.Then.Routes, b.Then.Routes) && a.Then.WithOutput == b.Then.WithOutput &&
		ValueOr(a.Then.Max, -1) == ValueOr(b.Then.Max, -1) && a.Then.Comment == b.Then.Comment
}
