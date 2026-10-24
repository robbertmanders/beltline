// Settings, and the
// DateInterval that Foundation codes for its pauses.

package store

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// legacyMarketplaceRepository is the marketplace repository settings named when
// nothing was configured. It is not a library repository, so it is not carried over.
const legacyMarketplaceRepository = "robbertmanders/beltline-marketplace"

// DateInterval is Foundation's Codable form of a time interval: its start as
// a date and its duration in seconds. Settings keeps the pauses that ended.
type DateInterval struct {
	Start    jsonx.Date `json:"start"`
	Duration float64    `json:"duration"`
}

// Settings is settings.json: the app's own switches and local policy.
//
// Every stored property is written, with the optional ones omitted when nil.
// Decoding is UnmarshalJSON, with its fallbacks.
// allowedModels encodes each set as a sorted array.
type Settings struct {
	// Login is your GitHub login, for triggers that only take issues assigned to you.
	Login         string `json:"login"`
	Notifications bool   `json:"notifications"`
	// LibraryRepositories are the git repositories your library is backed up to
	// and shared through, and the ones it installs from.
	LibraryRepositories   []core.LibraryRepository `json:"libraryRepositories"`
	ConfigurationBackend  core.Backend             `json:"configurationBackend"`
	ConfigurationModel    string                   `json:"configurationModel"`
	ConfigurationThinking *core.Thinking           `json:"configurationThinking,omitzero"`
	// Paused is whether the app's one global pause is on. It survives a restart.
	Paused bool `json:"paused"`
	// PausedAt is when this pause began, so a run's elapsed timer can stand still.
	PausedAt *jsonx.Date `json:"pausedAt,omitzero"`
	// Pauses are the pauses that have ended, newest last.
	Pauses []DateInterval `json:"pauses"`
	// AllowedModels is local policy only.
	// Keyed by runner raw value; each set is written as a sorted array.
	AllowedModels map[string]map[string]struct{} `json:"allowedModels"`
	// ModelCatalogs are the last catalog each runner reported, by runner raw value.
	ModelCatalogs map[string]core.ModelCatalogSnapshot `json:"modelCatalogs"`
	// Afk is AFK mode: whether it is on, what it may do and what it runs on.
	Afk core.AfkSettings `json:"afk"`
	// ShowInsights is whether the navigation lists the Insights page.
	ShowInsights bool `json:"showInsights"`
}

// DefaultSettings is the default settings: an
// empty login, notifications on, no library repositories, the Pi backend and
// nothing paused, allowed or cataloged, and the Insights page hidden.
func DefaultSettings() Settings {
	return Settings{
		Notifications:        true,
		LibraryRepositories:  []core.LibraryRepository{},
		ConfigurationBackend: core.BackendPi,
		Pauses:               []DateInterval{},
		AllowedModels:        map[string]map[string]struct{}{},
		ModelCatalogs:        map[string]core.ModelCatalogSnapshot{},
		Afk:                  core.DefaultAfkSettings(),
	}
}

// settingsFields is the raw form of Settings for decoding. Every key is
// optional here; each field then applies its own fallback.
type settingsFields struct {
	Login                 json.RawMessage `json:"login,omitzero"`
	Notifications         json.RawMessage `json:"notifications,omitzero"`
	LibraryRepositories   json.RawMessage `json:"libraryRepositories,omitzero"`
	FactoryRepository     json.RawMessage `json:"factoryRepository,omitzero"`
	MarketplaceRepository json.RawMessage `json:"marketplaceRepository,omitzero"`
	ConfigurationBackend  json.RawMessage `json:"configurationBackend,omitzero"`
	ConfigurationModel    json.RawMessage `json:"configurationModel,omitzero"`
	ConfigurationThinking json.RawMessage `json:"configurationThinking,omitzero"`
	Paused                json.RawMessage `json:"paused,omitzero"`
	PausedAt              json.RawMessage `json:"pausedAt,omitzero"`
	Pauses                json.RawMessage `json:"pauses,omitzero"`
	AllowedModels         json.RawMessage `json:"allowedModels,omitzero"`
	ModelCatalogs         json.RawMessage `json:"modelCatalogs,omitzero"`
	Afk                   json.RawMessage `json:"afk,omitzero"`
	ShowInsights          json.RawMessage `json:"showInsights,omitzero"`
}

// UnmarshalJSON reads settings with fallbacks. Login, notifications,
// paused and pausedAt use decodeIfPresent with their
// defaults, so a field of the wrong type fails the whole file. The backend,
// model, thinking, pauses, permissions, catalogs, AFK settings and Insights switch
// use try?, so a bad one falls back alone. The permissions and the catalogs
// are one try? over the whole dictionary: one bad entry drops all of them. The
// library repositories are decoded entry by entry, so one bad entry drops alone.
func (s *Settings) UnmarshalJSON(data []byte) error {
	var raw settingsFields
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	next := DefaultSettings()
	next.Login = ""
	if err := decodeIfPresent(raw.Login, &next.Login); err != nil {
		return err
	}
	if err := decodeIfPresent(raw.Notifications, &next.Notifications); err != nil {
		return err
	}
	next.LibraryRepositories = decodeLibraryRepositories(raw.LibraryRepositories)
	next.LibraryRepositories = append(next.LibraryRepositories, legacyLibraryRepositories(raw, next.LibraryRepositories)...)
	next.ConfigurationBackend = tryDecode(raw.ConfigurationBackend, core.BackendPi)
	next.ConfigurationModel = tryDecode(raw.ConfigurationModel, "")
	next.ConfigurationThinking = tryDecode[*core.Thinking](raw.ConfigurationThinking, nil)
	// A settings.json written before the global pause existed is not paused.
	next.Paused = false
	if err := decodeIfPresent(raw.Paused, &next.Paused); err != nil {
		return err
	}
	next.PausedAt = nil
	if err := decodeIfPresent(raw.PausedAt, &next.PausedAt); err != nil {
		return err
	}
	next.Pauses = tryDecode(raw.Pauses, []DateInterval{})
	next.AllowedModels = identifierSets(tryDecode(raw.AllowedModels, map[string][]string{}))
	next.ModelCatalogs = tryDecode(raw.ModelCatalogs, map[string]core.ModelCatalogSnapshot{})
	next.Afk = tryDecode(raw.Afk, core.DefaultAfkSettings())
	// Insights is off until you turn it on, also in a settings.json written before the switch existed.
	next.ShowInsights = tryDecode(raw.ShowInsights, false)
	*s = next
	return nil
}

// decodeLibraryRepositories reads the library repositories one entry at a time, so an entry that does not
// decode is dropped alone and the others are kept, in their order. A value that is not a list gives none.
func decodeLibraryRepositories(raw json.RawMessage) []core.LibraryRepository {
	repos := []core.LibraryRepository{}
	var entries []json.RawMessage
	if isAbsent(raw) || json.Unmarshal(raw, &entries) != nil {
		return repos
	}
	for _, entry := range entries {
		if isAbsent(entry) {
			continue
		}
		var repo core.LibraryRepository
		if json.Unmarshal(entry, &repo) == nil {
			repos = append(repos, repo)
		}
	}
	return repos
}

// legacyLibraryRepositories are what an older settings file's factory and marketplace
// repositories become: the factory is a backup, the marketplace a shared repository
// published by pull request. The marketplace at its old default is not carried over,
// and a repository listed already is not added again. Saving drops the old keys.
func legacyLibraryRepositories(raw settingsFields, listed []core.LibraryRepository) []core.LibraryRepository {
	var migrated []core.LibraryRepository
	if factory := tryDecode(raw.FactoryRepository, ""); factory != "" {
		migrated = append(migrated, core.LibraryRepository{Repository: factory, Role: core.LibraryRoleBackup, Publish: core.LibraryPublishPullRequest})
	}
	if marketplace := tryDecode(raw.MarketplaceRepository, ""); marketplace != "" && marketplace != legacyMarketplaceRepository {
		migrated = append(migrated, core.LibraryRepository{Repository: marketplace, Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest})
	}
	var added []core.LibraryRepository
	for _, repo := range migrated {
		if !libraryRepositoryListed(repo.Origin(), listed) && !libraryRepositoryListed(repo.Origin(), added) {
			added = append(added, repo)
		}
	}
	return added
}

// libraryRepositoryListed reports whether a repository with origin is in repos.
func libraryRepositoryListed(origin core.LibraryOrigin, repos []core.LibraryRepository) bool {
	for _, repo := range repos {
		if repo.Origin() == origin {
			return true
		}
	}
	return false
}

// settingsFile is the encoded form of Settings: the same keys, with the
// permissions as sorted arrays.
type settingsFile struct {
	Login                 string                               `json:"login"`
	Notifications         bool                                 `json:"notifications"`
	LibraryRepositories   []core.LibraryRepository             `json:"libraryRepositories"`
	ConfigurationBackend  core.Backend                         `json:"configurationBackend"`
	ConfigurationModel    string                               `json:"configurationModel"`
	ConfigurationThinking *core.Thinking                       `json:"configurationThinking,omitzero"`
	Paused                bool                                 `json:"paused"`
	PausedAt              *jsonx.Date                          `json:"pausedAt,omitzero"`
	Pauses                []DateInterval                       `json:"pauses"`
	AllowedModels         map[string][]string                  `json:"allowedModels"`
	ModelCatalogs         map[string]core.ModelCatalogSnapshot `json:"modelCatalogs"`
	Afk                   core.AfkSettings                     `json:"afk"`
	ShowInsights          bool                                 `json:"showInsights"`
}

// MarshalJSON writes the settings with every set as a sorted array, so the output is stable.
func (s Settings) MarshalJSON() ([]byte, error) {
	return jsonx.Marshal(settingsFile{
		Login:                 s.Login,
		Notifications:         s.Notifications,
		LibraryRepositories:   nonNilRepositories(s.LibraryRepositories),
		ConfigurationBackend:  s.ConfigurationBackend,
		ConfigurationModel:    s.ConfigurationModel,
		ConfigurationThinking: s.ConfigurationThinking,
		Paused:                s.Paused,
		PausedAt:              s.PausedAt,
		Pauses:                nonNilIntervals(s.Pauses),
		AllowedModels:         sortedIdentifiers(s.AllowedModels),
		ModelCatalogs:         nonNilCatalogs(s.ModelCatalogs),
		Afk:                   s.Afk,
		ShowInsights:          s.ShowInsights,
	})
}

// identifierSets turns decoded permission arrays into sets. The result is
// never nil.
func identifierSets(lists map[string][]string) map[string]map[string]struct{} {
	out := make(map[string]map[string]struct{}, len(lists))
	for runner, identifiers := range lists {
		set := make(map[string]struct{}, len(identifiers))
		for _, identifier := range identifiers {
			set[identifier] = struct{}{}
		}
		out[runner] = set
	}
	return out
}

// sortedIdentifiers turns the sets into sorted arrays, keyed by runner. The
// result is never nil.
func sortedIdentifiers(sets map[string]map[string]struct{}) map[string][]string {
	out := make(map[string][]string, len(sets))
	for runner, set := range sets {
		identifiers := make([]string, 0, len(set))
		for identifier := range set {
			identifiers = append(identifiers, identifier)
		}
		sort.Strings(identifiers)
		out[runner] = identifiers
	}
	return out
}

func nonNilIntervals(in []DateInterval) []DateInterval {
	if in == nil {
		return []DateInterval{}
	}
	return in
}

func nonNilRepositories(in []core.LibraryRepository) []core.LibraryRepository {
	if in == nil {
		return []core.LibraryRepository{}
	}
	return in
}

func nonNilCatalogs(in map[string]core.ModelCatalogSnapshot) map[string]core.ModelCatalogSnapshot {
	if in == nil {
		return map[string]core.ModelCatalogSnapshot{}
	}
	return in
}

// LoadSettings reads settings.json, or the defaults when it is missing or does
// not decode.
func (s Storage) LoadSettings() Settings {
	var settings Settings
	if s.Read(s.Paths.Settings(), &settings) != nil {
		return DefaultSettings()
	}
	return settings
}

// SaveSettings writes settings.json.
func (s Storage) SaveSettings(settings Settings) error {
	return s.Write(settings, s.Paths.Settings())
}

// Modelling helpers on Settings.

// ModelWarning is the warning for a model the policy refuses on this runner,
// or nil. A model missing from the usable catalog is reported as unavailable
// rather than not allowed. env is the process environment, for the runner's
// executable.
func (s Settings) ModelWarning(model string, runner core.Backend, name string, env func(string) string) *string {
	var label *string
	if snapshot, ok := s.ModelCatalogs[string(runner)]; ok && snapshot.IsUsable(runner, runner.Executable(env)) {
		for _, choice := range snapshot.Models {
			if choice.ID == model {
				text := choice.DisplayLabel()
				label = &text
				break
			}
		}
	}
	warning := core.ModelPolicyWarning(model, runner, name, s.ModelAdmission(env), label)
	if warning == nil {
		return nil
	}
	if strings.TrimSpace(model) != "" && label == nil {
		replaced := strings.ReplaceAll(*warning, "is not allowed", "is unavailable in catalog")
		return &replaced
	}
	return warning
}

// ModelAdmission is the admission the settings grant: each runner's allowed
// choices.
func (s Settings) ModelAdmission(env func(string) string) core.ModelAdmission {
	models := make(map[string][]core.ModelChoice, len(core.AllBackend))
	for _, runner := range core.AllBackend {
		models[string(runner)] = s.AllowedChoices(runner, "", env)
	}
	return core.NewModelAdmission(models, nil, nil)
}

// AllowedChoices are the models the settings allow on runner that the runner's
// catalog still lists, filtered by search.
func (s Settings) AllowedChoices(runner core.Backend, search string, env func(string) string) []core.ModelChoice {
	allowance := core.ModelAllowance{Identifiers: map[core.Backend]map[string]struct{}{
		runner: core.CopySet(s.AllowedModels[string(runner)]),
	}}
	var snapshot *core.ModelCatalogSnapshot
	if catalog, ok := s.ModelCatalogs[string(runner)]; ok {
		snapshot = &catalog
	}
	return allowance.Choices(runner, snapshot, runner.Executable(env), search)
}
