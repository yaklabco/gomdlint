package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// codeCatalog lists the rules that check code blocks and code spans.
func codeCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD014", Name: "commands-show-output", Tags: []string{"code"}, Fixable: true},
		{ID: "MD031", Name: "blanks-around-fences", Tags: []string{"code", "blank_lines"}, Fixable: true},
		{ID: "MD038", Name: "no-space-in-code", Tags: []string{"whitespace", "code"}, Fixable: true},
		{ID: "MD040", Name: "fenced-code-language", Tags: []string{"code", "language"}, Fixable: true},
		{ID: "MD046", Name: "code-block-style", Tags: []string{"code"}},
		{ID: "MD048", Name: "code-fence-style", Tags: []string{"code"}, Fixable: true},
	}
}
