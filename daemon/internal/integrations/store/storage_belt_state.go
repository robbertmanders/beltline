// BeltState, its decoding, and the pause fields Storage.storedState reads.

package store

import (
	"encoding/json"
	"time"

	"github.com/robbertmanders/beltline/daemon/internal/core"
	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// beltState is what a belt's state.json holds besides its versions.
// lastFired is omitted when nil; reading gives each field its fallback (see UnmarshalJSON).
type beltState struct {
	Paused    bool        `json:"paused"`
	Archived  bool        `json:"archived"`
	LastFired *jsonx.Date `json:"lastFired,omitzero"`
	// Schedules are per-entry schedule cursors. Empty when the file only has lastFired.
	Schedules      map[string]jsonx.Date `json:"schedules"`
	WatchOutcomes  bool                  `json:"watchOutcomes"`
	ReviewOutcomes bool                  `json:"reviewOutcomes"`
	// ShadowTests are the belt's shadow tests, the running one last. Absent before the first.
	ShadowTests []core.ShadowTest `json:"shadowTests,omitzero"`
}

// newBeltState is BeltState(paused:archived:lastFired:schedules:watchOutcomes:reviewOutcomes:).
func newBeltState(paused, archived bool, lastFired *jsonx.Date, schedules map[string]jsonx.Date, watchOutcomes, reviewOutcomes bool) beltState {
	if schedules == nil {
		schedules = map[string]jsonx.Date{}
	}
	return beltState{Paused: paused, Archived: archived, LastFired: lastFired, Schedules: schedules,
		WatchOutcomes: watchOutcomes, ReviewOutcomes: reviewOutcomes}
}

// UnmarshalJSON reads a state saved before watches saw more than a merge and
// a closure (no watchOutcomes), or before watches saw reviews (no
// reviewOutcomes): its belt still needs those outcomes, so each missing key
// reads as false.
func (s *beltState) UnmarshalJSON(data []byte) error {
	type plain beltState
	w := plain{Schedules: map[string]jsonx.Date{}}
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*s = beltState(w)
	if s.Schedules == nil {
		s.Schedules = map[string]jsonx.Date{}
	}
	return nil
}

// StoredBeltState is the pause and schedule fields currently on disk.
type StoredBeltState struct {
	Paused         bool
	Archived       bool
	LastFired      *time.Time
	Schedules      map[string]time.Time
	WatchOutcomes  bool
	ReviewOutcomes bool
	ShadowTests    []core.ShadowTest
}
