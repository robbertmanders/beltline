// Tests of the belt model: JSON round trips, AFK modes and backends.

package core

import (
	"slices"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// btDecode reads a JSON document into a T and fails the test on error.
func btDecode[T any](t *testing.T, data string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

// btEncode writes v in the wire form and fails the test on error.
// btWithoutKey is the object data without its top-level key, so a test can
// check a file that predates the key.
func btWithoutKey(t *testing.T, data, key string) string {
	t.Helper()
	var members map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &members); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	if _, ok := members[key]; !ok {
		t.Fatalf("%s key not found in %s", key, data)
	}
	delete(members, key)
	out, err := json.Marshal(members)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(out)
}

func btEncode(t *testing.T, v any) string {
	t.Helper()
	data, err := jsonx.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(data)
}

// btBelt is a belt of one trigger entry, with the given stages and no forks,
// as DefaultBelt gives it.
func btBelt(stages ...Stage) Belt {
	belt := DefaultBelt("belt", "Belt", []TriggerEntry{{ID: "1", Trigger: btIssues(), Repos: []string{"a/b"}}}, stages)
	belt.Forks = []Fork{}
	return belt
}

// btIssues is the issues trigger with the default filter.
func btIssues() Trigger { return TriggerKindIssues.Fresh() }

// btOutcome is a named outcome that moves on.
func btOutcome(name string) Outcome {
	return Outcome{ID: name, Name: name, When: name, Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionNext})}
}

// TestBeltMarshalExactBytes checks the keys and their order Belt.encode(to:) writes.
func TestBeltMarshalExactBytes(t *testing.T) {
	t.Parallel()
	belt := Belt{
		ID: "b", Name: "B", Version: 1,
		Triggers: []TriggerEntry{{ID: "1", Trigger: Trigger{Kind: TriggerKindManual}, Repos: []string{"a/b"}}},
		Policies: DefaultPolicies(), Stages: []Stage{}, Forks: []Fork{}, Afk: AfkModeGlobal,
	}
	want := `{"id":"b","name":"B","version":1,"triggers":[{"id":"1","trigger":{"kind":"manual"},"repos":["a/b"]}],"base":"","policies":{"retriageOnEdit":true,"stopOnClose":true,"commentOnHold":false,"waitForBlockers":true},"stages":[],"forks":[],"afk":"global"}`
	if got := btEncode(t, belt); got != want {
		t.Fatalf("belt JSON\n got %s\nwant %s", got, want)
	}
}

// TestBeltsRoundTripThroughJSON builds a literal belt with the same shape as the
// defaults-built one, and checks that it round-trips through JSON.
func TestBeltsRoundTripThroughJSON(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Agent: &AgentRef{ID: "build", Version: 1}, Outcomes: []Outcome{btOutcome("done")}})
	stage.Thinking = new(ThinkingHigh)
	watch := NewStage(StageArgs{ID: "merge", Type: StageTypeWatch, Outcomes: []Outcome{btOutcome("merged")}})
	belt := btBelt(stage, watch)
	belt.Stages[0].Thinking = new(ThinkingMedium)
	data := btEncode(t, belt)
	reopened := btDecode[Belt](t, data)
	if d := testkit.Diff(belt, reopened); d != "" {
		t.Fatalf("round trip differs:\n%s", d)
	}
	if reopened.Stages[0].Thinking == nil || *reopened.Stages[0].Thinking != ThinkingMedium {
		t.Fatalf("thinking = %v, want medium", reopened.Stages[0].Thinking)
	}
	if !strings.Contains(data, `"do":"next"`) {
		t.Errorf("belt JSON has no %q: %s", `"do":"next"`, data)
	}
	if strings.Contains(data, `"comment":false`) {
		t.Errorf("belt JSON writes a false comment: %s", data)
	}
}

// TestAMissingModeFollowsGlobalAndEachModeRoundTrips is ported with a literal belt.
func TestAMissingModeFollowsGlobalAndEachModeRoundTrips(t *testing.T) {
	t.Parallel()
	belt := btBelt()
	if belt.Afk != AfkModeGlobal {
		t.Fatalf("new belt afk = %q, want global", belt.Afk)
	}
	data := btEncode(t, belt)
	missing := btWithoutKey(t, data, "afk")
	if got := btDecode[Belt](t, missing).Afk; got != AfkModeGlobal {
		t.Fatalf("missing afk decodes to %q, want global", got)
	}
	for _, mode := range AllAfkMode {
		saved := btBelt()
		saved.Afk = mode
		if got := btDecode[Belt](t, btEncode(t, saved)).Afk; got != mode {
			t.Fatalf("mode %q decodes to %q", mode, got)
		}
	}
}

// TestABeltSavedWithTheOldOptInReadsAsAMode is ported with a literal belt.
func TestABeltSavedWithTheOldOptInReadsAsAMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		optedIn bool
		want    AfkMode
	}{{true, AfkModeAlways}, {false, AfkModeGlobal}}
	for _, tc := range cases {
		t.Run(map[bool]string{true: "on", false: "off"}[tc.optedIn], func(t *testing.T) {
			t.Parallel()
			data := btWithoutKey(t, btEncode(t, btBelt()), "afk")
			flag := "false"
			if tc.optedIn {
				flag = "true"
			}
			data = strings.Replace(data, "{", `{"afkEnabled":`+flag+`,`, 1)
			if got := btDecode[Belt](t, data).Afk; got != tc.want {
				t.Fatalf("afkEnabled %s reads as %q, want %q", flag, got, tc.want)
			}
		})
	}
}

// TestAfkModeFromChoice covers AfkMode from a choice and runChoice.
func TestAfkModeFromChoice(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	if AfkModeFromChoice(&yes) != AfkModeAlways || AfkModeFromChoice(&no) != AfkModeNever || AfkModeFromChoice(nil) != AfkModeGlobal {
		t.Fatal("AfkModeFromChoice does not map true, false and nil to always, never and global")
	}
	for _, m := range AllAfkMode {
		if AfkModeFromChoice(m.RunChoice()) != m {
			t.Errorf("RunChoice does not round trip for %q", m)
		}
	}
}

// TestCodexIsListedAndUsesItsExecutableOverride is ported with a fake environment.
func TestCodexIsListedAndUsesItsExecutableOverride_Belt(t *testing.T) {
	t.Parallel()
	if !slices.Contains(AllBackend, BackendCodex) {
		t.Fatal("codex is not listed")
	}
	if BackendCodex.Label() != "Codex CLI" || Backend("codex") != BackendCodex {
		t.Fatal("codex label or raw value is wrong")
	}
	none := func(string) string { return "" }
	if got := BackendCodex.Executable(none); got != "codex" {
		t.Errorf("default executable = %q, want codex", got)
	}
	override := func(key string) string {
		if key == "BELTLINE_CODEX" {
			return "/opt/codex"
		}
		return ""
	}
	if got := BackendCodex.Executable(override); got != "/opt/codex" {
		t.Errorf("override executable = %q, want /opt/codex", got)
	}
}

// TestSavedRunnersRoundTrip_Belt checks the belt part of saved runners.
func TestSavedRunnersRoundTrip_Belt(t *testing.T) {
	t.Parallel()
	for _, backend := range AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			stage := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Agent: &AgentRef{ID: "build", Version: 1}, Outcomes: []Outcome{btOutcome("done")}})
			stage.Runner = &backend
			belt := btBelt(stage)
			got := btDecode[Belt](t, btEncode(t, belt)).Stages[0].Runner
			if got == nil || *got != backend {
				t.Fatalf("runner = %v, want %q", got, backend)
			}
		})
	}
}

// TestMissingRunnersStillDefaultToPi_Belt checks the belt part of missing runners.
func TestMissingRunnersStillDefaultToPi_Belt(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "build", Type: StageTypeAgent, Agent: &AgentRef{ID: "build", Version: 1}, Outcomes: []Outcome{btOutcome("done")}})
	stage.Runner = nil
	decoded := btDecode[Belt](t, btEncode(t, btBelt(stage)))
	if decoded.Stages[0].Runner != nil {
		t.Fatalf("runner = %v, want none", *decoded.Stages[0].Runner)
	}
	if pool := StageAdjustBackend(decoded.Stages[0]); pool != BackendPi {
		t.Fatalf("runner or default = %q, want pi", pool)
	}
}

// TestBeltDecodeNumbersMissingTriggerIDs covers the id numbering of Belt.init(from:).
func TestBeltDecodeNumbersMissingTriggerIDs(t *testing.T) {
	t.Parallel()
	data := `{"id":"b","name":"B","version":1,"stages":[],"afk":"never",
		"triggers":[{"id":"","trigger":{"kind":"manual"},"repos":[]},{"id":"1","trigger":{"kind":"manual"},"repos":[]},{"id":"","trigger":{"kind":"manual"},"repos":[]}]}`
	belt := btDecode[Belt](t, data)
	got := []string{belt.Triggers[0].ID, belt.Triggers[1].ID, belt.Triggers[2].ID}
	want := []string{"2", "1", "3"}
	if testkit.Diff(want, got) != "" {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if belt.Afk != AfkModeNever {
		t.Fatalf("afk = %q, want never", belt.Afk)
	}
	if belt.Base != "" || testkit.Diff(DefaultPolicies(), belt.Policies) != "" {
		t.Fatalf("base %q and policies %+v, want the defaults", belt.Base, belt.Policies)
	}
}

// TestBeltLegacySingleTriggerShape reads a belt saved with one trigger and its
// repositories, base and policies nested on it.
func TestBeltLegacySingleTriggerShape(t *testing.T) {
	t.Parallel()
	data := `{"id":"b","name":"B","version":3,"stages":[],"trigger":{"kind":"jira","repos":["a/b"],"jql":"project = PAY","poll":30,"base":"develop","retriageOnEdit":false,"stopOnClose":true,"commentOnHold":false}}`
	belt := btDecode[Belt](t, data)
	if got := belt.Trigger(); testkit.Diff(Trigger{Kind: TriggerKindJira, Jira: &JiraQuery{Jql: "project = PAY", Poll: 30}}, got) != "" {
		t.Fatalf("trigger = %+v", got)
	}
	if belt.Triggers[0].ID != TriggerEntryLegacyID {
		t.Errorf("entry id = %q, want %q", belt.Triggers[0].ID, TriggerEntryLegacyID)
	}
	if testkit.Diff([]string{"a/b"}, belt.Repos()) != "" || belt.Base != "develop" {
		t.Errorf("repos %v, base %q", belt.Repos(), belt.Base)
	}
	want := Policies{RetriageOnEdit: false, StopOnClose: true, CommentOnHold: false, WaitForBlockers: true}
	if testkit.Diff(want, belt.Policies) != "" {
		t.Errorf("policies = %+v, want %+v", belt.Policies, want)
	}
}

// TestBeltLegacyTopLevelReposWin checks that repos at the belt's top level win over the nested ones.
func TestBeltLegacyTopLevelReposWin(t *testing.T) {
	t.Parallel()
	data := `{"id":"b","name":"B","version":1,"stages":[],"repos":["top/one"],"trigger":{"kind":"manual","repos":["nested/two"],"base":"main"},"base":"release"}`
	belt := btDecode[Belt](t, data)
	if testkit.Diff([]string{"top/one"}, belt.Repos()) != "" {
		t.Fatalf("repos = %v, want the top-level ones", belt.Repos())
	}
	if belt.Base != "release" {
		t.Fatalf("base = %q, want the top-level release", belt.Base)
	}
}

// TestBeltAfkEnabledIsReadWhenAfkIsAbsent covers the legacy afkEnabled key.
func TestBeltAfkEnabledIsReadWhenAfkIsAbsent(t *testing.T) {
	t.Parallel()
	base := `"id":"b","name":"B","version":1,"stages":[],"triggers":[]`
	if got := btDecode[Belt](t, `{`+base+`,"afkEnabled":true}`).Afk; got != AfkModeAlways {
		t.Errorf("afkEnabled true reads as %q", got)
	}
	if got := btDecode[Belt](t, `{`+base+`,"afkEnabled":false}`).Afk; got != AfkModeGlobal {
		t.Errorf("afkEnabled false reads as %q", got)
	}
	if got := btDecode[Belt](t, `{`+base+`,"afk":"never","afkEnabled":true}`).Afk; got != AfkModeNever {
		t.Errorf("afk key does not win over afkEnabled: %q", got)
	}
}

// TestPoliciesMissingKeysTakeTheirDefaults covers Policies.init(from:): every
// missing policy is on except the hold comment, which is off.
func TestPoliciesMissingKeysTakeTheirDefaults(t *testing.T) {
	t.Parallel()
	if got := btDecode[Policies](t, `{}`); testkit.Diff(DefaultPolicies(), got) != "" {
		t.Errorf("empty policies = %+v", got)
	}
	got := btDecode[Policies](t, `{"stopOnClose":false,"waitForBlockers":null}`)
	want := Policies{RetriageOnEdit: true, StopOnClose: false, CommentOnHold: false, WaitForBlockers: true}
	if testkit.Diff(want, got) != "" {
		t.Errorf("partial policies = %+v, want %+v", got, want)
	}
}

// TestPoliciesEncodeEveryKey checks that Policies writes every key, false ones included.
func TestPoliciesEncodeEveryKey(t *testing.T) {
	t.Parallel()
	p := Policies{RetriageOnEdit: false, StopOnClose: true, CommentOnHold: false, WaitForBlockers: true}
	want := `{"retriageOnEdit":false,"stopOnClose":true,"commentOnHold":false,"waitForBlockers":true}`
	if got := btEncode(t, p); got != want {
		t.Fatalf("policies JSON = %s, want %s", got, want)
	}
}

// TestBeltTriggerHelpers covers the first-entry accessors of Belt.
func TestBeltTriggerHelpers(t *testing.T) {
	t.Parallel()
	empty := Belt{Triggers: []TriggerEntry{}}
	if testkit.Diff(TriggerKindIssues.Fresh(), empty.Trigger()) != "" {
		t.Errorf("empty belt trigger = %+v", empty.Trigger())
	}
	if got := empty.Summary(); got != "Add at least one trigger" {
		t.Errorf("empty summary = %q", got)
	}
	empty.SetRepos([]string{"x/y"})
	if testkit.Diff([]string{"x/y"}, empty.Repos()) != "" || len(empty.Triggers) != 1 {
		t.Errorf("SetRepos on an empty belt created %d entries", len(empty.Triggers))
	}
	if empty.Triggers[0].ID != "1" {
		t.Errorf("created entry id = %q, want 1", empty.Triggers[0].ID)
	}
	empty.SetTrigger(Trigger{Kind: TriggerKindManual})
	if empty.Trigger().Kind != TriggerKindManual || empty.Repos()[0] != "x/y" {
		t.Errorf("SetTrigger changed the repositories or kept the old kind")
	}
	if got := empty.Stage("missing"); got != nil {
		t.Errorf("Stage(missing) = %+v, want nil", got)
	}
}

// TestOldRunPullRequestFieldsRemainUnknown
// The run is testModelRun (no Engine.start); the test only reads its pull request.
func TestOldRunPullRequestFieldsRemainUnknown(t *testing.T) {
	t.Parallel()
	run := testModelRun()
	run.SetPullRequest(&PullRequest{Number: 201, URL: "u", State: PullRequestStateOpen})
	loaded := btDecode[Run](t, btEncode(t, run))
	if d := testDiffEmpty(run.PullRequest(), loaded.PullRequest()); d != "" {
		t.Fatalf("pull request differs:\n%s", d)
	}
	pull := loaded.PullRequest()
	if pull == nil {
		t.Fatal("pull request lost")
	}
	if pull.HeadSha != nil || pull.BaseRef != nil || pull.Checks != nil || pull.Conflicted != nil || pull.Behind != nil {
		t.Errorf("fields an old run never had were read: %+v", pull)
	}
}

// TestARunStartsWithItsBeltsMode
func TestARunStartsWithItsBeltsMode(t *testing.T) {
	t.Parallel()
	for _, mode := range AllAfkMode {
		belt := testModelBelt()
		belt.Afk = mode
		run := NewRun(RunArgs{Belt: belt, Subject: Subject{Kind: SubjectKindIssue, Repo: "a/b", Number: 1, Title: "T"}, Now: testT0})
		if run.Afk() != mode {
			t.Errorf("%s: run mode = %s", mode, run.Afk())
		}
		if d := testDiffEmpty(mode.RunChoice(), run.AfkEnabled); d != "" {
			t.Errorf("%s: afkEnabled differs:\n%s", mode, d)
		}
	}
}
