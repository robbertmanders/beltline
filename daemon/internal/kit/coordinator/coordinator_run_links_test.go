// Coordinator-facing tests of run links. Both read runs and the repository ambiguity only, so they run
// over runs built here and over the demo's runs as the coordinator loads them (creation order, then
// id), without building a coordinator. This is an external test package: the demo may not be imported
// by the coordinator's own tests.

package coordinator_test

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/demo"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// linkRun is a run on an issues belt for subject, begun at now, with the given pull requests.
func linkRun(subject core.Subject, now time.Time, pulls ...core.PullRequest) core.Run {
	run := core.NewRun(core.RunArgs{
		Belt:    core.SampleBelt([]string{subject.Repo}, "agent", core.SampleAgents(), true),
		Subject: subject,
		Now:     now,
	})
	run.PullRequests = pulls
	return run
}

// linkIssue is an issue subject with the URL GitHub gives it unless url is given.
func linkIssue(repo string, number int, url *string) core.Subject {
	link := core.ValueOr(url, "https://github.com/"+repo+"/issues/"+strconv.Itoa(number))
	return core.Subject{Kind: core.SubjectKindIssue, Repo: repo, Number: number, Title: "Issue " + strconv.Itoa(number), URL: link}
}

// TestAmbiguityIncludesRunsOnOtherBeltsAndFinishedRuns checks that a repository's short name is ambiguous
// across every run, including runs on other belts and finished ones, and that a run's links use the full owner.
func TestAmbiguityIncludesRunsOnOtherBeltsAndFinishedRuns(t *testing.T) {
	t.Parallel()
	emptyURL := ""
	shown := linkRun(linkIssue("robbertmanders/beltline", 1, &emptyURL), time.Unix(1, 0),
		core.PullRequest{Repo: "robbertmanders/beltline", Number: 12, URL: "https://github.com/robbertmanders/beltline/pull/12", State: core.PullRequestStateOpen})
	hidden := linkRun(linkIssue("acme/beltline", 2, &emptyURL), time.Unix(2, 0))
	hidden.BeltID = "other-belt"
	hidden.State = core.RunStateShipped
	runs := []core.Run{shown, hidden}

	ambiguous := core.RepoLabelAmbiguousNames(runs)
	if _, ok := ambiguous["beltline"]; len(ambiguous) != 1 || !ok {
		t.Fatalf("ambiguous = %v, want [beltline]", ambiguous)
	}
	found := core.RunLinksTargets(shown.Subject, shown.PullRequests, ambiguous)
	if len(found) != 1 || found[0].Detail() != "Open PR beltline [robbertmanders]#12" {
		t.Fatalf("targets = %+v, want one target \"Open PR beltline [robbertmanders]#12\"", found)
	}
}

// TestDemoFixturesReplaceSeparateLinkButtons checks every demo run's links: a run with a URL offers a
// target for it first, a run without one offers none, and a review offers its pull request once.
func TestDemoFixturesReplaceSeparateLinkButtons(t *testing.T) {
	t.Parallel()
	// The seeded demo is built once per binary and copied here; the test only reads it.
	root := testkit.Template(t, "coordinator-demo", func(dir string) error {
		_, err := demo.Make(dir, testkit.ContractTime, func(string) string { return "" })
		return err
	})
	runs := store.NewStorage(store.Paths{Root: root}).LoadRuns()
	sort.SliceStable(runs, func(i, j int) bool {
		a, b := runs[i].CreatedAt.Time, runs[j].CreatedAt.Time
		if !a.Equal(b) {
			return a.Before(b)
		}
		return runs[i].ID < runs[j].ID
	})
	ambiguous := core.RepoLabelAmbiguousNames(runs)
	reviews, linked, unlinked := 0, 0, 0
	for _, run := range runs {
		found := core.RunLinksTargets(run.Subject, run.PullRequests, ambiguous)
		urls := []string{}
		if run.Subject.URL != "" {
			urls = append(urls, run.Subject.URL)
		}
		for _, pull := range run.PullRequests {
			if pull.URL != "" {
				urls = append(urls, pull.URL)
			}
		}
		if len(urls) == 0 {
			if len(found) != 0 {
				t.Errorf("%s: %d targets, want none", run.ID, len(found))
			}
			unlinked++
		} else {
			if len(found) == 0 {
				t.Errorf("%s: no targets, want some", run.ID)
			}
			linked++
		}
		ids := map[string]struct{}{}
		for _, target := range found {
			ids[target.ID] = struct{}{}
			if target.Detail() != target.Title+" "+target.Reference {
				t.Errorf("%s: detail %q", run.ID, target.Detail())
			}
		}
		if len(ids) != len(found) {
			t.Errorf("%s: duplicate target ids", run.ID)
		}
		if run.Subject.URL != "" {
			if len(found) == 0 || found[0].URL != run.Subject.URL {
				t.Errorf("%s: first target is not the subject's URL", run.ID)
			} else if found[0].Title != core.RunLinksTitleFor(run.Subject.Kind) {
				t.Errorf("%s: first target title %q", run.ID, found[0].Title)
			}
		}
		if run.Subject.Kind == core.SubjectKindPullRequest && run.Subject.URL != "" {
			reviews++
			id := "pr:" + run.Subject.Repo + "#" + strconv.Itoa(run.Subject.Number)
			var matched []string
			for _, target := range found {
				if target.ID == id {
					matched = append(matched, target.URL)
				}
			}
			if len(matched) != 1 || matched[0] != run.Subject.URL {
				t.Errorf("%s: review target %s = %v, want its URL once", run.ID, id, matched)
			}
		}
	}
	if reviews < 1 || linked < 1 || unlinked < 1 {
		t.Errorf("demo has reviews=%d linked=%d unlinked=%d, want each at least one", reviews, linked, unlinked)
	}
}
