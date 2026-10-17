package rules

import "github.com/yaklabco/gomdlint/pkg/lint"

// tablesCatalog lists the rules that check tables.
func tablesCatalog() []lint.RuleMeta {
	return []lint.RuleMeta{
		{ID: "MD055", Name: "table-pipe-style", Tags: []string{"table"}},
		{ID: "MD056", Name: "table-column-count", Tags: []string{"table"}},
		{ID: "MD058", Name: "blanks-around-tables", Tags: []string{"table"}, Fixable: true},
		{ID: "MD060", Name: "table-column-style", Tags: []string{"table"}},
		{ID: "MDL003", Name: "table-alignment", Tags: []string{"table"}, Fixable: true},
	}
}
