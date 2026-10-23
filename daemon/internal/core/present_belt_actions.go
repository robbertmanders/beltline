package core

import (
	"fmt"
	"strings"
)

// BeltAction is one command a belt resource offers, with whether the daemon would accept
// it now and why not when it would not. Clients must not trust a supplied
// `enabled`: the daemon checks the same table again when a command arrives.
type BeltAction struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	// Reason is nil when enabled; otherwise a sentence a person can read.
	Reason *string `json:"reason,omitzero"`
}

// BeltCommand is a command every belt resource describes. The values are the command
// names the API posts to.
type BeltCommand string

const (
	BeltCommandNew             BeltCommand = "new"
	BeltCommandSave            BeltCommand = "save"
	BeltCommandCopy            BeltCommand = "copy"
	BeltCommandArchive         BeltCommand = "archive"
	BeltCommandUnarchive       BeltCommand = "unarchive"
	BeltCommandPause           BeltCommand = "pause"
	BeltCommandResume          BeltCommand = "resume"
	BeltCommandAfk             BeltCommand = "afk"
	BeltCommandUpdateStage     BeltCommand = "update-stage"
	BeltCommandUpdateAllStages BeltCommand = "update-all-stages"
	BeltCommandTask            BeltCommand = "task"
	BeltCommandShare           BeltCommand = "share"
	BeltCommandShadowCreate    BeltCommand = "shadow-create"
	BeltCommandShadowSave      BeltCommand = "shadow-save"
	BeltCommandShadowAccept    BeltCommand = "shadow-accept"
	BeltCommandShadowDiscard   BeltCommand = "shadow-discard"
)

// AllBeltCommand lists the cases of BeltCommand in declaration order (CaseIterable).
var AllBeltCommand = []BeltCommand{
	BeltCommandNew, BeltCommandSave, BeltCommandCopy, BeltCommandArchive, BeltCommandUnarchive,
	BeltCommandPause, BeltCommandResume, BeltCommandAfk, BeltCommandUpdateStage, BeltCommandUpdateAllStages,
	BeltCommandTask, BeltCommandShare, BeltCommandShadowCreate, BeltCommandShadowSave, BeltCommandShadowAccept,
	BeltCommandShadowDiscard,
}

// BeltActionContext is what the daemon and the editor know about one belt when they decide which
// commands are available. Draft editing context (dirty/new) stays explicit
// and is never stored per client.
type BeltActionContext struct {
	// Saved is whether the belt has a stored version; a new draft is not saved yet.
	Saved    bool
	Archived bool
	// Paused is explicit pause, not merely archived.
	Paused bool
	// Warnings are the saved version's warnings, in order.
	Warnings []string
	// OutdatedStages counts stages whose agent has a newer library version.
	OutdatedStages   int
	HasManualTrigger bool
	// Dirty is whether the editor's copy has unsaved changes.
	Dirty bool
	// SharedRepository is whether a shared library repository is configured to share to.
	SharedRepository bool
	// Shadow is whether a shadow test of the belt is running.
	Shadow bool
}

// Copy used by BeltAvailability. Literal text is exact.
const (
	// BeltAvailabilityResumeBlocked is the editor's message when Resume is refused.
	BeltAvailabilityResumeBlocked = "Resolve this belt’s warnings and save before resuming."
	// BeltAvailabilitySavePausesWarning is the note that saving will pause the belt.
	BeltAvailabilitySavePausesWarning = "Saving will pause this belt until its warnings are resolved."
	// BeltAvailabilityUnsavedCopy is shown when a copy or archive waits for a save.
	BeltAvailabilityUnsavedCopy = "Save this belt before copying it."
	// BeltAvailabilityUnsavedShare is shown when a share waits for a save.
	BeltAvailabilityUnsavedShare = "Save this belt before sharing it."
	// BeltAvailabilityNoSharedRepository is shown when no shared library repository is configured.
	BeltAvailabilityNoSharedRepository = "Add a shared library repository in Settings first."
	// BeltAvailabilityArchivedNote is shown on a command an archived belt refuses.
	BeltAvailabilityArchivedNote = "This belt is archived. Restore it first."
	// BeltAvailabilityUnsavedTask is shown when a task waits for a save.
	BeltAvailabilityUnsavedTask = "Save this belt before starting a task on it."
	// BeltAvailabilityNoManualTrigger is shown when the belt has no manual trigger.
	BeltAvailabilityNoManualTrigger = "This belt has no manual trigger to start a task from."
	// BeltAvailabilityShadowRunning is shown when a shadow test is already running.
	BeltAvailabilityShadowRunning = "This belt already has a shadow. Accept or discard it first."
	// BeltAvailabilityNoShadow is shown on a shadow command while no test runs.
	BeltAvailabilityNoShadow = "This belt has no shadow."
)

// BeltAvailabilityActions is the ordered belt commands for context.
func BeltAvailabilityActions(context BeltActionContext) []BeltAction {
	saved := context.Saved
	archived := context.Archived
	paused := context.Paused
	warnings := context.Warnings
	blockedResume := len(warnings) > 0
	outdated := context.OutdatedStages > 0
	hasManual := context.HasManualTrigger

	var copyReason, archiveReason, unarchiveReason, pauseReason, resumeReason, afkReason, taskReason, updateReason, shareReason *string
	var createShadowReason, shadowReason *string
	if !saved {
		copyReason = new(BeltAvailabilityUnsavedCopy)
	}
	switch {
	case !saved:
		archiveReason = new(BeltAvailabilityUnsavedCopy)
	case archived:
		archiveReason = new("This belt is already archived.")
	}
	switch {
	case !saved:
		unarchiveReason = new(BeltAvailabilityUnsavedCopy)
	case !archived:
		unarchiveReason = new("This belt is not archived.")
	}
	switch {
	case !saved:
		pauseReason = new(BeltAvailabilityUnsavedCopy)
	case archived:
		pauseReason = new(BeltAvailabilityArchivedNote)
	case paused:
		pauseReason = new("This belt is already paused.")
	}
	switch {
	case !saved:
		resumeReason = new(BeltAvailabilityUnsavedCopy)
	case archived:
		resumeReason = new(BeltAvailabilityArchivedNote)
	case !paused:
		resumeReason = new("This belt is not paused.")
	case blockedResume:
		resumeReason = new(BeltAvailabilityResumeBlocked)
	}
	switch {
	case !saved:
		afkReason = new(BeltAvailabilityUnsavedCopy)
	case archived:
		afkReason = new(BeltAvailabilityArchivedNote)
	}
	// Updating stages works on a draft too: the editor offers it before the belt is saved.
	switch {
	case archived:
		updateReason = new(BeltAvailabilityArchivedNote)
	case !outdated:
		updateReason = new("Every stage already uses its agent’s latest version.")
	}
	switch {
	case !saved:
		taskReason = new(BeltAvailabilityUnsavedTask)
	case archived:
		taskReason = new(BeltAvailabilityArchivedNote)
	case !hasManual:
		taskReason = new(BeltAvailabilityNoManualTrigger)
	}
	switch {
	case !saved:
		shareReason = new(BeltAvailabilityUnsavedShare)
	case archived:
		shareReason = new(BeltAvailabilityArchivedNote)
	case !context.SharedRepository:
		shareReason = new(BeltAvailabilityNoSharedRepository)
	}
	switch {
	case !saved:
		createShadowReason = new(BeltAvailabilityUnsavedCopy)
	case archived:
		createShadowReason = new(BeltAvailabilityArchivedNote)
	case context.Shadow:
		createShadowReason = new(BeltAvailabilityShadowRunning)
	}
	if !context.Shadow {
		shadowReason = new(BeltAvailabilityNoShadow)
	}

	return []BeltAction{
		{ID: string(BeltCommandNew), Enabled: true},
		{ID: string(BeltCommandSave), Enabled: true},
		{ID: string(BeltCommandCopy), Enabled: saved, Reason: copyReason},
		{ID: string(BeltCommandArchive), Enabled: saved && !archived, Reason: archiveReason},
		{ID: string(BeltCommandUnarchive), Enabled: saved && archived, Reason: unarchiveReason},
		{ID: string(BeltCommandPause), Enabled: saved && !archived && !paused, Reason: pauseReason},
		{ID: string(BeltCommandResume), Enabled: saved && !archived && paused && !blockedResume, Reason: resumeReason},
		{ID: string(BeltCommandAfk), Enabled: saved && !archived, Reason: afkReason},
		{ID: string(BeltCommandUpdateStage), Enabled: !archived && outdated, Reason: updateReason},
		{ID: string(BeltCommandUpdateAllStages), Enabled: !archived && outdated, Reason: updateReason},
		{ID: string(BeltCommandTask), Enabled: saved && !archived && hasManual, Reason: taskReason},
		{ID: string(BeltCommandShare), Enabled: saved && !archived && context.SharedRepository, Reason: shareReason},
		{ID: string(BeltCommandShadowCreate), Enabled: createShadowReason == nil, Reason: createShadowReason},
		{ID: string(BeltCommandShadowSave), Enabled: context.Shadow, Reason: shadowReason},
		{ID: string(BeltCommandShadowAccept), Enabled: context.Shadow, Reason: shadowReason},
		{ID: string(BeltCommandShadowDiscard), Enabled: context.Shadow, Reason: shadowReason},
	}
}

// BeltAvailabilityAction is the action for one command, or nil when the command is not a belt
// command at all.
func BeltAvailabilityAction(command BeltCommand, context BeltActionContext) *BeltAction {
	for _, action := range BeltAvailabilityActions(context) {
		if action.ID == string(command) {
			return &action
		}
	}
	return nil
}

// BeltAvailabilityRefusal is a command the daemon refuses with 422, with the same reason the
// resource carried. Nil when the command is available.
func BeltAvailabilityRefusal(command BeltCommand, context BeltActionContext) *string {
	action := BeltAvailabilityAction(command, context)
	if action == nil || action.Enabled {
		return nil
	}
	if action.Reason != nil {
		return action.Reason
	}
	return new(string(command) + " is not available for this belt.")
}

// Constants of the resource token hash (FNV-1a, 64-bit).
const (
	resourceRevisionOffset uint64 = 0xcbf29ce484222325
	resourceRevisionPrime  uint64 = 0x00000100000001b3
)

// ResourceRevisionToken is an opaque, deterministic resource token. The same inputs produce
// the same token, so unchanged reads keep their revision; any input the actions or
// the returned belt depend on produces a different token.
func ResourceRevisionToken(parts []string) string {
	hash := resourceRevisionOffset
	joined := strings.Join(parts, "\u001f")
	for i := 0; i < len(joined); i++ {
		hash ^= uint64(joined[i])
		hash *= resourceRevisionPrime
	}
	return fmt.Sprintf("%016x", hash)
}
