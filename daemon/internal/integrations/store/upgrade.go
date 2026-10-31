// Storage.Upgrade: the one-time rewrite of a data folder's settings, belts,
// belt states and runs into the current format, saving the agents they pin. The rules for each value are core's
// (engine_upgrade.go); this walks the folder, keeps the originals and writes.

package store

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// UpgradeReport is what an upgrade did. Ran is false when the folder was
// already in the current format.
type UpgradeReport struct {
	Ran bool
	// Settings is set when settings.json was rewritten.
	Settings bool
	// Belts, States and Runs count the files rewritten.
	Belts, States, Runs int
	// Agents counts the agent versions saved that belts or runs pinned without
	// agents/ holding them (core.UpgradeSampleAgent).
	Agents int
	// Unreadable are the files, relative to the data folder, that did not
	// decode and were left as they are.
	Unreadable []string
	// Backup is the folder holding the rewritten files as they were, or empty
	// when none changed.
	Backup string
}

// NewerFormat is the refusal to open a data folder that a newer Beltline saved: its format.json
// names a format this version does not know, so reading or rewriting it could lose data.
type NewerFormat struct {
	Folder  string
	Found   int
	Current int
}

// Error says which format the folder has and what to do.
// Lock: none (pure).
func (f NewerFormat) Error() string {
	return fmt.Sprintf("The data folder %s is in format %d, saved by a newer Beltline; this version reads format %d. "+
		"Install the newer beltlined again, or point BELTLINE_DATA_DIR at another folder.", f.Folder, f.Found, f.Current)
}

// upgradeFormat is format.json.
type upgradeFormat struct {
	Version int `json:"version"`
}

// Upgrade rewrites the settings and every belt version, belt state and run
// that an older version saved into the current format, once, and saves each
// agent version they pin that an older version read from the app itself:
// format.json records that it ran, and a folder already at core.UpgradeFormatVersion is left alone. Each
// file whose bytes change is first copied, as it was, below
// upgrade-backups/<now>/ at its path in the folder. A file that does not
// decode is left as it is and reported. A failed write stops the upgrade
// before format.json is written, so the next start tries again. A folder at a
// newer format than this version's is refused with NewerFormat and left untouched.
// Lock: the caller owns the data folder.
func (s Storage) Upgrade(now time.Time) (UpgradeReport, error) {
	var format upgradeFormat
	if s.Read(s.Paths.Format(), &format) == nil {
		if format.Version > core.UpgradeFormatVersion {
			return UpgradeReport{}, NewerFormat{Folder: s.Paths.Root, Found: format.Version, Current: core.UpgradeFormatVersion}
		}
		if format.Version == core.UpgradeFormatVersion {
			return UpgradeReport{}, nil
		}
	}
	u := upgrader{storage: s, backup: filepath.Join(s.Paths.UpgradeBackups(), now.UTC().Format("20060102T150405Z")),
		agents: map[core.AgentRef]*core.Agent{}, samples: map[core.AgentRef]core.Agent{},
		belts: map[string]map[int]core.Belt{}, latest: map[string]core.Belt{}}
	if err := u.upgradeSettings(); err != nil {
		return u.report, err
	}
	if err := u.upgradeBelts(); err != nil {
		return u.report, err
	}
	if err := u.upgradeRuns(); err != nil {
		return u.report, err
	}
	if err := u.saveSamples(); err != nil {
		return u.report, err
	}
	if err := s.Write(upgradeFormat{Version: core.UpgradeFormatVersion}, s.Paths.Format()); err != nil {
		return u.report, err
	}
	u.report.Ran = true
	return u.report, nil
}

// upgrader is one upgrade's walk: the agents and belt versions read so far,
// and what it has done.
type upgrader struct {
	storage Storage
	backup  string
	agents  map[core.AgentRef]*core.Agent
	// samples are the pinned agents agents/ lacks that the upgrade saves.
	samples map[core.AgentRef]core.Agent
	// belts are the versions read, by belt id and version; latest is each belt's newest.
	belts  map[string]map[int]core.Belt
	latest map[string]core.Belt
	report UpgradeReport
}

// upgradeSettings upgrades settings.json: an old factory and marketplace
// repository become library repositories, and the examples repository is
// listed. A folder without settings.json gets one, with the default settings
// and the examples repository.
func (u *upgrader) upgradeSettings() error {
	path := u.storage.Paths.Settings()
	original, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		settings := DefaultSettings()
		settings.LibraryRepositories = core.UpgradeLibraryRepositories(settings.LibraryRepositories)
		if err := u.storage.SaveSettings(settings); err != nil {
			return err
		}
		u.report.Settings = true
		return nil
	}
	if err != nil {
		return nil
	}
	var settings Settings
	if jsonx.Unmarshal(original, &settings) != nil {
		u.unreadable(path)
		return nil
	}
	settings.LibraryRepositories = core.UpgradeLibraryRepositories(settings.LibraryRepositories)
	if !settingsHasKey(original, "enableShadowTests") {
		// Shadow tests were always on, so a folder with a test still running keeps it enabled.
		settings.EnableShadowTests = u.runningShadowTest()
	}
	u.report.Settings, err = u.rewrite(path, original, settings)
	return err
}

// settingsHasKey reports whether the settings file original has key at its top level.
func settingsHasKey(original []byte, key string) bool {
	var members map[string]json.RawMessage
	if json.Unmarshal(original, &members) != nil {
		return false
	}
	_, ok := members[key]
	return ok
}

// runningShadowTest reports whether any belt's state.json holds a shadow test that has not
// ended, whether its belt is live, paused or archived. It reads the states before the belts are upgraded.
func (u *upgrader) runningShadowTest() bool {
	for _, folder := range subfolders(u.storage.Paths.Belts()) {
		var state beltState
		if u.storage.Read(filepath.Join(folder, "state.json"), &state) == nil && core.ShadowRunning(state.ShadowTests) != nil {
			return true
		}
	}
	return false
}

// upgradeBelts upgrades every version of every belt, then its state.
func (u *upgrader) upgradeBelts() error {
	for _, folder := range subfolders(u.storage.Paths.Belts()) {
		id := filepath.Base(folder)
		u.belts[id] = map[int]core.Belt{}
		all := versions(folder, "json")
		for _, version := range all {
			path := filepath.Join(folder, "v"+strconv.Itoa(version)+".json")
			original, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var belt core.Belt
			if jsonx.Unmarshal(original, &belt) != nil {
				u.unreadable(path)
				continue
			}
			upgraded := core.UpgradeBelt(belt, u.agentsOf(belt.Stages))
			u.belts[id][version] = upgraded
			u.latest[id] = upgraded
			changed, err := u.rewrite(path, original, upgraded)
			if err != nil {
				return err
			}
			if changed {
				u.report.Belts++
			}
		}
		u.readShadows(folder)
		if err := u.upgradeState(folder, id); err != nil {
			return err
		}
	}
	return nil
}

// readShadows reads the belt's shadow versions for the agents they pin, which the
// upgrade saves when agents/ lacks them. The shadow files themselves stay as they are.
func (u *upgrader) readShadows(folder string) {
	shadows := filepath.Join(folder, "shadow")
	for _, version := range versions(shadows, "json") {
		var belt core.Belt
		if u.storage.Read(filepath.Join(shadows, "v"+strconv.Itoa(version)+".json"), &belt) == nil {
			u.agentsOf(belt.Stages)
		}
	}
}

// upgradeState upgrades a belt's state.json: a single lastFired becomes the cursor of
// the belt's schedule entry.
func (u *upgrader) upgradeState(folder, id string) error {
	path := filepath.Join(folder, "state.json")
	original, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state beltState
	if jsonx.Unmarshal(original, &state) != nil {
		u.unreadable(path)
		return nil
	}
	if len(state.Schedules) == 0 && state.LastFired != nil {
		if belt, ok := u.latest[id]; ok {
			state.Schedules = map[string]jsonx.Date{scheduleEntryID(belt): *state.LastFired}
		}
	}
	changed, err := u.rewrite(path, original, state)
	if changed {
		u.report.States++
	}
	return err
}

// upgradeRuns upgrades every run.json below runs/, except a run being erased.
func (u *upgrader) upgradeRuns() error {
	erasing := map[string]struct{}{}
	for _, pending := range u.storage.LoadPendingErasures() {
		erasing[pending.ID] = struct{}{}
	}
	var paths []string
	_ = filepath.WalkDir(u.storage.Paths.Runs(), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "run.json" {
			paths = append(paths, path)
		}
		return nil
	})
	for _, path := range paths {
		original, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var run core.Run
		if jsonx.Unmarshal(original, &run) != nil {
			u.unreadable(path)
			continue
		}
		if _, ok := erasing[run.ID]; ok {
			continue
		}
		var startedOn *core.Belt
		if belt, ok := u.belts[run.BeltID][run.BeltVersion]; ok {
			startedOn = &belt
		} else if belt, ok := u.latest[run.BeltID]; ok {
			startedOn = &belt
		}
		upgraded := core.UpgradeRun(run, u.agentsOf(run.Stages), startedOn)
		changed, err := u.rewrite(path, original, upgraded)
		if err != nil {
			return err
		}
		if changed {
			u.report.Runs++
		}
	}
	return nil
}

// agentsOf are the library's versions of the agents stages pin, those it has,
// and those an older version read from the app itself, which saveSamples saves.
func (u *upgrader) agentsOf(stages []core.Stage) map[core.AgentRef]core.Agent {
	out := map[core.AgentRef]core.Agent{}
	for _, stage := range stages {
		if stage.Kind() != core.StageKindAgent || stage.Agent == nil {
			continue
		}
		ref := *stage.Agent
		agent, seen := u.agents[ref]
		if !seen {
			if found, ok := u.storage.AgentRef(ref); ok {
				agent = &found
			} else if sample, ok := core.UpgradeSampleAgent(ref); ok {
				agent = &sample
				u.samples[ref] = sample
			}
			u.agents[ref] = agent
		}
		if agent != nil {
			out[ref] = *agent
		}
	}
	return out
}

// saveSamples saves each pinned agent version that agents/ lacked, as
// agents/<id>/v<n>.md, in id and version order, with the text the app read it from. A file that appeared since it was
// read is left as it is.
func (u *upgrader) saveSamples() error {
	refs := slices.SortedFunc(maps.Keys(u.samples), func(a, b core.AgentRef) int {
		return cmp.Or(strings.Compare(a.ID, b.ID), cmp.Compare(a.Version, b.Version))
	})
	for _, ref := range refs {
		path := filepath.Join(u.storage.agentFolder(ref.ID), "v"+strconv.Itoa(ref.Version)+".md")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
			return err
		}
		text, ok := core.SampleAgentText(ref.ID)
		if !ok {
			text = core.AgentFileWrite(u.samples[ref])
		}
		if err := WriteFileAtomic(path, []byte(text)); err != nil {
			return err
		}
		u.report.Agents++
	}
	return nil
}

// rewrite writes value at path when its current encoding differs from
// original, first copying original into the backup folder. It reports
// whether it wrote.
func (u *upgrader) rewrite(path string, original []byte, value any) (bool, error) {
	data, err := jsonx.MarshalStorage(value)
	if err != nil {
		return false, err
	}
	if bytes.Equal(data, original) {
		return false, nil
	}
	relative, err := filepath.Rel(u.storage.Paths.Root, path)
	if err != nil {
		return false, err
	}
	copied := filepath.Join(u.backup, relative)
	if err := os.MkdirAll(filepath.Dir(copied), 0o777); err != nil {
		return false, err
	}
	if err := WriteFileAtomic(copied, original); err != nil {
		return false, err
	}
	u.report.Backup = u.backup
	return true, WriteFileAtomic(path, data)
}

// unreadable records a file left as it is.
func (u *upgrader) unreadable(path string) {
	relative, err := filepath.Rel(u.storage.Paths.Root, path)
	if err != nil {
		relative = path
	}
	u.report.Unreadable = append(u.report.Unreadable, relative)
}
