package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// layoutCatalog lists the rules that check spaces, tabs, blank lines, line
// length, horizontal rules and the end of the file.
func layoutCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD009", Name: "no-trailing-spaces", Tags: []string{"whitespace"}, Fixable: true,
			Description: "Outside code blocks, no line ends in spaces or tabs, except 2 for a line break.",
			Rationale: "Trailing whitespace in text does not change the output, except where 2 or more spaces " +
				"make a hard line break. In code blocks it is part of the code.",
			Fix: "The fix deletes the trailing whitespace, or trims a hard line break to exactly 2 spaces. " + noFixWithRawHTML,
		},
		{
			ID: "MD010", Name: "no-hard-tabs", Tags: []string{"whitespace", "hard_tab"}, Fixable: true,
			Description: "Lines contain no tab characters, including inside code.",
			Rationale: "Editors and tools show tabs at different widths, such as 4 or 8 spaces, " +
				"so text aligned with tabs lines up differently in different tools.",
			Fix: "The fix replaces each tab with spaces. gomdlint reports tabs inside code without a fix, " +
				"because replacing them would change the code. " + noFixWithRawHTML,
		},
		{
			ID: "MD012", Name: "no-multiple-blank-lines", Aliases: []string{"no-multiple-blanks"}, MarkdownlintName: "no-multiple-blanks",
			Tags: []string{"whitespace", "blank_lines"}, Fixable: true,
			Description: "Outside code, no two blank lines follow each other.",
			Rationale:   "Extra blank lines outside code do not change the output.",
			Fix:         "The fix deletes the extra blank lines. " + noFixWithRawHTML,
		},
		{
			ID: "MD013", Name: "line-length", Tags: []string{"line_length"}, Fixable: true,
			Description: "Lines fit in 80 characters, with exemptions such as lone links and definitions.",
			Rationale: "Long lines are hard to work with in some editors. Diffs of short lines are also easier to read. " +
				"The exemptions are reference definitions, lines that hold only a link or an image, " +
				"and lines whose only overflow is the last word, such as a URL. " +
				"They often cannot be split without breaking a URL.",
			Fix: "The fix wraps long paragraph lines at spaces. gomdlint reports a line without a fix " +
				"when it is a heading, a table row or code, or when no break point is safe. " + noFixWithRawHTML,
		},
		{
			ID: "MD035", Name: "hr-style", Tags: []string{"hr"}, Fixable: true,
			Description: "Every horizontal rule uses the same characters as the first one, such as ---.",
			Rationale:   "One way of writing horizontal rules makes them easy to recognize in the source.",
			Fix: "The fix rewrites each horizontal rule in the expected style. gomdlint reports a horizontal rule " +
				"without a fix when the change would turn the line above it into a heading.",
		},
		{
			ID: "MD047", Name: "single-trailing-newline", Tags: []string{"blank_lines"}, Fixable: true,
			Description: "The file ends with a newline, or a last line of only spaces, > or comments.",
			Rationale: "Some programs mishandle a last line that has no newline. " +
				"A last line with only spaces, > or a comment counts as blank, as in markdownlint.",
			Fix: "The fix adds a newline at the end of the file. " + noFixWithRawHTML,
		},
	}
}
