// The run page, sessions,
// overview, work and waiting reads.
package routes

import (
	"math"
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// viewRunDefaultWidth is the width the run view draws at when none is sent.
const viewRunDefaultWidth = 1080

// AddViewRun GET api/v1/views/run/{id}.
// The optional width is the browser's width in points for the run's belt map.
// Lock: the handler calls ReadRunViewSnapshot, which takes the lock.
func AddViewRun(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/run/{id}", JSONRoute(c, func(r *http.Request) (any, error) {
		id, err := ReadRouteSupportID(r)
		if err != nil {
			return nil, err
		}
		width, err := viewWidth(r)
		if err != nil {
			return nil, err
		}
		drawn := core.ValueOr(width, float64(viewRunDefaultWidth))
		snapshot := c.ReadRunViewSnapshot(id, drawn, ReadRouteSupportQuery("stage", r.URL.RawQuery))
		if snapshot == nil {
			return nil, api.MissingResource{}
		}
		return api.NewRunViewResponse(*snapshot), nil
	}))
}

// AddViewRunSessions GET
// api/v1/views/run/{id}/sessions. `stage` and `attempt` are the browser's
// selection; the shared selection policy resolves whatever they name.
// Lock: the handler calls ReadSessionsView, which takes the lock.
func AddViewRunSessions(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/run/{id}/sessions", JSONRoute(c, func(r *http.Request) (any, error) {
		id, err := ReadRouteSupportID(r)
		if err != nil {
			return nil, err
		}
		stage := nonEmptyQuery("stage", r)
		attempt := nonEmptyQuery("attempt", r)
		view := c.ReadSessionsView(r.Context(), id, stage, attempt, nil)
		if view == nil {
			return nil, api.MissingResource{}
		}
		return *view, nil
	}))
}

// AddViewOverview GET api/v1/views/overview.
// The browser's width in points spreads the Floor's columns; without it the
// Floor draws at the minimum column.
// Lock: the handler calls ReadOverviewView, which takes the lock.
func AddViewOverview(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/overview", JSONRoute(c, func(r *http.Request) (any, error) {
		width, err := viewWidth(r)
		if err != nil {
			return nil, err
		}
		view := c.ReadOverviewView(nil, nil, width)
		return api.NewOverviewResponse(view), nil
	}))
}

// AddViewWork GET api/v1/views/work.
// Lock: the handler calls ReadWorkView, which takes the lock.
func AddViewWork(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/work", JSONRoute(c, func(r *http.Request) (any, error) {
		query, err := viewWorkQuery(r)
		if err != nil {
			return nil, err
		}
		return api.NewWorkResponse(c.ReadWorkView(query, nil, nil)), nil
	}))
}

// AddViewWaiting GET api/v1/waiting.
// Lock: the handler calls ReadWaitingSubjects, which takes the lock.
func AddViewWaiting(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/waiting", JSONRoute(c, func(r *http.Request) (any, error) {
		return api.NewWaitingResponse(c.ReadWaitingSubjects()), nil
	}))
}

// viewWidth is the width query the Overview and run reads share: at most one
// `width`, a finite positive number of points. Nil when the query has none.
func viewWidth(r *http.Request) (*float64, error) {
	widths := ReadRouteSupportQueryValues("width", r.URL.RawQuery)
	if len(widths) > 1 {
		return nil, ReadRouteSupportFail("Width must be provided once as a finite positive number of points.")
	}
	if len(widths) == 0 {
		return nil, nil
	}
	value, ok := Strtod(widths[0])
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return nil, ReadRouteSupportFail("Width must be a finite positive number of points.")
	}
	return &value, nil
}

// nonEmptyQuery is the first value of a query parameter, nil when it is absent or empty
// .
func nonEmptyQuery(name string, r *http.Request) *string {
	value := ReadRouteSupportQuery(name, r.URL.RawQuery)
	if value == nil || *value == "" {
		return nil
	}
	return value
}

// viewWorkQuery is the Work page's query, with
// the defaults applied and an unsupported nonempty filter or groupBy rejected with
// the shared 422 error shape.
func viewWorkQuery(r *http.Request) (core.WorkQuery, error) {
	query := r.URL.RawQuery
	filter, err := viewFilter(ReadRouteSupportQuery("filter", query))
	if err != nil {
		return core.WorkQuery{}, err
	}
	groupBy, err := viewGroupBy(ReadRouteSupportQuery("groupBy", query))
	if err != nil {
		return core.WorkQuery{}, err
	}
	belt := "all"
	if value := nonEmptyQuery("belt", r); value != nil {
		belt = *value
	}
	search := ""
	if value := ReadRouteSupportQuery("search", query); value != nil {
		search = *value
	}
	return core.WorkQuery{
		Filter:  filter,
		Belt:    belt,
		Stage:   nonEmptyQuery("stage", r),
		Search:  search,
		GroupBy: groupBy,
	}, nil
}

// viewFilter is an absent filter is the default; an
// unknown one is a validation failure.
func viewFilter(raw *string) (core.WorkFilter, error) {
	if raw == nil || *raw == "" {
		return core.WorkFilterActive, nil
	}
	for _, filter := range core.AllWorkFilter {
		if string(filter) == *raw {
			return filter, nil
		}
	}
	return "", ReadRouteSupportFail("filter must be one of active, you, parked, shipped, all")
}

// viewGroupBy is an absent group is the default; an
// unknown one is a validation failure.
func viewGroupBy(raw *string) (core.GroupBy, error) {
	if raw == nil || *raw == "" {
		return core.GroupByNone, nil
	}
	for _, groupBy := range core.AllGroupBy {
		if string(groupBy) == *raw {
			return groupBy, nil
		}
	}
	return "", ReadRouteSupportFail("groupBy must be one of none, repo, status, belt, epic")
}
