// Tests of cost and usage: the labels, spend and exact addition.

package core

import (
	"reflect"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // the DST fixture needs America/New_York on every platform

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// costTestRun is a run of the default belt on a/b, numbered 1, at now.
func costTestRun(now time.Time) Run {
	subject := DefaultSubject("a/b", "Test")
	subject.Number = 1
	return NewRun(RunArgs{Belt: SampleBelt(nil, "agent", SampleAgents(), true), Subject: subject, Now: now})
}

// costTestAttempt is an attempt of stage at spring that ended at ended (nil while open), with usage.
// The attempt helper of spendMarksPartialEstimatesAndIgnoresOpenAndNonAgentWork: approve has no session.
func costTestAttempt(stage string, spring time.Time, ended *time.Time, usage *Usage) Attempt {
	start := spring
	if ended != nil {
		start = ended.Add(-30 * time.Second)
	}
	attempt := NewAttempt(AttemptArgs{StageID: stage, Number: 1, StartedAt: jsonx.Date{Time: start}, Folder: stage})
	if ended != nil {
		end := jsonx.Date{Time: *ended}
		attempt.EndedAt = &end
	}
	if stage != "approve" {
		session := "bl-" + stage
		attempt.Session = &session
	}
	attempt.Usage = usage
	return attempt
}

// TestFormattingAndExactAdditionDollarLabels checks the dollar half of
// the formatting and exact addition, and the usage labels.
func TestFormattingAndExactAdditionDollarLabels(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		nanos int
		label string
	}{{340_000_000, "$0.34"}, {3_400_000, "$0.0034"}, {0, "$0.00"}, {1_204_500_000_000, "$1,204.50"}} {
		if got := CostDollars(tc.nanos); got != tc.label {
			t.Fatalf("dollars(%d) = %q, want %q", tc.nanos, got, tc.label)
		}
	}
}

// TestLabelsCoverReportedEstimatedPartialAndUnknown checks the usage labels.
func TestLabelsCoverReportedEstimatedPartialAndUnknown(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	ended := func(usage *Usage) Attempt {
		attempt := NewAttempt(AttemptArgs{StageID: "build", Number: 1, StartedAt: jsonx.Date{Time: now.Add(-60 * time.Second)}, Folder: "build"})
		end := jsonx.Date{Time: now}
		attempt.EndedAt = &end
		session := "bl"
		attempt.Session = &session
		attempt.Usage = usage
		return attempt
	}
	estimatedKind, complete, partialKind := CostProvenanceEstimated, UsageCoverageComplete, UsageCoveragePartial
	rate := &RateProvenance{Revision: "r", Source: "s", Models: []string{"m"}}
	estimated := NewUsage(UsageArgs{CostNanos: new(120_000_000), Input: new(4), TotalTokens: new(4), Provenance: &estimatedKind, Coverage: &complete, Rate: rate})
	if got := CostTokenDetail(estimated); got == nil || *got != "4 in · 4 total" {
		t.Fatalf("token detail = %v", got)
	}
	if !CostExplainsEstimate(&estimated) {
		t.Fatal("an estimated usage does not explain its estimate")
	}
	if CostEstimateExplanation != "API-equivalent usage cost from the bundled rate table; not subscription charges or invoices." {
		t.Fatalf("estimate explanation = %q", CostEstimateExplanation)
	}
	partial := NewUsage(UsageArgs{CostNanos: new(120_000_000), TotalTokens: new(4), Provenance: &estimatedKind, Coverage: &partialKind, Rate: rate})
	if got := CostAttemptLabel(&partial); got != "$0.12 · partial · estimated" {
		t.Fatalf("partial label = %q", got)
	}
	if !CostExplainsEstimate(&partial) {
		t.Fatal("a partial estimate does not explain its estimate")
	}
	tokensOnly := NewUsage(UsageArgs{Input: new(9), TotalTokens: new(9)})
	if got := CostAttemptLabel(&tokensOnly); got != "cost not tracked" {
		t.Fatalf("tokens-only label = %q", got)
	}
	if got := CostTokenDetail(tokensOnly); got == nil || *got != "9 in · 9 total" {
		t.Fatalf("tokens-only detail = %v", got)
	}
	if got := CostTokenDetail(NewUsage(UsageArgs{CostNanos: new(1)})); got != nil {
		t.Fatalf("cost-only detail = %v, want nil", *got)
	}
	if got := CostAttemptLabel(nil); got != "cost not tracked" {
		t.Fatalf("nil label = %q", got)
	}
	reported := NewUsage(UsageArgs{CostNanos: new(340_000_000)})
	if got := CostSession(ended(&reported), true, now); got != "1m 0s · $0.34" {
		t.Fatalf("reported session = %q", got)
	}
	zero := NewUsage(UsageArgs{CostNanos: new(0)})
	if got := CostSession(ended(&zero), true, now); got != "1m 0s · $0.00" {
		t.Fatalf("zero session = %q", got)
	}
	estimatedSession := NewUsage(UsageArgs{CostNanos: new(120_000_000), Input: new(4), TotalTokens: new(4), Provenance: &estimatedKind, Coverage: &complete, Rate: rate})
	if got := CostSession(ended(&estimatedSession), true, now); got != "1m 0s · $0.12 · estimated" {
		t.Fatalf("estimated session = %q", got)
	}
	open := ended(nil)
	open.EndedAt = nil
	if got := CostSession(open, true, now); got != "live" {
		t.Fatalf("open session = %q", got)
	}
	gate := ended(nil)
	gate.Session = nil
	if got := CostSession(gate, false, now); got != "1m 0s" {
		t.Fatalf("gated session = %q", got)
	}
}

// TestSpendMarksPartialEstimatesAndIgnoresOpenAndNonAgentWork
func TestSpendMarksPartialEstimatesAndIgnoresOpenAndNonAgentWork(t *testing.T) {
	t.Parallel()
	calendar, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	spring := time.Date(2026, time.March, 8, 3, 30, 0, 0, calendar)
	run := costTestRun(spring)
	reported := NewUsage(UsageArgs{CostNanos: new(340_000_000)})
	estimatedKind, complete := CostProvenanceEstimated, UsageCoverageComplete
	estimated := NewUsage(UsageArgs{CostNanos: new(120_000_000), Provenance: &estimatedKind, Coverage: &complete})
	ended := spring
	run.Attempts = []Attempt{
		costTestAttempt("build", spring, &ended, &reported),
		costTestAttempt("spec", spring, &ended, &estimated),
		costTestAttempt("verify", spring, &ended, nil),
		costTestAttempt("approve", spring, &ended, nil),
		costTestAttempt("triage", spring, nil, nil),
	}
	spend := CostSpendOf(run)
	if line := CostRunLine(spend); line == nil || *line != "$0.46 · partial · estimated · ended sessions" {
		t.Fatalf("run line = %v", line)
	}
	if got := CostAmount(CostToday([]Run{run}, calendar, spring)); got != "$0.46" {
		t.Fatalf("today amount = %q", got)
	}
	previousDay := time.Date(2026, time.March, 7, 23, 30, 0, 0, calendar)
	run.Attempts[0].EndedAt = &jsonx.Date{Time: previousDay}
	if got := CostToday([]Run{run}, calendar, spring); got.Nanos == nil || *got.Nanos != 120_000_000 {
		t.Fatalf("today after moving build = %v", got.Nanos)
	}
	run.Attempts = []Attempt{costTestAttempt("build", spring, &ended, nil), costTestAttempt("spec", spring, &ended, nil)}
	if got := CostAmount(CostSpendOf(run)); got != "$0.00" {
		t.Fatalf("unpriced amount = %q", got)
	}
	if line := CostRunLine(CostSpendOf(run)); line != nil && strings.Contains(*line, "$0.00") {
		t.Fatalf("unpriced run line = %q", *line)
	}
	run.Attempts = []Attempt{costTestAttempt("approve", spring, &ended, nil)}
	if line := CostRunLine(CostSpendOf(run)); line != nil {
		t.Fatalf("approval-only run line = %q, want nil", *line)
	}
	if got := CostAmount(CostToday([]Run{run}, calendar, spring)); got != "$0.00" {
		t.Fatalf("approval-only today = %q", got)
	}
}

// TestLegacyUsageStaysReportedAndIsNotBackfilled
func TestLegacyUsageStaysReportedAndIsNotBackfilled(t *testing.T) {
	t.Parallel()
	legacy := `{"costNanos":340000000,"input":1,"output":2,"cacheRead":3,"cacheWrite":4,"reasoning":5,"totalTokens":6}`
	var usage Usage
	if err := json.Unmarshal([]byte(legacy), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.CostNanos == nil || *usage.CostNanos != 340_000_000 {
		t.Fatalf("legacy cost = %v", usage.CostNanos)
	}
	if usage.Provenance == nil || *usage.Provenance != CostProvenanceReported {
		t.Fatalf("legacy provenance = %v, want reported", usage.Provenance)
	}
	if usage.Coverage != UsageCoverageComplete {
		t.Fatalf("legacy coverage = %v, want complete", usage.Coverage)
	}
	if usage.Input == nil || *usage.Input != 1 {
		t.Fatalf("legacy input = %v", usage.Input)
	}
	now := time.Unix(1_700_000_000, 0)
	run := costTestRun(now)
	attempt := NewAttempt(AttemptArgs{StageID: "build", Number: 1, StartedAt: jsonx.Date{Time: now}, Folder: "build"})
	end := jsonx.Date{Time: now}
	attempt.EndedAt = &end
	session := "bl"
	attempt.Session = &session
	logMark, usageMark := 4, 9
	attempt.LogMark = &logMark
	attempt.UsageMark = &usageMark
	run.Attempts = []Attempt{attempt}
	data, err := jsonx.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Run
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Attempts[0].LogMark == nil || *decoded.Attempts[0].LogMark != 4 || decoded.Attempts[0].UsageMark == nil || *decoded.Attempts[0].UsageMark != 9 {
		t.Fatalf("marks not kept: %+v", decoded.Attempts[0])
	}
	if decoded.Attempts[0].Usage != nil {
		t.Fatalf("usage backfilled: %+v", decoded.Attempts[0].Usage)
	}
	if got := CostAttemptLabel(decoded.Attempts[0].Usage); got != "cost not tracked" {
		t.Fatalf("label of unbackfilled usage = %q", got)
	}
	decoded.Attempts[0].Usage = &usage
	data, err = jsonx.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var again Run
	if err := json.Unmarshal(data, &again); err != nil {
		t.Fatal(err)
	}
	if again.Attempts[0].Usage == nil || again.Attempts[0].Usage.CostNanos == nil || *again.Attempts[0].Usage.CostNanos != 340_000_000 {
		t.Fatalf("usage after round trip = %+v", again.Attempts[0].Usage)
	}
	if again.Attempts[0].Usage.Provenance == nil || *again.Attempts[0].Usage.Provenance != CostProvenanceReported {
		t.Fatal("provenance after round trip is not reported")
	}
}

// TestAZeroCostAttemptKeepsSpendWhole
func TestAZeroCostAttemptKeepsSpendWhole(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	run := costTestRun(now)
	attempt := NewAttempt(AttemptArgs{StageID: "build", Number: 1, StartedAt: jsonx.Date{Time: now}, Folder: "build"})
	end := jsonx.Date{Time: now}
	attempt.EndedAt = &end
	session := "bl-build"
	attempt.Session = &session
	attempt.Usage = new(NewUsage(UsageArgs{CostNanos: new(0)}))
	run.Attempts = []Attempt{attempt}
	if got := CostAmount(CostSpendOf(run)); got != "$0.00" {
		t.Fatalf("zero-cost amount = %q", got)
	}
}

// TestCostDollarsTable pins CostDollars at the rounding and precision boundaries: four places
// below one cent (nanos above 0 and under 10 000 000), two otherwise, and half-to-even rounding
// as a number formatter does it.
func TestCostDollarsTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		nanos int
		want  string
	}{
		{125_000_000, "$0.12"},
		{375_000_000, "$0.38"},
		{615_000_000, "$0.62"},
		{5_000_000, "$0.0050"},
		{9_999_999, "$0.0100"},
		{10_000_000, "$0.01"},
		{9_999_999_999, "$10.00"},
		{-340_000_000, "-$0.34"},
		{0, "$0.00"},
	}
	for _, c := range cases {
		if got := CostDollars(c.nanos); got != c.want {
			t.Errorf("CostDollars(%d) = %q, want %q", c.nanos, got, c.want)
		}
	}
}

// TestLocalDayHoldsExactlyItsDate checks the day's quick bounds against the date itself, every
// quarter hour across changes to and from summer time and Samoa's skipped day.
func TestLocalDayHoldsExactlyItsDate(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		zone string
		day  time.Time
	}{
		{"America/New_York", time.Date(2025, 3, 9, 12, 0, 0, 0, time.UTC)},
		{"America/New_York", time.Date(2025, 11, 2, 12, 0, 0, 0, time.UTC)},
		{"Pacific/Apia", time.Date(2011, 12, 29, 12, 0, 0, 0, time.UTC)},
		{"Pacific/Apia", time.Date(2011, 12, 31, 12, 0, 0, 0, time.UTC)},
	} {
		loc, err := time.LoadLocation(row.zone)
		if err != nil {
			t.Fatal(err)
		}
		day := localDayOf(row.day, loc)
		y, m, d := row.day.In(loc).Date()
		for at := row.day.Add(-72 * time.Hour); at.Before(row.day.Add(72 * time.Hour)); at = at.Add(15 * time.Minute) {
			ay, am, ad := at.In(loc).Date()
			if want := ay == y && am == m && ad == d; day.holds(at) != want {
				t.Fatalf("%s %v: holds(%v) = %t, want %t", row.zone, row.day, at, !want, want)
			}
		}
	}
}

// TestSpendCountsDoneRunsLikeAnyOtherFinishedRun checks that a Done run's priced attempts count in
// today's spend, on its run line and in its sessions exactly as a Shipped run's do: only the shipped
// figures are kept apart, and spend and sessions are not among them.
func TestSpendCountsDoneRunsLikeAnyOtherFinishedRun(t *testing.T) {
	t.Parallel()
	calendar, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	spring := time.Date(2026, time.March, 8, 3, 30, 0, 0, calendar)
	ended := spring
	reported := NewUsage(UsageArgs{CostNanos: new(340_000_000)})
	done := costTestRun(spring)
	done.State = RunStateDone
	done.Attempts = []Attempt{costTestAttempt("build", spring, &ended, &reported)}
	shipped := costTestRun(spring)
	shipped.State = RunStateShipped
	shipped.Attempts = []Attempt{costTestAttempt("build", spring, &ended, &reported)}
	if got := CostAmount(CostToday([]Run{done}, calendar, spring)); got != "$0.34" {
		t.Fatalf("done run's spend today = %q, want $0.34", got)
	}
	if !reflect.DeepEqual(CostToday([]Run{done}, calendar, spring), CostToday([]Run{shipped}, calendar, spring)) {
		t.Fatalf("spend today differs between a Done and a Shipped run")
	}
	if !reflect.DeepEqual(CostRunLine(CostSpendOf(done)), CostRunLine(CostSpendOf(shipped))) {
		t.Fatalf("run line differs: done %v, shipped %v", CostRunLine(CostSpendOf(done)), CostRunLine(CostSpendOf(shipped)))
	}
	if RunSessionCount(done) != 1 || RunSessionCount(done) != RunSessionCount(shipped) {
		t.Fatalf("sessions: done %d, shipped %d, want one each", RunSessionCount(done), RunSessionCount(shipped))
	}
}
