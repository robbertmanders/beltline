package core

import (
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// testRunAt parses an RFC 3339 time into a jsonx.Date for the test fixtures.
func testRunAt(t *testing.T, s string) jsonx.Date {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return jsonx.Date{Time: parsed}
}

// testRealisticRun is a run in the middle of its second stage: one finished
// attempt with spend, and one open agent attempt with its settings and
// checkpoint. It has no stages, so it encodes without the belt's codecs.
func testRealisticRun(t *testing.T) Run {
	t.Helper()
	end := testRunAt(t, "2026-07-22T02:20:00Z")
	tone := ToneGood
	provenance := CostProvenanceEstimated
	coverage := UsageCoverageComplete
	backend := BackendPi
	thinking := ThinkingHigh
	sessionID := "00000000-0000-4000-8000-000000000001"
	build := "build"
	return Run{
		ID: "bug:acme/billing#142", BeltID: "bug", BeltName: "Bug", BeltVersion: 2,
		Policies: DefaultPolicies(), Base: "develop", Repos: []string{"acme/billing"},
		Subject: Subject{
			Kind: SubjectKindIssue, Repo: "acme/billing", Number: 142, Title: "Export", Body: "Add it.",
			Labels: []string{"agent"}, URL: "https://github.com/acme/billing/issues/142", IsOpen: true,
			Assignees: []string{}, Blockers: []string{},
		},
		State: RunStateRunning, StageID: build,
		Uses:  map[string]int{"build:retry": 1},
		Taken: RouteSet{Routes: map[string][]string{"size": {"simple"}}},
		Legs:  []Leg{{ID: "main", StageID: &build, State: RunStateRunning}},
		Attempts: []Attempt{
			{
				ID: "triage-1", StageID: "triage", Number: 1, StartedAt: testRunAt(t, "2026-07-22T02:18:00Z"),
				EndedAt: &end, Outcome: new("ready to implement"), Tone: &tone, Route: new("Take the Simple route"),
				Session: new("bl-demo-61-triage"), SessionID: &sessionID, Folder: "01-triage-1",
				Usage: &Usage{CostNanos: new(12000000), Provenance: &provenance, Coverage: coverage,
					Input: new(100), Output: new(20), TotalTokens: new(120)},
			},
			{
				ID: "build-1", StageID: "build", Number: 1, StartedAt: testRunAt(t, "2026-07-22T02:21:00Z"),
				Session: new("bl-demo-61-build"), SessionID: &sessionID, Folder: "02-build-1",
				Settings:        &SessionSettings{Runner: &backend, Model: "m-1", Thinking: &thinking},
				UsageCheckpoint: &UsageCheckpoint{SessionID: &sessionID, Lines: new(4), Totals: &TokenTotals{Input: 5, CacheRead: 6, Output: 7, Total: 18}},
				AgentName:       new("bl-build"), LogMark: new(2), UsageMark: new(9),
			},
		},
		Generation: 1, Operation: 2, PauseTime: []RunPauseInterval{},
		CreatedAt: testRunAt(t, "2026-07-22T02:18:00Z"), UpdatedAt: testRunAt(t, "2026-07-22T02:21:00Z"),
	}
}

// wantRunWire is testRealisticRun in the wire form: sorted keys, optionals
// omitted when nil, the subject under "issue" and the routes as one string
// per fork.
const wantRunWire = `{"adjustments":[],"agents":[],"attempts":[{"endedAt":"2026-07-22T02:20:00Z","folder":"01-triage-1","id":"triage-1","number":1,"outcome":"ready to implement","route":"Take the Simple route","session":"bl-demo-61-triage","sessionId":"00000000-0000-4000-8000-000000000001","stageId":"triage","startedAt":"2026-07-22T02:18:00Z","tone":"good","usage":{"costNanos":12000000,"coverage":"complete","input":100,"output":20,"provenance":"estimated","totalTokens":120}},{"agentName":"bl-build","folder":"02-build-1","id":"build-1","logMark":2,"number":1,"session":"bl-demo-61-build","sessionId":"00000000-0000-4000-8000-000000000001","settings":{"model":"m-1","runner":"pi","thinking":"high"},"stageId":"build","startedAt":"2026-07-22T02:21:00Z","usageCheckpoint":{"lines":4,"sessionId":"00000000-0000-4000-8000-000000000001","totals":{"cacheRead":6,"input":5,"output":7,"reasoning":0,"total":18}},"usageMark":9}],"base":"develop","beltId":"bug","beltName":"Bug","beltVersion":2,"createdAt":"2026-07-22T02:18:00Z","forks":[],"generation":1,"id":"bug:acme/billing#142","issue":{"assignees":[],"blockers":[],"body":"Add it.","isOpen":true,"key":"","kind":"issue","labels":["agent"],"number":142,"repo":"acme/billing","title":"Export","url":"https://github.com/acme/billing/issues/142"},"joins":[],"legs":[{"id":"main","stageId":"build","state":"running"}],"mergeBranches":[],"mergesWaiting":[],"operation":2,"pauseTime":[],"policies":{"commentOnHold":false,"retriageOnEdit":true,"stopOnClose":true,"waitForBlockers":true},"pullRequests":[],"repos":["acme/billing"],"routes":{"size":"simple"},"stageId":"build","stages":[],"state":"running","ticketObservations":{},"updatedAt":"2026-07-22T02:21:00Z","uses":{"build:retry":1},"watchObservations":{}}`

// wantRunStorage is testRealisticRun in the storage form: the wire form pretty
// printed with two-space indents, " : " separators, and empty arrays and
// objects written across two lines.
const wantRunStorage = `{
  "adjustments" : [

  ],
  "agents" : [

  ],
  "attempts" : [
    {
      "endedAt" : "2026-07-22T02:20:00Z",
      "folder" : "01-triage-1",
      "id" : "triage-1",
      "number" : 1,
      "outcome" : "ready to implement",
      "route" : "Take the Simple route",
      "session" : "bl-demo-61-triage",
      "sessionId" : "00000000-0000-4000-8000-000000000001",
      "stageId" : "triage",
      "startedAt" : "2026-07-22T02:18:00Z",
      "tone" : "good",
      "usage" : {
        "costNanos" : 12000000,
        "coverage" : "complete",
        "input" : 100,
        "output" : 20,
        "provenance" : "estimated",
        "totalTokens" : 120
      }
    },
    {
      "agentName" : "bl-build",
      "folder" : "02-build-1",
      "id" : "build-1",
      "logMark" : 2,
      "number" : 1,
      "session" : "bl-demo-61-build",
      "sessionId" : "00000000-0000-4000-8000-000000000001",
      "settings" : {
        "model" : "m-1",
        "runner" : "pi",
        "thinking" : "high"
      },
      "stageId" : "build",
      "startedAt" : "2026-07-22T02:21:00Z",
      "usageCheckpoint" : {
        "lines" : 4,
        "sessionId" : "00000000-0000-4000-8000-000000000001",
        "totals" : {
          "cacheRead" : 6,
          "input" : 5,
          "output" : 7,
          "reasoning" : 0,
          "total" : 18
        }
      },
      "usageMark" : 9
    }
  ],
  "base" : "develop",
  "beltId" : "bug",
  "beltName" : "Bug",
  "beltVersion" : 2,
  "createdAt" : "2026-07-22T02:18:00Z",
  "forks" : [

  ],
  "generation" : 1,
  "id" : "bug:acme/billing#142",
  "issue" : {
    "assignees" : [

    ],
    "blockers" : [

    ],
    "body" : "Add it.",
    "isOpen" : true,
    "key" : "",
    "kind" : "issue",
    "labels" : [
      "agent"
    ],
    "number" : 142,
    "repo" : "acme/billing",
    "title" : "Export",
    "url" : "https://github.com/acme/billing/issues/142"
  },
  "joins" : [

  ],
  "legs" : [
    {
      "id" : "main",
      "stageId" : "build",
      "state" : "running"
    }
  ],
  "mergeBranches" : [

  ],
  "mergesWaiting" : [

  ],
  "operation" : 2,
  "pauseTime" : [

  ],
  "policies" : {
    "commentOnHold" : false,
    "retriageOnEdit" : true,
    "stopOnClose" : true,
    "waitForBlockers" : true
  },
  "pullRequests" : [

  ],
  "repos" : [
    "acme/billing"
  ],
  "routes" : {
    "size" : "simple"
  },
  "stageId" : "build",
  "stages" : [

  ],
  "state" : "running",
  "ticketObservations" : {

  },
  "updatedAt" : "2026-07-22T02:21:00Z",
  "uses" : {
    "build:retry" : 1
  },
  "watchObservations" : {

  }
}`

// TestRunEncodesExactBytes checks the wire and storage encodings of a realistic
// run byte for byte.
func TestRunEncodesExactBytes(t *testing.T) {
	t.Parallel()
	run := testRealisticRun(t)
	wire, err := jsonx.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, wire, wantRunWire) {
		t.Fatalf("wire form differs:\n got %s\nwant %s", wire, wantRunWire)
	}
	storage, err := jsonx.MarshalStorage(run)
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, storage, wantRunStorage) {
		t.Fatalf("storage form differs:\n got %s\nwant %s", storage, wantRunStorage)
	}
}

// TestRouteSetEncodesEmptyAsObject checks that an empty route set is written as
// {}, as the goldens show, and that a fork with two routes is written as a list.
func TestRouteSetEncodesEmptyAsObject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		set  RouteSet
		want string
	}{
		{"empty", RouteSet{}, `{}`},
		{"one route", RouteSet{Routes: map[string][]string{"size": {"simple"}}}, `{"size":"simple"}`},
		{"two routes", RouteSet{Routes: map[string][]string{"size": {"simple", "complex"}}}, `{"size":["simple","complex"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := jsonx.Marshal(tc.set)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
