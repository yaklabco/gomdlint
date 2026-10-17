package rules

import (
	"cmp"
	"slices"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// catalogSection is the part of the catalog that one catalog_<category>.go
// file lists.
type catalogSection struct {
	category lint.Category
	entries  []lint.RuleMeta
}

// Catalog returns the catalog entry of every built-in rule, in rule ID
// order: its ID, name, markdownlint aliases, category, markdownlint tags
// and whether it can fix what it finds. Each catalog_<category>.go file
// lists the rules of one category.
func Catalog() []lint.RuleMeta {
	sections := []catalogSection{
		{lint.CategoryHeadings, headingsCatalog()},
		{lint.CategoryLists, listsCatalog()},
		{lint.CategoryLayout, layoutCatalog()},
		{lint.CategoryCode, codeCatalog()},
		{lint.CategoryLinks, linksCatalog()},
		{lint.CategoryTables, tablesCatalog()},
		{lint.CategoryEmphasis, emphasisCatalog()},
		{lint.CategoryBlockquotes, blockquotesCatalog()},
		{lint.CategoryHTML, htmlCatalog()},
		{lint.CategorySpelling, spellingCatalog()},
	}

	size := 0
	for _, section := range sections {
		size += len(section.entries)
	}
	catalog := make([]lint.RuleMeta, 0, size)
	for _, section := range sections {
		for _, entry := range section.entries {
			entry.Category = section.category
			catalog = append(catalog, entry)
		}
	}

	slices.SortFunc(catalog, func(left, right lint.RuleMeta) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return catalog
}
