// The card rules of the library browser: what each saved belt and agent shows on
// its library card, which filter it falls under and what it searches on. The
// coordinator reads the library and passes these plain states in.

package core

import (
	"fmt"
	"strconv"
	"strings"
)

// LibraryBeltState is one saved belt as the belt browser lists it. Archived wins
// over Paused when the card's status and filter are chosen.
type LibraryBeltState struct {
	Belt     Belt
	Archived bool
	Paused   bool
	// Shadow is the running shadow test's version, zero when none runs.
	Shadow int
}

// LibraryAgentState is one saved agent as the agent browser lists it: the record,
// the belts that use it and the glyph its stage boxes show.
type LibraryAgentState struct {
	Record AgentRecord
	UsedBy []AgentUsage
	Glyph  StageIcon
}

// Copy used by the library browsers. Literal text is exact.
const (
	libraryBrowserStatusArchived = "Archived"
	libraryBrowserActionNewBelt  = "New belt"
)

// libraryBrowserStageLimit is how many stage looks a belt card shows.
const libraryBrowserStageLimit = 7

// libraryBrowserBriefLimit is the longest brief, in characters, that is listed whole;
// a longer one is cut to libraryBrowserBriefKeep and an ellipsis.
const (
	libraryBrowserBriefLimit = 180
	libraryBrowserBriefKeep  = 177
)

// LibraryBeltBrowserOf is the belt library's list: one card per belt, with the
// belts' filters, search and commands. agentOf finds the agent a stage pins, so a
// card can show the stage looks.
func LibraryBeltBrowserOf(belts []LibraryBeltState, agentOf func(*AgentRef) *Agent, search, filter string) LibraryBrowser {
	entries := make([]LibraryBrowserProjectionEntry, 0, len(belts))
	for _, state := range belts {
		entries = append(entries, libraryBeltEntry(state, agentOf))
	}
	return LibraryBrowserProjectionMake(entries, libraryBrowserFilterDefinitions(), filter, search,
		[]LibraryBrowserAction{{ID: "new", Label: libraryBrowserActionNewBelt, Enabled: true}})
}

// LibraryAgentBrowserOf is the agent library's list: one card per agent, with the
// agents' filters, search and commands.
func LibraryAgentBrowserOf(agents []LibraryAgentState, search, filter string) LibraryBrowser {
	entries := make([]LibraryBrowserProjectionEntry, 0, len(agents))
	for _, state := range agents {
		entries = append(entries, libraryAgentEntry(state))
	}
	return LibraryBrowserProjectionMake(entries, []LibraryBrowserFilterDefinition{
		{ID: "all", Label: "All"}, {ID: "used", Label: "Used"}, {ID: "unused", Label: "Unused"}, {ID: "archived", Label: "Archived"},
	}, filter, search, []LibraryBrowserAction{{ID: "new", Label: agentActionsLabelNew, Enabled: true}})
}

// libraryBrowserFilterDefinitions are the belt browser's filters.
func libraryBrowserFilterDefinitions() []LibraryBrowserFilterDefinition {
	return []LibraryBrowserFilterDefinition{
		{ID: "all", Label: "All"}, {ID: "running", Label: "Running"}, {ID: "paused", Label: "Paused"}, {ID: "archived", Label: "Archived"},
	}
}

// libraryBeltEntry is one belt's card, its filter and its search text.
func libraryBeltEntry(state LibraryBeltState, agentOf func(*AgentRef) *Agent) LibraryBrowserProjectionEntry {
	belt := state.Belt
	looks := []LibraryCardStage{}
	for i, stage := range belt.Stages {
		if i == libraryBrowserStageLimit {
			break
		}
		looks = append(looks, LibraryCardStage(StageLookOf(stage, agentOf(stage.Agent))))
	}
	status, tone := "Running", LibraryCardStatusToneGood
	category := "running"
	switch {
	case state.Archived:
		status, tone = libraryBrowserStatusArchived, LibraryCardStatusToneGrey
		category = "archived"
	case state.Paused:
		status, tone = "Paused", LibraryCardStatusToneGrey
		category = "paused"
	}
	var firstKind *TriggerKind
	if len(belt.Triggers) > 0 {
		kind := belt.Triggers[0].Trigger.Kind
		firstKind = &kind
	}
	icon := StageLookTrigger(firstKind)
	tint := TintGrey
	detail := fmt.Sprintf("%d %s", len(belt.Stages), pluralWord(len(belt.Stages), "stage", "stages"))
	if belt.Origin != nil {
		detail += " · " + belt.Origin.Repository
	}
	card := LibraryCard{
		ID: belt.ID, Name: belt.Name, Version: belt.Version, Summary: belt.Summary(),
		Detail: detail, Status: status, StatusTone: tone, Icon: &icon, Tint: &tint, Stages: looks,
	}
	if state.Shadow > 0 {
		card.Shadow = new("Shadow v" + strconv.Itoa(state.Shadow))
	}
	var repos []string
	for _, entry := range belt.Triggers {
		repos = append(repos, entry.Repos...)
	}
	return LibraryBrowserProjectionEntry{
		Card: card, Filter: category,
		SearchText: strings.Join([]string{belt.Name, belt.Summary(), strings.Join(repos, " ")}, " "),
	}
}

// libraryAgentEntry is one agent's card, its filter and its search text.
func libraryAgentEntry(state LibraryAgentState) LibraryBrowserProjectionEntry {
	agent := state.Record.Agent
	uses := state.UsedBy
	usage := "Unused"
	if len(uses) > 0 {
		usage = fmt.Sprintf("Used by %d %s", len(uses), pluralWord(len(uses), "belt", "belts"))
	}
	model := agent.Model
	if model == "" {
		model = "default model"
	}
	detail := agent.Runner.Label() + " · " + model + " · " + usage
	if agent.Origin != nil {
		detail += " · " + agent.Origin.Repository
	}
	archived := state.Record.Archived
	category := "used"
	switch {
	case archived:
		category = "archived"
	case len(uses) == 0:
		category = "unused"
	}
	status, tone := "In use", LibraryCardStatusToneGood
	switch {
	case archived:
		status, tone = libraryBrowserStatusArchived, LibraryCardStatusToneGrey
	case len(uses) == 0:
		status, tone = "Unused", LibraryCardStatusToneGrey
	}
	glyph := state.Glyph
	tint := agent.Tint
	card := LibraryCard{
		ID: agent.ID, Name: agent.Name, Version: agent.Version,
		Summary: LibraryBrowserBriefSummary(agent.Brief), Detail: detail,
		Status: status, StatusTone: tone, Icon: &glyph, Tint: &tint, Stages: []LibraryCardStage{},
	}
	beltNames := make([]string, 0, len(uses))
	for _, use := range uses {
		beltNames = append(beltNames, use.BeltName)
	}
	searchText := strings.Join([]string{agent.Name, agent.Brief, detail, string(agent.Runner), agent.Model, strings.Join(beltNames, " ")}, " ")
	return LibraryBrowserProjectionEntry{Card: card, Filter: category, SearchText: searchText}
}

// LibraryBrowserBriefSummary is the one-line summary of a brief the browser lists:
// its first non-empty line, trimmed, shortened to 177 characters and an ellipsis
// when it is longer than 180.
func LibraryBrowserBriefSummary(brief string) string {
	lines := strings.FieldsFunc(brief, IsNewline)
	paragraph := ""
	if len(lines) > 0 {
		paragraph = strings.TrimSpace(lines[0])
	}
	characters := Characters(paragraph)
	if len(characters) <= libraryBrowserBriefLimit {
		return paragraph
	}
	return strings.TrimSpace(strings.Join(characters[:libraryBrowserBriefKeep], "")) + "…"
}

// pluralWord is singular when count is 1, plural otherwise.
func pluralWord(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
