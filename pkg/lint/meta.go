package lint

import (
	"slices"
	"strings"
)

// Category groups rules by the Markdown construct they check. Each rule
// belongs to exactly one. Categories order rule listings and documentation.
// Unlike tags, they are not accepted where a rule can be named: several
// share a name with a markdownlint tag that covers different rules.
type Category string

// The categories, in the order that rule listings present them.
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

// Categories returns every category, in the order that rule listings
// present them.
func Categories() []Category {
	return []Category{
		CategoryHeadings, CategoryLists, CategoryLayout, CategoryCode, CategoryLinks,
		CategoryTables, CategoryEmphasis, CategoryBlockquotes, CategoryHTML, CategorySpelling,
	}
}

// RuleMeta is a rule's catalog entry: the identifiers that name the rule and
// how it is classified. The rules package keeps one entry for each built-in
// rule and records it with Registry.Describe.
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

	// Fixable reports whether the rule can fix what it finds, as the rule's
	// own CanFix does.
	Fixable bool

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

// clone returns a copy of meta that shares no slices with it.
func (meta RuleMeta) clone() RuleMeta {
	meta.Aliases = slices.Clone(meta.Aliases)
	meta.Tags = slices.Clone(meta.Tags)
	meta.Options = slices.Clone(meta.Options)
	for idx := range meta.Options {
		meta.Options[idx].Aliases = slices.Clone(meta.Options[idx].Aliases)
		meta.Options[idx].Values = slices.Clone(meta.Options[idx].Values)
		meta.Options[idx].Default = cloneOptionValue(meta.Options[idx].Default)
	}
	return meta
}

// OptionSpec describes one option that a rule reads from its options map.
type OptionSpec struct {
	// Name is the option's canonical spelling.
	Name string

	// Aliases are other spellings of the option, such as markdownlint's
	// spelling where gomdlint names an option differently.
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
	// option of a rule that markdownlint lacks is one.
	GomdlintOnly bool

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

// IsGomdlintOnlyRule reports whether the rule with the given ID is one of
// gomdlint's own rules, which markdownlint does not have. gomdlint gives
// its own rules IDs in the MDL namespace, such as MDL001, and the rules it
// shares with markdownlint their markdownlint IDs, such as MD009.
func IsGomdlintOnlyRule(id string) bool {
	return strings.HasPrefix(id, "MDL")
}

// cloneOptionValue returns a copy of value, an option's value, that shares
// no list with it.
func cloneOptionValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return slices.Clone(typed)
	case []any:
		return slices.Clone(typed)
	}
	return value
}
