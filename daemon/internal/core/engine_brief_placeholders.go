// The placeholder values and the
// output paths a brief refers to.

package core

import (
	"path/filepath"
	"slices"
)

// briefPlaceholders are the values of each placeholder a brief can use, for stage
// in run: the fixed ones and each stage's key.
// The map has no order: callers replace in sorted key order.
func briefPlaceholders(run Run, stage Stage, context BriefContext) map[string]string {
	values := map[string]string{}
	if run.UsesStageKeys() {
		for _, other := range run.Stages {
			if other.Kind() != StageKindAgent {
				continue
			}
			key := StageKeysChosen(other)
			if key == "" || slices.Contains(BriefFixedPlaceholders, key) {
				continue
			}
			if _, taken := values[key]; taken {
				continue
			}
			values[key] = StageKeysPathOfKey(key, run, stage, context)
		}
	} else {
		for _, other := range run.Stages {
			if other.Kind() != StageKindAgent {
				continue
			}
			key := briefLegacyKey(other, context)
			if key == "" {
				continue
			}
			if _, taken := values[key]; taken {
				continue
			}
			values[key] = briefLegacyKeyPath(key, run, stage, context)
		}
		// Agent keys have precedence over stage ids when names collide.
		for _, other := range run.Stages {
			if other.Kind() != StageKindAgent {
				continue
			}
			if _, taken := values[other.ID]; taken {
				continue
			}
			values[other.ID] = briefPathOfStageID(other.ID, run, stage, context)
		}
		// An agent key or stage id named artifacts, from before the placeholder, keeps it.
		if _, taken := values["artifacts"]; !taken {
			values["artifacts"] = briefArtifactsPath(context)
		}
	}
	if run.UsesStageKeys() {
		values["artifacts"] = briefArtifactsPath(context)
	}
	values["output"] = BriefOutputPath(stage, context)
	values["base"] = context.Base
	values["branch"] = context.Branch
	values["issue"] = run.Subject.Ref() + " in " + run.Subject.Repo
	values["previous"] = briefPrevious(run, stage, context)
	return values
}

// briefPrevious is the path of the latest output an agent stage other than stage
// wrote.
func briefPrevious(run Run, stage Stage, context BriefContext) string {
	for index := len(run.Attempts) - 1; index >= 0; index-- {
		attempt := run.Attempts[index]
		if attempt.StageID == stage.ID || attempt.IsOpen() {
			continue
		}
		other, ok := briefStageNamed(run, attempt.StageID)
		if !ok || other.Kind() != StageKindAgent {
			continue
		}
		return filepath.Join(context.RunFolder, attempt.Folder, other.Writes())
	}
	return "(no earlier output yet)"
}

// briefLegacyKey is the placeholder of a saved run predating stage keys, which
// keeps its pinned agent-key behaviour.
func briefLegacyKey(stage Stage, context BriefContext) string {
	return StageKeysLegacyName(stage, context.Agents)
}

// briefPathOfStageID is the latest output path for a stage id; the current stage
// resolves to this attempt's output, even before it has written anything.
func briefPathOfStageID(id string, run Run, stage Stage, context BriefContext) string {
	if stage.ID == id {
		return BriefOutputPath(stage, context)
	}
	for index := len(run.Attempts) - 1; index >= 0; index-- {
		attempt := run.Attempts[index]
		if attempt.StageID != id || attempt.IsOpen() {
			continue
		}
		other, ok := briefStageNamed(run, id)
		if !ok || other.Kind() != StageKindAgent {
			continue
		}
		return filepath.Join(context.RunFolder, attempt.Folder, other.Writes())
	}
	writes := id
	for _, other := range run.Stages {
		if other.ID == id && other.Kind() == StageKindAgent {
			writes = other.Writes()
			break
		}
	}
	return "(no " + writes + " yet)"
}

// briefLegacyKeyPath is the path of the latest output of a stage whose agent has
// key, for a run saved before stage keys: this attempt's own output when this
// stage's agent has it. The first matching stage in belt order owns the name, and
// a later attempt of any of them can still be read.
func briefLegacyKeyPath(key string, run Run, stage Stage, context BriefContext) string {
	if briefLegacyKey(stage, context) == key {
		return BriefOutputPath(stage, context)
	}
	for index := len(run.Attempts) - 1; index >= 0; index-- {
		attempt := run.Attempts[index]
		other, ok := briefStageNamed(run, attempt.StageID)
		if !ok || briefLegacyKey(other, context) != key {
			continue
		}
		return filepath.Join(context.RunFolder, attempt.Folder, other.Writes())
	}
	writes := key
	for _, other := range run.Stages {
		if briefLegacyKey(other, context) == key {
			writes = other.Writes()
			break
		}
	}
	return "(no " + writes + " yet)"
}

// briefStageNamed is the stage of run with id, if there is one.
func briefStageNamed(run Run, id string) (Stage, bool) {
	for _, stage := range run.Stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return Stage{}, false
}
