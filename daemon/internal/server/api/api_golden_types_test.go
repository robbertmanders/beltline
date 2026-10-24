package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// The golden responses as TypeScript. The goldens are the daemon's actual output; each one is
// checked against the wire.gen.ts type of the response it holds, by the web app's typecheck:
// Web/scripts/golden-types.mjs writes Web/src/api/golden.gen.ts (gitignored), one literal per golden
// and per sample, each `satisfies` its type, so a field the type lacks, a missing field, a wrong
// kind or an enum value outside the union fails `npm run typecheck`.
//
// This file keeps the two JSON inputs of that script current: goldenTypes.json (each JSON golden's
// response type) and goldenSamples.json (Go-marshalled values of override types that no golden
// exercises). BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestGoldenResponses
// writes them after a wire change.

const (
	goldenDirectory   = "../../../testdata/golden"
	goldenTypesPath   = "../../../../Web/src/api/goldenTypes.json"
	goldenSamplesPath = "../../../../Web/src/api/goldenSamples.json"
	wireTSOutput      = "../../../../Web/src/api/wire.gen.ts"
	goldenTSUpdateEnv = "BELTLINE_UPDATE_GOLDENS"
)

// goldenResponses maps each JSON golden to the wire.gen.ts type of the response it holds. The
// registry names the golden of each endpoint; the other goldens are the variants a test records.
var goldenResponses = map[string]string{
	"afk-decisions.json":                   "AfkDecisionsResponse",
	"afk-review.json":                      "AfkDecisionsResponse",
	"agent-archive.json":                   "AgentRecordResponse",
	"agent-detail.json":                    "AgentDetailResponse",
	"agent-draft-presentation.json":        "AgentDraftResponse",
	"agent-draft-switch-runner.json":       "AgentDraftResponse",
	"agent-new.json":                       "Agent",
	"agent-reset.json":                     "AgentRecordResponse",
	"agent-save.json":                      "AgentRecordResponse",
	"agent-thinking-supplemental.json":     "AgentDetailResponse",
	"agent-unarchive.json":                 "AgentRecordResponse",
	"agents.json":                          "AgentsResponse",
	"artifact-link.json":                   "ArtifactLinkResponse",
	"assistant.json":                       "AssistantResponse",
	"belt-afk.json":                        "BeltResourceResponse",
	"belt-shadow-save.json":                "BeltResourceResponse",
	"belt-shadow-accept.json":              "BeltResourceResponse",
	"belt-shadow-discard.json":             "BeltResourceResponse",
	"belt-archive.json":                    "BeltResourceResponse",
	"belt-context.json":                    "BeltContext",
	"belt-copy.json":                       "BeltDraftResponse",
	"belt-detail.json":                     "BeltSnapshot",
	"belt-draft-feed-into.json":            "BeltDraftResponse",
	"belt-draft-fork-add.json":             "BeltDraftResponse",
	"belt-draft-fork-remove.json":          "BeltDraftResponse",
	"belt-draft-policies-update.json":      "BeltDraftResponse",
	"belt-draft-repository-add.json":       "BeltDraftResponse",
	"belt-draft-repository-remove.json":    "BeltDraftResponse",
	"belt-draft-route-add.json":            "BeltDraftResponse",
	"belt-draft-route-remove.json":         "BeltDraftResponse",
	"belt-draft-route-update.json":         "BeltDraftResponse",
	"belt-draft-stage-add.json":            "BeltDraftResponse",
	"belt-draft-stage-move.json":           "BeltDraftResponse",
	"belt-draft-stage-remove.json":         "BeltDraftResponse",
	"belt-draft-stage-update.json":         "BeltDraftResponse",
	"belt-draft-trigger-add.json":          "BeltDraftResponse",
	"belt-draft-trigger-kind.json":         "BeltDraftResponse",
	"belt-draft-trigger-move.json":         "BeltDraftResponse",
	"belt-draft-trigger-remove.json":       "BeltDraftResponse",
	"belt-draft-trigger-update.json":       "BeltDraftResponse",
	"belt-new.json":                        "BeltDraftResponse",
	"belt-pause.json":                      "BeltResourceResponse",
	"belt-save-repository-bases.json":      "BeltResourceResponse",
	"belt-save.json":                       "BeltResourceResponse",
	"belt-stage-template.json":             "BeltStageTemplateResponse",
	"belt-unarchive.json":                  "BeltResourceResponse",
	"belt-update-all-stages.json":          "BeltDraftResponse",
	"belt-update-stage.json":               "BeltDraftResponse",
	"belts.json":                           "BeltsResponse",
	"command-afk-disabled.json":            "StateResponse",
	"command-afk.json":                     "StateResponse",
	"command-assistant-configuration.json": "AssistantResponse",
	"command-assistant-start.json":         "AssistantResponse",
	"command-assistant-stop.json":          "AssistantResponse",
	"command-check-now-paused.json":        "StateResponse",
	"command-check-now.json":               "StateResponse",
	"command-model-allow.json":             "ModelRunnerResource",
	"command-model-refresh.json":           "ModelRunnerResource",
	"command-pause.json":                   "StateResponse",
	"command-quit.json":                    "StateResponse",
	"command-settings-patch-disabled.json": "SettingsResponse",
	"command-settings-patch.json":          "SettingsResponse",
	"desktop-ok.json":                      "DesktopSuccess",
	"error-failure.json":                   "ErrorResponse",
	"error-forbidden.json":                 "ErrorResponse",
	"error-not-found.json":                 "ErrorResponse",
	"error-problems.json":                  "ErrorResponse",
	"error-unauthorized.json":              "ErrorResponse",
	"event-changed.json":                   "EventMessage",
	"event-hello.json":                     "EventMessage",
	"event-notification.json":              "EventMessage",
	"library-backup.json":                  "LibraryBackupResponse",
	"library-install.json":                 "LibraryInstallResponse",
	"library-item.json":                    "LibraryItemResponse",
	"library-refresh.json":                 "LibraryRepoResponse",
	"library-repos.json":                   "LibraryReposResponse",
	"library-restore.json":                 "LibraryRestoreResponse",
	"library-share.json":                   "LibraryShareResponse",
	"models-empty.json":                    "ModelRunnerResource",
	"models.json":                          "ModelRunnerResource",
	"placeholders.json":                    "PlaceholdersResponse",
	"run-abort.json":                       "RunResponse",
	"run-adjust-stage.json":                "RunResponse",
	"run-adjust-switch.json":               "RunAdjustSwitchResponse",
	"run-afk.json":                         "RunResponse",
	"run-answer.json":                      "RunResponse",
	"run-attempt-artifacts.json":           "AttemptArtifactsResponse",
	"run-attempt-output-long.json":         "AttemptOutputResponse",
	"run-attempt-output.json":              "AttemptOutputResponse",
	"run-continue-session.json":            "ContinueSessionResponse",
	"run-decide.json":                      "RunResponse",
	"run-detail.json":                      "RunResponse",
	"run-drop.json":                        "RunResponse",
	"run-erase.json":                       "EraseResponse",
	"run-overrule-afk.json":                "RunResponse",
	"run-park.json":                        "RunResponse",
	"run-pause.json":                       "RunResponse",
	"run-resume-run.json":                  "RunResponse",
	"run-resume.json":                      "RunResponse",
	"run-retry-continuation.json":          "RunResponse",
	"run-start-now.json":                   "RunResponse",
	"run-unpark.json":                      "RunResponse",
	"runs.json":                            "RunsResponse",
	"settings.json":                        "SettingsResponse",
	"state.json":                           "StateResponse",
	"tasks.json":                           "RunResponse",
	"tools.json":                           "ToolsResponse",
	"version.json":                         "VersionResponse",
	"views-overview-empty.json":            "OverviewResponse",
	"views-overview.json":                  "OverviewResponse",
	"views-run-forked.json":                "RunViewResponse",
	"views-run-held.json":                  "RunViewResponse",
	"views-run-nested.json":                "RunViewResponse",
	"views-run-parked.json":                "RunViewResponse",
	"views-run-split.json":                 "RunViewResponse",
	"views-run-straight.json":              "RunViewResponse",
	"views-run-sessions.json":              "SessionsView",
	"views-run.json":                       "RunViewResponse",
	"views-work-empty.json":                "WorkResponse",
	"views-insights.json":                  "InsightsView",
	"views-insights-belt.json":             "InsightsView",
	"views-insights-empty.json":            "InsightsView",
	"views-insights-evolution.json":        "InsightsEvolutionView",
	"views-work.json":                      "WorkResponse",
	"waiting.json":                         "WaitingResponse",
}

// untypedGoldens are JSON goldens that hold no api type, with the reason.
var untypedGoldens = map[string]string{
	"terminal-upgrade.json": "the WebSocket upgrade status, written by the terminal route rather than an api type",
}

// goldenSample is a value the daemon encodes for an override type that no golden holds. Value is
// the Go encoding; the typecheck checks it against Type like a golden.
type goldenSample struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// goldenSampleValues are the samples, marshalled by the Go encoders the goldens do not reach.
// RunPauseDeferred is written by its MarshalJSON, which tsOverrides describes; no golden holds a
// deferred merge, so both of its forms are sampled here.
func goldenSampleValues() ([]goldenSample, error) {
	values := []struct {
		name, typeName string
		value          any
	}{
		{"RunPauseDeferred merged", "RunPauseDeferred", core.RunPauseDeferred{Kind: core.RunPauseDeferredMerged, Values: []string{"bl-demo-1"}}},
		{"RunPauseDeferred conflict", "RunPauseDeferred", core.RunPauseDeferred{Kind: core.RunPauseDeferredConflict, Values: []string{"bl-demo-2"}}},
	}
	samples := make([]goldenSample, 0, len(values))
	for _, v := range values {
		data, err := json.Marshal(v.value)
		if err != nil {
			return nil, fmt.Errorf("sample %s: %w", v.name, err)
		}
		samples = append(samples, goldenSample{Name: v.name, Type: v.typeName, Value: data})
	}
	return samples, nil
}

// TestEveryJSONGoldenHasAResponseType checks that each JSON golden is mapped to a response type or
// listed as untyped, and that every mapping names a golden that exists.
func TestEveryJSONGoldenHasAResponseType(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(filepath.FromSlash(goldenDirectory))
	if err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		onDisk[name] = true
		_, typed := goldenResponses[name]
		_, untyped := untypedGoldens[name]
		if typed == untyped {
			t.Errorf("golden %s must be in exactly one of goldenResponses and untypedGoldens", name)
		}
	}
	for name := range goldenResponses {
		if !onDisk[name] {
			t.Errorf("goldenResponses names %s, which is not a JSON golden", name)
		}
	}
	for name := range untypedGoldens {
		if !onDisk[name] {
			t.Errorf("untypedGoldens names %s, which is not a JSON golden", name)
		}
	}
}

// TestGoldenResponses checks that every type the goldens and samples name is exported by wire.gen.ts,
// and that goldenTypes.json and goldenSamples.json are the current encodings of the Go maps.
func TestGoldenResponses(t *testing.T) {
	t.Parallel()
	wireBytes, err := os.ReadFile(filepath.FromSlash(wireTSOutput))
	if err != nil {
		t.Fatal(err)
	}
	wire := string(wireBytes)
	exported := func(name string) bool {
		return strings.Contains(wire, "export interface "+name+" ") || strings.Contains(wire, "export type "+name+" ")
	}
	for golden, typeName := range goldenResponses {
		if !exported(typeName) {
			t.Errorf("golden %s maps to %s, which wire.gen.ts does not export", golden, typeName)
		}
	}
	samples, err := goldenSampleValues()
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if !exported(sample.Type) {
			t.Errorf("sample %s has type %s, which wire.gen.ts does not export", sample.Name, sample.Type)
		}
	}

	types, err := encodeIndented(goldenResponses)
	if err != nil {
		t.Fatal(err)
	}
	sampleBytes, err := encodeIndented(samples)
	if err != nil {
		t.Fatal(err)
	}
	checkOrWrite(t, goldenTypesPath, types)
	checkOrWrite(t, goldenSamplesPath, sampleBytes)
}

// encodeIndented is v as the two-space, newline-terminated JSON the web app's inputs are kept in.
func encodeIndented(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// checkOrWrite writes want to path when goldenTSUpdateEnv is 1, and otherwise fails unless path holds want.
func checkOrWrite(t *testing.T, path string, want []byte) {
	t.Helper()
	path = filepath.FromSlash(path)
	if os.Getenv(goldenTSUpdateEnv) == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	disk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(disk, want) {
		t.Errorf("%s is out of date with the Go maps; run BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestGoldenResponses", path)
	}
}
