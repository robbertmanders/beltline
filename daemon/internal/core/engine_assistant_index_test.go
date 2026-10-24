package core

import (
	"strings"
	"testing"
)

// TestAssistantIndexListsEachDefinitionInOneLine checks that the index names each
// definition's newest version with what it is, folds its state file and earlier
// versions into that line, and marks a file it cannot read.
func TestAssistantIndexListsEachDefinitionInOneLine(t *testing.T) {
	t.Parallel()
	agent := "---\nname: Helper\nkey: helper\nworkspace: worktree\nslots: 1\nrunner: pi\ntint: teal\noutcomes:\n  - name: done\n    when: Done.\n    tone: good\n    then: next\n---\nWork.\n"
	belt := `{"id":"tasks","name":"Tasks","version":4,"triggers":[{"id":"1","trigger":{"kind":"manual"},"repos":["o/r"]}],` +
		`"stages":[{"id":"helper","name":"Helper","type":"agent","agent":{"id":"helper","version":3},"outcomes":[]}]}`
	library := map[string][]byte{
		"agents/helper/v3.md": []byte(agent), "agents/helper/v1.md": {}, "agents/helper/v2.md": {},
		"belts/tasks/v4.json": []byte(belt), "belts/tasks/v1.json": {}, "belts/tasks/v3.json": {},
		"belts/tasks/state.json": []byte(`{"paused":true,"archived":true}`),
		"belts/broken/v1.json":   []byte("{"),
		"agents/gone/state.json": []byte(`{"archived":true}`),
	}
	index := assistantIndex(library)
	for _, line := range []string{
		`- belts/tasks/v4.json: "Tasks"; triggers: manual; stages: Helper (helper v3). Paused. Archived. Earlier: v1, v3.`,
		`- agents/helper/v3.md: "Helper"; runner: pi; outcomes: done. Earlier: v1–v2.`,
		`- belts/broken/v1.json: cannot be read.`,
		`- agents/gone/state.json`,
	} {
		if !strings.Contains(index, line+"\n") && !strings.HasSuffix(index, line) {
			t.Errorf("the index lacks %q:\n%s", line, index)
		}
	}
	if strings.Contains(index, "Work.") || strings.Contains(index, "belts/tasks/state.json") {
		t.Errorf("the index carries file contents or a folded state file:\n%s", index)
	}
}
