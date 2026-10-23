package coordinator

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// Notice is a line the app shows about a run or the coordinator.
// Identifiable). ID comes from the coordinator's IDs.
type Notice struct {
	ID    string     `json:"id"`
	Date  jsonx.Date `json:"date"`
	Text  string     `json:"text"`
	RunID *string    `json:"runId"`
}

// RunBook holds the runs and what happens to them: it applies the engine's
// events, saves every change right away and hands the effects to whoever
// carries them out. It is a part of Coordinator with no lock of its own; its
// methods run with c.mu held.
type RunBook struct {
	// c is the coordinator the book belongs to (the back pointer).
	c *Coordinator
	// runs are the live runs, in load order.
	// Change runs only in this file, and bump revision with every change: the
	// change projection re-encodes the runs only when revision moved.
	runs []core.Run
	// revision counts changes to runs; projected is runs' projection at that
	// revision.
	revision  uint64
	projected projectionMemo[uint64]
	// changedAt is the revision at which each run last changed (absent: since
	// load); runMemos caches each run's digest at that revision, so a turn
	// re-encodes only the runs it changed.
	changedAt map[string]uint64
	runMemos  map[string]runMemo
	// notices are the coordinator's notices.
	notices []Notice
	storage store.Storage
	clock   Clock
	// busy holds the runs with an operation in flight, so two never overlap.
	busy map[string]struct{}
	// restarting holds the runs whose overrule is between saving intent and
	// queuing the replacement. Late work from before the restart cannot apply.
	restarting map[string]struct{}
	// erasing holds the runs whose erasure is recorded and not yet finished.
	// A replacement cannot join while the id is here.
	erasing map[string]struct{}
	// carryOut carries out an event's effects on a run, with the session the run
	// was in before the event. The coordinator sets it.
	carryOut func(effects []core.Effect, run core.Run, session *string)
}

// newRunBook builds the run book over storage and clock.
// Lock: runs during New, before the coordinator is shared.
func newRunBook(c *Coordinator, storage store.Storage, clock Clock) RunBook {
	runs := storage.LoadRuns()
	sortRunsByCreation(runs)
	erasing := map[string]struct{}{}
	for _, pending := range storage.LoadPendingErasures() {
		erasing[pending.ID] = struct{}{}
	}
	return RunBook{
		c: c, runs: runs, storage: storage, clock: clock,
		busy: map[string]struct{}{}, restarting: map[string]struct{}{}, erasing: erasing,
	}
}

// isErasing reports whether the run's erasure has been recorded.
// Lock: runs with c.mu held.
func (b *RunBook) isErasing(id string) bool {
	_, ok := b.erasing[id]
	return ok
}

// accepts reports whether generation is still the live run and nothing bars an
// event from it. operation, when given, must still be the one the work started
// with.
// Lock: runs with c.mu held.
func (b *RunBook) accepts(id string, generation int, operation *int) bool {
	return b.liveIndex(id, workEvent, &generation, operation) >= 0
}

// markErasing records that the run's erasure has started.
// Lock: runs with c.mu held.
func (b *RunBook) markErasing(id string) { b.erasing[id] = struct{}{} }

// clearErasing forgets that the run's erasure started.
// Lock: runs with c.mu held.
func (b *RunBook) clearErasing(id string) { delete(b.erasing, id) }

// removeErased drops the run and the notices that belong to it, once erasure is durable.
// Lock: runs with c.mu held.
func (b *RunBook) removeErased(id string) {
	runs := b.runs[:0]
	for _, run := range b.runs {
		if run.ID != id {
			runs = append(runs, run)
		}
	}
	b.runs = runs
	b.touched(id)
	notices := b.notices[:0]
	for _, notice := range b.notices {
		if notice.RunID == nil || *notice.RunID != id {
			notices = append(notices, notice)
		}
	}
	b.notices = notices
	b.c.stateRevision++
	delete(b.erasing, id)
}

// Run returns the run with id, or nil.
// Lock: part of Coordinator; runs with c.mu held.
func (b *RunBook) Run(id string) *core.Run {
	index := b.index(id)
	if index < 0 {
		return nil
	}
	run := b.runs[index].Clone()
	return &run
}

// add adds a new run at its first stage and saves it.
// Lock: runs with c.mu held.
func (b *RunBook) add(run core.Run) core.Run {
	run = run.Clone()
	if b.isErasing(run.ID) {
		return run
	}
	run.Generation = b.storage.NextGeneration(run.ID)
	effects := core.EngineStart(&run, b.clock.Now())
	if !b.save(run) {
		return run
	}
	b.runs = append(b.runs, run)
	b.touched(run.ID)
	b.carryOut(effects, run, nil)
	// The caller gets its own copy, so the stored run's slices must not alias it.
	return run.Clone()
}

// apply applies event to the run, after change when there is one, and saves
// both in one write. False when the run is missing, erasing, of another
// generation, barred from events (see runWorkBars), or the save failed; the in-memory run is then left as it was
// . Effects are handed to carryOut.
// Lock: runs with c.mu held.
func (b *RunBook) apply(id string, event core.Event, generation *int, operation *int, change func(run *core.Run)) bool {
	if b.liveIndex(id, workEvent, generation, operation) < 0 {
		return false
	}
	before := b.Run(id)
	var session *string
	if attempt := before.LastAttempt(before.StageID); attempt != nil {
		session = attempt.Session
	}
	var effects []core.Effect
	if !b.update(id, generation, operation, func(run *core.Run) {
		if change != nil {
			change(run)
		}
		effects = core.EngineAdvance(run, event, b.clock.Now())
	}) {
		return false
	}
	updated := b.Run(id)
	if updated == nil {
		return false
	}
	if updated.State != before.State || updated.StageID != before.StageID {
		where := updated.StageID
		if stage := updated.Stage(); stage != nil {
			where = stage.Name
		}
		ref := updated.Subject.Ref()
		reason := core.ValueOf(updated.Reason)
		var text string
		switch updated.State {
		case core.RunStateWaiting:
			text = ref + " is waiting for you at " + where
		case core.RunStateHeld:
			text = ref + " is held at " + where + ": " + reason
		case core.RunStateShipped:
			text = ref + " shipped"
		case core.RunStateDone:
			text = ref + " done"
		case core.RunStateDropped:
			text = ref + " dropped: " + reason
		case core.RunStateParked:
			text = ref + " is parked at " + where
		default:
			text = ref + " moved to " + where
		}
		runID := id
		b.note(text, &runID)
	}
	b.carryOut(effects, *updated, session)
	return true
}

// update changes the run and saves it; false when it could not be saved.
// Lock: runs with c.mu held.
func (b *RunBook) update(id string, generation *int, operation *int, change func(run *core.Run)) bool {
	index := b.liveIndex(id, workWrite, generation, operation)
	if index < 0 {
		return false
	}
	run := b.runs[index].Clone()
	change(&run)
	if reflect.DeepEqual(run, b.runs[index]) {
		return true
	}
	if !b.save(run) {
		return false
	}
	b.runs[index] = run
	b.touched(run.ID)
	return true
}

// save writes the run; false when it could not be saved.
// Lock: runs with c.mu held.
func (b *RunBook) save(run core.Run) bool {
	if err := b.storage.SaveRun(run); err != nil {
		b.note("Could not save "+run.Subject.Ref()+": "+err.Error(), nil)
		return false
	}
	return true
}

// note appends a notice about run (or about the coordinator when run is nil).
// Lock: runs with c.mu held.
func (b *RunBook) note(text string, run *string) {
	notice := Notice{ID: b.c.ids.UUID(), Date: jsonx.Date{Time: b.clock.Now()}, Text: text, RunID: run}
	b.notices = append([]Notice{notice}, b.notices...)
	if len(b.notices) > 100 {
		b.notices = b.notices[:100]
	}
	b.c.stateRevision++
}

// Outputs reads what a run's agent stages wrote. It is a part
// of Coordinator; its methods run with c.mu held.
type Outputs struct {
	// c is the coordinator the outputs belong to (the back pointer).
	c     *Coordinator
	paths store.Paths
	// key is the placeholder key of a run's stage's agent.
	key func(stage core.Stage, run core.Run) *string
}

// newOutputs builds the outputs reader.
// Lock: runs during New.
func newOutputs(c *Coordinator, paths store.Paths, key func(stage core.Stage, run core.Run) *string) Outputs {
	return Outputs{c: c, paths: paths, key: key}
}

// latest returns the latest agent output on the run, or of the stages whose
// agent has key.
// Lock: runs with c.mu held.
func (o Outputs) latest(run core.Run, key *string) *string {
	for i := len(run.Attempts) - 1; i >= 0; i-- {
		attempt := run.Attempts[i]
		index := run.StageIndexOf(attempt.StageID)
		if index < 0 || run.Stages[index].Kind() != core.StageKindAgent {
			continue
		}
		stage := run.Stages[index]
		if key != nil {
			stageKey := o.key(stage, run)
			if stageKey == nil || *stageKey != *key {
				continue
			}
		}
		if text := o.output(run, attempt); text != nil {
			return text
		}
	}
	return nil
}

// artifacts lists an attempt's artifact file paths.
// Lock: runs with c.mu held.
func (o Outputs) artifacts(run core.Run, attempt core.Attempt) []string {
	if !run.HasAgentStage(attempt.StageID) {
		return nil
	}
	return store.ArtifactsList(o.paths.ArtifactsFolder(run, attempt))
}

// resolveArtifact returns the path of a listed artifact name, or nil.
// Lock: runs with c.mu held.
func (o Outputs) resolveArtifact(name string, run core.Run, attempt core.Attempt) *string {
	if !run.HasAgentStage(attempt.StageID) {
		return nil
	}
	path, ok := store.ArtifactsResolve(name, o.paths.ArtifactsFolder(run, attempt))
	if !ok {
		return nil
	}
	return &path
}

// output returns the output file text of one attempt, or nil.
// Lock: runs with c.mu held.
func (o Outputs) output(run core.Run, attempt core.Attempt) *string {
	index := run.StageIndexOf(attempt.StageID)
	if index < 0 || run.Stages[index].Kind() != core.StageKindAgent {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(o.paths.AttemptFolder(run, attempt), run.Stages[index].Writes()))
	if err != nil || !utf8.Valid(data) {
		return nil
	}
	text := string(data)
	return &text
}

// index is the position of the live run with id, or -1.
func (b *RunBook) index(id string) int {
	for i := range b.runs {
		if b.runs[i].ID == id {
			return i
		}
	}
	return -1
}

// runsProjection is the runs' change projection, made again only when the runs
// changed since the last call. A hit is checked against a digest of a fresh encoding
// of every run in test builds (verifyMemo), not against encodeRuns, which reuses the
// per-run memos and would hide a run that missed its touched.
// Lock: runs with c.mu held.
func (b *RunBook) runsProjection() []byte {
	if b.projected.ok && b.projected.key == b.revision {
		if shouldVerifyMemo() {
			verifyMemo("runs", b.projected.data, runsDigest(b.runs, nil))
		}
		return b.projected.data
	}
	return b.projected.of("runs", b.revision, b.encodeRuns)
}

// runMemo is one run's projection digest at the revision it last changed.
type runMemo struct {
	revision uint64
	digest   [sha256.Size]byte
}

// touched records a change to the run with id: the book's revision moves and
// the run's memo goes stale. Every write to runs calls it. Lock: with c.mu held.
func (b *RunBook) touched(id string) {
	b.revision++
	if b.changedAt == nil {
		b.changedAt = map[string]uint64{}
	}
	b.changedAt[id] = b.revision
}

// encodeRuns is runsDigest(b.runs), reusing the digest of every run that has
// not changed since it was encoded.
func (b *RunBook) encodeRuns() []byte {
	if b.runMemos == nil {
		b.runMemos = map[string]runMemo{}
	}
	b.digestStale()
	live := make(map[string]struct{}, len(b.runs))
	out := runsDigest(b.runs, func(run *core.Run) [sha256.Size]byte {
		id := run.ID
		live[id] = struct{}{}
		memo, ok := b.runMemos[id]
		if !ok || memo.revision != b.changedAt[id] {
			memo = runMemo{revision: b.changedAt[id], digest: sha256.Sum256(projectionBytes(*run))}
			b.runMemos[id] = memo
		}
		return memo.digest
	})
	for id := range b.runMemos {
		if _, ok := live[id]; !ok {
			delete(b.runMemos, id)
		}
	}
	return out
}

// parallelDigests is how many stale runs make digestStale spread their encoding
// over the processors; a turn that changed a few runs encodes them in place.
const parallelDigests = 64

// digestStale works out the memo of every run whose memo is stale, spread over the
// processors, when there are enough of them: the first projection after a start
// encodes every run in the book. The workers only read the runs, under the lock
// the caller holds, and the memos are written once they are done.
// Lock: runs with c.mu held.
func (b *RunBook) digestStale() {
	var stale []int
	for i := range b.runs {
		memo, ok := b.runMemos[b.runs[i].ID]
		if !ok || memo.revision != b.changedAt[b.runs[i].ID] {
			stale = append(stale, i)
		}
	}
	if len(stale) < parallelDigests {
		return
	}
	digests := make([][sha256.Size]byte, len(stale))
	next := make(chan int)
	var workers sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		workers.Go(func() {
			for k := range next {
				digests[k] = sha256.Sum256(projectionBytes(b.runs[stale[k]]))
			}
		})
	}
	for k := range stale {
		next <- k
	}
	close(next)
	workers.Wait()
	for k, i := range stale {
		id := b.runs[i].ID
		b.runMemos[id] = runMemo{revision: b.changedAt[id], digest: digests[k]}
	}
}

// runsDigest is the runs' projection: the SHA-256 of each run's encoding, in order, in brackets. Turns only
// compares projections for equality, and two digests are equal exactly when every run encodes
// as it did, so a change to one run of a large book costs that run's encoding and a few dozen
// bytes per run, rather than the encoding of every run, which grows to megabytes, copied for the
// runs and again for the state and compared byte for byte after the turn. digest gives each
// run's digest; nil encodes every run afresh.
// Lock: runs with c.mu held.
func runsDigest(runs []core.Run, digest func(run *core.Run) [sha256.Size]byte) []byte {
	out := make([]byte, 0, 2+len(runs)*sha256.Size)
	out = append(out, '[')
	for i := range runs {
		var sum [sha256.Size]byte
		if digest != nil {
			sum = digest(&runs[i])
		} else {
			sum = sha256.Sum256(projectionBytes(runs[i]))
		}
		out = append(out, sum[:]...)
	}
	return append(out, ']')
}

// encodeProjection is the library's change projection: its belts and agents, encoded.
func (l *Library) encodeProjection() []byte {
	belts := make([]beltProjectionRow, 0, len(l.belts))
	for _, record := range l.belts {
		belts = append(belts, beltRowOf(record))
	}
	return projectionBytes([]any{belts, l.agents})
}

// projection is the library's change-projection encoding, re-encoded only when
// the library changed since the last call. Lock: runs with c.mu held.
func (l *Library) projection() []byte {
	return l.projected.of("library", l.revision, l.encodeProjection)
}

// appendRunForContract appends run to the live runs without saving it
// (test-only; see coordinator_contract_seed.go).
// Lock: with c.mu held.
func (b *RunBook) appendRunForContract(run core.Run) {
	b.runs = append(b.runs, run)
	b.touched(run.ID)
}
