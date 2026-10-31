// Coordinator tests of settings that publish, the AFK switch, and the projection of the global actions.

package coordinator

import (
	"context"
	"reflect"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// coordSettingsJSON is the settings as the store writes them, so two reads compare as the disk would.
func coordSettingsJSON(t *testing.T, settings store.Settings) string {
	t.Helper()
	data, err := jsonx.Marshal(settings)
	if err != nil {
		t.Fatalf("encode settings: %v", err)
	}
	return string(data)
}

// TestPartialPatchTrimsRepositoriesSavesAndSurvivesRecreation: a partial patch trims its repositories, is saved, and
// is what a recreated coordinator reads back.
func TestPartialPatchTrimsRepositoriesSavesAndSurvivesRecreation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	saved, err := c.ApplySettingsPatch(SettingsPatch{
		Login: new("octocat"), Notifications: new(true),
		LibraryRepositories: &[]core.LibraryRepository{{Repository: "owner/shared", Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest}},
		Afk:                 &AfkSettingsPatch{Limit: new(10), Guidance: new("keep it short")},
	})
	if err != nil {
		t.Fatalf("ApplySettingsPatch: %v", err)
	}
	if saved.Login != "octocat" || !saved.Notifications {
		t.Errorf("login %q, notifications %v", saved.Login, saved.Notifications)
	}
	if len(saved.LibraryRepositories) != 1 || saved.LibraryRepositories[0].Repository != "owner/shared" {
		t.Errorf("library repositories %+v", saved.LibraryRepositories)
	}
	if saved.Afk.Limit != 10 || saved.Afk.Guidance != "keep it short" {
		t.Errorf("afk limit %d, guidance %q", saved.Afk.Limit, saved.Afk.Guidance)
	}

	recreated := coordinatorOn(t, h.storage)
	if got, want := coordSettingsJSON(t, recreated.ReadSettings()), coordSettingsJSON(t, saved); got != want {
		t.Errorf("recreated settings\n%s\nwant\n%s", got, want)
	}
	if got, want := coordSettingsJSON(t, h.storage.LoadSettings()), coordSettingsJSON(t, saved); got != want {
		t.Errorf("stored settings\n%s\nwant\n%s", got, want)
	}
}

// TestExperimentalSwitchesSurviveRecreation: the Insights and shadow test choices are saved apart, and a
// recreated coordinator reads each one back as the user left it.
func TestExperimentalSwitchesSurviveRecreation(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	if _, err := c.ApplySettingsPatch(SettingsPatch{ShowInsights: new(true)}); err != nil {
		t.Fatalf("show Insights: %v", err)
	}
	saved, err := c.ApplySettingsPatch(SettingsPatch{EnableShadowTests: new(true)})
	if err != nil || !saved.ShowInsights || !saved.EnableShadowTests {
		t.Fatalf("enable shadow tests: %+v, %v", saved, err)
	}
	recreated := coordinatorOn(t, h.storage)
	if got := recreated.ReadSettings(); !got.ShowInsights || !got.EnableShadowTests {
		t.Fatalf("recreated switches: Insights %v, shadow tests %v", got.ShowInsights, got.EnableShadowTests)
	}
	if _, err := recreated.ApplySettingsPatch(SettingsPatch{ShowInsights: new(false)}); err != nil {
		t.Fatal(err)
	}
	again := coordinatorOn(t, h.storage)
	if got := again.ReadSettings(); got.ShowInsights || !got.EnableShadowTests {
		t.Fatalf("hiding Insights changed shadow tests: Insights %v, shadow tests %v", got.ShowInsights, got.EnableShadowTests)
	}
}

// TestInvalidCandidatesNeverPartiallyWriteAndLimitsStopAtOneAndTen: a refused patch writes nothing, even when
// part of it is valid, and the AFK limit accepts exactly one to ten.
func TestInvalidCandidatesNeverPartiallyWriteAndLimitsStopAtOneAndTen(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	before := c.ReadSettings()
	for _, limit := range []int{-1, 0, 11} {
		if _, err := c.ApplySettingsPatch(SettingsPatch{Notifications: new(true), Afk: &AfkSettingsPatch{Limit: new(limit)}}); err == nil {
			t.Errorf("limit %d was accepted", limit)
		}
	}
	for _, repository := range []string{"one-word", "bad/", "/bad", "two words/name"} {
		if _, err := c.ApplySettingsPatch(SettingsPatch{LibraryRepositories: &[]core.LibraryRepository{{Repository: repository, Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest}}}); err == nil {
			t.Errorf("repository %q was accepted", repository)
		}
	}
	if got, want := coordSettingsJSON(t, c.ReadSettings()), coordSettingsJSON(t, before); got != want {
		t.Errorf("a refused patch changed the settings")
	}
	for _, limit := range []int{1, 10} {
		if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Limit: new(limit)}}); err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if got := c.ReadSettings().Afk.Limit; got != limit {
			t.Errorf("limit = %d, want %d", got, limit)
		}
	}
}

// TestDisabledBlankIsAcceptedAndGlobalAfkEnablesWithoutAModel: a blank model is accepted while AFK is off, and the global
// switch turns AFK on even with no allowed model, keeping the unrelated edit.
func TestDisabledBlankIsAcceptedAndGlobalAfkEnablesWithoutAModel(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Guidance: new("first run"), Model: new("")}}); err != nil {
		t.Fatalf("ApplySettingsPatch: %v", err)
	}
	if got := c.ReadSettings().Afk.Model; got != "" {
		t.Errorf("model = %q", got)
	}
	if c.ReadSettings().Afk.Enabled {
		t.Error("AFK is enabled by a settings patch")
	}
	// As the app did, the global switch takes effect even with no allowed model.
	c.SetGlobalAfk(context.Background(), true)
	if !c.ReadSettings().Afk.Enabled {
		t.Error("the global switch did not enable AFK")
	}
	if c.ReadSettings().Afk.Guidance == "" {
		t.Error("the unrelated edit was lost")
	}
}

// TestAfkFieldEditsSaveOnTheirOwnWhileEnabled: while AFK is enabled, each AFK field saves on its own, and a model
// whose permission was removed does not block other edits.
func TestAfkFieldEditsSaveOnTheirOwnWhileEnabled(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	pi := core.BackendPi
	claude := core.BackendClaude
	if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Backend: &pi, Model: new("fixture-model")}}); err != nil {
		t.Fatalf("ApplySettingsPatch: %v", err)
	}
	c.SetGlobalAfk(context.Background(), true)
	if !c.ReadSettings().Afk.Enabled {
		t.Fatal("AFK did not turn on")
	}

	// Changing the runner alone works, and an edit does not re-check the model.
	if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Backend: &claude}}); err != nil {
		t.Fatalf("change runner: %v", err)
	}
	if got := c.ReadSettings().Afk.Backend; got != core.BackendClaude {
		t.Errorf("backend = %q", got)
	}
	if got := c.ReadSettings().Afk.Model; got != "fixture-model" {
		t.Errorf("model = %q", got)
	}

	// A model whose permission was removed no longer blocks other edits.
	if err := c.SetModelAllowed("fixture-model", core.BackendPi, false); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Drop: new(true)}}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if !c.ReadSettings().Afk.Drop || !c.ReadSettings().Afk.Enabled {
		t.Errorf("drop %v, enabled %v", c.ReadSettings().Afk.Drop, c.ReadSettings().Afk.Enabled)
	}

	// The basic range check still applies.
	if _, err := c.ApplySettingsPatch(SettingsPatch{Afk: &AfkSettingsPatch{Limit: new(11)}}); err == nil {
		t.Error("limit 11 was accepted")
	}
	if c.ReadSettings().Afk.Limit == 11 {
		t.Error("limit 11 was saved")
	}
}

// TestGlobalActionsProjectPauseAfkAndCheckNowInOrder: the global controls are listed pause, AFK, check now, and their
// labels follow the global pause and AFK switches; the state snapshot carries the same list.
func TestGlobalActionsProjectPauseAfkAndCheckNowInOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	c := h.Coord()
	actions := c.ReadGlobalActions()
	if got := globalActionIDs(actions); !reflect.DeepEqual(got, []string{"pause", "afk", "check-now"}) {
		t.Fatalf("actions = %v", got)
	}
	if labels := globalActionLabels(actions); !reflect.DeepEqual(labels, []string{"Pause everything", "GLOBAL AFK OFF", "Fetch"}) {
		t.Errorf("labels = %v", labels)
	}
	for _, action := range actions {
		if !action.Enabled || action.Reason != nil {
			t.Errorf("action %q enabled %v, reason %v", action.ID, action.Enabled, action.Reason)
		}
	}
	if snapshot := c.ReadStateSnapshot(nil, nil); !reflect.DeepEqual(snapshot.Actions, actions) {
		t.Errorf("the state snapshot's actions differ: %+v", snapshot.Actions)
	}

	c.SetGloballyPaused(context.Background(), true)
	actions = c.ReadGlobalActions()
	if actions[0].Label != "Paused · Continue" {
		t.Errorf("paused label = %q", actions[0].Label)
	}
	if !actions[2].Enabled || actions[2].Label != "Fetch" {
		t.Errorf("fetch while paused: enabled %v, label %q", actions[2].Enabled, actions[2].Label)
	}
	c.SetGlobalAfk(context.Background(), true)
	if got := c.ReadGlobalActions()[1].Label; got != "GLOBAL AFK ON" {
		t.Errorf("afk label = %q", got)
	}
}

// globalActionIDs lists the ids of the global actions in order.
func globalActionIDs(actions []GlobalAction) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	return ids
}

// globalActionLabels lists the labels of the global actions in order.
func globalActionLabels(actions []GlobalAction) []string {
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, action.Label)
	}
	return labels
}
