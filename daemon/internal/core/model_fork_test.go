package core

import (
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// draftTestOutcome is an outcome named name with the given then, id = name.
func draftTestOutcome(name string, then Then) Outcome {
	return Outcome{ID: name, Name: name, When: "fine", Tone: ToneGood, Then: then}
}

// draftTestStage is a stage built from its id, type, agent, outcomes and route:
// builds it: its name from the type, pi as the runner of an agent stage, a
// read-only workspace, two slots and hold on failure. agent and route are ""
// for none.
func draftTestStage(id string, typ StageType, agent, route string, outcomes ...Outcome) Stage {
	s := Stage{ID: id, Type: typ, Name: typ.Title(), Workspace: WorkspaceReadOnly, Slots: 2, Fallback: FallbackHold, Outcomes: outcomes}
	if typ == StageTypeWatch {
		s.Name = WatchTargetPullRequest.Title()
	}
	if typ.Kind() == StageKindAgent {
		pi := BackendPi
		s.Runner = &pi
		if agent != "" {
			s.Agent = &AgentRef{ID: agent, Version: 1}
		}
	}
	if route != "" {
		s.Route = new(route)
	}
	return s
}

// draftTestBelt is a belt over stages and forks with the default settings:
// one issues trigger on acme/billing, version 1, DefaultPolicies and AfkModeGlobal.
func draftTestBelt(stages []Stage, forks ...Fork) Belt {
	trigger := DefaultIssueFilter()
	entry := TriggerEntry{ID: TriggerEntryLegacyID, Trigger: Trigger{Kind: TriggerKindIssues, Issues: &trigger}, Repos: []string{"acme/billing"}}
	return Belt{ID: "draft", Name: "Draft", Version: 1, Triggers: []TriggerEntry{entry}, Policies: DefaultPolicies(), Stages: stages, Forks: forks, Afk: AfkModeGlobal}
}

// forkedBeltWith is the forked belt for simple agents: triage, then Spec and Build on
// the Complex route or the simple agents on the Simple route (quick-<agent>),
// then Verify and Ship. Only the stages, agents, routes and the first outcome
// of Triage are kept; the layout tests read nothing else.
func forkedBeltWith(simple []string) Belt {
	fork := Fork{ID: "size", Routes: []Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	stages := []Stage{
		draftTestStage("triage", StageTypeAgent, "triage", "",
			draftTestOutcome("ready to implement", Then{Action: ThenActionRoute, Routes: []string{"simple"}}),
			draftTestOutcome("ready to spec", Then{Action: ThenActionNext})),
		draftTestStage("spec", StageTypeAgent, "spec", "complex"),
		draftTestStage("build", StageTypeAgent, "build", "complex"),
	}
	for _, agent := range simple {
		stages = append(stages, draftTestStage("quick-"+agent, StageTypeAgent, agent, "simple"))
	}
	stages = append(stages, draftTestStage("verify", StageTypeAgent, "verify", ""), draftTestStage("ship", StageTypeAgent, "ship", ""))
	return draftTestBelt(stages, fork)
}

// draftNestedBelt is a belt whose outer fork "wave" has a route "build" holding a
// nested fork "inner" (routes "ia" and "ib"), and a route "skip".
func draftNestedBelt() Belt {
	wave := Fork{ID: "wave", Routes: []Route{{ID: "build", Name: "Build"}, {ID: "skip", Name: "Skip"}}}
	inner := Fork{ID: "inner", Parent: new("build"), At: new(1), Routes: []Route{{ID: "ia", Name: "A"}, {ID: "ib", Name: "B"}}}
	stages := []Stage{
		draftTestStage("triage", StageTypeAgent, "triage", ""),
		draftTestStage("inner-a", StageTypeAgent, "spec", "ia"),
		draftTestStage("inner-b", StageTypeAgent, "build", "ib"),
		draftTestStage("assess", StageTypeAgent, "verify", "build"),
		draftTestStage("skip-stage", StageTypeAgent, "build", "skip"),
		draftTestStage("ship", StageTypeAgent, "ship", ""),
	}
	return draftTestBelt(stages, wave, inner)
}

// draftTestIDs is the ids of stages, in order (never nil).
func draftTestIDs(stages []Stage) []string {
	out := []string{}
	for _, s := range stages {
		out = append(out, s.ID)
	}
	return out
}

// draftTestRouteIDs is the ids of routes, in order (never nil).
func draftTestRouteIDs(routes []Route) []string {
	out := []string{}
	for _, r := range routes {
		out = append(out, r.ID)
	}
	return out
}

// draftCheck fails the test with the diff when got differs from want.
func draftCheck(t *testing.T, what string, want, got any) {
	t.Helper()
	if d := testkit.Diff(want, got); d != "" {
		t.Fatalf("%s:\n%s", what, d)
	}
}

// TestOnlyTheForkRightAfterAStageIsOnOfferThere: the fork after a stage is offered only
// where it comes directly before one.
func TestOnlyTheForkRightAfterAStageIsOnOfferThere(t *testing.T) {
	t.Parallel()
	layout := forkedBeltWith([]string{"build"}).Layout()
	// Triage comes directly before the fork: both its routes are a choice.
	fork := layout.ForkAfter("triage")
	if fork == nil || fork.ID != "size" {
		t.Fatalf("fork after triage = %v, want size", fork)
	}
	draftCheck(t, "routes after triage", []string{"complex", "simple"}, draftTestRouteIDs(fork.Routes))
	// A stage on a route goes to the next stage of that route, so no fork.
	// The stages where the routes meet and after them go on to a plain stage.
	for _, id := range []string{"spec", "build", "verify", "ship", "nonsense"} {
		if f := layout.ForkAfter(id); f != nil {
			t.Errorf("fork after %s = %s, want none", id, f.ID)
		}
	}
}

// TestThePathFollowsTheRoutes: the path takes the first route, or the routes given.
func TestThePathFollowsTheRoutes(t *testing.T) {
	t.Parallel()
	layout := forkedBeltWith([]string{"build"}).Layout()
	draftCheck(t, "path", []string{"triage", "spec", "build", "verify", "ship"}, draftTestIDs(layout.Path(nil, false)))
	draftCheck(t, "path via simple", []string{"triage", "quick-build", "verify", "ship"},
		draftTestIDs(layout.Path(map[string][]string{"size": {"simple"}}, false)))
	if n := len(layout.Segments()); n != 4 {
		t.Fatalf("segments = %d, want 4", n)
	}
}

// TestStagesAreNumberedByColumn: each route of a fork counts from the same column.
func TestStagesAreNumberedByColumn(t *testing.T) {
	t.Parallel()
	layout := forkedBeltWith([]string{"build"}).Layout()
	ids := []string{"triage", "spec", "build", "quick-build", "verify", "ship"}
	want := []int{1, 2, 3, 2, 4, 5}
	for i, id := range ids {
		if got := layout.Number(id); got != want[i] {
			t.Errorf("number of %s = %d, want %d", id, got, want[i])
		}
	}
}

// TestAGotoIntoAForkPairsTheSameAgentOnEachRoute: a goto back into a fork names the
// matching stage on the other route, and none for a jump from inside the fork.
func TestAGotoIntoAForkPairsTheSameAgentOnEachRoute(t *testing.T) {
	t.Parallel()
	layout := forkedBeltWith([]string{"build"}).Layout()
	draftCheck(t, "build from verify", []string{"quick-build"}, layout.Counterparts("build", "verify"))
	draftCheck(t, "spec from verify", []string{"quick-build"}, layout.Counterparts("spec", "verify"))
	if got := layout.Counterparts("spec", "build"); len(got) != 0 {
		t.Errorf("spec from build = %v, want none", got)
	}
}

// TestNestedForkLayoutFollowsRoutes: a fork inside a route is numbered, spanned,
// ended and walked inside the route that holds it.
func TestNestedForkLayoutFollowsRoutes(t *testing.T) {
	t.Parallel()
	layout := draftNestedBelt().Layout()
	if !layout.Nested() {
		t.Fatal("Nested() = false, want true")
	}
	wave := layout.Fork("wave")
	if wave == nil {
		t.Fatal("no wave fork")
	}
	span := layout.Span(*wave)
	if span == nil || *span != (IntRange{Lower: 1, Upper: 4}) {
		t.Fatalf("span of wave = %v, want 1...4", span)
	}
	draftCheck(t, "path", []string{"triage", "inner-a", "assess", "ship"}, draftTestIDs(layout.Path(nil, false)))
	draftCheck(t, "path via skip", []string{"triage", "skip-stage", "ship"},
		draftTestIDs(layout.Path(map[string][]string{"wave": {"skip"}}, false)))
	draftCheck(t, "path via ib", []string{"triage", "inner-b", "assess", "ship"},
		draftTestIDs(layout.Path(map[string][]string{"wave": {"build"}, "inner": {"ib"}}, false)))
	numbers := map[string]int{"triage": 1, "inner-a": 2, "inner-b": 2, "assess": 3, "skip-stage": 2, "ship": 4}
	for id, want := range numbers {
		if got := layout.Number(id); got != want {
			t.Errorf("number of %s = %d, want %d", id, got, want)
		}
	}
	for route, want := range map[string]int{"build": 4, "ia": 2, "skip": 5} {
		if got := layout.End(route); got != want {
			t.Errorf("end of %s = %d, want %d", route, got, want)
		}
	}
	place := layout.Place("inner-a")
	if place == nil || place.Fork.ID != "inner" || place.Route.ID != "ia" {
		t.Fatalf("place of inner-a = %+v, want inner/ia", place)
	}
	draftCheck(t, "path of names", []string{"Build", "A"}, layout.PathOf("inner-a"))
}

// TestForkSegmentHoldsItsRouteStages checks the segments of a forked belt: triage, then the fork with its routes.
func TestForkSegmentHoldsItsRouteStages(t *testing.T) {
	t.Parallel()
	segments := forkedBeltWith([]string{"build"}).Layout().Segments()
	if len(segments) != 4 {
		t.Fatalf("segments = %d, want 4", len(segments))
	}
	draftCheck(t, "stages of the fork", []string{"spec", "build", "quick-build"}, draftTestIDs(segments[1].Stages()))
}

// TestForkEncodesExactBytes: the wire form of a fork, with parent and at only when set.
func TestForkEncodesExactBytes(t *testing.T) {
	t.Parallel()
	fork := Fork{ID: "size", Routes: []Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple", Feeds: true}}}
	got, err := jsonx.Marshal(fork)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"size","routes":[{"id":"complex","name":"Complex"},{"feeds":true,"id":"simple","name":"Simple"}]}`
	if !sameJSON(t, got, want) {
		t.Fatalf("fork = %s, want %s", got, want)
	}
	nested := Fork{ID: "inner", Routes: []Route{}, Parent: new("route-1"), At: new(0)}
	got, err = jsonx.Marshal(nested)
	if err != nil {
		t.Fatal(err)
	}
	want = `{"at":0,"id":"inner","parent":"route-1","routes":[]}`
	if !sameJSON(t, got, want) {
		t.Fatalf("nested fork = %s, want %s", got, want)
	}
}

// TestForkDecodesLegacyShape: forks saved before nesting and before at have neither key.
func TestForkDecodesLegacyShape(t *testing.T) {
	t.Parallel()
	var legacy Fork
	if err := json.Unmarshal([]byte(`{"id":"size","routes":[{"id":"a","name":"A"}]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	draftCheck(t, "legacy fork", Fork{ID: "size", Routes: []Route{{ID: "a", Name: "A"}}}, legacy)
	var nulls Fork
	if err := json.Unmarshal([]byte(`{"id":"size","routes":[],"parent":null,"at":null}`), &nulls); err != nil {
		t.Fatal(err)
	}
	if nulls.Parent != nil || nulls.At != nil || nulls.Routes == nil {
		t.Fatalf("null parent and at = %+v, want nil", nulls)
	}
	var placed Fork
	if err := json.Unmarshal([]byte(`{"id":"inner","routes":[],"parent":"build","at":3}`), &placed); err != nil {
		t.Fatal(err)
	}
	draftCheck(t, "placed fork", Fork{ID: "inner", Routes: []Route{}, Parent: new("build"), At: new(3)}, placed)
}

// TestRouteEncodesFeedsOnlyWhenSet: a plain route has no feeds key, a feeding one does.
func TestRouteEncodesFeedsOnlyWhenSet(t *testing.T) {
	t.Parallel()
	plain, err := jsonx.Marshal(Route{ID: "a", Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != `{"id":"a","name":"A"}` {
		t.Fatalf("plain route = %s", plain)
	}
	feeding, err := jsonx.Marshal(Route{ID: "b", Name: "B", Feeds: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(feeding) != `{"id":"b","name":"B","feeds":true}` {
		t.Fatalf("feeding route = %s", feeding)
	}
}

// TestForkRoundTripsThroughJSON: decoding what a fork encodes gives the same fork.
func TestForkRoundTripsThroughJSON(t *testing.T) {
	t.Parallel()
	for _, fork := range []Fork{
		{ID: "size", Routes: []Route{{ID: "complex", Name: "Complex"}, {ID: "feed", Name: "Feed in", Feeds: true}}},
		{ID: "inner", Routes: []Route{{ID: "ia", Name: "A"}}, Parent: new("build"), At: new(2)},
	} {
		data, err := jsonx.Marshal(fork)
		if err != nil {
			t.Fatal(err)
		}
		var back Fork
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		draftCheck(t, "round trip of "+fork.ID, fork, back)
	}
}

// TestAForkedBeltSurvivesSaving The belt
// is forkedBeltWith.
func TestAForkedBeltSurvivesSaving(t *testing.T) {
	t.Parallel()
	belt := forkedBeltWith([]string{"build"})
	loaded := btDecode[Belt](t, btEncode(t, belt))
	if d := testDiffEmpty(belt, loaded); d != "" {
		t.Errorf("forked belt does not survive saving:\n%s", d)
	}
}

// TestRunsAndBeltsSavedBeforeForksHaveNone
// The run is testModelRun (no Engine.start); its forks and routes keys are removed.
func TestRunsAndBeltsSavedBeforeForksHaveNone(t *testing.T) {
	t.Parallel()
	obj := testJSONMap(t, testModelRun())
	delete(obj, "forks")
	delete(obj, "routes")
	run := testRunFrom(t, obj)
	if len(run.Forks) != 0 {
		t.Errorf("forks = %+v, want none", run.Forks)
	}
	if len(run.Routes()) != 0 {
		t.Errorf("routes = %v, want none", run.Routes())
	}
}
