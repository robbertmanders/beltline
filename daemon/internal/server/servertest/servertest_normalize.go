// The golden normaliser: volatile values (ids, times, paths) are replaced with stable placeholders
// before a comparison.

package servertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Normalize applies the one test-only normalisation pass to a response body: the
// UUIDs of notice ids and attempt session ids are renumbered by first appearance, the
// preview outcome ids, the model catalogs' executable names, belt `created` dates, the
// session and socket names, and the temporary root prefix. An empty body is returned as is.
func Normalize(body []byte, endpoint, temporaryRoot string) ([]byte, error) {
	return NormalizeAt(body, endpoint, temporaryRoot)
}

// NormalizeAt is Normalize for a fragment of a response: the fragment is normalised as if it sat
// at the object path given (the keys above it, such as "attempts" or "modelCatalogs", which the
// allowlist reads). A golden member compared on its own needs its path to renumber the same ids.
func NormalizeAt(body []byte, endpoint, temporaryRoot string, path ...string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%s: response is not JSON", endpoint)
	}
	n := &goldenNormalizer{bytes: body, endpoint: endpoint, uuidMap: map[string]string{}, roots: temporaryRoots(temporaryRoot)}
	return n.normalize(append([]string(nil), path...))
}

// goldenReplacement is one string token to rewrite: its byte range and the new string.
type goldenReplacement struct {
	start, end int
	value      string
}

// goldenNormalizer carries the state the normaliser keeps across one body.
type goldenNormalizer struct {
	bytes        []byte
	index        int
	endpoint     string
	roots        []string
	replacements []goldenReplacement
	uuidMap      map[string]string
	uuidIndex    int
}

var uuidShape = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

var uuidWhole = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// normalize parses the body, records the replacements and applies them from the end.
func (n *goldenNormalizer) normalize(base []string) ([]byte, error) {
	if err := n.value(base, ""); err != nil {
		return nil, err
	}
	if n.index != len(n.bytes) {
		return nil, fmt.Errorf("%s: trailing data after JSON body", n.endpoint)
	}
	result := append([]byte(nil), n.bytes...)
	// Replace from the last token to the first so earlier offsets stay valid.
	for i := len(n.replacements) - 1; i >= 0; i-- {
		r := n.replacements[i]
		encoded, err := encodeGoldenString(r.value)
		if err != nil {
			return nil, err
		}
		result = append(result[:r.start], append(encoded, result[r.end:]...)...)
	}
	return result, nil
}

// value parses one JSON value at the current index; path is where it sits and key the
// object key it is under ("" inside arrays and at the top).
func (n *goldenNormalizer) value(path []string, key string) error {
	if n.index >= len(n.bytes) {
		return fmt.Errorf("%s: truncated JSON body", n.endpoint)
	}
	switch n.bytes[n.index] {
	case '{':
		return n.object(path)
	case '[':
		return n.array(path)
	case '"':
		token, start, end, err := n.stringToken()
		if err != nil {
			return err
		}
		if normalized := n.normalizeString(token, path, key); normalized != token {
			n.replacements = append(n.replacements, goldenReplacement{start: start, end: end, value: normalized})
		}
		return nil
	default:
		start := n.index
		for n.index < len(n.bytes) && !isGoldenDelimiter(n.bytes[n.index]) {
			n.index++
		}
		if n.index == start {
			return fmt.Errorf("%s: invalid JSON token", n.endpoint)
		}
		return nil
	}
}

// isGoldenDelimiter reports whether b ends a bare JSON token: whitespace, ',', ']' or '}'.
func isGoldenDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', ',', ']', '}':
		return true
	}
	return false
}

// object parses an object.
func (n *goldenNormalizer) object(path []string) error {
	n.index++
	if n.consume('}') {
		return nil
	}
	for {
		if n.index >= len(n.bytes) || n.bytes[n.index] != '"' {
			return fmt.Errorf("%s: invalid JSON object key", n.endpoint)
		}
		key, _, _, err := n.stringToken()
		if err != nil {
			return err
		}
		if !n.consume(':') {
			return fmt.Errorf("%s: missing colon after JSON key", n.endpoint)
		}
		if err := n.value(appendPath(path, key), key); err != nil {
			return err
		}
		if n.consume('}') {
			return nil
		}
		if !n.consume(',') {
			return fmt.Errorf("%s: invalid JSON object separator", n.endpoint)
		}
	}
}

// array parses an array; its elements' paths carry their index.
func (n *goldenNormalizer) array(path []string) error {
	n.index++
	if n.consume(']') {
		return nil
	}
	for element := 0; ; element++ {
		if err := n.value(appendPath(path, "["+strconv.Itoa(element)+"]"), ""); err != nil {
			return err
		}
		if n.consume(']') {
			return nil
		}
		if !n.consume(',') {
			return fmt.Errorf("%s: invalid JSON array separator", n.endpoint)
		}
	}
}

// stringToken reads the string token at the index: its decoded value and its byte range.
func (n *goldenNormalizer) stringToken() (string, int, int, error) {
	start := n.index
	n.index++
	for n.index < len(n.bytes) {
		switch n.bytes[n.index] {
		case '\\':
			n.index += 2
			continue
		case '"':
			n.index++
			var value string
			if err := json.Unmarshal(n.bytes[start:n.index], &value); err != nil {
				return "", 0, 0, fmt.Errorf("%s: %w", n.endpoint, err)
			}
			return value, start, n.index, nil
		}
		n.index++
	}
	return "", 0, 0, fmt.Errorf("%s: unterminated JSON string", n.endpoint)
}

// consume advances past b when it is the next byte.
func (n *goldenNormalizer) consume(b byte) bool {
	if n.index < len(n.bytes) && n.bytes[n.index] == b {
		n.index++
		return true
	}
	return false
}

// normalizeString applies the allowlist to one string value.
func (n *goldenNormalizer) normalizeString(source string, path []string, key string) string {
	if strings.HasPrefix(source, "bl-assistant-") {
		return "<ASSISTANT_SESSION>"
	}
	if key == "sessionId" && containsStep(path, "attempts") && isUUIDString(source) {
		return n.normalizedUUID(source)
	}
	if key == "id" && containsStep(path, "notices") && isUUIDString(source) {
		return n.normalizedUUID(source)
	}
	if (key == "session" || key == "agentName" || key == "completionToken") && containsStep(path, "attempts") {
		return n.replacingUUIDs(source)
	}
	if key == "operation" && isUUIDString(source) {
		return n.normalizedUUID(source)
	}
	// Continue-session's attach command names the tmux server's socket, which the test picks per process.
	if len(path) == 2 && path[0] == "command" && path[1] == "[2]" {
		return "<SOCKET>"
	}
	if key == "tmuxSocket" {
		return "<SOCKET>"
	}
	if strings.Contains(strings.ToLower(n.endpoint), "preview") && key == "id" && containsStep(path, "outcomes") && isUUIDString(source) {
		return n.normalizedUUID(source)
	}
	if n.endpoint == "settings" && key == "executable" {
		if index := slices.Index(path, "modelCatalogs"); index >= 0 && index+1 < len(path) {
			runner := path[index+1]
			if runner == "cursor" {
				return "cursor-agent"
			}
			return runner
		}
	}
	if strings.HasPrefix(n.endpoint, "belt") && key == "created" {
		return "2000-01-01T00:00:00Z"
	}
	for _, root := range n.roots {
		source = strings.ReplaceAll(source, root, "<TEMP_ROOT>")
	}
	return source
}

// temporaryRoots lists the spellings of the temporary root a body may carry: the
// cleaned path and, when it differs, the path with its symbolic links resolved.
func temporaryRoots(root string) []string {
	if root == "" {
		return nil
	}
	roots := []string{filepath.Clean(root)}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && filepath.Clean(resolved) != roots[0] {
		roots = append(roots, filepath.Clean(resolved))
	}
	return roots
}

// replacingUUIDs replaces every UUID-shaped substring of source with its normalised token.
func (n *goldenNormalizer) replacingUUIDs(source string) string {
	return uuidShape.ReplaceAllStringFunc(source, n.normalizedUUID)
}

// normalizedUUID gives a UUID its token, numbered by first appearance in the body.
func (n *goldenNormalizer) normalizedUUID(source string) string {
	if existing, ok := n.uuidMap[source]; ok {
		return existing
	}
	n.uuidIndex++
	token := fmt.Sprintf("00000000-0000-4000-8000-%012d", n.uuidIndex)
	n.uuidMap[source] = token
	return token
}

// encodeGoldenString encodes a string as JSON without escaping slashes or HTML characters.
func encodeGoldenString(value string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// isUUIDString reports whether s is a UUID in its text form.
func isUUIDString(s string) bool { return uuidWhole.MatchString(s) }

// containsStep reports whether path has the step.
func containsStep(path []string, step string) bool { return slices.Index(path, step) >= 0 }

// appendPath is path with step added, without sharing path's backing array.
func appendPath(path []string, step string) []string {
	out := make([]string, len(path), len(path)+1)
	copy(out, path)
	return append(out, step)
}
