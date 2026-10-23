package core

import (
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ParkingRoute is one unfinished route as it was when the button parked the run.
type ParkingRoute struct {
	Leg       string   `json:"leg"`
	StageID   *string  `json:"stageId,omitzero"`
	State     RunState `json:"state"`
	Reason    *string  `json:"reason,omitzero"`
	Note      *string  `json:"note,omitzero"`
	Continues *string  `json:"continues,omitzero"`
	Replays   *string  `json:"replays,omitzero"`
	// Interrupted is the attempt closed as "Parked by you" when an agent was running.
	Interrupted *string `json:"interrupted,omitzero"`
	// Session is that attempt's session, which has to finish stopping.
	Session *string `json:"session,omitzero"`
}

// Parking is whole-run parking from the Run page button. Absent on older runs
// and on a run parked only by a configured outcome, which keeps that outcome's
// unpark.
//
// Encoding: synthesized: routes, stopping, stopError (omitted when nil),
// unparkPending, at. Decoding requires every key but stopError.
type Parking struct {
	Routes []ParkingRoute `json:"routes"`
	// Stopping are the sessions that must finish stopping before Unpark can start
	// a replacement. A failed stop stays here until a later stop succeeds.
	Stopping []string `json:"stopping"`
	// StopError is set when a stop failed. The run stays parked and shows this until Unpark retries the stop.
	StopError *string `json:"stopError,omitzero"`
	// UnparkPending is set when Unpark was asked while a stop was still finishing.
	UnparkPending bool       `json:"unparkPending"`
	At            jsonx.Date `json:"at"`
}
