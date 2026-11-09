// The files a library repository write puts in place: an install, a share and a
// backup. Each one is decided here and written by the caller.

package core

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// LibraryFile is one file of the library, by its path under the data folder or repository.
type LibraryFile struct {
	Path string
	Data []byte
}

// LibraryInstallFiles are the files installing item kind and id from origin writes:
// its latest version with its origin, and each agent version it pins that the
// library lacks, and the id it has here. An item that is installed writes nothing and
// has no id returned; a conflict is refused.
func LibraryInstallFiles(origin LibraryOrigin, kind, id string, repoFiles, localFiles map[string][]byte) (string, []LibraryFile, error) {
	items, _ := LibraryRepoItems(origin, repoFiles, localFiles)
	item, found := libraryFind(items, kind, id)
	if !found {
		return "", nil, NewFailure(fmt.Sprintf("%s %s is not in %s.", kind, id, origin.Repository))
	}
	switch item.State {
	case LibraryItemConflict:
		return "", nil, NewFailure(*item.Reason)
	case LibraryItemInstalled:
		return "", nil, nil
	}
	repoBelts, repoAgents, _ := libraryCheckRead(repoFiles)
	localBelts, localAgents, _ := libraryCheckRead(localFiles)
	mapping := libraryMapIDs(origin, repoBelts, repoAgents, localBelts, localAgents)
	if kind == "agent" {
		agent := libraryAgentInLibrary(repoAgents[id][item.Version], origin, mapping.agents.of(id), mapping.agents)
		return agent.ID, []LibraryFile{{Path: libraryVersionPath("agents", agent.ID, item.Version, "md"), Data: []byte(AgentFileWrite(agent))}}, nil
	}
	belt := repoBelts[id][item.Version]
	var files []LibraryFile
	for _, stage := range belt.Stages {
		if stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		localID := mapping.agents.of(ref.ID)
		// An id the repository doesn't hold says nothing about a local agent of that id.
		if _, have := localAgents[localID][ref.Version]; have && len(repoAgents[ref.ID]) > 0 {
			continue
		}
		agent, ok := repoAgents[ref.ID][ref.Version]
		if !ok {
			return "", nil, NewFailure(fmt.Sprintf("%s v%d uses %s v%d, which %s does not hold.", id, item.Version, ref.ID, ref.Version, origin.Repository))
		}
		agent = libraryAgentInLibrary(agent, origin, localID, mapping.agents)
		files = append(files, LibraryFile{Path: libraryVersionPath("agents", localID, ref.Version, "md"), Data: []byte(AgentFileWrite(agent))})
	}
	localID := mapping.belts.of(id)
	belt.OriginID = ""
	if localID != id {
		belt.OriginID = id
	}
	belt.ID, belt.Origin = localID, &origin
	belt.Stages = libraryMapPins(belt.Stages, mapping.agents)
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		return "", nil, err
	}
	return localID, append(files, LibraryFile{Path: libraryVersionPath("belts", localID, item.Version, "json"), Data: data}), nil
}

// LibraryShareFiles are the files sharing item kind and id from this library to
// origin's repository: its latest version as the repository should hold it, and
// the agent versions a belt pins. A belt sheds what belongs to one library: its
// repositories and base branch, its AFK mode and its origin. A version the
// repository already holds with the same content is not written; with other
// content it is refused, as a shared version must not change. An item is refused too
// when the repository holds something unrelated under the id it would go to.
func LibraryShareFiles(origin LibraryOrigin, kind, id string, localFiles, repoFiles map[string][]byte) ([]LibraryFile, error) {
	localBelts, localAgents, _ := libraryCheckRead(localFiles)
	repoBelts, repoAgents, _ := libraryCheckRead(repoFiles)
	mapping := libraryMapIDs(origin, repoBelts, repoAgents, localBelts, localAgents)
	agentsToRepo := mapping.agentsToRepo
	sameAgent := func(local, repo Agent) bool {
		return libraryAgentText(libraryAgentInRepo(local, origin, agentsToRepo)) == libraryAgentText(repo)
	}
	var files []LibraryFile
	if kind == "agent" {
		versions := localAgents[id]
		if len(versions) == 0 {
			return nil, NewFailure(fmt.Sprintf("%s is not in your library.", id))
		}
		agent := libraryAgentInRepo(versions[libraryNewest(versions)], origin, agentsToRepo)
		agent.Origin = nil
		if err := libraryShareIDFree(origin, "agent", id, agent.ID, len(repoAgents[agent.ID]) > 0, librarySharesAVersion(versions, repoAgents[agent.ID], sameAgent), mapping.agents); err != nil {
			return nil, err
		}
		return libraryShareAgents(files, origin, []Agent{agent}, repoAgents)
	}
	versions := localBelts[id]
	if len(versions) == 0 {
		return nil, NewFailure(fmt.Sprintf("%s is not in your library.", id))
	}
	sameBelt := func(local, repo Belt) bool {
		return libraryBeltShared(libraryBeltInRepo(local, origin, agentsToRepo)) == libraryBeltShared(repo)
	}
	belt := versions[libraryNewest(versions)]
	pinned := belt.Stages
	belt = libraryBeltInRepo(belt, origin, agentsToRepo)
	belt.Origin = nil
	belt.Base = ""
	belt.RepositoryBases = nil
	belt.Afk = AfkModeGlobal
	for index := range belt.Triggers {
		belt.Triggers[index].Repos = nil
	}
	if err := libraryShareIDFree(origin, "belt", id, belt.ID, len(repoBelts[belt.ID]) > 0, librarySharesAVersion(versions, repoBelts[belt.ID], sameBelt), mapping.belts); err != nil {
		return nil, err
	}
	var pins []Agent
	pinnedAs := map[string]string{}
	for _, stage := range pinned {
		if stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		agent, ok := localAgents[ref.ID][ref.Version]
		if !ok {
			return nil, NewFailure(fmt.Sprintf("%s v%d uses %s v%d, which is not in your library.", id, belt.Version, ref.ID, ref.Version))
		}
		agent = libraryAgentInRepo(agent, origin, agentsToRepo)
		agent.Origin = nil
		if err := libraryShareIDFree(origin, "agent", ref.ID, agent.ID, len(repoAgents[agent.ID]) > 0, librarySharesAVersion(localAgents[ref.ID], repoAgents[agent.ID], sameAgent), mapping.agents); err != nil {
			return nil, err
		}
		if other, ok := pinnedAs[agent.ID]; ok && other != ref.ID {
			return nil, NewFailure(fmt.Sprintf("%s pins both %s and %s, which are the same id, %s, in %s.", id, other, ref.ID, agent.ID, origin.Repository))
		}
		pinnedAs[agent.ID] = ref.ID
		pins = append(pins, agent)
	}
	files, err := libraryShareBelt(files, origin, belt, repoBelts)
	if err != nil {
		return nil, err
	}
	return libraryShareAgents(files, origin, pins, repoAgents)
}

// libraryShareIDFree refuses to share local item localID as repoID when the repository holds repoID for
// something unrelated, which would add a version to an item that isn't this one. sharesVersion says the
// repository holds one of the item's versions, as it does after an earlier share of an item from elsewhere.
func libraryShareIDFree(origin LibraryOrigin, kind, localID, repoID string, held, sharesVersion bool, ids libraryIDs) error {
	if held && !sharesVersion && ids.of(repoID) != localID {
		return NewFailure(fmt.Sprintf("%s already uses the id %s for a different %s, so %s can't be shared there.", origin.Repository, repoID, kind, localID))
	}
	return nil
}

// libraryShareBelt adds the belt's file to files unless the repository holds it already.
func libraryShareBelt(files []LibraryFile, origin LibraryOrigin, belt Belt, repoBelts libraryBelts) ([]LibraryFile, error) {
	data, err := jsonx.MarshalStorage(belt)
	if err != nil {
		return nil, err
	}
	var held *string
	if repo, ok := repoBelts[belt.ID][belt.Version]; ok {
		text := libraryBeltText(repo)
		held = &text
	}
	return libraryShareAdd(files, origin, libraryVersionPath("belts", belt.ID, belt.Version, "json"), belt.ID, belt.Version, string(data), held)
}

// libraryShareAgents adds each agent version to files, as libraryShareBelt does.
func libraryShareAgents(files []LibraryFile, origin LibraryOrigin, agents []Agent, repoAgents libraryAgents) ([]LibraryFile, error) {
	for _, agent := range agents {
		var held *string
		if repo, ok := repoAgents[agent.ID][agent.Version]; ok {
			text := libraryAgentText(repo)
			held = &text
		}
		var err error
		files, err = libraryShareAdd(files, origin, libraryVersionPath("agents", agent.ID, agent.Version, "md"), agent.ID, agent.Version, AgentFileWrite(agent), held)
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// libraryShareAdd adds one version's file, unless the repository holds the same text,
// and refuses a repository that holds other text for that version.
func libraryShareAdd(files []LibraryFile, origin LibraryOrigin, path, id string, version int, text string, held *string) ([]LibraryFile, error) {
	if held == nil {
		return append(files, LibraryFile{Path: path, Data: []byte(text)}), nil
	}
	if *held == text {
		return files, nil
	}
	return nil, NewFailure(fmt.Sprintf("%s v%d already exists in %s with other content; save a new version first.", id, version, origin.Repository))
}

// LibraryShareBranch is the branch a share of item kind ("belt" or "agent") and id at version goes on.
// The kind is in the name, so a belt and an agent with the same id and version never share a branch,
// and a share of one never replaces the other's open pull request.
func LibraryShareBranch(kind, id string, version int) string {
	return fmt.Sprintf("beltline/share-%s-%s-v%d", kind, id, version)
}

// LibraryShareTitle is the title of the commit and pull request of a share of item kind and id at version.
func LibraryShareTitle(kind, id string, version int) string {
	return fmt.Sprintf("Share %s %s v%d", kind, id, version)
}

// LibraryBackupFiles are the files a backup holds: every belt and agent version, but
// not the state files, which stay on this Mac.
func LibraryBackupFiles(files map[string][]byte) map[string][]byte {
	backup := map[string][]byte{}
	for path, data := range files {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && (parts[0] == "belts" || parts[0] == "agents") && parts[2] != "state.json" {
			backup[path] = data
		}
	}
	return backup
}

// libraryVersionPath is the library path of version n of an id's file: belts/<id>/v<n>.json.
func libraryVersionPath(folder, id string, version int, ext string) string {
	return folder + "/" + id + "/v" + strconv.Itoa(version) + "." + ext
}

// libraryFind is the item of kind and id in items.
func libraryFind(items []LibraryItem, kind, id string) (LibraryItem, bool) {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item, true
		}
	}
	return LibraryItem{}, false
}

// LibraryItemPaths are the paths of an item's latest version and, for a belt, the
// agent versions it pins, as a repository's files name them.
func LibraryItemPaths(kind, id string, repoFiles map[string][]byte) []string {
	repoBelts, repoAgents, _ := libraryCheckRead(repoFiles)
	if kind == "agent" {
		if versions := repoAgents[id]; len(versions) > 0 {
			return []string{libraryVersionPath("agents", id, libraryNewest(versions), "md")}
		}
		return nil
	}
	versions := repoBelts[id]
	if len(versions) == 0 {
		return nil
	}
	latest := libraryNewest(versions)
	paths := []string{libraryVersionPath("belts", id, latest, "json")}
	for _, stage := range versions[latest].Stages {
		if stage.Agent != nil {
			paths = append(paths, libraryVersionPath("agents", stage.Agent.ID, stage.Agent.Version, "md"))
		}
	}
	return paths
}
