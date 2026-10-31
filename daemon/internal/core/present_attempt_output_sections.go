// The sections of an agent attempt's output: the document title, then the blocks under each
// heading. The run page folds each section on its own, so the daemon decides where they begin and end.

package core

import "strconv"

// AttemptOutputPreambleID identifies the section of output that comes before the first heading
// (after any title). Heading sections always start with "heading:", so the two never collide.
const AttemptOutputPreambleID = "preamble"

// AttemptOutputPreambleLabel labels the preamble section, and any heading whose text is empty.
const AttemptOutputPreambleLabel = "Output"

// AttemptOutputSection is one section of an agent's output: a heading and the blocks up to the
// next heading, whatever its level, or the preamble before the first heading. Label is the
// heading's text. Level is the heading's level, and 0 for the preamble. ID stays the same when
// only the body changes: it is the level, the exact heading text and the occurrence among the
// headings with that level and text.
type AttemptOutputSection struct {
	ID     string          `json:"id"`
	Label  string          `json:"label"`
	Level  int             `json:"level"`
	Blocks []MarkdownBlock `json:"blocks"`
}

// AttemptOutputSections splits presented blocks into the document title and the sections. The
// title is an initial level-1 heading with text; it is not a section. Blocks before the first
// heading form the preamble section, and an output with no headings is that one section. A
// heading with no text is labelled AttemptOutputPreambleLabel. Output with nothing after the title
// has no sections.
func AttemptOutputSections(blocks []MarkdownBlock) (*MarkdownBlock, []AttemptOutputSection) {
	var title *MarkdownBlock
	rest := blocks
	if len(rest) > 0 && rest[0].Kind == MarkdownBlockKindHeading && rest[0].Level == 1 && rest[0].Text != "" {
		first := rest[0]
		title = &first
		rest = rest[1:]
	}
	sections := []AttemptOutputSection{}
	occurrences := map[string]int{}
	for _, block := range rest {
		if block.Kind == MarkdownBlockKindHeading {
			key := strconv.Itoa(block.Level) + "\x00" + block.Text
			occurrences[key]++
			label := block.Text
			if label == "" {
				label = AttemptOutputPreambleLabel
			}
			id := "heading:" + strconv.Itoa(block.Level) + ":" + strconv.Itoa(occurrences[key]) + ":" + block.Text
			sections = append(sections, AttemptOutputSection{ID: id, Label: label, Level: block.Level, Blocks: []MarkdownBlock{}})
			continue
		}
		if len(sections) == 0 {
			sections = append(sections, AttemptOutputSection{ID: AttemptOutputPreambleID, Label: AttemptOutputPreambleLabel, Blocks: []MarkdownBlock{}})
		}
		last := &sections[len(sections)-1]
		last.Blocks = append(last.Blocks, block)
	}
	return title, sections
}
