// ModelChoice, ContextSize,
// ModelCatalogContradiction, ModelCatalogSnapshot and ModelAllowance.

package core

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/robbertmanders/beltline/daemon/internal/wire/jsonx"
)

// ModelChoice is a model exposed by a runner's own catalog. Identifiers are
// opaque and runner-scoped; names are presentation only.
type ModelChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Variants is, for a runner whose catalog folds the thinking level into the
	// model (Cursor's `claude-opus-5-5-high`), the exact catalog identifier at
	// each level, keyed by Thinking raw value, plus "default" for the variant the
	// catalog names without a level. ID then only groups them.
	Variants map[string]string `json:"variants,omitzero"`
	// ContextTokens is the input-context tokens when the catalog reported a size. Absent metadata decodes as nil.
	// The size never becomes part of ID; only a runner that picks a tier by id
	// carries its own suffix (Claude's [1m], Copilot's [long_context]).
	ContextTokens *int `json:"contextTokens,omitzero"`
}

// DisplayLabel is "<name> - <size>" when a size was reported, for example
// "Opus 5.5 - 250k". A name that already contains that capacity keeps the size
// once.
func (m ModelChoice) DisplayLabel() string {
	if m.ContextTokens == nil || *m.ContextTokens <= 0 {
		return m.Name
	}
	size := ContextSizeLabel(*m.ContextTokens)
	base := modelChoiceWithoutCapacity(m.Name, size)
	if base == "" {
		return size
	}
	return base + " - " + size
}

// Matches reports whether the choice matches a search query, case-insensitively,
// by display label, name or id. An empty query matches.
func (m ModelChoice) Matches(query string) bool {
	return query == "" || LocalizedCaseInsensitiveContains(m.DisplayLabel(), query) || LocalizedCaseInsensitiveContains(m.Name, query) || LocalizedCaseInsensitiveContains(m.ID, query)
}

// modelChoiceWithoutCapacity is name with the size word and the dash before it
// removed, when name contains the size.
func modelChoiceWithoutCapacity(name, size string) string {
	words := strings.Fields(name)
	found := slices.ContainsFunc(words, func(word string) bool { return strings.EqualFold(word, size) })
	if !found {
		return name
	}
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if !strings.EqualFold(word, size) {
			kept = append(kept, word)
		}
	}
	for len(kept) > 0 && kept[len(kept)-1] == "-" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, " ")
}

// Identifier is the identifier the runner is given: the variant at thinking,
// else the nearest below it, else the nearest above. Without a level, the
// catalog's default variant.
func (m ModelChoice) Identifier(thinking *Thinking) string {
	if len(m.Variants) == 0 {
		return m.ID
	}
	if thinking == nil {
		if variant, ok := m.Variants["default"]; ok {
			return variant
		}
		medium := ThinkingMedium
		return m.Identifier(&medium)
	}
	index := 0
	for i, level := range AllThinking {
		if level == *thinking {
			index = i
			break
		}
	}
	for i := index; i >= 0; i-- {
		if variant, ok := m.Variants[string(AllThinking[i])]; ok {
			return variant
		}
	}
	for i := index; i < len(AllThinking); i++ {
		if variant, ok := m.Variants[string(AllThinking[i])]; ok {
			return variant
		}
	}
	if variant, ok := m.Variants["default"]; ok {
		return variant
	}
	return m.ID
}

// ContextSizeLabel is a token count as decimal k and M, without rounding a
// remainder away.
func ContextSizeLabel(tokens int) string {
	if tokens >= 1_000_000 {
		return contextSizeScaled(tokens, 1_000_000) + "M"
	}
	if tokens >= 1_000 {
		return contextSizeScaled(tokens, 1_000) + "k"
	}
	return strconv.Itoa(tokens)
}

// ContextSizeParse reads `250k`, `100K`, `1M`, `1.048576M`, or a raw positive
// integer such as `1048576` (raw only when raw is set). Nil when the text is not
// a size.
func ContextSizeParse(text string, raw bool) *int {
	token := strings.TrimSpace(text)
	if token == "" {
		return nil
	}
	last, width := utf8.DecodeLastRuneInString(token)
	var multiplier int
	var body string
	switch last {
	case 'M', 'm':
		multiplier, body = 1_000_000, token[:len(token)-width]
	case 'K', 'k':
		multiplier, body = 1_000, token[:len(token)-width]
	default:
		if !unicode.IsNumber(last) || !raw || !modelAllASCIIDigits(token) {
			return nil
		}
		value, err := strconv.Atoi(token)
		if err != nil || value <= 0 {
			return nil
		}
		return &value
	}
	pieces := strings.Split(body, ".")
	if len(pieces) != 1 && len(pieces) != 2 {
		return nil
	}
	whole, err := strconv.Atoi(pieces[0])
	if err != nil || whole < 0 {
		return nil
	}
	fraction := ""
	if len(pieces) == 2 {
		if pieces[1] == "" {
			return nil
		}
		fraction = pieces[1]
	}
	if !modelAllASCIIDigits(fraction) {
		return nil
	}
	return contextSizeCombine(whole, fraction, multiplier)
}

// contextSizeScaled is tokens divided by divisor as decimal digits with the
// remainder kept, trailing zeros trimmed.
func contextSizeScaled(tokens, divisor int) string {
	whole := tokens / divisor
	remainder := tokens % divisor
	if remainder == 0 {
		return strconv.Itoa(whole)
	}
	width := len(strconv.Itoa(divisor)) - 1
	digits := strconv.Itoa(remainder)
	if len(digits) < width {
		digits = strings.Repeat("0", width-len(digits)) + digits
	}
	digits = strings.TrimRight(digits, "0")
	return strconv.Itoa(whole) + "." + digits
}

// contextSizeCombine is whole and fraction scaled by multiplier, or nil when it
// does not come to a whole positive count.
func contextSizeCombine(whole int, fraction string, multiplier int) *int {
	base, ok := contextSizeMul(whole, multiplier)
	if !ok {
		return nil
	}
	if fraction == "" {
		if base > 0 {
			return &base
		}
		return nil
	}
	if len(fraction) > 9 {
		return nil
	}
	frac, err := strconv.Atoi(fraction)
	if err != nil {
		return nil
	}
	scale := contextSizePow10(len(fraction))
	if scale == nil {
		return nil
	}
	product, ok := contextSizeMul(frac, multiplier)
	if !ok || product%*scale != 0 {
		return nil
	}
	sum, ok := AddInts(base, product / *scale)
	if !ok || sum <= 0 {
		return nil
	}
	return &sum
}

// contextSizePow10 is 10 to the power count, or nil on overflow.
func contextSizePow10(count int) *int {
	value := 1
	for range count {
		next, ok := contextSizeMul(value, 10)
		if !ok {
			return nil
		}
		value = next
	}
	return &value
}

// ModelCatalogContradiction is the same selectable id reported with two context
// sizes. Deduplication must not hide it.
type ModelCatalogContradiction struct {
	Message string
}

// Error returns the message.
func (e ModelCatalogContradiction) Error() string {
	return e.Message
}

// ErrorDescription is the message the user sees, which
// the error middleware answers as a 422.
func (e ModelCatalogContradiction) ErrorDescription() (string, bool) {
	return e.Message, true
}

// ModelCatalogSnapshot is a complete successful discovery result. Failed or
// partial discovery must not be represented as a snapshot, so callers can
// retain the previous value.
type ModelCatalogSnapshot struct {
	Runner      Backend       `json:"runner"`
	Executable  string        `json:"executable"`
	RefreshedAt jsonx.Date    `json:"refreshedAt"`
	Models      []ModelChoice `json:"models"`
}

// NewModelCatalogSnapshot creates a snapshot from a discovery result: models with
// an empty id are dropped, a repeated id with a different context size is a
// ModelCatalogContradiction, and the rest are sorted by name (case-insensitive),
// then id.
func NewModelCatalogSnapshot(runner Backend, executable string, refreshedAt time.Time, models []ModelChoice) (ModelCatalogSnapshot, error) {
	byID := map[string]ModelChoice{}
	for _, model := range models {
		if model.ID == "" {
			continue
		}
		if existing, ok := byID[model.ID]; ok && !SamePointee(existing.ContextTokens, model.ContextTokens) {
			return ModelCatalogSnapshot{}, ModelCatalogContradiction{
				Message: "Contradictory context sizes for " + model.ID + ". The last successful catalog was not replaced.",
			}
		}
		byID[model.ID] = model
	}
	sorted := make([]ModelChoice, 0, len(byID))
	for _, model := range byID {
		sorted = append(sorted, model)
	}
	sort.Slice(sorted, func(i, j int) bool {
		order := LocalizedCaseInsensitiveCompare(sorted[i].Name, sorted[j].Name)
		if order == 0 {
			return sorted[i].ID < sorted[j].ID
		}
		return order < 0
	})
	return ModelCatalogSnapshot{
		Runner:      runner,
		Executable:  executable,
		RefreshedAt: jsonx.Date{Time: refreshedAt},
		Models:      sorted,
	}, nil
}

// IsUsable reports whether a cache is usable with the executable identity that
// discovered it.
func (s ModelCatalogSnapshot) IsUsable(runner Backend, executable string) bool {
	return s.Runner == runner && s.Executable == executable
}

// ModelAllowance is explicit, exact model permissions. There are no wildcards or defaults.
type ModelAllowance struct {
	// Identifiers are the allowed identifiers, by runner.
	Identifiers map[Backend]map[string]struct{}
}

// Choices are the current choices: the intersection of exact permissions and the
// runner's last usable catalog, filtered by search. Empty when the catalog is
// nil or not usable for runner and executable.
func (a ModelAllowance) Choices(runner Backend, snapshot *ModelCatalogSnapshot, executable string, search string) []ModelChoice {
	out := []ModelChoice{}
	if snapshot == nil || !snapshot.IsUsable(runner, executable) {
		return out
	}
	allowed := a.Identifiers[runner]
	for _, model := range snapshot.Models {
		if _, ok := allowed[model.ID]; ok && model.Matches(search) {
			out = append(out, model)
		}
	}
	return out
}

// UnavailableIdentifiers are the checked identifiers missing from a usable
// catalog: they stay discoverable for removal in Settings, but never appear
// among current choices. With no usable catalog, all of them. Sorted.
func (a ModelAllowance) UnavailableIdentifiers(runner Backend, snapshot *ModelCatalogSnapshot, executable string) []string {
	allowed := a.Identifiers[runner]
	out := []string{}
	if snapshot == nil || !snapshot.IsUsable(runner, executable) {
		for identifier := range allowed {
			out = append(out, identifier)
		}
		sort.Strings(out)
		return out
	}
	present := map[string]struct{}{}
	for _, model := range snapshot.Models {
		present[model.ID] = struct{}{}
	}
	for identifier := range allowed {
		if _, ok := present[identifier]; !ok {
			out = append(out, identifier)
		}
	}
	sort.Strings(out)
	return out
}

// contextSizeMul multiplies two non-negative counts, reporting false on overflow.
func contextSizeMul(a, b int) (int, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
}

// modelAllASCIIDigits reports whether every byte is a decimal digit (true for
// the empty string). Only ASCII digits count.
func modelAllASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// modelRunnerKnown reports whether raw is a Backend raw value (Backend(rawValue:) != nil).
func modelRunnerKnown(raw string) bool {
	return slices.ContainsFunc(AllBackend, func(runner Backend) bool { return string(runner) == raw })
}
