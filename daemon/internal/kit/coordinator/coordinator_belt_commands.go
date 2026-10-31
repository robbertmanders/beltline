// The belt commands the local API and the belt editor share: a saved belt's
// resource context and refusals, saving, archiving, pausing and the AFK mode,
// and the revision token.
//
// The commands the local API and the belt editor share. Every command checks
// availability again here, so a client can never bypass the table the resource
// advertised. Availability and validation problems are typed; the server maps
// them to 422 without writing anything.
// Lock: every exported method below is an exported entry point; it takes c.mu.
// Methods named ...Now are the same work with c.mu held, for the other parts.

package coordinator

import (
	"fmt"
	"strings"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// BeltResourceContext is what the server needs to describe one belt resource:
// the ordered actions the daemon would accept, the saved warnings and the
// opaque revision token.
type BeltResourceContext struct {
	Actions  []core.BeltAction
	Warnings []string
	Revision string
}

// BeltResourceContext returns the actions, warnings and revision of a saved belt
// record; dirty is the editor's own unsaved state and is never persisted.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) BeltResourceContext(record store.BeltRecord, dirty bool) BeltResourceContext {
	c.Lock()
	defer c.EndTurn()
	return c.beltResourceContextNow(record, dirty)
}

// beltResourceContextNow is BeltResourceContext with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) beltResourceContextNow(record store.BeltRecord, dirty bool) BeltResourceContext {
	warnings := c.library.Warnings(record.Belt)
	actions := core.BeltAvailabilityActions(c.actionContextWithWarnings(record, dirty, warnings))
	return BeltResourceContext{Actions: actions, Warnings: warnings, Revision: c.revisionNow(record, actions, warnings)}
}

// ActionContext is the facts the belt actions are computed from.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) ActionContext(record store.BeltRecord, dirty bool) core.BeltActionContext {
	c.Lock()
	defer c.EndTurn()
	return c.actionContextNow(record, dirty)
}

// actionContextNow is ActionContext with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) actionContextNow(record store.BeltRecord, dirty bool) core.BeltActionContext {
	return c.actionContextWithWarnings(record, dirty, c.library.Warnings(record.Belt))
}

// actionContextWithWarnings is actionContextNow for a caller that already has the belt's warnings,
// so a read that needs both does not validate the belt twice.
// Lock: runs with c.mu held.
func (c *Coordinator) actionContextWithWarnings(record store.BeltRecord, dirty bool, warnings []string) core.BeltActionContext {
	return core.BeltActionContext{
		Saved:              record.Belt.Version > 0,
		Archived:           record.Archived,
		Paused:             record.Paused,
		Warnings:           warnings,
		OutdatedStages:     c.outdatedStageCount(record.Belt),
		HasManualTrigger:   record.Belt.HasManualTrigger(),
		Dirty:              dirty,
		SharedRepository:   c.hasSharedLibraryRepository(),
		Shadow:             record.ShadowTest() != nil,
		ShadowTestsEnabled: c.settings.EnableShadowTests,
	}
}

// BeltCommandRefusal returns nil when the command may run; otherwise the reason
// the resource carried, ready for a 422.
// Lock: exported entry point; takes c.mu.
func (c *Coordinator) BeltCommandRefusal(command core.BeltCommand, id string, dirty bool) *string {
	c.Lock()
	defer c.EndTurn()
	return c.beltCommandRefusalNow(command, id, dirty)
}

// beltCommandRefusalNow is BeltCommandRefusal with c.mu held.
// Lock: runs with c.mu held.
func (c *Coordinator) beltCommandRefusalNow(command core.BeltCommand, id string, dirty bool) *string {
	record := c.library.Belt(id)
	if record == nil {
		return nil
	}
	return core.BeltAvailabilityRefusal(command, c.actionContextNow(*record, dirty))
}

// SaveBeltCommand saves a belt as its next stored version. Warnings stay
// saveable: they pause the belt before the version is written. Only what cannot
// be written is refused, with the field each problem is about.
// Lock: exported entry point; takes c.mu. Spawns: kick, through saveBelt.
func (c *Coordinator) SaveBeltCommand(belt core.Belt) (store.BeltRecord, error) {
	c.Lock()
	defer c.EndTurn()
	// Only what cannot be written is refused, with the field each problem is about; nothing is written. Every
	// other structural problem is a warning: the belt saves and is paused, as the old app did.
	if problems := core.BeltStructureRefusals(belt); len(problems) > 0 {
		return store.BeltRecord{}, core.ValidationFailureLocated(problems)
	}
	if _, err := c.saveBelt(belt); err != nil {
		return store.BeltRecord{}, err
	}
	record := c.library.Belt(belt.ID)
	if record == nil {
		return store.BeltRecord{}, core.NewValidationFailure("The saved belt "+belt.ID+" could not be read back.", nil)
	}
	return *record, nil
}

// ArchiveBeltCommand archives or restores a saved belt; a failed write leaves the record as it was.
// Lock: exported entry point; takes c.mu. Spawns: kick, when restoring.
func (c *Coordinator) ArchiveBeltCommand(id string, archived bool) (store.BeltRecord, error) {
	c.Lock()
	defer c.EndTurn()
	command := core.BeltCommandUnarchive
	if archived {
		command = core.BeltCommandArchive
	}
	if refusal := c.beltCommandRefusalNow(command, id, false); refusal != nil {
		return store.BeltRecord{}, core.NewValidationFailure(*refusal, nil)
	}
	record, err := c.library.ArchivingBelt(id, archived)
	if err != nil {
		return store.BeltRecord{}, err
	}
	if !archived {
		c.kick()
	}
	return record, nil
}

// PauseBeltCommand pauses or resumes a saved belt. Resume keeps the rule that a
// warned belt cannot resume.
// Lock: exported entry point; takes c.mu. Spawns: kick, on resume.
func (c *Coordinator) PauseBeltCommand(id string, paused bool) (store.BeltRecord, error) {
	c.Lock()
	defer c.EndTurn()
	command := core.BeltCommandResume
	if paused {
		command = core.BeltCommandPause
	}
	if refusal := c.beltCommandRefusalNow(command, id, false); refusal != nil {
		return store.BeltRecord{}, core.NewValidationFailure(*refusal, nil)
	}
	if err := c.setPausedNow(id, paused); err != nil {
		return store.BeltRecord{}, err
	}
	record := c.library.Belt(id)
	if record == nil {
		return store.BeltRecord{}, core.NewValidationFailure("No belt with id "+id+".", nil)
	}
	return *record, nil
}

// AfkBeltCommand saves a belt's AFK start mode through the normal save path.
// Existing runs keep their own mode.
// Lock: exported entry point; takes c.mu. Spawns: kick, through saveBelt.
func (c *Coordinator) AfkBeltCommand(id string, afk core.AfkMode) (store.BeltRecord, error) {
	c.Lock()
	defer c.EndTurn()
	if refusal := c.beltCommandRefusalNow(core.BeltCommandAfk, id, false); refusal != nil {
		return store.BeltRecord{}, core.NewValidationFailure(*refusal, nil)
	}
	if record := c.library.Belt(id); record != nil && record.Belt.Afk != afk {
		belt := record.Belt.Clone()
		belt.Afk = afk
		if _, err := c.saveBelt(belt); err != nil {
			return store.BeltRecord{}, err
		}
	}
	record := c.library.Belt(id)
	if record == nil {
		return store.BeltRecord{}, core.NewValidationFailure("No belt with id "+id+".", nil)
	}
	return *record, nil
}

// revisionNow is the resource token of a belt: it changes with the returned belt,
// its actions and the library and policy inputs that decide them.
// Lock: runs with c.mu held.
func (c *Coordinator) revisionNow(record store.BeltRecord, actions []core.BeltAction, warnings []string) string {
	beltJSON, err := jsonx.Marshal(record.Belt)
	if err != nil {
		beltJSON = []byte(record.Belt.ID)
	}
	actionsJSON, err := jsonx.Marshal(actions)
	if err != nil {
		actionsJSON = nil
	}
	paused := "running"
	if record.Paused {
		paused = "paused"
	}
	live := "live"
	if record.Archived {
		live = "archived"
	}
	parts := []string{
		record.Belt.ID,
		string(beltJSON),
		string(actionsJSON),
		strings.Join(warnings, "\u001e"),
		paused,
		live,
	}
	if test := record.ShadowTest(); test != nil {
		parts = append(parts, fmt.Sprintf("shadow:%s@%d", test.ID, test.Shadow))
	}
	for _, stage := range record.Belt.Stages {
		if stage.Kind() != core.StageKindAgent || stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		latestVersion := 0
		archivedNote := ""
		if latest := c.library.LatestAgent(ref.ID); latest != nil {
			latestVersion = latest.Agent.Version
			if latest.Archived {
				archivedNote = ":archived"
			}
		}
		parts = append(parts, fmt.Sprintf("%s@%d->%d%s", ref.ID, ref.Version, latestVersion, archivedNote))
	}
	return core.ResourceRevisionToken(parts)
}

// outdatedStageCount counts the stages whose agent has a newer library version.
// Lock: runs with c.mu held.
func (c *Coordinator) outdatedStageCount(belt core.Belt) int {
	count := 0
	for _, stage := range belt.Stages {
		if c.library.NewerAgent(stage) != nil {
			count++
		}
	}
	return count
}

// setPausedNow pauses or resumes a belt, and kicks on resume.
// Lock: runs with c.mu held. Spawns: kick, on resume.
func (c *Coordinator) setPausedNow(beltID string, paused bool) error {
	if err := c.library.SetPaused(beltID, paused); err != nil {
		return err
	}
	if !paused {
		c.kick()
	}
	return nil
}
