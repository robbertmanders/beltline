//
// The Floor: every merged stage as one stage box, each belt's entrance card on
// the left, and up to three run tags under each box. The browser lays them out.

package core

import "strconv"

// OverviewBoxesTagLimit is the most run tags a box shows before one count of the rest.
const OverviewBoxesTagLimit = 3

// Copy used by OverviewBoxes. Literal text is exact.
const (
	overviewBoxesRunning = " running"
)

// OverviewBoxesCaption is what a stage's box is captioned: its kind, and how many belts share it.
func OverviewBoxesCaption(stage Stage, belts int) string {
	var kind string
	switch stage.Kind() {
	case StageKindAgent:
		kind = "Agent"
	case StageKindGate:
		kind = "Approval"
	default:
		kind = "Watch"
	}
	if belts > 1 {
		return kind + " · " + strconv.Itoa(belts) + " belts"
	}
	return kind
}

// OverviewBoxesStatus is the status above a box, from the runs at it: held and waiting first,
// then running, else idle.
func OverviewBoxesStatus(here []Run) (text string, look StageStatusLook) {
	held, waiting, running := 0, 0, 0
	for _, run := range here {
		switch run.State {
		case RunStateHeld:
			held++
		case RunStateWaiting:
			waiting++
		case RunStateRunning:
			running++
		}
	}
	if held > 0 {
		if running > 0 {
			return strconv.Itoa(held) + " held · " + strconv.Itoa(running) + overviewBoxesRunning, StageStatusLookHeld
		}
		return strconv.Itoa(held) + " held", StageStatusLookHeld
	}
	if waiting > 0 {
		return strconv.Itoa(waiting) + " waiting for you", StageStatusLookWaiting
	}
	if running > 0 {
		return strconv.Itoa(running) + overviewBoxesRunning, StageStatusLookRunning
	}
	return "Idle", StageStatusLookPlain
}
