package tools

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
)

// Result is one tool's row in the Settings tools check: whether it works,
// what it is for, and what its detail says. Optional tools do not block the app.
type Result struct {
	Name     string `json:"name"`
	Purpose  string `json:"purpose"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
	Optional bool   `json:"optional"`
}

// GhosttyApplicationPath is where Ghostty is installed. The launch package's desktop actions
// use the same path.
const GhosttyApplicationPath = "/Applications/Ghostty.app"

// GhosttyProbe reports whether Ghostty is installed. It is injected so
// tests never touch the app.
type GhosttyProbe func() bool

// GhosttyInstalled is the rule the Settings tools check and the Ghostty
// route share: on macOS whether GhosttyApplicationPath exists, elsewhere whether
// the ghostty command is on the tools' PATH.
func GhosttyInstalled() bool {
	if runtime.GOOS != "darwin" {
		return shell.LookPath("ghostty")
	}
	_, err := os.Stat(GhosttyApplicationPath)
	return err == nil
}

// installHint is the advice a missing gh, git or tmux gets on goos: Homebrew on macOS, the
// system's package manager elsewhere.
func installHint(goos, tool string) string {
	if goos == "darwin" {
		switch tool {
		case "gh":
			return "not installed: brew install gh"
		case "git":
			return "not installed: xcode-select --install"
		default:
			return "not installed: brew install " + tool
		}
	}
	if tool == "gh" {
		return "not installed: see https://github.com/cli/cli#installation"
	}
	return "not installed: install " + tool + " with your package manager"
}

// acliInstallHint is the advice a missing acli gets on goos: the Homebrew tap on macOS,
// Atlassian's install page elsewhere.
func acliInstallHint(goos string) string {
	if goos == "darwin" {
		return "not installed: brew tap atlassian/homebrew-acli && brew install acli"
	}
	return "not installed: see https://developer.atlassian.com/cloud/acli/guides/install-acli/"
}

// Fixtures are the fixture rows demo mode reports, without inspecting
// anything installed: each tool named, ok, detailed as a fixture.
func Fixtures() []Result {
	names := []string{"gh", "git", "tmux"}
	for _, backend := range core.AllBackend {
		names = append(names, backend.Label())
	}
	names = append(names, "Atlassian CLI", "Ghostty")
	results := make([]Result, 0, len(names))
	for _, name := range names {
		results = append(results, Result{
			Name: name, Purpose: "demo fixture", OK: true, Detail: "Fixture — no real CLI called",
		})
	}
	return results
}

// Run checks gh, git, tmux, each agent CLI, the Atlassian CLI and
// Ghostty on this machine, one command each. A nil sh uses shell.ProcessRunner; a
// nil ghostty uses GhosttyInstalled. Missing tools (status 127) get an
// install hint as their detail.
func Run(ctx context.Context, sh shell.Runner, ghostty GhosttyProbe) []Result {
	if sh == nil {
		sh = shell.ProcessRunner{}
	}
	if ghostty == nil {
		ghostty = GhosttyInstalled
	}
	// Every probe runs at once; the rows keep their order.
	probes := []struct {
		executable string
		args       []string
	}{
		{"gh", []string{"auth", "status", "--active"}},
		{"git", []string{"--version"}},
		{"tmux", []string{"-V"}},
		{"copilot", []string{"--version"}},
		{"pi", []string{"--version"}},
		{"claude", []string{"--version"}},
		{"codex", []string{"--version"}},
		{core.BackendCursor.Executable(os.Getenv), []string{"--version"}},
		{"acli", []string{"jira", "auth", "status"}},
	}
	answers := make([]shell.Result, len(probes))
	var probing sync.WaitGroup
	for i, probe := range probes {
		probing.Add(1)
		go func() {
			defer probing.Done()
			answers[i] = sh.Run(ctx, probe.executable, probe.args, "", nil)
		}()
	}
	probing.Wait()
	gh, git, tmux, copilot, pi := answers[0], answers[1], answers[2], answers[3], answers[4]
	claude, codex, cursor, acli := answers[5], answers[6], answers[7], answers[8]

	ghDetail := "not signed in: run gh auth login"
	switch {
	case gh.OK():
		ghDetail = "signed in"
		for _, line := range toolCheckLines(gh.Stdout + gh.Stderr) {
			if strings.Contains(line, "Logged in") {
				ghDetail = strings.ReplaceAll(line, "✓ ", "")
				break
			}
		}
	case gh.Status == 127:
		ghDetail = installHint(runtime.GOOS, "gh")
	}

	acliDetail := "not signed in: run acli jira auth login"
	switch {
	case acli.OK():
		acliDetail = "signed in"
		for _, line := range toolCheckLines(acli.Stdout + "\n" + acli.Stderr) {
			if strings.HasPrefix(line, "Site:") {
				acliDetail = "signed in to " + strings.TrimSpace(line[len("Site:"):])
				break
			}
		}
	case acli.Status == 127:
		acliDetail = acliInstallHint(runtime.GOOS)
	}

	// Beltline needs one agent CLI, not each: once one is found the others are
	// optional, and with none found every one is marked as missing.
	agentFound := copilot.OK() || pi.OK() || claude.OK() || codex.OK() || cursor.OK()

	ghosttyInstalled := ghostty()
	ghosttyDetail := "not installed"
	if ghosttyInstalled {
		ghosttyDetail = "installed"
	}
	return []Result{
		{Name: "gh", Purpose: "issues, comments, pull requests, clones", OK: gh.OK(), Detail: ghDetail},
		{Name: "git", Purpose: "checkouts and worktrees", OK: git.OK(), Detail: toolCheckLine(git, installHint(runtime.GOOS, "git"))},
		{Name: "tmux", Purpose: "agent sessions", OK: tmux.OK(), Detail: toolCheckLine(tmux, installHint(runtime.GOOS, "tmux"))},
		{Name: "Copilot CLI", Purpose: "agent: copilot", OK: copilot.OK(), Detail: toolCheckLine(copilot, "not installed: npm install -g @github/copilot"), Optional: agentFound},
		{Name: "Pi", Purpose: "agent: pi", OK: pi.OK(), Detail: toolCheckLine(pi, "not installed"), Optional: agentFound},
		{Name: "Claude Code", Purpose: "agent: claude", OK: claude.OK(), Detail: toolCheckLine(claude, "not installed: npm install -g @anthropic-ai/claude-code"), Optional: agentFound},
		{Name: "Codex CLI", Purpose: "agent: codex", OK: codex.OK(), Detail: toolCheckLine(codex, "not installed: npm install -g @openai/codex"), Optional: agentFound},
		{Name: "Cursor CLI", Purpose: "agent: cursor", OK: cursor.OK(), Detail: toolCheckLine(cursor, "not installed: install Cursor CLI from https://cursor.com/install"), Optional: agentFound},
		{Name: "Atlassian CLI", Purpose: "Jira tickets (optional)", OK: acli.OK(), Detail: acliDetail, Optional: true},
		{Name: "Ghostty", Purpose: "Open in Ghostty (optional)", OK: ghosttyInstalled, Detail: ghosttyDetail, Optional: true},
	}
}

// toolCheckLines splits output into its trimmed, non-empty lines.
func toolCheckLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// toolCheckLine is the first line of a tool's output, or the explanation for a
// missing tool (status 127). A failed run with no output says its exit status.
func toolCheckLine(result shell.Result, missing string) string {
	if result.Status == 127 {
		return missing
	}
	text := ""
	if lines := toolCheckLines(result.Stdout + "\n" + result.Stderr); len(lines) > 0 {
		text = lines[0]
	}
	if result.OK() {
		return text
	}
	if text == "" {
		return "exit " + strconv.Itoa(result.Status)
	}
	return text
}
