// Tests of session identity: runs on different repositories, or on later review requests, never share
// an agent session.

package coordinator

import (
	"context"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestRepositoriesWithTheSameNameHaveSeparateSessions
func TestRepositoriesWithTheSameNameHaveSeparateSessions(t *testing.T) {
	t.Parallel()
	repos := []string{"first/web", "second/web"}
	issues := make([]core.Subject, 0, len(repos))
	for _, repo := range repos {
		issues = append(issues, flowIssue(repo, 1, "Work", "agent"))
	}
	h := newFlowHarness(t, flowOptions{issues: issues, belts: []core.Belt{core.SampleBelt(repos, "agent", core.SampleAgents(), true)}})
	h.Tick()
	var sessions []string
	for _, run := range h.Coord().Runs() {
		if attempt := run.CurrentAttempt(); attempt != nil && attempt.Session != nil {
			sessions = append(sessions, *attempt.Session)
		}
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %v, want two", sessions)
	}
	if sessions[0] == sessions[1] {
		t.Fatalf("both runs share session %s", sessions[0])
	}
}

// TestNewReviewRequestsHaveSeparateSessionsFromEarlierRequests
func TestNewReviewRequestsHaveSeparateSessionsFromEarlierRequests(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{defaultBelt()}})
	belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "reviews", Name: "Reviews", Trigger: core.TriggerKindReviews.Fresh(), Repos: []string{"acme/web"},
		Stages: core.SampleStages(core.TriggerKindReviews, core.SampleAgents(), true),
	})
	first := core.NewRun(core.RunArgs{Belt: belt, Subject: core.Subject{
		Kind: core.SubjectKindPullRequest, Repo: "acme/web", Number: 1, Title: "Review", IsOpen: true, Occurrence: new("1"),
	}, Now: flowT0})
	var firstID, secondID string
	h.Turn(func() {
		saved := h.coord.book.add(first)
		firstID = saved.ID
		h.coord.sessions.startQueued(context.Background())
	})
	first = *h.Coord().Run(firstID)
	h.Turn(func() {
		h.coord.book.apply(firstID, core.Event{Kind: core.EventKindAbort}, nil, nil, nil)
		// A completed request's continuation can still be alive when the
		// next request starts its first attempt at the same stage.
		h.coord.book.update(firstID, nil, nil, func(r *core.Run) { r.State = core.RunStateShipped })
	})
	subject := first.Subject
	subject.Occurrence = new("2")
	h.Turn(func() {
		saved := h.coord.book.add(core.NewRun(core.RunArgs{Belt: belt, Subject: subject, Now: flowT0}))
		secondID = saved.ID
		h.coord.sessions.startQueued(context.Background())
	})
	second := h.Coord().Run(secondID)
	if second == nil {
		t.Fatal("the second request has no run")
	}
	if first.Attempts[0].Session == nil || second.CurrentAttempt().Session == nil {
		t.Fatal("a request has no session, so the comparison would prove nothing")
	}
	if core.SamePointee(first.Attempts[0].Session, second.CurrentAttempt().Session) {
		t.Fatal("the second request shares the first request's session")
	}
}
