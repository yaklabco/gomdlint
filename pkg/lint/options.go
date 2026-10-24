package lint

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"regexp"
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
// and a list as a []any, or as a []string or []int, whose items all have
// the type of the list's items. Null, which leaves an option unset, suits
// every type, but not an item of a list.
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
		if _, ok := value.([]string); ok {
			return true
		}
		items, ok := value.([]any)
		return ok && len(t.wrongItems(items)) == 0
	case OptionIntegerOrList:
		if _, ok := value.([]int); ok {
			return true
		}
		if items, ok := value.([]any); ok {
			return len(t.wrongItems(items)) == 0
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

// wrongItems returns the positions, counted from 0, of the items that do
// not have the type of the items of a list of type t, a list type. It
// returns none for a type that is no list.
func (t OptionType) wrongItems(items []any) []int {
	var wrong []int
	for idx, item := range items {
		switch t {
		case OptionStringList:
			if _, ok := item.(string); !ok {
				wrong = append(wrong, idx)
			}
		case OptionIntegerOrList:
			if !isWholeNumber(item) {
				wrong = append(wrong, idx)
			}
		}
	}
	return wrong
}

// itemTypeText describes the type of the items of a list of type t, a list
// type, in prose.
func (t OptionType) itemTypeText() string {
	if t == OptionStringList {
		return OptionString.typeText()
	}
	return OptionInteger.typeText()
}

// jsDecimal matches a decimal literal as JavaScript's Number reads one from
// a string: an optional sign, digits with an optional point, or a point and
// digits, and an optional exponent. Number reads no underscores and no
// hexadecimal fractions, which Go's ParseFloat may, so they do not match.
var jsDecimal = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// jsNonDecimal matches an integer that JavaScript's Number reads from a
// string in hexadecimal, octal or binary, after 0x, 0o or 0b in either
// case, without a sign. Its groups hold the digits of each base.
var jsNonDecimal = regexp.MustCompile(`^0(?:[xX]([0-9a-fA-F]+)|[oO]([0-7]+)|[bB]([01]+))$`)

// jsStringNumber returns the number that JavaScript's Number reads from
// text, and whether it reads one: after the whitespace that Number trims at
// either end (see isJSWhitespace), a decimal literal such as -1.5e3, .5 or
// 7., Infinity with an optional sign, or an integer in hexadecimal, octal
// or binary, such as 0x1F. Number reads empty text, and nothing but
// whitespace, as 0, which counts as no number here, and anything else as
// NaN. A number beyond float64's range is an infinity, and one too small to
// tell from zero is zero, as in JavaScript.
func jsStringNumber(text string) (float64, bool) {
	text = strings.TrimFunc(text, isJSWhitespace)
	switch text {
	case "":
		return 0, false
	case "Infinity", "+Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	}
	if jsDecimal.MatchString(text) {
		number, err := strconv.ParseFloat(text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return 0, false // A decimal literal always parses; this is unreachable.
		}
		return number, true
	}
	match := jsNonDecimal.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	for idx, base := range []int{16, 8, 2} {
		digits := match[idx+1]
		if digits == "" {
			continue
		}
		integer, ok := new(big.Int).SetString(digits, base)
		if !ok {
			return 0, false // The pattern admits only digits of the base; this is unreachable.
		}
		number, _ := new(big.Float).SetInt(integer).Float64()
		return number, true
	}
	return 0, false
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

// OptionDeclarer is a rule that declares the options it reads itself, as
// every built-in rule does. A registry uses the declaration when the rule
// has no catalog entry that declares its options (RuleMeta.OptionsDeclared),
// so that a rule registered on its own, without its entry, still reads its
// options under every spelling that the declaration accepts, and under
// markdownlint's spellings in a markdownlint configuration (see
// Registry.MarkdownlintRuleConfig).
type OptionDeclarer interface {
	Rule

	// DeclaredOptions returns every option that the rule reads, in a list
	// that the caller may keep.
	DeclaredOptions() []OptionSpec
}

// OptionIssueKind classifies an OptionIssue.
type OptionIssueKind int

// The kinds of option issues.
const (
	// OptionIssueUnknown marks a key that spells no option of the rule. The
	// rule never reads it.
	OptionIssueUnknown OptionIssueKind = iota + 1

	// OptionIssueWrongType marks a value of the wrong type, or an item of
	// the wrong type in a list (see OptionIssue.Item). NormalizeOptions
	// drops the option either way, as if the options did not set it: a
	// list with a wrong item is a value of the wrong type.
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

	// Value is the value written under Key, or, for an issue about one
	// item of a list (see Item), that item.
	Value any

	// Item is, for an OptionIssueWrongType about one item of a list, the
	// item's position, counted from 1. It is 0 for an issue about the
	// whole value.
	Item int

	// Option is the option that Key spells. It is the zero OptionSpec for
	// OptionIssueUnknown.
	Option OptionSpec

	// Suggestions are, for OptionIssueUnknown, the spellings that Key most
	// likely means, and, for OptionIssueNotAllowed, the allowed values that
	// Value most likely means. There are none when nothing is close
	// enough, or when too many are.
	Suggestions []string

	// NoOptions reports, for OptionIssueUnknown, that the rule reads no
	// options at all, or, in a markdownlint configuration, none that
	// markdownlint has, such as single-trailing-newline, whose only option
	// is gomdlint's.
	NoOptions bool
}

// Text describes the issue for a warning or an error about the options of
// the rule named ruleName, in the manner of UnknownRuleText:
//
//	unknown option "lin_length" for line-length (did you mean "line_length"?); it is ignored
//	line-length option "max" must be an integer, not "100px"; it is ignored
//	heading-blank-lines option "lines_above" item 2 must be an integer, not 2.5; the option is ignored
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
		if issue.Item > 0 {
			return fmt.Sprintf("%s option %q item %d must be %s, not %s; the option is ignored",
				ruleName, issue.Key, issue.Item, issue.Option.Type.itemTypeText(), describeOptionValue(issue.Value))
		}
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

// UnknownTagOptionText describes option, an option in the setting of tag,
// a key of a markdownlint configuration that names several rules, that
// none of those rules reads, for a warning, in the manner of
// OptionIssue.Text for an option that one rule does not read:
//
//	unknown option "br_spcaes" for the rules of the tag "whitespace"; it is ignored
func UnknownTagOptionText(option, tag string) string {
	return fmt.Sprintf("unknown option %q for the rules of the tag %q; it is ignored", option, tag)
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
// quoted, null for nil, an infinity or NaN as JavaScript writes it, such as
// Infinity, and anything else as JSON writes it.
func describeOptionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return jsNull
	case string:
		return strconv.Quote(typed)
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) {
			return jsNumberString(typed)
		}
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
//   - a value of the wrong type is dropped, as if options did not set it,
//     and so is a list with an item of the wrong type, with an issue for
//     each such item;
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
// rule does not read are left out, as markdownlint ignores them. For a
// rule that markdownlint has, one with an ID of markdownlint's shape, MD
// and digits, an option's only spelling is markdownlint's, its first
// alias, else its name, so gomdlint's names for options that markdownlint
// spells otherwise, and options marked GomdlintOnly, are left out too, as
// markdownlint does not read them. The options of other rules, gomdlint's
// own and custom ones, keep every spelling. A value that turns the rule on
// resets the options that lower configuration layers give the rule (see
// config.RuleConfig.ResetOptions), so its settings hold the options that
// it writes and no others. A value that turns the rule off writes out the
// options marked MarkdownlintPinned, set to their defaults under the
// options that it writes, so that it replaces them in a lower
// configuration layer, as gomdlint's translation always has. The issues
// report each option left out or kept in doubt. A rule without a
// declaration of its options keeps them as written.
func (r *Registry) MarkdownlintRuleConfig(id string, value any) (config.RuleConfig, []OptionIssue) {
	var issues []OptionIssue
	table := r.lookups().markdownlintOptions[id]
	cfg := config.MarkdownlintRuleConfig(id, value, func(_ string, options map[string]any) map[string]any {
		if table == nil {
			return options
		}
		normalized, found := table.normalize(options)
		issues = found
		return normalized
	})
	if table != nil && !cfg.ResetOptions {
		options := table.pinned()
		maps.Copy(options, cfg.Options)
		if len(options) > 0 {
			cfg.Options = options
		}
	}
	return cfg, issues
}

// MarkdownlintDefaultRuleConfig converts value, the value of the default key
// of a markdownlint configuration, to the settings that it gives the
// registered rule with the given ID (see config.MarkdownlintDefaultRuleConfig):
// the rule's state and severity, and the options marked MarkdownlintPinned,
// set to their defaults. The value itself writes no options, and the
// settings reset none.
func (r *Registry) MarkdownlintDefaultRuleConfig(id string, value any) config.RuleConfig {
	return config.MarkdownlintDefaultRuleConfig(id, value, func(ruleID string, options map[string]any) map[string]any {
		table := r.lookups().markdownlintOptions[ruleID]
		if table == nil {
			return options
		}
		return table.pinned()
	})
}

// optionTable indexes the options that one rule declares by spelling.
type optionTable struct {
	specs     []OptionSpec
	spellings map[string]optionSpelling

	// renamed maps, in a table of the spellings that a markdownlint
	// configuration gives the options (see newMarkdownlintOptionTable),
	// gomdlint's name of each option that markdownlint spells otherwise to
	// markdownlint's spelling, which an issue about the name suggests. It
	// is nil in a table of gomdlint's spellings.
	renamed map[string]string

	// dropUnknown makes normalize leave out the keys that spell no option,
	// as markdownlint ignores them, where NormalizeOptions keeps them. In a
	// table of markdownlint's spellings, such a key can be the canonical
	// name of an option that another key spells, such as max beside
	// line_length.
	dropUnknown bool

	// markdownlintValues makes normalize read values as the rules of
	// markdownlint 0.41.1 read them (see markdownlintValue), in a table of
	// the spellings of a rule that markdownlint has.
	markdownlintValues bool
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
	for idx, spec := range specs {
		table.claim(spec.Name, optionSpelling{index: idx})
	}
	for idx, spec := range specs {
		for _, alias := range spec.Aliases {
			table.claim(alias.Name, optionSpelling{index: idx, alias: true, inverted: alias.Inverted})
		}
	}
	return table
}

// claim makes name a spelling of the option that spelling describes, unless
// another option has claimed it already.
func (t *optionTable) claim(name string, spelling optionSpelling) {
	if _, taken := t.spellings[name]; !taken {
		t.spellings[name] = spelling
	}
}

// newMarkdownlintOptionTable indexes specs, the options that the rule with
// the given ID declares, by the spellings that a markdownlint configuration
// gives them. markdownlint 0.41.1 reads each option of its rules under its
// own spelling only: the option's first alias, where gomdlint names the
// option differently, such as line_length for max, and its name otherwise.
// So gomdlint's names for such options, and the options that only gomdlint
// has (OptionSpec.GomdlintOnly), name no option there, as in markdownlint,
// and no option has two spellings. The options of a rule that markdownlint
// lacks (see markdownlintHasRule), such as gomdlint's own rules and custom
// rules, keep every spelling. Either way, normalizing leaves out the keys
// that spell no option.
func newMarkdownlintOptionTable(id string, specs []OptionSpec) *optionTable {
	if !markdownlintHasRule(id) {
		table := newOptionTable(specs)
		table.dropUnknown = true
		return table
	}
	table := &optionTable{
		specs: specs, spellings: make(map[string]optionSpelling, len(specs)),
		renamed: make(map[string]string), dropUnknown: true, markdownlintValues: true,
	}
	for idx, spec := range specs {
		switch {
		case spec.GomdlintOnly:
		case len(spec.Aliases) > 0:
			alias := spec.Aliases[0]
			table.claim(alias.Name, optionSpelling{index: idx, alias: true, inverted: alias.Inverted})
			table.renamed[spec.Name] = alias.Name
		default:
			table.claim(spec.Name, optionSpelling{index: idx})
		}
	}
	return table
}

// markdownlintHasRule reports whether markdownlint 0.41.1 may have the rule
// with the given ID: whether the ID has the shape of the IDs of
// markdownlint's own rules, MD followed by digits, such as MD013.
// gomdlint's own rules (see IsGomdlintOnlyRule), and custom rules under IDs
// of other shapes, are rules that markdownlint lacks.
func markdownlintHasRule(id string) bool {
	digits, found := strings.CutPrefix(id, "MD")
	if !found || digits == "" {
		return false
	}
	return !strings.ContainsFunc(digits, func(char rune) bool { return char < '0' || char > '9' })
}

// markdownlintValue returns value as the rule that spec, one of the
// table's options, reads it in markdownlint 0.41.1, and whether that
// differs from value, for a table that reads values so
// (optionTable.markdownlintValues). Each rule of markdownlint reads its
// numeric options with JavaScript's Number, as Number(value || default) or
// as value === undefined ? default : Number(value), so a numeric string,
// such as "10" or " 0x0A ", reads as its number (see jsStringNumber). The
// items of a list stay as they are, as MD022, the one rule whose numeric
// option takes a list, uses them. A limit (OptionSpec.Limit) beyond the
// range of an int, such as an infinite one, reads as the nearest int,
// math.MaxInt or math.MinInt, which compares with every count as the value
// does: no count reaches the one, and every count exceeds the other.
func (t *optionTable) markdownlintValue(spec OptionSpec, value any) (any, bool) {
	if !t.markdownlintValues || (spec.Type != OptionInteger && spec.Type != OptionIntegerOrList) {
		return value, false
	}
	read, differs := value, false
	if text, isText := value.(string); isText {
		if number, ok := jsStringNumber(text); ok {
			read, differs = number, true
		}
	}
	if number, isNumber := read.(float64); isNumber && spec.Limit {
		// The bounds are those of isWholeNumber: 2^63 is beyond an int, and
		// -2^63 is one.
		switch {
		case number >= math.MaxInt64:
			read, differs = math.MaxInt, true
		case number < math.MinInt64:
			read, differs = math.MinInt, true
		}
	}
	return read, differs
}

// suggest returns the spellings that key, which spells no option of the
// table, most likely means: markdownlint's spelling of the option that key
// is gomdlint's name for, in a table of a markdownlint configuration's
// spellings, and otherwise the closest spellings (see closestWords). In
// such a table, a key close to none of them that misspells one of
// gomdlint's names, such as ignore_tabels, gets markdownlint's spelling of
// that option.
func (t *optionTable) suggest(key string) []string {
	if spelling, ok := t.renamed[key]; ok {
		return []string{spelling}
	}
	suggestions := closestWords(key, t.allSpellings())
	if len(suggestions) > 0 || len(t.renamed) == 0 {
		return suggestions
	}
	for _, name := range closestWords(key, slices.Sorted(maps.Keys(t.renamed))) {
		suggestions = append(suggestions, t.renamed[name])
	}
	slices.Sort(suggestions)
	return slices.Compact(suggestions)
}

// normalize implements NormalizeOptions for the table's options, except
// that a table with dropUnknown leaves out the keys that spell no option,
// and one with markdownlintValues reads values as markdownlint's rules do
// (see markdownlintValue).
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
				Suggestions: t.suggest(key), NoOptions: len(t.spellings) == 0,
			})
			changed = changed || t.dropUnknown
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
		if read, differs := t.markdownlintValue(spec, value); differs {
			value, changed = read, true
		}
		if !spec.Type.Accepts(value) {
			issues = append(issues, wrongTypeIssues(key, options[key], spec)...)
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
	// The specs belong to the caller, such as a registry, so an issue gets
	// its own copy of its option.
	for idx := range issues {
		issues[idx].Option = issues[idx].Option.clone()
	}

	if !changed {
		return options, issues
	}
	normalized := make(map[string]any, len(options))
	for _, key := range keys {
		if _, known := t.spellings[key]; !known && !t.dropUnknown {
			normalized[key] = options[key]
		}
	}
	for index, value := range values {
		normalized[t.specs[index].Name] = value
	}
	return normalized, issues
}

// wrongTypeIssues describes value, the value of the wrong type that key
// sets for spec: one issue for each item of the wrong type in a list, or
// one for the whole value.
func wrongTypeIssues(key string, value any, spec OptionSpec) []OptionIssue {
	if items, ok := value.([]any); ok {
		if wrong := spec.Type.wrongItems(items); len(wrong) > 0 {
			issues := make([]OptionIssue, len(wrong))
			for idx, item := range wrong {
				issues[idx] = OptionIssue{
					Kind: OptionIssueWrongType, Key: key, Value: items[item], Item: item + 1, Option: spec,
				}
			}
			return issues
		}
	}
	return []OptionIssue{{Kind: OptionIssueWrongType, Key: key, Value: value, Option: spec}}
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

// pinned returns a new map of the table's options that the markdownlint
// settings of the rule that reset no options write out
// (OptionSpec.MarkdownlintPinned), each set to a copy of its default.
func (t *optionTable) pinned() map[string]any {
	pinned := make(map[string]any)
	for _, spec := range t.specs {
		if spec.MarkdownlintPinned {
			pinned[spec.Name] = cloneOptionValue(spec.Default)
		}
	}
	return pinned
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
	ranks := make([]int, len(words))
	bestRank := noMatch
	for idx, word := range words {
		ranks[idx] = matchRank(key, strings.ToLower(word), true)
		if ranks[idx] != noMatch && (bestRank == noMatch || ranks[idx] < bestRank) {
			bestRank = ranks[idx]
		}
	}
	if bestRank == noMatch {
		return nil
	}

	best := make([]string, 0, maxClosest+1)
	for idx, word := range words {
		if ranks[idx] == bestRank {
			best = append(best, word)
		}
	}
	if len(best) > maxClosest {
		return nil
	}
	slices.Sort(best)
	return best
}
