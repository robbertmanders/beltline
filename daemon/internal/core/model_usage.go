package core

import (
	"encoding/json"
)

// CostProvenance is where a known dollar amount came from. Legacy files omit
// this and are reported.
type CostProvenance string

const (
	CostProvenanceReported  CostProvenance = "reported"
	CostProvenanceEstimated CostProvenance = "estimated"
)

// AllCostProvenance lists every CostProvenance in declaration order (CaseIterable).
var AllCostProvenance = []CostProvenance{CostProvenanceReported, CostProvenanceEstimated}

// UsageCoverage is how much of an attempt's attributable work has a dollar amount.
type UsageCoverage string

const (
	// UsageCoverageComplete means every attributable record contributed dollars.
	UsageCoverageComplete UsageCoverage = "complete"
	// UsageCoveragePartial means some records contributed dollars and some did not.
	UsageCoveragePartial UsageCoverage = "partial"
	// UsageCoverageUntracked means no dollar amount is known. Tokens may still be present.
	UsageCoverageUntracked UsageCoverage = "untracked"
)

// AllUsageCoverage lists every UsageCoverage in declaration order (CaseIterable).
var AllUsageCoverage = []UsageCoverage{UsageCoverageComplete, UsageCoveragePartial, UsageCoverageUntracked}

// RateProvenance is the rate table revision that produced an estimate. Saved attempts keep theirs.
type RateProvenance struct {
	Revision string   `json:"revision"`
	Source   string   `json:"source"`
	Models   []string `json:"models"`
}

// TokenTotals are cumulative counters at an attempt boundary. A nil CacheWrite
// means that counter was not observed.
type TokenTotals struct {
	Input     int `json:"input"`
	CacheRead int `json:"cacheRead"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	Total     int `json:"total"`
	// CacheWrite is nil when the rollout did not report cache writes, as checkpoints saved before them did not.
	CacheWrite *int `json:"cacheWrite,omitzero"`
}

// TokenTotalsZero is the zero counters.
var TokenTotalsZero = TokenTotals{}

// Delta is the counters moved since baseline, or nil when any counter moved
// backwards.
func (t TokenTotals) Delta(baseline TokenTotals) *TokenTotals {
	if t.Input < baseline.Input || t.CacheRead < baseline.CacheRead || t.Output < baseline.Output ||
		t.Reasoning < baseline.Reasoning || t.Total < baseline.Total {
		return nil
	}
	var write *int
	if t.CacheWrite != nil && baseline.CacheWrite != nil {
		if *t.CacheWrite < *baseline.CacheWrite {
			return nil
		}
		w := *t.CacheWrite - *baseline.CacheWrite
		write = &w
	}
	return &TokenTotals{
		Input: t.Input - baseline.Input, CacheRead: t.CacheRead - baseline.CacheRead, Output: t.Output - baseline.Output,
		Reasoning: t.Reasoning - baseline.Reasoning, Total: t.Total - baseline.Total, CacheWrite: write,
	}
}

// UsageCheckpoint is where an attempt's usage starts. A missing checkpoint is
// not a zero baseline.
type UsageCheckpoint struct {
	// SessionID is the native rollout or session id this boundary belongs to.
	SessionID *string `json:"sessionId,omitzero"`
	// Lines are the transcript lines already written (Claude). Nil when this backend doesn't use one.
	Lines *int `json:"lines,omitzero"`
	// Totals are the cumulative counters already reported. Nil when they were not observed.
	Totals *TokenTotals `json:"totals,omitzero"`
	// Preexisting are the rollout ids that already existed for this checkout before a fresh launch.
	Preexisting []string `json:"preexisting,omitzero"`
}

// Usage is agent-reported usage. Money is whole nano-dollars. A nil cost is unknown, never zero.
//
// costNanos, input, output, cacheRead, cacheWrite, reasoning, totalTokens,
// provenance and rate are written when set; coverage is always written.
// Decoding defaults provenance and coverage from the cost (see UnmarshalJSON).
type Usage struct {
	CostNanos  *int            `json:"costNanos,omitzero"`
	Provenance *CostProvenance `json:"provenance,omitzero"`
	Coverage   UsageCoverage   `json:"coverage"`
	// Input, Output, CacheRead, CacheWrite, Reasoning and TotalTokens are nil
	// when that category was not reported. Zero is a reported zero.
	Input       *int            `json:"input,omitzero"`
	Output      *int            `json:"output,omitzero"`
	CacheRead   *int            `json:"cacheRead,omitzero"`
	CacheWrite  *int            `json:"cacheWrite,omitzero"`
	Reasoning   *int            `json:"reasoning,omitzero"`
	TotalTokens *int            `json:"totalTokens,omitzero"`
	Rate        *RateProvenance `json:"rate,omitzero"`
}

// UsageArgs are the parameters of NewUsage. Provenance and Coverage nil are
// derived from CostNanos, as Usage.init does.
type UsageArgs struct {
	CostNanos   *int
	Input       *int
	Output      *int
	CacheRead   *int
	CacheWrite  *int
	Reasoning   *int
	TotalTokens *int
	Provenance  *CostProvenance
	Coverage    *UsageCoverage
	Rate        *RateProvenance
}

// NewUsage creates a usage with Provenance and Coverage derived from CostNanos
// when not given: reported and complete when there is a cost, otherwise nil and
// untracked.
func NewUsage(a UsageArgs) Usage {
	u := Usage{
		CostNanos: a.CostNanos, Input: a.Input, Output: a.Output, CacheRead: a.CacheRead, CacheWrite: a.CacheWrite,
		Reasoning: a.Reasoning, TotalTokens: a.TotalTokens, Rate: a.Rate, Provenance: a.Provenance,
	}
	if u.Provenance == nil && a.CostNanos != nil {
		reported := CostProvenanceReported
		u.Provenance = &reported
	}
	switch {
	case a.Coverage != nil:
		u.Coverage = *a.Coverage
	case a.CostNanos != nil:
		u.Coverage = UsageCoverageComplete
	default:
		u.Coverage = UsageCoverageUntracked
	}
	return u
}

// UsageAdd is the sum of two usages: each category is added (nil when both are
// nil, the one value when only one is set, nil on overflow), and the cost is
// estimated when either side is.
func UsageAdd(lhs, rhs Usage) Usage {
	cost := usageAdding(lhs.CostNanos, rhs.CostNanos)
	estimated := (lhs.Provenance != nil && *lhs.Provenance == CostProvenanceEstimated) ||
		(rhs.Provenance != nil && *rhs.Provenance == CostProvenanceEstimated)
	var provenance *CostProvenance
	if cost != nil {
		p := CostProvenanceReported
		if estimated {
			p = CostProvenanceEstimated
		}
		provenance = &p
	}
	coverage := UsageCoverageUntracked
	if cost != nil {
		coverage = UsageCoveragePartial
		if lhs.Coverage == UsageCoverageComplete && rhs.Coverage == UsageCoverageComplete {
			coverage = UsageCoverageComplete
		}
	}
	return NewUsage(UsageArgs{
		CostNanos: cost, Input: usageAdding(lhs.Input, rhs.Input), Output: usageAdding(lhs.Output, rhs.Output),
		CacheRead: usageAdding(lhs.CacheRead, rhs.CacheRead), CacheWrite: usageAdding(lhs.CacheWrite, rhs.CacheWrite),
		Reasoning: usageAdding(lhs.Reasoning, rhs.Reasoning), TotalTokens: usageAdding(lhs.TotalTokens, rhs.TotalTokens),
		Provenance: provenance, Coverage: &coverage,
	})
}

// AddUsage adds rhs to u in place.
func (u *Usage) AddUsage(rhs Usage) {
	*u = UsageAdd(*u, rhs)
}

// usageAdding adds two optional counts.
func usageAdding(lhs, rhs *int) *int {
	switch {
	case lhs == nil && rhs == nil:
		return nil
	case lhs == nil:
		v := *rhs
		return &v
	case rhs == nil:
		v := *lhs
		return &v
	}
	sum, ok := AddInts(*lhs, *rhs)
	if !ok {
		return nil
	}
	return &sum
}

// usageWire is a usage as its keys are read. The enums are pointers, so an
// absent or null one takes the default that depends on the cost.
type usageWire struct {
	CostNanos   *int            `json:"costNanos"`
	Input       *int            `json:"input"`
	Output      *int            `json:"output"`
	CacheRead   *int            `json:"cacheRead"`
	CacheWrite  *int            `json:"cacheWrite"`
	Reasoning   *int            `json:"reasoning"`
	TotalTokens *int            `json:"totalTokens"`
	Provenance  *CostProvenance `json:"provenance"`
	Coverage    *UsageCoverage  `json:"coverage"`
	Rate        *RateProvenance `json:"rate"`
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	var w usageWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*u = NewUsage(UsageArgs(w))
	return nil
}
