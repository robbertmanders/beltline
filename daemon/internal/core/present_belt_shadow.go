// The belt editor's view of a running shadow test: its shadow version, how the
// shadow differs from live, and how many runs each version took.

package core

import (
	"strconv"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ShadowStageChange is how one stage of the shadow differs from live.
type ShadowStageChange string

const (
	ShadowStageAdded   ShadowStageChange = "added"
	ShadowStageChanged ShadowStageChange = "changed"
	ShadowStageRemoved ShadowStageChange = "removed"
)

// AllShadowStageChange lists the cases of ShadowStageChange in declaration order.
var AllShadowStageChange = []ShadowStageChange{ShadowStageAdded, ShadowStageChanged, ShadowStageRemoved}

// ShadowStageDifference is one stage the shadow added, changed or removed, by stage id.
type ShadowStageDifference struct {
	StageID string            `json:"stageId"`
	Name    string            `json:"name"`
	Change  ShadowStageChange `json:"change"`
	// Label is the tag the editor puts on the stage: "Added", "Changed" or "Removed".
	Label string `json:"label"`
}

// ShadowRunCounts is how many of a test's runs started on each version.
type ShadowRunCounts struct {
	Live   int `json:"live"`
	Shadow int `json:"shadow"`
}

// ShadowSummary is the running test as the belt resource carries it.
type ShadowSummary struct {
	Test string `json:"test"`
	// Version is the shadow's latest version, and Live the live version when the test started.
	Version   int             `json:"version"`
	Live      int             `json:"live"`
	StartedAt jsonx.Date      `json:"startedAt"`
	Runs      ShadowRunCounts `json:"runs"`
	// Draft is the shadow as the editor opens it.
	Draft Belt `json:"draft"`
	// Warnings are the shadow's warnings; while it has any, new runs all start on live.
	Warnings    []string                `json:"warnings"`
	Differences []ShadowStageDifference `json:"differences"`
	// PoliciesDiffer and AfkDiffers say whether the shadow changed the belt's policies or AFK mode.
	PoliciesDiffer bool `json:"policiesDiffer"`
	AfkDiffers     bool `json:"afkDiffers"`
	// Label is the shadow's name in chips, such as "Shadow v2"; Caption the editor's line about the test.
	Label   string `json:"label"`
	Caption string `json:"caption"`
}

// NewShadowSummary builds the summary of test, whose shadow is shadow beside the live belt.
func NewShadowSummary(test ShadowTest, live, shadow Belt, runs []Run, warnings []string) ShadowSummary {
	counts := ShadowArmCounts(test.ID, runs)
	caption := "Shadow test since " + test.StartedAt.Format("Jan 2") + ". Each new run starts on whichever version has had fewer, and keeps it to the end."
	if len(warnings) > 0 {
		caption = "New runs all start on live until the shadow's warnings are resolved."
	}
	return ShadowSummary{
		Test: test.ID, Version: shadow.Version, Live: test.Live, StartedAt: test.StartedAt, Runs: counts,
		Draft: shadow, Warnings: append([]string{}, warnings...), Differences: ShadowDifferences(live, shadow),
		PoliciesDiffer: live.Policies != shadow.Policies, AfkDiffers: live.Afk != shadow.Afk,
		Label: "Shadow v" + strconv.Itoa(shadow.Version), Caption: caption,
	}
}

// ShadowArmCounts counts the runs of test on each version.
func ShadowArmCounts(test string, runs []Run) ShadowRunCounts {
	var counts ShadowRunCounts
	for i := range runs {
		mark := runs[i].Shadow
		if mark == nil || mark.Test != test {
			continue
		}
		if mark.Arm == ShadowArmShadow {
			counts.Shadow++
		} else {
			counts.Live++
		}
	}
	return counts
}

// shadowSameStage reports whether two copies of a stage save alike: what a stage is, is what it saves.
func shadowSameStage(a, b Stage) bool {
	left, errA := jsonx.Marshal(a)
	right, errB := jsonx.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}

// ShadowDifferences are the shadow's stages that live does not have or has
// otherwise, in the shadow's order, then the live stages it removed.
func ShadowDifferences(live, shadow Belt) []ShadowStageDifference {
	byID := map[string]Stage{}
	for _, stage := range live.Stages {
		byID[stage.ID] = stage
	}
	out := []ShadowStageDifference{}
	kept := map[string]bool{}
	for _, stage := range shadow.Stages {
		before, ok := byID[stage.ID]
		kept[stage.ID] = ok
		switch {
		case !ok:
			out = append(out, ShadowStageDifference{StageID: stage.ID, Name: stage.Name, Change: ShadowStageAdded, Label: "Added"})
		case !shadowSameStage(before, stage):
			out = append(out, ShadowStageDifference{StageID: stage.ID, Name: stage.Name, Change: ShadowStageChanged, Label: "Changed"})
		}
	}
	for _, stage := range live.Stages {
		if !kept[stage.ID] {
			out = append(out, ShadowStageDifference{StageID: stage.ID, Name: stage.Name, Change: ShadowStageRemoved, Label: "Removed"})
		}
	}
	return out
}
