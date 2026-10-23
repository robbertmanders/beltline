package core

import "time"

// FactorySummaryCounts are one belt's run counts on the factory floor, or the factory's total.
// The zero value has every count at zero.
type FactorySummaryCounts struct {
	Running      int `json:"running"`
	NeedYou      int `json:"needYou"`
	InProgress   int `json:"inProgress"`
	ShippedToday int `json:"shippedToday"`
}

// FactorySummaryBelt is one belt's counts, as the factory summary holds them.
type FactorySummaryBelt struct {
	ID     string
	Name   string
	Paused bool
	Counts FactorySummaryCounts
}

// FactorySummaryBeltInput is a belt the summary counts, by identity and pause state.
type FactorySummaryBeltInput struct {
	ID     string
	Name   string
	Paused bool
}

// FactorySummary is the counts of every belt on the factory floor, and their total.
type FactorySummary struct {
	Belts []FactorySummaryBelt
	Total FactorySummaryCounts
}

// NewFactorySummary counts runs for each belt at now, in loc (nil for the local calendar).
func NewFactorySummary(belts []FactorySummaryBeltInput, runs []Run, now time.Time, loc *time.Location) FactorySummary {
	if loc == nil {
		loc = time.Local
	}
	summary := FactorySummary{Belts: []FactorySummaryBelt{}}
	for _, belt := range belts {
		var counts FactorySummaryCounts
		for i := range runs {
			run := &runs[i]
			if run.BeltID != belt.ID {
				continue
			}
			if run.State == RunStateRunning {
				counts.Running++
			}
			if run.State.NeedsYou() {
				counts.NeedYou++
			}
			if run.State.IsActive() {
				counts.InProgress++
			}
			if run.State == RunStateShipped && factorySummarySameDay(run.UpdatedAt.Time, now, loc) {
				counts.ShippedToday++
			}
		}
		summary.Belts = append(summary.Belts, FactorySummaryBelt{ID: belt.ID, Name: belt.Name, Paused: belt.Paused, Counts: counts})
	}
	for _, belt := range summary.Belts {
		summary.Total.Running += belt.Counts.Running
		summary.Total.NeedYou += belt.Counts.NeedYou
		summary.Total.InProgress += belt.Counts.InProgress
		summary.Total.ShippedToday += belt.Counts.ShippedToday
	}
	return summary
}

// factorySummarySameDay reports whether a and b fall on the same calendar day in loc.
func factorySummarySameDay(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}
