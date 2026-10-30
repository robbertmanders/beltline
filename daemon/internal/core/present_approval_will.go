// The "Beltline will" block of an approval: what its primary outcome does next,
// as lines of copy.

package core

// ApprovalWill is the "Beltline will" block of an approval: its heading and the lines under it.
type ApprovalWill struct {
	Heading string
	Lines   [][]InlineSpan
}

// ApprovalWillOf says what primary, an approval's primary outcome, does next for
// the run. When it moves on along the run's path, each stage after the approval
// gets a line; otherwise one line describes the outcome's route.
func ApprovalWillOf(run Run, stage Stage, primary Outcome) ApprovalWill {
	heading := primary.ButtonLabel() + " means Beltline will"
	routes := run.Routes()
	path := run.Layout().Path(routes, false)
	at := -1
	for i, next := range path {
		if next.ID == stage.ID {
			at = i
			break
		}
	}
	if primary.Then.Action != ThenActionNext || at < 0 {
		line := EngineDescribe(primary.Then, run.Layout(), stage.ID, routes)
		return ApprovalWill{Heading: heading, Lines: [][]InlineSpan{InlineSpanParse(CapitalizedFirst(line) + ".")}}
	}
	branch := ValueOr(run.Branch, RunBranch(run))
	lines := [][]InlineSpan{}
	for _, next := range path[at+1:] {
		agent := "the agent"
		if next.Runner != nil {
			agent = next.Runner.Label()
		}
		var line string
		switch {
		case next.Type == StageTypeAgent && next.Workspace == WorkspaceWorktree:
			line = "Create `" + branch + "` and have **" + agent + "** run " + next.Name + "."
		case next.Type == StageTypeAgent && next.Workspace == WorkspaceWorktreeReadOnly:
			line = "Have **" + agent + "** run " + next.Name + " on the change."
		case next.Type == StageTypeAgent:
			line = "Run " + next.Name + "."
		case next.Type == StageTypeApprove:
			line = "Wait for your " + LowercasedFull(next.Name) + "."
		case next.Type == StageTypePrompt:
			line = "Wait for your prompt at " + next.Name + "."
		default:
			if next.WatchesTicket() {
				line = "Watch " + run.Subject.Ref() + " and the tickets under it."
			} else {
				line = "Wait for the pull request to be merged."
			}
		}
		lines = append(lines, InlineSpanParse(line))
	}
	return ApprovalWill{Heading: heading, Lines: lines}
}
