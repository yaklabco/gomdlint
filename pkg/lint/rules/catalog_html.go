package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// htmlCatalog lists the rules that check raw HTML.
func htmlCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD033", Name: "no-inline-html", Tags: []string{"html"},
			Description: "Documents contain no raw HTML elements other than allowed ones.",
			Rationale:   "Raw HTML is not plain Markdown, and it does not carry over to output formats other than HTML.",
			Fix:         "There is no automatic fix.",
		},
	}
}
