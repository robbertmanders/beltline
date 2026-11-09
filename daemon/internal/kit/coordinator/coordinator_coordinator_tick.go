// The coordinator's tick and check passes: Tick, CheckNow and FetchNow, the pass
// over every part (tick), and the global pause that stops and resumes sessions.
// Each takes c.mu for callers outside a turn.

package coordinator

import (
	"context"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// Tick is one pass over everything in one sequence; the daemon runs the same
// parts as lanes instead (see TickLane). It recovers erasures, overrules, run pauses and park stops,
// polls the assistant, and, unless paused, polls triggers, fires schedules,
// and advances sessions, watches, ticket watches and AFK decisions.
// Lock: exported entry point; takes c.mu. Suspends: at each await
// (recoverErasures, recoverOverrules, recoverRunPauses, recoverParkStops,
// assistant.reconcile, intake.pollTriggers, sessions.check, watches.check,
// ticketWatches.check, autopilot.check, sessions.startQueued,
// sessions.startContinuations, sessions.stopStaleContinues). Spawns: none.
func (c *Coordinator) Tick(ctx context.Context) {
	c.Lock()
	defer c.EndTurn()
	c.tick(ctx)
}

// CheckNow checks every trigger and pull request now, then ticks.
// Lock: exported entry point; takes c.mu. Suspends: as checkNow.
func (c *Coordinator) CheckNow(ctx context.Context) {
	c.Lock()
	defer c.EndTurn()
	c.checkNow(ctx, true)
}

// FetchNow fetches source changes and returns before session reconciliation
// and checkout startup.
// Lock: exported entry point; takes c.mu. Suspends: as checkNow.
func (c *Coordinator) FetchNow(ctx context.Context) {
	c.Lock()
	defer c.EndTurn()
	c.checkNow(ctx, false)
}

// checkNow is CheckNow with waitForProcessing: true ticks, false kicks.
// Lock: runs with c.mu held. Suspends: recoverErasures, assistant.reconcile,
// intake.pollTriggers, watches.check, ticketWatches.check, and tick when
// waitForProcessing. Spawns: kick when not waiting.
func (c *Coordinator) checkNow(ctx context.Context, waitForProcessing bool) {
	c.recoverErasures(ctx)
	if c.assistant.running {
		c.assistant.Reconcile(ctx)
	}
	// Paused means paused: a check now finds nothing new to do.
	if c.globallyPaused {
		return
	}
	c.intake.pollTriggers(ctx, true)
	c.watches.check(ctx, true)
	c.ticketWatches.check(ctx, true)
	if waitForProcessing {
		c.tick(ctx)
	} else {
		c.kick()
	}
}

// SetGloballyPaused stops or starts every session the app owns and holds back
// intake, schedules, watches and queued stages while paused.
// The runs stay where they are.
// Lock: exported entry point; takes c.mu. Suspends: liveSessions, and each
// runner.suspend or runner.resume. Spawns: kick.
func (c *Coordinator) SetGloballyPaused(ctx context.Context, paused bool) {
	c.Lock()
	defer c.EndTurn()
	if paused == c.globallyPaused {
		return
	}
	c.setGloballyPausedFlag(paused)
	c.sessions.globallyPaused = paused
	c.intake.globallyPaused = paused
	names := c.liveSessions(ctx)
	now := c.clock.Now()
	c.changeSettings(func(s *store.Settings) {
		if paused {
			s.Paused = true
			at := jsonx.Date{Time: now}
			s.PausedAt = &at
			return
		}
		if since := s.PausedAt; since != nil && now.After(since.Time) {
			// A run outlives a few pauses at most; the oldest ones no longer matter.
			pauses := append(append([]store.DateInterval(nil), s.Pauses...), store.DateInterval{Start: *since, Duration: now.Sub(since.Time).Seconds()})
			if len(pauses) > 50 {
				pauses = pauses[len(pauses)-50:]
			}
			s.Pauses = pauses
		}
		s.PausedAt = nil
		s.Paused = false
	})
	for _, name := range names {
		name := name
		if paused {
			c.Suspend(func() { c.runner.Suspend(ctx, name) })
		} else {
			c.Suspend(func() { c.runner.Resume(ctx, name) })
		}
	}
	c.kick()
}

// liveSessions lists every session the app owns right now.
// Lock: runs with c.mu held. Suspends: runner.conversationAlive per continuation.
func (c *Coordinator) liveSessions(ctx context.Context) []string {
	var names []string
	if c.assistant.running {
		names = append(names, c.assistant.session)
	}
	for _, decision := range c.autopilot.Deciding() {
		if decision.Session != nil {
			names = append(names, *decision.Session)
		}
	}
	// A copy: the loop suspends, and an erasure
	// finishing meanwhile compacts c.book.runs in place.
	for _, run := range runBookSnapshot(&c.book) {
		for _, leg := range run.WorkingLegs() {
			stage := leg.Stage(run)
			if leg.State != core.RunStateRunning || stage == nil || stage.Kind() != core.StageKindAgent {
				continue
			}
			attempt := c.sessions.attempt(run, leg)
			if attempt == nil || attempt.Session == nil {
				continue
			}
			session := *attempt.Session
			names = append(names, session)
			continuing := runner.TmuxContinueName(session)
			alive := false
			c.Suspend(func() { alive = c.runner.ConversationAlive(ctx, continuing) })
			if alive {
				names = append(names, continuing)
			}
		}
	}
	return names
}

// kick wakes every lane once the daemon runs them, and else ticks in the background.
// Lanes that have nothing due pass cheaply.
// Lock: runs with c.mu held; the spawned tick takes it. Spawns: c.Spawn(tick) before StartLanes.
func (c *Coordinator) kick() {
	if c.laneWakes != nil {
		for _, lane := range AllLanes {
			c.wakeLane(lane)
		}
		return
	}
	c.Spawn(func(ctx context.Context) { c.Tick(ctx) })
}

// tick is Tick with c.mu held. Its calls into the parts run
// with the lock held: each part suspends itself at its own slow calls.
func (c *Coordinator) tick(ctx context.Context) {
	if c.ticking || len(c.runningLanes) > 0 {
		return
	}
	c.ticking = true
	defer func() { c.ticking = false }()
	c.tickUpkeep(ctx)
	// Paused means paused: nothing is picked up, fired, watched or started.
	if c.globallyPaused {
		return
	}
	c.intake.pollTriggers(ctx, false)
	c.intake.fireSchedules()
	c.sessions.check(ctx)
	c.watches.check(ctx, false)
	c.ticketWatches.check(ctx, false)
	c.autopilot.check(ctx)
	c.sessions.startQueued(ctx)
	c.sessions.startContinuations(ctx)
	c.sessions.stopStaleContinues(ctx)
}

// tickUpkeep is what a tick does whether or not the coordinator is paused: recovery,
// the library's files and the assistant.
// Lock: runs with c.mu held. Suspends: the recoveries and assistant.Reconcile.
func (c *Coordinator) tickUpkeep(ctx context.Context) {
	c.recoverErasures(ctx)
	c.recoverOverrules(ctx)
	c.recoverRunPauses(ctx)
	c.recoverParkStops(ctx)
	// Edits to the belts and agents on disk are loaded whether or not a conversation runs, and whether or not the coordinator is paused.
	c.library.reconcileFiles()
	// A running conversation is probed; otherwise only the library's files are looked at, cheaply.
	if c.assistant.running {
		c.assistant.Reconcile(ctx)
	} else {
		c.assistant.noticeChanges(false)
	}
}
