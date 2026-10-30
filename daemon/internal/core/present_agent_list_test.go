package core

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// agentListTestRecords are the built-in agents as library records, with the ids given archived.
func agentListTestRecords(archived ...string) []AgentRecord {
	gone := map[string]bool{}
	for _, id := range archived {
		gone[id] = true
	}
	records := []AgentRecord{}
	for _, agent := range SampleAgents() {
		records = append(records, AgentRecord{Agent: agent, Archived: gone[agent.ID]})
	}
	return records
}

func TestRowDetailsShowUsageWithoutAnAgentPlaceholder(t *testing.T) {
	t.Parallel()
	agent := SampleAgent("build")
	agent.Key = "legacy-build-name"
	if got, want := AgentListDetail(agent, 1), "v"+strconv.Itoa(agent.Version)+" · 1 belt"; got != want {
		t.Errorf("detail with one belt = %q, want %q", got, want)
	}
	if got, want := AgentListDetail(agent, 0), "v"+strconv.Itoa(agent.Version)+" · unused"; got != want {
		t.Errorf("detail unused = %q, want %q", got, want)
	}
	if got := AgentListDetail(agent, 1); strings.Contains(got, "legacy-build-name") {
		t.Errorf("detail %q names the legacy key", got)
	}
}

// TestAgentChoicesAreTheLiveAgentsInLibraryOrder pins what the belt editor's menu offers: archived
// agents are left out, and the rest keep the library's order with their name and version.
func TestAgentChoicesAreTheLiveAgentsInLibraryOrder(t *testing.T) {
	t.Parallel()
	records := agentListTestRecords("triage", "ship")
	choices := AgentChoices(records)
	want := []AgentChoice{}
	for _, record := range records {
		if !record.Archived {
			want = append(want, AgentChoice{ID: record.Agent.ID, Name: record.Agent.Name, Version: record.Agent.Version})
		}
	}
	if !reflect.DeepEqual(choices, want) {
		t.Fatalf("choices = %+v, want %+v", choices, want)
	}
	for _, choice := range choices {
		if choice.ID == "triage" || choice.ID == "ship" {
			t.Fatalf("archived agent %q offered as a choice", choice.ID)
		}
	}
	if got := AgentChoices([]AgentRecord{}); got == nil || len(got) != 0 {
		t.Fatalf("no records gave %#v, want an empty non-nil list", got)
	}
}
