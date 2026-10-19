package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// tablesCatalog lists the rules that check tables.
func tablesCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{
			ID: "MD055", Name: "table-pipe-style", Tags: []string{"table"},
			Description: "Table rows use leading and trailing pipes the same way throughout.",
			Rationale: "Consistent pipes make tables easier to read in the source. " +
				"Some parsers also have trouble with tables that leave out the outer pipes.",
			Fix: "There is no automatic fix.",
		},
		{
			ID: "MD056", Name: "table-column-count", Tags: []string{"table"},
			Description: "Every table row has as many cells as the header row.",
			Rationale:   "Renderers drop extra cells, so their data is lost. Missing cells render empty and look like an omission.",
			Fix:         "There is no automatic fix. Only the author knows which cells are missing or extra.",
		},
		{
			ID: "MD058", Name: "blanks-around-tables", Tags: []string{"table"}, Fixable: true,
			Description: "Tables have a blank line above and below.",
			Rationale:   "Some parsers misread a table that has no blank lines around it.",
			Fix: "The fix inserts the missing blank line, keeping any blockquote markers. " +
				"gomdlint reports a table inside a list without a fix when the blank line would fall between two items " +
				"or two blocks of one item. " + noFixNearCheckbox,
		},
		{
			ID: "MD060", Name: "table-column-style", Tags: []string{"table"},
			Description: "Each table follows one style: aligned pipes, one space of padding, or none.",
			Rationale:   "A table that follows one style is easier to read in the source.",
			Fix:         "There is no automatic fix.",
		},
		{
			ID: "MDL003", Name: "table-alignment", Tags: []string{"table"}, Fixable: true,
			Description: "Each cell of a table's delimiter row has at least 3 dashes.",
			Rationale:   "GFM accepts a single dash, but 3 dashes make the row under the header easy to see in the source.",
			Fix:         "The fix lengthens each short run of dashes to the minimum and keeps any colons. " + noFixNearCheckbox,
		},
	}
}
