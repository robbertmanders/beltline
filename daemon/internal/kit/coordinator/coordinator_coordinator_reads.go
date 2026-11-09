// The coordinator's public reads: the run book's runs and notices, the belts,
// agents, waiting subjects, pause and the trigger check state. Each takes c.mu
// for callers outside a turn.

package coordinator

import (
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// Runs is the run book's runs.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Runs() []core.Run {
	c.Lock()
	defer c.EndTurn()
	runs := make([]core.Run, 0, len(c.book.runs))
	for _, run := range c.book.runs {
		runs = append(runs, run.Clone())
	}
	return runs
}

// Notices is the run book's notices.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Notices() []Notice {
	c.Lock()
	defer c.EndTurn()
	return append([]Notice(nil), c.book.notices...)
}

// Belts is the saved belts with their records.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Belts() []store.BeltRecord {
	c.Lock()
	defer c.EndTurn()
	out := make([]store.BeltRecord, 0, len(c.library.belts))
	for _, record := range c.library.belts {
		out = append(out, cloneBeltRecord(record))
	}
	return out
}

// Agents is the agent library's records.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Agents() []core.AgentRecord {
	c.Lock()
	defer c.EndTurn()
	out := make([]core.AgentRecord, 0, len(c.library.agents))
	for _, record := range c.library.agents {
		record.Agent = cloneAgent(record.Agent)
		out = append(out, record)
	}
	return out
}

// Checking reports whether a trigger check is running.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Checking() bool {
	c.Lock()
	defer c.EndTurn()
	return c.intake.isChecking()
}

// LastCheck is when the last trigger check finished.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) LastCheck() *time.Time {
	c.Lock()
	defer c.EndTurn()
	if c.intake.lastCheck == nil {
		return nil
	}
	last := *c.intake.lastCheck
	return &last
}

// Pause is the global pause as the views and clocks read it.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Pause() core.Pause {
	c.Lock()
	defer c.EndTurn()
	return c.currentPause()
}

// Run returns the run with id, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Run(id string) *core.Run {
	c.Lock()
	defer c.EndTurn()
	return c.book.Run(id)
}

// Belt returns the belt record with id, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Belt(id string) *store.BeltRecord {
	c.Lock()
	defer c.EndTurn()
	return c.library.Belt(id)
}
