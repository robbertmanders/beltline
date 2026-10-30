// Tests of the agent adapters: their launch, parsing and outcome handling.

package runner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

func TestRunnersRoundTrip(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		variable := "BELTLINE_" + strings.ToUpper(string(backend))
		override := os.Getenv(variable)
		expected := string(backend)
		if backend == core.BackendCursor {
			expected = "cursor-agent"
		}
		if override != "" {
			expected = override
		}
		if got := backend.Executable(func(key string) string {
			if key == variable {
				return override
			}
			return ""
		}); got != expected {
			t.Errorf("%s executable = %q, want %q", backend, got, expected)
		}
	}
	if core.BackendClaude.Label() != "Claude Code" {
		t.Errorf("claude label = %q", core.BackendClaude.Label())
	}
}

func TestPiRunsAStageHeadlessLikeTheOtherBackends(t *testing.T) {
	t.Parallel()
	adapter := PiAdapter{}
	start := adapter.Arguments("abc", "bl-x", " model ", new(core.ThinkingHigh))
	mustEqual(t, []string{"--print", "--approve", "--session-id", "abc", "--name", "bl-x", "--model", "model", "--thinking", "high", "$PROMPT"}, start)
	resume := adapter.ResumeArguments("abc", "bl-x", "", nil, nil)
	mustEqual(t, []string{"--print", "--approve", "--session", "abc", "$PROMPT"}, resume)
	for _, arguments := range [][]string{start, resume} {
		for _, forbidden := range []string{"--tui-mode", "--no-session", "--resume"} {
			if slices.Contains(arguments, forbidden) {
				t.Errorf("headless Pi arguments carry %s: %v", forbidden, arguments)
			}
		}
	}
}

func TestPiContinueStaysInteractive(t *testing.T) {
	t.Parallel()
	adapter := PiAdapter{}
	continuing := adapter.ContinueArguments("abc", "bl-x", nil)
	mustEqual(t, []string{"--tui-mode", "fullscreen", "--session", "abc"}, continuing)
	conversation := adapter.ConversationArguments("abc", "bl-x", " model ", new(core.ThinkingHigh))
	mustEqual(t, []string{"--tui-mode", "fullscreen", "--session-id", "abc", "--name", "bl-x", "--model", "model", "--thinking", "high", "$PROMPT"}, conversation)
	for _, arguments := range [][]string{continuing, conversation} {
		if slices.Contains(arguments, "--print") || slices.Contains(arguments, "--approve") {
			t.Errorf("interactive Pi arguments carry a headless flag: %v", arguments)
		}
	}
}

func TestHeadlessBackendsSayTheyDrawNothingWhileRunning(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			note := AdapterFor(backend).RunningNote()
			if backend == core.BackendCopilot {
				if note != nil {
					t.Fatalf("Copilot note = %q, want nil", *note)
				}
				return
			}
			want := backend.Label() + " runs headless: nothing is drawn until the attempt ends."
			if note == nil || *note != want {
				t.Fatalf("note = %v, want %q", note, want)
			}
		})
	}
}

func TestPiLogCountsTurnsButNeverDeterminesCompletion(t *testing.T) {
	withHome(t)
	directory := t.TempDir()
	file := filepath.Join(PiSessionDirectory(directory), "test-abc.jsonl")
	testkit.WriteFile(t, file, "")
	first := `{"type":"message","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"Outcome: needs info"}]}}`
	tool := `{"type":"message","message":{"role":"assistant","stopReason":"toolUse"}}`
	last := `{"type":"message","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"Outcome: complete"}]}}`
	adapter := PiAdapter{}
	testkit.WriteFile(t, file, strings.Join([]string{first, tool}, "\n"))
	mark := adapter.Mark(directory, "abc")
	if mark != 2 {
		t.Fatalf("mark = %d, want 2", mark)
	}
	if got := adapter.Ending(directory, "abc", 0); got != nil {
		t.Fatalf("ending = %v, want nil", got)
	}
	if got := adapter.Reply(directory, "abc", mark); got != nil {
		t.Fatalf("reply = %v, want nil", *got)
	}
	testkit.WriteFile(t, file, strings.Join([]string{first, tool, last}, "\n"))
	if got := adapter.Mark(directory, "abc"); got != 3 {
		t.Fatalf("mark = %d, want 3", got)
	}
	if got := adapter.Ending(directory, "abc", mark); got != nil {
		t.Fatalf("ending = %v, want nil", got)
	}
	if got := adapter.Reply(directory, "abc", mark); got == nil || *got != "Outcome: complete" {
		t.Fatalf("reply = %v, want Outcome: complete", got)
	}
	errorLine := `{"type":"message","message":{"role":"assistant","stopReason":"error","errorMessage":"402: needs credits"}}`
	testkit.WriteFile(t, file, errorLine)
	if got := adapter.Ending(directory, "abc", 0); got != nil {
		t.Fatalf("ending after error line = %v, want nil", got)
	}
}

func TestClaudeRunsHeadlessAndResumesWithoutStartingANewSession(t *testing.T) {
	t.Parallel()
	adapter := ClaudeAdapter{}
	start := adapter.Arguments("abc", "bl-x", " haiku ", new(core.ThinkingMax))
	conversation := []string{"--session-id", "abc", "-n", "bl-x", "--allow-dangerously-skip-permissions", "--permission-mode", "bypassPermissions", "--model", "haiku", "--effort", "max", "$PROMPT"}
	mustEqual(t, append([]string{"-p", "--output-format", "text"}, conversation...), start)
	mustEqual(t, conversation, adapter.ConversationArguments("abc", "bl-x", " haiku ", new(core.ThinkingMax)))
	resumed := adapter.ResumeArguments("abc", "bl-x", "", nil, nil)
	if !hasPrefixArgs(resumed, []string{"-p", "--output-format", "text", "--resume", "abc"}) {
		t.Fatalf("resumed = %v", resumed)
	}
	if slices.Contains(resumed, "--session-id") {
		t.Fatalf("resume starts a session: %v", resumed)
	}
	continued := adapter.ContinueArguments("abc", "bl-x", nil)
	if slices.Contains(continued, "$PROMPT") || slices.Contains(continued, "-p") {
		t.Fatalf("continue is headless: %v", continued)
	}
	if !hasPrefixArgs(continued, []string{"--resume", "abc"}) {
		t.Fatalf("continue = %v", continued)
	}
	const context = "claude-opus-5-5[1m]"
	if !slices.Contains(adapter.Arguments("abc", "bl-x", context, new(core.ThinkingHigh)), context) {
		t.Errorf("start drops the context suffix")
	}
	if !slices.Contains(adapter.ConversationArguments("abc", "bl-x", context, new(core.ThinkingHigh)), context) {
		t.Errorf("conversation drops the context suffix")
	}
	if !slices.Contains(adapter.ResumeArguments("abc", "bl-x", context, new(core.ThinkingLow), nil), context) {
		t.Errorf("resume drops the context suffix")
	}
	if slices.Contains(continued, "--model") || slices.Contains(continued, context) {
		t.Errorf("continue carries a model: %v", continued)
	}
}

func TestTranscriptCountsOnlyMainTurnsAndOnlyNewReplies(t *testing.T) {
	t.Parallel()
	var lines []string
	for _, line := range strings.Split(string(testkit.ReadFixture(t, "claude-session.jsonl")), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	logOf := func(count int) string { return strings.Join(lines[:count], "\n") }
	if got := ClaudeEndingOf(logOf(3), 0); got != nil {
		t.Fatalf("ending(3) = %v", got)
	}
	if got := ClaudeEndingOf(strings.Replace(lines[1], `tool_use","content`, `end_turn","content`, 1), 0); got != nil {
		t.Fatalf("ending(end_turn) = %v", got)
	}
	if got := ClaudeMarkOf(logOf(3), 0); got != 1 {
		t.Fatalf("mark(3) = %d, want 1", got)
	}
	// A finished turn is left to the exit code.
	if got := ClaudeEndingOf(logOf(5), 0); got != nil {
		t.Fatalf("ending(5) = %v", got)
	}
	if got := ClaudeMarkOf(logOf(5), 0); got != 2 {
		t.Fatalf("mark(5) = %d, want 2", got)
	}
	reply := ClaudeReplyOf(logOf(5), 0)
	if reply == nil || *reply != "Done.\nOutcome: ready to spec" {
		t.Fatalf("reply(5) = %v", reply)
	}
	outcomes := core.SampleBelt([]string{"acme/calc"}, "agent", core.SampleAgents(), true).Stages[0].Outcomes
	if core.OutcomeParserCompleting("# Output", *reply, outcomes) == nil {
		t.Fatalf("the reply does not complete with an outcome")
	}
	if got := ClaudeEndingOf(logOf(5), 2); got != nil {
		t.Fatalf("ending(5, after 2) = %v", got)
	}
	if got := ClaudeReplyOf(logOf(5), 2); got != nil {
		t.Fatalf("reply(5, after 2) = %v", *got)
	}
	if got := ClaudeEndingOf(logOf(7), 2); got != nil {
		t.Fatalf("ending(7, after 2) = %v", got)
	}
	if got := ClaudeReplyOf(logOf(7), 2); got != nil {
		t.Fatalf("reply(7, after 2) = %v", *got)
	}
	if got := ClaudeEndingOf(logOf(8), 2); got == nil || got.Kind != LogEndingFailed || got.Message != "API Error: 401 Not logged in" {
		t.Fatalf("ending(8, after 2) = %v", got)
	}
	if got := ClaudeReplyOf(logOf(8), 2); got == nil || *got != "API Error: 401 Not logged in" {
		t.Fatalf("reply(8, after 2) = %v", got)
	}
	long := strings.Repeat("x", 400)
	for _, errText := range []string{"Not logged in", "You've hit your session limit", long} {
		entry := strings.Replace(lines[7], "API Error: 401 Not logged in", errText, 1)
		want := errText
		if len(want) > 300 {
			want = want[:300]
		}
		got := ClaudeEndingOf(entry, 0)
		if got == nil || got.Message != want {
			t.Errorf("ending(%.20s…) = %v, want failed(%d chars)", errText, got, len(want))
		}
	}
	synthetic := `{"type":"assistant","message":{"role":"assistant","model":"<synthetic>","stop_reason":"stop_sequence","content":[{"type":"text","text":"No response requested."}]}}`
	interrupted := logOf(5) + "\n" + synthetic
	if got := ClaudeEndingOf(interrupted, 0); got != nil {
		t.Fatalf("interrupted ending = %v", got)
	}
	prompted := interrupted + "\n" + `{"type":"user","message":{"role":"user","content":"Continue"}}`
	if got := ClaudeEndingOf(prompted, 0); got != nil {
		t.Fatalf("prompted ending = %v", got)
	}
	if got := ClaudeEndingOf(interrupted, ClaudeMarkOf(interrupted, 0)); got != nil {
		t.Fatalf("interrupted ending after its mark = %v", got)
	}
	if got := ClaudeEndingOf(logOf(8)+"\n"+lines[3], 4); got != nil {
		t.Fatalf("ending after the last reply = %v", got)
	}
	if got := ClaudeReplyOf(logOf(8)+"\n"+lines[3], 4); got == nil || *got != "Done.\nOutcome: ready to spec" {
		t.Fatalf("reply after the last reply = %v", got)
	}
	if got := ClaudeEndingOf("{partial", 0); got != nil {
		t.Fatalf("partial ending = %v", got)
	}
	if got := filepath.Base(ClaudeSessionDirectory("/private/tmp/y-git")); got != "-private-tmp-y-git" {
		t.Fatalf("session folder = %q", got)
	}
	if got := filepath.Base(ClaudeSessionDirectory("/nonexistent/Application Support/.x/a_b")); got != "-nonexistent-Application-Support--x-a-b" {
		t.Fatalf("session folder = %q", got)
	}
}

func TestCodexKeepsExtraHighAndMax(t *testing.T) {
	t.Parallel()
	if got := core.BackendCodex.Thinking(new(core.ThinkingXHigh)); got == nil || *got != core.ThinkingXHigh {
		t.Errorf("codex xhigh = %v", got)
	}
	if got := core.BackendCodex.Thinking(new(core.ThinkingMax)); got == nil || *got != core.ThinkingMax {
		t.Errorf("codex max = %v", got)
	}
	if got := core.BackendCodex.Thinking(new(core.ThinkingLow)); got == nil || *got != core.ThinkingLow {
		t.Errorf("codex low = %v", got)
	}
	if got := core.BackendClaude.Thinking(new(core.ThinkingMax)); got == nil || *got != core.ThinkingMax {
		t.Errorf("claude max = %v", got)
	}
	if got := core.BackendCodex.Thinking(nil); got != nil {
		t.Errorf("codex nil = %v", *got)
	}
	for _, level := range core.AllThinking {
		level := level
		if got := core.BackendCodex.Thinking(&level); got == nil || *got != level {
			t.Errorf("codex %s = %v", level, got)
		}
		if got := core.BackendClaude.Thinking(&level); got == nil || *got != level {
			t.Errorf("claude %s = %v", level, got)
		}
	}
}

// hasPrefixArgs reports whether a command line starts with the given arguments.
func hasPrefixArgs(arguments, prefix []string) bool {
	return len(prefix) <= len(arguments) && slices.Equal(arguments[:len(prefix)], prefix)
}
