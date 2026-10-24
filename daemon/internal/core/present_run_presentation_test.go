// The sample run is the one the
// engine tests build (engineSampleRun).

package core

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// rpStageNamed is the first stage of run with the given id, nil when there is none.
func rpStageNamed(view RunPresentationSnapshot, id string) *RunPresentationStagePanel {
	for i := range view.Stages {
		if view.Stages[i].ID == id {
			return &view.Stages[i]
		}
	}
	return nil
}

// TestProjectionSuppliesEveryStageAndKeepsActionsUnchanged
func TestProjectionSuppliesEveryStageAndKeepsActionsUnchanged(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	action := RunAction{ID: "park", Enabled: true}
	view := RunPresentationProject(run, false, PauseLive, engineT0, nil, []RunAction{action})
	if view.RunID != run.ID {
		t.Fatalf("run id = %q, want %q", view.RunID, run.ID)
	}
	if view.DefaultStage != run.StageID {
		t.Fatalf("default stage = %q, want %q", view.DefaultStage, run.StageID)
	}
	if want := "Belt v1 · " + strconv.Itoa(len(run.Stages)) + " stages"; view.Metadata != want {
		t.Fatalf("metadata = %q, want %q", view.Metadata, want)
	}
	var ids []string
	for _, stage := range view.Stages {
		ids = append(ids, stage.ID)
	}
	var want []string
	for _, stage := range run.Stages {
		want = append(want, stage.ID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("stage ids = %v, want %v", ids, want)
	}
	if !reflect.DeepEqual(view.Actions, []RunAction{action}) {
		t.Fatalf("actions = %+v, want %+v", view.Actions, []RunAction{action})
	}
	if first := view.Stages[0].EmptyMessage; first == nil || !strings.HasPrefix(*first, "Queued.") {
		t.Fatalf("first stage empty message = %v", first)
	}
	if last := view.Stages[len(view.Stages)-1].EmptyMessage; last == nil || !strings.Contains(*last, "hasn't reached") {
		t.Fatalf("last stage empty message = %v", last)
	}
}

// TestHistoricalSelectionRetainsResultAndNamesTheNewerRunningAttempt
func TestHistoricalSelectionRetainsResultAndNamesTheNewerRunningAttempt(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	earlierEnd := jsonx.Date{Time: engineT0.Add(15 * time.Second)}
	earlierNote, earlierOutcome, earlierDetail := "Earlier note", "ready", "The saved result"
	earlierTone := ToneGood
	earlier := NewAttempt(AttemptArgs{StageID: run.StageID, Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Note: &earlierNote, Folder: "one"})
	earlier.EndedAt = &earlierEnd
	earlier.Outcome = &earlierOutcome
	earlier.Tone = &earlierTone
	earlier.Detail = &earlierDetail
	current := NewAttempt(AttemptArgs{StageID: run.StageID, Number: 2, StartedAt: jsonx.Date{Time: engineT0.Add(20 * time.Second)}, Folder: "two"})
	session := "live-session"
	current.Session = &session
	run.Attempts = []Attempt{earlier, current}
	run.State = RunStateRunning
	running := RunStateRunning
	run.Legs[0].State = running
	view := RunPresentationProject(run, false, PauseLive, engineT0.Add(25*time.Second), nil, nil)
	stage := rpStageNamed(view, run.StageID)
	if stage == nil {
		t.Fatalf("no panel for stage %s", run.StageID)
	}
	if stage.DefaultAttempt == nil || *stage.DefaultAttempt != current.ID {
		t.Fatalf("default attempt = %v, want %s", stage.DefaultAttempt, current.ID)
	}
	var contents []RunPresentationAttemptEntryContent
	for _, entry := range stage.Attempts {
		contents = append(contents, entry.Content)
	}
	if !reflect.DeepEqual(contents, []RunPresentationAttemptEntryContent{RunPresentationAttemptEntryContentHistoricalOutput, RunPresentationAttemptEntryContentRunning}) {
		t.Fatalf("contents = %v", contents)
	}
	if first := stage.Attempts[0]; first.RunningAttempt == nil || *first.RunningAttempt != 2 {
		t.Fatalf("running attempt = %v, want 2", first.RunningAttempt)
	}
	if first := stage.Attempts[0]; first.Outcome == nil || *first.Outcome != "ready" {
		t.Fatalf("outcome = %v", first.Outcome)
	}
	if first := stage.Attempts[0]; first.Note == nil || *first.Note != "Earlier note" {
		t.Fatalf("note = %v", first.Note)
	}
	if first := stage.Attempts[0]; first.Detail == nil || *first.Detail != "The saved result" {
		t.Fatalf("detail = %v", first.Detail)
	}
}

// TestGatePanelsUseDecisionStateAndEveryEmptyMessageComesFromPresentation
func TestGatePanelsUseDecisionStateAndEveryEmptyMessageComesFromPresentation(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	run.StageID = "approve"
	run.State = RunStateWaiting
	run.Legs[0].StageID = new("approve")
	run.Legs[0].State = RunStateWaiting
	note := "Ready to approve"
	attempt := NewAttempt(AttemptArgs{StageID: "approve", Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Note: &note, Folder: "gate"})
	run.Attempts = []Attempt{attempt}
	view := RunPresentationProject(run, false, PauseLive, engineT0, nil, nil)
	gate := rpStageNamed(view, "approve")
	if gate == nil {
		t.Fatal("no gate panel")
	}
	if len(gate.Attempts) == 0 || gate.Attempts[0].Content != RunPresentationAttemptEntryContentDecision {
		t.Fatalf("gate attempts = %+v, want a decision", gate.Attempts)
	}
	if gate.DefaultAttempt == nil || *gate.DefaultAttempt != attempt.ID {
		t.Fatalf("gate default attempt = %v, want %s", gate.DefaultAttempt, attempt.ID)
	}
	if gate.Status.Text != "WAITING FOR YOU" {
		t.Fatalf("gate status = %q", gate.Status.Text)
	}
	last := run.Stages[len(run.Stages)-1]
	if got := RunPresentationEmptyMessage(last, run, StatusStepQueued); got != "Queued. It starts when "+last.Name+" is free." {
		t.Fatalf("empty message = %q", got)
	}
}

// TestLivePullRequestNotesAndTicketWatchSummariesAreShared
func TestLivePullRequestNotesAndTicketWatchSummariesAreShared(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	watchIndex := -1
	for i, stage := range run.Stages {
		if stage.Kind() == StageKindWatch {
			watchIndex = i
			break
		}
	}
	if watchIndex < 0 {
		t.Fatal("the sample belt has no watch stage")
	}
	watch := run.Stages[watchIndex]
	run.StageID = watch.ID
	run.State = RunStateRunning
	run.Legs[0].StageID = new(watch.ID)
	run.Legs[0].State = RunStateRunning
	head := "head"
	checks := PullRequestCheckStateFailing
	failures, total, behind := 2, 5, 3
	conflicted := true
	run.PullRequests = []PullRequest{{
		Number: 8, URL: "https://github.com/acme/billing/pull/8", Repo: "acme/billing",
		State: PullRequestStateOpen, HeadSha: &head, Checks: &checks, CheckFailures: &failures, CheckTotal: &total,
		Conflicted: &conflicted, Behind: &behind,
		Reviews: []PullRequestReview{{Author: "Mina", State: PullRequestReviewStateChangesRequested, Commit: &head}},
	}}
	notes := RunPresentationWatchNotes(run, nil)
	if len(notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(notes))
	}
	if notes[0].Reference != "billing#8" {
		t.Fatalf("reference = %q", notes[0].Reference)
	}
	for _, fragment := range []string{"Mina asked for changes", "merge conflicts", "2 of 5 checks failing", "3 commits behind its base"} {
		if !strings.Contains(notes[0].Note, fragment) {
			t.Fatalf("note %q does not contain %q", notes[0].Note, fragment)
		}
	}
	if summary := RunPresentationWatchingSummary(run, watch, nil); !strings.Contains(summary, "Right now:") {
		t.Fatalf("watching summary = %q, want Right now:", summary)
	}

	ticket := watch
	target := WatchTargetTicket
	ticket.Watch = &target
	run.TicketObservations = map[string]TicketObservation{watch.ID: {Status: "Open"}}
	if summary := RunPresentationWatchingSummary(run, ticket, nil); !strings.Contains(summary, run.Subject.Ref()) {
		t.Fatalf("ticket summary = %q, want %s", summary, run.Subject.Ref())
	}
}

// TestRunSessionCountCountsAttemptsWithASession pins the Sessions button's count: an attempt with
// no session, or an empty session name, is not counted.
func TestRunSessionCountCountsAttemptsWithASession(t *testing.T) {
	t.Parallel()
	session := "bl-run-1"
	empty := ""
	run := Run{Attempts: []Attempt{{Session: &session}, {}, {Session: &empty}, {Session: &session}}}
	if got := RunSessionCount(run); got != 2 {
		t.Fatalf("session count = %d, want 2", got)
	}
	if got := RunSessionCount(Run{}); got != 0 {
		t.Fatalf("session count without attempts = %d, want 0", got)
	}
}
