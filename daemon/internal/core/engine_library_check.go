package core

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// libraryBelts and libraryAgents are every version of each belt and agent, by id and then version.
type libraryBelts map[string]map[int]Belt

type libraryAgents map[string]map[int]Agent

// LibraryCheckProblems is what is wrong in the newest version of every belt and
// agent that isn't archived, in files (path to bytes). admission is nil when the
// caller has no model admission.
func LibraryCheckProblems(files map[string][]byte, admission *ModelAdmission) []string {
	archived := map[string]struct{}{}
	for path, data := range files {
		if !strings.HasSuffix(path, "/state.json") {
			continue
		}
		if StateFlag(data, "archived") {
			archived[strings.TrimSuffix(path, "state.json")] = struct{}{}
		}
	}
	current, _ := assistantPromptLatest(files)
	changed := map[string]struct{}{}
	for _, path := range current {
		live := true
		for prefix := range archived {
			if strings.HasPrefix(path, prefix) {
				live = false
				break
			}
		}
		if live {
			changed[path] = struct{}{}
		}
	}
	return LibraryCheckChangedProblems(files, changed, admission)
}

// LibraryCheckChangedProblems is what is wrong in the files of changed only, so an
// old version that the app still reads never shows up as a new problem.
func LibraryCheckChangedProblems(files map[string][]byte, changed map[string]struct{}, admission *ModelAdmission) []string {
	belts, agents, problems := libraryCheckRead(files)
	if len(problems) > 0 {
		return libraryCheckAbout(problems, changed)
	}
	latest := libraryCheckLatestAgents(agents)
	var found []string
	for _, path := range slices.Sorted(maps.Keys(changed)) {
		found = append(found, libraryCheckPathProblems(path, belts, agents, latest, admission)...)
	}
	return found
}

// libraryCheckRead decodes every belt and agent version in files, as the library
// reads them, and returns the problems: a path it cannot use, a file it cannot
// decode, a duplicate version or a belt stage pinned to a missing agent version.
// State files are read by the callers, not here.
func libraryCheckRead(files map[string][]byte) (libraryBelts, libraryAgents, []string) {
	belts := libraryBelts{}
	agents := libraryAgents{}
	var problems []string
	for _, path := range slices.Sorted(maps.Keys(files)) {
		parts := strings.Split(path, "/")
		if len(parts) != 3 || (parts[0] != "belts" && parts[0] != "agents") || !libraryCheckSafeID(parts[1]) {
			problems = append(problems, path+": unsafe or unsupported archive path")
			continue
		}
		if parts[2] == "state.json" {
			continue
		}
		if message := libraryCheckReadFile(files[path], parts[0] == "belts", parts[1], parts[2], belts, agents); message != "" {
			problems = append(problems, path+": "+message)
		}
	}
	problems = append(problems, libraryCheckPinProblems(belts, agents)...)
	slices.Sort(problems)
	libraryInheritOrigins(belts,
		func(belt Belt) (*LibraryOrigin, string) { return belt.Origin, belt.OriginID },
		func(belt *Belt, origin *LibraryOrigin, id string) { belt.Origin, belt.OriginID = origin, id })
	libraryInheritOrigins(agents,
		func(agent Agent) (*LibraryOrigin, string) { return agent.Origin, agent.OriginID },
		func(agent *Agent, origin *LibraryOrigin, id string) { agent.Origin, agent.OriginID = origin, id })
	return belts, agents, problems
}

// libraryCheckReadFile decodes one belt JSON or agent Markdown file into belts or
// agents and returns its problem, or "" when it is read. The path's id and version
// are authoritative; optional agent frontmatter may not contradict them.
func libraryCheckReadFile(data []byte, isBelt bool, id, filename string, belts libraryBelts, agents libraryAgents) string {
	suffix := ".md"
	if isBelt {
		suffix = ".json"
	}
	version, ok := libraryCheckVersion(filename, suffix)
	if !ok {
		return "expected a positive v<n>" + suffix
	}
	if isBelt {
		var belt Belt
		if err := jsonx.Unmarshal(data, &belt); err != nil {
			return err.Error()
		}
		if belt.ID != id || belt.Version != version {
			return "id/version disagrees with path"
		}
		if _, dup := belts[id][version]; dup {
			return fmt.Sprintf("duplicate version %d", version)
		}
		if belts[id] == nil {
			belts[id] = map[int]Belt{}
		}
		belts[id][version] = belt
		return ""
	}
	if !utf8.Valid(data) {
		return "invalid UTF-8"
	}
	text := string(data)
	if !libraryCheckIdentity(text, id, version) {
		return "id/version disagrees with path"
	}
	agent, err := AgentFileParse(text, id, version)
	if err != nil {
		return err.Error()
	}
	if _, dup := agents[id][version]; dup {
		return fmt.Sprintf("duplicate version %d", version)
	}
	if agents[id] == nil {
		agents[id] = map[int]Agent{}
	}
	agents[id][version] = agent
	return ""
}

// libraryCheckIdentity reports whether the frontmatter's id and version lines, when
// it has them, agree with the path's id and version.
func libraryCheckIdentity(text, id string, version int) bool {
	frontmatter := ""
	if parts := strings.Split(text, "---"); len(parts) > 1 {
		frontmatter = parts[1]
	}
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		pair := libraryCheckPair(line)
		if len(pair) != 2 {
			continue
		}
		if pair[0] == "id" && pair[1] != id {
			return false
		}
		if pair[0] == "version" {
			if n, err := strconv.Atoi(pair[1]); err != nil || n != version {
				return false
			}
		}
	}
	return true
}

// libraryCheckPair splits a frontmatter line at its first colon, omitting empty
// pieces, and trims each piece of whitespace and quotes.
func libraryCheckPair(line string) []string {
	var out []string
	for _, piece := range strings.SplitN(line, ":", 2) {
		if piece == "" {
			continue
		}
		out = append(out, strings.Trim(trimBlanks(piece), "\"'"))
	}
	return out
}

// libraryCheckPinProblems are the problems with the pins between belts and the agent
// versions they use.
func libraryCheckPinProblems(belts libraryBelts, agents libraryAgents) []string {
	var problems []string
	for _, id := range slices.Sorted(maps.Keys(belts)) {
		versions := belts[id]
		for _, version := range slices.Sorted(maps.Keys(versions)) {
			for _, stage := range versions[version].Stages {
				if stage.Kind() != StageKindAgent {
					continue
				}
				if stage.Agent == nil {
					problems = append(problems, fmt.Sprintf("belts/%s/v%d.json: stage %s has no agent pin", id, version, stage.ID))
					continue
				}
				ref := *stage.Agent
				if _, ok := agents[ref.ID][ref.Version]; !ok {
					problems = append(problems, fmt.Sprintf("belts/%s/v%d.json: missing agents/%s/v%d.md", id, version, ref.ID, ref.Version))
				}
			}
		}
	}
	return problems
}

// libraryCheckAbout keeps the problems that name a changed file.
func libraryCheckAbout(problems []string, changed map[string]struct{}) []string {
	var kept []string
	for _, message := range problems {
		for path := range changed {
			if strings.HasPrefix(message, path+":") {
				kept = append(kept, message)
				break
			}
		}
	}
	return kept
}

// libraryCheckVersion is the version a filename `v<n>` plus suffix names, when it is positive.
func libraryCheckVersion(filename, suffix string) (int, bool) {
	if !strings.HasPrefix(filename, "v") || !strings.HasSuffix(filename, suffix) {
		return 0, false
	}
	version, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filename, "v"), suffix))
	if err != nil || version <= 0 {
		return 0, false
	}
	return version, true
}

// libraryCheckLatestAgents is the newest version of each agent, the agents other
// agents must not share an id with.
func libraryCheckLatestAgents(agents libraryAgents) []Agent {
	var latest []Agent
	for _, id := range slices.Sorted(maps.Keys(agents)) {
		versions := agents[id]
		newest := 0
		for v := range versions {
			newest = max(newest, v)
		}
		if agent, ok := versions[newest]; ok {
			latest = append(latest, agent)
		}
	}
	return latest
}

// libraryCheckPathProblems is what is wrong in one changed file, an agent or a belt
// version. A path that is not a version of either is skipped.
func libraryCheckPathProblems(path string, belts libraryBelts, agents libraryAgents, latest []Agent, admission *ModelAdmission) []string {
	parts := strings.Split(path, "/")
	if len(parts) != 3 {
		return nil
	}
	switch parts[0] {
	case "agents":
		n, ok := libraryCheckVersion(parts[2], ".md")
		agent, found := agents[parts[1]][n]
		if !ok || !found {
			return nil
		}
		return libraryCheckAgentProblems(path, agent, latest, admission)
	case "belts":
		n, ok := libraryCheckVersion(parts[2], ".json")
		belt, found := belts[parts[1]][n]
		if !ok || !found {
			return nil
		}
		return libraryCheckBeltProblems(path, belt, agents, admission)
	}
	return nil
}

// libraryCheckAgentProblems checks an agent against the newest agents it must not
// share an id with, and its model policy when there is an admission.
func libraryCheckAgentProblems(path string, agent Agent, latest []Agent, admission *ModelAdmission) []string {
	var others []Agent
	for _, other := range latest {
		if other.ID != agent.ID {
			others = append(others, other)
		}
	}
	problems := libraryCheckPrefixed(path, ValidationAgentWarnings(agent, others))
	if admission != nil {
		problems = append(problems, libraryCheckPrefixed(path, ModelPolicyWarningsForPool(agent.Pool(), agent.Name, *admission, nil))...)
	}
	return problems
}

// libraryCheckBeltProblems checks a belt against the agent versions its stages pin,
// and its model policy when there is an admission.
func libraryCheckBeltProblems(path string, belt Belt, agents libraryAgents, admission *ModelAdmission) []string {
	pins := map[AgentRef]Agent{}
	for _, stage := range belt.Stages {
		if stage.Agent == nil {
			continue
		}
		if agent, ok := agents[stage.Agent.ID][stage.Agent.Version]; ok {
			pins[*stage.Agent] = agent
		}
	}
	// A file saved before stages had keys is converted from its
	// original pins before the checks, as Save would write it.
	converted := StageKeysExplicit(belt, pins)
	var keys []string
	for _, entry := range StageAdjustEntries(converted.Stages, func(s Stage) *string { return s.Key }, nil, false) {
		keys = append(keys, entry.Key)
	}
	problems := libraryCheckPrefixed(path, ValidationWarnings(converted, keys, pins))
	if admission != nil {
		problems = append(problems, libraryCheckPrefixed(path, ModelPolicyWarnings(converted.Stages, *admission, nil))...)
	}
	return problems
}

// libraryCheckPrefixed puts the file's path in front of each of its warnings.
func libraryCheckPrefixed(path string, warnings []string) []string {
	var problems []string
	for _, w := range warnings {
		problems = append(problems, path+": "+w)
	}
	return problems
}

// libraryCheckSafeID reports whether value is safe as a belt or agent id in a path:
// not empty, not "." or "..", and only ASCII letters, digits, - and _.
func libraryCheckSafeID(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
