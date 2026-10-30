// The in-place changes to a belt draft (Belt.apply, pickGoto, setGotoRoute,
// switchTriggerKind, addRepository, removeRepository) and the clamps the editor applies (slotRange,
// addRepository, removeRepository) and the clamps the editor applies (slotRange,
// maxRepeatRange, clampedSlots, clampedOutcomes, clampedTrigger).

package core

import "slices"

// BeltSlotRange is the slot counts the editor's stepper offers.
var BeltSlotRange = IntRange{Lower: 1, Upper: 6}

// BeltMaxRepeatRange is the most times an outcome's goto may repeat, as the
// editor's stepper allowed.
var BeltMaxRepeatRange = IntRange{Lower: 0, Upper: 9}

// BeltClampedSlots is slots limited to BeltSlotRange.
func BeltClampedSlots(slots int) int {
	return min(max(slots, BeltSlotRange.Lower), BeltSlotRange.Upper)
}

// BeltClampedOutcomes is the outcomes with each repeat limit kept to one to nine,
// or none at zero.
func BeltClampedOutcomes(outcomes []Outcome) []Outcome {
	out := make([]Outcome, len(outcomes))
	for i, outcome := range outcomes {
		if outcome.Then.Max != nil {
			if m := *outcome.Then.Max; m > 0 {
				v := min(m, BeltMaxRepeatRange.Upper)
				outcome.Then.Max = &v
			} else {
				outcome.Then.Max = nil
			}
		}
		out[i] = outcome
	}
	return out
}

// BeltClampedTrigger is a trigger whose poll is the nearest of Trigger.pollChoices
// when it has one.
func BeltClampedTrigger(trigger Trigger) Trigger {
	poll := trigger.Poll()
	if poll == nil || len(TriggerPollChoices) == 0 {
		return trigger
	}
	best := TriggerPollChoices[0]
	for _, choice := range TriggerPollChoices[1:] {
		if absInt(choice-*poll) < absInt(best-*poll) {
			best = choice
		}
	}
	clamped := trigger.Clone()
	clamped.SetPoll(&best)
	return clamped
}

// apply applies one editor stage update to the stage at index, preserving the
// stage id and route. A watch-target change resets that watch's default outcomes
// and renames the stage only when its name was the default. Outcomes are taken
// as sent: a goto keeps exactly the targets it names.
func (b *Belt) apply(update StageEdit, index int) {
	if index < 0 || index >= len(b.Stages) {
		return
	}
	stage := b.Stages[index]
	oldTarget := WatchTargetPullRequest
	if wt := stage.WatchTarget(); wt != nil {
		oldTarget = *wt
	}
	if update.Watch != nil && stage.Kind() == StageKindWatch && *update.Watch != oldTarget {
		watch := *update.Watch
		if watch == WatchTargetPullRequest {
			stage.Watch = nil
		} else {
			stage.Watch = &watch
		}
		if stage.Name == oldTarget.Title() {
			stage.Name = watch.Title()
		}
		if watch == WatchTargetTicket {
			stage.Outcomes = DefaultsTicketWatchOutcomes()
		} else {
			stage.Outcomes = DefaultsStage(StageTypeWatch, b.Stages[:index], nil, b.Stages).Outcomes
		}
	}
	if update.Source != nil && stage.Type == StageTypePrompt {
		stage.Source = new(*update.Source)
	}
	if update.Name != nil {
		stage.Name = *update.Name
	}
	if update.Brief != nil {
		stage.Brief = *update.Brief
	}
	if update.Writes != nil {
		stage.SetWrites(*update.Writes)
	}
	if update.Key != nil {
		key := *update.Key
		stage.SetKey(&key)
	}
	if update.Runner != nil {
		runner := *update.Runner
		stage.Runner = &runner
	}
	if update.Model != nil {
		stage.Model = *update.Model
	}
	if update.Thinking != nil {
		thinking := *update.Thinking
		stage.Thinking = &thinking
	}
	if draftIsTrue(update.ClearThinking) {
		stage.Thinking = nil
	}
	if update.Workspace != nil {
		stage.Workspace = *update.Workspace
	}
	if update.Slots != nil {
		stage.Slots = BeltClampedSlots(*update.Slots)
	}
	if update.Fallback != nil {
		stage.Fallback = *update.Fallback
	}
	if update.Agent != nil {
		agent := *update.Agent
		stage.Agent = &agent
	}
	if update.Outcomes != nil {
		stage.Outcomes = BeltClampedOutcomes(update.Outcomes)
	}
	if update.Alternates != nil {
		stage.Alternates = cloneEach(update.Alternates, cloneValue[SessionSettings])
	}
	b.Stages[index] = stage
}

// PickGoto picks a goto target for an outcome the way the editor does: the
// chosen stage plus its counterparts on every re-entered fork route.
func (b *Belt) PickGoto(target string, outcomeID string, stageID string) {
	si := b.stageIndex(stageID)
	if si < 0 {
		return
	}
	oi := draftOutcomeIndex(b.Stages[si], outcomeID)
	if oi < 0 {
		return
	}
	to := []string{}
	if target != "" {
		to = append([]string{target}, b.Layout().Counterparts(target, stageID)...)
	}
	b.Stages[si].Outcomes[oi].Then.To = to
}

// SetGotoRoute sets one goto target on a specific route of a re-entered fork.
func (b *Belt) SetGotoRoute(target string, route string, outcomeID string, stageID string) {
	si := b.stageIndex(stageID)
	if si < 0 {
		return
	}
	oi := draftOutcomeIndex(b.Stages[si], outcomeID)
	if oi < 0 {
		return
	}
	then := &b.Stages[si].Outcomes[oi].Then
	if len(then.To) == 0 {
		return
	}
	layout := b.Layout()
	targets := []string{then.To[0]}
	for _, id := range then.To[1:] {
		if !draftAncestryHasRoute(layout.Ancestry(id), route) {
			targets = append(targets, id)
		}
	}
	if target != "" {
		targets = append(targets, target)
	}
	then.To = targets
}

// SwitchTriggerKind switches a trigger entry's kind, restoring what the kind was
// set to in this editing session or using fresh defaults, and keeping only the
// first repository for a single-repository kind.
// stash is the session's stash, changed in place; it must not be nil.
func (b *Belt) SwitchTriggerKind(entryID string, kind TriggerKind, stash *TriggerStash) {
	index := draftTriggerIndex(*b, entryID)
	if index < 0 {
		return
	}
	current := b.Triggers[index].Trigger
	stash.Entry = entryID
	stash.Put(current.Kind, current.Clone())
	if restored := stash.Trigger(kind); restored != nil {
		b.Triggers[index].Trigger = restored.Clone()
	} else {
		b.Triggers[index].Trigger = kind.Fresh()
	}
	if kind.OneRepo() && len(b.Triggers[index].Repos) > 1 {
		b.Triggers[index].Repos = []string{b.Triggers[index].Repos[0]}
	}
}

// AddRepository adds a repository the way the editor does. Malformed or duplicate
// input is a no-op (false); a single-repository kind keeps only the new one.
func (b *Belt) AddRepository(raw string, entryID string) bool {
	index := draftTriggerIndex(*b, entryID)
	if index < 0 {
		return false
	}
	repo := RepositoryInputNormalize(raw)
	if repo == nil || slices.Contains(b.Triggers[index].Repos, *repo) {
		return false
	}
	if b.Triggers[index].Trigger.Kind.OneRepo() {
		b.Triggers[index].Repos = []string{*repo}
	} else {
		b.Triggers[index].Repos = append(cloneStrings(b.Triggers[index].Repos), *repo)
	}
	return true
}

// RemoveRepository removes a repository from an entry. Absent input is a no-op
// (false).
func (b *Belt) RemoveRepository(repo string, entryID string) bool {
	index := draftTriggerIndex(*b, entryID)
	if index < 0 {
		return false
	}
	normalized := repo
	if n := RepositoryInputNormalize(repo); n != nil {
		normalized = *n
	}
	before := b.Triggers[index].Repos
	kept := make([]string, 0, len(before))
	for _, r := range before {
		if r != normalized && r != repo {
			kept = append(kept, r)
		}
	}
	b.Triggers[index].Repos = kept
	b.pruneRepositoryBases()
	return len(kept) != len(before)
}
