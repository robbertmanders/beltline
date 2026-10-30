// and the
// flow tests of multi-repository triggers (the Jira flows): several
// triggers on one belt, each with its own repositories, intervals and cursors.

package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/github"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/jira"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/testkit"
)

// multiIssueEntry is a GitHub issues entry with a label and poll interval, on repos.
func multiIssueEntry(id, label string, poll int, repos ...string) core.TriggerEntry {
	filter := core.IssueFilter{Label: label, Assignee: core.TriggerAssigneeAnyone, Poll: poll}
	return core.TriggerEntry{ID: id, Trigger: core.Trigger{Kind: core.TriggerKindIssues, Issues: &filter}, Repos: repos}
}

// multiJiraEntry is a Jira entry with a query, on repos (a 60 second poll, the default).
func multiJiraEntry(id, jql string, repos ...string) core.TriggerEntry {
	query := core.DefaultJiraQuery()
	query.Jql = jql
	return core.TriggerEntry{ID: id, Trigger: core.Trigger{Kind: core.TriggerKindJira, Jira: &query}, Repos: repos}
}

// multiScheduleEntry is a weekday schedule at hour with a title and task, on repos.
func multiScheduleEntry(id string, hour int, title, task string, repos ...string) core.TriggerEntry {
	scheduled := core.ScheduledTask{Schedule: core.DefaultSchedule(), Title: title, Task: task}
	scheduled.Schedule.Days = core.ScheduleDaysWeekdays
	scheduled.Schedule.Hour = hour
	return core.TriggerEntry{ID: id, Trigger: core.Trigger{Kind: core.TriggerKindSchedule, Schedule: &scheduled}, Repos: repos}
}

// multiBelt is a belt named "Shared" with the given triggers, stages and forks.
func multiBelt(id string, triggers []core.TriggerEntry, stages []core.Stage, forks []core.Fork) core.Belt {
	belt := core.DefaultBelt(id, "Shared", triggers, stages)
	belt.Forks = forks
	return belt
}

// multiIssueBelt is the shared belt with Triage and an approval gate on issues.
func multiIssueBelt(triggers []core.TriggerEntry) core.Belt {
	return multiBelt("shared", triggers, flowTriageApproval(core.TriggerKindIssues), nil)
}

// multiGate holds one fake call where the test can see it: the call enters and waits until
// the gate is opened.
type multiGate struct {
	entered chan struct{}
	open    chan struct{}
	enter   sync.Once
	release sync.Once
}

// newMultiGate is a closed-for-now gate.
func newMultiGate() *multiGate {
	return &multiGate{entered: make(chan struct{}), open: make(chan struct{})}
}

// wait is the hook body: it marks the call entered, then waits for the gate to open.
func (g *multiGate) wait(context.Context) {
	g.enter.Do(func() { close(g.entered) })
	<-g.open
}

// releaseNow opens the gate. It is safe to call more than once.
func (g *multiGate) releaseNow() { g.release.Do(func() { close(g.open) }) }

// awaitEntered waits until a hooked call has entered the gate.
func (g *multiGate) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the hooked call never entered")
	}
}

// TestEntriesPollOnTheirOwnIntervalsAndCheckNowChecksAll
func TestEntriesPollOnTheirOwnIntervalsAndCheckNowChecksAll(t *testing.T) {
	t.Parallel()
	fast := multiIssueEntry("1", "agent", 30, "acme/billing")
	slow := multiIssueEntry("2", "bug", 300, "acme/web")
	h := newFlowHarness(t, flowOptions{
		issues: []core.Subject{flowIssue("acme/billing", 1, "One", "agent"), flowIssue("acme/web", 2, "Two", "bug")},
		belts:  []core.Belt{multiIssueBelt([]core.TriggerEntry{fast, slow})},
		now:    time.Unix(1_000, 0),
	})
	h.Tick()
	if got := sortedNumbers(h.Coord().Runs()); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("numbers = %v, want [1 2]", got)
	}
	h.github.Put(flowIssue("acme/billing", 3, "Three", "agent"))
	h.github.Put(flowIssue("acme/web", 4, "Four", "bug"))
	h.clock.Set(time.Unix(1_040, 0))
	h.Tick()
	if !hasRunNumber(h.Coord().Runs(), 3) {
		t.Fatal("the 30 second entry did not check its repository")
	}
	if hasRunNumber(h.Coord().Runs(), 4) {
		t.Fatal("the 300 second entry checked before its interval")
	}
	flowCheck(h)
	if !hasRunNumber(h.Coord().Runs(), 4) {
		t.Fatal("check now did not check every entry")
	}
}

// flowRunByID is the run with id as a copy, or nil.
func flowRunByID(h *harness, id string) *core.Run { return h.Coord().Run(id) }

// sortedNumbers is the subject numbers of runs, sorted.
func sortedNumbers(runs []core.Run) []int {
	out := make([]int, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.Subject.Number)
	}
	sort.Ints(out)
	return out
}

// hasRunNumber reports whether a run is about issue number.
func hasRunNumber(runs []core.Run, number int) bool {
	return slices.ContainsFunc(runs, func(run core.Run) bool { return run.Subject.Number == number })
}

// TestOneSourceFailingDoesNotStopTheOtherOrTheRuns
func TestOneSourceFailingDoesNotStopTheOtherOrTheRuns(t *testing.T) {
	t.Parallel()
	billing := multiIssueEntry("1", "agent", 30, "acme/billing")
	web := multiIssueEntry("2", "agent", 30, "acme/web")
	h := newFlowHarness(t, flowOptions{
		issues: []core.Subject{flowIssue("acme/billing", 1, "One", "agent"), flowIssue("acme/web", 2, "Two", "agent")},
		belts:  []core.Belt{multiIssueBelt([]core.TriggerEntry{billing, web})},
		now:    time.Unix(1_000, 0),
	})
	h.Tick()
	if n := len(h.Coord().Runs()); n != 2 {
		t.Fatalf("runs = %d, want 2", n)
	}
	h.github.SetFailing("acme/billing", true)
	h.github.Put(flowIssue("acme/web", 3, "Three", "agent"))
	h.clock.Set(time.Unix(1_040, 0))
	h.Tick()
	if !hasRunNumber(h.Coord().Runs(), 3) {
		t.Fatal("the other entry stopped admitting work")
	}
	for _, run := range h.Coord().Runs() {
		if run.State == core.RunStateDropped {
			t.Fatalf("run %s was dropped by a failing source", run.ID)
		}
	}
	if !hasNotice(h.Coord().Notices(), "acme/billing") {
		t.Fatal("no notice names the failing repository")
	}
}

// TestOverlappingSubjectsAdmitOnceAndOnlyTheOwnerRefreshes
func TestOverlappingSubjectsAdmitOnceAndOnlyTheOwnerRefreshes(t *testing.T) {
	t.Parallel()
	first := multiIssueEntry("1", "agent", 30, "acme/billing")
	second := multiIssueEntry("2", "bug", 30, "acme/billing")
	issue := core.Subject{Kind: core.SubjectKindIssue, Repo: "acme/billing", Number: 9, Title: "Both", Labels: []string{"agent", "bug"}, IsOpen: true}
	h := newFlowHarness(t, flowOptions{issues: []core.Subject{issue}, belts: []core.Belt{multiIssueBelt([]core.TriggerEntry{first, second})}, now: time.Unix(1_000, 0)})
	h.Tick()
	if n := len(h.Coord().Runs()); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	if source := h.Run().Source; source == nil || source.Entry != "1" {
		t.Fatalf("source = %+v, want entry 1", source)
	}
	// Only the second entry is due. It still lists the issue, and must not take it over.
	edited := issue
	edited.Title = "Renamed"
	edited.Labels = []string{"bug"}
	h.github.Put(edited)
	h.clock.Set(time.Unix(1_040, 0))
	h.Turn(func() {
		h.coord.intake.lastPoll[pollKey("shared", "1")] = h.clock.Now()
		h.coord.intake.lastPoll[pollKey("shared", "2")] = time.Unix(1_000, 0)
	})
	h.Tick()
	if title := h.Run().Subject.Title; title != "Both" {
		t.Fatalf("title = %s, want Both", title)
	}
	if h.Run().State == core.RunStateDropped {
		t.Fatal("the run was dropped by the entry that does not own it")
	}
	h.clock.Set(time.Unix(1_400, 0))
	h.Tick()
	if state := h.Run().State; state != core.RunStateDropped {
		t.Fatalf("state = %s, want dropped once its owner no longer lists it", state)
	}
}

// TestTheSameJiraTicketFromTwoQueriesIsOneRun
func TestTheSameJiraTicketFromTwoQueriesIsOneRun(t *testing.T) {
	t.Parallel()
	a := multiJiraEntry("1", "project = PAY", "acme/billing", "acme/infra")
	b := multiJiraEntry("2", "labels = agent", "acme/web")
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{multiBelt("shared", []core.TriggerEntry{a, b}, flowTriageApproval(core.TriggerKindJira), nil)},
		tickets: []core.Subject{flowTicket}, now: time.Unix(1_000, 0),
	})
	h.Tick()
	runs := h.Coord().Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if !reflect.DeepEqual(runs[0].Repos, []string{"acme/billing", "acme/infra"}) {
		t.Fatalf("repos = %v", runs[0].Repos)
	}
	if runs[0].Subject.Repo != "acme/billing" {
		t.Fatalf("subject repo = %s, want acme/billing", runs[0].Subject.Repo)
	}
	if source := runs[0].Source; source == nil || source.Entry != "1" {
		t.Fatalf("source = %+v, want entry 1", source)
	}
}

// TestTwoSchedulesFireTogetherAndOnlyTheirOwnRunSkips
func TestTwoSchedulesFireTogetherAndOnlyTheirOwnRunSkips(t *testing.T) {
	t.Parallel()
	morning := multiScheduleEntry("1", 9, "Morning", "One", "acme/platform")
	also := multiScheduleEntry("2", 9, "Also", "Two", "acme/platform")
	h := newFlowHarness(t, flowOptions{
		belts: []core.Belt{multiBelt("shared", []core.TriggerEntry{morning, also}, flowTriageApproval(core.TriggerKindSchedule), nil)},
		now:   flowLocal(9, 29, 8, 0),
	})
	h.Tick()
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d before the time, want none", n)
	}
	h.clock.Set(flowLocal(9, 29, 9, 30))
	h.Tick()
	runs := h.Coord().Runs()
	titles := map[string]struct{}{}
	keys := map[string]struct{}{}
	for _, run := range runs {
		titles[run.Subject.Title] = struct{}{}
		keys[run.Subject.Key] = struct{}{}
	}
	if _, ok := titles["Morning"]; !ok {
		t.Fatalf("titles = %v, want Morning", titles)
	}
	if _, ok := titles["Also"]; !ok {
		t.Fatalf("titles = %v, want Also", titles)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want two distinct", keys)
	}
	for _, run := range runs {
		// The subject's init defaults isOpen to true; a scheduled subject keeps it.
		if !run.Subject.IsOpen {
			t.Fatalf("scheduled run %s is not open", run.ID)
		}
	}
	var first core.Run
	for _, run := range runs {
		if run.Subject.Title == "Morning" {
			first = run
		}
	}
	h.Coord().Drop(first.ID, nil)
	h.clock.Set(flowLocal(9, 30, 9, 30))
	h.Tick()
	var morningNext, alsoCount int
	for _, run := range h.Coord().Runs() {
		if run.Subject.Title == "Morning" && run.Subject.Key == "2026-09-30" {
			morningNext++
		}
		if run.Subject.Title == "Also" {
			alsoCount++
		}
	}
	if morningNext != 1 {
		t.Fatal("the next morning run did not start")
	}
	if alsoCount != 1 {
		t.Fatalf("Also runs = %d, want 1", alsoCount)
	}
}

// TestAScheduleSaveFailureDoesNotConsumeTheSlot
func TestAScheduleSaveFailureDoesNotConsumeTheSlot(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		belts: []core.Belt{flowBelt(core.TriggerKindSchedule, "", flowTriageApproval(core.TriggerKindSchedule))},
		now:   flowLocal(9, 29, 8, 0),
	})
	h.Tick()
	folder := filepath.Join(h.Coord().Paths().Belts(), "schedule")
	if err := os.Chmod(folder, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	h.clock.Set(flowLocal(9, 29, 10, 0))
	h.Tick()
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d, want none", n)
	}
	if last := h.Coord().Belt("schedule").Fired[core.TriggerEntryLegacyID]; !last.Equal(flowLocal(9, 29, 8, 0)) {
		t.Fatalf("cursor = %v, want 08:00", last)
	}
	if err := os.Chmod(folder, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	reopened := h.RestartWith(h.idleSimulatedRunner())
	reopened.Lock()
	reopened.intake.fireSchedules()
	reopened.EndTurn()
	reopened.Idle()
	if got := flowKeys(reopened.Runs()); !reflect.DeepEqual(got, []string{"2026-09-29"}) {
		t.Fatalf("keys = %v, want [2026-09-29]", got)
	}
}

// TestAManualEntryOnAMixedBeltStartsTasksFromThatEntry
func TestAManualEntryOnAMixedBeltStartsTasksFromThatEntry(t *testing.T) {
	t.Parallel()
	issues := multiIssueEntry("1", "agent", 60, "acme/billing")
	tasks := core.TriggerEntry{ID: "2", Trigger: core.TriggerKindManual.Fresh(), Repos: []string{"acme/web"}}
	other := core.TriggerEntry{ID: "3", Trigger: core.TriggerKindManual.Fresh(), Repos: []string{"acme/docs"}}
	h := newFlowHarness(t, flowOptions{belts: []core.Belt{multiIssueBelt([]core.TriggerEntry{issues, tasks, other})}})
	entry2, entry3, entry1 := "2", "3", "1"
	run, err := h.Coord().StartTask("shared", "acme/web", "Tidy", "Copy", &entry2)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if run.Subject.Key != "1" || run.Source == nil || run.Source.Entry != "2" {
		t.Fatalf("run key %s source %+v, want key 1 from entry 2", run.Subject.Key, run.Source)
	}
	second, err := h.Coord().StartTask("shared", "acme/docs", "Docs", "", &entry3)
	if err != nil || second.Subject.Key != "2" {
		t.Fatalf("second task = %v, %v, want key 2", second.Subject.Key, err)
	}
	refused := []struct {
		name, repo, title string
		entry             *string
	}{
		{"repository of another entry", "acme/web", "No", &entry3},
		{"a trigger that is not manual", "acme/web", "No", &entry1},
		{"a blank title", "acme/web", "  ", &entry2},
	}
	for _, c := range refused {
		if _, err := h.Coord().StartTask("shared", c.repo, c.title, "", c.entry); err == nil {
			t.Fatalf("%s was accepted", c.name)
		}
	}
	if err := h.Coord().SetPaused("shared", true); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := h.Coord().StartTask("shared", "acme/web", "Paused", "", &entry2); err == nil {
		t.Fatal("a task on a paused belt was accepted")
	}
}

// TestRemovingAnEntryLeavesItsRunAndStopsNewAdmissions
func TestRemovingAnEntryLeavesItsRunAndStopsNewAdmissions(t *testing.T) {
	t.Parallel()
	first := multiIssueEntry("1", "agent", 30, "acme/billing")
	second := multiIssueEntry("2", "bug", 30, "acme/web")
	h := newFlowHarness(t, flowOptions{
		issues: []core.Subject{flowIssue("acme/billing", 1, "Keep", "agent"), flowIssue("acme/web", 2, "Also", "bug")},
		belts:  []core.Belt{multiBelt("shared", []core.TriggerEntry{first, second}, core.SampleStages(core.TriggerKindIssues, core.SampleAgents(), true), core.SampleForks(core.TriggerKindIssues))},
		now:    time.Unix(1_000, 0),
	})
	h.Tick()
	var owned core.Run
	for _, run := range h.Coord().Runs() {
		if run.Source != nil && run.Source.Entry == "1" {
			owned = run
		}
	}
	if owned.ID == "" {
		t.Fatal("no run from entry 1")
	}
	belt := h.Coord().Belts()[0].Belt
	belt.RemoveTrigger("1")
	belt.Base = "release"
	if _, err := h.Coord().SaveBelt(belt); err != nil {
		t.Fatalf("save: %v", err)
	}
	h.github.Put(flowIssue("acme/billing", 3, "New", "agent"))
	h.clock.Set(time.Unix(1_040, 0))
	flowCheck(h)
	if hasRunNumber(h.Coord().Runs(), 3) {
		t.Fatal("a removed entry admitted a run")
	}
	after := flowRunByID(h, owned.ID)
	if after == nil || after.State == core.RunStateDropped {
		t.Fatal("the removed entry's run was dropped")
	}
	if after.Base != owned.Base {
		t.Fatalf("base = %q, want %q", after.Base, owned.Base)
	}
	if !reflect.DeepEqual(after.Stages, owned.Stages) {
		t.Fatal("the run's stages changed with the belt")
	}
	closed := flowIssue("acme/billing", 1, "Keep", "agent")
	closed.IsOpen = false
	h.github.Put(closed)
	flowCheck(h)
	if state := flowRunByID(h, owned.ID).State; state == core.RunStateDropped {
		t.Fatal("a check dropped the run before it was reconciled")
	}
	h.Turn(func() {
		if err := h.coord.intake.reconcile(context.Background(), owned.ID); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	})
	if state := flowRunByID(h, owned.ID).State; state != core.RunStateDropped {
		t.Fatalf("state = %s after reconciling, want dropped", state)
	}
}

// TestAPauseOrArchiveDuringTheReadAdmitsNothing
func TestAPauseOrArchiveDuringTheReadAdmitsNothing(t *testing.T) {
	t.Parallel()
	for _, archive := range []bool{false, true} {
		name := "pause"
		if archive {
			name = "archive"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := multiIssueEntry("1", "agent", 30, "acme/billing")
			h := newFlowHarness(t, flowOptions{
				issues: []core.Subject{flowIssue("acme/billing", 1, "Keep", "agent")},
				belts:  []core.Belt{multiIssueBelt([]core.TriggerEntry{entry})},
				now:    time.Unix(1_000, 0),
			})
			h.Tick()
			if got := sortedNumbers(h.Coord().Runs()); !reflect.DeepEqual(got, []int{1}) {
				t.Fatalf("numbers = %v, want [1]", got)
			}
			h.github.Put(flowIssue("acme/billing", 1, "Renamed", "agent"))
			h.github.Put(flowIssue("acme/billing", 2, "New", "agent"))
			gate := newMultiGate()
			h.github.BeforeIssueList = gate.wait
			defer gate.releaseNow()
			checked := make(chan struct{})
			go func() {
				defer close(checked)
				flowCheck(h)
			}()
			gate.awaitEntered(t)
			if archive {
				h.Coord().SetBeltArchived("shared", true)
			} else if err := h.Coord().SetPaused("shared", true); err != nil {
				t.Fatalf("pause: %v", err)
			}
			gate.releaseNow()
			<-checked
			if got := sortedNumbers(h.Coord().Runs()); !reflect.DeepEqual(got, []int{1}) {
				t.Fatalf("numbers = %v after the read, want [1]", got)
			}
			if title := h.Run().Subject.Title; title != "Renamed" {
				t.Fatalf("title = %s, want Renamed", title)
			}
		})
	}
}

// TestTheSameBlockedJiraTicketIsOneWaitingRow
func TestTheSameBlockedJiraTicketIsOneWaitingRow(t *testing.T) {
	t.Parallel()
	blocked := flowTicket
	blocked.Blockers = []string{"PAY-9"}
	other := flowTicket
	other.Key = "PAY-32"
	other.Title = "Another"
	other.Blockers = []string{"PAY-8"}
	billing := multiJiraEntry("1", "project = PAY", "acme/billing", "acme/infra")
	web := multiJiraEntry("2", "labels = agent", "acme/web")
	h := newFlowHarness(t, flowOptions{
		belts:   []core.Belt{multiBelt("shared", []core.TriggerEntry{billing, web}, flowTriageApproval(core.TriggerKindJira), nil)},
		tickets: []core.Subject{blocked, other},
	})
	h.Tick()
	if n := len(h.Coord().Runs()); n != 0 {
		t.Fatalf("runs = %d, want none", n)
	}
	var keys []string
	for _, waiting := range h.Coord().ReadWaitingSubjects() {
		keys = append(keys, waiting.Subject.Key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"PAY-31", "PAY-32"}) {
		t.Fatalf("waiting = %v, want [PAY-31 PAY-32]", keys)
	}
}

// TestALegacyRunUsesItsSavedBeltVersionAfterTheLiveBeltChanges
func TestALegacyRunUsesItsSavedBeltVersionAfterTheLiveBeltChanges(t *testing.T) {
	t.Parallel()
	h := newFlowHarness(t, flowOptions{
		issues: []core.Subject{flowIssue("acme/billing", 1, "Old", "agent")},
		belts:  []core.Belt{flowBelt(core.TriggerKindIssues, "", nil)},
		now:    time.Unix(1_000, 0),
	})
	h.Tick()
	run := *h.Run()
	h.Turn(func() {
		h.coord.book.update(run.ID, nil, nil, func(r *core.Run) { r.Source = nil })
	})
	changed := h.Coord().Belts()[0].Belt
	changed.Triggers = []core.TriggerEntry{multiIssueEntry("9", "other", 60, "acme/other")}
	if _, err := h.Coord().SaveBelt(changed); err != nil {
		t.Fatalf("save: %v", err)
	}
	h.Turn(func() {
		if err := h.coord.intake.reconcile(context.Background(), run.ID); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	})
	after := flowRunByID(h, run.ID)
	if after == nil || after.ID != run.ID {
		t.Fatal("the run is gone or changed id")
	}
	if after.Source == nil || after.Source.Entry != "1" {
		t.Fatalf("source = %+v, want entry 1", after.Source)
	}
	if after.Source.Trigger.Kind != core.TriggerKindIssues || after.Source.Trigger.Issues == nil || after.Source.Trigger.Issues.Label != "agent" {
		t.Fatalf("source trigger = %+v, want the issues trigger the run was admitted by", after.Source.Trigger)
	}
}

// TestAnOldScheduleCursorIsNotFiredAgain
func TestAnOldScheduleCursorIsNotFiredAgain(t *testing.T) {
	t.Parallel()
	root := harnessFolder(t)
	storage := store.NewStorage(store.Paths{Root: root})
	definition := flowBelt(core.TriggerKindSchedule, "", []core.Stage{core.DefaultsStage(core.StageTypeApprove, nil, nil, nil)})
	saved, err := storage.SaveNewVersion(definition)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	slot := flowLocal(9, 29, 9, 0)
	record := store.NewBeltRecord(store.BeltRecordArgs{Belt: saved, Created: testkit.ContractTime, WatchOutcomes: true, ReviewOutcomes: true})
	record.Fired = map[string]time.Time{core.TriggerEntryLegacyID: slot}
	if err := storage.SaveState(record); err != nil {
		t.Fatalf("save state: %v", err)
	}
	file := filepath.Join(store.Paths{Root: root}.Belts(), saved.ID, "v"+strconv.Itoa(saved.Version)+".json")
	before := readBytes(t, file)
	loaded := storage.LoadBelts()
	if len(loaded) == 0 {
		t.Fatal("no belt loaded")
	}
	if got := loaded[0].Fired[core.TriggerEntryLegacyID]; !got.Equal(slot) {
		t.Fatalf("fired = %v, want %v", got, slot)
	}
	if got := readBytes(t, file); string(got) != string(before) {
		t.Fatal("loading the belt rewrote its saved version")
	}
	clock := testkit.NewClock(flowLocal(9, 29, 10, 0))
	coord, err := New(Services{
		Storage: storage, GitHub: github.NewFakeGitHub(nil, nil, nil, nil), Jira: jira.NewFakeJira(nil, nil, nil),
		Runner:     runner.NewSimulatedRunner(func(runner.SessionSpec) float64 { return 0 }, clock.Now, func(runner.SessionSpec) *string { return nil }),
		Workspaces: runner.FolderWorkspaces{}, Clock: clock, IDs: &SequenceIDs{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { coord.Close() })
	coord.Lock()
	coord.intake.fireSchedules()
	coord.EndTurn()
	coord.Idle()
	if n := len(coord.Runs()); n != 0 {
		t.Fatalf("runs = %d, want none: the cursor was fired again", n)
	}
}
