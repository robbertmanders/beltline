// The Coordinator's library commands: the agent methods and belt archiving. Each
// takes c.mu and forwards to the Library, which keeps the rules (coordinator_library*.go).

package coordinator

import "github.com/robbertmanders/beltline/daemon/internal/core"

// Agent returns an agent version of the live library, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Agent(ref *core.AgentRef) *core.Agent {
	c.Lock()
	defer c.EndTurn()
	return c.library.Agent(ref)
}

// LatestAgent returns an agent's latest record, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) LatestAgent(id string) *core.AgentRecord {
	c.Lock()
	defer c.EndTurn()
	return c.library.LatestAgent(id)
}

// AgentsOf returns the agent versions a run's stages use.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) AgentsOf(run core.Run) map[core.AgentRef]core.Agent {
	c.Lock()
	defer c.EndTurn()
	return c.library.AgentsOf(run)
}

// AgentFor returns the agent version a run's stage uses, from the factory the run started in.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) AgentFor(ref *core.AgentRef, run core.Run) *core.Agent {
	c.Lock()
	defer c.EndTurn()
	return c.library.AgentFor(ref, run)
}

// KeyIn returns a run's stage placeholder key.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) KeyIn(stage core.Stage, run core.Run) *string {
	c.Lock()
	defer c.EndTurn()
	return c.library.KeyIn(stage, run)
}

// Key returns a stage's placeholder key as stored.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Key(stage core.Stage) *string {
	c.Lock()
	defer c.EndTurn()
	return c.library.Key(stage)
}

// ConvertingLegacyKeys fills missing stage keys from each stage's pinned agent.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ConvertingLegacyKeys(belt core.Belt) core.Belt {
	c.Lock()
	defer c.EndTurn()
	return c.library.ConvertingLegacyKeys(belt)
}

// PlaceholderKeys lists a belt's stage keys.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) PlaceholderKeys(belt core.Belt) []string {
	c.Lock()
	defer c.EndTurn()
	return c.library.PlaceholderKeys(belt)
}

// Warnings returns the belt's save warnings.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Warnings(belt core.Belt) []string {
	c.Lock()
	defer c.EndTurn()
	return c.library.Warnings(belt)
}

// NewerAgent returns the latest version of a stage's agent when it is newer than the one used.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) NewerAgent(stage core.Stage) *core.Agent {
	c.Lock()
	defer c.EndTurn()
	return c.library.NewerAgent(stage)
}

// UpdatingStage returns the belt with the stage at index moved to its agent's latest version.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) UpdatingStage(index int, belt core.Belt) core.Belt {
	c.Lock()
	defer c.EndTurn()
	return c.library.UpdatingStage(index, belt)
}

// UpdatingAllStages returns the belt with every stage moved to its agent's latest version.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) UpdatingAllStages(belt core.Belt) core.Belt {
	c.Lock()
	defer c.EndTurn()
	return c.library.UpdatingAllStages(belt)
}

// BeltVersionUse is one belt whose latest version uses an agent, with the
// version it uses.
type BeltVersionUse struct {
	Belt    core.Belt
	Version int
}

// BeltsUsing lists the belts whose latest version uses the agent id.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) BeltsUsing(id string) []BeltVersionUse {
	c.Lock()
	defer c.EndTurn()
	return c.library.BeltsUsing(id)
}

// SaveAgent saves an agent as a new version, unless nothing changed.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SaveAgent(agent core.Agent) (core.Agent, error) {
	c.Lock()
	defer c.EndTurn()
	return c.library.SaveAgent(agent)
}

// OtherAgents lists the agents a draft must not share an id or key with.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) OtherAgents(agent core.Agent) []core.Agent {
	c.Lock()
	defer c.EndTurn()
	return c.library.OtherAgents(agent)
}

// SetBeltArchived archives or restores a belt, and kicks on restore.
// Lock: exported entry point; takes c.mu. Spawns: kick.
func (c *Coordinator) SetBeltArchived(id string, archived bool) {
	c.Lock()
	defer c.EndTurn()
	c.library.SetBeltArchived(id, archived)
	if !archived {
		c.kick()
	}
}

// SetAgentArchived archives or restores an agent.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SetAgentArchived(id string, archived bool) {
	c.Lock()
	defer c.EndTurn()
	c.library.SetAgentArchived(id, archived)
}

// saveBelt saves a belt as a new version and kicks.
// Lock: runs with c.mu held. Spawns: kick.
func (c *Coordinator) saveBelt(belt core.Belt) (core.Belt, error) {
	saved, err := c.library.SaveBelt(belt)
	if err != nil {
		return core.Belt{}, err
	}
	c.intake.forgetPolls(saved.ID)
	c.kick()
	return saved, nil
}
