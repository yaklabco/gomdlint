package rules

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
)

// definitionsDocument returns a document with definitions that links use,
// that a link looks up without finding, and that no link uses, at the top
// level, in a blockquote and in a list, and labels in escaped brackets,
// which no link looks up.
func definitionsDocument() string {
	return "# Title\n\n" +
		"Intro links to [used], [Twice] and ![logo], and mentions [missing].\n\n" +
		"[used]: https://example.com/used\n" +
		"[spare]: https://example.com/spare\n" +
		"[twice]: https://example.com/twice\n" +
		"[TWICE]: https://example.com/twice-again\n" +
		"[logo]: /logo.png \"Logo\"\n" +
		"[alone]: /alone\n\n" +
		"> Quoted [inner] link.\n>\n" +
		"> [inner]: /inner\n" +
		"> [quote spare]: /quote-spare\n>\n" +
		"> End of the quote.\n\n" +
		"- First item.\n\n" +
		"- Item with [listed].\n\n" +
		"  [listed]: /listed\n" +
		"  [list spare]: /list-spare\n\n" +
		"- Third item.\n\n" +
		"- Fourth item.\n\n" +
		"Closing \\[spare\\] and \\[fresh\\] paragraph.\n"
}

// TestRegionChecksWithDefinitions checks both region checks against the
// whole-document checks on edits that change link reference definitions.
// Region checks decide an edit whose changed labels no link looks up, in
// the original or in the edited regions: removing a definition that no link
// uses or one that a definition before it hides, changing or adding one
// that no link uses, at the top level, in a blockquote and in a list item.
// They leave to the whole document an edit that changes a label a link
// looks up, before or after the definition, and an edit that makes a link
// of escaped brackets, whose region would resolve it through the original's
// definitions.
func TestRegionChecksWithDefinitions(t *testing.T) {
	doc := definitionsDocument()
	type change struct{ old, new string }
	cases := []struct {
		name    string
		changes []change
		decided bool
	}{
		{"unused definition removed", []change{{"[spare]: https://example.com/spare\n", ""}}, true},
		{"hidden duplicate removed", []change{{"[TWICE]: https://example.com/twice-again\n", ""}}, true},
		{"unused destination changed", []change{{"[alone]: /alone", "[alone]: /elsewhere"}}, true},
		{"definition added for a new label", []change{{"[alone]: /alone\n", "[alone]: /alone\n[new]: /new\n"}}, true},
		{"definition respaced", []change{{"[used]: https://example.com/used", "[used]:   https://example.com/used"}}, true},
		{"unused definition in a quote removed", []change{{"> [quote spare]: /quote-spare\n", ""}}, true},
		{"unused definition in an item removed", []change{{"  [list spare]: /list-spare\n", ""}}, true},
		{"link to a kept definition made", []change{
			{"[spare]: https://example.com/spare\n", ""}, {"Closing \\[spare\\]", "Closing [used]"},
		}, true},
		{"used definition removed", []change{{"[used]: https://example.com/used\n", ""}}, false},
		{"first of two definitions removed", []change{{"[twice]: https://example.com/twice\n", ""}}, false},
		{"used title changed", []change{{"[logo]: /logo.png \"Logo\"", "[logo]: /logo.png \"Mark\""}}, false},
		{"definition added for a label a link looks up", []change{
			{"[alone]: /alone\n", "[alone]: /alone\n[missing]: /found\n"},
		}, false},
		{"used definition in an item changed", []change{{"[listed]: /listed", "[listed]: /moved"}}, false},
		{"link to a removed definition made", []change{
			{"[spare]: https://example.com/spare\n", ""}, {"Closing \\[spare\\]", "Closing [spare]"},
		}, false},
		{"link to an added definition made", []change{
			{"[alone]: /alone\n", "[alone]: /alone\n[fresh]: /fresh\n"}, {"\\[fresh\\]", "[fresh]"},
		}, false},
	}
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		checks := newBlockChecks(t, flavor, doc, fixAllowance{})
		require.NotNil(t, checks.outline)
		for _, tt := range cases {
			edits := make([]fix.TextEdit, 0, len(tt.changes))
			for _, change := range tt.changes {
				edits = append(edits, replaceOnce(t, doc, change.old, change.new))
			}
			prepared, err := fix.PrepareEdits(edits, len(doc))
			require.NoError(t, err)
			about := fmt.Sprintf("flavor %s, %s", flavor, tt.name)
			ok, outlineOK := checks.decide(t, prepared, about)
			assert.Equal(t, tt.decided, ok, "render signature, %s", about)
			assert.Equal(t, tt.decided, outlineOK, "outline, %s", about)
		}
	}
}

// TestTableRegionsWithDefinitionHeaders checks table regions whose header
// row is a link reference definition when it starts a paragraph, though in
// the document it continues one: a code span or a pipe in the label hides
// it from the table there. The parse of such a region adds a definition the
// document lacks and takes the first body row for the delimiter row, so the
// region must not hold, and both checks decide by the table's group.
func TestTableRegionsWithDefinitionHeaders(t *testing.T) {
	// Each edit puts a space after the first pipe of the first body row,
	// which the row before the one named by row locates.
	cases := []struct{ doc, row string }{
		{"Intro text\n[`a]: /url`\n| - |\n| - |\n| c |\n| d |\n", "| - |\n| c |"},
		{"Intro text\n[a|b]: /url\n| - | - |\n| - | - |\n| c | d |\n| e | f |\n", "| - | - |\n| c |"},
		{"> Intro text\n> [`a]: /url`\n> | - |\n> | - |\n> | c |\n> | d |\n", "| - |\n> | c |"},
	}
	for _, tt := range cases {
		require.Equal(t, 1, strings.Count(tt.doc, tt.row), tt.doc)
		at := strings.Index(tt.doc, tt.row) + 1
		edit := fix.TextEdit{StartOffset: at, EndOffset: at, NewText: " "}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			checks := newBlockChecks(t, flavor, tt.doc, fixAllowance{})
			require.NotNil(t, checks.outline)
			ok, outlineOK := checks.decide(t, []fix.TextEdit{edit}, fmt.Sprintf("flavor %s, document %q", flavor, tt.doc))
			assert.True(t, ok, "render signature, %s, %q", flavor, tt.doc)
			assert.True(t, outlineOK, "outline, %s, %q", flavor, tt.doc)
			if flavor != config.FlavorGFM {
				continue
			}
			layout := checks.outline.docLayout
			require.Len(t, layout.tables, 1, "%q", tt.doc)
			placed, _, placedOK := layout.placeEdits([]fix.TextEdit{edit})
			require.True(t, placedOK)
			parse := layout.parseTableRegion(region{kind: tableRegion}, []fix.TextEdit{placed[0].edit})
			assert.False(t, parse.holds, "the table region of %q holds", tt.doc)
		}
	}
}

// definitionSnippets hold link reference definitions and links that use
// them, links that look up a label no definition has, and labels shown as
// text: duplicates, labels that differ in case or spacing, titles,
// definitions in a blockquote and a list, one a paragraph keeps as text,
// one over three lines, and labels in a table, a heading, a code span and
// escaped brackets.
var definitionSnippets = []string{
	"See [docs], [Docs][] and [the guide][guide].\n",
	"An image ![logo] and a [missing] label.\n",
	"[docs]: https://example.com/docs\n",
	"[docs]: https://example.com/other \"Other\"\n",
	"[DOCS]: /upper\n",
	"[guide]: <./guide page.md> 'Guide'\n",
	"[logo]: /logo.png\n[spare]: /spare\n",
	"[spare]: /spare\n[unused]: /unused\n\nText after definitions.\n",
	"[a  b]: /spaced\n\nLinks to [a b] and [A\tB].\n",
	"Escaped \\[docs\\] and \\[spare\\] and \\[fresh\\] brackets.\n",
	"## Heading with [docs] and [spare]\n",
	"> [quoted]: /quoted\n>\n> Uses [quoted] and [docs].\n>\n> [spare]: /quoted-spare\n",
	"- [item]: /item\n  text of the item\n- uses [item]\n- [unused]: /unused-item\n- fourth\n",
	"Paragraph text\n[lazy]: /lazy\n",
	"[multi]:\n/multi\n\"Title\"\n",
	"| [docs] | b |\n| - | - |\n| [spare] | 2 |\n",
	"`[docs]` in code and [docs] outside.\n",
	"[fresh] is looked up before it is defined.\n",
}

// definitionEditTexts are the definitions and links that random definition
// edits insert.
var (
	definitionEditTexts = []string{
		"[docs]: /new\n", "[fresh]: /fresh\n", "[missing]: /found\n", "[Spare]: /spare\n",
		"[spare]: /elsewhere\n", "[logo]: /logo.png\n", "[unused]: /unused \"T\"\n", "> [quoted]: /q2\n",
	}
	definitionLinkTexts = []string{" [spare]", " [fresh]", " [docs]", " [missing]", " ![logo]", " [Unused][]"}
	definitionLine      = regexp.MustCompile(`(?m)^.*\]:.*$`)
	escapedBracket      = regexp.MustCompile(`\\[\[\]]`)
)

// randomDefinitionDocument joins random snippets, most of them about
// definitions, sometimes after front matter or a byte order mark, sometimes
// with CRLF line endings.
func randomDefinitionDocument(rng *rand.Rand) string {
	var doc strings.Builder
	if rng.IntN(10) == 0 {
		doc.WriteString("---\ntitle: x\n---\n")
	}
	for range 2 + rng.IntN(8) {
		switch rng.IntN(4) {
		case 0:
			doc.WriteString(regionSnippets[rng.IntN(len(regionSnippets))])
		case 1:
			doc.WriteString(outlineSnippets[rng.IntN(len(outlineSnippets))])
		default:
			doc.WriteString(definitionSnippets[rng.IntN(len(definitionSnippets))])
		}
		if rng.IntN(3) > 0 {
			doc.WriteString("\n")
		}
	}
	text := doc.String()
	if rng.IntN(10) == 0 {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if rng.IntN(15) == 0 {
		text = "\xef\xbb\xbf" + text
	}
	return text
}

// randomDefinitionEdits returns one to three edits of definitions and links:
// deleting a definition's line, changing its destination, inserting a
// definition at the start of a line or a link at the end of one, removing
// the backslash that escapes a bracket, or a random edit.
func randomDefinitionEdits(rng *rand.Rand, text string) []fix.TextEdit {
	var edits []fix.TextEdit
	for range 1 + rng.IntN(3) {
		lineStart := strings.LastIndexByte(text[:rng.IntN(len(text)+1)], '\n') + 1
		lineEnd := len(text)
		if end := strings.IndexAny(text[lineStart:], "\r\n"); end >= 0 {
			lineEnd = lineStart + end
		}
		lines := definitionLine.FindAllStringIndex(text, -1)
		escapes := escapedBracket.FindAllStringIndex(text, -1)
		switch kind := rng.IntN(6); {
		case kind == 0 && len(lines) > 0:
			// Delete the line and its line ending, as MD053 does.
			line := lines[rng.IntN(len(lines))]
			edits = append(edits, fix.TextEdit{StartOffset: line[0], EndOffset: min(line[1]+1, len(text))})
		case kind == 1 && len(lines) > 0:
			line := lines[rng.IntN(len(lines))]
			at := line[0] + strings.Index(text[line[0]:line[1]], "]:") + len("]:")
			edits = append(edits, fix.TextEdit{StartOffset: at, EndOffset: line[1], NewText: " /changed"})
		case kind == 2:
			edits = append(edits, fix.TextEdit{
				StartOffset: lineStart, EndOffset: lineStart,
				NewText: definitionEditTexts[rng.IntN(len(definitionEditTexts))],
			})
		case kind == 3:
			edits = append(edits, fix.TextEdit{
				StartOffset: lineEnd, EndOffset: lineEnd,
				NewText: definitionLinkTexts[rng.IntN(len(definitionLinkTexts))],
			})
		case kind == 4 && len(escapes) > 0:
			escape := escapes[rng.IntN(len(escapes))]
			edits = append(edits, fix.TextEdit{StartOffset: escape[0], EndOffset: escape[0] + 1})
		default:
			edits = append(edits, randomRegionEdit(rng, text))
		}
	}
	return edits
}

// definitionsOf returns the link reference definitions that a recording
// parse of content adds, in order, reading it as docLayout does.
func definitionsOf(flavor config.Flavor, content []byte) []recordedRef {
	source, _, _ := layoutSource(content)
	return parseRecorded(recordingMarkdown(string(flavor)), source, nil, false, false).rec.refs
}

// TestDefinitionRegionsDecideAsWholeDocument checks both region checks
// against the whole-document checks on random documents of definitions and
// links, and random edits of them: every decision a region check makes
// equals the whole document's, and region checks decide edits that change
// the definitions.
func TestDefinitionRegionsDecideAsWholeDocument(t *testing.T) {
	documents := 600
	if testing.Short() || raceEnabled {
		documents = 100
	}
	rng := rand.New(rand.NewPCG(20261019, 53))
	total, decided, changing := 0, 0, 0
	for range documents {
		text := randomDefinitionDocument(rng)
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			allow := outlineAllowances[rng.IntN(len(outlineAllowances))]
			checks := newBlockChecks(t, flavor, text, allow)
			before := definitionsOf(flavor, []byte(text))
			for range 10 {
				prepared, err := fix.PrepareEdits(randomDefinitionEdits(rng, text), len(text))
				if err != nil || len(prepared) == 0 {
					continue
				}
				total++
				ok, outlineOK := checks.decide(t, prepared,
					fmt.Sprintf("flavor %s, allow %+v, document %q", flavor, allow, text))
				if !ok && !outlineOK {
					continue
				}
				decided++
				after := definitionsOf(flavor, fix.ApplyEdits([]byte(text), prepared))
				if !slices.EqualFunc(before, after, func(a, b recordedRef) bool { return sameRef(a.ref, b.ref) }) {
					changing++
				}
			}
		}
	}
	t.Logf("%d edit sets, %d decided by a region check, %d of them change the definitions", total, decided, changing)
	assert.Greater(t, decided, total/4, "region checks decide many edit sets")
	assert.Greater(t, changing, total/20, "region checks decide edit sets that change the definitions")
}

// spareDefinitionEdits returns the edits that delete the lines of the
// definitions no link uses in definitionListDocument and
// definitionQuoteDocument, as MD053 deletes them.
func spareDefinitionEdits(text string) []fix.TextEdit {
	var edits []fix.TextEdit
	for start := 0; start < len(text); {
		end := len(text)
		if next := strings.IndexByte(text[start:], '\n'); next >= 0 {
			end = start + next + 1
		}
		if strings.Contains(text[start:end], "[spare ") {
			edits = append(edits, fix.TextEdit{StartOffset: start, EndOffset: end})
		}
		start = end
	}
	return edits
}

// TestDefinitionChecksGrowInBlocks checks that the region checks of fixes
// that delete definitions in a long list or blockquote, each checked on its
// own as fixes that share a block are, parse bytes that grow with the
// document. Such a fix changed the definitions, which made both checks give
// up and parse the whole document, so four times the document took sixteen
// times the bytes. The bytes counted are those of the region parses and of
// the whole-document parses of both checks.
func TestDefinitionChecksGrowInBlocks(t *testing.T) {
	const small, factor = 50, 4
	for _, doc := range []struct {
		name     string
		generate func(int) []byte
	}{
		{name: "definitions in a list", generate: definitionListDocument},
		{name: "definitions in a blockquote", generate: definitionQuoteDocument},
	} {
		t.Run(doc.name, func(t *testing.T) {
			parsed := func(n int) int {
				input := string(doc.generate(n))
				edits := spareDefinitionEdits(input)
				require.Len(t, edits, n)
				total := 0
				for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
					checks := newBlockChecks(t, flavor, input, fixAllowance{})
					require.NotNil(t, checks.outline)
					for _, edit := range edits {
						require.True(t, checks.verifier.accepts([]fix.TextEdit{edit}), "outline, edit %+v", edit)
						// The render signature holds a token for each
						// definition, so this check withholds the fix.
						assert.False(t, checks.check.keeps([]fix.TextEdit{edit}), "render signature, edit %+v", edit)
					}
					total += checks.verifier.wholeParsed + checks.outline.work.parsed + checks.check.wholeParsed
					for _, model := range checks.check.models {
						require.NotNil(t, model)
						total += model.work.parsed
					}
				}
				return total
			}
			short, long := parsed(small), parsed(factor*small)
			ratio := float64(long) / float64(short)
			t.Logf("%d bytes parsed for %d, %d for %d, ratio %.1f", short, small, long, factor*small, ratio)
			assert.Less(t, ratio, 1.5*factor, "definition checks grow faster than the document")
		})
	}
}
