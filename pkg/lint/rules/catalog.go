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

// noFixNearCheckbox is in the fix text of every fixing rule, after the
// rule's own text and before noFixWithRawHTML. The engine's checkbox guard
// (pkg/lint/checkbox_guard.go and checkbox_plain.go) withholds, whatever
// rule offers it, fixes that change or make a line holding "[", one
// character and "]", or that change lines around one, at most through the
// blank line or the edge of the file on each side. Which of those lines it
// guards, and which fixes it still lets through on plain task items such
// as "- [ ] todo", depend on the item and its list, so the text says only
// that a fix may be withheld and promises no fix.
const noFixNearCheckbox = "gomdlint may report a finding without a fix when the fix would change a line with " +
	"one character in brackets, such as [x] or [1], or the lines around it up to a blank line, " +
	"because it could add or remove a task list checkbox."

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
