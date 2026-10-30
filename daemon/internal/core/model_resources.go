// The bundled resources: the sample library's agent files (engine_sample_agents.go),
// whose Custom agent is also the template a new agent starts from, and the
// configuration assistant's prompt, embedded from the resources/ folder beside this file.

package core

import (
	"embed"
	"io/fs"
)

//go:embed resources/agents/*.md
var sampleAgentFiles embed.FS

//go:embed resources/assistant/prompt.md
var assistantPromptFile string

// SampleAgentText is the file of a sample agent, and whether there is one with
// that id, read from the embedded resources/agents/<id>.md.
func SampleAgentText(id string) (string, bool) {
	data, err := fs.ReadFile(sampleAgentFiles, "resources/agents/"+id+".md")
	if err != nil {
		return "", false
	}
	return string(data), true
}

// AgentTemplate is the agent a new agent starts from: the Custom agent, with no
// id, key or version of its own yet.
func AgentTemplate() Agent {
	if custom := SampleAgentNamed("custom", nil); custom != nil {
		return *custom
	}
	return NewAgent(AgentArgs{ID: "custom", Name: "Custom agent"})
}

// AssistantPromptTemplate is the contract the configuration assistant's first
// message starts with, bundled as Assistant/prompt.md.
func AssistantPromptTemplate() string {
	return assistantPromptFile
}
