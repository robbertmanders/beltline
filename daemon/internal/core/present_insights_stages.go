// Insights for one belt: the attempts and finished runs of a period, its row in the overview of
// every belt, and its stages with their models. A visit is one entry of a run into a stage on one leg: it starts
// when the leg's previous attempt was at another stage, so a run sent back to a stage visits it
// again, and its tries are its attempts. Attempts you stopped count nowhere but still end a
// visit, and a first try passes when its outcome's tone is good, whichever way the run went next.

package core

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// insightsAttempt is an ended attempt the view counts, with what it needs from its run and stage.
type insightsAttempt struct {
	run     *Run
	attempt *Attempt
	stage   *Stage
	visit   string
	agent   bool
	model   string
	runner  string
	ended   time.Time
	seconds float64
	// cost is nil when the attempt is an agent session without a dollar figure, or not an agent session.
	cost *int
}

// insightsFinished is a run finished in the period, done, shipped or dropped, with when it ended.
type insightsFinished struct {
	run     *Run
	ended   time.Time
	shipped bool
	lead    float64
	// cost is the run's agent spend, nil unless every agent attempt has a dollar figure.
	cost *int
}

// insightsScope is one belt's runs in the period.
type insightsScope struct {
	runs     []*Run
	attempts []insightsAttempt
	finished []insightsFinished
}

// insightsStopped reports whether you, or a move or an edit, stopped the attempt before it ended.
func insightsStopped(attempt *Attempt) bool {
	return attempt.Route != nil && strings.HasPrefix(*attempt.Route, "Stopped")
}

// insightsActive is the seconds from start to end that the run was not paused.
func insightsActive(run *Run, start, end time.Time) float64 {
	seconds := end.Sub(start).Seconds()
	for _, pause := range run.PauseTime {
		from, to := pause.Start.Time, pause.End.Time
		if from.Before(start) {
			from = start
		}
		if to.After(end) {
			to = end
		}
		if to.After(from) {
			seconds -= to.Sub(from).Seconds()
		}
	}
	return math.Max(0, seconds)
}

// add takes run's counted attempts that ended after since, and the run itself when it finished after since.
func (s *insightsScope) add(run *Run, since time.Time) {
	counted := false
	last := run.UpdatedAt.Time
	lastSet := false
	cost, priced := 0, true
	visits := insightsVisits(run)
	for i := range run.Attempts {
		attempt := &run.Attempts[i]
		if attempt.EndedAt == nil {
			continue
		}
		ended := attempt.EndedAt.Time
		if !lastSet || ended.After(last) {
			last, lastSet = ended, true
		}
		agent := costIsAgent(attempt, run)
		if agent {
			if attempt.Usage != nil && attempt.Usage.CostNanos != nil {
				cost += *attempt.Usage.CostNanos
			} else {
				priced = false
			}
		}
		if insightsStopped(attempt) || ended.Before(since) {
			continue
		}
		index := run.StageIndexOf(attempt.StageID)
		if index < 0 {
			continue
		}
		stage := &run.Stages[index]
		item := insightsAttempt{
			run: run, attempt: attempt, stage: stage, agent: agent, ended: ended,
			visit:   visits[i],
			seconds: insightsActive(run, attempt.StartedAt.Time, ended),
		}
		if agent {
			item.model, item.runner = stage.Model, ""
			if stage.Runner != nil {
				item.runner = string(*stage.Runner)
			}
			if attempt.Settings != nil {
				item.model = attempt.Settings.Model
				if attempt.Settings.Runner != nil {
					item.runner = string(*attempt.Settings.Runner)
				}
			}
			if attempt.Usage != nil && attempt.Usage.CostNanos != nil {
				item.cost = attempt.Usage.CostNanos
			}
		}
		s.attempts = append(s.attempts, item)
		counted = true
	}
	finished := !run.State.IsActive() && !last.Before(since)
	if finished {
		item := insightsFinished{run: run, ended: last, shipped: run.State == RunStateShipped, lead: insightsActive(run, run.CreatedAt.Time, last)}
		if priced && slices.ContainsFunc(run.Attempts, func(a Attempt) bool { return costIsAgent(&a, run) }) {
			item.cost = &cost
		}
		s.finished = append(s.finished, item)
	}
	if counted || finished {
		s.runs = append(s.runs, run)
	}
}

// insightsVisits is the visit of each of run's attempts, by index: the run, the leg, the stage
// and which entry into that stage on the leg it is, a new one whenever the leg's previous attempt,
// stopped or not, was at another stage.
func insightsVisits(run *Run) []string {
	out := make([]string, len(run.Attempts))
	previous := map[string]string{}
	entries := map[string]int{}
	for i, attempt := range run.Attempts {
		leg := ValueOr(attempt.Leg, "")
		key := leg + "|" + attempt.StageID
		if stage, ok := previous[leg]; !ok || stage != attempt.StageID {
			entries[key]++
		}
		previous[leg] = attempt.StageID
		out[i] = run.ID + "|" + key + "|" + strconv.Itoa(entries[key])
	}
	return out
}

// first is when the scope's earliest counted attempt started; zero when it has none.
func (s *insightsScope) first() time.Time {
	var first time.Time
	for _, item := range s.attempts {
		if start := item.attempt.StartedAt.Time; first.IsZero() || start.Before(first) {
			first = start
		}
	}
	return first
}

// insightsFirsts is the first attempt of each visit in attempts.
func insightsFirsts(attempts []insightsAttempt) []insightsAttempt {
	firsts := map[string]int{}
	var out []insightsAttempt
	for _, item := range attempts {
		if at, ok := firsts[item.visit]; ok {
			if item.attempt.Number < out[at].attempt.Number {
				out[at] = item
			}
			continue
		}
		firsts[item.visit] = len(out)
		out = append(out, item)
	}
	return out
}

func insightsPassed(item insightsAttempt) bool {
	return item.attempt.Tone != nil && *item.attempt.Tone == ToneGood
}

// insightsPassShare is the share of firsts that passed.
func insightsPassShare(firsts []insightsAttempt) *float64 {
	if len(firsts) == 0 {
		return nil
	}
	passed := 0
	for _, item := range firsts {
		if insightsPassed(item) {
			passed++
		}
	}
	share := float64(passed) / float64(len(firsts))
	return &share
}

// insightsPassRate is firsts' first-try pass as a rate.
func insightsPassRate(firsts []insightsAttempt) InsightsRate {
	passed := 0
	for _, item := range firsts {
		if insightsPassed(item) {
			passed++
		}
	}
	return insightsRateOf(passed, len(firsts))
}

var (
	insightsFirstTryLine = insightsMeasure[insightsAttempt]{
		at: func(item insightsAttempt) time.Time { return item.ended }, value: insightsPassShare, display: insightsPercent, share: true,
	}
	insightsLeadLine = insightsMeasure[insightsFinished]{
		at: func(item insightsFinished) time.Time { return item.ended }, lower: true,
		display: func(v float64) string { return StatusDuration(v) },
		value: func(window []insightsFinished) *float64 {
			leads := make([]float64, 0, len(window))
			for _, item := range window {
				leads = append(leads, item.lead)
			}
			return insightsMedian(leads)
		},
	}
	insightsCostLine = insightsMeasure[insightsFinished]{
		at: func(item insightsFinished) time.Time { return item.ended }, lower: true,
		display: func(v float64) string { return insightsDollars(&v) },
		value: func(window []insightsFinished) *float64 {
			if len(window) == 0 {
				return nil
			}
			total := 0
			for _, item := range window {
				total += *item.cost
			}
			mean := float64(total) / float64(len(window))
			return &mean
		},
	}
)

// shipped is the scope's shipped runs, and priced those whose every agent attempt has a dollar figure.
func (s *insightsScope) shipped() (shipped, priced []insightsFinished) {
	for _, item := range s.finished {
		if !item.shipped {
			continue
		}
		shipped = append(shipped, item)
		if item.cost != nil {
			priced = append(priced, item)
		}
	}
	return shipped, priced
}

// agentFirsts is the first attempt of every visit to an agent stage.
func (s *insightsScope) agentFirsts() []insightsAttempt {
	var agents []insightsAttempt
	for _, item := range s.attempts {
		if item.agent {
			agents = append(agents, item)
		}
	}
	return insightsFirsts(agents)
}

// beltRow is the scope as one row of the overview of every belt, its line drawn from history.
func (s *insightsScope) beltRow(belt InsightsBelt, history *insightsScope, days []time.Time, now time.Time) InsightsBeltRow {
	firsts := s.agentFirsts()
	shipped, priced := s.shipped()
	return InsightsBeltRow{
		ID: belt.ID, Name: belt.Label, Runs: len(s.runs),
		ShipRate: insightsRateOf(len(shipped), len(s.finished)), FirstTry: insightsPassRate(firsts),
		LeadTime: insightsDuration(insightsLeadLine.value(shipped)), CostPerRun: insightsDollars(insightsCostLine.value(priced)),
		Trend: insightsSeriesOf(history.agentFirsts(), insightsFirstTryLine, days, now),
	}
}

// InsightsFigures are what a stage and each of its models show. Visits are entries into the
// stage, VisitsPerRun those per run that visited it, Tries attempts per visit, and
// Cost the mean cost of a visit, all its tries together, over the visits whose every agent try has
// a dollar figure; CostCaption says how many those were.
type InsightsFigures struct {
	Visits       int          `json:"visits"`
	VisitsPerRun string       `json:"visitsPerRun"`
	LowSample    bool         `json:"lowSample"`
	MedianTime   string       `json:"medianTime"`
	FirstTry     InsightsRate `json:"firstTry"`
	Tries        string       `json:"tries"`
	Cost         string       `json:"cost"`
	CostCaption  string       `json:"costCaption"`
}

// InsightsModel is one model a stage's sessions ran on. Best marks the highest first try among
// the models with enough visits, the lower cost breaking a tie, when at least two have enough.
type InsightsModel struct {
	Model   string          `json:"model"`
	Runner  string          `json:"runner"`
	Best    bool            `json:"best"`
	Figures InsightsFigures `json:"figures"`
}

// InsightsStage is one stage of the belt, in belt order. Caption is its stage id when two stages
// share a name, and KindLabel names an approval or a watch. Models is empty for those.
type InsightsStage struct {
	Key       string          `json:"key"`
	Name      string          `json:"name"`
	Caption   string          `json:"caption"`
	KindLabel string          `json:"kindLabel"`
	Figures   InsightsFigures `json:"figures"`
	Models    []InsightsModel `json:"models"`
}

// insightsFiguresOf is attempts' figures, and the mean visit cost for ranking models.
func insightsFiguresOf(attempts []insightsAttempt) (InsightsFigures, *float64) {
	visits := map[string][]insightsAttempt{}
	runs := map[string]bool{}
	var order []string
	seconds := make([]float64, 0, len(attempts))
	for _, item := range attempts {
		runs[item.run.ID] = true
		if _, ok := visits[item.visit]; !ok {
			order = append(order, item.visit)
		}
		visits[item.visit] = append(visits[item.visit], item)
		seconds = append(seconds, item.seconds)
	}
	agentVisits, pricedVisits, total := 0, 0, 0
	for _, key := range order {
		visit := visits[key]
		if !slices.ContainsFunc(visit, func(item insightsAttempt) bool { return item.agent }) {
			continue
		}
		agentVisits++
		if slices.ContainsFunc(visit, func(item insightsAttempt) bool { return item.agent && item.cost == nil }) {
			continue
		}
		pricedVisits++
		for _, item := range visit {
			if item.cost != nil {
				total += *item.cost
			}
		}
	}
	figures := InsightsFigures{
		Visits: len(order), LowSample: len(order) < InsightsLowSample,
		MedianTime: insightsDuration(insightsMedian(seconds)), FirstTry: insightsPassRate(insightsFirsts(attempts)),
		VisitsPerRun: "–", Tries: "–", Cost: "–", CostCaption: "No agent sessions",
	}
	if len(order) == 0 {
		figures.CostCaption = "No visits in this period"
	}
	if len(order) > 0 {
		figures.VisitsPerRun = strconv.FormatFloat(float64(len(order))/float64(len(runs)), 'f', 2, 64)
		figures.Tries = strconv.FormatFloat(float64(len(attempts))/float64(len(order)), 'f', 2, 64)
	}
	var mean *float64
	if agentVisits > 0 {
		figures.CostCaption = strconv.Itoa(pricedVisits) + " of " + strconv.Itoa(agentVisits) + " visits priced"
		figures.Cost = "not tracked"
		if pricedVisits > 0 {
			value := float64(total) / float64(pricedVisits)
			mean = &value
			figures.Cost = insightsDollars(mean)
		}
	}
	return figures, mean
}

// insightsKindLabel names an approval, a prompt or a watch, and nothing for an agent stage.
func insightsKindLabel(stage Stage) string {
	switch stage.Kind() {
	case StageKindGate:
		if stage.Type == StageTypePrompt {
			return "PROMPT"
		}
		return "APPROVAL"
	case StageKindWatch:
		return "WATCH"
	}
	return ""
}

// stages is the definition's stages in its order, each with the scope's attempts at it. A stage the
// definition no longer has is left out, and one nobody visited in the period is listed empty.
func (s *insightsScope) stages(definition []Stage) []InsightsStage {
	byStage := map[string][]insightsAttempt{}
	for _, item := range s.attempts {
		byStage[item.stage.ID] = append(byStage[item.stage.ID], item)
	}
	names := map[string]int{}
	for _, stage := range definition {
		names[stage.Name]++
	}
	out := make([]InsightsStage, 0, len(definition))
	for _, stage := range definition {
		attempts := byStage[stage.ID]
		figures, _ := insightsFiguresOf(attempts)
		row := InsightsStage{
			Key: stage.ID, Name: stage.Name, KindLabel: insightsKindLabel(stage),
			Figures: figures, Models: insightsModels(attempts),
		}
		if names[stage.Name] > 1 {
			row.Caption = "stage " + stage.ID
		}
		out = append(out, row)
	}
	return out
}

// insightsModels is the models a stage's agent sessions ran on, best first, greyed ones last.
func insightsModels(attempts []insightsAttempt) []InsightsModel {
	byModel := map[string][]insightsAttempt{}
	var models []string
	for _, item := range attempts {
		if !item.agent || item.model == "" {
			continue
		}
		if _, ok := byModel[item.model]; !ok {
			models = append(models, item.model)
		}
		byModel[item.model] = append(byModel[item.model], item)
	}
	type ranked struct {
		model InsightsModel
		cost  *float64
	}
	rows := make([]ranked, 0, len(models))
	for _, model := range models {
		list := byModel[model]
		runner := ""
		for _, item := range list {
			if item.runner != "" {
				runner = item.runner
			}
		}
		figures, cost := insightsFiguresOf(list)
		rows = append(rows, ranked{model: InsightsModel{Model: model, Runner: runner, Figures: figures}, cost: cost})
	}
	share := func(r ranked) float64 {
		if r.model.Figures.FirstTry.Of == 0 {
			return 0
		}
		return float64(r.model.Figures.FirstTry.Count) / float64(r.model.Figures.FirstTry.Of)
	}
	costOf := func(r ranked) float64 {
		if r.cost == nil {
			return math.Inf(1)
		}
		return *r.cost
	}
	slices.SortStableFunc(rows, func(a, b ranked) int {
		if a.model.Figures.LowSample != b.model.Figures.LowSample {
			if a.model.Figures.LowSample {
				return 1
			}
			return -1
		}
		if c := cmp.Compare(share(b), share(a)); c != 0 {
			return c
		}
		if c := cmp.Compare(costOf(a), costOf(b)); c != 0 {
			return c
		}
		return strings.Compare(a.model.Model, b.model.Model)
	})
	out := make([]InsightsModel, 0, len(rows))
	enough := 0
	for _, row := range rows {
		if !row.model.Figures.LowSample {
			enough++
		}
		out = append(out, row.model)
	}
	if enough >= 2 {
		out[0].Best = true
	}
	return out
}
