package api

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator"
)

// AssistantConfigurationResponse is the complete assistant configuration on the
// wire. `thinking` is written as an explicit null when unset.
type AssistantConfigurationResponse struct {
	Backend  core.Backend   `json:"backend"`
	Model    string         `json:"model"`
	Thinking *core.Thinking `json:"thinking"`
}

// NewAssistantConfigurationResponse builds the assistant configuration response from its arguments.
func NewAssistantConfigurationResponse(configuration coordinator.AssistantConfiguration) AssistantConfigurationResponse {
	thinking := core.CopyPointer(configuration.Thinking)
	return AssistantConfigurationResponse{Backend: configuration.Backend, Model: configuration.Model, Thinking: thinking}
}

// AssistantResponse is the configuration assistant as the API returns it.
// `command` and `checkCommand` are written as explicit nulls when absent. They
// are pointers to slices because a nil slice encodes as [] (jsonx), not null.
type AssistantResponse struct {
	Running       bool                           `json:"running"`
	Busy          bool                           `json:"busy"`
	Session       string                         `json:"session"`
	Command       *[]string                      `json:"command"`
	Configuration AssistantConfigurationResponse `json:"configuration"`
	CheckCommand  *[]string                      `json:"checkCommand"`
	Problems      []string                       `json:"problems"`
	Actions       []core.AssistantAction         `json:"actions"`
}

// NewAssistantResponse builds the assistant response from its arguments.
func NewAssistantResponse(resource coordinator.AssistantResource) AssistantResponse {
	return AssistantResponse{
		Running:       resource.Running,
		Busy:          resource.Busy,
		Session:       resource.Session,
		Command:       apiOptionalStrings(resource.Command),
		Configuration: NewAssistantConfigurationResponse(resource.Configuration),
		CheckCommand:  apiOptionalStrings(resource.CheckCommand),
		Problems:      append([]string{}, resource.Problems...),
		Actions:       resource.Actions,
	}
}

// apiOptionalStrings copies an optional list: nil stays nil (null on the wire),
// anything else becomes its own non-nil copy.
func apiOptionalStrings(values []string) *[]string {
	if values == nil {
		return nil
	}
	copied := append([]string{}, values...)
	return &copied
}
