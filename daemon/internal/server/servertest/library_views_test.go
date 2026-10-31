package servertest

import (
	"encoding/json"
	"testing"
)

// TestLibraryBeltAndAgentReadsRefuseWhatNoSharedRepositoryHolds: an id the shared repository lacks is
// not found, and a repository that is not a shared one in the settings is refused with its reason.
func TestLibraryBeltAndAgentReadsRefuseWhatNoSharedRepositoryHolds(t *testing.T) {
	t.Parallel()
	f := libraryFixture(t)
	for _, path := range []string{
		"/api/v1/library/belt?repository=example%2Fshared&path=&id=no-such-belt",
		"/api/v1/library/agent?repository=example%2Fshared&path=&id=no-such-agent",
		// The repository holds this id as an agent, not as a belt.
		"/api/v1/library/belt?repository=example%2Fshared&path=&id=team-build",
	} {
		if reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken}); reply.Status != 404 {
			t.Fatalf("%s: status %d, want 404", path, reply.Status)
		}
	}
	for _, path := range []string{
		"/api/v1/library/belt?repository=example%2Fbackup&path=&id=team-nightly",
		"/api/v1/library/agent?repository=example%2Funknown&path=&id=team-build",
	} {
		if reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken}); reply.Status != 422 {
			t.Fatalf("%s: status %d, want 422", path, reply.Status)
		}
	}
}

// TestLibraryBrowserListsTheAgentsTabAndStateFilters: kind=agent lists the shared repository's agents, and a
// state filter with no match lists none while keeping the counts.
func TestLibraryBrowserListsTheAgentsTabAndStateFilters(t *testing.T) {
	t.Parallel()
	f := libraryFixture(t)
	var browser struct {
		Items []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"items"`
		TotalCount int `json:"totalCount"`
	}
	read := func(path string) {
		t.Helper()
		reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken})
		if reply.Status != 200 {
			t.Fatalf("%s: status %d", path, reply.Status)
		}
		if err := json.Unmarshal(reply.Body, &browser); err != nil {
			t.Fatal(err)
		}
	}
	read("/api/v1/library/browser?kind=agent")
	if len(browser.Items) != 3 || browser.Items[0].Kind != "agent" {
		t.Fatalf("agent tab %+v, want the three team agents", browser.Items)
	}
	read("/api/v1/library/browser?kind=agent&filter=installed")
	if len(browser.Items) != 0 || browser.TotalCount != 3 {
		t.Fatalf("installed agents %+v of %d, want none of three", browser.Items, browser.TotalCount)
	}
}
