// Coverage of the contract: every registry endpoint has exactly one exercised case (or is checked
// outside this harness), a golden file, and no golden in daemon/testdata/golden is orphaned.

package servertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// outsideHarness are the registry cases this harness does not serve, each with the test that does.
var outsideHarness = map[string]string{
	"terminals": "server terminal_integration_test.go (terminal-upgrade.json) and terminal_security_test.go",
	"events":    "server events_integration_test.go",
}

// extraGoldens are the golden files no registry endpoint names: the error bodies, the WebSocket
// messages, and the variants a test records for one endpoint (a layout, an empty fixture or
// a refusal). Each maps to the test that checks it; a golden file missing from both lists is orphaned.
var extraGoldens = map[string]string{
	"error-failure.json":                   "server server_error_test.go",
	"error-problems.json":                  "server server_error_test.go",
	"error-forbidden.json":                 "servertest runs_command_refusal_test.go",
	"error-not-found.json":                 "servertest run_view_read_test.go, runs_read_test.go",
	"error-unauthorized.json":              "servertest runs_command_refusal_test.go",
	"event-changed.json":                   "server events_message_test.go",
	"event-notification.json":              "server events_message_test.go",
	"agent-thinking-supplemental.json":     "servertest belts_write_test.go",
	"belt-save-repository-bases.json":      "servertest belts_write_test.go",
	"command-afk-disabled.json":            "servertest servertest_global_commands_test.go",
	"command-check-now-paused.json":        "servertest servertest_global_commands_test.go",
	"command-settings-patch-disabled.json": "servertest settings_patch_test.go",
	"models-empty.json":                    "servertest servertest_models_test.go",
	"run-attempt-output-long.json":         "servertest runs_read_test.go",
	"views-overview-empty.json":            "servertest views_empty_test.go",
	"views-work-empty.json":                "servertest views_empty_test.go",
	"views-insights-empty.json":            "servertest views_empty_test.go",
	"views-insights-belt.json":             "servertest servertest_contract_views.go (viewsInsights)",
	"views-run-straight.json":              "servertest run_view_read_test.go (layout loop)",
	"views-run-forked.json":                "servertest run_view_read_test.go (layout loop)",
	"views-run-nested.json":                "servertest run_view_read_test.go (layout loop)",
	"views-run-held.json":                  "servertest run_view_read_test.go (layout loop)",
	"views-run-parked.json":                "servertest run_view_read_test.go (layout loop)",
	"views-run-split.json":                 "servertest run_view_read_test.go",
}

// TestEveryRegistryEndpointIsExercisedOnce checks each registry case is served by exactly one
// exercised case, except the cases checked outside the harness, and that no exercised case is
// unknown to the registry.
func TestEveryRegistryEndpointIsExercisedOnce(t *testing.T) {
	t.Parallel()
	count := map[string]int{}
	for _, c := range exercisedCases {
		count[c]++
	}
	registry := map[string]bool{}
	for _, endpoint := range routes.Endpoints {
		registry[endpoint.Case] = true
		if _, ok := outsideHarness[endpoint.Case]; ok {
			if count[endpoint.Case] != 0 {
				t.Errorf("%s %s: case %s is listed outside the harness but exercised", endpoint.Method, endpoint.Path, endpoint.Case)
			}
			continue
		}
		if count[endpoint.Case] != 1 {
			t.Errorf("%s %s: case %s is exercised %d times, want once", endpoint.Method, endpoint.Path, endpoint.Case, count[endpoint.Case])
		}
	}
	for c := range count {
		if !registry[c] {
			t.Errorf("exercised case %s is not in the registry", c)
		}
	}
	for c := range outsideHarness {
		if !registry[c] {
			t.Errorf("case %s is listed outside the harness but is not in the registry", c)
		}
	}
}

// TestEveryRegistryEndpointHasItsGolden checks each registry endpoint's golden file exists.
func TestEveryRegistryEndpointHasItsGolden(t *testing.T) {
	t.Parallel()
	dir := testkit.Testdata(t, "golden")
	for _, endpoint := range routes.Endpoints {
		if _, err := os.Stat(filepath.Join(dir, endpoint.GoldenName)); err != nil {
			t.Errorf("%s %s: missing golden %s: %v", endpoint.Method, endpoint.Path, endpoint.GoldenName, err)
		}
	}
}

// TestNoGoldenIsOrphaned checks every golden file is named by a registry endpoint or listed in
// extraGoldens, and every extraGoldens entry is a file that exists.
func TestNoGoldenIsOrphaned(t *testing.T) {
	t.Parallel()
	dir := testkit.Testdata(t, "golden")
	named := map[string]bool{}
	for _, endpoint := range routes.Endpoints {
		named[endpoint.GoldenName] = true
	}
	for name, checkedBy := range extraGoldens {
		named[name] = true
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("golden %s (checked by %s) is missing: %v", name, checkedBy, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && !named[entry.Name()] {
			t.Errorf("golden %s is orphaned: no registry endpoint names it and no extra golden lists it", entry.Name())
		}
	}
}
