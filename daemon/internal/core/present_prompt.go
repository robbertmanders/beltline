// The prompt stage's editor on the run page: its words, the work item beside it
// and the source choice it starts with.

package core

// Copy of the prompt stage's editor. Literal text is exact.
const (
	PromptCopyLabel        = "Your prompt"
	PromptCopyPlaceholder  = "Describe what you want the agents to do, with any constraints and acceptance criteria."
	PromptCopyHelp         = "Write the run's task, then choose how to go on. Every stage after this one gets it."
	PromptCopyContextLabel = "Source context"
	PromptCopySourceLabel  = "Include source context for agents"
	PromptCopySourceHelp   = "Unchecked, agents get your prompt and the work item's reference only, not its title, description, labels, parent or link. Branch names and folders still carry the reference."
)

// PromptFacts is what a prompt stage's editor shows: its words, the task already
// written when the run comes back, the work item's details for you to read, and
// what the source checkbox starts as.
type PromptFacts struct {
	Label       string `json:"label"`
	Placeholder string `json:"placeholder"`
	Help        string `json:"help"`
	// Task is the task the run already has, to start the editor with on a visit back.
	Task         *string         `json:"task,omitzero"`
	ContextLabel string          `json:"contextLabel"`
	Context      []MarkdownBlock `json:"context"`
	SourceLabel  string          `json:"sourceLabel"`
	SourceHelp   string          `json:"sourceHelp"`
	// Source is what the checkbox starts as: the run's choice when it made one, else the stage's.
	Source SourceSharing `json:"source"`
}

// PromptFactsOf is the editor of prompt stage on run; nil for any other stage.
func PromptFactsOf(run Run, stage Stage) *PromptFacts {
	sharing := stage.SourceSharing()
	if sharing == nil {
		return nil
	}
	if run.Sharing.Known() {
		sharing = &run.Sharing
	}
	return &PromptFacts{
		Label: PromptCopyLabel, Placeholder: PromptCopyPlaceholder, Help: PromptCopyHelp,
		Task:         CopyPointer(run.Task),
		ContextLabel: PromptCopyContextLabel,
		Context:      MarkdownBlockParse(briefSubject(run.Subject, run.Base, run.Repos, false)),
		SourceLabel:  PromptCopySourceLabel, SourceHelp: PromptCopySourceHelp,
		Source: *sharing,
	}
}
