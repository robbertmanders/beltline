// A run's belt drawn as stage boxes for the run page: the daemon's geometry
// (BoxMap) with the words, state and chip each box shows, and the state of
// every track. The page draws it as it is.

package core

import (
	"fmt"
	"slices"
)

// RunBoxesState is what a box looks like for a run. Needs and held are the ones that want you;
// skip is a route the run did not take.
type RunBoxesState string

const (
	RunBoxesStateIdle     RunBoxesState = "idle"
	RunBoxesStateDone     RunBoxesState = "done"
	RunBoxesStateTodo     RunBoxesState = "todo"
	RunBoxesStateSkip     RunBoxesState = "skip"
	RunBoxesStateNeeds    RunBoxesState = "needs"
	RunBoxesStateHeld     RunBoxesState = "held"
	RunBoxesStateRunning  RunBoxesState = "running"
	RunBoxesStateWatching RunBoxesState = "watching"
	RunBoxesStatePaused   RunBoxesState = "paused"
)

// Reached reports whether the run has got to this box, or is at it.
func (s RunBoxesState) Reached() bool {
	switch s {
	case RunBoxesStateDone, RunBoxesStateNeeds, RunBoxesStateHeld, RunBoxesStateRunning, RunBoxesStateWatching, RunBoxesStatePaused:
		return true
	default:
		return false
	}
}

// RunBoxesDot is the corner dot of a box: needs for a stage waiting for you, held for a held run.
type RunBoxesDot string

const (
	RunBoxesDotNeeds RunBoxesDot = "needs"
	RunBoxesDotHeld  RunBoxesDot = "held"
)

// RunBoxesTrack is how a track is drawn: travelled, still to come, or a route not taken.
type RunBoxesTrack string

const (
	RunBoxesTrackDone RunBoxesTrack = "done"
	RunBoxesTrackTodo RunBoxesTrack = "todo"
	RunBoxesTrackSkip RunBoxesTrack = "skip"
)

// RunBoxesChip is the status chip above a box. Look is a StageStatus.Look raw value.
type RunBoxesChip struct {
	Text string `json:"text"`
	Look string `json:"look"`
}

// RunBoxesNode is one box of a run's belt.
type RunBoxesNode struct {
	// ID is the stage's id, or `trigger`.
	ID    string        `json:"id"`
	State RunBoxesState `json:"state"`
	// Caption is `02 · Agent`, `Approval` or `Watch`; `Trigger` for the trigger.
	Caption string `json:"caption"`
	Title   string `json:"title"`
	// Details are under the title: the runner, `you` or `github`; the trigger's kind.
	Details []string      `json:"details"`
	Chip    *RunBoxesChip `json:"chip,omitzero"`
	Dot     *RunBoxesDot  `json:"dot,omitzero"`
}

// RunBoxesEdge is one track of a run's belt.
type RunBoxesEdge struct {
	From  string        `json:"from"`
	To    string        `json:"to"`
	State RunBoxesTrack `json:"state"`
	// Route is the route the track runs on (as the map's edge), nil on the line. With From and To it names the track.
	Route *string `json:"route,omitzero"`
}

// RunBoxes is a run's belt as boxes for the run page: the map, its nodes and edges, the routes taken
// and the caption over the belt.
type RunBoxes struct {
	Map   BoxMap         `json:"map"`
	Nodes []RunBoxesNode `json:"nodes"`
	Edges []RunBoxesEdge `json:"edges"`
	// TakenRoutes are the routes the run took, as `fork:route`, matching the map's route pills.
	TakenRoutes []string `json:"takenRoutes"`
	// Caption is the caption over the belt: where the run is now, or what it shipped as.
	Caption string `json:"caption"`
}

// Node is the node with id, or nil.
func (b RunBoxes) Node(id string) *RunBoxesNode {
	for index := range b.Nodes {
		if b.Nodes[index].ID == id {
			node := b.Nodes[index]
			return &node
		}
	}
	return nil
}

// RunBoxesProject is the run's belt as boxes. stages are the run page's stage panels, which
// already hold each stage's step, status, number and worker.
func RunBoxesProject(run Run, stages []RunPresentationStagePanel, ambiguous map[string]struct{}, look func(stage Stage) StageLook) RunBoxes {
	var kind *TriggerKind
	if run.Source != nil {
		value := run.Source.Trigger.Kind
		kind = &value
	}
	mapped := BoxLayoutMap(run.Layout(), BoxMapMetricsRun, true, kind, look)
	panels := map[string]RunPresentationStagePanel{}
	for _, panel := range stages {
		if _, seen := panels[panel.ID]; !seen {
			panels[panel.ID] = panel
		}
	}
	nodes := []RunBoxesNode{{
		ID: "trigger", State: RunBoxesStateIdle, Caption: "Trigger", Title: run.Subject.Ref(),
		Details: []string{RunBoxesTriggerSubtitle(kind)},
	}}
	for _, stage := range run.Stages {
		panel, ok := panels[stage.ID]
		if !ok {
			continue
		}
		nodes = append(nodes, runBoxesNode(panel, stage, run))
	}
	// The trigger is always on the line, so its track counts as travelled.
	states := map[string]RunBoxesState{}
	for _, node := range nodes {
		if _, seen := states[node.ID]; !seen {
			states[node.ID] = node.State
		}
	}
	edges := []RunBoxesEdge{}
	for _, edge := range mapped.Edges {
		from := RunBoxesStateDone
		if edge.From != "trigger" {
			from = RunBoxesStateTodo
			if state, ok := states[edge.From]; ok {
				from = state
			}
		}
		to := RunBoxesStateTodo
		if state, ok := states[edge.To]; ok {
			to = state
		}
		track := RunBoxesTrackTodo
		switch {
		case from == RunBoxesStateSkip || to == RunBoxesStateSkip || (edge.Route != nil && runBoxesNotTaken(*edge.Route, run)):
			track = RunBoxesTrackSkip
		case from.Reached() && to.Reached():
			track = RunBoxesTrackDone
		}
		edges = append(edges, RunBoxesEdge{From: edge.From, To: edge.To, State: track, Route: edge.Route})
	}
	routes := run.Routes()
	taken := []string{}
	for _, route := range mapped.Routes {
		chosen := routes[route.Fork]
		isTaken := false
		if len(chosen) == 0 {
			for _, id := range route.StageIDs {
				if state, ok := states[id]; ok && state.Reached() {
					isTaken = true
					break
				}
			}
		} else {
			isTaken = slices.Contains(chosen, route.Route)
		}
		if isTaken {
			taken = append(taken, route.Fork+":"+route.Route)
		}
	}
	return RunBoxes{
		Map: mapped, Nodes: nodes, Edges: edges, TakenRoutes: taken,
		Caption: RunPresentationCaption(run, ambiguous),
	}
}

// runBoxesNotTaken reports whether a route is decided against: its fork has a route taken and this
// is not it, or a route around its fork is. An empty route has no box to say so, and its track follows
// the fork's choice.
func runBoxesNotTaken(routeID string, run Run) bool {
	layout := run.Layout()
	routes := run.Routes()
	current := routeID
	seen := map[string]struct{}{}
	for {
		if _, again := seen[current]; again {
			return false
		}
		seen[current] = struct{}{}
		fork := layout.ForkWithRoute(current)
		if fork == nil {
			return false
		}
		chosen := routes[fork.ID]
		if len(chosen) > 0 && !slices.Contains(chosen, current) {
			return true
		}
		if fork.Parent == nil {
			return false
		}
		current = *fork.Parent
	}
}

// RunBoxesTriggerSubtitle is the subtitle under a trigger's ref; nil is the issues trigger.
func RunBoxesTriggerSubtitle(kind *TriggerKind) string {
	if kind == nil {
		return runBoxesGitHubIssue
	}
	switch *kind {
	case TriggerKindJira:
		return "jira ticket"
	case TriggerKindSchedule:
		return "schedule"
	case TriggerKindManual:
		return "task"
	case TriggerKindReviews:
		return "pull request"
	default:
		return runBoxesGitHubIssue
	}
}

// runBoxesNode is one stage's box. The chip and state follow the stage's step, the same statuses the
// stage panels show.
func runBoxesNode(panel RunPresentationStagePanel, stage Stage, run Run) RunBoxesNode {
	caption := fmt.Sprintf("%02d", panel.Number) + " · " + runBoxesKindWord(stage.Kind())
	details := []string{}
	if panel.Worker != nil {
		details = []string{*panel.Worker}
	}
	paused := panel.Status.Paused
	node := func(state RunBoxesState, chip *RunBoxesChip, dot *RunBoxesDot) RunBoxesNode {
		return RunBoxesNode{ID: stage.ID, State: state, Caption: caption, Title: panel.Name, Details: details, Chip: chip, Dot: dot}
	}
	plain := func(text string) *RunBoxesChip {
		return &RunBoxesChip{Text: text, Look: string(StageStatusLookPlain)}
	}
	chip := func(text string, look StageStatusLook) *RunBoxesChip {
		return &RunBoxesChip{Text: text, Look: string(look)}
	}
	needs, held := RunBoxesDotNeeds, RunBoxesDotHeld
	switch panel.Step {
	case StatusStepDone:
		text := panel.Status.Text
		if stage.Kind() == StageKindGate {
			text = "✓ approved"
		}
		return node(RunBoxesStateDone, chip(text, StageStatusLookDone), nil)
	case StatusStepRunning:
		if stage.Kind() == StageKindWatch {
			return node(RunBoxesStateWatching, plain(panel.Status.Text), nil)
		}
		if paused {
			return node(RunBoxesStatePaused, chip(runBoxesPaused, StageStatusLookRunning), nil)
		}
		return node(RunBoxesStateRunning, chip("Running", StageStatusLookRunning), nil)
	case StatusStepWaiting:
		return node(RunBoxesStateNeeds, chip("Waiting for you", StageStatusLookWaiting), &needs)
	case StatusStepHeld:
		text := ValueOr(run.Reason, "Held")
		return node(RunBoxesStateHeld, chip(text, StageStatusLookHeld), &held)
	case StatusStepQueued:
		if paused {
			return node(RunBoxesStatePaused, chip(runBoxesPaused, StageStatusLookRunning), nil)
		}
		return node(RunBoxesStateTodo, plain("Queued"), nil)
	case StatusStepParked:
		return node(RunBoxesStateTodo, plain("Parked"), nil)
	case StatusStepDropped:
		return node(RunBoxesStateTodo, plain("Dropped"), nil)
	case StatusStepRedo:
		return node(RunBoxesStateTodo, plain("Will redo"), nil)
	case StatusStepTodo:
		return node(RunBoxesStateTodo, plain("Not started"), nil)
	default:
		return node(RunBoxesStateSkip, plain("Not taken"), nil)
	}
}

// runBoxesKindWord is a stage kind's caption word.
func runBoxesKindWord(kind StageKind) string {
	switch kind {
	case StageKindAgent:
		return "Agent"
	case StageKindGate:
		return "Approval"
	default:
		return "Watch"
	}
}

// Copy used by RunBoxes. Literal text is exact.
const (
	runBoxesPaused      = "Paused"
	runBoxesGitHubIssue = "github issue"
)
