// The
// deterministic draft (draft), the saved draft (saveDraft), the stage the stage-add and feed-into
// requests carry (addedStage) and every draft operation's request (draftRequests), with the
// endpoint each one is sent to.
package servertest

import (
	"fmt"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// contractDraftModel is the model every stage of the contract drafts names.
const contractDraftModel = "openrouter/stealth/space-bunny-alpha"

// contractStage is the `stage(…)` helper of the contract: an agent stage runs on pi, every stage
// is read-only and writes "<id>.md", and the agent's key is the one given.
func contractStage(id, name string, typ core.StageType, route *string, agent *core.AgentRef, key *string,
	outcomes []core.Outcome) core.Stage {
	var runner *core.Backend
	if agent != nil {
		runner = new(core.BackendPi)
	}
	workspace := core.WorkspaceReadOnly
	return core.NewStage(core.StageArgs{
		ID: id, Type: typ, Name: new(name), Agent: agent, Key: key, Runner: runner,
		Model: contractDraftModel, Workspace: &workspace, Writes: new(id + ".md"),
		Outcomes: outcomes, Route: route,
	})
}

// contractOutcome is one outcome with its stable id, name, when, tone and action.
func contractOutcome(id, name, when string, tone core.Tone, action core.ThenAction) core.Outcome {
	return core.Outcome{ID: id, Name: name, When: when, Tone: tone, Then: core.Then{Action: action}}
}

// contractDoneAndHold are the two outcomes most contract stages carry: done goes next, anything else holds.
func contractDoneAndHold(prefix string) []core.Outcome {
	return []core.Outcome{
		contractOutcome(prefix+"-done", "done", "The work is done", core.ToneGood, core.ThenActionNext),
		contractOutcome(prefix+"-hold", "needs me", "Anything else", core.ToneYou, core.ThenActionHold),
	}
}

// contractIssueTrigger is `.issues(IssueFilter(label:))`.
func contractIssueTrigger(label string) core.Trigger {
	filter := core.DefaultIssueFilter()
	filter.Label = label
	return core.Trigger{Kind: core.TriggerKindIssues, Issues: &filter}
}

// ContractDraft is the draft of the contract: an issues trigger labelled agent on acme/app
// four stages (approve, build on the fast route, check on the slow route, verify) and the split fork.
func ContractDraft() core.Belt {
	approve := contractStage("approve", "Approval", core.StageTypeApprove, nil, nil, nil, []core.Outcome{
		contractOutcome("approve-ok", "approved", "Approve", core.ToneGood, core.ThenActionNext),
		contractOutcome("approve-park", "parked", "Park", core.ToneNeutral, core.ThenActionPark),
	})
	build := contractStage("build", "Build", core.StageTypeAgent, new("fast"),
		&core.AgentRef{ID: "build", Version: 1}, new("build"), contractDoneAndHold("build"))
	check := contractStage("check", "Check", core.StageTypeAgent, new("slow"),
		&core.AgentRef{ID: "verify", Version: 1}, new("check"), contractDoneAndHold("check"))
	verify := contractStage("verify", "Verify", core.StageTypeAgent, nil,
		&core.AgentRef{ID: "verify", Version: 1}, new("verify"), contractDoneAndHold("verify"))
	belt := core.DefaultBelt("contract-draft", "Contract draft", []core.TriggerEntry{
		{ID: core.TriggerEntryLegacyID, Trigger: contractIssueTrigger("agent"), Repos: []string{"acme/app"}},
	}, []core.Stage{approve, build, check, verify})
	belt.Version = 0
	belt.Forks = []core.Fork{{ID: "split", Routes: []core.Route{{ID: "fast", Name: "Fast"}, {ID: "slow", Name: "Slow"}}}}
	return belt
}

// ContractSaveDraft is the saveDraft of the contract: the standard belt for acme/app under the
// id the write contract uses, with stable outcome ids and a custom Build outcome list.
func ContractSaveDraft() core.Belt {
	belt := core.SampleBelt([]string{"acme/app"}, "agent", core.SampleAgents(), true)
	belt.ID = "contract-draft"
	belt.Name = "Contract draft"
	belt.Version = 0
	outcome := 0
	for stageIndex := range belt.Stages {
		for outcomeIndex := range belt.Stages[stageIndex].Outcomes {
			outcome++
			belt.Stages[stageIndex].Outcomes[outcomeIndex].ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", outcome)
		}
	}
	for index := range belt.Stages {
		if belt.Stages[index].ID == "build" {
			belt.Stages[index].Outcomes = []core.Outcome{
				contractOutcome("build-done", "done", "The work is done", core.ToneGood, core.ThenActionNext),
				contractOutcome("build-hold", "needs me", "Anything else", core.ToneYou, core.ThenActionHold),
			}
		}
	}
	return belt
}

// ContractAddedStage is the addedStage of the contract: a review agent stage with a done and a hold outcome.
func ContractAddedStage() core.Stage {
	return core.NewStage(core.StageArgs{
		ID: "review2", Type: core.StageTypeAgent, Name: new("Review"),
		Agent: &core.AgentRef{ID: "review", Version: 1}, Key: new("review"),
		Runner: new(core.BackendPi), Model: contractDraftModel,
		Outcomes: []core.Outcome{
			contractOutcome("review2-done", "done", "The review is done", core.ToneGood, core.ThenActionNext),
			contractOutcome("review2-hold", "needs me", "Anything else", core.ToneYou, core.ThenActionHold),
		},
	})
}

// contractPoliciesWithoutHoldComment is Policies(commentOnHold: false): the other policies keep their defaults.
func contractPoliciesWithoutHoldComment() *core.Policies {
	policies := core.DefaultPolicies()
	policies.CommentOnHold = false
	return &policies
}

// ContractDraftRequest is one draft operation the contract sends: its case, the path it is posted to
// and the request body.
type ContractDraftRequest struct {
	Case    string
	Path    string
	Request core.BeltDraftRequest
}

// ContractDraftRequests is the draftRequests of the contract, in its order.
func ContractDraftRequests() []ContractDraftRequest {
	base := ContractDraft()
	update := core.StageEdit{Name: new("Renamed build"), Slots: new(3), Outcomes: []core.Outcome{
		contractOutcome("build-done", "done", "The work is done", core.ToneGood, core.ThenActionNext),
	}}
	moveBelt := ContractDraft()
	moveBelt.Triggers = append(moveBelt.Triggers, core.TriggerEntry{ID: "2", Trigger: core.Trigger{Kind: core.TriggerKindManual}, Repos: []string{"acme/app"}})
	stash := core.TriggerStash{Entry: core.TriggerEntryLegacyID}
	stash.Put(core.TriggerKindIssues, contractIssueTrigger("agent"))
	legacy := core.TriggerEntryLegacyID
	requests := []ContractDraftRequest{
		{"beltsDraftStageAdd", "stage-add", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationStageAdd,
			Stage: new(ContractAddedStage()), Index: new(1)}},
		{"beltsDraftStageRemove", "stage-remove", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationStageRemove,
			StageID: new("check")}},
		{"beltsDraftStageMove", "stage-move", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationStageMove,
			StageID: new("build"), TargetID: new("verify")}},
		{"beltsDraftStageUpdate", "stage-update", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationStageUpdate,
			StageID: new("build"), Update: &update, HelpFor: new("build")}},
		{"beltsDraftForkAdd", "fork-add", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationForkAdd,
			StageID: new("verify")}},
		{"beltsDraftForkRemove", "fork-remove", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationForkRemove,
			ForkID: new("split")}},
		{"beltsDraftRouteAdd", "route-add", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationRouteAdd,
			ForkID: new("split")}},
		{"beltsDraftRouteRemove", "route-remove", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationRouteRemove,
			RouteID: new("slow")}},
		{"beltsDraftRouteUpdate", "route-update", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationRouteUpdate,
			RouteID: new("fast"), RouteName: new("Express")}},
		{"beltsDraftTriggerAdd", "trigger-add", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationTriggerAdd,
			Entry: &core.TriggerEntry{ID: "2", Trigger: core.Trigger{Kind: core.TriggerKindManual}, Repos: []string{"acme/app"}}}},
		{"beltsDraftTriggerRemove", "trigger-remove", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationTriggerRemove,
			EntryID: &legacy}},
		{"beltsDraftTriggerMove", "trigger-move", core.BeltDraftRequest{Draft: moveBelt, Op: core.BeltDraftOperationTriggerMove,
			EntryID: new("2"), Offset: new(-1)}},
		{"beltsDraftTriggerUpdate", "trigger-update", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationTriggerUpdate,
			EntryID: &legacy, Trigger: new(contractIssueTrigger("other"))}},
		{"beltsDraftTriggerKind", "trigger-kind", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationTriggerKind,
			EntryID: &legacy, Kind: new(core.TriggerKindSchedule), Stash: &stash}},
		{"beltsDraftRepositoryAdd", "repository-add", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationRepositoryAdd,
			EntryID: &legacy, Repository: new("https://github.com/acme/two/")}},
		{"beltsDraftRepositoryRemove", "repository-remove", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationRepositoryRemove,
			EntryID: &legacy, Repository: new("acme/app")}},
		{"beltsDraftFeedInto", "feed-into", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationFeedInto,
			Stage: new(ContractAddedStage()), StageID: new("verify")}},
		{"beltsDraftPoliciesUpdate", "policies-update", core.BeltDraftRequest{Draft: base, Op: core.BeltDraftOperationPoliciesUpdate,
			Policies: contractPoliciesWithoutHoldComment()}},
	}
	for i := range requests {
		requests[i].Path = "/api/v1/belts/draft/" + string(requests[i].Request.Op)
	}
	return requests
}
