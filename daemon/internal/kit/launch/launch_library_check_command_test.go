package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// brokenLibrary is a folder whose agent and belt both have no name, in a path with a space in it.
func brokenLibrary(t *testing.T) string {
	t.Helper()
	folder := filepath.Join(t.TempDir(), "check broken")
	agent := core.SampleAgentNamed("custom", nil)
	if agent == nil {
		t.Fatal("the custom agent is missing")
	}
	agent.Name = " "
	stage := core.DefaultsStageForAgent(*agent, nil, nil, nil, core.TriggerKindIssues)
	stage.Runner = nil
	belt := core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "tasks", Name: " ", Trigger: core.Trigger{Kind: core.TriggerKindManual},
		Repos: []string{"acme/app"}, Stages: []core.Stage{stage},
	})
	data, err := jsonx.Marshal(belt)
	if err != nil {
		t.Fatalf("encode belt: %v", err)
	}
	writeLibraryFile(t, folder, "agents/custom/v1.md", core.AgentFileWrite(*agent))
	writeLibraryFile(t, folder, "belts/tasks/v1.json", string(data))
	return folder
}

// writeLibraryFile writes text to relative path under folder, making its folders.
func writeLibraryFile(t *testing.T, folder, relative, text string) {
	t.Helper()
	path := filepath.Join(folder, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestACleanLibraryHasNoProblems(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	result := LibraryCheckCommandRun([]string{"Beltline", LibraryCheckCommandFlag, folder}, nil)
	if result == nil || *result != (LibraryCheckCommandResult{Output: "No problems.\n", Status: 0}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestABrokenLibraryListsItsProblemsInOrder(t *testing.T) {
	t.Parallel()
	folder := brokenLibrary(t)
	problems := coordinator.LibraryAssistantCheck(store.Paths{Root: folder})
	if len(problems) < 2 {
		t.Fatalf("problems = %v", problems)
	}
	for _, want := range []string{"agents/custom/v1.md: Give the agent a name.", "belts/tasks/v1.json: Give the belt a name."} {
		if !slices.Contains(problems, want) {
			t.Errorf("problems %v lack %q", problems, want)
		}
	}
	if agentAt, beltAt := indexPrefix(problems, "agents/"), indexPrefix(problems, "belts/"); agentAt < 0 || beltAt < 0 || agentAt >= beltAt {
		t.Errorf("agents at %d, belts at %d", agentAt, beltAt)
	}
	result := LibraryCheckCommandCheck(store.Paths{Root: folder})
	want := LibraryCheckCommandResult{Output: strings.Join(problems, "\n") + "\n", Status: 1}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("check = %+v, want %+v", result, want)
	}
}

func TestAnExplicitFolderWinsAndNoFlagMeansNoCheck(t *testing.T) {
	t.Parallel()
	broken := brokenLibrary(t)
	clean := t.TempDir()
	if LibraryCheckCommandRun([]string{"Beltline", "--demo"}, nil) != nil {
		t.Fatal("a run without the flag checked the library")
	}
	standard := func() store.Paths { return store.Paths{Root: clean} }
	explicit := LibraryCheckCommandRun([]string{"Beltline", LibraryCheckCommandFlag, broken}, standard)
	if explicit == nil || explicit.Status != 1 {
		t.Errorf("explicit = %+v", explicit)
	}
	fallback := LibraryCheckCommandRun([]string{"Beltline", LibraryCheckCommandFlag}, standard)
	if fallback == nil || *fallback != (LibraryCheckCommandResult{Output: "No problems.\n", Status: 0}) {
		t.Errorf("standard = %+v", fallback)
	}
}

func TestCheckingLeavesNoLockOrFiles(t *testing.T) {
	t.Parallel()
	folder := brokenLibrary(t)
	before := snapshotFolder(t, folder)
	_ = LibraryCheckCommandCheck(store.Paths{Root: folder})
	if after := snapshotFolder(t, folder); !reflect.DeepEqual(after, before) {
		t.Error("the check changed the folder")
	}
	if _, err := os.Stat(OwnershipLockFile(folder)); err == nil {
		t.Error("the check left a lock file")
	}
}

// indexPrefix is the first index of a value starting with prefix, or -1.
func indexPrefix(values []string, prefix string) int {
	return slices.IndexFunc(values, func(value string) bool { return strings.HasPrefix(value, prefix) })
}
