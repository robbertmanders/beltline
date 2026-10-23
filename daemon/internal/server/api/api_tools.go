package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/kit/tools"
)

// ToolsResponse is `GET /api/v1/tools`: each tool's diagnostic row, and the
// private tmux socket the agents run on.
type ToolsResponse struct {
	Tools []tools.Result `json:"tools"`
	// TmuxSocket is the name of the private tmux server.
	TmuxSocket string `json:"tmuxSocket"`
}

// NewToolsResponse is the rows with the socket; an empty socket is the default one.
func NewToolsResponse(results []tools.Result, tmuxSocket string) ToolsResponse {
	if tmuxSocket == "" {
		tmuxSocket = runner.TmuxSocket()
	}
	return ToolsResponse{Tools: results, TmuxSocket: tmuxSocket}
}
