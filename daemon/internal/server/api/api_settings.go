package api

import (
	"maps"
	"sort"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// SettingsResponse is Settings as a transport contract. Persistence keeps sets
// in their original form; the wire format uses sorted arrays and explicit
// nulls for optionals.
//
// DataLocation and Demo are transient presentation facts, not persisted: where
// the daemon keeps its data and whether this launch is a demo. They belong on
// the read/patch response, never in the stored Settings.
type SettingsResponse struct {
	Login                string                   `json:"login"`
	Notifications        bool                     `json:"notifications"`
	LibraryRepositories  []core.LibraryRepository `json:"libraryRepositories"`
	ConfigurationBackend core.Backend             `json:"configurationBackend"`
	ConfigurationModel   string                   `json:"configurationModel"`
	// ConfigurationThinking is written as an explicit null when unset.
	ConfigurationThinking *core.Thinking `json:"configurationThinking"`
	Paused                bool           `json:"paused"`
	// PausedAt is written as an explicit null when the belts are not paused.
	PausedAt      *jsonx.Date                          `json:"pausedAt"`
	Pauses        []store.DateInterval                 `json:"pauses"`
	AllowedModels map[string][]string                  `json:"allowedModels"`
	ModelCatalogs map[string]core.ModelCatalogSnapshot `json:"modelCatalogs"`
	Afk           core.AfkSettings                     `json:"afk"`
	ShowInsights  bool                                 `json:"showInsights"`
	DataLocation  string                               `json:"dataLocation"`
	Demo          bool                                 `json:"demo"`
}

// NewSettingsResponse builds the settings response from its arguments.
// Allowed models are sorted per runner, as the initialiser does.
func NewSettingsResponse(settings store.Settings, dataLocation string, demo bool) SettingsResponse {
	thinking := core.CopyPointer(settings.ConfigurationThinking)
	pausedAt := core.CopyPointer(settings.PausedAt)
	allowed := make(map[string][]string, len(settings.AllowedModels))
	for runner, set := range settings.AllowedModels {
		identifiers := make([]string, 0, len(set))
		for identifier := range set {
			identifiers = append(identifiers, identifier)
		}
		sort.Strings(identifiers)
		allowed[runner] = identifiers
	}
	catalogs := make(map[string]core.ModelCatalogSnapshot, len(settings.ModelCatalogs))
	maps.Copy(catalogs, settings.ModelCatalogs)
	return SettingsResponse{
		Login:                 settings.Login,
		Notifications:         settings.Notifications,
		LibraryRepositories:   settings.LibraryRepositories,
		ConfigurationBackend:  settings.ConfigurationBackend,
		ConfigurationModel:    settings.ConfigurationModel,
		ConfigurationThinking: thinking,
		Paused:                settings.Paused,
		PausedAt:              pausedAt,
		Pauses:                settings.Pauses,
		AllowedModels:         allowed,
		ModelCatalogs:         catalogs,
		Afk:                   settings.Afk,
		ShowInsights:          settings.ShowInsights,
		DataLocation:          dataLocation,
		Demo:                  demo,
	}
}
