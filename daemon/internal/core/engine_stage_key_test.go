// New runs resolve only the selected stage key,
// on a sized belt (forkedBelt(simple: ["build"])) built with the
// real defaults.

package core

import (
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// stageKeyTestForkedBelt is the forked belt: triage, then Spec and Build on the
// Complex route, quick-build on the Simple route, then Verify and Ship. Triage's
// first outcome takes the Simple route.
func stageKeyTestForkedBelt() Belt {
	fork := Fork{ID: "size", Routes: []Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	var stages []Stage
	add := func(agent string, route *string, id string) {
		place := DefaultsPlace{Before: stages, Forks: []Fork{fork}, Route: route}
		stage := DefaultsStageForAgentAt(SampleAgent(agent), place, stages, TriggerKindIssues)
		if id != "" {
			stage.ID = id
		}
		stages = append(stages, stage)
	}
	add("triage", nil, "")
	add("spec", new("complex"), "")
	add("build", new("complex"), "")
	add("build", new("simple"), "quick-build")
	add("verify", nil, "")
	add("ship", nil, "")
	stages[0].Outcomes[0].Then = NewThen(ThenArgs{Action: ThenActionRoute, Routes: []string{"simple"}})
	return NewBeltFromTrigger(BeltFromTriggerArgs{ID: "sized", Name: "Sized",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &IssueFilter{Label: "agent", Assignee: TriggerAssigneeAnyone, Poll: 60}},
		Repos:   []string{"acme/billing"}, Stages: stages, Forks: []Fork{fork}})
}

// TestNewRunsResolveOnlyTheSelectedStageKey
func TestNewRunsResolveOnlyTheSelectedStageKey(t *testing.T) {
	t.Parallel()
	belt := stageKeyTestForkedBelt()
	belt.Stages[1].SetKey(new("notes"))
	belt.Stages[1].SetWrites("complex.md")
	belt.Stages[3].SetKey(new("notes"))
	belt.Stages[3].SetWrites("simple.md")
	belt.Stages[0].Brief = "Before {notes}. {writer-2} {build}"
	run := briefTestSampleRun(belt)
	const folder = "/runs/r"
	checkout := folder + "/checkout"
	context := func() BriefContext {
		return BriefContext{RunFolder: folder, AttemptFolder: folder + "/attempt", Base: "main", Branch: "b",
			Agents: placeholderTestAgents(), Checkouts: map[string]string{"acme/billing": checkout}}
	}

	triage := *run.Stage()
	if got := briefPlaceholders(run, triage, context())["notes"]; got != "(no notes output yet)" {
		t.Fatalf("notes before any output = %q, want (no notes output yet)", got)
	}
	if _, ok := briefPlaceholders(run, triage, context())["writer-2"]; ok {
		t.Fatal("writer-2 resolves to a value")
	}
	if text := BriefRender(run, triage, context()); !strings.Contains(text, "{writer-2}") {
		t.Fatalf("{writer-2} was resolved:\n%s", text)
	}
	if text := BriefRender(run, triage, context()); strings.Contains(text, checkout) {
		t.Fatalf("the brief names the checkout:\n%s", text)
	}

	briefTestFinish(&run, "ready to implement")
	quick := run.Stage()
	if quick == nil || quick.ID != "quick-build" {
		t.Fatalf("stage after triage = %v, want quick-build", quick)
	}
	if got, want := briefPlaceholders(run, *quick, context())["notes"], folder+"/attempt/simple.md"; got != want {
		t.Fatalf("notes on quick-build = %q, want %q", got, want)
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "s", SessionID: "id", Folder: "simple-1"}, engineT0)
	open := run
	if got := briefPlaceholders(open, open.Stages[4], context())["notes"]; got != "(no simple.md yet)" {
		t.Fatalf("notes on verify while quick-build runs = %q, want (no simple.md yet)", got)
	}
	briefTestFinish(&run, "done")
	var verify Stage
	for _, stage := range run.Stages {
		if stage.ID == "verify" {
			verify = stage
		}
	}
	simplePath := folder + "/simple-1/simple.md"
	if got := briefPlaceholders(run, verify, context())["notes"]; got != simplePath {
		t.Fatalf("notes on verify = %q, want %q", got, simplePath)
	}

	stale := NewAttempt(AttemptArgs{StageID: "spec", Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Folder: "complex-9"})
	stale.EndedAt = &jsonx.Date{Time: engineT0.Add(10 * time.Second)}
	run.Attempts = append(run.Attempts, stale)
	if got := briefPlaceholders(run, verify, context())["notes"]; got != simplePath {
		t.Fatalf("notes after a stale complex attempt = %q, want %q", got, simplePath)
	}
	run.Attempts = append(run.Attempts, NewAttempt(AttemptArgs{StageID: "quick-build", Number: 2, StartedAt: jsonx.Date{Time: engineT0}, Folder: "simple-open"}))
	if got := briefPlaceholders(run, verify, context())["notes"]; got != simplePath {
		t.Fatalf("notes after a second quick-build attempt = %q, want %q", got, simplePath)
	}

	run.State = RunStateHeld
	run.Legs[0].State = RunStateHeld
	EngineAdvance(&run, EventResume(new("spec"), nil, nil), engineT0)
	if run.StageID != "spec" {
		t.Fatalf("stage = %q, want spec", run.StageID)
	}
	// Going back to the complex route keeps its earlier output until spec writes again.
	stalePath := folder + "/complex-9/complex.md"
	if got := briefPlaceholders(run, verify, context())["notes"]; got != stalePath {
		t.Fatalf("notes after going back to the complex route = %q, want %q", got, stalePath)
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "s2", SessionID: "id2", Folder: "complex-1"}, engineT0)
	if got := briefPlaceholders(run, verify, context())["notes"]; got != stalePath {
		t.Fatalf("notes with a complex session started = %q, want %q", got, stalePath)
	}
	run = btDecode[Run](t, btEncode(t, run))
	if got := briefPlaceholders(run, verify, context())["notes"]; got != stalePath {
		t.Fatalf("notes after a reload = %q, want %q", got, stalePath)
	}
}
