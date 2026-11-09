package coordinator

import (
	"maps"
	"os"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
)

// LibraryResumeBlocked is shown when Resume is refused because the latest saved
// belt still has warnings.
const LibraryResumeBlocked = core.BeltAvailabilityResumeBlocked

// Library holds the belts and the agent library. Saving either makes a new
// version; runs keep the belt version they started on, and belts keep the agent
// version they were saved with until you move a stage to the new one. It is a
// part of Coordinator: its methods run with c.mu held.
type Library struct {
	// c is the coordinator the library belongs to (the back pointer).
	c       *Coordinator
	storage store.Storage
	// belts are the saved belts with their records.
	belts []store.BeltRecord
	// agents is the agent library, each agent at its latest version.
	agents []core.AgentRecord
	// revision counts changes to belts and agents (bump it with every write);
	// the change projection re-encodes the library only when it moved.
	revision  uint64
	projected projectionMemo[uint64]
	// modelAdmission reports the allowed models now. The coordinator sets it.
	modelAdmission func() core.ModelAdmission
	// modelLabel returns a model's catalog name, for warnings. The coordinator sets it.
	modelLabel func(model string, runner core.Backend) *string
	// agentCache holds the agent versions read so far. A saved version never
	// changes; it is emptied on a reload.
	agentCache map[core.AgentRef]core.Agent
	// snapshotCache holds runs' own agent copies, read once, by run and version.
	snapshotCache map[string]core.Agent
	// files is the library's files as the library last read or wrote them, by path. A
	// difference from the folder now is a change someone else made on disk, which
	// reconcileFiles reloads. It does not depend on the configuration assistant running.
	files map[string]time.Time
	// watch reads the library folders' stamps, keeping each folder's names while it is unchanged.
	watch *store.LibraryWatch
}

// newLibrary builds the library over storage and loads it. Nothing is made for
// an empty library: you import or start a factory.
// Lock: runs during New, before the coordinator is shared.
func newLibrary(c *Coordinator, storage store.Storage) Library {
	l := Library{
		c: c, storage: storage,
		belts: []store.BeltRecord{}, agents: []core.AgentRecord{},
		agentCache: map[core.AgentRef]core.Agent{}, snapshotCache: map[string]core.Agent{},
		watch: store.NewLibraryWatch(),
	}
	l.modelAdmission = func() core.ModelAdmission { return storage.LoadSettings().ModelAdmission(os.Getenv) }
	l.modelLabel = func(string, core.Backend) *string { return nil }
	l.load(true)
	return l
}

// reload reads the belts and agents again after they changed on disk. Runs keep
// theirs. A merge import passes migrating false, so the watch upgrade does not
// rewrite files a merge must leave alone.
// Lock: runs with c.mu held.
func (l *Library) reload(migrating bool) {
	l.agentCache = map[core.AgentRef]core.Agent{}
	l.load(migrating)
}

// reconcileFiles reloads the library when its files changed on disk since it last
// read or wrote them, so a configuration assistant's edit, or anyone's, shows in the
// next read without waiting for a tick. A reload clears the intake's poll times, so
// triggers the change touched are checked again. It reports whether it reloaded, and
// an unchanged folder changes nothing, so it publishes nothing.
// Lock: runs with c.mu held.
func (l *Library) reconcileFiles() bool {
	if sameLibraryStamps(l.watch.Stamps(l.storage.Paths), l.files) {
		return false
	}
	l.reload(true)
	l.c.intake.lastPoll = map[string]time.Time{}
	return true
}

// wroteFiles records the files as they are after the library's own write, so the write
// is not read back as a change from outside. Writes call reconcileFiles first, so a
// change from outside is loaded before this baseline moves past it.
// Lock: runs with c.mu held.
func (l *Library) wroteFiles() {
	l.files = l.watch.Stamps(l.storage.Paths)
}

// sameLibraryStamps compares two library file snapshots, removals included.
// Lock: none (pure).
func sameLibraryStamps(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for path, stamp := range a {
		other, ok := b[path]
		if !ok || !stamp.Equal(other) {
			return false
		}
	}
	return true
}

// Signature is each belt's and agent's id, latest version and flag. Every save,
// pause and archive changes it, so it tells whether the library still matches
// the factory version it came from.
// Lock: runs with c.mu held.
func (l *Library) Signature() []string {
	out := make([]string, 0, len(l.belts)+len(l.agents))
	for _, record := range l.belts {
		text := "belt " + record.Belt.ID + " v" + strconv.Itoa(record.Belt.Version)
		if record.Paused {
			text += " paused"
		}
		if record.Archived {
			text += " archived"
		}
		out = append(out, text)
	}
	for _, record := range l.agents {
		text := "agent " + record.Agent.ID + " v" + strconv.Itoa(record.Agent.Version)
		if record.Archived {
			text += " archived"
		}
		out = append(out, text)
	}
	sort.Strings(out)
	return out
}

// load reads the library from disk.
// Lock: runs with c.mu held.
func (l *Library) load(migrating bool) {
	// The baseline is taken before anything is read, so a change made while loading is found again.
	l.files = l.watch.Stamps(l.storage.Paths)
	l.agents = l.storage.LoadAgents()
	l.revision++
	l.belts = l.storage.LoadBelts()
	l.revision++
	if !migrating {
		return
	}
	for index := range l.belts {
		record := &l.belts[index]
		l.revision++ // writes go through this pointer
		if record.WatchOutcomes && record.ReviewOutcomes {
			continue
		}
		// Give a belt saved before watches saw more than a merge and a
		// closure, or before they saw reviews, the outcomes for the events
		// it is missing, once, so that taking one of them away again sticks.
		// None and new changes are opt-in. An upgrade that walked every case would add them.
		events := core.DefaultsMigratedWatchEvents()
		if record.WatchOutcomes {
			for event := range events {
				if _, review := core.WatchEventReviewEvents[event]; !review {
					delete(events, event)
				}
			}
		}
		if record.ReviewOutcomes {
			for event := range core.WatchEventReviewEvents {
				delete(events, event)
			}
		}
		upgraded := core.DefaultsUpgradingWatchOutcomes(record.Belt, events)
		// Belts compare with ==; an unchanged upgrade is a deep copy of the same value.
		if !reflect.DeepEqual(upgraded, record.Belt) {
			// Tried again on the next start when the new version can't be saved.
			saved, err := l.storage.SaveNewVersion(upgraded)
			if err != nil {
				continue
			}
			record.Belt = saved
			l.revision++
		}
		record.WatchOutcomes = true
		l.revision++
		record.ReviewOutcomes = true
		l.revision++
		_ = l.storage.SaveState(*record)
		// The migration's own writes are not changes from outside.
		l.wroteFiles()
	}
}

// Belt returns the belt record with id, or nil.
// Lock: runs with c.mu held.
func (l *Library) Belt(id string) *store.BeltRecord {
	index := l.beltIndex(id)
	if index < 0 {
		return nil
	}
	record := cloneBeltRecord(l.belts[index])
	return &record
}

// SetBeltArchived archives or restores a belt.
// Lock: runs with c.mu held.
func (l *Library) SetBeltArchived(id string, archived bool) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(id)
	if index < 0 {
		return
	}
	if err := l.storage.SetArchived(id, archived); err != nil {
		return
	}
	l.belts[index].Archived = archived
	l.revision++
}

// SetPaused pauses or resumes a belt. Resume is refused while the latest saved
// belt has warnings.
// Lock: runs with c.mu held.
func (l *Library) SetPaused(beltID string, paused bool) error {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(beltID)
	if index < 0 {
		return nil
	}
	if !paused && len(l.Warnings(l.belts[index].Belt)) > 0 {
		return core.NewFailure(LibraryResumeBlocked)
	}
	if err := l.storage.SetPaused(beltID, paused); err != nil {
		return err
	}
	l.belts[index].Paused = paused
	l.revision++
	return nil
}

// ArchivingBelt archives or restores a belt, propagating a storage failure and
// leaving the in-memory record alone when the write fails.
// Lock: runs with c.mu held.
func (l *Library) ArchivingBelt(id string, archived bool) (store.BeltRecord, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(id)
	if index < 0 {
		return store.BeltRecord{}, core.NewValidationFailure("No belt with id "+id+".", nil)
	}
	if err := l.storage.SetArchived(id, archived); err != nil {
		return store.BeltRecord{}, err
	}
	l.belts[index].Archived = archived
	l.revision++
	return cloneBeltRecord(l.belts[index]), nil
}

// SaveBelt saves a belt as a new version. Warnings pause the belt, and that pause
// is written before the version file.
// Lock: runs with c.mu held.
func (l *Library) SaveBelt(belt core.Belt) (core.Belt, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(belt.ID)
	var record store.BeltRecord
	if index >= 0 {
		record = cloneBeltRecord(l.belts[index])
	} else {
		record = store.NewBeltRecord(store.BeltRecordArgs{Belt: belt, Created: l.c.clock.Now(), WatchOutcomes: true, ReviewOutcomes: true})
	}
	definition := core.StageKeysExplicit(belt, l.pinnedAgents(belt))
	if index >= 0 {
		before := l.belts[index].Belt
		for _, entry := range definition.Triggers {
			previous := core.Trigger{Kind: core.TriggerKindManual}
			for _, existing := range before.Triggers {
				if existing.ID == entry.ID {
					previous = existing.Trigger
					break
				}
			}
			if !librarySchedulesEqual(librarySchedule(previous), librarySchedule(entry.Trigger)) {
				delete(record.Fired, entry.ID)
			}
		}
	}
	record.Paused = record.Paused || len(l.Warnings(definition)) > 0
	saved, err := l.storage.SaveVersion(definition, record)
	if err != nil {
		if index >= 0 {
			if state, ok := l.storage.StoredState(definition.ID); ok {
				l.belts[index].Paused = state.Paused
				l.revision++
				l.belts[index].Archived = state.Archived
				l.revision++
				l.belts[index].Fired = copyFiredTimes(state.Schedules)
				l.revision++
				l.belts[index].WatchOutcomes = state.WatchOutcomes
				l.revision++
				l.belts[index].ReviewOutcomes = state.ReviewOutcomes
				l.revision++
			}
		}
		return core.Belt{}, err
	}
	record.Belt = saved
	l.revision++
	if index >= 0 {
		l.belts[index] = record
		l.revision++
	} else {
		l.belts = append(l.belts, record)
		l.revision++
	}
	return saved.Clone(), nil
}

// librarySchedule is the trigger's schedule, when it has one.
// Lock: none (pure).
func librarySchedule(trigger core.Trigger) *core.Schedule {
	if trigger.Kind != core.TriggerKindSchedule || trigger.Schedule == nil {
		return nil
	}
	schedule := trigger.Schedule.Schedule
	return &schedule
}

// librarySchedulesEqual compares two optional schedules, nil included.
// Lock: none (pure).
func librarySchedulesEqual(a, b *core.Schedule) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(*a, *b)
}

// setFired saves when one schedule entry last fired. The cursor stays as it was
// when the save fails.
// Lock: runs with c.mu held.
func (l *Library) setFired(id, entry string, date time.Time) error {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(id)
	if index < 0 {
		return nil
	}
	record := &l.belts[index]
	l.revision++ // writes go through this pointer
	if record.Fired == nil {
		record.Fired = map[string]time.Time{}
	}
	previous, had := record.Fired[entry]
	record.Fired[entry] = date
	if err := l.storage.SaveState(*record); err != nil {
		if had {
			record.Fired[entry] = previous
		} else {
			delete(record.Fired, entry)
		}
		return err
	}
	return nil
}

// NewBelt returns a new belt that is not saved yet: a trigger and no stages.
// Lock: runs with c.mu held.
func (l *Library) NewBelt() core.Belt {
	version := 0
	return core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
		ID: l.unusedBeltID(nil), Name: "New belt", Version: &version,
		Trigger: core.TriggerKindIssues.Fresh(), Repos: []string{}, Stages: []core.Stage{},
	})
}

// CopyBelt returns an unsaved definition preserving the draft and its pinned agents.
// Lock: runs with c.mu held.
func (l *Library) CopyBelt(belt core.Belt) core.Belt {
	copied := core.StageKeysExplicit(belt, l.pinnedAgents(belt))
	copied.ID = l.unusedBeltID(&belt.ID)
	copied.Version = 0
	copied.Name = "Copy of " + belt.Name
	// A copy is yours, not the repository's it was installed from.
	copied.Origin, copied.OriginID = nil, ""
	return copied
}

// unusedBeltID returns a belt id no saved or draft belt uses, other than excluding.
// Lock: runs with c.mu held.
func (l *Library) unusedBeltID(excluding *string) string {
	n := len(l.belts) + 1
	for {
		candidate := "belt-" + strconv.Itoa(n)
		if excluding == nil || *excluding != candidate {
			if l.beltIndex(candidate) < 0 {
				return candidate
			}
		}
		n++
	}
}

// savedBeltID is the id a belt's first save gives it.
// Lock: runs with c.mu held.
func (l *Library) savedBeltID(belt core.Belt) string {
	return core.LibrarySavedID(belt.ID, belt.Name, belt.Version, l.c.clock.Now().Format(time.RFC3339Nano),
		func(candidate string) bool { return l.beltIndex(candidate) >= 0 })
}

// beltIndex is the index of the belt record with id, or -1.
func (l *Library) beltIndex(id string) int {
	for i := range l.belts {
		if l.belts[i].Belt.ID == id {
			return i
		}
	}
	return -1
}

// nonEmptyStageKey is a stage key as a pointer, or nil when it is empty.
func nonEmptyStageKey(key string) *string {
	if key == "" {
		return nil
	}
	return &key
}

// copyFiredTimes copies a schedule cursor map, nil staying nil.
func copyFiredTimes(in map[string]time.Time) map[string]time.Time {
	if in == nil {
		return nil
	}
	out := make(map[string]time.Time, len(in))
	maps.Copy(out, in)
	return out
}
