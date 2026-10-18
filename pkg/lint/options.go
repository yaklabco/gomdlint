package lint

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// OptionType is the type of a rule option's value, written as
// markdownlint's documentation writes the types of its options.
type OptionType string

// The types of rule options.
const (
	// OptionBoolean takes true or false.
	OptionBoolean OptionType = "boolean"

	// OptionInteger takes a whole number.
	OptionInteger OptionType = "integer"

	// OptionString takes a string.
	OptionString OptionType = "string"

	// OptionStringList takes a list of strings.
	OptionStringList OptionType = "string[]"

	// OptionIntegerOrList takes a whole number, or a list of whole numbers.
	OptionIntegerOrList OptionType = "integer|integer[]"

	// OptionStringOrFalse takes a string, or false to turn off what the
	// string sets.
	OptionStringOrFalse OptionType = "string|false"
)

// Accepts reports whether value is a value of the type, as a configuration
// file holds one: a whole number as an int, or as a float64 such as 1e2,
// and a list as a []any or a typed slice. Null, which leaves an option
// unset, suits every type. The type of a list's items is not checked:
// rules skip items of another type, as markdownlint does.
func (t OptionType) Accepts(value any) bool {
	if value == nil {
		return true
	}
	switch t {
	case OptionBoolean:
		_, ok := value.(bool)
		return ok
	case OptionInteger:
		return isWholeNumber(value)
	case OptionString:
		_, ok := value.(string)
		return ok
	case OptionStringList:
		switch value.(type) {
		case []any, []string:
			return true
		}
		return false
	case OptionIntegerOrList:
		switch value.(type) {
		case []any, []int:
			return true
		}
		return isWholeNumber(value)
	case OptionStringOrFalse:
		switch typed := value.(type) {
		case string:
			return true
		case bool:
			return !typed
		}
		return false
	}
	return false
}

// isWholeNumber reports whether value is a whole number that rules read as
// an int: an int, or a float64 with no fraction within int's range.
func isWholeNumber(value any) bool {
	switch number := value.(type) {
	case int:
		return true
	case float64:
		return number == math.Trunc(number) && number >= math.MinInt64 && number < math.MaxInt64
	}
	return false
}

// typeText describes the type in prose, for a message that names the type
// a value must have.
func (t OptionType) typeText() string {
	switch t {
	case OptionBoolean:
		return "true or false"
	case OptionInteger:
		return "an integer"
	case OptionString:
		return "a string"
	case OptionStringList:
		return "a list of strings"
	case OptionIntegerOrList:
		return "an integer or a list of integers"
	case OptionStringOrFalse:
		return "a string or false"
	}
	return string(t)
}

// OptionIssueKind classifies an OptionIssue.
type OptionIssueKind int

// The kinds of option issues.
const (
	// OptionIssueUnknown marks a key that spells no option of the rule. The
	// rule never reads it.
	OptionIssueUnknown OptionIssueKind = iota + 1

	// OptionIssueWrongType marks a value of the wrong type. NormalizeOptions
	// drops it, so the rule uses the option's default.
	OptionIssueWrongType

	// OptionIssueNotAllowed marks a string that the option does not allow.
	// NormalizeOptions keeps it, and the rule treats it as markdownlint
	// treats such a value.
	OptionIssueNotAllowed

	// OptionIssueConflict marks two spellings of one option, both set.
	// NormalizeOptions keeps the value of the canonical spelling.
	OptionIssueConflict
)

// OptionIssue is a problem with one entry of a rule's options, which
// NormalizeOptions reports.
type OptionIssue struct {
	// Kind classifies the problem.
	Kind OptionIssueKind

	// Key is the key as the options write it. For OptionIssueConflict it is
	// the first of the two spellings in byte order.
	Key string

	// Other is, for OptionIssueConflict, the second of the two spellings.
	Other string

	// Value is the value written under Key.
	Value any

	// Option is the option that Key spells. It is the zero OptionSpec for
	// OptionIssueUnknown.
	Option OptionSpec

	// Suggestions are, for OptionIssueUnknown, the spellings that Key most
	// likely means, and, for OptionIssueNotAllowed, the allowed values that
	// Value most likely means. There are none when nothing is close
	// enough, or when too many are.
	Suggestions []string

	// NoOptions reports, for OptionIssueUnknown, that the rule reads no
	// options at all.
	NoOptions bool
}

// Text describes the issue for a warning or an error about the options of
// the rule named ruleName, in the manner of UnknownRuleText:
//
//	unknown option "line_lenght" for line-length (did you mean "line_length"?); it is ignored
//	line-length option "max" must be an integer, not "100px"; it is ignored
//	ul-style option "style" must be one of "consistent", "dash" or "plus", not "dashes" (did you mean "dash"?)
//	"line_length" and "max" both set line-length option "max"; keep one of them
func (issue OptionIssue) Text(ruleName string) string {
	switch issue.Kind {
	case OptionIssueUnknown:
		text := fmt.Sprintf("unknown option %q for %s", issue.Key, ruleName)
		switch {
		case issue.NoOptions:
			text += " (it has no options)"
		case len(issue.Suggestions) > 0:
			text += " (did you mean " + quotedChoices(issue.Suggestions, "or") + "?)"
		}
		return text + "; it is ignored"
	case OptionIssueWrongType:
		return fmt.Sprintf("%s option %q must be %s, not %s; it is ignored",
			ruleName, issue.Key, issue.Option.Type.typeText(), describeOptionValue(issue.Value))
	case OptionIssueNotAllowed:
		text := fmt.Sprintf("%s option %q must be one of %s, not %s",
			ruleName, issue.Key, quotedChoices(issue.Option.Values, "or"), describeOptionValue(issue.Value))
		if len(issue.Suggestions) > 0 {
			text += " (did you mean " + quotedChoices(issue.Suggestions, "or") + "?)"
		}
		return text
	case OptionIssueConflict:
		return fmt.Sprintf("%q and %q both set %s option %q; keep one of them",
			issue.Key, issue.Other, ruleName, issue.Option.Name)
	}
	return fmt.Sprintf("%s option %q: unknown problem", ruleName, issue.Key)
}

// quotedChoices quotes each word and joins them as a list in prose.
func quotedChoices(words []string, conjunction string) string {
	quoted := make([]string, len(words))
	for idx, word := range words {
		quoted[idx] = strconv.Quote(word)
	}
	return joinChoices(quoted, conjunction)
}

// describeOptionValue writes an option's value for a message: a string
// quoted, null for nil, and anything else as JSON writes it.
func describeOptionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(typed)
	}
	if encoded, err := json.Marshal(value); err == nil {
		return string(encoded)
	}
	return fmt.Sprint(value)
}

// NormalizeOptions returns options, the options of one rule, keyed by the
// canonical names of the options that specs declare, and the problems it
// finds:
//
//   - a key that is an alias, such as markdownlint's spelling, becomes the
//     canonical name, and an inverted alias's boolean value is negated;
//   - a value of the wrong type is dropped, so the rule uses the option's
//     default;
//   - a string that the option does not allow is kept, because rules treat
//     such values as markdownlint does;
//   - of two spellings of one option, the canonical one is kept;
//   - a key that spells no option is kept, and the rule never reads it;
//   - a list becomes a []any, as a configuration file holds lists.
//
// It never changes options, and returns options itself when nothing
// changes. The problems are in order of key.
func NormalizeOptions(specs []OptionSpec, options map[string]any) (map[string]any, []OptionIssue) {
	return newOptionTable(specs).normalize(options)
}

// MarkdownlintRuleConfig converts value, the setting of the registered rule
// with the given ID in a markdownlint configuration, to gomdlint's settings
// for the rule (see config.MarkdownlintRuleConfig). Its options are
// rewritten through the rule's declared options, as NormalizeOptions does:
// markdownlint's spellings become canonical names, and options that the
// rule does not read are left out, as markdownlint ignores them. The
// issues report each option left out or kept in doubt, and two spellings
// of one option. A rule whose entry does not declare its options keeps
// them as written.
func (r *Registry) MarkdownlintRuleConfig(id string, value any) (config.RuleConfig, []OptionIssue) {
	var issues []OptionIssue
	cfg := config.MarkdownlintRuleConfig(id, value, func(ruleID string, options map[string]any) map[string]any {
		normalized, found, declared := r.NormalizeOptions(ruleID, options)
		if !declared {
			return options
		}
		issues = found
		var known map[string]any
		for _, issue := range found {
			if issue.Kind != OptionIssueUnknown {
				continue
			}
			if known == nil {
				known = maps.Clone(normalized)
			}
			delete(known, issue.Key)
		}
		if known != nil {
			return known
		}
		return normalized
	})
	return cfg, issues
}

// optionTable indexes the options that one rule declares by spelling.
type optionTable struct {
	specs     []OptionSpec
	spellings map[string]optionSpelling
}

// optionSpelling is a name or alias of the option specs[index].
type optionSpelling struct {
	index    int
	alias    bool
	inverted bool
}

// newOptionTable indexes specs. A spelling that two options claim names the
// first; the catalog's tests rule that out for the built-in rules.
func newOptionTable(specs []OptionSpec) *optionTable {
	table := &optionTable{specs: specs, spellings: make(map[string]optionSpelling, len(specs))}
	claim := func(name string, spelling optionSpelling) {
		if _, taken := table.spellings[name]; !taken {
			table.spellings[name] = spelling
		}
	}
	for idx, spec := range specs {
		claim(spec.Name, optionSpelling{index: idx})
	}
	for idx, spec := range specs {
		for _, alias := range spec.Aliases {
			claim(alias.Name, optionSpelling{index: idx, alias: true, inverted: alias.Inverted})
		}
	}
	return table
}

// normalize implements NormalizeOptions for the table's options.
func (t *optionTable) normalize(options map[string]any) (map[string]any, []OptionIssue) {
	if len(options) == 0 || t.canonical(options) {
		return options, nil
	}

	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	var issues []OptionIssue
	changed := false
	chosen := make(map[int]string, len(keys)) // spec index -> the key that sets it
	for _, key := range keys {
		spelling, known := t.spellings[key]
		if !known {
			issues = append(issues, OptionIssue{
				Kind: OptionIssueUnknown, Key: key, Value: options[key],
				Suggestions: closestWords(key, t.allSpellings()), NoOptions: len(t.specs) == 0,
			})
			continue
		}
		previous, conflict := chosen[spelling.index]
		if !conflict {
			chosen[spelling.index] = key
			continue
		}
		// Keys arrive in byte order, so previous sorts before key.
		issues = append(issues, OptionIssue{
			Kind: OptionIssueConflict, Key: previous, Other: key, Value: options[previous],
			Option: t.specs[spelling.index],
		})
		changed = true
		if !spelling.alias {
			chosen[spelling.index] = key // The canonical spelling wins.
		}
	}

	values := make(map[int]any, len(chosen))
	for index, key := range chosen {
		spec, spelling, value := t.specs[index], t.spellings[key], options[key]
		if spelling.alias {
			changed = true
		}
		if spelling.inverted {
			if enabled, ok := value.(bool); ok {
				value = !enabled
			}
		}
		if !spec.Type.Accepts(value) {
			issues = append(issues, OptionIssue{Kind: OptionIssueWrongType, Key: key, Value: options[key], Option: spec})
			changed = true
			continue
		}
		if text, ok := value.(string); ok && len(spec.Values) > 0 && !slices.Contains(spec.Values, text) {
			issues = append(issues, OptionIssue{
				Kind: OptionIssueNotAllowed, Key: key, Value: value, Option: spec,
				Suggestions: closestWords(text, spec.Values),
			})
		}
		if list, converted := listValues(value); converted {
			value = list
			changed = true
		}
		values[index] = value
	}
	slices.SortStableFunc(issues, func(left, right OptionIssue) int { return cmp.Compare(left.Key, right.Key) })

	if !changed {
		return options, issues
	}
	normalized := make(map[string]any, len(options))
	for _, key := range keys {
		if _, known := t.spellings[key]; !known {
			normalized[key] = options[key]
		}
	}
	for index, value := range values {
		normalized[t.specs[index].Name] = value
	}
	return normalized, issues
}

// canonical reports whether options need no change and have no problem:
// every key is the canonical name of an option, with a value of its type
// that it allows and that is no typed slice. Linting checks the options of
// every rule for every file, and they are usually canonical already.
func (t *optionTable) canonical(options map[string]any) bool {
	for key, value := range options {
		spelling, known := t.spellings[key]
		if !known || spelling.alias {
			return false
		}
		spec := t.specs[spelling.index]
		if !spec.Type.Accepts(value) {
			return false
		}
		switch typed := value.(type) {
		case []string, []int:
			return false
		case string:
			if len(spec.Values) > 0 && !slices.Contains(spec.Values, typed) {
				return false
			}
		}
	}
	return true
}

// allSpellings lists the names and aliases of the table's options.
func (t *optionTable) allSpellings() []string {
	spellings := make([]string, 0, len(t.spellings))
	for spelling := range t.spellings {
		spellings = append(spellings, spelling)
	}
	slices.Sort(spellings)
	return spellings
}

// listValues returns a typed slice as a []any, the form in which a
// configuration file holds lists, reporting whether value was one.
func listValues(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []string:
		list := make([]any, len(typed))
		for idx, item := range typed {
			list[idx] = item
		}
		return list, true
	case []int:
		list := make([]any, len(typed))
		for idx, item := range typed {
			list[idx] = item
		}
		return list, true
	}
	return nil, false
}

// closestWords returns the words that key most likely means, as
// Registry.Closest picks rules: those of the best rank, where a typo of
// at most two edits ranks by its distance and a substring ranks after
// every typo, or none when more than three share the best rank. Matching
// ignores case.
func closestWords(key string, words []string) []string {
	key = strings.ToLower(key)
	best, bestRank := []string(nil), noMatch
	for _, word := range words {
		rank := matchRank(key, strings.ToLower(word), true)
		switch {
		case rank == noMatch:
		case bestRank == noMatch || rank < bestRank:
			best, bestRank = []string{word}, rank
		case rank == bestRank:
			best = append(best, word)
		}
	}
	if len(best) > maxClosest {
		return nil
	}
	slices.Sort(best)
	return best
}
