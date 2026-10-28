package core

// ApprovalPrompt is the shared input of a gate that collects a task before any agent output.
type ApprovalPrompt struct {
	NoteKey      string          `json:"noteKey"`
	Label        string          `json:"label"`
	Placeholder  string          `json:"placeholder"`
	Help         string          `json:"help"`
	ContextLabel string          `json:"contextLabel"`
	Context      []MarkdownBlock `json:"context"`
	IncludeLabel string          `json:"includeLabel"`
	Include      bool            `json:"include"`
}

// ApprovalPromptOf distinguishes task entry from approvals that review an agent's work.
func ApprovalPromptOf(run Run, stage Stage, hasDocument bool) *ApprovalPrompt {
	if stage.Kind() != StageKindGate || hasDocument {
		return nil
	}
	key := ""
	for _, outcome := range stage.Outcomes {
		if !outcome.Then.Action.MovesOn() {
			continue
		}
		if !outcome.Then.WithOutput || outcome.Then.Action.Finishes() {
			return nil
		}
		if key == "" {
			key = outcome.ID
		}
	}
	if key == "" {
		return nil
	}
	return &ApprovalPrompt{
		NoteKey: key, Label: "Your prompt",
		Placeholder:  "Describe what you want the agent to do, including constraints and acceptance criteria.",
		Help:         "Write your prompt first, then choose how to proceed. The same prompt is sent with either action. Source context is excluded unless you include it.",
		ContextLabel: "Source context",
		Context:      MarkdownBlockParse(briefSubject(run.Subject, run.Base, run.Repos)),
		IncludeLabel: "Include source context for agents",
		Include:      ValueOr(run.IncludeSubject, false),
	}
}
