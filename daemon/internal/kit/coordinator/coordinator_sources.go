package coordinator

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
)

// FateKind is the case of a Fate.
type FateKind string

const (
	// FateChanged: still there, perhaps edited.
	FateChanged FateKind = "changed"
	// FateGone: closed, done, or no longer asked for: the run can stop.
	FateGone FateKind = "gone"
)

// Fate is what became of the thing a run is about, as a source sees it now
// . Subject is set for FateChanged; Reason for FateGone.
type Fate struct {
	Kind    FateKind
	Subject core.Subject
	Reason  string
}

// Source is a trigger that checks somewhere for work every so often: GitHub
// issues with a label, review requests, or a Jira query. Intake runs the same
// loop over each; what differs between them lives here, in a Source.
// Implementations are called with c.mu held by Intake,
// which suspends around each call that awaits.
type Source interface {
	// scopes lists the places it checks, each on its own so one that fails leaves
	// the others' runs alone. Empty when the belt is not set up enough to check.
	scopes(belt core.Belt) []string
	// beginCheck refreshes account context once before checking a belt's scopes.
	beginCheck(ctx context.Context) error
	// adoptingIdentity pins identity metadata absent from older saved subjects.
	adoptingIdentity(subject core.Subject) core.Subject
	// place says where scope is, for a note when checking it fails.
	place(scope string, belt core.Belt) string
	// list returns the work it lists in scope now.
	list(ctx context.Context, scope string, belt core.Belt) ([]core.Subject, error)
	// watches reports whether a run on the belt is one this source watches in scope.
	watches(run core.Run, scope string) bool
	// prepare fills in what tells a new subject's runs apart, such as the review request it answers.
	prepare(ctx context.Context, subject core.Subject, belt core.Belt) (core.Subject, error)
	// blockers lists the open issues or tickets that block a listed subject.
	blockers(ctx context.Context, subject core.Subject) ([]string, error)
	// admits reports whether a listed subject that has no run yet should start one.
	admits(subject core.Subject) bool
	// fate says what a listed subject means for the run already working on it.
	fate(subject core.Subject, belt core.Belt) Fate
	// missing says what became of a run's subject now that the listing no longer has it.
	missing(ctx context.Context, run core.Run, belt core.Belt) (Fate, error)
}

// sourceDefaults are the default implementations of Source.
// Sources embed it and override what they need.
type sourceDefaults struct{}

// beginCheck does nothing by default.
// Lock: none.
func (sourceDefaults) beginCheck(ctx context.Context) error { return nil }

// adoptingIdentity returns the subject unchanged by default.
// Lock: none.
func (sourceDefaults) adoptingIdentity(subject core.Subject) core.Subject { return subject }

// prepare returns the subject unchanged by default.
// Lock: none. Suspends: none (the default does not await).
func (sourceDefaults) prepare(ctx context.Context, subject core.Subject, belt core.Belt) (core.Subject, error) {
	return subject, nil
}

// blockers returns the subject's own listed blockers by default.
// Lock: none.
func (sourceDefaults) blockers(ctx context.Context, subject core.Subject) ([]string, error) {
	return append([]string{}, subject.Blockers...), nil
}

// admits reports whether the subject is open, by default.
// Lock: none.
func (sourceDefaults) admits(subject core.Subject) bool { return subject.IsOpen }

// fate reports the subject as changed, by default.
// Lock: none.
func (sourceDefaults) fate(subject core.Subject, belt core.Belt) Fate {
	return Fate{Kind: FateChanged, Subject: subject}
}

// IssuesSource lists open GitHub issues with the belt's label, one run per issue.
type IssuesSource struct {
	sourceDefaults
	github github.GitHub
}

// filter returns the belt's issue trigger filter.
// Lock: none.
func (s IssuesSource) filter(belt core.Belt) core.IssueFilter {
	if trigger := belt.Trigger(); trigger.Issues != nil {
		return *trigger.Issues
	}
	return core.DefaultIssueFilter()
}

// scopes are the belt's repositories.
// Lock: none.
func (s IssuesSource) scopes(belt core.Belt) []string { return belt.Repos() }

// place names the repository.
// Lock: none.
func (s IssuesSource) place(scope string, belt core.Belt) string { return scope }

// blockers asks GitHub which open issues block the subject.
// Lock: none. Suspends: github.blockers.
func (s IssuesSource) blockers(ctx context.Context, subject core.Subject) ([]string, error) {
	return s.github.Blockers(ctx, subject.Repo, subject.Number)
}

// list asks GitHub for the open issues with the belt's label, assigned to you when the filter says so.
// Lock: none. Suspends: github.issues.
func (s IssuesSource) list(ctx context.Context, scope string, belt core.Belt) ([]core.Subject, error) {
	filter := s.filter(belt)
	var assignee *string
	if filter.Assignee == core.TriggerAssigneeMe {
		me := "@me"
		assignee = &me
	}
	return s.github.Issues(ctx, scope, filter.Label, assignee)
}

// watches reports whether run is an issue in scope.
// Lock: none.
func (s IssuesSource) watches(run core.Run, scope string) bool {
	return run.Subject.Kind == core.SubjectKindIssue && run.Subject.Repo == scope
}

// fate reports the issue as changed, or gone when it is closed or unlabelled.
// Lock: none.
func (s IssuesSource) fate(subject core.Subject, belt core.Belt) Fate {
	if !subject.IsOpen {
		return Fate{Kind: FateGone, Reason: "the issue was closed"}
	}
	if !slices.Contains(subject.Labels, s.filter(belt).Label) {
		return Fate{Kind: FateGone, Reason: "the issue lost its label"}
	}
	return Fate{Kind: FateChanged, Subject: subject}
}

// missing reads the issue again, since it is no longer in the listing.
// Lock: none. Suspends: github.issue.
func (s IssuesSource) missing(ctx context.Context, run core.Run, belt core.Belt) (Fate, error) {
	issue, err := s.github.Issue(ctx, run.Subject.Repo, run.Subject.Number)
	if err != nil {
		return Fate{}, err
	}
	return s.fate(issue, belt), nil
}

// ReviewsSource lists pull requests that ask for your review. Each request is its
// own run: once a run has answered one, only asking again starts another.
type ReviewsSource struct {
	sourceDefaults
	github github.GitHub
	// login is refreshed once per belt check, shared by all its repository scopes.
	// loginMu guards it: beginCheck writes it on the main actor, but list and prepare
	// read it while their GitHub call runs without the lock, and a second belt check
	// can overlap the first (the tick and a check now both poll).
	loginMu sync.Mutex
	login   *string
}

// newReviewsSource builds the review requests source.
// Lock: none.
func newReviewsSource(github github.GitHub) *ReviewsSource { return &ReviewsSource{github: github} }

// beginCheck reads your login once per belt check.
// Lock: none. Suspends: github.login.
func (s *ReviewsSource) beginCheck(ctx context.Context) error {
	s.setLogin(nil)
	login, err := s.github.Login(ctx)
	if err != nil {
		return err
	}
	s.setLogin(&login)
	return nil
}

// setLogin stores the login read for this check, or nil to clear it.
// Lock: none; loginMu guards the field.
func (s *ReviewsSource) setLogin(login *string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	s.login = login
}

// currentLogin is a copy of the stored login, or nil.
// Lock: none; loginMu guards the field.
func (s *ReviewsSource) currentLogin() *string {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	return core.CopyPointer(s.login)
}

// adoptingIdentity gives a pull request saved without a reviewer your login, the reviewer the
// run was admitted for.
// Lock: none.
func (s *ReviewsSource) adoptingIdentity(subject core.Subject) core.Subject {
	if subject.Kind == core.SubjectKindPullRequest && subject.Reviewer == nil {
		subject.Reviewer = s.currentLogin()
	}
	return subject
}

// teams reports whether the belt asks for review requests of your teams too.
// Lock: none.
func (s *ReviewsSource) teams(belt core.Belt) bool {
	if trigger := belt.Trigger(); trigger.Reviews != nil {
		return trigger.Reviews.Reviewers == core.TriggerReviewersMeAndTeams
	}
	return false
}

// scopes are the belt's repositories.
// Lock: none.
func (s *ReviewsSource) scopes(belt core.Belt) []string { return belt.Repos() }

// place names the reviews in the repository.
// Lock: none.
func (s *ReviewsSource) place(scope string, belt core.Belt) string {
	return "reviews in " + scope
}

// list asks GitHub for the review requests in scope. Which request each asks is
// only read for new ones, in prepare.
// Lock: none. Suspends: github.reviewRequests.
func (s *ReviewsSource) list(ctx context.Context, scope string, belt core.Belt) ([]core.Subject, error) {
	pulls, err := s.github.ReviewRequests(ctx, scope, s.teams(belt))
	if err != nil {
		return nil, err
	}
	out := make([]core.Subject, 0, len(pulls))
	for _, pull := range pulls {
		pull.Occurrence = nil
		pull.Reviewer = s.currentLogin()
		out = append(out, pull)
	}
	return out, nil
}

// watches reports whether run is a pull request in scope.
// Lock: none.
func (s *ReviewsSource) watches(run core.Run, scope string) bool {
	return run.Subject.Kind == core.SubjectKindPullRequest && run.Subject.Repo == scope
}

// prepare reads the latest review request of a new subject, which tells its runs apart.
// Lock: none. Suspends: github.latestReviewRequest.
func (s *ReviewsSource) prepare(ctx context.Context, subject core.Subject, belt core.Belt) (core.Subject, error) {
	pull := s.adoptingIdentity(subject)
	reviewer, err := s.reviewerOf(pull)
	var occurrence *string
	if err == nil {
		occurrence, err = s.github.LatestReviewRequest(ctx, pull.Repo, pull.Number, reviewer, s.teams(belt))
	}
	if err != nil {
		return core.Subject{}, core.NewFailure("Could not read the review request on " + pull.Repo + "#" + strconv.Itoa(pull.Number) + ": " + err.Error())
	}
	if occurrence == nil {
		zero := "0"
		occurrence = &zero
	}
	pull.Occurrence = occurrence
	return pull, nil
}

// missing reads the pull request and its latest review request. Absence from
// your own listing says nothing about another account's request.
// Lock: none. Suspends: github.pullRequest, github.latestReviewRequest.
func (s *ReviewsSource) missing(ctx context.Context, run core.Run, belt core.Belt) (Fate, error) {
	subject := run.Subject
	wrap := func(err error) error {
		return core.NewFailure("Could not read the review request on " + subject.Repo + "#" + strconv.Itoa(subject.Number) + ": " + err.Error())
	}
	gone := Fate{Kind: FateGone, Reason: "the pull request was closed or no longer asks for your review"}
	pull, err := s.github.PullRequest(ctx, subject.Repo, subject.Number)
	if err != nil {
		return Fate{}, wrap(err)
	}
	if pull.State != core.PullRequestStateOpen {
		return gone, nil
	}
	reviewer, err := s.reviewerOf(subject)
	if err != nil {
		return Fate{}, wrap(err)
	}
	request, err := s.github.LatestReviewRequest(ctx, subject.Repo, subject.Number, reviewer, s.teams(belt))
	if err != nil {
		return Fate{}, wrap(err)
	}
	if request == nil {
		return gone, nil
	}
	return Fate{Kind: FateChanged, Subject: subject}, nil
}

// reviewerOf is the reviewer a subject's request is read for: the subject's own, else your login.
// Lock: none.
func (s *ReviewsSource) reviewerOf(subject core.Subject) (string, error) {
	if subject.Reviewer != nil {
		return *subject.Reviewer, nil
	}
	return s.myLogin()
}

// myLogin returns your login, from the once-per-check read.
// Lock: none.
func (s *ReviewsSource) myLogin() (string, error) {
	login := s.currentLogin()
	if login == nil {
		return "", core.NewFailure("Could not read your GitHub login")
	}
	return *login, nil
}

// JiraSource lists tickets that match the belt's query, worked on in its
// repository. A ticket that leaves the query keeps its run; the run stops once
// the ticket is done.
type JiraSource struct {
	sourceDefaults
	jira jira.Jira
}

// jql is the belt's query.
// Lock: none.
func (s JiraSource) jql(belt core.Belt) string {
	if trigger := belt.Trigger(); trigger.Jira != nil {
		return strings.TrimSpace(trigger.Jira.Jql)
	}
	return ""
}

// scopes is one scope per query, not per repository; the first anchors the subject.
// Lock: none.
func (s JiraSource) scopes(belt core.Belt) []string {
	// One scope per query, not per repository; the first anchors the subject.
	repos := belt.Repos()
	if s.jql(belt) == "" || len(repos) == 0 {
		return []string{}
	}
	return []string{repos[0]}
}

// place names the belt's Jira query.
// Lock: none.
func (s JiraSource) place(scope string, belt core.Belt) string { return "Jira for " + belt.Name }

// list runs the belt's query.
// Lock: none. Suspends: jira.search.
func (s JiraSource) list(ctx context.Context, scope string, belt core.Belt) ([]core.Subject, error) {
	tickets, err := s.jira.Search(ctx, s.jql(belt))
	if err != nil {
		return nil, err
	}
	for i := range tickets {
		tickets[i].Repo = scope
	}
	return tickets, nil
}

// watches reports whether run is a Jira ticket.
// Lock: none.
func (s JiraSource) watches(run core.Run, scope string) bool {
	return run.Subject.Kind == core.SubjectKindJira
}

// fate reports the ticket as changed, or gone when it is done.
// Lock: none.
func (s JiraSource) fate(subject core.Subject, belt core.Belt) Fate {
	if subject.IsOpen {
		return Fate{Kind: FateChanged, Subject: subject}
	}
	return Fate{Kind: FateGone, Reason: "the ticket is done"}
}

// missing reads the ticket again, since it is no longer in the query.
// Lock: none. Suspends: jira.ticket.
func (s JiraSource) missing(ctx context.Context, run core.Run, belt core.Belt) (Fate, error) {
	ticket, err := s.jira.Ticket(ctx, run.Subject.Key)
	if err != nil {
		return Fate{}, err
	}
	ticket.Repo = run.Subject.Repo
	return s.fate(ticket, belt), nil
}
