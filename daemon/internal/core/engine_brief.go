// The prompt an agent session
// starts with: the context, the rendering, follow-ups and reruns. The sections,
// placeholders and paths are in engine_brief_sections.go and
// engine_brief_placeholders.go.

package core

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// BriefContext is what a brief is rendered against. Paths are absolute file paths
// . Its init's defaults are Go's zero values
// (no agents, checkouts or bases), so a struct literal does the same.
type BriefContext struct {
	RunFolder     string
	AttemptFolder string
	Base          string
	Branch        string
	// Agents are the agent versions the run's stages use.
	Agents map[AgentRef]Agent
	// Checkouts are each repository's checkout folder.
	Checkouts map[string]string
	// Bases are the branch each repository's work starts from and merges into,
	// when there are several; Base is the subject's repository's.
	Bases map[string]string
}

// BriefFixedPlaceholders are the placeholders a brief can use that are not a
// stage's key: output, base, branch, issue, previous and artifacts.
var BriefFixedPlaceholders = []string{"output", "base", "branch", "issue", "previous", "artifacts"}

// BriefOutputPath is the file this attempt writes, as the URL's path.
func BriefOutputPath(stage Stage, context BriefContext) string {
	return filepath.Join(context.AttemptFolder, stage.Writes())
}

// BriefRender is the brief for stage in run, with its placeholders filled in, the
// subject, the note and the outcome footer.
func BriefRender(run Run, stage Stage, context BriefContext) string {
	return briefRenderWithEarlier(run, stage, context, "")
}

// BriefRerun is a fresh brief with the notes and output paths of this stage's
// earlier attempts (up to previous), without disclosing how those attempts ended.
func BriefRerun(run Run, stage Stage, previous Attempt, context BriefContext) string {
	var earlier []Attempt
	for _, attempt := range run.Attempts {
		if attempt.StageID == stage.ID && attempt.Number <= previous.Number {
			earlier = append(earlier, attempt)
		}
	}
	work := ""
	if len(earlier) > 0 {
		work = "\n## What this stage already had\n\nRead these earlier outputs as context only; write your whole output into this attempt's output file. Do not change the earlier attempts' folders.\n"
		for index := len(earlier) - 1; index >= 0; index-- {
			attempt := earlier[index]
			work += "\n### Attempt " + strconv.Itoa(attempt.Number) + "\n"
			if attempt.Note != nil && *attempt.Note != "" {
				work += "\nThe note it started with:\n\n" + *attempt.Note + "\n"
			}
			output := filepath.Join(context.RunFolder, attempt.Folder, stage.Writes())
			work += "\nIts output:\n" + output + "\n"
		}
	}
	return briefRenderWithEarlier(run, stage, context, work)
}

// briefRenderWithEarlier renders the brief with earlierWork already gathered.
func briefRenderWithEarlier(run Run, stage Stage, context BriefContext, earlierWork string) string {
	text := briefEffectiveBrief(stage, context)
	requestsArtifacts := briefRequestsArtifacts(text, run, context)
	values := briefPlaceholders(run, stage, context)
	for _, key := range slices.Sorted(maps.Keys(values)) {
		text = strings.ReplaceAll(text, "{"+key+"}", values[key])
	}

	text += "\n\n" + briefSubject(run.Subject, context.Base, run.Repos, run.WithholdsSource())
	text += briefTask(run)
	text += briefRepositories(run, context)
	for _, pull := range run.PullRequests {
		if run.Subject.Kind == SubjectKindPullRequest {
			continue
		}
		repo := ""
		if len(run.PullRequests) > 1 {
			repo = " in " + pull.Repo
		}
		text += "\nIts pull request: #" + strconv.Itoa(pull.Number) + repo + ", " + pull.URL + " (" + string(pull.State) + ")\n"
		if pull.Checks != nil && *pull.Checks == PullRequestCheckStateFailing {
			text += "Its checks are failing.\n"
		}
		if pull.Conflicted != nil && *pull.Conflicted {
			text += "It has merge conflicts.\n"
		}
	}

	if run.Note != nil {
		if note := strings.TrimSpace(*run.Note); note != "" {
			text += "\n## A note for this attempt\n\nA note came with this attempt. Act on it before anything else:\n\n" + note + "\n"
		}
	}
	if requestsArtifacts {
		text += "\n" + briefArtifacts(briefArtifactsPath(context))
	}
	text += earlierWork
	text += "\n" + BriefFooter(stage, BriefOutputPath(stage, context))
	return text
}

// BriefFollowUp is the prompt that continues a held session after you answered
// it. The agent still has its brief and its work, so this only says what changed.
func BriefFollowUp(run Run, stage Stage, held Attempt, context BriefContext) string {
	output := BriefOutputPath(stage, context)
	stopped := "a problem"
	if held.Outcome != nil {
		stopped = "“" + *held.Outcome + "”"
	} else if held.Detail != nil {
		stopped += " (" + *held.Detail + ")"
	}
	text := "# " + stage.Name + ", continued\n\nYou stopped at " + stopped + ", and the run was held for a person. They have answered.\n\n"
	answer := ""
	if run.Note != nil {
		answer = strings.TrimSpace(*run.Note)
	}
	if answer != "" {
		text += "## Their answer\n\n" + answer + "\n\n"
	} else {
		text += "They answered in this session. Use what they said there.\n\n"
	}
	text += "Carry on with the task from your brief. Write your whole output again to " + output + ", not only what changed: the earlier file is not read again.\n\n"
	if briefRequestsArtifacts(briefEffectiveBrief(stage, context), run, context) {
		text += briefArtifacts(briefArtifactsPath(context)) + "\n"
	}
	return text + briefRepositories(run, context) + BriefFooter(stage, output)
}

// briefEffectiveBrief is the stage's brief text as it renders: its own, else the
// pinned agent's.
func briefEffectiveBrief(stage Stage, context BriefContext) string {
	if stage.Brief != "" {
		return stage.Brief
	}
	if stage.Agent != nil {
		if agent, ok := context.Agents[*stage.Agent]; ok {
			return agent.Brief
		}
	}
	return ""
}
