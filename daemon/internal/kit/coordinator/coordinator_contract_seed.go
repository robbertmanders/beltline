// Test-only seeding for the shared contract harness in internal/server/servertest: the contract
// state the goldens were recorded from, the run book's runs, and the changes the route tests make.
// Production code never calls these.

package coordinator

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// SeedContractTemplate applies the contract template's changes to a demo's
// storage: one ended pause, the earliest belt archived, the second belt paused,
// the first sample agent archived beside a custom agent, and every run without
// a model admission given the admission the template gives it (the fixture
// models, plus the models its stages and attempts name). The fixture catalogs
// and the settings are the demo's own. Test-only: see the file comment.
// Lock: none (it works on storage before a coordinator exists).
func SeedContractTemplate(storage store.Storage, now time.Time) error {
	settings := storage.LoadSettings()
	settings.Pauses = []store.DateInterval{{Start: jsonx.Date{Time: now.Add(-time.Hour)}, Duration: 1800}}
	if err := storage.SaveSettings(settings); err != nil {
		return err
	}
	fixture := contractFixtureModels()
	for _, run := range storage.LoadRuns() {
		if run.ModelAdmission != nil {
			continue
		}
		models := map[string][]core.ModelChoice{}
		for backend, choices := range fixture {
			models[backend] = append([]core.ModelChoice(nil), choices...)
		}
		include := func(backend core.Backend, model string) {
			if strings.TrimSpace(model) == "" {
				return
			}
			for _, choice := range models[string(backend)] {
				if choice.ID == model {
					return
				}
			}
			models[string(backend)] = append(models[string(backend)], core.ModelChoice{ID: model, Name: model})
		}
		for _, stage := range run.Stages {
			if stage.Kind() != core.StageKindAgent {
				continue
			}
			backend := core.ValueOr(stage.Runner, core.BackendPi)
			include(backend, stage.Model)
		}
		for _, attempt := range run.Attempts {
			stage := findStage(run, attempt.StageID)
			if stage == nil || stage.Kind() != core.StageKindAgent || attempt.Settings == nil {
				continue
			}
			backendChanged := attemptSettingsBackendAdjusted(run, attempt, stage.ID)
			if attempt.Settings.Runner != nil || !backendChanged {
				backend := core.BackendPi
				if attempt.Settings.Runner != nil {
					backend = *attempt.Settings.Runner
				} else if stage.Runner != nil {
					backend = *stage.Runner
				}
				include(backend, attempt.Settings.Model)
			}
		}
		admission := core.NewModelAdmission(models, map[string]struct{}{}, attemptSettingsFallbacks(run))
		run.ModelAdmission = &admission
		if err := storage.SaveRun(run); err != nil {
			return err
		}
	}
	belts := storage.LoadBelts()
	sortBeltRecords(belts)
	if len(belts) > 0 {
		if _, err := storage.SaveNewVersion(belts[0].Belt); err != nil {
			return err
		}
		if err := storage.SetArchived(belts[0].Belt.ID, true); err != nil {
			return err
		}
	}
	if len(belts) > 1 {
		if err := storage.SetPaused(belts[1].Belt.ID, true); err != nil {
			return err
		}
	}
	agents := storage.LoadAgents()
	if len(agents) > 0 {
		first := agents[0]
		for _, record := range agents[1:] {
			if record.Agent.ID < first.Agent.ID {
				first = record
			}
		}
		if _, err := storage.SaveNewAgentVersion(first.Agent); err != nil {
			return err
		}
		if err := storage.SetAgentArchived(first.Agent.ID, true); err != nil {
			return err
		}
	}
	_, err := storage.SaveNewAgentVersion(core.NewAgent(core.AgentArgs{ID: "fixture-agent", Name: "Fixture custom"}))
	return err
}

// contractFixtureModels is the fixture model list every backend admits, sorted by id
// FixtureModelCatalog.models.sorted), keyed by backend.
func contractFixtureModels() map[string][]core.ModelChoice {
	models, _ := runner.FixtureModelCatalog{}.Discover(context.Background(), core.BackendPi, "")
	sort.SliceStable(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	out := map[string][]core.ModelChoice{}
	for _, backend := range core.AllBackend {
		out[string(backend)] = models
	}
	return out
}

// SeedContractStart applies what the contract fixture does to a non-empty
// coordinator before a test uses it: the intake's last check is now, and the run
// book has two notices, one without a run and one for the first run in readRuns
// order. Test-only. Lock: takes c.mu.
func (c *Coordinator) SeedContractStart(now time.Time) {
	runs := c.ReadRuns()
	c.Lock()
	defer c.EndTurn()
	at := now
	c.intake.setLastCheck(at)
	c.book.note("Contract notice without a run", nil)
	if len(runs) > 0 {
		id := runs[0].ID
		c.book.note("Contract notice for "+id, &id)
	}
}

// SeedContractViews applies the ContractHarness.seedViews: the waiting
// subjects of the first belt's intake, and the AFK decisions the autopilot
// holds for the first run in readRuns order (six decisions in every status the
// log shows). Test-only. Lock: takes c.mu for its synchronous part; the belt and run lists are read first.
func (c *Coordinator) SeedContractViews(now time.Time) {
	beltID := "issues"
	if belts := c.ReadBelts(); len(belts) > 0 {
		beltID = belts[0].Belt.ID
	}
	// The first run in readRuns order (created, then id), which ReadRuns gives.
	runs := c.ReadRuns()
	c.Lock()
	defer c.EndTurn()
	c.intake.setWaiting("contract|waiting|issues", []core.WaitingSubject{
		{
			BeltID: beltID,
			Subject: core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/web", Number: 9001,
				Title: "Contract waiting issue", Labels: []string{"agent"}, IsOpen: true},
			Reason: "waiting on PAY-42",
		},
		{
			BeltID: beltID,
			Subject: core.Subject{Kind: core.SubjectKindJira, Repo: "acme/billing", Key: "PAY-900",
				Title: "Contract waiting ticket", Labels: []string{"agent"}, IsOpen: true},
			Reason: "waiting on KAN-1",
		},
	})
	if len(runs) == 0 {
		return
	}
	run := runs[0]
	stageName := run.StageID
	if stage := run.Stage(); stage != nil {
		stageName = stage.Name
	}
	offset := func(seconds float64) jsonx.Date {
		return jsonx.Date{Time: now.Add(time.Duration(seconds * float64(time.Second)))}
	}
	situation := func(kind core.AfkSituationKind, key string) core.AfkSituation {
		return core.AfkSituation{Kind: kind, Key: key}
	}

	applied := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-applied", Run: run,
		Situation: situation(core.AfkSituationKindApproval, "contract-applied"), StartedAt: offset(-300), Folder: "afk/1-approve"})
	applied.Status = core.AfkDecisionStatusApplied
	applied.DecidedAt = &jsonx.Date{Time: now.Add(-240 * time.Second)}
	applied.Choice = new("decide approved")
	applied.Action = new("Approved: approved")
	applied.Message = new("Ship it.")
	applied.Why = new("The checks pass and the change is small.")
	backend := core.BackendPi
	applied.Backend = &backend
	applied.Model = new("contract-model")
	applied.Reviewed = true
	c.autopilot.add(applied)

	// A decision still in progress whose run is no longer present.
	deciding := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-deciding", Run: run,
		Situation: situation(core.AfkSituationKindFailure, "contract-deciding"), StartedAt: offset(-220), Folder: "afk/3-build"})
	deciding.RunID = "missing-run"
	deciding.BeltName = "Missing belt"
	c.autopilot.add(deciding)

	failed := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-failed", Run: run,
		Situation: situation(core.AfkSituationKindHold, "contract-failed"), StartedAt: offset(-200), Folder: "afk/4-merge"})
	failed.Status = core.AfkDecisionStatusFailed
	failed.DecidedAt = &jsonx.Date{Time: now.Add(-150 * time.Second)}
	failed.Detail = new("The session wrote no decision.")
	c.autopilot.add(failed)

	superseded := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-superseded", Run: run,
		Situation: situation(core.AfkSituationKindQuestion, "contract-superseded"), StartedAt: offset(-190), Folder: "afk/5-triage"})
	superseded.Status = core.AfkDecisionStatusSuperseded
	superseded.DecidedAt = &jsonx.Date{Time: now.Add(-140 * time.Second)}
	superseded.Choice = new("answer")
	c.autopilot.add(superseded)

	// An applied decision a person overruled and restarted.
	restarted := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-restarted", Run: run,
		Situation: situation(core.AfkSituationKindApproval, "contract-restarted"), StartedAt: offset(-185), Folder: "afk/6-approve"})
	restarted.Status = core.AfkDecisionStatusApplied
	restarted.DecidedAt = &jsonx.Date{Time: now.Add(-130 * time.Second)}
	restarted.Action = new("Chose “approved” at Approve")
	restarted.Overrule = &core.AfkOverrule{Comments: "Run it again with the smaller change.", StageID: run.StageID,
		StageName: stageName, RouteName: new("Build"), At: offset(-70), AttemptID: "contract-attempt-2",
		Phase: core.AfkOverrulePhaseRestarted}
	c.autopilot.add(restarted)

	overruled := core.NewAfkDecision(core.AfkDecisionArgs{ID: "contract-afk-overruled", Run: run,
		Situation: situation(core.AfkSituationKindQuestion, "contract-overruled"), StartedAt: offset(-180), Folder: "afk/2-triage"})
	overruled.Status = core.AfkDecisionStatusLeftForYou
	overruled.DecidedAt = &jsonx.Date{Time: now.Add(-120 * time.Second)}
	overruled.Reason = new("held: needs a decision")
	overruled.Detail = new("The instructions conflict.")
	overruled.Overrule = &core.AfkOverrule{Comments: "Use the second option.", StageID: run.StageID,
		StageName: stageName, At: offset(-60), AttemptID: "contract-attempt", Phase: core.AfkOverrulePhaseHeld,
		Detail: new("The run moved on.")}
	c.autopilot.add(overruled)
}

// AppendRunForContract appends run to the run book in memory, without saving it, as the
// Route tests append to book.runs. Test-only. Lock: takes c.mu.
func (c *Coordinator) AppendRunForContract(run core.Run) {
	c.Lock()
	defer c.EndTurn()
	c.book.appendRunForContract(run)
}

// AddAfkDecisionForContract adds decision to the AFK log in memory, as the route tests
// call `coordinator.autopilot.add(decision)`. Test-only. Lock: takes c.mu.
func (c *Coordinator) AddAfkDecisionForContract(decision core.AfkDecision) {
	c.Lock()
	defer c.EndTurn()
	c.autopilot.add(decision)
}

// UpdateRunForContract changes the run id in place through the run book, as the route
// tests change book.runs (RunBook.update: the change is saved). Test-only. Lock: takes c.mu.
func (c *Coordinator) UpdateRunForContract(id string, change func(run *core.Run)) bool {
	c.Lock()
	defer c.EndTurn()
	return c.book.update(id, nil, nil, change)
}

// SeedContractSupplemental applies the contract fixture's supplemental change: the sample
// triage agent saved as a new version with high thinking, then the library reloaded (ContractHarness
// `supplemental: true`). Test-only. Lock: takes c.mu.
func (c *Coordinator) SeedContractSupplemental() error {
	c.Lock()
	defer c.EndTurn()
	record := c.library.LatestAgent("triage")
	if record == nil {
		return core.NewFailure("the contract demo has no triage agent")
	}
	agent := record.Agent
	thinking := core.ThinkingHigh
	agent.Thinking = &thinking
	if _, err := c.storage.SaveNewAgentVersion(agent); err != nil {
		return err
	}
	c.library.reload(false)
	return nil
}

// ApplyEventForContract applies event to the run id through the run book, as the daemon
// tests call `coordinator.book.apply(id, event)`: effects are carried out and the change is saved.
// Test-only. Lock: takes c.mu.
func (c *Coordinator) ApplyEventForContract(id string, event core.Event) bool {
	c.Lock()
	defer c.EndTurn()
	return c.book.apply(id, event, nil, nil, nil)
}

// AutopilotCheckForContract runs one autopilot pass, as the daemon tests call
// `coordinator.autopilot.check()`. Test-only. Lock: takes c.mu; the pass suspends where it calls out.
func (c *Coordinator) AutopilotCheckForContract() {
	c.Lock()
	defer c.EndTurn()
	c.autopilot.check(c.Context())
}

// WaitForStartupProbeForContract waits until the startup reconciliation the coordinator spawned
// has ended, as `assistant.waitForStartupProbe()` does (Idle also waits for the work it
// spawns). Test-only. Lock: none; call it without c.mu held.
func (c *Coordinator) WaitForStartupProbeForContract() {
	c.Idle()
}
