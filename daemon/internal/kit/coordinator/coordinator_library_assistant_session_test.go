// Coordinator tests of the library assistant's conversations: the conversations that start,
// reconcile, stop and reload the assistant, with the admission they are started under. The pure
// tests are in coordinator_library_assistant_test.go.

package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// coordConversations records conversations instead of starting them. Its
// use is LibraryAssistantTests' conversations and PendingConversation.
// Work sessions are never started through it, so Status reports them gone.
type coordConversations struct {
	runner.AgentRunner
	mu    sync.Mutex
	specs []runner.ConversationSpec
	live  map[string]bool
	// launched receives each spec as StartConversation records it, before any launch gate.
	launched chan runner.ConversationSpec
	// probe, when set, holds the first ConversationAlive until it is closed. probeEntered
	// is closed when that first call arrives.
	probe        chan struct{}
	probeEntered chan struct{}
	probeOnce    sync.Once
	// launch, when set, holds StartConversation after it records the spec, until it is closed.
	launch chan struct{}
}

// newH5m6Conversations is a conversation fake with nothing live.
func newH5m6Conversations() *coordConversations {
	return &coordConversations{live: map[string]bool{}, launched: make(chan runner.ConversationSpec, 16)}
}

// Start refuses: the fake only starts conversations.
func (r *coordConversations) Start(context.Context, runner.SessionSpec) error {
	return core.NewFailure("Not a work session")
}

// Status reports every work session gone.
func (r *coordConversations) Status(context.Context, runner.SessionSpec) runner.SessionStatus {
	return runner.SessionStatus{Kind: runner.SessionStatusGone}
}

// Finish does nothing.
func (r *coordConversations) Finish(context.Context, runner.SessionSpec) {}

// Stop ends a conversation.
func (r *coordConversations) Stop(_ context.Context, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.live, session)
}

// AttachCommand is the attach command for a live conversation, nil otherwise.
func (r *coordConversations) AttachCommand(_ context.Context, session string, _ bool) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live[session] {
		return []string{"tmux", "attach", "-t", session}
	}
	return nil
}

// ConversationAlive reports whether a conversation is live; the first call waits for the probe gate.
func (r *coordConversations) ConversationAlive(_ context.Context, session string) bool {
	if r.probe != nil {
		r.probeOnce.Do(func() {
			close(r.probeEntered)
			<-r.probe
		})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[session]
}

// StartConversation records the spec, waits for the launch gate, then makes the conversation live.
func (r *coordConversations) StartConversation(_ context.Context, spec runner.ConversationSpec) error {
	r.mu.Lock()
	r.specs = append(r.specs, spec)
	r.mu.Unlock()
	r.launched <- spec
	if r.launch != nil {
		<-r.launch
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live[spec.Name] = true
	return nil
}

// Specs returns the conversations started so far, in order.
func (r *coordConversations) Specs() []runner.ConversationSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]runner.ConversationSpec(nil), r.specs...)
}

// assistantStorage is a data folder with the model fixtures and no agents or belts
// (the coordinator's fixture settings), and the helper agent when withHelper is set.
func assistantStorage(t *testing.T, withHelper bool) store.Storage {
	t.Helper()
	storage := unseededStorage(t)
	if err := storage.SaveSettings(modelFixtureSettings()); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if withHelper {
		if _, err := storage.SaveNewAgentVersion(assistantHelper(1)); err != nil {
			t.Fatalf("save helper: %v", err)
		}
	}
	return storage
}

// assistantCoordinator builds a coordinator over storage with the given runner and ids.
// `Coordinator(storage:github:jira:runner:workspaces:)`). It is closed when the test ends.
func assistantCoordinator(t *testing.T, storage store.Storage, sim runner.AgentRunner, ids IDs) *Coordinator {
	t.Helper()
	clock := testkit.NewClock(testkit.ContractTime)
	coord, err := New(Services{
		Storage: storage, GitHub: github.NewFakeGitHub(nil, nil, nil, nil), Jira: jira.NewFakeJira(nil, nil, nil),
		Runner: sim, Workspaces: runner.FolderWorkspaces{}, Clock: clock, IDs: ids,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { coord.Close() })
	// New starts the startup probe on a goroutine. The test waits for it here, unless
	// the probe is held open on purpose (a blocking conversation).
	if conversations, ok := sim.(*coordConversations); !ok || conversations.probe == nil {
		coord.Idle()
	}
	return coord
}

// coordReconcile runs one tick's library steps: the library's reload of what changed on disk, then the assistant's
// reconciliation. New starts a startup probe on its own goroutine, and a reconcile
// that finds one in flight does nothing, so this waits for the probe in the same turn.
func coordReconcile(c *Coordinator) {
	locked(c, func() {
		c.library.reconcileFiles()
		c.assistant.waitForProbe()
		c.assistant.Reconcile(context.Background())
	})
}

// stopAssistant stops the assistant's conversation.
func stopAssistant(c *Coordinator) {
	locked(c, func() { c.assistant.Stop(context.Background()) })
}

// coordWrite writes a library file as the assistant would, a moment later than what is there.
func coordWrite(t *testing.T, root, path string, data []byte) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	later := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(full, later, later); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// assistantAdmission reads the saved admission record of the data folder.
func assistantAdmission(t *testing.T, root string) LibraryAssistantAdmission {
	t.Helper()
	var record LibraryAssistantAdmission
	if err := store.NewStorage(store.Paths{Root: root}).Read(filepath.Join(root, "assistant", "admission.json"), &record); err != nil {
		t.Fatalf("read admission: %v", err)
	}
	return record
}

// hasText reports whether text contains want, failing the test otherwise.
func hasText(t *testing.T, what, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Errorf("%s lacks %q", what, want)
	}
}

// assistantSetup is the setup of the conversation tests: a data folder with the fixture
// settings and the helper agent, and the conversation fake.
func assistantSetup(t *testing.T) (store.Storage, *coordConversations) {
	t.Helper()
	return assistantStorage(t, true), newH5m6Conversations()
}

// TestItWorksInTheDataFolderAndKeepsItsPromptApart: a conversation starts in the data folder with
// the assistant's configuration and a prompt written apart from the folder.
func TestItWorksInTheDataFolderAndKeepsItsPromptApart(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		t.Run(string(backend), func(t *testing.T) {
			t.Parallel()
			storage, conversations := assistantSetup(t)
			root := storage.Paths.Root
			c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
			locked(c, func() {
				c.changeSettings(func(s *store.Settings) {
					s.ConfigurationBackend = backend
					s.ConfigurationModel = "gpt-test"
				})
			})
			if err := c.StartAssistant(context.Background(), "Add a belt for small tasks"); err != nil {
				t.Fatalf("StartAssistant: %v", err)
			}
			specs := conversations.Specs()
			if len(specs) != 1 {
				t.Fatalf("conversations = %d", len(specs))
			}
			spec := specs[0]
			locked(c, func() {
				if spec.Name != c.assistant.session || spec.Directory != root || spec.Backend != backend || spec.Model != "gpt-test" {
					t.Errorf("spec = name %q (session %q), directory %q, backend %q, model %q",
						spec.Name, c.assistant.session, spec.Directory, spec.Backend, spec.Model)
				}
				if filepath.Dir(spec.PromptFile) == root {
					t.Errorf("the prompt file %q sits in the data folder", spec.PromptFile)
				}
				hasText(t, "the prompt", spec.Prompt, "Add a belt for small tasks")
				hasText(t, "the prompt", spec.Prompt, "- agents/helper/v1.md: ")
				if !c.assistant.running || c.assistant.command == nil {
					t.Errorf("running %v, command %v", c.assistant.running, c.assistant.command)
				}
			})
			if err := c.StartAssistant(context.Background(), "Again"); err == nil {
				t.Errorf("a second conversation started while one runs")
			}
		})
	}
}

// TestWhatItWritesIsLiveAndChecked: the belts the assistant writes reach the library at once, and a version
// that cannot be read is reported with what is missing until a fixed version replaces it.
func TestWhatItWritesIsLiveAndChecked(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	if err := c.StartAssistant(context.Background(), "Add a belt"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	coordWrite(t, root, "belts/tasks/v1.json", encodeFor(t, assistantBelt(t, 1)))
	coordReconcile(c)
	if ids := beltIDs(c); len(ids) != 1 || ids[0] != "tasks" {
		t.Fatalf("belts = %v", ids)
	}
	locked(c, func() {
		if len(c.assistant.problems) != 0 {
			t.Errorf("problems = %v", c.assistant.problems)
		}
	})

	// A version that cannot be read is reported with what is missing.
	coordWrite(t, root, "belts/tasks/v2.json", withoutStageName(t, assistantBelt(t, 2)))
	coordReconcile(c)
	locked(c, func() {
		if len(c.assistant.problems) != 1 || !strings.HasPrefix(c.assistant.problems[0], "belts/tasks/v2.json:") {
			t.Errorf("problems = %v", c.assistant.problems)
		}
	})

	// Fixed with the next version, the problem goes away.
	fixed := assistantBelt(t, 3)
	fixed.Name = "Small tasks"
	coordWrite(t, root, "belts/tasks/v3.json", encodeFor(t, fixed))
	coordReconcile(c)
	beltNames := beltNamesOf(c)
	if len(beltNames) == 0 || beltNames[0] != "Small tasks" {
		t.Errorf("belt names = %v", beltNames)
	}
	locked(c, func() {
		for _, problem := range c.assistant.problems {
			if !strings.HasPrefix(problem, "belts/tasks/v2.json") {
				t.Errorf("a problem remains after the fix: %v", c.assistant.problems)
			}
		}
	})
}

// withoutStageName is the belt's file with its first stage's name removed, as a
// version the assistant wrote without one looks.
func withoutStageName(t *testing.T, belt core.Belt) []byte {
	t.Helper()
	const mark = "stage-name-to-remove"
	belt.Stages[0].Name = mark
	text := string(encodeFor(t, belt))
	for _, cut := range []string{`,"name":"` + mark + `"`, `"name":"` + mark + `",`} {
		if strings.Contains(text, cut) {
			return []byte(strings.Replace(text, cut, "", 1))
		}
	}
	t.Fatalf("the stage name is not in the encoded belt")
	return nil
}

// beltIDs lists the ids of the belts the coordinator loaded.
func beltIDs(c *Coordinator) []string {
	var ids []string
	for _, record := range c.Belts() {
		ids = append(ids, record.Belt.ID)
	}
	return ids
}

// beltNamesOf lists the names of the belts the coordinator loaded.
func beltNamesOf(c *Coordinator) []string {
	var names []string
	for _, record := range c.Belts() {
		names = append(names, record.Belt.Name)
	}
	return names
}

// TestAStoppedAssistantReloadsOnTickWhenFilesChange: after its conversation stops, the assistant's changes
// are still picked up on the coordinator's tick; one that never started watches nothing.
func TestAStoppedAssistantReloadsOnTickWhenFilesChange(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	// The library reloads for any edit on disk, a conversation or not: a never-started assistant's folder too.
	coordWrite(t, root, "belts/tasks/v1.json", encodeFor(t, assistantBelt(t, 1)))
	c.Tick(context.Background())
	c.Idle()
	if ids := beltIDs(c); len(ids) != 1 || ids[0] != "tasks" {
		t.Fatalf("a never-started assistant's folder was not reloaded: belts = %v", ids)
	}

	if err := c.StartAssistant(context.Background(), "Add a belt"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	stopAssistant(c)
	locked(c, func() {
		if c.assistant.running {
			t.Fatalf("the assistant still runs after stop")
		}
	})
	coordWrite(t, root, "belts/tasks/v2.json", encodeFor(t, assistantBelt(t, 2)))
	c.Tick(context.Background())
	c.Idle()
	if ids := beltIDs(c); len(ids) != 1 || ids[0] != "tasks" {
		t.Fatalf("belts after the tick = %v", ids)
	}
}

// TestStoppingEndsTheConversationAndARestartFindsALiveOne: a restart finds the live conversation and can stop it.
func TestStoppingEndsTheConversationAndARestartFindsALiveOne(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	if err := c.StartAssistant(context.Background(), "Help"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	restarted := assistantCoordinator(t, storage, conversations, &SequenceIDs{Next: 100})
	coordReconcile(restarted)
	locked(restarted, func() {
		if !restarted.assistant.running || restarted.assistant.command == nil {
			t.Errorf("restart: running %v, command %v", restarted.assistant.running, restarted.assistant.command)
		}
	})
	stopAssistant(restarted)
	locked(restarted, func() {
		if restarted.assistant.running || restarted.assistant.command != nil {
			t.Errorf("after stop: running %v, command %v", restarted.assistant.running, restarted.assistant.command)
		}
	})
	coordReconcile(c)
	locked(c, func() {
		if c.assistant.running {
			t.Errorf("the first coordinator still sees a running conversation")
		}
	})
}

// TestAdmissionSurvivesRevocationButCannotAuthorizeANewConversation: a live conversation stays after its model
// is revoked, but a new one needs the model allowed again.
func TestAdmissionSurvivesRevocationButCannotAuthorizeANewConversation(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	ctx := context.Background()
	if err := c.StartAssistant(ctx, "First"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	var first string
	locked(c, func() { first = c.assistant.session })
	if err := c.SetModelAllowed("gpt-test", core.BackendPi, false); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	coordReconcile(c)
	locked(c, func() {
		if !c.assistant.running {
			t.Errorf("revocation stopped the live conversation")
		}
	})

	restarted := assistantCoordinator(t, storage, conversations, &SequenceIDs{Next: 100})
	coordReconcile(restarted)
	locked(restarted, func() {
		if !restarted.assistant.running || restarted.assistant.session != first {
			t.Errorf("restart: running %v, session %q, want %q", restarted.assistant.running, restarted.assistant.session, first)
		}
	})
	stopAssistant(restarted)
	if err := restarted.StartAssistant(ctx, "Denied"); err == nil {
		t.Errorf("a revoked model started a conversation")
	}
	if n := len(conversations.Specs()); n != 1 {
		t.Errorf("conversations after the denied start = %d", n)
	}
	if err := restarted.SetModelAllowed("gpt-test", core.BackendPi, true); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	if err := restarted.StartAssistant(ctx, "Second"); err != nil {
		t.Fatalf("StartAssistant after allowing: %v", err)
	}
	locked(restarted, func() {
		if restarted.assistant.session == first {
			t.Errorf("the second conversation reused session %q", first)
		}
	})
	if n := len(conversations.Specs()); n != 2 {
		t.Errorf("conversations = %d, want 2", n)
	}
}

// TestAdmittedAssistantUsesCurrentPolicyForChangedFiles: a file the admitted assistant changes is checked against
// the model policy as it is now, not as it was when the conversation started.
func TestAdmittedAssistantUsesCurrentPolicyForChangedFiles(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	if err := c.StartAssistant(context.Background(), "Change helper"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	if err := c.SetModelAllowed("gpt-test", core.BackendPi, false); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	changed := assistantHelper(2)
	changed.Model = "gpt-test"
	data := []byte(core.AgentFileWrite(changed))
	coordWrite(t, root, "agents/helper/v2.md", data)
	coordReconcile(c)
	locked(c, func() {
		if !c.assistant.running {
			t.Errorf("the conversation stopped")
		}
		found := slices.ContainsFunc(c.assistant.problems, func(problem string) bool {
			return strings.Contains(problem, "gpt-test") && strings.Contains(problem, "not allowed")
		})
		if !found {
			t.Errorf("problems = %v", c.assistant.problems)
		}
	})
	if err := c.SetModelAllowed("gpt-test", core.BackendPi, true); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	locked(c, func() {
		if len(c.assistant.problems) != 0 {
			t.Errorf("problems after allowing = %v", c.assistant.problems)
		}
	})
	written, err := os.ReadFile(filepath.Join(root, "agents", "helper", "v2.md"))
	if err != nil || string(written) != string(data) {
		t.Errorf("the agent file changed: %v", err)
	}
}

// TestSurvivingLegacySessionIsNotAttributedToSidebarOrReused: a conversation the folder started before admissions
// existed is found and attributed to nothing, and it is never reused for a new one.
func TestSurvivingLegacySessionIsNotAttributedToSidebarOrReused(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	legacyName := libraryAssistantSession(root)
	if err := conversations.StartConversation(context.Background(), runner.ConversationSpec{
		Name: legacyName, SessionID: "legacy", Backend: core.BackendClaude, Model: "legacy-model",
		Prompt: "Existing conversation", Directory: root, PromptFile: filepath.Join(root, "legacy.md"),
	}); err != nil {
		t.Fatalf("legacy conversation: %v", err)
	}
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	locked(c, func() {
		c.changeSettings(func(s *store.Settings) { s.AllowedModels = map[string]map[string]struct{}{} })
	})
	coordReconcile(c)
	locked(c, func() {
		if !c.assistant.running || c.assistant.session != legacyName {
			t.Errorf("running %v, session %q, want %q", c.assistant.running, c.assistant.session, legacyName)
		}
		if _, err := os.Stat(filepath.Join(root, "assistant", "admission.json")); !os.IsNotExist(err) {
			t.Errorf("a legacy conversation wrote an admission: %v", err)
		}
	})
	stopAssistant(c)
	if err := c.StartAssistant(context.Background(), "New denied"); err == nil {
		t.Errorf("a new conversation started with no models allowed")
	}
	if n := len(conversations.Specs()); n != 1 {
		t.Errorf("conversations = %d, want only the legacy one", n)
	}
}

// TestAdmissionWriteFailurePreventsAssistantLaunch: when the admission cannot be written, nothing starts.
func TestAdmissionWriteFailurePreventsAssistantLaunch(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	if err := os.WriteFile(filepath.Join(root, "assistant"), []byte("not a folder"), 0o644); err != nil {
		t.Fatalf("block assistant folder: %v", err)
	}
	if err := c.StartAssistant(context.Background(), "Denied"); err == nil {
		t.Errorf("the assistant started although its folder is a file")
	}
	if n := len(conversations.Specs()); n != 0 {
		t.Errorf("conversations = %d", n)
	}
}

// TestAssistantKeepsTheExactContextIdentifier: a model's long-context identifier is the one started and saved,
// even after the catalog that named its context size changes.
func TestAssistantKeepsTheExactContextIdentifier(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	contextTokens := 1_000_000
	choice := core.ModelChoice{ID: "claude-opus-5-5[1m]", Name: "Opus 5.5", ContextTokens: &contextTokens}
	plain := core.ModelChoice{ID: "claude-opus-5-5", Name: "Opus 5.5"}
	snapshot, err := core.NewModelCatalogSnapshot(core.BackendClaude, core.BackendClaude.Executable(c.env), testkit.ContractTime, []core.ModelChoice{choice, plain})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if err := c.SaveModelCatalog(snapshot); err != nil {
		t.Fatalf("SaveModelCatalog: %v", err)
	}
	if err := c.SetModelAllowed(choice.ID, core.BackendClaude, true); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	thinking := core.ThinkingHigh
	if _, err := c.ConfigureAssistant(AssistantConfiguration{Backend: core.BackendClaude, Model: choice.ID, Thinking: &thinking}); err != nil {
		t.Fatalf("ConfigureAssistant: %v", err)
	}
	if err := c.StartAssistant(context.Background(), "Use the long context"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	specs := conversations.Specs()
	if len(specs) != 1 {
		t.Fatalf("conversations = %d", len(specs))
	}
	spec := specs[0]
	if spec.Model != "claude-opus-5-5[1m]" || spec.Thinking == nil || *spec.Thinking != core.ThinkingHigh {
		t.Errorf("spec model %q, thinking %v", spec.Model, spec.Thinking)
	}
	hasText(t, "the prompt", spec.Prompt, "claude-opus-5-5[1m] (Opus 5.5 - 1M)")

	if err := c.SetModelAllowed(choice.ID, core.BackendClaude, false); err != nil {
		t.Fatalf("SetModelAllowed: %v", err)
	}
	shrunk := 250_000
	snapshot, err = core.NewModelCatalogSnapshot(core.BackendClaude, core.BackendClaude.Executable(c.env), testkit.ContractTime,
		[]core.ModelChoice{{ID: "claude-opus-5-5", Name: "Opus 5.5", ContextTokens: &shrunk}})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if err := c.SaveModelCatalog(snapshot); err != nil {
		t.Fatalf("SaveModelCatalog: %v", err)
	}
	if saved := assistantAdmission(t, root); saved.Model != "claude-opus-5-5[1m]" {
		t.Errorf("saved admission model = %q", saved.Model)
	}
	if n := conversations.Specs(); len(n) == 0 || n[0].Model != "claude-opus-5-5[1m]" {
		t.Errorf("the started conversation's model changed")
	}
}

// TestCodexAssistantRequestsTheSelectedEffort: a Codex conversation asks for the reasoning effort the assistant selected.
func TestCodexAssistantRequestsTheSelectedEffort(t *testing.T) {
	t.Parallel()
	for _, thinking := range []core.Thinking{core.ThinkingXHigh, core.ThinkingMax} {
		t.Run(string(thinking), func(t *testing.T) {
			t.Parallel()
			storage, conversations := assistantSetup(t)
			c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
			level := thinking
			if _, err := c.ConfigureAssistant(AssistantConfiguration{Backend: core.BackendCodex, Model: "gpt-test", Thinking: &level}); err != nil {
				t.Fatalf("ConfigureAssistant: %v", err)
			}
			if err := c.StartAssistant(context.Background(), "Use this effort"); err != nil {
				t.Fatalf("StartAssistant: %v", err)
			}
			specs := conversations.Specs()
			if len(specs) != 1 {
				t.Fatalf("conversations = %d", len(specs))
			}
			spec := specs[0]
			if spec.Backend != core.BackendCodex || spec.Model != "gpt-test" || spec.Thinking == nil || *spec.Thinking != thinking {
				t.Fatalf("spec = backend %q, model %q, thinking %v", spec.Backend, spec.Model, spec.Thinking)
			}
			args := runner.NewCodexAdapter(nil).ConversationArguments(spec.SessionID, spec.Name, spec.Model, spec.Thinking)
			if !slices.Contains(args, "model_reasoning_effort="+string(thinking)) {
				t.Errorf("arguments %v lack the effort %q", args, thinking)
			}
		})
	}
}

// TestItStartsWithTheProblemsAndTheCheck: the assistant starts knowing what is wrong already, with the
// built-ins to copy and a command to check its work.
func TestItStartsWithTheProblemsAndTheCheck(t *testing.T) {
	t.Parallel()
	storage, conversations := assistantSetup(t)
	root := storage.Paths.Root
	broken := assistantBelt(t, 1)
	watch := core.Stage{
		ID: "watch", Type: core.StageTypeWatch, Name: "watch",
		Outcomes: []core.Outcome{{ID: "merged", Name: "merged", When: "merged", Tone: core.ToneGood, Then: core.NewThen(core.ThenArgs{Action: core.ThenActionNext})}},
	}
	broken.Stages = append([]core.Stage{watch}, broken.Stages...)
	coordWrite(t, root, "belts/tasks/v1.json", encodeFor(t, broken))
	c := assistantCoordinator(t, storage, conversations, &SequenceIDs{})
	c.SetAssistantCheckCommand([]string{"/Apps/Belt line.app/Beltline", "--check-library"})
	if err := c.StartAssistant(context.Background(), "Fix my belt"); err != nil {
		t.Fatalf("StartAssistant: %v", err)
	}
	specs := conversations.Specs()
	if len(specs) != 1 {
		t.Fatalf("conversations = %d", len(specs))
	}
	spec := specs[0]
	problems := LibraryAssistantCheck(storage.Paths)
	if len(problems) == 0 {
		t.Fatalf("the broken belt has no problems")
	}
	for _, problem := range problems {
		hasText(t, "the prompt", spec.Prompt, "- "+problem)
	}
	hasText(t, "the prompt", spec.Prompt, "run `'/Apps/Belt line.app/Beltline' '--check-library' '"+root+"'`")
	if _, err := os.Stat(filepath.Join(root, "assistant", "built-in")); err == nil {
		t.Error("agents were copied for the assistant; the app has none of its own")
	}
	for path := range store.LibraryFilesRead(storage.Paths) {
		if strings.HasPrefix(path, "assistant/") {
			t.Errorf("the library lists the assistant's own file %q", path)
		}
	}
}
