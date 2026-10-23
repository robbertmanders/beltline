package core

import (
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// GroupBy is Work's presentation dimension; never part of a saved run or belt.
type GroupBy string

const (
	GroupByNone   GroupBy = "none"
	GroupByRepo   GroupBy = "repo"
	GroupByStatus GroupBy = "status"
	GroupByBelt   GroupBy = "belt"
	GroupByEpic   GroupBy = "epic"
)

// AllGroupBy lists the cases of GroupBy in declaration order (CaseIterable).
var AllGroupBy = []GroupBy{GroupByNone, GroupByRepo, GroupByStatus, GroupByBelt, GroupByEpic}

// Copy used by GroupBy.title and WorkGroup. Literal text is exact.
const (
	groupByValueNeedsYou = "Needs you"
	groupByValueRunning  = "Running"
	groupByValuePaused   = "Paused"
	groupByValueQueued   = "Queued"
	groupByValueParked   = "Parked"
	groupByValueDropped  = "Dropped"
	groupByValueDone     = "Done"
	groupByValueShipped  = "Shipped"
)

// groupByStatusOrder is the order the status dimension lists its groups in.
// the local `status` array in WorkGroup.group(_:by:).
var groupByStatusOrder = []string{groupByValueNeedsYou, groupByValueRunning, groupByValuePaused, groupByValueQueued, groupByValueParked, groupByValueDropped, groupByValueDone, groupByValueShipped}

// Title is the dimension's name in the Work toolbar.
func (g GroupBy) Title() string {
	switch g {
	case GroupByNone:
		return "None"
	case GroupByRepo:
		return "Repository"
	case GroupByStatus:
		return "Status"
	case GroupByBelt:
		return "Belt"
	case GroupByEpic:
		return "Epic"
	}
	return ""
}

// Group is the group key a run falls into under this dimension.
func (g GroupBy) Group(run Run) WorkGroupKey {
	var value string
	switch g {
	case GroupByNone:
		value = ""
	case GroupByRepo:
		value = run.Subject.Repo
	case GroupByBelt:
		value = run.BeltName
	case GroupByEpic:
		if parent := WorkGroupParent(run, map[string]string{}); parent != nil {
			value = *parent
		} else {
			value = WorkGroupNoEpic
		}
	case GroupByStatus:
		if RunPauseStatusCounts(run) {
			value = groupByValuePaused
			break
		}
		switch run.State {
		case RunStateWaiting, RunStateHeld:
			value = groupByValueNeedsYou
		case RunStateRunning:
			value = groupByValueRunning
		case RunStateQueued:
			value = groupByValueQueued
		case RunStateParked:
			value = groupByValueParked
		case RunStateDropped:
			value = groupByValueDropped
		case RunStateDone:
			value = groupByValueDone
		case RunStateShipped:
			value = groupByValueShipped
		}
	}
	return WorkGroupKey{Dimension: g, Value: value}
}

// WorkGroupKey identifies one group: its dimension and value.
type WorkGroupKey struct {
	Dimension GroupBy `json:"dimension"`
	Value     string  `json:"value"`
}

// WorkGroupGroup is one group of runs in Work, in row order.
type WorkGroupGroup struct {
	Key   WorkGroupKey
	Count int
	Runs  []Run
}

// Title is "value · count", or empty for the ungrouped set.
func (g WorkGroupGroup) Title() string {
	if g.Key.Dimension == GroupByNone {
		return ""
	}
	return g.Key.Value + " · " + strconv.Itoa(g.Count)
}

// WorkGroupNoEpic is the value of the group for runs with no epic.
const WorkGroupNoEpic = "No epic"

// WorkGroupParent is the epic a run belongs to, as "KAN-12" or "acme/web#12": its ticket's
// parent, or else the run that watches it among its children, or itself when it
// watches tickets of its own. epics maps each child ref a ticket watch has seen to the
// epic watching it.
func WorkGroupParent(run Run, epics map[string]string) *string {
	if workGroupWatchesTickets(run) {
		if own := workGroupRef(run.Subject.Ref(), run.Subject.Repo); own != nil {
			return own
		}
	}
	if run.Subject.Parent != nil {
		if ref := workGroupRef(*run.Subject.Parent, run.Subject.Repo); ref != nil {
			return ref
		}
	}
	ref := workGroupRef(run.Subject.Ref(), run.Subject.Repo)
	if ref == nil {
		return nil
	}
	if epic, ok := epics[*ref]; ok {
		return &epic
	}
	return nil
}

// workGroupWatchesTickets reports whether any stage of the run is a ticket watch.
func workGroupWatchesTickets(run Run) bool {
	for _, stage := range run.Stages {
		if target := stage.WatchTarget(); target != nil && *target == WatchTargetTicket {
			return true
		}
	}
	return false
}

// workGroupRef is a Jira key as it is, and an issue ref with its repository.
func workGroupRef(ref, repo string) *string {
	if ref == "" {
		return nil
	}
	if strings.HasPrefix(ref, "#") {
		full := repo + ref
		return &full
	}
	return &ref
}

// WorkGroupEpics maps each child a ticket watch has seen to the epic watching it.
func WorkGroupEpics(runs []Run) map[string]string {
	epics := map[string]string{}
	for _, run := range runs {
		epic := workGroupRef(run.Subject.Ref(), run.Subject.Repo)
		if epic == nil {
			continue
		}
		for _, stageID := range slices.Sorted(maps.Keys(run.TicketObservations)) {
			observation := run.TicketObservations[stageID]
			for _, child := range slices.Sorted(maps.Keys(observation.Children)) {
				if ref := workGroupRef(child, run.Subject.Repo); ref != nil {
					epics[*ref] = *epic
				}
			}
		}
	}
	return epics
}

// WorkGroupGroupRuns groups runs by one dimension. Input is already filtered and sorted;
// gathering preserves that order. The groups hold the runs themselves, sharing their storage.
// Groups are ordered by a localized comparison, with the status order and "No epic" last for epics.
func WorkGroupGroupRuns(runs []Run, by GroupBy) []WorkGroupGroup {
	var epics map[string]string
	var titles map[string]string
	if by == GroupByEpic {
		epics = WorkGroupEpics(runs)
		titles = workGroupEpicTitles(runs)
	}
	buckets := map[WorkGroupKey][]Run{}
	for _, run := range runs {
		var key WorkGroupKey
		if by != GroupByEpic {
			key = by.Group(run)
		} else if parent := WorkGroupParent(run, epics); parent == nil {
			key = WorkGroupKey{Dimension: GroupByEpic, Value: WorkGroupNoEpic}
		} else {
			value := *parent
			if title, ok := titles[*parent]; ok {
				value = *parent + " " + title
			}
			key = WorkGroupKey{Dimension: GroupByEpic, Value: value}
		}
		buckets[key] = append(buckets[key], run)
	}
	keys := slices.Collect(maps.Keys(buckets))
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if by == GroupByStatus {
			return groupByStatusIndex(a.Value) < groupByStatusIndex(b.Value)
		}
		if by == GroupByEpic && (a.Value == WorkGroupNoEpic) != (b.Value == WorkGroupNoEpic) {
			return b.Value == WorkGroupNoEpic
		}
		order := LocalizedStandardCompare(a.Value, b.Value)
		if order == 0 {
			return a.Value < b.Value
		}
		return order < 0
	})
	groups := make([]WorkGroupGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, WorkGroupGroup{Key: key, Count: len(buckets[key]), Runs: buckets[key]})
	}
	return groups
}

// groupByStatusIndex is the position of a status value in the status order.
func groupByStatusIndex(value string) int {
	return slices.Index(groupByStatusOrder, value)
}

// workGroupEpicTitles are the titles of the runs that watch their own tickets, by ref.
func workGroupEpicTitles(runs []Run) map[string]string {
	titles := map[string]string{}
	for _, run := range runs {
		if !workGroupWatchesTickets(run) {
			continue
		}
		if ref := workGroupRef(run.Subject.Ref(), run.Subject.Repo); ref != nil {
			titles[*ref] = run.Subject.Title
		}
	}
	return titles
}

// WorkGroupPartition splits groups that were built from one eligible set. Each section keeps
// that set's keys, including an epic title learned from a run that lands
// in the other section, and counts only its own runs.
func WorkGroupPartition(groups []WorkGroupGroup, watching func(run Run) bool) (main []WorkGroupGroup, watchingGroups []WorkGroupGroup) {
	main = []WorkGroupGroup{}
	watchingGroups = []WorkGroupGroup{}
	for _, group := range groups {
		var watched, rest []Run
		for _, run := range group.Runs {
			if watching(run) {
				watched = append(watched, run)
			} else {
				rest = append(rest, run)
			}
		}
		if len(rest) > 0 {
			main = append(main, WorkGroupGroup{Key: group.Key, Count: len(rest), Runs: rest})
		}
		if len(watched) > 0 {
			watchingGroups = append(watchingGroups, WorkGroupGroup{Key: group.Key, Count: len(watched), Runs: watched})
		}
	}
	return main, watchingGroups
}
