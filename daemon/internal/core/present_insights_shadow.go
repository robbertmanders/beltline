// Insights for a shadow test: the live and shadow versions side by side over the runs that
// started during the test and have finished, with their sample sizes. The four figures and
// the stages both versions have are compared; a stage only one has is listed on its own.

package core

import (
	"math"
	"slices"
	"strconv"
	"time"
)

// InsightsShadowValue is one version's side of a figure: its value, its sample and how full
// its bar is, 0 to 1.
type InsightsShadowValue struct {
	Value   string  `json:"value"`
	Caption string  `json:"caption"`
	Bar     float64 `json:"bar"`
}

// InsightsShadowFigure is one of the four figures for both versions. Delta is the shadow
// against live, toned better or worse in the figure's own direction.
type InsightsShadowFigure struct {
	ID     InsightsKpiID       `json:"id"`
	Label  string              `json:"label"`
	Live   InsightsShadowValue `json:"live"`
	Shadow InsightsShadowValue `json:"shadow"`
	Delta  InsightsDelta       `json:"delta"`
}

// InsightsShadowTones says, for each stage figure, which way the shadow differs from live.
type InsightsShadowTones struct {
	VisitsPerRun InsightsDeltaTone `json:"visitsPerRun"`
	MedianTime   InsightsDeltaTone `json:"medianTime"`
	FirstTry     InsightsDeltaTone `json:"firstTry"`
	Tries        InsightsDeltaTone `json:"tries"`
	Cost         InsightsDeltaTone `json:"cost"`
}

// InsightsShadowStage is a stage both versions have, live's figures beside the shadow's.
// Changed says the shadow changed the stage.
type InsightsShadowStage struct {
	Key       string              `json:"key"`
	Name      string              `json:"name"`
	KindLabel string              `json:"kindLabel"`
	Changed   bool                `json:"changed"`
	Live      InsightsFigures     `json:"live"`
	Shadow    InsightsFigures     `json:"shadow"`
	Tones     InsightsShadowTones `json:"tones"`
}

// InsightsShadowArm is one version of the test: its label, such as "Shadow v2", and its runs.
type InsightsShadowArm struct {
	Label    string `json:"label"`
	Finished int    `json:"finished"`
	Going    int    `json:"going"`
	Caption  string `json:"caption"`
}

// InsightsShadow is one shadow test compared.
type InsightsShadow struct {
	Test    string                 `json:"test"`
	Running bool                   `json:"running"`
	Result  ShadowResult           `json:"result,omitzero"`
	Dates   string                 `json:"dates"`
	Caption string                 `json:"caption"`
	Live    InsightsShadowArm      `json:"live"`
	Shadow  InsightsShadowArm      `json:"shadow"`
	Figures []InsightsShadowFigure `json:"figures"`
	Stages  []InsightsShadowStage  `json:"stages"`
	// OnlyShadow and OnlyLive are the stages one version has and the other does not.
	OnlyShadow []InsightsStage `json:"onlyShadow"`
	OnlyLive   []InsightsStage `json:"onlyLive"`
}

// insightsShadowChoices are the version picker's entries for the belt's shadow tests, newest first.
func insightsShadowChoices(library []InsightsLibraryBelt, beltID string, loc *time.Location) []InsightsVersionChoice {
	out := []InsightsVersionChoice{}
	for _, belt := range library {
		if belt.ID != beltID {
			continue
		}
		for i := len(belt.ShadowTests) - 1; i >= 0; i-- {
			test := belt.ShadowTests[i]
			label := "Shadow test · v" + strconv.Itoa(test.Live) + " vs v" + strconv.Itoa(test.Shadow)
			if !test.Running() {
				label = "Shadow test · " + insightsShadowDates(test, loc) + " · " + string(test.Result)
			}
			out = append(out, InsightsVersionChoice{Value: 0, Shadow: test.ID, Label: label})
		}
	}
	return out
}

// insightsShadowDates is when the test ran, such as "Oct 2–16", or "since Oct 2" while it runs.
func insightsShadowDates(test ShadowTest, loc *time.Location) string {
	start := test.StartedAt.In(loc)
	if test.EndedAt == nil {
		return "since " + start.Format("Jan 2")
	}
	end := test.EndedAt.In(loc)
	if end.Month() == start.Month() && end.Year() == start.Year() {
		return start.Format("Jan 2") + "–" + end.Format("2")
	}
	return start.Format("Jan 2") + " – " + end.Format("Jan 2")
}

// insightsShadowTest finds the belt's test with id.
func insightsShadowTest(library []InsightsLibraryBelt, beltID, id string) *ShadowTest {
	for _, belt := range library {
		if belt.ID != beltID {
			continue
		}
		for i := range belt.ShadowTests {
			if belt.ShadowTests[i].ID == id {
				test := belt.ShadowTests[i]
				return &test
			}
		}
	}
	return nil
}

// insightsShadowOf compares test's two versions over runs: only runs of the belt that started
// during the test count, and only once they have finished.
func insightsShadowOf(runs []Run, beltID string, test ShadowTest, loc *time.Location) InsightsShadow {
	scopes := map[ShadowArm]*insightsScope{ShadowArmLive: {}, ShadowArmShadow: {}}
	going := map[ShadowArm]int{}
	newest := map[ShadowArm]*Run{}
	versions := map[ShadowArm][]int{}
	for i := range runs {
		run := &runs[i]
		if run.BeltID != beltID || run.Shadow == nil || run.Shadow.Test != test.ID {
			continue
		}
		arm := run.Shadow.Arm
		if !slices.Contains(versions[arm], run.BeltVersion) {
			versions[arm] = append(versions[arm], run.BeltVersion)
		}
		if newest[arm] == nil || run.CreatedAt.After(newest[arm].CreatedAt.Time) {
			newest[arm] = run
		}
		if run.State.IsActive() {
			going[arm]++
			continue
		}
		scopes[arm].add(run, time.Time{})
	}
	arm := func(which ShadowArm, fallback int) InsightsShadowArm {
		list := versions[which]
		if len(list) == 0 {
			list = []int{fallback}
		}
		slices.Sort(list)
		label := "v" + strconv.Itoa(list[len(list)-1])
		if len(list) > 1 {
			label = "v" + strconv.Itoa(list[0]) + "–" + strconv.Itoa(list[len(list)-1])
		}
		name := "Live "
		if which == ShadowArmShadow {
			name = "Shadow "
		}
		finished := len(scopes[which].finished)
		return InsightsShadowArm{
			Label: name + label, Finished: finished, Going: going[which],
			Caption: strconv.Itoa(finished) + " finished · " + strconv.Itoa(going[which]) + " going",
		}
	}
	view := InsightsShadow{
		Test: test.ID, Running: test.Running(), Result: test.Result,
		Dates:   "Runs started " + insightsShadowDates(test, loc),
		Caption: "Only runs that started during the test count, and only once they finish. Each new run took whichever version had fewer runs.",
		Live:    arm(ShadowArmLive, test.Live), Shadow: arm(ShadowArmShadow, test.Shadow),
		Figures: insightsShadowFigures(scopes[ShadowArmLive], scopes[ShadowArmShadow]),
	}
	var liveStages, shadowStages []Stage
	if newest[ShadowArmLive] != nil {
		liveStages = newest[ShadowArmLive].Stages
	}
	if newest[ShadowArmShadow] != nil {
		shadowStages = newest[ShadowArmShadow].Stages
	}
	view.Stages, view.OnlyLive, view.OnlyShadow = insightsShadowStages(scopes[ShadowArmLive], scopes[ShadowArmShadow], liveStages, shadowStages)
	return view
}

// insightsShadowFigures is the four figures for both versions.
func insightsShadowFigures(live, shadow *insightsScope) []InsightsShadowFigure {
	type side struct {
		value   *float64
		caption string
	}
	sides := func(s *insightsScope) [4]side {
		firsts := s.agentFirsts()
		shipped, priced := s.shipped()
		var shipShare *float64
		if len(s.finished) > 0 {
			share := float64(len(shipped)) / float64(len(s.finished))
			shipShare = &share
		}
		return [4]side{
			{insightsPassShare(firsts), strconv.Itoa(insightsPassRate(firsts).Count) + " of " + strconv.Itoa(len(firsts)) + " " + insightsPlural(len(firsts), "visit", "visits")},
			{shipShare, strconv.Itoa(len(shipped)) + " of " + strconv.Itoa(len(s.finished)) + " " + insightsPlural(len(s.finished), "run", "runs")},
			{insightsLeadLine.value(shipped), "Median of " + strconv.Itoa(len(shipped)) + " shipped"},
			{insightsCostLine.value(priced), strconv.Itoa(len(priced)) + " priced " + insightsPlural(len(priced), "run", "runs")},
		}
	}
	l, s := sides(live), sides(shadow)
	specs := []struct {
		id      InsightsKpiID
		label   string
		share   bool
		lower   bool
		display func(float64) string
	}{
		{InsightsKpiFirstTry, "First try", true, false, insightsPercent},
		{InsightsKpiShipRate, "Ship rate", true, false, insightsPercent},
		{InsightsKpiLeadTime, "Lead time", false, true, StatusDuration},
		{InsightsKpiCost, "Cost per run", false, true, func(v float64) string { return insightsDollars(&v) }},
	}
	out := make([]InsightsShadowFigure, 0, len(specs))
	for i, spec := range specs {
		top := 1.0
		if !spec.share {
			top = 0
			for _, v := range []*float64{l[i].value, s[i].value} {
				if v != nil && *v > top {
					top = *v
				}
			}
		}
		value := func(sd side) InsightsShadowValue {
			out := InsightsShadowValue{Value: "–", Caption: sd.caption}
			if sd.value != nil {
				out.Value = spec.display(*sd.value)
				if top > 0 {
					out.Bar = *sd.value / top
				}
			}
			return out
		}
		out = append(out, InsightsShadowFigure{
			ID: spec.id, Label: spec.label, Live: value(l[i]), Shadow: value(s[i]),
			Delta: insightsShadowDelta(l[i].value, s[i].value, spec.share, spec.lower, spec.display),
		})
	}
	return out
}

// insightsShadowDelta is the shadow's value against live's: points for a share, the difference
// in the figure's own unit otherwise. A change under 5 points, or under 5%, is flat.
func insightsShadowDelta(live, shadow *float64, share, lower bool, display func(float64) string) InsightsDelta {
	if live == nil || shadow == nil {
		return InsightsDelta{Label: "–", Tone: InsightsDeltaNone, Caption: "Nothing to compare yet"}
	}
	diff := *shadow - *live
	sign := "±"
	if diff > 0 {
		sign = "+"
	} else if diff < 0 {
		sign = "−"
	}
	var label string
	var flat bool
	if share {
		points := int(math.Round(100 * math.Abs(diff)))
		label, flat = sign+strconv.Itoa(points)+" pts", points < 5
		if points == 0 {
			sign = "±"
			label = sign + "0 pts"
		}
	} else {
		label = sign + display(math.Abs(diff))
		flat = *live == 0 || math.Abs(diff)/math.Abs(*live) < 0.05
	}
	tone := InsightsDeltaFlat
	if !flat {
		tone = InsightsDeltaWorse
		if (diff > 0) != lower {
			tone = InsightsDeltaBetter
		}
	}
	return InsightsDelta{Label: label, Tone: tone, Caption: "Shadow against live"}
}

// insightsShadowStages matches the two versions' stages by id: the stages both have in live's
// order, then those only live has and those only the shadow has, each in its version's order.
func insightsShadowStages(live, shadow *insightsScope, liveStages, shadowStages []Stage) (both []InsightsShadowStage, onlyLive, onlyShadow []InsightsStage) {
	byID := func(stages []Stage) map[string]Stage {
		out := map[string]Stage{}
		for _, stage := range stages {
			out[stage.ID] = stage
		}
		return out
	}
	attemptsAt := func(s *insightsScope) map[string][]insightsAttempt {
		out := map[string][]insightsAttempt{}
		for _, item := range s.attempts {
			out[item.stage.ID] = append(out[item.stage.ID], item)
		}
		return out
	}
	liveByID, shadowByID := byID(liveStages), byID(shadowStages)
	liveAt, shadowAt := attemptsAt(live), attemptsAt(shadow)
	both = []InsightsShadowStage{}
	for _, stage := range liveStages {
		other, ok := shadowByID[stage.ID]
		if !ok {
			continue
		}
		liveFigures, _ := insightsFiguresOf(liveAt[stage.ID])
		shadowFigures, _ := insightsFiguresOf(shadowAt[stage.ID])
		l, s := insightsStageNumbers(liveAt[stage.ID]), insightsStageNumbers(shadowAt[stage.ID])
		both = append(both, InsightsShadowStage{
			Key: stage.ID, Name: other.Name, KindLabel: insightsKindLabel(other),
			Changed: !shadowSameStage(stage, other), Live: liveFigures, Shadow: shadowFigures,
			Tones: InsightsShadowTones{
				MedianTime:   insightsShadowTone(l[0], s[0], true),
				FirstTry:     insightsShadowTone(l[1], s[1], false),
				Tries:        insightsShadowTone(l[2], s[2], true),
				Cost:         insightsShadowTone(l[3], s[3], true),
				VisitsPerRun: insightsShadowTone(l[4], s[4], true),
			},
		})
	}
	only := func(stages []Stage, other map[string]Stage, scope *insightsScope) []InsightsStage {
		var missing []Stage
		for _, stage := range stages {
			if _, ok := other[stage.ID]; !ok {
				missing = append(missing, stage)
			}
		}
		return scope.stages(missing)
	}
	return both, only(liveStages, shadowByID, live), only(shadowStages, liveByID, shadow)
}

// insightsStageNumbers is a stage's median seconds, first-try share, tries per visit, mean
// visit cost and visits per run, each nil when there is nothing to measure.
func insightsStageNumbers(attempts []insightsAttempt) [5]*float64 {
	var out [5]*float64
	if len(attempts) == 0 {
		return out
	}
	seconds := make([]float64, 0, len(attempts))
	visits, runs := map[string]bool{}, map[string]bool{}
	for _, item := range attempts {
		seconds = append(seconds, item.seconds)
		visits[item.visit] = true
		runs[item.run.ID] = true
	}
	out[0] = insightsMedian(seconds)
	out[1] = insightsPassShare(insightsFirsts(attempts))
	tries := float64(len(attempts)) / float64(len(visits))
	out[2] = &tries
	_, out[3] = insightsFiguresOf(attempts)
	perRun := float64(len(visits)) / float64(len(runs))
	out[4] = &perRun
	return out
}

// insightsShadowTone is which way shadow differs from live for one stage figure: any
// difference counts, as the table only marks direction.
func insightsShadowTone(live, shadow *float64, lower bool) InsightsDeltaTone {
	switch {
	case live == nil || shadow == nil:
		return InsightsDeltaNone
	case math.Abs(*shadow-*live) < 1e-9:
		return InsightsDeltaFlat
	case (*shadow > *live) != lower:
		return InsightsDeltaBetter
	}
	return InsightsDeltaWorse
}
