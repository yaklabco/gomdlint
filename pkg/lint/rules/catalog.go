package rules

import (
	"cmp"
	"slices"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// noFixWithRawHTML ends the fix text of every rule whose fixes keepSafeFixes
// checks. That check keeps no fix in a document with raw HTML that it cannot
// model, which is any raw HTML other than whitespace, well-formed comments
// and the start tags of br, wbr, hr and img with a few plain attributes.
const noFixWithRawHTML = "In a document with raw HTML other than comments and plain br, wbr, hr or img tags, " +
	"gomdlint reports every finding without a fix."

// catalogSection is the part of the catalog that one catalog_<category>.go
// file lists.
type catalogSection struct {
	category lint.Category
	entries  []lint.RuleMeta
}

// Catalog returns the catalog entry of every built-in rule, in rule ID
// order: its ID, name, markdownlint aliases, category, markdownlint tags,
// whether it can fix what it finds, and what it checks, why, and what its
// fix does. Each catalog_<category>.go file lists the rules of one
// category.
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
	options := catalogOptions()
	catalog := make([]lint.RuleMeta, 0, size)
	for _, section := range sections {
		for _, entry := range section.entries {
			entry.Category = section.category
			entry.Options, entry.OptionsDeclared = options[entry.ID]
			catalog = append(catalog, entry)
		}
	}

	slices.SortFunc(catalog, func(left, right lint.RuleMeta) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return catalog
}
