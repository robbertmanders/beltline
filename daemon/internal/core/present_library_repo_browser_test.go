package core

import (
	"testing"
)

// libraryTestContents is the test repository read against local, with a second repository holding
// the same files under another folder when twice is set.
func libraryTestContents(t *testing.T, local map[string][]byte, twice bool) []LibraryRepoContents {
	t.Helper()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	contents, problems := LibraryRepoContentsRead(libraryTestOrigin, repo, local)
	if len(problems) != 0 {
		t.Fatalf("problems %v", problems)
	}
	out := []LibraryRepoContents{contents}
	if twice {
		other := LibraryOrigin{Repository: "owner/other", Path: "team"}
		second, _ := LibraryRepoContentsRead(other, repo, local)
		out = append(out, second)
	}
	return out
}

func TestLibraryRepoBrowserListsOneKindWithItsStateAndRepository(t *testing.T) {
	t.Parallel()
	repo := libraryTestRepo(t, 2, &libraryTestOrigin)
	local := libraryTestLocal(t, repo, 1, &libraryTestOrigin)
	browser := LibraryRepoBrowserOf(libraryTestContents(t, local, true), "belt", "", "")
	if len(browser.Items) != 2 || browser.MatchCount != 2 || browser.TotalCount != 2 {
		t.Fatalf("belt cards %+v", browser.Items)
	}
	card := browser.Items[0]
	if card.Kind != "belt" || card.ID != "team" || card.Repository != "owner/shared" || card.Path != "" || card.Detail != "owner/shared" {
		t.Fatalf("first card %+v, want the team belt of owner/shared (sorted by name, then in reading order)", card)
	}
	if second := browser.Items[1]; second.Repository != "owner/other" || second.Path != "team" || second.Detail != "owner/other / team" {
		t.Fatalf("second card %+v, want the team belt of owner/other / team", second)
	}
	if card.Status != "Update" || card.StatusTone != LibraryCardStatusToneGold {
		t.Fatalf("status %q %q, want the update the local v1 has", card.Status, card.StatusTone)
	}
	if card.Icon == nil || len(card.Stages) == 0 || len(card.Stages) > libraryBrowserStageLimit {
		t.Fatalf("belt card has no trigger icon or stage looks: %+v", card)
	}
	if browser.Tabs[0].ID != "belt" || browser.Tabs[0].Count != 2 || !browser.Tabs[0].Attention {
		t.Fatalf("belt tab %+v, want two items with an update", browser.Tabs[0])
	}
	if agents := browser.Tabs[1]; agents.ID != "agent" || agents.Count != 2*len(SampleAgents()) || agents.Attention {
		t.Fatalf("agent tab %+v, want every sample agent twice, all installed", agents)
	}
}

func TestLibraryRepoBrowserFiltersByStateAndSearches(t *testing.T) {
	t.Parallel()
	repos := libraryTestContents(t, map[string][]byte{}, false)
	all := LibraryRepoBrowserOf(repos, "agent", "", "all")
	if len(all.Items) != len(SampleAgents()) || all.Items[0].Status != "Available" || all.Items[0].StatusTone != LibraryCardStatusToneGrey {
		t.Fatalf("agent cards %+v", all.Items)
	}
	counts := map[string]int{}
	for _, filter := range all.Filters {
		counts[filter.ID] = filter.Count
	}
	if counts["all"] != len(SampleAgents()) || counts["available"] != len(SampleAgents()) || counts["installed"] != 0 {
		t.Fatalf("filters %+v", all.Filters)
	}
	if installed := LibraryRepoBrowserOf(repos, "agent", "", "installed"); len(installed.Items) != 0 || installed.TotalCount != len(SampleAgents()) {
		t.Fatalf("installed filter %+v", installed)
	}
	found := LibraryRepoBrowserOf(repos, "agent", "  TRIAGE ", "")
	ids := map[string]bool{}
	for _, card := range found.Items {
		ids[card.ID] = true
	}
	if !ids["triage"] || ids["build"] || len(found.Items) == len(SampleAgents()) {
		t.Fatalf("search for triage %+v", found.Items)
	}
}

func TestLibraryRepoContentsAnswerTheItemsAnEditorOpens(t *testing.T) {
	t.Parallel()
	repo := libraryTestContents(t, map[string][]byte{}, false)[0]
	belt, ok := repo.Belt("team")
	if !ok || belt.Version != 2 {
		t.Fatalf("belt %+v %v", belt, ok)
	}
	if _, ok := repo.Belt("missing"); ok {
		t.Fatal("a belt the repository lacks was found")
	}
	if item, ok := repo.Item("belt", "team"); !ok || item.State != LibraryItemAvailable {
		t.Fatalf("item %+v %v", item, ok)
	}
	ref := belt.Stages[0].Agent
	if agent := repo.Agent(ref); agent == nil || agent.ID != ref.ID {
		t.Fatalf("pinned agent %+v of %+v", agent, ref)
	}
	if repo.Agent(&AgentRef{ID: ref.ID, Version: 99}) != nil {
		t.Fatal("a version the repository lacks was found")
	}
	presentation := LibraryRepoAgentPresentation(repo, *repo.Agent(ref))
	if len(presentation.UsedBy) != 1 || presentation.UsedBy[0].BeltID != "team" || presentation.UsedBy[0].Version != ref.Version {
		t.Fatalf("used by %+v", presentation.UsedBy)
	}
	if presentation.Footer == "" || len(presentation.Warnings) != 0 || presentation.IsNew || presentation.Archived {
		t.Fatalf("presentation %+v", presentation)
	}
	for _, target := range presentation.OutcomeTargets {
		if target.ID == ref.ID {
			t.Fatalf("the agent lists itself as an outcome target: %+v", presentation.OutcomeTargets)
		}
	}
}
