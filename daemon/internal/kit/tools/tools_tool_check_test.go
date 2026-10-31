package tools

import (
	"context"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
)

// toolCheckOrder is the Settings order of the tools check.
var toolCheckOrder = []string{"gh", "git", "tmux", "Copilot CLI", "Pi", "Claude Code", "Codex CLI", "Cursor CLI", "Atlassian CLI", "Ghostty"}

// toolRecorder answers each executable from results and 127 (not installed) for the rest.
func toolRecorder(results map[string]shell.Result) *shell.Recorder {
	return &shell.Recorder{Reply: func(c shell.Call) shell.Result {
		if result, ok := results[c.Executable]; ok {
			return result
		}
		return shell.Result{Status: 127}
	}}
}

func names(results []Result) []string {
	var out []string
	for _, result := range results {
		out = append(out, result.Name)
	}
	return out
}

func rowNamed(results []Result, name string) Result {
	for _, result := range results {
		if result.Name == name {
			return result
		}
	}
	return Result{}
}

func TestOneAgentCLIMakesTheOthersOptional(t *testing.T) {
	t.Parallel()
	rec := toolRecorder(map[string]shell.Result{"claude": {Stdout: "2.1.0 (Claude Code)"}})
	rows := Run(context.Background(), rec, func() bool { return false })
	for _, name := range []string{"Copilot CLI", "Pi", "Claude Code", "Codex CLI", "Cursor CLI"} {
		if !rowNamed(rows, name).Optional {
			t.Errorf("%s is required although Claude Code was found", name)
		}
	}
	for _, name := range []string{"gh", "git", "tmux"} {
		if rowNamed(rows, name).Optional {
			t.Errorf("%s is optional", name)
		}
	}
}

func TestRowsKeepTheSettingsOrderAndOptionalMeanings(t *testing.T) {
	t.Parallel()
	rec := toolRecorder(nil)
	rows := Run(context.Background(), rec, func() bool { return false })
	if !reflect.DeepEqual(names(rows), toolCheckOrder) {
		t.Fatalf("names = %v", names(rows))
	}
	var optional []bool
	for _, row := range rows {
		optional = append(optional, row.Optional)
	}
	if !reflect.DeepEqual(optional, []bool{false, false, false, false, false, false, false, false, true, true}) {
		t.Errorf("optional = %v", optional)
	}
	for _, row := range rows {
		if row.OK {
			t.Errorf("%s is ok with nothing installed", row.Name)
		}
	}
	if !strings.HasPrefix(rowNamed(rows, "Cursor CLI").Detail, "not installed") {
		t.Errorf("cursor detail = %q", rowNamed(rows, "Cursor CLI").Detail)
	}
	if got := rowNamed(rows, "Pi").Detail; got != "not installed: npm install -g --ignore-scripts @earendil-works/pi-coding-agent" {
		t.Errorf("pi detail = %q", got)
	}
	if last := rows[len(rows)-1]; last.Detail != "not installed" {
		t.Errorf("ghostty detail = %q", last.Detail)
	}
	executables := map[string]bool{}
	for _, c := range rec.Calls() {
		executables[c.Executable] = true
	}
	cursor := core.BackendCursor.Executable(os.Getenv)
	want := map[string]bool{"gh": true, "git": true, "tmux": true, "copilot": true, "pi": true, "claude": true, "codex": true, cursor: true, "acli": true}
	if !reflect.DeepEqual(executables, want) {
		t.Errorf("executables = %v, want %v", executables, want)
	}
	expected := map[string][]string{
		"gh": {"auth", "status", "--active"}, "git": {"--version"}, "tmux": {"-V"}, "copilot": {"--version"},
		"pi": {"--version"}, "claude": {"--version"}, "codex": {"--version"}, cursor: {"--version"},
		"acli": {"jira", "auth", "status"},
	}
	for _, c := range rec.Calls() {
		if !reflect.DeepEqual(expected[c.Executable], c.Args) {
			t.Errorf("%s called with %v", c.Executable, c.Args)
		}
	}
}

func TestPresentExecutablesParseVersionAndSignInDetails(t *testing.T) {
	t.Parallel()
	rec := toolRecorder(map[string]shell.Result{
		"gh":                                     {Stdout: "✓ Logged in to github.com as octocat (oauth_token)\n"},
		"git":                                    {Stdout: "git version 2.50.1\n"},
		"tmux":                                   {Stderr: "tmux 3.4\n"},
		"copilot":                                {Stdout: "GitHub Copilot CLI 1.0.1\n"},
		"pi":                                     {Stdout: "pi 0.9.0\n"},
		"claude":                                 {Stdout: "2.1.0 (Claude Code)\n"},
		"codex":                                  {Stdout: "codex-cli 0.5.0\n"},
		core.BackendCursor.Executable(os.Getenv): {Stdout: "0.42.0\n"},
		"acli":                                   {Stdout: "Site: example.atlassian.net\nlogged in\n"},
	})
	rows := Run(context.Background(), rec, func() bool { return true })
	for _, row := range rows {
		if !row.OK {
			t.Errorf("%s is not ok", row.Name)
		}
	}
	if got := rowNamed(rows, "gh").Detail; got != "Logged in to github.com as octocat (oauth_token)" {
		t.Errorf("gh detail = %q", got)
	}
	if got := rowNamed(rows, "git").Detail; got != "git version 2.50.1" {
		t.Errorf("git detail = %q", got)
	}
	if got := rowNamed(rows, "tmux").Detail; got != "tmux 3.4" {
		t.Errorf("tmux detail = %q (stderr is parsed like stdout)", got)
	}
	if got := rowNamed(rows, "Atlassian CLI").Detail; got != "signed in to example.atlassian.net" {
		t.Errorf("acli detail = %q", got)
	}
	if got := rows[len(rows)-1].Detail; got != "installed" {
		t.Errorf("ghostty detail = %q", got)
	}
}

func TestSignInFailuresAndOtherExitCodesAreDiagnosticRowsNotErrors(t *testing.T) {
	t.Parallel()
	rec := toolRecorder(map[string]shell.Result{
		"gh":     {Status: 1, Stderr: "not logged into any GitHub hosts\n"},
		"git":    {Status: 2, Stderr: "fatal: not a git repository\n"},
		"tmux":   {Status: 3},
		"acli":   {Status: 1, Stderr: "You are not logged in\n"},
		"claude": {Status: 0, Stdout: ""},
	})
	rows := Run(context.Background(), rec, func() bool { return false })
	if got := rowNamed(rows, "gh").Detail; got != "not signed in: run gh auth login" {
		t.Errorf("gh detail = %q", got)
	}
	if got := rowNamed(rows, "git").Detail; got != "fatal: not a git repository" {
		t.Errorf("git detail = %q", got)
	}
	if got := rowNamed(rows, "tmux").Detail; got != "exit 3" {
		t.Errorf("tmux detail = %q, want a silent nonzero exit named", got)
	}
	if got := rowNamed(rows, "Atlassian CLI").Detail; got != "not signed in: run acli jira auth login" {
		t.Errorf("acli detail = %q", got)
	}
	if row := rowNamed(rows, "Claude Code"); !row.OK || row.Detail != "" {
		t.Errorf("claude row = %+v", row)
	}
}

func TestGhExplainsAMissingExecutableWithInstallAdvice(t *testing.T) {
	t.Parallel()
	rows := Run(context.Background(), toolRecorder(map[string]shell.Result{"gh": {Status: 127}}), func() bool { return false })
	if rows[0].Detail != installHint(runtime.GOOS, "gh") {
		t.Errorf("gh detail = %q", rows[0].Detail)
	}
	if rows[0].OK {
		t.Error("gh is ok when missing")
	}
}

func TestDemoFixturesNeverRunATool(t *testing.T) {
	t.Parallel()
	fixtures := Fixtures()
	if !reflect.DeepEqual(names(fixtures), toolCheckOrder) {
		t.Fatalf("names = %v", names(fixtures))
	}
	for _, row := range fixtures {
		if !row.OK || !strings.Contains(row.Detail, "Fixture") {
			t.Errorf("%s = %+v", row.Name, row)
		}
	}
}

func TestTheCursorRowShowsItsExitOneDetail(t *testing.T) {
	t.Parallel()
	cursor := core.BackendCursor.Executable(os.Getenv)
	rows := Run(context.Background(), toolRecorder(map[string]shell.Result{
		cursor: {Status: 1, Stderr: "Cursor is not signed in\nrun cursor-agent login\n"},
	}), func() bool { return false })
	row := rowNamed(rows, "Cursor CLI")
	if row.OK || row.Detail != "Cursor is not signed in" {
		t.Errorf("cursor row = %+v", row)
	}
}

// TestToolProbesRunTogether holds every probe until all nine have started, so
// the check only finishes when the probes run concurrently.
func TestToolProbesRunTogether(t *testing.T) {
	t.Parallel()
	const probes = 9
	var started sync.WaitGroup
	started.Add(probes)
	release := make(chan struct{})
	rec := &shell.Recorder{Reply: func(shell.Call) shell.Result {
		started.Done()
		<-release
		return shell.Result{Status: 127}
	}}
	done := make(chan []Result, 1)
	go func() { done <- Run(context.Background(), rec, func() bool { return false }) }()
	allStarted := make(chan struct{})
	go func() { started.Wait(); close(allStarted) }()
	select {
	case <-allStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("the probes did not start together")
	}
	close(release)
	rows := <-done
	if !reflect.DeepEqual(names(rows), toolCheckOrder) {
		t.Fatalf("row order = %v", names(rows))
	}
}

func TestInstallHintsNameHomebrewOnMacOSAndThePackageManagerOnLinux(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ goos, tool, want string }{
		{"darwin", "gh", "not installed: brew install gh"},
		{"darwin", "git", "not installed: xcode-select --install"},
		{"darwin", "tmux", "not installed: brew install tmux"},
		{"linux", "gh", "not installed: see https://github.com/cli/cli#installation"},
		{"linux", "git", "not installed: install git with your package manager"},
		{"linux", "tmux", "not installed: install tmux with your package manager"},
	} {
		if got := installHint(c.goos, c.tool); got != c.want {
			t.Errorf("installHint(%s, %s) = %q, want %q", c.goos, c.tool, got, c.want)
		}
	}
}
