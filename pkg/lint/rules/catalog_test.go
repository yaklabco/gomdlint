package rules

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// builtInRuleCount is the number of rules gomdlint ships: markdownlint's 53
// and two of its own.
const builtInRuleCount = 55

const (
	// maxDescriptionLength is the number of characters that every rule
	// description stays under, so that it fits on one line beside the
	// rule's name.
	maxDescriptionLength = 80

	// noAutomaticFix opens the fix text of every rule that cannot fix what
	// it finds, which only reports.
	noAutomaticFix = "There is no automatic fix."

	// fixOpening opens the fix text of every rule that can fix what it
	// finds.
	fixOpening = "The fix "

	// reportsWithoutFix is the phrase with which the fix text of a fixing
	// rule says which findings it reports without a fix.
	reportsWithoutFix = "without a fix"

	// emDash and enDash are the dashes that catalog prose never uses.
	emDash = rune(0x2014)
	enDash = rune(0x2013)
)

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

// TestCatalogWordsAreASCII checks that every identifier and tag is ASCII.
// Configuration files resolve them through Registry.Lookup, which folds
// case Go's way, and markdownlint comments by markdownlint's key, which
// folds it as JavaScript's toUpperCase does. The two agree on ASCII words,
// so keeping the built-in words ASCII keeps them in agreement.
func TestCatalogWordsAreASCII(t *testing.T) {
	for _, meta := range Catalog() {
		words := append(append([]string{meta.ID, meta.Name, meta.MarkdownlintName}, meta.Aliases...), meta.Tags...)
		for _, word := range words {
			for _, char := range word {
				assert.Less(t, char, rune(utf8.RuneSelf), "%s: %q", meta.ID, word)
			}
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
			assert.Empty(t, meta.MarkdownlintName, "%s: markdownlint has no names for its own rules", meta.ID)
			assert.NotEmpty(t, meta.Tags, "%s: tags", meta.ID)
			continue
		}
		shared = append(shared, meta.ID)

		assert.Equal(t, row.Tags, meta.Tags, "%s: tags", meta.ID)

		// Markdownlint comments name the rule by markdownlint's first name,
		// which MarkdownlintName records only where it is not Name.
		require.NotEmpty(t, row.Aliases, "%s: markdownlint's names", meta.ID)
		assert.Equal(t, row.Aliases[0], cmp.Or(meta.MarkdownlintName, meta.Name), "%s: markdownlint's first name", meta.ID)
		assert.NotEqual(t, meta.Name, meta.MarkdownlintName, "%s: MarkdownlintName repeats Name", meta.ID)

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

// TestCatalogDescribesEachRule covers the prose of every catalog entry:
// what correct Markdown looks like, why it matters, and what the fix does.
// Each is plain text that any output can show as it is.
func TestCatalogDescribesEachRule(t *testing.T) {
	for _, meta := range Catalog() {
		t.Run(meta.ID, func(t *testing.T) {
			assertPlainProse(t, "description", meta.Description)
			assertPlainProse(t, "rationale", meta.Rationale)
			assertPlainProse(t, "fix", meta.Fix)

			assert.Less(t, utf8.RuneCountInString(meta.Description), maxDescriptionLength,
				"the description stays under %d characters: %q", maxDescriptionLength, meta.Description)
			assert.NotContains(t, strings.ToLower(meta.Description), "should",
				"the description says what correct Markdown looks like: %q", meta.Description)
		})
	}
}

// assertPlainProse checks that the text in field is plain prose: whole
// sentences on one line, starting with a capital letter and ending with a
// period, without Markdown markup and without dashes in place of
// punctuation.
func assertPlainProse(t *testing.T, field, text string) {
	t.Helper()

	if !assert.NotEmpty(t, text, "%s is missing", field) {
		return
	}
	first, _ := utf8.DecodeRuneInString(text)
	assert.True(t, unicode.IsUpper(first), "%s starts with a capital letter: %q", field, text)
	assert.True(t, strings.HasSuffix(text, "."), "%s ends with a period: %q", field, text)
	assert.Equal(t, strings.TrimSpace(text), text, "%s has no surrounding space", field)
	assert.False(t, strings.ContainsFunc(text, unicode.IsControl),
		"%s fits on one line, with no tabs or other control characters: %q", field, text)
	assert.NotContains(t, text, "  ", "%s separates words with one space: %q", field, text)
	assert.NotContains(t, text, "`", "%s is plain text, not Markdown: %q", field, text)
	assert.False(t, strings.ContainsRune(text, emDash), "%s has no em-dash: %q", field, text)
	assert.False(t, strings.ContainsRune(text, enDash), "%s has no en dash: %q", field, text)
	assert.NotContains(t, strings.ToLower(text), "this rule",
		"%s talks about the Markdown, not the rule: %q", field, text)
}

// TestCatalogSaysWhetherEachRuleFixes covers the fix text of every catalog
// entry. A rule that cannot fix what it finds says that it only reports,
// with no automatic fix. A fixing rule describes its fix instead, and names
// the findings a user can expect it to report without a fix, leaving out
// rare edge cases. Every fix also passes shared checks that parse the fixed
// document. The texts name those checks only where they hold back most of a
// rule's fixes, as the heading-anchor check does for MD018, MD020, MD026 and
// MD036.
func TestCatalogSaysWhetherEachRuleFixes(t *testing.T) {
	// partialFixes lists the fixing rules that report findings a user can
	// expect to meet without a fix.
	partialFixes := map[string]bool{
		"MD003": true, // Underlined headings are never converted.
		"MD004": true, // A marker change that would merge two lists.
		"MD005": true, // Bullet items, and items with more than one paragraph.
		"MD007": true, // Items with more than one paragraph.
		"MD010": true, // Tabs inside code are reported and never replaced.
		"MD013": true, // Only some long lines can be wrapped safely.
		"MD018": true, // A new heading may break links to heading anchors.
		"MD020": true, // A changed heading may break links to heading anchors.
		"MD026": true, // A changed heading may break links to heading anchors.
		"MD027": true, // Fence and HTML lines inside a quote.
		"MD029": true, // A list that starts at a wrong number is not renumbered.
		"MD030": true, // Items with more than one paragraph.
		"MD034": true, // Only some bare URLs can be wrapped safely.
		"MD035": true, // A dash rule under a paragraph would make a heading.
		"MD036": true, // Only some emphasized lines can become headings.
		"MD037": true, // Markers that may be meant literally.
		"MD040": true, // Only a language the code proves is inserted.
		"MD044": true, // Names inside raw HTML.
		"MD048": true, // A new fence that would end the block early.
		"MD049": true, // A new marker that could pair with nearby characters.
		"MD050": true, // A new marker that could pair with nearby characters.
	}

	fixing := make(map[string]bool)
	for _, meta := range Catalog() {
		fixing[meta.ID] = meta.Fixable
	}
	for id := range partialFixes {
		assert.True(t, fixing[id], "partialFixes lists %s, which is not a fixing rule in the catalog", id)
	}

	for _, meta := range Catalog() {
		if !meta.Fixable {
			assert.True(t, strings.HasPrefix(meta.Fix, noAutomaticFix),
				"%s: a rule without a fix says that it only reports: %q", meta.ID, meta.Fix)
			continue
		}

		assert.True(t, strings.HasPrefix(meta.Fix, fixOpening), "%s: a fixing rule describes its fix: %q", meta.ID, meta.Fix)
		if partialFixes[meta.ID] {
			assert.Contains(t, meta.Fix, reportsWithoutFix,
				"%s: the fix text says which findings get no fix: %q", meta.ID, meta.Fix)
		} else {
			assert.NotContains(t, meta.Fix, reportsWithoutFix,
				"%s: a rule missing from partialFixes fixes every finding it can: %q", meta.ID, meta.Fix)
		}
	}
}
