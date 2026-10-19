package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// spellingCatalog lists the rules that check spelling.
func spellingCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD044", Name: "proper-names", Tags: []string{"spelling"}, Fixable: true,
			Description: "Proper names match the capitals in the configured list, empty by default.",
			Rationale:   "Wrong capitalization of a proper name, such as Javascript for JavaScript, is usually a mistake.",
			Fix: "The fix replaces the name with its configured spelling. " +
				"gomdlint reports names inside raw HTML without a fix. " + noFixNearCheckbox,
		},
	}
}
