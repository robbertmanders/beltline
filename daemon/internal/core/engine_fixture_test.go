// Fixtures for the engine tests: the clock, the sample run and its finish helpers,
// and the brief context of the rerun tests, for the engine
// tests. The belt is Defaults.belt(repos: ["acme/billing"]) as SampleBelt builds
// it from the shipped agents.

package core

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// engineT0 is the tests' clock start.
var engineT0 = time.Unix(1_800_000_000, 0).UTC()

// engineIssueBelt is the issues belt the sample runs use:
// Defaults.belt(repos: ["acme/billing"]).
func engineIssueBelt() Belt {
	return SampleBelt([]string{"acme/billing"}, "agent", SampleAgents(), true)
}

// engineIssueSubject is the issue sampleRun works on: acme/billing#142.
func engineIssueSubject() Subject {
	return Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV", Body: "Add an export button."}
}

// engineStartRun is a run of belt for the test issue, started at its first stage.
func engineStartRun(belt Belt) Run {
	run := NewRun(RunArgs{Belt: belt, Subject: engineIssueSubject(), Now: engineT0})
	EngineStart(&run, engineT0)
	return run
}

// engineSampleRun is sampleRun() with the default belt.
func engineSampleRun() Run {
	return engineStartRun(engineIssueBelt())
}

// engineSessionStart is the sessionStarted event for the test session at folder.
func engineSessionStart(folder string) Event {
	return Event{Kind: EventKindSessionStarted, Session: "bl-test", SessionID: "id", Folder: folder}
}

// engineSessionEndsWithoutOutput is the sessionFinished event of a session that
// wrote nothing.
func engineSessionEndsWithoutOutput() Event {
	return Event{Kind: EventKindSessionFinished}
}

// engineFinishWithoutOutput starts the current agent stage's session and ends it
// with no output.
func engineFinishWithoutOutput(run *Run) []Effect {
	EngineAdvance(run, engineSessionStart("f"), engineT0)
	return EngineAdvance(run, engineSessionEndsWithoutOutput(), engineT0)
}

// engineFinishBody starts the current agent stage's session and finishes it with
// outcome (nil: no outcome line).
func engineFinishBody(run *Run, outcome *string, body string) []Effect {
	EngineAdvance(run, engineSessionStart("f"), engineT0)
	var output *string
	if outcome != nil {
		text := body + "\n\nOutcome: " + *outcome
		output = &text
	}
	return EngineAdvance(run, Event{Kind: EventKindSessionFinished, Output: output}, engineT0)
}

// engineFinish is finish(&run, outcome) with the default body.
func engineFinishOutcome(run *Run, outcome string) []Effect {
	return engineFinishBody(run, &outcome, "Some output.")
}

// engineToWatch takes a default-belt run to its watch stage: the Ship agent
// opened the pull request.
func engineToWatch(run *Run) {
	engineFinishOutcome(run, "ready to implement")
	engineFinishOutcome(run, "complete")
	EngineAdvance(run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	engineFinishOutcome(run, "done")
	engineFinishOutcome(run, "pass")
	engineFinishOutcome(run, "opened")
}

// engineRunningRun is a run whose triage session
// has started, on the beltline/issue-142 branch.
func engineRunningRun() Run {
	run := engineStartRun(engineIssueBelt())
	run.Branch = new("beltline/issue-142")
	EngineAdvance(&run, engineSessionStart("01-triage-1"), engineT0)
	return run
}

// engineRerunContext is RerunTests.context(folder): the run folder /runs/test, the
// attempt folder /runs/test/<folder>, and every built-in agent.
func engineRerunContext(folder string) BriefContext {
	agents := map[AgentRef]Agent{}
	for _, agent := range SampleAgents() {
		agents[agent.Ref()] = agent
	}
	return BriefContext{
		RunFolder: "/runs/test", AttemptFolder: "/runs/test/" + folder,
		Base: "main", Branch: "beltline/issue-142", Agents: agents,
	}
}

// engineFinishAt starts the current agent stage at the folder the engine would use
// (on leg, when given) and finishes it with outcome.
func engineFinishAt(run *Run, outcome string, leg *string) {
	stage := run.Stage()
	folder := EngineAttemptFolderLeg(*run, *stage, leg)
	EngineAdvance(run, Event{Kind: EventKindSessionStarted, Session: "s", SessionID: "id", Folder: folder, Leg: leg}, engineT0)
	text := "Some output.\n\nOutcome: " + outcome
	EngineAdvance(run, Event{Kind: EventKindSessionFinished, Output: &text, Leg: leg}, engineT0)
}

// engineHasEffect reports whether effects has one of kind.
func engineHasEffect(effects []Effect, kind EffectKind) bool {
	return slices.ContainsFunc(effects, func(effect Effect) bool { return effect.Kind == kind })
}

// engineHeldAtBuild is a run held at build,
// with the spec's session stopped and its note kept.
func engineHeldAtBuild() Run {
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	run.Note = new("Keep the CSV header.")
	folder := EngineAttemptFolder(run, *run.Stage())
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "spec", SessionID: "spec-id", Folder: folder}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionFinished, Output: new("Outcome: complete")}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindDecided, Outcome: "approved"}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "build", SessionID: "build-id", Folder: "04-build-1"}, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindAbort}, engineT0)
	return run
}

// engineAfterMarker is the text after a marker that
// occurs once: the text after its first occurrence. It fails when it is missing.
func engineAfterMarker(t *testing.T, text, marker string) string {
	t.Helper()
	i := strings.Index(text, marker)
	if i < 0 {
		t.Fatalf("text has no %q section:\n%s", marker, text)
	}
	return text[i+len(marker):]
}

// engineBeforeMarker is the text before the
// first occurrence of marker, or all of it when there is none.
func engineBeforeMarker(text, marker string) string {
	if i := strings.Index(text, marker); i >= 0 {
		return text[:i]
	}
	return text
}

// engineSection is the text of the section from from to to.
func engineSection(t *testing.T, text, from, to string) string {
	t.Helper()
	return engineBeforeMarker(engineAfterMarker(t, text, from), to)
}

// engineIndexOf is the position of marker in text, failing the test when absent.
func engineIndexOf(t *testing.T, text, marker string) int {
	t.Helper()
	i := strings.Index(text, marker)
	if i < 0 {
		t.Fatalf("text has no %q:\n%s", marker, text)
	}
	return i
}

// engineIDIs reports whether id is want, where an empty want means nil.
func engineIDIs(id *string, want string) bool {
	if want == "" {
		return id == nil
	}
	return id != nil && *id == want
}

// engineGateRun is a run waiting at its approval.
func engineGateRun() Run {
	run := engineSampleRun()
	engineFinishOutcome(&run, "ready to spec")
	engineFinishOutcome(&run, "complete")
	return run
}

// engineRunningAgent is a run whose
// triage session has started in folder 01-triage-1 with the id sid.
func engineRunningAgent() Run {
	run := engineSampleRun()
	EngineAdvance(&run, Event{Kind: EventKindSessionStarted, Session: "bl-test", SessionID: "sid", Folder: "01-triage-1"}, engineT0)
	return run
}
