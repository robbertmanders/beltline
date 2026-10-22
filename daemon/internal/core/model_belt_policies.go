// Policies: the run policies a belt
// has and a run keeps.

package core

import (
	"encoding/json"
)

// Policies are a belt's run policies. A run keeps the policies it started with.
//
// Decoding gives a missing key its DefaultPolicies value.
type Policies struct {
	// RetriageOnEdit: edited before any stage changes code, start again at the first stage.
	RetriageOnEdit bool `json:"retriageOnEdit"`
	// StopOnClose: closed, done, or no longer asked for, stop the run, unless it
	// has reached a watch stage.
	StopOnClose bool `json:"stopOnClose"`
	// CommentOnHold: comment on the issue, pull request or ticket when a stage
	// holds the run.
	CommentOnHold bool `json:"commentOnHold"`
	// WaitForBlockers: leave an issue or ticket alone while an issue or ticket
	// that blocks it is still open.
	WaitForBlockers bool `json:"waitForBlockers"`
}

// DefaultPolicies returns the policies a new belt starts with: every policy on
// except CommentOnHold, which would post an agent's output, unread, where
// anyone who can see the issue, pull request or ticket reads it.
func DefaultPolicies() Policies {
	return Policies{RetriageOnEdit: true, StopOnClose: true, WaitForBlockers: true}
}

// legacyPolicies are the policies of belts and runs saved before policies had
// their own key: every policy on, CommentOnHold included, as they ran then.
func legacyPolicies() Policies {
	return Policies{RetriageOnEdit: true, StopOnClose: true, CommentOnHold: true, WaitForBlockers: true}
}

// UnmarshalJSON reads the policies over the defaults, so a missing one keeps its default.
func (p *Policies) UnmarshalJSON(data []byte) error {
	type plain Policies
	*p = DefaultPolicies()
	return json.Unmarshal(data, (*plain)(p))
}
