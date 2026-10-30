// Validation tests of belts.

package core

import (
	"reflect"
	"slices"
	"testing"
)

// TestTheDefaultBeltHasNoWarnings
func TestTheDefaultBeltHasNoWarnings(t *testing.T) {
	t.Parallel()
	if got := vtWarnings(vtDefaultBelt("acme/billing")); len(got) != 0 {
		t.Fatalf("warnings %v, want none", got)
	}
}

// TestABeltNeedsARepository
func TestABeltNeedsARepository(t *testing.T) {
	t.Parallel()
	if !nfContains(vtWarnings(vtDefaultBelt()), "Add at least one repository to the trigger.") {
		t.Fatal("no repository warning")
	}
}

// TestCatchesOutcomeNames
func TestCatchesOutcomeNames(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	belt.Stages[0].Outcomes = append(belt.Stages[0].Outcomes, vtOutcome(" ", "x", ToneGood, nfThenOf(ThenActionNext)))
	belt.Stages[1].Outcomes = append(belt.Stages[1].Outcomes, vtOutcome("Complete", "x", ToneGood, nfThenOf(ThenActionNext)))
	warnings := vtWarnings(belt)
	if !slices.Contains(warnings, "Triage has an outcome without a name.") {
		t.Errorf("missing unnamed outcome warning in %v", warnings)
	}
	if !slices.Contains(warnings, "Spec has two outcomes with the same name.") {
		t.Errorf("missing duplicate outcome warning in %v", warnings)
	}
}

// TestCatchesDanglingAndSelfGotos
func TestCatchesDanglingAndSelfGotos(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	belt.Stages = slices.Delete(slices.Clone(belt.Stages), 1, 2)
	belt.Stages[3].Outcomes[1].Then.To = []string{"verify"}
	warnings := vtWarnings(belt)
	if !slices.Contains(warnings, "Approve: “sent back” goes to a stage that isn't on this belt anymore.") {
		t.Errorf("missing dangling goto in %v", warnings)
	}
	if !slices.Contains(warnings, "Verify: “code mismatch” goes to itself. Use “Run again” instead.") {
		t.Errorf("missing self goto in %v", warnings)
	}
}

// TestCatchesStagesARunCantLeave
func TestCatchesStagesARunCantLeave(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	for index, outcome := range belt.Stages[0].Outcomes {
		outcome.Then = nfThenOf(ThenActionHold)
		outcome.Tone = ToneYou
		belt.Stages[0].Outcomes[index] = outcome
	}
	warnings := vtWarnings(belt)
	if !slices.Contains(warnings, "Triage has no outcome that lets a run move on.") {
		t.Errorf("missing move-on warning in %v", warnings)
	}
	if !slices.Contains(warnings, "Triage has no good outcome. The run page uses it to know when the stage went well.") {
		t.Errorf("missing good-outcome warning in %v", warnings)
	}
}

// TestCatchesOrder
func TestCatchesOrder(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	stages := belt.Stages
	stages[3], stages[4] = stages[4], stages[3]
	moved := stages[2]
	stages = append(stages[:2:2], stages[3:]...)
	stages = append([]Stage{moved}, stages...)
	stages[4], stages[5] = stages[5], stages[4]
	belt.Stages = stages
	warnings := vtWarnings(belt)
	if !slices.Contains(warnings, "Verify reads the worktree before any stage changes code, so there is nothing to look at yet.") {
		t.Errorf("missing worktree order warning in %v", warnings)
	}
	if !slices.Contains(warnings, "Approve is the first stage, so you would approve issues before any agent has looked at them.") {
		t.Errorf("missing first-stage warning in %v", warnings)
	}
}

// TestWatchOutcomesNeedAnEvent
func TestWatchOutcomesNeedAnEvent(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	belt.Stages[6].Outcomes[0].When = "whenever"
	if !slices.Contains(vtWarnings(belt), "Merge: “merged” isn't tied to an event.") {
		t.Fatal("missing event warning")
	}
}

// TestEveryBuiltInAgentHasABriefAndValidSettings checks that every built-in agent has a brief and valid settings.
func TestEveryBuiltInAgentHasABriefAndValidSettings(t *testing.T) {
	t.Parallel()
	agents := DefaultsBuiltInAgents()
	ids := make([]string, len(agents))
	for index, agent := range agents {
		ids[index] = agent.ID
	}
	if !reflect.DeepEqual(ids, DefaultsBuiltInIDs) {
		t.Fatalf("ids %v, want %v", ids, DefaultsBuiltInIDs)
	}
	for _, agent := range agents {
		var others []Agent
		for _, other := range agents {
			if other.ID != agent.ID {
				others = append(others, other)
			}
		}
		if got := ValidationAgentWarnings(agent, others); len(got) != 0 {
			t.Errorf("%s: warnings %v", agent.ID, got)
		}
	}
}

// vtCodexFixture is CodexValidationTests.fixture: the custom agent and a manual belt that runs it.
func vtCodexFixture() (Agent, Belt) {
	agent := vtAgent("custom")
	belt := vtBeltOf("tasks", []string{"acme/app"}, []Stage{vtStageFor(agent, nil, nil)}, nil)
	return agent, belt
}

// TestCodexAcceptsExtraHighAndMax
func TestCodexAcceptsExtraHighAndMax(t *testing.T) {
	t.Parallel()
	for _, thinking := range []Thinking{ThinkingXHigh, ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			agent, belt := vtCodexFixture()
			for _, backend := range AllBackend {
				agent.Runner = backend
				agent.Thinking = &thinking
				runner := backend
				belt.Stages[0].Runner = &runner
				belt.Stages[0].Thinking = &thinking
				if got := ValidationAgentWarnings(agent, nil); len(got) != 0 {
					t.Fatalf("%s agent warnings %v", backend, got)
				}
				pinned := map[AgentRef]Agent{agent.Ref(): agent}
				if got := ValidationWarnings(belt, []string{agent.Key}, pinned); len(got) != 0 {
					t.Fatalf("%s belt warnings %v", backend, got)
				}
			}
		})
	}
}

// TestAStageThatTakesCodexFromItsAgentKeepsTheLevel
func TestAStageThatTakesCodexFromItsAgentKeepsTheLevel(t *testing.T) {
	t.Parallel()
	for _, thinking := range []Thinking{ThinkingXHigh, ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			agent, belt := vtCodexFixture()
			agent.Runner = BackendCodex
			agent.Thinking = &thinking
			belt.Stages[0].Runner = nil
			belt.Stages[0].Thinking = &thinking
			pinned := map[AgentRef]Agent{agent.Ref(): agent}
			if got := ValidationWarnings(belt, []string{agent.Key}, pinned); len(got) != 0 {
				t.Fatalf("warnings %v", got)
			}
		})
	}
}

// TestOneNoneIsAcceptedAndASecondIsRejected
func TestOneNoneIsAcceptedAndASecondIsRejected(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	watch := vtStageIndex(belt.Stages, StageKindWatch)
	belt.Stages[watch].Outcomes = append(belt.Stages[watch].Outcomes, vtOutcome("quiet", string(WatchEventNone), ToneNeutral, nfThenOf(ThenActionHold)))
	if got := vtWarnings(belt); len(got) != 0 {
		t.Fatalf("one none warned: %v", got)
	}
	belt.Stages[watch].Outcomes[0].When = "whenever"
	if !slices.Contains(vtWarnings(belt), "Merge: “merged” isn't tied to an event.") {
		t.Fatal("missing event warning for whenever")
	}
	belt.Stages[watch].Outcomes[0].When = string(WatchEventMerged)
	belt.Stages[watch].Outcomes = append(belt.Stages[watch].Outcomes, vtOutcome("settled", string(WatchEventNone), ToneYou, nfThenOf(ThenActionPark)))
	warnings := vtWarnings(belt)
	if !slices.Contains(warnings, "Merge can have only one outcome for when none of the others applies.") {
		t.Errorf("missing second-none warning in %v", warnings)
	}
	if nfContains(warnings, "same name") {
		t.Errorf("unexpected same-name warning in %v", warnings)
	}
}
