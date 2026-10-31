// Contract cases for the belt endpoints: the belt shape checks, placeholders and belt context, and
// every draft operation. Each case runs on a fresh fixture; the command cases replay one sequence on
// one fixture.

package servertest

import (
	"net/url"
	"slices"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// beltContractCases are the belt area's contract cases, in registry order.
var beltContractCases = []string{
	"belts", "belt", "placeholders",
	"beltsDraftStageAdd", "beltsDraftStageRemove", "beltsDraftStageMove", "beltsDraftStageUpdate",
	"beltsDraftForkAdd", "beltsDraftForkRemove", "beltsDraftRouteAdd", "beltsDraftRouteRemove", "beltsDraftRouteUpdate",
	"beltsDraftTriggerAdd", "beltsDraftTriggerRemove", "beltsDraftTriggerMove", "beltsDraftTriggerUpdate",
	"beltsDraftTriggerKind", "beltsDraftRepositoryAdd", "beltsDraftRepositoryRemove", "beltsDraftFeedInto",
	"beltsDraftPoliciesUpdate",
	"beltStageTemplate", "beltContext",
	"beltNew", "beltCopy", "beltSave", "beltUpdateStage", "beltUpdateAllStages",
	"beltArchive", "beltUnarchive", "beltPause", "beltAfk", "beltShadowSave", "beltShadowAccept", "beltShadowDiscard",
}

// beltCommandCases are the cases whose replies come from the command sequence.
var beltCommandCases = map[string]bool{
	"beltNew": true, "beltCopy": true, "beltSave": true, "beltUpdateStage": true, "beltUpdateAllStages": true,
	"beltArchive": true, "beltUnarchive": true, "beltPause": true, "beltAfk": true,
	"beltShadowSave": true, "beltShadowAccept": true, "beltShadowDiscard": true,
}

func init() {
	registerArea(beltContractCases, exerciseBeltContract)
}

// exerciseBeltContract serves one belt case on a fresh fixture and checks it against its golden.
// It reports false for a case of another area.
func exerciseBeltContract(f *Fixture, t testing.TB, contractCase string) bool {
	if !slices.Contains(beltContractCases, contractCase) {
		return false
	}
	switch {
	case contractCase == "belts":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/belts", Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		checkBeltsShape(t, g, reply.Body)
	case contractCase == "belt":
		g := Start(t, Options{})
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/belts/" + url.PathEscape(g.Coordinator.ReadBelts()[0].Belt.ID), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		checkBeltShape(t, reply.Body)
	case contractCase == "placeholders":
		g := Start(t, Options{})
		id := g.Coordinator.ReadBelts()[0].Belt.ID
		reply := g.Do(Request{Method: "GET", Path: "/api/v1/placeholders?belt=" + url.QueryEscape(id), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		entries := contractMember(t, contractTree(t, reply.Body), "placeholders")
		if values := contractMember(t, entries, "entries"); len(values.([]any)) == 0 {
			t.Fatal("placeholders: no entries")
		}
	case contractCase == "beltStageTemplate":
		g := Start(t, Options{})
		request := api.NewBeltStageTemplateRequest(ContractDraft(), core.StageTypeAgent, &core.AgentRef{ID: "verify", Version: 1}, nil, nil)
		reply := g.Do(Request{Method: "POST", Path: contractEndpoint(t, contractCase).Path, Body: contractBody(t, request), Token: ContractToken})
		CheckResponse(t, reply, ExpectJSON)
		checkStageTemplate(t, g, reply.Body)
	case contractCase == "beltContext":
		g := Start(t, Options{})
		request := api.NewBeltContextRequest(ContractDraft(), new("check"))
		reply := g.Do(Request{Method: "POST", Path: contractEndpoint(t, contractCase).Path, Body: contractBody(t, request), Token: ContractToken})
		CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, g.Root)
		if actions := contractMember(t, contractTree(t, reply.Body), "actions").([]any); len(actions) == 0 {
			t.Fatal("beltContext: no actions")
		}
	case contractIsDraftOperation(contractCase):
		// Draft operations change nothing on disk, so one fixture serves them all.
		seq := contractSequence(t, "belt-drafts", beltDraftReplies)
		checkSequenceReply(t, seq, contractCase)
	case beltCommandCases[contractCase]:
		seq := contractSequence(t, "belt-commands", beltCommandReplies)
		checkSequenceReply(t, seq, contractCase)
	}
	return true
}

// contractIsDraftOperation reports whether a case is one of the draft operations of ContractDraftRequests.
func contractIsDraftOperation(contractCase string) bool {
	return slices.ContainsFunc(ContractDraftRequests(), func(draft ContractDraftRequest) bool {
		return draft.Case == contractCase
	})
}

// checkSequenceReply checks the reply a sequence recorded for contractCase against its golden.
func checkSequenceReply(t testing.TB, seq *contractReplies, contractCase string) {
	t.Helper()
	reply, ok := seq.replies[contractCase]
	if !ok {
		t.Fatalf("%s: its sequence has no reply", contractCase)
	}
	CheckGolden(t, contractCase, contractEndpoint(t, contractCase).GoldenName, reply, seq.root)
}

// beltDraftReplies runs every draft operation of ContractDraftRequests on one fixture and returns each
// reply by its case. The operations only edit drafts, so the order does not change any reply.
func beltDraftReplies(t testing.TB) (*Fixture, map[string]Reply) {
	t.Helper()
	g := Start(t, Options{})
	replies := map[string]Reply{}
	for _, draft := range ContractDraftRequests() {
		replies[draft.Case] = g.Do(Request{Method: "POST", Path: draft.Path, Body: contractBody(t, draft.Request), Token: ContractToken})
	}
	return g, replies
}

// beltCommandReplies replays the belt command contract (beltCommandsReturnSavedResourcesAndGoldens) on
// a fresh fixture and returns each command's reply by its case. The belt archive, pause and AFK
// commands need the belt saved first, and the update commands need a newer Build version, so the
// sequence runs in the test's order.
func beltCommandReplies(t testing.TB) (*Fixture, map[string]Reply) {
	t.Helper()
	g := Start(t, Options{})
	replies := map[string]Reply{}
	post := func(contractCase, path string, body []byte) {
		replies[contractCase] = g.Do(Request{Method: "POST", Path: path, Body: body, Token: ContractToken})
	}
	post("beltNew", "/api/v1/belts/new", nil)
	draft := ContractSaveDraft()
	post("beltCopy", "/api/v1/belts/copy", contractBody(t, api.BeltDraftOnlyRequest{Draft: draft}))
	post("beltSave", "/api/v1/belts/save", contractBody(t, api.BeltSaveRequest{Belt: draft}))
	post("beltArchive", "/api/v1/belts/contract-draft/archive", nil)
	post("beltUnarchive", "/api/v1/belts/contract-draft/unarchive", nil)
	post("beltPause", "/api/v1/belts/contract-draft/pause", contractBody(t, api.BeltPauseRequest{Paused: true}))
	// Shadow tests are off until Settings turns them on.
	enabled := true
	if _, err := g.Coordinator.ApplySettingsPatch(coordinator.SettingsPatch{EnableShadowTests: &enabled}); err != nil {
		t.Fatalf("enable shadow tests: %v", err)
	}
	// A newer Build version makes the saved draft's pin outdated.
	record := g.Coordinator.LatestAgent("build")
	if record == nil {
		t.Fatal("no build agent in the contract demo")
	}
	newer := record.Agent
	newer.Brief = "A contract build."
	if _, err := g.Coordinator.SaveAgent(newer); err != nil {
		t.Fatalf("save newer build: %v", err)
	}
	post("beltUpdateStage", "/api/v1/belts/update-stage", contractBody(t, api.NewBeltUpdateStageRequest(draft, new("build"), nil)))
	post("beltUpdateAllStages", "/api/v1/belts/update-all-stages", contractBody(t, api.BeltDraftOnlyRequest{Draft: draft}))
	post("beltAfk", "/api/v1/belts/contract-draft/afk", contractBody(t, api.BeltAfkRequest{Afk: core.AfkModeAlways}))
	// A shadow that renames its first stage starts a test, which accept ends; a second test is discarded.
	shadow := draft.Clone()
	shadow.Stages[0].Name = "Shadow " + shadow.Stages[0].Name
	post("beltShadowSave", "/api/v1/belts/contract-draft/shadow/save", contractBody(t, api.BeltSaveRequest{Belt: shadow}))
	post("beltShadowAccept", "/api/v1/belts/contract-draft/shadow/accept", nil)
	g.Do(Request{Method: "POST", Path: "/api/v1/belts/contract-draft/shadow/save", Body: contractBody(t, api.BeltSaveRequest{Belt: shadow}), Token: ContractToken})
	post("beltShadowDiscard", "/api/v1/belts/contract-draft/shadow/discard", nil)
	return g, replies
}

// checkBeltsShape checks the `.belts` assertions: the listed rows match the coordinator's
// belts, each carries its actions and revision outside the belt, and the browser is the daemon's.
func checkBeltsShape(t testing.TB, g *Fixture, body []byte) {
	t.Helper()
	tree := contractTree(t, body)
	rows := contractMember(t, tree, "belts").([]any)
	if len(rows) != len(g.Coordinator.ReadBelts()) {
		t.Fatalf("belts: %d rows, want %d", len(rows), len(g.Coordinator.ReadBelts()))
	}
	contractSameAsValue(t, "belts browser", contractMember(t, tree, "browser"), g.Coordinator.ReadBeltBrowser("", "all"))
	var archivedAndPaused, pausedLive, fired, unfired bool
	for _, row := range rows {
		record := contractMember(t, row, "draft").(map[string]any)
		if _, ok := record["actions"]; ok {
			t.Fatal("belts: a belt definition carries its actions")
		}
		if actions, _ := contractMember(t, row, "actions").([]any); len(actions) == 0 {
			t.Fatal("belts: a row has no actions")
		}
		if revision, _ := contractMember(t, row, "revision").(string); revision == "" {
			t.Fatal("belts: a row has no revision")
		}
		archived := contractMember(t, row, "archived").(bool)
		paused := contractMember(t, row, "paused").(bool)
		effective := contractMember(t, row, "effectivelyPaused").(bool)
		archivedAndPaused = archivedAndPaused || (archived && effective)
		pausedLive = pausedLive || (paused && !archived && effective)
		fired = fired || contractMember(t, row, "lastFired") != nil
		unfired = unfired || contractMember(t, row, "lastFired") == nil
	}
	if !archivedAndPaused || !pausedLive || !fired || !unfired {
		t.Fatalf("belts: the demo lacks an archived, a paused, a fired or an unfired belt (%v %v %v %v)",
			archivedAndPaused, pausedLive, fired, unfired)
	}
}

// checkBeltShape checks the `.belt` assertions: the definition carries no actions, while
// the resource has its actions and a revision.
func checkBeltShape(t testing.TB, body []byte) {
	t.Helper()
	tree := contractTree(t, body)
	if _, ok := contractMember(t, tree, "draft").(map[string]any)["actions"]; ok {
		t.Fatal("belt: the definition carries its actions")
	}
	if actions, _ := contractMember(t, tree, "actions").([]any); len(actions) == 0 {
		t.Fatal("belt: no actions")
	}
	if revision, _ := contractMember(t, tree, "revision").(string); revision == "" {
		t.Fatal("belt: no revision")
	}
}

// checkStageTemplate renumbers the new stage's outcome ids (they are random) and compares the stage with
// its golden, as the belt stage template test does.
func checkStageTemplate(t testing.TB, g *Fixture, body []byte) {
	t.Helper()
	var response api.BeltStageTemplateResponse
	if err := decodeContract(body, &response); err != nil {
		t.Fatalf("decode stage template: %v", err)
	}
	for index := range response.Stage.Outcomes {
		response.Stage.Outcomes[index].ID = contractOutcomeID(index + 1)
	}
	CheckBody(t, "beltStageTemplate", contractEndpoint(t, "beltStageTemplate").GoldenName,
		contractBody(t, api.BeltStageTemplateResponse{Stage: response.Stage}), g.Root)
}
