package core

import (
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// WorkFilter is Work's state filter; never part of a saved run or belt. Its raw values are
// the query contract of `GET /api/v1/views/work`.
type WorkFilter string

const (
	WorkFilterActive  WorkFilter = "active"
	WorkFilterYou     WorkFilter = "you"
	WorkFilterParked  WorkFilter = "parked"
	WorkFilterShipped WorkFilter = "shipped"
	WorkFilterAll     WorkFilter = "all"
)

// AllWorkFilter lists the cases of WorkFilter in declaration order (CaseIterable).
var AllWorkFilter = []WorkFilter{WorkFilterActive, WorkFilterYou, WorkFilterParked, WorkFilterShipped, WorkFilterAll}

// WorkQuery is the decoded inputs of the Work view, with their defaults. The app builds
// one from its toolbar; the server builds one from the query string. Omitted
// or empty values mean active, all belts, no stage, no search and no grouping.
type WorkQuery struct {
	Filter WorkFilter
	// Belt is a belt id, or "all".
	Belt string
	// Stage is a merged stage's id on the floor; nil means every stage.
	Stage   *string
	Search  string
	GroupBy GroupBy
}

// WorkQueryDefault is the query with the defaults: active, all belts, no stage, no
// search and no grouping.
func WorkQueryDefault() WorkQuery {
	return WorkQuery{Filter: WorkFilterActive, Belt: "all", GroupBy: GroupByNone}
}

// TrimmedSearch is the search without surrounding whitespace, which is not part of a
// search.
func (q WorkQuery) TrimmedSearch() string {
	return trimBlanks(q.Search)
}

// IsSearching reports whether the trimmed search is not empty.
func (q WorkQuery) IsSearching() bool {
	return q.TrimmedSearch() != ""
}

// WorkPresentationRows are the runs Work lists, in row order. A stage id resolves against the
// Overview floor: an id no stage has selects nothing. A belt selects
// its exact id, so an unknown belt selects nothing. The rows are copies of the runs' values
// that share their storage, so read them only while runs cannot change; the presented rows
// (WorkRowPresentationRow) share nothing.
func WorkPresentationRows(runs []Run, query WorkQuery, track *FactoryTrack) []Run {
	var merged *FactoryTrackStage
	if query.Stage != nil {
		if track == nil {
			return []Run{}
		}
		resolved := track.Stage(*query.Stage)
		if resolved == nil {
			return []Run{}
		}
		merged = resolved
	}
	search := LowercasedFull(query.TrimmedSearch())
	rows := []Run{}
	for _, run := range runs {
		if query.Belt != "all" && run.BeltID != query.Belt {
			continue
		}
		if merged != nil && !merged.Contains(run) {
			continue
		}
		if search != "" {
			if query.Filter == WorkFilterActive && run.State == RunStateParked {
				continue
			}
		} else if !workFilterKeeps(query.Filter, run) {
			continue
		}
		if search != "" && !WorkPresentationMatches(run, search) {
			continue
		}
		rows = append(rows, run)
	}
	sort.SliceStable(rows, func(i, j int) bool { return WorkPresentationOrder(rows[i], rows[j]) })
	return rows
}

// workFilterKeeps is the state filter of WorkPresentation.rows when no search is given.
func workFilterKeeps(filter WorkFilter, run Run) bool {
	switch filter {
	case WorkFilterActive:
		return run.State.IsActive() && run.State != RunStateParked
	case WorkFilterYou:
		return run.State.NeedsYou()
	case WorkFilterParked:
		return run.State == RunStateParked
	case WorkFilterShipped:
		return run.State == RunStateShipped
	case WorkFilterAll:
		return true
	}
	return true
}

// WorkPresentationMatches is a case-insensitive substring of the combined subject reference,
// title, repository and belt name. search must already be trimmed and lowercased.
func WorkPresentationMatches(run Run, search string) bool {
	// `"…".lowercased().contains(search)`: full lower-casing and a
	// canonically equivalent substring match (NFC), not a case fold (ß stays ß).
	combined := run.Subject.Ref() + " " + run.Subject.Title + " " + run.Subject.Repo + " " + run.BeltName
	return strings.Contains(norm.NFC.String(LowercasedFull(combined)), norm.NFC.String(search))
}

// WorkPresentationOrder reports whether lhs sorts before rhs: runs that need a person
// first, then most recently updated first, with run id ascending for otherwise equal rows.
func WorkPresentationOrder(lhs, rhs Run) bool {
	lhsNeeds, rhsNeeds := 1, 1
	if lhs.State.NeedsYou() {
		lhsNeeds = 0
	}
	if rhs.State.NeedsYou() {
		rhsNeeds = 0
	}
	if lhsNeeds != rhsNeeds {
		return lhsNeeds < rhsNeeds
	}
	if !lhs.UpdatedAt.Equal(rhs.UpdatedAt.Time) {
		return lhs.UpdatedAt.After(rhs.UpdatedAt.Time)
	}
	return lhs.ID < rhs.ID
}

// WorkPresentationSections groups one eligible set once, then splits it so an epic title
// learned from a run in one section still names the group in the others: runs on a
// shadow version go to Shadow, passive watches to Watching, the rest to the main table.
func WorkPresentationSections(runs []Run, by GroupBy) (main, watching, shadow []WorkGroupGroup) {
	rest, shadow := WorkGroupPartition(WorkGroupGroupRuns(runs, by), Run.OnShadow)
	main, watching = WorkGroupPartition(rest, RunSummaryPassivelyWatching)
	return main, watching, shadow
}

// WorkPresentationWaiting is Work's waiting list: the coordinator's own list restricted only
// by belt. State filter, stage, search and grouping do not restrict it.
func WorkPresentationWaiting(waiting []WaitingSubject, belt string) []WaitingSubject {
	result := []WaitingSubject{}
	for _, subject := range waiting {
		if belt == "all" || subject.BeltID == belt {
			result = append(result, subject)
		}
	}
	return result
}

// WorkPresentationMainEmptyMessage is what the main table says when it has no row. Watchers
// or shadow runs that match on their own are not "no runs": they are listed in their sections.
func WorkPresentationMainEmptyMessage(hasRuns, watchersMatch bool) string {
	if !hasRuns {
		return "No runs yet. Label an issue on a watched repository, then check GitHub."
	}
	if watchersMatch {
		return "No regular runs match these filters."
	}
	return "No runs match these filters."
}

// WorkPresentationCaption is the global counts line above Work. It ignores every query.
func WorkPresentationCaption(counts RunCounts) string {
	return strconv.Itoa(counts.Active) + " active · " +
		strconv.Itoa(counts.Watching) + " watching · " +
		strconv.Itoa(counts.NeedsYou) + " need you · " +
		strconv.Itoa(counts.ShippedToday) + " shipped today"
}

// WorkPresentationBeltLabel is a belt's name in the Work picker: archived belts are marked.
func WorkPresentationBeltLabel(name string, archived bool) string {
	if archived {
		return name + " · archived"
	}
	return name
}

// WorkRowPresentation is one rendered Work row, shared by the table and the API so a
// client can show the same status. The full run does not travel with it; a row carries
// only the run's fields the row and its link read, so a Work read copies no run.
type WorkRowPresentation struct {
	Run WorkRowRun `json:"run"`
	// OpenStage is where the row opens (WorkRowPresentationOpenStage).
	OpenStage *string `json:"openStage,omitzero"`
	// Repository is the disambiguated display name of the run's repository.
	Repository string      `json:"repository"`
	Progress   float64     `json:"progress"`
	Location   StatusWhere `json:"location"`
	// PausedByYou is whether this run's own Pause is blocking progress.
	PausedByYou bool `json:"pausedByYou"`
	// PauseWord is the pause word after the stage name, when a pause or continuation has one.
	PauseWord *string `json:"pauseWord,omitzero"`
	// Chip is "Answer" for a held run, "Decide" for a waiting one, nil otherwise.
	Chip     *string `json:"chip,omitzero"`
	NeedsYou bool    `json:"needsYou"`
	// Held is whether the chip is the held (Answer) one, for the row's red styling.
	Held bool `json:"-"`
	// Reference is the subject's short reference, such as "#184" or "KAN-70".
	Reference string `json:"reference"`
	// UpdatedAgo is the Updated column: how long ago the run changed, from the daemon's clock.
	UpdatedAgo string `json:"updatedAgo"`
	// Shadow is the chip of a run on a shadow version, such as "Shadow v2"; nil otherwise.
	Shadow *string `json:"shadow,omitzero"`
}

// WorkRowRun is the part of a run a Work row carries: its identity, belt, stage, state, title
// and when it changed. Every field is a value, so a row shares nothing with the run.
type WorkRowRun struct {
	ID       string   `json:"id"`
	BeltID   string   `json:"beltId"`
	BeltName string   `json:"beltName"`
	StageID  string   `json:"stageId"`
	State    RunState `json:"state"`
	// Issue is the subject's title, under the key the run itself keeps it.
	Issue     WorkRowRunIssue `json:"issue"`
	UpdatedAt jsonx.Date      `json:"updatedAt"`
}

// WorkRowRunIssue is the subject a Work row names: its title.
type WorkRowRunIssue struct {
	Title string `json:"title"`
}

// WorkRowRunOf is the row's part of run.
func WorkRowRunOf(run *Run) WorkRowRun {
	return WorkRowRun{
		ID: run.ID, BeltID: run.BeltID, BeltName: run.BeltName, StageID: run.StageID, State: run.State,
		Issue: WorkRowRunIssue{Title: run.Subject.Title}, UpdatedAt: run.UpdatedAt,
	}
}

// WorkRowPresentationRow presents one run for Work.
func WorkRowPresentationRow(run Run, paused bool, pause Pause, ambiguous map[string]struct{}, now time.Time) WorkRowPresentation {
	return WorkRowPresentation{
		Run:         WorkRowRunOf(&run),
		OpenStage:   WorkRowPresentationOpenStage(run),
		Repository:  RepoLabelShort(run.Subject.Repo, ambiguous),
		Progress:    StatusProgress(run),
		Location:    StatusLocation(run, paused, pause),
		PausedByYou: RunPauseStatusCounts(run),
		PauseWord:   RunPauseStatusLocationWord(run),
		Chip:        WorkRowPresentationChipText(run),
		NeedsYou:    run.State.NeedsYou(),
		Held:        run.State == RunStateHeld,
		Reference:   run.Subject.Ref(),
		UpdatedAgo:  RelativeTimeAgo(run.UpdatedAt.Time, now),
		Shadow:      ShadowChip(run),
	}
}

// WorkRowPresentationOpenStage is where a row opens. A decision opens that stage; anything
// else follows the run.
func WorkRowPresentationOpenStage(run Run) *string {
	if !run.State.NeedsYou() {
		return nil
	}
	stageID := run.StageID
	return &stageID
}

// WorkRowPresentationChipText is the chip a row shows when the run needs a person. A
// passive watch has none.
func WorkRowPresentationChipText(run Run) *string {
	if !run.State.NeedsYou() {
		return nil
	}
	chip := "Decide"
	if run.State == RunStateHeld {
		chip = "Answer"
	}
	return &chip
}

// WorkGroupPresentation is a Work section's group with each row already presented. Count stays
// section-local, as WorkGroupGroup's does.
type WorkGroupPresentation struct {
	WorkGroupKey
	Title string `json:"title"`
	// Label is the header text: a repository's disambiguated short name, else the group value.
	Label string                `json:"label"`
	Count int                   `json:"count"`
	Rows  []WorkRowPresentation `json:"rows"`
}

// WorkGroupPresentationOf presents a group for Work.
func WorkGroupPresentationOf(group WorkGroupGroup, pausedBelts map[string]struct{}, pause Pause, ambiguous map[string]struct{}, now time.Time) WorkGroupPresentation {
	label := group.Key.Value
	if group.Key.Dimension == GroupByRepo {
		label = RepoLabelShort(group.Key.Value, ambiguous)
	}
	rows := make([]WorkRowPresentation, 0, len(group.Runs))
	for _, run := range group.Runs {
		_, paused := pausedBelts[run.BeltID]
		rows = append(rows, WorkRowPresentationRow(run, paused, pause, ambiguous, now))
	}
	return WorkGroupPresentation{WorkGroupKey: group.Key, Count: group.Count, Title: group.Title(), Label: label, Rows: rows}
}

// WorkView is everything the Work view shows for one query, built from one read. The
// summary is global and unaffected by the query.
type WorkView struct {
	Summary        RunCounts
	MainGroups     []WorkGroupPresentation
	WatchingGroups []WorkGroupPresentation
	// ShadowGroups are the runs on a shadow version, which the Overview floor leaves out.
	ShadowGroups []WorkGroupPresentation
	Waiting      []WaitingSubject
	// Stage is the stage the query selected, when it names one the floor has.
	Stage *WorkStage
	// Belts is every belt the picker offers, archived ones marked.
	Belts []WorkBelt
	// MainEmptyMessage is what the main table says when it has no row; nil when it has rows.
	MainEmptyMessage *string
}

// Caption is the global counts line; the same whatever the query.
func (v WorkView) Caption() string {
	return WorkPresentationCaption(v.Summary)
}

// MainRows are the rows of the main groups.
func (v WorkView) MainRows() []WorkRowPresentation {
	return workRowsOfGroups(v.MainGroups)
}

// WatchingRows are the rows of the watching groups.
func (v WorkView) WatchingRows() []WorkRowPresentation {
	return workRowsOfGroups(v.WatchingGroups)
}

// WatchingRowCount is how many rows the Watching section shows; the heading counts them.
func (v WorkView) WatchingRowCount() int {
	return len(v.WatchingRows())
}

// ShadowRowCount is how many rows the Shadow section shows; the heading counts them.
func (v WorkView) ShadowRowCount() int {
	return len(workRowsOfGroups(v.ShadowGroups))
}

// ShownRowCount is how many rows the page shows in every section together, the count it announces.
func (v WorkView) ShownRowCount() int {
	return len(v.MainRows()) + len(v.WatchingRows()) + v.ShadowRowCount()
}

// workRowsOfGroups is the rows of each group in order, flattened.
func workRowsOfGroups(groups []WorkGroupPresentation) []WorkRowPresentation {
	rows := []WorkRowPresentation{}
	for _, group := range groups {
		rows = append(rows, group.Rows...)
	}
	return rows
}

// WorkStage is a stage the Work query selected, as the page names it.
type WorkStage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkBelt is a belt the Work picker offers.
//
// Encoding: synthesized: id, name, archived, label (decoded as stored; label is
// derived in the initialiser, WorkBeltNew).
type WorkBelt struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Archived bool   `json:"archived"`
	// Label is the picker's text: the name, with "· archived" after an archived belt.
	Label string `json:"label"`
}

// WorkBeltNew is a picker belt; its label is derived from the name and archived flag.
func WorkBeltNew(id, name string, archived bool) WorkBelt {
	return WorkBelt{ID: id, Name: name, Archived: archived, Label: WorkPresentationBeltLabel(name, archived)}
}
