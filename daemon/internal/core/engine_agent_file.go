//
// An agent as a Markdown file: settings and outcomes in a small YAML
// frontmatter, the brief as the body. The reader takes what the writer
// writes, plus the hand-edited variations of it: comments, quoted or bare
// values, and `to: [build, spec]` or `to: build, spec`. No YAML library: the
// subset is read by hand.

package core

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// AgentFileProblem is a file the agent reader cannot read.
type AgentFileProblem struct {
	Message string
}

// Error returns the message, which is what the user sees.
func (p AgentFileProblem) Error() string {
	return p.Message
}

// ErrorDescription is the message the user sees, which
// the error middleware answers as a 422.
func (p AgentFileProblem) ErrorDescription() (string, bool) {
	return p.Message, true
}

// agentFileListKeys are the keys whose values are lists of small objects:
// outcomes and alternates.
var agentFileListKeys = map[string]struct{}{"outcomes": {}, "alternates": {}}

// agentFileSpecial are the characters that start a YAML value which must be quoted.
const agentFileSpecial = "\"'-?:,[]{}#&*!|>%@`"

// AgentFileParse reads an agent file. The id and version come from the library,
// not the file.
func AgentFileParse(text, id string, version int) (Agent, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	end := agentFileFrontmatterEnd(lines)
	if end < 0 {
		return Agent{}, AgentFileProblem{Message: "The file doesn't start with a --- frontmatter block."}
	}
	fields := map[string]string{}
	// The lists of small objects: outcomes and alternates.
	lists := map[string][]map[string]string{}
	list := ""
	inList := false
	for _, line := range lines[1:end] {
		trimmed := trimBlanks(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indented := line[0] == ' ' || line[0] == '\t'
		if !indented {
			key, value, err := agentFilePair(trimmed)
			if err != nil {
				return Agent{}, err
			}
			_, isList := agentFileListKeys[key]
			list, inList = key, isList
			if isList {
				lists[key] = []map[string]string{}
			} else {
				fields[key] = value
			}
			continue
		}
		if !inList {
			return Agent{}, AgentFileProblem{Message: fmt.Sprintf("“%s” is indented but isn't part of the outcomes or alternates.", trimmed)}
		}
		if strings.HasPrefix(trimmed, "-") {
			lists[list] = append(lists[list], map[string]string{})
			rest := trimBlanks(trimmed[1:])
			if rest == "" {
				continue
			}
			key, value, err := agentFilePair(rest)
			if err != nil {
				return Agent{}, err
			}
			lists[list][len(lists[list])-1][key] = value
			continue
		}
		if len(lists[list]) == 0 {
			return Agent{}, AgentFileProblem{Message: agentFileFirstItemMessage(list, trimmed)}
		}
		key, value, err := agentFilePair(trimmed)
		if err != nil {
			return Agent{}, err
		}
		lists[list][len(lists[list])-1][key] = value
	}
	return agentFileBuild(fields, lists, lines[end+1:], id, version)
}

// agentFileLegacyMachineIcons is the icon an agent file saved before icons gets from its
// machine: the drawing agents were shown as before stage boxes. The machine itself is no
// longer read, and an unknown one leaves the agent on the chip.
var agentFileLegacyMachineIcons = map[string]StageIcon{
	"sorter": StageIconFunnel, "typewriter": StageIconDoc, "mailbox": StageIconSend, "robot": StageIconWrench,
	"microscope": StageIconScope, "reader": StageIconEye, "telescope": StageIconEye, "van": StageIconShip,
	"box": StageIconChip, "stamp": StageIconApprove, "crane": StageIconBranch, "drill": StageIconWrench,
	"radar": StageIconGauge,
}

// agentFileFrontmatterEnd is the index of the closing --- line, or -1 when the
// text does not start with a frontmatter block.
func agentFileFrontmatterEnd(lines []string) int {
	if trimBlanks(lines[0]) != "---" {
		return -1
	}
	for i := 1; i < len(lines); i++ {
		if trimBlanks(lines[i]) == "---" {
			return i
		}
	}
	return -1
}

// agentFileFirstItemMessage is the problem with an indented line before a list's first item.
func agentFileFirstItemMessage(list, trimmed string) string {
	if list == "outcomes" {
		return fmt.Sprintf("“%s” comes before the first outcome's “- name:”.", trimmed)
	}
	return fmt.Sprintf("“%s” comes before the first alternate's “- runner:”.", trimmed)
}

// agentFileBuild turns the read fields and lists into an agent, checking each
// value in a fixed order, so the same file always reports the same first problem.
func agentFileBuild(fields map[string]string, lists map[string][]map[string]string, brief []string, id string, version int) (Agent, error) {
	name := id
	if value, ok := fields["name"]; ok {
		name = value
	}
	agent := NewAgent(AgentArgs{ID: id, Version: &version, Name: name})
	if repository, ok := fields["origin"]; ok {
		agent.Origin = &LibraryOrigin{Repository: repository, Path: fields["originPath"]}
	}
	if key, ok := fields["key"]; ok && key != "" {
		agent.Key = key
	}
	var err error
	if value, ok := fields["workspace"]; ok {
		if agent.Workspace, err = agentFileChoice(value, "workspace", AllWorkspace); err != nil {
			return Agent{}, err
		}
	}
	if value, ok := fields["slots"]; ok {
		if agent.Slots, err = agentFileNumber(value, "slots"); err != nil {
			return Agent{}, err
		}
	}
	if value, ok := fields["runner"]; ok {
		if agent.Runner, err = agentFileChoice(value, "runner", AllBackend); err != nil {
			return Agent{}, err
		}
	}
	agent.Model = fields["model"]
	if value, ok := fields["thinking"]; ok {
		thinking, err := agentFileChoice(value, "thinking", AllThinking)
		if err != nil {
			return Agent{}, err
		}
		agent.Thinking = &thinking
	}
	if agent.Alternates, err = agentFileAlternates(lists["alternates"]); err != nil {
		return Agent{}, err
	}
	if value, ok := fields["tint"]; ok {
		if agent.Tint, err = agentFileChoice(value, "tint", AllTint); err != nil {
			return Agent{}, err
		}
	}
	if value, ok := fields["icon"]; ok {
		icon, err := agentFileChoice(value, "icon", AllStageIcon)
		if err != nil {
			return Agent{}, err
		}
		agent.Icon = &icon
	} else if icon, ok := agentFileLegacyMachineIcons[fields["machine"]]; ok {
		agent.Icon = &icon
	}
	if agent.Outcomes, err = agentFileOutcomes(lists["outcomes"]); err != nil {
		return Agent{}, err
	}
	agent.Brief = strings.TrimSpace(strings.Join(brief, "\n"))
	return agent, nil
}

// AgentFileWrite writes an agent as its file: the frontmatter, then the brief.
func AgentFileWrite(agent Agent) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", agentFileQuote(agent.Name))
	fmt.Fprintf(&b, "key: %s\n", agentFileQuote(agent.Key))
	fmt.Fprintf(&b, "workspace: %s\n", agent.Workspace)
	fmt.Fprintf(&b, "slots: %d\n", agent.Slots)
	fmt.Fprintf(&b, "runner: %s\n", agent.Runner)
	if agent.Origin != nil {
		fmt.Fprintf(&b, "origin: %s\n", agentFileQuote(agent.Origin.Repository))
		if agent.Origin.Path != "" {
			fmt.Fprintf(&b, "originPath: %s\n", agentFileQuote(agent.Origin.Path))
		}
	}
	if agent.Model != "" {
		fmt.Fprintf(&b, "model: %s\n", agentFileQuote(agent.Model))
	}
	if agent.Thinking != nil {
		fmt.Fprintf(&b, "thinking: %s\n", *agent.Thinking)
	}
	if len(agent.Alternates) > 0 {
		b.WriteString("alternates:\n")
		for _, alternate := range agent.Alternates {
			runner := ValueOr(alternate.Runner, BackendPi)
			fmt.Fprintf(&b, "  - runner: %s\n", runner)
			if alternate.Model != "" {
				fmt.Fprintf(&b, "    model: %s\n", agentFileQuote(alternate.Model))
			}
			if alternate.Thinking != nil {
				fmt.Fprintf(&b, "    thinking: %s\n", *alternate.Thinking)
			}
		}
	}
	fmt.Fprintf(&b, "tint: %s\n", agent.Tint)
	if agent.Icon != nil {
		fmt.Fprintf(&b, "icon: %s\n", *agent.Icon)
	}
	b.WriteString("outcomes:\n")
	for _, outcome := range agent.Outcomes {
		fmt.Fprintf(&b, "  - name: %s\n", agentFileQuote(outcome.Name))
		fmt.Fprintf(&b, "    when: %s\n", agentFileQuote(outcome.When))
		fmt.Fprintf(&b, "    tone: %s\n", outcome.Tone)
		fmt.Fprintf(&b, "    then: %s\n", outcome.Then.Action)
		if len(outcome.To) > 0 {
			quoted := make([]string, len(outcome.To))
			for i, to := range outcome.To {
				quoted[i] = agentFileQuote(to)
			}
			fmt.Fprintf(&b, "    to: %s\n", strings.Join(quoted, ", "))
		}
		if outcome.Then.WithOutput {
			b.WriteString("    withOutput: true\n")
		}
		if outcome.Then.Max != nil {
			fmt.Fprintf(&b, "    max: %d\n", *outcome.Then.Max)
		}
		if outcome.Then.Comment {
			b.WriteString("    comment: true\n")
		}
	}
	b.WriteString("---\n\n")
	return b.String() + strings.TrimSpace(agent.Brief) + "\n"
}

// agentFileAlternates reads the alternates list.
func agentFileAlternates(items []map[string]string) ([]SessionSettings, error) {
	var out []SessionSettings
	for _, fields := range items {
		settings := SessionSettings{Model: fields["model"]}
		if value, ok := fields["thinking"]; ok {
			thinking, err := agentFileChoice(value, "thinking", AllThinking)
			if err != nil {
				return nil, err
			}
			settings.Thinking = &thinking
		}
		runner := BackendPi
		if value, ok := fields["runner"]; ok {
			chosen, err := agentFileChoice(value, "runner", AllBackend)
			if err != nil {
				return nil, err
			}
			runner = chosen
		}
		settings.Runner = &runner
		out = append(out, settings)
	}
	return out, nil
}

// agentFileOutcomes reads the outcomes list.
func agentFileOutcomes(items []map[string]string) ([]AgentOutcome, error) {
	var out []AgentOutcome
	for _, fields := range items {
		name, ok := fields["name"]
		if !ok || name == "" {
			return nil, AgentFileProblem{Message: "An outcome has no name."}
		}
		action := ThenActionNext
		if value, ok := fields["then"]; ok {
			chosen, err := agentFileChoice(value, "then", AllThenAction)
			if err != nil {
				return nil, err
			}
			action = chosen
		}
		then := Then{Action: action}
		var err error
		if value, ok := fields["withOutput"]; ok {
			if then.WithOutput, err = agentFileFlag(value, "withOutput"); err != nil {
				return nil, err
			}
		}
		if value, ok := fields["max"]; ok {
			max, err := agentFileNumber(value, "max")
			if err != nil {
				return nil, err
			}
			then.Max = &max
		}
		if value, ok := fields["comment"]; ok {
			if then.Comment, err = agentFileFlag(value, "comment"); err != nil {
				return nil, err
			}
		}
		var to []string
		if value, ok := fields["to"]; ok {
			to = agentFileList(value)
		}
		tone := ToneNeutral
		if value, ok := fields["tone"]; ok {
			if tone, err = agentFileChoice(value, "tone", AllTone); err != nil {
				return nil, err
			}
		}
		out = append(out, AgentOutcome{Name: name, When: fields["when"], Tone: tone, Then: then, To: to})
	}
	return out, nil
}

// agentFilePair reads `key: value`, with the value unquoted.
func agentFilePair(line string) (string, string, error) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", "", AgentFileProblem{Message: fmt.Sprintf("“%s” isn't a “key: value” line.", line)}
	}
	key := trimBlanks(line[:colon])
	return key, agentFileUnquote(trimBlanks(line[colon+1:])), nil
}

// agentFileUnquote removes the quotes of a value, as AgentFile.unquote does. A
// double-quoted value reads its escapes; a bare value ends at a comment.
//
// Strings are read rune by rune, not by grapheme cluster. The
// two differ only for a backslash followed by a combining mark; no grapheme
// library is used.
func agentFileUnquote(value string) string {
	if utf8.RuneCountInString(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		var b strings.Builder
		escaped := false
		for _, r := range value[1 : len(value)-1] {
			switch {
			case escaped:
				if r == 'n' {
					b.WriteRune('\n')
				} else {
					b.WriteRune(r)
				}
				escaped = false
			case r == '\\':
				escaped = true
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	if utf8.RuneCountInString(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	// A bare value ends at a comment.
	if hash := strings.Index(value, " #"); hash >= 0 {
		return trimBlanks(value[:hash])
	}
	return value
}

// agentFileList reads `[a, b]` or `a, b` as its unquoted, non-empty items.
func agentFileList(value string) []string {
	var out []string
	for _, piece := range strings.Split(strings.Trim(value, "[] "), ",") {
		if item := agentFileUnquote(trimBlanks(piece)); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// agentFileChoice reads one of the raw values of a case list.
func agentFileChoice[T ~string](value, field string, all []T) (T, error) {
	for _, candidate := range all {
		if string(candidate) == value {
			return candidate, nil
		}
	}
	names := make([]string, len(all))
	for i, candidate := range all {
		names[i] = string(candidate)
	}
	return "", AgentFileProblem{Message: fmt.Sprintf("“%s” isn't a %s. Use one of: %s.", value, field, strings.Join(names, ", "))}
}

// agentFileNumber reads a whole number: an optional sign and digits.
func agentFileNumber(value, field string) (int, error) {
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, AgentFileProblem{Message: fmt.Sprintf("%s must be a number, not “%s”.", field, value)}
	}
	return number, nil
}

// agentFileFlag reads true or false, in any case, with yes and no.
func agentFileFlag(value, field string) (bool, error) {
	switch LowercasedFull(value) {
	case "true", "yes":
		return true, nil
	case "false", "no":
		return false, nil
	}
	return false, AgentFileProblem{Message: fmt.Sprintf("%s must be true or false, not “%s”.", field, value)}
}

// agentFileQuote is the value as is when YAML reads it back unchanged, else
// double-quoted.
func agentFileQuote(value string) string {
	plain := strings.ReplaceAll(value, "\n", " ")
	needs := plain == "" || plain != trimBlanks(plain) ||
		agentFileStartsSpecial(plain) ||
		strings.Contains(plain, ": ") || strings.Contains(plain, " #") || strings.HasSuffix(plain, ":") ||
		agentFileReserved(plain) || agentFileReadsAsDouble(plain)
	if !needs {
		return plain
	}
	escaped := strings.ReplaceAll(plain, `\`, `\\`)
	return `"` + strings.ReplaceAll(escaped, `"`, `\"`) + `"`
}

// agentFileStartsSpecial reports whether the first character is one YAML needs quoted.
func agentFileStartsSpecial(value string) bool {
	first, _ := utf8.DecodeRuneInString(value)
	return value != "" && strings.ContainsRune(agentFileSpecial, first)
}

// agentFileReserved reports whether the value is a word YAML reads as a non-string.
func agentFileReserved(value string) bool {
	switch LowercasedFull(value) {
	case "true", "false", "yes", "no", "null", "~":
		return true
	}
	return false
}

// agentFileReadsAsDouble reports whether Double(String) would read the
// text as a number, which includes nan, inf and hexadecimal forms.
func agentFileReadsAsDouble(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	if err == nil || errors.Is(err, strconv.ErrRange) {
		return true
	}
	// strtod takes a hexadecimal fraction without its binary exponent; Go wants one.
	if lower := strings.ToLower(value); strings.Contains(lower, "0x") && !strings.ContainsAny(lower, "p") {
		_, err = strconv.ParseFloat(value+"p0", 64)
		return err == nil || errors.Is(err, strconv.ErrRange)
	}
	return false
}
