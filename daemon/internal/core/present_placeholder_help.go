//
// Read-only semantic references, using the same names and order as the editor
// chips. Callers resolve pinned versions (or run snapshots); this model never
// reads the library.

package core

import (
	"slices"
	"strings"
)

// PlaceholderHelpEntry is one placeholder name and what it means.
type PlaceholderHelpEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PlaceholderHelpProse is the explanatory prose the help shows around its
// entries, chosen by the daemon for the help's kind.
type PlaceholderHelpProse struct {
	Introduction    string `json:"introduction"`
	Resolution      string `json:"resolution"`
	Warnings        string `json:"warnings"`
	Syntax          string `json:"syntax"`
	Example         string `json:"example"`
	RenderedExample string `json:"renderedExample"`
	ExampleNote     string `json:"exampleNote"`
	// ResolutionNote is the note under the chips of a stage's placeholders
	// (StageAdjustResolutionNote, or its legacy form).
	ResolutionNote string `json:"resolutionNote"`
}

// PlaceholderHelpProseMake is the prose for the help's kind: legacy for a run saved
// before stage keys.
func PlaceholderHelpProseMake(legacy bool) PlaceholderHelpProse {
	resolution, warnings, resolutionNote := PlaceholderHelpResolution, PlaceholderHelpWarnings, StageAdjustResolutionNote
	if legacy {
		resolution, warnings, resolutionNote = PlaceholderHelpLegacyResolution, PlaceholderHelpLegacyWarnings, StageAdjustLegacyResolutionNote
	}
	return PlaceholderHelpProse{
		Introduction:    PlaceholderHelpIntroduction,
		Resolution:      resolution,
		Warnings:        warnings,
		Syntax:          PlaceholderHelpSyntax,
		Example:         PlaceholderHelpExample,
		RenderedExample: PlaceholderHelpRenderedExample,
		ExampleNote:     PlaceholderHelpExampleNote,
		ResolutionNote:  resolutionNote,
	}
}

// PlaceholderHelp is the read-only reference of the placeholders a brief can use:
// their entries in chip order, the qualification that applies, whether the help
// describes a legacy run, and the prose around them.
type PlaceholderHelp struct {
	Entries []PlaceholderHelpEntry `json:"entries"`
	// Qualification is the note on how the entries apply, when there is one.
	Qualification *string `json:"qualification,omitzero"`
	// Legacy is true for a run saved before stage keys. Its help still describes agent keys and stage ids.
	Legacy bool                 `json:"legacy"`
	Prose  PlaceholderHelpProse `json:"prose"`
}

// PlaceholderHelpFixed are the entries of the fixed placeholder names, with what
// each means, in BriefFixedPlaceholders order.
func PlaceholderHelpFixed() []PlaceholderHelpEntry {
	descriptions := map[string]string{
		"output":    "This attempt's output file path.",
		"base":      "The base branch name.",
		"branch":    "The run branch name.",
		"issue":     "A short subject reference and repository, not the body. Also covers pull requests, Jira tickets, manual and scheduled tasks, not just issues.",
		"previous":  "The latest finished output of another agent stage, or (no earlier output yet).",
		"artifacts": "This attempt's optional HTML artifact folder, outside the checkout. Using {artifacts} outside fenced code requests artifact instructions. Artifacts supplement Markdown output and its final Outcome: line. HTML can run scripts and contact the network when opened; self-contained authoring is not a sandbox.",
	}
	entries := make([]PlaceholderHelpEntry, 0, len(BriefFixedPlaceholders))
	for _, name := range BriefFixedPlaceholders {
		description, ok := descriptions[name]
		if !ok {
			panic("Missing fixed placeholder description: " + name)
		}
		entries = append(entries, PlaceholderHelpEntry{Name: name, Description: description})
	}
	return entries
}

// PlaceholderHelpAgent is the help for an agent brief: the fixed names only. Outputs
// other than its own are named by the placeholders chosen on the belt, so the
// library offers no key of its own.
func PlaceholderHelpAgent() PlaceholderHelp {
	fixedEntries := PlaceholderHelpFixed()
	entries := make([]PlaceholderHelpEntry, 0, len(BriefFixedPlaceholders))
	for _, name := range BriefFixedPlaceholders {
		for _, fixed := range fixedEntries {
			if fixed.Name != name {
				continue
			}
			description := fixed.Description
			if name == "output" {
				description += " Write this agent's current result here."
			}
			entries = append(entries, PlaceholderHelpEntry{Name: name, Description: description})
			break
		}
	}
	return PlaceholderHelp{Entries: entries, Qualification: nil, Legacy: false, Prose: PlaceholderHelpProseMake(false)}
}

// placeholderHelpLabels is the stage names a placeholder is written by, for a
// description.
func placeholderHelpLabels(stages []Stage) string {
	parts := make([]string, 0, len(stages))
	for _, stage := range stages {
		parts = append(parts, stage.Name+" (id: "+stage.ID+")")
	}
	return strings.Join(parts, ", ")
}

// placeholderHelpPlaces is where the stages that write a placeholder sit, by route.
func placeholderHelpPlaces(stages []Stage, route func(stage Stage) string) string {
	parts := make([]string, 0, len(stages))
	for _, stage := range stages {
		parts = append(parts, stage.Name+" on "+route(stage))
	}
	return strings.Join(parts, ", ")
}

// placeholderHelpDistinct is names without repeats, in their order.
func placeholderHelpDistinct(names []string) []string {
	var result []string
	for _, name := range names {
		if !slices.Contains(result, name) {
			result = append(result, name)
		}
	}
	return result
}

// Prose constants: the explanatory text the help shows.

// PlaceholderHelpIntroduction is the explanation of how placeholders work.
const PlaceholderHelpIntroduction = "Beltline replaces recognised {name} tokens when preparing an agent's brief. Output placeholders insert paths, not file contents. The issue or other subject, any note and the outcome footer are added automatically."

// PlaceholderHelpExample is the example brief.
const PlaceholderHelpExample = "Read {spec}. Write your result to {output}."

// PlaceholderHelpRenderedExample is the example with its paths filled in.
const PlaceholderHelpRenderedExample = "Read /run/02-spec-1/spec.md. Write your result to /run/03-build-1/build.md."

// PlaceholderHelpExampleNote is the note under the example.
const PlaceholderHelpExampleNote = "Illustrative only: the spec key and paths are examples, not exact storage paths. Not every belt has a Spec stage."

// PlaceholderHelpResolution says how a placeholder resolves on a belt.
const PlaceholderHelpResolution = "A stage's placeholder is the key chosen on that stage. Briefs use that key and the fixed names. A stage id or agent id is not another name for the output unless it is also the chosen key. A key used on mutually exclusive routes is the output of the route this run took, including after the routes join, and never another route's output. Going back into a fork keeps the outputs its stages already wrote until they write again. The current stage's own key is this attempt's output path. Another stage's key is that stage's latest finished output. With no finished output, the brief shows (no <filename> yet). When no route has selected one writer, it shows (no <key> output yet). These markers are not files in the checkout. Fixed names keep their own meanings and cannot be stage keys."

// PlaceholderHelpLegacyResolution is PlaceholderHelpResolution for a run saved before stage keys.
const PlaceholderHelpLegacyResolution = "An agent's Placeholder key refers to that agent's latest output on the run, across stages using that key. A stage id refers to that particular agent stage's latest finished output. The current stage's own key or id refers to this attempt's output path. Missing earlier output yields (no <output filename> yet). Agent keys win over stage ids; fixed names win over both, except legacy keys or ids named artifacts, which keep their old output meaning and do not request artifact instructions. New agent keys cannot reuse fixed names. Use distinct keys and ids."

// PlaceholderHelpWarnings says what the belt save checks.
const PlaceholderHelpWarnings = "Belt Save checks effective briefs: a stage override, otherwise its pinned agent's brief. It accepts fixed names and the placeholder keys chosen on this belt's stages. The same key may be used on fork routes that a run cannot take together. Unknown names are warned as text and keep their literal spelling. A belt saved before stages had their own keys converts missing keys from the originally pinned agents; a brief that used a stage id must be corrected to use the chosen key. Renaming a stage key does not rewrite brief tokens."

// PlaceholderHelpLegacyWarnings is PlaceholderHelpWarnings for a run saved before stage keys.
const PlaceholderHelpLegacyWarnings = "Belt Save checks effective briefs: a stage override, otherwise its pinned agent's brief. It accepts fixed names and keys/agent stage ids on that belt. Unknown names and ids shadowed by another stage's agent key block Save: fix the name or collision, or discard edits. Existing saved belts still load and run. This is a belt Save check, not a new restriction on agent editing or Adjust."

// PlaceholderHelpSyntax says which names the syntax check recognises.
const PlaceholderHelpSyntax = "The name check recognises nonempty lowercase letters, digits, - and _. {}, {Draft} and JSON-like braces are not names for that check. Fenced code is ignored by validation; inline code is checked. Fences do not escape rendering: recognised tokens are still replaced there. Fenced {artifacts} alone does not request instructions. Unknown tokens in an existing brief remain literal text."
