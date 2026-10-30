// Tests of the default belts and of the fallback when a belt has no stage for a case.

package core

import (
	"reflect"
	"strings"
	"testing"
)

// engineWatchIndex is the index of the run's watch stage.
func engineWatchIndex(t *testing.T, run Run) int {
	t.Helper()
	for i, stage := range run.Stages {
		if stage.Kind() == StageKindWatch {
			return i
		}
	}
	t.Fatal("the run has no watch stage")
	return -1
}

// engineOutcomeIndex is the index of the outcome of stage whose when is when.
func engineOutcomeIndex(t *testing.T, stage Stage, when string) int {
	t.Helper()
	for i, outcome := range stage.Outcomes {
		if outcome.When == when {
			return i
		}
	}
	t.Fatalf("stage %s has no outcome for %q", stage.ID, when)
	return -1
}

// engineSetNewChanges sets what the watch does on new changes.
func engineSetNewChanges(t *testing.T, run *Run, then Then) {
	t.Helper()
	index := engineWatchIndex(t, *run)
	outcome := engineOutcomeIndex(t, run.Stages[index], string(WatchEventNewChanges))
	run.Stages[index].Outcomes[outcome].Then = then
}

// TestStartsQueuedAtTriage: a new run is queued at triage with every default stage.
func TestStartsQueuedAtTriage(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	if run.State != RunStateQueued || run.StageID != "triage" {
		t.Fatalf("state %q stage %q", run.State, run.StageID)
	}
	want := []string{"triage", "spec", "approve", "build", "verify", "ship", "watch", "wrap-up", "resolve-conflicts", "fix-checks"}
	var got []string
	for _, stage := range run.Stages {
		got = append(got, stage.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stages = %v, want %v", got, want)
	}
}

// TestBothReadyVerdictsMoveToSpec: either ready outcome moves triage on to spec.
func TestBothReadyVerdictsMoveToSpec(t *testing.T) {
	t.Parallel()
	for _, verdict := range []string{"ready to implement", "ready to spec"} {
		run := engineSampleRun()
		engineFinishOutcome(&run, verdict)
		if run.StageID != "spec" || run.State != RunStateQueued {
			t.Errorf("%s: stage %q state %q", verdict, run.StageID, run.State)
		}
		last := run.Attempts[len(run.Attempts)-1]
		if engineStr(last.Outcome) != verdict || engineStr(last.Route) != "Next: Spec" {
			t.Errorf("%s: attempt outcome %q route %q", verdict, engineStr(last.Outcome), engineStr(last.Route))
		}
	}
}

// TestNeedsInfoHoldsWithoutACommentByDefault: a new belt's policies keep the hold
// comment off, so a hold only notifies you.
func TestNeedsInfoHoldsWithoutACommentByDefault(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	effects := engineFinishBody(&run, new("needs info"), "Unclear.\n\n## Open questions\n1. Which page?")
	if run.State != RunStateHeld {
		t.Fatalf("state %q", run.State)
	}
	if engineHasEffect(effects, EffectKindComment) {
		t.Errorf("effects = %+v, want no comment", effects)
	}
	if !engineHasEffect(effects, EffectKindNotify) {
		t.Error("no notification")
	}
}

// TestNeedsInfoHoldsAndCommentsTheQuestions: with the hold comment on, a hold posts
// the open questions, not the notes after them, and notifies you.
func TestNeedsInfoHoldsAndCommentsTheQuestions(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	run.Policies.CommentOnHold = true
	effects := engineFinishBody(&run, new("needs info"), "Unclear.\n\n## Open questions\n1. Which page?\n\n## Notes\nx")
	if run.State != RunStateHeld || engineStr(run.Reason) != "needs info" {
		t.Fatalf("state %q reason %q", run.State, engineStr(run.Reason))
	}
	if len(effects) == 0 || effects[0].Kind != EffectKindComment {
		t.Fatalf("effects = %+v, want a comment first", effects)
	}
	if !strings.Contains(effects[0].Text, "1. Which page?") || strings.Contains(effects[0].Text, "## Notes") {
		t.Errorf("comment = %q", effects[0].Text)
	}
	if !engineHasEffect(effects, EffectKindNotify) {
		t.Error("no notification")
	}
}

// TestHoldCommentsFindQuestionsInEitherOrderAndFallBackToStrippedOutput: the questions
// are found wherever they are in the output, else the output without its outcome line.
func TestHoldCommentsFindQuestionsInEitherOrderAndFallBackToStrippedOutput(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	stage := *run.Stage()
	outcome := stage.Outcome("needs info")
	if outcome == nil {
		t.Fatal("no needs info outcome")
	}
	comment := func(body string) string {
		return engineHoldComment(run.Subject, stage, *outcome, new(body))
	}
	questions := "## Open questions\n1. Which page?\n2. New items only?"
	first := comment("# Title\n\n" + questions + "\n\n## Notes\nx")
	last := comment("# Title\n\n## Notes\nx\n\n" + questions)
	if first != last {
		t.Errorf("questions first %q, last %q", first, last)
	}
	if !strings.HasSuffix(first, "1. Which page?\n2. New items only?") {
		t.Errorf("comment = %q", first)
	}
	body := "# Title\n\n## Notes\nx"
	fallback := comment(body + "\n\nOutcome: needs info")
	if !strings.HasSuffix(fallback, OutcomeParserStrip(body+"\n\nOutcome: needs info")) {
		t.Errorf("fallback = %q", fallback)
	}
	if strings.Contains(fallback, "Outcome:") {
		t.Errorf("fallback keeps the outcome line: %q", fallback)
	}
}

// TestHoldSkipsTheCommentWhenTheRunSaysSo: with comment-on-hold off, a hold posts nothing.
func TestHoldSkipsTheCommentWhenTheRunSaysSo(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	run.Policies.CommentOnHold = false
	effects := engineFinishOutcome(&run, "needs info")
	if engineHasEffect(effects, EffectKindComment) {
		t.Errorf("effects = %+v, want no comment", effects)
	}
}

// TestWaitToImplementParks: a wait outcome parks the run without effects.
func TestWaitToImplementParks(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	effects := engineFinishOutcome(&run, "wait to implement")
	if run.State != RunStateParked {
		t.Errorf("state = %q, want parked", run.State)
	}
	if len(effects) != 0 {
		t.Errorf("effects = %+v, want none", effects)
	}
}

// TestSpecIncompleteRerunsOnceThenHolds: an incomplete spec retries once, then holds.
func TestSpecIncompleteRerunsOnceThenHolds(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	engineFinishOutcome(&run, "incomplete")
	if run.StageID != "spec" || run.State != RunStateQueued {
		t.Fatalf("first retry: stage %q state %q", run.StageID, run.State)
	}
	engineFinishOutcome(&run, "incomplete")
	if run.State != RunStateHeld {
		t.Fatalf("second: state %q, want held", run.State)
	}
	if route := run.Attempts[len(run.Attempts)-1].Route; route == nil || !strings.Contains(*route, "reached its limit") {
		t.Errorf("route = %q", engineStr(route))
	}
}

// TestApprovalWaitsThenApproveMovesToBuild: the approval waits for you and notifies,
// then approving moves on to build.
func TestApprovalWaitsThenApproveMovesToBuild(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	effects := engineFinishOutcome(&run, "complete")
	if run.StageID != "approve" || run.State != RunStateWaiting {
		t.Fatalf("stage %q state %q", run.StageID, run.State)
	}
	if !engineHasEffect(effects, EffectKindNotify) {
		t.Error("no notification")
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	if run.StageID != "build" || run.State != RunStateQueued {
		t.Errorf("after approve: stage %q state %q", run.StageID, run.State)
	}
}

// TestSendBackGoesToSpecWithTheNote: sending back carries the note to the spec's new
// attempt, and the run's own note is cleared once it starts.
func TestSendBackGoesToSpecWithTheNote(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	engineFinishOutcome(&run, "complete")
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "sent back", Note: new("Use the current filter.")}, engineT0)
	if run.StageID != "spec" || engineStr(run.Note) != "Use the current filter." {
		t.Fatalf("stage %q note %q", run.StageID, engineStr(run.Note))
	}
	EngineAdvance(&run, engineSessionStart("s"), engineT0)
	if engineStr(run.Attempts[len(run.Attempts)-1].Note) != "Use the current filter." || run.Note != nil {
		t.Errorf("attempt note %q, run note %q", engineStr(run.Attempts[len(run.Attempts)-1].Note), engineStr(run.Note))
	}
}

// TestCodeMismatchGoesBackToBuildOnceWithTheFindings: verify's code mismatch sends
// build back once, with the findings; a second mismatch holds.
func TestCodeMismatchGoesBackToBuildOnceWithTheFindings(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "complete")
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	engineFinishOutcome(&run, "done")
	if run.StageID != "verify" {
		t.Fatalf("stage %q, want verify", run.StageID)
	}
	engineFinishBody(&run, new("code mismatch"), "rule 3: 500 on empty")
	if run.StageID != "build" || run.Note == nil || !strings.Contains(*run.Note, "rule 3: 500 on empty") {
		t.Fatalf("stage %q note %q", run.StageID, engineStr(run.Note))
	}
	engineFinishOutcome(&run, "done")
	engineFinishOutcome(&run, "code mismatch")
	if run.State != RunStateHeld || run.StageID != "verify" {
		t.Errorf("second mismatch: state %q stage %q", run.State, run.StageID)
	}
}

// TestSpecMismatchHolds: a spec mismatch holds the run with its outcome as the reason.
func TestSpecMismatchHolds(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "complete")
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	engineFinishOutcome(&run, "done")
	engineFinishOutcome(&run, "spec mismatch")
	if run.State != RunStateHeld || engineStr(run.Reason) != "spec mismatch" {
		t.Errorf("state %q reason %q", run.State, engineStr(run.Reason))
	}
}

// TestPassShipsThenWatchesForTheMergeThenWrapsUp: ship opens the pull request, the
// watch sees the merge, and wrap-up finishes the run.
func TestPassShipsThenWatchesForTheMergeThenWrapsUp(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "complete")
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	engineFinishOutcome(&run, "done")
	engineFinishOutcome(&run, "pass")
	if run.StageID != "ship" || run.State != RunStateQueued {
		t.Fatalf("pass: stage %q state %q", run.StageID, run.State)
	}
	opened := engineFinishOutcome(&run, "opened")
	if run.StageID != "watch" || run.State != RunStateRunning || len(opened) != 0 {
		t.Fatalf("opened: stage %q state %q effects %+v", run.StageID, run.State, opened)
	}
	if cur := run.CurrentAttempt(); cur == nil || cur.StageID != "watch" {
		t.Fatalf("current attempt = %+v", cur)
	}
	EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventMerged, Detail: new("#201")}, engineT0)
	if run.StageID != "wrap-up" {
		t.Fatalf("merged: stage %q", run.StageID)
	}
	if last := run.LastAttempt("watch"); last == nil || engineStr(last.Detail) != "#201" {
		t.Errorf("watch attempt detail = %+v", last)
	}
	done := engineFinishOutcome(&run, "done")
	if run.State != RunStateShipped || !reflect.DeepEqual(done, []Effect{{Kind: EffectKindCleanUp}}) {
		t.Errorf("done: state %q effects %+v", run.State, done)
	}
}

// TestAWatchHoldsOnAnEventNoOutcomeListensTo: a watch event with no outcome for it
// holds the run with the event's reason.
func TestAWatchHoldsOnAnEventNoOutcomeListensTo(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineToWatch(&run)
	index := engineWatchIndex(t, run)
	closed := engineOutcomeIndex(t, run.Stages[index], string(WatchEventClosed))
	run.Stages[index].Outcomes = append(run.Stages[index].Outcomes[:closed:closed], run.Stages[index].Outcomes[closed+1:]...)
	EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventClosed, Detail: new("#201")}, engineT0)
	if run.State != RunStateHeld || engineStr(run.Reason) != "the pull request is closed unmerged" {
		t.Errorf("state %q reason %q", run.State, engineStr(run.Reason))
	}
}

// TestNewChangesFollowsTheOutcomeItWasGiven: new changes hold, route, send back, or
// hold once the retry limit is spent, as the outcome says.
func TestNewChangesFollowsTheOutcomeItWasGiven(t *testing.T) {
	t.Parallel()
	const detail = "#201 has new changes"
	held := engineSampleRun()
	engineToWatch(&held)
	EngineAdvance(&held, Event{Kind: EventKindWatched, Watch: WatchEventNewChanges, Detail: new(detail)}, engineT0)
	if held.State != RunStateHeld || engineStr(held.Reason) != "new changes" {
		t.Fatalf("held: state %q reason %q", held.State, engineStr(held.Reason))
	}
	last := held.LastAttempt("watch")
	if last == nil || engineStr(last.Outcome) != "new changes" || ValueOf(last.Tone) != ToneYou || engineStr(last.Detail) != detail {
		t.Errorf("held attempt = %+v", last)
	}

	routed := engineSampleRun()
	engineToWatch(&routed)
	engineSetNewChanges(t, &routed, Then{Action: ThenActionRoute, Routes: []string{"checks"}})
	EngineAdvance(&routed, Event{Kind: EventKindWatched, Watch: WatchEventNewChanges, Detail: new(detail)}, engineT0)
	if routed.StageID != "fix-checks" || routed.State != RunStateQueued {
		t.Errorf("routed: stage %q state %q", routed.StageID, routed.State)
	}

	sent := engineSampleRun()
	engineToWatch(&sent)
	engineSetNewChanges(t, &sent, Then{Action: ThenActionGoto, To: []string{"build"}, WithOutput: true})
	EngineAdvance(&sent, Event{Kind: EventKindWatched, Watch: WatchEventNewChanges, Detail: new(detail)}, engineT0)
	if sent.StageID != "build" || engineStr(sent.Note) != detail || sent.State != RunStateQueued {
		t.Errorf("sent: stage %q note %q state %q", sent.StageID, engineStr(sent.Note), sent.State)
	}

	limited := engineSampleRun()
	engineToWatch(&limited)
	engineSetNewChanges(t, &limited, Then{Action: ThenActionRetry, Max: new(1)})
	EngineAdvance(&limited, Event{Kind: EventKindWatched, Watch: WatchEventNewChanges, Detail: new(detail)}, engineT0)
	if limited.State != RunStateRunning || limited.StageID != "watch" {
		t.Errorf("first retry: state %q stage %q", limited.State, limited.StageID)
	}
	EngineAdvance(&limited, Event{Kind: EventKindWatched, Watch: WatchEventNewChanges, Detail: new(detail)}, engineT0)
	if limited.State != RunStateHeld || engineStr(limited.Reason) != "new changes, 1 time already" || limited.StageID != "watch" {
		t.Errorf("limit: state %q reason %q stage %q", limited.State, engineStr(limited.Reason), limited.StageID)
	}
}

// TestApprovedFollowsTheOutcomeItWasGivenWithoutMerging: an approval holds for you by
// default, and moves on only when its outcome says so; it never ships by itself.
func TestApprovedFollowsTheOutcomeItWasGivenWithoutMerging(t *testing.T) {
	t.Parallel()
	const detail = "#201 is approved by alice"
	held := engineSampleRun()
	engineToWatch(&held)
	EngineAdvance(&held, Event{Kind: EventKindWatched, Watch: WatchEventApproved, Detail: new(detail)}, engineT0)
	if held.State != RunStateHeld || engineStr(held.Reason) != "is approved" || held.StageID != "watch" {
		t.Fatalf("held: state %q reason %q stage %q", held.State, engineStr(held.Reason), held.StageID)
	}
	last := held.LastAttempt("watch")
	if last == nil || engineStr(last.Outcome) != "is approved" || engineStr(last.Route) != "Held for you" || engineStr(last.Detail) != detail {
		t.Errorf("held attempt = %+v", last)
	}
	if held.State == RunStateShipped {
		t.Error("an approval shipped the run")
	}

	onward := engineSampleRun()
	engineToWatch(&onward)
	index := engineWatchIndex(t, onward)
	outcome := engineOutcomeIndex(t, onward.Stages[index], string(WatchEventApproved))
	onward.Stages[index].Outcomes[outcome].Then = Then{Action: ThenActionNext}
	EngineAdvance(&onward, Event{Kind: EventKindWatched, Watch: WatchEventApproved, Detail: new(detail)}, engineT0)
	if onward.StageID != "wrap-up" || onward.State != RunStateQueued {
		t.Errorf("onward: stage %q state %q", onward.StageID, onward.State)
	}
	if last := onward.LastAttempt("watch"); last == nil || engineStr(last.Outcome) != "is approved" {
		t.Errorf("onward watch attempt = %+v", last)
	}
	if onward.State == RunStateShipped {
		t.Error("an approval shipped the run")
	}
}

// TestDeliveryFlowsShipAndAReviewFinishesAsDone checks which bundled flows ship when they finish:
// the Wrap up of an issue, Jira and epic flow, and a manual or scheduled delivery at its merged
// watch. A pull request opened on the way is not shipping, and a posted review is done.
func TestDeliveryFlowsShipAndAReviewFinishesAsDone(t *testing.T) {
	t.Parallel()
	agents := SampleAgents()
	outcomeOf := func(stages []Stage, id, name string) Then {
		t.Helper()
		for _, stage := range stages {
			if stage.ID != id {
				continue
			}
			for _, outcome := range stage.Outcomes {
				if outcome.Name == name {
					return outcome.Then
				}
			}
		}
		t.Fatalf("no outcome %q on stage %q", name, id)
		return Then{}
	}
	issues := SampleBelt([]string{"acme/billing"}, "agent", agents, true).Stages
	if got := outcomeOf(issues, "wrap-up", "done").Action; got != ThenActionShip {
		t.Errorf("issue wrap-up done = %q, want ship", got)
	}
	if got := outcomeOf(issues, "ship", "opened").Action; got != ThenActionNext {
		t.Errorf("opened pull request = %q, want next: opening is not shipping", got)
	}
	if got := outcomeOf(issues, "watch", "merged").Action; got != ThenActionNext {
		t.Errorf("issue merged = %q, want next to wrap up", got)
	}
	if got := outcomeOf(issues, "wrap-up", "blocked").Action; got != ThenActionHold {
		t.Errorf("issue wrap-up blocked = %q, want hold", got)
	}
	if got := outcomeOf(issues, "ship", "failed").Action; got != ThenActionHold {
		t.Errorf("issue ship failed = %q, want hold", got)
	}
	jira := SampleStages(TriggerKindJira, agents, true)
	if got := outcomeOf(jira, "wrap-up", "done").Action; got != ThenActionShip {
		t.Errorf("Jira wrap-up done = %q, want ship", got)
	}
	if got := outcomeOf(jira, "wrap-up", "blocked").Action; got != ThenActionHold {
		t.Errorf("Jira wrap-up blocked = %q, want hold", got)
	}
	if got := outcomeOf(jira, "ship", "opened").Action; got != ThenActionNext {
		t.Errorf("Jira opened pull request = %q, want next: opening is not shipping", got)
	}
	if got := outcomeOf(jira, "ship", "failed").Action; got != ThenActionHold {
		t.Errorf("Jira ship failed = %q, want hold", got)
	}
	if got := outcomeOf(jira, "watch", "merged").Action; got != ThenActionNext {
		t.Errorf("Jira merged = %q, want next to wrap up", got)
	}
	for _, kind := range []TriggerKind{TriggerKindManual, TriggerKindSchedule} {
		stages := SampleStages(kind, agents, true)
		var watch *Stage
		for i := range stages {
			if stages[i].Kind() == StageKindWatch {
				watch = &stages[i]
			}
		}
		if watch == nil {
			t.Fatalf("%s has no watch", kind)
		}
		if got := outcomeOf(stages, watch.ID, "merged").Action; got != ThenActionShip {
			t.Errorf("%s merged = %q, want ship", kind, got)
		}
	}
	reviews := SampleStages(TriggerKindReviews, agents, true)
	if got := outcomeOf(reviews, "post-review", "submitted").Action; got != ThenActionDone {
		t.Errorf("review submitted = %q, want done", got)
	}
	if got := outcomeOf(reviews, "post-review", "failed").Action; got != ThenActionHold {
		t.Errorf("review failed = %q, want hold", got)
	}
	if SampleAgentVersion("wrap-up") != 2 || SampleAgentVersion("post-review") != 2 {
		t.Errorf("wrap-up and post-review ship as version %d and %d, want 2 and 2",
			SampleAgentVersion("wrap-up"), SampleAgentVersion("post-review"))
	}
	if SampleAgentVersion("build") != 1 {
		t.Errorf("build ships as version %d, want 1", SampleAgentVersion("build"))
	}
}
