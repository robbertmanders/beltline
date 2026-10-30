package core

import (
	"testing"
)

// TestParses: each way the outcome line can be written names the outcome.
func TestParses(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsOutcomesForAgentAt(SampleAgent("triage"), DefaultsPlace{})
	cases := []struct{ line, name string }{
		{"Outcome: ready to spec", "ready to spec"},
		{"**Outcome:** `needs info`", "needs info"},
		{"- Outcome: Ready To Implement.", "ready to implement"},
		{"Outcome: needs info — no reproduction steps", "needs info"},
		{"Outcome: <wait to implement>", "wait to implement"},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			t.Parallel()
			got := OutcomeParserParse("# Title\n\nReason.\n\n"+c.line+"\n", outcomes)
			if got == nil || got.Name != c.name {
				t.Errorf("parse %q = %+v, want %q", c.line, got, c.name)
			}
		})
	}
}

// TestTheLastOutcomeLineWins: when several outcome lines appear, the last one decides.
func TestTheLastOutcomeLineWins(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsOutcomesForAgentAt(SampleAgent("triage"), DefaultsPlace{})
	text := "Outcome: needs info\nThen I read more.\nOutcome: ready to spec"
	if got := OutcomeParserParse(text, outcomes); got == nil || got.Name != "ready to spec" {
		t.Errorf("parse = %+v, want ready to spec", got)
	}
}

// TestAnUnknownLastLineIsNoOutcome: a last outcome line that names no outcome, or no
// outcome line at all, parses to nothing.
func TestAnUnknownLastLineIsNoOutcome(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsOutcomesForAgentAt(SampleAgent("triage"), DefaultsPlace{})
	if got := OutcomeParserParse("Outcome: ready to spec\nOutcome: maybe", outcomes); got != nil {
		t.Errorf("parse = %+v, want nil", got)
	}
	if got := OutcomeParserParse("No verdict here.", outcomes); got != nil {
		t.Errorf("parse = %+v, want nil", got)
	}
}

// TestANameIsNotMatchedInsideALongerWord: "pass" does not match "passable".
func TestANameIsNotMatchedInsideALongerWord(t *testing.T) {
	t.Parallel()
	verify := DefaultsOutcomesForAgentAt(SampleAgent("verify"), DefaultsPlace{})
	if got := OutcomeParserParse("Outcome: passable", verify); got != nil {
		t.Errorf("parse = %+v, want nil", got)
	}
}

// TestAReplyCompletesAnOutputWithoutAnOutcomeLine: a reply that gives the outcome
// completes the output; a reply that does not gives nothing.
func TestAReplyCompletesAnOutputWithoutAnOutcomeLine(t *testing.T) {
	t.Parallel()
	outcomes := DefaultsOutcomesForAgentAt(SampleAgent("triage"), DefaultsPlace{})
	reply := "Wrote triage.md.\n\n**Outcome:** ready to spec"
	if got := OutcomeParserCompleting("# Title\n\nReason.\n", reply, outcomes); got == nil || *got != "# Title\n\nReason.\n\n**Outcome:** ready to spec\n" {
		t.Errorf("completing = %v", engineStr(got))
	}
	if got := OutcomeParserCompleting("# Title\n\nOutcome: maybe", reply, outcomes); got != nil {
		t.Errorf("completing with unknown output = %q, want nil", *got)
	}
	if got := OutcomeParserCompleting("# Title", "Outcome: maybe", outcomes); got != nil {
		t.Errorf("completing with unknown reply = %q, want nil", *got)
	}
	if got := OutcomeParserCompleting("# Title", "Done.", outcomes); got != nil {
		t.Errorf("completing with no outcome = %q, want nil", *got)
	}
}

// TestStripRemovesTheOutcomeLine: stripping leaves the text before the outcome line.
func TestStripRemovesTheOutcomeLine(t *testing.T) {
	t.Parallel()
	if got := OutcomeParserStrip("Reason.\n\nOutcome: pass"); got != "Reason." {
		t.Errorf("strip = %q, want Reason.", got)
	}
}

// TestANameFollowedByAMarkIsNotMatched: names compare whole characters, so a
// name followed by a combining mark, a zero-width joiner or a variation selector
// does not start a word of its own: "ready" plus U+0301 is "ready" with an accent.
func TestANameFollowedByAMarkIsNotMatched(t *testing.T) {
	t.Parallel()
	outcomes := []Outcome{{ID: "r", Name: "ready"}}
	for _, mark := range []string{"́", "̈", "⃣", "\u200d", "️"} {
		text := "Outcome: ready" + mark
		if got := OutcomeParserParse(text, outcomes); got != nil {
			t.Errorf("%U after the name: parse = %+v, want nil", []rune(mark)[0], got)
		}
	}
	for _, text := range []string{"Outcome: ready.", "Outcome: ready now", "Outcome: ready!"} {
		if got := OutcomeParserParse(text, outcomes); got == nil || got.Name != "ready" {
			t.Errorf("%q: parse = %+v, want ready", text, got)
		}
	}
}

// TestCharacterCountCountsMarksWithTheirBase: the tie-break counts characters,
// so a base rune and the marks that extend it count once.
func TestCharacterCountCountsMarksWithTheirBase(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"ready":   5,
		"readý":  5,
		"ée":     2,
		"a\u200d": 1,
		"❤️":      1,
		"":        0,
	}
	for name, want := range cases {
		if got := outcomeParserCharacterCount(name); got != want {
			t.Errorf("outcomeParserCharacterCount(%q) = %d, want %d", name, got, want)
		}
	}
}
