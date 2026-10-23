// PullRequest and its nested
// State, CheckState and Review types.

package core

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// PullRequestState is the state of a pull request.
type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "open"
	PullRequestStateMerged PullRequestState = "merged"
	PullRequestStateClosed PullRequestState = "closed"
)

// PullRequestCheckState is the state of a pull request's checks.
type PullRequestCheckState string

const (
	PullRequestCheckStatePending PullRequestCheckState = "pending"
	PullRequestCheckStatePassing PullRequestCheckState = "passing"
	PullRequestCheckStateFailing PullRequestCheckState = "failing"
)

// PullRequestReviewState is the state of one review.
type PullRequestReviewState string

const (
	PullRequestReviewStateApproved         PullRequestReviewState = "approved"
	PullRequestReviewStateChangesRequested PullRequestReviewState = "changesRequested"
	PullRequestReviewStateCommented        PullRequestReviewState = "commented"
	PullRequestReviewStateDismissed        PullRequestReviewState = "dismissed"
	PullRequestReviewStateOther            PullRequestReviewState = "other"
)

// PullRequestReview is one review of a pull request.
type PullRequestReview struct {
	ID          *string                `json:"id,omitzero"`
	Author      string                 `json:"author"`
	State       PullRequestReviewState `json:"state"`
	Commit      *string                `json:"commit,omitzero"`
	SubmittedAt *jsonx.Date            `json:"submittedAt,omitzero"`
}

// PullRequestKeyedReview is one approval with its key, as KeyedApprovals returns
// it.
type PullRequestKeyedReview struct {
	Key    string
	Review PullRequestReview
}

// PullRequest is a pull request found by a watch, with its repository and latest state.
//
// Encoding: synthesized (repo, number, url, state, headSha, baseRef, checks,
// checkFailures, checkTotal, conflicted, behind and reviews; the optional ones
// are omitted when nil). Decoding is custom: see UnmarshalJSON.
type PullRequest struct {
	Repo          string                 `json:"repo"`
	Number        int                    `json:"number"`
	URL           string                 `json:"url"`
	State         PullRequestState       `json:"state"`
	HeadSha       *string                `json:"headSha,omitzero"`
	BaseRef       *string                `json:"baseRef,omitzero"`
	Checks        *PullRequestCheckState `json:"checks,omitzero"`
	CheckFailures *int                   `json:"checkFailures,omitzero"`
	CheckTotal    *int                   `json:"checkTotal,omitzero"`
	Conflicted    *bool                  `json:"conflicted,omitzero"`
	Behind        *int                   `json:"behind,omitzero"`
	Reviews       []PullRequestReview    `json:"reviews,omitzero"`
}

// HeadReviews are the reviews on the pull request's current head commit.
func (p PullRequest) HeadReviews() []PullRequestReview {
	if p.HeadSha == nil {
		return nil
	}
	var out []PullRequestReview
	for _, review := range p.Reviews {
		if review.Commit != nil && *review.Commit == *p.HeadSha {
			out = append(out, review)
		}
	}
	return out
}

// ReviewKey is the key of a review: its id, or a fallback from its place in
// Reviews (before any filter) and its author and submission time.
func (p PullRequest) ReviewKey(review PullRequestReview, index int) string {
	if review.ID != nil {
		return *review.ID
	}
	var submitted float64
	if review.SubmittedAt != nil {
		submitted = reviewUnixSeconds(review.SubmittedAt.Time)
	}
	return strconv.Itoa(index) + ":" + review.Author + ":" + reviewDouble(submitted)
}

// KeyedApprovals is every review whose current state is approved, on any commit,
// each with its key.
func (p PullRequest) KeyedApprovals() []PullRequestKeyedReview {
	var out []PullRequestKeyedReview
	for index, review := range p.Reviews {
		if review.State == PullRequestReviewStateApproved {
			out = append(out, PullRequestKeyedReview{Key: p.ReviewKey(review, index), Review: review})
		}
	}
	return out
}

// LatestReviews is the latest head review per author, sorted by author.
func (p PullRequest) LatestReviews() []PullRequestReview {
	latest := map[string]PullRequestReview{}
	for _, review := range p.HeadReviews() {
		latest[LowercasedFull(review.Author)] = review
	}
	out := make([]PullRequestReview, 0, len(latest))
	for _, review := range latest {
		out = append(out, review)
	}
	slices.SortFunc(out, func(a, b PullRequestReview) int { return strings.Compare(a.Author, b.Author) })
	return out
}

// reviewUnixSeconds is a time as seconds since 1970, with
// their fraction.
func reviewUnixSeconds(t time.Time) float64 {
	return float64(t.Unix()) + float64(t.Nanosecond())/1e9
}

// reviewDouble formats a float as the shortest form
// that reads back, with ".0" when it has no fraction.
func reviewDouble(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
