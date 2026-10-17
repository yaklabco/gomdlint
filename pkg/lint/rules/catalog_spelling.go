package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// spellingCatalog lists the rules that check spelling.
func spellingCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD044", Name: "proper-names", Tags: []string{"spelling"}, Fixable: true},
	}
}
