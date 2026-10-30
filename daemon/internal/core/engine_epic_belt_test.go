// Engine and validation tests of the epic belt.

package core

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TestABaseLabelSetsTheRunsBase
func TestABaseLabelSetsTheRunsBase(t *testing.T) {
	t.Parallel()
	ticket := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-1", Title: "T", Labels: []string{"agent", "Base:beltline/PAY-40", "base:"}, Blockers: []string{}}
	if hint := ticket.BaseHint(); hint == nil || *hint != "beltline/PAY-40" {
		t.Fatalf("base hint = %v, want beltline/PAY-40", hint)
	}
	approval := NewStage(StageArgs{ID: "approve", Type: StageTypeApprove, Outcomes: []Outcome{}})
	query := JiraQuery{Jql: "x"}
	belt := NewBeltFromTrigger(BeltFromTriggerArgs{ID: "j", Name: "J", Trigger: Trigger{Kind: TriggerKindJira, Jira: &query},
		Repos: []string{"acme/billing", "acme/web"}, Base: "main", Stages: []Stage{approval}})
	run := NewRun(RunArgs{Belt: belt, Subject: ticket, Now: testT0})
	// The label is copied when the run starts, so it outranks the belt's shared base.
	if got := run.RequestedBase("acme/billing"); got == nil || *got != "beltline/PAY-40" {
		t.Fatalf("requested base = %v, want beltline/PAY-40", got)
	}
	if run.Base != "beltline/PAY-40" || run.BaseFromLabel == nil || !*run.BaseFromLabel {
		t.Fatalf("run base = %q (from label %v), want the label", run.Base, run.BaseFromLabel)
	}
	if !reflect.DeepEqual(run.Repos, []string{"acme/billing", "acme/web"}) {
		t.Fatalf("run repos = %v", run.Repos)
	}
	plain := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-2", Title: "T", Blockers: []string{}}
	if got := NewRun(RunArgs{Belt: belt, Subject: plain, Now: testT0}).RequestedBase("acme/billing"); got == nil || *got != "main" {
		t.Fatalf("requested base without a label = %v, want main", got)
	}
}

// TestOldPoliciesAndStagesLoadAsBefore
func TestOldPoliciesAndStagesLoadAsBefore(t *testing.T) {
	t.Parallel()
	var policies Policies
	if err := json.Unmarshal([]byte(`{"retriageOnEdit":false,"stopOnClose":true,"commentOnHold":true}`), &policies); err != nil {
		t.Fatal(err)
	}
	want := Policies{RetriageOnEdit: false, StopOnClose: true, CommentOnHold: true, WaitForBlockers: true}
	if !reflect.DeepEqual(policies, want) {
		t.Fatalf("policies = %+v, want %+v", policies, want)
	}
	watch := DefaultsStageAt(StageTypeWatch, DefaultsPlace{}, nil)
	if target := watch.WatchTarget(); target == nil || *target != WatchTargetPullRequest {
		t.Fatalf("a new watch waits on %v, want the pull request", target)
	}
	saved, err := jsonx.Marshal(watch)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), `"watch":`) {
		t.Fatalf("a pull request watch saves its target: %s", saved)
	}
	ticket := NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Name: new("Watch the ticket"),
		Outcomes: DefaultsTicketWatchOutcomes(), Watch: new(WatchTargetTicket)})
	data, err := jsonx.Marshal(ticket)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded Stage
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if target := reloaded.WatchTarget(); target == nil || *target != WatchTargetTicket {
		t.Fatalf("reloaded watch waits on %v, want the ticket", target)
	}
	needs := reloaded.Needs()
	if _, ok := needs[InputTicket]; !ok || len(needs) != 1 {
		t.Fatalf("needs = %v, want only the ticket", needs)
	}
}

// epicTestEpic is a Jira epic the epic belt works on.
func epicTestEpic() Subject {
	return Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-40", Title: "Split invoicing", Labels: []string{"epic"}, IsOpen: true, Blockers: []string{}}
}

// epicTestTicketWatch is a watch on the run's own
// ticket with outcomes (nil: the shipped ticket watch outcomes).
func epicTestTicketWatch(outcomes []Outcome) Stage {
	if outcomes == nil {
		outcomes = DefaultsTicketWatchOutcomes()
	}
	return NewStage(StageArgs{ID: "watch", Type: StageTypeWatch, Name: new("Watch the ticket"),
		Outcomes: outcomes, Watch: new(WatchTargetTicket)})
}

// epicTestEpicTrigger is the epic belt's default trigger: the Epic query
// the same trigger SampleEpicBelt uses.
func epicTestEpicTrigger() Trigger {
	return Trigger{Kind: TriggerKindJira, Jira: &JiraQuery{Jql: "issuetype = Epic AND labels = beltline", Poll: 60}}
}

// epicTestWatching is a run of a
// Jira belt on the ticket watch and an approval, started.
func epicTestWatching() Run {
	run := NewRun(RunArgs{Belt: triggerTestBelt(TriggerKindJira, []Stage{epicTestTicketWatch(nil), DefaultsStage(StageTypeApprove, nil, nil, nil)}),
		Subject: epicTestEpic(), Now: engineT0})
	EngineStart(&run, engineT0)
	return run
}

// TestATicketEventTakesItsOutcomeWithTheChanges
func TestATicketEventTakesItsOutcomeWithTheChanges(t *testing.T) {
	t.Parallel()
	run := epicTestWatching()
	approvalID := run.Stages[1].ID
	if effects := EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventMerged}, engineT0); len(effects) != 0 {
		t.Fatalf("a pull request event on a ticket watch made effects %v", effects)
	}
	if run.StageID != "watch" {
		t.Fatalf("stage = %q, want watch", run.StageID)
	}
	EngineAdvance(&run, Event{Kind: EventKindTicketWatched, Ticket: TicketEventChildrenChanged, Detail: new("PAY-42 finished")}, engineT0)
	if run.StageID != approvalID {
		t.Fatalf("stage = %q, want %q", run.StageID, approvalID)
	}
	if got := engineStr(run.Attempts[0].Outcome); got != "child tickets changed" {
		t.Fatalf("first outcome = %q, want child tickets changed", got)
	}
	if got := engineStr(run.Attempts[len(run.Attempts)-1].Note); got != "PAY-42 finished" {
		t.Fatalf("last note = %q, want PAY-42 finished", got)
	}
}

// TestAnEventWithoutAnOutcomeHolds
func TestAnEventWithoutAnOutcomeHolds(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsTicketWatchOutcomes()[:1]
	run := NewRun(RunArgs{Belt: triggerTestBelt(TriggerKindJira, []Stage{epicTestTicketWatch(outcomes), DefaultsStage(StageTypeApprove, nil, nil, nil)}),
		Subject: epicTestEpic(), Now: engineT0})
	EngineStart(&run, engineT0)
	EngineAdvance(&run, Event{Kind: EventKindTicketWatched, Ticket: TicketEventNewComment}, engineT0)
	if run.State != RunStateHeld {
		t.Fatalf("state = %v, want held", run.State)
	}
	if got := engineStr(run.Reason); got != TicketEventNewComment.Label() {
		t.Fatalf("reason = %q, want %q", got, TicketEventNewComment.Label())
	}
}

// TestATicketWatchNeedsARunAboutATicket
func TestATicketWatchNeedsARunAboutATicket(t *testing.T) {
	t.Parallel()
	manual := triggerTestBelt(TriggerKindManual, []Stage{epicTestTicketWatch(nil)})
	want := []string{"Watch the ticket watches the issue or ticket, but this belt's runs aren't about one."}
	if got := validationInputs(manual); !reflect.DeepEqual(got, want) {
		t.Fatalf("manual warnings = %v, want %v", got, want)
	}
	if got := validationInputs(triggerTestBelt(TriggerKindJira, []Stage{epicTestTicketWatch(nil)})); len(got) != 0 {
		t.Fatalf("jira warnings = %v, want none", got)
	}
	if got := validationInputs(triggerTestBelt(TriggerKindIssues, []Stage{epicTestTicketWatch(nil)})); len(got) != 0 {
		t.Fatalf("issues warnings = %v, want none", got)
	}
}

// TestTheEpicBeltIsValid
func TestTheEpicBeltIsValid(t *testing.T) {
	t.Parallel()
	belt := SampleEpicBelt(epicTestEpicTrigger(), []string{"acme/billing"}, SampleAgents())
	agents := map[AgentRef]Agent{}
	for _, agent := range SampleAgents() {
		agents[agent.Ref()] = agent
	}
	var keys []string
	for _, stage := range belt.Stages {
		if stage.Agent != nil {
			if agent, ok := agents[*stage.Agent]; ok {
				keys = append(keys, agent.Key)
			} else {
				keys = append(keys, stage.Agent.ID)
			}
		}
	}
	for _, stage := range belt.Stages {
		if stage.Type == StageTypeAgent {
			keys = append(keys, stage.ID)
		}
	}
	if warnings := ValidationWarnings(belt, keys, agents); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	var ids []string
	for _, stage := range belt.Stages {
		ids = append(ids, stage.ID)
	}
	wantIDs := []string{"plan-epic", "approve-plan", "open-branches", "file-wave", "watch", "assess-epic", "replan-epic",
		"approve-wave", "land-epic", "merge", "wrap-up"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("stage ids = %v, want %v", ids, wantIDs)
	}
	outcomes := func(id string) []string {
		var got []string
		for _, outcome := range belt.Stage(id).Outcomes {
			route := ""
			if outcome.Then.Route() != nil {
				route = *outcome.Then.Route()
			}
			got = append(got, fmt.Sprintf("%s %s %s%s", outcome.Name, outcome.Then.Action, strings.Join(outcome.Then.To, ""), route))
		}
		return got
	}
	if got, want := outcomes("watch"), []string{"child tickets changed next ", "commented next ", "closed drop "}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watch outcomes = %q, want %q", got, want)
	}
	if got, want := outcomes("assess-epic"), []string{"wait goto watch", "wave done route next-wave", "epic done route land", "stuck hold "}; !reflect.DeepEqual(got, want) {
		t.Fatalf("assess outcomes = %q, want %q", got, want)
	}
	if got, want := outcomes("replan-epic"), []string{"next wave next ", "nothing left goto assess-epic", "open questions hold "}; !reflect.DeepEqual(got, want) {
		t.Fatalf("replan outcomes = %q, want %q", got, want)
	}
	if got := outcomes("approve-wave"); len(got) == 0 || got[0] != "approved goto file-wave" {
		t.Fatalf("approve-wave outcomes = %q, want first approved goto file-wave", got)
	}
	var routes []string
	for _, stage := range belt.Stages {
		route := ValueOr(stage.Route, "-")
		routes = append(routes, route)
	}
	if want := []string{"-", "-", "-", "-", "-", "-", "next-wave", "next-wave", "land", "land", "land"}; !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %v, want %v", routes, want)
	}
	if belt.Policies.RetriageOnEdit {
		t.Fatalf("the epic belt retriages on edit, want it off")
	}
}

// epicTestAssessing is the epic belt's run,
// at Assess with what the watch saw.
func epicTestAssessing() Run {
	run := NewRun(RunArgs{Belt: SampleEpicBelt(epicTestEpicTrigger(), []string{"acme/billing"}, SampleAgents()), Subject: epicTestEpic(), Now: engineT0})
	run.State = RunStateRunning
	run.StageID = "assess-epic"
	run.Attempts = []Attempt{NewAttempt(AttemptArgs{StageID: "assess-epic", Number: 1, StartedAt: jsonx.Date{Time: engineT0}, Folder: "06-assess-epic-1"})}
	return run
}

// epicTestFinish ends the run's session with the Outcome line outcome.
func epicTestFinish(run *Run, outcome string) {
	EngineAdvance(run, Event{Kind: EventKindSessionFinished, Output: new("Done.\n\nOutcome: " + outcome)}, engineT0)
}

// TestADoneWaveTakesTheNextWaveRoute
func TestADoneWaveTakesTheNextWaveRoute(t *testing.T) {
	t.Parallel()
	run := epicTestAssessing()
	epicTestFinish(&run, "wave done")
	if run.StageID != "replan-epic" {
		t.Fatalf("stage = %q, want replan-epic", run.StageID)
	}
	if got := run.Routes()["after-assess"]; !reflect.DeepEqual(got, []string{"next-wave"}) {
		t.Fatalf("after-assess route = %v, want [next-wave]", got)
	}
}

// TestADoneEpicTakesTheLandRoute
func TestADoneEpicTakesTheLandRoute(t *testing.T) {
	t.Parallel()
	run := epicTestAssessing()
	epicTestFinish(&run, "epic done")
	if run.StageID != "land-epic" {
		t.Fatalf("stage = %q, want land-epic", run.StageID)
	}
	if got := run.Routes()["after-assess"]; !reflect.DeepEqual(got, []string{"land"}) {
		t.Fatalf("after-assess route = %v, want [land]", got)
	}
}

// TestAnEpicThatLandsShipsWhenWrapUpIsDone checks the epic's end: the pull requests open, the merge
// watch sees them merged, and Wrap up's done outcome ships the epic, while a blocked Wrap up holds it.
func TestAnEpicThatLandsShipsWhenWrapUpIsDone(t *testing.T) {
	t.Parallel()
	landed := func(t *testing.T, outcome string) Run {
		t.Helper()
		run := epicTestAssessing()
		epicTestFinish(&run, "epic done")
		EngineAdvance(&run, engineSessionStart("07-land-epic-1"), engineT0)
		epicTestFinish(&run, "opened")
		if run.StageID != "merge" {
			t.Fatalf("stage = %q, want merge", run.StageID)
		}
		EngineAdvance(&run, Event{Kind: EventKindWatched, Watch: WatchEventMerged, Detail: new("#7")}, engineT0)
		if run.StageID != "wrap-up" {
			t.Fatalf("stage = %q, want wrap-up", run.StageID)
		}
		EngineAdvance(&run, engineSessionStart("08-wrap-up-1"), engineT0)
		epicTestFinish(&run, outcome)
		return run
	}
	done := landed(t, "done")
	if done.State != RunStateShipped || !done.Shipping {
		t.Errorf("done: state %q shipping %t, want shipped", done.State, done.Shipping)
	}
	if blocked := landed(t, "blocked"); blocked.State != RunStateHeld || blocked.Shipping {
		t.Errorf("blocked: state %q shipping %t, want held and not shipped", blocked.State, blocked.Shipping)
	}
}

// TestWaitingGoesBackToTheWatch
func TestWaitingGoesBackToTheWatch(t *testing.T) {
	t.Parallel()
	run := epicTestAssessing()
	epicTestFinish(&run, "wait")
	if run.StageID != "watch" {
		t.Fatalf("stage = %q, want watch", run.StageID)
	}
}
