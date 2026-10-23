// Shadow tests: a belt runs a second, shadow version beside its live one, and
// each new run starts on one of the two and keeps it to the end.

package core

import (
	"slices"
	"strconv"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ShadowArm is the version a run of a shadow test started on.
type ShadowArm string

const (
	ShadowArmLive   ShadowArm = "live"
	ShadowArmShadow ShadowArm = "shadow"
)

// AllShadowArm lists the cases of ShadowArm in declaration order.
var AllShadowArm = []ShadowArm{ShadowArmLive, ShadowArmShadow}

// ShadowResult is how a shadow test ended.
//
// Encoding: a string (its raw value); empty while the test runs.
type ShadowResult string

const (
	ShadowResultAccepted  ShadowResult = "accepted"
	ShadowResultDiscarded ShadowResult = "discarded"
)

// AllShadowResult lists the cases of ShadowResult in declaration order.
var AllShadowResult = []ShadowResult{ShadowResultAccepted, ShadowResultDiscarded}

// ShadowTest is one shadow test of a belt, kept in its state.json after it ends
// so Insights can still compare it.
type ShadowTest struct {
	// ID tells the belt's tests apart: "t1", "t2" and so on.
	ID string `json:"id"`
	// Live is the live version when the test started.
	Live int `json:"live"`
	// Shadow is the shadow's latest version.
	Shadow    int         `json:"shadow"`
	StartedAt jsonx.Date  `json:"startedAt"`
	EndedAt   *jsonx.Date `json:"endedAt,omitzero"`
	// Result is how the test ended; empty while it runs.
	Result ShadowResult `json:"result,omitzero"`
}

// Running reports whether the test has not ended.
func (t ShadowTest) Running() bool { return t.EndedAt == nil }

// RunShadow marks a run that started during a shadow test, on either version.
type RunShadow struct {
	Test string    `json:"test"`
	Arm  ShadowArm `json:"arm"`
}

// OnShadow reports whether the run started on a shadow version.
func (r Run) OnShadow() bool { return r.Shadow != nil && r.Shadow.Arm == ShadowArmShadow }

// ShadowChip is the chip of a run on a shadow version, such as "Shadow v2"; nil for any other run.
func ShadowChip(run Run) *string {
	if !run.OnShadow() {
		return nil
	}
	return new("Shadow v" + strconv.Itoa(run.BeltVersion))
}

// ShadowRunning is the belt's running test, or nil.
func ShadowRunning(tests []ShadowTest) *ShadowTest {
	for i := range tests {
		if tests[i].Running() {
			test := tests[i]
			return &test
		}
	}
	return nil
}

// ShadowNextTestID is the id of a belt's next test.
func ShadowNextTestID(tests []ShadowTest) string {
	return "t" + strconv.Itoa(len(tests)+1)
}

// ShadowEnd ends the running test with result at now, or returns tests unchanged when none runs.
func ShadowEnd(tests []ShadowTest, result ShadowResult, now jsonx.Date) []ShadowTest {
	out := slices.Clone(tests)
	for i := range out {
		if out[i].Running() {
			out[i].EndedAt = &now
			out[i].Result = result
		}
	}
	return out
}

// ShadowPickArm chooses the version a new run of test starts on: the one fewer of
// the test's runs started on, a tie drawn with random.
func ShadowPickArm(test string, runs []Run, random ModelPoolRNG) ShadowArm {
	counts := map[ShadowArm]int{}
	for i := range runs {
		if mark := runs[i].Shadow; mark != nil && mark.Test == test {
			counts[mark.Arm]++
		}
	}
	switch {
	case counts[ShadowArmLive] < counts[ShadowArmShadow]:
		return ShadowArmLive
	case counts[ShadowArmShadow] < counts[ShadowArmLive]:
		return ShadowArmShadow
	}
	return AllShadowArm[modelPoolRandomIndex(random, uint64(len(AllShadowArm)))]
}

// ShadowDefinition is the shadow version as a run starts on it: its own stages,
// forks, policies and AFK, with live's triggers, base and repository bases, so
// both versions take the same work.
func ShadowDefinition(shadow, live Belt) Belt {
	out := shadow.Clone()
	out.Name = live.Name
	out.Triggers = cloneEach(live.Triggers, TriggerEntry.Clone)
	out.Base = live.Base
	out.RepositoryBases = RepositoryBasesClean(live.RepositoryBases)
	return out
}
