// Belts(), nested() and the step
// lists that build the demo's belts from the defaults.

package demo

import (
	"fmt"
	"slices"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// Entry is one of the demo's belts with whether it starts paused.
type Entry struct {
	Belt   core.Belt
	Paused bool
}

// demoStep is one stage of a demo belt: a sample agent by id, or an approval
// or watch by its type name.
type demoStep struct {
	Step string
	Name string
	// Runner is the backend the stage runs on; empty keeps the agent's own.
	Runner core.Backend
	// Route is the fork route the stage sits on; empty for the line.
	Route string
}

// st is a demoStep, for the step lists below.
func st(step, name string, runner core.Backend, route string) demoStep {
	return demoStep{Step: step, Name: name, Runner: runner, Route: route}
}

// Belts is the demo's belts, each with a trigger of its kind, in order.
func Belts() []Entry {
	agents := core.SampleAgents()
	// Issues Triage finds ready to implement skip Spec and the approval, and Pi builds them.
	size := core.Fork{ID: "size", Routes: []core.Route{{ID: "complex", Name: "Complex"}, {ID: "simple", Name: "Simple"}}}
	// Once a pull request is open, conflicts and failing checks are fixed and watched again.
	fixes := []demoStep{st("resolve-conflicts", "Resolve conflicts", core.BackendPi, "conflicts"), st("fix-checks", "Fix checks", core.BackendPi, "checks")}
	afterMerge := core.SampleAfterMerge()

	issues := pipelineBelt(agents, "issues", "Issues to pull requests", issuesTrigger("agent", 30),
		[]string{"acme/billing", "acme/web", "acme/platform"}, []core.Fork{size, afterMerge}, append([]demoStep{
			st("triage", "Triage", core.BackendPi, ""), st("spec", "Spec", core.BackendPi, "complex"), st("approve", "Approve", "", "complex"),
			st("build", "Build", core.BackendCopilot, "complex"), st("build", "Build", core.BackendPi, "simple"), st("verify", "Verify", core.BackendPi, ""),
			st("ship", "Ship", core.BackendPi, ""), st("watch", "Merge", "", ""), st("wrap-up", "Wrap up", core.BackendPi, "merged"),
		}, fixes...))
	issues.Stages[0].Outcomes[0].Then = core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Route: new("simple")})

	reviews := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "reviews", Name: "Pull request reviews", Trigger: reviewsTrigger(30),
		Repos: []string{"acme/web", "acme/platform"}, Stages: core.SampleStages(core.TriggerKindReviews, agents, true),
	})
	reviews.Stages[0].Name = "Read"

	// Schedule(days: .weekdays, hour: 6, minute: 30), with the default weekday.
	timetable := core.DefaultSchedule()
	timetable.Days, timetable.Hour, timetable.Minute = core.ScheduleDaysWeekdays, 6, 30
	nightly := core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &core.ScheduledTask{
		Schedule: timetable,
		Title:    "Sweep flaky tests",
		Task:     "Run the test suite five times. Fix or quarantine every test that fails some of the time.",
	}}

	return []Entry{
		{Belt: issues},
		{Belt: pipelineBelt(agents, "security", "Security fixes", issuesTrigger("security", 30), []string{"acme/platform", "acme/billing"}, nil, []demoStep{
			st("triage", "Triage", core.BackendPi, ""), st("spec", "Spec", core.BackendPi, ""), st("approve", "Approve", "", ""), st("build", "Build", core.BackendCopilot, ""),
			st("verify", "Verify", core.BackendPi, ""), st("approve", "Sign-off", "", ""), st("ship", "Ship", core.BackendPi, ""), st("watch", "Merge", "", ""),
		})},
		{Belt: pipelineBelt(agents, "docs", "Docs fixes", issuesTrigger("docs", 30), []string{"acme/docs"}, nil, []demoStep{
			st("triage", "Triage", core.BackendPi, ""), st("build", "Write", core.BackendPi, ""), st("ship", "Ship", core.BackendPi, ""),
		})},
		{Belt: pipelineBelt(agents, "deps", "Dependency bumps", issuesTrigger("dependencies", 30), []string{"acme/web", "acme/billing", "acme/platform"}, nil, []demoStep{
			st("build", "Upgrade", core.BackendCopilot, ""), st("verify", "Verify", core.BackendPi, ""), st("ship", "Ship", core.BackendPi, ""), st("watch", "Merge", "", ""),
		}), Paused: true},
		{Belt: reviews},
		{Belt: pipelineBelt(agents, "jira", "Payments tickets", jiraTrigger("project = PAY AND labels = agent", 30), []string{"acme/billing", "acme/infra"}, []core.Fork{afterMerge}, append([]demoStep{
			st("triage", "Triage", core.BackendPi, ""), st("spec", "Spec", core.BackendPi, ""), st("approve", "Approve", "", ""), st("build", "Build", core.BackendCopilot, ""),
			st("verify", "Verify", core.BackendPi, ""), st("ship", "Ship", core.BackendPi, ""), st("watch", "Merge", "", ""), st("wrap-up", "Close the ticket", core.BackendPi, "merged"),
		}, fixes...))},
		{Belt: core.SampleEpicBelt(jiraTrigger("project = PAY AND issuetype = Epic", 30), []string{"acme/billing", "acme/infra"}, agents)},
		{Belt: pipelineBelt(agents, "tasks", "Tasks", core.TriggerKindManual.Fresh(), []string{"acme/web", "acme/billing", "acme/platform"}, nil, []demoStep{
			st("spec", "Spec", core.BackendClaude, ""), st("approve", "Approve", "", ""), st("build", "Build", core.BackendClaude, ""),
			st("verify", "Verify", core.BackendClaude, ""), st("ship", "Ship", core.BackendClaude, ""), st("watch", "Merge", "", ""),
		})},
		{Belt: pipelineBelt(agents, "nightly", "Nightly test sweep", nightly, []string{"acme/platform"}, nil, []demoStep{
			st("build", "Sweep", core.BackendCopilot, ""), st("verify", "Verify", core.BackendPi, ""), st("ship", "Ship", core.BackendPi, ""), st("watch", "Merge", "", ""),
		})},
		{Belt: nestedBelt(agents)},
	}
}

// issuesTrigger is GitHub issues with a label, as IssueFilter(label:poll:) builds it.
func issuesTrigger(label string, poll int) core.Trigger {
	filter := core.IssueFilter{Label: label, Assignee: core.TriggerAssigneeAnyone, Poll: poll}
	return core.Trigger{Kind: core.TriggerKindIssues, Issues: &filter}
}

// reviewsTrigger is review requests for you, as ReviewFilter(poll:) builds it.
func reviewsTrigger(poll int) core.Trigger {
	filter := core.ReviewFilter{Reviewers: core.TriggerReviewersMe, Poll: poll}
	return core.Trigger{Kind: core.TriggerKindReviews, Reviews: &filter}
}

// jiraTrigger is a Jira query, as JiraQuery(jql:poll:) builds it.
func jiraTrigger(jql string, poll int) core.Trigger {
	query := core.JiraQuery{Jql: jql, Poll: poll}
	return core.Trigger{Kind: core.TriggerKindJira, Jira: &query}
}

// pipelineBelt makes the belt from its steps: the steps placed on the
// forks, the fixes sent back to the watch, and the task briefs adapted.
func pipelineBelt(agents []core.Agent, id, name string, trigger core.Trigger, repos []string, forks []core.Fork, steps []demoStep) core.Belt {
	stages := buildStages(agents, forks, steps, func(stage *core.Stage, s demoStep) {
		if s.Name == "Sign-off" {
			stage.ID = "signoff"
		}
		if s.Route == "simple" {
			stage.ID = "quick-" + s.Step
		}
	})
	stages = core.SampleSendingFixes(stages, forks)
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: id, Name: name, Trigger: trigger, Repos: repos, Stages: core.SampleTaskBriefs(stages, agents), Forks: forks,
	})
}

// buildStages places each step after the ones before it, on its route:
// an approval or watch by its type, anything else by its
// sample agent. The tune hook renames a stage's id for its step.
func buildStages(agents []core.Agent, forks []core.Fork, steps []demoStep, tune func(stage *core.Stage, s demoStep)) []core.Stage {
	var stages []core.Stage
	for _, s := range steps {
		place := core.DefaultsPlace{Before: stages, Forks: forks, Route: optionalString(s.Route)}
		var stage core.Stage
		if typ, ok := stageTypeNamed(s.Step); ok {
			stage = core.DefaultsStageAt(typ, place, stages)
		} else {
			stage = core.DefaultsStageForAgentAt(agentNamed(agents, s.Step), place, stages, core.TriggerKindIssues)
		}
		stage.Name = s.Name
		if s.Runner != "" && (stage.Runner == nil || *stage.Runner != s.Runner) {
			runner := s.Runner
			stage.Runner = &runner
			stage.Model = "fixture-model"
		}
		if tune != nil {
			tune(&stage, s)
		}
		stages = append(stages, stage)
	}
	return stages
}

// stageTypeNamed is the stage type whose raw value is name, if there is one.
func stageTypeNamed(name string) (core.StageType, bool) {
	for _, typ := range core.AllStageType {
		if string(typ) == name {
			return typ, true
		}
	}
	return "", false
}

// agentNamed is the sample agent with id. It force-unwraps,
// so an id that is not a sample panics here too.
func agentNamed(agents []core.Agent, id string) core.Agent {
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	panic("demo: no sample agent " + id)
}

// optionalString is nil for the empty string.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nestedBelt is a belt with an outer fork whose Build route splits again, and
// whose Skip route holds a fork the sample run does not take.
func nestedBelt(agents []core.Agent) core.Belt {
	wave := core.Fork{ID: "wave", Routes: []core.Route{{ID: "build", Name: "Build"}, {ID: "skip", Name: "Skip"}}}
	pace := core.Fork{ID: "pace", Routes: []core.Route{{ID: "quick", Name: "Quick"}, {ID: "deep", Name: "Build"}}, Parent: new("build")}
	later := core.Fork{ID: "later", Routes: []core.Route{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}, Parent: new("skip")}
	forks := []core.Fork{wave, pace, later}
	specBrief := agentNamed(agents, "spec").Brief
	stages := buildStages(agents, forks, []demoStep{
		st("triage", "Triage", core.BackendPi, ""),
		st("spec", "Assess", core.BackendPi, "build"),
		st("build", "Build", core.BackendCopilot, "quick"),
		st("spec", "Spec", core.BackendPi, "deep"),
		st("build", "Build", core.BackendCopilot, "deep"),
		st("verify", "Verify", core.BackendPi, "build"),
		st("build", "Glance", core.BackendPi, "skip"),
		st("spec", "Note", core.BackendPi, "one"),
		st("ship", "Ship", core.BackendPi, ""),
	}, func(stage *core.Stage, s demoStep) {
		if s.Route == "quick" {
			stage.ID = "quick-build"
		}
		if s.Name == "Assess" {
			stage.ID = "assess"
		}
		if s.Route == "deep" && s.Name == "Spec" {
			stage.ID = "deep-spec"
			stage.Key = new("deep-spec")
			stage.Brief = strings.ReplaceAll(specBrief, "{spec}", "{output}")
		}
		if s.Route == "deep" && s.Name == "Build" {
			stage.ID = "deep-build"
		}
		if s.Name == "Glance" {
			stage.ID = "glance"
		}
		if s.Name == "Note" {
			stage.ID = "note"
		}
	})
	belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "nested", Name: "Nested forks", Trigger: issuesTrigger("agent", 30), Repos: []string{"acme/billing"},
		Stages: core.SampleTaskBriefs(stages, agents), Forks: forks,
	})
	belt.Stages[0].Outcomes[0].Then = core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Route: new("skip")})
	if assess := stageIndex(belt, func(s core.Stage) bool { return s.Name == "Assess" }); assess >= 0 {
		belt.Stages[assess].Outcomes = append(belt.Stages[assess].Outcomes, core.Outcome{
			Name: "take the long route", When: "the change needs a spec first", Tone: core.ToneNeutral,
			Then: core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Route: new("deep")}),
		})
	}
	if glance := stageIndex(belt, func(s core.Stage) bool { return s.Name == "Glance" }); glance >= 0 {
		belt.Stages[glance].Outcomes = append(belt.Stages[glance].Outcomes, core.Outcome{
			Name: "take the other route", When: "the other route applies", Tone: core.ToneNeutral,
			Then: core.NewThen(core.ThenArgs{Action: core.ThenActionRoute, Route: new("two")}),
		})
	}
	quick := stageIndex(belt, func(s core.Stage) bool { return s.ID == "quick-build" })
	ship := stageIndex(belt, func(s core.Stage) bool { return s.Name == "Ship" })
	if quick >= 0 && ship >= 0 {
		belt.Stages[quick].Outcomes = append(belt.Stages[quick].Outcomes, core.Outcome{
			Name: "ship it", When: "the work is already done", Tone: core.ToneNeutral,
			Then: core.NewThen(core.ThenArgs{Action: core.ThenActionGoto, To: []string{belt.Stages[ship].ID}}),
		})
	}
	return belt
}

// stageIndex is the index of the first stage of belt that matches, or -1.
func stageIndex(belt core.Belt, match func(core.Stage) bool) int {
	return slices.IndexFunc(belt.Stages, match)
}

// outcomeID is the fixed id the n-th outcome of a demo belt gets, counted in
// belt and stage order, formatted as twelve digits.
func outcomeID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}
