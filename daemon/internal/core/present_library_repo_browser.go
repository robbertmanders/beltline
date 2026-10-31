// The Library page's Belts and Agents tabs: what the shared library repositories
// hold, as cards like the Belts and Agents pages show, and the belt or agent a card
// opens. The coordinator reads each repository's files and passes them in.

package core

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// LibraryRepoContents is one shared repository's belts and agents, read once from its files,
// with where each item stands against this library.
type LibraryRepoContents struct {
	Origin LibraryOrigin
	Items  []LibraryItem
	belts  libraryBelts
	agents libraryAgents
}

// LibraryRepoContentsRead reads a shared repository's files: its items, as LibraryRepoItems
// says how they stand, and every version of its belts and agents. problems are what is
// wrong with its files.
func LibraryRepoContentsRead(origin LibraryOrigin, repoFiles, localFiles map[string][]byte) (LibraryRepoContents, []string) {
	items, problems := LibraryRepoItems(origin, repoFiles, localFiles)
	belts, agents, _ := libraryCheckRead(repoFiles)
	return LibraryRepoContents{Origin: origin, Items: items, belts: belts, agents: agents}, problems
}

// Item is the repository's item of kind and id.
func (c LibraryRepoContents) Item(kind, id string) (LibraryItem, bool) {
	for _, item := range c.Items {
		if item.Kind == kind && item.ID == id {
			return item, true
		}
	}
	return LibraryItem{}, false
}

// Belt is the newest version of the repository's belt id.
func (c LibraryRepoContents) Belt(id string) (Belt, bool) {
	versions := c.belts[id]
	if len(versions) == 0 {
		return Belt{}, false
	}
	return versions[libraryNewest(versions)], true
}

// LatestAgent is the newest version of the repository's agent id.
func (c LibraryRepoContents) LatestAgent(id string) (Agent, bool) {
	versions := c.agents[id]
	if len(versions) == 0 {
		return Agent{}, false
	}
	return versions[libraryNewest(versions)], true
}

// Agent is the agent version ref names, or nil when the repository lacks it.
func (c LibraryRepoContents) Agent(ref *AgentRef) *Agent {
	if ref == nil {
		return nil
	}
	agent, ok := c.agents[ref.ID][ref.Version]
	if !ok {
		return nil
	}
	return &agent
}

// AgentRecords are the newest version of each of the repository's agents, by id.
func (c LibraryRepoContents) AgentRecords() []AgentRecord {
	records := make([]AgentRecord, 0, len(c.agents))
	for _, id := range slices.Sorted(maps.Keys(c.agents)) {
		agent, _ := c.LatestAgent(id)
		records = append(records, AgentRecord{Agent: agent})
	}
	return records
}

// AgentUsages are the repository's belts whose newest version pins agent id, with the
// highest version they pin, by belt id.
func (c LibraryRepoContents) AgentUsages(id string) []AgentUsage {
	usages := []AgentUsage{}
	for _, beltID := range slices.Sorted(maps.Keys(c.belts)) {
		belt, _ := c.Belt(beltID)
		highest := 0
		for _, stage := range belt.Stages {
			if stage.Agent != nil && stage.Agent.ID == id {
				highest = max(highest, stage.Agent.Version)
			}
		}
		if highest > 0 {
			usages = append(usages, AgentUsage{BeltID: belt.ID, BeltName: belt.Name, Version: highest})
		}
	}
	return usages
}

// LibraryRepoCard is one card of the Library page: a belt or agent of a shared repository,
// drawn as the Belts and Agents pages draw theirs, with the repository it opens from.
type LibraryRepoCard struct {
	LibraryCard
	Kind       string `json:"kind"`
	Repository string `json:"repository"`
	Path       string `json:"path"`
}

// LibraryRepoTab is one tab of the Library page, with how many items it lists and whether
// one of them has an update or a conflict.
type LibraryRepoTab struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Count     int    `json:"count"`
	Attention bool   `json:"attention"`
}

// LibraryRepoBrowser is one tab of the Library page: its cards, filters and counts, and
// both tabs' headings.
type LibraryRepoBrowser struct {
	Tabs       []LibraryRepoTab  `json:"tabs"`
	Items      []LibraryRepoCard `json:"items"`
	Filters    []LibraryFilter   `json:"filters"`
	TotalCount int               `json:"totalCount"`
	MatchCount int               `json:"matchCount"`
}

// libraryRepoStates are the statuses a Library card shows for each item state.
var libraryRepoStates = map[LibraryItemState]struct {
	status string
	tone   LibraryCardStatusTone
}{
	LibraryItemAvailable: {"Available", LibraryCardStatusToneGrey},
	LibraryItemInstalled: {"Installed", LibraryCardStatusToneGood},
	LibraryItemUpdate:    {"Update", LibraryCardStatusToneGold},
	LibraryItemConflict:  {"Conflict", LibraryCardStatusToneProblem},
}

// LibraryRepoBrowserOf is the Library page's tab of kind ("belt" or "agent"): one card per
// item of that kind in each repository, searched and filtered by state as the Belts and Agents
// pages search and filter theirs.
func LibraryRepoBrowserOf(repos []LibraryRepoContents, kind, search, filter string) LibraryRepoBrowser {
	tabs := []LibraryRepoTab{{ID: "belt", Label: "Belts"}, {ID: "agent", Label: "Agents"}}
	var entries []LibraryBrowserProjectionEntry
	var cards []LibraryRepoCard
	for _, repo := range repos {
		for _, item := range repo.Items {
			for index := range tabs {
				if tabs[index].ID == item.Kind {
					tabs[index].Count++
					tabs[index].Attention = tabs[index].Attention || item.State == LibraryItemUpdate || item.State == LibraryItemConflict
				}
			}
			if item.Kind != kind {
				continue
			}
			card, searchText := libraryRepoCard(repo, item)
			// The projection sorts by name and then id; the key keeps the card's place in cards.
			key := fmt.Sprintf("%06d", len(cards))
			cards = append(cards, card)
			projected := card.LibraryCard
			projected.ID = key
			entries = append(entries, LibraryBrowserProjectionEntry{Card: projected, Filter: string(item.State), SearchText: searchText})
		}
	}
	projection := LibraryBrowserProjectionMake(entries, []LibraryBrowserFilterDefinition{
		{ID: "all", Label: "All"}, {ID: string(LibraryItemAvailable), Label: "Available"}, {ID: string(LibraryItemInstalled), Label: "Installed"},
		{ID: string(LibraryItemUpdate), Label: "Updates"}, {ID: string(LibraryItemConflict), Label: "Conflicts"},
	}, filter, search, nil)
	items := make([]LibraryRepoCard, 0, len(projection.Items))
	for _, projected := range projection.Items {
		index, _ := strconv.Atoi(projected.ID)
		items = append(items, cards[index])
	}
	return LibraryRepoBrowser{Tabs: tabs, Items: items, Filters: projection.Filters, TotalCount: projection.TotalCount, MatchCount: len(items)}
}

// libraryRepoCard is one item's card and the text a search matches it on.
func libraryRepoCard(repo LibraryRepoContents, item LibraryItem) (LibraryRepoCard, string) {
	state := libraryRepoStates[item.State]
	source := repo.Origin.Repository
	if repo.Origin.Path != "" {
		source += " / " + repo.Origin.Path
	}
	card := LibraryCard{ID: item.ID, Name: item.Name, Version: item.Version, Detail: source,
		Status: state.status, StatusTone: state.tone, Stages: []LibraryCardStage{}}
	if item.Kind == "belt" {
		if belt, ok := repo.Belt(item.ID); ok {
			var firstKind *TriggerKind
			if len(belt.Triggers) > 0 {
				kind := belt.Triggers[0].Trigger.Kind
				firstKind = &kind
			}
			icon, tint := StageLookTrigger(firstKind), TintGrey
			card.Icon, card.Tint, card.Summary = &icon, &tint, belt.Summary()
			for index, stage := range belt.Stages {
				if index == libraryBrowserStageLimit {
					break
				}
				card.Stages = append(card.Stages, LibraryCardStage(StageLookOf(stage, repo.Agent(stage.Agent))))
			}
		}
	} else if agent, ok := repo.LatestAgent(item.ID); ok {
		glyph, tint := agent.Glyph(), agent.Tint
		card.Icon, card.Tint, card.Summary = &glyph, &tint, LibraryBrowserBriefSummary(agent.Brief)
	}
	return LibraryRepoCard{LibraryCard: card, Kind: item.Kind, Repository: repo.Origin.Repository, Path: repo.Origin.Path},
		strings.Join([]string{item.Name, item.ID, card.Summary, source}, " ")
}

// LibraryRepoAgentPresentation is what the agent editor shows beside a shared agent: its
// outcome footer and placeholders, the repository's belts that use it and the agents its
// outcomes may go to. A shared agent has no warnings or draft status here.
func LibraryRepoAgentPresentation(repo LibraryRepoContents, agent Agent) AgentPresentation {
	stage := DefaultsStageForAgent(agent, nil, nil, nil, TriggerKindIssues)
	return AgentPresentation{
		Warnings:       []string{},
		Footer:         BriefFooter(stage, stage.Writes()),
		Placeholders:   PlaceholderHelpAgent(),
		UsedBy:         repo.AgentUsages(agent.ID),
		OutcomeTargets: AgentOutcomeTargets(agent.ID, repo.AgentRecords()),
		Glyph:          agent.Glyph(),
	}
}
