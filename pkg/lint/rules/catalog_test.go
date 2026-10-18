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
// document. The texts name those checks where they hold back whole classes
// of fixes: the heading-anchor check for MD018, MD020, MD026 and MD036, and
// the raw HTML check of keepSafeFixes for every rule that uses it.
func TestCatalogSaysWhetherEachRuleFixes(t *testing.T) {
	// rawHTMLChecked lists the rules whose fixes keepSafeFixes checks, so
	// that raw HTML it cannot model anywhere in a document holds back all of
	// their fixes.
	rawHTMLChecked := map[string]bool{
		"MD004": true, "MD005": true, "MD007": true, "MD009": true, "MD010": true, "MD012": true,
		"MD013": true, "MD027": true, "MD029": true, "MD030": true, "MD032": true, "MD047": true,
	}

	// partialFixes lists the fixing rules that report findings a user can
	// expect to meet without a fix. The rules in rawHTMLChecked are among
	// them, and the comments give each rule's other reasons.
	partialFixes := map[string]bool{
		"MD003": true, // Underlined headings are never converted.
		"MD004": true, // A marker change that would merge two lists.
		"MD005": true, // Bullet items, and items with more than one paragraph.
		"MD007": true, // Items with more than one paragraph.
		"MD009": true, // Raw HTML only.
		"MD010": true, // Tabs inside code are reported and never replaced.
		"MD012": true, // Raw HTML only.
		"MD013": true, // Only some long lines can be wrapped safely.
		"MD018": true, // A new heading may break links to heading anchors.
		"MD020": true, // A changed heading may break links to heading anchors.
		"MD026": true, // A changed heading may break links to heading anchors.
		"MD027": true, // Fence and HTML lines, and items with more than one paragraph.
		"MD029": true, // A list that starts at a wrong number is not renumbered.
		"MD030": true, // Items with more than one paragraph.
		"MD032": true, // Raw HTML only.
		"MD034": true, // Only some bare URLs can be wrapped safely.
		"MD035": true, // A dash rule under a paragraph would make a heading.
		"MD036": true, // Only some emphasized lines can become headings.
		"MD037": true, // Markers that may be meant literally.
		"MD040": true, // Only a language the code proves is inserted.
		"MD044": true, // Names inside raw HTML.
		"MD047": true, // Raw HTML only.
		"MD048": true, // A new fence that would end the block early.
		"MD049": true, // A new marker that could pair with nearby characters, or a reference label.
		"MD050": true, // A new marker that could pair with nearby characters, or a reference label.
		"MD053": true, // Text on the line after the definition.
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

// catalogFinding is a finding of one rule, by line and by whether it
// carries a fix.
type catalogFinding struct {
	line  int
	fixed bool
}

// TestCatalogTextsHoldForTheRules lints, with the built-in rules and their
// default options, inputs that pin claims the catalog texts make. Each case
// names a phrase that the rule's description, rationale or fix text must
// contain, and the findings that the claim behind the phrase predicts. A
// text fails here when its rule stops behaving as it says, and when it
// drops the phrase that states the behaviour, such as an exemption.
func TestCatalogTextsHoldForTheRules(t *testing.T) {
	// long has spaces past column 80, and short leaves room for a long last
	// word that ends past it.
	const (
		long  = "aaa bbb ccc ddd eee fff ggg hhh iii jjj kkk lll mmm nnn ooo ppp qqq rrr sss ttt uuu vvv"
		short = "aaa bbb ccc ddd eee fff ggg hhh iii jjj kkk lll mmm nnn ooo ppp"
	)
	tests := []struct {
		id, name, input string
		// phrase is text that the rule's catalog texts state the pinned
		// behaviour with.
		phrase string
		want   []catalogFinding
		// fixText is the text that the rule's fix inserts, when the case
		// pins it.
		fixText string
	}{
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
		{
			id: "MD012", name: "inline HTML elsewhere", phrase: noFixWithRawHTML, want: []catalogFinding{{3, false}},
			input: "# Title\n\n\n\nPress <kbd>Ctrl</kbd>.\n",
		},
		{id: "MD012", name: "no HTML", phrase: "deletes the extra blank lines", input: "# Title\n\n\n\nPress Ctrl.\n", want: []catalogFinding{{3, true}}},
		{
			id: "MD047", name: "HTML block", phrase: noFixWithRawHTML, want: []catalogFinding{{5, false}},
			input: "# Title\n\n<details>\n<summary>More</summary>\n</details>",
		},
		{id: "MD047", name: "comment and br", phrase: "other than comments and plain br", input: "# Title\n\nA<br>b <!-- note -->\nEnd", want: []catalogFinding{{4, true}}},

		// MD047 asks for a last line that counts as blank, not for a final
		// newline byte.
		{id: "MD047", name: "last line of text", phrase: "ends with a newline", input: "# Title\n\nText.", want: []catalogFinding{{3, true}}},
		{id: "MD047", name: "last line of spaces", phrase: "a last line of only spaces", input: "# Title\n\nText.\n "},
		{id: "MD047", name: "last line holding a comment", phrase: "or comments", input: "# Title\n\nText.\n<!-- end -->"},
		{id: "MD047", name: "last line holding a quote marker", phrase: "only spaces, > or comments", input: "# Title\n\n> Quote.\n>"},

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
		{id: "MD013", name: "lone link", phrase: "only a link or an image", input: "# Title\n\n[" + long + "](https://example.com/page)\n"},
		{id: "MD013", name: "lone image", phrase: "only a link or an image", input: "# Title\n\n![" + long + "](image.png)\n"},
		{
			id: "MD013", name: "definition with a title", phrase: "reference definitions",
			input: "# Title\n\nSee [x].\n\n[x]: https://example.com/page \"" + long + "\"\n",
		},
		{
			id: "MD013", name: "long last word", phrase: "only overflow is the last word",
			input: "# Title\n\n" + short + " https://example.com/a-path-that-runs-past-the-limit\n",
		},
		{id: "MD013", name: "paragraph line", phrase: "wraps long paragraph lines", input: "# Title\n\n" + long + "\n", want: []catalogFinding{{3, true}}},
		{id: "MD013", name: "heading", phrase: "when it is a heading", input: "# Title\n\n## " + long + "\n", want: []catalogFinding{{3, false}}},

		// MD038 allows one space on each side and a space beside a backtick.
		{id: "MD038", name: "allowed padding", phrase: "except one each side or by a backtick", input: "# Title\n\nUse `` `foo`` and `   ` and ` code ` here.\n"},
		{id: "MD038", name: "one-sided space", phrase: "deletes the extra spaces", input: "# Title\n\nUse ` code` here.\n", want: []catalogFinding{{3, true}}},

		// MDL001 ignores fragment links.
		{id: "MDL001", name: "fragment link", phrase: "other than #fragment links", input: "# Title\n\nSee [a](docs/a.md) and [top](#title).\n"},
		{
			id: "MDL001", name: "mixed kinds", phrase: "all relative or all absolute", want: []catalogFinding{{3, false}},
			input: "# Title\n\nSee [a](docs/a.md) and [b](https://example.com).\n",
		},

		// MD049 and MD050 allow asterisks inside a word, which underscores
		// cannot replace, and hold back fixes in reference labels.
		{id: "MD049", name: "asterisks inside a word", phrase: "except * inside a word", input: "# Title\n\n_one_ a*two*b\n"},
		{id: "MD050", name: "asterisks inside a word", phrase: "except ** inside a word", input: "# Title\n\n__one__ a**two**b\n"},
		{
			id: "MD049", name: "italic in a reference label", phrase: "the label of a reference link", want: []catalogFinding{{3, false}},
			input: "# Title\n\n_first_ [*second*]\n\n[*second*]: /target\n",
		},
		{
			id: "MD050", name: "bold in a reference label", phrase: "the label of a reference link", want: []catalogFinding{{3, false}},
			input: "# Title\n\n__first__ [**second**]\n\n[**second**]: /target\n",
		},

		// Other findings that fix texts name as reported without a fix.
		{
			id: "MD027", name: "quoted item with a second paragraph", phrase: "a list item that holds a second paragraph",
			input: "# Title\n\n>   - item\n>\n>     second paragraph\n", want: []catalogFinding{{3, false}},
		},
		{
			id: "MD053", name: "text after the definition", phrase: "other text follows it on the next line",
			input: "# Title\n\n[unused]: /target\nParagraph.\n", want: []catalogFinding{{3, false}},
		},
		{
			id: "MD053", name: "blank line after the definition", phrase: "deletes the unused or duplicate definition",
			input: "# Title\n\n[unused]: /target\n\nParagraph.\n", want: []catalogFinding{{3, true}},
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
	}

	texts := make(map[string]string) // rule ID -> description, rationale and fix text
	for _, meta := range Catalog() {
		texts[meta.ID] = meta.Description + " " + meta.Rationale + " " + meta.Fix
	}

	for _, tt := range tests {
		t.Run(tt.id+" "+tt.name, func(t *testing.T) {
			require.NotEmpty(t, tt.phrase, "each case names the phrase it pins")
			assert.Contains(t, texts[tt.id], tt.phrase, "the texts of %s state the behaviour this case pins", tt.id)

			diags := lintWithDefaultRules(t, tt.input, config.NewConfig())

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
