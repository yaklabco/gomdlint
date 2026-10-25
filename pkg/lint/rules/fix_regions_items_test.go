package rules

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// blankLineEdits returns an edit that inserts a blank line before each line
// of input for which needs reports true, given the line before it, both
// without their indentation and line ending, as MD058, MD031 and MD022 fix
// a block that text runs into inside a list item.
func blankLineEdits(input string, needs func(prev, line string) bool) []fix.TextEdit {
	var edits []fix.TextEdit
	prev := ""
	offset := 0
	for _, line := range strings.SplitAfter(input, "\n") {
		text := strings.TrimSpace(line)
		if prev != "" && needs(prev, text) {
			edits = append(edits, fix.TextEdit{StartOffset: offset, EndOffset: offset, NewText: "\n"})
		}
		prev = text
		offset += len(line)
	}
	return edits
}

// tableBlankLines reports a table's header row right after text.
func tableBlankLines(_, line string) bool {
	return strings.HasPrefix(line, "| a |")
}

// fenceBlankLines reports a fence's opening line right after text, and text
// right after a closing fence.
func fenceBlankLines(prev, line string) bool {
	return strings.HasPrefix(line, "```sh") || (prev == "```" && line != "")
}

// headingBlankLines reports a heading right after text, and text right after
// a heading.
func headingBlankLines(prev, line string) bool {
	return strings.HasPrefix(line, "## ") || (strings.HasPrefix(prev, "## ") && line != "")
}

// itemParts are blocks that list items hold in the differential tests,
// written without the item's indentation: text that runs into a table, a
// fence, a heading or HTML, tables, fences closed and left open, headings
// with text right after them, setext headings, HTML blocks of several kinds,
// definitions and links through them, thematic breaks, indented code, hard
// breaks, blockquotes and tabs. Nested lists come from randomItemList.
var itemParts = []string{
	"text\n| a | b |\n| - | - |\n",
	"| h1 | h2 |\n| -- | -- |\n| 1 | 2 |   \n| 3 | 4 |\n",
	"text before\n```sh\necho x  \n```\ntext after\n",
	"```\nfenced  \n```\n",
	"~~~\nunclosed fence\n",
	"## Heading in item\ntext after the heading\n",
	"text before a heading\n## Heading\n",
	"## Heading one\n",
	"Setext in item\n---\n",
	"<div>\nhtml  \n</div>\n",
	"<!-- comment -->\ntext after a comment\n",
	"<!--\nopen comment\n-->\n",
	"<custom-tag>\n",
	"[ref]: /url\n",
	"See [ref] and [missing].\n",
	"***\ntext after a break\n",
	"    indented code\n",
	"text with a hard break  \nand more   \n",
	"> quoted in item\n> more\n",
	"\ttab first\n",
	"- [ ] task\n",
	"plain paragraph\n",
	"another paragraph   \n",
}

// itemMarkers are the list markers the differential tests open items with.
var itemMarkers = []string{"- ", "* ", "-   ", "1. ", "2) ", "10. "}

// randomItemList returns a list of one to four items, each a marker line and
// parts or nested lists, indented to the item's content, with blank lines
// or none between them; some lines lose their indentation, as lazy
// continuation lines do.
func randomItemList(rng *rand.Rand, depth int) string {
	marker := itemMarkers[rng.IntN(len(itemMarkers))]
	indent := strings.Repeat(" ", len(marker))
	var list strings.Builder
	for item := range 1 + rng.IntN(4) {
		if item > 0 && rng.IntN(2) == 0 {
			list.WriteString("\n")
		}
		first := []string{"item text\n", "item text   \n", "## Item heading\n", "[ ] item task\n"}[rng.IntN(4)]
		list.WriteString(marker + first)
		for range rng.IntN(5) {
			switch rng.IntN(3) {
			case 0:
				list.WriteString("\n")
			case 1:
				list.WriteString(indent + "\n")
			}
			part := itemParts[rng.IntN(len(itemParts))]
			if depth < 2 && rng.IntN(4) == 0 {
				part = randomItemList(rng, depth+1)
			}
			for idx, line := range strings.SplitAfter(strings.TrimSuffix(part, "\n"), "\n") {
				switch {
				case line == "\n":
					list.WriteString(line)
				case idx > 0 && rng.IntN(12) == 0:
					list.WriteString(line)
				default:
					list.WriteString(indent + line)
				}
			}
			list.WriteString("\n")
		}
	}
	return list.String()
}

// randomItemDocument returns a document of lists from randomItemList, at the
// top level or in a blockquote, between other snippets, sometimes after
// front matter or a byte order mark, with LF, CRLF or CR line endings.
func randomItemDocument(rng *rand.Rand) string {
	var doc strings.Builder
	switch rng.IntN(12) {
	case 0:
		doc.WriteString("---\ntitle: x\n---\n")
	case 1:
		doc.WriteString("\xef\xbb\xbf")
	}
	doc.WriteString("# Title\n\n")
	for range 1 + rng.IntN(3) {
		switch rng.IntN(5) {
		case 0:
			doc.WriteString(regionSnippets[rng.IntN(len(regionSnippets))])
		case 1:
			for _, line := range strings.SplitAfter(strings.TrimSuffix(randomItemList(rng, 0), "\n"), "\n") {
				doc.WriteString("> " + line)
			}
			doc.WriteString("\n")
		default:
			doc.WriteString(randomItemList(rng, 0))
		}
		doc.WriteString("\n")
	}
	text := doc.String()
	switch rng.IntN(6) {
	case 0:
		text = strings.ReplaceAll(text, "\n", "\r\n")
	case 1:
		text = strings.ReplaceAll(text, "\n", "\r")
	}
	return text
}

// blankLineEdit returns an edit that inserts a line ending at the start of a
// random line of text, as the blank-line rules do.
func blankLineEdit(rng *rand.Rand, text string) fix.TextEdit {
	lineStart := strings.LastIndexAny(text[:rng.IntN(len(text)+1)], "\n\r") + 1
	ending := "\n"
	if strings.Contains(text, "\r\n") {
		ending = "\r\n"
	} else if strings.Contains(text, "\r") {
		ending = "\r"
	}
	return fix.TextEdit{StartOffset: lineStart, EndOffset: lineStart, NewText: ending}
}

// ruleFixes returns the fixes that the default rules offer for text under
// flavor, each on its own.
func ruleFixes(t *testing.T, flavor config.Flavor, text string) [][]fix.TextEdit {
	t.Helper()
	registry := lint.NewRegistry()
	RegisterAll(registry)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	engine := lint.NewEngine(goldmark.New(string(flavor)), registry)
	result, err := engine.LintFile(context.Background(), "doc.md", []byte(text), cfg)
	require.NoError(t, err)
	var fixes [][]fix.TextEdit
	for _, diag := range result.Diagnostics {
		if len(diag.FixEdits) > 0 {
			fixes = append(fixes, diag.FixEdits)
		}
	}
	return fixes
}

// TestItemRegionsDecideAsWholeDocument checks the region checks against the
// whole-document checks they stand for, on random documents made of lists
// whose items hold lists, tables, fences, headings and HTML, at the top
// level and in blockquotes, under both flavors and with LF, CRLF and CR line
// endings: every edit set that the render signature's, the outline's or the
// heading rules' item window decides, it decides as the whole document does.
// The edit sets are random edits, blank lines inserted at the start of a
// line, and the fixes that the default rules offer, each alone.
func TestItemRegionsDecideAsWholeDocument(t *testing.T) {
	documents := 120
	if testing.Short() || raceEnabled {
		documents = 25
	}
	checkItemRegions(t, rand.New(rand.NewPCG(20261025, 85)), documents)
}

// checkItemRegions runs TestItemRegionsDecideAsWholeDocument on documents
// random documents from rng.
func checkItemRegions(t *testing.T, rng *rand.Rand, documents int) {
	t.Helper()
	total, decided, windows := 0, 0, 0
	var kinds [regionKinds]int
	for range documents {
		text := randomItemDocument(rng)
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			allow := outlineAllowances[rng.IntN(len(outlineAllowances))]
			about := fmt.Sprintf("flavor %s, allow %+v, document %q", flavor, allow, text)
			checks := newBlockChecks(t, flavor, text, allow)
			snapshot := parseForCheckWith(t, flavor, text)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			checker := newFixChecker(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			candidates := ruleFixes(t, flavor, text)
			for trial := range 10 {
				switch trial % 3 {
				case 0:
					candidates = append(candidates, randomRegionEdits(rng, text, trial%2 == 0))
				case 1:
					candidates = append(candidates, []fix.TextEdit{blankLineEdit(rng, text)})
				default:
					candidates = append(candidates, randomOutlineEdits(rng, text))
				}
			}
			validator := fix.NewValidator([]byte(text))
			for _, edits := range candidates {
				// The engine drops a fix that splits a CRLF pair or a UTF-8
				// character before any check sees it.
				if _, err := validator.Validate(edits); err != nil {
					continue
				}
				// The checks read edits as they read the document (see
				// checkedEdits).
				prepared, err := fix.PrepareEdits(checkedEdits(edits), len(text))
				if err != nil || len(prepared) == 0 {
					continue
				}
				total++
				if ok, outlineOK := checks.decide(t, prepared, about); ok && outlineOK {
					decided++
				}
				intents := []fixIntent{{}}
				window, windowDecided, kept := checker.itemWindow(prepared, intents)
				if !windowDecided {
					continue
				}
				windows++
				if kept {
					kept = slices.EqualFunc(window.want, window.after, sameBlock) &&
						(slices.Equal(headingTexts(window.before), headingTexts(window.after)) || !checker.mayReferToAnchors())
				}
				require.Equal(t, checker.documentKept(prepared, intents), kept, "item window, %s, edits %q, fixed %q",
					about, prepared, fix.ApplyEdits([]byte(text), prepared))
			}
			for kind, count := range checks.kinds() {
				kinds[kind] += count
			}
		}
	}
	t.Logf("%d edit sets, %d decided by both region checks, %d by item windows, regions parsed by kind %v",
		total, decided, windows, kinds)
	assert.Greater(t, decided, total/2, "region checks decide most edit sets")
	assert.Positive(t, kinds[itemRegion], "item regions are parsed")
	assert.Positive(t, windows, "item windows decide some edit sets")
}

// itemCommentsDocument returns a document with one list item that holds,
// after its first paragraph, HTML comments over several lines, each followed
// right away by a line of text that ends in trailing spaces, which MD009
// fixes. No blank line separates a comment from the text after it.
func itemCommentsDocument(comments int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n- outer\n\n")
	for idx := range comments {
		fmt.Fprintf(&doc, "  <!--\n  comment %d\n  -->\n  text %d   \n", idx, idx)
	}
	doc.WriteString("- end\n")
	return []byte(doc.String())
}

// TestRegionParsesGrowInListItems checks that region checks parse text that
// grows with the document, not with its square, when the fixes of a document
// sit in one list item: blank lines before tables that follow text in the
// item or in a loose list nested in it, and around fences and headings that
// text runs into, and trailing spaces on text right after HTML comments. Each fix is checked on its own; four times the document
// must take about four times the bytes, not sixteen.
func TestRegionParsesGrowInListItems(t *testing.T) {
	const small, factor = 30, 4
	both := []config.Flavor{config.FlavorGFM, config.FlavorCommonMark}
	gfm := []config.Flavor{config.FlavorGFM}
	blankLines := func(needs func(prev, line string) bool) func(string) []fix.TextEdit {
		return func(input string) []fix.TextEdit { return blankLineEdits(input, needs) }
	}
	for _, doc := range []struct {
		name     string
		generate func(int) []byte
		edits    func(string) []fix.TextEdit
		perBlock int
		flavors  []config.Flavor
	}{
		{name: "tables in a list item", generate: itemTablesDocument, edits: blankLines(tableBlankLines), perBlock: 1, flavors: gfm},
		{name: "tables in a nested list", generate: nestedTablesDocument, edits: blankLines(tableBlankLines), perBlock: 1, flavors: gfm},
		{name: "fences in a list item", generate: itemFencesDocument, edits: blankLines(fenceBlankLines), perBlock: 2, flavors: both},
		{name: "headings in a list item", generate: itemHeadingsDocument, edits: blankLines(headingBlankLines), perBlock: 2, flavors: both},
		{name: "comments in a list item", generate: itemCommentsDocument, edits: trailingSpaceEdits, perBlock: 1, flavors: both},
	} {
		t.Run(doc.name, func(t *testing.T) {
			parsed := func(n int) int {
				input := string(doc.generate(n))
				edits := doc.edits(input)
				require.Len(t, edits, doc.perBlock*n)
				total := 0
				for _, flavor := range doc.flavors {
					checks := newBlockChecks(t, flavor, input, fixAllowance{})
					require.NotNil(t, checks.outline)
					for _, edit := range edits {
						ok, outlineOK := checks.decide(t, []fix.TextEdit{edit}, "blank line")
						require.True(t, ok, "render signature, edit %+v", edit)
						require.True(t, outlineOK, "outline, edit %+v", edit)
					}
					for _, model := range checks.check.models {
						require.NotNil(t, model)
						total += model.work.parsed
					}
					total += checks.outline.work.parsed
				}
				return total
			}
			short, long := parsed(small), parsed(factor*small)
			ratio := float64(long) / float64(short)
			t.Logf("%d bytes parsed for %d, %d for %d, ratio %.1f", short, small, long, factor*small, ratio)
			assert.Less(t, ratio, 1.5*factor, "region parses grow faster than the document")
		})
	}
}
