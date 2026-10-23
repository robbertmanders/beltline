package routes

import (
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// AddModelsList GET api/v1/models/{runner}.
// Lock: the handler calls ReadModelRunner, which takes the lock.
func AddModelsList(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/models/{runner}", JSONRoute(c, func(r *http.Request) (any, error) {
		runner, err := modelsRunner(r)
		if err != nil {
			return nil, err
		}
		text, _ := HummingbirdQueryParameter("q", r.URL.RawQuery)
		return c.ReadModelRunner(runner, text), nil
	}))
}

// AddModelsRefresh POST api/v1/models/{runner}/refresh.
// Lock: the handler calls RefreshModelCatalog, which takes the lock.
func AddModelsRefresh(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/models/{runner}/refresh", JSONRoute(c, func(r *http.Request) (any, error) {
		if err := DecodeEmptyObjectOrNoBody(r, DefaultBodyLimit); err != nil {
			return nil, err
		}
		runner, err := modelsRunner(r)
		if err != nil {
			return nil, err
		}
		return c.RefreshModelCatalog(r.Context(), runner), nil
	}))
}

// AddModelsAllow POST api/v1/models/{runner}/allow.
// Lock: the handler calls SetModelAllowedAndRead, which takes the lock.
func AddModelsAllow(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/models/{runner}/allow", JSONRoute(c, func(r *http.Request) (any, error) {
		runner, err := modelsRunner(r)
		if err != nil {
			return nil, err
		}
		body, err := DecodeBody[api.ModelAllowRequest](r, DefaultBodyLimit)
		if err != nil {
			return nil, err
		}
		resource, err := c.SetModelAllowedAndRead(body.Model, runner, body.Allowed)
		if err != nil {
			return nil, err
		}
		return resource, nil
	}))
}

// modelsRunner is the {runner} path segment as a
// Backend; an unknown runner is a 404 (the route is not one the daemon serves).
// The parameter is read raw (`parameters.require`), so an escaped name such as
// `%63laude` is not a runner. The segment is the fourth after the leading slash in
// /api/v1/models/{runner}.
func modelsRunner(r *http.Request) (core.Backend, error) {
	raw := rawPathSegment(r, 4)
	for _, backend := range core.AllBackend {
		if string(backend) == raw {
			return backend, nil
		}
	}
	return "", api.MissingResource{}
}
