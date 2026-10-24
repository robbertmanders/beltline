// The views snapshot, the
// Overview and Work reads and the factory summary.

package coordinator

import (
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// ReadFactorySummary returns the factory summary at now, in the calendar's time zone.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadFactorySummary(now *time.Time, loc *time.Location) core.FactorySummary {
	c.Lock()
	defer c.EndTurn()
	return FactorySummaryProjectionSummary(c.library.belts, c.book.runs, c.momentOf(now), loc)
}

// ReadViewsSnapshot captures runs, belt pause state, global pause, waiting and AFK
// decisions in one main-actor read, with the coordinator's own clock.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadViewsSnapshot(now *time.Time) CoordinatorViewsSnapshot {
	c.Lock()
	defer c.EndTurn()
	return c.viewsSnapshotNow(now)
}

// viewsSharedNow is viewsSnapshotNow for a read that builds its whole view while it holds c.mu, as
// ReadOverviewView and ReadWorkView do. The view code only reads what it is given, and nothing else
// runs until the read ends, so the snapshot shares the book's values rather than copying every run.
// Lock: runs with c.mu held, until the view is built.
func (c *Coordinator) viewsSharedNow(now *time.Time) CoordinatorViewsSnapshot {
	belts := make([]store.BeltRecord, len(c.library.belts))
	copy(belts, c.library.belts)
	return CoordinatorViewsSnapshot{
		Runs:           c.book.runs,
		Belts:          belts,
		GloballyPaused: c.globallyPaused,
		Pause:          c.currentPause(),
		Waiting:        intakeWaitingRows(c.intake.waitingBy),
		Decisions:      c.autopilot.decisions,
		Now:            c.momentOf(now),
	}
}

// viewsSnapshotNow is ReadViewsSnapshot with c.mu held: copies of the runs, belts, waiting
// subjects and decisions, so the snapshot stays as it was read.
// Lock: runs with c.mu held.
func (c *Coordinator) viewsSnapshotNow(now *time.Time) CoordinatorViewsSnapshot {
	runs := make([]core.Run, 0, len(c.book.runs))
	for _, run := range c.book.runs {
		runs = append(runs, run.Clone())
	}
	belts := make([]store.BeltRecord, 0, len(c.library.belts))
	for _, record := range c.library.belts {
		belts = append(belts, cloneBeltRecord(record))
	}
	waiting := intakeWaitingRows(c.intake.waitingBy)
	for i := range waiting {
		waiting[i].Subject = waiting[i].Subject.Clone()
	}
	return CoordinatorViewsSnapshot{
		Runs:           runs,
		Belts:          belts,
		GloballyPaused: c.globallyPaused,
		Pause:          c.currentPause(),
		Waiting:        waiting,
		Decisions:      append([]core.AfkDecision(nil), c.autopilot.decisions...),
		Now:            c.momentOf(now),
	}
}

// ReadOverviewView is the Overview with the shared counts, floor eligibility and
// layout. width spreads the Floor's columns over the browser's width in points;
// nil draws the minimum columns.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadOverviewView(now *time.Time, loc *time.Location, width *float64) core.OverviewView {
	c.Lock()
	defer c.EndTurn()
	snapshot := c.viewsSharedNow(now)
	belts := snapshot.OverviewBelts()
	track := c.floorTrackOf(belts, snapshot.Runs)
	// A stage is drawn with its agent's icon; without the agent every agent stage is a plain box.
	return core.NewOverviewView(core.OverviewViewArgs{
		Belts: belts, Runs: snapshot.Runs, Now: snapshot.Now, Location: loc,
		Pause: snapshot.Pause, Width: width, Track: &track,
		Agent: func(stage core.Stage) *core.Agent { return c.library.Agent(stage.Agent) },
	})
}

// ReadWorkView is the selected Work view, with the same filtering, grouping and
// row presentation the app uses. The stage resolves against the Overview floor.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ReadWorkView(query core.WorkQuery, now *time.Time, loc *time.Location) core.WorkView {
	c.Lock()
	defer c.EndTurn()
	snapshot := c.viewsSharedNow(now)
	track := c.floorTrackOf(snapshot.OverviewBelts(), snapshot.Runs)
	rows := core.WorkPresentationRows(snapshot.Runs, query, &track)
	main, watching, shadow := core.WorkPresentationSections(rows, query.GroupBy)
	pausedBelts := map[string]struct{}{}
	for _, record := range snapshot.Belts {
		if record.EffectivelyPaused() {
			pausedBelts[record.Belt.ID] = struct{}{}
		}
	}
	ambiguous := core.RepoLabelAmbiguousNames(snapshot.Runs)
	mainRowCount := 0
	for _, group := range main {
		mainRowCount += len(group.Runs)
	}
	watchersMatch := slices.ContainsFunc(append(slices.Clone(watching), shadow...), func(group core.WorkGroupGroup) bool {
		return len(group.Runs) > 0
	})
	view := core.WorkView{
		Summary:    core.RunSummaryCounts(snapshot.Runs, snapshot.Now, loc),
		Waiting:    core.WorkPresentationWaiting(snapshot.Waiting, query.Belt),
		MainGroups: make([]core.WorkGroupPresentation, 0, len(main)),
		Belts:      make([]core.WorkBelt, 0, len(snapshot.Belts)),
	}
	for _, group := range main {
		view.MainGroups = append(view.MainGroups, core.WorkGroupPresentationOf(group, pausedBelts, snapshot.Pause, ambiguous, snapshot.Now))
	}
	view.WatchingGroups = make([]core.WorkGroupPresentation, 0, len(watching))
	for _, group := range watching {
		view.WatchingGroups = append(view.WatchingGroups, core.WorkGroupPresentationOf(group, pausedBelts, snapshot.Pause, ambiguous, snapshot.Now))
	}
	view.ShadowGroups = make([]core.WorkGroupPresentation, 0, len(shadow))
	for _, group := range shadow {
		view.ShadowGroups = append(view.ShadowGroups, core.WorkGroupPresentationOf(group, pausedBelts, snapshot.Pause, ambiguous, snapshot.Now))
	}
	if query.Stage != nil {
		if merged := track.Stage(*query.Stage); merged != nil {
			view.Stage = &core.WorkStage{ID: merged.ID, Name: merged.Name}
		}
	}
	for _, record := range snapshot.Belts {
		view.Belts = append(view.Belts, core.WorkBeltNew(record.Belt.ID, record.Belt.Name, record.Archived))
	}
	if mainRowCount == 0 {
		message := core.WorkPresentationMainEmptyMessage(len(snapshot.Runs) > 0, watchersMatch)
		view.MainEmptyMessage = &message
	}
	return view
}

// floorTrackMemo keeps the factory track of the floor belts with what it was built from: the
// library's revision and the ids of the belts on the floor.
type floorTrackMemo struct {
	revision uint64
	belts    string
	track    core.FactoryTrack
	ok       bool
}

// floorTrackOf is OverviewPresentationTrack of the belts the Floor shows. Laying the belts out
// on the track is a good part of every Overview and Work read, and the track depends only on
// the belts, which the library's revision covers, and on which of them show, which an archived
// belt's active runs decide, so it is built again only when either moved. Reads share the
// track and never change it. A test build checks some hits against a fresh track, as the
// projection memos are checked.
// Lock: runs with c.mu held.
func (c *Coordinator) floorTrackOf(belts []core.OverviewBelt, runs []core.Run) core.FactoryTrack {
	floor := core.OverviewPresentationFloorBelts(belts, runs)
	ids := make([]string, len(floor))
	for i, belt := range floor {
		ids[i] = belt.ID
	}
	key := strings.Join(ids, "\x00")
	memo := &c.floorTrack
	if memo.ok && memo.revision == c.library.revision && memo.belts == key {
		if shouldVerifyMemo() && !reflect.DeepEqual(memo.track, core.OverviewPresentationTrack(floor)) {
			panic("coordinator: the floor track changed without a library revision bump")
		}
		return memo.track
	}
	*memo = floorTrackMemo{revision: c.library.revision, belts: key, track: core.OverviewPresentationTrack(floor), ok: true}
	return memo.track
}
