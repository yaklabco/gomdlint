package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// listsCatalog lists the rules that check lists.
func listsCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD004", Name: "unordered-list-style", Aliases: []string{"ul-style"}, Tags: []string{"bullet", "ul"}, Fixable: true},
		{ID: "MD005", Name: "list-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true},
		{ID: "MD007", Name: "ul-indent", Tags: []string{"bullet", "ul", "indentation"}, Fixable: true},
		{ID: "MD029", Name: "ol-prefix", Tags: []string{"ol"}, Fixable: true},
		{ID: "MD030", Name: "list-marker-space", Tags: []string{"ol", "ul", "whitespace"}, Fixable: true},
		{ID: "MD032", Name: "blanks-around-lists", Tags: []string{"bullet", "ul", "ol", "blank_lines"}, Fixable: true},
	}
}
