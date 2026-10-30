package core

import (
	"reflect"
	"strings"
	"testing"
)

// briefTestEpicChild is the Jira child ticket PAY-42 under the epic PAY-40.
func briefTestEpicChild(key string, labels []string) Subject {
	parent := "PAY-40"
	return Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: key, Title: "Part " + key, Labels: labels, IsOpen: true, Parent: &parent, Blockers: []string{}}
}

// TestTheBriefNamesTheParentAndTheBase
func TestTheBriefNamesTheParentAndTheBase(t *testing.T) {
	t.Parallel()
	text := briefSubject(briefTestEpicChild("PAY-42", []string{"base:beltline/PAY-40"}), "beltline/PAY-40", nil)
	if !strings.Contains(text, "It is part of PAY-40") {
		t.Fatalf("no parent line:\n%s", text)
	}
	if !strings.Contains(text, "Its work starts from beltline/PAY-40, and its pull requests merge into beltline/PAY-40.") {
		t.Fatalf("no base line:\n%s", text)
	}
	ticket := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-1", Title: "T", Labels: []string{"agent"}, Blockers: []string{}}
	if strings.Contains(briefSubject(ticket, "main", nil), "merge into") {
		t.Fatal("a ticket without a base hint names its merge target")
	}
}

// TestEveryAgentsFooterHasCopyableOutcomeNamesNotDescriptions checks that each agent footer names outcomes, not their descriptions, for the built-in
// agents and a custom stage.
func TestEveryAgentsFooterHasCopyableOutcomeNamesNotDescriptions(t *testing.T) {
	t.Parallel()
	var stages []Stage
	for _, agent := range SampleAgents() {
		stages = append(stages, DefaultsStageForAgent(agent, nil, nil, nil, TriggerKindIssues))
	}
	for _, kind := range AllTriggerKind {
		for _, stage := range SampleStages(kind, SampleAgents(), true) {
			if stage.Kind() == StageKindAgent {
				stages = append(stages, stage)
			}
		}
	}
	custom := stages[0]
	custom.Outcomes = []Outcome{{ID: "custom-verdict", Name: "custom verdict", When: "A custom description.", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})}}
	stages = append(stages, custom)
	for _, stage := range stages {
		text := BriefFooter(stage, stage.Writes())
		if !strings.Contains(text, "Write it in the output file, not only in your reply.") {
			t.Fatalf("%s: the footer does not say where the outcome goes", stage.ID)
		}
		if !strings.Contains(text, "these descriptions are not outcome values") {
			t.Fatalf("%s: the footer does not separate descriptions from outcomes", stage.ID)
		}
		var lines []string
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "Outcome: ") {
				lines = append(lines, line)
			}
		}
		if len(lines) != len(stage.Outcomes) {
			t.Fatalf("%s: %d Outcome lines for %d outcomes", stage.ID, len(lines), len(stage.Outcomes))
		}
		for index, outcome := range stage.Outcomes {
			if lines[index] != "Outcome: "+outcome.Name {
				t.Fatalf("%s: line %q, want Outcome: %s", stage.ID, lines[index], outcome.Name)
			}
			parsed := OutcomeParserParse(lines[index], stage.Outcomes)
			if parsed == nil || !reflect.DeepEqual(*parsed, outcome) {
				t.Fatalf("%s: %q parses to %+v, want %+v", stage.ID, lines[index], parsed, outcome)
			}
		}
	}
}

// briefTestContext is the brief context: the run folder /runs/r, the
// attempt folder /runs/r/02-spec-1, base main and every built-in agent.
func briefTestContext() BriefContext {
	agents := map[AgentRef]Agent{}
	for _, agent := range SampleAgents() {
		agents[agent.Ref()] = agent
	}
	return BriefContext{RunFolder: "/runs/r", AttemptFolder: "/runs/r/02-spec-1", Base: "main", Branch: "beltline/issue-142", Agents: agents}
}

// briefTestSampleRun is a run of belt on acme/billing#142,
// started at its first stage.
func briefTestSampleRun(belt Belt) Run {
	run := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142,
		Title: "Export invoices as CSV", Body: "Add an export button.", IsOpen: true, Blockers: []string{}}, Now: engineT0})
	EngineStart(&run, engineT0)
	return run
}

// briefTestFinish starts the current agent stage's session and finishes it with
// outcome.
func briefTestFinish(run *Run, outcome string) {
	EngineAdvance(run, Event{Kind: EventKindSessionStarted, Session: "bl-test", SessionID: "id", Folder: "f"}, engineT0)
	EngineAdvance(run, Event{Kind: EventKindSessionFinished, Output: new("Some output.\n\nOutcome: " + outcome)}, engineT0)
}

func TestBriefSourceContextRequiresConsentAfterPromptEntry(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	run.Subject = Subject{Kind: SubjectKindJira, Key: "SECRET-123", Title: "Source title",
		Body: "Source description", URL: "https://jira.example/SECRET-123", Labels: []string{"source-label"}, Parent: new("SECRET-1")}
	run.Note = new("Implement only my explicit task.")
	stage := *run.Stage()
	stage.Brief = "Task reference: {issue}. Write {output}."
	for _, include := range []*bool{nil, new(true), new(false)} {
		run.IncludeSubject = include
		for _, text := range []string{
			BriefRender(run, stage, briefTestContext()),
			BriefRerun(run, stage, Attempt{}, briefTestContext()),
		} {
			for _, detail := range []string{"SECRET-123", "Source title", "Source description", "https://jira.example/", "source-label", "SECRET-1"} {
				if strings.Contains(text, detail) != ValueOr(include, true) {
					t.Fatalf("include=%v: source detail %q shared incorrectly:\n%s", include, detail, text)
				}
			}
			if !strings.Contains(text, *run.Note) {
				t.Fatal("human prompt was excluded with source context")
			}
			if include != nil && !*include && !strings.Contains(text, "do not fetch or infer the ticket") {
				t.Fatal("excluded source lacks retrieval instructions")
			}
		}
	}
}

// TestArtifactsAreOptInAndUseTheEffectiveBrief
func TestArtifactsAreOptInAndUseTheEffectiveBrief(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	stage := *run.Stage()
	ctx := briefTestContext()
	stage.Brief = "Write {output}. Diagram in {artifacts}. No scripts."
	text := BriefRender(run, stage, ctx)
	for _, want := range []string{"Diagram in /runs/r/02-spec-1/artifacts. No scripts.", briefArtifacts("/runs/r/02-spec-1/artifacts"),
		"inline CSS", "no required remote resources"} {
		if !strings.Contains(text, want) {
			t.Fatalf("brief lacks %q:\n%s", want, text)
		}
	}
	if !strings.HasSuffix(text, BriefFooter(stage, BriefOutputPath(stage, ctx))) {
		t.Fatalf("brief does not end with the footer:\n%s", text)
	}
	agent := SampleAgent("triage")
	agent.Brief = stage.Brief
	ctx.Agents[agent.Ref()] = agent
	stage.Brief = ""
	if got := BriefRender(run, stage, ctx); got != text {
		t.Fatalf("the pinned agent's brief renders differently:\n%s\nwant:\n%s", got, text)
	}
	stage.Brief = "Write {output}."
	plain := BriefRender(run, stage, ctx)
	want := "Write /runs/r/02-spec-1/triage.md.\n\n" + briefSubject(run.Subject, ctx.Base, run.Repos) + "\n" + BriefFooter(stage, BriefOutputPath(stage, ctx))
	if plain != want {
		t.Fatalf("plain brief = %q, want %q", plain, want)
	}
	if strings.Contains(plain, "artifacts") || strings.Contains(plain, "inline CSS") {
		t.Fatalf("a brief without {artifacts} mentions artifacts:\n%s", plain)
	}
}

// TestFillsPlaceholdersAndAddsTheFooter
func TestFillsPlaceholdersAndAddsTheFooter(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	briefTestFinish(&run, "ready to spec")
	run.Attempts[0].Folder = "01-triage-1"
	text := BriefRender(run, *run.Stage(), briefTestContext())
	for _, want := range []string{"its triage at /runs/r/01-triage-1/triage.md", "Write only /runs/r/02-spec-1/spec.md",
		"acme/billing#142: Export invoices as CSV", "End /runs/r/02-spec-1/spec.md with one line copied exactly",
		"Write each paragraph and list item of /runs/r/02-spec-1/spec.md on one line; don't wrap them at a column.",
		"- open questions: Some rules depend on answers only a person can give."} {
		if !strings.Contains(text, want) {
			t.Fatalf("brief lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "{") {
		t.Fatalf("brief keeps a placeholder:\n%s", text)
	}
}

// TestOnlyTheChosenStageKeyResolves
func TestOnlyTheChosenStageKeyResolves(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	briefTestFinish(&run, "ready to spec")
	run.Attempts[0].Folder = "01-triage-1"
	triageIndex := -1
	for index := range run.Stages {
		if run.Stages[index].ID == run.Attempts[0].StageID {
			triageIndex = index
		}
	}
	run.Stages[triageIndex].ID = "draft"
	run.Attempts[0].StageID = "draft"
	specIndex := -1
	for index := range run.Stages {
		if run.Stages[index].ID == run.Stage().ID {
			specIndex = index
		}
	}
	run.Stages[specIndex].ID = "triage"
	run.Stages[specIndex].Brief = "{draft} {triage} {spec}"
	text := BriefRender(run, run.Stages[specIndex], briefTestContext())
	if !strings.Contains(text, "{draft}") {
		t.Fatalf("{draft} was resolved:\n%s", text)
	}
	if !strings.Contains(text, "/runs/r/01-triage-1/triage.md") || !strings.Contains(text, "/runs/r/02-spec-1/spec.md") {
		t.Fatalf("placeholders did not resolve:\n%s", text)
	}
	if strings.Contains(text, "{triage}") || strings.Contains(text, "{spec}") {
		t.Fatalf("a resolved placeholder was left:\n%s", text)
	}
}

// TestIncludesTheNote
func TestIncludesTheNote(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	run.Note = new("Mind the empty filter.")
	text := BriefRender(run, *run.Stage(), briefTestContext())
	for _, want := range []string{"## A note for this attempt", "Mind the empty filter.",
		"A note came with this attempt. Act on it before anything else:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("brief lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "Mind the empty filter.") >= strings.Index(text, "## When you are done") {
		t.Fatalf("the note comes after the closing section:\n%s", text)
	}
}

// TestAGateAttemptsMessageDoesNotReachABrief
func TestAGateAttemptsMessageDoesNotReachABrief(t *testing.T) {
	t.Parallel()
	run := briefTestSampleRun(engineIssueBelt())
	run.State = RunStateHeld
	EngineAdvance(&run, EventResume(new("approve"), new("Gate message"), nil), engineT0)
	if last := run.Attempts[len(run.Attempts)-1]; engineStr(last.Note) != "Gate message" {
		t.Fatalf("gate attempt note = %q, want Gate message", engineStr(last.Note))
	}
	text := BriefRender(run, *run.Stage(), briefTestContext())
	if strings.Contains(text, "Gate message") || strings.Contains(text, "## A note for this attempt") {
		t.Fatalf("the gate message reached the brief:\n%s", text)
	}
}

// TestTheHandoffFinalisesTheOutputThenWritesTheLaunchToken checks that an
// interactive stage is told to finish its output first and then write this
// launch's token, renamed into place, as its last step.
func TestTheHandoffFinalisesTheOutputThenWritesTheLaunchToken(t *testing.T) {
	t.Parallel()
	text := BriefHandoff("/runs/b/o/r/1/01-build-1", "TOKEN-42")
	for _, want := range []string{
		"When your output file is final, Outcome line included",
		"printf '%s\\n' 'TOKEN-42' > '/runs/b/o/r/1/01-build-1/handoff.tmp' && mv '/runs/b/o/r/1/01-build-1/handoff.tmp' '/runs/b/o/r/1/01-build-1/handoff'",
		"Don't run it while work is left or while you are waiting for an answer",
		"After handing back, stop",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the handoff lacks %q:\n%s", want, text)
		}
	}
	if !strings.HasPrefix(text, "\n## Handing the work back\n") {
		t.Errorf("the handoff is not its own section:\n%s", text)
	}
}
