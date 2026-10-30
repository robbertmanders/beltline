package core

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// builtInShippedVersion is the version a built-in agent ships at: triage, spec and ship are version 2.
func builtInShippedVersion(id string) int {
	switch id {
	case "triage", "spec", "ship":
		return 2
	}
	return 1
}

// mustBuiltInAgent parses a bundled agent at its shipped version.
func mustBuiltInAgent(t *testing.T, id string) Agent {
	t.Helper()
	file, ok := SampleAgentText(id)
	if !ok {
		t.Fatalf("no bundled agent %q", id)
	}
	agent, err := AgentFileParse(file, id, builtInShippedVersion(id))
	if err != nil {
		t.Fatalf("parse %s: %v", id, err)
	}
	return agent
}

// TestBuiltInAgentFileLookup checks that only bundled agent ids are found.
func TestBuiltInAgentFileLookup(t *testing.T) {
	t.Parallel()
	for _, id := range SampleAgentIDs {
		if _, ok := SampleAgentText(id); !ok {
			t.Errorf("built-in %s has no file", id)
		}
	}
	if _, ok := SampleAgentText("missing"); ok {
		t.Fatal("a missing agent is reported as bundled")
	}
	if _, ok := SampleAgentText("../assistant/prompt"); ok {
		t.Fatal("an id that leaves the folder is reported as bundled")
	}
}

// TestBuiltInAgentsParseAndRoundTrip checks that every built-in agent parses
// with the AgentFile port and that writing it again gives the same agent and bytes.
func TestBuiltInAgentsParseAndRoundTrip(t *testing.T) {
	t.Parallel()
	for _, id := range SampleAgentIDs {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			file, ok := SampleAgentText(id)
			if !ok {
				t.Fatalf("no bundled file for %s", id)
			}
			agent, err := AgentFileParse(file, id, builtInShippedVersion(id))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			written := AgentFileWrite(agent)
			again, err := AgentFileParse(written, id, builtInShippedVersion(id))
			if err != nil {
				t.Fatalf("parse written: %v", err)
			}
			if !agentEqual(again, agent) {
				t.Fatalf("re-read agent differs:\n%s", testkit.Diff(agent, again))
			}
			if AgentFileWrite(again) != written {
				t.Fatal("writing the re-read agent changes its bytes")
			}
		})
	}
}
