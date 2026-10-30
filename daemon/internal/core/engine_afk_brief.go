package core

import (
	"strconv"
	"strings"
)

// AfkBriefContext is what an AFK decision brief is rendered against. Paths are
// absolute file paths.
type AfkBriefContext struct {
	RunFolder string
	// Decision is the file the session writes its decision to.
	Decision string
	// LatestOutput is the output the stop is about: the held stage's, or for an
	// approval the latest agent output before it. Nil when there is none.
	LatestOutput *string
	// Guidance is what you told the assistant to keep in mind.
	Guidance string
}

// afkBriefQuote writes the backtick of the test literals as §, which no brief uses.
func afkBriefQuote(text string) string {
	return strings.ReplaceAll(text, "§", "`")
}

// AfkBriefRender is the prompt an AFK decision session starts with: what the run
// is about, how it got here, why it waits, the options and the decision file's
// format.
func AfkBriefRender(run Run, situation AfkSituation, context AfkBriefContext) string {
	stage := run.Stage()
	noun := run.Subject.Kind.Noun()
	text := afkBriefQuote(`# You are deciding for someone who is away

Beltline runs work through belts of stages. The person who owns this run is away from their keyboard, and they asked you to take the steps that would otherwise wait for them. Decide as they would: carefully, conservatively, and only within the options below. When you are unsure, or the decision is risky or needs knowledge only they have, choose §leave§. They will read your decision and your reasons when they are back.

Do not change any files, push, comment, or contact anyone. Read what you need, then write your decision.

## The run

Belt: `) + run.BeltName + "\n" + CapitalizedFirst(noun) + ": " + run.Subject.Ref()
	if run.WithholdsSource() {
		text += "\n\n" + BriefCopyWithheld(run.Subject.Kind)
	} else {
		text += " " + run.Subject.Title
		if run.Subject.URL != "" {
			text += "\nLink: " + run.Subject.URL
		}
		if body := strings.TrimSpace(run.Subject.Body); body != "" {
			text += "\n\n" + body
		}
	}
	if task := strings.TrimSpace(ValueOf(run.Task)); task != "" {
		text += "\n\nThe task they wrote for the run:\n\n" + task
	}
	for _, pull := range run.PullRequests {
		text += "\n\nPull request: " + pull.URL + " (" + string(pull.State) + ")"
		if pull.Checks != nil && *pull.Checks == PullRequestCheckStateFailing {
			text += ", checks failing"
		}
		if pull.Conflicted != nil && *pull.Conflicted {
			text += ", merge conflicts"
		}
	}
	text += "\n\n## How it got here\n\nThe run's folder is " + context.RunFolder +
		". Each attempt's folder in it holds its brief, output and session log.\n"
	for _, attempt := range run.Attempts {
		name := attempt.StageID
		for _, candidate := range run.Stages {
			if candidate.ID == attempt.StageID {
				name = candidate.Name
				break
			}
		}
		line := "\n- " + name + ", attempt " + strconv.Itoa(attempt.Number) + " (" + attempt.Folder + ")"
		if attempt.Outcome != nil {
			line += ": " + *attempt.Outcome
		}
		if attempt.Route != nil {
			line += " → " + *attempt.Route
		}
		if attempt.Detail != nil {
			line += ". " + *attempt.Detail
		}
		text += line
		if attempt.Note != nil {
			if note := strings.TrimSpace(*attempt.Note); note != "" {
				text += "\n  It started with this note: " + strings.ReplaceAll(note, "\n", " ")
			}
		}
	}
	text += "\n\n## Why it waits\n\n" + afkBriefWhy(run, situation, stage) + "\n"
	if context.LatestOutput != nil {
		if output := strings.TrimSpace(*context.LatestOutput); output != "" {
			text += "\n## The latest output\n\n" + output + "\n"
		}
	}
	if guidance := strings.TrimSpace(context.Guidance); guidance != "" {
		text += "\n## What the person asked you to keep in mind\n\n" + guidance + "\n"
	}
	text += afkBriefQuote("\n## Your options\n\nChoose exactly one. Write its words after §Decision:§ exactly as shown, such as §Decision: " +
		situation.Options[0].ID + "§.\n")
	for _, option := range situation.Options {
		text += afkBriefQuote("\n- §" + option.ID + "§: " + option.Label)
	}
	text += afkBriefQuote(`

## Your decision

Write it to `) + context.Decision + afkBriefQuote(`, in this shape:

§§§
## Message

What you tell the agent, or the note that goes with your choice. For questions, answer every one; say what you assumed. Leave it empty when there is nothing to say.

## Why

Your reasons, in a few sentences, for the person to check when they are back.

Decision: <one of the options above, as written>
§§§

The last §Decision:§ line decides.`)
	return text + "\n"
}

// afkBriefWhy is why the run waits, for the brief's section.
func afkBriefWhy(run Run, situation AfkSituation, stage *Stage) string {
	name := run.StageID
	if stage != nil {
		name = stage.Name
	}
	reason := ""
	if run.Reason != nil {
		reason = ": " + *run.Reason
	}
	switch situation.Kind {
	case AfkSituationKindQuestion:
		return name + " held the run with questions" + reason + ". Its output below asks them. If you answer, its session continues with your message."
	case AfkSituationKindFailure:
		return name + " failed" + reason + ". See its attempt's session log for what went wrong."
	case AfkSituationKindApproval:
		text := name + " is an approval stage: it waits for a decision on the work so far. Its choices:\n"
		if stage != nil {
			for _, outcome := range stage.Outcomes {
				text += "\n- " + outcome.Name + " (" + outcome.When + ")"
			}
		}
		return text
	}
	return name + " held the run" + reason + "."
}
