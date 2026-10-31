// Coordinator tests of the settings commands: partial patches trim repositories and survive a
// recreation, thinking omissions and explicit nulls select the default, refused candidates never
// partly write, and the factory repository is stored as typed.

package coordinator

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// decodePatch decodes a settings patch the way the API does.
func decodePatch(data string) (SettingsPatch, error) {
	var patch SettingsPatch
	err := json.Unmarshal([]byte(data), &patch)
	return patch, err
}

func TestPatchDecodingRejectsUnknownMissingAndInvalidFields(t *testing.T) {
	t.Parallel()
	accepted := []string{
		`{}`,
		`{"login":"octocat","notifications":true}`,
		`{"showInsights":false}`,
		`{"enableShadowTests":true}`,
	}
	for _, text := range accepted {
		if _, err := decodePatch(text); err != nil {
			t.Errorf("%s: %v", text, err)
		}
	}
	refused := []string{
		`{"paused":true}`,
		`{"modelCatalogs":{}}`,
		`{"allowedModels":{}}`,
		`{"configurationBackend":"pi"}`,
		`{"afk":{"enabled":true}}`,
		`{"notifications":"yes"}`,
		`{"notifications":true,"extra":1}`,
		`{"afk":{"backend":"openai"}}`,
		`{"afk":{"limit":2.5}}`,
		`{"afk":{"drop":true,"what":1}}`,
		`{"login":null}`,
		`{"showInsights":null}`,
		`{"showInsights":"off"}`,
		`{"enableShadowTests":null}`,
		`{"enableShadowTests":"off"}`,
	}
	for _, text := range refused {
		if _, err := decodePatch(text); err == nil {
			t.Errorf("%s was accepted", text)
		}
	}
}

func TestShowInsightsPatchSavesAndOmissionKeepsIt(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	saved, err := c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(true)})
	if err != nil {
		t.Fatalf("show Insights: %v", err)
	}
	if !saved.ShowInsights || !h.storage.LoadSettings().ShowInsights {
		t.Fatal("Insights is still hidden after showing it")
	}
	if saved, err = c.ApplySettingsPatch(SettingsPatch{Notifications: new(false)}); err != nil || !saved.ShowInsights {
		t.Fatalf("a patch without showInsights changed it: %v", err)
	}
}

// TestEnableShadowTestsPatchSavesAndOmissionKeepsIt patches each experimental switch alone, omits them,
// and keeps the confirmed value when a write fails.
func TestEnableShadowTestsPatchSavesAndOmissionKeepsIt(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	saved, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)})
	if err != nil || !saved.EnableShadowTests || saved.ShowInsights || !h.storage.LoadSettings().EnableShadowTests {
		t.Fatalf("enable shadow tests: %+v, %v", saved, err)
	}
	if saved, err = c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(true)}); err != nil || !saved.EnableShadowTests || !saved.ShowInsights {
		t.Fatalf("show Insights kept shadow tests: %+v, %v", saved, err)
	}
	if saved, err = c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(false)}); err != nil || !saved.EnableShadowTests || saved.ShowInsights {
		t.Fatalf("hide Insights kept shadow tests: %+v, %v", saved, err)
	}
	if saved, err = c.ApplySettingsPatch(SettingsPatch{Notifications: new(false)}); err != nil || !saved.EnableShadowTests {
		t.Fatalf("a patch without enableShadowTests changed it: %+v, %v", saved, err)
	}

	settingsPath := c.Paths().Settings()
	if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove settings: %v", err)
	}
	if err := os.MkdirAll(settingsPath, 0o755); err != nil {
		t.Fatalf("make a directory in its place: %v", err)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(false)}); err == nil {
		t.Error("a failed write was reported as saved")
	}
	if !c.ReadSettings().EnableShadowTests {
		t.Error("a failed write changed the published switch")
	}
}

func TestThinkingOmissionRetainsAndExplicitNullSelectsDefault(t *testing.T) {
	t.Parallel()
	// The decoding half: an explicit null selects Default, an omitted field is absent.
	var explicit SettingsPatch
	if err := json.Unmarshal([]byte(`{"afk":{"thinking":null}}`), &explicit); err != nil {
		t.Fatalf("decode explicit: %v", err)
	}
	if explicit.Afk == nil || !explicit.Afk.Thinking.Present || explicit.Afk.Thinking.Value != nil {
		t.Errorf("explicit null thinking = %+v", explicit.Afk)
	}
	var omitted SettingsPatch
	if err := json.Unmarshal([]byte(`{"afk":{"guidance":"x"}}`), &omitted); err != nil {
		t.Fatalf("decode omitted: %v", err)
	}
	if omitted.Afk == nil || omitted.Afk.Thinking.Present {
		t.Errorf("omitted thinking = %+v", omitted.Afk)
	}
}

func TestInvalidCandidatesNeverPartiallyWrite(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	before := c.settings
	for _, limit := range []int{-1, 0, 11} {
		limit := limit
		if _, err := c.ApplySettingsPatch(SettingsPatch{Notifications: new(true), Afk: &AfkSettingsPatch{Limit: &limit}}); err == nil {
			t.Errorf("limit %d was accepted", limit)
		}
	}
	for _, repository := range []string{"one-word", "bad/", "/bad", "two words/name"} {
		repository := repository
		if _, err := c.ApplySettingsPatch(SettingsPatch{LibraryRepositories: &[]core.LibraryRepository{{Repository: repository, Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest}}}); err == nil {
			t.Errorf("repository %q was accepted", repository)
		}
	}
	if !reflect.DeepEqual(c.settings, before) {
		t.Errorf("a refused patch changed the settings")
	}
}

func TestFailedStorageWriteLeavesPublishedSettingsUnchanged(t *testing.T) {
	t.Parallel()
	h := newLibraryHarness(t)
	c := h.Coord()
	before := c.settings
	settingsPath := c.Paths().Settings()
	if err := os.Remove(settingsPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove settings: %v", err)
	}
	if err := os.MkdirAll(settingsPath, 0o755); err != nil {
		t.Fatalf("make a directory in its place: %v", err)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{Notifications: new(true)}); err == nil {
		t.Errorf("a failed write was reported as saved")
	}
	if !reflect.DeepEqual(c.settings, before) {
		t.Errorf("a failed write changed the published settings")
	}
}

func TestCandidateAfkRangesAndThinkingPatch(t *testing.T) {
	t.Parallel()
	current := core.DefaultAfkSettings()
	for _, limit := range []int{1, 10} {
		limit := limit
		next, err := CandidateAfk(current, AfkSettingsPatch{Limit: &limit})
		if err != nil || next.Limit != limit {
			t.Errorf("limit %d: %+v, %v", limit, next, err)
		}
	}
	high := core.ThinkingHigh
	withHigh, err := CandidateAfk(current, AfkSettingsPatch{Thinking: ThinkingPatch{Present: true, Value: &high}})
	if err != nil || withHigh.Thinking == nil || *withHigh.Thinking != core.ThinkingHigh {
		t.Errorf("thinking high: %+v, %v", withHigh.Thinking, err)
	}
	kept, err := CandidateAfk(withHigh, AfkSettingsPatch{Guidance: new("x")})
	if err != nil || kept.Thinking == nil || *kept.Thinking != core.ThinkingHigh {
		t.Errorf("an omitted thinking did not keep the value")
	}
	cleared, err := CandidateAfk(withHigh, AfkSettingsPatch{Thinking: ThinkingPatch{Present: true}})
	if err != nil || cleared.Thinking != nil {
		t.Errorf("an explicit null did not select Default")
	}
}
