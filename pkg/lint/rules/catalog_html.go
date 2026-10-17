package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// htmlCatalog lists the rules that check raw HTML.
func htmlCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD033", Name: "no-inline-html", Tags: []string{"html"}},
	}
}
