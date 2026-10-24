// Contract cases for the view, state and settings endpoints: the Overview, Work, waiting and AFK
// decisions, and the settings commands.

package servertest

import (
	"bytes"
	"context"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// The view, state and settings contract cases this file serves.
var viewsAreaCases = []string{
	"state", "settings", "settingsPatch", "viewsOverview", "viewsWork", "viewsInsights", "viewsInsightsEvolution", "viewsRun", "viewsRunSessions",
	"waiting", "afkDecisions", "afkReview",
}

func init() {
	registerArea(viewsAreaCases, exerciseViewsArea)
}

// exerciseViewsArea serves the view, state and settings cases. The contract seeds the views
// (ContractFixture.seedViews) before the reads that record them; the two mutating commands run on
// their own fixture, so the shared fixture's goldens are not changed by them.
func exerciseViewsArea(f *Fixture, t testing.TB, contractCase string) bool {
	t.Helper()
	switch contractCase {
	case "state":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/state", Token: ContractToken})
		CheckGolden(t, "state", "state.json", reply, f.Root)
		want := mustMarshal(t, api.NewStateResponse(f.Coordinator.ReadStateSnapshot(nil, nil)))
		if !bytes.Equal(reply.Body, want) {
			t.Fatalf("state: the response is not the coordinator's state snapshot")
		}
	case "settings":
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/settings", Token: ContractToken})
		CheckGolden(t, "settings", "settings.json", reply, f.Root)
		want := mustMarshal(t, api.NewSettingsResponse(f.Coordinator.ReadSettings(), f.Coordinator.Paths().Root, false))
		if !bytes.Equal(reply.Body, want) {
			t.Fatalf("settings: the response is not the coordinator's settings")
		}
		if bytes.Contains(reply.Body, []byte(ContractToken)) || bytes.Contains(reply.Body, []byte("beltline_auth")) {
			t.Fatalf("settings: the response exposes the token or the auth cookie")
		}
	case "settingsPatch":
		own := Start(t, Options{})
		reply := own.Do(Request{Method: "POST", Path: "/api/v1/settings/patch", Token: ContractToken,
			Body: []byte(`{"notifications":true,"libraryRepositories":[{"repository":"owner/repo","path":"","role":"shared","publish":"pullRequest"}]}`)})
		CheckGolden(t, "settings patch", "command-settings-patch.json", reply, own.Root)
		var response struct {
			Notifications       bool                     `json:"notifications"`
			LibraryRepositories []core.LibraryRepository `json:"libraryRepositories"`
		}
		if err := json.Unmarshal(reply.Body, &response); err != nil {
			t.Fatalf("settings patch: %v", err)
		}
		if !response.Notifications || len(response.LibraryRepositories) != 1 || response.LibraryRepositories[0].Repository != "owner/repo" {
			t.Fatalf("settings patch: notifications %v, libraryRepositories %v", response.Notifications, response.LibraryRepositories)
		}
		if got := own.Coordinator.ReadSettings().LibraryRepositories; len(got) != 1 || got[0].Repository != "owner/repo" {
			t.Fatalf("settings patch: coordinator holds %v", got)
		}
		saved := store.NewStorage(store.Paths{Root: own.Root}).LoadSettings()
		if len(saved.LibraryRepositories) != 1 || saved.LibraryRepositories[0].Repository != "owner/repo" {
			t.Fatalf("settings patch: the accepted patch is not persisted (%v)", saved.LibraryRepositories)
		}
		// An empty body is a valid no-op patch: the same golden answer, nothing changed.
		unchanged := own.Coordinator.ReadSettings()
		noOp := own.Do(Request{Method: "POST", Path: "/api/v1/settings/patch", Token: ContractToken, Body: []byte("{}")})
		CheckGolden(t, "settings patch", "command-settings-patch.json", noOp, own.Root)
		if got := own.Coordinator.ReadSettings(); len(got.LibraryRepositories) != len(unchanged.LibraryRepositories) || got.Login != unchanged.Login {
			t.Fatalf("settings patch: an empty patch changed settings")
		}
	case "viewsOverview":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/overview", Token: ContractToken})
		CheckGolden(t, "viewsOverview", "views-overview.json", reply, f.Root)
		want := mustMarshal(t, api.NewOverviewResponse(f.Coordinator.ReadOverviewView(nil, nil, nil)))
		if !bytes.Equal(reply.Body, want) {
			t.Fatalf("viewsOverview: the response is not the coordinator's overview")
		}
	case "viewsWork":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/work", Token: ContractToken})
		CheckGolden(t, "viewsWork", "views-work.json", reply, f.Root)
		want := mustMarshal(t, api.NewWorkResponse(f.Coordinator.ReadWorkView(core.WorkQueryDefault(), nil, nil)))
		if !bytes.Equal(reply.Body, want) {
			t.Fatalf("viewsWork: the response is not the coordinator's work view")
		}
	case "viewsInsights":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights", Token: ContractToken})
		CheckGolden(t, "viewsInsights", "views-insights.json", reply, f.Root)
		view, ok := f.Coordinator.ReadInsightsView(core.InsightsQuery{Period: core.InsightsPeriodAll}, nil, nil)
		if !ok || !bytes.Equal(reply.Body, mustMarshal(t, view)) {
			t.Fatalf("viewsInsights: the response is not the coordinator's insights view")
		}
		runs := f.Coordinator.ReadRuns()
		if len(runs) == 0 {
			t.Fatalf("viewsInsights: the contract seed has no runs")
		}
		belt := runs[0].BeltID
		reply = f.Do(Request{Method: "GET", Path: "/api/v1/views/insights?belt=" + EncodeSegment(belt) + "&period=90d", Token: ContractToken})
		CheckGolden(t, "viewsInsights belt", "views-insights-belt.json", reply, f.Root)
		view, ok = f.Coordinator.ReadInsightsView(core.InsightsQuery{Belt: belt, Period: core.InsightsPeriodQuarter}, nil, nil)
		if !ok || !bytes.Equal(reply.Body, mustMarshal(t, view)) || len(view.Stages) == 0 {
			t.Fatalf("viewsInsights: the belt response is not the coordinator's view of %s, or has no stages", belt)
		}
	case "viewsInsightsEvolution":
		f.SeedViews()
		belt := f.Coordinator.ReadRuns()[0].BeltID
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights/evolution?belt=" + EncodeSegment(belt), Token: ContractToken})
		CheckGolden(t, "viewsInsightsEvolution", "views-insights-evolution.json", reply, f.Root)
		view, ok := f.Coordinator.ReadInsightsEvolution(core.InsightsEvolutionQuery{Belt: belt}, nil)
		if !ok || !bytes.Equal(reply.Body, mustMarshal(t, view)) || len(view.Figures) == 0 {
			t.Fatalf("viewsInsightsEvolution: the response is not the coordinator's evolution view of %s", belt)
		}
		if missing := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights/evolution", Token: ContractToken}); missing.Status != 422 {
			t.Fatalf("viewsInsightsEvolution without a belt: status %d, want 422", missing.Status)
		}
		if unknown := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights/evolution?belt=no-such-belt", Token: ContractToken}); unknown.Status != 404 {
			t.Fatalf("viewsInsightsEvolution of an unknown belt: status %d, want 404", unknown.Status)
		}
	case "viewsRun":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/" + f.ActionRunID, Token: ContractToken})
		CheckGolden(t, "viewsRun", "views-run.json", reply, f.Root)
		snapshot := f.Coordinator.ReadRunViewSnapshot(f.ActionRunID, nil)
		if snapshot == nil {
			t.Fatalf("viewsRun: the action run has no view snapshot")
		}
		if !bytes.Equal(reply.Body, mustMarshal(t, api.NewRunViewResponse(*snapshot))) {
			t.Fatalf("viewsRun: the response is not the coordinator's run view")
		}
	case "viewsRunSessions":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/" + f.ActionRunID + "/sessions", Token: ContractToken})
		CheckGolden(t, "viewsRunSessions", "views-run-sessions.json", reply, f.Root)
		view := f.Coordinator.ReadSessionsView(context.Background(), f.ActionRunID, nil, nil, nil)
		if view == nil {
			t.Fatalf("viewsRunSessions: the action run has no sessions view")
		}
		if !bytes.Equal(reply.Body, mustMarshal(t, *view)) {
			t.Fatalf("viewsRunSessions: the response is not the coordinator's sessions view")
		}
	case "waiting":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/waiting", Token: ContractToken})
		CheckGolden(t, "waiting", "waiting.json", reply, f.Root)
		if !bytes.Equal(reply.Body, mustMarshal(t, api.NewWaitingResponse(f.Coordinator.ReadWaitingSubjects()))) {
			t.Fatalf("waiting: the response is not the coordinator's waiting list")
		}
	case "afkDecisions":
		f.SeedViews()
		reply := f.Do(Request{Method: "GET", Path: "/api/v1/afk/decisions", Token: ContractToken})
		CheckGolden(t, "afkDecisions", "afk-decisions.json", reply, f.Root)
		want := api.NewAfkDecisionsResponse(f.Coordinator.ReadAfkDecisions(), f.Coordinator.ReadRuns(),
			f.Coordinator.ReadAfkFoldersPresent(), core.AfkLogFilterAll)
		if !bytes.Equal(reply.Body, mustMarshal(t, want)) {
			t.Fatalf("afkDecisions: the response is not the coordinator's decision log")
		}
	case "afkReview":
		own := Start(t, Options{})
		var before []string
		for _, decision := range own.Coordinator.ReadAfkDecisions() {
			if decision.NeedsReview() {
				before = append(before, decision.ID)
			}
		}
		if len(before) == 0 {
			t.Fatalf("afkReview: the seeded views hold no decision to review")
		}
		first := before[0]
		reply := own.Do(Request{Method: "POST", Path: "/api/v1/afk/review", Token: ContractToken,
			Body: []byte(`{"ids":["` + first + `"]}`)})
		CheckGolden(t, "afkReview", "afk-review.json", reply, own.Root)
		var response struct {
			Decisions []struct {
				ID          string `json:"id"`
				NeedsReview bool   `json:"needsReview"`
			} `json:"decisions"`
		}
		if err := json.Unmarshal(reply.Body, &response); err != nil {
			t.Fatalf("afkReview: %v", err)
		}
		for _, decision := range response.Decisions {
			if decision.ID == first && decision.NeedsReview {
				t.Fatalf("afkReview: %s still needs review after it was marked", first)
			}
		}
	default:
		return false
	}
	return true
}

// mustMarshal is the wire encoding of v (jsonx.Marshal), failing the test on an encoding error.
func mustMarshal(t testing.TB, v any) []byte {
	t.Helper()
	data, err := jsonx.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return data
}
