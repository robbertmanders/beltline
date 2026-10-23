// Run's JSON decoding, with the legacy trigger and pullRequest keys. The keys are
// the struct tags, except that the subject is written under "issue".

package core

import (
	"encoding/json"
)

// UnmarshalJSON reads a run over its struct tags, filling what runs saved by
// older versions lack: the policies, base and repositories kept on a legacy
// trigger, the single pull request under "pullRequest", the model admission
// (which falls back rather than failing the run), a leg for a run saved before
// legs, and a hold for a run at an action stage that is now an agent stage.
func (r *Run) UnmarshalJSON(data []byte) error {
	type plain Run
	var w struct {
		plain
		// ModelAdmission shadows plain's, which is read apart so a bad one falls back.
		ModelAdmission json.RawMessage `json:"modelAdmission"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	f, err := codecFieldsOf(data)
	if err != nil {
		return err
	}
	var legacy struct {
		Trigger     *legacyTrigger `json:"trigger"`
		PullRequest *PullRequest   `json:"pullRequest"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	run := Run(w.plain)
	run.fillEmptyMaps()
	// Runs saved before belts had policies kept a copy of their trigger.
	if !f.present("policies") {
		run.Policies = legacyPolicies()
		if legacy.Trigger != nil {
			run.Policies = legacy.Trigger.Policies()
		}
	}
	if !f.present("base") {
		run.Base = ""
		if legacy.Trigger != nil && legacy.Trigger.Base != nil {
			run.Base = *legacy.Trigger.Base
		}
	}
	if !f.present("repos") {
		run.Repos = []string{run.Subject.Repo}
	}
	if !f.present("pullRequests") {
		run.PullRequests = nil
		if legacy.PullRequest != nil {
			run.PullRequests = []PullRequest{*legacy.PullRequest}
		}
	}
	for index := range run.PullRequests {
		if run.PullRequests[index].Repo == "" {
			run.PullRequests[index].Repo = run.Subject.Repo
		}
	}
	if f.has("modelAdmission") {
		// Invalid present metadata fails closed: an empty admission, not a missing one.
		var admission ModelAdmission
		if codecIsNull(w.ModelAdmission) || json.Unmarshal(w.ModelAdmission, &admission) != nil {
			admission = NewModelAdmission(map[string][]ModelChoice{}, map[string]struct{}{}, nil)
		}
		run.ModelAdmission = &admission
	}
	if len(run.Legs) == 0 && run.State.IsActive() {
		// A run saved before legs: one leg where it is now. A finished run has
		// none, as it had none when it was saved.
		var route *string
		if way := run.Layout().Place(run.StageID); way != nil {
			id := way.Route.ID
			route = &id
		}
		stageID := run.StageID
		run.Legs = []Leg{{ID: "main", Route: route, StageID: &stageID, State: run.State, Reason: run.Reason, Branch: run.Branch}}
	}
	// A run that was at an action stage when those became agent stages has an
	// attempt no session belongs to: hold it, so you choose whether the agent
	// does it again.
	if run.State == RunStateRunning {
		if stage := run.stageRef(); stage != nil && stage.Kind() == StageKindAgent {
			if attempt := run.CurrentAttempt(); attempt != nil && attempt.Session == nil {
				ended := run.UpdatedAt
				run.ChangeCurrentAttempt(func(a *Attempt) {
					a.EndedAt = &ended
					detail := "Beltline made this call itself before agents did"
					a.Detail = &detail
				})
				run.State = RunStateHeld
				reason := stage.Name + " is an agent stage now"
				run.Reason = &reason
			}
		}
	}
	*r = run
	return nil
}
