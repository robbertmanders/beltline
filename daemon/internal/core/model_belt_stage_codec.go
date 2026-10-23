// Stage's JSON decoding and encoding. The types are in model_belt_stage.go.

package core

import (
	"encoding/json"
	"fmt"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
	"slices"
)

// MarshalJSON writes the stage with its keys, the saved file name as writes, and
// alternates only when there are any.
func (s Stage) MarshalJSON() ([]byte, error) {
	return jsonx.Marshal(s.wire())
}

// WireShape is the value a stage's keys are written by: stageWire, whose tags name every
// key a stage reads and writes (the stage's own tags leave out writes). A request body's
// stage is checked against it (routes).
func (s Stage) WireShape() any {
	return s.wire()
}

// wire is the stage's encoded form.
func (s Stage) wire() stageWire {
	w := stageWire{
		ID: s.ID, Type: s.Type, Name: s.Name, Agent: s.Agent, Key: s.Key, Runner: s.Runner,
		Model: s.Model, Thinking: s.Thinking, Workspace: s.Workspace, Writes: s.Writes(),
		Slots: s.Slots, Brief: s.Brief, Outcomes: s.Outcomes, Fallback: s.Fallback,
		Route: s.Route, Watch: s.Watch,
	}
	if len(s.Alternates) > 0 {
		w.Alternates = s.Alternates
	}
	return w
}

// stageWire is the encoded form of a Stage: every stored field under its key,
// with writes in place of the saved file name and alternates only when there are any.
// A request body's stage must give id, type, name and outcomes (required:"true").
type stageWire struct {
	ID         string            `json:"id" required:"true"`
	Type       StageType         `json:"type" required:"true"`
	Name       string            `json:"name" required:"true"`
	Agent      *AgentRef         `json:"agent,omitzero"`
	Key        *string           `json:"key,omitzero"`
	Runner     *Backend          `json:"runner,omitzero"`
	Model      string            `json:"model"`
	Thinking   *Thinking         `json:"thinking,omitzero"`
	Alternates []SessionSettings `json:"alternates,omitzero"`
	Workspace  Workspace         `json:"workspace"`
	Writes     string            `json:"writes"`
	Slots      int               `json:"slots"`
	Brief      string            `json:"brief"`
	Outcomes   []Outcome         `json:"outcomes" required:"true"`
	Fallback   Fallback          `json:"fallback"`
	Route      *string           `json:"route,omitzero"`
	Watch      *WatchTarget      `json:"watch,omitzero"`
}

// stageLegacyActionNames are the stage types LegacyAction reads: the action stages
// saved before agents made every outward call. Decoding a stage offers each
// other type to LegacyAction, which returns nothing for it.
var stageLegacyActionNames = map[string]struct{}{
	"ship": {}, "comment": {}, "label": {}, "submitReview": {}, "transition": {},
}

// UnmarshalJSON reads a stage over the defaults of its missing keys. The legacy
// action types (see legacyAction) become agent stages. A type saved before
// agents had their own files names the built-in agent, and its `agent` key is
// the CLI. Any other type must be a known StageType.
func (s *Stage) UnmarshalJSON(data []byte) error {
	var head struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	if _, ok := stageLegacyActionNames[head.Type]; ok {
		var fields legacyActionFields
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		legacy, err := newLegacyAction(head.Type, fields)
		if err != nil {
			return err
		}
		if legacy != nil {
			*s = legacy.stage(head.ID)
			var route struct {
				Route *string `json:"route"`
			}
			if err := json.Unmarshal(data, &route); err != nil {
				return err
			}
			if route.Route != nil {
				s.Route = route.Route
			}
			return nil
		}
	}
	type plain Stage
	var w struct {
		plain
		// Agent shadows plain's agent: a legacy agent type's CLI, or the pin.
		Agent  json.RawMessage `json:"agent"`
		Writes *string         `json:"writes"`
	}
	w.Workspace, w.Slots, w.Fallback = WorkspaceReadOnly, 2, FallbackHold
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*s = Stage(w.plain)
	if _, ok := StageTypeLegacyAgents[head.Type]; ok {
		s.Type = StageTypeAgent
		s.Agent = &AgentRef{ID: head.Type, Version: 1}
		runner := BackendPi
		var cli *Backend
		if err := codecDecodeRaw(w.Agent, &cli); err != nil {
			return err
		}
		if cli != nil {
			runner = *cli
		}
		s.Runner = &runner
	} else {
		if !slices.Contains(AllStageType, s.Type) {
			return fmt.Errorf("unknown stage type %q", head.Type)
		}
		if err := codecDecodeRaw(w.Agent, &s.Agent); err != nil {
			return err
		}
	}
	if s.Type.Kind() != StageKindAgent {
		s.Key = nil
	}
	if len(s.Alternates) == 0 {
		s.Alternates = nil
	}
	if w.Writes != nil {
		s.SetWrites(*w.Writes)
	}
	return nil
}
