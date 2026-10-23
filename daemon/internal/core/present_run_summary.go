package core

import "time"

// RunCounts are the counts shared by the app overview and the daemon's state endpoint.
// Passive watches stay visible in their own count, while parked runs remain
// active as they do in the app's work model.
type RunCounts struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	Watching     int `json:"watching"`
	Running      int `json:"running"`
	NeedsYou     int `json:"needsYou"`
	ShippedToday int `json:"shippedToday"`
}

// RunSummaryPassivelyWatching reports whether a run is a passive watch: a saved watch
// stage is passive only while it is queued or running.
func RunSummaryPassivelyWatching(run Run) bool {
	return runPassivelyWatching(&run)
}

// runPassivelyWatching is RunSummaryPassivelyWatching of a run read in place, for a pass over
// every run that would otherwise copy each one to ask.
func runPassivelyWatching(run *Run) bool {
	if run.State != RunStateQueued && run.State != RunStateRunning {
		return false
	}
	stage := run.stageRef()
	return stage != nil && stage.Kind() == StageKindWatch
}

// RunSummaryCounts calculates every count from one run collection and one supplied
// clock and calendar, so app and API results cannot drift. The calendar day of now
// is the local day in loc.
func RunSummaryCounts(runs []Run, now time.Time, loc *time.Location) RunCounts {
	if loc == nil {
		loc = time.Local
	}
	year, month, day := now.In(loc).Date()
	dayStart := time.Date(year, month, day, 0, 0, 0, 0, loc)
	dayEnd := time.Date(year, month, day+1, 0, 0, 0, 0, loc)
	counts := RunCounts{Total: len(runs)}
	for i := range runs {
		run := &runs[i]
		passive := runPassivelyWatching(run)
		if passive {
			counts.Watching++
		}
		if run.State.IsActive() && !passive {
			counts.Active++
		}
		if run.State == RunStateRunning && !passive {
			counts.Running++
		}
		if run.State.NeedsYou() {
			counts.NeedsYou++
		}
		updated := run.UpdatedAt.Time
		if run.State == RunStateShipped && !updated.Before(dayStart) && updated.Before(dayEnd) {
			counts.ShippedToday++
		}
	}
	return counts
}
