// The agent half of the coordinator's library.

package coordinator

import (
	"slices"
	"strconv"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// Agent returns an agent version of the live library, read once.
// Lock: runs with c.mu held.
func (l *Library) Agent(ref *core.AgentRef) *core.Agent {
	if ref == nil {
		return nil
	}
	if cached, ok := l.agentCache[*ref]; ok {
		agent := cloneAgent(cached)
		return &agent
	}
	agent, ok := l.storage.AgentRef(*ref)
	if !ok {
		return nil
	}
	l.agentCache[*ref] = agent
	out := cloneAgent(agent)
	return &out
}

// AgentFor returns the agent version a run's stage uses: the run's own copy, or
// the library's for a run saved before runs kept one.
// Lock: runs with c.mu held.
func (l *Library) AgentFor(ref *core.AgentRef, run core.Run) *core.Agent {
	if ref == nil {
		return nil
	}
	var snapshot *core.AgentSnapshot
	for i := range run.Agents {
		if run.Agents[i].Ref() == *ref {
			snapshot = &run.Agents[i]
			break
		}
	}
	if snapshot == nil {
		return l.Agent(ref)
	}
	key := run.ID + "|" + ref.ID + "|" + strconv.Itoa(ref.Version)
	if cached, ok := l.snapshotCache[key]; ok {
		agent := cloneAgent(cached)
		return &agent
	}
	parsed := snapshot.Agent()
	if parsed == nil {
		return nil
	}
	l.snapshotCache[key] = *parsed
	out := cloneAgent(*parsed)
	return &out
}

// snapshots returns copies of the agent versions run's stages use, for it to keep.
// Lock: runs with c.mu held.
func (l *Library) snapshots(run core.Run) []core.AgentSnapshot {
	var refs []core.AgentRef
	for _, stage := range run.Stages {
		if stage.Agent != nil && !slices.Contains(refs, *stage.Agent) {
			refs = append(refs, *stage.Agent)
		}
	}
	out := make([]core.AgentSnapshot, 0, len(refs))
	for i := range refs {
		if agent := l.Agent(&refs[i]); agent != nil {
			out = append(out, core.NewAgentSnapshot(*agent))
		}
	}
	return out
}

// LatestAgent returns an agent's latest record, or nil.
// Lock: runs with c.mu held.
func (l *Library) LatestAgent(id string) *core.AgentRecord {
	index := l.agentIndex(id)
	if index < 0 {
		return nil
	}
	record := l.agents[index]
	record.Agent = cloneAgent(record.Agent)
	return &record
}

// AgentsOf returns the agent versions a run's stages use.
// Lock: runs with c.mu held.
func (l *Library) AgentsOf(run core.Run) map[core.AgentRef]core.Agent {
	result := map[core.AgentRef]core.Agent{}
	for _, stage := range run.Stages {
		if stage.Agent == nil {
			continue
		}
		if agent := l.AgentFor(stage.Agent, run); agent != nil {
			result[*stage.Agent] = *agent
		}
	}
	return result
}

// PlaceholderKeys lists a belt's stage keys in the order the editor draws them.
// A belt saved before stages had keys is converted from its pinned agents for
// this reading only.
// Lock: runs with c.mu held.
func (l *Library) PlaceholderKeys(belt core.Belt) []string {
	converted := core.StageKeysExplicit(belt, l.pinnedAgents(belt))
	entries := core.StageAdjustEntries(converted.Stages, func(stage core.Stage) *string {
		return nonEmptyStageKey(core.StageKeysChosen(stage))
	}, nil, false)
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return keys
}

// PlaceholderHelp returns the placeholder keys and descriptions the editor and
// the API share.
// Lock: runs with c.mu held.
func (l *Library) PlaceholderHelp(belt core.Belt, stage *string) core.PlaceholderHelp {
	converted := core.StageKeysExplicit(belt, l.pinnedAgents(belt))
	layout := converted.Layout()
	return core.PlaceholderHelpStages(converted.Stages, stage, l.Key,
		func(s core.Stage) string { return core.StageKeysPathOf(s, layout) }, false)
}

// pinnedAgents returns the pinned agent versions a belt's stages name.
// Lock: runs with c.mu held.
func (l *Library) pinnedAgents(belt core.Belt) map[core.AgentRef]core.Agent {
	pinned := map[core.AgentRef]core.Agent{}
	for _, stage := range belt.Stages {
		if stage.Kind() != core.StageKindAgent || stage.Agent == nil {
			continue
		}
		if agent := l.Agent(stage.Agent); agent != nil {
			pinned[*stage.Agent] = *agent
		}
	}
	return pinned
}

// Warnings returns a belt's save warnings, using the same resolved versions and
// names Save uses.
// Lock: runs with c.mu held.
func (l *Library) Warnings(belt core.Belt) []string {
	converted := core.StageKeysExplicit(belt, l.pinnedAgents(belt))
	pinned := l.pinnedAgents(converted)
	out := []string{}
	out = append(out, core.ValidationWarnings(converted, l.PlaceholderKeys(converted), pinned)...)
	out = append(out, core.ModelPolicyWarnings(converted.Stages, l.modelAdmission(), l.modelLabel)...)
	return out
}

// Key returns the placeholder key chosen on a stage's output, exactly as stored.
// Lock: runs with c.mu held.
func (l *Library) Key(stage core.Stage) *string {
	return nonEmptyStageKey(core.StageKeysChosen(stage))
}

// ConvertingLegacyKeys fills missing stage keys from each stage's pinned agent,
// for opening an old definition as an editable draft.
// Lock: runs with c.mu held.
func (l *Library) ConvertingLegacyKeys(belt core.Belt) core.Belt {
	return core.StageKeysExplicit(belt, l.pinnedAgents(belt))
}

// KeyIn returns the placeholder key of a run's stage: the stage's own key, or for
// an older run the agent snapshot's, then the library's.
// Lock: runs with c.mu held.
func (l *Library) KeyIn(stage core.Stage, run core.Run) *string {
	agents := map[core.AgentRef]core.Agent{}
	if stage.Agent != nil {
		if agent := l.AgentFor(stage.Agent, run); agent != nil {
			agents[*stage.Agent] = *agent
		}
	}
	name := core.StageKeysLegacyName(stage, agents)
	if run.UsesStageKeys() {
		name = core.StageKeysChosen(stage)
	}
	return nonEmptyStageKey(name)
}

// NewerAgent returns the latest version of a stage's agent, when it is newer
// than the one the stage uses.
// Lock: runs with c.mu held.
func (l *Library) NewerAgent(stage core.Stage) *core.Agent {
	if stage.Agent == nil {
		return nil
	}
	latest := l.LatestAgent(stage.Agent.ID)
	if latest == nil || latest.Agent.Version <= stage.Agent.Version {
		return nil
	}
	agent := cloneAgent(latest.Agent)
	return &agent
}

// UpdatingStage returns the belt with the stage at index moved to its agent's latest version.
// Lock: runs with c.mu held.
func (l *Library) UpdatingStage(index int, belt core.Belt) core.Belt {
	stage := belt.Stages[index]
	latest := l.NewerAgent(stage)
	if latest == nil {
		return belt
	}
	updated := belt.Clone()
	place := core.DefaultsPlace{
		Before: append([]core.Stage{}, belt.Stages[:index]...),
		Later:  append([]core.Stage{}, belt.Stages[index+1:]...),
		Forks:  belt.Forks, Route: stage.Route,
	}
	updated.Stages[index] = core.DefaultsUpdateAt(stage, l.Agent(stage.Agent), *latest, place, belt.StageKind())
	return updated
}

// UpdatingAllStages returns the belt with every stage moved to its agent's latest version.
// Lock: runs with c.mu held.
func (l *Library) UpdatingAllStages(belt core.Belt) core.Belt {
	result := belt
	for index := range belt.Stages {
		result = l.UpdatingStage(index, result)
	}
	return result
}

// BeltsUsing lists the belts whose latest version uses the agent id, with the version each uses.
// Lock: runs with c.mu held.
func (l *Library) BeltsUsing(id string) []BeltVersionUse {
	out := []BeltVersionUse{}
	l.eachBeltUsing(id, func(belt *core.Belt, version int) {
		out = append(out, BeltVersionUse{Belt: belt.Clone(), Version: version})
	})
	return out
}

// AgentUsages is who uses agent id, as the agent's presentation lists them: each belt
// with the highest version its stages pin, read in place without copying the belts.
// Lock: runs with c.mu held.
func (l *Library) AgentUsages(id string) []core.AgentUsage {
	out := []core.AgentUsage{}
	l.eachBeltUsing(id, func(belt *core.Belt, version int) {
		out = append(out, core.AgentUsage{BeltID: belt.ID, BeltName: belt.Name, Version: version})
	})
	return out
}

// eachBeltUsing calls use, in library order, with each belt one of whose stages pins agent
// id, and the highest version its stages pin. The belt is the library's: use reads it only.
// Lock: runs with c.mu held.
func (l *Library) eachBeltUsing(id string, use func(belt *core.Belt, version int)) {
	for i := range l.belts {
		belt := &l.belts[i].Belt
		found := false
		highest := 0
		for _, stage := range belt.Stages {
			if stage.Agent == nil || stage.Agent.ID != id {
				continue
			}
			if !found || stage.Agent.Version > highest {
				highest = stage.Agent.Version
			}
			found = true
		}
		if found {
			use(belt, highest)
		}
	}
}

// SavedVersionWarnings refuses a saved version whose id no longer matches an agent the library has.
// Lock: runs with c.mu held.
func (l *Library) SavedVersionWarnings(agent core.Agent) []string {
	if agent.Version > 0 && l.LatestAgent(agent.ID) == nil {
		return []string{"No agent with the id “" + agent.ID + "” is in the library, so this can't save a new version of it. Save it as a new agent."}
	}
	return nil
}

// SaveAgent saves an agent as a new version, unless nothing changed. A rejected
// save carries its problems in order and writes nothing.
// Lock: runs with c.mu held.
func (l *Library) SaveAgent(agent core.Agent) (core.Agent, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	others := l.OtherAgents(agent)
	versionWarnings := l.SavedVersionWarnings(agent)
	if len(versionWarnings) > 0 {
		return core.Agent{}, core.NewFailure(strings.Join(versionWarnings, " "), versionWarnings...)
	}
	warnings := core.ValidationAgentWarnings(agent, others)
	warnings = append(warnings, core.ModelPolicyWarningsForPool(agent.Pool(), agent.Name, l.modelAdmission(), l.modelLabel)...)
	if len(warnings) > 0 {
		return core.Agent{}, core.NewFailure(strings.Join(warnings, " "), warnings...)
	}
	if latest := l.LatestAgent(agent.ID); latest != nil && latest.Agent.SameContent(agent) {
		return cloneAgent(latest.Agent), nil
	}
	saved, err := l.storage.SaveNewAgentVersion(agent)
	if err != nil {
		return core.Agent{}, err
	}
	// The library keeps its own copy: a caller's later edit of saved (or of the agent it passed in) must not reach it.
	l.agentCache[saved.Ref()] = cloneAgent(saved)
	if index := l.agentIndex(saved.ID); index >= 0 {
		l.agents[index].Agent = cloneAgent(saved)
		l.revision++
	} else {
		l.agents = append(l.agents, core.AgentRecord{Agent: cloneAgent(saved)})
		l.revision++
	}
	return cloneAgent(saved), nil
}

// OtherAgents lists the agents a draft must not share an id or key with.
// Lock: runs with c.mu held.
func (l *Library) OtherAgents(agent core.Agent) []core.Agent {
	out := []core.Agent{}
	for _, record := range l.agents {
		if agent.Version == 0 || record.Agent.ID != agent.ID {
			out = append(out, cloneAgent(record.Agent))
		}
	}
	return out
}

// SetAgentArchived archives or restores an agent; its versions stay for the belts that use them.
// Lock: runs with c.mu held.
func (l *Library) SetAgentArchived(id string, archived bool) {
	_, _ = l.ArchivingAgent(id, archived)
}

// ArchivingAgent archives or restores an agent, propagating a storage failure
// and leaving the in-memory record alone when the write fails.
// Lock: runs with c.mu held.
func (l *Library) ArchivingAgent(id string, archived bool) (core.AgentRecord, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.agentIndex(id)
	if index < 0 {
		return core.AgentRecord{}, core.NewValidationFailure("No agent with id "+id+".", nil)
	}
	if err := l.storage.SetAgentArchived(id, archived); err != nil {
		return core.AgentRecord{}, err
	}
	l.agents[index].Archived = archived
	l.revision++
	record := l.agents[index]
	record.Agent = cloneAgent(record.Agent)
	return record, nil
}

// NewAgent returns a new agent that is not saved yet, starting from the agent template.
// Lock: runs with c.mu held.
func (l *Library) NewAgent() core.Agent {
	n := 1
	for {
		candidate := "agent-" + strconv.Itoa(n)
		if l.agentIndex(candidate) < 0 {
			break
		}
		n++
	}
	agent := core.AgentTemplate()
	agent.ID = "agent-" + strconv.Itoa(n)
	agent.Version = 0
	agent.Name = "New agent"
	agent.Key = "agent" + strconv.Itoa(n)
	return agent
}

// agentIndex is the index of the agent record with id, or -1.
func (l *Library) agentIndex(id string) int {
	for i := range l.agents {
		if l.agents[i].Agent.ID == id {
			return i
		}
	}
	return -1
}
