// Helpers of the nested fork tests: the nfBelt builder, the leg and finish helpers,
// and the bothInner/bothOuter/jumped builders.

package core

import (
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// nfStage is a new agent stage of the nested belt, built by the real defaults:
// DefaultsStageForAgentAt, then named and routed.
func nfStage(id, agent string, route *string, outcome Outcome) Stage {
	stage := DefaultsStageForAgentAt(SampleAgent(agent), DefaultsPlace{Route: route}, nil, TriggerKindIssues)
	stage.ID = id
	switch id {
	case "quick-build", "thorough-build", "skip-build":
		stage.Name = "Build"
	case "thorough-spec":
		stage.Name = "Spec"
	case "assess":
		stage.Name = "Assess"
	default:
		stage.Name = strings.ToUpper(id[:1]) + id[1:]
	}
	stage.Route = route
	stage.Outcomes = []Outcome{outcome}
	return stage
}

// nfBelt is the nested belt: triage, then an outer
// fork "wave" (Build, Skip). Build holds assess, an inner fork "pace" (Quick,
// Thorough, parent build) with its stages, verify, and ship after the outer fork.
// Skip is empty unless skipStage adds a stage there; verify and quick can be left out.
func nfBelt(skipStage, verify, quick bool) Belt {
	wave := Fork{ID: "wave", Routes: []Route{{ID: "build", Name: "Build"}, {ID: "skip", Name: "Skip"}}}
	pace := Fork{ID: "pace", Parent: new("build"), Routes: []Route{{ID: "quick", Name: "Quick"}, {ID: "thorough", Name: "Thorough"}}}
	goOn := nfOutcome("go", nfThenOf(ThenActionNext))
	done := nfOutcome("done", nfThenOf(ThenActionNext))
	stages := []Stage{
		nfStage("triage", "triage", nil, goOn),
		nfStage("assess", "spec", new("build"), goOn),
	}
	if quick {
		stages = append(stages, nfStage("quick-build", "build", new("quick"), done))
	}
	stages = append(stages,
		nfStage("thorough-spec", "spec", new("thorough"), done),
		nfStage("thorough-build", "build", new("thorough"), done),
	)
	if verify {
		stages = append(stages, nfStage("verify", "verify", new("build"), done))
	}
	if skipStage {
		stages = append(stages, nfStage("skip-build", "build", new("skip"), done))
	}
	stages = append(stages, nfStage("ship", "ship", nil, nfOutcome("opened", nfThenOf(ThenActionDone))))
	trigger := DefaultIssueFilter()
	trigger.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "nested", Name: "Nested",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger},
		Repos:   []string{"acme/billing"}, Stages: stages, Forks: []Fork{wave, pace},
	})
}

// nfOutcome is an Outcome with neutral tone; the name
// is the id too, as the tests compare outcomes by name.
func nfOutcome(name string, then Then) Outcome {
	return Outcome{ID: name, Name: name, When: name, Tone: ToneNeutral, Then: then}
}

// nfThenOf is a Then for action with no target.
func nfThenOf(action ThenAction) Then {
	return NewThen(ThenArgs{Action: action})
}

// nfRouteThen is Then(.route, route: route).
func nfRouteThen(route string) Then {
	return NewThen(ThenArgs{Action: ThenActionRoute, Route: new(route)})
}

// nfRoutesThen is Then(.route, routes: routes).
func nfRoutesThen(routes ...string) Then {
	return NewThen(ThenArgs{Action: ThenActionRoute, Routes: routes})
}

// nfGotoThen is Then(.goto, to: to).
func nfGotoThen(to string) Then {
	return NewThen(ThenArgs{Action: ThenActionGoto, To: []string{to}})
}

// nfStageIndex is the index of the stage with id; it fails the test when there is none.
func nfStageIndex(t *testing.T, belt Belt, id string) int {
	t.Helper()
	for i := range belt.Stages {
		if belt.Stages[i].ID == id {
			return i
		}
	}
	t.Fatalf("no stage %q on the belt", id)
	return -1
}

// nfSetOutcomes replaces the outcomes of the stage with id.
func nfSetOutcomes(t *testing.T, belt *Belt, id string, outcomes ...Outcome) {
	t.Helper()
	belt.Stages[nfStageIndex(t, *belt, id)].Outcomes = outcomes
}

// nfBothInner makes assess split into the Quick and Thorough routes.
func nfBothInner(t *testing.T, belt Belt) Belt {
	t.Helper()
	nfSetOutcomes(t, &belt, "assess", nfOutcome("split", nfRoutesThen("quick", "thorough")))
	return belt
}

// nfBothOuter makes triage split into the Build and Skip routes.
func nfBothOuter(t *testing.T, belt Belt) Belt {
	t.Helper()
	nfSetOutcomes(t, &belt, "triage", nfOutcome("split", nfRoutesThen("build", "skip")))
	return belt
}

// nfInsertStage puts stage in front of the stage at index.
func nfInsertStage(belt *Belt, index int, stage Stage) {
	stages := append([]Stage{}, belt.Stages[:index]...)
	stages = append(stages, stage)
	belt.Stages = append(stages, belt.Stages[index:]...)
}

// nfStarted is a sessionStarted event; leg is nil for the run's focus.
func nfStarted(session, folder string, leg *string) Event {
	return Event{Kind: EventKindSessionStarted, Session: session, SessionID: session, Folder: folder, Leg: leg}
}

// nfEnded is a sessionFinished event with output; leg is nil for the run's focus.
func nfEnded(output string, leg *string) Event {
	return Event{Kind: EventKindSessionFinished, Output: new(output), Leg: leg}
}

// nfFinish finishes the current agent stage: its session
// starts and finishes with the outcome named in its output.
func nfFinish(run *Run, outcome string) []Effect {
	EngineAdvance(run, nfStarted("bl-test", "f", nil), engineT0)
	return EngineAdvance(run, nfEnded("Some output.\n\nOutcome: "+outcome, nil), engineT0)
}

// nfFinishLeg finishes a leg: the leg's session starts
// and finishes with the outcome.
func nfFinishLeg(run *Run, outcome, leg string) []Effect {
	EngineAdvance(run, nfStarted("bl-"+leg, "f", new(leg)), engineT0)
	return EngineAdvance(run, nfEnded("Some output.\n\nOutcome: "+outcome, new(leg)), engineT0)
}

// nfLegOn is the leg on route; nil when there is no such leg.
func nfLegOn(run Run, route string) *Leg {
	return run.LegOn(&route)
}

// nfLegByID is the leg with id; nil when there is no such leg.
func nfLegByID(run Run, id string) *Leg {
	return run.LegByID(&id)
}

// nfStageOf is the stage a leg is at, or "" for a missing leg or a finished one,
// so a test can compare the stage with a plain string.
func nfStageOf(leg *Leg) string {
	if leg == nil || leg.StageID == nil {
		return ""
	}
	return *leg.StageID
}

// nfStateOf is the state of a leg, or "" for a missing leg.
func nfStateOf(leg *Leg) RunState {
	if leg == nil {
		return ""
	}
	return leg.State
}

// nfLegsAt is the legs at stage.
func nfLegsAt(run Run, stage string) []Leg {
	var found []Leg
	for _, leg := range run.Legs {
		if nfStageOf(&leg) == stage {
			found = append(found, leg)
		}
	}
	return found
}

// nfUnfinishedLegsAt is the legs at stage that have not finished.
func nfUnfinishedLegsAt(run Run, stage string) []Leg {
	var found []Leg
	for _, leg := range nfLegsAt(run, stage) {
		if !leg.IsFinished() {
			found = append(found, leg)
		}
	}
	return found
}

// nfAttemptsAt is the attempts at stage.
func nfAttemptsAt(run Run, stage string) []Attempt {
	var found []Attempt
	for _, attempt := range run.Attempts {
		if attempt.StageID == stage {
			found = append(found, attempt)
		}
	}
	return found
}

// nfRouteSet is the set of routes of the legs that have one.
func nfRouteSet(legs []Leg) map[string]struct{} {
	set := map[string]struct{}{}
	for _, leg := range legs {
		if leg.Route != nil {
			set[*leg.Route] = struct{}{}
		}
	}
	return set
}

// nfWarnings is the warnings of the belt: the
// belt checked with its explicit stage keys and the built-in agents.
func nfWarnings(belt Belt) []string {
	agents := map[AgentRef]Agent{}
	for _, agent := range SampleAgents() {
		agents[agent.Ref()] = agent
	}
	converted := StageKeysExplicit(belt, agents)
	var keys []string
	for _, stage := range converted.Stages {
		if stage.Type == StageTypeAgent {
			keys = append(keys, StageKeysChosen(stage))
		}
	}
	return ValidationWarnings(converted, keys, agents)
}

// nfContains reports whether some string in list contains part.
func nfContains(list []string, part string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.Contains(s, part) })
}

// nfForkedBelt is the forked belt with the simple agents: triage, then
// Spec and Build on the Complex route and the simple agents on the Simple route
// (quick-<agent>), then Verify and Ship; triage's first outcome takes Simple.
func nfForkedBelt(simple []string) Belt {
	fork := Fork{ID: "size", Routes: []Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	var stages []Stage
	add := func(agent string, route *string, id string) {
		stage := DefaultsStageForAgentAt(SampleAgent(agent), DefaultsPlace{Before: stages, Forks: []Fork{fork}, Route: route}, stages, TriggerKindIssues)
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
	stages[0].Outcomes[0].Then = nfRouteThen("simple")
	trigger := DefaultIssueFilter()
	trigger.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "sized", Name: "Sized",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger},
		Repos:   []string{"acme/billing"}, Stages: stages, Forks: []Fork{fork},
	})
}

// nfBothRoutes is the forked belt
// with Simple's quick-build, whose triage takes both routes, Complex first.
func nfBothRoutes() Belt {
	return nfBothRoutesOf([]string{"build"})
}

// nfBothRoutesOf is the forked belt for any list of simple agents; nil
// leaves the Simple route without stages.
func nfBothRoutesOf(simple []string) Belt {
	belt := nfForkedBelt(simple)
	belt.Stages[0].Outcomes[0].Then = nfRoutesThen("complex", "simple")
	return belt
}

// nfIndexOfStage is the index of the stage with id in stages; it fails the test when there is none.
func nfIndexOfStage(t *testing.T, stages []Stage, id string) int {
	t.Helper()
	for i := range stages {
		if stages[i].ID == id {
			return i
		}
	}
	t.Fatalf("no stage %q", id)
	return -1
}

// nfIssuesBelt is an issues belt
// on acme/billing with the given stages and no forks.
func nfIssuesBelt(stages []Stage) Belt {
	trigger := DefaultIssueFilter()
	trigger.Label = "agent"
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: "issues", Name: "Issues",
		Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger},
		Repos:   []string{"acme/billing"}, Stages: stages,
	})
}

// nfApprovalBelt is triage, then
// an approval that keeps its Park it outcome only when park is set.
func nfApprovalBelt(park bool) Belt {
	triage := DefaultsStageForAgentAt(SampleAgent("triage"), DefaultsPlace{}, nil, TriggerKindIssues)
	gate := DefaultsStage(StageTypeApprove, []Stage{triage}, nil, []Stage{triage})
	if !park {
		var kept []Outcome
		for _, outcome := range gate.Outcomes {
			if outcome.Then.Action != ThenActionPark {
				kept = append(kept, outcome)
			}
		}
		gate.Outcomes = kept
	}
	return nfIssuesBelt([]Stage{triage, gate})
}

// nfSameJSON reports whether a and b encode to the same JSON. Comparing
// contents, where a nil and an empty array are equal; the canonical encoding
// is the same comparison.
func nfSameJSON(t *testing.T, a, b any) bool {
	t.Helper()
	x, err := jsonx.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := jsonx.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(x) == string(y)
}

// nfFinishBody finishes the current agent stage: the session's output is
// body and the outcome line.
func nfFinishBody(run *Run, outcome, body string) []Effect {
	EngineAdvance(run, nfStarted("bl-test", "f", nil), engineT0)
	return EngineAdvance(run, nfEnded(body+"\n\nOutcome: "+outcome, nil), engineT0)
}

// nfStartsWith reports whether list starts with prefix.
func nfStartsWith(list, prefix []string) bool {
	return len(prefix) <= len(list) && slices.Equal(list[:len(prefix)], prefix)
}
