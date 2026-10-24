package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// ThinkingPatch is an AFK or assistant thinking field that a patch may set. It
// ThinkingPatch is an AFK or assistant thinking field that a patch may set. It
// Present false is an omitted field, Present true with a nil Value is an explicit null, which selects Default.
type ThinkingPatch struct {
	Present bool
	Value   *core.Thinking
}

// SettingsPatch is the fields a settings patch may change. Managed state (pause
// and its history, AFK enabled, the assistant configuration, catalogs and model
// permissions) is absent: those have their own commands. Requests are strict:
// unknown keys are refused, and a declared field may not be null.
type SettingsPatch struct {
	Login               *string                   `json:"login,omitzero"`
	Notifications       *bool                     `json:"notifications,omitzero"`
	LibraryRepositories *[]core.LibraryRepository `json:"libraryRepositories,omitzero"`
	Afk                 *AfkSettingsPatch         `json:"afk,omitzero"`
	ShowInsights        *bool                     `json:"showInsights,omitzero"`
}

// UnmarshalJSON decodes a settings patch, refusing undeclared keys and nulls.
// Lock: none (pure decoding).
func (p *SettingsPatch) UnmarshalJSON(data []byte) error {
	type plain SettingsPatch
	var w plain
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if err := rejectNulls(data, "login", "notifications", "libraryRepositories", "afk", "showInsights"); err != nil {
		return err
	}
	*p = SettingsPatch(w)
	return nil
}

// AfkSettingsPatch is a partial edit to the AFK preferences. Thinking
// distinguishes omitted from an explicit null, which selects Default.
type AfkSettingsPatch struct {
	AnswerQuestions *bool         `json:"answerQuestions,omitzero"`
	RerunFailures   *bool         `json:"rerunFailures,omitzero"`
	Approve         *bool         `json:"approve,omitzero"`
	ResolveHolds    *bool         `json:"resolveHolds,omitzero"`
	Drop            *bool         `json:"drop,omitzero"`
	Limit           *int          `json:"limit,omitzero"`
	Guidance        *string       `json:"guidance,omitzero"`
	Backend         *core.Backend `json:"backend,omitzero"`
	Model           *string       `json:"model,omitzero"`
	Thinking        ThinkingPatch `json:"thinking,omitzero"`
}

// UnmarshalJSON decodes an AFK settings patch, refusing undeclared keys and nulls
// other than thinking, which an explicit null selects Default for.
// Lock: none (pure decoding).
func (p *AfkSettingsPatch) UnmarshalJSON(data []byte) error {
	type plain AfkSettingsPatch
	var w struct {
		plain
		// Thinking shadows plain's, so the patch sees whether the key was given.
		Thinking json.RawMessage `json:"thinking"`
	}
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if err := rejectNulls(data, "answerQuestions", "rerunFailures", "approve", "resolveHolds", "drop", "limit", "guidance", "backend", "model"); err != nil {
		return err
	}
	*p = AfkSettingsPatch(w.plain)
	if p.Backend != nil && !slices.Contains(core.AllBackend, *p.Backend) {
		return fmt.Errorf("unknown backend %q", *p.Backend)
	}
	if w.Thinking != nil {
		p.Thinking = ThinkingPatch{Present: true}
		if !isJSONNull(w.Thinking) {
			var thinking core.Thinking
			if err := json.Unmarshal(w.Thinking, &thinking); err != nil {
				return err
			}
			p.Thinking.Value = &thinking
		}
	}
	return nil
}

// AssistantConfiguration is the complete assistant selection a session starts with.
type AssistantConfiguration struct {
	Backend  core.Backend   `json:"backend"`
	Model    string         `json:"model"`
	Thinking *core.Thinking `json:"thinking,omitzero"`
}

// UnmarshalJSON decodes an assistant configuration, which must give its backend,
// model and thinking (null for the default), and refuses undeclared keys.
// Lock: none (pure decoding).
func (a *AssistantConfiguration) UnmarshalJSON(data []byte) error {
	type plain AssistantConfiguration
	var w struct {
		plain
		// Shadows plain's fields, so a missing key is told from a null one.
		Backend  json.RawMessage `json:"backend"`
		Model    json.RawMessage `json:"model"`
		Thinking json.RawMessage `json:"thinking"`
	}
	if err := decodeStrict(data, &w); err != nil {
		return err
	}
	if w.Backend == nil || isJSONNull(w.Backend) || w.Model == nil || isJSONNull(w.Model) {
		return errors.New("The data couldn't be read because it is missing.") //nolint:staticcheck // ST1005: the wording is the one a missing request field has always had.
	}
	if w.Thinking == nil {
		return core.NewFailure(`Missing field "thinking"`)
	}
	*a = AssistantConfiguration(w.plain)
	if err := json.Unmarshal(w.Backend, &a.Backend); err != nil {
		return err
	}
	if err := json.Unmarshal(w.Model, &a.Model); err != nil {
		return err
	}
	a.Thinking = nil
	if !isJSONNull(w.Thinking) {
		var thinking core.Thinking
		if err := json.Unmarshal(w.Thinking, &thinking); err != nil {
			return err
		}
		a.Thinking = &thinking
	}
	return nil
}

// decodeStrict reads data into v, refusing a key v does not declare at any depth.
// Lock: none (pure).
func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// rejectNulls refuses a declared optional field, named by keys, that holds null:
// The optional patch fields decode their value, which null is not.
// Lock: none (pure).
func rejectNulls(data []byte, keys ...string) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	for _, key := range keys {
		if raw, ok := members[key]; ok && isJSONNull(raw) {
			return core.NewFailure(fmt.Sprintf("Expected a value for %q but found null.", key))
		}
	}
	return nil
}

// isJSONNull reports whether raw is the JSON null literal.
// Lock: none (pure).
func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// ApplySettingsPatch validates the whole candidate, then persists it before
// publishing, so a rejected patch or a failed write never changes the effective
// settings.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ApplySettingsPatch(patch SettingsPatch) (store.Settings, error) {
	c.Lock()
	defer c.EndTurn()
	return c.applySettingsPatchNow(patch)
}

// applySettingsPatchNow is ApplySettingsPatch with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) applySettingsPatchNow(patch SettingsPatch) (store.Settings, error) {
	draft := c.settings
	if patch.Login != nil {
		draft.Login = *patch.Login
	}
	if patch.Notifications != nil {
		draft.Notifications = *patch.Notifications
	}
	if patch.LibraryRepositories != nil {
		if problems := core.LibraryRepositoriesProblems(*patch.LibraryRepositories); len(problems) > 0 {
			return store.Settings{}, core.NewFailure("The library repositories are not valid.", problems...)
		}
		draft.LibraryRepositories = *patch.LibraryRepositories
	}
	if patch.ShowInsights != nil {
		draft.ShowInsights = *patch.ShowInsights
	}
	if patch.Afk != nil {
		afk, err := CandidateAfk(c.settings.Afk, *patch.Afk)
		if err != nil {
			return store.Settings{}, err
		}
		draft.Afk = afk
	}
	if err := c.publishSettings(draft); err != nil {
		return store.Settings{}, err
	}
	return cloneSettings(draft), nil
}

// PatchAfk applies the AFK page's partial edits through the same validator as
// the API.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) PatchAfk(patch AfkSettingsPatch) (store.Settings, error) {
	c.Lock()
	defer c.EndTurn()
	return c.applySettingsPatchNow(SettingsPatch{Afk: &patch})
}

// SetGlobalAfk turns global AFK on or off. As the app did, it takes effect
// without checking the AFK model: a run that cannot be decided for is left for you
// .
// Lock: none; it delegates to SetAfk, which takes c.mu.
func (c *Coordinator) SetGlobalAfk(ctx context.Context, enabled bool) {
	c.SetAfk(ctx, enabled)
}

// ConfigureAssistant saves the complete assistant selection for the next
// session. A running or busy assistant refuses changes.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ConfigureAssistant(configuration AssistantConfiguration) (store.Settings, error) {
	c.Lock()
	defer c.EndTurn()
	if c.assistant.busy || c.assistant.running {
		return store.Settings{}, core.NewFailure("The assistant is running. Stop it before changing its configuration.")
	}
	if err := c.validateConfiguration(configuration, c.settings, "Assistant"); err != nil {
		return store.Settings{}, err
	}
	draft := c.settings
	draft.ConfigurationBackend = configuration.Backend
	draft.ConfigurationModel = configuration.Model
	draft.ConfigurationThinking = configuration.Thinking
	if err := c.publishSettings(draft); err != nil {
		return store.Settings{}, err
	}
	return cloneSettings(draft), nil
}

// StopAssistant stops the configuration assistant.
// Lock: exported entry point; takes c.mu. Suspends: assistant.Stop, which releases the
// lock around its runner calls itself.
func (c *Coordinator) StopAssistant(ctx context.Context) error {
	c.Lock()
	defer c.EndTurn()
	if c.assistant.busy {
		return core.NewFailure("The assistant is busy.")
	}
	c.assistant.Stop(ctx)
	return nil
}

// publishSettings persists draft before publishing it, so an unsuccessful write
// never changes the effective settings. The model-permission commands and the
// settings patch share this write path.
// Lock: runs with c.mu held.
func (c *Coordinator) publishSettings(draft store.Settings) error {
	if err := c.storage.SaveSettings(draft); err != nil {
		return err
	}
	c.publishingSavedSettings = true
	defer func() { c.publishingSavedSettings = false }()
	// The settings change goes through changeSettings, as the settings' change hook does:
	// the saved write above is not repeated because publishingSavedSettings is set.
	c.changeSettings(func(s *store.Settings) { *s = draft })
	c.assistant.noticeChanges(true)
	return nil
}

// CandidateAfk applies the AFK field edits, each checked for type and range
// only: AFK may be on with a model that is no longer allowed.
// Lock: none (pure).
func CandidateAfk(current core.AfkSettings, patch AfkSettingsPatch) (core.AfkSettings, error) {
	candidate := current
	if patch.AnswerQuestions != nil {
		candidate.AnswerQuestions = *patch.AnswerQuestions
	}
	if patch.RerunFailures != nil {
		candidate.RerunFailures = *patch.RerunFailures
	}
	if patch.Approve != nil {
		candidate.Approve = *patch.Approve
	}
	if patch.ResolveHolds != nil {
		candidate.ResolveHolds = *patch.ResolveHolds
	}
	if patch.Drop != nil {
		candidate.Drop = *patch.Drop
	}
	if patch.Limit != nil {
		if *patch.Limit < 1 || *patch.Limit > 10 {
			return core.AfkSettings{}, core.NewFailure("AFK makes between 1 and 10 decisions per stage of a run.")
		}
		candidate.Limit = *patch.Limit
	}
	if patch.Guidance != nil {
		candidate.Guidance = *patch.Guidance
	}
	if patch.Backend != nil {
		candidate.Backend = *patch.Backend
	}
	if patch.Model != nil {
		candidate.Model = *patch.Model
	}
	if patch.Thinking.Present {
		candidate.Thinking = patch.Thinking.Value
	}
	return candidate, nil
}

// validateConfiguration checks a nonblank, currently admitted model and a thinking
// level the runner takes; the model must still be in the saved permission set
// . It reads the agent
// executables through c.env, never the process environment.
// Lock: runs with c.mu held.
func (c *Coordinator) validateConfiguration(configuration AssistantConfiguration, settings store.Settings, name string) error {
	model := strings.TrimSpace(configuration.Model)
	if model == "" {
		return core.NewFailure(name + ": choose an allowed model.")
	}
	if warning := settings.ModelWarning(configuration.Model, configuration.Backend, name, c.env); warning != nil {
		return core.NewFailure(*warning)
	}
	if configuration.Thinking != nil {
		if _, ok := configuration.Backend.SupportedThinking()[*configuration.Thinking]; !ok {
			return core.NewFailure(fmt.Sprintf("%s: %s doesn't take the thinking level %s.", name, configuration.Backend.Label(), configuration.Thinking.Label()))
		}
	}
	return nil
}
