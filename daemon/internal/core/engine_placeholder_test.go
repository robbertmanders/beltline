// Placeholder tests that need the engine's briefs and validation.

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// placeholderTestBelt is Defaults.belt(repos:) with the given repositories.
func placeholderTestBelt(repos ...string) Belt {
	return SampleBelt(repos, "agent", SampleAgents(), true)
}

// placeholderTestAgents is every built-in agent keyed by its reference.
func placeholderTestAgents() map[AgentRef]Agent {
	agents := map[AgentRef]Agent{}
	for _, agent := range SampleAgents() {
		agents[agent.Ref()] = agent
	}
	return agents
}

// placeholderTestWarnings checks a belt:
// the belt's warnings with its stage keys converted from the pinned agents.
func placeholderTestWarnings(belt Belt) []string {
	agents := placeholderTestAgents()
	converted := StageKeysExplicit(belt, agents)
	var keys []string
	for _, stage := range converted.Stages {
		if stage.Kind() == StageKindAgent && stage.Key != nil {
			keys = append(keys, *stage.Key)
		}
	}
	return ValidationWarnings(converted, keys, agents)
}

// placeholderTestKeys are the keys a belt's agent stages answer to, as the
// shipped-belt test reads them: the stage's key, else its agent's key, else
// the agent's id.
func placeholderTestKeys(belt Belt, agents map[AgentRef]Agent) []string {
	var keys []string
	for _, stage := range belt.Stages {
		if stage.Kind() != StageKindAgent {
			continue
		}
		if stage.Key != nil {
			keys = append(keys, *stage.Key)
		} else if stage.Agent != nil {
			if agent, ok := agents[*stage.Agent]; ok {
				keys = append(keys, agent.Key)
			} else {
				keys = append(keys, stage.Agent.ID)
			}
		}
	}
	return keys
}

// placeholderTestWrite creates the folder of path and writes text to it.
func placeholderTestWrite(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSkippedSpecNeverFallsBackToAnEarlierRunsCheckout checks that a skipped spec never falls back to an earlier run's checkout, once per
// stage id (build and verify).
func TestSkippedSpecNeverFallsBackToAnEarlierRunsCheckout(t *testing.T) {
	t.Parallel()
	for _, stageID := range []string{"build", "verify"} {
		t.Run(stageID, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			checkout := filepath.Join(root, "checkout")
			staleSpec := filepath.Join(checkout, "spec.md")
			placeholderTestWrite(t, staleSpec, "Issue 21 requirements")

			belt := placeholderTestBelt("acme/billing")
			agents := placeholderTestAgents()
			firstFolder := filepath.Join(root, "runs/21")
			secondFolder := filepath.Join(root, "runs/25")
			firstContext := BriefContext{RunFolder: firstFolder, AttemptFolder: filepath.Join(firstFolder, "02-spec-1"),
				Base: "main", Branch: "issue-21", Agents: agents, Checkouts: map[string]string{"acme/billing": checkout}}
			first := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 21, Title: "Earlier issue", IsOpen: true, Blockers: []string{}}, Now: engineT0})
			specStage, ok := placeholderTestStage(first.Stages, "spec")
			if !ok {
				t.Fatal("no spec stage")
			}
			specPath := BriefOutputPath(specStage, firstContext)
			specPrompt := BriefRender(first, specStage, firstContext)
			if !strings.Contains(specPrompt, "Write only "+specPath) {
				t.Fatalf("spec prompt does not write %s", specPath)
			}
			if !strings.Contains(specPrompt, "Never create, replace or commit a repository-root spec") {
				t.Fatal("spec prompt lacks the repository-root rule")
			}
			placeholderTestWrite(t, specPath, "Run 21 spec")
			specAttempt := NewAttempt(AttemptArgs{StageID: specStage.ID, Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Folder: "02-spec-1"})
			specAttempt.EndedAt = &jsonx.Date{Time: engineT0}
			first.Attempts = []Attempt{specAttempt}
			firstStage, ok := placeholderTestStage(first.Stages, stageID)
			if !ok {
				t.Fatalf("no %s stage", stageID)
			}
			if got := briefPlaceholders(first, firstStage, firstContext)["spec"]; got != specPath {
				t.Fatalf("spec placeholder = %q, want %q", got, specPath)
			}

			second := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 25, Title: "Current issue",
				Body: "Stack the import controls.", IsOpen: true, Blockers: []string{}}, Now: engineT0})
			second.Note = new("Use the current issue's layout requirements.")
			stage, ok := placeholderTestStage(second.Stages, stageID)
			if !ok {
				t.Fatalf("no %s stage", stageID)
			}
			context := BriefContext{RunFolder: secondFolder, AttemptFolder: filepath.Join(secondFolder, stageID+"-2"),
				Base: "main", Branch: "issue-25", Agents: agents, Checkouts: map[string]string{"acme/billing": checkout}}
			earlier := NewAttempt(AttemptArgs{StageID: stage.ID, Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Folder: stageID + "-1"})
			earlier.EndedAt = &jsonx.Date{Time: engineT0}
			second.Attempts = []Attempt{earlier}
			prompts := []string{
				BriefRender(second, stage, context),
				BriefRerun(second, stage, earlier, context),
				BriefFollowUp(second, stage, earlier, context),
			}
			for _, prompt := range prompts {
				for _, want := range []string{
					"Stage outputs, including specs, belong to this run's attempt folders",
					"Never create, replace or commit a repository-root spec",
					"not that you should search the checkout for it",
					"build or verify against this run's subject and supplied notes instead",
					"Do not create a replacement spec in the checkout or report a spec mismatch solely because no spec was produced.",
					BriefOutputPath(stage, context),
				} {
					if !strings.Contains(prompt, want) {
						t.Fatalf("prompt lacks %q:\n%s", want, prompt)
					}
				}
				if strings.Contains(prompt, firstFolder) || strings.Contains(prompt, staleSpec) {
					t.Fatalf("prompt names an earlier run's files:\n%s", prompt)
				}
			}
			if !strings.Contains(prompts[0], "(no spec.md yet)") {
				t.Fatalf("first prompt lacks the missing spec line:\n%s", prompts[0])
			}
			if !strings.Contains(prompts[0], "Stack the import controls.") || !strings.Contains(prompts[0], "Use the current issue's layout requirements.") {
				t.Fatalf("first prompt lacks the subject or the note:\n%s", prompts[0])
			}
			if data, err := os.ReadFile(staleSpec); err != nil || string(data) != "Issue 21 requirements" {
				t.Fatalf("stale spec = %q, %v", data, err)
			}
			if data, err := os.ReadFile(specPath); err != nil || string(data) != "Run 21 spec" {
				t.Fatalf("earlier spec = %q, %v", data, err)
			}
		})
	}
}

// placeholderTestStage is the stage with id in stages.
func placeholderTestStage(stages []Stage, id string) (Stage, bool) {
	for _, stage := range stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return Stage{}, false
}

// TestUnknownNamesAreOrderedAndReportedOncePerStage
func TestUnknownNamesAreOrderedAndReportedOncePerStage(t *testing.T) {
	t.Parallel()
	belt := placeholderTestBelt("a/b")
	belt.Stages[0].Brief = "{missing}"
	belt.Stages[1].Brief = "Write {draft} {draft} {draft} from {issue}. Then {draft-2} {draft_2}."
	want := []string{
		"Triage: {missing} isn't a placeholder on this belt, so the agent reads it as text.",
		"Spec: {draft} isn't a placeholder on this belt, so the agent reads it as text.",
		"Spec: {draft-2} isn't a placeholder on this belt, so the agent reads it as text.",
		"Spec: {draft_2} isn't a placeholder on this belt, so the agent reads it as text.",
	}
	if got := placeholderTestWarnings(belt); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
}

// TestIgnoresExamplesButChecksInlineCodeAndNamesOutsideFences
func TestIgnoresExamplesButChecksInlineCodeAndNamesOutsideFences(t *testing.T) {
	t.Parallel()
	belt := placeholderTestBelt("a/b")
	for _, brief := range []string{"{} { } {Draft} {1, 2} {\"body\": \"Ship it\"}",
		"```\ngh pr view --json {draft}\n```", "~~~\n{draft}\n~~~",
		"```\n{draft}", "~~~sh\n{draft}",
		"````swift\n```\n{draft}\n````", "  ```sh\n{draft}\n  ```"} {
		belt.Stages[1].Brief = brief
		if got := placeholderTestWarnings(belt); len(got) != 0 {
			t.Fatalf("%q warns %q, want none", brief, got)
		}
	}
	for _, brief := range []string{"```\n{draft}\n```\n{draft}", "~~~\n{draft}\n~~~\n{draft}", "`{draft}`"} {
		belt.Stages[1].Brief = brief
		if got := placeholderTestWarnings(belt); len(got) != 1 {
			t.Fatalf("%q warns %q, want one warning", brief, got)
		}
	}
}

// TestOnlyChosenStageKeysAreAccepted
func TestOnlyChosenStageKeysAreAccepted(t *testing.T) {
	t.Parallel()
	belt := placeholderTestBelt("a/b")
	var names []string
	for _, name := range append(append([]string{}, BriefFixedPlaceholders...), "spec", "complex-build") {
		names = append(names, "{"+name+"}")
	}
	belt.Stages[1].Brief = strings.Join(names, " ")
	// A stage id is not a brief name unless that id is also the chosen key.
	belt.Stages[3].ID = "complex-build"
	// Keep existing gotos valid after renaming the stage.
	for index := range belt.Stages {
		for outcome := range belt.Stages[index].Outcomes {
			to := append([]string{}, belt.Stages[index].Outcomes[outcome].Then.To...)
			for k := range to {
				if to[k] == "build" {
					to[k] = "complex-build"
				}
			}
			belt.Stages[index].Outcomes[outcome].Then.To = to
		}
	}
	want := []string{"Spec: {complex-build} isn't a placeholder on this belt, so the agent reads it as text."}
	if got := placeholderTestWarnings(belt); !reflect.DeepEqual(got, want) {
		t.Fatalf("warnings = %q, want %q", got, want)
	}
	belt.Stages[1].Brief += " {review}"
	if got := placeholderTestWarnings(belt); len(got) != 2 {
		t.Fatalf("warnings = %q, want 2", got)
	}
}

// TestAnIdMatchingAKeyIsNotACollision
func TestAnIdMatchingAKeyIsNotACollision(t *testing.T) {
	t.Parallel()
	belt := placeholderTestBelt("a/b")
	belt.Stages[1].ID = "triage"
	belt.Stages[0].ID = "first"
	for _, warning := range placeholderTestWarnings(belt) {
		if strings.Contains(warning, "wins over") {
			t.Fatalf("unexpected collision warning %q", warning)
		}
		if strings.Contains(warning, "{triage}") && strings.Contains(warning, "isn't a placeholder") {
			t.Fatalf("unexpected placeholder warning %q", warning)
		}
	}
}

// TestLegacyAgentKeysAreNotValidatedAsBeltPlaceholders
func TestLegacyAgentKeysAreNotValidatedAsBeltPlaceholders(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"", "Draft", "two words", "é"} {
		agent := SampleAgent("build")
		agent.Key = key
		for _, warning := range ValidationAgentWarnings(agent, nil) {
			if strings.Contains(warning, "placeholder") {
				t.Fatalf("key %q warns %q", key, warning)
			}
		}
	}
}

// TestAllShippedBriefsAcceptTheShippedNames
func TestAllShippedBriefsAcceptTheShippedNames(t *testing.T) {
	t.Parallel()
	agents := placeholderTestAgents()
	for _, kind := range AllTriggerKind {
		belt := placeholderTestBelt("a/b")
		belt.Stages = SampleStages(kind, SampleAgents(), true)
		keys := placeholderTestKeys(belt, agents)
		if got := validationPlaceholders(belt, keys, agents); len(got) != 0 {
			t.Fatalf("%s: placeholders warn %q", kind, got)
		}
	}
	for _, id := range SampleAgentIDs {
		// The epic agents read each other's outputs, so they are tried on the epic belt.
		belt := placeholderTestBelt("a/b")
		if slices.Contains(SampleEpicAgents, id) {
			belt = SampleEpicBelt(epicTestEpicTrigger(), []string{"a/b"}, SampleAgents())
		}
		for index := range belt.Stages {
			belt.Stages[index].ID = fmt.Sprintf("stage-%d", index)
		}
		stage := DefaultsStageForAgent(SampleAgent(id), belt.Stages, nil, belt.Stages, TriggerKindIssues)
		stage.ID = "agent-under-test"
		belt.Stages = append(belt.Stages, stage)
		keys := placeholderTestKeys(belt, agents)
		if got := validationPlaceholders(belt, keys, agents); len(got) != 0 {
			t.Fatalf("%s: placeholders warn %q", id, got)
		}
	}
}
