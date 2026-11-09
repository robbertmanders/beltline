// The reads, the
// attempt reads and the resource projections are in coordinator_coordinator_*.go.

package coordinator

import (
	"context"
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/library"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/kit/tools"
)

// Coordinator runs the belts. It wires the parts together and is what the API
// talks to: the run book applies events and saves runs, the library holds belts
// and agents, intake starts runs, sessions run agents, and watches wait on pull
// requests. Everything they change is written to disk right away.
//
// Lock discipline (AGENT-BRIEF.md, "The coordinator"): c.mu is the main actor.
// Exported methods take it (c.Lock(); defer c.EndTurn()); unexported methods
// run with it held unless their comment says otherwise. Parts have no lock of
// their own. The public var fields are unexported here: the API reads
// them through the exported methods.
type Coordinator struct {
	Turns

	// services is what New was given. Parts read their dependencies from it
	// through the fields below.
	services Services
	storage  store.Storage
	paths    store.Paths
	github   github.GitHub
	jira     jira.Jira
	runner   runner.AgentRunner
	// workspaces makes checkouts and merges.
	workspaces runner.Workspaces
	clock      Clock
	ids        IDs
	notifier   Notifier
	// env reads the process environment.
	env func(string) string
	// toolCheck runs the tools check.
	toolCheck ToolCheckFunc
	// quitHandler stops the daemon cleanly, as SIGTERM does. Nil in tests and demo mode.
	quitHandler func()

	// The parts, each a struct the coordinator owns, reaching back through
	// 	// their `c` field where the part reaches the coordinator.
	book    RunBook
	library Library
	// libraryRepos keeps the clones of the library repositories.
	libraryRepos library.Repos
	// assistant is the configuration conversation.
	assistant LibraryAssistant
	// autopilot is AFK mode's decision assistant.
	autopilot     Autopilot
	intake        Intake
	sessions      Sessions
	watches       Watches
	ticketWatches TicketWatches
	outputs       Outputs
	modelCatalogs ModelCatalogs

	// settings are the saved settings. Every write goes through publishSettings
	// unless publishingSavedSettings is set.
	settings                store.Settings
	publishingSavedSettings bool
	// settingsRevision counts writes to settings (changeSettings and the load
	// bump it); the settings and models projections are memoised at it.
	settingsRevision uint64
	settingsMemo     projectionMemo[uint64]
	modelsMemo       projectionMemo[[2]uint64]
	// stateRevision counts writes to what the state projection reads beside the
	// runs and settings: globallyPaused, the intake's checking and lastCheck, and
	// the notices. The state projection is memoised at it, with the book's and
	// settings' revisions.
	stateRevision uint64
	stateMemo     projectionMemo[[3]uint64]
	// floorTrack is the factory track the Overview and Work reads last drew (floorTrackOf).
	floorTrack floorTrackMemo
	// beltContexts are the saved belts' contexts the belts read last worked out (savedBeltContext).
	beltContexts beltContextMemo
	// globallyPaused is the app's one global pause, saved in the settings.
	globallyPaused bool
	// libraryRepoSyncs is when each library repository was last synced here, and what came of it.
	libraryRepoSyncs map[core.LibraryOrigin]librarySync
	// libraryBusy holds the repositories with a library command in flight, by owner/repo: a clone is
	// reset and checked out by one command at a time, and the lock is released while git runs.
	libraryBusy map[string]struct{}
	// libraryRepoRevision counts changes to the clones and syncs; the Library projection reads it.
	libraryRepoRevision uint64
	libraryReposMemo    projectionMemo[[3]uint64]

	// ticking guards Tick against re-entry, and against the lanes.
	ticking bool
	// runningLanes holds the lanes with a pass in progress.
	runningLanes map[Lane]struct{}
	// laneWakes are the lanes' wake signals, nil until StartLanes.
	laneWakes map[Lane]chan struct{}
	// effects are the stop and cleanup effects still running, by run, so erasure
	// can wait for Drop.
	effects map[string][]*EffectTask
	// erasureRunning holds the erasures inside performErasure, so a tick does not
	// run them twice.
	erasureRunning map[string]struct{}
	// erasuresClear is set once a recovery pass found no intent left on disk, so later passes skip the
	// erasures folder; erasureIntents counts the intents erase has written, so a pass that an erase
	// overlapped does not set it. Only this coordinator writes intents while it owns the folder.
	erasuresClear  bool
	erasureIntents int
	// recovering, recoveringOverrules and recoveringParks guard the recovery passes.
	recovering          bool
	recoveringOverrules bool
	recoveringParks     bool
	// parkStops holds the park stops already being waited on.
	parkStops map[string]struct{}
	// overruleHalt is where an in-flight overrule stops, for tests to reload at.
	overruleHalt *OverruleHalt
	// erasureErrors is the last erasure error noted per run.
	erasureErrors map[string]string
	// runPauseBusy holds the runs whose Pause or Resume is between click and confirmation.
	runPauseBusy map[string]struct{}
}

// SetQuitHandler sets the function the daemon calls to stop its process cleanly,
// the way SIGTERM does.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SetQuitHandler(handler func()) {
	c.Lock()
	defer c.EndTurn()
	c.quitHandler = handler
}

// InvokeQuitHandler calls the quit handler when one is set, so the daemon stops
// as SIGTERM does. The turn ends before the
// handler runs: the daemon's stop path may call back into the coordinator.
// Lock: exported entry point; takes c.mu for the read of the handler only.
func (c *Coordinator) InvokeQuitHandler() {
	c.Lock()
	handler := c.quitHandler
	c.EndTurn()
	if handler != nil {
		handler()
	}
}

// Paths is the data folder's layout.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Paths() store.Paths {
	c.Lock()
	defer c.EndTurn()
	return c.paths
}

// CheckTools runs the shared tools check on this Mac.
// Lock: exported entry point; takes c.mu. Suspends: the toolCheck call runs
// without the lock.
func (c *Coordinator) CheckTools(ctx context.Context) []tools.Result {
	c.Lock()
	defer c.EndTurn()
	if c.toolCheck == nil {
		return nil
	}
	var results []tools.Result
	c.Suspend(func() { results = c.toolCheck(ctx) })
	return results
}

// initMaps makes the coordinator's maps, so New and the tests never write to a nil map.
// Lock: runs during New, before the coordinator is shared.
func (c *Coordinator) initMaps() {
	c.libraryRepoSyncs = map[core.LibraryOrigin]librarySync{}
	c.libraryBusy = map[string]struct{}{}
	c.effects = map[string][]*EffectTask{}
	c.erasureRunning = map[string]struct{}{}
	c.parkStops = map[string]struct{}{}
	c.erasureErrors = map[string]string{}
	c.runPauseBusy = map[string]struct{}{}
}

// SetChangeListener sets the function that receives the resources changed by each turn, sorted
// (the event hub). It is called with c.mu held, so it must not block or call back.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SetChangeListener(listener func([]Resource)) {
	c.Lock()
	defer c.EndTurn()
	c.OnChange = listener
}

// SetNotifier sets the function that delivers notifications.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) SetNotifier(notifier Notifier) {
	c.Lock()
	defer c.EndTurn()
	c.notifier = notifier
}

// setGloballyPausedFlag records the app's global pause flag, which the state projection reads, so
// the write bumps the state revision. Lock: runs with c.mu held.
func (c *Coordinator) setGloballyPausedFlag(paused bool) {
	c.globallyPaused = paused
	c.stateRevision++
}

// changeSettings applies change to the settings the way the didSet does: the
// change takes effect in memory, and the settings are saved unless a saved write is being
// published. A failed save is ignored, as the `try?` ignores it.
// Lock: runs with c.mu held.
func (c *Coordinator) changeSettings(change func(s *store.Settings)) {
	change(&c.settings)
	c.settingsRevision++
	if !c.publishingSavedSettings {
		_ = c.storage.SaveSettings(c.settings)
	}
}
