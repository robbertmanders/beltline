//
// The blocks of the Markdown agents write: headings, list items, code blocks
// and paragraphs. The run page draws them at the width it has. The daemon sends
// the parsed blocks; no Markdown library is used.
//
// Characters are grapheme clusters, compared with
// canonical equivalence. Go has no grapheme segmentation in its standard
// library, so Characters approximates it: combining marks, joiners,
// variation selectors, skin tones, tags, regional-indicator pairs and CRLF
// stay with the character before them.

package core

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// MarkdownBlockKind is the kind of a parsed block.
type MarkdownBlockKind string

const (
	MarkdownBlockKindHeading   MarkdownBlockKind = "heading"
	MarkdownBlockKindItem      MarkdownBlockKind = "item"
	MarkdownBlockKindCode      MarkdownBlockKind = "code"
	MarkdownBlockKindParagraph MarkdownBlockKind = "paragraph"
)

// MarkdownBlock is one block of agent Markdown. Level is set for a heading, Marker for an
// item (its bullet or number) and Text for every kind. It is comparable with ==.
//
// Encoding: each kind writes its own keys: heading {type, level, text}, item {type, marker,
// text}, code and paragraph {type, text}.
type MarkdownBlock struct {
	Kind   MarkdownBlockKind
	Level  int
	Marker string
	Text   string
}

// MarshalJSON writes the block's type and the keys its kind has.
func (b MarkdownBlock) MarshalJSON() ([]byte, error) {
	switch b.Kind {
	case MarkdownBlockKindHeading:
		return jsonx.Marshal(struct {
			Type  MarkdownBlockKind `json:"type"`
			Level int               `json:"level"`
			Text  string            `json:"text"`
		}{b.Kind, b.Level, b.Text})
	case MarkdownBlockKindItem:
		return jsonx.Marshal(struct {
			Type   MarkdownBlockKind `json:"type"`
			Marker string            `json:"marker"`
			Text   string            `json:"text"`
		}{b.Kind, b.Marker, b.Text})
	default:
		return jsonx.Marshal(struct {
			Type MarkdownBlockKind `json:"type"`
			Text string            `json:"text"`
		}{b.Kind, b.Text})
	}
}

// Copy and syntax used by MarkdownBlock. Literal text is exact.
const (
	// markdownBlockWrapColumn is the column below which lines were written one by one, not wrapped.
	markdownBlockWrapColumn = 60
	markdownBlockHeading    = "#"
	markdownBlockDash       = "- "
	markdownBlockStar       = "* "
	markdownBlockSpace      = " "
	markdownBlockBackslash  = "\\"
)

// MarkdownBlockParse parses agent Markdown into blocks.
func MarkdownBlockParse(text string) []MarkdownBlock {
	var blocks []MarkdownBlock
	// The paragraph or list item being read: its marker, if an item, and its lines as written.
	var marker *string
	var lines []string
	// code is the open fenced block's lines; inCode says whether one is open.
	var code []string
	inCode := false
	flush := func() {
		if len(lines) > 0 {
			text := markdownBlockReflow(lines)
			if marker != nil {
				blocks = append(blocks, MarkdownBlock{Kind: MarkdownBlockKindItem, Marker: *marker, Text: text})
			} else {
				blocks = append(blocks, MarkdownBlock{Kind: MarkdownBlockKindParagraph, Text: text})
			}
		}
		marker = nil
		lines = nil
	}
	for _, raw := range strings.Split(text, "\n") {
		line := trimBlanks(raw)
		if markdownHasPrefix(line, "```") {
			if inCode {
				blocks = append(blocks, MarkdownBlock{Kind: MarkdownBlockKindCode, Text: strings.Join(code, "\n")})
				code = nil
				inCode = false
			} else {
				flush()
				code = []string{}
				inCode = true
			}
			continue
		}
		if inCode {
			code = append(code, raw)
			continue
		}
		if line == "" {
			flush()
			continue
		}
		if markdownHasPrefix(line, markdownBlockHeading) {
			flush()
			level := markdownLeadingHashes(line)
			blocks = append(blocks, MarkdownBlock{
				Kind:  MarkdownBlockKindHeading,
				Level: level,
				Text:  trimBlanks(line[level:]),
			})
		} else if markdownHasPrefix(line, markdownBlockDash) || markdownHasPrefix(line, markdownBlockStar) {
			flush()
			bullet := "•"
			marker = &bullet
			lines = []string{raw}
		} else if numbered, ok := markdownNumberedMarker(line); ok {
			flush()
			marker = &numbered
			lines = []string{raw}
		} else {
			// A line under an item, indented or not, continues it.
			lines = append(lines, raw)
		}
	}
	if inCode {
		blocks = append(blocks, MarkdownBlock{Kind: MarkdownBlockKindCode, Text: strings.Join(code, "\n")})
	}
	flush()
	return blocks
}

// markdownBlockReflow joins the lines of one paragraph or item. Agents often wrap their prose
// at a column, and those breaks give way so the text fills the page; a break the next
// word would have fitted before was meant, and stays.
func markdownBlockReflow(lines []string) string {
	trimmed := make([]string, len(lines))
	ends := make([]string, len(lines))
	for i, line := range lines {
		trimmed[i] = trimBlanks(line)
		ends[i] = strings.TrimRight(line, " \t")
	}
	width := 0
	for _, end := range ends {
		if n := markdownCharacterCount(end); n > width {
			width = n
		}
	}
	text := markdownBlockContent(trimmed[0])
	for i := 1; i < len(trimmed); i++ {
		before := ends[i-1]
		word := 0
		for _, c := range Characters(trimmed[i]) {
			if c == markdownBlockSpace {
				break
			}
			word++
		}
		hard := strings.HasSuffix(before, markdownBlockBackslash) || strings.HasSuffix(lines[i-1], "  ") ||
			width < markdownBlockWrapColumn || markdownCharacterCount(before)+1+word <= width
		if strings.HasSuffix(before, markdownBlockBackslash) {
			text = markdownDropLastCharacter(text)
		}
		if hard {
			text += "\n"
		} else {
			text += markdownBlockSpace
		}
		text += trimmed[i]
	}
	return text
}

// markdownBlockContent is a line's text without its list marker.
func markdownBlockContent(line string) string {
	if markdownHasPrefix(line, markdownBlockDash) || markdownHasPrefix(line, markdownBlockStar) {
		return line[len(markdownBlockDash):]
	}
	if marker, ok := markdownNumberedMarker(line); ok {
		return line[len(marker)+len(markdownBlockSpace):]
	}
	return line
}

// markdownNumberedMarker is the marker of a numbered item, such as "1.", when line starts with
// one: digits, a dot (the first dot of the line), then a space.
func markdownNumberedMarker(line string) (string, bool) {
	chars := Characters(line)
	dot := -1
	for i, c := range chars {
		if c == "." {
			dot = i
			break
		}
	}
	if dot <= 0 {
		return "", false
	}
	for _, c := range chars[:dot] {
		r, _ := utf8.DecodeRuneInString(c)
		if !unicode.IsNumber(r) {
			return "", false
		}
	}
	if dot+1 >= len(chars) || chars[dot+1] != markdownBlockSpace {
		return "", false
	}
	return strings.Join(chars[:dot+1], ""), true
}

// markdownLeadingHashes is how many "#" characters start line.
func markdownLeadingHashes(line string) int {
	n := 0
	for _, c := range Characters(line) {
		if c != markdownBlockHeading {
			break
		}
		n++
	}
	return n
}

// markdownHasPrefix reports whether s starts with the characters of prefix,
// so a following combining mark or joiner makes the last character of prefix differ.
func markdownHasPrefix(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	if len(s) == len(prefix) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(s[len(prefix):])
	last, _ := utf8.DecodeLastRuneInString(prefix)
	return !markdownExtends(last, next, 0, 0)
}

// markdownDropLastCharacter is s without its last character, for
// the backslash the reflow drops.
func markdownDropLastCharacter(s string) string {
	chars := Characters(s)
	if len(chars) == 0 {
		return s
	}
	return strings.Join(chars[:len(chars)-1], "")
}

// markdownCharacterCount is the number of characters in s.
func markdownCharacterCount(s string) int {
	return len(Characters(s))
}

// Characters splits s into the approximate characters described in the file comment.
func Characters(s string) []string {
	var out []string
	start := -1
	var prev rune
	clusterRunes := 0
	var clusterFirst rune
	for i, r := range s {
		if start >= 0 && markdownExtends(prev, r, clusterRunes, clusterFirst) {
			clusterRunes++
		} else {
			if start >= 0 {
				out = append(out, s[start:i])
			}
			start = i
			clusterRunes = 1
			clusterFirst = r
		}
		prev = r
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// markdownExtends reports whether rune r, after prev, joins the character prev belongs to
// rather than starting one. clusterRunes and clusterFirst describe that character so far.
func markdownExtends(prev, r rune, clusterRunes int, clusterFirst rune) bool {
	switch {
	case prev == '\r' && r == '\n':
		return true
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc):
		return true
	case r == 0x200D:
		return true
	case prev == 0x200D && r > 0x7F:
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0x1F3FB && r <= 0x1F3FF, r >= 0xE0020 && r <= 0xE007F:
		return true
	case markdownRegionalIndicator(r) && clusterRunes == 1 && markdownRegionalIndicator(clusterFirst):
		return true
	}
	return false
}

// markdownRegionalIndicator reports whether r is one of the regional indicator symbols that pair into a flag.
func markdownRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}
