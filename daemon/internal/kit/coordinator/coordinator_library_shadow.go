// The library's shadow tests: a belt's shadow version is saved apart from its
// live versions, and the test ends when you accept or discard it.

package coordinator

import (
	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ShadowTestRunning reports whether any belt has a shadow test that has not ended, whether the
// belt is live, paused or archived.
// Lock: runs with c.mu held.
func (l *Library) ShadowTestRunning() bool {
	for _, record := range l.belts {
		if record.ShadowTest() != nil {
			return true
		}
	}
	return false
}

// SaveShadow saves draft as the belt's next shadow version, starting a test when
// none runs. The shadow keeps live's triggers, base and repository bases.
// Lock: runs with c.mu held.
func (l *Library) SaveShadow(draft core.Belt) (core.Belt, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index := l.beltIndex(draft.ID)
	if index < 0 {
		return core.Belt{}, core.NewValidationFailure("No belt with id "+draft.ID+".", nil)
	}
	record := cloneBeltRecord(l.belts[index])
	if record.ShadowTest() == nil {
		record.ShadowTests = append(record.ShadowTests, core.ShadowTest{
			ID:        core.ShadowNextTestID(record.ShadowTests),
			Live:      record.Belt.Version,
			StartedAt: jsonx.Date{Time: l.c.clock.Now()},
		})
	}
	definition := core.ShadowDefinition(core.StageKeysExplicit(draft, l.pinnedAgents(draft)), record.Belt)
	saved, updated, err := l.storage.SaveShadowVersion(definition, record)
	if err != nil {
		return core.Belt{}, err
	}
	l.belts[index] = updated
	l.revision++
	return saved.Clone(), nil
}

// AcceptShadow makes the running test's shadow the live version and ends the
// test. Warnings pause the belt, as a save does, before the version is live.
// Lock: runs with c.mu held.
func (l *Library) AcceptShadow(id string) (core.Belt, error) {
	l.reconcileFiles()
	defer l.wroteFiles()
	index, record, err := l.shadowRecord(id)
	if err != nil {
		return core.Belt{}, err
	}
	test := record.ShadowTest()
	if len(l.Warnings(*record.Shadow)) > 0 && !record.Paused {
		record.Paused = true
		if err := l.storage.SaveState(record); err != nil {
			return core.Belt{}, err
		}
		l.belts[index].Paused = true
		l.revision++
	}
	promoted, err := l.storage.PromoteShadow(id, test.Shadow)
	if err != nil {
		return core.Belt{}, err
	}
	record.Belt = promoted
	record.ShadowTests = core.ShadowEnd(record.ShadowTests, core.ShadowResultAccepted, jsonx.Date{Time: l.c.clock.Now()})
	record.Shadow = nil
	if err := l.storage.SaveState(record); err != nil {
		return core.Belt{}, err
	}
	l.belts[index] = record
	l.revision++
	return promoted.Clone(), nil
}

// DiscardShadow ends the running test without changing the live version. The
// shadow's versions stay on disk for the runs that used them.
// Lock: runs with c.mu held.
func (l *Library) DiscardShadow(id string) error {
	l.reconcileFiles()
	defer l.wroteFiles()
	index, record, err := l.shadowRecord(id)
	if err != nil {
		return err
	}
	record.ShadowTests = core.ShadowEnd(record.ShadowTests, core.ShadowResultDiscarded, jsonx.Date{Time: l.c.clock.Now()})
	record.Shadow = nil
	if err := l.storage.SaveState(record); err != nil {
		return err
	}
	l.belts[index] = record
	l.revision++
	return nil
}

// shadowRecord is the belt with a running test and its shadow, or the failure that says why not.
// Lock: runs with c.mu held.
func (l *Library) shadowRecord(id string) (int, store.BeltRecord, error) {
	index := l.beltIndex(id)
	if index < 0 {
		return -1, store.BeltRecord{}, core.NewValidationFailure("No belt with id "+id+".", nil)
	}
	record := cloneBeltRecord(l.belts[index])
	if record.ShadowTest() == nil || record.Shadow == nil {
		return -1, store.BeltRecord{}, core.NewValidationFailure(core.BeltAvailabilityNoShadow, nil)
	}
	return index, record, nil
}

// startDefinition is the belt version a new run of record starts on, with the
// run's shadow mark: the version the test's arm picks while a test runs and its
// shadow has no warnings, otherwise the live version without a mark.
// Lock: runs with c.mu held.
func (l *Library) startDefinition(record store.BeltRecord, runs []core.Run) (core.Belt, *core.RunShadow) {
	test := record.ShadowTest()
	if test == nil || record.Shadow == nil {
		return record.Belt, nil
	}
	shadow := core.ShadowDefinition(*record.Shadow, record.Belt)
	if len(l.Warnings(shadow)) > 0 {
		return record.Belt, nil
	}
	arm := core.ShadowPickArm(test.ID, runs, &l.c.sessions.random)
	mark := &core.RunShadow{Test: test.ID, Arm: arm}
	if arm == core.ShadowArmShadow {
		return shadow, mark
	}
	return record.Belt, mark
}
