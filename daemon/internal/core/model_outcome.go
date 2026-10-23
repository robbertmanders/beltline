// Tone, Outcome, WatchEvent,
// TicketEvent and WatchTarget; Then is in model_then.go.

package core

import "strings"

// Tone is how an outcome is coloured and what it asks of a person.
type Tone string

const (
	ToneGood    Tone = "good"
	ToneYou     Tone = "you"
	ToneProblem Tone = "problem"
	ToneNeutral Tone = "neutral"
)

// AllTone lists every Tone in declaration order (CaseIterable).
var AllTone = []Tone{ToneGood, ToneYou, ToneProblem, ToneNeutral}

// Known reports whether x is one of the cases of Tone.
func (x Tone) Known() bool { return enumKnown(x, AllTone) }

// Label is the name shown for the tone.
func (t Tone) Label() string {
	switch t {
	case ToneGood:
		return "Good"
	case ToneYou:
		return "Needs you"
	case ToneProblem:
		return "Problem"
	case ToneNeutral:
		return "Neutral"
	}
	return ""
}

// Outcome is a named result of a stage, and what it does next.
type Outcome struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// When: for an agent, when to pick it. For an approval, the button label.
	// For a watch, the event it is tied to (a WatchEvent raw value).
	When string `json:"when"`
	Tone Tone   `json:"tone"`
	Then Then   `json:"then"`
}

// outcomeNormalize is the name as outcome matching compares it: trimmed of
// whitespace and newlines, lower-cased.
func outcomeNormalize(name string) string {
	return LowercasedFull(strings.TrimSpace(name))
}

// WatchEvent is what a watch stage sees happen to its pull request. `none` is
// not one of those events: it is taken only when no other configured outcome
// applies.
type WatchEvent string

const (
	WatchEventMerged           WatchEvent = "merged"
	WatchEventClosed           WatchEvent = "closed"
	WatchEventChecksFailed     WatchEvent = "checksFailed"
	WatchEventConflicted       WatchEvent = "conflicted"
	WatchEventBehind           WatchEvent = "behind"
	WatchEventApproved         WatchEvent = "approved"
	WatchEventCommented        WatchEvent = "commented"
	WatchEventChangesRequested WatchEvent = "changesRequested"
	WatchEventNewChanges       WatchEvent = "newChanges"
	WatchEventNone             WatchEvent = "none"
)

// AllWatchEvent lists every WatchEvent in declaration order (CaseIterable).
var AllWatchEvent = []WatchEvent{
	WatchEventMerged, WatchEventClosed, WatchEventChecksFailed, WatchEventConflicted, WatchEventBehind,
	WatchEventApproved, WatchEventCommented, WatchEventChangesRequested, WatchEventNewChanges, WatchEventNone,
}

// WatchEventReviewEvents are the review activity events. A new head is not one
// of these: it is the pull request's commits, not a review.
var WatchEventReviewEvents = map[WatchEvent]struct{}{
	WatchEventApproved: {}, WatchEventCommented: {}, WatchEventChangesRequested: {},
}

// Label is the sentence after "When" for the event, used by the Happens-when
// selector.
func (e WatchEvent) Label() string {
	switch e {
	case WatchEventMerged:
		return "the pull request is merged"
	case WatchEventClosed:
		return "the pull request is closed unmerged"
	case WatchEventChecksFailed:
		return "the pull request's checks are failing"
	case WatchEventConflicted:
		return "the pull request has merge conflicts"
	case WatchEventBehind:
		return "the pull request is behind its base branch"
	case WatchEventApproved:
		return "the pull request is approved"
	case WatchEventCommented:
		return "a reviewer commented on the pull request"
	case WatchEventChangesRequested:
		return "a reviewer asked for changes"
	case WatchEventNewChanges:
		return "the pull request has new changes"
	case WatchEventNone:
		return "None — none of the other configured outcomes applies"
	}
	return ""
}

// WhenClause is the watch preview's sentence after "When".
func (e WatchEvent) WhenClause() string {
	if e == WatchEventNone {
		return "none of the other configured outcomes applies"
	}
	return e.Label()
}

// TicketEvent is what a watch on the run's issue or ticket reports. Changes are
// gathered between reports, so one report can stand for several.
type TicketEvent string

const (
	TicketEventChildrenChanged TicketEvent = "childrenChanged"
	TicketEventNewComment      TicketEvent = "newComment"
	TicketEventTicketClosed    TicketEvent = "ticketClosed"
)

// AllTicketEvent lists every TicketEvent in declaration order (CaseIterable).
var AllTicketEvent = []TicketEvent{TicketEventChildrenChanged, TicketEventNewComment, TicketEventTicketClosed}

// ticketEventPriority is the order a person would want to hear of the events
// in.
var ticketEventPriority = []TicketEvent{TicketEventTicketClosed, TicketEventChildrenChanged, TicketEventNewComment}

// Label describes the event.
func (e TicketEvent) Label() string {
	switch e {
	case TicketEventChildrenChanged:
		return "a child ticket was added or changed status"
	case TicketEventNewComment:
		return "someone commented on the ticket"
	case TicketEventTicketClosed:
		return "the ticket is closed or done"
	}
	return ""
}

// Gathers reports whether the event waits for the ticket to be quiet before it
// reports.
func (e TicketEvent) Gathers() bool { return e != TicketEventTicketClosed }

// WatchTarget is what a watch stage waits on.
type WatchTarget string

const (
	// WatchTargetPullRequest is the pull request under review, or the run's own in each repository.
	WatchTargetPullRequest WatchTarget = "pullRequest"
	// WatchTargetTicket is the issue or ticket the run is about: its status, comments and children.
	WatchTargetTicket WatchTarget = "ticket"
)

// AllWatchTarget lists every WatchTarget in declaration order (CaseIterable).
var AllWatchTarget = []WatchTarget{WatchTargetPullRequest, WatchTargetTicket}

// Known reports whether x is one of the cases of WatchTarget.
func (x WatchTarget) Known() bool { return enumKnown(x, AllWatchTarget) }

// WatchTargetEvent is one value its outcomes may name on a watch of a target,
// with what it means: the `when` value and its label.
type WatchTargetEvent struct {
	When  string
	Label string
}

// Label is the name shown for the target.
func (t WatchTarget) Label() string {
	switch t {
	case WatchTargetPullRequest:
		return "The pull request"
	case WatchTargetTicket:
		return "The issue or ticket"
	}
	return ""
}

// Title is the name a new watch on the target gets.
func (t WatchTarget) Title() string {
	switch t {
	case WatchTargetPullRequest:
		return "Watch the pull request"
	case WatchTargetTicket:
		return "Watch the ticket"
	}
	return ""
}

// Events are the `when` values the target's outcomes may name, with what each
// means.
func (t WatchTarget) Events() []WatchTargetEvent {
	var out []WatchTargetEvent
	switch t {
	case WatchTargetPullRequest:
		for _, e := range AllWatchEvent {
			out = append(out, WatchTargetEvent{When: string(e), Label: e.Label()})
		}
	case WatchTargetTicket:
		for _, e := range AllTicketEvent {
			out = append(out, WatchTargetEvent{When: string(e), Label: e.Label()})
		}
	}
	return out
}

// LabelOf is what `when` means on a watch of this target.
func (t WatchTarget) LabelOf(when string) string {
	for _, e := range t.Events() {
		if e.When == when {
			return e.Label
		}
	}
	return when
}

// Clone is a deep copy of the outcome, including its then.
func (o Outcome) Clone() Outcome {
	return cloneValue(o)
}

// ButtonLabel is the words an approval's button for the outcome shows: its when,
// else its name.
func (o Outcome) ButtonLabel() string {
	if o.When != "" {
		return o.When
	}
	return o.Name
}

// ApprovalPrimary is the outcome an approval stage puts its wide button on: the
// first with the good tone, else the first outcome. Nil when there are none.
// It returns a pointer into outcomes, so callers must not keep it past a change.
func ApprovalPrimary(outcomes []Outcome) *Outcome {
	for i := range outcomes {
		if outcomes[i].Tone == ToneGood {
			return &outcomes[i]
		}
	}
	if len(outcomes) > 0 {
		return &outcomes[0]
	}
	return nil
}

// IsWatchEvent reports whether e is one of the WatchEvent cases.
func IsWatchEvent(e WatchEvent) bool { return enumKnown(e, AllWatchEvent) }

// IsTicketEvent reports whether e is one of the TicketEvent cases.
func IsTicketEvent(e TicketEvent) bool { return enumKnown(e, AllTicketEvent) }
