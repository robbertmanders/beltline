// The factory track's reads: the merged stage, belt and layer lookups,
// the runs at a merged stage and the short names a run's chip shows. The types are in
// present_factory_track.go; the build is in present_factory_track_build.go.

package core

import (
	"slices"
	"strings"
	"unicode"
)

// factoryTrackGenericWords are the words a short belt name does not take as its second word.
var factoryTrackGenericWords = map[string]struct{}{
	"belt": {}, "fixes": {}, "tasks": {}, "issues": {}, "requests": {},
	"to": {}, "of": {}, "the": {}, "for": {}, "and": {},
}

// Stage is the merged stage with id, or nil.
func (f FactoryTrack) Stage(id string) *FactoryTrackStage {
	for i := range f.Stages {
		if f.Stages[i].ID == id {
			merged := f.Stages[i]
			return &merged
		}
	}
	return nil
}

// Belt is the belt with id, or nil.
func (f FactoryTrack) Belt(id string) *FactoryTrackBelt {
	for i := range f.Belts {
		if f.Belts[i].ID == id {
			belt := f.Belts[i]
			return &belt
		}
	}
	return nil
}

// StageOf is the merged stage a belt's stage is part of, or nil.
func (f FactoryTrack) StageOf(stage, belt string) *FactoryTrackStage {
	for i := range f.Stages {
		for _, member := range f.Stages[i].Members {
			if member.StageID == stage && member.BeltID == belt {
				merged := f.Stages[i]
				return &merged
			}
		}
	}
	return nil
}

// LayerOf is the layer a node sits in, counted from the entrances; nil when it is not placed.
func (f FactoryTrack) LayerOf(node FactoryTrackNode) *int {
	for i, layer := range f.Layers {
		if slices.Contains(layer, node) {
			index := i
			return &index
		}
	}
	return nil
}

// RunsAt is the active runs at a merged stage.
func (f FactoryTrack) RunsAt(merged FactoryTrackStage, runs []Run) []Run {
	out := []Run{}
	for i := range runs {
		if runs[i].State.IsActive() && merged.Contains(runs[i]) {
			out = append(out, runs[i])
		}
	}
	return out
}

// factoryTrackKey is a stage's name as a key: trimmed of whitespace and lower-cased.
func factoryTrackKey(name string) string {
	return LowercasedFull(strings.TrimSpace(name))
}

// FactoryTrackShortName is a belt's name, short enough for a run's chip: its first word, or two
// when the first is short, leaving out words like "fixes".
func FactoryTrackShortName(beltName string) string {
	words := strings.FieldsFunc(beltName, unicode.IsSpace)
	if len(words) == 0 {
		return ""
	}
	first := words[0]
	if markdownCharacterCount(first) < factoryTrackShortNameMin && len(words) > 1 {
		if _, generic := factoryTrackGenericWords[LowercasedFull(words[1])]; !generic {
			return PrefixCharacters(first+" "+words[1], factoryTrackShortNameLimit)
		}
	}
	return PrefixCharacters(first, factoryTrackShortNameLimit)
}

// PrefixCharacters is the first n characters of s,
// counting characters, not runes.
func PrefixCharacters(s string, n int) string {
	chars := Characters(s)
	if len(chars) > n {
		chars = chars[:n]
	}
	return strings.Join(chars, "")
}
