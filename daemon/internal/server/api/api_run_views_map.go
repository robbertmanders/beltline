// An attempt's output as Markdown blocks, and its artifacts.
package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// AttemptOutputResponse is an attempt's output as Markdown blocks.
// Availability is `available`, `missing`, or `notApplicable` for a non-agent
// attempt. Message is omitted when nil.
type AttemptOutputResponse struct {
	RunID        string               `json:"runId"`
	AttemptID    string               `json:"attemptId"`
	Availability string               `json:"availability"`
	Blocks       []core.MarkdownBlock `json:"blocks"`
	Message      *string              `json:"message,omitzero"`
}

// NewAttemptOutputResponse is the snapshot's output as core.AttemptOutputPresent presents it.
func NewAttemptOutputResponse(snapshot coordinator.CoordinatorAttemptOutputSnapshot) AttemptOutputResponse {
	presented := core.AttemptOutputPresent(snapshot.Stage, snapshot.Attempt, snapshot.Text)
	out := AttemptOutputResponse{
		RunID:        snapshot.RunID,
		AttemptID:    snapshot.Attempt.ID,
		Availability: string(presented.Availability),
		Blocks:       presented.Blocks,
		Message:      presented.Message,
	}
	return out
}

// AttemptArtifactWire is one file an attempt left in its artifact folder. The
// The `id` is the name and is not encoded.
type AttemptArtifactWire struct {
	Name      string     `json:"name"`
	SizeBytes int64      `json:"sizeBytes"`
	URL       string     `json:"url"`
	ExpiresAt jsonx.Date `json:"expiresAt"`
}

// AttemptArtifactsResponse is an attempt's artifact list. Warning is the
// sentence shown above the list: what opening an agent's HTML may do.
type AttemptArtifactsResponse struct {
	RunID     string                `json:"runId"`
	AttemptID string                `json:"attemptId"`
	Artifacts []AttemptArtifactWire `json:"artifacts"`
	Warning   string                `json:"warning"`
}

// NewAttemptArtifactsResponse ports `AttemptArtifactsResponse.init(runId:attemptId:artifacts:)`;
// Warning is `StageCopy.artifactWarning`.
func NewAttemptArtifactsResponse(runID, attemptID string, artifacts []AttemptArtifactWire) AttemptArtifactsResponse {
	return AttemptArtifactsResponse{
		RunID:     runID,
		AttemptID: attemptID,
		Artifacts: artifacts,
		Warning:   core.StageCopyArtifactWarning,
	}
}
