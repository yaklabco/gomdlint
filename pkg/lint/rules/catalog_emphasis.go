package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// emphasisCatalog lists the rules that check emphasis and strong emphasis.
func emphasisCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD037", Name: "no-space-in-emphasis", Tags: []string{"whitespace", "emphasis"}, Fixable: true},
		{ID: "MD049", Name: "emphasis-style", Tags: []string{"emphasis"}, Fixable: true},
		{ID: "MD050", Name: "strong-style", Tags: []string{"emphasis"}, Fixable: true},
	}
}
