package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// headingsCatalog lists the rules that check headings.
func headingsCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD001", Name: "heading-increment", Tags: []string{"headings"}},
		{ID: "MD003", Name: "heading-style", Tags: []string{"headings"}, Fixable: true},
		{ID: "MD018", Name: "no-missing-space-atx", Tags: []string{"headings", "atx", "spaces"}, Fixable: true},
		{ID: "MD019", Name: "no-multiple-space-atx", Tags: []string{"headings", "atx", "spaces"}, Fixable: true},
		{ID: "MD020", Name: "no-missing-space-closed-atx", Tags: []string{"headings", "atx_closed", "spaces"}, Fixable: true},
		{ID: "MD021", Name: "no-multiple-space-closed-atx", Tags: []string{"headings", "atx_closed", "spaces"}, Fixable: true},
		{
			ID: "MD022", Name: "heading-blank-lines", Aliases: []string{"blanks-around-headings"}, MarkdownlintName: "blanks-around-headings",
			Tags: []string{"headings", "blank_lines"}, Fixable: true,
		},
		{ID: "MD023", Name: "heading-start-left", Tags: []string{"headings", "spaces"}, Fixable: true},
		{ID: "MD024", Name: "no-duplicate-heading", Tags: []string{"headings"}},
		{ID: "MD025", Name: "single-h1", Aliases: []string{"single-title"}, MarkdownlintName: "single-title", Tags: []string{"headings"}},
		{ID: "MD026", Name: "no-trailing-punctuation", Tags: []string{"headings"}, Fixable: true},
		{ID: "MD036", Name: "no-emphasis-as-heading", Tags: []string{"headings", "emphasis"}, Fixable: true},
		{ID: "MD041", Name: "first-line-heading", Aliases: []string{"first-line-h1"}, Tags: []string{"headings"}},
		{ID: "MD043", Name: "required-headings", Tags: []string{"headings"}},
	}
}
