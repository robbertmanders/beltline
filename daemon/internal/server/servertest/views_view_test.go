// The Work, waiting, Overview and AFK decision reads through the contract fixture: each answers the
// coordinator's shared view for its query, byte for byte. The rows themselves are checked in core.
package servertest

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// workQueryPath is the Work path for the query string q.
func workQueryPath(q string) string { return "/api/v1/views/work?" + q }

// workBody is the body a Work request must answer: the shared view for query.
func workBody(t *testing.T, f *Fixture, query core.WorkQuery) []byte {
	t.Helper()
	return mustMarshal(t, api.NewWorkResponse(f.Coordinator.ReadWorkView(query, nil, nil)))
}

// TestWorkAnswersTheSharedViewForEachQuery checks every Work query parameter: the defaults, each filter,
// a search, a belt, a stage, each grouping, and empty values meaning the defaults.
func TestWorkAnswersTheSharedViewForEachQuery(t *testing.T) {
	t.Parallel()
	f := SeededReadOnlyFixture(t)
	f.SeedViews()
	runs := f.Coordinator.ReadRuns()

	shipped := findRunIn(t, runs, func(r core.Run) bool { return r.State == core.RunStateShipped })
	title := shipped.Subject.Title
	if words := strings.Fields(title); len(words) > 0 {
		title = words[0]
	}
	var belt string
	for _, run := range runs {
		belt = run.BeltID
		break
	}
	track := f.Coordinator.ReadOverviewView(nil, nil, nil).FactoryTrack
	var merged string
	for i := range track.Stages {
		if slices.ContainsFunc(runs, track.Stages[i].Contains) {
			merged = track.Stages[i].ID
			break
		}
	}
	if belt == "" || merged == "" {
		t.Fatal("the seeded views have no belt or stage with a run")
	}

	all := core.WorkQueryDefault()
	all.Filter = core.WorkFilterAll
	withBelt := all
	withBelt.Belt = belt
	withStage := all
	withStage.Stage = &merged
	withSearch := all
	withSearch.Search = title
	queries := []struct {
		query string
		want  core.WorkQuery
	}{
		{"", core.WorkQueryDefault()},
		{"filter=&groupBy=&belt=&stage=&search=", core.WorkQueryDefault()},
		{"filter=all&belt=" + url.QueryEscape(belt), withBelt},
		{"filter=all&stage=" + url.QueryEscape(merged), withStage},
		{"filter=all&search=" + url.QueryEscape("  "+title+"  "), withSearch},
	}
	for _, filter := range core.AllWorkFilter {
		q := core.WorkQueryDefault()
		q.Filter = filter
		queries = append(queries, struct {
			query string
			want  core.WorkQuery
		}{"filter=" + string(filter), q})
	}
	for _, groupBy := range core.AllGroupBy {
		q := all
		q.GroupBy = groupBy
		queries = append(queries, struct {
			query string
			want  core.WorkQuery
		}{"filter=all&groupBy=" + string(groupBy), q})
	}
	summary := mustMarshal(t, core.RunSummaryCounts(runs, ContractNow, nil))
	for _, tc := range queries {
		reply := getCase(f, workQueryPath(tc.query))
		if reply.Status != 200 || reply.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("work %q: status %d", tc.query, reply.Status)
		}
		if !bytes.Equal(reply.Body, workBody(t, f, tc.want)) {
			t.Fatalf("work %q: not the shared work view", tc.query)
		}
		members, err := jsonx.Members(reply.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(members["summary"], summary) {
			t.Fatalf("work %q: the summary must be global", tc.query)
		}
	}
}

// TestWorkRejectsAnInvalidFilterOrGroupingWith422 checks the 422 shape of a nonempty invalid filter or grouping.
func TestWorkRejectsAnInvalidFilterOrGroupingWith422(t *testing.T) {
	t.Parallel()
	f := SeededReadOnlyFixture(t)
	f.SeedViews()
	for _, c := range []struct{ query, message string }{
		{"filter=paused", "filter must be one of active, you, parked, shipped, all"},
		{"filter=ALL", "filter must be one of active, you, parked, shipped, all"},
		{"groupBy=repository", "groupBy must be one of none, repo, status, belt, epic"},
	} {
		reply := getCase(f, workQueryPath(c.query))
		if reply.Status != 422 {
			t.Fatalf("%s: status %d, want 422", c.query, reply.Status)
		}
		if want := mustMarshal(t, api.ErrorResponse{Message: c.message, Problems: []string{c.message}}); !bytes.Equal(reply.Body, want) {
			t.Fatalf("%s: body %s, want %s", c.query, reply.Body, want)
		}
	}
}

// TestWaitingReturnsTheCoordinatorsListWithoutCallingOutward checks that waiting returns the coordinator's
// list, and that it makes no GitHub or Jira call while it does.
func TestWaitingReturnsTheCoordinatorsListWithoutCallingOutward(t *testing.T) {
	t.Parallel()
	f := SeededReadOnlyFixture(t)
	f.SeedViews()
	expected := f.Coordinator.ReadWaitingSubjects()
	if len(expected) != 2 {
		t.Fatalf("the seeded waiting list has %d rows, want 2", len(expected))
	}
	reply := getCase(f, "/api/v1/waiting")
	if reply.Status != 200 || reply.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("waiting: status %d, headers %v", reply.Status, reply.Header)
	}
	if !bytes.Equal(reply.Body, mustMarshal(t, api.NewWaitingResponse(expected))) {
		t.Fatal("waiting: not the coordinator's list")
	}
	if n := len(f.GitHub.Calls()) + len(f.Jira.Calls()) + len(f.Jira.Searches()); n != 0 {
		t.Fatalf("waiting made %d outward calls, want none", n)
	}
}

// TestOverviewAnswersTheSharedView checks the Overview against the shared view, and that its Floor sends
// its boxes and their layout.
func TestOverviewAnswersTheSharedView(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	f.SeedViews()
	reply := getCase(f, "/api/v1/views/overview")
	if !bytes.Equal(reply.Body, mustMarshal(t, api.NewOverviewResponse(f.Coordinator.ReadOverviewView(nil, nil, nil)))) {
		t.Fatal("not the shared overview")
	}
	members, err := jsonx.Members(reply.Body)
	if err != nil {
		t.Fatal(err)
	}
	var floor map[string]json.RawMessage
	if err := json.Unmarshal(members["floor"], &floor); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"boxes", "layout"} {
		if _, ok := floor[key]; !ok {
			t.Fatalf("the floor sends no %s", key)
		}
	}
}

// TestAfkDecisionsAnswerTheSharedLogWithoutChangingIt checks the AFK log against the shared log, and that
// reading it leaves each decision's reviewed flag as it was.
func TestAfkDecisionsAnswerTheSharedLogWithoutChangingIt(t *testing.T) {
	t.Parallel()
	f := SeededReadOnlyFixture(t)
	f.SeedViews()
	decisions := f.Coordinator.ReadAfkDecisions()
	if len(decisions) < 2 {
		t.Fatalf("the seeded log holds %d decisions, want at least 2", len(decisions))
	}
	reply := getCase(f, "/api/v1/afk/decisions")
	if reply.Status != 200 || reply.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("afk decisions: status %d, headers %v", reply.Status, reply.Header)
	}
	want := api.NewAfkDecisionsResponse(decisions, f.Coordinator.ReadRuns(), f.Coordinator.ReadAfkFoldersPresent(), core.AfkLogFilterAll)
	if !bytes.Equal(reply.Body, mustMarshal(t, want)) {
		t.Fatal("afk decisions: not the shared decision log")
	}
	for i, d := range f.Coordinator.ReadAfkDecisions() {
		if d.ID != decisions[i].ID || d.Reviewed != decisions[i].Reviewed {
			t.Fatalf("reading the log changed decision %d", i)
		}
	}
}

// findRunIn is the first run that matches, failing the test when none does.
func findRunIn(t *testing.T, runs []core.Run, match func(core.Run) bool) core.Run {
	t.Helper()
	for _, run := range runs {
		if match(run) {
			return run
		}
	}
	t.Fatal("no run matches")
	return core.Run{}
}
