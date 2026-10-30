package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// LibraryAssistantAdmission is the model admission the conversation was started
// with, saved in the data folder.
type LibraryAssistantAdmission struct {
	SchemaVersion int          `json:"schemaVersion"`
	Identity      string       `json:"identity"`
	Session       string       `json:"session"`
	Runner        core.Backend `json:"runner"`
	Model         string       `json:"model"`
}

// LibraryAssistant is a conversation with an agent that changes your belts and
// agents directly, in the data folder. There is one at a time, in its own tmux
// session, so a restart finds it again; the library reloads whenever it writes a
// file. A part of Coordinator; its methods run with c.mu held and suspend
// through c.Suspend around the runner calls.
type LibraryAssistant struct {
	// c is the coordinator the assistant belongs to (the back pointer).
	c *Coordinator
	// session is the tmux session, one per data folder, so tests and other folders never share it.
	session string
	// currentAdmission reports the allowed models now.
	currentAdmission func() core.ModelAdmission
	// admission is the saved admission of the running conversation, nil when none.
	admission *LibraryAssistantAdmission
	// running is set while the conversation exists.
	running bool
	// busy is set while starting or stopping it.
	busy bool
	// command attaches a terminal to the conversation; nil when there is none.
	command []string
	// problems is what is wrong in the files it changed since it started.
	problems []string
	// checkCommand checks the library from a shell, such as the app's own executable
	// with --check-library; nil when there is none to run.
	checkCommand []string
	paths        store.Paths
	runner       runner.AgentRunner
	// watch reads the library folders' stamps for the changes a conversation makes.
	watch *store.LibraryWatch
	// started holds the library's files when the conversation started; nil while
	// there is no conversation to watch.
	started map[string]time.Time
	// seen holds the library's files when last looked at.
	seen map[string]time.Time
	// probing is set while the startup probe runs.
	probing bool
	// probeDone is closed when the probe that is running ends; Start waits on it.
	probeDone chan struct{}
	// startupProbeFinished is set once the coordinator's one startup reconciliation has run.
	startupProbeFinished bool
	// startupDone is closed the first time a probe finishes, so a test can wait for the startup reconciliation.
	startupDone chan struct{}
}

// newLibraryAssistant builds the assistant over paths and runner.
// It reads the saved admission: a conversation the folder started earlier keeps its session name.
// Lock: runs during New, before the coordinator is shared.
func newLibraryAssistant(c *Coordinator, paths store.Paths, runner runner.AgentRunner) LibraryAssistant {
	a := LibraryAssistant{
		c: c, paths: paths, runner: runner, watch: store.NewLibraryWatch(),
		currentAdmission: func() core.ModelAdmission { return c.currentModelAdmission() },
		probeDone:        make(chan struct{}),
		startupDone:      make(chan struct{}),
	}
	var saved LibraryAssistantAdmission
	if err := store.NewStorage(paths).Read(a.admissionFile(), &saved); err == nil {
		a.admission = &saved
	}
	if a.admission != nil && a.admission.SchemaVersion == 1 {
		a.session = a.admission.Session
	} else {
		a.session = libraryAssistantSession(paths.Root)
	}
	return a
}

// SetAssistantCheckCommand sets the command that checks the library, which
// LaunchSetup passes.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SetAssistantCheckCommand(checkCommand []string) {
	c.Lock()
	defer c.EndTurn()
	c.assistant.checkCommand = checkCommand
}

// libraryAssistantSession is a stable name for a folder's conversation: FNV-1a of
// its path, since Go's hash of a string changes per launch.
// Lock: none (pure).
func libraryAssistantSession(root string) string {
	hash := uint64(0xcbf29ce484222325)
	for _, b := range []byte(filepath.Clean(root)) {
		hash = (hash ^ uint64(b)) * 0x100000001b3
	}
	return runner.TmuxSessionName([]string{"assistant", strconv.FormatUint(hash, 36)})
}

// admissionFile is the file the conversation's admission is saved in.
// Lock: runs with c.mu held.
func (a *LibraryAssistant) admissionFile() string {
	return filepath.Join(a.paths.Root, "assistant", "admission.json")
}

// PromptFile is the file the assistant's prompt is written to.
// Lock: part of Coordinator; runs with c.mu held.
func (a *LibraryAssistant) PromptFile() string {
	return filepath.Join(a.paths.Root, "assistant", "prompt.md")
}

// LibraryAssistantCheck is what `--check-library` prints: what is wrong in the
// newest version of every belt and agent in the folder, under its saved model
// policy. The settings' allowed models
// read the process environment for the agent executables, as the app does.
// Lock: none; it reads the folder only.
func LibraryAssistantCheck(paths store.Paths) []string {
	admission := store.NewStorage(paths).LoadSettings().ModelAdmission(os.Getenv)
	return core.LibraryCheckProblems(store.LibraryFilesRead(paths), &admission)
}

// checkLine is the check command as one line for the shell, run on this folder, or nil.
// Lock: runs with c.mu held.
func (a *LibraryAssistant) checkLine() *string {
	if a.checkCommand == nil {
		return nil
	}
	words := append(append([]string{}, a.checkCommand...), a.paths.Root)
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, libraryAssistantQuote(word))
	}
	line := strings.Join(quoted, " ")
	return &line
}

// libraryAssistantQuote quotes one word for the shell.
// Lock: none (pure).
func libraryAssistantQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// Reconcile checks whether the conversation is still there, and what it changed since last time.
// Lock: part of Coordinator; runs with c.mu held. Suspends: runner.conversationAlive,
// runner.attachCommand.
func (a *LibraryAssistant) Reconcile(ctx context.Context) {
	if a.busy || a.probing {
		return
	}
	a.probing = true
	a.probeDone = make(chan struct{})
	defer a.endProbe()

	session := a.session
	var alive bool
	a.c.Suspend(func() { alive = a.runner.ConversationAlive(ctx, session) })
	if a.busy {
		return
	}
	wasRunning := a.running
	if alive != a.running {
		a.running = alive
		// A conversation found after a restart is checked from now on.
		if alive && a.started == nil {
			a.started = a.watch.Stamps(a.paths)
			a.seen = a.started
		}
	}
	var attached []string
	if alive {
		// The attach command names the session, which keeps its name while it lives, so a
		// conversation seen alive at the last check keeps the command it had: asking the
		// runner again is one more tmux call per tick for the same answer.
		if wasRunning && a.command != nil {
			attached = a.command
		} else {
			a.c.Suspend(func() { attached = a.runner.AttachCommand(ctx, session, false) })
		}
	}
	if !assistantStringsEqual(attached, a.command) {
		a.command = attached
	}
	a.noticeChanges(false)
}

// endProbe ends a startup or reconciliation probe: it wakes anything waiting for it
// .
// Lock: runs with c.mu held.
func (a *LibraryAssistant) endProbe() {
	a.probing = false
	if !a.startupProbeFinished {
		a.startupProbeFinished = true
		close(a.startupDone)
	}
	close(a.probeDone)
}

// waitForProbe waits for a probe in flight to end, so Start does not mistake its
// temporarily false running value for a new admission. Lock: runs with c.mu held;
// suspends while a probe runs.
func (a *LibraryAssistant) waitForProbe() {
	for a.probing {
		done := a.probeDone
		a.c.Suspend(func() { <-done })
	}
}

// noticeChanges checks what the assistant changed since its conversation started
// . The library reloads on its own, in reconcileFiles.
// Lock: runs with c.mu held.
func (a *LibraryAssistant) noticeChanges(force bool) {
	if a.started == nil {
		return
	}
	now := a.watch.Stamps(a.paths)
	filesChanged := !sameLibraryStamps(now, a.seen)
	if !force && !filesChanged {
		return
	}
	a.seen = now
	changed := map[string]struct{}{}
	for _, path := range store.LibraryFilesChanged(a.started, now) {
		changed[path] = struct{}{}
	}
	admission := a.currentAdmission()
	found := core.LibraryCheckChangedProblems(store.LibraryFilesRead(a.paths), changed, &admission)
	if !assistantStringsEqual(found, a.problems) {
		a.problems = found
	}
}

// Start starts a conversation about the library with the given selection.
// Lock: part of Coordinator; runs with c.mu held. Suspends: the startup probe wait,
// reconcile, runner.startConversation, runner.attachCommand.
func (a *LibraryAssistant) Start(ctx context.Context, backend core.Backend, model string, thinking *core.Thinking, request string) error {
	a.waitForProbe()
	a.Reconcile(ctx)
	if a.busy || a.running {
		return core.NewFailure("The assistant is already running")
	}
	a.busy = true
	defer func() { a.busy = false }()
	before := a.watch.Stamps(a.paths)
	a.started = before
	a.seen = before
	a.problems = []string{}
	policy := a.currentAdmission()
	if warning := core.ModelPolicyWarning(model, backend, "Assistant", policy, nil); warning != nil {
		return core.NewFailure(*warning)
	}
	identity := a.c.ids.UUID()
	name := libraryAssistantSession(a.paths.Root) + "-" + strings.ToLower(identity)
	record := LibraryAssistantAdmission{SchemaVersion: 1, Identity: identity, Session: name, Runner: backend, Model: model}
	if err := store.NewStorage(a.paths).Write(record, a.admissionFile()); err != nil {
		return err
	}
	a.admission = &record
	a.session = name

	library := store.LibraryFilesRead(a.paths)
	found := core.LibraryCheckProblems(library, &policy)
	lines := make([]string, 0, len(core.AllBackend))
	for _, backend := range core.AllBackend {
		choices := make([]string, 0)
		for _, choice := range policy.Choices(backend) {
			choices = append(choices, choice.ID+" ("+choice.DisplayLabel()+")")
		}
		lines = append(lines, backend.Label()+": "+strings.Join(choices, ", "))
	}
	prompt := core.AssistantPromptPrompt(library, request, found, a.checkLine()) +
		"\n\nCurrent allowed models (explicit model required):\n" + strings.Join(lines, "\n")
	spec := runner.ConversationSpec{
		Name: name, SessionID: identity, Backend: backend,
		Model:      policy.RunnerModel(model, thinking, backend),
		Thinking:   thinking,
		Prompt:     prompt,
		Directory:  a.paths.Root,
		PromptFile: a.PromptFile(),
	}
	var err error
	a.c.Suspend(func() { err = a.runner.StartConversation(ctx, spec) })
	if err != nil {
		_ = os.Remove(a.admissionFile())
		a.admission = nil
		return err
	}
	a.running = true
	var command []string
	a.c.Suspend(func() { command = a.runner.AttachCommand(ctx, name, false) })
	a.command = command
	return nil
}

// Stop stops the conversation.
// Lock: part of Coordinator; runs with c.mu held. Suspends: runner.stop, runner.conversationAlive.
func (a *LibraryAssistant) Stop(ctx context.Context) {
	if a.busy {
		return
	}
	a.busy = true
	defer func() { a.busy = false }()
	session := a.session
	a.c.Suspend(func() { a.runner.Stop(ctx, session) })
	var alive bool
	a.c.Suspend(func() { alive = a.runner.ConversationAlive(ctx, session) })
	a.running = alive
	if !alive {
		a.command = nil
	}
	a.noticeChanges(false)
}

// StartAssistant starts a conversation about your belts and agents with the
// assistant settings. A blank request is refused before any conversation or
// admission is created.
// Lock: exported entry point; takes c.mu. Suspends: assistant.Start.
func (c *Coordinator) StartAssistant(ctx context.Context, request string) error {
	c.Lock()
	defer c.EndTurn()
	trimmed := strings.TrimSpace(request)
	if trimmed == "" {
		return core.NewFailure("Write what the assistant should change.")
	}
	return c.assistant.Start(ctx, c.settings.ConfigurationBackend, c.settings.ConfigurationModel,
		c.settings.ConfigurationThinking, trimmed)
}

// assistantStringsEqual compares two string slices, treating nil and empty as equal.
// Lock: none (pure).
func assistantStringsEqual(a, b []string) bool {
	return (a == nil) == (b == nil) && slices.Equal(a, b)
}
