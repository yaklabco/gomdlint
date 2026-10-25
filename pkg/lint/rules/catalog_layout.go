package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// layoutCatalog lists the rules that check spaces, tabs, blank lines, line
// length, horizontal rules and the end of the file.
func layoutCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD009", Name: "no-trailing-spaces", Tags: []string{"whitespace"}, Fixable: true,
			Summary:     "Lines should not have trailing spaces",
			Description: "Outside code blocks, lines end in no spaces or tabs, or in exactly 2 of them.",
			Rationale: "Trailing whitespace in text does not change the output, except where 2 or more spaces " +
				"make a hard line break. In code blocks it is part of the code.",
			Fix: "The fix deletes the trailing whitespace, or trims a hard line break to exactly 2 spaces. " +
				"gomdlint reports some trailing whitespace without a fix, such as whitespace inside a code span, which is code, " +
				"or a tab where the paragraph goes on to the next line, which some renderers read as a hard line break. " +
				noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD010", Name: "no-hard-tabs", Tags: []string{"whitespace", "hard_tab"}, Fixable: true,
			RuleTags:    []string{"hard_tab", "whitespace"},
			Summary:     "Hard tabs should not be used",
			Description: "Lines contain no tab characters, including inside code.",
			Rationale: "Editors and tools show tabs at different widths, such as 4 or 8 spaces, " +
				"so text aligned with tabs lines up differently in different tools.",
			Fix: "The fix replaces each tab with spaces. gomdlint reports some tabs without a fix, such as tabs inside code, " +
				"where replacing them would change the code. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD012", Name: "no-multiple-blank-lines", Aliases: []string{"no-multiple-blanks"}, MarkdownlintName: "no-multiple-blanks",
			Tags: []string{"whitespace", "blank_lines"}, Fixable: true,
			RuleTags:    []string{"whitespace", "layout"},
			Summary:     "Multiple consecutive blank lines should be collapsed",
			Description: "Outside code, no two blank lines follow each other, except after front matter.",
			Rationale: "Extra blank lines outside code do not change the output. " +
				"As in markdownlint, the blank lines right after front matter are not checked.",
			Fix: "The fix deletes the extra blank lines. gomdlint reports blank lines inside an HTML comment without a fix. " +
				noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD013", Name: "line-length", Tags: []string{"line_length"}, Fixable: true,
			Summary:     "Line length should not exceed the configured maximum",
			Description: "Lines fit in 80 characters, with exemptions such as lone links and definitions.",
			Rationale: "Long lines are hard to work with in some editors. Diffs of short lines are also easier to read. " +
				"The exemptions are reference definitions, lines that hold only a link or an image, " +
				"and lines whose last word, such as a URL, crosses the limit. " +
				"They often cannot be split without breaking a URL.",
			Fix: "The fix wraps long paragraph lines at spaces. gomdlint reports a line without a fix " +
				"when it is not paragraph text, such as a heading, a table row or code, or when no break point is safe. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
		{
			ID: "MD035", Name: "hr-style", Tags: []string{"hr"}, Fixable: true,
			Summary:     "Horizontal rule style",
			Description: "Every horizontal rule is written the same way as the first one, such as ---.",
			Rationale: "One way of writing horizontal rules makes them easy to recognize in the source. " +
				"Indentation and trailing spaces do not count.",
			Fix: "The fix rewrites each horizontal rule in the expected style. gomdlint reports some horizontal rules " +
				"without a fix, such as one whose change would turn the line above it into a heading. " + noFixNearCheckbox,
		},
		{
			ID: "MD047", Name: "single-trailing-newline", Tags: []string{"blank_lines"}, Fixable: true,
			Summary:     "Files should end with a single newline character",
			Description: "The file ends with a newline, or a last line of only whitespace, > or comments.",
			Rationale: "Some programs mishandle a last line that has no newline. " +
				"A last line with only whitespace, > or a comment counts as blank, as in markdownlint.",
			Fix: "The fix adds a newline at the end of the file. " + noFixNearCheckbox + " " + noFixWithRawHTML,
		},
	}
}
