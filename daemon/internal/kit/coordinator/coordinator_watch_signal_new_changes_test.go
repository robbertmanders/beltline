// Tests of new changes on a watch: a head change is a baseline, then one event per visit, and the
// repositories compete for it.

package coordinator

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestAHeadChangeIsABaselineThenOneEvent
func TestAHeadChangeIsABaselineThenOneEvent(t *testing.T) {
	t.Parallel()
	f := newH5m3Watch(t, watchOptions{events: []core.WatchEvent{core.WatchEventNewChanges}})
	f.open()
	f.check()
	coordCheck(t, f.run().State == core.RunStateRunning, "state = %v, want running", f.run().State)
	coordCheck(t, len(f.newChanges()) == 0, "new changes = %v, want none", f.newChanges())
	coordCheck(t, f.seen("acme/billing") != nil && *f.seen("acme/billing").HeadSha == "a", "baseline head is not a")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "pending head is set after the baseline")
	f.check()
	coordCheck(t, len(f.newChanges()) == 0, "new changes after second poll = %v, want none", f.newChanges())
	f.gh.SetHead(201, new("b"))
	f.gh.SetHead(201, new("c"))
	f.gh.SetHead(201, new("force-pushed"))
	f.check()
	coordCheck(t, len(f.newChanges()) == 1, "new changes = %v, want one", f.newChanges())
	coordCheck(t, f.run().State == core.RunStateHeld, "state = %v, want held", f.run().State)
	coordCheck(t, f.run().Reason != nil && *f.run().Reason == "new changes", "reason = %v", f.run().Reason)
	coordCheck(t, f.lastDetail() == "#201 has new changes", "detail = %q", f.lastDetail())
	attempt := f.run().LastAttempt("watch")
	coordCheck(t, attempt != nil && attempt.Tone != nil && *attempt.Tone == core.ToneYou, "tone is not you")
	stored := f.stored()
	coordCheck(t, len(stored) == 1 && coordLastDetail(stored[0], "watch") == "#201 has new changes", "saved detail differs")
	coordCheck(t, *f.seen("acme/billing").HeadSha == "force-pushed", "baseline head is not force-pushed")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "pending head is still set")
}

// TestAnEmptyHeadKeepsTheBaselineUntilADifferentOneReturns
func TestAnEmptyHeadKeepsTheBaselineUntilADifferentOneReturns(t *testing.T) {
	t.Parallel()
	f := newH5m3Watch(t, watchOptions{events: []core.WatchEvent{core.WatchEventNewChanges}})
	f.open()
	f.check()
	f.gh.SetHead(201, nil)
	f.check()
	coordCheck(t, f.run().State == core.RunStateRunning, "state = %v, want running", f.run().State)
	coordCheck(t, *f.seen("acme/billing").HeadSha == "a", "baseline head changed")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "pending head is set")
	f.gh.SetHead(201, new(""))
	f.check()
	coordCheck(t, len(f.newChanges()) == 0, "new changes = %v, want none", f.newChanges())
	coordCheck(t, *f.seen("acme/billing").HeadSha == "a", "baseline head changed to empty")
	f.gh.SetHead(201, new("a"))
	f.check()
	coordCheck(t, len(f.newChanges()) == 0, "new changes after a again = %v, want none", f.newChanges())
	f.gh.SetHead(201, nil)
	f.check()
	f.gh.SetHead(201, new("b"))
	f.check()
	coordEqual(t, "new changes", f.newChanges(), []string{"new changes"})
	coordCheck(t, f.lastDetail() == "#201 has new changes", "detail = %q", f.lastDetail())
}

// pullIn is an open pull request numbered number in repo, with head and an optional URL
// (a pull request without one takes the repository's default URL).
func pullIn(repo string, number int, head, url string) core.PullRequest {
	if url == "" {
		url = fmt.Sprintf("https://github.com/%s/pull/%d", repo, number)
	}
	pull := coordPull(number, url, head, "main")
	pull.Repo = repo
	return pull
}

// TestNewChangesWinsAmongOpenPullsAndOnlyTheWinnerIsConsumed
func TestNewChangesWinsAmongOpenPullsAndOnlyTheWinnerIsConsumed(t *testing.T) {
	t.Parallel()
	f := coordTwoRepositories(t, nil)
	f.setNewChanges(retryThen())
	f.check()
	coordCheck(t, len(f.newChanges()) == 0, "new changes = %v, want none", f.newChanges())
	f.gh.SetHead(201, new("b"))
	f.gh.SetHead(88, new("b"))
	f.gh.SetConflicted(201, true)
	f.gh.RequestChanges(201, "carol", coordNow())
	f.check()
	coordCheck(t, len(f.newChanges()) == 1, "new changes = %v, want one", f.newChanges())
	coordCheck(t, lastOutcomeDetail(f.run(), "new changes") == "#201 in acme/billing has new changes", "winner detail differs")
	coordCheck(t, *f.seen("acme/billing").HeadSha == "b", "billing baseline is not b")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "billing pending head is set")
	coordCheck(t, *f.seen("acme/web").HeadSha == "a", "web baseline is not a")
	coordCheck(t, f.seen("acme/web").PendingHead != nil && *f.seen("acme/web").PendingHead == "b", "web pending head is not b")
	f.gh.SetHead(88, new("c"))
	f.check()
	coordCheck(t, len(f.newChanges()) == 2, "new changes = %v, want two", f.newChanges())
	coordCheck(t, lastOutcomeDetail(f.run(), "new changes") == "#88 in acme/web has new changes", "second winner detail differs")
	coordCheck(t, *f.seen("acme/web").HeadSha == "c", "web baseline is not c")
	coordCheck(t, f.seen("acme/web").PendingHead == nil, "web pending head is set")
	f.check()
	coordCheck(t, len(f.newChanges()) == 2, "new changes after nothing new = %v, want two", f.newChanges())
	coordCheck(t, f.watchOutcomes()[len(f.watchOutcomes())-1] == "changes requested", "last outcome is not changes requested")
}

// lastOutcomeDetail is the detail of the last attempt whose outcome is outcome.
func lastOutcomeDetail(run core.Run, outcome string) string {
	for index := len(run.Attempts) - 1; index >= 0; index-- {
		attempt := run.Attempts[index]
		if attempt.Outcome != nil && *attempt.Outcome == outcome {
			if attempt.Detail == nil {
				return "(nil)"
			}
			return *attempt.Detail
		}
	}
	return "(none)"
}

// coordNow is the real clock, which the fixtures use where a default argument would be the current time.
func coordNow() time.Time { return time.Now() }

// TestAPendingChangeSurvivesResumeAndRestart
func TestAPendingChangeSurvivesResumeAndRestart(t *testing.T) {
	t.Parallel()
	f := coordTwoRepositories(t, []core.WatchEvent{core.WatchEventNewChanges})
	f.check()
	f.gh.SetHead(201, new("b"))
	f.gh.SetHead(88, new("b"))
	f.check()
	coordCheck(t, f.lastDetail() == "#201 in acme/billing has new changes", "detail = %q", f.lastDetail())
	pending := false
	for key, observation := range f.stored()[0].WatchObservations {
		if strings.HasSuffix(key, "|acme/web") && observation.PendingHead != nil && *observation.PendingHead == "b" {
			pending = true
		}
	}
	coordCheck(t, pending, "saved web observation has no pending head b")
	f.gh.SetHead(88, new("c"))
	restarted := f.restart()
	coordResume(restarted, f.run().ID)
	checkWatches(restarted, true)
	outcomes := countOutcome(coordWatchOutcomes(restarted.Runs()[0]), "new changes")
	coordCheck(t, outcomes == 2, "new changes = %d, want 2", outcomes)
	coordCheck(t, coordLastDetail(restarted.Runs()[0], "watch") == "#88 in acme/web has new changes", "restarted detail differs")
	web := coordSeen(restarted.Runs()[0], "acme/web")
	coordCheck(t, web != nil && *web.HeadSha == "c", "web baseline is not c")
	coordCheck(t, web.PendingHead == nil, "web pending head is set")
}

// TestClosureAndAFullMergeOutrankNewChanges checks that a closure and a full merge outrank new changes,
// with closed waiting for every pull request: one closed leaves the other's new changes to report.
func TestClosureAndAFullMergeOutrankNewChanges(t *testing.T) {
	t.Parallel()
	partly := coordTwoRepositories(t, nil)
	partly.check()
	partly.gh.SetHead(201, new("b"))
	partly.gh.SetPullState(88, core.PullRequestStateClosed, new("acme/web"))
	partly.check()
	coordEqual(t, "partly closed outcomes", partly.watchOutcomes(), []string{"new changes"})

	closed := coordTwoRepositories(t, nil)
	closed.check()
	closed.gh.SetHead(201, new("b"))
	closed.gh.SetPullState(201, core.PullRequestStateClosed, new("acme/billing"))
	closed.gh.SetPullState(88, core.PullRequestStateClosed, new("acme/web"))
	closed.check()
	coordEqual(t, "closed outcomes", closed.watchOutcomes(), []string{"closed"})
	coordCheck(t, len(closed.newChanges()) == 0, "closed: new changes = %v", closed.newChanges())

	merged := coordTwoRepositories(t, nil)
	merged.check()
	merged.gh.SetHead(201, new("b"))
	merged.gh.SetHead(88, new("b"))
	merged.gh.SetPullState(201, core.PullRequestStateMerged, new("acme/billing"))
	merged.gh.SetPullState(88, core.PullRequestStateMerged, new("acme/web"))
	merged.check()
	coordEqual(t, "merged outcomes", merged.watchOutcomes(), []string{"merged"})
	coordCheck(t, len(merged.newChanges()) == 0, "merged: new changes = %v", merged.newChanges())
}

// TestNewChangesFiresFirstAndLeavesChecksReviewsAndBehind
func TestNewChangesFiresFirstAndLeavesChecksReviewsAndBehind(t *testing.T) {
	t.Parallel()
	f := newH5m3Watch(t, watchOptions{events: []core.WatchEvent{core.WatchEventNewChanges, core.WatchEventChangesRequested, core.WatchEventConflicted, core.WatchEventChecksFailed, core.WatchEventBehind}})
	f.open()
	f.check()
	f.gh.SetHead(201, new("b"))
	f.gh.RequestChanges(201, "carol", coordNow())
	f.gh.SetConflicted(201, true)
	f.gh.SetChecks(201, 2, 4, false)
	f.gh.SetBehind(201, 3)
	f.check()
	coordEqual(t, "outcomes", f.watchOutcomes(), []string{"new changes"})
	for _, name := range []string{"changes requested", "has conflicts", "checks failing", "is behind"} {
		f.resume()
		f.check()
		outcomes := f.watchOutcomes()
		coordCheck(t, len(outcomes) > 0 && outcomes[len(outcomes)-1] == name, "last outcome = %v, want %s", outcomes, name)
	}
}

// TestAddingNewChangesReachesNewRunsOnly
// The library is built over a seeded storage; here the coordinator's SaveBelt does the saving.
func TestAddingNewChangesReachesNewRunsOnly(t *testing.T) {
	t.Parallel()
	// No issues: a save kicks a tick, and the default issue would start a run on the new belt.
	h := newHarness(t, harnessOptions{Issues: []core.Subject{}})
	c := h.Coord()
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	watch := coordWatchIndex(t, belt)
	var kept []core.Outcome
	for _, outcome := range belt.Stages[watch].Outcomes {
		if outcome.When != string(core.WatchEventNewChanges) {
			kept = append(kept, outcome)
		}
	}
	belt.Stages[watch].Outcomes = kept
	belt.ID = c.NewBeltDraft().ID
	belt.Version = 0
	saved, err := c.SaveBelt(belt)
	if err != nil {
		t.Fatalf("save belt: %v", err)
	}
	pinned := core.NewRun(core.RunArgs{Belt: saved, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Pinned", IsOpen: true}, Now: sessionSampleT0})
	if err := h.storage.SaveRun(pinned); err != nil {
		t.Fatalf("save pinned run: %v", err)
	}
	edited := saved
	edited.Stages = append([]core.Stage(nil), saved.Stages...)
	edited.Stages[watch].Outcomes = append(append([]core.Outcome(nil), saved.Stages[watch].Outcomes...), core.Outcome{
		ID: core.NewUUID(), Name: "fresh commits", When: string(core.WatchEventNewChanges), Tone: core.ToneProblem, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionPark}),
	})
	next, err := c.SaveBelt(edited)
	if err != nil {
		t.Fatalf("save edited belt: %v", err)
	}
	for _, run := range h.storage.LoadRuns() {
		if run.ID != pinned.ID {
			continue
		}
		for _, outcome := range run.Stages[watch].Outcomes {
			coordCheck(t, outcome.When != string(core.WatchEventNewChanges), "pinned run gained the new outcome")
		}
	}
	started := core.NewRun(core.RunArgs{Belt: next, Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 10, Title: "Next", IsOpen: true}, Now: sessionSampleT0})
	var added *core.Outcome
	for index := range started.Stages[watch].Outcomes {
		if started.Stages[watch].Outcomes[index].When == string(core.WatchEventNewChanges) {
			added = &started.Stages[watch].Outcomes[index]
			break
		}
	}
	coordCheck(t, added != nil, "new run has no new-changes outcome")
	coordCheck(t, added.Name == "fresh commits", "name = %q", added.Name)
	coordCheck(t, added.Tone == core.ToneProblem, "tone = %v", added.Tone)
	coordCheck(t, added.Then.Action == core.ThenActionPark, "then = %v", added.Then.Action)
}

// coordWatchIndex is the index of a belt's watch stage.
func coordWatchIndex(t *testing.T, belt core.Belt) int {
	t.Helper()
	for index, stage := range belt.Stages {
		if stage.Type == core.StageTypeWatch {
			return index
		}
	}
	t.Fatal("belt has no watch stage")
	return -1
}

// TestAFailedSaveDoesNotAdvanceTheBaseline
func TestAFailedSaveDoesNotAdvanceTheBaseline(t *testing.T) {
	t.Parallel()
	f := newH5m3Watch(t, watchOptions{events: []core.WatchEvent{core.WatchEventNewChanges}})
	f.open()
	f.check()
	folder := f.h.storage.Paths.RunFolder(f.run())
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	f.gh.SetHead(201, new("b"))
	f.check()
	coordCheck(t, f.run().State == core.RunStateRunning, "state = %v, want running", f.run().State)
	coordCheck(t, len(f.newChanges()) == 0, "new changes = %v, want none", f.newChanges())
	coordCheck(t, *f.seen("acme/billing").HeadSha == "a", "saved baseline advanced")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "saved pending head set")
	pull := f.run().PullRequest()
	coordCheck(t, pull != nil, "run has no pull request")
	var cached core.WatchObservation
	var ok bool
	coordTurn(f.h.coord, func() {
		run := f.h.coord.book.runs[0]
		cached, ok = f.h.coord.watches.observations[f.h.coord.watches.observationKey(run, *pull)]
	})
	coordCheck(t, ok && *cached.HeadSha == "a", "cached baseline advanced")
	coordCheck(t, ok && cached.PendingHead == nil, "cached pending head set")
	stored := f.stored()[0]
	for _, observation := range stored.WatchObservations {
		coordCheck(t, *observation.HeadSha == "a", "stored baseline advanced")
		break
	}
	if err := os.Chmod(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	f.check()
	coordEqual(t, "new changes", f.newChanges(), []string{"new changes"})
	coordCheck(t, *f.seen("acme/billing").HeadSha == "b", "baseline is not b")
	coordCheck(t, f.seen("acme/billing").PendingHead == nil, "pending head set")
	coordCheck(t, f.stored()[0].State == core.RunStateHeld, "saved state = %v, want held", f.stored()[0].State)
}
