package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// TestALocalRepositoryRefusesAPullRequestShare checks that a shared repository on a local remote, as
// demo mode's are, refuses a share by pull request before any git or gh runs: the refusal names the
// fix, and no clone is made.
func TestALocalRepositoryRefusesAPullRequestShare(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.git")
	h := newHarness(t, harnessOptions{LibraryURL: func(string) string { return missing }})
	c := h.Coord()
	shared := core.LibraryOrigin{Repository: "owner/shared"}
	if _, err := c.ApplySettingsPatch(SettingsPatch{LibraryRepositories: &[]core.LibraryRepository{
		{Repository: shared.Repository, Role: core.LibraryRoleShared, Publish: core.LibraryPublishPullRequest},
	}}); err != nil {
		t.Fatalf("settings: %v", err)
	}

	want := "Pull requests aren't available for a local repository; set it to push."
	if _, err := c.ShareLibraryItem(context.Background(), shared, "belt", "nightly"); err == nil || err.Error() != want {
		t.Fatalf("share by pull request to a local remote: err %v, want %q", err, want)
	}
	if _, err := os.Stat(c.libraryRepos.Dir(shared.Repository)); !os.IsNotExist(err) {
		t.Fatalf("the refused share still made a clone: %v", err)
	}
}

// TestTheExamplesRepositoryRefusesEveryShare checks that a share to the examples repository is refused
// before any git or gh runs, whatever its entry's publish setting says, and that listing it alone does not
// offer Share.
func TestTheExamplesRepositoryRefusesEveryShare(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.git")
	h := newHarness(t, harnessOptions{LibraryURL: func(string) string { return missing }})
	c := h.Coord()
	examples := core.LibraryExamples()
	examples.Publish = core.LibraryPublishPush
	if _, err := c.ApplySettingsPatch(SettingsPatch{LibraryRepositories: &[]core.LibraryRepository{examples}}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := c.ShareLibraryItem(context.Background(), examples.Origin(), "agent", "triage"); err == nil || err.Error() != core.LibraryShareRefusedExamples {
		t.Fatalf("share to the examples repository: err %v, want %q", err, core.LibraryShareRefusedExamples)
	}
	if _, err := os.Stat(c.libraryRepos.Dir(examples.Repository)); !os.IsNotExist(err) {
		t.Fatalf("the refused share still made a clone: %v", err)
	}
	h.Turn(func() {
		if c.hasSharedLibraryRepository() {
			t.Error("the examples repository alone offers Share")
		}
	})
}
