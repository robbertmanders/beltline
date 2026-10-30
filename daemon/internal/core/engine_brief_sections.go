// The brief's sections: the
// artifacts request, unfenced lines, repositories, the subject and the footer.

package core

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// briefRequestsArtifacts reports whether a brief asks for the artifacts folder:
// `{artifacts}` outside a fenced example. A run from before stage keys still lets
// an agent key or stage id named artifacts keep the output meaning.
func briefRequestsArtifacts(brief string, run Run, context BriefContext) bool {
	if briefOwnsArtifacts(run, context) {
		return false
	}
	return slices.ContainsFunc(briefUnfenced(brief), func(line string) bool {
		return strings.Contains(line, "{artifacts}")
	})
}

// briefOwnsArtifacts reports whether the run's stages own the artifacts name.
func briefOwnsArtifacts(run Run, context BriefContext) bool {
	if run.UsesStageKeys() {
		return false
	}
	return slices.ContainsFunc(run.Stages, func(stage Stage) bool {
		return stage.Kind() == StageKindAgent && (stage.ID == "artifacts" || briefLegacyKey(stage, context) == "artifacts")
	})
}

// briefUnfenced is the lines of a brief outside fenced code blocks, without the
// fences.
func briefUnfenced(brief string) []string {
	var lines []string
	var fence rune
	fenceOpen := false
	fenceLength := 0
	for _, line := range briefSplitNewlines(brief) {
		trimmed := strings.TrimLeft(line, " \t")
		marker, markerSize := rune(0), 0
		if trimmed != "" && (trimmed[0] == '`' || trimmed[0] == '~') {
			marker = rune(trimmed[0])
			markerSize = 1
		}
		if markerSize > 0 {
			length := 0
			for length < len(trimmed) && rune(trimmed[length]) == marker {
				length++
			}
			if fenceOpen {
				if marker == fence && length >= fenceLength && strings.TrimSpace(trimmed[length:]) == "" {
					fenceOpen = false
				}
			} else if length >= 3 {
				fence = marker
				fenceOpen = true
				fenceLength = length
			}
			if length >= 3 {
				continue
			}
		}
		if !fenceOpen {
			lines = append(lines, line)
		}
	}
	return lines
}

// briefSplitNewlines splits text at each newline character, keeping empty pieces.
func briefSplitNewlines(text string) []string {
	var lines []string
	start := 0
	for index, r := range text {
		if IsNewline(r) {
			lines = append(lines, text[start:index])
			start = index + len(string(r))
		}
	}
	return append(lines, text[start:])
}

// briefArtifactsPath is the artifacts folder of the attempt. Shared disk
// convention with Paths.artifactsFolder: artifacts/ in the attempt.
func briefArtifactsPath(context BriefContext) string {
	return filepath.Join(context.AttemptFolder, "artifacts")
}

// briefArtifacts is the text that tells the agent about the artifacts folder.
func briefArtifacts(folder string) string {
	return "## Artifacts\n\n" +
		"Optionally write standalone .html files in " + folder + ", creating the directory only if needed.\n" +
		"Use inline CSS, optional inline JavaScript and inline SVG, with no required remote resources.\n" +
		"Follow any more restrictive instructions in your brief. Artifacts supplement the normal Markdown output; its final `Outcome:` line alone determines the outcome.\n"
}

// briefRepositories is the Repositories section of a brief: each repository and
// its checkout.
func briefRepositories(run Run, context BriefContext) string {
	if len(run.Repos) <= 1 {
		return ""
	}
	text := "\n## Repositories\n\n"
	for _, repo := range run.Repos {
		folder, hasFolder := context.Checkouts[repo]
		if !hasFolder {
			folder = briefLastComponent(repo)
		}
		base, hasBase := context.Bases[repo]
		if !hasBase {
			base = context.Base
		}
		text += "- " + repo + ": " + folder + ", run branch `" + context.Branch + "`, merges into `" + base + "`\n"
	}
	return text + "\nYou choose which repositories to work in. Commit and push the same run branch in each repository you change, and open their pull requests yourself, each into the branch it merges into.\n\n"
}

// briefLastComponent is the last non-empty "/"-separated part of repo, or repo
// itself when it has none.
func briefLastComponent(repo string) string {
	last := ""
	found := false
	for _, part := range strings.Split(repo, "/") {
		if part != "" {
			last = part
			found = true
		}
	}
	if !found {
		return repo
	}
	return last
}

// briefSubject is the section about what the run works on. repositories are the
// repositories the subject touches. A withheld subject keeps its reference and
// where its work happens, and leaves out its title, link, labels, parent and
// description.
func briefSubject(subject Subject, base string, repositories []string, withheld bool) string {
	title := ": " + subject.Title
	if withheld {
		title = ""
	}
	var text string
	switch subject.Kind {
	case SubjectKindIssue:
		text = "## The issue\n\n" + subject.Repo + "#" + strconv.Itoa(subject.Number) + title + "\n"
	case SubjectKindPullRequest:
		text = "## The pull request\n\n" + subject.Repo + "#" + strconv.Itoa(subject.Number) + title + "\n"
	case SubjectKindJira:
		text = "## The Jira ticket\n\n" + subject.Key + title + "\n"
	case SubjectKindManual:
		if withheld {
			text = "## The task\n\n" + subject.Ref() + "\n"
		} else {
			text = "## The task\n\n" + subject.Title + "\n"
		}
	case SubjectKindScheduled:
		if withheld {
			text = "## The scheduled task\n\nFor " + subject.Key + "\n"
		} else {
			text = "## The scheduled task\n\n" + subject.Title + ", for " + subject.Key + "\n"
		}
	}
	if !withheld {
		if subject.URL != "" {
			text += subject.URL + "\n"
		}
		if len(subject.Labels) > 0 {
			text += "Labels: " + strings.Join(subject.Labels, ", ") + "\n"
		}
		if subject.Parent != nil {
			text += "It is part of " + *subject.Parent + "; read it for the design it follows.\n"
		}
	}
	if subject.Kind != SubjectKindPullRequest && subject.BaseHint() != nil {
		if len(repositories) > 1 {
			text += "Its work starts from " + *subject.BaseHint() + " where a repository has that branch, and from the repository's default branch elsewhere; each repository below says which.\n"
		} else {
			text += "Its work starts from " + base + ", and its pull requests merge into " + base + ".\n"
		}
	}
	switch subject.Kind {
	case SubjectKindPullRequest:
		text += "Its head is checked out in your working folder. It merges into " + base + ".\n"
	case SubjectKindJira, SubjectKindManual, SubjectKindScheduled:
		// Several repositories are listed below, each with its folder.
		if len(repositories) > 1 {
			text += "The code is spread over several repositories.\n"
		} else {
			text += "The code is in " + subject.Repo + ".\n"
		}
	case SubjectKindIssue:
	}
	if withheld {
		return text + "\n" + BriefCopyWithheld(subject.Kind) + "\n"
	}
	body := strings.TrimSpace(subject.Body)
	if body == "" {
		body = "(The " + subject.Kind.Noun() + " has no description.)"
	}
	return text + "\n" + body + "\n"
}

// BriefCopyWithheld is what a brief says in place of a withheld subject's details.
func BriefCopyWithheld(kind SubjectKind) string {
	noun := kind.Noun()
	return "The person withheld this " + noun + "'s details from agents. Work from their task below and the handoffs you are given. Use the reference above only where your brief asks you to link to the " + noun + ", comment on it or move it; don't read its title, description, comments or linked items."
}

// briefTask is the section with the task you wrote at a prompt stage; empty when
// the run has none.
func briefTask(run Run) string {
	task := strings.TrimSpace(ValueOf(run.Task))
	if task == "" {
		return ""
	}
	text := "\n## Your task\n\nThe person wrote this task for the run. It is what the run is for"
	if run.WithholdsSource() {
		text += "."
	} else {
		text += ": where it and the " + run.Subject.Kind.Noun() + " above disagree, follow the task."
	}
	return text + "\n\n" + task + "\n"
}

// BriefFooter is the lines added to the end of every agent brief: the done
// section, the output line rules and the outcome choices of stage.
func BriefFooter(stage Stage, output string) string {
	text := "## When you are done\n\n" +
		"Stage outputs, including specs, belong to this run's attempt folders, outside the repository checkout. Use only the output paths supplied in this brief. Never create, replace or commit a repository-root spec or other stage output, or substitute a file from another run.\n\n" +
		"A placeholder marked `(no <filename> yet)` means that output was not produced for this run, not that you should search the checkout for it. When no spec was produced, build or verify against this run's subject and supplied notes instead. Do not create a replacement spec in the checkout or report a spec mismatch solely because no spec was produced.\n"
	// Beltline draws the output at the page's width, so a column of its own only narrows it.
	text += "\nWrite each paragraph and list item of " + output + " on one line; don't wrap them at a column.\n\n"
	text += "End " + output + " with one line copied exactly from the choices below. Write it in the output file, not only in your reply.\n\nGuidance for choosing (these descriptions are not outcome values):\n"
	for _, outcome := range stage.Outcomes {
		text += "- " + outcome.Name + ": " + outcome.When + "\n"
	}
	text += "\nCopy exactly one of these final lines; do not substitute a description:\n\n```text\n"
	for _, outcome := range stage.Outcomes {
		text += "Outcome: " + outcome.Name + "\n"
	}
	text += "```\n\nIf none fits, pick the closest one and explain why before the final line.\n"
	return text
}
