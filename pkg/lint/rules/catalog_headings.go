package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// headingsCatalog lists the rules that check headings.
func headingsCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD001", Name: "heading-increment", Tags: []string{"headings"},
			Summary:     "Heading levels should only increment by one level at a time",
			Description: "Each heading is at most one level deeper than the heading or title before it.",
			Rationale: "Headings form the outline of a document. A skipped level confuses readers, " +
				"especially people who use a screen reader to move between headings.",
			Fix: "There is no automatic fix. Only the author knows which level was meant.",
		},
		{
			ID: "MD003", Name: "heading-style", Tags: []string{"headings"}, Fixable: true,
			RuleTags:    []string{"headings", "style"},
			Summary:     "Heading style should be consistent",
			Description: "Headings all use the first heading's style: # Title, # Title #, or underlined.",
			Rationale:   "One heading style makes the source easier to read and to search.",
			Fix: "The fix converts headings between the # Title and # Title # styles. " +
				"gomdlint reports an underlined heading, or one that needs an underline, without a fix. " + noFixNearCheckbox,
		},
		{
			ID: "MD018", Name: "no-missing-space-atx", Tags: []string{"headings", "atx", "spaces"}, Fixable: true,
			RuleTags:    []string{"atx", "headings", "spaces"},
			Summary:     "No space after hash on ATX style heading",
			Description: "A space follows the # marks that open a heading at the start of a line.",
			Rationale:   "Without the space, a line such as #Heading renders as a paragraph.",
			Fix: "The fix inserts a space after the # marks. gomdlint reports the line without a fix " +
				"when the document may link to heading anchors, because a new heading adds an anchor. " +
				"It also reports a line that starts with seven or more # marks without a fix, " +
				"because a space cannot make it a heading. " + noFixNearCheckbox,
		},
		{
			ID: "MD019", Name: "no-multiple-space-atx", Tags: []string{"headings", "atx", "spaces"}, Fixable: true,
			RuleTags:    []string{"atx", "headings", "spaces"},
			Summary:     "Multiple spaces after hash on ATX style heading",
			Description: "Only one space follows the # marks that open a heading.",
			Rationale:   "The extra spaces have no purpose, because they do not change the rendered heading.",
			Fix:         "The fix reduces the spaces to one. " + noFixNearCheckbox,
		},
		{
			ID: "MD020", Name: "no-missing-space-closed-atx", Tags: []string{"headings", "atx_closed", "spaces"}, Fixable: true,
			RuleTags:    []string{"atx_closed", "headings", "spaces"},
			Summary:     "No space inside hashes on closed ATX style heading",
			Description: "A closed heading at the start of a line has a space inside each run of #.",
			Rationale:   "Without the spaces, the closing # marks show as heading text, or the line is not a heading at all.",
			Fix: "The fix inserts the missing spaces. gomdlint reports the line without a fix " +
				"when the document may link to heading anchors, because the fix may add or change an anchor. " + noFixNearCheckbox,
		},
		{
			ID: "MD021", Name: "no-multiple-space-closed-atx", Tags: []string{"headings", "atx_closed", "spaces"}, Fixable: true,
			RuleTags:    []string{"atx_closed", "headings", "spaces"},
			Summary:     "Multiple spaces inside hashes on closed ATX style heading",
			Description: "A closed heading has only one space inside each run of #.",
			Rationale:   "The extra spaces have no purpose, because they do not change the rendered heading.",
			Fix:         "The fix reduces each run of spaces to one. " + noFixNearCheckbox,
		},
		{
			ID: "MD022", Name: "heading-blank-lines", Aliases: []string{"blanks-around-headings"}, MarkdownlintName: "blanks-around-headings",
			Tags: []string{"headings", "blank_lines"}, Fixable: true,
			RuleTags:    []string{"headings", "whitespace", "blank_lines"},
			Summary:     "Headings should be surrounded by blank lines",
			Description: "Headings have a blank or comment line above and below.",
			Rationale: "Blank lines set headings apart in the source. " +
				"Some parsers, such as kramdown, read a heading with no blank line before it as plain text. " +
				"As in markdownlint, a heading right after front matter needs none above it.",
			Fix: "The fix inserts the missing blank lines, keeping any blockquote markers. " +
				"gomdlint reports a heading in a tight list without a fix when the blank line would make the list loose. " + noFixNearCheckbox,
		},
		{
			ID: "MD023", Name: "heading-start-left", Tags: []string{"headings", "spaces"}, Fixable: true,
			Summary:     "Headings must start at the beginning of the line",
			Description: "Headings start at the beginning of their line, list item or blockquote.",
			Rationale:   "Some parsers do not recognize an indented heading and show it as plain text.",
			Fix:         "The fix deletes the extra indentation. " + noFixNearCheckbox,
		},
		{
			ID: "MD024", Name: "no-duplicate-heading", Tags: []string{"headings"},
			Summary:     "Multiple headings with the same content",
			Description: "No two headings have exactly the same text.",
			Rationale: "Some renderers, such as GitHub, build link anchors from heading text. " +
				"Repeated headings get numbered anchors, so a link that uses the text alone reaches only the first one.",
			Fix: "There is no automatic fix. Only the author can choose new heading text.",
		},
		{
			ID: "MD025", Name: "single-h1", Aliases: []string{"single-title"}, MarkdownlintName: "single-title",
			Tags:        []string{"headings"},
			Summary:     "Multiple top-level headings in the same document",
			Description: "A document that starts with a title has no other top-level heading.",
			Rationale:   "The top-level heading at the start is the document's title, and a document has only one title.",
			Fix:         "There is no automatic fix. Only the author knows which heading should stay the title.",
		},
		{
			ID: "MD026", Name: "no-trailing-punctuation", Tags: []string{"headings"}, Fixable: true,
			Summary:     "Trailing punctuation in heading",
			Description: "Headings do not end with a period, comma, semicolon, colon or exclamation mark.",
			Rationale:   "Headings are not full sentences, so they need no closing punctuation.",
			Fix: "The fix deletes the trailing punctuation. gomdlint reports a heading without a fix " +
				"when the deletion would leave it showing nothing. It does the same " +
				"when the document may link to heading anchors, because the fix may change the heading's anchor. " + noFixNearCheckbox,
		},
		{
			ID: "MD036", Name: "no-emphasis-as-heading", Tags: []string{"headings", "emphasis"}, Fixable: true,
			RuleTags:    []string{"emphasis", "headings"},
			Summary:     "Emphasis used instead of a heading",
			Description: "Outside lists and blockquotes, a bold or italic line is not a stand-in heading.",
			Rationale: "Tools read a document's structure from its headings, " +
				"so they miss a section that starts with emphasized text instead.",
			Fix: "The fix turns a bold line into a heading one level deeper than the heading above it, " +
				"at most level 6. With no heading above, it uses level 2. " +
				"gomdlint reports italic lines without a fix. It holds back the fix for bold lines too " +
				"when the document may link to heading anchors. " + noFixNearCheckbox,
		},
		{
			ID: "MD041", Name: "first-line-heading", Aliases: []string{"first-line-h1"}, Tags: []string{"headings"},
			RuleTags:    []string{"headings", "metadata"},
			Summary:     "First line in a file should be a top-level heading",
			Description: "Comments aside, the file starts with a top-level heading or front matter title.",
			Rationale:   "The top-level heading is usually the document's title.",
			Fix:         "There is no automatic fix. Only the author can write the title.",
		},
		{
			ID: "MD043", Name: "required-headings", Tags: []string{"headings"},
			Summary:     "Required heading structure",
			Description: "Headings follow the configured outline in order. No outline is set by default.",
			Rationale:   "Projects can require a set of similar documents, such as runbooks, to share one structure.",
			Fix:         "There is no automatic fix. Only the author can write the missing sections.",
		},
	}
}
