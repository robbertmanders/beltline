// Contract tests of the Insights archive filter: the showArchived query and an archived belt's detail.

package servertest

import (
	"bytes"
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// insightsAt reads the Insights view at path with the contract's bearer, and decodes it.
func insightsAt(t *testing.T, g *Fixture, path string) (Reply, core.InsightsView) {
	t.Helper()
	reply := g.Do(Request{Method: "GET", Path: path, Token: ContractToken})
	var view core.InsightsView
	if reply.Status == 200 {
		mustDecode(t, reply, &view)
	}
	return reply, view
}

// TestInsightsShowArchivedQueryFiltersTheArchivedBelt checks the showArchived parameter: with no value,
// or any value but 1, an archived belt is left out of the picker and the rows; with 1 it is offered and
// its rows are the coordinator's view with the opt-in.
func TestInsightsShowArchivedQueryFiltersTheArchivedBelt(t *testing.T) {
	t.Parallel()
	g := Start(t, Options{})
	g.SeedViews()
	belt := g.Coordinator.ReadRuns()[0].BeltID
	g.Coordinator.SetBeltArchived(belt, true)
	hasBelt := func(view core.InsightsView) bool {
		return slices.ContainsFunc(view.Belts, func(item core.InsightsBelt) bool { return item.ID == belt })
	}
	hasRow := func(view core.InsightsView) bool {
		return slices.ContainsFunc(view.BeltRows, func(row core.InsightsBeltRow) bool { return row.ID == belt })
	}

	off, offView := insightsAt(t, g, "/api/v1/views/insights")
	requireStatus(t, off, 200)
	if hasBelt(offView) || hasRow(offView) {
		t.Fatalf("the default overview offers the archived belt")
	}
	for _, value := range []string{"", "0", "true", "yes"} {
		reply, view := insightsAt(t, g, "/api/v1/views/insights?showArchived="+value)
		requireStatus(t, reply, 200)
		if !bytes.Equal(reply.Body, off.Body) || hasBelt(view) {
			t.Fatalf("showArchived=%q does not follow the default", value)
		}
	}

	on, onView := insightsAt(t, g, "/api/v1/views/insights?showArchived=1")
	requireStatus(t, on, 200)
	if !hasBelt(onView) || !hasRow(onView) {
		t.Fatalf("showArchived=1 does not offer the archived belt in the picker and the rows")
	}
	want := mustMarshal(t, mustInsightsView(t, g, core.InsightsQuery{Period: core.InsightsPeriodAll, ShowArchived: true}))
	if !bytes.Equal(on.Body, want) {
		t.Fatalf("showArchived=1 is not the coordinator's view with the opt-in")
	}
}

// TestInsightsArchivedBeltDetailIsFoundWithoutTheOptIn checks an archived belt's detail: its direct link
// answers 200 without the opt-in and shows the belt, and an unknown belt is still not found.
func TestInsightsArchivedBeltDetailIsFoundWithoutTheOptIn(t *testing.T) {
	t.Parallel()
	g := Start(t, Options{})
	g.SeedViews()
	belt := g.Coordinator.ReadRuns()[0].BeltID
	g.Coordinator.SetBeltArchived(belt, true)

	reply, view := insightsAt(t, g, "/api/v1/views/insights?belt="+EncodeSegment(belt))
	requireStatus(t, reply, 200)
	if view.Belt != belt || view.Title == "" || len(view.Stages) == 0 {
		t.Fatalf("the archived belt's detail is %q with %d stages", view.Title, len(view.Stages))
	}
	if !slices.ContainsFunc(view.Belts, func(item core.InsightsBelt) bool { return item.ID == belt }) {
		t.Fatalf("the chosen archived belt is missing from the picker")
	}
	missing, _ := insightsAt(t, g, "/api/v1/views/insights?belt=no-such-belt")
	requireStatus(t, missing, 404)
}

// mustInsightsView is the coordinator's Insights view for query, read at the contract clock.
func mustInsightsView(t *testing.T, g *Fixture, query core.InsightsQuery) core.InsightsView {
	t.Helper()
	view, ok := g.Coordinator.ReadInsightsView(query, nil, nil)
	if !ok {
		t.Fatalf("the coordinator refused %+v", query)
	}
	return view
}
