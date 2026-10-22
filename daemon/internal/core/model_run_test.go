package core

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

// testRunObject is a run's keys as a JSON object: the keys Run.init(from:) requires,
// with no stages, policies or trigger source, so it decodes without the belt's
// codecs. Tests add or remove keys on it.
func testRunObject() map[string]any {
	return map[string]any{
		"id": "bug:acme/billing#142", "beltId": "bug", "beltName": "Bug", "beltVersion": 2,
		"stages": []any{}, "issue": map[string]any{"repo": "acme/billing", "title": "Export"},
		"state": "queued", "stageId": "", "uses": map[string]any{}, "attempts": []any{},
		"legs":      []any{map[string]any{"id": "main", "state": "queued"}},
		"createdAt": "2026-07-22T02:18:00Z", "updatedAt": "2026-07-22T02:18:00Z",
	}
}

// testRunFrom decodes obj as a run and returns it, failing the test on error.
func testRunFrom(t *testing.T, obj map[string]any) Run {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	var run Run
	testDecode(t, string(data), &run)
	return run
}

// TestEachKindHasItsOwnIdsAndFolders checks each subject kind's run id, folder,
// ref and slug.
func TestEachKindHasItsOwnIdsAndFolders(t *testing.T) {
	t.Parallel()
	pull := Subject{Kind: SubjectKindPullRequest, Repo: "acme/web", Number: 57, Title: "Move checkout", Occurrence: new("100")}
	ticket := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment"}
	manual := Subject{Kind: SubjectKindManual, Repo: "acme/web", Key: "3", Title: "Tidy"}
	scheduled := Subject{Kind: SubjectKindScheduled, Repo: "acme/platform", Key: "2026-09-29", Title: "Sweep"}

	if got := RunID("b", pull); got != "b:acme/web#57@100" {
		t.Errorf("pull id = %q", got)
	}
	if got := RunID("b", ticket); got != "b:jira:PAY-31" {
		t.Errorf("ticket id = %q", got)
	}
	if got := RunID("b", manual); got != "b:manual:3" {
		t.Errorf("manual id = %q", got)
	}
	folders := map[string]Run{
		"reviews/acme/web/pr-57-100":    {BeltID: "reviews", Subject: pull},
		"jira/jira/PAY-31":              {BeltID: "jira", Subject: ticket},
		"schedule/scheduled/2026-09-29": {BeltID: "schedule", Subject: scheduled},
		"tasks/manual/3":                {BeltID: "tasks", Subject: manual},
	}
	for want, run := range folders {
		if got := run.Folder(); got != want {
			t.Errorf("folder = %q, want %q", got, want)
		}
	}
	if got := []string{pull.Ref(), ticket.Ref(), manual.Ref(), scheduled.Ref()}; !slices.Equal(got, []string{"#57", "PAY-31", "T3", "09-29"}) {
		t.Errorf("refs = %v", got)
	}
	if got := []string{pull.Slug(), ticket.Slug(), manual.Slug(), scheduled.Slug()}; !slices.Equal(got, []string{"pr-57", "PAY-31", "task-3", "2026-09-29"}) {
		t.Errorf("slugs = %v", got)
	}
	// The folders of a run started on each kind's belt.
	if got := NewRun(RunArgs{Belt: testKindBelt(TriggerKindReviews), Subject: pull, Now: testT0}).Folder(); got != "reviews/acme/web/pr-57-100" {
		t.Errorf("reviews run folder = %q", got)
	}
	if got := NewRun(RunArgs{Belt: testKindBelt(TriggerKindJira), Subject: ticket, Now: testT0}).Folder(); got != "jira/jira/PAY-31" {
		t.Errorf("jira run folder = %q", got)
	}
	if got := NewRun(RunArgs{Belt: testKindBelt(TriggerKindSchedule), Subject: scheduled, Now: testT0}).Folder(); got != "schedule/scheduled/2026-09-29" {
		t.Errorf("schedule run folder = %q", got)
	}
}

// TestRunStateActiveAndNeedsYou checks which states are still going and which wait for a person.
func TestRunStateActiveAndNeedsYou(t *testing.T) {
	t.Parallel()
	for _, state := range AllRunState {
		wantActive := state != RunStateDone && state != RunStateShipped && state != RunStateDropped
		wantNeeds := state == RunStateWaiting || state == RunStateHeld
		if state.IsActive() != wantActive || state.NeedsYou() != wantNeeds {
			t.Errorf("%s: active %t needsYou %t", state, state.IsActive(), state.NeedsYou())
		}
	}
}

// TestRunDecodeLegacyTriggerSuppliesPoliciesAndBase checks that a run saved with
// a trigger copy and no policies or base reads them from that trigger, with each
// policy flag missing taken as on, the hold comment included, as those runs ran.
func TestRunDecodeLegacyTriggerSuppliesPoliciesAndBase(t *testing.T) {
	t.Parallel()
	obj := testRunObject()
	obj["trigger"] = map[string]any{"repos": []string{"acme/billing"}, "base": "develop", "retriageOnEdit": false}
	run := testRunFrom(t, obj)
	if run.Base != "develop" {
		t.Errorf("base = %q, want develop", run.Base)
	}
	want := legacyPolicies()
	want.RetriageOnEdit = false
	if run.Policies != want {
		t.Errorf("policies = %+v, want %+v", run.Policies, want)
	}
	if run.Source != nil {
		t.Errorf("source = %+v, want nil on a legacy run", run.Source)
	}
	if got := testRunFrom(t, testRunObject()); got.Policies != legacyPolicies() || got.Base != "" {
		t.Errorf("a run with no trigger read policies %+v and base %q", got.Policies, got.Base)
	}
}

// TestRunDecodeLegacyPullRequestAndRepos checks the legacy shapes Run.init(from:)
// reads: a single pullRequest becomes the list, its empty repository takes the
// subject's, and the repositories default to the subject's.
func TestRunDecodeLegacyPullRequestAndRepos(t *testing.T) {
	t.Parallel()
	obj := testRunObject()
	obj["pullRequest"] = map[string]any{"number": 7, "url": "https://github.com/acme/billing/pull/7", "state": "open"}
	run := testRunFrom(t, obj)
	if !slices.Equal(run.Repos, []string{"acme/billing"}) {
		t.Errorf("repos = %v, want the subject's", run.Repos)
	}
	wantPulls := []PullRequest{{Repo: "acme/billing", Number: 7, URL: "https://github.com/acme/billing/pull/7", State: PullRequestStateOpen}}
	if d := testDiffEmpty(wantPulls, run.PullRequests); d != "" {
		t.Errorf("pull requests: %s", d)
	}
	obj["pullRequests"] = []any{}
	if run := testRunFrom(t, obj); len(run.PullRequests) != 0 {
		t.Errorf("pullRequests present and empty still read the legacy one: %v", run.PullRequests)
	}
	delete(obj, "pullRequest")
	delete(obj, "pullRequests")
	if run := testRunFrom(t, obj); len(run.PullRequests) != 0 {
		t.Errorf("no pull request keys read %v", run.PullRequests)
	}
}

// TestRunDecodeNullModelAdmissionFallsBack checks that a present but null model
// admission reads as an empty admission, not as a missing one
// since the decoder falls back on an unreadable field.
func TestRunDecodeNullModelAdmissionFallsBack(t *testing.T) {
	t.Parallel()
	obj := testRunObject()
	obj["modelAdmission"] = nil
	run := testRunFrom(t, obj)
	if run.ModelAdmission == nil {
		t.Fatal("a present null admission read as missing")
	}
	if run.ModelAdmission.SchemaVersion != 1 || len(run.ModelAdmission.Models) != 0 || len(run.ModelAdmission.LegacyRepairs) != 0 {
		t.Errorf("fallback admission = %+v", *run.ModelAdmission)
	}
	delete(obj, "modelAdmission")
	if run := testRunFrom(t, obj); run.ModelAdmission != nil {
		t.Errorf("an absent admission read as %+v", *run.ModelAdmission)
	}
}

// TestRunDecodeFinishedRunHasNoLegs checks that a run that has finished and
// was saved without legs keeps none, as it had none when it was saved.
func TestRunDecodeFinishedRunHasNoLegs(t *testing.T) {
	t.Parallel()
	obj := testRunObject()
	obj["state"] = "shipped"
	delete(obj, "legs")
	if run := testRunFrom(t, obj); len(run.Legs) != 0 {
		t.Errorf("a shipped run read %d legs", len(run.Legs))
	}
}

// TestRunRoutesAndPullRequestSetters checks the routes accessors and that the
// single pull request setter fills in the subject's repository.
func TestRunRoutesAndPullRequestSetters(t *testing.T) {
	t.Parallel()
	run := Run{Subject: Subject{Kind: SubjectKindIssue, Repo: "acme/web", Number: 1, Title: "T"}}
	run.SetRoutes(map[string][]string{"size": {"simple"}})
	if !slices.Equal(run.Routes()["size"], []string{"simple"}) {
		t.Errorf("routes = %v", run.Routes())
	}
	run.SetPullRequest(&PullRequest{Number: 9, URL: "u", State: PullRequestStateOpen})
	if got := run.PullRequest(); got == nil || got.Repo != "acme/web" || got.Number != 9 {
		t.Errorf("pull request = %+v", got)
	}
	run.SetPullRequest(nil)
	if run.PullRequest() != nil || len(run.PullRequests) != 0 {
		t.Errorf("clearing the pull request left %v", run.PullRequests)
	}
}

// TestRunAfkEffectiveAndSetAfk checks that an explicit run choice wins over the
// global switch, and that a choice saved from a mode is the mode's run choice.
func TestRunAfkEffectiveAndSetAfk(t *testing.T) {
	t.Parallel()
	var run Run
	if !run.AfkEffective(true) || run.AfkEffective(false) {
		t.Error("no choice should follow the global switch")
	}
	yes, no := true, false
	run.AfkEnabled = &no
	if run.AfkEffective(true) {
		t.Error("an explicit no should win over a global yes")
	}
	run.AfkEnabled = &yes
	if !run.AfkEffective(false) {
		t.Error("an explicit yes should win over a global no")
	}
	run.SetAfk(AfkModeNever)
	if run.AfkEnabled == nil || *run.AfkEnabled {
		t.Errorf("never saved as %v", run.AfkEnabled)
	}
	run.SetAfk(AfkModeGlobal)
	if run.AfkEnabled != nil {
		t.Errorf("global saved as %v, want nil", *run.AfkEnabled)
	}
}

// TestTheButtonCyclesThroughAllThree checks the AFK button:
// the button moves global to always to never to global, and each mode has its label.
func TestTheButtonCyclesThroughAllThree(t *testing.T) {
	t.Parallel()
	if AfkModeGlobal.Next() != AfkModeAlways {
		t.Errorf("global.next = %s, want always", AfkModeGlobal.Next())
	}
	if AfkModeAlways.Next().Next() != AfkModeGlobal {
		t.Errorf("always.next.next = %s, want global", AfkModeAlways.Next().Next())
	}
	labels := map[string]bool{}
	for _, mode := range AllAfkMode {
		labels[mode.Label()] = true
	}
	if len(labels) != 3 || !labels["AFK: Always"] || !labels["AFK: Never"] || !labels["AFK: Global"] {
		t.Errorf("labels = %v", labels)
	}
}

// TestRunDecodeWithoutRestartOrOperation checks that a run saved before overrule
// restarts reads with no restart and operation 0, and that a saved restart and
// operation are read back. The Run half of AfkTests.oldDecisionsAndRunsDecodeWithoutAnOverrule.
func TestRunDecodeWithoutRestartOrOperation(t *testing.T) {
	t.Parallel()
	older := testRunFrom(t, testRunObject())
	if older.Restart != nil || older.Operation != 0 {
		t.Errorf("older run = restart %+v operation %d", older.Restart, older.Operation)
	}
	obj := testRunObject()
	obj["operation"] = 4
	obj["restart"] = map[string]any{"decisionId": "d1", "attemptId": "approve-1", "stageId": "spec",
		"comments": "Again", "at": "2026-07-22T02:18:00Z", "phase": "intent"}
	restarted := testRunFrom(t, obj)
	if restarted.Operation != 4 || restarted.Restart == nil || restarted.Restart.Phase != RestartIntentPhaseIntent ||
		restarted.Restart.Comments != "Again" || restarted.Restart.Detail != nil {
		t.Errorf("restarted run = operation %d restart %+v", restarted.Operation, restarted.Restart)
	}
}

// testT0 is the tests' clock: 1_800_000_000 seconds after 1970.
var testT0 = time.Unix(1_800_000_000, 0).UTC()

// testJSONMap encodes v and reads it back as a JSON object, so a test can add or
// remove keys from the map.
func testJSONMap(t *testing.T, v any) map[string]any {
	t.Helper()
	var obj map[string]any
	testDecode(t, btEncode(t, v), &obj)
	return obj
}

// testKindBelt is a belt fixture for a trigger kind: one entry with the
// kind's fresh trigger, the repositories that kind gets, and the stages
// given (none when there are none).
func testKindBelt(kind TriggerKind, stages ...Stage) Belt {
	repos := map[TriggerKind][]string{
		TriggerKindIssues:   {"acme/billing"},
		TriggerKindReviews:  {"acme/web"},
		TriggerKindJira:     {"acme/billing"},
		TriggerKindSchedule: {"acme/platform"},
		TriggerKindManual:   {"acme/web", "acme/billing"},
	}[kind]
	belt := DefaultBelt(string(kind), kind.Title(), []TriggerEntry{{ID: TriggerEntryLegacyID, Trigger: kind.Fresh(), Repos: repos}}, stages)
	belt.Forks = []Fork{}
	return belt
}

// testModelBelt is the issues belt that the sample run starts from, with the given stages.
func testModelBelt(stages ...Stage) Belt {
	return testKindBelt(TriggerKindIssues, stages...)
}

// testModelRun is a run with no engine start: the issues belt for
// issue acme/billing#142, queued at its first stage.
func testModelRun(stages ...Stage) Run {
	subject := Subject{Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export invoices as CSV", Body: "Add an export button."}
	return NewRun(RunArgs{Belt: testModelBelt(stages...), Subject: subject, Now: testT0})
}

// TestRunDecodeMalformedAdmissionFailsClosed checks that a present admission which
// does not decode, or whose legacyRepairs is null, gives an empty admission and
// never legacy compatibility, while the run itself still decodes.
func TestRunDecodeMalformedAdmissionFailsClosed(t *testing.T) {
	t.Parallel()
	for _, admission := range []any{
		map[string]any{"schemaVersion": "corrupt"},
		map[string]any{"schemaVersion": 1, "models": map[string]any{}, "legacyRepairs": nil},
		map[string]any{"schemaVersion": 1, "models": map[string]any{}},
		map[string]any{"schemaVersion": 1, "models": "many", "legacyRepairs": []any{}},
		map[string]any{"schemaVersion": 1, "models": map[string]any{"pi": []any{"not a choice"}}, "legacyRepairs": []any{}},
		map[string]any{"schemaVersion": 1, "models": map[string]any{}, "legacyRepairs": []any{}, "legacyAttemptSettings": []any{}},
		"an admission",
		[]any{},
		nil,
	} {
		obj := testRunObject()
		obj["modelAdmission"] = admission
		run := testRunFrom(t, obj)
		if run.ModelAdmission == nil {
			t.Fatalf("%v: a present admission read as missing", admission)
		}
		if len(run.ModelAdmission.Models) != 0 || len(run.ModelAdmission.LegacyRepairs) != 0 {
			t.Errorf("%v: fallback admission = %+v", admission, *run.ModelAdmission)
		}
	}
}

// TestNewRunStartsWithEmptyDictionaries checks that a new run's dictionaries are
// empty maps, which encode as {} rather than null.
func TestNewRunStartsWithEmptyDictionaries(t *testing.T) {
	t.Parallel()
	run := testModelRun()
	if run.WatchObservations == nil || run.TicketObservations == nil || run.Uses == nil || run.Routes() == nil {
		t.Fatalf("a new run has a nil dictionary: %+v", run)
	}
	encoded := btEncode(t, run)
	for _, key := range []string{`"watchObservations":{}`, `"ticketObservations":{}`, `"uses":{}`, `"routes":{}`} {
		if !strings.Contains(encoded, key) {
			t.Errorf("encoded run lacks %s", key)
		}
	}
}
