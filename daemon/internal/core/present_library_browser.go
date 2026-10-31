package core

import (
	"sort"
	"strings"
)

// LibraryCardStage is one stage's icon and tint on a library card.
type LibraryCardStage struct {
	Icon StageIcon `json:"icon"`
	Tint Tint      `json:"tint"`
}

// LibraryCard is a compact library card whose content and appearance are prepared by the daemon.
type LibraryCard struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Version    int                   `json:"version"`
	Summary    string                `json:"summary"`
	Detail     string                `json:"detail"`
	Status     string                `json:"status"`
	StatusTone LibraryCardStatusTone `json:"statusTone"`
	Icon       *StageIcon            `json:"icon,omitzero"`
	Tint       *Tint                 `json:"tint,omitzero"`
	Stages     []LibraryCardStage    `json:"stages"`
	// Shadow is a belt's chip while a shadow test runs, such as "Shadow v2".
	Shadow *string `json:"shadow,omitzero"`
}

// LibraryCardStatusTone is the tone of a library card's status.
type LibraryCardStatusTone string

const (
	LibraryCardStatusToneGood LibraryCardStatusTone = "good"
	LibraryCardStatusToneGrey LibraryCardStatusTone = "grey"
	LibraryCardStatusToneNeed LibraryCardStatusTone = "need"
	// LibraryCardStatusToneGold and LibraryCardStatusToneProblem mark a Library item with an update or a conflict.
	LibraryCardStatusToneGold    LibraryCardStatusTone = "gold"
	LibraryCardStatusToneProblem LibraryCardStatusTone = "problem"
)

// LibraryFilter is one filter button of a library browser, with how many cards it matches.
type LibraryFilter struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// LibraryBrowserAction is one command a library browser offers.
type LibraryBrowserAction struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Enabled bool    `json:"enabled"`
	Reason  *string `json:"reason,omitzero"`
}

// LibraryBrowser is the library's list: its cards, filters, counts and commands.
type LibraryBrowser struct {
	Items      []LibraryCard          `json:"items"`
	Filters    []LibraryFilter        `json:"filters"`
	TotalCount int                    `json:"totalCount"`
	MatchCount int                    `json:"matchCount"`
	Actions    []LibraryBrowserAction `json:"actions"`
}

// NewLibraryBrowser is a browser over items, with matchCount set to len(items).
func NewLibraryBrowser(items []LibraryCard, filters []LibraryFilter, totalCount int, actions []LibraryBrowserAction) LibraryBrowser {
	if items == nil {
		items = []LibraryCard{}
	}
	if filters == nil {
		filters = []LibraryFilter{}
	}
	if actions == nil {
		actions = []LibraryBrowserAction{}
	}
	return LibraryBrowser{Items: items, Filters: filters, TotalCount: totalCount, MatchCount: len(items), Actions: actions}
}

// Copy and filter ids used by LibraryBrowserProjection. Literal text is exact.
const (
	libraryBrowserFilterAll      = "all"
	libraryBrowserFilterArchived = "archived"
)

// LibraryBrowserProjectionEntry is one library entry as the projection searches and filters it.
type LibraryBrowserProjectionEntry struct {
	Card       LibraryCard
	Filter     string
	SearchText string
}

// LibraryBrowserFilterDefinition is one filter the projection offers, by id and label.
type LibraryBrowserFilterDefinition struct {
	ID    string
	Label string
}

// LibraryBrowserProjectionMake is the shared search, filter counts and ordering for the belt
// and agent library browsers. Search is trimmed of whitespace and matched without case.
// An unknown selected filter falls back to all.
func LibraryBrowserProjectionMake(entries []LibraryBrowserProjectionEntry, filterDefinitions []LibraryBrowserFilterDefinition,
	selectedFilter, search string, actions []LibraryBrowserAction) LibraryBrowser {
	term := strings.TrimSpace(search)
	searched := make([]LibraryBrowserProjectionEntry, 0, len(entries))
	for _, entry := range entries {
		if term == "" || LocalizedCaseInsensitiveContains(entry.SearchText, term) {
			searched = append(searched, entry)
		}
	}
	filters := make([]LibraryFilter, 0, len(filterDefinitions))
	for _, definition := range filterDefinitions {
		count := 0
		for _, entry := range searched {
			if (definition.ID == libraryBrowserFilterAll && entry.Filter != libraryBrowserFilterArchived) || entry.Filter == definition.ID {
				count++
			}
		}
		filters = append(filters, LibraryFilter{ID: definition.ID, Label: definition.Label, Count: count})
	}
	validFilter := libraryBrowserFilterAll
	for _, definition := range filterDefinitions {
		if definition.ID == selectedFilter {
			validFilter = selectedFilter
			break
		}
	}
	var selected []LibraryCard
	for _, entry := range searched {
		if (validFilter == libraryBrowserFilterAll && entry.Filter != libraryBrowserFilterArchived) || entry.Filter == validFilter {
			selected = append(selected, entry.Card)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		left := FoldCaseDiacritics(selected[i].Name)
		right := FoldCaseDiacritics(selected[j].Name)
		if left != right {
			return left < right
		}
		return selected[i].ID < selected[j].ID
	})
	if selected == nil {
		selected = []LibraryCard{}
	}
	return NewLibraryBrowser(selected, filters, len(entries), actions)
}
