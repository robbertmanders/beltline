package api

// The typed request bodies of the run command endpoints. Enum-like fields
// arrive as strings so an invalid value is a well-formed command argument
// that can be refused with 422, not a body the server cannot parse.

// RunEmptyRequest is a command that needs no operands. An absent body, `{}`
// and an object with unknown keys are all accepted; malformed JSON is not.
type RunEmptyRequest struct{}

// RunDecideRequest is the body of POST /api/v1/runs/:id/decide.
type RunDecideRequest struct {
	Outcome string  `json:"outcome" required:"true"`
	Note    *string `json:"note,omitzero"`
	// Source is, at a prompt stage, whether agents get the work item's details:
	// shared or withheld.
	Source *string `json:"source,omitzero"`
}

// RunResumeRequest is the body of POST /api/v1/runs/:id/resume.
type RunResumeRequest struct {
	Stage *string `json:"stage,omitzero"`
	Note  *string `json:"note,omitzero"`
	// Earlier is earlier-attempt inclusion; true when omitted.
	Earlier *bool   `json:"earlier,omitzero"`
	Leg     *string `json:"leg,omitzero"`
}

// RunAnswerRequest is the body of POST /api/v1/runs/:id/answer. Nil
// acknowledges that the person answered or continued the session
// interactively; a string is the answer to send.
type RunAnswerRequest struct {
	Answer *string `json:"answer,omitzero"`
}

// RunLegRequest is a body that names an optional leg.
type RunLegRequest struct {
	Leg *string `json:"leg,omitzero"`
}

// RunAttemptRequest is a body that names one attempt.
type RunAttemptRequest struct {
	Attempt string `json:"attempt" required:"true"`
}

// RunAdjustSwitchRequest is the body of POST /api/v1/runs/:id/adjust/switch:
// the Adjust draft after a runner change, as the daemon normalizes it.
type RunAdjustSwitchRequest struct {
	Stage    string  `json:"stage" required:"true"`
	Runner   string  `json:"runner" required:"true"`
	Model    *string `json:"model,omitzero"`
	Thinking *string `json:"thinking,omitzero"`
}

// RunAdjustSwitchResponse is the reply to an adjust switch.
type RunAdjustSwitchResponse struct {
	Runner string `json:"runner"`
	// Model is empty when this run may not use the model on the new runner.
	Model string `json:"model"`
	// Thinking is nil when the new runner does not take the effort level.
	Thinking *string `json:"thinking,omitzero"`
}

// RunAdjustStageRequest is the body of POST /api/v1/runs/:id/adjust-stage.
type RunAdjustStageRequest struct {
	Stage    string  `json:"stage" required:"true"`
	Model    string  `json:"model" required:"true"`
	Thinking *string `json:"thinking,omitzero"`
	Brief    *string `json:"brief,omitzero"`
	Runner   *string `json:"runner,omitzero"`
}

// RunAfkRequest is the body of POST /api/v1/runs/:id/afk.
type RunAfkRequest struct {
	Mode string `json:"mode" required:"true"`
}

// RunOverruleAfkRequest is the body of POST /api/v1/runs/:id/overrule-afk.
type RunOverruleAfkRequest struct {
	Decision        string `json:"decision" required:"true"`
	Stage           string `json:"stage" required:"true"`
	Comments        string `json:"comments" required:"true"`
	AcceptedWarning bool   `json:"acceptedWarning" required:"true"`
}

// RunTaskRequest is a task you start on a manual belt.
type RunTaskRequest struct {
	Belt  string  `json:"belt" required:"true"`
	Entry *string `json:"entry,omitzero"`
	Repo  string  `json:"repo" required:"true"`
	Title string  `json:"title" required:"true"`
	Body  string  `json:"body" required:"true"`
}

// ContinueSessionResponse is Continue-session's response: the run after the
// launch, the writable tmux attach command for its interactive conversation,
// and the session that command attaches, resolved by the daemon so the
// browser never parses it.
type ContinueSessionResponse struct {
	Run     RunSnapshot `json:"run"`
	Command []string    `json:"command"`
	Target  string      `json:"target"`
}

// EraseResponse is Erase's response: the id that is now erased.
type EraseResponse struct {
	ID     string `json:"id"`
	Erased bool   `json:"erased"`
}
