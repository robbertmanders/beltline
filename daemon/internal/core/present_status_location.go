// The status reads for a run's place on the belt: where it is in one line, and how far
// along the belt it has got. The types and stage statuses are in present_status.go.

package core

import (
	"slices"
	"strconv"
	"strings"
)

// StatusWhere is where a run is, in one line, with its look and glyph.
type StatusWhere struct {
	Text  string          `json:"text"`
	Look  StageStatusLook `json:"look"`
	Glyph string          `json:"glyph"`
}

// StatusLocation is where a run is, in one line: "Spec · running".
func StatusLocation(run Run, paused bool, pause Pause) StatusWhere {
	name := run.StageID
	if stage := run.stageRef(); stage != nil {
		name = stage.Name
	}
	if word := RunPauseStatusLocationWord(run); word != nil {
		look := StageStatusLookRunning
		if run.State == RunStateHeld {
			look = StageStatusLookHeld
		}
		return StatusWhere{Text: name + " · " + *word, Look: look, Glyph: "◐"}
	}
	switch run.State {
	case RunStateShipped, RunStateDone:
		word := runCompletedWord(run)
		if pull := run.PullRequest(); pull != nil {
			return StatusWhere{Text: word + " · #" + strconv.Itoa(pull.Number), Look: StageStatusLookDone, Glyph: "✓"}
		}
		return StatusWhere{Text: word, Look: StageStatusLookDone, Glyph: "✓"}
	case RunStateHeld:
		if run.Reason != nil && strings.HasPrefix(*run.Reason, "WAITING FOR ROUTES") {
			return StatusWhere{Text: "Waiting for routes · " + *run.Reason, Look: StageStatusLookHeld, Glyph: "✕"}
		}
		// A held run without a reason shows "held".
		reason := ValueOr(run.Reason, "held")
		return StatusWhere{Text: name + " · " + reason, Look: StageStatusLookHeld, Glyph: "✕"}
	case RunStateParked:
		return StatusWhere{Text: name + " · parked", Look: StageStatusLookPlain, Glyph: "–"}
	case RunStateDropped:
		return StatusWhere{Text: name + " · dropped", Look: StageStatusLookPlain, Glyph: "–"}
	case RunStateWaiting:
		return StatusWhere{Text: name + " · waiting for you", Look: StageStatusLookWaiting, Glyph: "!"}
	case RunStateRunning:
		// Only an agent stage has a session to suspend, so only it says paused.
		// While paused the freeze is the news, and a split run's reason, which
		// names the route it is working, comes back when you continue.
		word := statusReasonOr(run, "running")
		if pause.IsPaused && run.stageRef() != nil && run.stageRef().Kind() == StageKindAgent {
			word = "paused"
		}
		return StatusWhere{Text: name + " · " + word, Look: StageStatusLookRunning, Glyph: "◐"}
	case RunStateQueued:
		word := statusReasonOr(run, "queued")
		if paused || pause.IsPaused {
			word = "paused"
		}
		return StatusWhere{Text: name + " · " + word, Look: StageStatusLookPlain, Glyph: "·"}
	}
	return StatusWhere{}
}

// runCompletedWord is how a completed run's ending is named: "Shipped", else "Done".
func runCompletedWord(run Run) string {
	if run.State == RunStateShipped {
		return "Shipped"
	}
	return "Done"
}

// statusReasonOr is the run's reason, or fallback when it has none.
func statusReasonOr(run Run, fallback string) string {
	if run.Reason != nil {
		return *run.Reason
	}
	return fallback
}

// StatusProgress is how far along the belt a run is, from 0 to 1: over every route it
// took at once, so a run part-way along two legs sits below halfway.
func StatusProgress(run Run) float64 {
	layout := run.Layout()
	routes := run.Routes()
	paths := make([][]Stage, 0, len(run.Legs))
	for _, leg := range run.Legs {
		choices := map[string][]string{}
		if leg.StageID != nil {
			for _, way := range layout.Ancestry(*leg.StageID) {
				choices[way.Fork.ID] = []string{way.Route.ID}
			}
		}
		if len(choices) == 0 && leg.Route != nil {
			if fork := layout.ForkWithRoute(*leg.Route); fork != nil {
				choices[fork.ID] = []string{*leg.Route}
			}
		}
		if len(choices) == 0 {
			paths = append(paths, layout.Path(routes, true))
			continue
		}
		for fork, taken := range routes {
			if _, ok := choices[fork]; !ok {
				choices[fork] = taken
			}
		}
		paths = append(paths, layout.Path(choices, true))
	}
	anyStage := slices.ContainsFunc(paths, func(path []Stage) bool { return len(path) > 0 })
	if len(paths) == 0 || !anyStage {
		return 0
	}
	if run.State.Completed() {
		return 1
	}
	done := 0.0
	for i, leg := range run.Legs {
		if i >= len(paths) {
			break
		}
		path := paths[i]
		if len(path) == 0 {
			continue
		}
		if leg.IsFinished() {
			done += float64(len(path))
			continue
		}
		at := 0.0
		if leg.StageID != nil {
			for index, stage := range path {
				if stage.ID == *leg.StageID {
					at = float64(index)
					break
				}
			}
		}
		half := 0.0
		if leg.State == RunStateRunning {
			half = 0.5
		}
		done += at + half
	}
	total := 0.0
	for _, path := range paths {
		total += float64(len(path))
	}
	if total == 0 {
		return 0
	}
	return done / total
}
