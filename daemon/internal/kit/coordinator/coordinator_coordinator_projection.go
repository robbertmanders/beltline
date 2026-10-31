// The coordinator's resource projections: what each resource shows, encoded with jsonx, and the
// encoding helpers they share. Turns compares them after every turn.

package coordinator

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// Projection returns resource's projection, as projection does, taking c.mu for
// callers outside a turn (the server reads it this way).
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) Projection(r Resource) []byte {
	c.Lock()
	defer c.EndTurn()
	return c.projection(r)
}

// projection returns resource's projection: the coordinator state that
// resource shows, encoded with jsonx. Turns compares it after every turn, and
// the API's event hub announces a changed resource. It
// CoordinatorObservation.snapshot, which covers, per resource:
// - runs: the run book's runs, as a digest of each run's encoding (runsDigest).
// - library: the belts (with their records), the agents and the library repositories.
// - settings: the saved settings as a whole.
// - state: globallyPaused, checking, lastCheck, the notices, the runs, and the
// settings' login, notifications and afk.
// - intake: the waiting subjects.
// - assistant: the session, running, busy, command and problems.
// - afk: the AFK decisions.
// - models: the loading runners, the errors, the settings' model catalogs
// and allowed models.
//
// Lock: runs with c.mu held (Turns calls it at the end of each turn).
func (c *Coordinator) projection(r Resource) []byte {
	switch r {
	case ResourceRuns:
		return c.book.runsProjection()
	case ResourceLibrary:
		// The library's belts and agents, then the library repositories, which change with the
		// library's files, the clones' syncs and the settings that list them.
		key := [3]uint64{c.library.revision, c.libraryRepoRevision, c.settingsRevision}
		return c.libraryReposMemo.of("library", key, func() []byte {
			repos := projectionBytes(c.libraryRepoStates())
			return append(append(append([]byte{}, c.library.projection()...), '\n'), repos...)
		})
	case ResourceSettings:
		return c.settingsMemo.of("settings", c.settingsRevision, func() []byte {
			return projectionBytes(c.settings)
		})
	case ResourceState:
		// The runs part is the runs resource's own projection (memoised by the
		// run book): only byte equality matters here, so the parts are joined.
		key := [3]uint64{c.settingsRevision, c.book.revision, c.stateRevision}
		return c.stateMemo.of("state", key, func() []byte {
			rest := projectionBytes([]any{
				c.globallyPaused, c.intake.checking, dateOrNilOf(c.intake.lastCheck), c.book.notices,
				c.settings.Login, c.settings.Notifications, c.settings.Afk,
			})
			runs := c.book.runsProjection()
			out := make([]byte, 0, len(rest)+1+len(runs))
			return append(append(append(out, rest...), '\n'), runs...)
		})
	case ResourceIntake:
		return c.intake.projection()
	case ResourceAssistant:
		return projectionBytes([]any{
			c.assistant.session, c.assistant.running, c.assistant.busy, c.assistant.command, c.assistant.problems,
		})
	case ResourceAfk:
		return c.autopilot.projection()
	case ResourceModels:
		// Read from settings and the model catalogs' loading and errors: each has a revision.
		key := [2]uint64{c.settingsRevision, c.modelCatalogs.revision}
		return c.modelsMemo.of("models", key, func() []byte {
			return projectionBytes([]any{
				c.modelCatalogs.loading, c.modelCatalogs.errors, c.settings.ModelCatalogs, c.settings.AllowedModels,
			})
		})
	}
	return nil
}

// projectionBytes encodes a projection with jsonx. An encoding error is kept as its text, so the change it
// hides is still seen. Lock: runs with c.mu held.
func projectionBytes(v any) []byte {
	data, err := jsonx.Marshal(v)
	if err != nil {
		return []byte("error: " + err.Error())
	}
	return data
}

// intakeWaitingRows is Intake.waiting: the rows of every check's waiting list, in key order,
// each subject once (a Jira ticket by its key, others by repository and number). Lock: runs with c.mu held.
func intakeWaitingRows(by map[string][]core.WaitingSubject) []core.WaitingSubject {
	keys := slices.Sorted(maps.Keys(by))
	seen := map[string]struct{}{}
	rows := []core.WaitingSubject{}
	for _, key := range keys {
		for _, row := range by[key] {
			// A Jira ticket's repository is whichever entry listed it. The key is the subject.
			repo := row.Subject.Repo
			if row.Subject.Kind == core.SubjectKindJira {
				repo = ""
			}
			token := row.BeltID + "|" + string(row.Subject.Kind) + "|" + repo + "|" + strconv.Itoa(row.Subject.Number) + "|" + row.Subject.Key
			if _, dup := seen[token]; dup {
				continue
			}
			seen[token] = struct{}{}
			rows = append(rows, row)
		}
	}
	return rows
}

// beltProjectionRow is a belt record as the library projection encodes it: the store's record keeps
// time.Time, which jsonx does not encode, so its dates are written as jsonx.Date here.
type beltProjectionRow struct {
	Belt           core.Belt
	Paused         bool
	Archived       bool
	Created        jsonx.Date
	Fired          map[string]jsonx.Date
	WatchOutcomes  bool
	ReviewOutcomes bool
	// ShadowTests are the belt's tests, which the settings' switch availability depends on.
	ShadowTests []core.ShadowTest
}

// beltRowOf is the projection row of a belt record.
func beltRowOf(record store.BeltRecord) beltProjectionRow {
	row := beltProjectionRow{
		Belt: record.Belt, Paused: record.Paused, Archived: record.Archived,
		Created: jsonx.Date{Time: record.Created}, WatchOutcomes: record.WatchOutcomes, ReviewOutcomes: record.ReviewOutcomes,
		ShadowTests: record.ShadowTests,
	}
	if record.Fired != nil {
		row.Fired = make(map[string]jsonx.Date, len(record.Fired))
		for id, at := range record.Fired {
			row.Fired[id] = jsonx.Date{Time: at}
		}
	}
	return row
}

// dateOrNilOf is a time as the projection encodes it (jsonx.Date), or nil.
func dateOrNilOf(when *time.Time) *jsonx.Date {
	if when == nil {
		return nil
	}
	date := jsonx.Date{Time: *when}
	return &date
}
