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

	"github.com/yaklabco/gomdlint/pkg/config"
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
// document, and the engine's checkbox guard. The texts name those checks
// where they hold back whole classes of fixes: the heading-anchor check for
// MD018, MD020, MD026 and MD036, the raw HTML check of keepSafeFixes for
// every rule that uses it, and the checkbox guard for every fixing rule.
func TestCatalogSaysWhetherEachRuleFixes(t *testing.T) {
	// rawHTMLChecked lists the rules whose fixes keepSafeFixes checks, so
	// that raw HTML it cannot model anywhere in a document holds back all of
	// their fixes.
	rawHTMLChecked := map[string]bool{
		"MD004": true, "MD005": true, "MD007": true, "MD009": true, "MD010": true, "MD012": true,
		"MD013": true, "MD027": true, "MD029": true, "MD030": true, "MD032": true, "MD047": true,
	}

	// partialFixes lists the fixing rules that report findings a user can
	// expect to meet without a fix, other than those the checkbox guard may
	// hold back. The rules in rawHTMLChecked are among them, and the
	// comments give each rule's other reasons.
	partialFixes := map[string]bool{
		"MD003": true, // Underlined headings are never converted.
		"MD004": true, // A marker change that would merge two lists.
		"MD005": true, // Bullet items, and items that hold more than a paragraph and nested lists.
		"MD007": true, // Items that hold more than a paragraph and nested lists.
		"MD009": true, // Whitespace in a code span, whitespace other than one space before a soft line break, a hard break when br_spaces is below 2, and raw HTML.
		"MD010": true, // Tabs inside code are never replaced, and some others, such as in a heading.
		"MD012": true, // Blank lines inside an HTML comment, and raw HTML.
		"MD013": true, // Only some long lines can be wrapped safely.
		"MD018": true, // A new heading may break links to heading anchors, and a space cannot make seven or more # marks a heading.
		"MD020": true, // A changed heading may break links to heading anchors.
		"MD022": true, // A blank line that would loosen a tight list.
		"MD026": true, // A heading left showing nothing, or changed where links may point at anchors.
		"MD027": true, // Fence and HTML lines, and items with more than one paragraph.
		"MD029": true, // A list that starts at a wrong number is not renumbered.
		"MD030": true, // Items that hold more than a paragraph and nested lists.
		"MD031": true, // A blank line that would loosen a tight list, or follow a fence that is never closed.
		"MD032": true, // Raw HTML only.
		"MD034": true, // Only some bare URLs can be wrapped safely.
		"MD035": true, // A dash rule under a paragraph would make a heading, and some rules in list items.
		"MD036": true, // Only some emphasized lines can become headings.
		"MD037": true, // Markers that may be meant literally, and markers inside square brackets.
		"MD039": true, // Link text that starts or ends with a line break, ends in a space after a backslash, or holds only spaces.
		"MD040": true, // Only a language the code proves, and allowed_languages permits, is inserted.
		"MD044": true, // Names inside raw HTML.
		"MD047": true, // Raw HTML only.
		"MD048": true, // A new fence that would end the block early.
		"MD049": true, // A new marker that could pair with nearby characters, or a reference label.
		"MD050": true, // A new marker that could pair with nearby characters, or a reference label.
		"MD053": true, // Text on the line after the definition, and labels that start with ^.
		"MD058": true, // A blank line that would loosen a tight list.
	}

	fixing := make(map[string]bool)
	for _, meta := range Catalog() {
		fixing[meta.ID] = meta.Fixable
	}
	for id := range partialFixes {
		assert.True(t, fixing[id], "partialFixes lists %s, which is not a fixing rule in the catalog", id)
	}
	for id := range rawHTMLChecked {
		assert.True(t, partialFixes[id], "rawHTMLChecked lists %s, which partialFixes leaves out", id)
	}

	for _, meta := range Catalog() {
		assert.Equal(t, rawHTMLChecked[meta.ID], strings.HasSuffix(meta.Fix, " "+noFixWithRawHTML),
			"%s: the fix text ends with the raw HTML sentence exactly when keepSafeFixes checks the rule's fixes: %q", meta.ID, meta.Fix)

		// The checkbox guard may hold back a fix of any rule, so the
		// checkbox sentence appears once in every fixing rule's fix text,
		// and in no other.
		want := 0
		if meta.Fixable {
			want = 1
		}
		assert.Equal(t, want, strings.Count(meta.Fix, noFixNearCheckbox),
			"%s: the fix text holds the checkbox sentence exactly when the rule fixes: %q", meta.ID, meta.Fix)

		if !meta.Fixable {
			assert.True(t, strings.HasPrefix(meta.Fix, noAutomaticFix),
				"%s: a rule without a fix says that it only reports: %q", meta.ID, meta.Fix)
			continue
		}

		assert.True(t, strings.HasPrefix(meta.Fix, fixOpening), "%s: a fixing rule describes its fix: %q", meta.ID, meta.Fix)

		// The checkbox sentence follows the rule's own text, and only the
		// raw HTML sentence comes after it.
		end := ""
		if rawHTMLChecked[meta.ID] {
			end = " " + noFixWithRawHTML
		}
		if !assert.True(t, strings.HasSuffix(meta.Fix, " "+noFixNearCheckbox+end),
			"%s: the checkbox sentence ends the fix text, before any raw HTML sentence: %q", meta.ID, meta.Fix) {
			continue
		}

		// The rule's own text is the fix text without the checkbox
		// sentence, which every fixing rule shares.
		own := strings.TrimSuffix(meta.Fix, " "+noFixNearCheckbox+end) + end
		if partialFixes[meta.ID] {
			assert.Contains(t, own, reportsWithoutFix,
				"%s: the fix text says which findings get no fix: %q", meta.ID, meta.Fix)
		} else {
			assert.NotContains(t, own, reportsWithoutFix,
				"%s: a rule missing from partialFixes fixes every finding it can, outside the checkbox guard: %q", meta.ID, meta.Fix)
		}
	}
}

// catalogFinding is a finding of one rule, by line and by whether it
// carries a fix.
type catalogFinding struct {
	line  int
	fixed bool
}

// catalogBehaviourCase is a case of TestCatalogTextsHoldForTheRules: an
// input for rule id and the findings it gives with default options.
type catalogBehaviourCase struct {
	id, name, input string
	// phrase is text that the rule's catalog texts state the pinned
	// behaviour with.
	phrase string
	// options sets the rule's options, when the case needs any.
	options map[string]any
	want    []catalogFinding
	// fixText is the text that the rule's fix inserts, when the case
	// pins it.
	fixText string
}

// catalogLongLine has spaces past column 80, and catalogShortLine leaves
// room for a long last word that ends past it.
const (
	catalogLongLine  = "aaa bbb ccc ddd eee fff ggg hhh iii jjj kkk lll mmm nnn ooo ppp qqq rrr sss ttt uuu vvv"
	catalogShortLine = "aaa bbb ccc ddd eee fff ggg hhh iii jjj kkk lll mmm nnn ooo ppp"
)

// catalogBehaviourCases are the cases of TestCatalogTextsHoldForTheRules.
var catalogBehaviourCases = []catalogBehaviourCase{
	// MD007 checks bullets outside ordered lists.
	{id: "MD007", name: "bullet under an ordered item", phrase: "Outside ordered lists", input: "# Title\n\n1. Parent\n   - Child\n"},
	{
		id: "MD007", name: "bullet under a bullet", phrase: "indent 2 spaces per level", want: []catalogFinding{{4, true}},
		input: "# Title\n\n- Parent\n   - Child\n",
	},

	// MD009 leaves code blocks alone.
	{id: "MD009", name: "trailing spaces in a code block", phrase: "Outside code blocks", input: "# Title\n\n```text\ncode  \nmore   \n```\n"},
	{id: "MD009", name: "trailing space in text", phrase: "deletes the trailing whitespace", input: "# Title\n\nText \n", want: []catalogFinding{{3, true}}},

	// keepSafeFixes keeps no fix in a document with raw HTML other than
	// comments and plain br, wbr, hr or img tags.
	{
		id: "MD009", name: "HTML block elsewhere", phrase: noFixWithRawHTML, want: []catalogFinding{{3, false}},
		input: "# Title\n\nText \n\n<details>\n<summary>More</summary>\n</details>\n",
	},
	// MD012 reports each blank line past the maximum, one by default, as
	// markdownlint does.
	{
		id: "MD012", name: "inline HTML elsewhere", phrase: noFixWithRawHTML, want: []catalogFinding{{3, false}, {4, false}},
		input: "# Title\n\n\n\nPress <kbd>Ctrl</kbd>.\n",
	},
	{
		id: "MD012", name: "no HTML", phrase: "deletes the extra blank lines", want: []catalogFinding{{3, true}, {4, true}},
		input: "# Title\n\n\n\nPress Ctrl.\n",
	},
	{
		id: "MD047", name: "HTML block", phrase: noFixWithRawHTML, want: []catalogFinding{{5, false}},
		input: "# Title\n\n<details>\n<summary>More</summary>\n</details>",
	},
	{id: "MD047", name: "comment and br", phrase: "other than comments and plain br", input: "# Title\n\nA<br>b <!-- note -->\nEnd", want: []catalogFinding{{4, true}}},

	// MD047 asks for a last line that counts as blank, not for a final
	// newline byte.
	{id: "MD047", name: "last line of text", phrase: "ends with a newline", input: "# Title\n\nText.", want: []catalogFinding{{3, true}}},
	{id: "MD047", name: "last line of spaces", phrase: "a last line of only whitespace", input: "# Title\n\nText.\n "},
	{id: "MD047", name: "last line holding a comment", phrase: "or comments", input: "# Title\n\nText.\n<!-- end -->"},
	{id: "MD047", name: "last line holding a quote marker", phrase: "only whitespace, > or comments", input: "# Title\n\n> Quote.\n>"},

	// MD011 checks inline links.
	{
		id: "MD011", name: "reference link and autolink", phrase: "Inline links",
		input: "# Title\n\nSee [text][ref] and <https://example.com>.\n\n[ref]: https://example.com\n",
	},
	{
		id: "MD011", name: "reversed inline link", phrase: "swaps the two parts", want: []catalogFinding{{3, true}},
		input: "# Title\n\nSee (text)[https://example.com].\n",
	},

	// MD042 asks inline links for a destination, and every link for more
	// than #.
	{id: "MD042", name: "empty inline destination", phrase: "Inline links have a destination", input: "# Title\n\nSee [text]().\n", want: []catalogFinding{{3, false}}},
	{
		id: "MD042", name: "reference to an empty definition", phrase: "Inline links have a destination",
		input: "# Title\n\nSee [text][ref].\n\n[ref]: <>\n",
	},
	{
		id: "MD042", name: "reference to a bare fragment", phrase: "no link points at just #", want: []catalogFinding{{3, false}},
		input: "# Title\n\nSee [text][ref].\n\n[ref]: #\n",
	},

	// MD045 asks Markdown images for alt text, and HTML images for alt,
	// even empty, or aria-hidden="true".
	{
		id: "MD045", name: "images", phrase: "aria-hidden=\"true\"", want: []catalogFinding{{7, false}, {9, false}},
		input: "# Title\n\n<img src=\"a.png\" alt=\"\">\n\n<img src=\"b.png\" aria-hidden=\"true\">\n\n" +
			"<img src=\"c.png\">\n\n![](d.png) and ![ok](e.png)\n",
	},

	// MD052 checks full and collapsed references, not shortcuts, and
	// ignores the label x.
	{
		id: "MD052", name: "shortcut references", phrase: "Shortcut references such as [label] are not checked",
		input: "# Title\n\nSee [missing] and ![missing].\n",
	},
	{id: "MD052", name: "label x", phrase: "except x", input: "# Title\n\nSee [text][x].\n"},
	{
		id: "MD052", name: "full and collapsed references", phrase: "[text][label] and [label][]",
		input: "# Title\n\nSee [text][nope] and [nope][].\n", want: []catalogFinding{{3, false}, {3, false}},
	},

	// MD053 ignores definitions labeled //.
	{id: "MD053", name: "comment definition", phrase: "except [//] comments", input: "# Title\n\nText.\n\n[//]: # (note)\n"},

	// MD013 exempts lone links and images, definitions and a long last
	// word, and its fix leaves headings alone.
	{id: "MD013", name: "lone link", phrase: "only a link or an image", input: "# Title\n\n[" + catalogLongLine + "](https://example.com/page)\n"},
	{id: "MD013", name: "lone image", phrase: "only a link or an image", input: "# Title\n\n![" + catalogLongLine + "](image.png)\n"},
	{
		id: "MD013", name: "definition with a title", phrase: "reference definitions",
		input: "# Title\n\nSee [x].\n\n[x]: https://example.com/page \"" + catalogLongLine + "\"\n",
	},
	{
		id: "MD013", name: "long last word", phrase: "crosses the limit",
		input: "# Title\n\n" + catalogShortLine + " https://example.com/a-path-that-runs-past-the-limit\n",
	},
	{id: "MD013", name: "paragraph line", phrase: "wraps long paragraph lines", input: "# Title\n\n" + catalogLongLine + "\n", want: []catalogFinding{{3, true}}},
	{id: "MD013", name: "heading", phrase: "such as a heading", input: "# Title\n\n## " + catalogLongLine + "\n", want: []catalogFinding{{3, false}}},

	// MD038 allows one space on each side and a space beside a backtick.
	{id: "MD038", name: "allowed padding", phrase: "except one each side or by a backtick", input: "# Title\n\nUse `` `foo`` and `   ` and ` code ` here.\n"},
	{id: "MD038", name: "one-sided space", phrase: "deletes the extra spaces", input: "# Title\n\nUse ` code` here.\n", want: []catalogFinding{{3, true}}},

	// MDL001 compares only links that could take either form: relative
	// links, and file: URLs that name no other host.
	{
		id: "MDL001", name: "mixed kinds", phrase: "are not mixed", want: []catalogFinding{{3, false}},
		input: "# Title\n\nSee [a](docs/a.md) and [b](file:///home/user/b.md).\n",
	},
	{
		id: "MDL001", name: "file URL", phrase: "file: URLs that name no other host", want: []catalogFinding{{3, false}},
		input: "# Title\n\nSee [b](file://localhost/home/user/b.md) and [a](docs/a.md).\n",
	},
	{id: "MDL001", name: "file URL on another host", phrase: "name no other host", input: "# Title\n\nSee [a](docs/a.md) and [b](file://server/share/b.md).\n"},
	{id: "MDL001", name: "link to another host", phrase: "link to another host", input: "# Title\n\nSee [a](docs/a.md) and [b](https://example.com).\n"},
	{id: "MDL001", name: "mailto link", phrase: "mailto:", input: "# Title\n\nSee [a](docs/a.md) and [mail us](mailto:team@example.com).\n"},
	{id: "MDL001", name: "autolink", phrase: "autolinks", input: "# Title\n\nSee [a](docs/a.md) and <file:///home/user/b.md>.\n"},
	{id: "MDL001", name: "fragment link", phrase: "a #fragment", input: "# Title\n\nSee [b](file:///home/user/b.md) and [top](#title).\n"},

	// MD049 and MD050 allow asterisks inside a word, which underscores
	// cannot replace, and hold back fixes in reference labels.
	{id: "MD049", name: "asterisks inside a word", phrase: "except * inside a word", input: "# Title\n\n_one_ a*two*b\n"},
	{id: "MD050", name: "asterisks inside a word", phrase: "except ** inside a word", input: "# Title\n\n__one__ a**two**b\n"},
	{
		id: "MD049", name: "italic in a reference label", phrase: "the label of a reference link", want: []catalogFinding{{3, false}, {3, false}},
		input: "# Title\n\n_first_ [*second*]\n\n[*second*]: /target\n",
	},
	{
		id: "MD050", name: "bold in a reference label", phrase: "the label of a reference link", want: []catalogFinding{{3, false}, {3, false}},
		input: "# Title\n\n__first__ [**second**]\n\n[**second**]: /target\n",
	},

	// Other findings that fix texts name as reported without a fix.
	{
		id: "MD027", name: "quoted item with a second paragraph", phrase: "one that holds a second paragraph",
		input: "# Title\n\n>   - item\n>\n>     second paragraph\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD053", name: "text after the definition", phrase: "other text follows on the next line",
		input: "# Title\n\n[unused]: /target\nParagraph.\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD053", name: "blank line after the definition", phrase: "deletes the unused or duplicate definition",
		input: "# Title\n\n[unused]: /target\n\nParagraph.\n", want: []catalogFinding{{3, true}},
	},
	{
		id: "MD053", name: "unused footnote definition", phrase: "such as [^1]: A note. counts too",
		input: "# Title\n\nText.\n\n[^1]: A note.\n", want: []catalogFinding{{5, false}},
	},
	{
		id: "MD053", name: "called footnote definition", phrase: "such as [^1]: A note. counts too",
		input: "# Title\n\nText[^1].\n\n[^1]: A note.\n",
	},
	{
		id: "MD053", name: "footnote definition in a list", phrase: "outside lists and blockquotes",
		input: "# Title\n\nText.\n\n- [^1]: A note.\n",
	},
	{
		id: "MD053", name: "label that starts with a caret", phrase: "one whose label starts with ^",
		input: "# Title\n\nText.\n\n[^b]: /url\n", want: []catalogFinding{{5, false}},
	},

	// MD036 makes a bold line a heading one level deeper than the one
	// above it, at most level 6, or level 2 with none above, and never
	// fixes an italic line.
	{
		id: "MD036", name: "no heading above", phrase: "With no heading above, it uses level 2",
		input: "**Bold line**\n\nText.\n", want: []catalogFinding{{1, true}}, fixText: "## Bold line",
	},
	{
		id: "MD036", name: "level 6 above", phrase: "at most level 6", want: []catalogFinding{{5, true}}, fixText: "###### Bold line",
		input: "# One\n\n###### Six\n\n**Bold line**\n\nText.\n",
	},
	{
		id: "MD036", name: "italic line", phrase: "reports italic lines without a fix",
		input: "# Title\n\n*Italic line*\n\nText.\n", want: []catalogFinding{{3, false}},
	},

	// MD022, MD031 and MD058 add no blank line that would make a tight list
	// loose, and can add them in loose lists.
	{
		id: "MD022", name: "heading in a tight list", phrase: "a heading in a tight list without a fix",
		input: "# Title\n\n- # Heading\n  text\n- item\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD022", name: "heading and table in a tight item", phrase: "a heading in a tight list without a fix",
		input: "# Title\n\n- # Heading\n  | a | b |\n  | --- | --- |\n- next\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD022", name: "heading in a loose list", phrase: "inserts the missing blank lines",
		input: "# Title\n\n- # Heading\n  text\n\n- item\n", want: []catalogFinding{{3, true}},
	},
	{
		id: "MD031", name: "fence in a tight list", phrase: "a fence in a tight list",
		input: "# Title\n\n1. Run:\n   ```sh\n   make\n   ```\n2. Next\n", want: []catalogFinding{{4, false}, {6, false}},
	},
	{
		id: "MD031", name: "fence in a loose list", phrase: "inserts the missing blank line",
		input: "# Title\n\n1. Run:\n   ```sh\n   make\n   ```\n\n2. Next\n", want: []catalogFinding{{4, true}},
	},
	{
		id: "MD058", name: "table in a tight list", phrase: "a table in a tight list when the blank line would make the list loose",
		input: "# Title\n\n- item\n  | a | b |\n  | --- | --- |\n- next\n", want: []catalogFinding{{4, false}, {5, false}},
	},
	{
		id: "MD058", name: "table in a loose list", phrase: "inserts the missing blank line",
		input: "# Title\n\n- item\n\n  text\n  | a | b |\n  | --- | --- |\n\n- next\n", want: []catalogFinding{{6, true}},
	},
	{
		id: "MD058", name: "table in a quote inside an item", phrase: "keeping any blockquote markers",
		input: "# Title\n\n- item\n  > text\n  > | a | b |\n  > | --- | --- |\n", want: []catalogFinding{{5, true}},
	},

	// MD040 inserts a detected language only when allowed_languages, if
	// set, names it exactly.
	{
		id: "MD040", name: "detected language not allowed", phrase: "allowed_languages, if set, names it exactly",
		options: map[string]any{"allowed_languages": []any{"go", "ruby"}},
		input:   "# Title\n\n```\n#!/bin/bash\necho hi\n```\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD040", name: "detected language in another case", phrase: "allowed_languages, if set, names it exactly",
		options: map[string]any{"allowed_languages": []any{"Bash"}},
		input:   "# Title\n\n```\n#!/bin/bash\necho hi\n```\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD040", name: "detected language allowed", phrase: "allowed_languages, if set, names it exactly",
		options: map[string]any{"allowed_languages": []any{"bash"}},
		input:   "# Title\n\n```\n#!/bin/bash\necho hi\n```\n", want: []catalogFinding{{3, true}}, fixText: "bash",
	},

	// MD026 keeps a heading that would show nothing.
	{id: "MD026", name: "punctuation only", phrase: "leave it showing nothing", input: "# Title\n\n## ...\n", want: []catalogFinding{{3, false}}},
	{id: "MD026", name: "text and punctuation", phrase: "deletes the trailing punctuation", input: "# Title\n\n## Done.\n", want: []catalogFinding{{3, true}}},

	// MD022 and MD031 add the blank line above a tight list, which does not
	// loosen it.
	{
		id: "MD022", name: "heading that starts a tight list", phrase: "when the blank line would make the list loose",
		input: "# Title\n\nText.\n- # Heading\n- next\n", want: []catalogFinding{{4, true}, {4, false}},
	},
	{
		id: "MD031", name: "fence that starts a tight list", phrase: "when the blank line would make the list loose",
		input: "# Title\n\nText.\n- ```sh\n  make\n  ```\n- next\n", want: []catalogFinding{{4, true}, {6, false}},
	},

	// MD023 measures a heading's indentation from its list item or
	// blockquote.
	{
		id: "MD023", name: "heading at the start of its list item", phrase: "beginning of their line, list item or blockquote",
		input: "# Title\n\n- item\n  # Heading\n",
	},
	{
		id: "MD023", name: "heading indented past its list item", phrase: "deletes the extra indentation",
		input: "# Title\n\n- item\n   # Heading\n", want: []catalogFinding{{4, true}},
	},

	// MD027 leaves alone the indentation of code and of list item text in a
	// blockquote.
	{id: "MD027", name: "indented code in a blockquote", phrase: "Outside code and list item text", input: "# Title\n\n>     code\n"},
	{
		id: "MD027", name: "second paragraph of a quoted list item", phrase: "Outside code and list item text",
		input: "# Title\n\n> - item\n>\n>   second paragraph\n",
	},

	// MD032 checks lists that are not inside another list.
	{
		id: "MD032", name: "nested list", phrase: "Lists outside other lists",
		input: "# Title\n\n- Parent\n  - Child\n  - Child two\n- Next\n",
	},

	// MD018 reports seven or more # marks, which a space cannot make a
	// heading, without a fix.
	{id: "MD018", name: "seven marks", phrase: "seven or more # marks", input: "# Title\n\n#######seven\n", want: []catalogFinding{{3, false}}},
	{id: "MD018", name: "six marks", phrase: "inserts a space after the # marks", input: "# Title\n\n######six\n", want: []catalogFinding{{3, true}}},

	// The checkbox guard may hold back a fix that changes a line with one
	// character in brackets, or the lines around it up to a blank line.
	{
		id: "MD009", name: "trailing space three lines below [1]", phrase: "the lines around it up to a blank line",
		input: "# Title\n\nSee note [1] here.\nline two\nline three\nline four \n", want: []catalogFinding{{6, false}},
	},
	{
		id: "MDL003", name: "table with [x]", phrase: "one character in brackets, such as [x] or [1]",
		input: "# Title\n\n| a | b |\n| - | --- |\n| [x] | y |\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD034", name: "bare URL in a task item", phrase: "could add or remove a task list checkbox",
		input: "# Title\n\n- [ ] Read https://example.com today.\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD010", name: "tab in a task item's text", phrase: "one character in brackets, such as [x] or [1]",
		input: "# Title\n\n- [ ] do\tthis\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD010", name: "tab after a checkbox", phrase: "one character in brackets, such as [x] or [1]",
		input: "# Title\n\n- [ ]\tthis\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD034", name: "bare URL on the line after a task item", phrase: "the lines around it up to a blank line",
		input: "# Title\n\n- [ ] todo\n  see https://example.com today\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD022", name: "heading below a task item", phrase: "the lines around it up to a blank line",
		input: "# Title\n\n- [ ] todo\n## Next\n\nText.\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD013", name: "long line directly above a task item", phrase: "the lines around it up to a blank line",
		input: "# Title\n\n" + catalogLongLine + "\n- [ ] todo\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD032", name: "blank line below a task list", phrase: "the lines around it up to a blank line",
		input: "# Title\n\n- [ ] todo\n---\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD022", name: "heading above a quoted task list", phrase: "the lines around it up to a blank line",
		input: "# Title\n> - [ ] todo\n", want: []catalogFinding{{1, false}},
	},
	{
		id: "MD032", name: "heading above a quoted task list", phrase: "the lines around it up to a blank line",
		input: "# Title\n> - [ ] todo\n", want: []catalogFinding{{2, false}},
	},
	{
		id: "MD022", name: "heading above a task list that starts at 3", phrase: "the lines around it up to a blank line",
		input: "# Title\n3. [ ] todo\n", want: []catalogFinding{{1, false}},
	},
	{
		id: "MD032", name: "heading above a task list that starts at 3", phrase: "the lines around it up to a blank line",
		input: "# Title\n3. [ ] todo\n", want: []catalogFinding{{2, false}},
	},

	// MD009 reports some trailing whitespace without a fix of its own
	// accord: whitespace in a code span is code, and before the next line
	// of a paragraph some renderers read a tab as a hard line break.
	{
		id: "MD009", name: "trailing space in a code span", phrase: "whitespace inside a code span",
		input: "# Title\n\nUse `a \nb` here.\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD009", name: "trailing tab before the paragraph's next line", phrase: "a tab where the paragraph goes on to the next line",
		input: "# Title\n\nText\t\nnext\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD009", name: "trailing space before the paragraph's next line", phrase: "deletes the trailing whitespace",
		input: "# Title\n\nText \nnext\n", want: []catalogFinding{{3, true}},
	},

	// Each row below pins an exemption that a description qualifies, or a
	// finding that a fix text says may get no fix.
	{
		id: "MD001", name: "front matter title before the first heading", phrase: "the heading or title before it",
		input: "---\ntitle: X\n---\n\n### H3\n", want: []catalogFinding{{5, false}},
	},
	{
		id: "MD018", name: "hashes inside containers", phrase: "open a heading at the start of a line",
		input: "# Title\n\n> #Heading\n\n- #Heading\n",
	},
	{id: "MD020", name: "closed heading in a blockquote", phrase: "A closed heading at the start of a line", input: "# Title\n\n> ## Sub#\n"},
	{
		id: "MD022", name: "comment lines around a heading", phrase: "a blank or comment line above and below",
		input: "# Title\n\nText\n<!-- c -->\n## H\n<!-- d -->\nMore\n",
	},
	{
		id: "MD022", name: "heading right after front matter", phrase: "a heading right after front matter needs none above it",
		input: "---\ntitle: x\n---\n# Title\n\nText\n",
	},
	{
		id: "MD036", name: "bold lines in a blockquote and a list item", phrase: "Outside lists and blockquotes",
		input: "# Title\n\n> **Quote bold**\n>\n> Text\n\n- **List bold**\n\n  Text\n",
	},
	{id: "MD041", name: "comment before the title", phrase: "Comments aside", input: "<!-- markdownlint-disable MD013 -->\n\n# Title\n"},
	{
		id: "MD005", name: "code span in an item", phrase: "a second paragraph, a code block or a quote",
		input: "# Title\n\n1. a\n 2. `b`\n", want: []catalogFinding{{4, true}},
	},
	{
		id: "MD007", name: "code block in an item", phrase: "a second paragraph, a code block or a quote",
		input: "# Title\n\n- a\n    - b\n\n      ```sh\n      x\n      ```\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD007", name: "code span in an item", phrase: "a second paragraph, a code block or a quote",
		input: "# Title\n\n- a\n    - `b`\n", want: []catalogFinding{{4, true}},
	},
	{id: "MD029", name: "list counting up from 0", phrase: "or 0, 1, 2, and so on", input: "# Title\n\n0. a\n1. b\n2. c\n"},
	{
		id: "MD029", name: "single item numbered 0", phrase: "would change its first number",
		input: "# Title\n\n0. a\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD029", name: "list counting up from 3", phrase: "would change its first number",
		input: "# Title\n\n3. a\n4. b\n", want: []catalogFinding{{3, false}, {4, false}},
	},
	{id: "MD030", name: "tab after a marker", phrase: "One space, or one tab,", input: "# Title\n\n-\ta\n"},
	{
		id: "MD032", name: "comment line above a list", phrase: "a blank or comment line above and below",
		input: "# Title\n\n<!-- markdownlint-disable-next-line MD004 -->\n- a\n",
	},
	{
		id: "MD037", name: "markers inside square brackets", phrase: "It does the same inside square brackets",
		input: "# Title\n\nNote [a ** b ** c] here.\n", want: []catalogFinding{{3, false}, {3, false}},
	},
	{
		id: "MD044", name: "name in a URL", phrase: "Outside URLs", options: map[string]any{"names": []any{"GitHub"}},
		input: "# Title\n\nSee https://github.com/x and [repo](https://github.com/z) and github.\n", want: []catalogFinding{{3, true}},
	},
	{
		id: "MD009", name: "two trailing whitespace characters anywhere", phrase: "or in exactly 2 of them",
		input: "# T  \n\nfoo\t\t\nbar\n",
	},
	{
		id: "MD010", name: "tab in a heading", phrase: "reports some tabs without a fix",
		input: "# Title\n\n# a\tb\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD012", name: "blank lines after front matter", phrase: "right after front matter are not checked",
		input: "---\ntitle: x\n---\n\n\n\n## T\n",
	},
	{
		id: "MD012", name: "blank lines in an HTML comment", phrase: "blank lines inside an HTML comment without a fix",
		input: "# Title\n\n<!--\n\n\n-->\n\nfoo\n", want: []catalogFinding{{5, false}},
	},
	{
		id: "MD013", name: "long HTML comment", phrase: "when it is not paragraph text",
		input: "# Title\n\n<!-- " + catalogLongLine + " -->\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD035", name: "longer rule of the same character", phrase: "written the same way as the first one",
		input: "# Title\n\n---\n\ntext\n\n-----\n", want: []catalogFinding{{7, true}},
	},
	{
		id: "MD035", name: "rule in a list item", phrase: "reports some horizontal rules without a fix",
		input: "# Title\n\n---\n\n- * * *\n", want: []catalogFinding{{5, false}},
	},
	{
		id: "MD035", name: "indented rule", phrase: "Indentation and trailing spaces do not count",
		input: "# Title\n\n---\n\ntext\n\n  ---\n",
	},
	{
		id: "MD007", name: "heading in an item", phrase: "reports some items without a fix",
		input: "# Title\n\n- a\n   - ## b\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD044", name: "names in an email address and a fence info string", phrase: "email addresses and fence info strings",
		options: map[string]any{"names": []any{"JavaScript", "GitHub"}},
		input:   "# Title\n\nMail me@github.com now.\n\n```javascript\nx\n```\n\nUse javascript.\n", want: []catalogFinding{{9, true}},
	},
	{id: "MD051", name: "link to a GitHub line number", phrase: "such as #L20", input: "# Title\n\n[b](#L20)\n"},
	{id: "MD047", name: "last line of a tab", phrase: "a last line of only whitespace", input: "# Title\n\nText.\n\t"},
	{
		id: "MD031", name: "comment lines around a fence", phrase: "a blank or comment line above and below",
		input: "# Title\n\n<!-- prettier-ignore -->\n```text\nx\n```\n<!-- end -->\n",
	},
	{
		id: "MD031", name: "fence that is never closed", phrase: "reports some fences without a fix",
		input: "# Title\n\n> ```\n> x\ntext\n", want: []catalogFinding{{4, false}},
	},
	{
		id: "MD058", name: "comment lines around a table", phrase: "a blank or comment line above and below",
		input: "# Title\n\n<!-- c -->\n| a | b |\n| - | - |\n<!-- d -->\n\nEnd.\n",
	},
	{
		id: "MD034", name: "URLs in code and HTML", phrase: "Outside code and HTML",
		input: "# Title\n\nCode: `https://example.com/d`\n\n```sh\ncurl https://example.com/e\n```\n\n<p>https://example.com/f</p>\n",
	},
	{
		id: "MD034", name: "URL right after an emphasis marker", phrase: "right after an emphasis marker",
		input: "# Title\n\nSee *https://example.com/c more* here.\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD034", name: "URL later inside emphasis", phrase: "right after an emphasis marker",
		input: "# Title\n\nSee *docs at https://example.com/a now* here.\n", want: []catalogFinding{{3, true}},
	},
	{
		id: "MD039", name: "link text that ends in a line break", phrase: "such as one whose text ends in a line break",
		input: "# Title\n\nSee [text\n](https://example.com) here.\n", want: []catalogFinding{{3, false}},
	},
	{
		id: "MD039", name: "full reference link", phrase: "The fix deletes the spaces",
		input: "# Title\n\nSee [text ][label] here.\n\n[label]: /destination\n", want: []catalogFinding{{3, true}},
	},
	{id: "MD051", name: "link to #top", phrase: "or at #top", input: "# Title\n\n[b](#top)\n"},
	{
		id: "MD051", name: "id on a later line of an HTML block", phrase: "an element's id attribute",
		input: "# Title\n\n<div>\n<span id=\"x\">y</span>\n</div>\n\n[b](#x)\n",
	},
	{
		id: "MD051", name: "name attributes", phrase: "or the name attribute of an a element",
		input: "# Title\n\n<a name=\"x\"></a> <div name=\"y\"></div>\n\n[b](#x) [c](#y)\n", want: []catalogFinding{{5, false}},
	},
}

// TestCatalogTextsHoldForTheRules lints, with the built-in rules and their
// default options, inputs that pin claims the catalog texts make. Each case
// names a phrase that the rule's description, rationale or fix text must
// contain, and the findings that the claim behind the phrase predicts. A
// text fails here when its rule stops behaving as it says, and when it
// drops the phrase that states the behaviour, such as an exemption.
func TestCatalogTextsHoldForTheRules(t *testing.T) {
	texts := make(map[string]string) // rule ID -> description, rationale and fix text
	for _, meta := range Catalog() {
		texts[meta.ID] = meta.Description + " " + meta.Rationale + " " + meta.Fix
	}

	for _, tt := range catalogBehaviourCases {
		t.Run(tt.id+" "+tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.phrase, "each case names the phrase it pins")
			assert.Contains(t, texts[tt.id], tt.phrase, "the texts of %s state the behaviour this case pins", tt.id)

			cfg := config.NewConfig()
			if tt.options != nil {
				cfg.Rules = map[string]config.RuleConfig{tt.id: {Options: tt.options}}
			}
			diags := lintWithDefaultRules(t, tt.input, cfg)

			var got []catalogFinding
			var inserted []string
			for _, diag := range diags {
				if diag.RuleID != tt.id {
					continue
				}
				got = append(got, catalogFinding{line: diag.StartLine, fixed: len(diag.FixEdits) > 0})
				for _, edit := range diag.FixEdits {
					inserted = append(inserted, edit.NewText)
				}
			}
			assert.Equal(t, tt.want, got, "findings of %s", tt.id)
			if tt.fixText != "" {
				assert.Contains(t, inserted, tt.fixText, "text that the fix of %s inserts", tt.id)
			}
		})
	}
}

// TestCatalogNamesMarkdownlintsRules checks lint.IsMarkdownlintRule and
// lint.IsGomdlintOnlyRule against the catalog: each rule that the catalog
// describes is one of markdownlint 0.41.1's 53 rules or one of gomdlint's
// own, and never both. With the sizes of the two sets, which an internal
// test of pkg/lint pins, no other ID is in either.
func TestCatalogNamesMarkdownlintsRules(t *testing.T) {
	markdownlintRules, gomdlintRules := 0, 0
	for _, meta := range Catalog() {
		assert.NotEqual(t, lint.IsGomdlintOnlyRule(meta.ID), lint.IsMarkdownlintRule(meta.ID), meta.ID)
		if lint.IsMarkdownlintRule(meta.ID) {
			markdownlintRules++
		}
		if lint.IsGomdlintOnlyRule(meta.ID) {
			gomdlintRules++
		}
	}
	assert.Equal(t, 53, markdownlintRules, "markdownlint 0.41.1 has 53 rules")
	assert.Equal(t, 2, gomdlintRules)
	for _, id := range []string{"MD002", "MD006", "MD057", "MD900", "MDL002", "MDL900", "md013", ""} {
		assert.False(t, lint.IsMarkdownlintRule(id), id)
		assert.False(t, lint.IsGomdlintOnlyRule(id), id)
	}
}
