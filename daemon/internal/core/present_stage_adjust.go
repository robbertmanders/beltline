// Stage adjustment: run-local agent
// settings (neither the belt nor the agent pin is edited), the placeholder
// entries a brief offers, and the Adjustment history a stage keeps. Adjustment
// and its fields are stored on Run.

package core

import (
	"slices"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// AdjustmentField is a stage setting a person changed.
type AdjustmentField string

const (
	AdjustmentFieldBackend  AdjustmentField = "backend"
	AdjustmentFieldModel    AdjustmentField = "model"
	AdjustmentFieldThinking AdjustmentField = "thinking"
	AdjustmentFieldBrief    AdjustmentField = "brief"
)

// Label is the name shown for the field: "Effort" for thinking, otherwise the
// raw value capitalised.
func (f AdjustmentField) Label() string {
	if f == AdjustmentFieldThinking {
		return "Effort"
	}
	return CapitalizedFirst(string(f))
}

// Adjustment is a person's change to a stage, in save order. The engine never reads this history.
type Adjustment struct {
	StageID string            `json:"stageId"`
	Date    jsonx.Date        `json:"date"`
	Fields  []AdjustmentField `json:"fields"`
}

// StageAdjustDraft is what Adjust is editing for one stage. Switching backend
// normalizes model and effort; the brief stays.
type StageAdjustDraft struct {
	Runner   Backend
	Model    string
	Thinking *Thinking
	Brief    string
}

// StageAdjustNoAgentStage is the refusal when Adjust names a stage that is not an agent stage of
// the run. The Adjust commands refuse with it.
const StageAdjustNoAgentStage = "Choose an agent stage in this run."

// StageAdjustFreshSessionNote tells the person that changes apply to the next
// fresh session.
const StageAdjustFreshSessionNote = "Changes apply to the next fresh session. Answering or continuing an existing session keeps its original backend, model, and effort."

// StageAdjustNoModelsAllowed is shown when no model is allowed for the backend in
// the run.
const StageAdjustNoModelsAllowed = "No models allowed for this backend in this run. Choose another backend."

// StageAdjustLegacyRepairNote is shown under the model list of a legacy stage that
// was saved with no model.
const StageAdjustLegacyRepairNote = "This legacy blank stage can be repaired once using a currently allowed model."

// StageAdjustResolutionNote says what each chip resolves to, once under the rows
// of chips.
const StageAdjustResolutionNote = "A stage key is that stage's output on the route this run took. The current stage's own key is this attempt's file. Going back into a fork keeps the outputs already written there until they are written again. A missing output reads (no <filename> yet), and a key no route has selected reads (no <key> output yet). Fixed names keep their meanings."

// StageAdjustLegacyResolutionNote is the note under the chips of a run saved before
// stage keys.
const StageAdjustLegacyResolutionNote = "Agent names resolve to that agent's latest output; stage ids resolve to that stage's latest output. Fixed names take precedence. {artifacts} is the current attempt's artifact folder, outside the checkout, for optional standalone HTML files."

// StageAdjustFixed are the fixed placeholder names.
var StageAdjustFixed = BriefFixedPlaceholders

// StageAdjustCanAdjust reports whether stage can be adjusted in run: an agent
// stage, while the run is active and the stage is still in it.
func StageAdjustCanAdjust(stage Stage, run Run) bool {
	if stage.Kind() != StageKindAgent || !run.State.IsActive() {
		return false
	}
	return slices.ContainsFunc(run.Stages, func(candidate Stage) bool { return candidate.ID == stage.ID })
}

// AdjustmentLabels joins the labels of the fields an adjustment changed, in order:
// "Model, Effort".
func AdjustmentLabels(fields []AdjustmentField) string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, field.Label())
	}
	return strings.Join(labels, ", ")
}

// AdjustmentHistoryLine is one adjustment's line in the stage panel's history:
// "Build adjusted 4 min ago · Model, Effort".
func AdjustmentHistoryLine(stageName string, adjustment Adjustment, now time.Time) string {
	return stageName + " adjusted " + RelativeTimeAgo(adjustment.Date.Time, now) + " · " + AdjustmentLabels(adjustment.Fields)
}

// StageAdjustBackend is the backend a stage runs on. Older stages omitted it; that
// is Pi.
func StageAdjustBackend(stage Stage) Backend {
	if stage.Runner != nil {
		return *stage.Runner
	}
	return BackendPi
}

// StageAdjustSupportedEffort is the effort menu for runner: the runner's default,
// then each level it accepts, in level order.
func StageAdjustSupportedEffort(runner Backend) []Thinking {
	supported := runner.SupportedThinking()
	var efforts []Thinking
	for _, level := range AllThinking {
		if _, ok := supported[level]; ok {
			efforts = append(efforts, level)
		}
	}
	return efforts
}

// StageAdjustSwitching is draft moved to runner. The model is kept only when its
// exact id is allowed for runner, and the effort only when runner accepts it.
// Switching back uses this same rule, so a cleared model or effort is not restored
// from a hidden draft.
func StageAdjustSwitching(draft StageAdjustDraft, to Backend, allowedModelIDs []string) StageAdjustDraft {
	next := draft
	next.Runner = to
	if !slices.Contains(allowedModelIDs, draft.Model) {
		next.Model = ""
	}
	if draft.Thinking != nil {
		if _, ok := to.SupportedThinking()[*draft.Thinking]; !ok {
			next.Thinking = nil
		}
	}
	return next
}

// StageAdjustEffectiveBrief is the brief a stage runs with: its own when it has
// one, else the agent's (nil agent gives "").
func StageAdjustEffectiveBrief(stage Stage, agent *Agent) string {
	if stage.Brief != "" {
		return stage.Brief
	}
	if agent == nil {
		return ""
	}
	return agent.Brief
}

// StageAdjustOwnBrief is the brief text a stage keeps for itself, given the
// agent's.
func StageAdjustOwnBrief(text *string, agent *Agent) string {
	value := ValueOf(text)
	if agent != nil && value == agent.Brief {
		return ""
	}
	return value
}

// Placeholder is one output name a brief can use, with the labels of
// the stages that write it.
type Placeholder struct {
	Key    string
	Labels []string
}

// StageAdjustEntries is one output name, with every stage that writes it. Keys come
// first in belt order. A key shared by several stages names each of them, and its
// route when route is supplied (nil when not). legacyIDs also offers each agent
// stage's id, for a run saved before stage keys. key is the stage's key, nil when
// it has none.
func StageAdjustEntries(stages []Stage, key func(stage Stage) *string, route func(stage Stage) string, legacyIDs bool) []Placeholder {
	var entries []Placeholder
	var agents []Stage
	for _, stage := range stages {
		if stage.Kind() == StageKindAgent {
			agents = append(agents, stage)
		}
	}
	type keyedStage struct {
		Stage Stage
		Name  string
	}
	var keyed []keyedStage
	for _, stage := range agents {
		if name := key(stage); name != nil && *name != "" {
			keyed = append(keyed, keyedStage{Stage: stage, Name: *name})
		}
	}
	count := map[string]int{}
	for _, item := range keyed {
		count[item.Name]++
	}
	for _, item := range keyed {
		label := item.Stage.Name
		if count[item.Name] > 1 && route != nil {
			label = item.Stage.Name + " (" + route(item.Stage) + ")"
		}
		stageAdjustAdd(item.Name, label, &entries)
	}
	if legacyIDs {
		for _, stage := range agents {
			stageAdjustAdd(stage.ID, stage.Name, &entries)
		}
	}
	return entries
}

// StageAdjustPlaceholders are the names StageAdjustEntries draws, in the same
// order, so the chips and this flat list never disagree about what a belt offers.
// stageIDs are the stage ids a legacy run also offers.
func StageAdjustPlaceholders(keys []string, stageIDs []string) []string {
	var unique []string
	for _, key := range append(append([]string{}, keys...), stageIDs...) {
		if !slices.Contains(unique, key) {
			unique = append(unique, key)
		}
	}
	return append(unique, BriefFixedPlaceholders...)
}

// stageAdjustAdd adds name with label to entries, or adds label to the entry that
// has name already. An empty name is ignored.
func stageAdjustAdd(name, label string, entries *[]Placeholder) {
	if name == "" {
		return
	}
	for index := range *entries {
		if (*entries)[index].Key != name {
			continue
		}
		for _, existing := range (*entries)[index].Labels {
			if existing == label {
				return
			}
		}
		labels := append(append([]string{}, (*entries)[index].Labels...), label)
		(*entries)[index] = Placeholder{Key: name, Labels: labels}
		return
	}
	*entries = append(*entries, Placeholder{Key: name, Labels: []string{label}})
}

// StageAdjustTarget is the backend an adjustment saves: the one the person chose,
// else the backend the stage's session runs on now (the settings' runner, else the
// stage's own).
func StageAdjustTarget(chosen *Backend, current SessionSettings, stage Stage) Backend {
	switch {
	case chosen != nil:
		return *chosen
	case current.Runner != nil:
		return *current.Runner
	default:
		return StageAdjustBackend(stage)
	}
}

// StageAdjustChoice is the model with exactly modelID among choices, or nil when
// modelID is blank or no choice has it.
func StageAdjustChoice(choices []ModelChoice, modelID string) *ModelChoice {
	if strings.TrimSpace(modelID) == "" {
		return nil
	}
	for i := range choices {
		if choices[i].ID == modelID {
			return &choices[i]
		}
	}
	return nil
}

// StageAdjustChangedFields lists, in field order, the settings an adjustment to
// backend, modelID, thinking and ownBrief would change on stage now, given its
// current session settings. It is nil when nothing changes.
func StageAdjustChangedFields(current SessionSettings, stage Stage, backend Backend, modelID string, thinking *Thinking, ownBrief string) []AdjustmentField {
	var fields []AdjustmentField
	if stageAdjustRunsOn(current, stage) != backend {
		fields = append(fields, AdjustmentFieldBackend)
	}
	if current.Model != modelID {
		fields = append(fields, AdjustmentFieldModel)
	}
	if !SamePointee(current.Thinking, thinking) {
		fields = append(fields, AdjustmentFieldThinking)
	}
	if stage.Brief != ownBrief {
		fields = append(fields, AdjustmentFieldBrief)
	}
	return fields
}

// StageAdjustChangesRunner reports whether fields change what the stage's session
// runs with (its backend, model or effort), as opposed to only its brief.
func StageAdjustChangesRunner(fields []AdjustmentField) bool {
	return slices.ContainsFunc(fields, func(field AdjustmentField) bool {
		return field != AdjustmentFieldBrief
	})
}

// stageAdjustRunsOn is the backend a stage's session runs on now: the current
// settings' runner, else the stage's own.
func stageAdjustRunsOn(current SessionSettings, stage Stage) Backend {
	if current.Runner != nil {
		return *current.Runner
	}
	return StageAdjustBackend(stage)
}
