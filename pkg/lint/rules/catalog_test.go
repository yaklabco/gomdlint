package rules

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// builtInRuleCount is the number of rules gomdlint ships: markdownlint's 53
// and two of its own.
const builtInRuleCount = 55

func catalogRegistry(t *testing.T) *lint.Registry {
	t.Helper()
	registry := lint.NewRegistry()
	RegisterAll(registry)
	return registry
}

func TestCatalogDescribesEveryRegisteredRule(t *testing.T) {
	registry := catalogRegistry(t)
	catalog := Catalog()

	ids := make([]string, 0, len(catalog))
	for _, meta := range catalog {
		ids = append(ids, meta.ID)
	}

	assert.Len(t, catalog, builtInRuleCount)
	assert.True(t, slices.IsSorted(ids), "Catalog lists entries in rule ID order")
	assert.Len(t, slices.Compact(slices.Clone(ids)), len(ids), "no rule has two entries")
	assert.Equal(t, registry.IDs(), ids, "every registered rule has an entry, and every entry is registered")
}

func TestCatalogEntriesMatchTheirRules(t *testing.T) {
	registry := catalogRegistry(t)

	for _, meta := range Catalog() {
		rule, ok := registry.GetByID(meta.ID)
		require.True(t, ok, meta.ID)
		assert.Equal(t, rule.Name(), meta.Name, "%s: name", meta.ID)
		assert.Equal(t, rule.CanFix(), meta.Fixable, "%s: fix flag", meta.ID)
	}
}

func TestCatalogIdentifiersNameOneRule(t *testing.T) {
	owners := make(map[string]string) // lower-cased ID, name or alias -> rule ID
	claim := func(identifier, id string) {
		key := strings.ToLower(identifier)
		if owner, taken := owners[key]; taken {
			assert.Failf(t, "identifier names two rules", "%q identifies both %s and %s", identifier, owner, id)
			return
		}
		owners[key] = id
	}

	catalog := Catalog()
	for _, meta := range catalog {
		claim(meta.ID, meta.ID)
		claim(meta.Name, meta.ID)
	}
	// An alias may not repeat its own rule's name, nor any identifier of
	// another rule.
	for _, meta := range catalog {
		for _, alias := range meta.Aliases {
			claim(alias, meta.ID)
		}
	}

	// markdownlint resolves names, aliases and tags through one table, so a
	// tag must not spell a rule identifier either.
	for _, meta := range catalog {
		for _, tag := range meta.Tags {
			owner, taken := owners[strings.ToLower(tag)]
			assert.False(t, taken, "%s: tag %q also identifies %s", meta.ID, tag, owner)
		}
	}
}

func TestCatalogCategories(t *testing.T) {
	rulesPerCategory := make(map[lint.Category]int)
	for _, meta := range Catalog() {
		assert.Contains(t, lint.Categories(), meta.Category, "%s: category", meta.ID)
		rulesPerCategory[meta.Category]++
	}
	for _, category := range lint.Categories() {
		assert.Positive(t, rulesPerCategory[category], "category %s has no rules", category)
	}
}

func TestCatalogFollowsMarkdownlint(t *testing.T) {
	registry := catalogRegistry(t)
	vocabulary := make(map[string]bool)
	for _, row := range markdownlintRules {
		for _, tag := range row.Tags {
			vocabulary[tag] = true
		}
	}

	shared := make([]string, 0, len(markdownlintRules))
	for _, meta := range Catalog() {
		for _, tag := range meta.Tags {
			assert.True(t, vocabulary[tag], "%s: markdownlint has no tag %q", meta.ID, tag)
		}

		row, fromMarkdownlint := markdownlintRules[meta.ID]
		assert.Equal(t, !fromMarkdownlint, lint.IsGomdlintOnlyRule(meta.ID), "%s: gomdlint-only rule", meta.ID)
		if !fromMarkdownlint {
			assert.Empty(t, meta.Aliases, "%s: markdownlint has no names for its own rules", meta.ID)
			assert.NotEmpty(t, meta.Tags, "%s: tags", meta.ID)
			continue
		}
		shared = append(shared, meta.ID)

		assert.Equal(t, row.Tags, meta.Tags, "%s: tags", meta.ID)

		// The aliases are markdownlint's names for the rule other than its
		// gomdlint name, in markdownlint's order.
		var aliases []string
		for _, name := range row.Aliases {
			if name != meta.Name {
				aliases = append(aliases, name)
			}
		}
		assert.Equal(t, aliases, meta.Aliases, "%s: aliases", meta.ID)

		for _, name := range row.Aliases {
			rule, ok := registry.Lookup(name)
			if assert.True(t, ok, "%s: markdownlint name %q", meta.ID, name) {
				assert.Equal(t, meta.ID, rule.ID(), "markdownlint name %q", name)
			}
		}
	}

	assert.ElementsMatch(t, slices.Collect(maps.Keys(markdownlintRules)), shared,
		"the catalog has every markdownlint rule")
}

func TestDefaultRegistryDescribesEveryRule(t *testing.T) {
	catalog := Catalog()
	assert.Len(t, lint.DefaultRegistry.Rules(), len(catalog))

	for _, meta := range catalog {
		described, ok := lint.DefaultRegistry.Meta(meta.ID)
		if assert.True(t, ok, meta.ID) {
			assert.Equal(t, meta, described)
		}
	}
}
