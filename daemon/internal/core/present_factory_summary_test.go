// with
// local fixtures.

package core

import (
	"testing"
	"time"
)

// TestCountsSumAndKeepPauseState
func TestCountsSumAndKeepPauseState(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_791_356_400, 0)
	ids := []struct{ id, name string }{{"issues", "Issues"}, {"deps", "Dependencies"}, {"reviews", "Reviews"}}
	var inputs []FactorySummaryBeltInput
	var runs []Run
	for _, entry := range ids {
		belt := SampleBelt([]string{"a/b"}, "agent", SampleAgents(), true)
		belt.ID = entry.id
		belt.Name = entry.name
		inputs = append(inputs, FactorySummaryBeltInput{ID: entry.id, Name: entry.name, Paused: entry.id == "deps"})
		for number, state := range []RunState{RunStateRunning, RunStateWaiting, RunStateHeld, RunStateQueued, RunStateShipped} {
			run := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: number, Title: "Test"}, Now: now})
			run.State = state
			runs = append(runs, run)
		}
	}
	summary := NewFactorySummary(inputs, runs, now, nil)
	if summary.Total.Running != len(ids) {
		t.Fatalf("total running = %d, want %d", summary.Total.Running, len(ids))
	}
	needYou, inProgress := 0, 0
	for _, belt := range summary.Belts {
		needYou += belt.Counts.NeedYou
		inProgress += belt.Counts.InProgress
	}
	if summary.Total.NeedYou != needYou || summary.Total.InProgress != inProgress {
		t.Fatalf("totals %+v do not sum the belts", summary.Total)
	}
	if summary.Total.ShippedToday != len(ids) {
		t.Fatalf("shipped today = %d, want %d", summary.Total.ShippedToday, len(ids))
	}
	paused := map[string]bool{}
	for _, belt := range summary.Belts {
		paused[belt.ID] = belt.Paused
	}
	if !paused["deps"] || paused["issues"] || paused["reviews"] {
		t.Fatalf("paused = %v, want only deps paused", paused)
	}
}
