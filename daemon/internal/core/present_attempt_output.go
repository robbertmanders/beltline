// The decisions for an attempt's output response: which availability
// an attempt's output has, what it shows as Markdown, and the message when it has none.

package core

// AttemptOutputAvailability says whether an attempt's output exists. Its wire
// strings are `available`, `missing` and `notApplicable`.
type AttemptOutputAvailability string

// The AttemptOutputAvailability values.
const (
	// AttemptOutputAvailable means the agent's output text exists.
	AttemptOutputAvailable AttemptOutputAvailability = "available"
	// AttemptOutputMissing means an agent attempt has no output text yet, or never wrote one.
	AttemptOutputMissing AttemptOutputAvailability = "missing"
	// AttemptOutputNotApplicable means the stage is not an agent, so it writes no output.
	AttemptOutputNotApplicable AttemptOutputAvailability = "notApplicable"
)

// attemptOutputWorkingMessage is shown while an open attempt's agent has not written yet.
const attemptOutputWorkingMessage = "The agent is working. Its output appears here when it is done."

// AttemptOutputPresentation is what an attempt's output shows. Blocks is never nil
// (it is empty unless the text exists) and is the whole output as one flat list. Title is the
// document's title, nil when there is none, and Sections are the output's sections after it
// (see AttemptOutputSections), never nil. Message is nil unless the output is missing.
type AttemptOutputPresentation struct {
	Availability AttemptOutputAvailability
	Blocks       []MarkdownBlock
	Title        *MarkdownBlock
	Sections     []AttemptOutputSection
	Message      *string
}

// AttemptOutputPresent decides an attempt's output from its stage, the attempt and the
// output text, if it could be read. A non-agent stage is notApplicable; text that exists
// is parsed after stripping outcome lines and hoisting the document; a missing text says
// what is still running or what the attempt wrote.
func AttemptOutputPresent(stage Stage, attempt Attempt, text *string) AttemptOutputPresentation {
	if stage.Kind() != StageKindAgent {
		return AttemptOutputPresentation{Availability: AttemptOutputNotApplicable, Blocks: []MarkdownBlock{}, Sections: []AttemptOutputSection{}}
	}
	if text != nil {
		body := DocumentHoisting(OutcomeParserStrip(*text), DocumentHoistingDefaultSection)
		blocks := MarkdownBlockParse(body)
		if blocks == nil {
			blocks = []MarkdownBlock{}
		}
		title, sections := AttemptOutputSections(blocks)
		return AttemptOutputPresentation{Availability: AttemptOutputAvailable, Blocks: blocks, Title: title, Sections: sections}
	}
	message := AttemptOutputMissingMessage(stage.Writes(), attempt.IsOpen())
	return AttemptOutputPresentation{Availability: AttemptOutputMissing, Blocks: []MarkdownBlock{}, Sections: []AttemptOutputSection{}, Message: &message}
}

// AttemptOutputMissingMessage says why an agent attempt has no output: the agent is
// still working on an open attempt, otherwise the attempt wrote no file named writes.
func AttemptOutputMissingMessage(writes string, open bool) string {
	if open {
		return attemptOutputWorkingMessage
	}
	return "This attempt wrote no " + writes + "."
}
