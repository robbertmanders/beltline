// Tests that the library's git and gh commands run under their fixed limits, and that
// one which hits its limit fails with the timeout, whatever it printed first.

package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
)

// limitOf is the limit a call's deadline was set to, to the minute.
func limitOf(c shell.Call) time.Duration {
	return time.Until(c.Deadline).Round(time.Minute)
}

// stallWhen stalls each command whose arguments include word until its context ends,
// and answers every other command with ok.
type stallWhen struct {
	word    string
	stalled shell.Result
	ok      shell.Result
}

func (s stallWhen) Run(ctx context.Context, _ string, args []string, _ string, _ *string) shell.Result {
	for _, arg := range args {
		if arg == s.word {
			<-ctx.Done()
			return s.stalled
		}
	}
	return s.ok
}

// boundedContext is a caller context that ends after a short time: the stalled command
// stops at that deadline, long before its own limit.
func boundedContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

func TestLibraryCommandsRunUnderTheirLimits(t *testing.T) {
	t.Parallel()
	rec := &shell.Recorder{Reply: func(shell.Call) shell.Result { return shell.Result{Stdout: "https://github.com/acme/library/pull/3\n"} }}
	dir := t.TempDir()
	repos := Repos{Root: filepath.Join(dir, "clones"), Runner: rec}
	ctx := context.Background()
	clone := filepath.Join(repos.Root, "acme__library")
	if err := repos.git(ctx, clone, "-c", "user.name=Beltline", "-c", "user.email=b@localhost", "fetch", "-q", "origin"); err != nil {
		t.Fatal(err)
	}
	if err := repos.git(ctx, clone, "push", "-q", "origin", "HEAD:refs/heads/x"); err != nil {
		t.Fatal(err)
	}
	if err := repos.git(ctx, clone, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := repos.clone(ctx, "acme/library", clone); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.OpenPullRequest(ctx, "acme/library", "beltline/x", "main", "Title", "Body"); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		line  string
		limit time.Duration
	}{
		{"git -C " + clone + " -c core.symlinks=false -c user.name=Beltline -c user.email=b@localhost fetch -q origin", shell.TransferLimit},
		{"git -C " + clone + " -c core.symlinks=false push -q origin HEAD:refs/heads/x", shell.TransferLimit},
		{"git -C " + clone + " -c core.symlinks=false rev-parse HEAD", shell.CommandLimit},
		{"gh repo clone acme/library " + clone + " -- -c core.symlinks=false", shell.TransferLimit},
		{"gh pr create --repo acme/library --head beltline/x --base main --title Title --body Body", shell.CommandLimit},
	}
	calls := rec.Calls()
	if len(calls) != len(want) {
		t.Fatalf("calls = %d, want %d: %v", len(calls), len(want), calls)
	}
	for i, call := range calls {
		if call.Line() != want[i].line {
			t.Errorf("call %d = %q, want %q", i, call.Line(), want[i].line)
		}
		if call.Deadline.IsZero() {
			t.Fatalf("%s has no deadline", call.Line())
		}
		if got := limitOf(call); got != want[i].limit {
			t.Errorf("%s: limit %s, want %s", call.Line(), got, want[i].limit)
		}
	}
}

// lookupStall answers pr create with "already exists", then stalls pr view until its
// context ends, printing a URL first: a lookup that was stopped is no answer.
type lookupStall struct{}

func (lookupStall) Run(ctx context.Context, _ string, args []string, _ string, _ *string) shell.Result {
	if len(args) > 1 && args[1] == "create" {
		return shell.Result{Status: 1, Stderr: "a pull request for branch already exists"}
	}
	<-ctx.Done()
	return shell.Result{Stdout: "https://github.com/acme/library/pull/9\n"}
}

func TestAPullRequestLookupThatHitsItsLimitNamesGHPRView(t *testing.T) {
	t.Parallel()
	repos := Repos{Root: t.TempDir(), Runner: lookupStall{}}
	url, err := repos.OpenPullRequest(boundedContext(t), "acme/library", "beltline/x", "main", "Title", "Body")
	if err == nil || url != "" {
		t.Fatalf("url %q, err %v; want the lookup's timeout", url, err)
	}
	if got := err.Error(); got != "Could not open a pull request in acme/library: gh pr view: "+shell.TimedOutReason {
		t.Fatalf("err = %q, want gh pr view named as timed out", got)
	}
}

func TestAPullRequestCreateThatHitsItsLimitNamesGHPRCreate(t *testing.T) {
	t.Parallel()
	stalled := stallWhen{word: "create", stalled: shell.Result{Stderr: "Creating pull request...\n"}}
	repos := Repos{Root: t.TempDir(), Runner: stalled}
	_, err := repos.OpenPullRequest(boundedContext(t), "acme/library", "beltline/x", "main", "Title", "Body")
	if err == nil || err.Error() != "Could not open a pull request in acme/library: gh pr create: "+shell.TimedOutReason {
		t.Fatalf("err = %v, want gh pr create named as timed out", err)
	}
}

func TestACloneThatHitsItsLimitNamesGHRepoClone(t *testing.T) {
	t.Parallel()
	stalled := stallWhen{word: "clone", stalled: shell.Result{Status: 1, Stderr: "Cloning into 'library'...\n"}}
	repos := Repos{Root: t.TempDir(), Runner: stalled}
	err := repos.clone(boundedContext(t), "acme/library", filepath.Join(repos.Root, "acme__library"))
	if err == nil || err.Error() != "Could not clone acme/library: gh repo clone: "+shell.TimedOutReason {
		t.Fatalf("err = %v, want gh repo clone named as timed out", err)
	}
}

func TestAStagedChangeReadThatHitsItsLimitNamesGitDiff(t *testing.T) {
	t.Parallel()
	repos := Repos{Root: t.TempDir(), Runner: stallWhen{word: "--cached", stalled: shell.Result{Status: 1}}}
	_, changed, err := repos.Commit(boundedContext(t), t.TempDir(), "Message")
	if err == nil || changed || err.Error() != "Could not read the staged changes: git diff: "+shell.TimedOutReason {
		t.Fatalf("changed %v, err %v; want git diff named as timed out and nothing committed", changed, err)
	}
}

// cancelOnWord cancels the caller's context when a command whose arguments include word
// starts, and answers with stalled once that context has ended.
type cancelOnWord struct {
	word    string
	cancel  context.CancelFunc
	stalled shell.Result
}

func (c *cancelOnWord) Run(ctx context.Context, _ string, args []string, _ string, _ *string) shell.Result {
	for _, arg := range args {
		if arg == c.word {
			c.cancel()
			<-ctx.Done()
			return c.stalled
		}
	}
	return shell.Result{}
}

// TestACancelledDefaultBranchReadIsNotGuessed: a read the caller cancelled fails the sync
// as cancelled, rather than taking main as the default branch.
func TestACancelledDefaultBranchReadIsNotGuessed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repos := Repos{Root: t.TempDir(), Runner: &cancelOnWord{word: "symbolic-ref", cancel: cancel, stalled: shell.Result{Status: 1}}}
	if err := os.MkdirAll(filepath.Join(repos.Dir("acme/library"), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := repos.Sync(ctx, "acme/library")
	if err == nil || !strings.HasSuffix(err.Error(), shell.CancelledReason) {
		t.Fatalf("sync error = %v, want the cancellation", err)
	}
}

func TestAPushThatHitsItsLimitFailsAsTimedOut(t *testing.T) {
	t.Parallel()
	// git printed a line before it stalled; the push still fails with the timeout.
	stalled := stallWhen{word: "push", stalled: shell.Result{Stderr: "To github.com:acme/library\n"}}
	repos := Repos{Root: t.TempDir(), Runner: stalled}
	err := repos.Push(boundedContext(t), t.TempDir(), "beltline/x")
	if err == nil || err.Error() != "git push failed: "+shell.TimedOutReason {
		t.Fatalf("push error = %v, want the timeout", err)
	}
}

func TestAFetchThatHitsItsLimitRefusesToRefreshTheLibrary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repos := Repos{Root: root, Runner: stallWhen{word: "fetch", stalled: shell.Result{Status: 0, Stdout: "partial"}}}
	// An existing clone skips the clone, so the sync's first command is the fetch.
	if err := os.MkdirAll(filepath.Join(repos.Dir("acme/library"), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := repos.Sync(boundedContext(t), "acme/library")
	if err == nil || err.Error() != "git fetch failed: "+shell.TimedOutReason {
		t.Fatalf("sync error = %v, want git fetch failed: command timed out", err)
	}
}

// TestATimedOutDefaultBranchReadIsNotGuessed: when origin's HEAD read hits its limit,
// the sync fails instead of taking main as the default branch and resetting to it.
func TestATimedOutDefaultBranchReadIsNotGuessed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repos := Repos{Root: root, Runner: stallWhen{word: "symbolic-ref", stalled: shell.Result{Status: 1}}}
	if err := os.MkdirAll(filepath.Join(repos.Dir("acme/library"), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := repos.Sync(boundedContext(t), "acme/library")
	if err == nil || !strings.HasSuffix(err.Error(), shell.TimedOutReason) {
		t.Fatalf("sync error = %v, want the timeout", err)
	}
}

// createShell stalls pr create, which prints "already exists" before it stops, and
// answers pr view with a URL: an earlier open pull request the timeout must not reuse.
type createShell struct {
	mu    sync.Mutex
	views int
}

func (c *createShell) Run(ctx context.Context, _ string, args []string, _ string, _ *string) shell.Result {
	if len(args) > 1 && args[0] == "pr" && args[1] == "create" {
		<-ctx.Done()
		return shell.Result{Status: 1, Stderr: "a pull request for branch already exists"}
	}
	c.mu.Lock()
	c.views++
	c.mu.Unlock()
	return shell.Result{Stdout: "https://github.com/acme/library/pull/9\n"}
}

func TestATimedOutPullRequestCreateIsNotAnAlreadyOpenOne(t *testing.T) {
	t.Parallel()
	sh := &createShell{}
	repos := Repos{Root: t.TempDir(), Runner: sh}
	url, err := repos.OpenPullRequest(boundedContext(t), "acme/library", "beltline/x", "main", "Title", "Body")
	if err == nil || url != "" {
		t.Fatalf("url %q, err %v; want the timeout", url, err)
	}
	if !strings.HasSuffix(err.Error(), shell.TimedOutReason) {
		t.Fatalf("err = %q, want the timeout", err.Error())
	}
	if sh.views != 0 {
		t.Fatalf("pr view ran %d times after a timed-out create", sh.views)
	}
}

// TestTheAlreadyOpenPullRequestLookupRunsUnderItsLimit: an ordinary "already exists" from
// create still falls back to looking up the open pull request, and that lookup is bounded.
func TestTheAlreadyOpenPullRequestLookupRunsUnderItsLimit(t *testing.T) {
	t.Parallel()
	rec := &shell.Recorder{Reply: func(c shell.Call) shell.Result {
		if c.Args[1] == "create" {
			return shell.Result{Status: 1, Stderr: "a pull request for branch already exists"}
		}
		return shell.Result{Stdout: "https://github.com/acme/library/pull/9\n"}
	}}
	repos := Repos{Root: t.TempDir(), Runner: rec}
	url, err := repos.OpenPullRequest(context.Background(), "acme/library", "beltline/x", "main", "Title", "Body")
	if err != nil || url != "https://github.com/acme/library/pull/9" {
		t.Fatalf("url %q, err %v; want the open pull request", url, err)
	}
	calls := rec.Calls()
	if len(calls) != 2 || calls[1].Args[1] != "view" {
		t.Fatalf("calls = %v, want create then view", calls)
	}
	for _, call := range calls {
		if got := limitOf(call); got != shell.CommandLimit {
			t.Errorf("%s: limit %s, want %s", call.Line(), got, shell.CommandLimit)
		}
	}
}
