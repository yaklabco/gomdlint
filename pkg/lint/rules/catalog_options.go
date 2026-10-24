package rules

import (
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// Values that several options share.
const (
	// defaultSpacesPerLevel is the default indentation of a nested list
	// level, in MD007.
	defaultSpacesPerLevel = 2

	// defaultMinDashes is MDL003's default for the dashes of a delimiter
	// cell.
	defaultMinDashes = 3
)

// catalogOptions returns the options of every built-in rule, keyed by rule
// ID: each option the rule reads, with its canonical name, markdownlint's
// spelling where that differs, its type, its default and the values it
// allows. A rule that reads no options has an empty list, so that every
// rule declares its options (RuleMeta.OptionsDeclared). Catalog joins them
// to the entries of the catalog_<category>.go files.
//
// Every default equals the rule's own, which matches markdownlint 0.41.
// Options that markdownlint lacks are gomdlint-only, and their defaults
// keep markdownlint's behaviour. The options that gomdlint's translation of
// markdownlint settings has always written out, for MD001, MD004, MD009,
// MD013, MD022, MD025, MD029, MD033, MD046, MD048, MD059 and MD060, are
// marked MarkdownlintPinned, so that a markdownlint setting that resets no
// options, such as the default key, still replaces them in a lower
// configuration layer.
//
//nolint:funlen,maintidx // One entry per rule: a table, not logic.
func catalogOptions() map[string][]lint.OptionSpec {
	return map[string][]lint.OptionSpec{
		"MD001": {markdownlintPinned(frontMatterTitleOption(
			"counts as the top-level heading, so the first heading may be a level 2 heading"))},
		"MD003": {
			{
				Name: "style", Type: lint.OptionString, Default: string(StyleConsistent),
				Values:      []string{"consistent", "atx", "atx_closed", "setext", "setext_with_atx", "setext_with_atx_closed"},
				Description: "Heading syntax to require. consistent takes the syntax of the first heading.",
			},
			{
				Name: "require_closing_atx", Type: lint.OptionBoolean, Default: false, GomdlintOnly: true,
				Description: "Expect closed headings, such as # Title #, wherever the style expects ATX headings.",
			},
		},
		"MD004": {{
			Name: "style", Type: lint.OptionString, Default: string(BulletConsistent), MarkdownlintPinned: true,
			Values: []string{"consistent", "asterisk", "plus", "dash", "sublist"},
			Description: "Bullet to require. consistent takes the bullet of the first list, and sublist " +
				"gives each nesting level a bullet that differs from its parent level's.",
		}},
		"MD005": {},
		"MD007": {
			{
				Name: "indent", Type: lint.OptionInteger, Default: defaultSpacesPerLevel, MarkdownlintZeroIsDefault: true,
				Description: "Spaces of indentation for each level of a nested list.",
			},
			{
				Name: "start_indented", Type: lint.OptionBoolean, Default: false,
				Description: "Indent the first level of a list too.",
			},
			{
				Name: "start_indent", Type: lint.OptionInteger, Default: defaultSpacesPerLevel, DefaultFrom: "indent", MarkdownlintZeroIsDefault: true,
				Description: "Spaces of indentation for the first level when start_indented is set.",
			},
		},
		"MD009": {
			{
				Name: "br_spaces", Type: lint.OptionInteger, Default: defaultBreakSpaces, MarkdownlintPinned: true,
				Description: "Trailing spaces allowed for a hard line break. Values below 2 allow none.",
			},
			{
				Name: "list_item_empty_lines", Type: lint.OptionBoolean, Default: false,
				Description: "Allow trailing spaces on empty lines inside list items.",
			},
			{
				Name: "strict", Type: lint.OptionBoolean, Default: false,
				Description: "Also report br_spaces where they make no line break, such as at the end of a paragraph.",
			},
			{
				Name: "ignore_code_blocks", Aliases: []lint.OptionAlias{{Name: "code_blocks", Inverted: true}},
				Type: lint.OptionBoolean, Default: true, MarkdownlintPinned: true,
				Description: "Skip the lines of code blocks.",
			},
		},
		"MD010": {
			{
				Name: "code_blocks", Type: lint.OptionBoolean, Default: true,
				Description: "Check code blocks and code spans too.",
			},
			{
				Name: "ignore_code_languages", Type: lint.OptionStringList, Default: []string{},
				Description: "Languages of fenced code blocks to skip, matched against the first word of the info string " +
					"in any case. When it names any language, fences that name none are skipped too.",
			},
			{
				Name: "spaces_per_tab", Type: lint.OptionInteger, Default: 1,
				Description: "Spaces that replace each tab in a fix.",
			},
		},
		"MD011": {},
		"MD012": {{
			Name: "max_consecutive", Aliases: []lint.OptionAlias{{Name: "maximum"}}, Type: lint.OptionInteger, Default: 1,
			Limit: true, ZeroIsDefault: true,
			Description: "Most blank lines allowed in a row. Values below 1 count as 1.",
		}},
		"MD013": {
			{
				Name: "max", Aliases: []lint.OptionAlias{{Name: "line_length"}}, Type: lint.OptionInteger,
				Default: defaultMaxLineLength, Limit: true, ZeroIsDefault: true, MarkdownlintZero: -1, MarkdownlintPinned: true,
				Description: "Most characters allowed on a line. 0 counts as 80, as in markdownlint.",
			},
			{
				Name: "heading_max", Aliases: []lint.OptionAlias{{Name: "heading_line_length"}}, Type: lint.OptionInteger,
				Default: defaultMaxLineLength, DefaultFrom: "max", Limit: true, ZeroIsDefault: true, MarkdownlintZero: -1,
				Description: "Most characters allowed on a heading line. 0 counts as max.",
			},
			{
				Name: "code_block_max", Aliases: []lint.OptionAlias{{Name: "code_block_line_length"}}, Type: lint.OptionInteger,
				Default: defaultMaxLineLength, DefaultFrom: "max", Limit: true, ZeroIsDefault: true, MarkdownlintZero: -1,
				Description: "Most characters allowed on a line of a code block. 0 counts as max.",
			},
			{
				Name: "ignore_code_blocks", Aliases: []lint.OptionAlias{{Name: "code_blocks", Inverted: true}},
				Type: lint.OptionBoolean, Default: false, MarkdownlintPinned: true, Description: "Skip code blocks.",
			},
			{
				Name: "ignore_tables", Aliases: []lint.OptionAlias{{Name: "tables", Inverted: true}},
				Type: lint.OptionBoolean, Default: false, Description: "Skip tables.",
			},
			{
				Name: "ignore_headings", Aliases: []lint.OptionAlias{{Name: "headings", Inverted: true}},
				Type: lint.OptionBoolean, Default: false, Description: "Skip headings.",
			},
			{
				Name: "ignore_urls", Type: lint.OptionBoolean, Default: false, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "Skip lines that contain a URL.",
			},
			{
				Name: "strict", Type: lint.OptionBoolean, Default: false,
				Description: "Report every line over the limit, also one whose only overflow is its last word, " +
					"and one that holds only a link.",
			},
			{
				Name: "stern", Type: lint.OptionBoolean, Default: false,
				Description: "Also report a line whose only overflow is its last word, " +
					"unless the line has no space to wrap at.",
			},
			{
				Name: "markdownlint_compatible", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "Allow what markdownlint allows over the limit: reference definitions, lines that hold " +
					"only a link, and a last word that starts within the limit. With false, every line over the limit is reported.",
			},
		},
		"MD014": {},
		"MD018": {},
		"MD019": {},
		"MD020": {},
		"MD021": {},
		"MD022": {
			{
				Name: "lines_above", Type: lint.OptionIntegerOrList, Default: 1,
				Description: "Blank lines required above a heading, or a list with one value for each heading level. " +
					"0 or -1 requires none.",
			},
			{
				Name: "lines_below", Type: lint.OptionIntegerOrList, Default: 1,
				Description: "Blank lines required below a heading, or a list with one value for each heading level. " +
					"0 or -1 requires none.",
			},
			{
				Name: "include_front_matter", Type: lint.OptionBoolean, Default: false,
				Description: "Require blank lines above a heading that directly follows front matter.",
			},
			{
				Name: "markdownlint_compatible", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "Require blank lines between adjacent headings, as markdownlint does. " +
					"With false, a heading may touch another heading.",
			},
		},
		"MD023": {},
		"MD024": {{
			Name: "siblings_only", Type: lint.OptionBoolean, Default: false,
			Description: "Report a duplicate only among headings that share a parent heading.",
		}},
		"MD025": {
			{
				Name: "level", Type: lint.OptionInteger, Default: 1, MarkdownlintZeroIsDefault: true,
				Description: "Level of the title heading.",
			},
			frontMatterTitleOption("counts as the title, so every heading of the title's level is reported"),
			{
				Name: "title_only", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "Report more title headings only when the first one is the document's title, " +
					"its first content. With false, every title heading after the first is reported.",
			},
			{
				Name: "allow_no_h1", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true,
				Description: "Allow a document without a title heading. With false, such a document is reported.",
			},
		},
		"MD026": {{
			Name: "punctuation", Type: lint.OptionString, Default: defaultPunctuation,
			Description: "Characters that a heading may not end with.",
		}},
		"MD027": {{
			Name: "list_items", Type: lint.OptionBoolean, Default: true,
			Description: "Check the space after > in front of list items too.",
		}},
		"MD028": {},
		"MD029": {
			{
				Name: "style", Type: lint.OptionString, Default: orderedStyleOneOrOrdered, MarkdownlintPinned: true,
				Values: []string{"one", "ordered", "one_or_ordered", "zero"},
				Description: "Numbering to require: all ones, increasing numbers, all zeros, " +
					"or one_or_ordered, which accepts either of the first two.",
			},
			{
				Name: "allow_renumbering", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true,
				Description: "Fix a wrong number. With false, wrong numbers are reported without a fix.",
			},
		},
		"MD030": {
			listMarkerSpaceOption("ul_single", "bullet list whose items each take one line"),
			listMarkerSpaceOption("ol_single", "ordered list whose items each take one line"),
			listMarkerSpaceOption("ul_multi", "bullet list with an item of several lines"),
			listMarkerSpaceOption("ol_multi", "ordered list with an item of several lines"),
		},
		"MD031": {{
			Name: "list_items", Type: lint.OptionBoolean, Default: true,
			Description: "Check fenced code blocks in list items too.",
		}},
		"MD032": {},
		"MD033": {
			{
				Name: "allowed_elements", Type: lint.OptionStringList, Default: []string{}, MarkdownlintPinned: true,
				Description: "HTML elements to allow, in any case.",
			},
			{
				Name: "table_allowed_elements", Type: lint.OptionStringList, Default: []string{}, DefaultFrom: "allowed_elements",
				Description: "HTML elements to allow in tables, in any case.",
			},
		},
		"MD034": {},
		"MD035": {{
			Name: "style", Type: lint.OptionString, Default: styleConsistent,
			Description: "Horizontal rule to require, such as ---. consistent takes the style of the first one.",
		}},
		"MD036": {{
			Name: "punctuation", Type: lint.OptionString, Default: defaultEmphasisPunctuation,
			Description: "Characters that end a line of emphasis that reads as a sentence, which is not reported.",
		}},
		"MD037": {},
		"MD038": {},
		"MD039": {},
		"MD040": {
			{
				Name: "allowed_languages", Type: lint.OptionStringList, Default: []string{},
				Description: "Languages to allow. An empty list allows every language.",
			},
			{
				Name: "language_only", Type: lint.OptionBoolean, Default: false,
				Description: "Report an info string that holds more than the language.",
			},
		},
		"MD041": {
			{
				Name: "level", Type: lint.OptionInteger, Default: 1, MarkdownlintZeroIsDefault: true,
				Description: "Level that the first heading must have.",
			},
			frontMatterTitleOption("counts as the first heading"),
			{
				Name: "allow_preamble", Type: lint.OptionBoolean, Default: false,
				Description: "Allow content before the first heading.",
			},
		},
		"MD042": {},
		"MD043": {
			{
				Name: "headings", Type: lint.OptionStringList, Default: []string{},
				Description: "Headings the document must have, in order, such as ## Usage. " +
					"* stands for any number of headings, + for one or more, and ? for exactly one.",
			},
			{
				Name: "match_case", Type: lint.OptionBoolean, Default: false,
				Description: "Compare the text of headings in the same case.",
			},
		},
		"MD044": {
			{
				Name: "names", Type: lint.OptionStringList, Default: []string{},
				Description: "Names to check, in the capitalization they require.",
			},
			{
				Name: "code_blocks", Type: lint.OptionBoolean, Default: true,
				Description: "Check code blocks and code spans too.",
			},
			{
				Name: "html_elements", Type: lint.OptionBoolean, Default: true,
				Description: "Check HTML elements too.",
			},
		},
		"MD045": {},
		"MD046": {{
			Name: "style", Type: lint.OptionString, Default: string(CodeBlockConsistent), MarkdownlintPinned: true,
			Values:      []string{"consistent", "fenced", "indented"},
			Description: "Code block style to require. consistent takes the style of the first block.",
		}},
		"MD047": {{
			Name: "max_trailing_blank_lines", Type: lint.OptionInteger, Default: nil, GomdlintOnly: true,
			Description: "Most blank lines allowed at the end of the file. Unset, any number is allowed.",
		}},
		"MD048": {{
			Name: "style", Type: lint.OptionString, Default: string(FenceConsistent), MarkdownlintPinned: true,
			Values:      []string{"consistent", "backtick", "tilde"},
			Description: "Fence character to require. consistent takes the fence of the first block.",
		}},
		"MD049": {emphasisStyleOption("emphasis")},
		"MD050": {emphasisStyleOption("strong emphasis")},
		"MD051": {
			{
				Name: "ignore_case", Type: lint.OptionBoolean, Default: false,
				Description: "Match fragments to anchors in any case.",
			},
			{
				Name: "ignored_pattern", Type: lint.OptionString, Default: "",
				Description: "Regular expression for fragments that are not checked. Empty checks every fragment.",
			},
		},
		"MD052": {
			{
				Name: "ignored_labels", Type: lint.OptionStringList, Default: []string{"x"},
				Description: "Labels that need no definition. The default, x, keeps task list items such as [x] from being reported.",
			},
			{
				Name: "shortcut_syntax", Type: lint.OptionBoolean, Default: false,
				Description: "Check shortcut references, such as [label], too.",
			},
		},
		"MD053": {{
			Name: "ignored_definitions", Type: lint.OptionStringList, Default: []string{"//"},
			Description: "Labels of definitions that may go unused. The default, //, allows comments written as [//]: # (text).",
		}},
		"MD054": {
			linkStyleOption("autolink", "autolinks, such as <https://example.com>"),
			linkStyleOption("inline", "inline links and images, such as [text](url)"),
			linkStyleOption("full", "full reference links and images, such as [text][label]"),
			linkStyleOption("collapsed", "collapsed reference links and images, such as [label][]"),
			linkStyleOption("shortcut", "shortcut reference links and images, such as [label]"),
			linkStyleOption("url_inline", "inline links whose text is their URL, such as [https://example.com](https://example.com)"),
		},
		"MD055": {{
			Name: "style", Type: lint.OptionString, Default: string(PipeStyleConsistent),
			Values: []string{"consistent", "leading_only", "trailing_only", "leading_and_trailing", "no_leading_or_trailing"},
			Description: "Pipes to require at the start and end of table rows. " +
				"consistent takes those of the first row.",
		}},
		"MD056": {},
		"MD058": {},
		"MD059": {
			{
				Name: "patterns", Aliases: []lint.OptionAlias{{Name: "prohibited_texts"}}, Type: lint.OptionStringList,
				Default: getDefaultGenericPatterns(), MarkdownlintPinned: true,
				Description: "Link texts to report, compared in any case.",
			},
			{
				Name: "markdownlint_compatible", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "Compare link text as markdownlint does, ignoring punctuation, " +
					"and skip links whose text holds code or HTML.",
			},
		},
		"MD060": {
			{
				Name: "style", Type: lint.OptionString, Default: string(ColumnStyleAny),
				Values: []string{"any", "aligned", "compact", "tight"},
				Description: "Cell padding to require. any accepts whichever of aligned, compact and tight " +
					"a table comes closest to.",
			},
			{
				Name: "aligned_delimiter", Type: lint.OptionBoolean, Default: false,
				Description: "With the compact or tight style, require the pipes of the delimiter row " +
					"to line up with those of the header row.",
			},
			{
				Name: "markdownlint_compatible", Type: lint.OptionBoolean, Default: true, GomdlintOnly: true, MarkdownlintPinned: true,
				Description: "With the style any, report padding that matches no style, as markdownlint does. " +
					"With false, any padding is allowed.",
			},
		},
		"MDL001": {{
			Name: "style", Type: lint.OptionString, Default: string(LinkDestConsistent), GomdlintOnly: true,
			Values: []string{"consistent", "relative", "absolute"},
			Description: "Kind of link destination to require: relative paths or absolute URLs. " +
				"Only relative links and file: URLs that name no other host count. " +
				"consistent takes the kind of the first link that counts.",
		}},
		"MDL003": {{
			Name: "min_dashes", Type: lint.OptionInteger, Default: defaultMinDashes, GomdlintOnly: true,
			Description: "Fewest dashes allowed in a cell of a table's delimiter row.",
		}},
	}
}

// markdownlintPinned marks spec as an option that the markdownlint settings
// of its rule that reset no options write out (see
// lint.OptionSpec.MarkdownlintPinned).
func markdownlintPinned(spec lint.OptionSpec) lint.OptionSpec {
	spec.MarkdownlintPinned = true
	return spec
}

// frontMatterTitleOption is the front_matter_title option of MD001, MD025
// and MD041, whose front matter title counts as role describes.
func frontMatterTitleOption(role string) lint.OptionSpec {
	return lint.OptionSpec{
		Name: "front_matter_title", Type: lint.OptionStringOrFalse, Default: config.MarkdownlintFrontMatterTitlePattern,
		Description: "Regular expression, matched in any case, for the front matter line that gives the document " +
			"a title, which " + role + ". An empty string or false turns this off.",
	}
}

// listMarkerSpaceOption is an option of MD030 for the spaces after the
// markers of a list.
func listMarkerSpaceOption(name, list string) lint.OptionSpec {
	return lint.OptionSpec{
		Name: name, Type: lint.OptionInteger, Default: 1, ZeroIsDefault: true,
		Description: "Spaces after the marker of each item of a " + list + ". 0 counts as 1, as in markdownlint.",
	}
}

// emphasisStyleOption is the style option of MD049 or MD050, which check
// the markers of the given kind of emphasis.
func emphasisStyleOption(kind string) lint.OptionSpec {
	return lint.OptionSpec{
		Name: "style", Type: lint.OptionString, Default: styleConsistent,
		Values:      []string{"consistent", "asterisk", "underscore"},
		Description: "Marker of " + kind + " to require. consistent takes the marker of the first " + kind + ".",
	}
}

// linkStyleOption is an option of MD054, which allows the links written in
// one style.
func linkStyleOption(name, style string) lint.OptionSpec {
	return lint.OptionSpec{
		Name: name, Type: lint.OptionBoolean, Default: true,
		Description: "Allow " + style + ".",
	}
}
