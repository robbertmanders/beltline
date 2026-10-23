// The contract's golden tests: every case this harness serves is checked against its golden, and the
// body normaliser keeps the volatile values out of the goldens.

package servertest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestExercisedCasesMatchTheirGoldens serves every exercised case through the contract fixture.
func TestExercisedCasesMatchTheirGoldens(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	for _, c := range exercisedCases {
		c := c
		t.Run(c, func(t *testing.T) {
			// Read cases leave the shared fixture as it is; every command case starts a
			// fixture of its own, so the cases can run side by side.
			t.Parallel()
			f.Exercise(t, c)
		})
	}
}

// TestNormalizationSubstitutesOnlyTheDocumentedVolatileValues checks that normalization substitutes only the documented volatile values and rejects bodies that are not canonical JSON.
func TestNormalizationSubstitutesOnlyTheDocumentedVolatileValues(t *testing.T) {
	t.Parallel()
	root := "/var/folders/beltline-test/root"
	uuid := "9A4C4370-41C2-47DF-84F1-24AE19B9178B"
	body := []byte(`{"attempts":[{"otherUUID":"` + uuid + `","sessionId":"` + uuid + `"}],` +
		`"belts":[{"created":"2026-10-07T10:00:00Z","id":"domain"}],"modelCatalogs":{"pi":{"executable":"/custom/pi"}},` +
		`"notices":[{"date":"2026-10-07T10:00:00Z","id":"` + uuid + `","runId":null}],"order":["second","first"],` +
		`"otherDate":"2026-10-07T10:00:00Z","outcomes":[{"id":"` + uuid + `"}],"path":"` + root + `/runs/example"}`)
	decode := func(endpoint string) map[string]any {
		t.Helper()
		normalized, err := Normalize(body, endpoint, root)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(normalized, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	normalized := decode("belts")
	attempts := normalized["attempts"].([]any)[0].(map[string]any)
	notices := normalized["notices"].([]any)[0].(map[string]any)
	outcomes := normalized["outcomes"].([]any)[0].(map[string]any)
	belts := normalized["belts"].([]any)[0].(map[string]any)
	if attempts["sessionId"] != notices["id"] {
		t.Errorf("sessionId %v and notice id %v should share one token", attempts["sessionId"], notices["id"])
	}
	if outcomes["id"] != uuid || attempts["otherUUID"] != uuid {
		t.Errorf("only session ids and notice ids are renumbered in belts: %v %v", outcomes["id"], attempts["otherUUID"])
	}
	if belts["created"] != "2000-01-01T00:00:00Z" {
		t.Errorf("created %v", belts["created"])
	}
	if normalized["otherDate"] != "2026-10-07T10:00:00Z" {
		t.Errorf("otherDate %v", normalized["otherDate"])
	}
	if !reflect.DeepEqual(normalized["order"], []any{"second", "first"}) {
		t.Errorf("order %v", normalized["order"])
	}
	if normalized["path"] != "<TEMP_ROOT>/runs/example" {
		t.Errorf("path %v", normalized["path"])
	}
	if got := normalized["modelCatalogs"].(map[string]any)["pi"].(map[string]any)["executable"]; got != "/custom/pi" {
		t.Errorf("executable %v outside settings must stay", got)
	}
	settings := decode("settings")
	if got := settings["modelCatalogs"].(map[string]any)["pi"].(map[string]any)["executable"]; got != "pi" {
		t.Errorf("settings executable %v, want pi", got)
	}
	if state, err := Normalize(body, "state", root); err != nil || !strings.Contains(string(state), "2026-10-07T10:00:00Z") {
		t.Errorf("state dates must stay: %v", err)
	}
}

// TestNormalizationRejectsTrailingData checks that a body is one JSON value.
func TestNormalizationRejectsTrailingData(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"a":1} x`} {
		if _, err := Normalize([]byte(body), "shape", ""); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

// TestSupplementalFixtureSavesTheTriageAgentWithHighThinking checks the supplemental change of the
// contract fixture: a new triage version with high thinking, visible through the library.
func TestSupplementalFixtureSavesTheTriageAgentWithHighThinking(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{Supplemental: true})
	record := f.Coordinator.ReadAgent("triage")
	if record == nil || record.Agent.Thinking == nil || string(*record.Agent.Thinking) != "high" {
		t.Fatalf("triage thinking = %v, want high", record)
	}
}
