package coordinator

import (
	"context"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// EffectTask is one stop or cleanup effect started for a run.
// done is set when the work has finished; wait is closed then (
// Task<Void, Never>, becomes a channel).
type EffectTask struct {
	done bool
	wait chan struct{}
	// stop marks a session stop: until it finishes, no session of its run may start.
	stop bool
	// awaited is set when a start of its run waited for this stop, which then ticks once it finishes.
	awaited bool
}

// trackEffect starts one stop or cleanup effect for run id, and records it until
// it finishes, so erasure can wait for it.
// Lock: runs with c.mu held. Spawns: the work, through c.Spawn. The work runs with
// c.mu held: it suspends (c.Suspend) for its slow calls.
func (c *Coordinator) trackEffect(id string, work func(ctx context.Context)) {
	c.track(id, &EffectTask{wait: make(chan struct{})}, work)
}

// trackStop is trackEffect for a session stop. The run's next start waits for it
// (stopPending), so a session that Send to displaced has stopped before its
// destination starts. Lock: runs with c.mu held.
func (c *Coordinator) trackStop(id string, work func(ctx context.Context)) {
	c.track(id, &EffectTask{wait: make(chan struct{}), stop: true}, work)
}

// track records box for run id and starts work for it.
// Lock: runs with c.mu held. Spawns: the work, through c.Spawn.
func (c *Coordinator) track(id string, box *EffectTask, work func(ctx context.Context)) {
	c.effects[id] = append(c.effects[id], box)
	c.Spawn(func(ctx context.Context) {
		c.Lock()
		defer c.EndTurn()
		defer c.finishEffect(id, box)
		work(ctx)
	})
}

// pendingStop is a session stop of run id that has not finished yet, or nil.
// Lock: runs with c.mu held.
func (c *Coordinator) pendingStop(id string) *EffectTask {
	for _, effect := range c.effects[id] {
		if effect.stop && !effect.done {
			return effect
		}
	}
	return nil
}

// finishEffect marks an effect done, drops the finished ones of its run, and wakes anyone waiting for it.
// A stop that a start waited for ticks, so that start goes on.
// Lock: runs with c.mu held.
func (c *Coordinator) finishEffect(id string, box *EffectTask) {
	box.done = true
	close(box.wait)
	c.dropDoneEffects(id)
	if box.stop && box.awaited {
		c.kick()
	}
}

// dropDoneEffects removes the finished effects of id, and forgets the run when none are left (
// `effects[id]?.removeAll { $0.done }`).
// Lock: runs with c.mu held.
func (c *Coordinator) dropDoneEffects(id string) {
	var kept []*EffectTask
	for _, effect := range c.effects[id] {
		if !effect.done {
			kept = append(kept, effect)
		}
	}
	if len(kept) == 0 {
		delete(c.effects, id)
		return
	}
	c.effects[id] = kept
}

// waitUntilQuiet waits until id has no in-flight book operation, Continue launch,
// or outstanding stop or cleanup.
// Lock: not held while waiting: the caller releases c.mu through c.Suspend.
// Suspends: waits on the effects' channels and yields.
func (c *Coordinator) waitUntilQuiet(ctx context.Context, id string) {
	for c.isOccupied(id) {
		var pending []*EffectTask
		for _, effect := range c.effects[id] {
			if !effect.done {
				pending = append(pending, effect)
			}
		}
		c.dropDoneEffects(id)
		c.Suspend(func() {
			for _, effect := range pending {
				<-effect.wait
			}
		})
		c.dropDoneEffects(id)
		if watchesHas(c.book.busy, id) || watchesHas(c.sessions.continuing, id) {
			c.Suspend(func() {
				runtime.Gosched()
				time.Sleep(5 * time.Millisecond)
			})
		}
	}
}

// isOccupied reports whether a run operation, a Continue launch, or a stop or
// cleanup effect is still in flight for id.
// Lock: runs with c.mu held.
func (c *Coordinator) isOccupied(id string) bool {
	if watchesHas(c.book.busy, id) || watchesHas(c.sessions.continuing, id) {
		return true
	}
	return slices.ContainsFunc(c.effects[id], func(effect *EffectTask) bool { return !effect.done })
}

// recoverErasures finishes erasures that were still in progress when the app last
// quit, before intake or sessions reuse those identities.
// Lock: runs with c.mu held. Suspends: performErasure.
func (c *Coordinator) recoverErasures(ctx context.Context) {
	if c.recovering {
		for c.recovering {
			c.Suspend(func() {
				runtime.Gosched()
				time.Sleep(5 * time.Millisecond)
			})
		}
		return
	}
	if c.erasuresClear {
		return
	}
	c.recovering = true
	defer func() { c.recovering = false }()
	intents := c.erasureIntents
	settled := true
	defer func() { c.erasuresClear = settled && intents == c.erasureIntents }()
	for _, pending := range c.storage.LoadPendingErasures() {
		if _, running := c.erasureRunning[pending.ID]; running {
			settled = false
			continue
		}
		c.book.markErasing(pending.ID)
		err := c.performErasure(ctx, pending)
		if err == nil {
			delete(c.erasureErrors, pending.ID)
			continue
		}
		settled = false
		message := err.Error()
		if c.erasureErrors[pending.ID] != message {
			c.erasureErrors[pending.ID] = message
			ref := pending.ID
			if run := c.book.Run(pending.ID); run != nil {
				ref = run.Subject.Ref()
			}
			runID := pending.ID
			if c.book.Run(pending.ID) == nil {
				c.book.note("Could not finish erasing "+ref+": "+message, nil)
			} else {
				c.book.note("Could not finish erasing "+ref+": "+message, &runID)
			}
		}
	}
}

// Erase permanently removes one shipped or dropped run and its local history.
// Lock: exported entry point; takes c.mu. Suspends: recoverErasures, performErasure,
// waitUntilQuiet.
func (c *Coordinator) Erase(ctx context.Context, id string) error {
	c.Lock()
	defer c.EndTurn()
	return c.erase(ctx, id)
}

// erase is Erase with c.mu held.
// Lock: runs with c.mu held. Suspends: as Erase.
func (c *Coordinator) erase(ctx context.Context, id string) error {
	c.recoverErasures(ctx)
	if existing, ok := c.storage.PendingErasure(id); ok {
		if err := c.performErasure(ctx, existing); err != nil {
			return err
		}
	}
	if c.book.Run(id) == nil {
		if _, pending := c.storage.PendingErasure(id); pending || c.book.isErasing(id) {
			return core.NewFailure("Could not erase this run. It is still here; try again.")
		}
		if c.storage.NextGeneration(id) > 0 {
			return nil
		}
		return core.NewFailure("This run is gone.")
	}
	initial := c.book.Run(id)
	if initial == nil {
		return core.NewFailure("This run is gone.")
	}
	if initial.State.IsActive() {
		return core.NewFailure("Drop the run before erasing it.")
	}
	if _, running := c.erasureRunning[id]; running {
		for {
			if _, still := c.erasureRunning[id]; !still {
				break
			}
			c.Suspend(func() {
				runtime.Gosched()
				time.Sleep(5 * time.Millisecond)
			})
		}
		if c.book.Run(id) != nil {
			return core.NewFailure("Could not erase this run. It is still here; try again.")
		}
		if _, pending := c.storage.PendingErasure(id); pending {
			return core.NewFailure("Could not erase this run. It is still here; try again.")
		}
		return nil
	}
	c.waitUntilQuiet(ctx, id)
	run := c.book.Run(id)
	if run == nil || run.Generation != initial.Generation {
		return core.NewFailure("This run changed before it could be erased. Try again.")
	}
	if run.State.IsActive() {
		return core.NewFailure("Drop the run before erasing it.")
	}
	pending := store.PendingErasure{ID: run.ID, Generation: run.Generation, Folder: run.Folder(), Repos: run.Repos, Sessions: c.sessionNames(*run)}
	c.book.markErasing(run.ID)
	c.erasureIntents++
	c.erasuresClear = false
	if err := c.storage.SavePendingErasure(pending); err != nil {
		c.book.clearErasing(run.ID)
		return core.NewFailure("Could not record erasure: " + err.Error() + ". Nothing was removed.")
	}
	if err := c.performErasure(ctx, pending); err != nil {
		return err
	}
	return nil
}

// sessionNames returns the session names belonging to run: each attempt, its
// continued session, and its AFK sessions.
// Lock: runs with c.mu held.
func (c *Coordinator) sessionNames(run core.Run) []string {
	names := map[string]struct{}{}
	for _, attempt := range run.Attempts {
		if attempt.Session == nil {
			continue
		}
		names[*attempt.Session] = struct{}{}
		names[runner.TmuxContinueName(*attempt.Session)] = struct{}{}
	}
	for _, decision := range c.autopilot.DecisionsFor(run.ID) {
		if decision.Session != nil {
			names[*decision.Session] = struct{}{}
		}
	}
	return sortedNames(names)
}

// sortedNames returns the keys of a set in order.
// Lock: none (pure).
func sortedNames(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// performErasure removes the run's folders, sessions and records, in the order
// that keeps a restart able to finish it.
// Lock: runs with c.mu held. Suspends: runner.ensureStopped, workspaces.remove.
func (c *Coordinator) performErasure(ctx context.Context, pending store.PendingErasure) error {
	if _, running := c.erasureRunning[pending.ID]; running {
		for {
			if _, still := c.erasureRunning[pending.ID]; !still {
				break
			}
			c.Suspend(func() {
				runtime.Gosched()
				time.Sleep(5 * time.Millisecond)
			})
		}
		return nil
	}
	c.erasureRunning[pending.ID] = struct{}{}
	defer delete(c.erasureRunning, pending.ID)
	c.book.markErasing(pending.ID)
	if !pending.StoppedSessions {
		names := map[string]struct{}{}
		for _, name := range pending.Sessions {
			names[name] = struct{}{}
		}
		if run := c.book.Run(pending.ID); run != nil {
			for _, name := range c.sessionNames(*run) {
				names[name] = struct{}{}
			}
		}
		for _, decision := range c.autopilot.DecisionsFor(pending.ID) {
			if decision.Session != nil {
				names[*decision.Session] = struct{}{}
			}
		}
		ordered := sortedNames(names)
		for _, name := range ordered {
			var err error
			c.Suspend(func() { err = c.runner.EnsureStopped(ctx, name) })
			if err != nil {
				return core.NewFailure("Could not stop session " + name + ": " + err.Error() + ". The run is still here; try erasing it again.")
			}
		}
		pending.Sessions = ordered
		pending.StoppedSessions = true
		if err := c.storage.SavePendingErasure(pending); err != nil {
			return err
		}
	}
	if !pending.RemovedWorkspace {
		folder := filepath.Join(c.paths.Work(), pending.Folder)
		var err error
		c.Suspend(func() { err = c.workspaces.Remove(ctx, pending.Repos, folder) })
		if err != nil {
			return err
		}
		pending.RemovedWorkspace = true
		if err := c.storage.SavePendingErasure(pending); err != nil {
			return err
		}
	}
	if !pending.RemovedDecisions {
		if err := c.autopilot.removeDecisions(pending.ID); err != nil {
			return core.NewFailure("Could not save AFK decisions: " + err.Error() + ". The run is still here; try erasing it again.")
		}
		pending.RemovedDecisions = true
		if err := c.storage.SavePendingErasure(pending); err != nil {
			return err
		}
	}
	if !pending.RemovedRun {
		if err := c.storage.DeleteRunFolder(pending.Folder); err != nil {
			return err
		}
		pending.RemovedRun = true
		if err := c.storage.SavePendingErasure(pending); err != nil {
			return err
		}
	}
	if err := c.storage.RecordErased(pending.ID, pending.Generation); err != nil {
		return err
	}
	if err := c.storage.DeletePendingErasure(pending.ID); err != nil {
		return err
	}
	c.book.removeErased(pending.ID)
	delete(c.erasureErrors, pending.ID)
	return nil
}
