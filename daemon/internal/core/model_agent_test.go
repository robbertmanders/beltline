package core

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestNewAgentDefaults checks the defaults of Agent.init: version 1, the key
// is the id, read-only, two slots, Pi, grey, no thinking level.
func TestNewAgentDefaults(t *testing.T) {
	t.Parallel()
	agent := NewAgent(AgentArgs{ID: "writer", Name: "Writer"})
	want := Agent{
		ID: "writer", Version: 1, Name: "Writer", Key: "writer",
		Workspace: WorkspaceReadOnly, Slots: 2, Runner: BackendPi,
		Tint: TintGrey,
	}
	if !agentEqual(agent, want) {
		t.Fatalf("NewAgent defaults:\n%s", testkit.Diff(want, agent))
	}
	if agent.Ref() != (AgentRef{ID: "writer", Version: 1}) {
		t.Fatalf("ref = %+v", agent.Ref())
	}
	if agent.ChangesCode() {
		t.Fatal("a read-only agent changes code")
	}
}

// TestNewAgentTakesGivenValues checks that every pointer argument overrides its default.
func TestNewAgentTakesGivenValues(t *testing.T) {
	t.Parallel()
	workspace := WorkspaceWorktree
	slots := 5
	runner := BackendClaude
	tint := TintRose
	icon := StageIcon("wrench")
	agent := NewAgent(AgentArgs{
		ID: "build", Version: new(4), Name: "Build", Key: new("evidence"), Workspace: &workspace,
		Slots: &slots, Runner: &runner, Model: "m", Tint: &tint, Icon: &icon, Brief: "Build.",
	})
	if agent.Version != 4 || agent.Key != "evidence" || agent.Slots != 5 || agent.Runner != BackendClaude {
		t.Fatalf("given values not taken: %+v", agent)
	}
	if agent.Tint != TintRose || agent.Model != "m" || agent.Brief != "Build." {
		t.Fatalf("given values not taken: %+v", agent)
	}
	if agent.Icon == nil || *agent.Icon != icon || !agent.ChangesCode() {
		t.Fatalf("icon or workspace not taken: %+v", agent)
	}
}

// TestAgentSameContentIgnoresOnlyTheVersion checks Agent.sameContent(as:): an
// edit that changes nothing but the version is the same content.
func TestAgentSameContentIgnoresOnlyTheVersion(t *testing.T) {
	t.Parallel()
	agent := mustBuiltInAgent(t, "build")
	newer := agent
	newer.Version = 9
	if !agent.SameContent(newer) {
		t.Fatal("a new version of the same content is a change")
	}
	edited := agent
	edited.Brief = "Build it differently."
	if agent.SameContent(edited) {
		t.Fatal("an edited brief is the same content")
	}
	renamed := agent
	renamed.ID = "other"
	if agent.SameContent(renamed) {
		t.Fatal("a different id is the same content")
	}
}

// TestAgentSnapshotKeepsTheFile checks that a snapshot keeps the agent's file,
// and that it reads back to the agent it was taken from.
func TestAgentSnapshotKeepsTheFile(t *testing.T) {
	t.Parallel()
	agent := mustBuiltInAgent(t, "verify")
	snapshot := NewAgentSnapshot(agent)
	if snapshot.File != AgentFileWrite(agent) {
		t.Fatal("the snapshot's file is not AgentFileWrite of the agent")
	}
	if snapshot.Ref() != agent.Ref() {
		t.Fatalf("snapshot ref = %+v, want %+v", snapshot.Ref(), agent.Ref())
	}
	read := snapshot.Agent()
	if read == nil || !agentEqual(*read, agent) {
		t.Fatalf("snapshot agent differs:\n%s", testkit.Diff(agent, read))
	}
	broken := AgentSnapshot{ID: "x", Version: 1, File: "no frontmatter"}
	if broken.Agent() != nil {
		t.Fatal("a snapshot that does not parse returns an agent")
	}
}

// TestAgentDecodesStrictKeys checks the agent's required keys and its enums.
// TestAgentOutcomeDecodesStrictKeys checks an agent outcome's required keys, which
// include its to list.
// TestAgentRefDecodesStrictKeys checks that an agent reference needs its id and version.
// TestAgentSnapshotDecodesStrictKeys checks an agent snapshot's required keys.
