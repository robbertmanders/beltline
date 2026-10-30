package demo

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// demoHistoryRuns is the number of shipped runs seedHistory writes: one per
// entry of its weekly counts, 1+1+2+2+3+2+3+4+4+5+6+7.
const demoHistoryRuns = 40

// TestSeedLoadsBackThroughStore: every belt, agent, run, setting and decision of
// the seed decodes through the store, and every run file re-encodes to the
// bytes it was written with.
func TestSeedLoadsBackThroughStore(t *testing.T) {
	t.Parallel()
	dir := sharedSeed(t)
	storage := store.NewStorage(store.Paths{Root: dir})

	records := storage.LoadBelts()
	if want := len(Belts()); len(records) != want {
		t.Errorf("belts loaded = %d, want %d", len(records), want)
	}
	for _, record := range records {
		if storage.Belt(record.Belt.ID, record.Belt.Version) == nil {
			t.Errorf("belt %s v%d does not decode", record.Belt.ID, record.Belt.Version)
		}
	}
	if agents := storage.LoadAgents(); len(agents) != len(core.SampleAgentIDs) {
		t.Errorf("agents loaded = %d, want %d", len(agents), len(core.SampleAgentIDs))
	}

	runs := storage.LoadRuns()
	if want := len(demoSeeds()) + 1 + demoHistoryRuns; len(runs) != want {
		t.Errorf("runs loaded = %d, want %d", len(runs), want)
	}
	again := t.TempDir()
	reencoded := store.NewStorage(store.Paths{Root: again})
	for _, run := range runs {
		if err := reencoded.SaveRun(run); err != nil {
			t.Fatalf("re-save %s: %v", run.ID, err)
		}
		rel := filepath.Join(strings.TrimPrefix(storage.Paths.RunFolder(run), dir), "run.json")
		want, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(again, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("run %s does not round-trip (%d bytes written, %d re-encoded)", run.ID, len(want), len(got))
		}
	}

	settings := storage.LoadSettings()
	if settings.ConfigurationModel != "opus-5.5-1m" || settings.Afk.Model != "opus-5.5-250k" {
		t.Errorf("configuration and AFK models = %q, %q", settings.ConfigurationModel, settings.Afk.Model)
	}
	if len(settings.ModelCatalogs) != len(core.AllBackend) {
		t.Errorf("model catalogs = %d, want one per runner (%d)", len(settings.ModelCatalogs), len(core.AllBackend))
	}
	decisions := storage.LoadAfkDecisions()
	if len(decisions) != 2 {
		t.Errorf("AFK decisions = %d, want 2", len(decisions))
	}
	if len(settings.LibraryRepositories) != 2 {
		t.Errorf("library repositories = %+v, want the shared and the backup repository", settings.LibraryRepositories)
	}
}

// sessionIDPattern matches the session id of an attempt, the one random part of the seed.
var sessionIDPattern = regexp.MustCompile(`"sessionId" ?: ?"[^"]*"`)

// TestSeedIsIdenticalForTheSameNow: two seeds with the same now write the same
// files, except the session ids of attempts, which are random. The
// second seed is built while the shared one is, so the test costs one build of
// wall time.
func TestSeedIsIdenticalForTheSameNow(t *testing.T) {
	t.Parallel()
	second := t.TempDir()
	built := make(chan error, 1)
	go func() {
		_, err := Make(second, testkit.DemoTime, noEnv)
		built <- err
	}()
	first := sharedSeed(t)
	if err := <-built; err != nil {
		t.Fatal(err)
	}
	firstFiles := walkFiles(t, first)
	secondFiles := walkFiles(t, second)
	if testkit.Diff(firstFiles, secondFiles) != "" {
		t.Fatalf("file lists differ: %s", testkit.Diff(firstFiles, secondFiles))
	}
	rawSessionDiffs := 0
	for _, rel := range firstFiles {
		a, err := os.ReadFile(filepath.Join(first, rel))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(second, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			rawSessionDiffs++
		}
		const placeholder = `"sessionId":"?"`
		if !bytes.Equal(sessionIDPattern.ReplaceAll(a, []byte(placeholder)), sessionIDPattern.ReplaceAll(b, []byte(placeholder))) {
			t.Errorf("%s differs beyond session ids", rel)
		}
	}
	if rawSessionDiffs == 0 {
		t.Errorf("no file differs at all; the session ids should be random")
	}
}

// walkFiles lists the regular files under root, relative and sorted.
func walkFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestLongMarkdownSeedWritesTheSample: with BELTLINE_DEMO_LONG_MARKDOWN=1 the
// issue run's last spec output is the long Markdown sample; without it, nothing
// changes. The run is one the engine takes through triage and spec, saved in a
// folder of its own, so the test writes no shared data.
func TestLongMarkdownSeedWritesTheSample(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	storage := store.NewStorage(store.Paths{Root: root})
	issues := Belts()[0].Belt
	subject := demoFixtures()[0].Subject
	if subject.Ref() != "#139" {
		t.Fatalf("first fixture is %s, want #139", subject.Ref())
	}
	run := core.NewRun(core.RunArgs{Belt: issues, Subject: subject, Now: testkit.DemoTime})
	core.EngineStart(&run, testkit.DemoTime)
	for _, outcome := range []string{"ready to spec", "complete"} {
		if err := complete(&run, outcome, storage, testkit.DemoTime); err != nil {
			t.Fatal(err)
		}
	}
	if run.StageID != "approve" {
		t.Fatalf("run is at %s, want approve", run.StageID)
	}
	if err := storage.SaveRun(run); err != nil {
		t.Fatal(err)
	}
	var spec *core.Attempt
	for i := range run.Attempts {
		if run.Attempts[i].StageID == "spec" {
			spec = &run.Attempts[i]
		}
	}
	if spec == nil {
		t.Fatal("no spec attempt")
	}
	writes := ""
	for _, stage := range run.Stages {
		if stage.ID == "spec" {
			writes = stage.Writes()
		}
	}
	file := filepath.Join(storage.Paths.AttemptFolder(run, *spec), writes)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedLongMarkdown(storage, noEnv); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); !bytes.Equal(got, before) {
		t.Errorf("the sample was written without the switch")
	}
	long := func(key string) string {
		if key == "BELTLINE_DEMO_LONG_MARKDOWN" {
			return "1"
		}
		return ""
	}
	if err := seedLongMarkdown(storage, long); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != seedLongMarkdownSample() {
		t.Errorf("long markdown differs from the sample")
	}
}
