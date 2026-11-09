// The daemon's tick lanes: the parts of a tick that wait on different things, each
// passing on its own loop, so a slow GitHub, Jira or checkout holds up only its own lane.

package coordinator

import "context"

// Lane is one independently ticked part of the coordinator's work.
type Lane string

const (
	// LaneSessions is recovery, the library and assistant, schedules, agent
	// sessions and AFK decisions: only local files and tmux.
	LaneSessions Lane = "sessions"
	// LaneStarts starts queued stages, one at a time, so a stage's slots stay
	// counted. A start can wait on git.
	LaneStarts Lane = "starts"
	// LaneIntake checks the triggers, waiting on GitHub and Jira.
	LaneIntake Lane = "intake"
	// LaneWatches checks pull requests and tickets, waiting on GitHub and Jira.
	LaneWatches Lane = "watches"
)

// AllLanes lists every lane.
var AllLanes = []Lane{LaneSessions, LaneStarts, LaneIntake, LaneWatches}

// TickLane is one pass of lane. It does nothing while that lane or a full Tick is running.
// Lock: exported entry point; takes c.mu. Suspends: at each await of the lane's parts.
func (c *Coordinator) TickLane(ctx context.Context, lane Lane) {
	c.Lock()
	defer c.EndTurn()
	if _, running := c.runningLanes[lane]; running || c.ticking {
		return
	}
	if c.runningLanes == nil {
		c.runningLanes = map[Lane]struct{}{}
	}
	c.runningLanes[lane] = struct{}{}
	defer delete(c.runningLanes, lane)
	c.passLane(ctx, lane)
}

// StartLanes makes kick wake the lanes instead of ticking, and returns each lane's
// wake signal for its loop. Call it once, after a first LaneSessions pass has
// recovered what a restart left, so no other lane acts on unrecovered runs.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) StartLanes() map[Lane]<-chan struct{} {
	c.Lock()
	defer c.EndTurn()
	c.laneWakes = map[Lane]chan struct{}{}
	wakes := map[Lane]<-chan struct{}{}
	for _, lane := range AllLanes {
		wake := make(chan struct{}, 1)
		c.laneWakes[lane] = wake
		wakes[lane] = wake
	}
	return wakes
}

// wakeLane ends the lane's wait, or its next one when a pass is running. Wakes
// coalesce. Before StartLanes it does nothing.
// Lock: runs with c.mu held.
func (c *Coordinator) wakeLane(lane Lane) {
	select {
	case c.laneWakes[lane] <- struct{}{}:
	default:
	}
}

// passLane runs lane's parts. While paused, only what also runs paused in a tick does.
// Lock: runs with c.mu held. Suspends: at each part's awaits. Spawns: none.
func (c *Coordinator) passLane(ctx context.Context, lane Lane) {
	if lane == LaneSessions {
		c.tickUpkeep(ctx)
	}
	if c.globallyPaused {
		return
	}
	switch lane {
	case LaneSessions:
		c.intake.fireSchedules()
		c.sessions.check(ctx)
		c.autopilot.check(ctx)
		c.sessions.startContinuations(ctx)
		c.sessions.stopStaleContinues(ctx)
	case LaneStarts:
		c.sessions.startQueued(ctx)
	case LaneIntake:
		c.intake.pollTriggers(ctx, false)
	case LaneWatches:
		c.watches.check(ctx, false)
		c.ticketWatches.check(ctx, false)
	}
	// What this pass moved on may be queued now: a tick would start it in the same pass.
	if lane != LaneStarts {
		c.wakeLane(LaneStarts)
	}
}
