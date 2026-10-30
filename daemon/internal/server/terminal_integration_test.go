//go:build integration

// Tests of the terminal socket against real tmux sessions on a private tmux server: exact sessions
// relay output, input, resize and disconnects. Isolation: runner.TmuxSocket is read once per process,
// so this test binary's tmux server is named by its process id through BELTLINE_TMUX_SOCKET, set in
// init before anything reads it. It is killed, and its socket file removed, in t.Cleanup.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/shell"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// terminalIntegrationSocketEnv names the variable runner reads for the tmux server.
const terminalIntegrationSocketEnv = "BELTLINE_TMUX_SOCKET"

// terminalIntegrationSocket is this test binary's own tmux server name.
var terminalIntegrationSocket = fmt.Sprintf("beltline-test-%d", os.Getpid())

func init() {
	// Set before any runner reads the socket (runner.TmuxSocket is read once).
	_ = os.Setenv(terminalIntegrationSocketEnv, terminalIntegrationSocket)
}

// terminalTmux runs tmux on the test's own server with the daemon's environment.
// It returns stdout, stderr and whether tmux exited 0.
func terminalTmux(args ...string) (string, string, bool) {
	cmd := exec.Command("tmux", append([]string{"-L", terminalIntegrationSocket}, args...)...)
	cmd.Env = shell.Environment()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err == nil
}

// terminalSocketPath is where tmux keeps the named server's socket.
func terminalSocketPath() string {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), terminalIntegrationSocket)
}

// terminalTmuxServerCleanup kills the test's tmux server and removes its socket
// file. It is registered once per test.
func terminalTmuxServerCleanup(t *testing.T) {
	t.Cleanup(func() {
		_, _, _ = terminalTmux("kill-server")
		_ = os.Remove(terminalSocketPath())
	})
}

// terminalIntegrationRun is a run with one open agent
// attempt on session, backed by backend, in the running state.
func terminalIntegrationRun(id, session string, backend core.Backend) core.Run {
	now := testkit.ContractTime
	belt := core.SampleBelt([]string{"acme/billing"}, "terminal", core.SampleAgents(), true)
	for i := range belt.Stages {
		if belt.Stages[i].Kind() == core.StageKindAgent {
			chosen := backend
			belt.Stages[i].Runner = &chosen
			break
		}
	}
	run := core.NewRun(core.RunArgs{
		Belt:    belt,
		Subject: core.Subject{Kind: core.SubjectKindManual, Key: id, Title: "Terminal fixture"},
		Now:     now,
	})
	run.ID = id
	var stage core.Stage
	for _, candidate := range run.Stages {
		if candidate.Kind() == core.StageKindAgent {
			stage = candidate
			break
		}
	}
	run.StageID = stage.ID
	run.State = core.RunStateRunning
	attempt := core.NewAttempt(core.AttemptArgs{StageID: stage.ID, Number: 1, StartedAt: jsonx.Date{Time: now}, Folder: "attempt-1"})
	attempt.Session = &session
	sessionID := "fixture-session"
	attempt.SessionID = &sessionID
	attempt.Settings = &core.SessionSettings{Runner: &backend, Model: "fixture-model"}
	if backend == core.BackendCopilot {
		interactive := true
		attempt.CopilotInteractive = &interactive
	}
	run.Attempts = []core.Attempt{attempt}
	return run
}

// terminalCreateShellSession starts a shell session that prints the ready marker.
func terminalCreateShellSession(t *testing.T, name string) {
	t.Helper()
	_, stderr, ok := terminalTmux("new-session", "-d", "-s", name, "-x", "80", "-y", "24", "/bin/sh", "-c", "printf 'terminal-ready\\n'; exec /bin/sh -i")
	if !ok {
		t.Fatalf("could not create tmux fixture %s: %s", name, stderr)
	}
}

// terminalClientCount is the number of tmux clients attached to session.
func terminalClientCount(session string) int {
	stdout, _, _ := terminalTmux("list-clients", "-t", "="+session, "-F", "#{client_name}")
	return len(strings.Fields(stdout))
}

// terminalWaitForTmuxClient waits until a client of session has the given size.
func terminalWaitForTmuxClient(t *testing.T, session, dimensions string) {
	t.Helper()
	testkit.Eventually(t, 5*time.Second, fmt.Sprintf("tmux client for %s did not resize to %s", session, dimensions), func() bool {
		stdout, _, _ := terminalTmux("list-clients", "-t", "="+session, "-F", "#{client_width}x#{client_height}")
		return strings.TrimSpace(stdout) == dimensions
	})
}

// terminalWaitForTmuxClientCount waits until session has count clients.
func terminalWaitForTmuxClientCount(t *testing.T, session string, count int) {
	t.Helper()
	testkit.Eventually(t, 5*time.Second, fmt.Sprintf("tmux session %s did not have %d clients", session, count), func() bool {
		return terminalClientCount(session) == count
	})
}

// terminalWaitForNoTmuxClients waits until no client is attached to session.
func terminalWaitForNoTmuxClients(t *testing.T, session string) {
	t.Helper()
	testkit.Eventually(t, 5*time.Second, fmt.Sprintf("tmux client for %s did not detach", session), func() bool {
		stdout, stderr, _ := terminalTmux("list-clients", "-t", "="+session)
		return stdout+stderr == ""
	})
}

// terminalWaitForFile waits until path exists.
func terminalWaitForFile(t *testing.T, path string) {
	t.Helper()
	testkit.Eventually(t, 5*time.Second, "terminal input did not create "+filepath.Base(path), func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

// terminalSocketClient is a terminal WebSocket the test drives.
type terminalSocketClient struct {
	t        *testing.T
	conn     *websocket.Conn
	lastKind websocket.MessageType
}

// terminalDial opens the terminal socket for session, taking over when asked,
// and fails the test unless the upgrade is accepted.
func terminalDial(t *testing.T, port int, session string, takeOver bool) *terminalSocketClient {
	t.Helper()
	query := "session=" + session
	if takeOver {
		query += "&takeOver=1"
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+terminalTestToken)
	header.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", port))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/api/v1/terminals?%s", port, query), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("terminal connect for %s: %v", session, err)
	}
	conn.SetReadLimit(16 * 1024 * 1024)
	client := &terminalSocketClient{t: t, conn: conn}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return client
}

// sendBinary sends keystrokes as a binary frame.
func (c *terminalSocketClient) sendBinary(data []byte) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		c.t.Fatalf("terminal binary write: %v", err)
	}
}

// sendText sends a control message as a text frame.
func (c *terminalSocketClient) sendText(text string) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, []byte(text)); err != nil {
		c.t.Fatalf("terminal text write: %v", err)
	}
}

// waitForOutput reads frames until their bytes contain marker, and returns them.
func (c *terminalSocketClient) waitForOutput(marker []byte) []byte {
	c.t.Helper()
	var received []byte
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		kind, data, err := c.conn.Read(ctx)
		cancel()
		if err != nil {
			c.t.Fatalf("terminal output did not contain %q; read failed: %v (got %q)", marker, err, received)
		}
		c.lastKind = kind
		received = append(received, data...)
		if bytes.Contains(received, marker) {
			return received
		}
	}
	c.t.Fatalf("terminal output did not contain %q; got %q", marker, received)
	return nil
}

// waitForCloseCode reads until the socket closes, and returns the close code.
func (c *terminalSocketClient) waitForCloseCode() websocket.StatusCode {
	c.t.Helper()
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, _, err := c.conn.Read(ctx)
		cancel()
		if err != nil {
			if code := websocket.CloseStatus(err); code != -1 {
				return code
			}
			c.t.Fatalf("terminal socket failed without a close code: %v", err)
		}
	}
	c.t.Fatal("terminal WebSocket did not close")
	return -1
}

// disconnect sends a normal close and waits for the reply.
func (c *terminalSocketClient) disconnect() {
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}

// disconnectAbruptly drops the TCP connection without a close frame.
func (c *terminalSocketClient) disconnectAbruptly() {
	_ = c.conn.CloseNow()
}

// terminalReadFile reads a file the pane wrote.
func terminalReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// terminalRunsSorted is the stored runs, ordered by id as the test compares them.
func terminalRunsSorted(storage store.Storage) []core.Run {
	runs := storage.LoadRuns()
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs
}

func TestExactSessionsRelayOutputInputResizeAndDisconnectWithoutKillingThePane(t *testing.T) {
	terminalTmuxServerCleanup(t)
	root := t.TempDir()
	storage := store.NewStorage(store.Paths{Root: root})
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	readOnlySession := "bl-terminal-read-" + suffix
	writableSession := "bl-terminal-write-" + suffix
	missingSession := "bl-terminal-missing-" + suffix
	neighboringSession := missingSession + "-neighbor"
	for _, run := range []core.Run{
		terminalIntegrationRun("readonly-"+suffix, readOnlySession, core.BackendPi),
		terminalIntegrationRun("writable-"+suffix, writableSession, core.BackendCopilot),
		terminalIntegrationRun("missing-"+suffix, missingSession, core.BackendPi),
	} {
		if err := storage.SaveRun(run); err != nil {
			t.Fatalf("save run: %v", err)
		}
	}
	c, err := coordinator.New(coordinator.Services{
		Storage:    storage,
		GitHub:     github.NewFakeGitHub(nil, nil, nil, nil),
		Jira:       jira.NewFakeJira(nil, nil, nil),
		Runner:     runner.NewTmuxRunner(nil),
		Workspaces: runner.FolderWorkspaces{},
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}
	t.Cleanup(c.Close)
	assistantSession := c.AssistantSessionName()
	for _, name := range []string{readOnlySession, writableSession, neighboringSession, assistantSession} {
		terminalCreateShellSession(t, name)
	}
	port := terminalServerFor(t, c)

	stdout, _, _ := terminalTmux("list-panes", "-t", "="+readOnlySession, "-F", "#{pane_pid}")
	panePID := strings.TrimSpace(stdout)
	if panePID == "" {
		t.Fatal("read-only session has no pane")
	}
	storedRuns := terminalRunsSorted(storage)

	// The read-only session drops keystrokes and applies resizes.
	readOnly := terminalDial(t, port, readOnlySession, false)
	readOnly.waitForOutput([]byte("terminal-ready"))
	droppedMarker := filepath.Join(root, "must-not-exist")
	readOnly.sendBinary([]byte("touch " + droppedMarker + "\n"))
	readOnly.sendText(`{"type":"resize","cols":100,"rows":32}`)
	terminalWaitForTmuxClient(t, readOnlySession, "100x32")
	if _, err := os.Stat(droppedMarker); err == nil {
		t.Fatal("keystrokes on a read-only session reached the pane")
	}
	readOnly.sendText(`{"type":"resize","cols":100.5,"rows":32}`)
	if code := readOnly.waitForCloseCode(); code != websocket.StatusPolicyViolation {
		t.Fatalf("invalid resize closed with %d, want 1008", code)
	}
	terminalWaitForNoTmuxClients(t, readOnlySession)
	if _, _, ok := terminalTmux("has-session", "-t", "="+readOnlySession); !ok {
		t.Fatal("the read-only session was killed with its client")
	}
	if pid, _, _ := terminalTmux("list-panes", "-t", "="+readOnlySession, "-F", "#{pane_pid}"); strings.TrimSpace(pid) != panePID {
		t.Fatal("the read-only session's pane changed")
	}

	// A takeover writes keystrokes, sizes the client, and shares its output with an observer.
	writable := terminalDial(t, port, writableSession, true)
	writable.waitForOutput([]byte("terminal-ready"))
	writable.sendText(`{"type":"resize","cols":120,"rows":40}`)
	terminalWaitForTmuxClient(t, writableSession, "120x40")
	observer := terminalDial(t, port, writableSession, false)
	terminalWaitForTmuxClientCount(t, writableSession, 2)
	writable.sendBinary([]byte("stty -echo\n"))
	byteMarker := "terminal-binary-" + suffix
	writable.sendBinary([]byte("printf '\\033[31m" + byteMarker + "\\033[0m\\n'\n"))
	writable.waitForOutput([]byte(byteMarker))
	if writable.lastKind != websocket.MessageBinary {
		t.Fatal("PTY output is not sent in a binary WebSocket frame")
	}
	observer.waitForOutput([]byte(byteMarker))
	marker := filepath.Join(root, "takeover-worked")
	writable.sendBinary([]byte("printf '%s|%s' \"$BELTLINE_TMUX_SOCKET\" \"$PATH\" > " + marker + "\n"))
	terminalWaitForFile(t, marker)
	environment := terminalReadFile(t, marker)
	if !strings.Contains(environment, terminalIntegrationSocket) {
		t.Fatalf("pane environment %q does not name the test socket", environment)
	}
	if !strings.Contains(environment, "/opt/homebrew/bin") && !strings.Contains(environment, "/usr/local/bin") {
		t.Fatalf("pane PATH %q has no Homebrew directory", environment)
	}
	termnames, _, _ := terminalTmux("list-clients", "-t", "="+writableSession, "-F", "#{client_termname}")
	names := strings.Fields(termnames)
	if len(names) != 2 || names[0] != "xterm-256color" || names[1] != "xterm-256color" {
		t.Fatalf("client terminal names = %v, want two xterm-256color", names)
	}
	writable.disconnect()
	terminalWaitForTmuxClientCount(t, writableSession, 1)
	observerMarker := "observer-survived-" + suffix
	replacement := terminalDial(t, port, writableSession, true)
	terminalWaitForTmuxClientCount(t, writableSession, 2)
	replacement.sendBinary([]byte("printf '" + observerMarker + "\\n'\n"))
	replacement.waitForOutput([]byte(observerMarker))
	observer.waitForOutput([]byte(observerMarker))
	replacement.disconnect()
	terminalWaitForTmuxClientCount(t, writableSession, 1)
	observer.disconnect()
	terminalWaitForNoTmuxClients(t, writableSession)
	if _, _, ok := terminalTmux("has-session", "-t", "="+writableSession); !ok {
		t.Fatal("the writable session was killed with its clients")
	}

	// The configuration assistant's conversation accepts input without a takeover.
	assistant := terminalDial(t, port, assistantSession, false)
	assistant.waitForOutput([]byte("terminal-ready"))
	assistantMarker := filepath.Join(root, "assistant-writable")
	assistant.sendBinary([]byte("touch " + assistantMarker + "\n"))
	terminalWaitForFile(t, assistantMarker)
	assistant.disconnect()
	terminalWaitForNoTmuxClients(t, assistantSession)

	// A client that vanishes without a close frame ends its relay and nothing else.
	abrupt := terminalDial(t, port, assistantSession, false)
	abrupt.waitForOutput([]byte("terminal-ready"))
	abrupt.sendBinary([]byte("while :; do printf 'abrupt-keepalive\\n'; sleep 0.05; done\n"))
	abrupt.waitForOutput([]byte("abrupt-keepalive"))
	abrupt.disconnectAbruptly()
	terminalWaitForNoTmuxClients(t, assistantSession)
	if _, _, ok := terminalTmux("has-session", "-t", "="+assistantSession); !ok {
		t.Fatal("the assistant session was killed with its client")
	}

	if !reflect.DeepEqual(terminalRunsSorted(storage), storedRuns) {
		t.Fatal("the terminal changed the saved runs")
	}

	// A takeover on a paused run is revoked: its keystrokes are dropped and the socket closes.
	revoked := terminalDial(t, port, writableSession, true)
	terminalWaitForTmuxClientCount(t, writableSession, 1)
	c.PauseRun(context.Background(), "writable-"+suffix)
	revokedMarker := filepath.Join(root, "revoked-takeover-must-not-run")
	revoked.sendBinary([]byte("touch " + revokedMarker + "\n"))
	if code := revoked.waitForCloseCode(); code != websocket.StatusPolicyViolation {
		t.Fatalf("revoked takeover closed with %d, want 1008", code)
	}
	if _, err := os.Stat(revokedMarker); err == nil {
		t.Fatal("keystrokes after a revoked takeover reached the pane")
	}
	terminalWaitForNoTmuxClients(t, writableSession)
	pausedRuns := terminalRunsSorted(storage)

	// Anything that is not an exact known session is refused, before the upgrade.
	for _, c := range []struct {
		query  string
		status int
	}{
		{"session=" + neighboringSession, http.StatusNotFound},
		{"session=" + missingSession, http.StatusNotFound},
		{"session=" + readOnlySession + "*", http.StatusNotFound},
		{"session==" + readOnlySession, http.StatusNotFound},
		{"session=" + readOnlySession + "-continue", http.StatusNotFound},
		{"session=" + readOnlySession + "&takeOver=1", http.StatusUnprocessableEntity},
	} {
		headers := []string{fmt.Sprintf("Host: 127.0.0.1:%d", port), fmt.Sprintf("Origin: http://127.0.0.1:%d", port), "Authorization: Bearer " + terminalTestToken}
		reply := terminalRawRequest(t, port, terminalHandshake("/api/v1/terminals?"+c.query, headers))
		if reply.status != c.status {
			t.Errorf("rejected terminal query %q: status %d, want %d", c.query, reply.status, c.status)
		}
	}
	if stdout, stderr, _ := terminalTmux("list-clients"); stdout+stderr != "" {
		t.Fatalf("rejected queries left clients attached: %q", stdout+stderr)
	}
	if !reflect.DeepEqual(terminalRunsSorted(storage), pausedRuns) {
		t.Fatal("rejected queries changed the saved runs")
	}

	// A known session upgrades to a WebSocket, as the terminals golden records.
	upgradeHeaders := []string{fmt.Sprintf("Host: 127.0.0.1:%d", port), fmt.Sprintf("Origin: http://127.0.0.1:%d", port), "Authorization: Bearer " + terminalTestToken}
	upgrade := terminalRawRequest(t, port, terminalHandshake("/api/v1/terminals?session="+readOnlySession, upgradeHeaders))
	var recorded struct {
		Transport     string `json:"transport"`
		UpgradeStatus int    `json:"upgradeStatus"`
	}
	if err := json.Unmarshal(terminalGolden(t, "terminal-upgrade.json"), &recorded); err != nil {
		t.Fatalf("terminal-upgrade.json: %v", err)
	}
	if upgrade.status != recorded.UpgradeStatus || !strings.EqualFold(upgrade.header.Get("Upgrade"), recorded.Transport) {
		t.Fatalf("upgrade of a known session: status %d, Upgrade %q; want %d, %q", upgrade.status, upgrade.header.Get("Upgrade"), recorded.UpgradeStatus, recorded.Transport)
	}
	terminalWaitForNoTmuxClients(t, readOnlySession)

	// Detaching the tmux client ends the socket with a normal close, and the session lives on.
	detached := terminalDial(t, port, assistantSession, false)
	terminalWaitForTmuxClientCount(t, assistantSession, 1)
	clientName, _, _ := terminalTmux("list-clients", "-t", "="+assistantSession, "-F", "#{client_name}")
	if strings.TrimSpace(clientName) == "" {
		t.Fatal("could not find the terminal's tmux client")
	}
	if _, _, ok := terminalTmux("detach-client", "-t", strings.TrimSpace(clientName)); !ok {
		t.Fatal("could not detach the terminal's tmux client")
	}
	if code := detached.waitForCloseCode(); code != websocket.StatusNormalClosure {
		t.Fatalf("detached client closed with %d, want 1000", code)
	}
	terminalWaitForNoTmuxClients(t, assistantSession)
	if _, _, ok := terminalTmux("has-session", "-t", "="+assistantSession); !ok {
		t.Fatal("the assistant session died with its detached client")
	}

	// A client still attached when the daemon goes away is ended, and the panes live on.
	// Go's Shutdown does not close hijacked sockets, so the drop is made here.
	shutdownClient := terminalDial(t, port, assistantSession, false)
	terminalWaitForTmuxClientCount(t, assistantSession, 1)
	shutdownClient.disconnectAbruptly()
	terminalWaitForNoTmuxClients(t, assistantSession)
	for _, name := range []string{readOnlySession, writableSession} {
		if _, _, ok := terminalTmux("has-session", "-t", "="+name); !ok {
			t.Fatalf("session %s died with the terminal socket", name)
		}
	}
}
