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
	"github.com/yuin/goldmark"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// blockChecks holds both region checks of a document under one flavor, with
// the whole-document checks they stand for.
type blockChecks struct {
	snapshotContent []byte
	check           *fixCheck
	verifier        *fixVerifier
	outline         *outlineModel
}

// newBlockChecks builds both region checks of text under flavor, the
// outline's with the allowance allow.
func newBlockChecks(t *testing.T, flavor config.Flavor, text string, allow fixAllowance) blockChecks {
	t.Helper()
	snapshot := parseForCheckWith(t, flavor, text)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	verifier := newFixVerifier(lint.NewRuleContext(context.Background(), snapshot, cfg, nil), allow)
	return blockChecks{
		// The checks read a carriage return alone as a line feed, as the
		// parser does (see checkedContent).
		snapshotContent: checkedContent(snapshot.Content),
		check:           newFixCheck(snapshot, flavor),
		verifier:        verifier,
		outline:         verifier.regions(),
	}
}

// decide checks edits, prepared, with both region checks against the
// whole-document checks, and reports whether each region check decided.
func (c blockChecks) decide(t *testing.T, prepared []fix.TextEdit, about string) (bool, bool) {
	t.Helper()
	ok, got := c.check.regionsUnchanged(prepared)
	if ok {
		want := c.check.unchanged(fix.ApplyEdits(c.snapshotContent, prepared))
		require.Equal(t, want, got, "render signature, %s, edits %q, fixed %q",
			about, prepared, fix.ApplyEdits(c.snapshotContent, prepared))
	}
	outlineOK := false
	if c.outline != nil {
		var outlineGot bool
		outlineOK, outlineGot = c.outline.checkEdits(prepared)
		if outlineOK {
			require.Equal(t, c.verifier.acceptsWhole(prepared), outlineGot, "outline, %s, edits %q, fixed %q",
				about, prepared, fix.ApplyEdits(c.snapshotContent, prepared))
		}
	}
	return ok, outlineOK
}

// kinds returns the regions of each kind that both region checks parsed.
func (c blockChecks) kinds() [regionKinds]int {
	var total [regionKinds]int
	add := func(work regionWork) {
		for kind, count := range work.kinds {
			total[kind] += count
		}
	}
	if c.outline != nil {
		add(c.outline.work)
	}
	for _, model := range c.check.models {
		if model != nil {
			add(model.work)
		}
	}
	return total
}

// TestQuoteGroupsStart checks where docLayout's groups of top-level
// blockquotes start: after a quoted blank line, after a closed block, which
// it marks, after a closed fence, whose next block follows no blank line,
// and after the quote line that opens a blockquote with no block, but not
// where a block stays open, nor in a blockquote nested in another.
func TestQuoteGroupsStart(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		starts      []string
		afterClosed []bool
		blank       []bool
	}{
		{"blank lines", "> a\n>\n> b\n>\n> c\n", []string{"> a", "> b", "> c"}, []bool{false, false, false}, []bool{true, true, true}},
		{"thematic break", "> a\n> ***\n> b\n", []string{"> a", "> b"}, []bool{false, true}, []bool{true, false}},
		{"ATX heading", "> # H\n> b\n", []string{"> # H", "> b"}, []bool{false, true}, []bool{true, false}},
		{"closed fence", "> ```\n> x\n> ```\n> b\n", []string{"> ```", "> b"}, []bool{false, false}, []bool{true, false}},
		{"empty first line", ">\n> a\n>\n> b\n", []string{"> a", "> b"}, []bool{false, false}, []bool{false, true}},
		{"lazy line", "> a\nlazy\n>\n> b\n", []string{"> a", "> b"}, []bool{false, false}, []bool{true, true}},
		{"list stays open", "> - a\n>\n> b\n", []string{"> - a"}, nil, nil},
		{"setext heading", "> H\n> ---\n> b\n", []string{"> H"}, nil, nil},
		{"nested quote", "> > a\n> >\n> > b\n", []string{"> > a"}, nil, nil},
	}
	gfm, commonMark := recordingInstances()
	for _, tt := range tests {
		for _, md := range []struct {
			name     string
			markdown goldmark.Markdown
		}{{"gfm", gfm}, {"commonmark", commonMark}} {
			t.Run(tt.name+"/"+md.name, func(t *testing.T) {
				layout := newDocLayout(md.markdown, []byte(tt.input))
				require.NotNil(t, layout)
				require.Len(t, layout.quotes, 1)
				quote := layout.quotes[0]
				var starts []string
				for _, offset := range quote.bounds {
					line, _, _ := strings.Cut(tt.input[offset:], "\n")
					starts = append(starts, line)
				}
				assert.Equal(t, tt.starts, starts)
				assert.Equal(t, len(tt.starts) > 1, quote.regions)
				if quote.regions {
					assert.Equal(t, tt.afterClosed, quote.afterClosed)
					assert.Equal(t, tt.blank, quote.blank)
				}
			})
		}
	}
}

// TestQuotesOpenTheirGroups checks which top-level blockquotes docLayout
// finds opening with no other block open, so that a quote region may start
// on their first line: one that starts its document group, after blank
// lines, front matter or a closed block, but not one that interrupts a
// paragraph or follows an open list.
func TestQuotesOpenTheirGroups(t *testing.T) {
	tests := []struct {
		name  string
		input string
		opens bool
	}{
		{"first block", "> a\n>\n> b\n", true},
		{"after blank lines", "\n\n> a\n>\n> b\n", true},
		{"after front matter", "---\nt: x\n---\n> a\n>\n> b\n", true},
		{"after a heading", "# h\n> a\n>\n> b\n", true},
		{"after a paragraph", "para\n> a\n>\n> b\n", false},
		{"after a list", "- a\n- e\n\n> a\n>\n> b\n", false},
		{"after definitions", "[r]: /u\n> a\n>\n> b\n", false},
	}
	gfm, _ := recordingInstances()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newDocLayout(gfm, []byte(tt.input))
			require.NotNil(t, layout)
			require.Len(t, layout.quotes, 1)
			assert.True(t, layout.quotes[0].regions)
			assert.Equal(t, tt.opens, layout.quotes[0].opensGroup)
			first := region{kind: quoteRegion, first: 0, last: 0}
			assert.Equal(t, tt.opens, layout.quoteRegionFits(first), "a quote region of the first group")
		})
	}
}

// TestTablesRecorded checks the tables docLayout records, with the lines of
// their header and body rows: at the top level, after paragraph text, in a
// blockquote with a lazy row, but not in a list item or under CommonMark.
func TestTablesRecorded(t *testing.T) {
	tests := []struct {
		name  string
		input string
		head  string
		rows  []string
	}{
		{"top level", "| a | b |\n| - | - |\n| 1 | 2 |\n| 3 | 4 |\n", "| a | b |", []string{"| 1 | 2 |", "| 3 | 4 |"}},
		{"after text", "text\n| a |\n| - |\n| 1 |\n", "| a |", []string{"| 1 |"}},
		{"in a quote", "> | a |\n> | - |\n> | 1 |\n| 2 |\n", "> | a |", []string{"> | 1 |", "| 2 |"}},
		{"without body rows", "| a |\n| - |\n", "", nil},
		{"in a list item", "- | a |\n  | - |\n  | 1 |\n", "", nil},
	}
	gfm, commonMark := recordingInstances()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout := newDocLayout(gfm, []byte(tt.input))
			require.NotNil(t, layout)
			if tt.rows == nil {
				assert.Empty(t, layout.tables)
				return
			}
			require.Len(t, layout.tables, 1)
			table := layout.tables[0]
			head, _, _ := strings.Cut(tt.input[table.head:], "\n")
			assert.Equal(t, tt.head, head)
			var rows []string
			for _, offset := range table.rows[:len(table.rows)-1] {
				line, _, _ := strings.Cut(tt.input[offset:], "\n")
				rows = append(rows, line)
			}
			assert.Equal(t, tt.rows, rows)
			assert.Equal(t, len(tt.input), table.rows[len(table.rows)-1])
			plain := newDocLayout(commonMark, []byte(tt.input))
			require.NotNil(t, plain)
			assert.Empty(t, plain.tables, "CommonMark has no tables")
		})
	}
}

// TestOutlineModelKeepsRegions checks that the outline model finds the parts
// of every list, blockquote and table that the region model finds them for,
// on every snippet: a block whose parts the outline does not match takes no
// regions of its own there, which would cost a parse of its container per
// check.
func TestOutlineModelKeepsRegions(t *testing.T) {
	type flags struct{ quotes, tables, lists []bool }
	flagsOf := func(layout *docLayout) flags {
		var got flags
		for _, quote := range layout.quotes {
			got.quotes = append(got.quotes, quote.regions)
		}
		for _, table := range layout.tables {
			got.tables = append(got.tables, table.regions)
		}
		for _, list := range layout.lists {
			got.lists = append(got.lists, list.regions)
		}
		return got
	}
	for _, snippet := range slices.Concat(regionSnippets, outlineSnippets) {
		if strings.TrimSpace(snippet) == "" {
			continue
		}
		for idx, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			checks := newBlockChecks(t, flavor, snippet, fixAllowance{})
			require.NotNil(t, checks.outline, "%q under %s", snippet, flavor)
			checks.check.ensureModels()
			model := checks.check.models[idx]
			require.NotNil(t, model, "%q under %s", snippet, flavor)
			assert.Equal(t, flagsOf(model.docLayout), flagsOf(checks.outline.docLayout), "%q under %s", snippet, flavor)
		}
	}
}

// longQuoteDocument returns a document with a blockquote of paragraphs, a
// heading and a break that blocks follow with no blank line, a closed fence
// that text follows, and a list, each line with trailing spaces.
func longQuoteDocument() string {
	var doc strings.Builder
	doc.WriteString("# Title\n\nIntro.\n\n")
	for idx := range 8 {
		fmt.Fprintf(&doc, "> Paragraph %d with trailing spaces   \n>\n", idx)
	}
	doc.WriteString("> ## Heading in the quote\n> Text right after the heading   \n> ***\n> Text right after a break   \n>\n")
	doc.WriteString("> ```\n> code in the quote\n> ```\n> Text right after a fence   \n>\n")
	for idx := range 8 {
		fmt.Fprintf(&doc, "> - item %d with trailing spaces   \n", idx)
	}
	doc.WriteString(">\n> Last paragraph of the quote.\n\nAfter the quote.\n")
	return doc.String()
}

// TestRegionChecksInBlockquotes checks quote regions and list regions in a
// blockquote against the whole-document checks on edits inside a long
// blockquote: edits that keep its blocks, that join or split its groups,
// that take a line out of it, make it lazy or nest it, that leave a fence
// open, that break a closed block or loosen a list in it, or that add a
// definition. Both region checks decide every edit but one that adds a
// definition and a link that looks it up, and quote and list regions decide
// some.
func TestRegionChecksInBlockquotes(t *testing.T) {
	doc := longQuoteDocument()
	edits := []struct {
		name, old, new string
	}{
		{"trailing spaces in a paragraph", "Paragraph 3 with trailing spaces   ", "Paragraph 3 with trailing spaces"},
		{"trailing spaces in an item", "item 4 with trailing spaces   ", "item 4 with trailing spaces"},
		{"quoted blank line removed", "spaces   \n>\n> Paragraph 4", "spaces   \n> Paragraph 4"},
		{"quoted blank line added", "> Paragraph 5 with", ">\n> Paragraph 5 with"},
		{"marker taken off after a blank line", "> Paragraph 5 with", "Paragraph 5 with"},
		{"lazy line", "spaces   \n>\n> Paragraph 6", "spaces   \nParagraph 6"},
		{"blank line ends the quote", ">\n> Paragraph 6", "\n> Paragraph 6"},
		{"nested quote", "> Paragraph 2 with", "> > Paragraph 2 with"},
		{"marker indented", "> Paragraph 2 with", "   > Paragraph 2 with"},
		{"marker indented as code", "> Paragraph 2 with", "    > Paragraph 2 with"},
		{"code in the quote", "> Paragraph 2 with", ">     Paragraph 2 with"},
		{"fence left open", "> ```\n> code in the quote\n> ```", "> ```\n> code in the quote\n>"},
		{"heading broken", "> ## Heading in the quote", "> ##Heading in the quote"},
		{"break made a setext underline", "> ***\n", "> ---\n"},
		{"blank line after a break", "> ***\n", "> ***\n>\n"},
		{"blank line after a fence", "> ```\n> Text right", "> ```\n>\n> Text right"},
		{"blank line between items", "spaces   \n> - item 5", "spaces   \n>\n> - item 5"},
		{"item nested", "> - item 5", ">   - item 5"},
		{"item made a paragraph", "> - item 5", "> item 5"},
		{"item made lazy", "> - item 5", "item 5"},
		{"item marker changed", "> - item 5", "> * item 5"},
		{"heading added", "> Paragraph 1 with", "> # Paragraph 1 with"},
		{"definition added", "> Paragraph 1 with", "> [x]: /x\n>\n> Paragraph 1 with"},
		{"definition and its link added", "> Paragraph 1 with", "> [x]: /x\n>\n> Paragraph 1 [x] with"},
	}
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		checks := newBlockChecks(t, flavor, doc, fixAllowance{})
		require.NotNil(t, checks.outline)
		decided := 0
		for _, tt := range edits {
			require.Equal(t, 1, strings.Count(doc, tt.old), tt.name)
			ok, outlineOK := checks.decide(t, []fix.TextEdit{replaceOnce(t, doc, tt.old, tt.new)}, tt.name)
			if ok && outlineOK {
				decided++
			}
		}
		// A definition reaches links before the region as well as after, so
		// the whole document decides an edit whose region looks up a label
		// it defines; one that no link looks up leaves every link as it was.
		assert.Equal(t, len(edits)-1, decided, "flavor %s: both region checks decide every edit but one", flavor)
		kinds := checks.kinds()
		assert.Positive(t, kinds[quoteRegion], "quote regions decide some edits")
		assert.Positive(t, kinds[listRegion], "list regions in the quote decide some edits")
	}
}

// longTableDocument returns a document with a table of rows with trailing
// spaces, and the same table in a blockquote with a lazy row.
func longTableDocument() string {
	var doc strings.Builder
	doc.WriteString("# Title\n\nIntro.\n\n| Name | Value |\n| ---- | :---: |\n")
	for idx := range 10 {
		fmt.Fprintf(&doc, "| row %d | value %d |   \n", idx, idx)
	}
	doc.WriteString("\nBetween the tables.\n\n> | Quoted | Value |\n> | ------ | ----- |\n")
	for idx := range 10 {
		fmt.Fprintf(&doc, "> | quoted %d | value %d |   \n", idx, idx)
		if idx == 4 {
			doc.WriteString("| lazy row | lazy value |\n")
		}
	}
	doc.WriteString("\nAfter the tables.\n")
	return doc.String()
}

// TestRegionChecksInTables checks table regions against the whole-document
// checks on edits of the body rows of a table and of a table in a
// blockquote: edits that keep the rows, change their cells, split or join
// them, end the table with a blank line or another block, make a row a
// setext underline, or take the blockquote marker off a row.
func TestRegionChecksInTables(t *testing.T) {
	doc := longTableDocument()
	edits := []struct {
		name, old, new string
	}{
		{"trailing spaces", "| row 3 | value 3 |   ", "| row 3 | value 3 |"},
		{"trailing spaces in the quote", "| quoted 3 | value 3 |   ", "| quoted 3 | value 3 |"},
		{"cell changed", "| row 4 | value 4 |", "| row 4 | other |"},
		{"cell added", "| row 4 | value 4 |", "| row 4 | value 4 | more |"},
		{"pipe removed", "| row 4 | value 4 |", "| row 4 value 4 |"},
		{"escaped pipe", "| row 4 | value 4 |", "| row 4 \\| value 4 |"},
		{"row split", "| row 5 | value 5 |", "| row 5 |\n| value 5 |"},
		{"rows joined", "| row 5 | value 5 |   \n", "| row 5 | value 5 |   "},
		{"blank line ends the table", "| row 6 | value 6 |   \n", "| row 6 | value 6 |   \n\n"},
		{"row made a heading", "| row 6 | value 6 |", "# row 6 | value 6 |"},
		{"row made a list item", "| row 6 | value 6 |", "- row 6 | value 6 |"},
		{"row made a setext underline", "| row 7 | value 7 |", "---"},
		{"row made a delimiter row", "| row 7 | value 7 |", "| --- | --- |"},
		{"row indented", "| row 7 | value 7 |", "    | row 7 | value 7 |"},
		{"last row", "| row 9 | value 9 |   ", "| row 9 | value 9 |"},
		{"first row", "| row 0 | value 0 |   ", "| row 0 | value 0 |"},
		{"quoted row made lazy", "> | quoted 6 | value 6 |", "| quoted 6 | value 6 |"},
		{"lazy row", "| lazy row | lazy value |", "| lazy row | other |"},
		{"quoted blank line ends the table", "> | quoted 7 | value 7 |   \n", "> | quoted 7 | value 7 |   \n>\n"},
		{"quoted row nested", "> | quoted 8 | value 8 |", "> > | quoted 8 | value 8 |"},
	}
	checks := newBlockChecks(t, config.FlavorGFM, doc, fixAllowance{})
	require.NotNil(t, checks.outline)
	decided := 0
	for _, tt := range edits {
		require.Equal(t, 1, strings.Count(doc, tt.old), tt.name)
		ok, outlineOK := checks.decide(t, []fix.TextEdit{replaceOnce(t, doc, tt.old, tt.new)}, tt.name)
		if ok && outlineOK {
			decided++
		}
	}
	assert.Equal(t, len(edits), decided, "both region checks decide every edit")
	assert.Positive(t, checks.kinds()[tableRegion], "table regions decide some edits")
}

// TestTableRegionsAtTheEnd checks table regions on a table whose last row
// ends the document without a line ending, where an edit at the end of the
// document is on the last row's line: removing the row's trailing spaces,
// adding a line ending or a cell, or a paragraph after the table.
func TestTableRegionsAtTheEnd(t *testing.T) {
	doc := "# Title\n\n| a | b |\n| - | - |\n| 1 | 2 |   \n| 3 | 4 |   \n| 5 | 6 |   "
	end := len(doc)
	edits := []struct {
		name string
		edit fix.TextEdit
	}{
		{"trailing spaces", fix.TextEdit{StartOffset: end - 3, EndOffset: end}},
		{"line ending", fix.TextEdit{StartOffset: end, EndOffset: end, NewText: "\n"}},
		{"cell", fix.TextEdit{StartOffset: end, EndOffset: end, NewText: " 7 |"}},
		{"paragraph after", fix.TextEdit{StartOffset: end, EndOffset: end, NewText: "\n\nAfter."}},
	}
	checks := newBlockChecks(t, config.FlavorGFM, doc, fixAllowance{})
	require.NotNil(t, checks.outline)
	for _, tt := range edits {
		ok, outlineOK := checks.decide(t, []fix.TextEdit{tt.edit}, tt.name)
		assert.True(t, ok && outlineOK, tt.name)
		// Together with an edit of another row.
		other := replaceOnce(t, doc, "| 1 | 2 |   ", "| 1 | 2 |")
		ok, outlineOK = checks.decide(t, []fix.TextEdit{other, tt.edit}, tt.name+" with another row")
		assert.True(t, ok && outlineOK, tt.name)
	}
	assert.Positive(t, checks.kinds()[tableRegion])
}

// TestQuoteRegionsStartWhereTheQuoteContinues checks an edit of the line
// that opens a blockquote after an open list: indented, the line joins the
// list's last item, which a parse of the blockquote's group alone cannot
// see, so a quote region does not start there.
func TestQuoteRegionsStartWhereTheQuoteContinues(t *testing.T) {
	doc := "# Title\n\n- a\n- e\n\n> # Quoted heading\n> Text after the heading   \n> ***\n> Text after a break   \n>\n> End of quote\n"
	at := strings.Index(doc, "> # Quoted")
	edit := []fix.TextEdit{{StartOffset: at, EndOffset: at, NewText: "   "}}
	for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
		checks := newBlockChecks(t, flavor, doc, fixAllowance{})
		require.NotNil(t, checks.outline)
		require.False(t, checks.check.unchanged(fix.ApplyEdits([]byte(doc), edit)), "the quote joins the list")
		ok, outlineOK := checks.decide(t, edit, "indented first line")
		assert.True(t, ok)
		assert.True(t, outlineOK)
		// Edits after the first group of the blockquote still get quote
		// regions.
		after := replaceOnce(t, doc, "after a break   ", "after a break")
		ok, outlineOK = checks.decide(t, []fix.TextEdit{after}, "trailing spaces")
		assert.True(t, ok && outlineOK)
		assert.Positive(t, checks.kinds()[quoteRegion])
	}
}

// blockSnippets are the snippets of regionSnippets with blockquotes or
// tables, which quote, list and table regions are for.
func blockSnippets() []string {
	var snippets []string
	for _, snippet := range regionSnippets {
		if strings.Contains(snippet, ">") || strings.Contains(snippet, "|") {
			snippets = append(snippets, snippet)
		}
	}
	return snippets
}

// TestRegionChecksInBlocksDecideAsWholeDocument checks both region checks
// against the whole-document checks on random documents made mostly of
// blockquotes and tables, under both flavors, with random edits alone and in
// sets, so that quote, list and table regions decide many of them.
func TestRegionChecksInBlocksDecideAsWholeDocument(t *testing.T) {
	documents := 400
	if testing.Short() || raceEnabled {
		documents = 60
	}
	snippets := blockSnippets()
	rng := rand.New(rand.NewPCG(20261019, 3))
	total, decided := 0, 0
	var kinds [regionKinds]int
	for range documents {
		var doc strings.Builder
		for range 2 + rng.IntN(6) {
			if rng.IntN(4) == 0 {
				doc.WriteString(regionSnippets[rng.IntN(len(regionSnippets))])
			} else {
				doc.WriteString(snippets[rng.IntN(len(snippets))])
			}
			switch rng.IntN(3) {
			case 0:
				doc.WriteString("\n")
			case 1:
				doc.WriteString(">\n")
			}
		}
		text := doc.String()
		switch rng.IntN(12) {
		case 0:
			text = "---\ntitle: x\n---\n" + text
		case 1:
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		for _, flavor := range []config.Flavor{config.FlavorGFM, config.FlavorCommonMark} {
			allow := outlineAllowances[rng.IntN(len(outlineAllowances))]
			checks := newBlockChecks(t, flavor, text, allow)
			for trial := range 8 {
				var edits []fix.TextEdit
				if trial%4 == 3 {
					edits = randomOutlineEdits(rng, text)
				} else {
					edits = randomRegionEdits(rng, text, trial%2 == 0)
				}
				prepared, err := fix.PrepareEdits(edits, len(text))
				if err != nil || len(prepared) == 0 {
					continue
				}
				total++
				ok, outlineOK := checks.decide(t, prepared, fmt.Sprintf("flavor %s, allow %+v, document %q", flavor, allow, text))
				if ok && outlineOK {
					decided++
				}
			}
			for kind, count := range checks.kinds() {
				kinds[kind] += count
			}
		}
	}
	t.Logf("%d edit sets, %d decided by both region checks, regions parsed by kind %v", total, decided, kinds)
	assert.Greater(t, decided, total/2, "region checks decide most edit sets")
	assert.Positive(t, kinds[quoteRegion], "quote regions are parsed")
	assert.Positive(t, kinds[listRegion], "list regions are parsed")
	assert.Positive(t, kinds[tableRegion], "table regions are parsed")
}

// TestRegionParsesGrowInBlocks checks that region checks parse text that
// grows with the document, not with its square, when the fixes of a document
// sit in one long list in a blockquote, one blockquote of paragraphs or one
// long table, on its own or in a blockquote. Each fix is checked on its own,
// as fixes that share a block are; four times the document must take about
// four times the bytes, not sixteen. Tables are GFM's: under CommonMark a
// table is one paragraph, whose every check parses it whole.
func TestRegionParsesGrowInBlocks(t *testing.T) {
	const small, factor = 50, 4
	both := []config.Flavor{config.FlavorGFM, config.FlavorCommonMark}
	for _, doc := range []struct {
		name     string
		generate func(int) []byte
		flavors  []config.Flavor
	}{
		{name: "list in a blockquote", generate: quoteListDocument, flavors: both},
		{name: "paragraphs in a blockquote", generate: quoteParagraphsDocument, flavors: both},
		{name: "table", generate: tableRowsDocument, flavors: []config.Flavor{config.FlavorGFM}},
		{name: "table in a blockquote", generate: quoteTableDocument, flavors: []config.Flavor{config.FlavorGFM}},
	} {
		t.Run(doc.name, func(t *testing.T) {
			parsed := func(n int) int {
				input := string(doc.generate(n))
				edits := trailingSpaceEdits(input)
				require.Len(t, edits, n)
				total := 0
				for _, flavor := range doc.flavors {
					checks := newBlockChecks(t, flavor, input, fixAllowance{})
					require.NotNil(t, checks.outline)
					for _, edit := range edits {
						ok, outlineOK := checks.decide(t, []fix.TextEdit{edit}, "trailing spaces")
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
