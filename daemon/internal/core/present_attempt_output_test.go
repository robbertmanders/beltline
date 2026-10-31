package core

import (
	"reflect"
	"testing"
)

// TestAttemptOutputIsNotApplicableToANonAgentStage: an approval writes no output, so it has no blocks or message.
func TestAttemptOutputIsNotApplicableToANonAgentStage(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "approve", Type: StageTypeApprove, Outcomes: []Outcome{}})
	attempt := NewAttempt(AttemptArgs{StageID: "approve", Number: 1, Folder: "first"})
	got := AttemptOutputPresent(stage, attempt, new("ignored"))
	if got.Availability != AttemptOutputNotApplicable || got.Message != nil || got.Blocks == nil || len(got.Blocks) != 0 {
		t.Fatalf("presented = %+v, want notApplicable with no blocks or message", got)
	}
}

// TestAttemptOutputParsesTheTextAfterStrippingOutcomeLines: text that exists is available, with its outcome line removed.
func TestAttemptOutputParsesTheTextAfterStrippingOutcomeLines(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "spec", Type: StageTypeAgent, Agent: &AgentRef{ID: "spec", Version: 1}, Key: new("spec"), Outcomes: []Outcome{}})
	attempt := NewAttempt(AttemptArgs{StageID: "spec", Number: 1, Folder: "first"})
	text := "# Plan\n\nDo the work.\n\nOutcome: complete"
	got := AttemptOutputPresent(stage, attempt, &text)
	if got.Availability != AttemptOutputAvailable || got.Message != nil {
		t.Fatalf("presented = %+v, want available with no message", got)
	}
	want := []MarkdownBlock{
		{Kind: MarkdownBlockKindHeading, Level: 1, Text: "Plan"},
		{Kind: MarkdownBlockKindParagraph, Text: "Do the work."},
	}
	if !reflect.DeepEqual(got.Blocks, want) {
		t.Fatalf("blocks = %+v, want %+v", got.Blocks, want)
	}
}

// TestAttemptOutputWithoutTextSaysWhatIsMissing: an open attempt is still working; a closed one names the file it wrote.
func TestAttemptOutputWithoutTextSaysWhatIsMissing(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "spec", Type: StageTypeAgent, Agent: &AgentRef{ID: "spec", Version: 1}, Key: new("spec"), Outcomes: []Outcome{}})
	open := NewAttempt(AttemptArgs{StageID: "spec", Number: 1, Folder: "first"})
	got := AttemptOutputPresent(stage, open, nil)
	if got.Availability != AttemptOutputMissing || got.Blocks == nil || got.Message == nil ||
		*got.Message != "The agent is working. Its output appears here when it is done." {
		t.Fatalf("open attempt presented = %+v", got)
	}
	closed := open
	ended := open.StartedAt
	closed.EndedAt = &ended
	got = AttemptOutputPresent(stage, closed, nil)
	if got.Message == nil || *got.Message != "This attempt wrote no spec.md." {
		t.Fatalf("closed attempt message = %+v", got.Message)
	}
}

// para and heading build the blocks the section tests compare.
func para(text string) MarkdownBlock {
	return MarkdownBlock{Kind: MarkdownBlockKindParagraph, Text: text}
}
func heading(level int, text string) MarkdownBlock {
	return MarkdownBlock{Kind: MarkdownBlockKindHeading, Level: level, Text: text}
}

// TestAttemptOutputSectionsSplitsEveryHeadingIntoAFlatSection covers the title, flat sections of
// any level, skipped levels, repeated labels, headings with no body and the empty-heading label.
func TestAttemptOutputSectionsSplitsEveryHeadingIntoAFlatSection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		blocks       []MarkdownBlock
		wantTitle    *MarkdownBlock
		wantSections []AttemptOutputSection
	}{
		{
			name: "title then level-2, level-3 and level-2 headings",
			blocks: []MarkdownBlock{
				heading(1, "Spec"), para("intro"), heading(2, "Goal"), para("goal body"),
				heading(3, "Detail"), para("detail body"), heading(2, "Behaviour"), para("behaviour body"),
			},
			wantTitle: new(heading(1, "Spec")),
			wantSections: []AttemptOutputSection{
				{ID: AttemptOutputPreambleID, Label: "Output", Blocks: []MarkdownBlock{para("intro")}},
				{ID: "heading:2:1:Goal", Label: "Goal", Level: 2, Blocks: []MarkdownBlock{para("goal body")}},
				{ID: "heading:3:1:Detail", Label: "Detail", Level: 3, Blocks: []MarkdownBlock{para("detail body")}},
				{ID: "heading:2:1:Behaviour", Label: "Behaviour", Level: 2, Blocks: []MarkdownBlock{para("behaviour body")}},
			},
		},
		{
			name: "a skipped level starts a section of its own",
			blocks: []MarkdownBlock{
				heading(2, "Goal"), para("a"), heading(4, "Deep"), para("b"),
			},
			wantSections: []AttemptOutputSection{
				{ID: "heading:2:1:Goal", Label: "Goal", Level: 2, Blocks: []MarkdownBlock{para("a")}},
				{ID: "heading:4:1:Deep", Label: "Deep", Level: 4, Blocks: []MarkdownBlock{para("b")}},
			},
		},
		{
			name: "repeated labels get separate occurrences, and a heading with no body is its own section",
			blocks: []MarkdownBlock{
				heading(2, "Notes"), para("first"), heading(2, "Notes"), heading(2, "Empty"), para("last"), heading(2, "Notes"),
			},
			wantSections: []AttemptOutputSection{
				{ID: "heading:2:1:Notes", Label: "Notes", Level: 2, Blocks: []MarkdownBlock{para("first")}},
				{ID: "heading:2:2:Notes", Label: "Notes", Level: 2, Blocks: []MarkdownBlock{}},
				{ID: "heading:2:1:Empty", Label: "Empty", Level: 2, Blocks: []MarkdownBlock{para("last")}},
				{ID: "heading:2:3:Notes", Label: "Notes", Level: 2, Blocks: []MarkdownBlock{}},
			},
		},
		{
			name: "a same-text heading at another level is a different section",
			blocks: []MarkdownBlock{
				heading(2, "Notes"), para("a"), heading(3, "Notes"), para("b"),
			},
			wantSections: []AttemptOutputSection{
				{ID: "heading:2:1:Notes", Label: "Notes", Level: 2, Blocks: []MarkdownBlock{para("a")}},
				{ID: "heading:3:1:Notes", Label: "Notes", Level: 3, Blocks: []MarkdownBlock{para("b")}},
			},
		},
		{
			name:      "an empty heading is labelled Output",
			blocks:    []MarkdownBlock{heading(1, "Title"), heading(2, ""), para("body")},
			wantTitle: new(heading(1, "Title")),
			wantSections: []AttemptOutputSection{
				{ID: "heading:2:1:", Label: "Output", Level: 2, Blocks: []MarkdownBlock{para("body")}},
			},
		},
		{
			name:   "a level-1 heading with no text is not a title",
			blocks: []MarkdownBlock{heading(1, ""), para("body")},
			wantSections: []AttemptOutputSection{
				{ID: "heading:1:1:", Label: "Output", Level: 1, Blocks: []MarkdownBlock{para("body")}},
			},
		},
		{
			name:      "a level-1 heading after the title is a section",
			blocks:    []MarkdownBlock{heading(1, "Title"), heading(1, "Second"), para("body")},
			wantTitle: new(heading(1, "Title")),
			wantSections: []AttemptOutputSection{
				{ID: "heading:1:1:Second", Label: "Second", Level: 1, Blocks: []MarkdownBlock{para("body")}},
			},
		},
		{
			name:         "output with no headings is one Output section",
			blocks:       []MarkdownBlock{para("one"), para("two")},
			wantSections: []AttemptOutputSection{{ID: AttemptOutputPreambleID, Label: "Output", Blocks: []MarkdownBlock{para("one"), para("two")}}},
		},
		{
			name:      "title and introductory text before a heading give an Output section before the first heading",
			blocks:    []MarkdownBlock{heading(1, "Plan"), para("intro"), heading(2, "Goal"), para("goal")},
			wantTitle: new(heading(1, "Plan")),
			wantSections: []AttemptOutputSection{
				{ID: AttemptOutputPreambleID, Label: "Output", Blocks: []MarkdownBlock{para("intro")}},
				{ID: "heading:2:1:Goal", Label: "Goal", Level: 2, Blocks: []MarkdownBlock{para("goal")}},
			},
		},
		{
			name:         "empty output has no title and no sections",
			blocks:       []MarkdownBlock{},
			wantSections: []AttemptOutputSection{},
		},
		{
			name:         "title-only output has the title and no sections",
			blocks:       []MarkdownBlock{heading(1, "Only a title")},
			wantTitle:    new(heading(1, "Only a title")),
			wantSections: []AttemptOutputSection{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			title, sections := AttemptOutputSections(test.blocks)
			if !reflect.DeepEqual(title, test.wantTitle) {
				t.Fatalf("title = %+v, want %+v", title, test.wantTitle)
			}
			if sections == nil || !reflect.DeepEqual(sections, test.wantSections) {
				t.Fatalf("sections = %+v, want %+v", sections, test.wantSections)
			}
		})
	}
}

// TestAttemptOutputSectionsKeepBodyOnlyChangesAndRenamesApart: body edits keep a section's ID, a rename
// gives it a new one.
func TestAttemptOutputSectionsKeepBodyOnlyChangesAndRenamesApart(t *testing.T) {
	t.Parallel()
	_, before := AttemptOutputSections([]MarkdownBlock{heading(2, "Goal"), para("old")})
	_, bodyEdit := AttemptOutputSections([]MarkdownBlock{heading(2, "Goal"), para("new"), para("more")})
	_, renamed := AttemptOutputSections([]MarkdownBlock{heading(2, "Aims"), para("old")})
	if before[0].ID != bodyEdit[0].ID {
		t.Fatalf("body edit changed the id: %q then %q", before[0].ID, bodyEdit[0].ID)
	}
	if before[0].ID == renamed[0].ID {
		t.Fatalf("rename kept the id %q", renamed[0].ID)
	}
}

// TestAttemptOutputPresentOrdersSectionsAfterStrippingAndHoisting: the outcome line is removed and
// Open questions is hoisted under the title before the sections are cut.
func TestAttemptOutputPresentOrdersSectionsAfterStrippingAndHoisting(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "spec", Type: StageTypeAgent, Agent: &AgentRef{ID: "spec", Version: 1}, Key: new("spec"), Outcomes: []Outcome{}})
	attempt := NewAttempt(AttemptArgs{StageID: "spec", Number: 1, Folder: "first"})
	text := "# Plan\n\n## Goal\n\nDo it.\n\n## Open questions\n\n- Which one?\n\n## Testing\n\nRun it.\n\nOutcome: complete"
	got := AttemptOutputPresent(stage, attempt, &text)
	if got.Title == nil || *got.Title != heading(1, "Plan") {
		t.Fatalf("title = %+v", got.Title)
	}
	var labels []string
	for _, section := range got.Sections {
		labels = append(labels, section.Label)
	}
	if want := []string{"Open questions", "Goal", "Testing"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	if !reflect.DeepEqual(got.Sections[1].Blocks, []MarkdownBlock{para("Do it.")}) {
		t.Fatalf("Goal blocks = %+v", got.Sections[1].Blocks)
	}
	// The flat list keeps the title, every section heading and every body, in the hoisted order.
	if len(got.Blocks) != 7 {
		t.Fatalf("flat blocks = %d, want 7 (title, three headings, three bodies)", len(got.Blocks))
	}
}

// TestAttemptOutputPresentHasNoSectionsWhenTheOutputIsMissing
func TestAttemptOutputPresentHasNoSectionsWhenTheOutputIsMissing(t *testing.T) {
	t.Parallel()
	stage := NewStage(StageArgs{ID: "spec", Type: StageTypeAgent, Agent: &AgentRef{ID: "spec", Version: 1}, Key: new("spec"), Outcomes: []Outcome{}})
	attempt := NewAttempt(AttemptArgs{StageID: "spec", Number: 1, Folder: "first"})
	got := AttemptOutputPresent(stage, attempt, nil)
	if got.Sections == nil || len(got.Sections) != 0 || got.Title != nil {
		t.Fatalf("missing output presented sections %+v title %+v", got.Sections, got.Title)
	}
}
