package core

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// StageStatusLook is the colour family of a stage's status.
type StageStatusLook string

const (
	StageStatusLookDone    StageStatusLook = "done"
	StageStatusLookRunning StageStatusLook = "running"
	StageStatusLookWaiting StageStatusLook = "waiting"
	StageStatusLookHeld    StageStatusLook = "held"
	StageStatusLookPlain   StageStatusLook = "plain"
)

// StageStatus is how a stage looks for one run: its status text, its colour, and whether a pause holds it.
type StageStatus struct {
	Text   string          `json:"text"`
	Look   StageStatusLook `json:"look"`
	Paused bool            `json:"paused"`
}

// StatusStep is where a stage stands for one run; skipped is
// a stage on a route the run didn't take.
type StatusStep string

const (
	StatusStepDone    StatusStep = "done"
	StatusStepRunning StatusStep = "running"
	StatusStepWaiting StatusStep = "waiting"
	StatusStepQueued  StatusStep = "queued"
	StatusStepHeld    StatusStep = "held"
	StatusStepParked  StatusStep = "parked"
	StatusStepDropped StatusStep = "dropped"
	StatusStepRedo    StatusStep = "redo"
	StatusStepTodo    StatusStep = "todo"
	StatusStepSkipped StatusStep = "skipped"
)

// StatusStepOf is the step of the stage with stageID in run.
func StatusStepOf(run Run, stageID string) StatusStep {
	index := -1
	for i := range run.Stages {
		if run.Stages[i].ID == stageID {
			index = i
			break
		}
	}
	if index < 0 {
		return StatusStepTodo
	}
	// A stage on a route the run took is never "not taken", even while
	// several routes are being worked at once.
	routes := run.Routes()
	for _, way := range run.Layout().Ancestry(stageID) {
		if taken, ok := routes[way.Fork.ID]; ok && len(taken) > 0 && !slices.Contains(taken, way.Route.ID) {
			return StatusStepSkipped
		}
	}
	if run.State.Completed() {
		return StatusStepDone
	}
	if leg := run.LegAt(&stageID); leg != nil {
		if step, ok := runStateStep(leg.State); ok {
			return step
		}
	}
	current := run.StageIndex()
	if current == nil {
		return StatusStepTodo
	}
	if index < *current {
		return StatusStepDone
	}
	if index > *current {
		for _, attempt := range run.Attempts {
			if attempt.StageID == stageID && !attempt.IsOpen() {
				return StatusStepRedo
			}
		}
		return StatusStepTodo
	}
	if step, ok := runStateStep(run.State); ok {
		return step
	}
	return StatusStepTodo
}

// runStateStep is the step a run or leg in state shows for a stage it is at.
// It reports false for a state that shows no step of its own.
func runStateStep(state RunState) (StatusStep, bool) {
	switch state {
	case RunStateRunning:
		return StatusStepRunning, true
	case RunStateWaiting:
		return StatusStepWaiting, true
	case RunStateQueued:
		return StatusStepQueued, true
	case RunStateHeld:
		return StatusStepHeld, true
	case RunStateParked:
		return StatusStepParked, true
	case RunStateDropped:
		return StatusStepDropped, true
	case RunStateDone, RunStateShipped:
		return StatusStepDone, true
	}
	return "", false
}

// StatusLegRow is one row per route the run took, with its stage and that stage's status.
type StatusLegRow struct {
	ID    string          `json:"id"`
	Route *string         `json:"route,omitzero"`
	Stage string          `json:"stage"`
	State RunState        `json:"state"`
	Look  StageStatusLook `json:"look"`
	// Paused is whether a pause holds the leg's stage.
	Paused bool `json:"paused"`
}

// StatusLegs are the legs of a split run, one per route taken. Each leg's row reads
// through the same global pause as the stage's own.
func StatusLegs(run Run, paused bool, pause Pause, now time.Time) []StatusLegRow {
	if !run.IsSplit() {
		return []StatusLegRow{}
	}
	layout := run.Layout()
	rows := make([]StatusLegRow, 0, len(run.Legs))
	for _, leg := range run.Legs {
		stage := leg.Stage(run)
		if stage == nil || leg.IsFinished() {
			rows = append(rows, StatusLegRow{ID: leg.ID, Route: leg.RouteName(layout), Stage: "finished", State: leg.State,
				Look: StageStatusLookDone})
			continue
		}
		status := StatusStage(run, *stage, paused, pause, now)
		path := layout.PathOf(stage.ID)
		route := leg.RouteName(layout)
		if len(path) > 0 {
			joined := strings.Join(path, " / ")
			route = &joined
		}
		rows = append(rows, StatusLegRow{ID: leg.ID, Route: route, Stage: stage.Name, State: leg.State, Look: status.Look, Paused: status.Paused})
	}
	return rows
}

// StatusStage is the status of stage in run. paused is the belt's own pause; pause
// is the app's one global pause.
func StatusStage(run Run, stage Stage, paused bool, pause Pause, now time.Time) StageStatus {
	return RunPauseStatusOverlay(statusUnderlying(run, stage, paused, pause, now), run, stage)
}

// statusUnderlying is the stage status before the pause overlay.
func statusUnderlying(run Run, stage Stage, paused bool, pause Pause, now time.Time) StageStatus {
	attempt := run.LastAttempt(stage.ID)
	switch StatusStepOf(run, stage.ID) {
	case StatusStepDone:
		if attempt != nil && attempt.EndedAt != nil {
			return StageStatus{Text: "✓ " + StatusDuration(attempt.EndedAt.Sub(attempt.StartedAt.Time).Seconds()), Look: StageStatusLookDone}
		}
		return StageStatus{Text: "✓ DONE", Look: StageStatusLookDone}
	case StatusStepRunning:
		elapsed := 0.0
		if attempt != nil {
			elapsed = pause.Elapsed(attempt.StartedAt.Time, now)
		}
		if statusIsTicketWatch(stage) {
			text := "WATCHING " + run.Subject.Ref()
			if observation, ok := run.TicketObservations[stage.ID]; ok {
				open := 0
				for _, child := range observation.Children {
					if !child.IsOpen {
						open++
					}
				}
				text = strconv.Itoa(open) + " OF " + strconv.Itoa(len(observation.Children)) + " DONE"
			}
			return StageStatus{Text: text, Look: StageStatusLookRunning}
		}
		if stage.Kind() == StageKindWatch {
			text := "LOOKING FOR THE PR"
			if len(run.PullRequests) > 1 {
				text = "WATCHING " + strconv.Itoa(len(run.PullRequests)) + " PRS"
			} else if pull := run.PullRequest(); pull != nil {
				text = "WATCHING #" + strconv.Itoa(pull.Number)
			}
			return StageStatus{Text: text, Look: StageStatusLookRunning}
		}
		// A suspended agent is still running; only its clock waits.
		if pause.IsPaused {
			return StageStatus{Text: "PAUSED", Look: StageStatusLookRunning, Paused: true}
		}
		number := 1
		if attempt != nil {
			number = attempt.Number
		}
		word := "RUNNING"
		if number > 1 {
			word = "RETRY"
		}
		return StageStatus{Text: word + " " + StatusClock(elapsed), Look: StageStatusLookRunning}
	case StatusStepWaiting:
		return StageStatus{Text: "WAITING FOR YOU", Look: StageStatusLookWaiting}
	case StatusStepQueued:
		stopped := paused || pause.IsPaused
		if stopped {
			return StageStatus{Text: "PAUSED", Look: StageStatusLookPlain, Paused: true}
		}
		return StageStatus{Text: "QUEUED", Look: StageStatusLookPlain}
	case StatusStepHeld:
		reason := ValueOr(run.Reason, "held")
		return StageStatus{Text: UppercasedFull(reason), Look: StageStatusLookHeld}
	case StatusStepParked:
		return StageStatus{Text: "PARKED", Look: StageStatusLookPlain, Paused: true}
	case StatusStepDropped:
		return StageStatus{Text: "DROPPED", Look: StageStatusLookPlain, Paused: true}
	case StatusStepRedo:
		return StageStatus{Text: "WILL REDO", Look: StageStatusLookPlain}
	case StatusStepTodo:
		return StageStatus{Text: "NOT STARTED", Look: StageStatusLookPlain}
	case StatusStepSkipped:
		return StageStatus{Text: "NOT TAKEN", Look: StageStatusLookPlain}
	}
	return StageStatus{}
}

// statusIsTicketWatch reports whether stage is a watch that waits on its ticket.
func statusIsTicketWatch(stage Stage) bool {
	target := stage.WatchTarget()
	return target != nil && *target == WatchTargetTicket
}
