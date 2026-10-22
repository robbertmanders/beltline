// The GitHub protocol over the gh command, and the JSON shapes it reads.

package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
)

// GHCLI talks to GitHub through the gh command.
type GHCLI struct {
	Shell shell.Runner
}

// NewGHCLI creates the gh adapter over shell. A nil runner is the process runner.
func NewGHCLI(sh shell.Runner) GHCLI {
	if sh == nil {
		sh = shell.ProcessRunner{}
	}
	return GHCLI{Shell: sh}
}

var _ GitHub = GHCLI{}

// issueFields is the --json list an issue read asks for.
const issueFields = "number,title,body,labels,url,state,assignees"

// nameJSON is a label, assignee or user as gh writes it: each key optional.
type nameJSON struct {
	Name  *string `json:"name,omitzero"`
	Login *string `json:"login,omitzero"`
}

// issueJSON is gh's issue object.
type issueJSON struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      *string    `json:"body,omitzero"`
	Labels    []nameJSON `json:"labels,omitzero"`
	URL       *string    `json:"url,omitzero"`
	State     *string    `json:"state,omitzero"`
	Assignees []nameJSON `json:"assignees,omitzero"`
}

// issue is the subject an issue object reads as.
func (j issueJSON) issue(repo string) core.Subject {
	open := strings.ToUpper(core.ValueOr(j.State, "OPEN")) == "OPEN"
	return core.Subject{
		Kind: core.SubjectKindIssue, Repo: repo, Number: j.Number, Title: j.Title,
		Body: core.ValueOf(j.Body), Labels: compactNames(j.Labels, true), URL: core.ValueOf(j.URL),
		IsOpen: open, Assignees: compactNames(j.Assignees, false), Blockers: []string{},
	}
}

// compactNames returns the non-nil names or logins, in order.
func compactNames(list []nameJSON, name bool) []string {
	out := []string{}
	for _, n := range list {
		v := n.Login
		if name {
			v = n.Name
		}
		if v != nil {
			out = append(out, *v)
		}
	}
	return out
}

// reviewFields is the --json list a review request read asks for.
const reviewFields = "number,title,body,labels,url,state,baseRefName,isCrossRepository"

// reviewJSON is gh's pull request object in a review request list.
type reviewJSON struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        *string    `json:"body,omitzero"`
	Labels      []nameJSON `json:"labels,omitzero"`
	URL         *string    `json:"url,omitzero"`
	BaseRefName *string    `json:"baseRefName,omitzero"`
	// IsCrossRepository is a pull request from another repository, such as a fork.
	IsCrossRepository *bool `json:"isCrossRepository,omitzero"`
}

// fromThisRepository reports whether the pull request's branch lives in the
// repository itself. A fork's, or one gh did not say, is not: its checkout is
// code anyone can write, and the agents run it with your sign-ins.
func (j reviewJSON) fromThisRepository() bool {
	return j.IsCrossRepository != nil && !*j.IsCrossRepository
}

func (j reviewJSON) subject(repo string) core.Subject {
	base := j.BaseRefName
	return core.Subject{
		Kind: core.SubjectKindPullRequest, Repo: repo, Number: j.Number, Title: j.Title,
		Body: core.ValueOf(j.Body), Labels: compactNames(j.Labels, true), URL: core.ValueOf(j.URL),
		IsOpen: true, Assignees: []string{}, Base: base, Blockers: []string{},
	}
}

// gh runs gh with arguments and returns its stdout, or the Failure that names
// the first two arguments and gh's message.
func (g GHCLI) gh(ctx context.Context, arguments []string, input *string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result := shell.Bounded(ctx, g.Shell, "gh", arguments, "", input)
	if !result.OK() {
		prefix := arguments
		if len(prefix) > 2 {
			prefix = prefix[:2]
		}
		return "", core.NewFailure(fmt.Sprintf("gh %s: %s", strings.Join(prefix, " "), result.Message()))
	}
	return result.Stdout, nil
}

// decode reads gh's answer into v, or a Failure saying it could not.
func decodeGH(text string, v any) error {
	if err := json.Unmarshal([]byte(text), v); err != nil {
		return core.NewFailure("Could not read gh's answer: " + err.Error())
	}
	return nil
}

// Issues lists the open issues with label, assigned to assignee when it is set.
func (g GHCLI) Issues(ctx context.Context, repo, label string, assignee *string) ([]core.Subject, error) {
	arguments := []string{"issue", "list", "-R", repo, "--label", label, "--state", "open", "--limit", "200", "--json", issueFields}
	if assignee != nil && *assignee != "" {
		arguments = append(arguments, "--assignee", *assignee)
	}
	text, err := g.gh(ctx, arguments, nil)
	if err != nil {
		return nil, err
	}
	var list []issueJSON
	if err := decodeGH(text, &list); err != nil {
		return nil, err
	}
	out := []core.Subject{}
	for _, j := range list {
		out = append(out, j.issue(repo))
	}
	return out, nil
}

// Issue reads one issue.
func (g GHCLI) Issue(ctx context.Context, repo string, number int) (core.Subject, error) {
	text, err := g.gh(ctx, []string{"issue", "view", strconv.Itoa(number), "-R", repo, "--json", issueFields}, nil)
	if err != nil {
		return core.Subject{}, err
	}
	var j issueJSON
	if err := decodeGH(text, &j); err != nil {
		return core.Subject{}, err
	}
	return j.issue(repo), nil
}

// Comment posts body on an issue.
func (g GHCLI) Comment(ctx context.Context, repo string, number int, body string) error {
	_, err := g.gh(ctx, []string{"issue", "comment", strconv.Itoa(number), "-R", repo, "--body-file", "-"}, &body)
	return err
}

// ReviewRequests lists the open pull requests that ask for your review, and with
// teams, the ones that ask a team you are on. Pull requests from forks are left
// out (see fromThisRepository).
func (g GHCLI) ReviewRequests(ctx context.Context, repo string, teams bool) ([]core.Subject, error) {
	// `review-requested` also matches requests to your teams; `user-review-requested` only you by name.
	query := "user-review-requested:@me"
	if teams {
		query = "review-requested:@me"
	}
	arguments := []string{"pr", "list", "-R", repo, "--search", query, "--state", "open", "--limit", "100", "--json", reviewFields}
	text, err := g.gh(ctx, arguments, nil)
	if err != nil {
		return nil, err
	}
	var list []reviewJSON
	if err := decodeGH(text, &list); err != nil {
		return nil, err
	}
	out := []core.Subject{}
	for _, j := range list {
		if j.fromThisRepository() {
			out = append(out, j.subject(repo))
		}
	}
	return out, nil
}

// reviewRequestFilter is the jq filter over the timeline: one line per request,
// "<id> <login> <team>".
const reviewRequestFilter = `.[] | select(.event == "review_requested") | "\(.id) \(.requested_reviewer.login // "") \(.requested_team.slug // "")"`

// LatestReviewRequest is the id of the latest review request for reviewer, or
// for a team when teams is set; nil when there is none.
func (g GHCLI) LatestReviewRequest(ctx context.Context, repo string, number int, reviewer string, teams bool) (*string, error) {
	text, err := g.gh(ctx, []string{"api", "repos/" + repo + "/issues/" + strconv.Itoa(number) + "/timeline", "--paginate", "--jq", reviewRequestFilter}, nil)
	if err != nil {
		return nil, err
	}
	return latestRequest(text, reviewer, teams), nil
}

// latestRequest is the last request line, "<id> <login> <team>", for reviewer or a team.
func latestRequest(text, reviewer string, teams bool) *string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if line == "" {
			continue // the split omits empty subsequences.
		}
		parts := strings.Split(line, " ")
		if len(parts) < 3 || parts[0] == "" {
			continue
		}
		if strings.EqualFold(parts[1], reviewer) {
			return new(parts[0])
		}
		if teams && parts[2] != "" {
			return new(parts[0])
		}
	}
	return nil
}

// CommentOnPullRequest posts body on a pull request.
func (g GHCLI) CommentOnPullRequest(ctx context.Context, repo string, number int, body string) error {
	_, err := g.gh(ctx, []string{"pr", "comment", strconv.Itoa(number), "-R", repo, "--body-file", "-"}, &body)
	return err
}

// Login is the signed-in user's login.
func (g GHCLI) Login(ctx context.Context) (string, error) {
	text, err := g.gh(ctx, []string{"api", "user", "-q", ".login"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// DefaultBranch is the repository's default branch name.
func (g GHCLI) DefaultBranch(ctx context.Context, repo string) (string, error) {
	text, err := g.gh(ctx, []string{"repo", "view", repo, "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

// CommitsBehind is how many commits base has that head doesn't.
func (g GHCLI) CommitsBehind(ctx context.Context, repo, base, head string) (int, error) {
	text, err := g.gh(ctx, []string{"api", "repos/" + repo + "/compare/" + base + "..." + head, "-q", ".behind_by"}, nil)
	if err != nil {
		return 0, err
	}
	count, convErr := strconv.Atoi(strings.TrimSpace(text))
	if convErr != nil || count < 0 {
		return 0, core.NewFailure("Could not read gh's behind count")
	}
	return count, nil
}
