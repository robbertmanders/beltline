// The state, run, belt,
// agent, settings and AFK reads.

package coordinator

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// ReadGlobalActions returns the global controls in pause, AFK, check-now order.
// They stay available as in the app, including Fetch during pause.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadGlobalActions() []GlobalAction {
	c.Lock()
	defer c.EndTurn()
	return c.globalActionsNow()
}

// globalActionsNow is ReadGlobalActions with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) globalActionsNow() []GlobalAction {
	return core.GlobalActionsOf(c.globallyPaused, c.settings.Afk.Enabled, len(c.autopilot.Deciding()) > 0)
}

// ReadStateSnapshot captures the state read's coordinator-owned values together
// . loc is the calendar's time zone; nil is local.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadStateSnapshot(now *time.Time, loc *time.Location) CoordinatorStateSnapshot {
	c.Lock()
	defer c.EndTurn()
	moment := c.momentOf(now)
	belts, agents := 0, 0
	for _, record := range c.library.belts {
		if !record.Archived {
			belts++
		}
	}
	for _, record := range c.library.agents {
		if !record.Archived {
			agents++
		}
	}
	lastCheck := core.CopyPointer(c.intake.lastCheck)
	return CoordinatorStateSnapshot{
		GloballyPaused: c.globallyPaused,
		Checking:       c.intake.isChecking(),
		LastCheck:      lastCheck,
		Counts:         core.RunSummaryCounts(c.book.runs, moment, loc),
		Notices:        append([]Notice(nil), c.book.notices...),
		Login:          c.settings.Login,
		Notifications:  c.settings.Notifications,
		AfkEnabled:     c.settings.Afk.Enabled,
		Actions:        c.globalActionsNow(),
		Navigation: NavigationSummary{
			Belts:      belts,
			Agents:     agents,
			AfkToCheck: len(c.autopilot.Unreviewed()),
			Library:    c.libraryNavigation(),
		},
		ClockLabel:   core.ClockLampLabel(c.globallyPaused, c.intake.isChecking(), moment),
		ClockTooltip: core.ClockLampTooltip(c.globallyPaused, c.intake.lastCheck, moment),
	}
}

// ReadRuns returns the runs.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadRuns() []core.Run {
	c.Lock()
	defer c.EndTurn()
	runs := make([]core.Run, 0, len(c.book.runs))
	for _, run := range c.book.runs {
		runs = append(runs, run.Clone())
	}
	sortRunsByCreation(runs)
	return runs
}

// sortRunsByCreation orders runs by when each was created, then by id
// `readRuns()`). It sorts in place.
func sortRunsByCreation(runs []core.Run) {
	sort.SliceStable(runs, func(i, j int) bool {
		a, b := runs[i].CreatedAt.Time, runs[j].CreatedAt.Time
		if !a.Equal(b) {
			return a.Before(b)
		}
		return runs[i].ID < runs[j].ID
	})
}

// ReadRun returns the run with id, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadRun(id string) *core.Run {
	c.Lock()
	defer c.EndTurn()
	return c.book.Run(id)
}

// ReadAttemptOutputSnapshot reads only the named attempt's output. A non-agent
// attempt is still a known attempt, with an empty output represented by a nil text.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAttemptOutputSnapshot(runID, attemptID string) *CoordinatorAttemptOutputSnapshot {
	c.Lock()
	defer c.EndTurn()
	run := c.book.Run(runID)
	if run == nil {
		return nil
	}
	attempt := attemptNamed(*run, attemptID)
	if attempt == nil {
		return nil
	}
	stage := findStage(*run, attempt.StageID)
	if stage == nil {
		return nil
	}
	var text *string
	if stage.Kind() == core.StageKindAgent {
		text = c.outputs.output(*run, *attempt)
	}
	return &CoordinatorAttemptOutputSnapshot{RunID: run.ID, Attempt: *attempt, Stage: *stage, Text: text}
}

// ReadArtifactsSnapshot uses the storage layer's no-follow metadata walk; missing
// folders and non-agent attempts produce an empty list without creating it.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadArtifactsSnapshot(runID, attemptID string) *CoordinatorArtifactsSnapshot {
	c.Lock()
	defer c.EndTurn()
	run := c.book.Run(runID)
	if run == nil {
		return nil
	}
	attempt := attemptNamed(*run, attemptID)
	if attempt == nil {
		return nil
	}
	files := []store.ArtifactFile{}
	if stage := findStage(*run, attempt.StageID); stage != nil && stage.Kind() == core.StageKindAgent {
		for _, file := range store.ArtifactsListConfined(c.paths.ArtifactsFolder(*run, *attempt), c.paths.Root) {
			if store.ServableArtifactName(file.Name) {
				files = append(files, file)
			}
		}
	}
	return &CoordinatorArtifactsSnapshot{RunID: run.ID, Attempt: *attempt, Files: files}
}

// attemptNamed is the run's attempt with id, or nil.
// Lock: none (pure).
func attemptNamed(run core.Run, id string) *core.Attempt {
	for i := range run.Attempts {
		if run.Attempts[i].ID == id {
			attempt := run.Attempts[i]
			return &attempt
		}
	}
	return nil
}

// ReadBelts returns the belt records.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadBelts() []store.BeltRecord {
	c.Lock()
	defer c.EndTurn()
	c.library.reconcileFiles()
	return c.readBeltsNow()
}

// readBeltsNow is ReadBelts with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) readBeltsNow() []store.BeltRecord {
	out := make([]store.BeltRecord, 0, len(c.library.belts))
	for _, record := range c.library.belts {
		out = append(out, cloneBeltRecord(record))
	}
	sortBeltRecords(out)
	return out
}

// sortBeltRecords orders belt records by when each was first saved, then by id
// . It sorts in place.
func sortBeltRecords(records []store.BeltRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if !records[i].Created.Equal(records[j].Created) {
			return records[i].Created.Before(records[j].Created)
		}
		return records[i].Belt.ID < records[j].Belt.ID
	})
}

// ReadBelt returns the belt record with id, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadBelt(id string) *store.BeltRecord {
	c.Lock()
	defer c.EndTurn()
	c.library.reconcileFiles()
	return c.library.Belt(id)
}

// ReadAgents returns the agents in the library's order, as the app listed them.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAgents() []core.AgentRecord {
	c.Lock()
	defer c.EndTurn()
	c.library.reconcileFiles()
	return c.readAgentsNow()
}

// readAgentsNow is ReadAgents with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) readAgentsNow() []core.AgentRecord {
	out := make([]core.AgentRecord, 0, len(c.library.agents))
	for _, record := range c.library.agents {
		record.Agent = cloneAgent(record.Agent)
		out = append(out, record)
	}
	return out
}

// ReadAgent returns an agent's latest record, or nil.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAgent(id string) *core.AgentRecord {
	c.Lock()
	defer c.EndTurn()
	c.library.reconcileFiles()
	return c.library.LatestAgent(id)
}

// ReadSettings returns the saved settings.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadSettings() store.Settings {
	c.Lock()
	defer c.EndTurn()
	return cloneSettings(c.settings)
}

// ReadWaitingSubjects returns the waiting subjects as intake last found them; no fresh check.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadWaitingSubjects() []core.WaitingSubject {
	c.Lock()
	defer c.EndTurn()
	rows := intakeWaitingRows(c.intake.waitingBy)
	out := make([]core.WaitingSubject, 0, len(rows))
	for _, row := range rows {
		row.Subject = row.Subject.Clone()
		out = append(out, row)
	}
	return out
}

// ReadAfkDecisions returns every AFK decision in stored order; review state is not changed.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAfkDecisions() []core.AfkDecision {
	c.Lock()
	defer c.EndTurn()
	return append([]core.AfkDecision(nil), c.autopilot.decisions...)
}

// ReadAfkFoldersPresent returns the ids of decisions whose session folder is still
// on disk, so the page can make "Show session files" unavailable when it is gone.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAfkFoldersPresent() map[string]struct{} {
	c.Lock()
	defer c.EndTurn()
	return c.afkFoldersPresent()
}

// afkFoldersPresent is ReadAfkFoldersPresent's set, reading each decision's run in place.
// Lock: runs with c.mu held.
func (c *Coordinator) afkFoldersPresent() map[string]struct{} {
	present := map[string]struct{}{}
	for _, decision := range c.autopilot.decisions {
		if decision.Folder == "" {
			continue
		}
		index := c.book.index(decision.RunID)
		if index < 0 {
			continue
		}
		folder := filepath.Join(c.paths.RunFolder(c.book.runs[index]), decision.Folder)
		if _, err := os.Stat(folder); err == nil {
			present[decision.ID] = struct{}{}
		}
	}
	return present
}

// AfkLogSnapshot is what the AFK log reads in one turn: every decision in stored order, copies of
// the runs they name in creation order, and the ids of the decisions whose folder is on disk.
type AfkLogSnapshot struct {
	Decisions      []core.AfkDecision
	Runs           []core.Run
	FoldersPresent map[string]struct{}
}

// ReadAfkLog reads the AFK log in one turn, copying only the runs its decisions name, not the
// whole book; review state is not changed.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadAfkLog() AfkLogSnapshot {
	c.Lock()
	defer c.EndTurn()
	snapshot := AfkLogSnapshot{
		Decisions:      append([]core.AfkDecision(nil), c.autopilot.decisions...),
		FoldersPresent: c.afkFoldersPresent(),
	}
	named := make(map[string]struct{}, len(c.autopilot.decisions))
	for _, decision := range c.autopilot.decisions {
		named[decision.RunID] = struct{}{}
	}
	for i := range c.book.runs {
		if _, ok := named[c.book.runs[i].ID]; ok {
			snapshot.Runs = append(snapshot.Runs, c.book.runs[i].Clone())
		}
	}
	sortRunsByCreation(snapshot.Runs)
	return snapshot
}

// ReadRepoLabelAmbiguousNames is the set of repository names more than one owner uses across
// every run, read in place rather than through a copy of the book.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadRepoLabelAmbiguousNames() map[string]struct{} {
	c.Lock()
	defer c.EndTurn()
	return core.RepoLabelAmbiguousNames(c.book.runs)
}
