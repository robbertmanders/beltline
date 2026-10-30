// Tests of the Cursor, Claude and Copilot catalog parsing and the variant choice. The admission checks
// are not in this file.

package runner

import (
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
)

// cursorCatalogFixture is `cursor-agent models` output for the variant tests.
const cursorCatalogFixture = "Available models\n\n" +
	"auto - Auto (default)\n" +
	"composer-2.5 - Composer 2.5 (current)\n" +
	"claude-opus-5-5-low - Claude Opus 5.5 1M Low\n" +
	"claude-opus-5-5-low-fast - Claude Opus 5.5 1M Low Fast\n" +
	"claude-opus-5-5-medium - Claude Opus 5.5 1M\n" +
	"claude-opus-5-5-medium-fast - Claude Opus 5.5 1M Fast\n" +
	"claude-opus-5-5-high - Claude Opus 5.5 1M High\n" +
	"claude-opus-5-5-xhigh - Claude Opus 5.5 1M Extra High\n" +
	"claude-opus-5-5-max - Claude Opus 5.5 1M Max\n" +
	"claude-opus-5-thinking-high - Claude Opus 5 1M Thinking\n" +
	"claude-opus-5-thinking-low - Claude Opus 5 1M Low Thinking\n" +
	"claude-4.6-opus-high-thinking - Claude Opus 4.6 1M Thinking\n" +
	"claude-4.6-opus-max-thinking - Claude Opus 4.6 1M Max Thinking\n" +
	"gpt-5.5-medium - GPT-5.5 1M\n" +
	"gpt-5.5-extra-high - GPT-5.5 1M Extra High\n" +
	"gpt-5.3-codex-low - Codex 5.3 Low\n" +
	"gpt-5.3-codex - Codex 5.3\n" +
	"gpt-5.3-codex-high - Codex 5.3 High\n" +
	"grok-4.7-low - Grok 4.7  Low\n" +
	"grok-4.7-high-fast - Grok 4.7  High Fast\u200b\u200b\n\n" +
	"Tip: use --model <id> (or /model <id> in interactive mode) to switch.\n"

// modelsByID indexes choices by identifier.
func modelsByID(models []core.ModelChoice) map[string]core.ModelChoice {
	out := make(map[string]core.ModelChoice, len(models))
	for _, m := range models {
		out[m.ID] = m
	}
	return out
}

// variantsOf is a choice's variants, or nil.
func variantsOf(m core.ModelChoice) map[string]string { return m.Variants }

func TestCursorFoldsLevelsIntoOneChoicePerModelAndSpeed(t *testing.T) {
	t.Parallel()
	choices, err := CLIModelCatalogParseCursor(cursorCatalogFixture)
	if err != nil {
		t.Fatal(err)
	}
	models := modelsByID(choices)
	wantIDs := []string{"auto", "composer-2.5", "claude-opus-5-5", "claude-opus-5-5-fast", "claude-opus-5-thinking",
		"claude-4.6-opus-thinking", "gpt-5.5", "gpt-5.3-codex", "grok-4.7", "grok-4.7-fast"}
	if len(models) != len(wantIDs) {
		t.Fatalf("ids = %d, want %d", len(models), len(wantIDs))
	}
	for _, id := range wantIDs {
		if _, ok := models[id]; !ok {
			t.Errorf("missing %s", id)
		}
	}
	// The name comes from the default variant, which Cursor names without a level.
	checkName := func(id, name string) {
		t.Helper()
		if models[id].Name != name {
			t.Errorf("%s name = %q, want %q", id, models[id].Name, name)
		}
	}
	checkName("claude-opus-5-5", "Claude Opus 5.5 1M")
	if c := models["claude-opus-5-5"].ContextTokens; c == nil || *c != 1_000_000 {
		t.Errorf("opus context = %v", c)
	}
	if got := models["claude-opus-5-5"].DisplayLabel(); got != "Claude Opus 5.5 - 1M" {
		t.Errorf("opus label = %q", got)
	}
	checkName("claude-opus-5-5-fast", "Claude Opus 5.5 1M Fast")
	if got := models["claude-opus-5-5-fast"].DisplayLabel(); got != "Claude Opus 5.5 Fast - 1M" {
		t.Errorf("fast label = %q", got)
	}
	checkName("claude-opus-5-thinking", "Claude Opus 5 1M Thinking")
	if got := models["claude-opus-5-thinking"].DisplayLabel(); got != "Claude Opus 5 Thinking - 1M" {
		t.Errorf("thinking label = %q", got)
	}
	checkName("grok-4.7", "Grok 4.7")
	if models["grok-4.7"].ContextTokens != nil || models["grok-4.7"].DisplayLabel() != "Grok 4.7" {
		t.Errorf("grok = %+v", models["grok-4.7"])
	}
	checkName("grok-4.7-fast", "Grok 4.7 Fast")
	if !modelEqual(models["composer-2.5"], core.ModelChoice{ID: "composer-2.5", Name: "Composer 2.5"}) {
		t.Errorf("composer = %+v", models["composer-2.5"])
	}
	mustEqual(t, map[string]string{
		"default": "claude-opus-5-5-medium", "low": "claude-opus-5-5-low", "medium": "claude-opus-5-5-medium",
		"high": "claude-opus-5-5-high", "xhigh": "claude-opus-5-5-xhigh", "max": "claude-opus-5-5-max",
	}, variantsOf(models["claude-opus-5-5"]))
	if variantsOf(models["gpt-5.5"])["xhigh"] != "gpt-5.5-extra-high" {
		t.Errorf("gpt-5.5 variants = %v", variantsOf(models["gpt-5.5"]))
	}
	// An unsuffixed default stands for medium.
	if variantsOf(models["gpt-5.3-codex"])["medium"] != "gpt-5.3-codex" {
		t.Errorf("codex variants = %v", variantsOf(models["gpt-5.3-codex"]))
	}
}

// modelEqual compares two choices by value.
func modelEqual(a, b core.ModelChoice) bool {
	if a.ID != b.ID || a.Name != b.Name || (a.ContextTokens == nil) != (b.ContextTokens == nil) || len(a.Variants) != len(b.Variants) {
		return false
	}
	if a.ContextTokens != nil && *a.ContextTokens != *b.ContextTokens {
		return false
	}
	for k, v := range a.Variants {
		if b.Variants[k] != v {
			return false
		}
	}
	return true
}

func TestALevelPicksTheListedVariantNearestIt(t *testing.T) {
	t.Parallel()
	choices, err := CLIModelCatalogParseCursor(cursorCatalogFixture)
	if err != nil {
		t.Fatal(err)
	}
	models := modelsByID(choices)
	opus := models["claude-opus-5-5"]
	if got := opus.Identifier(nil); got != "claude-opus-5-5-medium" {
		t.Errorf("nil = %q", got)
	}
	if got := opus.Identifier(new(core.ThinkingHigh)); got != "claude-opus-5-5-high" {
		t.Errorf("high = %q", got)
	}
	if got := opus.Identifier(new(core.ThinkingMax)); got != "claude-opus-5-5-max" {
		t.Errorf("max = %q", got)
	}
	fast := models["claude-opus-5-5-fast"]
	if got := fast.Identifier(new(core.ThinkingMax)); got != "claude-opus-5-5-medium-fast" {
		t.Errorf("fast max = %q", got)
	}
	older := models["claude-4.6-opus-thinking"]
	for _, level := range []*core.Thinking{new(core.ThinkingLow), new(core.ThinkingXHigh), nil} {
		if got := older.Identifier(level); got != "claude-4.6-opus-high-thinking" {
			t.Errorf("older = %q", got)
		}
	}
	// Without variants the identifier is passed as is, at any level.
	if got := (core.ModelChoice{ID: "auto", Name: "Auto"}).Identifier(new(core.ThinkingHigh)); got != "auto" {
		t.Errorf("plain = %q", got)
	}
}

func TestClaudeChoicesPinTheResolvedVersion(t *testing.T) {
	t.Parallel()
	models, err := serverCatalogDecodeClaude([]any{
		map[string]any{"value": "default", "resolvedModel": "claude-opus-5-5", "displayName": "Default (recommended)", "description": "Use the default model (currently Opus 5.5)"},
		map[string]any{"value": "opus", "resolvedModel": "claude-opus-5-5", "displayName": "Opus", "description": "Opus 5.5 · Best for everyday, complex tasks · $4/$20 per Mtok"},
		map[string]any{"value": "claude-fable-5-1[1m]", "resolvedModel": "claude-fable-5-1", "displayName": "Fable", "description": "Fable 5.1 · Most capable"},
		map[string]any{"value": "haiku", "resolvedModel": "claude-haiku-4-5-20251001", "displayName": "Haiku", "description": "Haiku 4.5 · Fastest for quick answers"},
		map[string]any{"value": "legacy", "displayName": "Legacy", "description": "Something else"},
		map[string]any{"value": "claude-opus-4-8", "resolvedModel": "claude-opus-4-8", "displayName": "Opus 4.8", "description": "Best for everyday, complex tasks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []core.ModelChoice{
		{ID: "claude-opus-5-5", Name: "Opus 5.5"},
		{ID: "claude-fable-5-1[1m]", Name: "Fable 5.1", ContextTokens: new(1_000_000)},
		{ID: "claude-haiku-4-5-20251001", Name: "Haiku 4.5"},
		{ID: "legacy", Name: "Legacy"},
		{ID: "claude-opus-4-8", Name: "Opus 4.8"},
	}
	if len(models) != len(want) {
		t.Fatalf("models = %+v", models)
	}
	for i := range want {
		if !modelEqual(models[i], want[i]) {
			t.Errorf("model %d = %+v, want %+v", i, models[i], want[i])
		}
	}
	if models[1].DisplayLabel() != "Fable 5.1 - 1M" || models[0].DisplayLabel() != "Opus 5.5" {
		t.Errorf("labels = %q %q", models[1].DisplayLabel(), models[0].DisplayLabel())
	}
	if _, err := serverCatalogDecodeClaude([]any{map[string]any{"value": "opus"}}); err == nil {
		t.Errorf("a row without a name decoded")
	}
}

func TestClaudeKeepsEachContextIdentityAndDoesNotInferASize(t *testing.T) {
	t.Parallel()
	models, err := serverCatalogDecodeClaude([]any{
		map[string]any{"value": "opus", "resolvedModel": "claude-opus-5-5", "displayName": "Opus", "description": "Opus 5.5 · Best"},
		map[string]any{"value": "opus[1m]", "resolvedModel": "claude-opus-5-5", "displayName": "Opus", "description": "Opus 5.5 · Best"},
		map[string]any{"value": "claude-opus-5-5[1m]", "resolvedModel": "claude-opus-5-5", "displayName": "Opus", "description": "Opus 5.5 1M · Long"},
		map[string]any{"value": "sonnet", "resolvedModel": "claude-sonnet-4-6", "displayName": "Sonnet", "description": "Sonnet 4.6 with 1M context · not a suffix"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"claude-opus-5-5", "opus[1m]", "claude-opus-5-5[1m]", "claude-sonnet-4-6"}
	wantLabels := []string{"Opus 5.5", "Opus 5.5 - 1M", "Opus 5.5 - 1M", "Sonnet 4.6 with 1M context"}
	for i, m := range models {
		if m.ID != wantIDs[i] {
			t.Errorf("id %d = %q, want %q", i, m.ID, wantIDs[i])
		}
		if m.DisplayLabel() != wantLabels[i] {
			t.Errorf("label %d = %q, want %q", i, m.DisplayLabel(), wantLabels[i])
		}
	}
	if models[0].ContextTokens != nil || models[3].ContextTokens != nil || *models[1].ContextTokens != 1_000_000 || *models[2].ContextTokens != 1_000_000 {
		t.Errorf("contexts = %+v", models)
	}
	if models[3].Name != "Sonnet 4.6 with 1M context" {
		t.Errorf("sonnet name = %q", models[3].Name)
	}
}

func TestThinkingStaysInsideTheSelectedContext(t *testing.T) {
	t.Parallel()
	choices, err := CLIModelCatalogParseCursor(`Available models
claude-opus-5-5-low - Claude Opus 5.5 1M Low
claude-opus-5-5-high - Claude Opus 5.5 1M High
claude-opus-5-5-250k-low - Claude Opus 5.5 250k Low
claude-opus-5-5-250k-high - Claude Opus 5.5 250k High
claude-opus-5-5-250k - Claude Opus 5.5 250k
composer-2.5 - Composer 2.5
`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range choices {
		ids = append(ids, m.ID)
	}
	mustEqual(t, []string{"claude-opus-5-5", "claude-opus-5-5-250k", "composer-2.5"}, ids)
	models := modelsByID(choices)
	million, smaller, plain := models["claude-opus-5-5"], models["claude-opus-5-5-250k"], models["composer-2.5"]
	if million.ContextTokens == nil || *million.ContextTokens != 1_000_000 || million.DisplayLabel() != "Claude Opus 5.5 - 1M" {
		t.Errorf("million = %+v", million)
	}
	if smaller.ContextTokens == nil || *smaller.ContextTokens != 250_000 || smaller.DisplayLabel() != "Claude Opus 5.5 - 250k" {
		t.Errorf("smaller = %+v", smaller)
	}
	if plain.ContextTokens != nil || plain.DisplayLabel() != "Composer 2.5" {
		t.Errorf("plain = %+v", plain)
	}
	cases := []struct {
		m     core.ModelChoice
		level core.Thinking
		want  string
	}{
		{million, core.ThinkingHigh, "claude-opus-5-5-high"},
		{million, core.ThinkingMax, "claude-opus-5-5-high"},
		{million, core.ThinkingLow, "claude-opus-5-5-low"},
		{smaller, core.ThinkingHigh, "claude-opus-5-5-250k-high"},
		{smaller, core.ThinkingLow, "claude-opus-5-5-250k-low"},
		{smaller, core.ThinkingMax, "claude-opus-5-5-250k-high"},
	}
	for _, c := range cases {
		level := c.level
		if got := c.m.Identifier(&level); got != c.want {
			t.Errorf("%s at %s = %q, want %q", c.m.ID, level, got, c.want)
		}
	}
	if got := smaller.Identifier(nil); got != "claude-opus-5-5-250k" {
		t.Errorf("smaller unset = %q", got)
	}
}

func TestMixedContextThinkingGroupsAreRejected(t *testing.T) {
	t.Parallel()
	if _, err := CLIModelCatalogParseCursor("Available models\nclaude-opus-5-5-high - Claude Opus 5.5 1M High\nclaude-opus-5-5-low - Claude Opus 5.5 250k Low\n"); err == nil {
		t.Errorf("mixed sizes in one group accepted")
	}
	if _, err := CLIModelCatalogParseCursor("Available models\nclaude-opus-5-5-high - Claude Opus 5.5 1M 250k High\n"); err == nil {
		t.Errorf("two sizes in one name accepted")
	}
}

func TestSavedAgentAndStageIdentifiersStayStrings(t *testing.T) {
	t.Parallel()
	agent := core.SampleAgent("triage")
	agent.Model = "claude-opus-5-5[1m]"
	parsed, err := core.AgentFileParse(core.AgentFileWrite(agent), agent.ID, agent.Version)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Model != "claude-opus-5-5[1m]" {
		t.Errorf("agent model = %q", parsed.Model)
	}
}

func TestAcceptedLevelsAgreeWithValidation(t *testing.T) {
	t.Parallel()
	for _, backend := range core.AllBackend {
		for _, level := range core.AllThinking {
			backend, level := backend, level
			t.Run(string(backend)+"/"+string(level), func(t *testing.T) {
				t.Parallel()
				if !backendTakes(backend, level) {
					if got := backend.Thinking(&level); got != nil && *got == level {
						t.Fatalf("%s keeps unsupported level %s", backend, level)
					}
					return
				}
				agent := core.SampleAgent("triage")
				agent.Runner = backend
				agent.Thinking = &level
				if warnings := core.ValidationAgentWarnings(agent, nil); len(warnings) != 0 {
					t.Fatalf("agent warnings = %v", warnings)
				}
				belt := core.SampleBelt([]string{"acme/calc"}, "agent", core.SampleAgents(), true)
				belt.Stages[0].Runner = &backend
				belt.Stages[0].Thinking = &level
				agents := map[core.AgentRef]core.Agent{}
				var keys []string
				for _, builtIn := range core.SampleAgents() {
					agents[builtIn.Ref()] = builtIn
					keys = append(keys, builtIn.Key)
				}
				if warnings := core.ValidationWarnings(belt, keys, agents); len(warnings) != 0 {
					t.Fatalf("belt warnings = %v", warnings)
				}
				if got := backend.Thinking(&level); got == nil || *got != level {
					t.Fatalf("%s drops %s", backend, level)
				}
			})
		}
	}
}

// backendTakes reports whether a backend takes a thinking level (Backend.supportedThinking).
func backendTakes(backend core.Backend, level core.Thinking) bool {
	_, ok := backend.SupportedThinking()[level]
	return ok
}
