package core

import (
	"reflect"
	"strconv"
	"testing"
	"time"
)

// rlRun is a run of the default issue belt on repo, numbered n.
func rlRun(n int, repo string) Run {
	return NewRun(RunArgs{
		Belt:    SampleBelt([]string{repo}, "agent", SampleAgents(), true),
		Subject: Subject{Kind: SubjectKindIssue, Repo: repo, Number: n, Title: "T" + strconv.Itoa(n)},
		Now:     time.Unix(int64(n), 0),
	})
}

// TestUniqueNamesAndSurfaceInputs
func TestUniqueNamesAndSurfaceInputs(t *testing.T) {
	t.Parallel()
	run := rlRun(1, "robbertmanders/beltline")
	ambiguous := RepoLabelAmbiguousNames([]Run{run, run})
	if len(ambiguous) != 0 {
		t.Fatalf("ambiguous = %v, want none", ambiguous)
	}
	// Work row, Run tag and Floor tooltip use the subject; repository
	// headers retain the full key and only shorten its drawn text.
	if got := RepoLabelShort(run.Subject.Repo, ambiguous); got != "beltline" {
		t.Fatalf("short = %q", got)
	}
	key := WorkGroupGroupRuns([]Run{run}, GroupByRepo)[0].Key
	if got := RepoLabelShort(key.Value, ambiguous); got != "beltline" {
		t.Fatalf("group short = %q", got)
	}
	// Run and stage-panel chips and shipped summaries share this formatter.
	if got := RepoLabelPullRequest(run.Subject.Repo, 12, ambiguous); got != "beltline#12" {
		t.Fatalf("pull request = %q", got)
	}
}

// TestAmbiguousNamesSpanOwners
func TestAmbiguousNamesSpanOwners(t *testing.T) {
	t.Parallel()
	runs := []Run{rlRun(1, "robbertmanders/beltline"), rlRun(2, "acme/beltline"), rlRun(3, "acme/beltline")}
	ambiguous := RepoLabelAmbiguousNames(runs)
	if !reflect.DeepEqual(ambiguous, map[string]struct{}{"beltline": {}}) {
		t.Fatalf("ambiguous = %v, want beltline", ambiguous)
	}
	if got := RepoLabelShort("robbertmanders/beltline", ambiguous); got != "beltline [robbertmanders]" {
		t.Fatalf("robbertmanders short = %q", got)
	}
	if got := RepoLabelShort("acme/beltline", ambiguous); got != "beltline [acme]" {
		t.Fatalf("acme short = %q", got)
	}
	if got := RepoLabelPullRequest("acme/beltline", 12, ambiguous); got != "beltline [acme]#12" {
		t.Fatalf("acme pull request = %q", got)
	}
}

// TestMultiRepoWorkContributesToAmbiguity
func TestMultiRepoWorkContributesToAmbiguity(t *testing.T) {
	t.Parallel()
	run := rlRun(1, "acme/beltline")
	run.Repos = []string{"acme/beltline", "other/beltline"}
	if got := RepoLabelAmbiguousNames([]Run{run}); !reflect.DeepEqual(got, map[string]struct{}{"beltline": {}}) {
		t.Fatalf("multi-repo ambiguous = %v", got)
	}
	run.Repos = []string{run.Subject.Repo}
	run.PullRequests = []PullRequest{{Number: 12, URL: "", Repo: "other/beltline"}}
	if got := RepoLabelAmbiguousNames([]Run{run}); !reflect.DeepEqual(got, map[string]struct{}{"beltline": {}}) {
		t.Fatalf("pull request ambiguous = %v", got)
	}
}

// TestNamesAreCaseSensitiveAndSlashlessValuesStayIntact
func TestNamesAreCaseSensitiveAndSlashlessValuesStayIntact(t *testing.T) {
	t.Parallel()
	runs := []Run{rlRun(1, "acme/Beltline"), rlRun(2, "acme/beltline")}
	if got := RepoLabelAmbiguousNames(runs); len(got) != 0 {
		t.Fatalf("case-different names ambiguous: %v", got)
	}
	if got := RepoLabelShort(runs[0].Subject.Repo, nil); got != "Beltline" {
		t.Fatalf("Beltline = %q", got)
	}
	if got := RepoLabelShort(runs[1].Subject.Repo, nil); got != "beltline" {
		t.Fatalf("beltline = %q", got)
	}
	if got := RepoLabelShort("task", map[string]struct{}{"task": {}}); got != "task" {
		t.Fatalf("slashless = %q", got)
	}
	if got := RepoLabelShort("task [owner]", map[string]struct{}{"task [owner]": {}}); got != "task [owner]" {
		t.Fatalf("slashless bracketed = %q", got)
	}
	if got := RepoLabelShort("", nil); got != "" {
		t.Fatalf("empty = %q", got)
	}
}
