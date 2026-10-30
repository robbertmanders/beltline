// Tests of the shipping intent on a saved run: an in-progress run keeps it across a save and read,
// and the work it still has to finish ends as Shipped; a run saved before Done existed stays Shipped.

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// shipStoreBelt is a belt of one build stage whose done outcome takes the given action.
func shipStoreBelt(action core.ThenAction) core.Belt {
	stage := core.DefaultsStageForAgentAt(core.SampleAgent("build"), core.DefaultsPlace{}, nil, core.TriggerKindIssues)
	stage.Outcomes[0].Then = core.NewThen(core.ThenArgs{Action: action})
	trigger := core.DefaultIssueFilter()
	trigger.Label = "agent"
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: "ship", Name: "Ship", Trigger: core.Trigger{Kind: core.TriggerKindIssues, Issues: &trigger},
		Repos: []string{"acme/billing"}, Stages: []core.Stage{stage},
	})
}

// shipStoreFinish starts the run's session and ends it with the outcome done.
func shipStoreFinish(run *core.Run, now time.Time) []core.Effect {
	core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionStarted, Session: "bl-ship", SessionID: "id", Folder: "f"}, now)
	output := "Work done.\n\nOutcome: done"
	return core.EngineAdvance(run, core.Event{Kind: core.EventKindSessionFinished, Output: &output}, now)
}

// TestAnInProgressShippedRunSavesAndFinishesAsShipped saves a run that has its shipping intent
// before its last stage, reads it back, finishes that stage and checks the run is Shipped.
func TestAnInProgressShippedRunSavesAndFinishesAsShipped(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	subject := core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 7, Title: "Export"}
	run := core.NewRun(core.RunArgs{Belt: shipStoreBelt(core.ThenActionNext), Subject: subject, Now: now})
	core.EngineStart(&run, now)
	run.Shipping = true
	if err := s.SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	loaded := s.LoadRuns()
	if len(loaded) != 1 {
		t.Fatalf("loaded %d runs", len(loaded))
	}
	if !loaded[0].Shipping {
		t.Fatalf("the shipping intent did not survive a save and read")
	}
	want, err := jsonx.MarshalStorage(run)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jsonx.MarshalStorage(loaded[0]); err != nil || string(got) != string(want) {
		t.Fatalf("loaded run differs (%v):\n%s", err, testkit.Diff(string(want), string(got)))
	}
	back := loaded[0]
	shipStoreFinish(&back, now)
	if back.State != core.RunStateShipped {
		t.Fatalf("state = %q, want shipped", back.State)
	}
}

// TestALegacyShippedRunStaysShipped checks that a pull request review saved as shipped before Done
// existed loads as shipped, keeps no shipping intent to infer from, and still counts in the
// historical shipping figures.
func TestALegacyShippedRunStaysShipped(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	run := goldenRun(t)
	run.Subject = core.Subject{Kind: core.SubjectKindPullRequest, Repo: "acme/billing", Number: 12, Title: "Review", IsOpen: true}
	run.State = core.RunStateShipped
	run.Shipping = false
	if err := s.SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	loaded := s.LoadRuns()
	if len(loaded) != 1 || loaded[0].State != core.RunStateShipped || loaded[0].Subject.Kind != core.SubjectKindPullRequest {
		t.Fatalf("loaded %+v, want one shipped review", loaded)
	}
	data, err := json.Marshal(loaded[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, saved := fields["shipping"]; saved {
		t.Errorf("a run with no shipping intent saved a shipping key: %s", data)
	}
	now := run.UpdatedAt.Add(time.Hour)
	view := core.Insights(loaded, nil, core.InsightsQuery{Period: core.InsightsPeriodAll}, now, time.UTC)
	if len(view.BeltRows) != 1 || view.BeltRows[0].ShipRate.Percent != "100%" || view.BeltRows[0].LeadTime == "–" {
		t.Errorf("historical insights = %+v, want the legacy shipped review counted", view.BeltRows)
	}
}

// TestACompletedDoneRunSavesAndLoadsAsDone finishes a run with a Done action, saves it and reads it
// back: it loads as Done, with no shipping intent, and reads back the same as it was saved.
func TestACompletedDoneRunSavesAndLoadsAsDone(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	now := time.Unix(1_800_000_000, 0).UTC()
	subject := core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Export"}
	run := core.NewRun(core.RunArgs{Belt: shipStoreBelt(core.ThenActionDone), Subject: subject, Now: now})
	core.EngineStart(&run, now)
	shipStoreFinish(&run, now)
	if run.State != core.RunStateDone || run.Shipping {
		t.Fatalf("finished as %q shipping %t, want done and not shipping", run.State, run.Shipping)
	}
	if err := s.SaveRun(run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	loaded := s.LoadRuns()
	if len(loaded) != 1 || loaded[0].State != core.RunStateDone || loaded[0].Shipping {
		t.Fatalf("loaded %+v, want one done run that is not shipped", loaded)
	}
	want, err := jsonx.MarshalStorage(run)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jsonx.MarshalStorage(loaded[0]); err != nil || string(got) != string(want) {
		t.Fatalf("loaded run differs (%v):\n%s", err, testkit.Diff(string(want), string(got)))
	}
}

// TestAnUpgradedBeltKeepsItsFinishingActionsAndItsRunsEndAsDone upgrades a folder whose belt
// finishes with Done, and one whose belt finishes with Next. It holds a run already saved as Done and
// a run still at its last stage: the stored belt and the active run's snapshot keep the action, the
// saved Done run stays Done and unshipped, and the active run ends as Done, since nothing ships it.
func TestAnUpgradedBeltKeepsItsFinishingActionsAndItsRunsEndAsDone(t *testing.T) {
	t.Parallel()
	for _, action := range []core.ThenAction{core.ThenActionDone, core.ThenActionNext} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			s := seededStorage(t)
			belt := shipStoreBelt(action)
			if err := s.Write(belt, filepath.Join(s.Paths.Belts(), belt.ID, "v1.json")); err != nil {
				t.Fatal(err)
			}
			now := time.Unix(1_800_000_000, 0).UTC()
			subject := func(number int) core.Subject {
				return core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: number, Title: "Export"}
			}
			active := core.NewRun(core.RunArgs{Belt: belt, Subject: subject(8), Now: now})
			core.EngineStart(&active, now)
			finished := core.NewRun(core.RunArgs{Belt: belt, Subject: subject(9), Now: now})
			core.EngineStart(&finished, now)
			shipStoreFinish(&finished, now)
			if finished.State != core.RunStateDone {
				t.Fatalf("the saved run finished as %q, want done", finished.State)
			}
			for _, run := range []core.Run{active, finished} {
				if err := s.SaveRun(run); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Upgrade(now); err != nil {
				t.Fatal(err)
			}
			stored := s.Belt(belt.ID, 1)
			if stored == nil || stored.Stages[0].Outcomes[0].Then.Action != action {
				t.Fatalf("belt after upgrade = %+v, want its %s action kept", stored, action)
			}
			byID := map[string]core.Run{}
			for _, run := range s.LoadRuns() {
				byID[run.ID] = run
			}
			if len(byID) != 2 {
				t.Fatalf("loaded %d runs after upgrade, want 2", len(byID))
			}
			if saved := byID[finished.ID]; saved.State != core.RunStateDone || saved.Shipping {
				t.Fatalf("saved done run after upgrade = %q shipping %t, want done and not shipping", saved.State, saved.Shipping)
			}
			left := byID[active.ID]
			if left.State != core.RunStateQueued || left.Shipping {
				t.Fatalf("active run after upgrade = %q shipping %t, want queued and not shipping", left.State, left.Shipping)
			}
			if got := left.Stages[0].Outcomes[0].Then.Action; got != action {
				t.Fatalf("active run's snapshot after upgrade has %q, want its %s action kept", got, action)
			}
			shipStoreFinish(&left, now)
			if left.State != core.RunStateDone {
				t.Fatalf("state = %q, want done", left.State)
			}
		})
	}
}
