package store

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TestOldSettingsRetainUnrelatedFieldsAndDenyAll
func TestOldSettingsRetainUnrelatedFieldsAndDenyAll(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"login":"rob","notifications":false,"factoryRepository":"owner/factories"}`)
	if settings.Login != "rob" || settings.Notifications || len(settings.LibraryRepositories) != 1 || settings.LibraryRepositories[0].Repository != "owner/factories" {
		t.Fatalf("kept fields: %+v", settings)
	}
	if len(settings.AllowedModels) != 0 || len(settings.ModelCatalogs) != 0 {
		t.Fatal("an old file granted permissions")
	}
	if got := settings.AllowedChoices(core.BackendPi, "", noEnv); len(got) != 0 {
		t.Fatalf("choices %v", got)
	}
}

// TestSettingsSavedBeforeAfkReadAsOff
func TestSettingsSavedBeforeAfkReadAsOff(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"login":"me","notifications":true,"factoryRepository":""}`)
	if !reflect.DeepEqual(settings.Afk, core.DefaultAfkSettings()) || settings.Afk.Enabled {
		t.Fatalf("afk %+v", settings.Afk)
	}
}

// TestInsightsIsHiddenUntilTurnedOn
func TestInsightsIsHiddenUntilTurnedOn(t *testing.T) {
	t.Parallel()
	if DefaultSettings().ShowInsights {
		t.Fatal("the defaults show Insights")
	}
	if settings := mustDecode[Settings](t, `{"login":"me","notifications":true}`); settings.ShowInsights {
		t.Fatal("a file without showInsights showed Insights")
	}
	if settings := mustDecode[Settings](t, `{"showInsights":"yes"}`); settings.ShowInsights {
		t.Fatal("a showInsights of the wrong type showed Insights")
	}
	s := newTestStorage(t)
	shown := DefaultSettings()
	shown.ShowInsights = true
	if err := s.SaveSettings(shown); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if !s.LoadSettings().ShowInsights {
		t.Fatal("a saved true came back as hidden")
	}
}

// TestShadowTestsAreOffUntilTurnedOn checks shadow tests are off by default and in a settings file without
// the switch, and that both experimental switches round-trip through the store with the other.
func TestShadowTestsAreOffUntilTurnedOn(t *testing.T) {
	t.Parallel()
	if DefaultSettings().EnableShadowTests {
		t.Fatal("the defaults enable shadow tests")
	}
	if settings := mustDecode[Settings](t, `{"login":"me","showInsights":true}`); settings.EnableShadowTests {
		t.Fatal("a file without enableShadowTests enabled shadow tests")
	}
	if settings := mustDecode[Settings](t, `{"enableShadowTests":"yes"}`); settings.EnableShadowTests {
		t.Fatal("an enableShadowTests of the wrong type enabled shadow tests")
	}
	s := newTestStorage(t)
	saved := DefaultSettings()
	saved.EnableShadowTests = true
	if err := s.SaveSettings(saved); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if loaded := s.LoadSettings(); !loaded.EnableShadowTests || loaded.ShowInsights {
		t.Fatalf("a saved true came back as %+v", loaded)
	}
	saved.ShowInsights, saved.EnableShadowTests = true, false
	if err := s.SaveSettings(saved); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if loaded := s.LoadSettings(); loaded.EnableShadowTests || !loaded.ShowInsights {
		t.Fatalf("a saved false came back as %+v", loaded)
	}
}

// TestSavedRunnersRoundTrip
func TestSavedRunnersRoundTrip(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			settings := DefaultSettings()
			settings.ConfigurationBackend = backend
			data, err := jsonx.MarshalStorage(settings)
			if err != nil {
				t.Fatal(err)
			}
			if got := mustDecode[Settings](t, string(data)).ConfigurationBackend; got != backend {
				t.Fatalf("decoded %s, want %s", got, backend)
			}
		})
	}
}

// TestMissingRunnersStillDefaultToPi
func TestMissingRunnersStillDefaultToPi(t *testing.T) {
	t.Parallel()
	if got := mustDecode[Settings](t, `{}`).ConfigurationBackend; got != core.BackendPi {
		t.Fatalf("backend %s", got)
	}
}

// TestABadCatalogDropsEveryCatalogAndNoPermission checks one bad catalog entry
// drops the whole of modelCatalogs, while allowedModels, which has its own
// fallback, is kept.
func TestABadCatalogDropsEveryCatalogAndNoPermission(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"login":"kept","allowedModels":{"pi":["a"]},`+
		`"modelCatalogs":{"pi":{"runner":"pi","executable":"pi","refreshedAt":"2026-10-07T07:00:00Z","models":[]},`+
		`"claude":{"runner":"claude","executable":"claude","refreshedAt":"2026-10-07T07:00:00Z","models":"bad"}}}`)
	if settings.Login != "kept" {
		t.Fatalf("login %q", settings.Login)
	}
	if len(settings.ModelCatalogs) != 0 {
		t.Fatalf("catalogs kept despite one bad entry: %v", settings.ModelCatalogs)
	}
	if !reflect.DeepEqual(settings.AllowedModels, map[string]map[string]struct{}{"pi": {"a": {}}}) {
		t.Fatalf("allowedModels %v", settings.AllowedModels)
	}
}

// TestABadPermissionDropsEveryPermission checks one bad permission list drops
// all of allowedModels, and the catalogs are kept.
func TestABadPermissionDropsEveryPermission(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"allowedModels":{"pi":["a"],"claude":"bad"},`+
		`"modelCatalogs":{"pi":{"runner":"pi","executable":"pi","refreshedAt":"2026-10-07T07:00:00Z","models":[]}}}`)
	if len(settings.AllowedModels) != 0 {
		t.Fatalf("allowedModels kept despite one bad entry: %v", settings.AllowedModels)
	}
	if len(settings.ModelCatalogs) != 1 {
		t.Fatalf("catalogs %v", settings.ModelCatalogs)
	}
}

// TestAPauseThatDoesNotDecodeFallsBackAlone checks a bad pause list is empty
// while the paused flag is kept.
func TestAPauseThatDoesNotDecodeFallsBackAlone(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"paused":true,"pauses":[{"start":"not a date","duration":1}]}`)
	if !settings.Paused || len(settings.Pauses) != 0 {
		t.Fatalf("paused %v pauses %v", settings.Paused, settings.Pauses)
	}
}

// TestABadLibraryRepositoryDropsAloneAndTheRestStay checks that one entry of the library repositories that
// does not decode is dropped by itself, so the next save does not lose the others.
func TestABadLibraryRepositoryDropsAloneAndTheRestStay(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"libraryRepositories":[
		{"repository":"owner/first","path":"","role":"backup","publish":"push"},
		{"repository":"owner/broken","role":7},
		null,
		{"repository":"owner/last","path":"lib","role":"shared","publish":"pullRequest"}]}`)
	want := []core.LibraryRepository{
		{Repository: "owner/first", Role: core.LibraryRoleBackup, Publish: core.LibraryPublishPush},
		{Repository: "owner/last", Path: "lib", Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest},
	}
	if !reflect.DeepEqual(settings.LibraryRepositories, want) {
		t.Fatalf("repositories %+v, want %+v", settings.LibraryRepositories, want)
	}
	notAList := mustDecode[Settings](t, `{"libraryRepositories":{"repository":"owner/x"}}`)
	if len(notAList.LibraryRepositories) != 0 {
		t.Fatalf("a value that is not a list gave %+v", notAList.LibraryRepositories)
	}
}

// TestAFieldOfTheWrongTypeFailsTheWholeFile checks decodeIfPresent fields
// are not lenient: the file is an error, and loading gives the defaults.
func TestAFieldOfTheWrongTypeFailsTheWholeFile(t *testing.T) {
	t.Parallel()
	var settings Settings
	if err := json.Unmarshal([]byte(`{"login":5}`), &settings); err == nil {
		t.Fatal("a number login decoded")
	}
	s := newTestStorage(t)
	writeText(t, s.Paths.Settings(), `{"login":5}`)
	if diff := testkit.Diff(DefaultSettings(), s.LoadSettings()); diff != "" {
		t.Fatalf("loaded settings differ from the defaults:\n%s", diff)
	}
}

// TestMissingSettingsLoadAsDefaults checks a first start has the defaults.
func TestMissingSettingsLoadAsDefaults(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	if diff := testkit.Diff(DefaultSettings(), s.LoadSettings()); diff != "" {
		t.Fatalf("defaults differ:\n%s", diff)
	}
}

// TestSettingsRoundTripKeepsEveryField saves the settings golden, loads it and
// compares both the values and the bytes.
func TestSettingsRoundTripKeepsEveryField(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	want := goldenSettings(t)
	if err := s.SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	marshaled, err := jsonx.MarshalStorage(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileText(t, s.Paths.Settings()); got != string(marshaled) {
		t.Fatalf("saved bytes differ from MarshalStorage: %s", testkit.Diff(string(marshaled), got))
	}
	if diff := testkit.Diff(want, s.LoadSettings()); diff != "" {
		t.Fatalf("reloaded settings differ:\n%s", diff)
	}
}

// TestSetsAreWrittenSorted checks each permission set is written as a sorted
// array, whatever order it was held in.
func TestSetsAreWrittenSorted(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	settings.AllowedModels = map[string]map[string]struct{}{
		"pi": {"zeta": {}, "alpha": {}, "mid": {}},
	}
	data, err := jsonx.MarshalStorage(settings)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	alpha, mid, zeta := strings.Index(text, `"alpha"`), strings.Index(text, `"mid"`), strings.Index(text, `"zeta"`)
	if alpha < 0 || alpha >= mid || mid >= zeta {
		t.Fatalf("set not sorted in %s", text)
	}
	if got := mustDecode[Settings](t, text).AllowedModels["pi"]; len(got) != 3 {
		t.Fatalf("decoded set %v", got)
	}
}

// TestAllowedChoicesNeedAUsableCatalog checks a permitted model is a choice
// only while the runner's catalog still lists it for the executable.
func TestAllowedChoicesNeedAUsableCatalog(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	settings.AllowedModels = map[string]map[string]struct{}{"pi": {"m": {}}}
	models := []core.ModelChoice{{ID: "m", Name: "Model"}}
	catalog, err := core.NewModelCatalogSnapshot(core.BackendPi, "pi", time.Unix(100, 0), models)
	if err != nil {
		t.Fatal(err)
	}
	settings.ModelCatalogs = map[string]core.ModelCatalogSnapshot{"pi": catalog}
	if got := settings.AllowedChoices(core.BackendPi, "", noEnv); len(got) != 1 || got[0].ID != "m" {
		t.Fatalf("usable catalog choices %v", got)
	}
	other := catalog
	other.Executable = "elsewhere"
	settings.ModelCatalogs["pi"] = other
	if got := settings.AllowedChoices(core.BackendPi, "", noEnv); len(got) != 0 {
		t.Fatalf("an unusable catalog gave choices %v", got)
	}
}

// TestModelWarningSaysAModelIsUnavailableWithoutACatalog checks a model the
// catalog does not list is reported as unavailable, not as merely not allowed.
func TestModelWarningSaysAModelIsUnavailableWithoutACatalog(t *testing.T) {
	t.Parallel()
	settings := DefaultSettings()
	warning := settings.ModelWarning("m", core.BackendPi, "Triage", noEnv)
	if warning == nil || !strings.Contains(*warning, "unavailable in catalog") {
		t.Fatalf("warning %v", warning)
	}
	if settings.ModelWarning("", core.BackendPi, "Triage", noEnv) == nil {
		t.Fatal("an empty model with no permission gave no warning")
	}
}

// TestSettingsFileIsWhereThePathsSaysItIs checks settings.json sits at the
// data root, the place the golden-based test reads from.
func TestSettingsFileIsWhereThePathsSaysItIs(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	if want := filepath.Join(s.Paths.Root, "settings.json"); s.Paths.Settings() != want {
		t.Fatalf("settings at %s", s.Paths.Settings())
	}
}

// TestPreChangeCatalogJSONLoadsWithoutContextMetadata checks that a catalog saved before context sizes existed still loads, without them.
func TestPreChangeCatalogJSONLoadsWithoutContextMetadata(t *testing.T) {
	t.Parallel()
	choice := mustDecode[core.ModelChoice](t, `{"id":"claude-fable-5-1[1m]","name":"Fable 5.1 1M"}`)
	if choice.ContextTokens != nil || choice.Variants != nil || choice.DisplayLabel() != "Fable 5.1 1M" {
		t.Fatalf("choice %+v", choice)
	}
	legacy := mustDecode[core.ModelCatalogSnapshot](t, `{"runner":"pi","executable":"pi","refreshedAt":"1970-01-01T00:01:40Z",`+
		`"models":[{"id":"claude-fable-5-1[1m]","name":"Fable 5.1 1M"}]}`)
	if len(legacy.Models) != 1 || legacy.Models[0].ID != choice.ID || legacy.Models[0].ContextTokens != nil {
		t.Fatalf("legacy catalog %+v", legacy)
	}
}

// TestLegacyRepositoriesBecomeLibraryRepositories checks the one-time move of an old settings file's
// factory and marketplace repositories: the factory is a backup, a marketplace set away from its old
// default a shared repository, and the default marketplace is not carried over.
func TestLegacyRepositoriesBecomeLibraryRepositories(t *testing.T) {
	t.Parallel()
	settings := mustDecode[Settings](t, `{"factoryRepository":"owner/factories","marketplaceRepository":"owner/market"}`)
	want := []core.LibraryRepository{
		{Repository: "owner/factories", Role: core.LibraryRoleBackup, Publish: core.LibraryPublishPullRequest},
		{Repository: "owner/market", Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest},
	}
	if !reflect.DeepEqual(settings.LibraryRepositories, want) {
		t.Fatalf("migrated %+v, want %+v", settings.LibraryRepositories, want)
	}
	defaulted := mustDecode[Settings](t, `{"factoryRepository":"owner/factories","marketplaceRepository":"robbertmanders/beltline-marketplace"}`)
	if len(defaulted.LibraryRepositories) != 1 || defaulted.LibraryRepositories[0].Repository != "owner/factories" {
		t.Fatalf("the default marketplace was carried over: %+v", defaulted.LibraryRepositories)
	}
}
