package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// TestEveryBuiltInAgentRoundTrips
func TestEveryBuiltInAgentRoundTrips(t *testing.T) {
	t.Parallel()
	for _, id := range DefaultsBuiltInIDs {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			agent := mustBuiltInAgent(t, id)
			parsed, err := AgentFileParse(AgentFileWrite(agent), id, agent.Version)
			if err != nil {
				t.Fatal(err)
			}
			if !agentEqual(parsed, agent) {
				t.Fatalf("round trip changed the agent:\n%s", testkit.Diff(agent, parsed))
			}
		})
	}
}

// TestBuiltInFilesAreWrittenCanonically checks that each bundled file is exactly
// what AgentFileWrite writes for it, so an agent saved by the app reads back to the same bytes.
func TestBuiltInFilesAreWrittenCanonically(t *testing.T) {
	t.Parallel()
	for _, id := range DefaultsBuiltInIDs {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			file, _ := BuiltInAgentFile(id)
			agent := mustBuiltInAgent(t, id)
			if got := AgentFileWrite(agent); got != file {
				t.Fatalf("written file differs from the bundled one:\n%s", testkit.Diff(file, got))
			}
		})
	}
}

// TestThinkingChoicesStartAtTheRunnersDefault
func TestThinkingChoicesStartAtTheRunnersDefault(t *testing.T) {
	t.Parallel()
	var labels, raws []string
	for _, level := range AllThinking {
		labels = append(labels, level.Label())
		raws = append(raws, string(level))
	}
	if got, want := strings.Join(labels, ","), "Low,Medium,High,Extra high,Max"; got != want {
		t.Fatalf("labels = %s, want %s", got, want)
	}
	if got, want := strings.Join(raws, ","), "low,medium,high,xhigh,max"; got != want {
		t.Fatalf("raw values = %s, want %s", got, want)
	}
	if got := NewAgent(AgentArgs{ID: "x", Name: "X"}); got.Thinking != nil {
		t.Fatal("a new agent has a thinking level")
	}
	if mustBuiltInAgent(t, "build").Thinking != nil {
		t.Fatal("the build agent sets a thinking level")
	}
	for _, id := range DefaultsBuiltInIDs {
		if agent := mustBuiltInAgent(t, id); agent.Thinking != nil {
			t.Fatalf("built-in %s sets a thinking level", id)
		}
	}
}

// TestThinkingRoundTripsThroughFrontmatter
func TestThinkingRoundTripsThroughFrontmatter(t *testing.T) {
	t.Parallel()
	for _, level := range AllThinking {
		t.Run(string(level), func(t *testing.T) {
			t.Parallel()
			text := "---\nname: X\nthinking: " + string(level) + "\n---\nBrief"
			agent, err := AgentFileParse(text, "x", 1)
			if err != nil {
				t.Fatal(err)
			}
			if agent.Thinking == nil || *agent.Thinking != level {
				t.Fatalf("thinking = %v, want %s", agent.Thinking, level)
			}
			written := AgentFileWrite(agent)
			if !strings.Contains("\n"+written, "\nthinking: "+string(level)+"\n") {
				t.Fatalf("written file lacks the thinking line:\n%s", written)
			}
			again, err := AgentFileParse(written, "x", 1)
			if err != nil {
				t.Fatal(err)
			}
			if !agentEqual(again, agent) {
				t.Fatalf("round trip changed the agent:\n%s", testkit.Diff(agent, again))
			}
		})
	}
}

// TestUnsetThinkingHasNoFrontmatterLine
func TestUnsetThinkingHasNoFrontmatterLine(t *testing.T) {
	t.Parallel()
	agent, err := AgentFileParse("---\nname: X\n---\nBrief", "x", 1)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Thinking != nil {
		t.Fatal("thinking should be unset")
	}
	if strings.Contains(AgentFileWrite(agent), "thinking:") {
		t.Fatal("an unset thinking level is written")
	}
}

// TestUnknownThinkingNamesTheChoices
func TestUnknownThinkingNamesTheChoices(t *testing.T) {
	t.Parallel()
	_, err := AgentFileParse("---\nname: X\nthinking: extreme\n---\nBrief", "x", 1)
	var problem AgentFileProblem
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want an AgentFileProblem", err)
	}
	if !strings.Contains(problem.Message, "low, medium, high, xhigh, max") {
		t.Fatalf("message %q does not name the choices", problem.Message)
	}
}

// TestTheBuiltInsMatchWhatTheAppDidBefore checks that the built-in agents match the ones shipped before.
func TestTheBuiltInsMatchWhatTheAppDidBefore(t *testing.T) {
	t.Parallel()
	verify := mustBuiltInAgent(t, "verify")
	if verify.Key != "verdict" {
		t.Fatalf("verify key = %q", verify.Key)
	}
	if verify.Workspace != WorkspaceWorktreeReadOnly {
		t.Fatalf("verify workspace = %q", verify.Workspace)
	}
	if got := verify.Outcomes[1].To; len(got) != 1 || got[0] != "build" {
		t.Fatalf("verify goto = %v", got)
	}
	want := Then{Action: ThenActionGoto, WithOutput: true, Max: new(1)}
	if !agentEqual(verify.Outcomes[1].Then, want) {
		t.Fatalf("verify then = %+v, want %+v", verify.Outcomes[1].Then, want)
	}
	if mustBuiltInAgent(t, "build").Slots != 1 {
		t.Fatal("build slots should be 1")
	}
	if !mustBuiltInAgent(t, "triage").Outcomes[2].Then.Comment {
		t.Fatal("triage's third outcome should comment")
	}
	if !strings.HasPrefix(mustBuiltInAgent(t, "spec").Brief, "You are the spec agent.") {
		t.Fatal("spec brief does not start as the app shipped it")
	}
}

// TestReadsHandEditedFiles
func TestReadsHandEditedFiles(t *testing.T) {
	t.Parallel()
	text := `---
# A hand-written agent.
name: 'Security review'
key: security   # used as {security}
writes: "security: notes.md"
workspace: worktreeReadOnly
machine: reader
tint: rose
outcomes:
  -
    name: clear
    tone: good
  - name: findings
    when: "Something must change: say what."
    tone: problem
    then: goto
    to: [build, spec]
    withOutput: yes
---

Look for {security}.`
	agent, err := AgentFileParse(text, "security", 3)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Security review" || agent.Key != "security" {
		t.Fatalf("name, key = %q, %q", agent.Name, agent.Key)
	}
	// A file name of its own, from before it followed the key, is ignored.
	if strings.Contains(AgentFileWrite(agent), "writes:") {
		t.Fatal("a writes line is written")
	}
	// A machine, from before agents had icons, gives the icon it drew as, and is not written back.
	if agent.Icon == nil || *agent.Icon != StageIconEye {
		t.Fatalf("icon = %v, want the reader's eye", agent.Icon)
	}
	if file := AgentFileWrite(agent); strings.Contains(file, "machine:") || !strings.Contains(file, "icon: eye") {
		t.Fatalf("written file keeps the machine or loses the icon:\n%s", file)
	}
	withIcon, err := AgentFileParse(strings.Replace(text, "machine: reader", "machine: reader\nicon: flag", 1), "security", 3)
	if err != nil || withIcon.Icon == nil || *withIcon.Icon != StageIconFlag {
		t.Fatalf("an icon beside a machine = %v (%v), want flag", withIcon.Icon, err)
	}
	if agent.Version != 3 {
		t.Fatalf("version = %d", agent.Version)
	}
	if len(agent.Outcomes) != 2 || agent.Outcomes[0].Name != "clear" || agent.Outcomes[1].Name != "findings" {
		t.Fatalf("outcomes = %+v", agent.Outcomes)
	}
	if agent.Outcomes[0].Then.Action != ThenActionNext {
		t.Fatalf("clear then = %q", agent.Outcomes[0].Then.Action)
	}
	if agent.Outcomes[1].When != "Something must change: say what." {
		t.Fatalf("findings when = %q", agent.Outcomes[1].When)
	}
	if got := agent.Outcomes[1].To; len(got) != 2 || got[0] != "build" || got[1] != "spec" {
		t.Fatalf("findings to = %v", got)
	}
	if !agent.Outcomes[1].Then.WithOutput {
		t.Fatal("findings should pass its output")
	}
	if agent.Brief != "Look for {security}." {
		t.Fatalf("brief = %q", agent.Brief)
	}
}

// TestExplainsWhatItCantRead
func TestExplainsWhatItCantRead(t *testing.T) {
	t.Parallel()
	_, err := AgentFileParse("No frontmatter", "x", 1)
	var problem AgentFileProblem
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want an AgentFileProblem", err)
	}
	_, err = AgentFileParse("---\nname: X\nworkspace: anywhere\n---\nBrief", "x", 1)
	if err == nil || !strings.Contains(err.Error(), "readOnly, worktree, worktreeReadOnly") {
		t.Fatalf("error = %v, want the workspace choices", err)
	}
}

// TestQuotesOnlyWhatNeedsIt
func TestQuotesOnlyWhatNeedsIt(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"ready to spec":  "ready to spec",
		"a: b":           `"a: b"`,
		"yes":            `"yes"`,
		`"quoted"`:       `"\"quoted\""`,
		"":               `""`,
		"1.5":            `"1.5"`,
		"nan":            `"nan"`,
		"0x1.8":          `"0x1.8"`,
		"-flag":          `"-flag"`,
		"ends with #":    `"ends with #"`,
		"a # b":          `"a # b"`,
		"back\\slash":    "back\\slash",
		" padded":        `" padded"`,
		"multi\nline":    "multi line",
		"ends:":          `"ends:"`,
		"claude[1m]":     "claude[1m]",
		"[1m]":           `"[1m]"`,
		"openrouter/x:y": "openrouter/x:y",
	}
	for in, want := range cases {
		if got := agentFileQuote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestAgentFileReadsQuotingAndLists checks the reader's quoting, comment and list
// rules that the writer's output depends on.
func TestAgentFileReadsQuotingAndLists(t *testing.T) {
	t.Parallel()
	text := `---
name: "Quoted \"name\" \n here" # trailing comment
key: 'it''s'
model: openrouter/x # a comment
slots: +3
outcomes:
  - name: "a: b"
    when: 'single'
    tone: you
    then: goto
    to: build, , 'spec'
    max: 2
    comment: NO
---
`
	agent, err := AgentFileParse(text, "x", 1)
	if err != nil {
		t.Fatal(err)
	}
	// A bare value ends at a comment, so the quotes stay: the value is `"Quoted \"name\" \n here"`.
	if want := `"Quoted \"name\" \n here"`; agent.Name != want {
		t.Fatalf("name = %q, want %q", agent.Name, want)
	}
	if agent.Key != "it's" {
		t.Fatalf("key = %q", agent.Key)
	}
	if agent.Model != "openrouter/x" {
		t.Fatalf("model = %q", agent.Model)
	}
	if agent.Slots != 3 {
		t.Fatalf("slots = %d", agent.Slots)
	}
	outcome := agent.Outcomes[0]
	if outcome.Name != "a: b" || outcome.When != "single" || outcome.Tone != ToneYou {
		t.Fatalf("outcome = %+v", outcome)
	}
	if got := outcome.To; len(got) != 2 || got[0] != "build" || got[1] != "spec" {
		t.Fatalf("to = %v", got)
	}
	if outcome.Then.Max == nil || *outcome.Then.Max != 2 || outcome.Then.Comment {
		t.Fatalf("then = %+v", outcome.Then)
	}
}

// TestAgentFileRejectsMalformedLines checks the reader's problems for stray and malformed lines.
func TestAgentFileRejectsMalformedLines(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"---\nname: X\n  stray: 1\n---\n":                           "is indented but isn't part of the outcomes or alternates",
		"---\nname: X\noutcomes:\n  tone: good\n---\n":              "comes before the first outcome's “- name:”.",
		"---\nname: X\nalternates:\n  model: m\n---\n":              "comes before the first alternate's “- runner:”.",
		"---\nname: X\noutcomes:\n  - when: x\n---\n":               "An outcome has no name.",
		"---\nname X\n---\n":                                        "isn't a “key: value” line.",
		"---\nslots: many\n---\n":                                   "slots must be a number, not “many”.",
		"---\noutcomes:\n  - name: a\n    withOutput: maybe\n---\n": "withOutput must be true or false, not “maybe”.",
		"---\nname: X\n":                                            "doesn't start with a --- frontmatter block",
	}
	for text, want := range cases {
		_, err := AgentFileParse(text, "x", 1)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parse(%q) error = %v, want it to contain %q", text, err, want)
		}
	}
}

// TestAgentFileWriteRoundTripsAwkwardValues checks that a written file with
// quoted values reads back to the same agent and writes the same bytes.
func TestAgentFileWriteRoundTripsAwkwardValues(t *testing.T) {
	t.Parallel()
	thinking := ThinkingMax
	icon := StageIcon("wrench")
	runner := BackendCodex
	piRunner := BackendPi
	limit := 4
	agent := NewAgent(AgentArgs{
		ID:       "awkward",
		Name:     "Name: with \"quotes\"",
		Key:      new("yes"),
		Model:    "claude-opus-5-5[1m]",
		Thinking: &thinking,
		Alternates: []SessionSettings{
			{Runner: &runner, Model: "gpt # 5", Thinking: &thinking},
			{Runner: &piRunner, Model: ""},
		},
		Icon: &icon,
		Outcomes: []AgentOutcome{{
			Name: "again", When: "Try: once more", Tone: ToneProblem,
			Then: Then{Action: ThenActionGoto, WithOutput: true, Max: &limit, Comment: true},
			To:   []string{"build", "spec: v2"},
		}},
		Brief: "Do the thing.",
	})
	written := AgentFileWrite(agent)
	parsed, err := AgentFileParse(written, "awkward", 1)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, written)
	}
	if !agentEqual(parsed, agent) {
		t.Fatalf("round trip changed the agent:\n%s", testkit.Diff(agent, parsed))
	}
	if again := AgentFileWrite(parsed); again != written {
		t.Fatalf("second write differs:\n%s", testkit.Diff(written, again))
	}
}

// TestShipAndDoneRoundTripThroughFrontmatter checks that both finishing actions are read and
// written by an agent's outcomes, and that Ship keeps its own name.
func TestShipAndDoneRoundTripThroughFrontmatter(t *testing.T) {
	t.Parallel()
	agent := mustBuiltInAgent(t, "wrap-up")
	agent.Outcomes = append(agent.Outcomes,
		AgentOutcome{Name: "finished", When: "It is done.", Tone: ToneGood, Then: NewThen(ThenArgs{Action: ThenActionDone})},
		AgentOutcome{Name: "shipped", When: "It shipped.", Tone: ToneNeutral, Then: NewThen(ThenArgs{Action: ThenActionShip})},
	)
	text := AgentFileWrite(agent)
	if !strings.Contains(text, "then: done") || !strings.Contains(text, "then: ship") {
		t.Fatalf("frontmatter lacks the finishing actions:\n%s", text)
	}
	parsed, err := AgentFileParse(text, agent.ID, agent.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !agentEqual(parsed, agent) {
		t.Fatalf("round trip changed the agent:\n%s", testkit.Diff(agent, parsed))
	}
	if got := parsed.Outcomes[len(parsed.Outcomes)-1].Then.Action; got != ThenActionShip {
		t.Errorf("last outcome = %q, want ship", got)
	}
}
