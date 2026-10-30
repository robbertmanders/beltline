package store

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// agentState is what agents/<id>/state.json holds.
type agentState struct {
	Archived bool `json:"archived"`
}

// agentFolder is agents/<id>/.
func (s Storage) agentFolder(id string) string {
	return filepath.Join(s.Paths.Agents(), id)
}

// LoadAgents is every agent's latest version, by name, then by id.
// The library is one folder per agent, each version a Markdown file you can
// also edit by hand, and state.json saying whether it is archived.
func (s Storage) LoadAgents() []core.AgentRecord {
	records := []core.AgentRecord{}
	for _, folder := range subfolders(s.Paths.Agents()) {
		id := filepath.Base(folder)
		all := versions(folder, "md")
		if len(all) == 0 {
			continue
		}
		agent, ok := s.Agent(id, slices.Max(all))
		if !ok {
			continue
		}
		var state agentState
		archived := s.Read(filepath.Join(folder, "state.json"), &state) == nil && state.Archived
		records = append(records, core.AgentRecord{Agent: agent, Archived: archived})
	}
	slices.SortStableFunc(records, func(a, b core.AgentRecord) int {
		if order := core.LocalizedStandardCompare(a.Agent.Name, b.Agent.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Agent.ID, b.Agent.ID)
	})
	return records
}

// Agent is one agent version as saved, and whether it exists.
func (s Storage) Agent(id string, version int) (core.Agent, bool) {
	path := filepath.Join(s.agentFolder(id), "v"+strconv.Itoa(version)+".md")
	if data, err := os.ReadFile(path); err == nil && utf8.Valid(data) {
		agent, err := core.AgentFileParse(string(data), id, version)
		if err != nil {
			return core.Agent{}, false
		}
		return agent, true
	}
	return core.Agent{}, false
}

// AgentRef is the agent version a reference names.
func (s Storage) AgentRef(ref core.AgentRef) (core.Agent, bool) {
	return s.Agent(ref.ID, ref.Version)
}

// AgentVersions is the versions an agent has on disk, oldest first.
func (s Storage) AgentVersions(id string) []int {
	all := versions(s.agentFolder(id), "md")
	sort.Ints(all)
	return all
}

// SaveNewAgentVersion saves agent as its next version and returns it with that
// version number. This write does not consult the write fault.
func (s Storage) SaveNewAgentVersion(agent core.Agent) (core.Agent, error) {
	folder := s.agentFolder(agent.ID)
	if err := os.MkdirAll(folder, 0o777); err != nil {
		return core.Agent{}, err
	}
	saved := agent
	saved.Version = nextVersion(versions(folder, "md"))
	path := filepath.Join(folder, "v"+strconv.Itoa(saved.Version)+".md")
	if err := WriteFileAtomic(path, []byte(core.AgentFileWrite(saved))); err != nil {
		return core.Agent{}, err
	}
	return saved, nil
}

// SetAgentArchived records whether an agent is archived.
func (s Storage) SetAgentArchived(id string, archived bool) error {
	return s.Write(agentState{Archived: archived}, filepath.Join(s.agentFolder(id), "state.json"))
}

// SeedAgents adds each sample agent (core.SampleAgents) at its sample version
// when the library lacks it or has only older versions, for demo mode and the
// tests; a live data folder is never seeded. Saved versions and versions at or
// above the sample's stay untouched. Errors are ignored.
func (s Storage) SeedAgents() {
	for _, id := range core.SampleAgentIDs {
		shipped := core.SampleAgentVersion(id)
		highest := 0
		if all := s.AgentVersions(id); len(all) > 0 {
			highest = all[len(all)-1]
		}
		if highest >= shipped {
			continue
		}
		text, ok := core.SampleAgentText(id)
		if !ok {
			continue
		}
		folder := s.agentFolder(id)
		if os.MkdirAll(folder, 0o777) != nil {
			continue
		}
		_ = WriteFileAtomic(filepath.Join(folder, "v"+strconv.Itoa(shipped)+".md"), []byte(text))
	}
}
