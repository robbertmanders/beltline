// Tests of the library assistant's pure parts: the conversation's name, what the library check
// reports, and the prompt.

package coordinator

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// assistantHelper is the agent the assistant tests add a belt around.
func assistantHelper(version int) core.Agent {
	agent := *core.SampleAgentNamed("custom", nil)
	agent.ID = "helper"
	agent.Version = version
	agent.Name = "Helper"
	agent.Key = "helper"
	agent.Brief = "Do the requested work. Write to {output}."
	return agent
}

// assistantBelt is the belt the assistant tests save: one stage on the helper agent.
func assistantBelt(t *testing.T, version int) core.Belt {
	t.Helper()
	helper := assistantHelper(1)
	stage := core.DefaultsStageForAgent(helper, nil, nil, nil, core.TriggerKindIssues)
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "tasks", Name: "Tasks", Version: &version, Trigger: core.Trigger{Kind: core.TriggerKindManual},
		Repos: []string{"acme/web"}, Stages: []core.Stage{stage},
	})
}

// encodeFor is the file's bytes for value, as the library writes them.
func encodeFor(t *testing.T, value any) []byte {
	t.Helper()
	data, err := jsonx.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return data
}

func TestEachDataFolderHasItsOwnConversation(t *testing.T) {
	t.Parallel()
	a := libraryAssistantSession("/tmp/a")
	if a != libraryAssistantSession("/tmp/a") {
		t.Errorf("the same folder names different conversations: %q", a)
	}
	if a == libraryAssistantSession("/tmp/b") {
		t.Errorf("two folders share the conversation %q", a)
	}
	if !strings.HasPrefix(a, "bl-assistant-") {
		t.Errorf("session = %q", a)
	}
}

func TestProblemsInFilesItDidNotTouchAreNotItsOwn(t *testing.T) {
	t.Parallel()
	agent := assistantHelper(1)
	files := map[string][]byte{
		"belts/old/v1.json":   []byte(`{"id":"old"}`),
		"agents/helper/v1.md": []byte(core.AgentFileWrite(agent)),
	}
	if problems := core.LibraryCheckChangedProblems(files, map[string]struct{}{"agents/helper/v1.md": {}}, nil); len(problems) != 0 {
		t.Errorf("the agent's own file reports %v", problems)
	}
	problems := core.LibraryCheckChangedProblems(files, map[string]struct{}{"belts/old/v1.json": {}}, nil)
	if len(problems) == 0 || !strings.HasPrefix(problems[0], "belts/old/v1.json:") {
		t.Errorf("the broken belt reports %v", problems)
	}
}

func TestASaveTheEditorWouldRefuseIsAProblem(t *testing.T) {
	t.Parallel()
	belt := assistantBelt(t, 1)
	belt.Name = ""
	files := map[string][]byte{
		"agents/helper/v1.md": []byte(core.AgentFileWrite(assistantHelper(1))),
		"belts/tasks/v1.json": encodeFor(t, belt),
	}
	problems := core.LibraryCheckChangedProblems(files, map[string]struct{}{"belts/tasks/v1.json": {}}, nil)
	found := slices.ContainsFunc(problems, func(problem string) bool {
		return strings.HasPrefix(problem, "belts/tasks/v1.json: ")
	})
	if !found {
		t.Errorf("problems = %v", problems)
	}
}

func TestThePromptIndexesTheLatestVersionsAndHasAnExampleThatDecodes(t *testing.T) {
	t.Parallel()
	first, second := assistantHelper(1), assistantHelper(2)
	first.Brief, second.Brief = "first helper brief", "second helper brief"
	second.Name = "Second helper"
	library := map[string][]byte{
		"agents/helper/v1.md":    []byte(core.AgentFileWrite(first)),
		"agents/helper/v2.md":    []byte(core.AgentFileWrite(second)),
		"belts/tasks/v1.json":    encodeFor(t, assistantBelt(t, 1)),
		"belts/tasks/state.json": []byte(`{"paused":true,"lastFired":"2026-01-01T00:00:00Z"}`),
	}
	prompt := core.AssistantPromptPrompt(library, "Rename it", nil, nil)
	for _, text := range []string{"- agents/helper/v2.md: \"Second helper\"", "Earlier: v1.", "- belts/tasks/v1.json: ", "(helper v1)", "Paused.", "Rename it", "never rewrite, move or delete one"} {
		if !strings.Contains(prompt, text) {
			t.Errorf("the prompt lacks %q", text)
		}
	}
	for _, text := range []string{"first helper brief", "second helper brief", "2026-01-01T00:00:00Z"} {
		if strings.Contains(prompt, text) {
			t.Errorf("the prompt carries %q from a file", text)
		}
	}
	const marker = "Example belts/tasks/v1.json:\n```json\n"
	index := strings.LastIndex(prompt, marker)
	if index < 0 {
		t.Fatal("the prompt has no example belt")
	}
	example := prompt[index+len(marker):]
	if end := strings.Index(example, "\n```"); end >= 0 {
		example = example[:end]
	}
	var belt core.Belt
	if err := json.Unmarshal([]byte(example), &belt); err != nil {
		t.Fatalf("the example does not decode: %v", err)
	}
	if len(belt.Stages) == 0 || belt.Stages[0].Agent == nil || *belt.Stages[0].Agent != (core.AgentRef{ID: "helper", Version: 1}) {
		t.Errorf("the example's first stage = %+v", belt.Stages)
	}
}
