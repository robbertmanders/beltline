// Coordinator tests of the AFK autopilot: the assistant's decisions for a run at its approval,
// its questions and limits, the global switch and each run's own choice, cancellation,
// restarts and the Codex effort.

package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

var bgCtx = context.Background()

// TestOffItDecidesNothing
func TestOffItDecidesNothing(t *testing.T) {
	t.Parallel()
	h := coordGate(t, harnessOptions{})
	for range 3 {
		h.Tick()
	}
	if n := len(coordDecisions(h)); n != 0 {
		t.Fatalf("decisions = %d, want none while AFK is off", n)
	}
	if run := h.Run(); run == nil || run.State != core.RunStateWaiting {
		t.Fatalf("run = %+v, want waiting", run)
	}
}

// TestItOnlyRunsOnAnAllowedModel
func TestItOnlyRunsOnAnAllowedModel(t *testing.T) {
	t.Parallel()
	h := coordGate(t, harnessOptions{})
	setAfk(h, func(afk *core.AfkSettings) { afk.Model = "outside" })
	h.coord.SetAfk(bgCtx, true)
	h.Tick()
	decisions := coordDecisions(h)
	if len(decisions) == 0 {
		t.Fatal("no decision recorded")
	}
	decision := decisions[0]
	if decision.Status != core.AfkDecisionStatusFailed || decision.Session != nil {
		t.Fatalf("decision = %s session %v, want failed with no session", decision.Status, decision.Session)
	}
	if decision.Detail == nil || !strings.Contains(*decision.Detail, "outside is not allowed") {
		t.Fatalf("detail = %v, want the model refusal", decision.Detail)
	}
	if run := h.Run(); run.State != core.RunStateWaiting {
		t.Fatalf("state = %s, want waiting", run.State)
	}
}

// TestItApprovesAndLogsTheDecision
func TestItApprovesAndLogsTheDecision(t *testing.T) {
	t.Parallel()
	h := coordGate(t, harnessOptions{})
	gate := h.Run().StageID
	h.Script.add("afk", "decide approved")
	h.coord.SetAfk(bgCtx, true)
	h.Until("run := h.nowRun(); return run != nil && run.StageID != gate", func() bool { run := h.nowRun(); return run != nil && run.StageID != gate })
	run := h.Run()
	if run.StageID == gate {
		t.Fatal("run did not move past its approval")
	}
	decisions := coordDecisions(h)
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	decision := decisions[0]
	if decision.Status != core.AfkDecisionStatusApplied || decision.Situation != core.AfkSituationKindApproval {
		t.Fatalf("decision = %s %s, want applied approval", decision.Status, decision.Situation)
	}
	if decision.Choice == nil || *decision.Choice != "decide approved" {
		t.Fatalf("choice = %v, want decide approved", decision.Choice)
	}
	if decision.Why == nil || *decision.Why == "" || !decision.NeedsReview() {
		t.Fatalf("why = %v, needs review = %v, want reasons and a review", decision.Why, decision.NeedsReview())
	}
	attempt := run.LastAttempt(gate)
	if attempt == nil || attempt.Outcome == nil || *attempt.Outcome != "approved" {
		t.Fatalf("approval attempt = %+v, want outcome approved", attempt)
	}
	if attempt.AfkDecision == nil || *attempt.AfkDecision != decision.ID {
		t.Fatalf("attempt afkDecision = %v, want %s", attempt.AfkDecision, decision.ID)
	}
	folder := filepath.Join(h.coord.Paths().RunFolder(*run), decision.Folder)
	for _, name := range []string{"brief.md", core.AfkDecisionFile} {
		if _, err := os.Stat(filepath.Join(folder, name)); err != nil {
			t.Fatalf("%s missing from the decision folder: %v", name, err)
		}
	}
	saved := h.storage.LoadAfkDecisions()
	if len(saved) != 1 || saved[0].ID != decision.ID || saved[0].Status != core.AfkDecisionStatusApplied {
		t.Fatalf("saved decisions = %+v, want the applied one", saved)
	}
	if saved[0].Why == nil || decision.Why == nil || *saved[0].Why != *decision.Why {
		t.Fatalf("saved why = %v, want %v", saved[0].Why, decision.Why)
	}
	h.coord.ReviewAfk(nil)
	if n := len(coordUnreviewed(h)); n != 0 {
		t.Fatalf("unreviewed = %d after review, want 0", n)
	}
	if reviewed := h.storage.LoadAfkDecisions(); len(reviewed) != 1 || !reviewed[0].Reviewed {
		t.Fatalf("saved decision not reviewed: %+v", reviewed)
	}
}

// TestItAnswersQuestionsByContinuingTheSession
func TestItAnswersQuestionsByContinuingTheSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordOptedInBelt()}})
	h.Script.add("triage", "needs info", "ready to spec")
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateHeld", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateHeld })
	held := h.Run().Answerable()
	if held == nil {
		t.Fatal("held run has nothing to answer")
	}
	h.Script.add("afk", "answer")
	h.coord.SetAfk(bgCtx, true)
	h.Until("run := h.nowRun(); return run != nil && len(run.Attempts) == 2", func() bool { run := h.nowRun(); return run != nil && len(run.Attempts) == 2 })
	run := h.Run()
	last := run.Attempts[len(run.Attempts)-1]
	if last.Continues == nil || *last.Continues != held.ID {
		t.Fatalf("continues = %v, want %s", last.Continues, held.ID)
	}
	if last.Note == nil || !strings.Contains(*last.Note, "Invoices") {
		t.Fatalf("note = %v, want the answer mentioning Invoices", last.Note)
	}
	decisions := coordDecisions(h)
	if len(decisions) == 0 || decisions[0].Situation != core.AfkSituationKindQuestion {
		t.Fatalf("decisions = %+v, want a question", decisions)
	}
}

// TestItStopsAtTheLimitAndLeavesTheRunForYou
func TestItStopsAtTheLimitAndLeavesTheRunForYou(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{Belts: []core.Belt{coordOptedInBelt()}})
	setAfk(h, func(afk *core.AfkSettings) { afk.Limit = 1 })
	h.Script.add("triage", "needs info", "needs info")
	h.Until("run := h.nowRun(); return run != nil && run.State == core.RunStateHeld", func() bool { run := h.nowRun(); return run != nil && run.State == core.RunStateHeld })
	h.Script.add("afk", "answer")
	h.coord.SetAfk(bgCtx, true)
	h.Until("len(decisionsNow(h)) == 2", func() bool { return len(decisionsNow(h)) == 2 })
	decisions := coordDecisions(h)
	if len(decisions) != 2 || decisions[0].Status != core.AfkDecisionStatusApplied || decisions[1].Status != core.AfkDecisionStatusLeftForYou {
		t.Fatalf("decisions = %+v, want applied then left for you", decisions)
	}
	if decisions[1].Detail == nil || !strings.Contains(*decisions[1].Detail, "already decided 1 time") {
		t.Fatalf("detail = %v, want the limit", decisions[1].Detail)
	}
	if run := h.Run(); run.State != core.RunStateHeld {
		t.Fatalf("state = %s, want held", run.State)
	}
}

// TestADecisionForARunThatMovedOnIsDropped
func TestADecisionForARunThatMovedOnIsDropped(t *testing.T) {
	t.Parallel()
	h := coordGate(t, harnessOptions{})
	run := h.Run()
	setAfk(h, func(afk *core.AfkSettings) { afk.Enabled = true })
	var settings core.AfkSettings
	h.Turn(func() { settings = h.coord.settings.Afk })
	situation := core.AfkSituationOf(*run, settings)
	if situation == nil {
		t.Fatal("no situation at the approval")
	}
	h.Turn(func() {
		h.coord.autopilot.start(bgCtx, *run, *situation, settings)
	})
	// You decide before the assistant's session is looked at.
	h.coord.Decide(run.ID, "approved", nil)
	h.Turn(func() {
		h.coord.autopilot.follow(bgCtx)
	})
	decisions := coordDecisions(h)
	if len(decisions) == 0 || decisions[0].Status != core.AfkDecisionStatusSuperseded {
		t.Fatalf("decisions = %+v, want superseded", decisions)
	}
	if n := len(coordUnreviewed(h)); n != 0 {
		t.Fatalf("unreviewed = %d, want 0", n)
	}
}

// twoBeltWorld is the AFK world over belts: each belt has one run waiting at its approval.
// Belt names are the ids; optedIn belts follow the global switch, the others never.
func twoBeltWorld(t *testing.T, belts []core.Belt, opts harnessOptions) *harness {
	t.Helper()
	opts.Belts = belts
	opts.Issues = []core.Subject{}
	opts.Prepare = func(storage store.Storage) {
		for index, record := range storage.LoadBelts() {
			if err := storage.SaveRun(waitingRun(t, record.Belt, index+1)); err != nil {
				t.Fatalf("save run: %v", err)
			}
		}
	}
	return newHarness(t, opts)
}

// afkBelt is a manual-trigger belt on acme/billing for the AFK world: belt(id, optedIn:).
func afkBelt(id string, optedIn bool) core.Belt {
	belt := core.SampleBelt([]string{"acme/billing"}, "agent", core.SampleAgents(), true)
	belt.ID = id
	belt.Name = id
	belt.Triggers = []core.TriggerEntry{manualEntry("manual", "acme/billing")}
	if optedIn {
		belt.Afk = core.AfkModeGlobal
	} else {
		belt.Afk = core.AfkModeNever
	}
	return belt
}

// TestRunAfkFollowsTheOverrideThenTheGlobalSwitch checks that a run's AFK mode follows its override, then the global switch.
// (one subtest per run AFK case row).
func TestRunAfkFollowsTheOverrideThenTheGlobalSwitch(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	rows := []struct {
		name   string
		choice *bool
		global bool
		starts bool
	}{
		{"inherited, global on", nil, true, true},
		{"inherited, global off", nil, false, false},
		{"explicit on, global on", &yes, true, true},
		{"explicit on, global off", &yes, false, true},
		{"explicit off, global on", &no, true, false},
		{"explicit off, global off", &no, false, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			h := coordGate(t, harnessOptions{})
			h.Script.add("afk", "leave")
			// The run's choice goes first: a tick that SetAfk spawns may run as soon as the lock
			// is free, so the choice must already be saved for a run the switch would start.
			if row.choice != nil {
				h.coord.SetRunAfk(bgCtx, h.Run().ID, modeOf(row.choice))
			}
			if row.global {
				h.coord.SetAfk(bgCtx, true)
			}
			if row.starts {
				h.Until("len(decisionsNow(h)) > 0", func() bool { return len(decisionsNow(h)) > 0 })
			} else {
				for range 4 {
					h.Tick()
				}
			}
			run := h.Run()
			if got := len(coordDecisions(h)) > 0; got != row.starts {
				t.Fatalf("decision started = %v, want %v", got, row.starts)
			}
			if globalAfk(h.coord) != row.global {
				t.Fatalf("global AFK = %v, want %v", !row.global, row.global)
			}
			if !core.SamePointee(run.AfkEnabled, row.choice) {
				t.Fatalf("run choice = %v, want %v", run.AfkEnabled, row.choice)
			}
			if run.AfkEffective(row.global) != row.starts {
				t.Fatalf("effective = %v, want %v", run.AfkEffective(row.global), row.starts)
			}

			beforeFlip := len(coordDecisions(h))
			h.coord.SetAfk(bgCtx, !row.global)
			run = h.Run()
			if !core.SamePointee(run.AfkEnabled, row.choice) {
				t.Fatalf("choice after the switch = %v, want %v", run.AfkEnabled, row.choice)
			}
			if saved := h.storage.LoadRuns(); len(saved) == 0 || !core.SamePointee(saved[0].AfkEnabled, row.choice) {
				t.Fatalf("saved choice after the switch = %+v", saved)
			}
			flipped := !row.global
			wantsDecision := run.AfkEffective(flipped)
			if beforeFlip == 0 && wantsDecision {
				h.Until("len(decisionsNow(h)) == 1", func() bool { return len(decisionsNow(h)) == 1 })
			} else {
				for range 3 {
					h.Tick()
				}
			}
			if got, want := len(coordDecisions(h)), max(beforeFlip, boolInt(wantsDecision)); got != want {
				t.Fatalf("decisions after the switch = %d, want %d", got, want)
			}

			h.coord.SetRunAfk(bgCtx, run.ID, core.AfkModeGlobal)
			run = h.Run()
			if run.AfkEnabled != nil {
				t.Fatalf("choice after resetting = %v, want nil", *run.AfkEnabled)
			}
			if run.AfkEffective(flipped) != flipped {
				t.Fatalf("effective after resetting = %v, want %v", run.AfkEffective(flipped), flipped)
			}
			beforeReset := len(coordDecisions(h))
			if beforeReset == 0 && flipped {
				h.Until("len(decisionsNow(h)) == 1", func() bool { return len(decisionsNow(h)) == 1 })
			} else {
				for range 3 {
					h.Tick()
				}
			}
			if got, want := len(coordDecisions(h)), max(beforeReset, boolInt(flipped)); got != want {
				t.Fatalf("decisions after resetting = %d, want %d", got, want)
			}
		})
	}
}

// boolInt is 1 for true and 0 for false.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
