package routes

import (
	"net/http"
	"path/filepath"

	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
)

// AddAgentList GET api/v1/agents.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentList(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/agents", JSONRoute(c, func(r *http.Request) (any, error) {
		searchText, filterText := "", "all"
		if search := ReadRouteSupportQuery("search", r.URL.RawQuery); search != nil {
			searchText = *search
		}
		if filter := ReadRouteSupportQuery("filter", r.URL.RawQuery); filter != nil {
			filterText = *filter
		}
		return api.NewAgentsResponseFromCoordinator(c, searchText, filterText), nil
	}))
}

// AddAgentDetail GET api/v1/agents/{id}.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentDetail(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("GET /api/v1/agents/{id}", JSONRoute(c, func(r *http.Request) (any, error) {
		id, err := ReadRouteSupportID(r)
		if err != nil {
			return nil, err
		}
		reads, found := c.ReadAgentDetail(id)
		if !found {
			return nil, api.MissingResource{}
		}
		folder := filepath.Join(c.Paths().Agents(), id)
		return api.NewAgentDetailResponse(reads.Resource, folder), nil
	}))
}

// AddAgentDraftPresentation POST api/v1/agents/draft/presentation.
// A draft's presentation, computed by the daemon from the supplied agent. It writes nothing and invalidates no read.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentDraftPresentation(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/agents/draft/presentation", JSONRoute(c, func(r *http.Request) (any, error) {
		body, err := ReadRouteSupportJSON[api.AgentDraftPresentationRequest](r, false)
		if err != nil {
			return nil, err
		}
		return api.NewAgentDraftResponse(c.DraftAgentResource(body.Agent)), nil
	}))
}

// AddAgentDraftSwitchRunner POST api/v1/agents/draft/switch-runner.
// Normalize a draft after its runner changes. An alternate row is named by its
// index; otherwise the primary settings change.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentDraftSwitchRunner(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/agents/draft/switch-runner", JSONRoute(c, func(r *http.Request) (any, error) {
		body, err := ReadRouteSupportJSON[api.AgentDraftSwitchRunnerRequest](r, false)
		if err != nil {
			return nil, err
		}
		normalized := c.SwitchAgentRunnerDraft(body.Agent, body.Runner, body.AlternateIndex)
		return api.NewAgentDraftResponse(c.DraftAgentResource(normalized)), nil
	}))
}

// AddAgentNew POST api/v1/agents/new.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentNew(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/agents/new", JSONRoute(c, func(r *http.Request) (any, error) {
		if _, err := ReadRouteSupportJSON[api.BeltEmptyRequest](r, true); err != nil {
			return nil, err
		}
		return c.NewAgentDraft(), nil
	}))
}

// AddAgentSave POST api/v1/agents/save.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentSave(mux *http.ServeMux, c *coordinator.Coordinator) {
	mux.Handle("POST /api/v1/agents/save", JSONRoute(c, func(r *http.Request) (any, error) {
		body, err := ReadRouteSupportJSON[api.AgentSaveRequest](r, false)
		if err != nil {
			return nil, err
		}
		record, err := c.SaveAgentCommand(body.Agent)
		if err != nil {
			return nil, err
		}
		return api.NewAgentRecordResponse(record, nil), nil
	}))
}

// AddAgentArchive POST
// api/v1/agents/{id}/archive and api/v1/agents/{id}/unarchive.
// Lock: the handler calls coordinator methods, which take the lock.
func AddAgentArchive(mux *http.ServeMux, c *coordinator.Coordinator) {
	for action, archived := range map[string]bool{"archive": true, "unarchive": false} {
		mux.Handle("POST /api/v1/agents/{id}/"+action, JSONRoute(c, func(r *http.Request) (any, error) {
			id, err := ReadRouteSupportID(r)
			if err != nil {
				return nil, err
			}
			if _, err := ReadRouteSupportJSON[api.BeltEmptyRequest](r, true); err != nil {
				return nil, err
			}
			return agentSetArchived(c, id, archived)
		}))
	}
}

// agentSetArchived is the shared archive and unarchive handler.
// The caller has read the optional empty body; this checks the agent exists, then runs the command.
// Lock: calls coordinator methods, which take the lock.
func agentSetArchived(c *coordinator.Coordinator, id string, archived bool) (api.AgentRecordResponse, error) {
	if c.ReadAgent(id) == nil {
		return api.AgentRecordResponse{}, api.MissingResource{}
	}
	record, err := c.ArchiveAgentCommand(id, archived)
	if err != nil {
		return api.AgentRecordResponse{}, err
	}
	return api.NewAgentRecordResponse(record, nil), nil
}
