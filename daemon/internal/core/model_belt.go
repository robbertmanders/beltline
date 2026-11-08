// Policies are in
// model_belt_policies.go, StageKind, Workspace, Fallback and AfkMode in
// model_belt_enums.go; Stage, StageType and Backend are in model_belt_stage.go.

package core

import (
	"encoding/json"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
	"strconv"
	"strings"
)

// Belt is stages in order, fed by one or more triggers, saved as numbered
// versions.
//
// The keys are id, name, version, triggers, base, policies, stages, forks and afk.
// Decoding also accepts the legacy keys trigger, repos and afkEnabled (see
// UnmarshalJSON). A request body must give id, name, version, triggers and stages
// (required:"true", checked by the request walk, not by data-file loading).
type Belt struct {
	ID   string `json:"id" required:"true"`
	Name string `json:"name" required:"true"`
	// Version is the belt's saved version number.
	Version int `json:"version" required:"true"`
	// Triggers is where its work comes from, in check order. Each entry keeps
	// its own repositories and kind settings. Repeated kinds are allowed.
	Triggers []TriggerEntry `json:"triggers" required:"true"`
	// Base is the branch work starts from; empty means each repository's default.
	Base string `json:"base"`
	// RepositoryBases overrides Base in each repository, by "owner/repository".
	// Empty overrides are not kept; an override needs a trigger entry that lists its repository.
	RepositoryBases map[string]string `json:"repositoryBases,omitzero"`
	// Policies apply to every run of the belt.
	Policies Policies `json:"policies"`
	// Stages are the belt's stages, in order.
	Stages []Stage `json:"stages" required:"true"`
	// Forks is where the stages split into routes; each stage on one names its route.
	Forks []Fork `json:"forks"`
	// Afk says whether its new runs start in AFK mode: always, never, or as the
	// global switch says. Runs already going keep their own.
	Afk AfkMode `json:"afk"`
	// Origin is the library repository this belt was installed from; nil for your own.
	Origin *LibraryOrigin `json:"origin,omitzero"`
	// OriginID is the id Origin's repository has for this belt, when its id here had to differ.
	OriginID string `json:"originId,omitzero"`
}

// MarshalJSON writes the belt with its struct tags, its repository overrides cleaned.
func (b Belt) MarshalJSON() ([]byte, error) {
	type plain Belt
	b.RepositoryBases = RepositoryBasesClean(b.RepositoryBases)
	return jsonx.Marshal(plain(b))
}

// UnmarshalJSON reads a belt. A belt saved with one trigger keeps its
// repositories, base and policies on that trigger, and one saved before afk
// has the afkEnabled boolean. Missing trigger ids are numbered after the ids
// in use, and stages have their keys followed.
func (b *Belt) UnmarshalJSON(data []byte) error {
	type plain Belt
	f, err := codecFieldsOf(data)
	if err != nil {
		return err
	}
	w := plain{Forks: []Fork{}, Policies: DefaultPolicies()}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*b = Belt(w)
	if f.has("triggers") {
		numberTriggerIDs(b.Triggers)
	} else if err := b.decodeLegacyTrigger(f); err != nil {
		return err
	}
	for i := range b.Stages {
		b.Stages[i] = b.Stages[i].followingKey()
	}
	if !f.present("afk") {
		b.Afk = AfkModeGlobal
		var enabled bool
		if f.present("afkEnabled") {
			if err := json.Unmarshal(f["afkEnabled"], &enabled); err != nil {
				return err
			}
		}
		if enabled {
			b.Afk = AfkModeAlways
		}
	}
	return nil
}

// numberTriggerIDs gives each entry saved without an id the first number after
// the ids already used, counting from its position.
func numberTriggerIDs(entries []TriggerEntry) {
	used := map[string]struct{}{}
	for _, e := range entries {
		if e.ID != "" {
			used[e.ID] = struct{}{}
		}
	}
	for index := range entries {
		if entries[index].ID != "" {
			continue
		}
		n := index + 1
		for {
			if _, taken := used[strconv.Itoa(n)]; !taken {
				break
			}
			n++
		}
		entries[index].ID = strconv.Itoa(n)
		used[entries[index].ID] = struct{}{}
	}
}

// decodeLegacyTrigger reads a belt saved with one trigger. The trigger object
// holds its repositories, base and policies, which become the single entry
// TriggerEntryLegacyID; a key at the belt's top level wins over the nested one.
func (b *Belt) decodeLegacyTrigger(f codecFields) error {
	if !f.has("trigger") {
		b.Triggers = []TriggerEntry{}
		return nil
	}
	var trigger Trigger
	if err := json.Unmarshal(f["trigger"], &trigger); err != nil {
		return err
	}
	var legacy legacyTrigger
	if err := json.Unmarshal(f["trigger"], &legacy); err != nil {
		return err
	}
	repos := legacy.Repos
	if f.present("repos") {
		repos = nil
		if err := json.Unmarshal(f["repos"], &repos); err != nil {
			return err
		}
	}
	if repos == nil {
		repos = []string{}
	}
	b.Triggers = []TriggerEntry{{ID: TriggerEntryLegacyID, Trigger: trigger, Repos: repos}}
	b.Base = ""
	if legacy.Base != nil {
		b.Base = *legacy.Base
	}
	if f.present("base") {
		if err := json.Unmarshal(f["base"], &b.Base); err != nil {
			return err
		}
	}
	b.Policies = legacy.Policies()
	if f.present("policies") {
		return json.Unmarshal(f["policies"], &b.Policies)
	}
	return nil
}

// NewBeltFromTrigger creates a belt with one trigger entry, the legacy form of
// Belt.init(id:name:version:trigger:repos:...). It wraps the trigger in an
// entry with TriggerEntryLegacyID.
// The defaults are version 1, base "", DefaultPolicies() and AfkModeGlobal;
// callers set them when they differ from the Go zero values.
func NewBeltFromTrigger(a BeltFromTriggerArgs) Belt {
	version := ValueOr(a.Version, 1)
	policies := ValueOr(a.Policies, DefaultPolicies())
	afk := ValueOr(a.Afk, AfkModeGlobal)
	return Belt{
		ID:       a.ID,
		Name:     a.Name,
		Version:  version,
		Triggers: []TriggerEntry{{ID: TriggerEntryLegacyID, Trigger: a.Trigger.Clone(), Repos: append([]string{}, a.Repos...)}},
		Base:     a.Base,
		Policies: policies,
		Stages:   append([]Stage{}, cloneEach(a.Stages, Stage.Clone)...),
		Forks:    append([]Fork{}, cloneEach(a.Forks, Fork.Clone)...),
		Afk:      afk,
	}
}

// BeltFromTriggerArgs are the parameters of NewBeltFromTrigger.
type BeltFromTriggerArgs struct {
	ID      string
	Name    string
	Version *int
	Trigger Trigger
	Repos   []string
	Base    string
	// Policies is nil for DefaultPolicies().
	Policies *Policies
	Stages   []Stage
	Forks    []Fork
	// Afk is nil for AfkModeGlobal.
	Afk *AfkMode
}

// Stage returns the stage with id, or nil.
func (b Belt) Stage(id string) *Stage {
	for i := range b.Stages {
		if b.Stages[i].ID == id {
			s := b.Stages[i].Clone()
			return &s
		}
	}
	return nil
}

// Trigger is the first entry's trigger, nil-safe: the default trigger
// (IssueFilter with no fields) when there are no entries.
func (b Belt) Trigger() Trigger {
	if len(b.Triggers) == 0 {
		return TriggerKindIssues.Fresh()
	}
	return b.Triggers[0].Trigger.Clone()
}

// SetTrigger changes the first entry's trigger and leaves the others. It
// creates the first entry when there is none.
func (b *Belt) SetTrigger(t Trigger) {
	b.updateFirst(func(e *TriggerEntry) { e.Trigger = t.Clone() })
}

// Repos is the first entry's repositories.
func (b Belt) Repos() []string {
	if len(b.Triggers) == 0 {
		return []string{}
	}
	return append([]string{}, b.Triggers[0].Repos...)
}

// SetRepos changes the first entry's repositories and leaves the others.
func (b *Belt) SetRepos(repos []string) {
	b.updateFirst(func(e *TriggerEntry) { e.Repos = append([]string{}, repos...) })
}

// updateFirst applies change to the first entry, creating it when the belt has
// none.
func (b *Belt) updateFirst(change func(*TriggerEntry)) {
	entries := cloneEach(b.Triggers, TriggerEntry.Clone)
	if len(entries) == 0 {
		e := TriggerEntry{ID: b.NextTriggerID(), Trigger: TriggerKindIssues.Fresh(), Repos: []string{}}
		change(&e)
		entries = []TriggerEntry{e}
	} else {
		change(&entries[0])
	}
	b.Triggers = entries
}

// Summary is one line about where the belt's work comes from. Several entries
// are all named.
func (b Belt) Summary() string {
	switch len(b.Triggers) {
	case 0:
		return "Add at least one trigger"
	case 1:
		return b.Triggers[0].Line()
	}
	parts := make([]string, len(b.Triggers))
	for i, e := range b.Triggers {
		parts[i] = e.Trigger.Kind.Title() + ": " + e.Trigger.Summary()
	}
	return strings.Join(parts, " · ")
}

// DefaultBelt is the belt with the given id, name, triggers and stages, built with
// its defaults: version 1, no base, DefaultPolicies(), no forks, and AfkModeGlobal.
func DefaultBelt(id, name string, triggers []TriggerEntry, stages []Stage) Belt {
	return Belt{ID: id, Name: name, Version: 1, Triggers: triggers, Policies: DefaultPolicies(), Stages: stages, Afk: AfkModeGlobal}
}

// Clone is a deep copy of the belt: its trigger entries, stages, forks, policies
// and every list in them, so editing the copy never changes the belt.
func (b Belt) Clone() Belt {
	return cloneValue(b)
}
