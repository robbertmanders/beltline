package core

import (
	"reflect"
	"strconv"
	"testing"
)

// rlIssue is an issue subject in repo, numbered number, with url (the GitHub issue URL when url is nil).
func rlIssue(repo string, number int, url *string) Subject {
	link := ValueOr(url, "https://github.com/"+repo+"/issues/"+strconv.Itoa(number))
	return Subject{Kind: SubjectKindIssue, Repo: repo, Number: number, Title: "Issue " + strconv.Itoa(number), URL: link}
}

// rlReview is a review subject: a pull request with its own URL.
func rlReview(repo string, number int, url string) Subject {
	return Subject{Kind: SubjectKindPullRequest, Repo: repo, Number: number, Title: "Review " + strconv.Itoa(number), URL: url}
}

// rlPull is a pull request in repo, numbered number, with url in state.
func rlPull(repo string, number int, url string, state PullRequestState) PullRequest {
	return PullRequest{Number: number, URL: url, State: state, Repo: repo}
}

// rlOpen is the open state of a pull request, for the cases that do not name one.
const rlOpen = PullRequestStateOpen

// rlAmbiguous is the set of repositories whose names need the full owner in the links tests.
var rlAmbiguous = map[string]struct{}{"beltline": {}}

// rlTitles, rlURLs, rlRefs and rlDetails are the fields of each target, in order.
func rlTitles(targets []RunLinksTarget) []string {
	out := []string{}
	for _, target := range targets {
		out = append(out, target.Title)
	}
	return out
}

func rlURLs(targets []RunLinksTarget) []string {
	out := []string{}
	for _, target := range targets {
		out = append(out, target.URL)
	}
	return out
}

func rlRefs(targets []RunLinksTarget) []string {
	out := []string{}
	for _, target := range targets {
		out = append(out, target.Reference)
	}
	return out
}

func rlDetails(targets []RunLinksTarget) []string {
	out := []string{}
	for _, target := range targets {
		out = append(out, target.Detail())
	}
	return out
}

func rlIDs(targets []RunLinksTarget) []string {
	out := []string{}
	for _, target := range targets {
		out = append(out, target.ID)
	}
	return out
}

// TestNoUrlsOfferNothing
func TestNoUrlsOfferNothing(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/beltline", 1, new(""))
	if got := RunLinksTargets(subject, []PullRequest{rlPull("acme/beltline", 12, "", rlOpen)}, nil); len(got) != 0 {
		t.Fatalf("links = %+v, want none", got)
	}
}

// TestSubjectOnlyAndPullRequestOnly
func TestSubjectOnlyAndPullRequestOnly(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 7, nil)
	if got := rlTitles(RunLinksTargets(subject, nil, nil)); !reflect.DeepEqual(got, []string{"Open Issue"}) {
		t.Fatalf("subject titles = %v", got)
	}
	if got := rlURLs(RunLinksTargets(subject, nil, nil)); !reflect.DeepEqual(got, []string{subject.URL}) {
		t.Fatalf("subject urls = %v", got)
	}
	bare := rlIssue("acme/app", 7, new(""))
	only := rlPull("acme/app", 9, "https://github.com/acme/app/pull/9", rlOpen)
	if got := rlTitles(RunLinksTargets(bare, []PullRequest{only}, nil)); !reflect.DeepEqual(got, []string{"Open PR"}) {
		t.Fatalf("pull titles = %v", got)
	}
	if got := rlURLs(RunLinksTargets(bare, []PullRequest{only}, nil)); !reflect.DeepEqual(got, []string{only.URL}) {
		t.Fatalf("pull urls = %v", got)
	}
}

// TestSeveralTargetsKeepSubjectThenPullRequests
func TestSeveralTargetsKeepSubjectThenPullRequests(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 7, nil)
	pulls := []PullRequest{
		rlPull("acme/app", 9, "https://github.com/acme/app/pull/9", rlOpen),
		rlPull("acme/web", 4, "https://github.com/acme/web/pull/4", rlOpen),
	}
	found := RunLinksTargets(subject, pulls, nil)
	if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open Issue", "Open PR", "Open PR"}) {
		t.Fatalf("titles = %v", got)
	}
	if got := rlURLs(found); !reflect.DeepEqual(got, []string{subject.URL, pulls[0].URL, pulls[1].URL}) {
		t.Fatalf("urls = %v", got)
	}
	if ids := rlIDs(found); len(ids) != len(uniqueStrings(ids)) {
		t.Fatalf("ids repeat: %v", ids)
	}
}

// uniqueStrings is the distinct values of ss.
func uniqueStrings(ss []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range ss {
		if _, dup := seen[s]; !dup {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// TestIssueThenPullRequestAndJiraThenSeveralPullRequests
func TestIssueThenPullRequestAndJiraThenSeveralPullRequests(t *testing.T) {
	t.Parallel()
	subject := rlIssue("robbertmanders/beltline", 232, nil)
	request := rlPull("robbertmanders/beltline", 12, "https://github.com/robbertmanders/beltline/pull/12", rlOpen)
	found := RunLinksTargets(subject, []PullRequest{request}, rlAmbiguous)
	if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open Issue", "Open PR"}) {
		t.Fatalf("titles = %v", got)
	}
	if got := rlRefs(found); !reflect.DeepEqual(got, []string{"beltline [robbertmanders]#232", "beltline [robbertmanders]#12"}) {
		t.Fatalf("references = %v", got)
	}
	if got := rlDetails(found); !reflect.DeepEqual(got, []string{"Open Issue beltline [robbertmanders]#232", "Open PR beltline [robbertmanders]#12"}) {
		t.Fatalf("details = %v", got)
	}

	ticket := Subject{Kind: SubjectKindJira, Repo: "acme/billing", Key: "PAY-31", Title: "Refund", URL: "https://acme.atlassian.net/browse/PAY-31"}
	pulls := []PullRequest{
		rlPull("acme/billing", 3, "https://github.com/acme/billing/pull/3", rlOpen),
		rlPull("acme/web", 4, "https://github.com/acme/web/pull/4", rlOpen),
	}
	jira := RunLinksTargets(ticket, pulls, nil)
	if got := rlTitles(jira); !reflect.DeepEqual(got, []string{"Open Ticket", "Open PR", "Open PR"}) {
		t.Fatalf("jira titles = %v", got)
	}
	if got := rlRefs(jira); !reflect.DeepEqual(got, []string{"PAY-31", "billing#3", "web#4"}) {
		t.Fatalf("jira references = %v", got)
	}
	if got := rlURLs(jira); !reflect.DeepEqual(got, []string{ticket.URL, pulls[0].URL, pulls[1].URL}) {
		t.Fatalf("jira urls = %v", got)
	}
}

// TestReviewPullRequestIsListedOncePreferringTheSubjectURL
func TestReviewPullRequestIsListedOncePreferringTheSubjectURL(t *testing.T) {
	t.Parallel()
	subjectURL := "https://github.com/acme/web/pull/160"
	subject := rlReview("acme/web", 160, subjectURL)
	repeated := []PullRequest{
		rlPull("acme/web", 160, subjectURL+"?w=1", PullRequestStateOpen),
		rlPull("acme/web", 160, "https://github.com/acme/web/pull/160/files", PullRequestStateMerged),
	}
	found := RunLinksTargets(subject, repeated, nil)
	if got := rlIDs(found); !reflect.DeepEqual(got, []string{"pr:acme/web#160"}) {
		t.Fatalf("ids = %v", got)
	}
	if got := rlURLs(found); !reflect.DeepEqual(got, []string{subjectURL}) {
		t.Fatalf("urls = %v", got)
	}
	if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open PR"}) {
		t.Fatalf("titles = %v", got)
	}
}

// TestEmptyURLsAreDroppedBeforeDeduplication
func TestEmptyURLsAreDroppedBeforeDeduplication(t *testing.T) {
	t.Parallel()
	subject := rlReview("acme/web", 160, "")
	first := "https://github.com/acme/web/pull/160"
	pulls := []PullRequest{
		rlPull("acme/web", 160, "", rlOpen),
		rlPull("acme/web", 160, first, PullRequestStateClosed),
		rlPull("acme/web", 160, first+"?late=1", PullRequestStateMerged),
	}
	if got := rlURLs(RunLinksTargets(subject, pulls, nil)); !reflect.DeepEqual(got, []string{first}) {
		t.Fatalf("urls = %v", got)
	}
	withSubject := rlReview("acme/web", 160, "https://github.com/acme/web/pull/160?subject=1")
	if got := rlURLs(RunLinksTargets(withSubject, pulls, nil)); !reflect.DeepEqual(got, []string{withSubject.URL}) {
		t.Fatalf("subject urls = %v", got)
	}
}

// TestRepeatedPullRequestsKeepTheFirstLinkedURL
func TestRepeatedPullRequestsKeepTheFirstLinkedURL(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 1, new(""))
	first := "https://github.com/acme/app/pull/8"
	pulls := []PullRequest{
		rlPull("acme/app", 8, first, PullRequestStateOpen),
		rlPull("acme/app", 8, first+"?dup=1", PullRequestStateMerged),
	}
	if got := rlURLs(RunLinksTargets(subject, pulls, nil)); !reflect.DeepEqual(got, []string{first}) {
		t.Fatalf("urls = %v", got)
	}
}

// TestRepositoryAndNumberKeepDistinctChoices
func TestRepositoryAndNumberKeepDistinctChoices(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/beltline", 12, nil)
	pulls := []PullRequest{
		rlPull("acme/beltline", 12, "https://github.com/acme/beltline/pull/12", rlOpen),
		rlPull("other/beltline", 12, "https://github.com/other/beltline/pull/12", rlOpen),
		rlPull("acme/beltline", 13, "https://github.com/acme/beltline/pull/13", rlOpen),
	}
	found := RunLinksTargets(subject, pulls, rlAmbiguous)
	if got := rlIDs(found); !reflect.DeepEqual(got, []string{"issue:acme/beltline#12", "pr:acme/beltline#12", "pr:other/beltline#12", "pr:acme/beltline#13"}) {
		t.Fatalf("ids = %v", got)
	}
	if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open Issue", "Open PR", "Open PR", "Open PR"}) {
		t.Fatalf("titles = %v", got)
	}
	if got := rlRefs(found); !reflect.DeepEqual(got, []string{"beltline [acme]#12", "beltline [acme]#12", "beltline [other]#12", "beltline [acme]#13"}) {
		t.Fatalf("references = %v", got)
	}
	if got := uniqueStrings(rlDetails(found)); len(got) != len(found) {
		t.Fatalf("details are not distinct: %v", rlDetails(found))
	}
}

// TestMergedAndClosedPullRequestsStayAvailable
func TestMergedAndClosedPullRequestsStayAvailable(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 1, new(""))
	pulls := []PullRequest{
		rlPull("acme/app", 2, "https://github.com/acme/app/pull/2", PullRequestStateMerged),
		rlPull("acme/app", 3, "https://github.com/acme/app/pull/3", PullRequestStateClosed),
	}
	want := []string{pulls[0].URL, pulls[1].URL}
	if got := rlURLs(RunLinksTargets(subject, pulls, nil)); !reflect.DeepEqual(got, want) {
		t.Fatalf("urls = %v, want %v", got, want)
	}
}

// TestTasksWithoutASubjectURLStillOfferPullRequests
func TestTasksWithoutASubjectURLStillOfferPullRequests(t *testing.T) {
	t.Parallel()
	for _, kind := range []SubjectKind{SubjectKindManual, SubjectKindScheduled} {
		key := "2026-09-29"
		if kind == SubjectKindManual {
			key = "3"
		}
		subject := Subject{Kind: kind, Repo: "acme/web", Key: key, Title: "Task", URL: ""}
		request := rlPull("acme/web", 5, "https://github.com/acme/web/pull/5", rlOpen)
		found := RunLinksTargets(subject, []PullRequest{request}, nil)
		if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open PR"}) {
			t.Fatalf("%s titles = %v", kind, got)
		}
		if got := rlURLs(found); !reflect.DeepEqual(got, []string{request.URL}) {
			t.Fatalf("%s urls = %v", kind, got)
		}
	}
}

// TestEverySubjectKindKeepsItsWebRowIncludingWhenTheURLIsMissing
func TestEverySubjectKindKeepsItsWebRowIncludingWhenTheURLIsMissing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		subject Subject
		title   string
	}{
		{rlIssue("acme/app", 4, nil), "Open Issue"},
		{rlReview("acme/app", 8, "https://github.com/acme/app/pull/8"), "Open PR"},
		{Subject{Kind: SubjectKindJira, Repo: "acme/app", Key: "PAY-9", Title: "Ticket", URL: "https://example.test/PAY-9"}, "Open Ticket"},
		{Subject{Kind: SubjectKindManual, Repo: "acme/app", Key: "3", Title: "Task", URL: "https://example.test/t3"}, "Open Task"},
		{Subject{Kind: SubjectKindScheduled, Repo: "acme/app", Key: "2026-09-29", Title: "Sweep", URL: "https://example.test/sweep"}, "Open Task"},
	}
	for _, c := range cases {
		found := RunLinksTargets(c.subject, nil, nil)
		if got := rlTitles(found); !reflect.DeepEqual(got, []string{c.title}) {
			t.Fatalf("%s titles = %v, want %s", c.subject.Kind, got, c.title)
		}
		if got := rlURLs(found); !reflect.DeepEqual(got, []string{c.subject.URL}) {
			t.Fatalf("%s urls = %v", c.subject.Kind, got)
		}
		bare := c.subject
		bare.URL = ""
		if got := RunLinksTargets(bare, nil, nil); len(got) != 0 {
			t.Fatalf("%s without a URL offers %+v", c.subject.Kind, got)
		}
	}
}

// TestTitlesAndDetailsCoverEverySubjectKind
func TestTitlesAndDetailsCoverEverySubjectKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		subject   Subject
		title     string
		reference string
	}{
		{rlIssue("acme/app", 4, nil), "Open Issue", "app#4"},
		{rlReview("acme/app", 8, "https://github.com/acme/app/pull/8"), "Open PR", "app#8"},
		{Subject{Kind: SubjectKindJira, Repo: "acme/app", Key: "PAY-9", Title: "Ticket", URL: "https://example.test/PAY-9"}, "Open Ticket", "PAY-9"},
		{Subject{Kind: SubjectKindManual, Repo: "acme/app", Key: "3", Title: "Task", URL: "https://example.test/t3"}, "Open Task", "T3"},
		{Subject{Kind: SubjectKindScheduled, Repo: "acme/app", Key: "2026-09-29", Title: "Sweep", URL: "https://example.test/sweep"}, "Open Task", "09-29"},
	}
	for _, c := range cases {
		found := RunLinksTargets(c.subject, nil, nil)
		if got := rlTitles(found); !reflect.DeepEqual(got, []string{c.title}) {
			t.Fatalf("%s titles = %v, want %s", c.subject.Kind, got, c.title)
		}
		if got := rlRefs(found); !reflect.DeepEqual(got, []string{c.reference}) {
			t.Fatalf("%s references = %v, want %s", c.subject.Kind, got, c.reference)
		}
		if got := rlDetails(found); !reflect.DeepEqual(got, []string{c.title + " " + c.reference}) {
			t.Fatalf("%s details = %v", c.subject.Kind, got)
		}
	}
}

// TestRepeatedPullRequestLabelsStayDistinct
func TestRepeatedPullRequestLabelsStayDistinct(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 1, new(""))
	pulls := []PullRequest{
		rlPull("robbertmanders/beltline", 12, "https://github.com/robbertmanders/beltline/pull/12", rlOpen),
		rlPull("acme/beltline", 12, "https://github.com/acme/beltline/pull/12", rlOpen),
	}
	found := RunLinksTargets(subject, pulls, rlAmbiguous)
	if got := rlTitles(found); !reflect.DeepEqual(got, []string{"Open PR", "Open PR"}) {
		t.Fatalf("titles = %v", got)
	}
	if got := rlDetails(found); !reflect.DeepEqual(got, []string{"Open PR beltline [robbertmanders]#12", "Open PR beltline [acme]#12"}) {
		t.Fatalf("details = %v", got)
	}
}

// TestLinksIgnoreRunState
func TestLinksIgnoreRunState(t *testing.T) {
	t.Parallel()
	subject := rlIssue("acme/app", 4, nil)
	request := rlPull("acme/app", 9, "https://github.com/acme/app/pull/9", PullRequestStateMerged)
	expected := RunLinksTargets(subject, []PullRequest{request}, nil)
	for _, state := range AllRunState {
		run := NewRun(RunArgs{Belt: SampleBelt([]string{subject.Repo}, "agent", SampleAgents(), true), Subject: subject, Now: testT0})
		run.State = state
		run.PullRequests = []PullRequest{request}
		if got := RunLinksTargets(run.Subject, run.PullRequests, nil); !reflect.DeepEqual(got, expected) {
			t.Fatalf("state %s links = %+v, want %+v", state, got, expected)
		}
	}
}
