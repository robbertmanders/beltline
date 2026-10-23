// The Insights read.

package routes

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// AddViewInsights GET api/v1/views/insights. The optional `belt` chooses one belt, and every
// belt is shown without it; the optional `version`, with a belt, keeps to one of its versions; the
// optional `shadow`, with a belt, compares one of its shadow tests instead; the
// optional `period` is 7d, 30d, 90d or all (the default); the optional `showArchived=1` offers the archived
// belts too, and any other value leaves them out. An unknown period or a version that is not
// a positive number is a validation failure, and a belt or version neither the library nor any run
// has is not found.
// Lock: the handler calls ReadInsightsView, which takes the lock.
func AddViewInsights(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/insights", JSONRoute(c, func(r *http.Request) (any, error) {
		query, err := viewInsightsQuery(r)
		if err != nil {
			return nil, err
		}
		view, ok := c.ReadInsightsView(query, nil, nil)
		if !ok {
			return nil, api.MissingResource{}
		}
		return view, nil
	}))
}

// viewInsightsQuery reads the belt and the period.
func viewInsightsQuery(r *http.Request) (core.InsightsQuery, error) {
	query := core.InsightsQuery{Period: core.InsightsPeriodAll}
	if belt := nonEmptyQuery("belt", r); belt != nil {
		query.Belt = *belt
	}
	if version := nonEmptyQuery("version", r); version != nil {
		value, err := strconv.Atoi(*version)
		if err != nil || value < 1 {
			return core.InsightsQuery{}, ReadRouteSupportFail("Version must be a positive whole number.")
		}
		query.Version = value
	}
	if shadow := nonEmptyQuery("shadow", r); shadow != nil {
		if query.Belt == "" {
			return core.InsightsQuery{}, ReadRouteSupportFail("A shadow test needs its belt.")
		}
		query.Shadow = *shadow
	}
	if period := nonEmptyQuery("period", r); period != nil {
		query.Period = core.InsightsPeriod(*period)
		if !slices.Contains(core.AllInsightsPeriod, query.Period) {
			return core.InsightsQuery{}, ReadRouteSupportFail("Period must be 7d, 30d, 90d or all.")
		}
	}
	query.ShowArchived = r.URL.Query().Get("showArchived") == "1"
	return query, nil
}

// AddViewInsightsEvolution GET api/v1/views/insights/evolution. `belt` is required; the optional `by`
// is version (the default) or week; `separate=1` keeps every version in its own column; the optional
// `from`, a positive number, is the first recent column; `showArchived=1` offers the archived belts
// in the picker. A missing belt, an unknown `by` or a `from` that is not a positive number is a
// validation failure, and a belt neither the library nor any run has is not found.
// Lock: the handler calls ReadInsightsEvolution, which takes the lock.
func AddViewInsightsEvolution(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/views/insights/evolution", JSONRoute(c, func(r *http.Request) (any, error) {
		query, err := viewInsightsEvolutionQuery(r)
		if err != nil {
			return nil, err
		}
		view, ok := c.ReadInsightsEvolution(query, nil)
		if !ok {
			return nil, api.MissingResource{}
		}
		return view, nil
	}))
}

// viewInsightsEvolutionQuery reads the belt, the columns and where recent starts.
func viewInsightsEvolutionQuery(r *http.Request) (core.InsightsEvolutionQuery, error) {
	query := core.InsightsEvolutionQuery{Axis: core.InsightsEvolutionByVersion}
	belt := nonEmptyQuery("belt", r)
	if belt == nil {
		return core.InsightsEvolutionQuery{}, ReadRouteSupportFail("Evolution needs a belt.")
	}
	query.Belt = *belt
	if by := nonEmptyQuery("by", r); by != nil {
		query.Axis = core.InsightsEvolutionAxis(*by)
		if !slices.Contains(core.AllInsightsEvolutionAxis, query.Axis) {
			return core.InsightsEvolutionQuery{}, ReadRouteSupportFail("By must be version or week.")
		}
	}
	if from := nonEmptyQuery("from", r); from != nil {
		value, err := strconv.Atoi(*from)
		if err != nil || value < 1 {
			return core.InsightsEvolutionQuery{}, ReadRouteSupportFail("From must be a positive whole number.")
		}
		query.From = value
	}
	query.Separate = r.URL.Query().Get("separate") == "1"
	query.ShowArchived = r.URL.Query().Get("showArchived") == "1"
	return query, nil
}
