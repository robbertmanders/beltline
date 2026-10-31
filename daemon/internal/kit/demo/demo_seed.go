// Prepare, open, make and the seed
// table: where each issue already is when the demo opens.

package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/runner"
	"github.com/robbertmanders/beltline/daemon/internal/integrations/store"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// LockFileName is the lock a launch that owns the folder keeps
// (OwnershipLock.fileName); a fresh demo keeps it. It lives here, below launch,
// because launch imports this package: launch's OwnershipLockFileName is this
// constant.
const LockFileName = "daemon.lock"

// Setup is a demo folder opened for use: its storage, the data its fake
// services start from, and the simulated runner and folder workspaces the
// coordinator is given (see newSetup).
type Setup struct {
	Storage  store.Storage
	Services Services
	// LibraryURL gives the demo's library repositories' local remotes.
	LibraryURL func(repository string) string
	// Runner runs the demo's sessions on a clock.
	Runner *runner.SimulatedRunner
	// Workspaces are plain folders, for demo mode.
	Workspaces runner.FolderWorkspaces
}

// seed is one run the demo starts with: where it is, and how long ago it began.
type seed struct {
	Belt string
	// Ref is the subject's ref, such as "#139" or "PAY-40".
	Ref      string
	Outcomes []string
	// Age is how long ago the run started, in seconds.
	Age float64
}

// demoSeeds are the runs seeded at the start, each taken through its outcomes.
func demoSeeds() []seed {
	return []seed{
		{"issues", "#139", []string{"ready to spec", "complete"}, 3600},
		{"issues", "#142", []string{"ready to spec", "complete", "approved"}, 7200},
		{"issues", "#145", []string{"ready to implement", "done"}, 5400},
		{"issues", "#147", []string{"ready to spec"}, 1800},
		{"issues", "#151", []string{"needs info"}, 2400},
		{"issues", "#152", nil, 1200},
		{"security", "#93", nil, 800},
		{"issues", "#153", []string{"wait to implement"}, 86400},
		{"issues", "#155", []string{"ready to implement", "done", "pass", "opened"}, 14400},
		{"security", "#88", []string{"ready to spec", "complete"}, 5000},
		{"security", "#91", []string{"ready to implement", "complete", "approved", "done"}, 9000},
		{"security", "#86", []string{"ready to implement", "complete", "approved", "done", "pass"}, 12000},
		{"docs", "#412", []string{"ready to implement", "done", "opened"}, 20000},
		{"docs", "#415", []string{"ready to implement"}, 900},
		{"reviews", "#160", nil, 600},
		{"jira", "PAY-31", []string{"ready to spec", "complete"}, 4000},
		{"jira", "PAY-34", []string{"ready to spec", "complete", "approved", "done", "pass", "opened"}, 10000},
		{"tasks", "T1", []string{"complete", "approved", "done"}, 6000},
		{"nested", "#180", []string{"ready to spec", "complete"}, 2400},
		{"epics", "PAY-40", []string{"ready", "approved", "opened", "filed"}, 30000},
		{"jira", "PAY-42", []string{"ready to implement"}, 2000},
	}
}

// Prepare opens the demo in root, seeding it first when root holds no demo yet
// (no settings.json). A folder that already holds a demo is opened as it is, so
// a test can copy one seeded folder instead of writing it again.
func Prepare(root string, now time.Time, env func(string) string) (Setup, error) {
	if _, err := os.Stat(filepath.Join(root, "settings.json")); err == nil {
		return Open(root), nil
	}
	return Make(root, now, env)
}

// Open is the fake services and storage for a demo folder that is already on
// disk. Its pull requests are the runs' own, each with the run's branch as its
// head.
func Open(root string) Setup {
	storage := store.NewStorage(store.Paths{Root: root})
	return OpenWith(root, PullHeads(storage.LoadRuns()))
}

// PullHeads is every run's pull requests, each with the run's branch as its head
// (the pulls Open gives the fake GitHub).
func PullHeads(runs []core.Run) []PullRequestHead {
	var pulls []PullRequestHead
	for _, run := range runs {
		head := core.ValueOf(run.Branch)
		for _, pull := range run.PullRequests {
			pulls = append(pulls, PullRequestHead{Pull: pull, Head: head})
		}
	}
	return pulls
}

// OpenWith is Open with the pull requests already read: tests that open many copies
// of one seeded folder read its runs once.
func OpenWith(root string, pulls []PullRequestHead) Setup {
	return newSetup(store.NewStorage(store.Paths{Root: root}), services(pulls))
}

// Make writes a fresh demo in root, which is emptied first. The folder itself
// and its daemon.lock stay, so a launch that owns the folder keeps it. now is
// the demo's clock: every seeded time is relative to it, so the same now always
// writes the same files, except the session ids of attempts (random).
// env is the process environment, read for the agent executables and
// the BELTLINE_DEMO_LONG_MARKDOWN switch.
func Make(root string, now time.Time, env func(string) string) (Setup, error) {
	if err := clearFolder(root); err != nil {
		return Setup{}, err
	}
	paths := store.Paths{Root: root}
	storage := store.NewStorage(paths)
	storage.SeedAgents()
	saved := map[string]core.Belt{}
	records := map[string]store.BeltRecord{}
	outcomes := 0
	demoBelts := Belts()
	for index, entry := range demoBelts {
		belt := entry.Belt
		// A fresh demo has the same domain ids every time. Outcome ids are saved in
		// belts and frozen into runs, so tests keep normalisation to session data.
		for si := range belt.Stages {
			for oi := range belt.Stages[si].Outcomes {
				outcomes++
				belt.Stages[si].Outcomes[oi].ID = outcomeID(outcomes)
			}
		}
		version, err := storage.SaveNewVersion(belt)
		if err != nil {
			return Setup{}, err
		}
		// Belts are listed by when their first version was written. Linux stamps files with a coarse
		// clock, so belts written within a few milliseconds would tie; a second apart each, in seed
		// order, they list the same everywhere (macOS moves the birth time back with it).
		first := filepath.Join(storage.Paths.Belts(), belt.ID, "v"+strconv.Itoa(version.Version)+".json")
		stamp := now.Add(-time.Duration(len(demoBelts)-index) * time.Second)
		if err := os.Chtimes(first, stamp, stamp); err != nil {
			return Setup{}, err
		}
		saved[belt.ID] = version
		record := store.NewBeltRecord(store.BeltRecordArgs{
			Belt: belt, Paused: entry.Paused, Created: now, LastFired: lastFiredFor(belt, now),
			WatchOutcomes: true, ReviewOutcomes: true,
		})
		if err := storage.SaveState(record); err != nil {
			return Setup{}, err
		}
		record.Belt = version
		records[belt.ID] = record
	}
	if err := seedSettings(storage, now, env); err != nil {
		return Setup{}, err
	}
	if err := seedLibraryRepos(root, saved); err != nil {
		return Setup{}, err
	}
	shadow, err := seedShadow(storage, records["issues"], now)
	if err != nil {
		return Setup{}, err
	}
	pulls, err := seedRuns(storage, saved, shadow, now)
	if err != nil {
		return Setup{}, err
	}
	if err := seedHistory(storage, saved, shadow, now); err != nil {
		return Setup{}, err
	}
	if err := seedLongMarkdown(storage, env); err != nil {
		return Setup{}, err
	}
	return newSetup(storage, services(pulls)), nil
}

// lastFiredFor is when a scheduled belt last fired: the demo's now. Nil for other kinds.
func lastFiredFor(belt core.Belt, now time.Time) *time.Time {
	if belt.Trigger().Kind != core.TriggerKindSchedule {
		return nil
	}
	return &now
}

// clearFolder removes everything in root except the lock file. Errors on
// individual entries are ignored.
func clearFolder(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == LockFileName {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, entry.Name()))
	}
	return nil
}

// seedSettings writes the settings, each runner's fixture model catalog and the
// configuration and AFK models, with the demo's two library repositories listed and Insights shown.
func seedSettings(storage store.Storage, now time.Time, env func(string) string) error {
	settings := store.DefaultSettings()
	settings.Login = "you"
	settings.Notifications = false
	// The demo shows every page, Insights and shadow tests included.
	settings.ShowInsights = true
	settings.EnableShadowTests = true
	settings.LibraryRepositories = []core.LibraryRepository{
		{Repository: demoSharedRepository, Role: core.LibraryRoleShared, Publish: core.LibraryPublishPush},
		{Repository: demoBackupRepository, Role: core.LibraryRoleBackup, Publish: core.LibraryPublishPullRequest},
	}
	models := fixtureModels()
	for _, runner := range core.AllBackend {
		snapshot, err := core.NewModelCatalogSnapshot(runner, runner.Executable(env), now, models)
		if err != nil {
			return err
		}
		settings.ModelCatalogs[string(runner)] = snapshot
		allowed := map[string]struct{}{}
		for _, model := range models {
			allowed[model.ID] = struct{}{}
		}
		settings.AllowedModels[string(runner)] = allowed
	}
	settings.ConfigurationModel = "opus-5.5-1m"
	settings.Afk.Model = "opus-5.5-250k"
	return storage.SaveSettings(settings)
}

// demoSubject pairs a subject with the belt it is queued on.
type demoSubject struct {
	Belt    string
	Subject core.Subject
}

// seedRuns starts each seeded run, takes it through its outcomes, gives the
// runs that reached a watch their pull requests, and saves them with their AFK
// decisions. It returns the pull requests it opened, with their heads.
func seedRuns(storage store.Storage, saved map[string]core.Belt, shadow *demoShadow, now time.Time) ([]PullRequestHead, error) {
	var subjects []demoSubject
	for _, f := range demoFixtures() {
		subjects = append(subjects, demoSubject{Belt: f.Belt, Subject: f.Subject})
	}
	// Yesterday's sweep, shipped.
	yesterday := addSeconds(now, -86400)
	sweep := core.Subject{
		Kind: core.SubjectKindScheduled, Repo: "acme/platform", Key: core.ScheduleKey(yesterday, time.Local), IsOpen: true,
		Title: "Sweep flaky tests",
		Body:  "Run the test suite five times. Fix or quarantine every test that fails some of the time.",
	}
	subjects = append(subjects, demoSubject{Belt: "nightly", Subject: sweep})
	var pulls []PullRequestHead
	var decisions []core.AfkDecision
	seeds := append(demoSeeds(), seed{Belt: "nightly", Ref: sweep.Ref(), Outcomes: []string{"done", "pass", "opened"}, Age: 80000})
	for _, sd := range seeds {
		belt, ok := saved[sd.Belt]
		subject, found := findSubject(subjects, sd.Belt, sd.Ref)
		if !ok || !found {
			continue
		}
		arm, onTest := demoShadowArms[sd.Belt+sd.Ref]
		if shadow != nil && onTest && arm == core.ShadowArmShadow {
			belt = shadow.belt
		}
		run := core.NewRun(core.RunArgs{Belt: belt, Subject: subject, Now: addSeconds(now, -sd.Age)})
		if shadow != nil && onTest {
			run.Shadow = &core.RunShadow{Test: shadow.test.ID, Arm: arm}
		}
		clock := addSeconds(now, -sd.Age)
		core.EngineStart(&run, clock)
		for _, outcome := range sd.Outcomes {
			clock = addSeconds(clock, sd.Age/float64(len(sd.Outcomes)+1))
			if err := complete(&run, outcome, storage, clock); err != nil {
				return nil, err
			}
		}
		// A run that reached its watch has the pull request its Ship agent opened.
		if stage := run.Stage(); stage != nil && stage.WatchTarget() != nil && *stage.WatchTarget() == core.WatchTargetPullRequest && run.Branch != nil {
			for _, repo := range run.Repos {
				number := 181 + len(pulls)
				failing := sd.Belt == "issues"
				checks := core.PullRequestCheckStatePassing
				failures := 0
				if failing {
					checks = core.PullRequestCheckStateFailing
					failures = 2
				}
				conflicted := sd.Belt == "nightly"
				pull := core.PullRequest{
					Number: number, URL: fmt.Sprintf("https://github.com/%s/pull/%d", repo, number), Repo: repo,
					State: core.PullRequestStateOpen, HeadSha: new(fmt.Sprintf("demo-%s-%d", repo, number)), BaseRef: new("main"),
					Checks: &checks, CheckFailures: new(failures), CheckTotal: new(14), Conflicted: &conflicted,
				}
				run.PullRequests = append(run.PullRequests, pull)
				pulls = append(pulls, PullRequestHead{Pull: pull, Head: *run.Branch})
			}
			run.FirstPullRequestAt = &jsonx.Date{Time: clock}
			// The nightly sweep comes back with conflicts, so the demo holds a run on a watch outcome that is not a merge.
			if sd.Belt == "nightly" && len(run.PullRequests) > 0 {
				detail := fmt.Sprintf("#%d has merge conflicts", run.PullRequests[0].Number)
				core.EngineAdvance(&run, core.Event{Kind: core.EventKindWatched, Watch: core.WatchEventConflicted, Detail: &detail}, now)
			}
		}
		// The epic's watch has seen its first wave.
		if stage := run.Stage(); stage != nil && stage.WatchTarget() != nil && *stage.WatchTarget() == core.WatchTargetTicket {
			run.TicketObservations[run.StageID] = core.TicketObservation{
				Status: "In Progress", Comments: map[string]struct{}{},
				Children: map[string]core.TicketObservationSeen{
					"PAY-41": {Status: "Done", IsOpen: false},
					"PAY-42": {Status: "In Progress", IsOpen: true},
					"PAY-43": {Status: "To Do", IsOpen: true},
				},
			}
		}
		if sd.Ref == "#152" {
			if err := pauseSeed(&run, storage, now, false); err != nil {
				return nil, err
			}
		}
		if sd.Ref == "#93" {
			if err := pauseSeed(&run, storage, now, true); err != nil {
				return nil, err
			}
		}
		if decision, ok := afkDecision(&run, now); ok {
			decisions = append(decisions, decision)
		}
		if sd.Ref == "#139" {
			coverSpend(&run)
		}
		if err := storage.SaveRun(run); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(decisions, func(i, j int) bool { return decisions[i].StartedAt.Before(decisions[j].StartedAt.Time) })
	if err := storage.SaveAfkDecisions(decisions); err != nil {
		return nil, err
	}
	return pulls, nil
}

// findSubject is the first subject queued on belt whose ref is ref.
func findSubject(subjects []demoSubject, belt, ref string) (core.Subject, bool) {
	for _, s := range subjects {
		if s.Belt == belt && s.Subject.Ref() == ref {
			return s.Subject, true
		}
	}
	return core.Subject{}, false
}

// addSeconds moves t by s seconds, which may be fractional,
// as a time interval does.
func addSeconds(t time.Time, s float64) time.Time {
	return t.Add(time.Duration(s * float64(time.Second)))
}
