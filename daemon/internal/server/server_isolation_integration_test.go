//go:build integration

// Tests that the server's whole life, requests and stop included, leaves a running coordinator, its
// agent session and the data folder's bytes as they were. It binds a port, so it runs under the
// integration tag.

package server_test

import (
	"bytes"
	"context"
	"io"
	"maps"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"github.com/robbertmanders/beltline/daemon/internal/server/servertest"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// noEnvironment reads no environment variable, so the agent executables are the defaults.
func noEnvironment(string) string { return "" }

// countingRunner counts every call the coordinator, or anything else, makes of the agent runner, and
// passes it on to the simulated runner it wraps.
type countingRunner struct {
	runner.AgentRunner
	mu     sync.Mutex
	counts map[string]int
}

// note counts one call of name.
func (r *countingRunner) note(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counts[name]++
}

// count is how many calls of name were made.
func (r *countingRunner) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[name]
}

// snapshotCounts copies the call counts.
func (r *countingRunner) snapshotCounts() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	maps.Copy(out, r.counts)
	return out
}

func (r *countingRunner) Start(ctx context.Context, spec runner.SessionSpec) error {
	r.note("start")
	return r.AgentRunner.Start(ctx, spec)
}

func (r *countingRunner) Stop(ctx context.Context, session string) {
	r.note("stop")
	r.AgentRunner.Stop(ctx, session)
}

func (r *countingRunner) EnsureStopped(ctx context.Context, session string) error {
	r.note("ensureStopped")
	return r.AgentRunner.EnsureStopped(ctx, session)
}

func (r *countingRunner) Finish(ctx context.Context, spec runner.SessionSpec) {
	r.note("finish")
	r.AgentRunner.Finish(ctx, spec)
}

func (r *countingRunner) EndContinuing(ctx context.Context, spec runner.SessionSpec) {
	r.note("endContinuing")
	r.AgentRunner.EndContinuing(ctx, spec)
}

func (r *countingRunner) Suspend(ctx context.Context, session string) {
	r.note("suspend")
	r.AgentRunner.Suspend(ctx, session)
}

func (r *countingRunner) Interrupt(ctx context.Context, spec runner.SessionSpec) runner.SessionInterrupt {
	r.note("interrupt")
	return r.AgentRunner.Interrupt(ctx, spec)
}

func (r *countingRunner) Status(ctx context.Context, spec runner.SessionSpec) runner.SessionStatus {
	r.note("status")
	return r.AgentRunner.Status(ctx, spec)
}

// freePort is a loopback port nothing holds at the moment it is read.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }() // the listener is closed at the end of the test; nothing listens after it
	return listener.Addr().(*net.TCPAddr).Port
}

// TestStoppingLeavesCoordinatorWorkSessionsAndDataAlone checks that stopping leaves the coordinator, work sessions and data alone.
func TestStoppingLeavesCoordinatorWorkSessionsAndDataAlone(t *testing.T) {
	root := t.TempDir()
	storage := store.NewStorage(store.Paths{Root: root})
	storage.SeedAgents()
	// Every model the built-in agents name is known and allowed, so the run can start.
	var models []core.ModelChoice
	seen := map[string]bool{"fixture-model": true}
	names := []string{"fixture-model"}
	for _, agent := range core.SampleAgents() {
		if !seen[agent.Model] {
			seen[agent.Model] = true
			names = append(names, agent.Model)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		models = append(models, core.ModelChoice{ID: name, Name: name})
	}
	settings := store.DefaultSettings()
	for _, backend := range core.AllBackend {
		allowed := map[string]struct{}{}
		for _, model := range models {
			allowed[model.ID] = struct{}{}
		}
		settings.AllowedModels[string(backend)] = allowed
		snapshot, err := core.NewModelCatalogSnapshot(backend, backend.Executable(noEnvironment), testkit.ContractTime, models)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		settings.ModelCatalogs[string(backend)] = snapshot
	}
	if err := storage.SaveSettings(settings); err != nil {
		t.Fatalf("settings: %v", err)
	}
	belt, err := storage.SaveNewVersion(core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true))
	if err != nil {
		t.Fatalf("belt: %v", err)
	}
	if err := storage.SaveState(store.NewBeltRecord(store.BeltRecordArgs{Belt: belt, WatchOutcomes: true, ReviewOutcomes: true, Created: testkit.ContractTime})); err != nil {
		t.Fatalf("belt state: %v", err)
	}
	clock := testkit.NewClock(testkit.ContractTime)
	counting := &countingRunner{
		AgentRunner: runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 3600 }, clock.Now,
			func(runner.SessionSpec) *string { return nil }),
		counts: map[string]int{},
	}
	issue := core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 1, Title: "Export", Labels: []string{"agent"}, IsOpen: true}
	c, err := coordinator.New(coordinator.Services{
		Storage: storage, GitHub: github.NewFakeGitHub([]core.Subject{issue}, nil, nil, nil), Jira: jira.NewFakeJira(nil, nil, nil),
		Runner: counting, Workspaces: runner.FolderWorkspaces{}, Clock: clock, Env: noEnvironment,
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}
	t.Cleanup(c.Close)
	c.Idle()
	// Tick until the run's first agent session is live, then a few more times so the folder settles.
	for i := 0; i < 100 && counting.count("start") == 0; i++ {
		c.Tick(context.Background())
		c.Idle()
	}
	for i := 0; i < 5; i++ {
		c.Tick(context.Background())
		c.Idle()
	}
	if counting.count("start") == 0 {
		t.Fatal("no agent session started")
	}
	session := ""
	for _, run := range c.ReadRuns() {
		for _, attempt := range run.Attempts {
			if attempt.Session != nil {
				session = *attempt.Session
			}
		}
	}
	if session == "" || !counting.AgentRunner.(*runner.SimulatedRunner).IsRunning(session) {
		t.Fatalf("the run's session %q is not running", session)
	}

	files := testkit.FolderSnapshot(t, root)
	calls := counting.snapshotCounts()
	runs := c.ReadRuns()

	port := freePort(t)
	srv := server.NewBeltlineServer(servertest.ContractToken, "dev", c, port, filepath.Join(t.TempDir(), "missing-web"), routes.Dependencies{})
	started, err := srv.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := "http://127.0.0.1:" + strconv.Itoa(started)
	expectStatus(t, client, base+"/api/v1/version", "Bearer "+servertest.ContractToken, 200)
	expectStatus(t, client, base+"/api/v1/missing", "Bearer "+servertest.ContractToken, 404)
	expectStatus(t, client, base+"/api/v1/version", "", 401)
	srv.Stop(context.Background())
	srv.Stop(context.Background())

	if got := testkit.FolderSnapshot(t, root); !sameFiles(got, files) {
		t.Error("the server changed the data folder")
	}
	if got := counting.snapshotCounts(); !maps.Equal(got, calls) {
		t.Errorf("the server touched the coordinator or sessions: %v, want %v", got, calls)
	}
	for _, call := range []string{"stop", "ensureStopped", "finish", "endContinuing", "suspend", "interrupt"} {
		if counting.count(call) != 0 {
			t.Errorf("%s was called %d times", call, counting.count(call))
		}
	}
	if !counting.AgentRunner.(*runner.SimulatedRunner).IsRunning(session) {
		t.Error("the session stopped")
	}
	if got := c.ReadRuns(); len(got) != len(runs) {
		t.Errorf("the runs changed: %d, want %d", len(got), len(runs))
	}
}

// expectStatus fails unless a GET of url with the authorization header answers status.
func expectStatus(t *testing.T, client *http.Client, url, authorization string, status int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = res.Body.Close() }() // the response is read; a close error has nothing left to report
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != status {
		t.Errorf("GET %s: status %d, want %d", url, res.StatusCode, status)
	}
}

// sameFiles reports whether two snapshots hold the same files with the same bytes.
func sameFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for path, data := range a {
		if other, ok := b[path]; !ok || !bytes.Equal(other, data) {
			return false
		}
	}
	return true
}
