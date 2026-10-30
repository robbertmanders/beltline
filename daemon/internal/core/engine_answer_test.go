// Tests of answering a stage's questions, and of the run's updated time.

package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TestAnAnswerContinuesTheHeldSession: answering a held triage continues its session,
// and the next verdict moves the run on.
func TestAnAnswerContinuesTheHeldSession(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "needs info")
	if answerable := run.Answerable(); answerable == nil || answerable.ID != "triage-1" {
		t.Fatalf("answerable = %+v", answerable)
	}
	if effects := EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("The invoices page.")}, engineT0); len(effects) != 0 {
		t.Fatalf("effects = %+v, want none", effects)
	}
	if run.State != RunStateQueued || run.StageID != "triage" || run.Reason != nil {
		t.Errorf("answered: state %q stage %q reason %q", run.State, run.StageID, engineStr(run.Reason))
	}
	if engineStr(run.Note) != "The invoices page." || engineStr(run.Continues) != "triage-1" {
		t.Errorf("note %q continues %q", engineStr(run.Note), engineStr(run.Continues))
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "bl-test-2", SessionID: "id", Folder: "g"}, engineT0)
	second := run.Attempts[1]
	if engineStr(second.Continues) != "triage-1" || engineStr(second.AgentName) != "bl-test" || engineStr(second.AgentSessionName()) != "bl-test" {
		t.Errorf("second attempt continues %q agent %q session %q", engineStr(second.Continues), engineStr(second.AgentName), engineStr(second.AgentSessionName()))
	}
	if engineStr(second.Note) != "The invoices page." || run.Continues != nil {
		t.Errorf("second note %q, run continues %q", engineStr(second.Note), engineStr(run.Continues))
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Output: new("Clear now.\n\nOutcome: ready to spec")}, engineT0)
	if run.StageID != "spec" {
		t.Errorf("after the verdict: stage %q, want spec", run.StageID)
	}
}

// TestEveryAgentStageThatHoldsCanBeAnswered: any held agent stage takes an answer,
// and so does a session that failed.
func TestEveryAgentStageThatHoldsCanBeAnswered(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "open questions")
	if answerable := run.Answerable(); answerable == nil || answerable.StageID != "spec" {
		t.Fatalf("answerable = %+v", answerable)
	}
	EngineAdvance(&run, Event{Kind: EventKindAnswered}, engineT0)
	if run.State != RunStateQueued || run.Note != nil {
		t.Errorf("answered without a message: state %q note %q", run.State, engineStr(run.Note))
	}
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "bl-test-3", SessionID: "id", Folder: "h"}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Detail: new("the agent stopped")}, engineT0)
	if run.State != RunStateHeld || run.Answerable() == nil {
		t.Errorf("failed session: state %q answerable %+v", run.State, run.Answerable())
	}
}

// TestOnlyAHeldAgentStageTakesAnAnswer: an answer to a waiting approval changes
// nothing, and a closed watch has nothing to answer.
func TestOnlyAHeldAgentStageTakesAnAnswer(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to implement")
	engineFinishOutcome(&run, "complete")
	if run.State != RunStateWaiting {
		t.Fatalf("state = %q, want waiting", run.State)
	}
	before := run.Clone()
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("x")}, engineT0)
	if !reflect.DeepEqual(run, before) {
		t.Errorf("an answer to a waiting approval changed the run:\n%s", testkit.Diff(before, run))
	}
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	engineFinishOutcome(&run, "done")
	engineFinishOutcome(&run, "pass")
	engineFinishOutcome(&run, "opened")
	EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventClosed, Detail: new("#201")}, engineT0)
	if run.State != RunStateHeld || run.Answerable() != nil {
		t.Errorf("closed watch: state %q answerable %+v", run.State, run.Answerable())
	}
}

// TestRunningItAgainStartsANewSession: a resume after a hold starts a new session,
// not a continuation.
func TestRunningItAgainStartsANewSession(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "needs info")
	EngineAdvance(&run, EventResume(nil, nil, nil), engineT0)
	if run.Continues != nil {
		t.Errorf("continues = %q, want nil", *run.Continues)
	}
}

// TestTheFollowUpBriefCarriesTheAnswerAndTheOutcomes: the follow-up brief quotes the
// answer and the outcomes, ends with the footer, and has no issue or artifacts section
// unless the stage asks for artifacts.
func TestTheFollowUpBriefCarriesTheAnswerAndTheOutcomes(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	engineFinishOutcome(&run, "needs info")
	EngineAdvance(&run, Event{Kind: EventKindAnswered, Answer: new("The invoices page.")}, engineT0)
	context := BriefContext{RunFolder: "/tmp/run", AttemptFolder: "/tmp/run/01-triage-2", Base: "main", Branch: "b"}
	stage := *run.Stage()
	held := run.Attempts[0]
	text := BriefFollowUp(run, stage, held, context)
	for _, want := range []string{
		"You stopped at “needs info”", "## Their answer\n\nThe invoices page.",
		"End /tmp/run/01-triage-2/triage.md with one line", "- ready to spec:", "\nOutcome: ready to spec\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("follow-up lacks %q", want)
		}
	}
	if !strings.HasSuffix(text, BriefFooter(stage, "/tmp/run/01-triage-2/triage.md")) {
		t.Error("follow-up does not end with the footer")
	}
	if strings.Contains(text, "## The issue") || strings.Contains(text, "## Artifacts") {
		t.Error("follow-up has an issue or artifacts section")
	}

	stage.Brief = "Diagram in {artifacts}"
	withArtifacts := BriefFollowUp(run, stage, held, context)
	if !strings.Contains(withArtifacts, briefArtifacts("/tmp/run/01-triage-2/artifacts")) {
		t.Error("follow-up lacks the artifacts section")
	}
	if !strings.HasSuffix(withArtifacts, BriefFooter(stage, "/tmp/run/01-triage-2/triage.md")) {
		t.Error("artifacts follow-up does not end with the footer")
	}

	agent := SampleAgent("triage")
	agent.Brief = stage.Brief
	inherited := context
	inherited.Agents = map[AgentRef]Agent{agent.Ref(): agent}
	stage.Brief = ""
	if got := BriefFollowUp(run, stage, held, inherited); got != withArtifacts {
		t.Errorf("inherited brief differs:\n%s\n---\n%s", got, withArtifacts)
	}
}

// TestArtifactsKeepEarlierNamesAndIgnoreFencedExamples: {artifacts} is filled only
// outside code fences, and a run from before stage keys keeps its old meaning.
func TestArtifactsKeepEarlierNamesAndIgnoreFencedExamples(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	context := BriefContext{RunFolder: "/tmp/run", AttemptFolder: "/tmp/run/01-triage-1", Base: "main", Branch: "b"}
	stage := *run.Stage()
	stage.Brief = "An example brief:\n\n```\nDiagram in {artifacts}\n```\n"
	if strings.Contains(BriefRender(run, stage, context), "## Artifacts") {
		t.Error("a fenced {artifacts} produced an artifacts section")
	}
	stage.Brief = "Diagram in {artifacts}"
	if !strings.Contains(BriefRender(run, stage, context), "Diagram in /tmp/run/01-triage-1/artifacts") {
		t.Error("{artifacts} is not filled")
	}
	// A new run keeps the folder meaning even when a stage id is artifacts.
	run.Stages[1].ID = "artifacts"
	text := BriefRender(run, stage, context)
	if !strings.Contains(text, "## Artifacts") || !strings.Contains(text, "Diagram in /tmp/run/01-triage-1/artifacts") {
		t.Errorf("new run lost the artifacts meaning:\n%s", text)
	}
	// A run from before stage keys still lets that id keep the output meaning.
	legacy := run.Clone()
	legacy.References = nil
	for index := range legacy.Stages {
		legacy.Stages[index].Key = nil
	}
	earlier := BriefRender(legacy, stage, context)
	if strings.Contains(earlier, "## Artifacts") || strings.Contains(earlier, "Diagram in /tmp/run/01-triage-1/artifacts") {
		t.Errorf("legacy run gained the artifacts meaning:\n%s", earlier)
	}
	held := NewAttempt(AttemptArgs{StageID: stage.ID, Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Folder: "01-triage-1"})
	if strings.Contains(BriefFollowUp(legacy, stage, held, context), "## Artifacts") {
		t.Error("legacy follow-up has an artifacts section")
	}
}

// TestAnEventThatChangesNothingKeepsTheTime: an event that changes nothing keeps the
// run's updated time; one that changes the run moves it.
func TestAnEventThatChangesNothingKeepsTheTime(t *testing.T) {
	t.Parallel()
	run := engineSampleRun()
	later := engineT0.Add(600 * time.Second)
	EngineAdvance(&run, Event{Kind: EventKindSubjectChanged, Subject: run.Subject}, later)
	if !run.UpdatedAt.Equal(engineT0) {
		t.Errorf("updatedAt = %v, want %v", run.UpdatedAt.Time, engineT0)
	}
	EngineAdvance(&run, engineSessionStart("s"), later)
	if !run.UpdatedAt.Equal(later) {
		t.Errorf("updatedAt = %v, want %v", run.UpdatedAt.Time, later)
	}
}
