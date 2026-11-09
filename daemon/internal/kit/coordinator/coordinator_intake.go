package coordinator

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// Intake is where runs come from: the triggers that check GitHub and Jira, the
// schedules, and the tasks you start yourself. A part of Coordinator; its
// methods run with c.mu held, and the ones that await suspend through c.Suspend.
type Intake struct {
	// c is the coordinator the intake belongs to (the back pointer).
	c       *Coordinator
	book    *RunBook
	library *Library
	clock   Clock
	// admission reports the allowed models now.
	admission func() core.ModelAdmission
	// currentModelWarning returns the model warning for settings, or nil.
	currentModelWarning func(settings core.SessionSettings, name string) *string
	// checks counts the trigger checks running: a tick's and a check now's can overlap.
	// Write it with setChecking.
	checks int
	// lastCheck is when the last trigger check finished. Write it with setLastCheck.
	lastCheck *time.Time
	// globallyPaused: while the app's one global pause is on, nothing joins a belt:
	// no issue, review request or ticket, and no scheduled task.
	globallyPaused bool
	// lastPoll is the last check time per "<belt>|<entry>".
	lastPoll map[string]time.Time
	// refused holds each belt's last refusal to start a run, noted once.
	refused map[string]string
	// waitingBy is what each belt's last check of each place left waiting, by "<belt>|<scope>".
	// Write it with setWaiting: its revision and the intake projection's memo follow it.
	waitingBy map[string][]core.WaitingSubject
	// waitingRevision counts writes to waitingBy; waitingMemo is the intake projection
	// at that revision.
	waitingRevision uint64
	waitingMemo     projectionMemo[uint64]
	// sources are the triggers that check somewhere every so often, by kind.
	sources map[core.TriggerKind]Source
}

// setChecking records that a trigger check began or ended. The state projection reads it, so the write
// bumps the coordinator's state revision. Lock: runs with c.mu held.
func (i *Intake) setChecking(checking bool) {
	if checking {
		i.checks++
	} else if i.checks > 0 {
		i.checks--
	}
	i.c.stateRevision++
}

// isChecking reports whether any trigger check runs. Lock: runs with c.mu held.
func (i *Intake) isChecking() bool { return i.checks > 0 }

// setLastCheck records when the last trigger check finished (see setChecking). Lock: runs with c.mu held.
func (i *Intake) setLastCheck(at time.Time) {
	i.lastCheck = &at
	i.c.stateRevision++
}

// setWaiting replaces what a check of one place left waiting; an empty list clears it. Lock: runs with c.mu held.
func (i *Intake) setWaiting(key string, waiting []core.WaitingSubject) {
	if len(waiting) == 0 {
		delete(i.waitingBy, key)
	} else {
		i.waitingBy[key] = waiting
	}
	i.waitingRevision++
}

// projection is the intake's change projection: the waiting subjects, encoded at their revision.
// Lock: runs with c.mu held.
func (i *Intake) projection() []byte {
	return i.waitingMemo.of("intake", i.waitingRevision, func() []byte {
		return projectionBytes(intakeWaitingRows(i.waitingBy))
	})
}

// newIntake builds the intake and its trigger sources.
// Lock: runs during New, before the coordinator is shared.
func newIntake(c *Coordinator, book *RunBook, library *Library, github github.GitHub, jira jira.Jira, clock Clock) Intake {
	return Intake{
		c: c, book: book, library: library, clock: clock,
		lastPoll: map[string]time.Time{}, refused: map[string]string{}, waitingBy: map[string][]core.WaitingSubject{},
		sources: map[core.TriggerKind]Source{
			core.TriggerKindIssues:  &IssuesSource{github: github},
			core.TriggerKindReviews: newReviewsSource(github),
			core.TriggerKindJira:    &JiraSource{jira: jira},
		},
	}
}

// pollTriggers checks each trigger entry whose interval is due, or every entry
// with force. One entry's failure leaves the others to admit work.
// Lock: runs with c.mu held. Suspends: each source's beginCheck, list, and every poll.
func (i *Intake) pollTriggers(ctx context.Context, force bool) {
	now := i.clock.Now()
	type job struct {
		record store.BeltRecord
		entry  core.TriggerEntry
		source Source
	}
	var due []job
	for _, record := range i.library.belts {
		for _, entry := range record.Belt.Triggers {
			if !entry.Trigger.Kind.Polls() {
				continue
			}
			source, ok := i.sources[entry.Trigger.Kind]
			if !ok {
				continue
			}
			view := record.Belt.Focusing(entry)
			if len(source.scopes(view)) == 0 {
				continue
			}
			key := pollKey(record.Belt.ID, entry.ID)
			interval := 60
			if poll := entry.Trigger.Poll(); poll != nil {
				interval = *poll
			}
			seconds := float64(max(interval, 15))
			if last, ok := i.lastPoll[key]; !force && ok && now.Sub(last).Seconds() < seconds {
				continue
			}
			due = append(due, job{record: record, entry: entry, source: source})
		}
	}
	if len(due) == 0 {
		return
	}
	i.setChecking(true)
	defer func() {
		i.setChecking(false)
		i.setLastCheck(i.clock.Now())
	}()
	for _, job := range due {
		record := job.record
		if live := i.library.Belt(job.record.Belt.ID); live != nil {
			record = *live
		}
		key := pollKey(record.Belt.ID, job.entry.ID)
		i.lastPoll[key] = now
		var err error
		i.c.Suspend(func() { err = job.source.beginCheck(ctx) })
		if err != nil {
			i.book.note("Could not check "+record.Belt.Name+": "+err.Error(), nil)
			continue
		}
		// Bind legacy runs once, without changing their identity or state.
		for _, run := range i.runsSnapshot() {
			if run.BeltID != record.Belt.ID || !i.owns(run, job.entry) {
				continue
			}
			subject := job.source.adoptingIdentity(run.Subject)
			if !reflect.DeepEqual(subject, run.Subject) {
				i.book.update(run.ID, nil, nil, func(r *core.Run) { r.Subject = subject })
			}
		}
		view := record.Belt.Focusing(job.entry)
		for _, scope := range job.source.scopes(view) {
			i.poll(ctx, record, job.entry, job.source, scope)
		}
	}
}

// pollKey is the key of one trigger entry's last-check time.
// Lock: none (pure).
func pollKey(beltID, entryID string) string { return beltID + "|" + entryID }

// forgetPolls drops remembered check times for a belt, so a save checks its entries again.
// Lock: runs with c.mu held.
func (i *Intake) forgetPolls(beltID string) {
	for key := range i.lastPoll {
		if key == beltID || strings.HasPrefix(key, beltID+"|") {
			delete(i.lastPoll, key)
		}
	}
}

// tell gives a run what its source saw, unless it is busy, being erased, or
// already replaced.
// Lock: runs with c.mu held. Spawns: none.
func (i *Intake) tell(id string, fate Fate, generation, operation int, forced bool) {
	work := workIntake
	if forced {
		work = workIntakeForced
	}
	if i.book.liveIndex(id, work, &generation, nil) < 0 {
		return
	}
	current := i.book.Run(id)
	switch fate.Kind {
	case FateChanged:
		subject := fate.Subject
		// A read that started before Park, or any edit while parked, updates
		// the subject and does not start the belt again.
		if current.Parking != nil || current.Operation != operation {
			if reflect.DeepEqual(current.Subject, subject) {
				return
			}
			i.book.update(id, &generation, nil, func(run *core.Run) {
				run.Subject = subject
				run.UpdatedAt = jsonx.Date{Time: i.clock.Now()}
			})
			return
		}
		g, o := generation, operation
		i.book.apply(id, core.Event{Kind: core.EventKindSubjectChanged, Subject: subject}, &g, &o, nil)
	case FateGone:
		g := generation
		i.book.apply(id, core.Event{Kind: core.EventKindSubjectGone, Reason: fate.Reason}, &g, nil, nil)
	}
}

// reconcile applies the latest source state to a run, even while Resume is
// still reconciling. The run keeps the entry that admitted it.
// Lock: runs with c.mu held. Suspends: source.missing.
func (i *Intake) reconcile(ctx context.Context, id string) error {
	run := i.book.Run(id)
	if run == nil || !run.State.IsActive() {
		return nil
	}
	snapshot := i.provenance(*run)
	if snapshot == nil {
		return nil
	}
	source, ok := i.sources[snapshot.Trigger.Kind]
	if !ok {
		return nil
	}
	var belt core.Belt
	if live := i.library.Belt(run.BeltID); live != nil {
		belt = live.Belt
	} else {
		belt = core.NewBeltFromTrigger(core.BeltFromTriggerArgs{
			ID: run.BeltID, Name: run.BeltName, Trigger: snapshot.Trigger, Repos: snapshot.Repos, Stages: run.Stages,
		})
	}
	view := belt.Focusing(core.TriggerEntry{ID: snapshot.Entry, Trigger: snapshot.Trigger, Repos: snapshot.Repos})
	view.Policies = run.Policies
	view.Base = run.Base
	var fate Fate
	var err error
	i.c.Suspend(func() { fate, err = source.missing(ctx, *run, view) })
	if err != nil {
		return err
	}
	i.tell(id, fate, run.Generation, run.Operation, true)
	return nil
}

// runsSnapshot is a copy of the live runs, for loops that read the book while they suspend.
// Lock: runs with c.mu held.
func (i *Intake) runsSnapshot() []core.Run {
	return append([]core.Run(nil), i.book.runs...)
}

// runBeltsSnapshot is a copy of the saved belt records, for loops that read the library while they suspend.
// Lock: runs with c.mu held.
func (i *Intake) runBeltsSnapshot() []store.BeltRecord {
	return append([]store.BeltRecord(nil), i.library.belts...)
}
