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
		func(local, repo Agent) bool {
			return libraryAgentText(libraryAgentInRepo(local, origin, toRepo)) == libraryAgentText(repo)
		})
	belts := libraryMapKind(origin, repoBelts, localBelts,
		func(belt Belt) (*LibraryOrigin, string) { return belt.Origin, belt.OriginID },
		func(local, repo Belt) bool {
			return libraryBeltShared(libraryBeltInRepo(local, origin, toRepo)) == libraryBeltShared(repo)
		})
	return libraryIDMapping{belts: belts, agents: agents, agentsToRepo: toRepo}
}

// libraryMapKind maps one kind's repository ids to local ones. same says whether a local version is
// the repository's version of that number, which counts an item installed before origins were kept.
func libraryMapKind[T any](origin LibraryOrigin, repo, local map[string]map[int]T, provenance func(T) (*LibraryOrigin, string), same func(local, repo T) bool) libraryIDs {
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
		newest := libraryNewest(versions)
		if held, ok := repo[id][newest]; ok && same(versions[newest], held) {
			if _, taken := used[id]; !taken {
				ids[id] = id
				used[id] = struct{}{}
			}
		}
	}
	for _, id := range repoIDs {
		if _, mapped := ids[id]; mapped {
			continue
		}
		taken := func(candidate string) bool {
			_, assigned := used[candidate]
			return assigned || len(local[candidate]) > 0
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

// LibraryLocalID is the id item kind and id of origin's repository has, or would get, in this library.
func LibraryLocalID(origin LibraryOrigin, kind, id string, repoFiles, localFiles map[string][]byte) string {
	mapping := libraryMapFiles(origin, repoFiles, localFiles)
	if kind == "agent" {
		return mapping.agents.of(id)
	}
	return mapping.belts.of(id)
}

// LibraryLocalPath is the path in this library of a file at path in origin's repository.
func LibraryLocalPath(origin LibraryOrigin, path string, repoFiles, localFiles map[string][]byte) string {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return path
	}
	kind := "belt"
	if parts[0] == "agents" {
		kind = "agent"
	}
	parts[1] = LibraryLocalID(origin, kind, parts[1], repoFiles, localFiles)
	return strings.Join(parts, "/")
}

// libraryMapFiles is libraryMapIDs over a repository's and this library's files.
func libraryMapFiles(origin LibraryOrigin, repoFiles, localFiles map[string][]byte) libraryIDMapping {
	repoBelts, repoAgents, _ := libraryCheckRead(repoFiles)
	localBelts, localAgents, _ := libraryCheckRead(localFiles)
	return libraryMapIDs(origin, repoBelts, repoAgents, localBelts, localAgents)
}

// libraryAgentInRepo is a local agent as origin's repository holds it.
func libraryAgentInRepo(agent Agent, origin LibraryOrigin, agentsToRepo libraryIDs) Agent {
	if agent.Origin != nil && *agent.Origin == origin && agent.OriginID != "" {
		agent.ID = agent.OriginID
	}
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
	if belt.Origin != nil && *belt.Origin == origin && belt.OriginID != "" {
		belt.ID = belt.OriginID
	}
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
			to[at] = ids.of(to[at])
		}
		mapped[index].To = to
	}
	return mapped
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
