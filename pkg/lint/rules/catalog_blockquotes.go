package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// blockquotesCatalog lists the rules that check blockquotes.
func blockquotesCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD027", Name: "no-multiple-space-blockquote", Tags: []string{"blockquote", "whitespace", "indentation"}, Fixable: true},
		{ID: "MD028", Name: "no-blanks-blockquote", Tags: []string{"blockquote", "whitespace"}},
	}
}
