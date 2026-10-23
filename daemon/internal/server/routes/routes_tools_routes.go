package routes

import (
	"net/http"

	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// AddTools GET api/v1/tools, the tool
// diagnostics and the private tmux socket.
// Lock: the handler calls CheckTools, which takes the lock.
func AddTools(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/tools", JSONRoute(c, func(r *http.Request) (any, error) {
		return api.NewToolsResponse(c.CheckTools(r.Context()), runner.TmuxSocket()), nil
	}))
}
