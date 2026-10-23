// The stage copy, with RelativeTime
// and InlineSpan.

package core

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// StageCopyNoAgentWroteYet is shown when no agent has written for the run.
const StageCopyNoAgentWroteYet = "No agent has written anything for this run yet."

// StageCopyNotDecidedYet is shown while an outcome is undecided.
const StageCopyNotDecidedYet = "Not decided yet"

// StageCopyWaitsForYou is shown on a stage that waits for you.
const StageCopyWaitsForYou = "This stage waits for you."

// StageCopyFailedOutcome is the outcome name shown for a failed stage.
const StageCopyFailedOutcome = "failed"

// StageCopyOutcomeButtonsHelp explains the decision buttons.
const StageCopyOutcomeButtonsHelp = "These buttons are this stage's outcomes. Change them in Belts."

// StageCopyArtifactWarning warns before an agent's HTML artifact is opened.
const StageCopyArtifactWarning = "Agent-generated HTML. Opening in your browser may run scripts and reach the network."

// stageCopyMessagePrefix starts "Your message: …" under an agent's outcome. Copy from StageCopy.messageLine.
const stageCopyMessagePrefix = "Your message: "

// stageCopyGatePrefix starts a gate's note. Copy from StageCopy.detailLine.
const stageCopyGatePrefix = "Your note: "

// stageCopyNotePrefix starts "Note for Build". Copy from StageCopy.noteLabel.
const stageCopyNotePrefix = "Note for "

// stageCopyNextRunTarget is the note target when the outcome leaves the belt's line. Copy from StageCopy.noteTarget.
const stageCopyNextRunTarget = "the next run"

// stageCopyDecidedSuffix sits between the decider and the time in the decided line. Copy from StageCopy.decidedLine.
const stageCopyDecidedSuffix = " decided "

// stageCopyThenPrefix starts the hover help of a decision button. Copy from StageCopy.thenHelp.
const stageCopyThenPrefix = "Then: "

// StageCopyMessageLine is "Your message: …" under an agent's outcome.
func StageCopyMessageLine(note string) string {
	return stageCopyMessagePrefix + note
}

// StageCopyDetailLine is a gate's note, "Your note: …", or an agent's detail as it was
// written.
func StageCopyDetailLine(detail string, gate bool) string {
	if gate {
		return stageCopyGatePrefix + detail
	}
	return detail
}

// StageCopyNoteLabel is "Note for Build".
func StageCopyNoteLabel(target string) string {
	return stageCopyNotePrefix + target
}

// StageCopyDecidedLine is "You decided 4 min ago." or "The AFK assistant decided 4 min ago.",
// from who decided.
func StageCopyDecidedLine(by string, at, now time.Time) string {
	return by + stageCopyDecidedSuffix + RelativeTimeAgo(at, now) + "."
}

// StageCopyNoteTarget is where a decided outcome's note goes: the stage it jumps to,
// or the next run.
func StageCopyNoteTarget(outcome Outcome, run Run) string {
	if outcome.Then.Action == ThenActionGoto {
		if to := run.Layout().Target(outcome.Then.To, run.Routes(), nil); to != nil {
			for _, stage := range run.Stages {
				if stage.ID == *to {
					return stage.Name
				}
			}
		}
	}
	return stageCopyNextRunTarget
}

// StageCopyThenHelp is the hover help of a decision button: what choosing the outcome
// does.
func StageCopyThenHelp(outcome Outcome, run Run, stage Stage) string {
	return stageCopyThenPrefix + EngineDescribe(outcome.Then, run.Layout(), stage.ID, run.Routes())
}

// RelativeTimeAgo is relative time in the words the app used: "just now", "4 min ago",
// "2 h ago", "3 d ago".
func RelativeTimeAgo(date, now time.Time) string {
	seconds := int(now.Sub(date).Seconds())
	if seconds < 60 {
		return "just now"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return strconv.Itoa(minutes) + " min ago"
	}
	hours := minutes / 60
	if hours < 24 {
		return strconv.Itoa(hours) + " h ago"
	}
	return strconv.Itoa(hours/24) + " d ago"
}

// InlineSpanKind is the kind of an inline span.
type InlineSpanKind string

const (
	InlineSpanKindText   InlineSpanKind = "text"
	InlineSpanKindStrong InlineSpanKind = "strong"
	InlineSpanKindCode   InlineSpanKind = "code"
)

// InlineSpan is a piece of inline Markdown the daemon has parsed, so a client never
// sees the markup. Text carries the span's text for every kind.
type InlineSpan struct {
	Kind InlineSpanKind `json:"type"`
	Text string         `json:"text"`
}

// InlineSpanParse splits `**strong**` and “ `code` “ out of a line. An unclosed
// marker stays as text. The line is split into characters,
// so a marker followed by a combining mark is not a marker.
func InlineSpanParse(text string) []InlineSpan {
	chars := Characters(text)
	spans := []InlineSpan{}
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			spans = append(spans, InlineSpan{Kind: InlineSpanKindText, Text: plain.String()})
			plain.Reset()
		}
	}
	// closing finds the first place at or after start where token occurs, or -1.
	closing := func(token []string, start int) int {
		if len(token) == 0 || start > len(chars)-len(token) {
			return -1
		}
		for index := start; index <= len(chars)-len(token); index++ {
			if slices.Equal(chars[index:index+len(token)], token) {
				return index
			}
		}
		return -1
	}
	index := 0
	for index < len(chars) {
		if chars[index] == "*" && index+1 < len(chars) && chars[index+1] == "*" {
			if end := closing([]string{"*", "*"}, index+2); end > index+2 {
				flush()
				spans = append(spans, InlineSpan{Kind: InlineSpanKindStrong, Text: strings.Join(chars[index+2:end], "")})
				index = end + 2
				continue
			}
		}
		if chars[index] == "`" {
			if end := closing([]string{"`"}, index+1); end > index+1 {
				flush()
				spans = append(spans, InlineSpan{Kind: InlineSpanKindCode, Text: strings.Join(chars[index+1:end], "")})
				index = end + 1
				continue
			}
		}
		plain.WriteString(chars[index])
		index++
	}
	flush()
	return spans
}
