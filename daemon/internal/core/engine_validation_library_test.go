// Library checks of multi-trigger belts.
// The files are the JSON and Markdown a data folder holds, keyed by path.

package core

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// vtCustomTasksFiles is the agent file and the manual belt that runs it, for the
// library checks: agents/custom/v1.md and belts/tasks/v1.json.
const (
	vtAgentPath = "agents/custom/v1.md"
	vtBeltPath  = "belts/tasks/v1.json"
)

// vtTasksBelt is the manual belt named Tasks on acme/app, running stages.
func vtTasksBelt(stages []Stage) Belt {
	return vtBeltOf("tasks", []string{"acme/app"}, stages, nil)
}

// TestCodexEffortFilesAreAccepted
func TestCodexEffortFilesAreAccepted(t *testing.T) {
	t.Parallel()
	for _, thinking := range []Thinking{ThinkingXHigh, ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			agent := vtAgent("custom")
			belt := vtTasksBelt([]Stage{vtStageFor(agent, nil, nil)})
			agent.Runner = BackendCodex
			agent.Thinking = &thinking
			codex := BackendCodex
			belt.Stages[0].Runner = &codex
			belt.Stages[0].Thinking = &thinking
			changed := map[string]struct{}{vtAgentPath: {}, vtBeltPath: {}}
			explicit := map[string][]byte{vtAgentPath: []byte(AgentFileWrite(agent)), vtBeltPath: vtMarshal(t, belt)}
			if got := LibraryCheckChangedProblems(explicit, changed, nil); len(got) != 0 {
				t.Fatalf("explicit effort problems %v", got)
			}
			belt.Stages[0].Runner = nil
			inherited := map[string][]byte{vtAgentPath: explicit[vtAgentPath], vtBeltPath: vtMarshal(t, belt)}
			if got := LibraryCheckChangedProblems(inherited, changed, nil); len(got) != 0 {
				t.Fatalf("inherited effort problems %v", got)
			}
		})
	}
}

// TestUnrelatedInvalidFilesStillReportTheirOwnErrors
func TestUnrelatedInvalidFilesStillReportTheirOwnErrors(t *testing.T) {
	t.Parallel()
	agent := vtAgent("custom")
	belt := vtTasksBelt([]Stage{vtStageFor(agent, nil, nil)})
	thinkingMax, thinkingXHigh := ThinkingMax, ThinkingXHigh
	codex := BackendCodex
	agent.Runner = BackendCodex
	agent.Thinking = &thinkingMax
	agent.Name = " "
	belt.Stages[0].Runner = &codex
	belt.Stages[0].Thinking = &thinkingXHigh
	belt.Name = " "
	files := map[string][]byte{vtAgentPath: []byte(AgentFileWrite(agent)), vtBeltPath: vtMarshal(t, belt)}
	problems := LibraryCheckChangedProblems(files, map[string]struct{}{vtAgentPath: {}, vtBeltPath: {}}, nil)
	if !slices.Contains(problems, vtAgentPath+": Give the agent a name.") {
		t.Errorf("missing agent name problem in %v", problems)
	}
	if !slices.Contains(problems, vtBeltPath+": Give the belt a name.") {
		t.Errorf("missing belt name problem in %v", problems)
	}
	for _, problem := range problems {
		if strings.Contains(strings.ToLower(problem), "thinking") {
			t.Errorf("unexpected thinking problem %q", problem)
		}
	}
}

// TestPlaceholderDiagnosticsMatchBeltValidation
func TestPlaceholderDiagnosticsMatchBeltValidation(t *testing.T) {
	t.Parallel()
	agent := vtAgent("custom")
	first := vtStageFor(agent, nil, nil)
	first.ID = "writer-one"
	first.Name = "Writer one"
	first.SetKey(new("notes"))
	first.Brief = "Read {missing}."
	second := first
	second.ID = "writer-two"
	second.Name = "Writer two"
	second.Brief = ""
	belt := vtTasksBelt([]Stage{first, second})
	files := map[string][]byte{vtAgentPath: []byte(AgentFileWrite(agent)), vtBeltPath: vtMarshal(t, belt)}
	pins := map[AgentRef]Agent{agent.Ref(): agent}
	var expected []string
	for _, warning := range ValidationWarnings(belt, []string{"notes", "notes"}, pins) {
		expected = append(expected, vtBeltPath+": "+warning)
	}
	checked := LibraryCheckChangedProblems(files, map[string]struct{}{vtBeltPath: {}}, nil)
	if !reflect.DeepEqual(checked, expected) {
		t.Fatalf("checked %v, want %v", checked, expected)
	}
	if !vtAny(checked, func(p string) bool {
		return strings.Contains(p, "{notes} is used by Writer one (main line) and Writer two (main line)")
	}) {
		t.Errorf("missing shared key problem in %v", checked)
	}
	if !vtAny(checked, func(p string) bool { return strings.Contains(p, "{missing} isn't a placeholder") }) {
		t.Errorf("missing placeholder problem in %v", checked)
	}
}

// TestCheckLibraryAcceptsSeveralEntriesAndALegacyFile
func TestCheckLibraryAcceptsSeveralEntriesAndALegacyFile(t *testing.T) {
	t.Parallel()
	multi := vtBeltOfKind(TriggerKindIssues, nil)
	multi.ID = "multi"
	jira := DefaultJiraQuery()
	jira.Jql = "project = PAY"
	multi.Triggers = append(multi.Triggers, TriggerEntry{ID: "2", Trigger: Trigger{Kind: TriggerKindJira, Jira: &jira}, Repos: []string{"acme/billing"}})
	files := map[string][]byte{
		"belts/multi/v1.json": vtMarshal(t, multi),
		"belts/old/v1.json":   vtLegacyBeltJSON(t, vtBeltOfKind(TriggerKindManual, nil)),
	}
	for _, agent := range SampleAgents() {
		files[fmt.Sprintf("agents/%s/v%d.md", agent.ID, agent.Version)] = []byte(AgentFileWrite(agent))
	}
	problems := LibraryCheckProblems(files, nil)
	for _, problem := range problems {
		if strings.HasPrefix(problem, "belts/multi/") {
			t.Errorf("multi-entry belt problem %q", problem)
		}
		if strings.HasPrefix(problem, "belts/old/") && strings.Contains(problem, "trigger") {
			t.Errorf("legacy belt problem %q", problem)
		}
	}
}

// vtMarshal is the JSON of v, failing the test on an error.
func vtMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := jsonx.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return data
}

// vtLegacyBeltJSON is belt written in the shape before several triggers: one
// trigger and its repositories at the top level, and no triggers key.
func vtLegacyBeltJSON(t *testing.T, belt Belt) []byte {
	t.Helper()
	members, err := jsonx.Members(vtMarshal(t, belt))
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	var entries []TriggerEntry
	if err := json.Unmarshal(members["triggers"], &entries); err != nil || len(entries) == 0 {
		t.Fatalf("triggers: %v", err)
	}
	fields := map[string][]byte{
		"trigger": vtMarshal(t, entries[0].Trigger),
		"repos":   vtMarshal(t, entries[0].Repos),
	}
	for key, raw := range members {
		if key != "triggers" {
			fields[key] = []byte(raw)
		}
	}
	keys := slices.Sorted(maps.Keys(fields))
	var out strings.Builder
	out.WriteString("{")
	for index, key := range keys {
		if index > 0 {
			out.WriteString(",")
		}
		fmt.Fprintf(&out, "%q:%s", key, fields[key])
	}
	out.WriteString("}")
	return []byte(out.String())
}
