// Tests of the gh JSON parsing, and the opt-in read-only gh checks.

package github

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
)

func TestTheLatestReviewRequestIsYoursOrYourTeams(t *testing.T) {
	t.Parallel()
	timeline := "11 alice \n12  core\n13 you \n14 bob \n"
	check := func(reviewer string, teams bool, want string) {
		t.Helper()
		got := latestRequest(timeline, reviewer, teams)
		if want == "" {
			if got != nil {
				t.Fatalf("%s teams=%v: got %s, want none", reviewer, teams, *got)
			}
			return
		}
		if got == nil || *got != want {
			t.Fatalf("%s teams=%v: got %v, want %s", reviewer, teams, got, want)
		}
	}
	check("you", false, "13")
	check("You", false, "13")
	check("carol", true, "12")
	check("carol", false, "")
}

func TestRollupRequiresAllChecksToFinish(t *testing.T) {
	t.Parallel()
	pull := func(checks string) core.PullRequest {
		t.Helper()
		text := `{"number":201,"url":"u","state":"OPEN","statusCheckRollup":` + checks + `}`
		var j pullJSON
		if err := json.Unmarshal([]byte(text), &j); err != nil {
			t.Fatal(err)
		}
		return j.pullRequest()
	}
	if p := pull("[]"); p.Checks != nil {
		t.Fatalf("empty rollup: checks %v", *p.Checks)
	}
	if p := pull(`[{"status":"COMPLETED","conclusion":"FAILURE"},{"status":"IN_PROGRESS","conclusion":""}]`); p.Checks == nil || *p.Checks != core.PullRequestCheckStatePending {
		t.Fatalf("running rollup: %+v", p.Checks)
	}
	for _, bad := range []string{"FAILURE", "ERROR", "TIMED_OUT", "CANCELLED"} {
		p := pull(`[{"status":"COMPLETED","conclusion":"` + bad + `"},{"state":"SUCCESS"}]`)
		if p.Checks == nil || *p.Checks != core.PullRequestCheckStateFailing {
			t.Fatalf("%s: checks %+v", bad, p.Checks)
		}
		if p.CheckFailures == nil || *p.CheckFailures != 1 {
			t.Fatalf("%s: failures %+v", bad, p.CheckFailures)
		}
		if p.CheckTotal == nil || *p.CheckTotal != 2 {
			t.Fatalf("%s: total %+v", bad, p.CheckTotal)
		}
	}
	if p := pull(`[{"status":"COMPLETED","conclusion":"SKIPPED"},{"state":"SUCCESS"}]`); p.Checks == nil || *p.Checks != core.PullRequestCheckStatePassing {
		t.Fatalf("skipped rollup: %+v", p.Checks)
	}
	if p := pull(`[{"state":"PENDING"},{"state":"FAILURE"}]`); p.Checks == nil || *p.Checks != core.PullRequestCheckStatePending {
		t.Fatalf("pending rollup: %+v", p.Checks)
	}
}

// branchShell answers each call with the next reply, recording every call.
type branchShell struct {
	mu      sync.Mutex
	replies []shell.Result
	calls   [][]string
}

func (s *branchShell) Run(_ context.Context, executable string, arguments []string, _ string, _ *string) shell.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, append([]string{executable}, arguments...))
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return reply
}

func TestGhAsksForTheBranchAndANotFoundIsMissing(t *testing.T) {
	t.Parallel()
	sh := &branchShell{replies: []shell.Result{
		{Status: 0, Stdout: "beltline/PAY-40\n"},
		{Status: 1, Stderr: "gh: Branch not found (HTTP 404)"},
		{Status: 1, Stderr: "gh: Bad credentials (HTTP 401)"},
	}}
	gh := NewGHCLI(sh)
	ctx := context.Background()
	exists, err := gh.BranchExists(ctx, "a/b", "beltline/PAY-40")
	if err != nil || exists == nil || !*exists {
		t.Fatalf("existing branch: %v %v", exists, err)
	}
	missing, err := gh.BranchExists(ctx, "a/c", "beltline/PAY-40")
	if err != nil || missing == nil || *missing {
		t.Fatalf("missing branch: %v %v", missing, err)
	}
	if _, err := gh.BranchExists(ctx, "a/d", "x"); !isFailure(err) {
		t.Fatalf("bad credentials not refused: %v", err)
	}
	want := []string{"gh", "api", "repos/a/b/branches/beltline/PAY-40", "-q", ".name"}
	if len(sh.calls) == 0 || !slices.Equal(want, sh.calls[0]) {
		t.Fatalf("first call %v", sh.calls)
	}
}

// isFailure reports whether err is a core.Failure.
func isFailure(err error) bool {
	_, ok := failure(err)
	return ok
}

// TestRefsArePercentEncodedAsFoundationDoes: Foundation keeps the Unicode letters,
// marks and numbers of CharacterSet.alphanumerics, the ASCII punctuation "-._~/",
// and encodes every other scalar's UTF-8 bytes.
func TestRefsArePercentEncodedAsFoundationDoes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"feature/x y", "feature/x%20y"},
		{"a+b", "a%2Bb"},
		{"caf\u00e9", "caf\u00e9"},
		{"cafe\u0301", "cafe\u0301"},
		{"x\u00b2", "x\u00b2"},
		{"\U0001F600", "%F0%9F%98%80"},
	} {
		got, ok := percentEncodeRef(c.in)
		if !ok || got != c.want {
			t.Fatalf("percentEncodeRef(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}

// TestTheFakeKeepsAReviewerAsGiven: an empty reviewer is stored as an empty name, as
// the fake keeps it (its "you" default belongs to the call site), and withdrawing
// it removes that name.
func TestTheFakeKeepsAReviewerAsGiven(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := NewFakeGitHub(nil, nil, nil, nil)
	pull := core.Subject{Repo: "acme/billing", Number: 7, IsOpen: true}
	f.RequestReview(pull, "", "r1")
	if got, _ := f.LatestReviewRequest(ctx, "acme/billing", 7, "", false); got == nil || *got != "r1" {
		t.Fatalf("empty reviewer: %v", got)
	}
	if got, _ := f.LatestReviewRequest(ctx, "acme/billing", 7, "you", false); got != nil {
		t.Fatalf("\"you\" was stored for an empty reviewer: %s", *got)
	}
	f.WithdrawReview("acme/billing", 7, "")
	if got, _ := f.LatestReviewRequest(ctx, "acme/billing", 7, "", false); got != nil {
		t.Fatalf("withdrawn: %s", *got)
	}
}

func TestLiveGitHubReadsIssuesAndTheDefaultBranch(t *testing.T) {
	t.Parallel()
	repo := os.Getenv("BELTLINE_LIVE_REPO")
	if repo == "" {
		t.Skip("set BELTLINE_LIVE_REPO")
	}
	ctx := context.Background()
	gh := NewGHCLI(nil)
	base, err := gh.DefaultBranch(ctx, repo)
	if err != nil || base == "" {
		t.Fatalf("default branch %q: %v", base, err)
	}
	issues, err := gh.Issues(ctx, repo, "beltline-test-label-that-does-not-exist", nil)
	if err != nil || len(issues) != 0 {
		t.Fatalf("issues %v: %v", issues, err)
	}
	login, err := gh.Login(ctx)
	if err != nil || login == "" {
		t.Fatalf("login %q: %v", login, err)
	}
}

func TestLiveGitHubReadsReviewRequests(t *testing.T) {
	t.Parallel()
	repo := os.Getenv("BELTLINE_LIVE_REPO")
	if repo == "" {
		t.Skip("set BELTLINE_LIVE_REPO")
	}
	ctx := context.Background()
	gh := NewGHCLI(nil)
	login, err := gh.Login(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pulls, err := gh.ReviewRequests(ctx, repo, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, pull := range pulls {
		if pull.Kind != core.SubjectKindPullRequest || pull.Base == nil || *pull.Base == "" {
			t.Fatalf("pull %+v", pull)
		}
		if _, err := gh.LatestReviewRequest(ctx, repo, pull.Number, login, true); err != nil {
			t.Fatal(err)
		}
	}
}

// TestReviewRequestsLeaveOutForks: a pull request from a fork, or one gh does not
// say the origin of, starts no review; one from the repository's own branch does.
func TestReviewRequestsLeaveOutForks(t *testing.T) {
	t.Parallel()
	list := `[{"number":1,"title":"Own branch","baseRefName":"main","isCrossRepository":false},` +
		`{"number":2,"title":"From a fork","baseRefName":"main","isCrossRepository":true},` +
		`{"number":3,"title":"Unknown","baseRefName":"main"}]`
	sh := &branchShell{replies: []shell.Result{{Status: 0, Stdout: list}}}
	pulls, err := NewGHCLI(sh).ReviewRequests(context.Background(), "a/b", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 || pulls[0].Number != 1 {
		t.Fatalf("pulls = %+v, want only #1", pulls)
	}
	if call := sh.calls[0]; !slices.Contains(call, reviewFields) || !strings.Contains(reviewFields, "isCrossRepository") {
		t.Fatalf("call %v does not ask whether a pull request is from a fork", call)
	}
}
