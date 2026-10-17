package rules

// markdownlintRule is one row of markdownlint's rule table: the names that
// follow the rule's ID, which markdownlint calls its aliases, and its tags,
// both in markdownlint's order.
type markdownlintRule struct {
	Aliases []string
	Tags    []string
}

// markdownlintRules is markdownlint v0.41.0's rule table, keyed by rule ID.
//
// Source: the "names" and "tags" of every rule definition in lib/md*.mjs of
// markdownlint v0.41.0 (tag v0.41.0, commit
// ccb517357f3a3405b7a444f000808d3a77a6f9bf). markdownlint's README.md lists
// the same names under "Rules / Aliases" and the same tags under "Tags":
// https://github.com/DavidAnson/markdownlint/blob/v0.41.0/README.md#rules--aliases
// The table is unchanged in v0.41.1 and v0.42.0.
var markdownlintRules = map[string]markdownlintRule{
	"MD001": {Aliases: []string{"heading-increment"}, Tags: []string{"headings"}},
	"MD003": {Aliases: []string{"heading-style"}, Tags: []string{"headings"}},
	"MD004": {Aliases: []string{"ul-style"}, Tags: []string{"bullet", "ul"}},
	"MD005": {Aliases: []string{"list-indent"}, Tags: []string{"bullet", "ul", "indentation"}},
	"MD007": {Aliases: []string{"ul-indent"}, Tags: []string{"bullet", "ul", "indentation"}},
	"MD009": {Aliases: []string{"no-trailing-spaces"}, Tags: []string{"whitespace"}},
	"MD010": {Aliases: []string{"no-hard-tabs"}, Tags: []string{"whitespace", "hard_tab"}},
	"MD011": {Aliases: []string{"no-reversed-links"}, Tags: []string{"links"}},
	"MD012": {Aliases: []string{"no-multiple-blanks"}, Tags: []string{"whitespace", "blank_lines"}},
	"MD013": {Aliases: []string{"line-length"}, Tags: []string{"line_length"}},
	"MD014": {Aliases: []string{"commands-show-output"}, Tags: []string{"code"}},
	"MD018": {Aliases: []string{"no-missing-space-atx"}, Tags: []string{"headings", "atx", "spaces"}},
	"MD019": {Aliases: []string{"no-multiple-space-atx"}, Tags: []string{"headings", "atx", "spaces"}},
	"MD020": {Aliases: []string{"no-missing-space-closed-atx"}, Tags: []string{"headings", "atx_closed", "spaces"}},
	"MD021": {Aliases: []string{"no-multiple-space-closed-atx"}, Tags: []string{"headings", "atx_closed", "spaces"}},
	"MD022": {Aliases: []string{"blanks-around-headings"}, Tags: []string{"headings", "blank_lines"}},
	"MD023": {Aliases: []string{"heading-start-left"}, Tags: []string{"headings", "spaces"}},
	"MD024": {Aliases: []string{"no-duplicate-heading"}, Tags: []string{"headings"}},
	"MD025": {Aliases: []string{"single-title", "single-h1"}, Tags: []string{"headings"}},
	"MD026": {Aliases: []string{"no-trailing-punctuation"}, Tags: []string{"headings"}},
	"MD027": {Aliases: []string{"no-multiple-space-blockquote"}, Tags: []string{"blockquote", "whitespace", "indentation"}},
	"MD028": {Aliases: []string{"no-blanks-blockquote"}, Tags: []string{"blockquote", "whitespace"}},
	"MD029": {Aliases: []string{"ol-prefix"}, Tags: []string{"ol"}},
	"MD030": {Aliases: []string{"list-marker-space"}, Tags: []string{"ol", "ul", "whitespace"}},
	"MD031": {Aliases: []string{"blanks-around-fences"}, Tags: []string{"code", "blank_lines"}},
	"MD032": {Aliases: []string{"blanks-around-lists"}, Tags: []string{"bullet", "ul", "ol", "blank_lines"}},
	"MD033": {Aliases: []string{"no-inline-html"}, Tags: []string{"html"}},
	"MD034": {Aliases: []string{"no-bare-urls"}, Tags: []string{"links", "url"}},
	"MD035": {Aliases: []string{"hr-style"}, Tags: []string{"hr"}},
	"MD036": {Aliases: []string{"no-emphasis-as-heading"}, Tags: []string{"headings", "emphasis"}},
	"MD037": {Aliases: []string{"no-space-in-emphasis"}, Tags: []string{"whitespace", "emphasis"}},
	"MD038": {Aliases: []string{"no-space-in-code"}, Tags: []string{"whitespace", "code"}},
	"MD039": {Aliases: []string{"no-space-in-links"}, Tags: []string{"whitespace", "links"}},
	"MD040": {Aliases: []string{"fenced-code-language"}, Tags: []string{"code", "language"}},
	"MD041": {Aliases: []string{"first-line-heading", "first-line-h1"}, Tags: []string{"headings"}},
	"MD042": {Aliases: []string{"no-empty-links"}, Tags: []string{"links"}},
	"MD043": {Aliases: []string{"required-headings"}, Tags: []string{"headings"}},
	"MD044": {Aliases: []string{"proper-names"}, Tags: []string{"spelling"}},
	"MD045": {Aliases: []string{"no-alt-text"}, Tags: []string{"accessibility", "images"}},
	"MD046": {Aliases: []string{"code-block-style"}, Tags: []string{"code"}},
	"MD047": {Aliases: []string{"single-trailing-newline"}, Tags: []string{"blank_lines"}},
	"MD048": {Aliases: []string{"code-fence-style"}, Tags: []string{"code"}},
	"MD049": {Aliases: []string{"emphasis-style"}, Tags: []string{"emphasis"}},
	"MD050": {Aliases: []string{"strong-style"}, Tags: []string{"emphasis"}},
	"MD051": {Aliases: []string{"link-fragments"}, Tags: []string{"links"}},
	"MD052": {Aliases: []string{"reference-links-images"}, Tags: []string{"images", "links"}},
	"MD053": {Aliases: []string{"link-image-reference-definitions"}, Tags: []string{"images", "links"}},
	"MD054": {Aliases: []string{"link-image-style"}, Tags: []string{"images", "links"}},
	"MD055": {Aliases: []string{"table-pipe-style"}, Tags: []string{"table"}},
	"MD056": {Aliases: []string{"table-column-count"}, Tags: []string{"table"}},
	"MD058": {Aliases: []string{"blanks-around-tables"}, Tags: []string{"table"}},
	"MD059": {Aliases: []string{"descriptive-link-text"}, Tags: []string{"accessibility", "links"}},
	"MD060": {Aliases: []string{"table-column-style"}, Tags: []string{"table"}},
}
