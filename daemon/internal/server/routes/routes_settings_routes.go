package routes

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/kit/demo"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// AddSettingsRead GET api/v1/settings.
// Lock: the handler calls ReadSettings and Paths, which take the lock.
func AddSettingsRead(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/settings", JSONRoute(c, func(r *http.Request) (any, error) {
		return SettingsResponseFor(r.Context(), c, nil), nil
	}))
}

// AddSettingsPatch POST api/v1/settings/patch.
// The body is a strict SettingsPatch; the answer is the settings after the patch.
// Lock: the handler calls ApplySettingsPatch, which takes the lock.
func AddSettingsPatch(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/settings/patch", JSONRoute(c, func(r *http.Request) (any, error) {
		patch, err := DecodeBody[coordinator.SettingsPatch](r, DefaultBodyLimit)
		if err != nil {
			return nil, err
		}
		saved, err := c.ApplySettingsPatch(patch)
		if err != nil {
			return nil, err
		}
		return SettingsResponseFor(r.Context(), c, &saved), nil
	}))
}

// SettingsResponseFor is the settings
// response for the coordinator, with the data location and demo flag, from the
// saved settings when given.
// Lock: takes the lock through ReadSettings and Paths (no lock is held across the call).
func SettingsResponseFor(ctx context.Context, c *coordinator.Coordinator, saved *store.Settings) api.SettingsResponse {
	var settings store.Settings
	if saved != nil {
		settings = *saved
	} else {
		settings = c.ReadSettings()
	}
	root := c.Paths().Root
	parent := demo.DemoRootParent(os.Getenv, os.TempDir())
	isDemo := strings.HasPrefix(root, parent+"/")
	return api.NewSettingsResponse(settings, c.ShadowTestsToggle(), root, isDemo)
}
