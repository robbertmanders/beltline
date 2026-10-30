// Validation tests of stage keys: duplicate keys follow route exclusivity,
// nested alternatives may share a key, reserved, invalid and artifacts keys,
// inherited legacy self-references warn and output fixes them, and approval and
// watch stages never expose output names. The writer stages are built
// with the real stage builder, then given their key, route and file.

package core

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// vtWriter is StageKeyTests' writer(_:_:_:_:writes:): a build stage with the
// given id, name, key, route and file, and one good outcome that moves on.
func vtWriter(id, name, key string, route *string, writes string) Stage {
	stage := vtStageFor(vtAgent("build"), nil, nil)
	stage.ID = id
	stage.Name = name
	stage.SetKey(new(key))
	stage.Route = route
	stage.SetWrites(writes)
	stage.Outcomes = []Outcome{vtOutcome("done", "done", ToneGood, nfThenOf(ThenActionNext))}
	return stage
}

// vtWriterOn is vtWriter with the default file "out.md".
func vtWriterOn(id, name, key string, route *string) Stage {
	return vtWriter(id, name, key, route, "out.md")
}

// vtKeysBelt is StageKeyTests' belt(_:_:): a manual belt over a/b with the stages and forks.
func vtKeysBelt(stages []Stage, forks []Fork) Belt {
	return vtBeltOf("keys", []string{"a/b"}, stages, forks)
}

// vtDuplicateWarnings is StageKeyTests' duplicate(_:_:): the duplicate-key warnings
// for key, checked with the stage keys as they are stored.
func vtDuplicateWarnings(belt Belt, key string) []string {
	var keys []string
	for _, stage := range belt.Stages {
		if stage.Key != nil {
			keys = append(keys, *stage.Key)
		}
	}
	var out []string
	for _, warning := range ValidationWarnings(belt, keys, vtPinned(SampleAgents())) {
		if strings.Contains(warning, "{"+key+"} is used by") {
			out = append(out, warning)
		}
	}
	return out
}

// TestDuplicateKeysFollowRouteExclusivity
func TestDuplicateKeysFollowRouteExclusivity(t *testing.T) {
	t.Parallel()
	fork := vtForkOf("size", vtRoute("complex", "Complex"), vtRoute("simple", "Simple"))
	triage := vtWriterOn("triage", "Triage", "triage", nil)
	triage.Outcomes = []Outcome{vtOutcome("simple", "simple", ToneGood, vtThenRoute("simple"))}
	complexSpec := vtWriter("spec", "Spec", "notes", new("complex"), "complex.md")
	simpleQuick := vtWriter("quick", "Quick", "notes", new("simple"), "simple.md")
	after := vtWriterOn("verify", "Verify", "verify", nil)
	if got := vtDuplicateWarnings(vtKeysBelt([]Stage{triage, complexSpec, simpleQuick, after}, []Fork{fork}), "notes"); len(got) != 0 {
		t.Fatalf("exclusive routes warned: %v", got)
	}

	both := triage
	both.Outcomes = []Outcome{vtOutcome("both", "both", ToneGood, nfRoutesThen("complex", "simple"))}
	together := vtDuplicateWarnings(vtKeysBelt([]Stage{both, complexSpec, simpleQuick, after}, []Fork{fork}), "notes")
	if len(together) != 1 {
		t.Fatalf("together warnings %v, want one", together)
	}
	if !strings.Contains(together[0], "Spec (Complex)") || !strings.Contains(together[0], "Quick (Simple)") {
		t.Errorf("together warning %q", together[0])
	}

	opening := vtKeysBelt([]Stage{complexSpec, simpleQuick, after}, []Fork{fork})
	if len(vtDuplicateWarnings(opening, "notes")) == 0 {
		t.Error("routes that are not exclusive did not warn")
	}

	againStage := vtWriterOn("again", "Again", "notes", new("complex"))
	same := []Stage{triage, complexSpec, againStage, after}
	sameWarnings := vtDuplicateWarnings(vtKeysBelt(same, []Fork{fork}), "notes")
	if len(sameWarnings) == 0 || !strings.Contains(sameWarnings[0], "Spec (Complex)") || !strings.Contains(sameWarnings[0], "Again (Complex)") {
		t.Errorf("same route warnings %v", sameWarnings)
	}

	main := vtKeysBelt([]Stage{vtWriterOn("a", "Alpha", "notes", nil), vtWriterOn("b", "Beta", "notes", nil)}, nil)
	mainWarning := vtDuplicateWarnings(main, "notes")
	if len(mainWarning) == 0 || !strings.Contains(mainWarning[0], "Alpha (main line)") || !strings.Contains(mainWarning[0], "Beta (main line)") {
		t.Errorf("main line warnings %v", mainWarning)
	}

	later := vtForkOf("later", vtRoute("one", "One"), vtRoute("two", "Two"))
	first := vtWriterOn("gate", "Gate", "gate", nil)
	first.Outcomes = []Outcome{
		vtOutcome("simple", "simple", ToneGood, vtThenRoute("simple")),
		vtOutcome("two", "two", ToneNeutral, vtThenRoute("two")),
	}
	sequential := vtKeysBelt([]Stage{
		first, complexSpec, simpleQuick, vtWriter("one", "One", "notes", new("one"), "out.md"), vtWriter("two", "Two", "other", new("two"), "out.md"),
	}, []Fork{fork, later})
	if !vtAny(vtDuplicateWarnings(sequential, "notes"), func(w string) bool {
		return strings.Contains(w, "Spec (Complex)") && strings.Contains(w, "One (One)")
	}) {
		t.Error("sequential forks did not warn for Spec and One")
	}

	other := vtKeysBelt([]Stage{vtWriterOn("only", "Only", "notes", nil)}, nil)
	if len(vtDuplicateWarnings(other, "notes")) != 0 {
		t.Error("a single stage warned")
	}
	if got := vtDuplicateWarnings(main, "notes"); len(got) != 1 {
		t.Errorf("main warnings %v, want one", got)
	}

	distinct := main.Clone()
	distinct.Stages[0].ID = "alpha-id"
	distinct.Stages[1].ID = "beta-id"
	if len(vtDuplicateWarnings(distinct, "notes")) == 0 {
		t.Error("distinct stages with one key did not warn")
	}

	aliased := vtKeysBelt([]Stage{vtWriterOn("draft", "Draft", "draft", nil), vtWriterOn("writer-2", "Writer", "copy", nil)}, nil)
	aliased.Stages[1].ID = "draft"
	for _, warning := range ValidationWarnings(aliased, []string{"draft", "copy"}, vtPinned(SampleAgents())) {
		if strings.Contains(warning, "wins over") || strings.Contains(warning, "{draft} is used by") {
			t.Errorf("alias warning %q", warning)
		}
	}
}

// TestNestedAlternativesMayShareAKey
func TestNestedAlternativesMayShareAKey(t *testing.T) {
	t.Parallel()
	outer := vtForkOf("wave", vtRoute("build", "Build"), vtRoute("skip", "Skip"))
	inner := Fork{ID: "pace", Routes: []Route{vtRoute("quick", "Quick"), vtRoute("thorough", "Thorough")}, Parent: new("build")}
	triage := vtWriterOn("triage", "Triage", "triage", nil)
	triage.Outcomes = []Outcome{vtOutcome("skip", "skip", ToneGood, vtThenRoute("skip"))}
	assess := vtWriterOn("assess", "Assess", "assess", new("build"))
	assess.Outcomes = []Outcome{vtOutcome("long", "long", ToneNeutral, vtThenRoute("thorough"))}
	quick := vtWriterOn("quick-build", "Build", "notes", new("quick"))
	thorough := vtWriterOn("thorough-build", "Build", "notes", new("thorough"))
	glance := vtWriterOn("glance", "Glance", "notes", new("skip"))
	siblings := vtKeysBelt([]Stage{triage, assess, quick, thorough, glance}, []Fork{outer, inner})
	if got := vtDuplicateWarnings(siblings, "notes"); len(got) != 0 {
		t.Fatalf("siblings warned: %v", got)
	}
	layout := siblings.Layout()
	if !StageKeysExclusive(quick, thorough, layout) {
		t.Error("quick and thorough are not exclusive")
	}
	if !StageKeysExclusive(quick, glance, layout) {
		t.Error("quick and glance are not exclusive")
	}
	descendant := vtWriterOn("deep-spec", "Deep spec", "assess", new("thorough"))
	if StageKeysExclusive(assess, descendant, layout) {
		t.Error("assess and its descendant are exclusive")
	}
	ancestorConflict := vtKeysBelt([]Stage{triage, assess, quick, thorough, glance, descendant}, []Fork{outer, inner})
	conflicts := vtDuplicateWarnings(ancestorConflict, "assess")
	if !vtAny(conflicts, func(w string) bool {
		return strings.Contains(w, "Assess (Build)") && strings.Contains(w, "Deep spec (Build / Thorough)")
	}) {
		t.Fatalf("ancestor conflict warnings %v", conflicts)
	}
}

// TestReservedInvalidAndArtifactsKeys
func TestReservedInvalidAndArtifactsKeys(t *testing.T) {
	t.Parallel()
	belt := vtDefaultBelt("a/b")
	for _, reserved := range BriefFixedPlaceholders {
		belt.Stages[0].SetKey(new(reserved))
		if !vtAny(vtWarnings(belt), func(w string) bool { return strings.Contains(w, "{"+reserved+"} is already a placeholder") }) {
			t.Errorf("%s: no reserved warning", reserved)
		}
	}
	belt.Stages[0].SetKey(new(""))
	if !vtAny(vtWarnings(belt), func(w string) bool { return strings.Contains(w, "no output placeholder") }) {
		t.Error("blank key did not warn")
	}
	belt.Stages[0].SetKey(new("Draft"))
	if !vtAny(vtWarnings(belt), func(w string) bool { return strings.Contains(w, "lowercase letters") }) {
		t.Error("capitalised key did not warn")
	}
	belt.Stages[0].SetKey(new("triage"))
	belt.Stages[0].ID = "artifacts"

	run := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: 1, Title: "T"}, Now: vtNow()})
	run.Stages[0].ID = "artifacts"
	context := BriefContext{RunFolder: "/run", AttemptFolder: "/attempt", Base: "main", Branch: "b", Agents: vtPinned(SampleAgents())}
	values := briefPlaceholders(run, run.Stages[1], context)
	if values["artifacts"] != "/attempt/artifacts" {
		t.Errorf("artifacts placeholder %q", values["artifacts"])
	}
	if _, ok := values["triage"]; !ok {
		t.Error("triage placeholder missing")
	}
	if _, ok := values["artifacts-id"]; ok {
		t.Error("artifacts-id placeholder present")
	}
	if !briefRequestsArtifacts("See {artifacts}", run, context) {
		t.Error("a brief naming {artifacts} does not request artifacts")
	}
	if briefRequestsArtifacts("```\n{artifacts}\n```", run, context) {
		t.Error("a fenced {artifacts} requests artifacts")
	}
}

// TestInheritedLegacySelfReferencesWarnAndOutputFixesThem
func TestInheritedLegacySelfReferencesWarnAndOutputFixesThem(t *testing.T) {
	t.Parallel()
	spec := vtAgent("spec")
	spec.Key = "spec"
	spec.Brief = "Write the result to {spec}."
	stage := vtWriterOn("design-stage", "Design", "design", nil)
	belt := vtKeysBelt([]Stage{stage}, nil)
	belt.Stages[0].Agent = new(spec.Ref())
	belt.Stages[0].Brief = ""
	pins := map[AgentRef]Agent{spec.Ref(): spec}
	hasBrief := func(warning string, parts ...string) bool {
		lower := strings.ToLower(warning)
		if !strings.Contains(lower, "brief of spec") {
			return false
		}
		return !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(warning, part) })
	}

	missing := ValidationWarnings(belt, []string{"design"}, pins)
	if !vtAny(missing, func(w string) bool { return hasBrief(w, "{spec}", "{design}", "{output}") }) {
		t.Fatalf("missing self-reference warning in %v", missing)
	}

	other := vtWriterOn("spec-stage", "Spec writer", "spec", nil)
	belt.Stages = append(belt.Stages, other)
	sharedOldName := ValidationWarnings(belt, []string{"design", "spec"}, pins)
	if !vtAny(sharedOldName, func(w string) bool { return hasBrief(w, "{spec}", "{design}") }) {
		t.Fatalf("shared old name warnings %v", sharedOldName)
	}
	if nfContains(sharedOldName, "{spec} isn't a placeholder") {
		t.Fatalf("unexpected placeholder warning in %v", sharedOldName)
	}

	spec.Brief = "Write the result to {output}."
	belt.Stages[0].Agent = new(spec.Ref())
	fixed := ValidationWarnings(belt, []string{"design", "spec"}, map[AgentRef]Agent{spec.Ref(): spec})
	if vtAny(fixed, func(w string) bool { return hasBrief(w, "writes its output") }) {
		t.Fatalf("fixed agent still warns: %v", fixed)
	}

	spec.Brief = "Write the result to {spec}."
	belt.Stages[0].Brief = "Write the result to {output}."
	stageFixed := ValidationWarnings(belt, []string{"design", "spec"}, map[AgentRef]Agent{spec.Ref(): spec})
	if vtAny(stageFixed, func(w string) bool { return hasBrief(w, "writes its output") }) {
		t.Fatalf("fixed stage still warns: %v", stageFixed)
	}

	belt.Stages[0].Brief = "Read the other stage's deliberate {spec} reference."
	override := ValidationWarnings(belt, []string{"design", "spec"}, map[AgentRef]Agent{spec.Ref(): spec})
	if vtAny(override, func(w string) bool { return hasBrief(w, "writes its output") }) {
		t.Fatalf("override warns: %v", override)
	}
	if nfContains(override, "{spec} isn't a placeholder") {
		t.Fatalf("override placeholder warning in %v", override)
	}
}

// TestApprovalAndWatchStagesNeverExposeOutputNames
func TestApprovalAndWatchStagesNeverExposeOutputNames(t *testing.T) {
	t.Parallel()
	agent := vtWriterOn("writer", "Writer", "draft", nil)
	gate := DefaultsStage(StageTypeApprove, []Stage{agent}, nil, []Stage{agent})
	watch := DefaultsStage(StageTypeWatch, []Stage{agent, gate}, nil, []Stage{agent, gate})
	belt := vtKeysBelt([]Stage{agent, gate, watch}, nil)
	entries := StageAdjustEntries(belt.Stages, func(stage Stage) *string { return stage.Key }, func(Stage) string { return "" }, false)
	if len(entries) != 1 || entries[0].Key != "draft" {
		t.Fatalf("entries %+v, want [draft]", entries)
	}
	if gate.Key != nil || watch.Key != nil {
		t.Fatal("approval or watch stage has a key")
	}
}

// vtNow is a fixed time for the runs the validation tests start.
func vtNow() time.Time {
	return time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
}
