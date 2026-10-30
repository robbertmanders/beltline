// Tests of the placeholder help: the fixed names, and the belts and briefs that use them.

package core

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// customTestWriter and customTestChecker are the two agent stages
// of the fixture belt: the writer keyed draft, the checker keyed check.
func placeholderHelpTestStages() []Stage {
	writerKey := "draft"
	checkerKey := "check"
	writerRef := AgentRef{ID: "writer", Version: 1}
	checkerRef := AgentRef{ID: "checker", Version: 1}
	writer := NewStage(StageArgs{ID: "writer", Type: StageTypeAgent, Name: new("Writer"), Agent: &writerRef, Key: &writerKey, Outcomes: []Outcome{}})
	checker := NewStage(StageArgs{ID: "checker", Type: StageTypeAgent, Name: new("Checker"), Agent: &checkerRef, Key: &checkerKey, Outcomes: []Outcome{}})
	return []Stage{writer, checker}
}

// placeholderHelpTestKey is the key the reference tests give each stage: the writer's
// draft, every other agent's check.
func placeholderHelpTestKey(stage Stage) *string {
	key := "check"
	if stage.Agent != nil && stage.Agent.ID == "writer" {
		key = "draft"
	}
	return &key
}

// placeholderHelpTestReference is PlaceholderHelp.stages with the test key function.
func placeholderHelpTestReference(stages []Stage, current *string) PlaceholderHelp {
	return PlaceholderHelpStages(stages, current, placeholderHelpTestKey, nil, false)
}

// placeholderHelpTestNames is the set of names an entry list offers.
func placeholderHelpTestNames(names []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}

// TestFixedReferenceIsCompleteAndEmptyBeltsStillShowIt checks the first two checks of
// the fixed reference, and the empty belts' help.
func TestFixedReferenceIsCompleteAndEmptyBeltsStillShowIt(t *testing.T) {
	t.Parallel()
	fixed := PlaceholderHelpFixed()
	var names []string
	for _, entry := range fixed {
		names = append(names, entry.Name)
		if entry.Description == "" {
			t.Fatalf("%s has no description", entry.Name)
		}
	}
	if !reflect.DeepEqual(names, BriefFixedPlaceholders) {
		t.Fatalf("fixed names = %v, want %v", names, BriefFixedPlaceholders)
	}
	if got := PlaceholderHelpStages(nil, nil, placeholderHelpTestKey, nil, false); !reflect.DeepEqual(got.Entries, fixed) {
		t.Fatalf("an empty belt's entries = %+v, want the fixed reference", got.Entries)
	}
	if !strings.Contains(fixed[len(fixed)-1].Description, "not a sandbox") {
		t.Fatal("the artifacts description does not warn that HTML is not a sandbox")
	}
}

// TestNamesMatchChipsAndChangedUnsavedDraft
func TestNamesMatchChipsAndChangedUnsavedDraft(t *testing.T) {
	t.Parallel()
	stages := placeholderHelpTestStages()
	help := placeholderHelpTestReference(stages, nil)
	chips := StageAdjustEntries(stages, func(stage Stage) *string { return placeholderHelpTestKey(stage) }, nil, false)
	var chipNames []string
	for _, chip := range chips {
		chipNames = append(chipNames, chip.Key)
	}
	var helpNames []string
	for _, entry := range help.Entries {
		helpNames = append(helpNames, entry.Name)
	}
	if !reflect.DeepEqual(placeholderHelpTestNames(helpNames), placeholderHelpTestNames(append(chipNames, BriefFixedPlaceholders...))) {
		t.Fatalf("help names %v do not match the chips %v and the fixed names", helpNames, chipNames)
	}
	if len(placeholderHelpTestNames(helpNames)) != len(helpNames) {
		t.Fatalf("help names repeat: %v", helpNames)
	}
	if !strings.Contains(help.Entries[0].Description, "Writer") || !strings.Contains(help.Entries[0].Description, "writer") {
		t.Fatalf("first entry %q does not name its stage and id", help.Entries[0].Description)
	}
	stages[0].ID = "unsaved-id"
	stages[0].Name = "Unsaved name"
	changed := placeholderHelpTestReference(stages, nil)
	for _, entry := range changed.Entries {
		if entry.Name == "unsaved-id" {
			t.Fatal("an unsaved stage id became a placeholder")
		}
	}
	found := slices.ContainsFunc(changed.Entries, func(entry PlaceholderHelpEntry) bool {
		return entry.Name == "draft" && strings.Contains(entry.Description, "Unsaved name")
	})
	if !found {
		t.Fatal("the draft entry does not show the unsaved name")
	}
	if reflect.DeepEqual(changed, help) {
		t.Fatal("the changed draft shows the same help")
	}
}

// TestSharedKeysOwnReferencesAndMissingOutput
func TestSharedKeysOwnReferencesAndMissingOutput(t *testing.T) {
	t.Parallel()
	stages := placeholderHelpTestStages()
	second := stages[0]
	second.ID = "another-writer"
	second.Name = "Second writer"
	stages = append(stages, second)
	current := second.ID
	help := placeholderHelpTestReference(stages, &current)
	var own *PlaceholderHelpEntry
	count := 0
	for index := range help.Entries {
		if help.Entries[index].Name == "draft" {
			own = &help.Entries[index]
			count++
		}
	}
	if own == nil || !strings.Contains(own.Description, "Second writer") || !strings.Contains(own.Description, "this attempt's output path") {
		t.Fatalf("draft entry = %+v, want the second writer's own output", own)
	}
	if count != 1 {
		t.Fatalf("draft appears %d times, want once", count)
	}
	earlier := placeholderHelpTestReference(stages, nil)
	var earlierDraft *PlaceholderHelpEntry
	for index := range earlier.Entries {
		if earlier.Entries[index].Name == "draft" {
			earlierDraft = &earlier.Entries[index]
		}
	}
	if earlierDraft == nil || !strings.Contains(earlierDraft.Description, "route the run took") || !strings.Contains(earlierDraft.Description, "(no draft.md yet)") || !strings.Contains(earlierDraft.Description, "(no draft output yet)") {
		t.Fatalf("earlier draft entry = %+v", earlierDraft)
	}
}

// TestSharedKeyAlsoWinsOverOneOfItsOwnStageIds
func TestSharedKeyAlsoWinsOverOneOfItsOwnStageIds(t *testing.T) {
	t.Parallel()
	stages := placeholderHelpTestStages()
	stages[0].ID = "draft"
	stages[1].SetWrites("second.md")
	help := PlaceholderHelpStages(stages, nil, func(Stage) *string { key := "draft"; return &key }, nil, false)
	var entry *PlaceholderHelpEntry
	count := 0
	for index := range help.Entries {
		if help.Entries[index].Name == "draft" {
			entry = &help.Entries[index]
			count++
		}
	}
	if count != 1 || entry == nil {
		t.Fatalf("draft appears %d times", count)
	}
	if !strings.Contains(entry.Description, "Writer") || strings.Contains(entry.Description, "stage id") || !strings.Contains(entry.Description, "(no draft.md yet)") {
		t.Fatalf("draft entry = %q", entry.Description)
	}
}

// TestLegacyArtifactsKeyAndIdRetainOutputMeaning
func TestLegacyArtifactsKeyAndIdRetainOutputMeaning(t *testing.T) {
	t.Parallel()
	stages := placeholderHelpTestStages()
	stages[0].ID = "artifacts"
	entryNamed := func(help PlaceholderHelp, name string) *PlaceholderHelpEntry {
		for index := range help.Entries {
			if help.Entries[index].Name == name {
				return &help.Entries[index]
			}
		}
		return nil
	}
	fresh := entryNamed(placeholderHelpTestReference(stages, nil), "artifacts")
	if fresh == nil || !strings.Contains(fresh.Description, "optional HTML artifact folder") || strings.Contains(fresh.Description, "Stage id") {
		t.Fatalf("fresh artifacts entry = %+v", fresh)
	}
	byID := entryNamed(PlaceholderHelpStages(stages, nil, placeholderHelpTestKey, nil, true), "artifacts")
	if byID == nil || !strings.Contains(byID.Description, "Stage id for Writer") || !strings.Contains(byID.Description, "Legacy artifacts ownership") {
		t.Fatalf("legacy artifacts-by-id entry = %+v", byID)
	}
	current := stages[1].ID
	byKey := PlaceholderHelpStages(stages, &current, func(stage Stage) *string {
		key := "draft"
		if stage.Agent != nil && stage.Agent.ID == "checker" {
			key = "artifacts"
		}
		return &key
	}, nil, true)
	entry := entryNamed(byKey, "artifacts")
	if entry == nil || !strings.Contains(entry.Description, "Agent key for Checker") || !strings.Contains(entry.Description, "stage id of Writer") || !strings.Contains(entry.Description, "does not request artifact instructions") {
		t.Fatalf("legacy artifacts-by-key entry = %+v", entry)
	}
}

// TestAgentHelpOffersFixedPlaceholdersOnly
func TestAgentHelpOffersFixedPlaceholdersOnly(t *testing.T) {
	t.Parallel()
	help := PlaceholderHelpAgent()
	var names []string
	for _, entry := range help.Entries {
		names = append(names, entry.Name)
	}
	if !reflect.DeepEqual(names, BriefFixedPlaceholders) {
		t.Fatalf("names = %v, want %v", names, BriefFixedPlaceholders)
	}
	if help.Qualification != nil {
		t.Fatal("the agent help has a qualification")
	}
	for _, entry := range help.Entries {
		if entry.Name == "output" && !strings.Contains(entry.Description, "current result") {
			t.Fatalf("output description = %q", entry.Description)
		}
	}
}

// TestArtifactsAreOfferedAndReserved checks the fixed-name part of the artifacts check.
// It includes the validation check: an agent keyed artifacts has no warning.
func TestArtifactsAreOfferedAndReserved(t *testing.T) {
	t.Parallel()
	agent := SampleAgent("build")
	agent.Key = "artifacts"
	if warnings := ValidationAgentWarnings(agent, nil); len(warnings) != 0 {
		t.Fatalf("an agent keyed artifacts warns %q", warnings)
	}
	if !slices.Contains(BriefFixedPlaceholders, "artifacts") {
		t.Fatal("artifacts is not a fixed brief name")
	}
	if !slices.Contains(StageAdjustFixed, "artifacts") {
		t.Fatal("the stage adjust fixed names do not offer artifacts")
	}
	if !slices.Contains(StageAdjustPlaceholders(nil, nil), "artifacts") {
		t.Fatal("the offered placeholder names do not include artifacts")
	}
	if !strings.Contains(StageAdjustResolutionNote, "Fixed names keep their meanings.") {
		t.Fatal("the resolution note does not say fixed names keep their meanings")
	}
}

// placeholderHelpTestRun is a run of belt on a.b#1,
// not started.
func placeholderHelpTestRun(belt Belt) Run {
	return NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: 1, Title: "Test", IsOpen: true, Blockers: []string{}}, Now: engineT0})
}

// placeholderHelpTestContext is a brief context with /run, /attempt and
// the given agents.
func placeholderHelpTestContext(agents ...Agent) BriefContext {
	context := BriefContext{RunFolder: "/run", AttemptFolder: "/attempt", Base: "main", Branch: "test", Agents: map[AgentRef]Agent{}}
	for _, agent := range agents {
		context.Agents[agent.Ref()] = agent
	}
	return context
}

// placeholderHelpTestAgentStages is the agent stages of customBelt.
func placeholderHelpTestAgentStages(belt Belt) []Stage {
	var stages []Stage
	for _, stage := range belt.Stages {
		if stage.Kind() == StageKindAgent {
			stages = append(stages, stage)
		}
	}
	return stages
}

// placeholderHelpTestKeyOf is the key function of the rendering tests: the
// writer's draft, every other agent's check.
func placeholderHelpTestKeyOf(stage Stage) *string {
	if stage.Agent != nil && stage.Agent.ID == "writer" {
		return new("draft")
	}
	return new("check")
}

// TestCollisionsDescribeOnlyTheWinnerAndMatchRendering
func TestCollisionsDescribeOnlyTheWinnerAndMatchRendering(t *testing.T) {
	t.Parallel()
	writer, checker := customTestWriter(), customTestChecker()
	stages := placeholderHelpTestAgentStages(customTestBelt(writer, checker))
	stages[1].ID = "draft"
	help := PlaceholderHelpStages(stages, new(stages[1].ID), placeholderHelpTestKeyOf, nil, false)
	entry, ok := placeholderHelpEntry(help, "draft")
	if !ok {
		t.Fatal("no draft entry")
	}
	if !strings.Contains(entry.Description, "Writer") {
		t.Fatalf("draft describes %q, want the writer", entry.Description)
	}
	if strings.Contains(entry.Description, "stage id") || strings.Contains(entry.Description, "this attempt's output path") {
		t.Fatalf("draft describes a loser: %q", entry.Description)
	}
	if _, ok := placeholderHelpEntry(help, "writer"); ok {
		t.Fatal("the writer's stage id is offered")
	}
	run := placeholderHelpTestRun(NewBeltFromTrigger(BeltFromTriggerArgs{ID: "test", Name: "Test", Trigger: Trigger{Kind: TriggerKindManual},
		Repos: []string{"a/b"}, Stages: stages}))
	context := placeholderHelpTestContext(writer, checker)
	if got := briefPlaceholders(run, stages[1], context)["draft"]; got != "(no draft.md yet)" {
		t.Fatalf("draft renders %q, want (no draft.md yet)", got)
	}

	stages[0].ID = "base"
	fixed, ok := placeholderHelpEntry(PlaceholderHelpStages(stages, nil, placeholderHelpTestKeyOf, nil, false), "base")
	if !ok || !strings.HasPrefix(fixed.Description, "The base branch name.") || strings.Contains(fixed.Description, "Collision") {
		t.Fatalf("base describes %+v, want the fixed name", fixed)
	}
	fixedKey := PlaceholderHelpStages(stages, nil, func(Stage) *string { return new("output") }, nil, false)
	if entry, ok := placeholderHelpEntry(fixedKey, "output"); !ok || !strings.HasPrefix(entry.Description, "This attempt's output file path.") {
		t.Fatalf("output describes %+v", entry)
	}
}

// placeholderHelpEntry is the entry of help named name.
func placeholderHelpEntry(help PlaceholderHelp, name string) (PlaceholderHelpEntry, bool) {
	for _, entry := range help.Entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return PlaceholderHelpEntry{}, false
}

// TestFixedAndLegacyCollisionDescriptionsMatchRendering
func TestFixedAndLegacyCollisionDescriptionsMatchRendering(t *testing.T) {
	t.Parallel()
	writer, checker := customTestWriter(), customTestChecker()
	legacyWriter := writer
	legacyWriter.Key = "artifacts"
	stages := placeholderHelpTestAgentStages(customTestBelt(writer, checker))
	stages[1].ID = "artifacts"
	context := placeholderHelpTestContext(legacyWriter, checker)
	belt := NewBeltFromTrigger(BeltFromTriggerArgs{ID: "test", Name: "Test", Trigger: Trigger{Kind: TriggerKindManual}, Repos: []string{"a/b"}, Stages: stages})
	run := placeholderHelpTestRun(belt)
	run.References = nil
	// A run saved before stage keys kept the file each stage wrote.
	for index := range run.Stages {
		writes := run.Stages[index].Writes()
		run.Stages[index].SetKey(nil)
		run.Stages[index].SetWrites(writes)
	}
	if got := briefPlaceholders(run, stages[1], context)["artifacts"]; got != "(no draft.md yet)" {
		t.Fatalf("artifacts = %q, want (no draft.md yet)", got)
	}
	if briefRequestsArtifacts("{artifacts}", run, context) {
		t.Fatal("a legacy collision requests artifact instructions")
	}
	if got := briefPlaceholders(run, run.Stages[0], context)["artifacts"]; got != "/attempt/draft.md" {
		t.Fatalf("writer artifacts = %q, want /attempt/draft.md", got)
	}

	normalContext := placeholderHelpTestContext(writer, checker)
	if got := briefPlaceholders(run, run.Stages[0], normalContext)["artifacts"]; got != "(no check.md yet)" {
		t.Fatalf("artifacts with the writer's own key = %q, want (no check.md yet)", got)
	}
	if briefRequestsArtifacts("{artifacts}", run, normalContext) {
		t.Fatal("artifacts requested with the writer's own key")
	}

	legacyWriter.Key = "output"
	normalContext.Agents[writer.Ref()] = legacyWriter
	belt.Stages[0].ID = "base"
	run = placeholderHelpTestRun(belt)
	values := briefPlaceholders(run, belt.Stages[1], normalContext)
	if values["output"] != "/attempt/check.md" {
		t.Fatalf("output = %q, want /attempt/check.md", values["output"])
	}
	if values["base"] != "main" {
		t.Fatalf("base = %q, want main", values["base"])
	}
}

// TestRenderingReplacesKnownFencedTokensAndPreservesUnknownTokens
func TestRenderingReplacesKnownFencedTokensAndPreservesUnknownTokens(t *testing.T) {
	t.Parallel()
	writer, checker := customTestWriter(), customTestChecker()
	belt := customTestBelt(writer, checker)
	belt.Stages[1].Brief = "```\n{output} {draft} {unknown} {artifacts}\n```"
	run := placeholderHelpTestRun(belt)
	rendered := BriefRender(run, belt.Stages[1], placeholderHelpTestContext(writer, checker))
	if !strings.Contains(rendered, "/attempt/check.md (no draft.md yet) {unknown} /attempt/artifacts") {
		t.Fatalf("rendered brief lacks the replaced tokens:\n%s", rendered)
	}
	if strings.Contains(rendered, "## Artifacts") {
		t.Fatalf("a brief without an artifacts request has an artifacts section:\n%s", rendered)
	}
}
