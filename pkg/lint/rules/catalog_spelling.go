package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// spellingCatalog lists the rules that check spelling.
func spellingCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD044", Name: "proper-names", Tags: []string{"spelling"}, Fixable: true,
			Description: "Outside URLs, proper names match the configured capitals, none by default.",
			Rationale: "Wrong capitalization of a proper name, such as Javascript for JavaScript, is usually a mistake. " +
				"As in markdownlint, names in email addresses and fence info strings are not checked either.",
			Fix: "The fix replaces the name with its configured spelling. " +
				"gomdlint reports names inside raw HTML without a fix. " + noFixNearCheckbox,
		},
	}
}
