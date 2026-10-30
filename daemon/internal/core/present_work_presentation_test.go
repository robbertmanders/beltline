package core

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// wpBase is the fixture clock: 1_700_000_000 seconds after the Unix epoch.
var wpBase = time.Unix(1_700_000_000, 0)

// wpMakeRun builds a run for the Work tests.
// updated is in seconds after wpBase; key non-empty makes a Jira run.
func wpMakeRun(id string, state RunState, updated float64, beltID, beltName, repo, title string, number int, key string) Run {
	subject := DefaultSubject(repo, title)
	subject.Number = number
	subject.Key = key
	if key != "" {
		subject.Kind = SubjectKindJira
	}
	run := NewRun(RunArgs{Belt: SampleBelt([]string{repo}, "agent", SampleAgents(), true), Subject: subject, Now: wpBase})
	run.ID = id
	run.BeltID = beltID
	run.BeltName = beltName
	run.State = state
	run.UpdatedAt = jsonx.Date{Time: wpBase.Add(time.Duration(updated * float64(time.Second)))}
	return run
}

// wpRun is wpMakeRun with the defaults: issues belt, Issues, acme/billing, title T, number 1.
func wpRun(id string, state RunState) Run {
	return wpMakeRun(id, state, 0, "issues", "Issues", "acme/billing", "T", 1, "")
}

// wpQuery is a Work query with the defaults (active, all belts) and the given filter.
func wpQuery(filter WorkFilter) WorkQuery {
	query := WorkQueryDefault()
	query.Filter = filter
	return query
}

// wpIDs is the sorted ids of runs.
func wpIDs(runs []Run) []string {
	ids := []string{}
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	sort.Strings(ids)
	return ids
}

// TestEveryFilterSelectsItsExactStates
func TestEveryFilterSelectsItsExactStates(t *testing.T) {
	t.Parallel()
	runs := []Run{
		wpRun("queued", RunStateQueued),
		wpRun("running", RunStateRunning),
		wpRun("waiting", RunStateWaiting),
		wpRun("held", RunStateHeld),
		wpRun("parked", RunStateParked),
		wpRun("shipped", RunStateShipped),
		wpRun("done", RunStateDone),
		wpRun("dropped", RunStateDropped),
	}
	ids := func(filter WorkFilter) []string { return wpIDs(WorkPresentationRows(runs, wpQuery(filter), nil)) }
	expect := func(filter WorkFilter, want ...string) {
		t.Helper()
		if got := ids(filter); !reflect.DeepEqual(got, want) {
			t.Fatalf("filter %s = %v, want %v", filter, got, want)
		}
	}
	expect(WorkFilterActive, "held", "queued", "running", "waiting")
	expect(WorkFilterYou, "held", "waiting")
	expect(WorkFilterParked, "parked")
	expect(WorkFilterShipped, "shipped")
	expect(WorkFilterAll, "done", "dropped", "held", "parked", "queued", "running", "shipped", "waiting")
	// Done is finished, not shipped: it leaves Active and Shipped unless a search is given.
	done := wpMakeRun("done-review", RunStateDone, 0, "reviews", "Reviews", "acme/billing", "Review", 5, "")
	if got := wpIDs(WorkPresentationRows([]Run{done}, wpQuery(WorkFilterActive), nil)); len(got) != 0 {
		t.Errorf("active = %v, want no done run", got)
	}
	if got := wpIDs(WorkPresentationRows([]Run{done}, wpQuery(WorkFilterShipped), nil)); len(got) != 0 {
		t.Errorf("shipped = %v, want no done run", got)
	}
	query := wpQuery(WorkFilterActive)
	query.Search = "review"
	if got := wpIDs(WorkPresentationRows([]Run{done}, query, nil)); !reflect.DeepEqual(got, []string{"done-review"}) {
		t.Errorf("active search = %v, want the done run", got)
	}
}

// TestSearchTrimsMatchesEveryFieldCaseInsensitivelyAndOverridesState
func TestSearchTrimsMatchesEveryFieldCaseInsensitivelyAndOverridesState(t *testing.T) {
	t.Parallel()
	shipped := wpMakeRun("shipped", RunStateShipped, 0, "issues", "Billing belt", "Acme/Billing", "Export Invoices", 42, "")
	parked := wpMakeRun("parked", RunStateParked, 0, "issues", "Issues", "acme/other", "Parked retry", 7, "")
	dropped := wpMakeRun("dropped", RunStateDropped, 0, "issues", "Issues", "acme/third", "Dropped cleanup", 9, "")
	runs := []Run{shipped, parked, dropped}
	search := func(filter WorkFilter, text string) []Run {
		query := wpQuery(filter)
		query.Search = text
		return WorkPresentationRows(runs, query, nil)
	}
	for _, field := range []string{"#42", "export invoices", "ACME/BILLING", "billing belt"} {
		if got := wpIDs(search(WorkFilterAll, "  "+field+"  ")); !reflect.DeepEqual(got, []string{"shipped"}) {
			t.Fatalf("searching %s = %v", field, got)
		}
	}
	// A nonblank search includes finished work under Active, keeps parked out of Active,
	// and admits parked under every other filter.
	if got := wpIDs(search(WorkFilterActive, "#42")); !reflect.DeepEqual(got, []string{"shipped"}) {
		t.Fatalf("active #42 = %v", got)
	}
	if got := search(WorkFilterActive, "parked"); len(got) != 0 {
		t.Fatalf("active parked = %v, want none", wpIDs(got))
	}
	for _, filter := range []WorkFilter{WorkFilterYou, WorkFilterParked, WorkFilterShipped, WorkFilterAll} {
		if got := wpIDs(search(filter, "parked")); !reflect.DeepEqual(got, []string{"parked"}) {
			t.Fatalf("%s parked = %v", filter, got)
		}
	}
	// Whitespace only is no search; no match is empty.
	if got := search(WorkFilterAll, " \t "); len(got) != 3 {
		t.Fatalf("whitespace search count = %d, want 3", len(got))
	}
	query := WorkQueryDefault()
	query.Search = "does-not-exist"
	if got := WorkPresentationRows(runs, query, nil); len(got) != 0 {
		t.Fatalf("no-match search = %v", wpIDs(got))
	}
	if !WorkPresentationMatches(shipped, "#42") || WorkPresentationMatches(shipped, "#43") {
		t.Fatal("matches(#42)/matches(#43) wrong")
	}
}

// TestOrderIsNeedsYouThenUpdatedThenID
func TestOrderIsNeedsYouThenUpdatedThenID(t *testing.T) {
	t.Parallel()
	waiting := wpMakeRun("waiting", RunStateWaiting, 100, "issues", "Issues", "acme/billing", "T", 1, "")
	runningOld := wpMakeRun("running-old", RunStateRunning, 10, "issues", "Issues", "acme/billing", "T", 1, "")
	runningNew := wpMakeRun("running-new", RunStateRunning, 50, "issues", "Issues", "acme/billing", "T", 1, "")
	held := wpMakeRun("held", RunStateHeld, 1, "issues", "Issues", "acme/billing", "T", 1, "")
	tieA := wpMakeRun("a", RunStateParked, 30, "issues", "Issues", "acme/billing", "T", 1, "")
	tieB := wpMakeRun("b", RunStateParked, 30, "issues", "Issues", "acme/billing", "T", 1, "")
	ordered := WorkPresentationRows([]Run{runningOld, tieB, waiting, tieA, runningNew, held}, wpQuery(WorkFilterAll), nil)
	if got := wpIDsInOrder(ordered); !reflect.DeepEqual(got, []string{"waiting", "held", "running-new", "a", "b", "running-old"}) {
		t.Fatalf("order = %v", got)
	}
	if !WorkPresentationOrder(tieA, tieB) || WorkPresentationOrder(tieB, tieA) {
		t.Fatal("tie order wrong")
	}
}

// wpIDsInOrder is the ids of runs in their given order.
func wpIDsInOrder(runs []Run) []string {
	ids := []string{}
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

// TestBeltSelectsItsExactIDIncludingDuringSearch
func TestBeltSelectsItsExactIDIncludingDuringSearch(t *testing.T) {
	t.Parallel()
	one := wpMakeRun("one", RunStateRunning, 0, "one", "One", "acme/billing", "T", 1, "")
	two := wpMakeRun("two", RunStateParked, 0, "two", "Two", "acme/billing", "match me", 1, "")
	runs := []Run{one, two}
	query := wpQuery(WorkFilterAll)
	query.Belt = "one"
	if got := wpIDs(WorkPresentationRows(runs, query, nil)); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("belt one = %v", got)
	}
	query.Belt = "missing"
	if got := WorkPresentationRows(runs, query, nil); len(got) != 0 {
		t.Fatalf("missing belt = %v", wpIDs(got))
	}
	query = wpQuery(WorkFilterParked)
	query.Belt = "two"
	query.Search = "match"
	if got := wpIDs(WorkPresentationRows(runs, query, nil)); !reflect.DeepEqual(got, []string{"two"}) {
		t.Fatalf("belt two search = %v", got)
	}
}

// TestWaitingIsRestrictedOnlyByBelt
func TestWaitingIsRestrictedOnlyByBelt(t *testing.T) {
	t.Parallel()
	first := WaitingSubject{BeltID: "one", Subject: wpIssue("acme/a", 1, "One"), Reason: "blocked"}
	second := WaitingSubject{BeltID: "two", Subject: wpIssue("acme/b", 2, "Two"), Reason: "blocked"}
	all := []WaitingSubject{first, second}
	refs := func(subjects []WaitingSubject) []string {
		values := []string{}
		for _, subject := range subjects {
			values = append(values, subject.Subject.Ref())
		}
		return values
	}
	if got := refs(WorkPresentationWaiting(all, "all")); !reflect.DeepEqual(got, []string{"#1", "#2"}) {
		t.Fatalf("all waiting = %v", got)
	}
	if got := refs(WorkPresentationWaiting(all, "one")); !reflect.DeepEqual(got, []string{"#1"}) {
		t.Fatalf("one waiting = %v", got)
	}
	if got := WorkPresentationWaiting(all, "missing"); len(got) != 0 {
		t.Fatalf("missing waiting = %v", got)
	}
	// A query with a stage, search or state does not touch waiting.
	query := WorkQuery{Filter: WorkFilterShipped, Belt: "one", Stage: new("missing"), Search: "nothing"}
	if !query.IsSearching() {
		t.Fatal("query is not searching")
	}
	if got := WorkPresentationWaiting(all, query.Belt); !reflect.DeepEqual(got, []WaitingSubject{first}) {
		t.Fatalf("waiting for query belt = %v", got)
	}
}

// wpIssue is an issue subject in repo, numbered number, titled title.
func wpIssue(repo string, number int, title string) Subject {
	subject := DefaultSubject(repo, title)
	subject.Number = number
	return subject
}

// TestEmptyCopyAndCaptionAreSharedPresentation
func TestEmptyCopyAndCaptionAreSharedPresentation(t *testing.T) {
	t.Parallel()
	if got := WorkPresentationMainEmptyMessage(true, true); got != "No regular runs match these filters." {
		t.Fatalf("watchers match = %q", got)
	}
	if got := WorkPresentationMainEmptyMessage(true, false); got != "No runs match these filters." {
		t.Fatalf("no match = %q", got)
	}
	if got := WorkPresentationMainEmptyMessage(false, false); !strings.HasPrefix(got, "No runs yet.") {
		t.Fatalf("no runs = %q", got)
	}
	counts := RunCounts{Total: 9, Active: 4, Watching: 2, Running: 1, NeedsYou: 2, ShippedToday: 1}
	if got := WorkPresentationCaption(counts); got != "4 active · 2 watching · 2 need you · 1 shipped today" {
		t.Fatalf("caption = %q", got)
	}
}

// TestArchivedBeltsAreMarkedInThePicker
func TestArchivedBeltsAreMarkedInThePicker(t *testing.T) {
	t.Parallel()
	if got := WorkBeltNew("a", "Issues", true).Label; got != "Issues · archived" {
		t.Fatalf("archived label = %q", got)
	}
	if got := WorkBeltNew("b", "Issues", false).Label; got != "Issues" {
		t.Fatalf("live label = %q", got)
	}
}
