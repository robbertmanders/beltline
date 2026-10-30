// Validation tests of multi-trigger belts, including the model checks.

package core

import (
	"slices"
	"strings"
	"testing"
)

// vtBeltOfKind is a belt with an id and stages: a
// belt of kind with its repositories and default stages and forks, or the given
// stages with no forks.
func vtBeltOfKind(kind TriggerKind, stages []Stage) Belt {
	var trigger Trigger
	var repos []string
	switch kind {
	case TriggerKindIssues:
		filter := DefaultIssueFilter()
		filter.Label = "agent"
		trigger, repos = Trigger{Kind: kind, Issues: &filter}, []string{"acme/billing"}
	case TriggerKindReviews:
		filter := DefaultReviewFilter()
		trigger, repos = Trigger{Kind: kind, Reviews: &filter}, []string{"acme/web"}
	case TriggerKindJira:
		query := DefaultJiraQuery()
		query.Jql = "project = PAY"
		trigger, repos = Trigger{Kind: kind, Jira: &query}, []string{"acme/billing"}
	case TriggerKindSchedule:
		task := DefaultScheduledTask()
		task.Schedule.Days = ScheduleDaysWeekdays
		task.Schedule.Hour = 9
		task.Title = "Sweep flaky tests"
		task.Task = "Fix or quarantine them."
		trigger, repos = Trigger{Kind: kind, Schedule: &task}, []string{"acme/platform"}
	default:
		trigger, repos = Trigger{Kind: TriggerKindManual}, []string{"acme/web", "acme/billing"}
	}
	var forks []Fork
	if stages == nil {
		stages = SampleStages(kind, SampleAgents(), true)
		forks = SampleForks(kind)
	}
	return NewBeltFromTrigger(BeltFromTriggerArgs{
		ID: string(kind), Name: kind.Title(), Trigger: trigger, Repos: repos, Stages: stages, Forks: forks,
	})
}

// vtMultiBelt is a belt with several triggers, stages and forks, with the
// defaults: version 1, default policies and the global AFK mode.
func vtMultiBelt(id, name string, triggers []TriggerEntry, stages []Stage, forks []Fork) Belt {
	policies := DefaultPolicies()
	afk := AfkModeGlobal
	return Belt{
		ID: id, Name: name, Version: 1, Triggers: triggers, Policies: policies,
		Stages: stages, Forks: forks, Afk: afk,
	}
}

// vtEntry is a trigger entry with id, trigger and repositories.
func vtEntry(id string, trigger Trigger, repos ...string) TriggerEntry {
	return TriggerEntry{ID: id, Trigger: trigger, Repos: repos}
}

// vtIssuesTrigger is the issues trigger with the agent label.
func vtIssuesTrigger() Trigger {
	filter := DefaultIssueFilter()
	filter.Label = "agent"
	return Trigger{Kind: TriggerKindIssues, Issues: &filter}
}

// vtReviewsTrigger is the review trigger with default filters.
func vtReviewsTrigger() Trigger {
	filter := DefaultReviewFilter()
	return Trigger{Kind: TriggerKindReviews, Reviews: &filter}
}

// vtJiraTrigger is the Jira trigger with a JQL query.
func vtJiraTrigger(jql string) Trigger {
	query := DefaultJiraQuery()
	query.Jql = jql
	return Trigger{Kind: TriggerKindJira, Jira: &query}
}

// vtScheduleTrigger is the schedule trigger with a task.
func vtScheduleTrigger(task ScheduledTask) Trigger {
	return Trigger{Kind: TriggerKindSchedule, Schedule: &task}
}

// TestEveryKindsStartingBeltHasNoWarnings
func TestEveryKindsStartingBeltHasNoWarnings(t *testing.T) {
	t.Parallel()
	for _, kind := range AllTriggerKind {
		if got := vtWarnings(vtBeltOfKind(kind, nil)); len(got) != 0 {
			t.Errorf("%s: warnings %v", kind, got)
		}
	}
}

// TestAReviewBeltDoesNotChangeCode
func TestAReviewBeltDoesNotChangeCode(t *testing.T) {
	t.Parallel()
	review := vtBeltOfKind(TriggerKindReviews, nil)
	review.Stages[0].Workspace = WorkspaceWorktree
	if !slices.Contains(vtWarnings(review), "Review changes code, but a review works on someone else's pull request. Give it a read-only checkout.") {
		t.Fatalf("missing read-only warning in %v", vtWarnings(review))
	}
}

// TestTheReviewAgentCanBeAddedToAReviewBelt
func TestTheReviewAgentCanBeAddedToAReviewBelt(t *testing.T) {
	t.Parallel()
	postReview := DefaultsStageForAgent(vtAgent("post-review"), nil, nil, nil, TriggerKindReviews)
	review := vtBeltOfKind(TriggerKindReviews, []Stage{postReview})
	agent := vtAgent("review")
	added := DefaultsStageForAgent(agent, nil, review.Stages, review.Stages, TriggerKindReviews)
	review.Stages = append([]Stage{added}, review.Stages...)
	if review.Stages[0].Workspace != WorkspaceReadOnly {
		t.Fatalf("workspace %q, want readOnly", review.Stages[0].Workspace)
	}
	var actions []ThenAction
	for _, outcome := range review.Stages[0].Outcomes {
		actions = append(actions, outcome.Then.Action)
	}
	if len(actions) != 2 || actions[0] != ThenActionNext || actions[1] != ThenActionNext {
		t.Fatalf("actions %v, want [next next]", actions)
	}
	if got := vtWarnings(review); len(got) != 0 {
		t.Fatalf("warnings %v, want none", got)
	}
}

// TestAReviewBeltReadsThePullRequestWhateverTheWorkspace
func TestAReviewBeltReadsThePullRequestWhateverTheWorkspace(t *testing.T) {
	t.Parallel()
	review := vtBeltOfKind(TriggerKindReviews, nil)
	review.Stages[0].Workspace = WorkspaceWorktreeReadOnly
	if got := vtWarnings(review); len(got) != 0 {
		t.Fatalf("warnings %v, want none", got)
	}
}

// TestAWatchNeedsAPullRequestBeforeIt
func TestAWatchNeedsAPullRequestBeforeIt(t *testing.T) {
	t.Parallel()
	manual := vtBeltOfKind(TriggerKindManual, nil)
	watch := DefaultsStage(StageTypeWatch, nil, nil, manual.Stages)
	manual.Stages = append(manual.Stages[:1:1], append([]Stage{watch}, manual.Stages[1:]...)...)
	if !slices.Contains(vtWarnings(manual), "Watch the pull request comes before any stage that changes code, so there is no pull request to watch.") {
		t.Fatalf("missing watch warning in %v", vtWarnings(manual))
	}
}

// TestEachKindChecksItsOwnFields
func TestEachKindChecksItsOwnFields(t *testing.T) {
	t.Parallel()
	jira := vtBeltOfKind(TriggerKindJira, nil)
	jira.SetTrigger(vtJiraTrigger(" "))
	jira.SetRepos([]string{"a/b", "c/d"})
	warnings := vtWarnings(jira)
	if !slices.Contains(warnings, "Write the JQL query that picks the tickets.") {
		t.Fatalf("missing JQL warning in %v", warnings)
	}
	if TriggerKindJira.OneRepo() {
		t.Fatal("jira is one repository")
	}
	if nfContains(warnings, "repository") {
		t.Fatalf("unexpected repository warning in %v", warnings)
	}
	jira.SetRepos(nil)
	if !slices.Contains(vtWarnings(jira), "Add at least one repository to the trigger.") {
		t.Fatalf("missing repository warning in %v", vtWarnings(jira))
	}

	schedule := vtBeltOfKind(TriggerKindSchedule, nil)
	task := DefaultScheduledTask()
	task.Title = ""
	task.Task = "Fix them."
	schedule.SetTrigger(vtScheduleTrigger(task))
	schedule.SetRepos(nil)
	warnings = vtWarnings(schedule)
	if !slices.Contains(warnings, "Give the scheduled task a title.") {
		t.Fatalf("missing title warning in %v", warnings)
	}
	if !slices.Contains(warnings, "Set the repository the work happens in.") {
		t.Fatalf("missing repository warning in %v", warnings)
	}
	schedule.SetRepos([]string{"a/b", "c/d"})
	if !slices.Contains(vtWarnings(schedule), "A schedule belt works in one repository. Remove the others.") {
		t.Fatalf("missing one-repository warning in %v", vtWarnings(schedule))
	}
}

// vtMixedTriggers is three trigger entries
// (issues, issues with two repositories, Jira with two) and one approval stage.
func vtMixedTriggers() []TriggerEntry {
	issues := DefaultIssueFilter()
	issues.Label = "agent"
	issues.Poll = 30
	bugs := DefaultIssueFilter()
	bugs.Label = "bug"
	bugs.Assignee = TriggerAssigneeMe
	bugs.Poll = 300
	jira := DefaultJiraQuery()
	jira.Jql = "project = PAY"
	jira.Poll = 60
	return []TriggerEntry{
		vtEntry("1", Trigger{Kind: TriggerKindIssues, Issues: &issues}, "acme/billing"),
		vtEntry("2", Trigger{Kind: TriggerKindIssues, Issues: &bugs}, "acme/web", "acme/platform"),
		vtEntry("3", Trigger{Kind: TriggerKindJira, Jira: &jira}, "acme/billing", "acme/infra"),
	}
}

// vtMixedBelt is the belt with the mixed triggers, one approval stage,
// base develop, retriage-on-edit off and AFK always.
func vtMixedBelt() Belt {
	belt := vtMultiBelt("shared", "Shared", vtMixedTriggers(), []Stage{DefaultsStage(StageTypeApprove, nil, nil, nil)}, nil)
	belt.Base = "develop"
	belt.Policies.RetriageOnEdit = false
	belt.Afk = AfkModeAlways
	return belt
}

// TestAnEmptyListWarnsAndCannotBeSaved
func TestAnEmptyListWarnsAndCannotBeSaved(t *testing.T) {
	t.Parallel()
	belt := vtMixedBelt()
	belt.Triggers = nil
	if !slices.Contains(vtWarnings(belt), "Add at least one trigger") {
		t.Fatalf("missing trigger warning in %v", vtWarnings(belt))
	}
}

// TestOnlyAScheduleWithSeveralRepositoriesWarns
func TestOnlyAScheduleWithSeveralRepositoriesWarns(t *testing.T) {
	t.Parallel()
	belt := vtMixedBelt()
	task := DefaultScheduledTask()
	task.Title = "Sweep"
	task.Task = "Fix"
	belt.Triggers = append(belt.Triggers, vtEntry("4", vtScheduleTrigger(task), "a/b", "c/d"))
	warnings := vtWarnings(belt)
	if !vtAny(warnings, func(w string) bool {
		return strings.Contains(w, "Trigger 4 (Schedule)") && strings.Contains(w, "one repository")
	}) {
		t.Fatalf("missing schedule warning in %v", warnings)
	}
	if vtAny(warnings, func(w string) bool { return strings.Contains(w, "Trigger 3") && strings.Contains(w, "repository") }) {
		t.Fatalf("unexpected warning for trigger 3 in %v", warnings)
	}
}

// vtAny reports whether one of the strings satisfies the test.
func vtAny(list []string, test func(string) bool) bool {
	return slices.ContainsFunc(list, func(item string) bool { return test(item) })
}

// TestReviewAndManualShareReadOnlyAgentsAndRejectAWorktree
func TestReviewAndManualShareReadOnlyAgentsAndRejectAWorktree(t *testing.T) {
	t.Parallel()
	review := vtEntry("1", vtReviewsTrigger(), "acme/web")
	manual := vtEntry("2", Trigger{Kind: TriggerKindManual}, "acme/web")
	read := DefaultsStageForAgent(vtAgent("review"), nil, nil, nil, TriggerKindReviews)
	ok := vtMultiBelt("mix", "Mix", []TriggerEntry{review, manual}, []Stage{read}, nil)
	for _, warning := range vtWarnings(ok) {
		if strings.Contains(warning, "needs") {
			t.Fatalf("unexpected needs warning %q", warning)
		}
	}
	ok.Stages[0].Workspace = WorkspaceWorktree
	if !vtAny(vtWarnings(ok), func(w string) bool {
		return strings.HasPrefix(w, "Trigger 1 (Review requests):") && strings.Contains(w, "read-only")
	}) {
		t.Fatalf("missing read-only warning in %v", vtWarnings(ok))
	}
	reversed := vtMultiBelt("mix", "Mix", []TriggerEntry{manual, review}, ok.Stages, nil)
	if reversed.StageKind() != ok.StageKind() {
		t.Fatalf("stage kind %q, want %q", reversed.StageKind(), ok.StageKind())
	}
	if !vtAny(vtWarnings(reversed), func(w string) bool { return strings.HasPrefix(w, "Trigger 2 (Review requests):") }) {
		t.Fatalf("missing reversed warning in %v", vtWarnings(reversed))
	}
	menu := ok.MenuInputs()
	if _, has := menu[InputPullRequest]; !has {
		t.Fatal("menu lacks the pull request")
	}
	if _, has := menu[InputBaseBranch]; has {
		t.Fatal("menu has the base branch")
	}
}

// TestATicketWatchIsIncompatibleWithAManualEntry
func TestATicketWatchIsIncompatibleWithAManualEntry(t *testing.T) {
	t.Parallel()
	stages := SampleStages(TriggerKindIssues, SampleAgents(), true)
	if watch := vtStageIndex(stages, StageKindWatch); watch >= 0 {
		target := WatchTargetTicket
		stages[watch].Watch = &target
	}
	belt := vtMultiBelt("mix", "Mix", []TriggerEntry{
		vtEntry("2", Trigger{Kind: TriggerKindManual}, "acme/web"),
		vtEntry("1", vtIssuesTrigger(), "acme/billing"),
	}, stages, SampleForks(TriggerKindIssues))
	warnings := vtWarnings(belt)
	if !vtAny(warnings, func(w string) bool {
		return strings.HasPrefix(w, "Trigger 1 (Manual):") && strings.Contains(w, "issue or ticket")
	}) {
		t.Fatalf("missing ticket warning in %v", warnings)
	}
	if _, has := belt.MenuInputs()[InputTicket]; has {
		t.Fatal("menu offers a ticket")
	}
	if belt.StageKind() != TriggerKindIssues {
		t.Fatalf("stage kind %q, want issues", belt.StageKind())
	}
}

// TestAForkWarnsPerEntryWhenARouteDoesNotProduceTheInput
func TestAForkWarnsPerEntryWhenARouteDoesNotProduceTheInput(t *testing.T) {
	t.Parallel()
	fork := vtForkOf("size", vtRoute("complex", "Complex"), vtRoute("simple", "Simple"))
	triage := DefaultsStageForAgent(vtAgent("triage"), nil, nil, nil, TriggerKindIssues)
	build := DefaultsStageForAgent(vtAgent("build"), []Stage{triage}, nil, []Stage{triage}, TriggerKindIssues)
	build.Route = new("complex")
	watch := DefaultsStage(StageTypeWatch, []Stage{triage, build}, nil, []Stage{triage, build})
	belt := vtMultiBelt("mix", "Mix", []TriggerEntry{
		vtEntry("1", vtIssuesTrigger(), "acme/billing"),
		vtEntry("2", Trigger{Kind: TriggerKindManual}, "acme/web"),
	}, []Stage{triage, build, watch}, []Fork{fork})
	warnings := vtWarnings(belt)
	if !vtAny(warnings, func(w string) bool {
		return strings.Contains(w, "Trigger 1 (GitHub issues):") && strings.Contains(w, "Simple")
	}) {
		t.Fatalf("missing issues warning in %v", warnings)
	}
	if !vtAny(warnings, func(w string) bool {
		return strings.Contains(w, "Trigger 2 (Manual):") && strings.Contains(w, "Simple")
	}) {
		t.Fatalf("missing manual warning in %v", warnings)
	}
}
