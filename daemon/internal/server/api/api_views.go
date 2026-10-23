package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// Nested types are named by joining their names: a nested type `X.Y` is `XY` at package level.

// WaitingResponse is `GET /api/v1/waiting`: the subjects intake last found waiting.
type WaitingResponse struct {
	Waiting []WaitingSubjectWire `json:"waiting"`
}

// NewWaitingResponse builds the waiting response from its arguments.
func NewWaitingResponse(waiting []core.WaitingSubject) WaitingResponse {
	out := WaitingResponse{Waiting: []WaitingSubjectWire{}}
	for _, subject := range waiting {
		out.Waiting = append(out.Waiting, NewWaitingSubjectWire(subject))
	}
	return out
}

// WaitingSubjectWire is one subject intake found waiting.
type WaitingSubjectWire struct {
	BeltID  string       `json:"beltId"`
	Subject core.Subject `json:"subject"`
	// Reference is the subject's short reference, such as "#9001" or "PAY-42".
	Reference string `json:"reference"`
	Reason    string `json:"reason"`
}

// NewWaitingSubjectWire builds the waiting subject wire from its arguments.
func NewWaitingSubjectWire(waiting core.WaitingSubject) WaitingSubjectWire {
	return WaitingSubjectWire{
		BeltID:    waiting.BeltID,
		Subject:   waiting.Subject,
		Reference: waiting.Subject.Ref(),
		Reason:    waiting.Reason,
	}
}
