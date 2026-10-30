// The data folder upgrade's rules: what a belt version and a run read from an
// older file become when they are written back in the current format. The
// walk over the folder is the store's (store.Storage.Upgrade).

package core

import (
	"slices"
	"strings"
)

// UpgradeFormatVersion is the data folder format the upgrade writes: belts with
// trigger entries and stage keys, runs with legs, sources, agent copies and
// stage keys, and belt states with a cursor per schedule entry (1); and a first
// approval that asked for a prompt made a prompt stage, on belts and runs (2).
const UpgradeFormatVersion = 2

// UpgradeBelt is a belt version as the current format saves it. Decoding has
// already turned a single trigger into an entry, afkEnabled into afk, and the
// action and agent stage types into agent stages; this fills each agent
// stage's missing key from its pinned agent, as saving the belt would.
func UpgradeBelt(belt Belt, agents map[AgentRef]Agent) Belt {
	out := StageKeysExplicit(belt, agents)
	out.Stages = UpgradePromptStage(out.Stages)
	return out
}

// UpgradePromptStage is stages with a first approval that asked for a prompt
// made a prompt stage. Before prompt stages, the run page opened a prompt editor
// at an approval with no agent output before it whose outcomes that move the run
// on all pass a note on and none finishes it; at the first stage that was every
// visit. The prompt stage keeps its name and outcomes, which no longer pass the
// text on as a note: it is the run's task now. Its source choice starts withheld,
// as that editor's did.
func UpgradePromptStage(stages []Stage) []Stage {
	if len(stages) == 0 || stages[0].Type != StageTypeApprove || stages[0].Route != nil {
		return stages
	}
	moves := false
	for _, outcome := range stages[0].Outcomes {
		if !outcome.Then.Action.MovesOn() {
			continue
		}
		if !outcome.Then.WithOutput || outcome.Then.Action.Finishes() {
			return stages
		}
		moves = true
	}
	if !moves {
		return stages
	}
	out := cloneEach(stages, Stage.Clone)
	out[0].Type = StageTypePrompt
	for index := range out[0].Outcomes {
		out[0].Outcomes[index].Then.WithOutput = false
	}
	return out
}

// UpgradeRun is a run as the current format saves it. A run without a source
// takes it from startedOn, the belt version it started on (nil when that is
// gone). A stage whose agent the run has no copy of gets one from agents. A
// run started before stage keys gets them: each agent stage without a key
// takes its agent's key, or else the name of the file it wrote, so the
// outputs its attempts left keep their paths; a stage neither gives a usable
// key stays without one.
func UpgradeRun(run Run, agents map[AgentRef]Agent, startedOn *Belt) Run {
	out := run.Clone()
	out.Stages = UpgradePromptStage(out.Stages)
	if out.Source == nil && startedOn != nil {
		out.Source = RunSourceOf(*startedOn, out.Subject.Kind)
	}
	for _, stage := range out.Stages {
		if stage.Kind() != StageKindAgent || stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		if slices.ContainsFunc(out.Agents, func(s AgentSnapshot) bool { return s.Ref() == ref }) {
			continue
		}
		if agent, ok := agents[ref]; ok {
			out.Agents = append(out.Agents, NewAgentSnapshot(agent))
		}
	}
	if out.UsesStageKeys() {
		return out
	}
	own := map[AgentRef]Agent{}
	for _, snapshot := range out.Agents {
		if agent := snapshot.Agent(); agent != nil {
			own[snapshot.Ref()] = *agent
		}
	}
	for index := range out.Stages {
		stage := &out.Stages[index]
		if stage.Kind() != StageKindAgent || stage.Key != nil {
			continue
		}
		if key := upgradeStageKey(*stage, own); key != "" {
			stage.SetKey(&key)
		}
	}
	out.References = new(StageKeysVersion)
	return out
}

// upgradeStageKey is the key a stage of a run from before stage keys takes:
// its legacy name when that names the file the stage wrote, else that file's
// name without .md, else none.
func upgradeStageKey(stage Stage, agents map[AgentRef]Agent) string {
	usable := func(key string) bool {
		return validationLooksLikePlaceholder(key) && !slices.Contains(BriefFixedPlaceholders, key)
	}
	writes := stage.Writes()
	if name := StageKeysLegacyName(stage, agents); usable(name) && StageFile(&name, stage.Type) == writes {
		return name
	}
	if name, ok := strings.CutSuffix(writes, ".md"); ok && usable(name) {
		return name
	}
	return ""
}

// RunSourceOf is the trigger entry of belt a run about a subject of kind came
// from: the first entry whose trigger finds that kind, else the first entry,
// else nil.
func RunSourceOf(belt Belt, kind SubjectKind) *RunSource {
	if len(belt.Triggers) == 0 {
		return nil
	}
	entry := belt.Triggers[0]
	for _, candidate := range belt.Triggers {
		if candidate.Trigger.Kind.Subject() == kind {
			entry = candidate
			break
		}
	}
	return &RunSource{Entry: entry.ID, Trigger: entry.Trigger.Clone(), Repos: append([]string{}, entry.Repos...)}
}
