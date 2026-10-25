package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// listsCatalog lists the rules that check lists.
func listsCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"}, MarkdownlintName: "ul-style",
			Tags: []string{"bullet", "ul"}, Fixable: true,
			RuleTags:    []string{"lists", "style"},
			Summary:     "Unordered list style should be consistent",
			Description: "Every bullet uses the same marker as the first bullet in the file.",
			Rationale:   "Mixed markers make the source harder to read. Inside a list, a different marker also starts a new list.",
			Fix: "The fix changes each marker to the expected one. gomdlint reports a list without a fix " +
				"when the new marker would merge it with a list next to it. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD005", Name: "list-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true,
			RuleTags:    []string{"bullet", "indentation", "ul"},
			Summary:     "Inconsistent indentation for list items at the same level",
			Description: "Items in a list start at the same column. Ordered numbers may right-align.",
			Rationale:   "Misaligned items can make later lines attach to the wrong item.",
			Fix: "The fix moves the items of an ordered list into line. gomdlint reports bullet items without a fix, " +
				"and some others, such as items that hold a second paragraph, a code block or a quote. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD007", Name: "ul-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true,
			RuleTags:    []string{"bullet", "indentation", "ul"},
			Summary:     "Unordered list indentation",
			Description: "Outside ordered lists, bullets sit at the margin and indent 2 spaces per level.",
			Rationale:   "Two spaces line up a nested bullet with the text of the bullet above it.",
			Fix: "The fix re-indents the bullet. gomdlint reports some items without a fix, " +
				"such as one that holds a second paragraph, a code block or a quote. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD029", Name: "ol-prefix", Tags: []string{"ol"}, Fixable: true,
			Summary:     "Ordered list item prefix",
			Description: "Ordered list items are numbered 1, 1, 1, or 1, 2, 3, or 0, 1, 2, and so on.",
			Rationale: "Renderers number the items themselves and use only the first number, " +
				"so other numbers in the source can disagree with the output.",
			Fix: "The fix renumbers the items. gomdlint reports a list without a fix when renumbering it would change its first number, " +
				"which sets where the rendered list starts. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD030", Name: "list-marker-space", Tags: []string{"ol", "ul", "whitespace"}, Fixable: true,
			Summary:     "Spaces after list markers",
			Description: "One space, or one tab, separates each list marker from the item's text.",
			Rationale: "The spaces after a marker set the column that the item's later lines must reach. " +
				"Extra spaces make that column easy to miss.",
			Fix: "The fix sets the spacing to the expected number of spaces. gomdlint reports some items without a fix, " +
				"such as one that holds a second paragraph, a code block or a quote. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD032", Name: "blanks-around-lists", Tags: []string{"bullet", "ul", "ol", "blank_lines"}, Fixable: true,
			RuleTags:    []string{"blank_lines", "bullet", "ol", "ul"},
			Summary:     "Lists should be surrounded by blank lines",
			Description: "Lists outside other lists have a blank or comment line above and below.",
			Rationale:   "Some parsers, such as kramdown, do not recognize a list without the blank lines.",
			Fix: "The fix inserts the missing blank line, keeping any blockquote markers. " +
				noFixNearCheckbox + " " + noFixWithRawHTML,
		},
	}
}
