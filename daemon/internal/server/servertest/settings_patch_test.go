// Contract tests of the settings patch: partial and persistent, refusal of invalid candidates, wrong
// methods, bodies and auth.

package servertest

import (
	"reflect"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// settingsPatchPath is the settings patch command's path.
const settingsPatchPath = "/api/v1/settings/patch"

// postSettingsPatch sends body as a settings patch with the contract token.
func postSettingsPatch(f *Fixture, body string) Reply {
	return f.Do(Request{Method: "POST", Path: settingsPatchPath, Token: ContractToken, Body: []byte(body)})
}

// TestSettingsPatchIsPartialPersistentAndRejectsInvalidCandidates checks that settings patches are partial and persistent, and reject invalid candidates.
// settingsPatchIsPartialPersistentAndRejectsInvalidCandidates.
func TestSettingsPatchIsPartialPersistentAndRejectsInvalidCandidates(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})

	reply := postSettingsPatch(f, `{"notifications":true,"libraryRepositories":[{"repository":"owner/repo","path":"","role":"shared","publish":"pullRequest"}]}`)
	CheckGolden(t, "settings patch", "command-settings-patch.json", reply, f.Root)
	var response struct {
		Notifications       bool                     `json:"notifications"`
		LibraryRepositories []core.LibraryRepository `json:"libraryRepositories"`
	}
	if err := json.Unmarshal(reply.Body, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Notifications || len(response.LibraryRepositories) != 1 || response.LibraryRepositories[0].Repository != "owner/repo" {
		t.Fatalf("patch: notifications %v, libraryRepositories %v", response.Notifications, response.LibraryRepositories)
	}
	if got := f.Coordinator.ReadSettings().LibraryRepositories; len(got) != 1 || got[0].Repository != "owner/repo" {
		t.Fatalf("coordinator holds libraryRepositories %v", got)
	}
	if got := store.NewStorage(store.Paths{Root: f.Root}).LoadSettings().LibraryRepositories; len(got) != 1 || got[0].Repository != "owner/repo" {
		t.Fatalf("accepted patches persist: saved %v", got)
	}

	unchanged := f.Coordinator.ReadSettings()
	empty := postSettingsPatch(f, `{}`)
	CheckGolden(t, "settings patch", "command-settings-patch.json", empty, f.Root)
	if !reflect.DeepEqual(f.Coordinator.ReadSettings(), unchanged) {
		t.Fatalf("an empty patch changed settings")
	}

	// A rejected candidate never partially writes the accepted fields.
	for _, invalid := range []string{
		`{"notifications":false,"afk":{"limit":11}}`,
		`{"notifications":false,"afk":{"limit":0}}`,
		`{"notifications":false,"afk":{"limit":2.5}}`,
		`{"notifications":false,"libraryRepositories":[{"repository":"not-a-repository","path":"","role":"shared","publish":"pullRequest"}]}`,
		`{"paused":true}`,
		`{"modelCatalogs":{}}`,
		`{"allowedModels":{}}`,
		`{"configurationBackend":"pi"}`,
		`{"notifications":"yes"}`,
		`{"notifications":null}`,
		`{"afk":{"thinking":"high","guidance":null}}`,
		`{"afk":null}`,
	} {
		rejected := postSettingsPatch(f, invalid)
		if rejected.Status != 422 {
			t.Fatalf("patch %s: status %d, want 422", invalid, rejected.Status)
		}
		if !reflect.DeepEqual(f.Coordinator.ReadSettings(), unchanged) {
			t.Fatalf("rejected patch %s changed settings", invalid)
		}
	}

	// Disabled blank AFK stays editable; it just does not authorize AFK.
	blank := postSettingsPatch(f, `{"afk":{"model":"","guidance":"keep it short"}}`)
	CheckGolden(t, "settings patch disabled", "command-settings-patch-disabled.json", blank, f.Root)
	var afk struct {
		Afk struct {
			Model string `json:"model"`
		} `json:"afk"`
	}
	if err := json.Unmarshal(blank.Body, &afk); err != nil {
		t.Fatal(err)
	}
	if afk.Afk.Model != "" {
		t.Fatalf("blank AFK model: %q", afk.Afk.Model)
	}
	if got := f.Coordinator.ReadSettings().Afk.Guidance; got != "keep it short" {
		t.Fatalf("AFK guidance %q", got)
	}
}

// TestSettingsPatchRejectsWrongMethodsBodiesAndAuth checks the settings lines of the wrong-method, body and auth cases.
// commandTransportRejectsWrongMethodsUnknownRunnersBodiesAndAuth.
func TestSettingsPatchRejectsWrongMethodsBodiesAndAuth(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	if got := f.Do(Request{Method: "POST", Path: settingsPatchPath}).Status; got != 401 {
		t.Fatalf("no token: status %d, want 401", got)
	}
	if got := f.Do(Request{Method: "POST", Path: settingsPatchPath, Token: "wrong", Body: []byte(`{}`)}).Status; got != 401 {
		t.Fatalf("wrong token: status %d, want 401", got)
	}
	// `{}` is a valid no-op patch; an unknown field is refused before the command runs.
	if got := postSettingsPatch(f, `{}`).Status; got != 200 {
		t.Fatalf("empty patch: status %d, want 200", got)
	}
	if got := postSettingsPatch(f, `{"unknownField":true}`).Status; got != 422 {
		t.Fatalf("unknown field: status %d, want 422", got)
	}
	if got := f.Do(Request{Method: "GET", Path: settingsPatchPath, Token: ContractToken}).Status; got != 404 {
		t.Fatalf("GET of the patch route: status %d, want 404", got)
	}
}

// getSettings reads the settings with the contract token.
func getSettings(f *Fixture) Reply {
	return f.Do(Request{Method: "GET", Path: "/api/v1/settings", Token: ContractToken})
}

// TestShadowTestSwitchRefusesShadowsWhileOff checks over HTTP that a shadow cannot start while Settings keeps
// shadow tests off: the save is refused with the shared reason and stores nothing, and the read reports the
// switch available to turn off once nothing runs.
func TestShadowTestSwitchRefusesShadowsWhileOff(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{Empty: true})
	draft := ContractSaveDraft()
	if save := f.Do(Request{Method: "POST", Path: "/api/v1/belts/save", Body: contractBody(t, api.BeltSaveRequest{Belt: draft}), Token: ContractToken}); save.Status != 200 {
		t.Fatalf("save the belt: %d %s", save.Status, save.Body)
	}
	shadow := draft.Clone()
	shadow.Stages[0].Name = "Shadow " + shadow.Stages[0].Name
	shadowBody := contractBody(t, api.BeltSaveRequest{Belt: shadow})
	if refused := f.Do(Request{Method: "POST", Path: "/api/v1/belts/contract-draft/shadow/save", Body: shadowBody, Token: ContractToken}); refused.Status != 422 ||
		!strings.Contains(string(refused.Body), reasonShadowTestsOff) {
		t.Fatalf("a shadow with the switch off: %d %s", refused.Status, refused.Body)
	}
	if got := f.Coordinator.ReadBelt("contract-draft"); got == nil || len(got.ShadowTests) != 0 || got.Shadow != nil {
		t.Fatal("a refused shadow stored a test or a version")
	}
	if reply := getSettings(f); reply.Status != 200 || !strings.Contains(string(reply.Body), `"shadowTestsToggle":{"enabled":true}`) {
		t.Fatalf("settings with no test: %d %s", reply.Status, reply.Body)
	}

	if patch := postSettingsPatch(f, `{"enableShadowTests":true}`); patch.Status != 200 || !strings.Contains(string(patch.Body), `"enableShadowTests":true`) {
		t.Fatalf("enable: %d %s", patch.Status, patch.Body)
	}
	if !store.NewStorage(store.Paths{Root: f.Root}).LoadSettings().EnableShadowTests {
		t.Fatal("the enabled switch was not saved")
	}
	if read := getSettings(f); read.Status != 200 || !strings.Contains(string(read.Body), `"enableShadowTests":true`) {
		t.Fatalf("settings read after enabling: %d %s", read.Status, read.Body)
	}
	if started := f.Do(Request{Method: "POST", Path: "/api/v1/belts/contract-draft/shadow/save", Body: shadowBody, Token: ContractToken}); started.Status != 200 {
		t.Fatalf("start a test once enabled: %d %s", started.Status, started.Body)
	}
}

// TestShadowTestSwitchStaysOnWhileATestRuns checks over HTTP that the switch cannot be turned off while the
// demo's running test is going: the read shows it disabled with the shared reason, a false patch is refused
// with that reason and changes nothing, and a true patch is allowed.
func TestShadowTestSwitchStaysOnWhileATestRuns(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	if reply := getSettings(f); reply.Status != 200 || !strings.Contains(string(reply.Body), `"shadowTestsToggle":{"enabled":false,"reason":"`+reasonShadowTestsRunning+`"}`) {
		t.Fatalf("settings with a test running: %d %s", reply.Status, reply.Body)
	}

	before := f.Coordinator.ReadSettings()
	refused := postSettingsPatch(f, `{"enableShadowTests":false,"notifications":false}`)
	if refused.Status != 422 || !strings.Contains(string(refused.Body), reasonShadowTestsRunning) {
		t.Fatalf("a false patch with a test running: %d %s", refused.Status, refused.Body)
	}
	if !reflect.DeepEqual(f.Coordinator.ReadSettings(), before) {
		t.Fatal("a refused false patch changed the settings")
	}
	if !store.NewStorage(store.Paths{Root: f.Root}).LoadSettings().EnableShadowTests {
		t.Fatal("the saved switch changed on a refused patch")
	}
	if kept := postSettingsPatch(f, `{"enableShadowTests":true,"showInsights":true}`); kept.Status != 200 || !strings.Contains(string(kept.Body), `"showInsights":true`) {
		t.Fatalf("a true patch with a test running: %d %s", kept.Status, kept.Body)
	}
	if saved := store.NewStorage(store.Paths{Root: f.Root}).LoadSettings(); !saved.ShowInsights || !saved.EnableShadowTests {
		t.Fatalf("the true patch was not saved: showInsights %v, enableShadowTests %v", saved.ShowInsights, saved.EnableShadowTests)
	}
}

// TestExperimentalSwitchesPatchIndependently checks over HTTP that each experimental switch is saved on its own:
// every patch answers with both values, the read agrees, the stored settings hold them, and the other switch
// keeps its value. No test runs in the empty folder, so turning shadow tests off is allowed.
func TestExperimentalSwitchesPatchIndependently(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{Empty: true})
	type switches struct {
		ShowInsights      bool `json:"showInsights"`
		EnableShadowTests bool `json:"enableShadowTests"`
	}
	steps := []struct {
		body string
		want switches
	}{
		{`{"showInsights":true}`, switches{ShowInsights: true}},
		{`{"enableShadowTests":true}`, switches{ShowInsights: true, EnableShadowTests: true}},
		{`{"showInsights":false}`, switches{EnableShadowTests: true}},
		{`{"enableShadowTests":false}`, switches{}},
	}
	for _, step := range steps {
		reply := postSettingsPatch(f, step.body)
		if reply.Status != 200 {
			t.Fatalf("patch %s: status %d %s", step.body, reply.Status, reply.Body)
		}
		var answered switches
		if err := json.Unmarshal(reply.Body, &answered); err != nil || answered != step.want {
			t.Fatalf("patch %s answered %+v (%v), want %+v", step.body, answered, err, step.want)
		}
		var read switches
		if err := json.Unmarshal(getSettings(f).Body, &read); err != nil || read != step.want {
			t.Fatalf("after patch %s the read is %+v (%v), want %+v", step.body, read, err, step.want)
		}
		saved := store.NewStorage(store.Paths{Root: f.Root}).LoadSettings()
		if got := (switches{ShowInsights: saved.ShowInsights, EnableShadowTests: saved.EnableShadowTests}); got != step.want {
			t.Fatalf("after patch %s the stored settings are %+v, want %+v", step.body, got, step.want)
		}
	}
}

// reasonShadowTestsOff and reasonShadowTestsRunning are the shared explanations the daemon gives, as the
// settings and the belt actions carry them.
const (
	reasonShadowTestsOff     = "Enable shadow tests in Settings → Experimental to start a new test."
	reasonShadowTestsRunning = "Accept or discard all running shadow tests before turning this off."
)

// TestInsightsReadSucceedsWithBothSwitchesOff checks the experimental switches gate only the app's pages and
// commands: the Insights read answers with both off.
func TestInsightsReadSucceedsWithBothSwitchesOff(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{Empty: true})
	if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights", Token: ContractToken}); reply.Status != 200 {
		t.Fatalf("insights with both switches off: %d %s", reply.Status, reply.Body)
	}
}
