// Contract tests of the run collection, task, continue-session, repeated-command and erase-recovery
// cases.

package servertest

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
)

// TestCollectionAndDetailCarryTheSameActionsForASnapshot
func TestCollectionAndDetailCarryTheSameActionsForASnapshot(t *testing.T) {
	t.Parallel()
	f := ReadOnlyFixture(t)
	list := getCase(f, casePath("runs", "", ""))
	var response api.RunsResponse
	if err := routes.JSONDecode(list.Body, &response); err != nil {
		t.Fatalf("runs: %v", err)
	}
	hasErase := false
	for _, snapshot := range response.Runs {
		for _, action := range snapshot.Actions {
			if action.ID == string(core.RunCommandErase) {
				hasErase = true
			}
		}
	}
	if !hasErase {
		t.Fatal("no run lists the erase action")
	}
	for _, snapshot := range response.Runs {
		detail := getCase(f, casePath("run", snapshot.Run.ID, ""))
		one := decodeRunActions(t, detail)
		if !reflect.DeepEqual(one.Actions, snapshot.Actions) {
			t.Fatalf("run %s: detail actions differ from the list's", snapshot.Run.ID)
		}
	}
	again := getCase(f, casePath("runs", "", ""))
	var repeat api.RunsResponse
	if err := routes.JSONDecode(again.Body, &repeat); err != nil {
		t.Fatalf("runs again: %v", err)
	}
	if !reflect.DeepEqual(repeat, response) {
		t.Fatal("the list changed between two reads of the same state")
	}
}

// decodeRunActions is the run resource of a RunResponse reply, with its actions.
func decodeRunActions(t *testing.T, reply Reply) api.RunSnapshot {
	t.Helper()
	var response api.RunResponse
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("run response: %v", err)
	}
	return response.Run
}

// manualRunCount is how many saved runs are tasks you started (the intake keeps starting issue runs).
func manualRunCount(t *testing.T, f *Fixture) int {
	t.Helper()
	count := 0
	for _, run := range store.NewStorage(store.Paths{Root: f.Root}).LoadRuns() {
		if run.Subject.Kind == core.SubjectKindManual {
			count++
		}
	}
	return count
}

// TestTasksCreateRunsAndRefuseInvalidInput
func TestTasksCreateRunsAndRefuseInvalidInput(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	belt, entry := manualBeltEntry(t, f)
	repo := entry.Repos[0]
	taskPath := casePath("tasks", "", "")

	reply := postRunCase(f, taskPath, mustJSON(t, api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: repo, Title: "  A task  ", Body: "  Body  "}))
	checkRunCommand(t, f, reply, "tasks")
	var created api.RunResponse
	if err := routes.JSONDecode(reply.Body, &created); err != nil {
		t.Fatalf("task: %v", err)
	}
	run := created.Run.Run
	if run.Subject.Kind != core.SubjectKindManual || run.Subject.Title != "A task" || run.Subject.Body != "Body" {
		t.Fatalf("subject %+v, want the trimmed manual task", run.Subject)
	}
	if run.Source == nil || run.Source.Entry != entry.ID {
		t.Fatalf("source %+v, want entry %s", run.Source, entry.ID)
	}
	if stageKind(run) != core.StageKindAgent {
		t.Fatalf("the task's stage kind %s, want agent", stageKind(run))
	}
	if len(created.Run.Actions) == 0 {
		t.Fatal("the task carries no actions")
	}
	reloaded := store.NewStorage(store.Paths{Root: f.Root}).LoadRuns()
	if !slices.ContainsFunc(reloaded, func(candidate core.Run) bool { return candidate.ID == run.ID }) {
		t.Fatal("the task is not saved")
	}

	second := postRunCase(f, taskPath, mustJSON(t, api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: repo, Title: "Second task", Body: ""}))
	if second.Status != http.StatusOK {
		t.Fatalf("second task: HTTP %d", second.Status)
	}
	if decodeRunReply(t, second).ID == run.ID {
		t.Fatal("the second task reused the first run's id")
	}

	concurrentBody := mustJSON(t, api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: repo, Title: "Concurrent task", Body: ""})
	concurrent := postTwice(f, taskPath, concurrentBody)
	for _, reply := range concurrent {
		if reply.Status != http.StatusOK {
			t.Fatalf("concurrent task: HTTP %d", reply.Status)
		}
	}
	first, other := decodeRunReply(t, concurrent[0]), decodeRunReply(t, concurrent[1])
	if first.ID == other.ID {
		t.Fatal("two concurrent tasks got one run")
	}
	for _, got := range []core.Run{first, other} {
		if got.Subject.Title != "Concurrent task" || got.Source == nil || got.Source.Entry != entry.ID {
			t.Fatalf("concurrent task %s: %+v", got.ID, got.Subject)
		}
	}
	saved := 0
	for _, candidate := range f.Coordinator.ReadRuns() {
		if candidate.Subject.Title == "Concurrent task" && candidate.Source != nil && candidate.Source.Entry == entry.ID {
			saved++
		}
	}
	if saved != 2 {
		t.Fatalf("%d saved concurrent tasks, want 2", saved)
	}
	third := postRunCase(f, taskPath, concurrentBody)
	if third.Status != http.StatusOK {
		t.Fatalf("third task: HTTP %d", third.Status)
	}
	thirdRun := decodeRunReply(t, third)
	if thirdRun.ID == first.ID || thirdRun.ID == other.ID {
		t.Fatal("the third task reused a concurrent task's id")
	}
	if got := f.Coordinator.ReadRun(thirdRun.ID); got == nil || got.Subject.Title != "Concurrent task" {
		t.Fatal("the third task is not readable by id")
	}

	refusals := []struct {
		request api.RunTaskRequest
		message string
	}{
		{api.RunTaskRequest{Belt: belt.ID, Entry: new("nope"), Repo: repo, Title: "T", Body: ""}, "This belt doesn't take tasks you start."},
		{api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: "acme/other", Title: "T", Body: ""}, "Pick one of the belt's repositories."},
		{api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: repo, Title: "   ", Body: ""}, "Give the task a title."},
		{api.RunTaskRequest{Belt: "issues", Repo: repo, Title: "T", Body: ""}, "This belt doesn't take tasks you start."},
	}
	tasksBefore := manualRunCount(t, f)
	for _, refusal := range refusals {
		refused := postRunCase(f, taskPath, mustJSON(t, refusal.request))
		if refused.Status != http.StatusUnprocessableEntity || errorMessage(t, refused) != refusal.message {
			t.Fatalf("refusal %q: HTTP %d %q", refusal.message, refused.Status, errorMessage(t, refused))
		}
	}
	if manualRunCount(t, f) != tasksBefore {
		t.Fatal("a refused start wrote a run")
	}

	f.Coordinator.SetGloballyPaused(context.Background(), true)
	paused := postRunCase(f, taskPath, mustJSON(t, api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: repo, Title: "Paused task", Body: ""}))
	if paused.Status != http.StatusUnprocessableEntity || errorMessage(t, paused) != "Everything is paused." {
		t.Fatalf("paused: HTTP %d %q", paused.Status, errorMessage(t, paused))
	}
	f.Coordinator.SetGloballyPaused(context.Background(), false)

	// Several manual entries need an explicit choice; each accepted submission is another task.
	two := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	two.ID = "contract-two"
	two.Name = "Two manuals"
	two.Version = 0
	two.Triggers = []core.TriggerEntry{
		{ID: "one", Trigger: core.Trigger{Kind: core.TriggerKindManual}, Repos: []string{"acme/billing"}},
		{ID: "two", Trigger: core.Trigger{Kind: core.TriggerKindManual}, Repos: []string{"acme/billing"}},
	}
	if _, err := f.Coordinator.SaveBelt(two); err != nil {
		t.Fatalf("save belt: %v", err)
	}
	unnamed := postRunCase(f, taskPath, mustJSON(t, api.RunTaskRequest{Belt: "contract-two", Repo: "acme/billing", Title: "T", Body: ""}))
	if unnamed.Status != http.StatusUnprocessableEntity || errorMessage(t, unnamed) != "Pick a trigger." {
		t.Fatalf("unnamed entry: HTTP %d %q", unnamed.Status, errorMessage(t, unnamed))
	}
	chosen := postRunCase(f, taskPath, mustJSON(t, api.RunTaskRequest{Belt: "contract-two", Entry: new("two"), Repo: "acme/billing", Title: "Chosen", Body: ""}))
	if chosen.Status != http.StatusOK {
		t.Fatalf("chosen entry: HTTP %d", chosen.Status)
	}
	if got := decodeRunReply(t, chosen); got.Source == nil || got.Source.Entry != "two" {
		t.Fatalf("chosen source %+v, want two", got.Source)
	}

	if missingBody := postRunCase(f, taskPath, []byte{}); missingBody.Status != http.StatusBadRequest {
		t.Fatalf("a task with no body: HTTP %d, want 400", missingBody.Status)
	}
}

// TestPausedAndArchivedBeltsRefuseTaskCreation
func TestPausedAndArchivedBeltsRefuseTaskCreation(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	belt, entry := manualBeltEntry(t, f)
	if err := f.Coordinator.SetPaused(belt.ID, true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	paused := postRunCase(f, casePath("tasks", "", ""), mustJSON(t, api.RunTaskRequest{Belt: belt.ID, Entry: new(entry.ID), Repo: entry.Repos[0], Title: "T", Body: ""}))
	if paused.Status != http.StatusUnprocessableEntity || errorMessage(t, paused) != "This belt is paused or archived." {
		t.Fatalf("paused belt: HTTP %d %q", paused.Status, errorMessage(t, paused))
	}
}

// TestContinueSessionLaunchesAndReusesTheWritableConversation
func TestContinueSessionLaunchesAndReusesTheWritableConversation(t *testing.T) {
	t.Parallel()
	shell := &continueShell{}
	f := Start(t, Options{Runner: runner.NewTmuxRunner(shell)})
	run := findRun(t, f.Coordinator, "held with an answerable session", func(run core.Run) bool {
		return run.State == core.RunStateHeld && run.Answerable() != nil
	})
	attempt := *run.Answerable()
	makeContinueCheckouts(t, f, run, attempt)
	body := mustJSON(t, api.RunAttemptRequest{Attempt: attempt.ID})
	path := casePath("runContinueSession", run.ID, "")

	// The second click is refused while the first still launches, or reuses the session once it has
	// launched; either way every accepted click gets the same command and the shell launches once.
	replies := postTwice(f, path, body)
	var accepted []Reply
	for _, reply := range replies {
		if reply.Status != http.StatusOK && reply.Status != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected response %d", reply.Status)
		}
		if reply.Status == http.StatusUnprocessableEntity && errorMessage(t, reply) == "" {
			t.Fatal("a refusal omitted its reason")
		}
		if reply.Status == http.StatusOK {
			accepted = append(accepted, reply)
		}
	}
	if len(accepted) < 1 || len(accepted) > 2 {
		t.Fatalf("expected one or two accepted clicks, got %d", len(accepted))
	}
	reply := accepted[0]
	if reply.Header.Get("Content-Type") != "application/json" || reply.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers %q %q", reply.Header.Get("Content-Type"), reply.Header.Get("Cache-Control"))
	}
	var response api.ContinueSessionResponse
	if err := routes.JSONDecode(reply.Body, &response); err != nil {
		t.Fatalf("continue response: %v", err)
	}
	for _, other := range accepted {
		var got api.ContinueSessionResponse
		if err := routes.JSONDecode(other.Body, &got); err != nil || !reflect.DeepEqual(got.Command, response.Command) {
			t.Fatalf("accepted clicks disagree on the command: %v", err)
		}
	}
	if len(response.Command) == 0 || response.Command[0] != "tmux" {
		t.Fatalf("command %v, want tmux", response.Command)
	}
	if !slices.Contains(response.Command, "attach-session") {
		t.Fatalf("command %v does not attach a session", response.Command)
	}
	name := response.Command[len(response.Command)-1]
	if len(name) < len("-continue") || name[len(name)-len("-continue"):] != "-continue" {
		t.Fatalf("attached session %q, want a -continue conversation", name)
	}

	again := postRunCase(f, path, body)
	if again.Status != http.StatusOK {
		t.Fatalf("again: HTTP %d", again.Status)
	}
	var repeat api.ContinueSessionResponse
	if err := routes.JSONDecode(again.Body, &repeat); err != nil || !reflect.DeepEqual(repeat.Command, response.Command) {
		t.Fatalf("a reused session answered another command: %v", err)
	}
	CheckGolden(t, "runContinueSession", caseEndpoint("runContinueSession").GoldenName, again, f.Root)
	if got := len(shell.launched); got != 1 {
		t.Fatalf("the shell launched %d sessions, want 1", got)
	}

	foreign := postRunCase(f, path, mustJSON(t, api.RunAttemptRequest{Attempt: "missing"}))
	if foreign.Status != http.StatusUnprocessableEntity || errorMessage(t, foreign) != "This run has no saved attempt with that id." {
		t.Fatalf("a foreign attempt: HTTP %d %q", foreign.Status, errorMessage(t, foreign))
	}

	// A terminal run refuses Continue even when the attempt exists.
	terminal := findRun(t, f.Coordinator, "finished with a session", func(run core.Run) bool {
		return !run.State.IsActive() && hasSessionAttempt(run)
	})
	var terminalAttempt core.Attempt
	for _, candidate := range terminal.Attempts {
		if candidate.Session != nil {
			terminalAttempt = candidate
			break
		}
	}
	terminalReply := postRunCase(f, casePath("runContinueSession", terminal.ID, ""), mustJSON(t, api.RunAttemptRequest{Attempt: terminalAttempt.ID}))
	if terminalReply.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a terminal run: HTTP %d, want 422", terminalReply.Status)
	}
	want := core.RunAvailabilityRefusal(core.RunCommandContinueSession, f.Coordinator.RunActionContext(*f.Coordinator.ReadRun(terminal.ID)), core.RunActionTarget{})
	if want == nil || errorMessage(t, terminalReply) != *want {
		t.Fatalf("message %q, want %v", errorMessage(t, terminalReply), want)
	}
}

// TestRepeatedAdjustmentIsANoOpNotASecondRecord checks that an adjustment repeated unchanged is accepted
// without a second record.
func TestRepeatedAdjustmentIsANoOpNotASecondRecord(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	adjustable := findRun(t, f.Coordinator, "queued at an agent stage", func(run core.Run) bool {
		return run.State == core.RunStateQueued && stageKind(run) == core.StageKindAgent
	})
	stage := adjustable.Stage()
	models := f.Coordinator.AdjustmentModels(adjustable, *stage, nil)
	if len(models) == 0 {
		t.Fatal("the stage has no admitted model")
	}
	request := mustJSON(t, api.RunAdjustStageRequest{Stage: stage.ID, Model: models[0].ID})
	path := casePath("runAdjustStage", adjustable.ID, "")
	if first := postRunCase(f, path, request); first.Status != http.StatusOK {
		t.Fatalf("first adjustment: HTTP %d", first.Status)
	}
	before := len(f.Coordinator.ReadRun(adjustable.ID).Adjustments)
	if repeat := postRunCase(f, path, request); repeat.Status != http.StatusOK {
		t.Fatalf("repeated adjustment: HTTP %d", repeat.Status)
	}
	if got := len(f.Coordinator.ReadRun(adjustable.ID).Adjustments); got != before {
		t.Fatalf("adjustments %d, want %d", got, before)
	}
}

// TestEraseAllowsAnErasedIDAgainAndGovernsReplacementWork
func TestEraseAllowsAnErasedIDAgainAndGovernsReplacementWork(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	run := findRun(t, f.Coordinator, "finished", func(run core.Run) bool { return !run.State.IsActive() })
	belt := f.Coordinator.ReadBelt(run.BeltID)
	if belt == nil {
		t.Fatal("the run's belt is gone")
	}
	subject := run.Subject
	path := casePath("runErase", run.ID, "")
	if erased := postRunCase(f, path, nil); erased.Status != http.StatusOK {
		t.Fatalf("erase: HTTP %d", erased.Status)
	}
	if f.Coordinator.ReadRun(run.ID) != nil {
		t.Fatal("the erased run is still there")
	}
	repeated := postRunCase(f, path, nil)
	if repeated.Status != http.StatusOK {
		t.Fatalf("repeated erase: HTTP %d", repeated.Status)
	}
	var erased api.EraseResponse
	if err := routes.JSONDecode(repeated.Body, &erased); err != nil || !erased.Erased {
		t.Fatalf("repeated erase did not answer erased: %v", err)
	}

	unknown := postRunCase(f, casePath("runErase", "never-existed", ""), nil)
	if unknown.Status != http.StatusNotFound {
		t.Fatalf("an unknown run: HTTP %d, want 404", unknown.Status)
	}
	CheckBody(t, "unknown erase", "error-not-found.json", unknown.Body, f.Root)

	// Replacement work with the same id is governed by its own state.
	replacement := core.NewRun(core.RunArgs{Belt: belt.Belt, Subject: subject, Now: ContractNow.Add(60 * time.Second)})
	f.Coordinator.AppendRunForContract(replacement)
	if f.Coordinator.ReadRun(run.ID) == nil {
		t.Fatal("the replacement is not readable by the erased id")
	}
	replacementErase := postRunCase(f, path, nil)
	if replacementErase.Status != http.StatusUnprocessableEntity || errorMessage(t, replacementErase) != "Drop the run before erasing it." {
		t.Fatalf("replacement erase: HTTP %d %q", replacementErase.Status, errorMessage(t, replacementErase))
	}
	if f.Coordinator.ReadRun(run.ID) == nil {
		t.Fatal("a refused erase removed the replacement")
	}
}
