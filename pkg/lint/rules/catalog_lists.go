package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// listsCatalog lists the rules that check lists.
func listsCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"}, MarkdownlintName: "ul-style",
			Tags: []string{"bullet", "ul"}, Fixable: true,
			Description: "Every bullet uses the same marker as the first bullet in the file.",
			Rationale:   "Mixed markers make the source harder to read. Inside a list, a different marker also starts a new list.",
			Fix: "The fix changes each marker to the expected one. gomdlint reports a list without a fix " +
				"when the new marker would merge it with a list next to it.",
		},
		{
			ID: "MD005", Name: "list-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true,
			Description: "Items in a list start at the same column. Ordered numbers may right-align.",
			Rationale:   "Misaligned items can make later lines attach to the wrong item.",
			Fix: "The fix moves the items of an ordered list into line. gomdlint reports bullet items without a fix, " +
				"and items that hold a second paragraph, code or a quote.",
		},
		{
			ID: "MD007", Name: "ul-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true,
			Description: "Bullets start at the margin, and each nested level adds 2 spaces.",
			Rationale:   "Two spaces line up a nested bullet with the text of the item above it.",
			Fix: "The fix re-indents the bullet. gomdlint reports an item without a fix " +
				"when it holds a second paragraph, code or a quote.",
		},
		{
			ID: "MD029", Name: "ol-prefix", Tags: []string{"ol"}, Fixable: true,
			Description: "Ordered list items are all numbered 1, or count up by one from 0 or 1.",
			Rationale: "Renderers number the items themselves and use only the first number, " +
				"so other numbers in the source can disagree with the output.",
			Fix: "The fix renumbers the items. gomdlint reports a list that starts at a number other than 0 or 1 without a fix, " +
				"because renumbering it would change where the rendered list starts.",
		},
		{
			ID: "MD030", Name: "list-marker-space", Tags: []string{"ol", "ul", "whitespace"}, Fixable: true,
			Description: "One space separates each list marker from the item's text.",
			Rationale: "The spaces after a marker set the column that the item's later lines must reach. " +
				"Extra spaces make that column easy to miss.",
			Fix: "The fix sets the spacing to the expected number of spaces. gomdlint reports an item without a fix " +
				"when it holds a second paragraph, code or a quote.",
		},
		{
			ID: "MD032", Name: "blanks-around-lists", Tags: []string{"bullet", "ul", "ol", "blank_lines"}, Fixable: true,
			Description: "Lists have a blank line above and below.",
			Rationale:   "Some parsers, such as kramdown, do not recognize a list without the blank lines.",
			Fix:         "The fix inserts the missing blank line, keeping any blockquote markers.",
		},
	}
}
