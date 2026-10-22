// The configuration assistant's index of the library: one line per
// definition instead of its whole file.

package core

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// assistantIndex lists every belt and agent in library by the path of its newest
// version, with what it is (name, triggers and stages, or runner and model), its
// paused and archived flags and its earlier versions, and then any other file by
// its path. A file that cannot be read says so; the problems list says why.
func assistantIndex(library map[string][]byte) string {
	current, older := assistantPromptLatest(library)
	earlier := map[string][]int{}
	for _, path := range older {
		earlier[assistantPromptFolder(path)] = append(earlier[assistantPromptFolder(path)], *assistantPromptVersion(path))
	}
	var belts, agents, others []string
	for _, path := range current {
		folder := assistantPromptFolder(path)
		if strings.HasSuffix(path, "/state.json") {
			if !slices.ContainsFunc(current, func(other string) bool {
				return other != path && assistantPromptFolder(other) == folder
			}) {
				others = append(others, "- "+path)
			}
			continue
		}
		if assistantPromptVersion(path) == nil {
			others = append(others, "- "+path)
			continue
		}
		line := "- " + path + ": "
		switch {
		case strings.HasPrefix(path, "belts/"):
			belts = append(belts, line+assistantIndexBelt(library[path])+assistantIndexExtras(library, folder, earlier[folder]))
		case strings.HasPrefix(path, "agents/"):
			agents = append(agents, line+assistantIndexAgent(library[path], path)+assistantIndexExtras(library, folder, earlier[folder]))
		default:
			others = append(others, "- "+path)
		}
	}
	section := func(title string, lines []string) string {
		if len(lines) == 0 {
			return "### " + title + "\nNone."
		}
		return "### " + title + "\n" + strings.Join(lines, "\n")
	}
	text := section("Belts", belts) + "\n\n" + section("Agents", agents)
	if len(others) > 0 {
		text += "\n\n" + section("Other files", others)
	}
	return text
}

// assistantIndexBelt is a belt's name, triggers and stages in order, each agent
// stage with the agent version it pins.
func assistantIndexBelt(data []byte) string {
	var belt Belt
	if err := jsonx.Unmarshal(data, &belt); err != nil {
		return "cannot be read."
	}
	kinds := make([]string, 0, len(belt.Triggers))
	for _, entry := range belt.Triggers {
		kinds = append(kinds, string(entry.Trigger.Kind))
	}
	stages := make([]string, 0, len(belt.Stages))
	for _, stage := range belt.Stages {
		name := stage.Name
		if stage.Agent != nil {
			name += " (" + stage.Agent.ID + " v" + strconv.Itoa(stage.Agent.Version) + ")"
		}
		stages = append(stages, name)
	}
	text := strconv.Quote(belt.Name) + "; triggers: " + assistantIndexList(kinds) + "; stages: " + assistantIndexList(stages)
	if len(belt.Forks) > 0 {
		text += "; forks: " + strconv.Itoa(len(belt.Forks))
	}
	return text + "."
}

// assistantIndexAgent is an agent's name, runner and model, and its outcomes.
func assistantIndexAgent(data []byte, path string) string {
	if !utf8.Valid(data) {
		return "cannot be read."
	}
	id := strings.Split(path, "/")[1]
	agent, err := AgentFileParse(string(data), id, *assistantPromptVersion(path))
	if err != nil {
		return "cannot be read."
	}
	runner := string(agent.Runner)
	if agent.Model != "" {
		runner += " " + agent.Model
	}
	outcomes := make([]string, 0, len(agent.Outcomes))
	for _, outcome := range agent.Outcomes {
		outcomes = append(outcomes, outcome.Name)
	}
	return strconv.Quote(agent.Name) + "; runner: " + runner + "; outcomes: " + assistantIndexList(outcomes) + "."
}

// assistantIndexExtras is a definition's paused and archived flags and its
// earlier versions.
func assistantIndexExtras(library map[string][]byte, folder string, versions []int) string {
	text := ""
	if state, ok := library[folder+"/state.json"]; ok {
		for _, flag := range []string{"paused", "archived"} {
			if StateFlag(state, flag) {
				text += " " + strings.ToUpper(flag[:1]) + flag[1:] + "."
			}
		}
	}
	if len(versions) > 0 {
		text += " Earlier: " + assistantIndexVersions(versions) + "."
	}
	return text
}

// assistantIndexVersions writes version numbers with each run of consecutive
// ones as a range: v1–v4, v7.
func assistantIndexVersions(versions []int) string {
	sorted := slices.Compact(slices.Sorted(slices.Values(versions)))
	var parts []string
	for i := 0; i < len(sorted); {
		j := i
		for j+1 < len(sorted) && sorted[j+1] == sorted[j]+1 {
			j++
		}
		part := "v" + strconv.Itoa(sorted[i])
		if j > i {
			part += "–v" + strconv.Itoa(sorted[j])
		}
		parts = append(parts, part)
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// assistantIndexList joins items with commas, or says there are none.
func assistantIndexList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
