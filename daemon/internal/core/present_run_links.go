package core

import "strconv"

// RunLinksTarget is one choice in the link menu. Title is the visible row; Detail is
// the help and accessibility label, so repeated "Open PR" rows stay distinct. The
// API sends it with its detail (api.RunLinkWire).
type RunLinksTarget struct {
	ID        string
	Title     string
	Reference string
	URL       string
}

// Detail is "\(title) \(reference)".
func (t RunLinksTarget) Detail() string {
	return t.Title + " " + t.Reference
}

// RunLinksTargets is the link menu: the subject first, then each linked pull request
// once. Empty URLs are dropped before that, so they never hide a later URL for the
// same destination. ambiguous is the set of repositories whose names need the full
// owner.
func RunLinksTargets(subject Subject, pullRequests []PullRequest, ambiguous map[string]struct{}) []RunLinksTarget {
	chosen := []RunLinksTarget{}
	seen := map[string]struct{}{}
	add := func(target RunLinksTarget) {
		if _, dup := seen[target.ID]; dup {
			return
		}
		seen[target.ID] = struct{}{}
		chosen = append(chosen, target)
	}
	if subject.URL != "" {
		add(runLinksTarget(subject, ambiguous))
	}
	for _, pull := range pullRequests {
		if pull.URL != "" {
			add(runLinksPullTarget(pull, ambiguous))
		}
	}
	return chosen
}

// RunLinksTitleFor is what the row says for a subject kind. A review is a pull
// request; a task you started and a schedule share one label.
func RunLinksTitleFor(kind SubjectKind) string {
	switch kind {
	case SubjectKindIssue:
		return "Open Issue"
	case SubjectKindPullRequest:
		return "Open PR"
	case SubjectKindJira:
		return "Open Ticket"
	default:
		return "Open Task"
	}
}

// runLinksTarget is the menu row for a subject.
func runLinksTarget(subject Subject, ambiguous map[string]struct{}) RunLinksTarget {
	return RunLinksTarget{
		ID:        runLinksIdentity(subject),
		Title:     RunLinksTitleFor(subject.Kind),
		Reference: runLinksReference(subject, ambiguous),
		URL:       subject.URL,
	}
}

// runLinksPullTarget is the menu row for a pull request.
func runLinksPullTarget(pull PullRequest, ambiguous map[string]struct{}) RunLinksTarget {
	return RunLinksTarget{
		ID:        runLinksPullIdentity(pull.Repo, pull.Number),
		Title:     RunLinksTitleFor(SubjectKindPullRequest),
		Reference: RepoLabelPullRequest(pull.Repo, pull.Number, ambiguous),
		URL:       pull.URL,
	}
}

// runLinksIdentity separates issues, pull requests, tickets and tasks that share a
// repository and number. A review subject uses the pull request's identity, so the
// same one in the associated list is not offered twice.
func runLinksIdentity(subject Subject) string {
	switch subject.Kind {
	case SubjectKindIssue:
		return "issue:" + subject.Repo + "#" + strconv.Itoa(subject.Number)
	case SubjectKindPullRequest:
		return runLinksPullIdentity(subject.Repo, subject.Number)
	case SubjectKindJira:
		return "ticket:" + subject.Key
	case SubjectKindManual:
		return "task:manual:" + subject.Key
	default:
		return "task:scheduled:" + subject.Key
	}
}

// runLinksPullIdentity is the identity of a pull request.
func runLinksPullIdentity(repo string, number int) string {
	return "pr:" + repo + "#" + strconv.Itoa(number)
}

// runLinksReference keeps GitHub names with the shared repository shortening. Tickets
// and tasks keep the reference the rest of the app already shows.
func runLinksReference(subject Subject, ambiguous map[string]struct{}) string {
	switch subject.Kind {
	case SubjectKindIssue, SubjectKindPullRequest:
		return RepoLabelPullRequest(subject.Repo, subject.Number, ambiguous)
	default:
		return subject.Ref()
	}
}
