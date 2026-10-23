// The watch cards of a stage panel.

package core

// StagePanelWatchCard is one pull request a watch waits on, as a stage panel shows it.
type StagePanelWatchCard struct {
	ID string `json:"id"`
	// Title is "owner/repo#12", shortened where two belts share a name.
	Title string `json:"title"`
	// Subtitle is the run's title and branch, as inline spans: "Title · branch " then the branch as code.
	Subtitle []InlineSpan `json:"subtitle"`
	// State is "The pull request is merged." or "It waits for a review and a merge."
	State string `json:"state"`
	URL   string `json:"url"`
}

// StagePanelWatchCardsOf is one card per pull request the run's watch waits on, in the run's order.
// ambiguous is the repository labels that need a name in full.
func StagePanelWatchCardsOf(run Run, ambiguous map[string]struct{}) []StagePanelWatchCard {
	branch := ValueOr(run.Branch, RunBranch(run))
	cards := make([]StagePanelWatchCard, 0, len(run.PullRequests))
	for _, pull := range run.PullRequests {
		state := "It waits for a review and a merge."
		if pull.State != PullRequestStateOpen {
			state = "The pull request is " + string(pull.State) + "."
		}
		cards = append(cards, StagePanelWatchCard{
			ID:    pull.Repo,
			Title: RepoLabelPullRequest(pull.Repo, pull.Number, ambiguous),
			Subtitle: []InlineSpan{
				{Kind: InlineSpanKindText, Text: run.Subject.Title + " · branch "},
				{Kind: InlineSpanKindCode, Text: branch},
			},
			State: state,
			URL:   pull.URL,
		})
	}
	return cards
}
