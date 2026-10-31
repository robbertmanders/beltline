package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// upgradeOldBelt is a belt version as it was saved with one trigger, the
// afkEnabled switch and a build stage from before agents had their own files.
const upgradeOldBelt = `{"id":"epics","name":"Epics","version":1,"afkEnabled":true,` +
	`"trigger":{"kind":"jira","jql":"project = PAY","poll":30,"repos":["acme/billing"]},` +
	`"stages":[{"id":"build","type":"build","name":"Build","outcomes":[]}]}`

// upgradeOldFolder is a data folder an older version saved: settings with a
// factory repository, the old belt with a state that only knows lastFired,
// and the golden run as it was saved before sources, agent copies and stage keys.
func upgradeOldFolder(t *testing.T) (Storage, string) {
	t.Helper()
	s := seededStorage(t)
	writeText(t, s.Paths.Settings(), `{"login":"me","factoryRepository":"owner/factories"}`)
	writeText(t, filepath.Join(s.Paths.Belts(), "epics", "v1.json"), upgradeOldBelt)
	writeText(t, filepath.Join(s.Paths.Belts(), "epics", "state.json"),
		`{"paused":false,"archived":false,"lastFired":"2026-01-01T00:00:00Z","watchOutcomes":true,"reviewOutcomes":true}`)
	run := goldenRun(t)
	var fields map[string]any
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "references")
	delete(fields, "source")
	fields["agents"] = []any{}
	for _, stage := range fields["stages"].([]any) {
		delete(stage.(map[string]any), "key")
	}
	path := filepath.Join(s.Paths.RunFolder(run), "run.json")
	old, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	writeText(t, path, string(old))
	return s, path
}

// upgradeFields decodes a file into its top-level keys.
func upgradeFields(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(fileText(t, path)), &fields); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return fields
}

// TestUpgradeRewritesAnOldFolderOnce checks every old file is written in the
// current format, its original kept in the backup, and that a folder already
// upgraded is left alone.
func TestUpgradeRewritesAnOldFolderOnce(t *testing.T) {
	t.Parallel()
	s, runPath := upgradeOldFolder(t)
	beltPath := filepath.Join(s.Paths.Belts(), "epics", "v1.json")
	originalRun := fileText(t, runPath)
	report, err := s.Upgrade(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ran || !report.Settings || report.Belts != 1 || report.States != 1 || report.Runs != 1 || len(report.Unreadable) != 0 {
		t.Fatalf("report %+v", report)
	}
	if want := filepath.Join(s.Paths.UpgradeBackups(), "20261016T090000Z"); report.Backup != want {
		t.Fatalf("backup %q, want %q", report.Backup, want)
	}
	if fileText(t, filepath.Join(report.Backup, "belts", "epics", "v1.json")) != upgradeOldBelt ||
		fileText(t, filepath.Join(report.Backup, "runs", s.Paths.RunFolder(goldenRun(t))[len(s.Paths.Runs())+1:], "run.json")) != originalRun {
		t.Fatal("the backup does not hold the files as they were")
	}

	belt := upgradeFields(t, beltPath)
	if _, ok := belt["trigger"]; ok || string(belt["afk"]) != `"always"` {
		t.Fatalf("belt %s", fileText(t, beltPath))
	}
	build, _ := s.AgentRef(core.AgentRef{ID: "build", Version: 1})
	var stored core.Belt
	if err := json.Unmarshal([]byte(fileText(t, beltPath)), &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Triggers) != 1 || stored.Triggers[0].Repos[0] != "acme/billing" || stored.Stages[0].Type != core.StageTypeAgent ||
		core.ValueOf(stored.Stages[0].Key) != build.Key {
		t.Fatalf("belt %+v", stored)
	}

	state, ok := s.StoredState("epics")
	if fired, has := state.Schedules["1"]; !ok || len(state.Schedules) != 1 || !has || !fired.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("state %s", fileText(t, filepath.Join(s.Paths.Belts(), "epics", "state.json")))
	}
	settings := upgradeFields(t, s.Paths.Settings())
	repos := s.LoadSettings().LibraryRepositories
	if _, ok := settings["factoryRepository"]; ok || len(repos) != 2 || repos[0].Repository != "owner/factories" || repos[1] != core.LibraryExamples() {
		t.Fatalf("settings %s", fileText(t, s.Paths.Settings()))
	}

	var run core.Run
	if err := json.Unmarshal([]byte(fileText(t, runPath)), &run); err != nil {
		t.Fatal(err)
	}
	if !run.UsesStageKeys() || run.Source == nil || run.Source.Entry != "1" || len(run.Agents) == 0 {
		t.Fatalf("run references %v source %+v agents %d", run.References, run.Source, len(run.Agents))
	}
	if key := core.StageKeysChosen(run.Stages[0]); key != "plan" {
		t.Errorf("plan-epic key %q, want plan", key)
	}
	for _, stage := range run.Stages {
		if stage.Kind() == core.StageKindAgent && core.StageFile(stage.Key, stage.Type) != stage.Writes() {
			t.Errorf("%s: key %v moved its file to %s", stage.ID, stage.Key, stage.Writes())
		}
	}
	if format := fileText(t, s.Paths.Format()); format == "" {
		t.Fatal("no format.json")
	}

	again, err := s.Upgrade(time.Now())
	if err != nil || again.Ran {
		t.Fatalf("second upgrade %+v %v", again, err)
	}
	if err := os.Remove(s.Paths.Format()); err != nil {
		t.Fatal(err)
	}
	third, err := s.Upgrade(time.Now())
	if err != nil || !third.Ran || third.Settings || third.Belts+third.States+third.Runs != 0 || third.Backup != "" {
		t.Fatalf("an upgraded folder changed again: %+v %v", third, err)
	}
}

// TestUpgradeLeavesAnUnreadableFileAsItIs checks a file that does not decode
// is reported and kept, and the upgrade still finishes.
func TestUpgradeLeavesAnUnreadableFileAsItIs(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	broken := filepath.Join(s.Paths.Belts(), "broken", "v1.json")
	writeText(t, broken, `{"id":`)
	report, err := s.Upgrade(time.Now())
	if err != nil || !report.Ran || len(report.Unreadable) != 1 || report.Unreadable[0] != filepath.Join("belts", "broken", "v1.json") {
		t.Fatalf("report %+v %v", report, err)
	}
	if fileText(t, broken) != `{"id":` {
		t.Fatal("the unreadable file was changed")
	}
}

// TestUpgradeRefusesAFolderInANewerFormat checks a folder a newer version saved
// is refused and left exactly as it is.
func TestUpgradeRefusesAFolderInANewerFormat(t *testing.T) {
	t.Parallel()
	s, path := upgradeOldFolder(t)
	writeText(t, s.Paths.Format(), `{"version":`+strconv.Itoa(core.UpgradeFormatVersion+1)+`}`)
	before := fileText(t, path)
	report, err := s.Upgrade(time.Now())
	var newer NewerFormat
	if !errors.As(err, &newer) || newer.Found != core.UpgradeFormatVersion+1 || newer.Current != core.UpgradeFormatVersion || report.Ran {
		t.Fatalf("report %+v %v, want a NewerFormat refusal", report, err)
	}
	if fileText(t, path) != before {
		t.Fatal("a refused upgrade changed a run")
	}
	if _, err := os.Stat(s.Paths.UpgradeBackups()); err == nil {
		t.Fatal("a refused upgrade made a backup")
	}
}

// TestUpgradeSavesTheSampleAgentsAFolderPins checks the agent versions an older
// folder pinned without saving, which the app used to read from itself, are
// saved with the text it read, and that only those are.
func TestUpgradeSavesTheSampleAgentsAFolderPins(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	writeText(t, filepath.Join(s.Paths.Belts(), "epics", "v1.json"), upgradeOldBelt)
	belt := core.SampleBelt([]string{"acme/app"}, "agent", nil, true)
	for index := range belt.Stages {
		if ref := belt.Stages[index].Agent; ref != nil && ref.ID == "spec" {
			belt.Stages[index].Agent = &core.AgentRef{ID: "spec", Version: 1}
		}
	}
	if err := s.Write(belt, filepath.Join(s.Paths.Belts(), belt.ID, "v1.json")); err != nil {
		t.Fatal(err)
	}
	report, err := s.Upgrade(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []core.AgentRef{{ID: "build", Version: 1}, {ID: "triage", Version: 2}, {ID: "wrap-up", Version: 2}} {
		want, _ := core.SampleAgentText(ref.ID)
		if got := fileText(t, filepath.Join(s.agentFolder(ref.ID), "v"+strconv.Itoa(ref.Version)+".md")); got != want {
			t.Errorf("%s v%d was not saved as the app read it", ref.ID, ref.Version)
		}
	}
	if _, err := os.Stat(filepath.Join(s.agentFolder("spec"), "v1.md")); err == nil {
		t.Error("spec v1, which the app never read from itself, was saved")
	}
	if _, err := os.Stat(filepath.Join(s.agentFolder("plan-epic"), "v1.md")); err == nil {
		t.Error("an agent no belt or run pins was saved")
	}
	if report.Agents != len(s.LoadAgents()) || report.Agents == 0 {
		t.Errorf("report counts %d agents, the library holds %d", report.Agents, len(s.LoadAgents()))
	}
}

// TestUpgradeListsTheExamplesRepositoryOnce checks a new folder gets settings
// with the examples repository, and settings that list it already keep their entry.
func TestUpgradeListsTheExamplesRepositoryOnce(t *testing.T) {
	t.Parallel()
	fresh := newTestStorage(t)
	report, err := fresh.Upgrade(time.Now())
	if err != nil || !report.Settings || report.Backup != "" {
		t.Fatalf("report %+v %v", report, err)
	}
	if repos := fresh.LoadSettings().LibraryRepositories; len(repos) != 1 || repos[0] != core.LibraryExamples() {
		t.Fatalf("a new folder lists %+v", repos)
	}

	listed := newTestStorage(t)
	own := core.LibraryRepository{Repository: core.LibraryExamplesRepository, Path: "mine", Role: core.LibraryRoleShared, Publish: core.LibraryPublishPush}
	settings := DefaultSettings()
	settings.LibraryRepositories = []core.LibraryRepository{own}
	if err := listed.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := listed.Upgrade(time.Now()); err != nil {
		t.Fatal(err)
	}
	if repos := listed.LoadSettings().LibraryRepositories; len(repos) != 1 || repos[0] != own {
		t.Fatalf("settings that list the examples repository became %+v", repos)
	}
}

// upgradeShadowFolder is a format 3 folder with one belt and its state: the belt's
// shadow test is recorded as given, the belt is paused and archived when asked, and the
// test's shadow version (v2) is saved beside the live one, as the app saves it. It returns the belt's id.
func upgradeShadowFolder(t *testing.T, test core.ShadowTest, paused, archived bool) (Storage, string) {
	t.Helper()
	s := newTestStorage(t)
	belt := core.SampleBelt([]string{"acme/app"}, "tasks", nil, true)
	record := shadowRecord(t, s, belt)
	_, record, err := s.SaveShadowVersion(belt, record)
	if err != nil {
		t.Fatal(err)
	}
	record.ShadowTests = []core.ShadowTest{test}
	record.Paused, record.Archived = paused, archived
	if err := s.SaveState(record); err != nil {
		t.Fatal(err)
	}
	writeText(t, s.Paths.Format(), `{"version":3}`)
	return s, record.Belt.ID
}

// TestUpgradeEnablesShadowTestsOnlyWhileATestRuns checks the switch an upgrade infers: on for a folder with a
// running test, paused or archived belts included, and off for ended tests alone. It keeps the Insights switch,
// leaves the belt, its live and shadow versions, its test and the run records as they were, and does not infer it again.
func TestUpgradeEnablesShadowTestsOnlyWhileATestRuns(t *testing.T) {
	t.Parallel()
	running := core.ShadowTest{ID: "t1", Live: 1, Shadow: 2, StartedAt: jsonDate(1_700_000_000)}
	ended := running
	ended.EndedAt, ended.Result = new(jsonDate(1_700_100_000)), core.ShadowResultAccepted
	cases := []struct {
		name        string
		test        core.ShadowTest
		paused      bool
		archived    bool
		wantEnabled bool
	}{
		{name: "running", test: running, wantEnabled: true},
		{name: "running on a paused belt", test: running, paused: true, wantEnabled: true},
		{name: "running on an archived belt", test: running, archived: true, wantEnabled: true},
		{name: "ended", test: ended, wantEnabled: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, id := upgradeShadowFolder(t, tc.test, tc.paused, tc.archived)
			writeText(t, s.Paths.Settings(), `{"login":"me","showInsights":true}`)
			// A run still going on the test's shadow: it does not decide the switch, only the test does.
			run := goldenRun(t)
			run.Shadow = &core.RunShadow{Test: "t1", Arm: core.ShadowArmShadow}
			runPath := filepath.Join(s.Paths.RunFolder(run), "run.json")
			if err := s.Write(run, runPath); err != nil {
				t.Fatal(err)
			}
			beltState := fileText(t, filepath.Join(s.Paths.Belts(), id, "state.json"))
			beltVersion := fileText(t, filepath.Join(s.Paths.Belts(), id, "v1.json"))
			shadowVersion := fileText(t, filepath.Join(s.Paths.Belts(), id, "shadow", "v2.json"))

			report, err := s.Upgrade(time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC))
			if err != nil || !report.Ran || !report.Settings {
				t.Fatalf("report %+v %v", report, err)
			}
			settings := s.LoadSettings()
			if settings.EnableShadowTests != tc.wantEnabled || !settings.ShowInsights || settings.Login != "me" {
				t.Fatalf("settings %+v, want shadow tests %v with Insights kept", settings, tc.wantEnabled)
			}
			if fileText(t, filepath.Join(report.Backup, "settings.json")) != `{"login":"me","showInsights":true}` {
				t.Fatal("the original settings were not backed up")
			}
			var kept core.Run
			if err := json.Unmarshal([]byte(fileText(t, runPath)), &kept); err != nil || kept.BeltVersion != run.BeltVersion || kept.Shadow == nil || *kept.Shadow != *run.Shadow {
				t.Fatalf("the run on the test lost its version or its test: %+v, %v", kept.Shadow, err)
			}
			if fileText(t, filepath.Join(s.Paths.Belts(), id, "state.json")) != beltState ||
				fileText(t, filepath.Join(s.Paths.Belts(), id, "v1.json")) != beltVersion ||
				fileText(t, filepath.Join(s.Paths.Belts(), id, "shadow", "v2.json")) != shadowVersion {
				t.Fatal("the belt, its live or shadow version, or its test records changed")
			}

			saved := fileText(t, s.Paths.Settings())
			if again, err := s.Upgrade(time.Now()); err != nil || again.Ran || fileText(t, s.Paths.Settings()) != saved {
				t.Fatalf("a second upgrade %+v %v changed the settings", again, err)
			}
			if fileText(t, filepath.Join(s.Paths.Belts(), id, "shadow", "v2.json")) != shadowVersion ||
				fileText(t, filepath.Join(s.Paths.Belts(), id, "state.json")) != beltState {
				t.Fatal("a second upgrade changed the shadow version or the test records")
			}
		})
	}
}

// TestUpgradeKeepsAnExplicitShadowTestChoice checks a settings file that already names the switch is not
// inferred again, whether or not a test runs, and that a fresh folder starts with shadow tests off.
func TestUpgradeKeepsAnExplicitShadowTestChoice(t *testing.T) {
	t.Parallel()
	s, _ := upgradeShadowFolder(t, core.ShadowTest{ID: "t1", Live: 1, Shadow: 2, StartedAt: jsonDate(1_700_000_000)}, false, false)
	writeText(t, s.Paths.Settings(), `{"login":"me","enableShadowTests":false}`)
	if _, err := s.Upgrade(time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.LoadSettings().EnableShadowTests {
		t.Fatal("an explicit false was inferred as true")
	}

	fresh := newTestStorage(t)
	if _, err := fresh.Upgrade(time.Now()); err != nil {
		t.Fatal(err)
	}
	if fresh.LoadSettings().EnableShadowTests {
		t.Fatal("a fresh folder starts with shadow tests on")
	}
}

// jsonDate is a jsonx.Date at the given Unix second.
func jsonDate(seconds int64) jsonx.Date {
	return jsonx.Date{Time: time.Unix(seconds, 0).UTC()}
}
