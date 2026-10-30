package core

import (
	"strings"
	"testing"
)

// engineForkedBelt is the forked belt for simple agents: triage, spec and build
// on the complex route, one quick stage per simple agent, then verify and ship. The
// stages are the shipped defaults for their agents.
func engineForkedBelt(simple []string) Belt {
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
	for _, agent := range simple {
		add(agent, new("simple"), "quick-"+agent)
	}
	add("verify", nil, "")
	add("ship", nil, "")
	stages[0].Outcomes[0].Then = Then{Action: ThenActionRoute, Routes: []string{"simple"}}
	filter := DefaultIssueFilter()
	filter.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "sized", Name: "Sized", Trigger: Trigger{Kind: TriggerKindIssues, Issues: &filter},
		Repos: []string{"acme/billing"}, Stages: stages, Forks: []Fork{fork},
	})
}

// engineBothRoutes is the forked belt whose
// first stage routes to both the complex and the simple route.
func engineBothRoutes() Belt {
	belt := engineForkedBelt([]string{"build"})
	belt.Stages[0].Outcomes[0].Then = Then{Action: ThenActionRoute, Routes: []string{"complex", "simple"}}
	return belt
}

// engineReplayPrompt is the rerun prompt of the
// run's replayed attempt, in the folder the stage would write to.
func engineReplayPrompt(t *testing.T, run Run) string {
	t.Helper()
	stage := *run.Stage()
	folder := EngineAttemptFolder(run, stage)
	previous := run.AttemptByID(run.Replays)
	if previous == nil {
		t.Fatalf("run replays %q, which is not an attempt", engineStr(run.Replays))
	}
	return BriefRerun(run, stage, *previous, engineRerunContext(folder))
}

// TestSendingBackToSpecListsEarlierAttemptsAndTheAnswers: a send back to spec lists
// every spec attempt, with the answers, and the next session replays the last one.
func TestSendingBackToSpecListsEarlierAttemptsAndTheAnswers(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	engineFinishAt(&run, "open questions", nil)
	if run.State != RunStateHeld {
		t.Fatalf("state = %q, want held", run.State)
	}
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("Use the current filter.")}, engineT0)
	engineFinishAt(&run, "complete", nil)
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "sent back", Note: new("Bring back the target policy.")}, engineT0)
	if run.StageID != "spec" || run.State != RunStateQueued || run.Continues != nil {
		t.Fatalf("sent back: stage %q state %q continues %q", run.StageID, run.State, engineStr(run.Continues))
	}
	earlier := run.ClosedAttempts("spec", nil)
	if len(earlier) != 2 || earlier[0].Folder != "02-spec-1" || earlier[1].Folder != "02-spec-2" {
		t.Fatalf("closed spec attempts = %+v", earlier)
	}
	if engineStr(earlier[1].Note) != "Use the current filter." {
		t.Errorf("second attempt note = %q", engineStr(earlier[1].Note))
	}
	if engineStr(run.Replays) != earlier[1].ID {
		t.Errorf("replays = %q, want %q", engineStr(run.Replays), earlier[1].ID)
	}
	text := engineReplayPrompt(t, run)
	if !strings.Contains(text, "## A note for this attempt") || !strings.Contains(text, "Bring back the target policy.") {
		t.Errorf("prompt lacks the note:\n%s", text)
	}
	work := engineSection(t, text, "## What this stage already had", "## When you are done")
	if engineIndexOf(t, text, "### Attempt 2") >= engineIndexOf(t, text, "### Attempt 1") {
		t.Error("attempt 2 is not listed before attempt 1")
	}
	if !strings.Contains(work, "Use the current filter.") {
		t.Error("earlier work lacks the answer")
	}
	if !strings.Contains(work, "/runs/test/02-spec-1/spec.md") || !strings.Contains(work, "/runs/test/02-spec-2/spec.md") {
		t.Error("earlier work lacks the spec files")
	}
	if strings.Contains(work, "Bring back the target policy.") {
		t.Error("earlier work carries the new note")
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "spec-3", SessionID: "spec-3", Folder: "02-spec-3"}, engineT0)
	last := run.Attempts[len(run.Attempts)-1]
	if engineStr(last.Replays) != "spec-2" || last.Continues != nil || engineStr(last.SessionID) == engineStr(earlier[1].SessionID) {
		t.Errorf("new attempt replays %q continues %q", engineStr(last.Replays), engineStr(last.Continues))
	}
}

// TestNextIntoAStageThatAlreadyRanReplaysIt: a stage the run has been through before
// replays its last attempt, and a code mismatch sends build back with its findings.
func TestNextIntoAStageThatAlreadyRanReplaysIt(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "complete")
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	if run.StageID != "build" || run.Replays != nil {
		t.Fatalf("build: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	engineFinishOutcome(&run, "done")
	if run.StageID != "verify" || run.Replays != nil {
		t.Fatalf("verify: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	engineFinishBody(&run, new("code mismatch"), "rule 3: 500 on empty")
	if run.StageID != "build" || engineStr(run.Replays) != "build-1" {
		t.Fatalf("back to build: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	if run.Note == nil || !strings.Contains(*run.Note, "rule 3: 500 on empty") {
		t.Errorf("note = %q", engineStr(run.Note))
	}
	engineFinishOutcome(&run, "done")
	if run.StageID != "verify" || engineStr(run.Replays) != "verify-1" {
		t.Fatalf("verify again: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	if !strings.Contains(engineReplayPrompt(t, run), "/runs/test/f/verdict.md") {
		t.Error("prompt does not point at the earlier verdict")
	}
}

// TestARouteBackToAStageReplaysThatRoutesEarlierWork: a watch that sends a run to a
// route replays that route's last attempt when it comes back.
func TestARouteBackToAStageReplaysThatRoutesEarlierWork(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineToWatch(&run)
	EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventChecksFailed, Detail: new("#201")}, engineT0)
	if run.StageID != "fix-checks" || run.Replays != nil {
		t.Fatalf("fix-checks: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	engineFinishAt(&run, "fixed", nil)
	if run.StageID != "watch" {
		t.Fatalf("after fix: stage %q", run.StageID)
	}
	EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventChecksFailed, Detail: new("#201 again")}, engineT0)
	if run.StageID != "fix-checks" || engineStr(run.Replays) != "fix-checks-1" {
		t.Fatalf("again: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	earlier := run.AttemptByID(run.Replays)
	if earlier == nil {
		t.Fatal("replayed attempt not found")
	}
	if !strings.Contains(engineReplayPrompt(t, run), "/runs/test/"+earlier.Folder+"/checks.md") {
		t.Errorf("prompt does not point at the earlier checks file")
	}
}

// TestARouteReplaysTheStageTheLegLandedOn: a leg replays the stage its own route
// reached, not the stage of the other route.
func TestARouteReplaysTheStageTheLegLandedOn(t *testing.T) {
	t.Parallel()
	run := engineStartRun(engineBothRoutes())
	engineFinishOutcome(&run, "ready to implement")
	complex, simple := new("r:complex"), new("r:simple")
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "spec", SessionID: "spec", Folder: "spec-folder", Leg: complex}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Output: new("Outcome: complete"), Leg: complex}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "build", SessionID: "build", Folder: "build-folder", Leg: complex}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Output: new("Outcome: done"), Leg: complex}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "quick", SessionID: "quick", Folder: "quick-folder", Leg: simple}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Output: new("Outcome: done"), Leg: simple}, engineT0)
	if run.StageID != "verify" || run.Replays != nil {
		t.Fatalf("verify: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	engineFinishBody(&run, new("code mismatch"), "Rule 2 fails.")
	if run.StageID != "quick-build" || engineStr(run.Replays) != "quick-build-r:simple-1" {
		t.Fatalf("back: stage %q replays %q", run.StageID, engineStr(run.Replays))
	}
	text := engineReplayPrompt(t, run)
	if !strings.Contains(text, "/runs/test/quick-folder/evidence.md") || strings.Contains(text, "build-folder") {
		t.Errorf("prompt follows the wrong leg:\n%s", text)
	}
	if run.Continues != nil {
		t.Errorf("continues = %q, want nil", *run.Continues)
	}
}

// TestRetriesFallbacksEditsAndAFreshResumeDoNotReplay: a retry, a fallback retry,
// an edit that triages again, and a resume with earlier=false never replay.
func TestRetriesFallbacksEditsAndAFreshResumeDoNotReplay(t *testing.T) {
	t.Parallel()
	retry := engineSampleRun()
	engineFinishOutcome(&retry, "ready to spec")
	engineFinishOutcome(&retry, "incomplete")
	if retry.StageID != "spec" || retry.Replays != nil {
		t.Errorf("retry: stage %q replays %q", retry.StageID, engineStr(retry.Replays))
	}

	belt := engineIssueBelt()
	belt.Stages[0].Fallback = FallbackRetry
	fallback := engineStartRun(belt)
	engineFinishWithoutOutput(&fallback)
	if fallback.StageID != "triage" || fallback.State != RunStateQueued || fallback.Replays != nil {
		t.Errorf("fallback: stage %q state %q replays %q", fallback.StageID, fallback.State, engineStr(fallback.Replays))
	}

	edited := engineSampleRun()
	engineFinishOutcome(&edited, "ready to spec")
	EngineAdvance(&edited, engineSessionStart("s"), engineT0)
	subject := edited.Subject
	subject.Body = "Now with steps."
	EngineAdvance(&edited, Event{Kind: EventKindSubjectChanged, Subject: subject}, engineT0)
	if edited.StageID != "triage" || edited.State != RunStateQueued || edited.Replays != nil {
		t.Errorf("edit: stage %q state %q replays %q", edited.StageID, edited.State, engineStr(edited.Replays))
	}
	if len(edited.ClosedAttempts("triage", nil)) == 0 {
		t.Error("the closed triage attempt is gone")
	}

	fresh := engineHeldAtBuild()
	previous := fresh.LastAttempt("spec")
	ev := EventResume(new("spec"), new("Use semicolons."), nil)
	ev.Earlier = false
	EngineAdvance(&fresh, ev, engineT0)
	if fresh.StageID != "spec" || fresh.Replays != nil || previous == nil {
		t.Errorf("fresh resume: stage %q replays %q", fresh.StageID, engineStr(fresh.Replays))
	}
	if strings.Contains(BriefRender(fresh, *fresh.Stage(), engineRerunContext("02-spec-1")), "## What this stage already had") {
		t.Error("a fresh resume carries earlier work")
	}
}
