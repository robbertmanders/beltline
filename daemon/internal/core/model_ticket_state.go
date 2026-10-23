//
// Comments is a set saved as a sorted array; see setOf.

package core

import (
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// TicketStateComment is one comment on an issue or ticket.
//
// Encoding: synthesized: id, author, body. All three are required to decode.
type TicketStateComment struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Body   string `json:"body"`
}

// TicketStateChild is a sub-issue, or a ticket under an epic.
type TicketStateChild struct {
	// Ref is "KAN-14", "#31", or "acme/web#31" for an issue in another repository.
	Ref    string   `json:"ref"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	IsOpen bool     `json:"isOpen"`
	Labels []string `json:"labels"`
}

// TicketState is what a ticket watch reads of an issue or Jira ticket: its
// status, its comments and its children (sub-issues, or the tickets under an epic).
type TicketState struct {
	Status   string               `json:"status"`
	IsOpen   bool                 `json:"isOpen"`
	Comments []TicketStateComment `json:"comments"`
	Children []TicketStateChild   `json:"children"`
}

// TicketObservationSeen is a child's status as the watch last saw it.
type TicketObservationSeen struct {
	Status string `json:"status"`
	IsOpen bool   `json:"isOpen"`
}

// TicketObservation is what a ticket watch last reported, saved on the run.
// Unlike a pull request's, it carries over when the run comes back to the
// watch, so a child that finished while an agent looked at the last change
// still counts.
//
// Comments is written as a sorted array; see setOf[string].
type TicketObservation struct {
	Status string `json:"status"`
	// Comments are the comment ids already counted as seen. A set here.
	Comments setOf[string] `json:"comments"`
	// Children is each child's status, by ref.
	Children map[string]TicketObservationSeen `json:"children"`
	// PendingSince is the first change not reported yet, for the quiet period.
	PendingSince *jsonx.Date `json:"pendingSince,omitzero"`
}

// NewTicketObservation is the observation with everything in state counted as
// seen. A ref that appears
// twice keeps its first child.
func NewTicketObservation(state TicketState) TicketObservation {
	o := TicketObservation{
		Status:   state.Status,
		Comments: map[string]struct{}{},
		Children: map[string]TicketObservationSeen{},
	}
	for _, comment := range state.Comments {
		o.Comments[comment.ID] = struct{}{}
	}
	for _, child := range state.Children {
		if _, seen := o.Children[child.Ref]; !seen {
			o.Children[child.Ref] = TicketObservationSeen{Status: child.Status, IsOpen: child.IsOpen}
		}
	}
	return o
}
