package lint

import (
	"slices"
)

// Category groups rules by the Markdown construct they check. Each rule
// belongs to exactly one. Rule listings, such as gomdlint rules, and
// documentation, such as the generated rule reference, group the rules by
// category, in the order of Categories. The registry keeps rule ID order.
// Unlike tags, categories are not accepted where rules are configured or
// selected, in config keys, rule flags and markdownlint comments: several
// share a name with a markdownlint tag that covers different rules. Only
// discovery accepts them, such as gomdlint explain and the --category
// filter of gomdlint rules.
type Category string

// The categories, in the order that listings and documentation present
// them.
const (
	CategoryHeadings    Category = "headings"
	CategoryLists       Category = "lists"
	CategoryLayout      Category = "layout" // Spaces, tabs, blank lines, line length, horizontal rules and file endings.
	CategoryCode        Category = "code"
	CategoryLinks       Category = "links" // Links and images.
	CategoryTables      Category = "tables"
	CategoryEmphasis    Category = "emphasis"
	CategoryBlockquotes Category = "blockquotes"
	CategoryHTML        Category = "html"
	CategorySpelling    Category = "spelling"
)

// Categories returns every category, in the order that listings and
// documentation present them.
func Categories() []Category {
	return []Category{
		CategoryHeadings, CategoryLists, CategoryLayout, CategoryCode, CategoryLinks,
		CategoryTables, CategoryEmphasis, CategoryBlockquotes, CategoryHTML, CategorySpelling,
	}
}

// Title returns the name that listings and documentation give the category
// in headings, such as "Whitespace and layout" for layout. A category that
// gomdlint does not define is titled by its key.
func (c Category) Title() string {
	switch c {
	case CategoryHeadings:
		return "Headings"
	case CategoryLists:
		return "Lists"
	case CategoryLayout:
		return "Whitespace and layout"
	case CategoryCode:
		return "Code"
	case CategoryLinks:
		return "Links and images"
	case CategoryTables:
		return "Tables"
	case CategoryEmphasis:
		return "Emphasis"
	case CategoryBlockquotes:
		return "Blockquotes"
	case CategoryHTML:
		return "HTML"
	case CategorySpelling:
		return "Spelling"
	}
	return string(c)
}

// RuleMeta is a rule's catalog entry: the identifiers that name the rule,
// how it is classified, and what it checks, why, and what its fix does. The
// rules package keeps one entry for each built-in rule and records it with
// Registry.Describe.
type RuleMeta struct {
	// ID is the rule's permanent identifier, such as MD009.
	ID string

	// Name is the rule's canonical name, such as no-trailing-spaces. It is
	// the name the rule itself reports.
	Name string

	// Aliases are the other names that identify the rule wherever a rule
	// can be named: markdownlint's names for it that differ from Name, in
	// markdownlint's order, such as ul-style for MD004.
	Aliases []string

	// MarkdownlintName is the name that markdownlint lists first for the
	// rule, when that is not Name, such as ul-style for MD004, and so one of
	// Aliases, as a catalog test checks for the built-in rules. Where gomdlint
	// suggests a rule for a markdownlint comment, it writes this name, which
	// markdownlint reads too. It is empty when Name is markdownlint's first
	// name, and for rules that markdownlint lacks.
	MarkdownlintName string

	// Category is the construct the rule checks.
	Category Category

	// Tags are markdownlint's tags for the rule, in markdownlint's order,
	// such as whitespace. Rules that markdownlint lacks take tags from the
	// same vocabulary. Inline directives and migrated markdownlint configs
	// resolve tags through these, not through Rule.Tags.
	Tags []string

	// RuleTags are the tags that the rule reports itself (Rule.Tags), where
	// they differ from Tags: gomdlint's own classification of the rule,
	// which configuration templates and SARIF rule properties show. A rule
	// built from an entry without them (nil) reports Tags (see
	// NewBaseRuleFromMeta). An empty, non-nil RuleTags gives the rule no
	// tags.
	RuleTags []string

	// Fixable reports whether the rule can fix what it finds, as the rule's
	// own CanFix does.
	Fixable bool

	// Summary is the short phrase that the rule reports as its own
	// description (Rule.Description), such as "Horizontal rule style".
	// Configuration templates show it. Rule listings and documentation show
	// Description instead.
	Summary string

	// Description says what correct Markdown looks like under the rule's
	// default options, such as "Lines do not end in spaces or tabs, except 2
	// spaces for a hard line break." It is plain text without Markdown, on
	// one line of under 80 characters, made of sentences that end with a
	// period. Rule listings and SARIF short descriptions show it as it is.
	Description string

	// Rationale gives the reason for the rule: what goes wrong in Markdown
	// that breaks it. It is plain text on one line, made of sentences.
	Rationale string

	// Fix says what the rule's fix changes, and which findings a user can
	// expect the rule to report without a fix. A rule that cannot fix what
	// it finds says that there is no automatic fix. It is plain text on one
	// line, made of sentences. Shared checks that parse the fixed document
	// can hold back other fixes too.
	Fix string

	// Options describes the options the rule reads. An empty list declares
	// nothing: it does not mean that the rule has no options.
	Options []OptionSpec

	// OptionsDeclared reports that Options lists every option the rule
	// reads, so that an empty list means it reads none. The registry then
	// rewrites the rule's options to their canonical names before the rule
	// runs (see NormalizeOptions), and configuration checks report any
	// other option as unknown. Every built-in rule's entry declares its
	// options.
	OptionsDeclared bool
}

// RuleDescription returns the one-line description of rule: the description
// in meta, its catalog entry, or the rule's own description when meta has
// none, as the zero entry of a rule without one has none.
func RuleDescription(rule Rule, meta RuleMeta) string {
	if meta.Description != "" {
		return meta.Description
	}
	return rule.Description()
}

// clone returns a copy of meta that shares no slices with it.
func (meta RuleMeta) clone() RuleMeta {
	meta.Aliases = slices.Clone(meta.Aliases)
	meta.Tags = slices.Clone(meta.Tags)
	meta.RuleTags = slices.Clone(meta.RuleTags)
	meta.Options = slices.Clone(meta.Options)
	for idx := range meta.Options {
		meta.Options[idx] = meta.Options[idx].clone()
	}
	return meta
}

// OptionSpec describes one option that a rule reads from its options map.
type OptionSpec struct {
	// Name is the option's canonical spelling.
	Name string

	// Aliases are other spellings of the option, such as markdownlint's
	// spelling where gomdlint names an option differently. For one of
	// markdownlint's rules (see IsMarkdownlintRule), a markdownlint
	// configuration reads the option under its first alias only (see
	// Registry.MarkdownlintRuleConfig).
	Aliases []OptionAlias

	// Type is the type of the option's value.
	Type OptionType

	// Default is the value the rule uses when the option is not set. It is
	// nil for an option that turns on a check of its own when it is set,
	// such as MD047's max_trailing_blank_lines, which limits nothing
	// unless set.
	Default any

	// DefaultFrom names another option of the rule, whose value this one
	// takes when it is not set, as heading_max takes max's. Default is
	// then the value that both take when neither is set.
	DefaultFrom string

	// Values lists the strings that the option allows, for an option that
	// allows only these. It is empty for an option that takes any value of
	// its type.
	Values []string

	// GomdlintOnly reports that markdownlint has no such option. Every
	// option of a rule that markdownlint lacks is one. A markdownlint
	// configuration cannot set such an option of one of markdownlint's rules
	// (see IsMarkdownlintRule), as markdownlint does not read it
	// (see Registry.MarkdownlintRuleConfig).
	GomdlintOnly bool

	// Limit reports that the option, an integer, is a most that a count
	// may reach, such as line-length's max, which markdownlint 0.41.1
	// compares counts with. So a value beyond every count, such as
	// Infinity, which a markdownlint configuration may write as YAML's
	// .inf, a JSON number beyond float64's range or the string "Infinity",
	// or a number of 2^63 or more, lifts the limit, since no count reaches
	// it. gomdlint reads such a value there as math.MaxInt, which no count
	// reaches either, and one below -2^63, such as -Infinity, as
	// math.MinInt, which every count exceeds. line-length then checks no
	// line, as markdownlint does, while no-multiple-blanks reads any
	// maximum below 1 as 1, where markdownlint reports every line of the
	// document for a maximum below 0. A value beyond int's range of any
	// other option is of the wrong type.
	Limit bool

	// ZeroIsDefault reports that the rule reads 0 as the option's default,
	// as markdownlint 0.41.1 does, which reads the option as
	// Number(value || default). A string that JavaScript's Number reads as
	// 0, such as "0" or " ", is not false to ||, so markdownlint uses 0
	// itself, while the empty string, which is, means the default. A
	// markdownlint configuration's empty string reads as 0, and such a
	// string as MarkdownlintZero, or, when the option has none, as of the
	// wrong type, with a warning.
	ZeroIsDefault bool

	// MarkdownlintZero is, for an option whose rule reads 0 as the default
	// (ZeroIsDefault), the value with which the rule does what
	// markdownlint 0.41.1 does with 0 itself, such as -1 for line-length's
	// lengths, with which neither checks a line. It is nil when the rule
	// has no such value, as no-multiple-blanks has none for a maximum of
	// 0, of which markdownlint reports every blank line.
	MarkdownlintZero any

	// MarkdownlintZeroIsDefault reports that markdownlint 0.41.1 reads 0 as
	// the option's default, as it reads the option as
	// Number(value || default), while the rule reads 0 as it is, such as
	// ul-indent's indent. A markdownlint configuration's 0, or empty
	// string, leaves the option unset, while a string that reads as 0,
	// such as "0", which is not false to ||, reads as 0.
	MarkdownlintZeroIsDefault bool

	// MarkdownlintPinned reports that a markdownlint configuration sets the
	// option to Default explicitly in each setting of the rule that resets
	// no options, the default key and a rule key that turns the rule off, so
	// that the setting replaces the option of a lower configuration layer.
	// gomdlint's translation of markdownlint settings has always written
	// these options out (see Registry.MarkdownlintRuleConfig). A rule key
	// that turns the rule on resets every option instead (see
	// config.RuleConfig.ResetOptions).
	MarkdownlintPinned bool

	// Description says what the option controls, in plain text.
	Description string
}

// OptionAlias is another spelling of a rule option.
type OptionAlias struct {
	// Name is the alternative spelling.
	Name string

	// Inverted reports whether the alias holds the opposite boolean value,
	// as markdownlint's tables does for gomdlint's ignore_tables.
	Inverted bool
}

// gomdlintRuleIDs are the IDs of gomdlint's own rules, which its catalog
// describes, as a catalog test checks.
//
//nolint:gochecknoglobals // Read-only lookup table.
var gomdlintRuleIDs = map[string]bool{"MDL001": true, "MDL003": true}

// IsGomdlintOnlyRule reports whether the rule with the given ID is one of
// gomdlint's own rules, which markdownlint does not have. gomdlint gives
// its own rules IDs in the MDL namespace, such as MDL001, and the rules it
// shares with markdownlint their markdownlint IDs, such as MD009. A custom
// rule is neither, even under an ID such as MDL900 in that namespace.
func IsGomdlintOnlyRule(id string) bool {
	return gomdlintRuleIDs[id]
}

// markdownlintRuleIDs are the IDs of markdownlint 0.41.1's rules, each of
// which gomdlint's catalog describes under the same ID, as a catalog test
// checks.
//
//nolint:gochecknoglobals // Read-only lookup table.
var markdownlintRuleIDs = map[string]bool{
	"MD001": true, "MD003": true, "MD004": true, "MD005": true, "MD007": true, "MD009": true,
	"MD010": true, "MD011": true, "MD012": true, "MD013": true, "MD014": true, "MD018": true,
	"MD019": true, "MD020": true, "MD021": true, "MD022": true, "MD023": true, "MD024": true,
	"MD025": true, "MD026": true, "MD027": true, "MD028": true, "MD029": true, "MD030": true,
	"MD031": true, "MD032": true, "MD033": true, "MD034": true, "MD035": true, "MD036": true,
	"MD037": true, "MD038": true, "MD039": true, "MD040": true, "MD041": true, "MD042": true,
	"MD043": true, "MD044": true, "MD045": true, "MD046": true, "MD047": true, "MD048": true,
	"MD049": true, "MD050": true, "MD051": true, "MD052": true, "MD053": true, "MD054": true,
	"MD055": true, "MD056": true, "MD058": true, "MD059": true, "MD060": true,
}

// IsMarkdownlintRule reports whether the rule with the given ID is one of
// markdownlint 0.41.1's rules, which gomdlint reimplements under the same
// ID. A custom rule is not, even under an ID such as MD900 that has the
// shape of markdownlint's, as markdownlint lacks it too.
func IsMarkdownlintRule(id string) bool {
	return markdownlintRuleIDs[id]
}

// clone returns a copy of spec that shares no slice with it: not its
// aliases, its allowed values or a list default.
func (spec OptionSpec) clone() OptionSpec {
	spec.Aliases = slices.Clone(spec.Aliases)
	spec.Values = slices.Clone(spec.Values)
	spec.Default = cloneOptionValue(spec.Default)
	return spec
}

// cloneOptionValue returns a copy of value, an option's value, that shares
// no list with it: a list of strings, of integers or of any values, as the
// option types hold, gets a copy of its own.
func cloneOptionValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return slices.Clone(typed)
	case []int:
		return slices.Clone(typed)
	case []any:
		return slices.Clone(typed)
	}
	return value
}
