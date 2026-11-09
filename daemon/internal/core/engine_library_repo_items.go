// What a shared library repository holds, and where each of its belts and agents
// stands against this library.

package core

import (
	"fmt"
	"maps"
	"slices"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// LibraryItemState is where a repository's belt or agent stands against the library.
type LibraryItemState string

const (
	// LibraryItemAvailable is an item the library does not have.
	LibraryItemAvailable LibraryItemState = "available"
	// LibraryItemInstalled is the latest version, from this repository, already in the library.
	LibraryItemInstalled LibraryItemState = "installed"
	// LibraryItemUpdate is an item from this repository with a newer version in it.
	LibraryItemUpdate LibraryItemState = "update"
	// LibraryItemConflict is an item the library cannot take without losing a change.
	LibraryItemConflict LibraryItemState = "conflict"
)

// LibraryItem is one belt or agent a shared repository holds.
type LibraryItem struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	// LocalID is the id of this library's copy, which differs from ID when ID was taken here; nil without one.
	LocalID      *string          `json:"localId"`
	LocalVersion *int             `json:"localVersion"`
	State        LibraryItemState `json:"state"`
	Reason       *string          `json:"reason"`
	Actions      []LibraryAction  `json:"actions"`
}

// LibraryRepoItems says what a repository holds and where each item stands here.
// repoFiles are the repository's library files under its path, localFiles this
// data folder's belts and agents, and origin the repository's own. The items come
// out sorted, belts first then agents, by id. problems are what is wrong with the
// repository's files; a file that does not decode is left out of the items.
func LibraryRepoItems(origin LibraryOrigin, repoFiles, localFiles map[string][]byte) ([]LibraryItem, []string) {
	repoBelts, repoAgents, problems := libraryCheckRead(repoFiles)
	localBelts, localAgents, _ := libraryCheckRead(localFiles)
	mapping := libraryMapIDs(origin, repoBelts, repoAgents, localBelts, localAgents)
	var items []LibraryItem
	for _, id := range slices.Sorted(maps.Keys(repoBelts)) {
		latest := libraryNewest(repoBelts[id])
		belt := repoBelts[id][latest]
		state, reason := libraryBeltStanding(origin, belt, latest, localBelts, localAgents, repoAgents, repoBelts[id], mapping)
		items = append(items, libraryItem("belt", belt.ID, mapping.belts.of(id), belt.Name, latest, localBelts[mapping.belts.of(id)], len(localBelts[id]) > 0, state, reason))
	}
	for _, id := range slices.Sorted(maps.Keys(repoAgents)) {
		latest := libraryNewest(repoAgents[id])
		agent := repoAgents[id][latest]
		state, reason := libraryAgentStanding(origin, agent, latest, localAgents[mapping.agents.of(id)], repoAgents[id], mapping)
		items = append(items, libraryItem("agent", agent.ID, mapping.agents.of(id), agent.Name, latest, localAgents[mapping.agents.of(id)], len(localAgents[id]) > 0, state, reason))
	}
	return items, problems
}

// libraryBeltStanding is where a belt stands. The agent versions it pins must not
// have changed here, and then its own versions decide. repoVersions are the
// repository's versions of the belt, by number.
func libraryBeltStanding(origin LibraryOrigin, belt Belt, latest int, localBelts libraryBelts, localAgents libraryAgents, repoAgents libraryAgents, repoVersions map[int]Belt, mapping libraryIDMapping) (LibraryItemState, string) {
	for _, stage := range belt.Stages {
		if stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		localID := mapping.agents.of(ref.ID)
		repoAgent, inRepo := repoAgents[ref.ID][ref.Version]
		localAgent, inLibrary := localAgents[localID][ref.Version]
		if inRepo && inLibrary && libraryAgentText(libraryAgentInRepo(localAgent, origin, mapping.agentsToRepo)) != libraryAgentText(repoAgent) {
			return LibraryItemConflict, libraryChangedReason(localID, ref.Version)
		}
	}
	versions := localBelts[mapping.belts.of(belt.ID)]
	if len(versions) == 0 {
		return LibraryItemAvailable, ""
	}
	newest := libraryNewest(versions)
	localBelt := versions[newest]
	same := func(local, repo Belt) bool {
		return libraryBeltShared(libraryBeltInRepo(local, origin, mapping.agentsToRepo)) == libraryBeltShared(repo)
	}
	switch {
	case newest < latest && libraryLocalOnly(origin, localBelt.Origin, localBelt, newest, repoVersions, same):
		return LibraryItemConflict, libraryChangedReason(localBelt.ID, newest)
	case newest < latest:
		return LibraryItemUpdate, ""
	case newest == latest && libraryBeltShared(libraryBeltInRepo(localBelt, origin, mapping.agentsToRepo)) != libraryBeltShared(belt):
		return LibraryItemConflict, libraryChangedReason(localBelt.ID, latest)
	}
	return LibraryItemInstalled, ""
}

// libraryAgentStanding is where an agent stands, from this library's versions of it and the repository's.
func libraryAgentStanding(origin LibraryOrigin, agent Agent, latest int, versions, repoVersions map[int]Agent, mapping libraryIDMapping) (LibraryItemState, string) {
	if len(versions) == 0 {
		return LibraryItemAvailable, ""
	}
	newest := libraryNewest(versions)
	localAgent := versions[newest]
	same := func(local, repo Agent) bool {
		return libraryAgentText(libraryAgentInRepo(local, origin, mapping.agentsToRepo)) == libraryAgentText(repo)
	}
	switch {
	case newest < latest && libraryLocalOnly(origin, localAgent.Origin, localAgent, newest, repoVersions, same):
		return LibraryItemConflict, libraryChangedReason(localAgent.ID, newest)
	case newest < latest:
		return LibraryItemUpdate, ""
	case newest == latest && libraryAgentText(libraryAgentInRepo(localAgent, origin, mapping.agentsToRepo)) != libraryAgentText(agent):
		return LibraryItemConflict, libraryChangedReason(localAgent.ID, latest)
	}
	return LibraryItemInstalled, ""
}

// libraryItem is one item with its state, the local version it has and its install action. idInUse says
// the library holds something under id; without that, a different localID means the id is reserved.
func libraryItem[T any](kind, id, localID, name string, version int, local map[int]T, idInUse bool, state LibraryItemState, reason string) LibraryItem {
	item := LibraryItem{Kind: kind, ID: id, Name: name, Version: version, State: state}
	if len(local) > 0 {
		newest := libraryNewest(local)
		item.LocalID, item.LocalVersion = &localID, &newest
	}
	if reason == "" && localID != id {
		reason = fmt.Sprintf("Your library uses the id %s for something else, so this is kept as a separate %s, %s.", id, kind, localID)
		if !idInUse {
			reason = fmt.Sprintf("An agent you installed from another repository goes to the id %s, so this is kept as a separate %s, %s.", id, kind, localID)
		}
	}
	action := libraryAction("install", true, "")
	switch state {
	case LibraryItemInstalled:
		action = libraryAction("install", false, fmt.Sprintf("%s v%d is already in your library.", id, version))
	case LibraryItemConflict:
		action = libraryAction("install", false, reason)
	}
	if reason != "" {
		item.Reason = &reason
	}
	item.Actions = []LibraryAction{action}
	return item
}

// libraryChangedReason says a local version was changed, so the repository's can't replace it.
func libraryChangedReason(id string, version int) string {
	return fmt.Sprintf("You changed %s v%d locally, so it can't be updated from this repository.", id, version)
}

// libraryNewest is the highest version number in versions.
func libraryNewest[T any](versions map[int]T) int {
	newest := 0
	for v := range versions {
		newest = max(newest, v)
	}
	return newest
}

// libraryBeltText is a belt as its file reads, with its origin left out, so two
// belts with the same text have the same content.
func libraryBeltText(belt Belt) string {
	belt.Origin, belt.OriginID = nil, ""
	data, _ := jsonx.MarshalStorage(belt)
	return string(data)
}

// libraryBeltShared is a belt as a share writes it: without its origin, and without the repositories,
// base branches and AFK mode that belong to one library (LibraryShareFiles strips the same).
func libraryBeltShared(belt Belt) string {
	belt.Origin = nil
	belt.Base = ""
	belt.RepositoryBases = nil
	belt.Afk = AfkModeGlobal
	belt.Triggers = slices.Clone(belt.Triggers)
	for index := range belt.Triggers {
		belt.Triggers[index].Repos = nil
	}
	return libraryBeltText(belt)
}

// libraryAgentText is an agent as its file reads, with its origin left out.
func libraryAgentText(agent Agent) string {
	agent.Origin, agent.OriginID = nil, ""
	return AgentFileWrite(agent)
}
