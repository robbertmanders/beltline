// Coordinator tests of the base branch a run's work starts from, in each repository. The
// resolver suspends around GitHub, so each call runs in a harness turn.

package coordinator

import (
	"context"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// basesTestTicket is the ticket the base tests use.
var basesTestTicket = core.Subject{
	Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund part of a payment",
	Body: "Up to what was paid.", URL: "https://acme.atlassian.net/browse/PAY-31",
}

// basesTestEpicTicket is the epic's child with a `base:` label (MultiRepositoryBaseTests.epicTicket).
var basesTestEpicTicket = core.Subject{
	Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-42", Title: "Part",
	Labels: []string{"agent", "base:beltline/PAY-40"},
}

// basesTestRun is a run of a Jira belt over two repositories, with the belt's base.
func basesTestRun(base string, subject core.Subject) core.Run {
	trigger := core.TriggerKindJira.Fresh()
	query := core.DefaultJiraQuery()
	query.Jql = "project = PAY"
	trigger.Jira = &query
	belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "jira", Name: "Jira", Trigger: trigger, Repos: []string{"acme/billing", "acme/web"},
		Stages: core.SampleStages(core.TriggerKindJira, core.SampleAgents(), true),
		Forks:  core.SampleForks(core.TriggerKindJira), Base: base,
	})
	return core.NewRun(core.RunArgs{Belt: belt, Subject: subject, Now: sessionSampleT0})
}

// basesTestBases builds the resolver over the harness's coordinator and fake GitHub.
func basesTestBases(h *harness) *Bases {
	return &Bases{c: h.Coord(), github: h.github, defaults: map[string]string{}, found: map[string]struct{}{}}
}

func TestABaseOnlySomeRepositoriesHaveFallsBackToTheirDefault(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.github.SetBranch("acme/web", "beltline/PAY-40", false)
	bases := basesTestBases(h)
	run := basesTestRun("", basesTestEpicTicket)
	h.Turn(func() {
		ctx := context.Background()
		assertBase(t, bases, ctx, run, "acme/billing", "beltline/PAY-40")
		assertBase(t, bases, ctx, run, "acme/web", "main")
		// A feature branch pushed later is found on the next ask.
		h.github.SetBranch("acme/web", "beltline/PAY-40", true)
		assertBase(t, bases, ctx, run, "acme/web", "beltline/PAY-40")
		// One that was found isn't asked about again.
		asked := h.github.BranchLookups()
		assertBase(t, bases, ctx, run, "acme/billing", "beltline/PAY-40")
		if got := h.github.BranchLookups(); got != asked {
			t.Fatalf("branch lookups = %d, want %d: a found branch is asked again", got, asked)
		}
	})
}

func TestABeltsBaseFallsBackTooAndNoBaseIsTheDefault(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	h.github.SetBranch("acme/web", "develop", false)
	bases := basesTestBases(h)
	h.Turn(func() {
		ctx := context.Background()
		developing := basesTestRun("develop", basesTestTicket)
		if got := developing.RequestedBase("acme/web"); got == nil || *got != "develop" {
			t.Fatalf("requested base = %v, want develop", got)
		}
		assertBase(t, bases, ctx, developing, "acme/billing", "develop")
		assertBase(t, bases, ctx, developing, "acme/web", "main")
		plain := basesTestRun("", basesTestTicket)
		if plain.RequestedBase("acme/web") != nil {
			t.Fatalf("requested base = %v, want none", *plain.RequestedBase("acme/web"))
		}
		assertBase(t, bases, ctx, plain, "acme/web", "main")
		if got := h.github.BranchLookups(); got != 2 {
			t.Fatalf("branch lookups = %d, want 2", got)
		}
	})
}

// assertBase checks what base returns for run in repo.
// Lock: runs with the harness turn held.
func assertBase(t *testing.T, bases *Bases, ctx context.Context, run core.Run, repo, want string) {
	t.Helper()
	got, err := bases.base(ctx, run, repo)
	if err != nil {
		t.Fatalf("base(%s) failed: %v", repo, err)
	}
	if got != want {
		t.Fatalf("base(%s) = %q, want %q", repo, got, want)
	}
}
