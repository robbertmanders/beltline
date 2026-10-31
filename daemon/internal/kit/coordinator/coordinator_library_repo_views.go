// The Library page's Belts and Agents tabs and the read-only editors a card opens:
// read from the shared repositories' clones on disk, with no network.

package coordinator

import (
	"path/filepath"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// LibraryBeltDetail is a shared belt as the read-only belt editor shows it: where it stands
// here, its newest version, and the editor's context for it, with the agents it pins read
// from the repository.
type LibraryBeltDetail struct {
	Item    core.LibraryItem `json:"item"`
	Belt    core.Belt        `json:"belt"`
	Context BeltContext      `json:"context"`
}

// LibraryAgentDetail is a shared agent as the read-only agent editor shows it.
type LibraryAgentDetail struct {
	Item         core.LibraryItem       `json:"item"`
	Agent        core.Agent             `json:"agent"`
	Presentation core.AgentPresentation `json:"presentation"`
}

// LibraryBrowser is the Library page's tab of kind ("belt" or "agent") over every shared
// repository. Lock: exported entry point; takes c.mu.
func (c *Coordinator) LibraryBrowser(kind, search, filter string) core.LibraryRepoBrowser {
	c.Lock()
	defer c.EndTurn()
	local := store.LibraryFilesRead(c.paths)
	var repos []core.LibraryRepoContents
	for _, repo := range c.settings.LibraryRepositories {
		if repo.Role == core.LibraryRoleShared {
			repos = append(repos, c.libraryRepoContents(repo.Origin(), local))
		}
	}
	return core.LibraryRepoBrowserOf(repos, kind, search, filter)
}

// LibraryBelt is a shared repository's belt id with the editor's context, and the stage form's
// for stage when it is set. found is false when the repository holds no such belt.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) LibraryBelt(origin core.LibraryOrigin, id string, stage *string) (detail LibraryBeltDetail, found bool, err error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleShared); err != nil {
		return LibraryBeltDetail{}, false, err
	}
	repo := c.libraryRepoContents(origin, store.LibraryFilesRead(c.paths))
	item, ok := repo.Item("belt", id)
	belt, held := repo.Belt(id)
	if !ok || !held {
		return LibraryBeltDetail{}, false, nil
	}
	return LibraryBeltDetail{Item: item, Belt: belt, Context: c.libraryBeltContext(repo, belt, stage)}, true, nil
}

// LibraryAgent is a shared repository's agent id with the editor's presentation for it. found is
// false when the repository holds no such agent. Lock: exported entry point; takes c.mu.
func (c *Coordinator) LibraryAgent(origin core.LibraryOrigin, id string) (detail LibraryAgentDetail, found bool, err error) {
	c.Lock()
	defer c.EndTurn()
	if _, err := c.configuredRepo(origin, core.LibraryRoleShared); err != nil {
		return LibraryAgentDetail{}, false, err
	}
	repo := c.libraryRepoContents(origin, store.LibraryFilesRead(c.paths))
	item, ok := repo.Item("agent", id)
	agent, held := repo.LatestAgent(id)
	if !ok || !held {
		return LibraryAgentDetail{}, false, nil
	}
	return LibraryAgentDetail{Item: item, Agent: agent, Presentation: core.LibraryRepoAgentPresentation(repo, agent)}, true, nil
}

// libraryRepoContents reads a shared repository's clone against the library's files local.
// Lock: runs with c.mu held.
func (c *Coordinator) libraryRepoContents(origin core.LibraryOrigin, local map[string][]byte) core.LibraryRepoContents {
	files := store.LibraryFilesRead(store.Paths{Root: filepath.Join(c.libraryRepos.Dir(origin.Repository), origin.Path)})
	contents, _ := core.LibraryRepoContentsRead(origin, files, local)
	return contents
}

// libraryBeltContext is the editor's context for a shared belt, read-only: no actions, warnings or
// notes, a stage line that offers no agents and no updates, and the pinned agent read from the
// repository, or else from this library. Lock: runs with c.mu held.
func (c *Coordinator) libraryBeltContext(repo core.LibraryRepoContents, belt core.Belt, stage *string) BeltContext {
	line := core.StageLineOf(belt, nil)
	form := core.TriggerFormPresentationMake(belt)
	now := c.clock.Now()
	context := BeltContext{
		Actions:       []core.BeltAction{},
		Warnings:      []string{},
		Routes:        core.BeltContextPresentationRoutes(belt),
		Stages:        core.BeltContextPresentationStages(belt),
		Outcomes:      core.BeltContextPresentationOutcomes(belt),
		Schedules:     core.BeltContextPresentationSchedules(belt, &now, nil),
		Policies:      core.BeltContextPresentationPolicies(belt),
		PolicySection: core.BeltContextPresentationPolicySectionOf(belt),
		TriggerForm:   &form,
		Line:          &line,
		TaskEntries:   []core.TaskDraftEntry{},
	}
	if stage == nil {
		return context
	}
	context.Stage = core.StageMenuMake(belt, *stage)
	if pinned := beltStageNamed(belt, *stage); pinned != nil && pinned.Kind() == core.StageKindAgent && pinned.Agent != nil {
		ref := *pinned.Agent
		agent := repo.Agent(&ref)
		if agent == nil {
			agent = c.library.Agent(&ref)
		}
		context.Agent = &PinnedAgent{ID: ref.ID, Version: ref.Version, Name: ref.ID, Missing: agent == nil}
		if agent != nil {
			context.Agent.Name = agent.Name
			context.Agent.Brief = agent.Brief
			if key := core.StageKeysLegacyName(*pinned, map[core.AgentRef]core.Agent{ref: *agent}); key != "" {
				context.Agent.Key = &key
			}
		}
	}
	return context
}
