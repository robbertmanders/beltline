// How a shared repository's ids and this library's ids relate. An id belongs to the library or
// repository that holds it: an item installed under an id this library already uses for something
// else gets a local id of its own, and remembers the repository's id as OriginID.

package core

import (
	"crypto/sha256"
	"encoding/base32"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// libraryIDs maps ids from one side to the other. An id it does not name is the same on both.
type libraryIDs map[string]string

// of is id on the other side.
func (ids libraryIDs) of(id string) string {
	if other, ok := ids[id]; ok {
		return other
	}
	return id
}

// libraryIDMapping is how one repository's ids read in this library.
type libraryIDMapping struct {
	belts        libraryIDs
	agents       libraryIDs
	agentsToRepo libraryIDs
}

// libraryMapIDs relates a repository's belts and agents to this library's. An item installed from
// the repository keeps its local id, an id free here is used as it is, and any other gets a new one.
func libraryMapIDs(origin LibraryOrigin, repoBelts libraryBelts, repoAgents libraryAgents, localBelts libraryBelts, localAgents libraryAgents) libraryIDMapping {
	toRepo := libraryIDs{}
	for id, versions := range localAgents {
		newest := versions[libraryNewest(versions)]
		if newest.OriginID != "" && newest.Origin != nil && *newest.Origin == origin {
			toRepo[id] = newest.OriginID
		}
	}
	agents := libraryMapKind(origin, repoAgents, localAgents,
		func(agent Agent) (*LibraryOrigin, string) { return agent.Origin, agent.OriginID },
		libraryOtherOriginTargets(origin, localAgents),
		func(local, repo Agent) bool {
			return libraryAgentText(libraryAgentInRepo(local, origin, toRepo)) == libraryAgentText(repo)
		})
	// An agent can go to a repository agent that isn't installed yet, under the id that agent will have here.
	for repoID, localID := range agents {
		if localID != repoID {
			toRepo[localID] = repoID
		}
	}
	belts := libraryMapKind(origin, repoBelts, localBelts,
		func(belt Belt) (*LibraryOrigin, string) { return belt.Origin, belt.OriginID },
		nil,
		func(local, repo Belt) bool {
			return libraryBeltShared(libraryBeltInRepo(local, origin, toRepo)) == libraryBeltShared(repo)
		})
	return libraryIDMapping{belts: belts, agents: agents, agentsToRepo: toRepo}
}

// libraryOtherOriginTargets are the agent ids that agents installed from other repositories go to. Such
// an id means that repository's agent, so this repository's agent must not take it, even while it is free.
func libraryOtherOriginTargets(origin LibraryOrigin, localAgents libraryAgents) map[string]struct{} {
	targets := map[string]struct{}{}
	for _, versions := range localAgents {
		agent := versions[libraryNewest(versions)]
		if agent.Origin == nil || *agent.Origin == origin {
			continue
		}
		for _, outcome := range agent.Outcomes {
			for _, target := range outcome.To {
				if !libraryTargetIsStage(target) {
					targets[target] = struct{}{}
				}
			}
		}
	}
	return targets
}

// libraryMapKind maps one kind's repository ids to local ones. An item with no origin is the
// repository's when same says one of its versions is the repository's version of that number, which
// counts an item shared from here or installed before origins were kept. reserved ids are not given out.
func libraryMapKind[T any](origin LibraryOrigin, repo, local map[string]map[int]T, provenance func(T) (*LibraryOrigin, string), reserved map[string]struct{}, same func(local, repo T) bool) libraryIDs {
	ids := libraryIDs{}
	used := map[string]struct{}{}
	repoIDs := slices.Sorted(maps.Keys(repo))
	for _, localID := range slices.Sorted(maps.Keys(local)) {
		from, originID := provenance(local[localID][libraryNewest(local[localID])])
		if originID == "" {
			originID = localID
		}
		if _, known := repo[originID]; !known || from == nil || *from != origin {
			continue
		}
		if _, taken := ids[originID]; !taken {
			ids[originID] = localID
			used[localID] = struct{}{}
		}
	}
	for _, id := range repoIDs {
		versions := local[id]
		if _, mapped := ids[id]; mapped || len(versions) == 0 {
			continue
		}
		if from, _ := provenance(versions[libraryNewest(versions)]); from != nil {
			continue
		}
		if _, taken := used[id]; !taken && librarySharesAVersion(versions, repo[id], same) {
			ids[id] = id
			used[id] = struct{}{}
		}
	}
	for _, id := range repoIDs {
		if _, mapped := ids[id]; mapped {
			continue
		}
		taken := func(candidate string) bool {
			_, assigned := used[candidate]
			_, kept := reserved[candidate]
			return assigned || kept || len(local[candidate]) > 0
		}
		localID := id
		if taken(id) {
			localID = librarySuffixedID(id, origin.Repository+"/"+origin.Path+"/"+id, taken)
		}
		ids[id] = localID
		used[localID] = struct{}{}
	}
	return ids
}

// libraryInheritOrigins gives each version that doesn't say where it came from what the version before it
// says, so one newer file written without them doesn't cut a copy off from its repository.
func libraryInheritOrigins[T any](items map[string]map[int]T, read func(T) (*LibraryOrigin, string), write func(*T, *LibraryOrigin, string)) {
	for _, versions := range items {
		var from *LibraryOrigin
		var originID string
		for _, version := range slices.Sorted(maps.Keys(versions)) {
			item := versions[version]
			own, ownID := read(item)
			sameOrigin := own != nil && from != nil && *own == *from
			if ownID == "" && (own == nil || sameOrigin) {
				write(&item, from, originID)
				versions[version] = item
				continue
			}
			from, originID = own, ownID
		}
	}
}

// libraryLocalOnly says a version from has no claim to be the repository's: it didn't come from origin,
// and the repository's version of that number is missing or differs.
func libraryLocalOnly[T any](origin LibraryOrigin, from *LibraryOrigin, local T, version int, repo map[int]T, same func(local, repo T) bool) bool {
	if from != nil && *from == origin {
		return false
	}
	held, ok := repo[version]
	return !ok || !same(local, held)
}

// librarySharesAVersion says whether some version local has is the same as repo's of that number.
func librarySharesAVersion[T any](local, repo map[int]T, same func(local, repo T) bool) bool {
	for version, ours := range local {
		if held, ok := repo[version]; ok && same(ours, held) {
			return true
		}
	}
	return false
}

// LibraryRepoID is the id origin's repository has for a local item with id, installed from from under originID.
func LibraryRepoID(origin LibraryOrigin, id string, from *LibraryOrigin, originID string) string {
	if from != nil && *from == origin && originID != "" {
		return originID
	}
	return id
}

// LibraryLocalPaths maps each file path in origin's repository to its path in this library.
func LibraryLocalPaths(origin LibraryOrigin, paths []string, repoFiles, localFiles map[string][]byte) map[string]string {
	repoBelts, repoAgents, _ := libraryCheckRead(repoFiles)
	localBelts, localAgents, _ := libraryCheckRead(localFiles)
	mapping := libraryMapIDs(origin, repoBelts, repoAgents, localBelts, localAgents)
	local := make(map[string]string, len(paths))
	for _, path := range paths {
		local[path] = path
		parts := strings.Split(path, "/")
		if len(parts) != 3 {
			continue
		}
		ids := mapping.belts
		if parts[0] == "agents" {
			ids = mapping.agents
		}
		parts[1] = ids.of(parts[1])
		local[path] = strings.Join(parts, "/")
	}
	return local
}

// libraryAgentInRepo is a local agent as origin's repository holds it.
func libraryAgentInRepo(agent Agent, origin LibraryOrigin, agentsToRepo libraryIDs) Agent {
	agent.ID = LibraryRepoID(origin, agent.ID, agent.Origin, agent.OriginID)
	agent.OriginID = ""
	agent.Outcomes = libraryMapOutcomes(agent.Outcomes, agentsToRepo)
	return agent
}

// libraryAgentInLibrary is a repository's agent as this library keeps it under localID.
func libraryAgentInLibrary(agent Agent, origin LibraryOrigin, localID string, agents libraryIDs) Agent {
	agent.OriginID = ""
	if localID != agent.ID {
		agent.OriginID = agent.ID
	}
	agent.ID, agent.Origin = localID, &origin
	agent.Outcomes = libraryMapOutcomes(agent.Outcomes, agents)
	return agent
}

// libraryBeltInRepo is a local belt as origin's repository holds it.
func libraryBeltInRepo(belt Belt, origin LibraryOrigin, agentsToRepo libraryIDs) Belt {
	belt.ID = LibraryRepoID(origin, belt.ID, belt.Origin, belt.OriginID)
	belt.OriginID = ""
	belt.Stages = libraryMapPins(belt.Stages, agentsToRepo)
	return belt
}

// libraryMapOutcomes is outcomes with the agents they go to mapped by ids.
func libraryMapOutcomes(outcomes []AgentOutcome, ids libraryIDs) []AgentOutcome {
	mapped := slices.Clone(outcomes)
	for index := range mapped {
		to := slices.Clone(mapped[index].To)
		for at := range to {
			if !libraryTargetIsStage(to[at]) {
				to[at] = ids.of(to[at])
			}
		}
		mapped[index].To = to
	}
	return mapped
}

// libraryTargetIsStage says an outcome's target names the watch stage by its type, not an agent by its id.
func libraryTargetIsStage(target string) bool {
	return target == string(StageTypeWatch)
}

// libraryMapPins is stages with the agents they pin mapped by ids.
func libraryMapPins(stages []Stage, ids libraryIDs) []Stage {
	mapped := slices.Clone(stages)
	for index := range mapped {
		if ref := mapped[index].Agent; ref != nil {
			mapped[index].Agent = &AgentRef{ID: ids.of(ref.ID), Version: ref.Version}
		}
	}
	return mapped
}

// librarySuffixedID is base with a short suffix drawn from seed, the first such id not taken.
func librarySuffixedID(base, seed string, taken func(string) bool) string {
	for n := 0; ; n++ {
		sum := sha256.Sum256([]byte(seed + "#" + strconv.Itoa(n)))
		suffix := strings.ToLower(base32.StdEncoding.EncodeToString(sum[:3]))[:4]
		if candidate := base + "-" + suffix; !taken(candidate) {
			return candidate
		}
	}
}

// libraryDraftID is what a new, unsaved belt or agent is called until its first save.
var libraryDraftID = regexp.MustCompile(`^(belt|agent)-[0-9]+$`)

// libraryIDNameLimit is how much of a name goes into an id.
const libraryIDNameLimit = 32

// LibrarySavedID is the id a first save gives an item: its name made fit for a folder, with a suffix
// from seed so another library's item of that name has another id. Saved items and ids that are not
// the draft's own are kept, as they are already chosen.
func LibrarySavedID(id, name string, version int, seed string, taken func(string) bool) string {
	match := libraryDraftID.FindStringSubmatch(id)
	if version != 0 || match == nil {
		return id
	}
	base := libraryIDFromName(name)
	if base == "" {
		base = match[1]
	}
	return librarySuffixedID(base, seed, taken)
}

// libraryIDFromName is name in lowercase letters, digits and single hyphens.
func libraryIDFromName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() == libraryIDNameLimit {
			break
		}
	}
	return strings.TrimRight(b.String(), "-")
}
