// Repeated reads and error answers leave the
// data folder and the coordinator's state unchanged.
package servertest

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// folderEntry is one entry of a data folder manifest: a directory, a symlink (its target) or a file (its bytes).
type folderEntry struct {
	kind   string
	target string
	data   []byte
}

// folderManifest lists every entry under root by its path relative to root.
func folderManifest(t *testing.T, root string) map[string]folderEntry {
	t.Helper()
	found := map[string]folderEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			found[relative] = folderEntry{kind: "symlink", target: target}
		case entry.IsDir():
			found[relative] = folderEntry{kind: "directory"}
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			found[relative] = folderEntry{kind: "file", data: data}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("manifest of %s: %v", root, err)
	}
	return found
}

// manifestsEqual reports whether two manifests hold the same entries.
func manifestsEqual(a, b map[string]folderEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for key, entry := range a {
		other, ok := b[key]
		if !ok || other.kind != entry.kind || other.target != entry.target || !bytes.Equal(other.data, entry.data) {
			return false
		}
	}
	return true
}

// TestRepeatedReadAndErrorMatricesLeaveDiskAndCoordinatorStateUnchanged checks that repeated reads and errors leave disk and coordinator state unchanged.
func TestRepeatedReadAndErrorMatricesLeaveDiskAndCoordinatorStateUnchanged(t *testing.T) {
	t.Parallel()
	f := Start(t, Options{})
	f.SeedViews()
	f.SyncLibrary(t)
	disk := folderManifest(t, f.Root)
	c := f.Coordinator
	runsBefore := c.ReadRuns()
	beltsBefore := c.ReadBelts()
	agentsBefore := c.ReadAgents()
	settingsBefore := c.ReadSettings()
	waitingBefore := c.ReadWaitingSubjects()
	decisionsBefore := c.ReadAfkDecisions()
	noticesBefore := c.ReadStateSnapshot(nil, nil).Notices

	for round := 0; round < 2; round++ {
		exerciseReadMatrix(t, f)

		for _, query := range []string{
			"/api/v1/views/work?filter=all&groupBy=repo&search=%23139",
			"/api/v1/views/work?filter=you&belt=issues&stage=contract",
			"/api/v1/views/work?filter=&groupBy=&belt=&stage=&search=",
			"/api/v1/views/work?filter=shipped&groupBy=status",
			"/api/v1/views/work?groupBy=epic",
			"/api/v1/views/work?filter=active&groupBy=belt",
			"/api/v1/views/work?belt=no-such-belt&stage=no-such-stage",
			"/api/v1/views/work?search=no-such-run",
			"/api/v1/waiting",
			"/api/v1/views/overview",
			"/api/v1/afk/decisions",
			"/api/v1/views/insights?period=7d",
			"/api/v1/views/insights?belt=&period=",
			"/api/v1/views/insights?belt=issues&version=1&period=30d",
		} {
			if reply := f.Do(Request{Method: "GET", Path: query, Token: ContractToken}); reply.Status != 200 {
				t.Fatalf("%s: status %d", query, reply.Status)
			}
		}
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/work?filter=bogus", Token: ContractToken}); reply.Status != 422 {
			t.Fatalf("invalid filter: status %d, want 422", reply.Status)
		}
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights?period=1y", Token: ContractToken}); reply.Status != 422 {
			t.Fatalf("invalid insights period: status %d, want 422", reply.Status)
		}
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights?belt=no-such-belt", Token: ContractToken}); reply.Status != 404 {
			t.Fatalf("unknown insights belt: status %d, want 404", reply.Status)
		}
		for _, query := range []string{"version=0", "version=two"} {
			if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights?belt=issues&" + query, Token: ContractToken}); reply.Status != 422 {
				t.Fatalf("insights %s: status %d, want 422", query, reply.Status)
			}
		}
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/insights?belt=issues&version=999", Token: ContractToken}); reply.Status != 404 {
			t.Fatalf("unknown insights version: status %d, want 404", reply.Status)
		}

		runID := f.ActionRunID
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/" + runID, Token: ContractToken}); reply.Status != 200 {
			t.Fatalf("run view: status %d", reply.Status)
		}
		if reply := f.Do(Request{Method: "GET", Path: "/api/v1/views/overview", Token: ContractToken}); reply.Status != 200 {
			t.Fatalf("overview: status %d", reply.Status)
		}
		unknown := f.Do(Request{Method: "GET", Path: "/api/v1/views/run/no-such-run", Token: ContractToken})
		if unknown.Status != 404 || !bytes.Equal(unknown.Body, goldenBytes(t, "error-not-found.json")) {
			t.Fatalf("unknown run view: status %d", unknown.Status)
		}
		for _, suffix := range []string{"/output", "/artifacts"} {
			path := "/api/v1/runs/" + EncodeSegment(runID) + "/attempts/not-recorded" + suffix
			reply := f.Do(Request{Method: "GET", Path: path, Token: ContractToken})
			if reply.Status != 404 || !bytes.Equal(reply.Body, goldenBytes(t, "error-not-found.json")) {
				t.Fatalf("%s: status %d, want the not-found golden", path, reply.Status)
			}
		}

		if !manifestsEqual(folderManifest(t, f.Root), disk) {
			t.Fatalf("round %d: the data folder changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadRuns(), runsBefore) {
			t.Fatalf("round %d: runs changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadBelts(), beltsBefore) {
			t.Fatalf("round %d: belts changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadAgents(), agentsBefore) {
			t.Fatalf("round %d: agents changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadSettings(), settingsBefore) {
			t.Fatalf("round %d: settings changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadWaitingSubjects(), waitingBefore) {
			t.Fatalf("round %d: waiting changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadAfkDecisions(), decisionsBefore) {
			t.Fatalf("round %d: decisions changed after repeated reads", round)
		}
		if !reflect.DeepEqual(c.ReadStateSnapshot(nil, nil).Notices, noticesBefore) {
			t.Fatalf("round %d: notices changed after repeated reads", round)
		}
		if n := len(f.GitHub.Calls()); n != 0 {
			t.Fatalf("round %d: %d GitHub calls, want none", round, n)
		}
		if n := len(f.Jira.Calls()); n != 0 {
			t.Fatalf("round %d: %d Jira calls, want none", round, n)
		}
		if n := len(f.Jira.Searches()); n != 0 {
			t.Fatalf("round %d: %d Jira searches, want none", round, n)
		}
		if trail := f.Runner.Trail(); len(trail) != 0 {
			t.Fatalf("round %d: the runner trail %v, want none", round, trail)
		}
	}
}
