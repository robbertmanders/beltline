// The coordinator's snapshot types, pure projections and lock-free helpers. The exported reads are in
// coordinator_read_state.go, coordinator_read_views.go and coordinator_read_run_view.go.

package coordinator

import (
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// GlobalAction is one of the app's global controls, rendered by the app and
// returned by the state resource; see core.GlobalActionsOf.
type GlobalAction = core.GlobalAction

// FactorySummaryProjectionFloorBelts is the Floor's selection of belts.
// Lock: none (pure).
func FactorySummaryProjectionFloorBelts(belts []store.BeltRecord, runs []core.Run) []store.BeltRecord {
	out := []store.BeltRecord{}
	for _, record := range belts {
		if core.OverviewPresentationShowsOnFloor(record.Archived, record.Belt.ID, runs) {
			out = append(out, record)
		}
	}
	return out
}

// FactorySummaryProjectionSummary is the shared Floor summary.
// loc is the calendar's time zone; nil is the local one.
// Lock: none (pure).
func FactorySummaryProjectionSummary(belts []store.BeltRecord, runs []core.Run, now time.Time, loc *time.Location) core.FactorySummary {
	floor := FactorySummaryProjectionFloorBelts(belts, runs)
	inputs := make([]core.FactorySummaryBeltInput, 0, len(floor))
	for _, record := range floor {
		inputs = append(inputs, core.FactorySummaryBeltInput{ID: record.Belt.ID, Name: record.Belt.Name, Paused: record.EffectivelyPaused()})
	}
	return core.NewFactorySummary(inputs, runs, now, loc)
}

// CoordinatorStateSnapshot is the state read endpoint's coordinator-owned values,
// captured together on the main actor before they are mapped to server wire types.
type CoordinatorStateSnapshot struct {
	GloballyPaused bool
	Checking       bool
	LastCheck      *time.Time
	Counts         core.RunCounts
	Notices        []Notice
	Login          string
	Notifications  bool
	AfkEnabled     bool
	Actions        []GlobalAction
	Navigation     NavigationSummary
	// ClockLabel and ClockTooltip are the clock lamp's label and hover text, derived by the daemon.
	ClockLabel   string
	ClockTooltip string
}

// NavigationSummary is what the navigation shows beside its destinations: the live
// belts and agents, the AFK decisions still to check, the factory the library came from,
// and whether the Market destination is offered (a marketplace repository is set).
type NavigationSummary struct {
	Belts      int `json:"belts"`
	Agents     int `json:"agents"`
	AfkToCheck int `json:"afkToCheck"`
	// Library says whether any library repository is configured, and whether the
	// library changed since the backup repository last took it (nil without one).
	Library LibraryNavigation `json:"library"`
}

// LibraryNavigation is the navigation's view of the library repositories.
type LibraryNavigation struct {
	Offered            bool  `json:"offered"`
	ChangedSinceBackup *bool `json:"changedSinceBackup"`
}

// CoordinatorViewsSnapshot is the Work and Overview reads' coordinator-owned
// values, captured together on the main actor before they are mapped to presentation and wire types.
type CoordinatorViewsSnapshot struct {
	Runs           []core.Run
	Belts          []store.BeltRecord
	GloballyPaused bool
	Pause          core.Pause
	Waiting        []core.WaitingSubject
	Decisions      []core.AfkDecision
	Now            time.Time
}

// OverviewBelts are the belts as the shared Overview presentation reads them.
// Lock: none (pure).
func (s CoordinatorViewsSnapshot) OverviewBelts() []core.OverviewBelt {
	out := make([]core.OverviewBelt, 0, len(s.Belts))
	for _, record := range s.Belts {
		out = append(out, core.OverviewBeltFromBelt(record.Belt, record.Archived, record.EffectivelyPaused()))
	}
	return out
}

// CoordinatorRunViewSnapshot is all coordinator-owned values needed for one
// run-page read, gathered once on the main actor. The projection contains no
// selection or saved preference.
type CoordinatorRunViewSnapshot struct {
	Run            core.Run
	AvailableWidth float64
	BeltPaused     bool
	Pause          core.Pause
	Now            time.Time
	// AmbiguousRepositories are the repositories whose short names two belts share.
	AmbiguousRepositories map[string]struct{}
	Actions               []core.RunAction
	Presentation          core.RunPresentationSnapshot
	Map                   core.MapGeometry
	MapContentWidth       float64
	Worktrees             []runner.WorktreeTarget
	// AdjustHelp is what the Adjust stage sheet shows beside each agent stage's brief, by stage id.
	AdjustHelp map[string]StageAdjustHelp
	// PanelFacts is what each stage's panel shows beyond the shared projection, by stage id.
	PanelFacts map[string]StagePanelFacts
	// AfkGlobal is the global AFK switch, which a run in the global mode follows.
	AfkGlobal bool
	// Look is how the belt is drawn, from the settings.
	Look core.BeltLook
	// Boxes is the belt as stage boxes, when the look is boxes.
	Boxes *core.RunBoxes
}

// StageAdjustHelp is the placeholders an agent stage's brief can use in this run,
// how they resolve, and the brief its pinned agent version starts from.
type StageAdjustHelp struct {
	Placeholders core.PlaceholderHelp
	Note         string
	AgentBrief   *string
	// Runner, Model, Thinking and Brief are what the sheet opens on: the runner,
	// model and effort this run works the stage with, and the brief it reads.
	Runner   core.Backend
	Model    string
	Thinking *core.Thinking
	Brief    string
	// Runners is every runner Adjust offers, in the app's order, with the models this run may use on each.
	Runners []StageAdjustRunner
	// RepairNote is under the model list of a legacy blank stage: one allowed model may repair it.
	RepairNote *string
}

// StageAdjustRunner is one runner the Adjust sheet offers: the models this run may
// use on it, and the effort levels it takes.
type StageAdjustRunner struct {
	Backend      core.Backend
	Models       []core.ModelChoice
	Efforts      []core.Thinking
	ThinkingNote *string
}

// CoordinatorAttemptOutputSnapshot is the output file, if readable, for one
// attempt. The stage is resolved against the run so callers never substitute the latest attempt's output.
type CoordinatorAttemptOutputSnapshot struct {
	RunID   string
	Attempt core.Attempt
	Stage   core.Stage
	Text    *string
}

// CoordinatorArtifactsSnapshot is the secure metadata for one attempt's eligible browser artifacts.
type CoordinatorArtifactsSnapshot struct {
	RunID   string
	Attempt core.Attempt
	Files   []store.ArtifactFile
}

// momentOf is now when the caller gave one, else the coordinator's clock.
// Lock: runs with c.mu held (it reads the clock only).
func (c *Coordinator) momentOf(now *time.Time) time.Time {
	if now != nil {
		return *now
	}
	return c.clock.Now()
}

// currentPause is the global pause as the views and clocks read it: the same value
// Pause returns, without taking the lock.
// Lock: runs with c.mu held.
func (c *Coordinator) currentPause() core.Pause {
	pause := core.Pause{IsPaused: c.globallyPaused}
	if c.settings.PausedAt != nil {
		at := c.settings.PausedAt.Time
		pause.PausedAt = &at
	}
	pause.Pauses = coreDateIntervals(c.settings.Pauses)
	return pause
}

// cloneSettings is a copy of settings that shares no map or slice with them.
// Lock: none (pure).
func cloneSettings(settings store.Settings) store.Settings {
	out := settings
	if settings.AllowedModels != nil {
		out.AllowedModels = copyAllowedModels(settings.AllowedModels)
	}
	if settings.ModelCatalogs != nil {
		out.ModelCatalogs = make(map[string]core.ModelCatalogSnapshot, len(settings.ModelCatalogs))
		for key, snapshot := range settings.ModelCatalogs {
			snapshot.Models = append([]core.ModelChoice(nil), snapshot.Models...)
			out.ModelCatalogs[key] = snapshot
		}
	}
	out.Pauses = append([]store.DateInterval(nil), settings.Pauses...)
	out.PausedAt = core.CopyPointer(settings.PausedAt)
	return out
}
