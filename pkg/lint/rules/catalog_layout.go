package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// layoutCatalog lists the rules that check spaces, tabs, blank lines, line
// length, horizontal rules and the end of the file.
func layoutCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD009", Name: "no-trailing-spaces", Tags: []string{"whitespace"}, Fixable: true},
		{ID: "MD010", Name: "no-hard-tabs", Tags: []string{"whitespace", "hard_tab"}, Fixable: true},
		{
			ID: "MD012", Name: "no-multiple-blank-lines", Aliases: []string{"no-multiple-blanks"}, MarkdownlintName: "no-multiple-blanks",
			Tags: []string{"whitespace", "blank_lines"}, Fixable: true,
		},
		{ID: "MD013", Name: "line-length", Tags: []string{"line_length"}, Fixable: true},
		{ID: "MD035", Name: "hr-style", Tags: []string{"hr"}, Fixable: true},
		{ID: "MD047", Name: "single-trailing-newline", Tags: []string{"blank_lines"}, Fixable: true},
	}
}
