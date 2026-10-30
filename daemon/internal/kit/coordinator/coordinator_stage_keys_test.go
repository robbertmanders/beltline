// Coordinator tests of stage keys: saving stores keys, and a new run keeps them after the library
// changes. Placeholder assertions go through core.BriefRender: a key that resolves is replaced in the
// brief, and one that does not stays as written.

package coordinator

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestSavingStoresKeysAndANewRunKeepsThemAfterTheLibraryChanges: a belt saved without keys stores none; the keys a
// draft converts to are saved with it; a run started afterwards keeps them when the library changes.
func TestSavingStoresKeysAndANewRunKeepsThemAfterTheLibraryChanges(t *testing.T) {
	t.Parallel()
	belt := core.SampleBelt([]string{"a/b"}, "agent", core.SampleAgents(), true)
	belt.Triggers = []core.TriggerEntry{manualEntry("manual", "a/b")}
	for i := range belt.Stages {
		belt.Stages[i].SetKey(nil)
	}
	h := newLibraryHarness(t, belt)
	c := h.Coord()
	saved := beltRecordOf(t, c, belt.ID).Belt
	file := filepath.Join(h.root, "belts", saved.ID, "v"+strconv.Itoa(saved.Version)+".json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read belt file: %v", err)
	}
	if strings.Contains(string(raw), `"key"`) {
		t.Errorf("a belt saved without keys stores keys: %s", raw)
	}

	editing := beltRecordOf(t, c, saved.ID).Belt
	if beltStageNamedForTest(editing, "spec").Key != nil {
		t.Errorf("a saved stage without a key has one: %v", beltStageNamedForTest(editing, "spec").Key)
	}
	converted := c.ConvertingLegacyKeys(editing)
	if key := beltStageNamedForTest(converted, "spec").Key; key == nil || *key != "spec" {
		t.Errorf("converted spec key = %v, want spec", key)
	}
	if key := beltStageNamedForTest(editing, "spec").Key; key != nil {
		t.Errorf("converting changed the saved draft's spec key to %q", *key)
	}
	intakeStarted, err := c.StartTask(saved.ID, "a/b", "Old belt", "", nil)
	if err != nil {
		t.Fatalf("StartTask: %v", err)
	}
	if key := stageNamedForRun(intakeStarted, "spec").Key; key == nil || *key != "spec" {
		t.Errorf("the started run's spec key = %v", key)
	}
	if after, err := os.ReadFile(file); err != nil || string(after) != string(raw) {
		t.Errorf("starting a run rewrote the belt file: %v", err)
	}

	for i := range editing.Stages {
		if editing.Stages[i].Type == core.StageTypeAgent {
			editing.Stages[i].Brief = "Read {triage}."
		}
	}
	editing.Stages[1].SetKey(new("design"))
	editing.Stages[1].Brief = "{triage} {design}"
	stored, err := c.SaveBelt(editing)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if key := stored.Stages[0].Key; key == nil || *key != "triage" {
		t.Errorf("stored first key = %v, want triage", key)
	}
	if key := stored.Stages[1].Key; key == nil || *key != "design" {
		t.Errorf("stored second key = %v, want design", key)
	}
	reloaded := beltRecordOf(t, coordinatorOn(t, h.storage), stored.ID).Belt
	if key := reloaded.Stages[1].Key; key == nil || *key != "design" {
		t.Errorf("reloaded second key = %v, want design", key)
	}

	broken := stored.Clone()
	broken.Stages[3].Brief = "See {approve}."
	warned, err := c.SaveBelt(broken)
	if err != nil {
		t.Fatalf("save broken: %v", err)
	}
	if !anyContains(c.Warnings(warned), "{approve}") {
		t.Errorf("warnings = %v, want one about {approve}", c.Warnings(warned))
	}
	if !beltRecordOf(t, c, warned.ID).Paused {
		t.Error("the belt with a warning is not paused")
	}

	specRef := beltStageNamedForTest(stored, "spec").Agent
	if specRef == nil {
		t.Fatal("the spec stage has no agent")
	}
	spec := c.Agent(specRef)
	if spec == nil {
		t.Fatal("the spec agent is missing")
	}
	spec.Key = "specification"
	if _, err := c.SaveAgent(*spec); err != nil {
		t.Fatalf("save agent: %v", err)
	}

	fresh := core.NewRun(core.RunArgs{
		Belt:    reloaded,
		Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "a/b", Number: 9, Title: "Kept"},
		Now:     sessionSampleT0,
	})
	if key := fresh.Stages[1].Key; key == nil || *key != "design" {
		t.Errorf("the fresh run's second key = %v", key)
	}
	if !fresh.UsesStageKeys() {
		t.Error("the fresh run does not use stage keys")
	}
	// Brief placeholders: "design" resolves in the brief, "specification" is not a key of this belt's run.
	ctx := core.BriefContext{RunFolder: h.root, AttemptFolder: h.root, Base: "main", Branch: "b", Agents: c.AgentsOf(fresh)}
	probe := fresh.Stages[2]
	probe.Brief = "{design} {specification}"
	rendered := core.BriefRender(fresh, probe, ctx)
	if strings.Contains(rendered, "{design}") {
		t.Errorf("the design placeholder did not resolve: %q", rendered)
	}
	if !strings.Contains(rendered, "{specification}") {
		t.Errorf("the specification placeholder resolved: %q", rendered)
	}
}

// stageNamedForRun returns the run's stage with id "spec", or the zero stage.
func stageNamedForRun(run core.Run, id string) core.Stage {
	for _, stage := range run.Stages {
		if stage.ID == id {
			return stage
		}
	}
	return core.Stage{}
}

// anyContains reports whether one of texts contains want.
func anyContains(texts []string, want string) bool {
	return slices.ContainsFunc(texts, func(text string) bool { return strings.Contains(text, want) })
}
