package core

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// wgRun is a run of the default issue belt on repo, numbered n, in state, named belt.
func wgRun(n int, repo string, state RunState, belt string) Run {
	subject := DefaultSubject(repo, "T"+strconv.Itoa(n))
	subject.Number = n
	run := NewRun(RunArgs{Belt: SampleBelt([]string{repo}, "agent", SampleAgents(), true), Subject: subject, Now: time.Unix(int64(n), 0)})
	run.State = state
	run.BeltName = belt
	return run
}

// wgRunDefault is wgRun with the defaults: acme/billing, running, Build.
func wgRunDefault(n int) Run {
	return wgRun(n, "acme/billing", RunStateRunning, "Build")
}

// TestGroupOrderAndHeaders
func TestGroupOrderAndHeaders(t *testing.T) {
	t.Parallel()
	if !reflect.DeepEqual(AllGroupBy, []GroupBy{GroupByNone, GroupByRepo, GroupByStatus, GroupByBelt, GroupByEpic}) {
		t.Fatalf("GroupBy cases = %v", AllGroupBy)
	}
	runs := []Run{
		wgRun(1, "acme/billing", RunStateShipped, "Zebra"),
		wgRun(2, "acme/web", RunStateRunning, "Build"),
		wgRun(3, "acme/billing", RunStateHeld, "Build"),
		wgRun(4, "acme/billing", RunStateWaiting, "Build"),
	}
	if got := WorkGroupGroupRuns(runs, GroupByStatus); !wgKeysAre(got, "Needs you", "Running", "Shipped") {
		t.Fatalf("status groups = %v, want Needs you, Running, Shipped", wgValues(got))
	}
	var allStates []Run
	for i, state := range AllRunState {
		allStates = append(allStates, wgRun(i, "acme/billing", state, "Build"))
	}
	if got := WorkGroupGroupRuns(allStates, GroupByStatus); !wgKeysAre(got, "Needs you", "Running", "Queued", "Parked", "Dropped", "Done", "Shipped") {
		t.Fatalf("all-state groups = %v", wgValues(got))
	}
	paused := wgRun(9, "acme/billing", RunStateRunning, "Build")
	paused.Pause = &RunPause{Phase: RunPausePhasePaused, Operation: "p", RequestedAt: jsonx.Date{Time: testT0}, Snapshots: []RunPauseSnapshot{}}
	if got := WorkGroupGroupRuns([]Run{paused, wgRun(1, "acme/billing", RunStateQueued, "Build")}, GroupByStatus); !wgKeysAre(got, "Paused", "Queued") {
		t.Fatalf("paused groups = %v, want Paused, Queued", wgValues(got))
	}
	if got := WorkGroupGroupRuns(runs, GroupByRepo); !wgKeysAre(got, "acme/billing", "acme/web") {
		t.Fatalf("repo groups = %v, want acme/billing, acme/web", wgValues(got))
	}
	if got := WorkGroupGroupRuns(runs, GroupByBelt); !wgKeysAre(got, "Build", "Zebra") {
		t.Fatalf("belt groups = %v, want Build, Zebra", wgValues(got))
	}
	if got := WorkGroupGroupRuns(runs, GroupByNone); len(got) == 0 || got[0].Title() != "" {
		t.Fatalf("none group title = %v, want empty", got)
	}
	titled := WorkGroupGroupRuns([]Run{wgRunDefault(1), wgRunDefault(2), wgRun(3, "acme/docs", RunStateRunning, "Build")}, GroupByRepo)
	if len(titled) != 2 || titled[0].Title() != "acme/billing · 2" || titled[1].Title() != "acme/docs · 1" {
		t.Fatalf("repo titles = %v", wgTitles(titled))
	}
	if got := WorkGroupGroupRuns([]Run{wgRunDefault(1), wgRunDefault(2)}, GroupByBelt); len(got) != 1 {
		t.Fatalf("belt groups count = %d, want 1", len(got))
	}
	if got := WorkGroupGroupRuns(nil, GroupByRepo); len(got) != 0 {
		t.Fatalf("empty groups = %v", got)
	}
	sameNamed := []Run{wgRun(1, "acme/beltline", RunStateRunning, "Build"), wgRun(2, "robbertmanders/beltline", RunStateRunning, "Build")}
	groups := WorkGroupGroupRuns(sameNamed, GroupByRepo)
	ambiguous := RepoLabelAmbiguousNames(sameNamed)
	if !wgKeysAre(groups, "acme/beltline", "robbertmanders/beltline") {
		t.Fatalf("same-named groups = %v", wgValues(groups))
	}
	if len(groups) != 2 || groups[0].Count != 1 || groups[1].Count != 1 {
		t.Fatalf("same-named counts wrong")
	}
	if RepoLabelShort(groups[0].Key.Value, ambiguous) != "beltline [acme]" || RepoLabelShort(groups[1].Key.Value, ambiguous) != "beltline [robbertmanders]" {
		t.Fatalf("disambiguated labels wrong")
	}

}

// TestChildrenGroupUnderTheirEpic
func TestChildrenGroupUnderTheirEpic(t *testing.T) {
	t.Parallel()
	epicSubject := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-40", Title: "Split invoicing", Labels: []string{"epic"}, IsOpen: true}
	epics := SampleEpicBelt(epicTestEpicTrigger(), []string{"acme/billing"}, SampleAgents())
	tickets := SampleBelt([]string{"acme/billing"}, "agent", SampleAgents(), true)
	tickets.Name = "Tickets"
	parent := "PAY-40"
	child := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-42", Title: "Part PAY-42", Parent: &parent, IsOpen: true}
	ticket := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment", IsOpen: true}
	epicRun := NewRun(RunArgs{Belt: epics, Subject: epicSubject, Now: testT0})
	epicRun.TicketObservations = map[string]TicketObservation{"watch": {Children: map[string]TicketObservationSeen{"#9": {Status: "open", IsOpen: true}}}}
	webEpic := NewRun(RunArgs{Belt: epics, Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/web", Number: 3, Title: "Web epic", IsOpen: true}, Now: testT0})
	webEpic.TicketObservations = map[string]TicketObservation{"watch": {Children: map[string]TicketObservationSeen{"#9": {Status: "open", IsOpen: true}}}}
	subIssue := Subject{Kind: SubjectKindIssue, Repo: "acme/web", Number: 9, Title: "Sub-issue", IsOpen: true}
	runs := []Run{
		epicRun,
		NewRun(RunArgs{Belt: tickets, Subject: child, Now: testT0}),
		NewRun(RunArgs{Belt: tickets, Subject: ticket, Now: testT0}),
		webEpic,
		NewRun(RunArgs{Belt: SampleBelt([]string{"acme/web"}, "agent", SampleAgents(), true), Subject: subIssue, Now: testT0}),
	}
	groups := WorkGroupGroupRuns(runs, GroupByEpic)
	if !wgKeysAre(groups, "acme/web#3 Web epic", "PAY-40 Split invoicing", WorkGroupNoEpic) {
		t.Fatalf("epic groups = %v", wgValues(groups))
	}
	counts := []int{}
	for _, group := range groups {
		counts = append(counts, group.Count)
	}
	if !reflect.DeepEqual(counts, []int{2, 2, 1}) {
		t.Fatalf("epic counts = %v, want [2 2 1]", counts)
	}
}

// TestRepositoryIdentityRoundTrips
func TestRepositoryIdentityRoundTrips(t *testing.T) {
	t.Parallel()
	original := wgRunDefault(1)
	data, err := jsonx.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	members, err := jsonx.Members(data)
	if err != nil {
		t.Fatal(err)
	}
	var subject map[string]any
	if err := json.Unmarshal(members["issue"], &subject); err != nil {
		t.Fatal(err)
	}
	if subject["repo"] != "acme/billing" {
		t.Fatalf("subject repo = %v", subject["repo"])
	}
	var decoded Run
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Subject.Repo != "acme/billing" || !reflect.DeepEqual(decoded.Repos, []string{"acme/billing"}) || decoded.ID != original.ID {
		t.Fatalf("decoded run = %+v", decoded)
	}
	// Folds are keyed by the dimension: each saved key is a dimension with its group values.
	var keys map[GroupBy][]WorkGroupKey
	if err := json.Unmarshal([]byte(`{"repo":[{"dimension":"repo","value":"acme/billing"}]}`), &keys); err != nil {
		t.Fatal(err)
	}
	groups := WorkGroupGroupRuns([]Run{decoded}, GroupByRepo)
	if !slices.Contains(keys[GroupByRepo], groups[0].Key) {
		t.Fatalf("saved key %v does not name group %v", keys[GroupByRepo], groups[0].Key)
	}

}

// wgKeysAre reports whether the groups' values are exactly want, in order.
func wgKeysAre(groups []WorkGroupGroup, want ...string) bool {
	values := wgValues(groups)
	return reflect.DeepEqual(values, want)
}

// wgValues is each group's key value, in order.
func wgValues(groups []WorkGroupGroup) []string {
	values := []string{}
	for _, group := range groups {
		values = append(values, group.Key.Value)
	}
	return values
}

// wgTitles is each group's title, in order.
func wgTitles(groups []WorkGroupGroup) []string {
	titles := []string{}
	for _, group := range groups {
		titles = append(titles, group.Title())
	}
	return titles
}

// wgRows is every run of the groups, in order.
func wgRows(groups []WorkGroupGroup) []Run {
	var rows []Run
	for _, group := range groups {
		rows = append(rows, group.Runs...)
	}
	return rows
}
