// Tests of the tools endpoint's shape check.

package servertest

import (
	"testing"

	"context"
	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/server/api"
	"github.com/robbertmanders/beltline/daemon/internal/server/routes"
	"slices"
)

// toolsWire is the part of the tools response these tests read.
type toolsWire struct {
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
}

func TestToolsAnswersTheChecksInOrderAndEqualTheCoordinatorsResults(t *testing.T) {
	t.Parallel()
	f := ReadOnlyFixture(t)
	reply := f.Do(Request{Method: "GET", Path: "/api/v1/tools", Token: ContractToken})
	requireStatus(t, reply, 200)

	var response toolsWire
	if err := json.Unmarshal(reply.Body, &response); err != nil {
		t.Fatalf("tools body: %v", err)
	}
	var names []string
	for _, tool := range response.Tools {
		names = append(names, tool.Name)
	}
	want := []string{"gh", "git", "tmux", "Copilot CLI", "Pi", "Claude Code", "Codex CLI", "Cursor CLI", "Atlassian CLI", "Ghostty"}
	if !slices.Equal(names, want) {
		t.Fatalf("tool names %v, want %v", names, want)
	}

	source, err := routes.JSONEncode(api.NewToolsResponse(f.Coordinator.CheckTools(context.Background()), runner.TmuxSocket()))
	if err != nil {
		t.Fatalf("tools: %v", err)
	}
	if string(reply.Body) != string(source) {
		t.Fatalf("tools body differs from the coordinator's results:\n%s\n%s", reply.Body, source)
	}
}
