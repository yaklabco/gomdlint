package rules

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// trailingSectionsDocument returns a document of sections, each a heading
// and a paragraph of two lines that end in trailing spaces, the second
// longer than 80 characters. Every section gives MD009 two fixes and MD013
// one, so the document has three fixable findings per section.
func trailingSectionsDocument(sections int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range sections {
		fmt.Fprintf(&doc, "## Section %d\n\n", idx)
		fmt.Fprintf(&doc, "Paragraph %d has trailing spaces at the end of this line.   \n", idx)
		doc.WriteString("This line is long enough to go past the default limit of eighty characters, yes.   \n\n")
	}
	return []byte(doc.String())
}

// longListDocument returns a document with one list of items, each with a
// finding that MD009 and MD030 fix, and a nested item under every fifth.
func longListDocument(items int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range items {
		fmt.Fprintf(&doc, "-  Item %d has trailing spaces at the end of this line.   \n", idx)
		if idx%5 == 0 {
			fmt.Fprintf(&doc, "   - Nested %d item text\n", idx)
		}
	}
	return []byte(doc.String())
}

// adjacentListsDocument returns a document of pairs of lists that touch,
// whose bullets differ, so that MD032 reports the missing blank line after
// the first list and the one before the second, both fixed by one shared
// insertion.
func adjacentListsDocument(pairs int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range pairs {
		fmt.Fprintf(&doc, "- item %d\n* other %d\n\n", idx, idx)
	}
	return []byte(doc.String())
}

// codeListDocument returns a document with one loose list of items, each
// holding a fenced shell block whose commands carry a dollar prompt and no
// output, a finding that MD014 fixes at the start of a line of code. A fix
// there does not stay inside the text of one line, so the code and link
// rules check it together with every other fix in its top-level block.
func codeListDocument(items int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range items {
		fmt.Fprintf(&doc, "- Item %d shows a command:\n\n  ```\n  $ echo item%d\n  $ ls -la /tmp/item%d\n  ```\n\n", idx, idx, idx)
	}
	return []byte(doc.String())
}

// headingCodeDocument returns a document of sections whose headings end in a
// code span with a space on one side, a finding that MD038 fixes. The fix
// keeps the outline the check compares but changes the heading's anchor,
// so the check must work out the anchors of the edited document.
func headingCodeDocument(sections int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range sections {
		fmt.Fprintf(&doc, "## Section %d ` code`\n\nText of section %d.\n\n", idx, idx)
	}
	return []byte(doc.String())
}

// quoteListDocument returns a document with one list of items in a
// blockquote, each item a line that ends in trailing spaces, a finding that
// MD009 fixes. Every fix shares the blockquote with the others, so each is
// checked on its own.
func quoteListDocument(items int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range items {
		fmt.Fprintf(&doc, "> - Item %d has trailing spaces at the end of this line.   \n", idx)
	}
	return []byte(doc.String())
}

// quoteParagraphsDocument returns a document with one blockquote of
// paragraphs, each a line that ends in trailing spaces, followed by a line
// that holds only the blockquote marker.
func quoteParagraphsDocument(paragraphs int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range paragraphs {
		fmt.Fprintf(&doc, "> Paragraph %d has trailing spaces at the end of this line.   \n>\n", idx)
	}
	return []byte(doc.String())
}

// tableRowsDocument returns a document with one table whose body rows end in
// trailing spaces.
func tableRowsDocument(rows int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n| Name | Value |\n| ---- | ----- |\n")
	for idx := range rows {
		fmt.Fprintf(&doc, "| Row %d | Value %d |   \n", idx, idx)
	}
	return []byte(doc.String())
}

// quoteTableDocument returns a document with one table in a blockquote whose
// body rows end in trailing spaces.
func quoteTableDocument(rows int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n> | Name | Value |\n> | ---- | ----- |\n")
	for idx := range rows {
		fmt.Fprintf(&doc, "> | Row %d | Value %d |   \n", idx, idx)
	}
	return []byte(doc.String())
}

// definitionListDocument returns a document with one loose list of items,
// each a line with a reference link and, after a blank line, the definition
// the link uses and a definition that no link uses, a finding that MD053
// fixes by deleting its line. Every fix shares the list with the others, so
// each is checked on its own.
func definitionListDocument(items int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range items {
		fmt.Fprintf(&doc, "- Item %d links to [page %d].\n\n", idx, idx)
		fmt.Fprintf(&doc, "  [page %d]: https://example.com/page/%d\n", idx, idx)
		fmt.Fprintf(&doc, "  [spare %d]: https://example.com/spare/%d\n\n", idx, idx)
	}
	return []byte(doc.String())
}

// definitionQuoteDocument returns a document with one blockquote of
// paragraphs, each with a reference link and followed by the definition the
// link uses and a definition that no link uses, which MD053 deletes.
func definitionQuoteDocument(paragraphs int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range paragraphs {
		fmt.Fprintf(&doc, "> Paragraph %d links to [page %d].\n>\n", idx, idx)
		fmt.Fprintf(&doc, "> [page %d]: https://example.com/page/%d\n", idx, idx)
		fmt.Fprintf(&doc, "> [spare %d]: https://example.com/spare/%d\n>\n", idx, idx)
	}
	return []byte(doc.String())
}

// itemTablesDocument returns a document with one list item that holds, after
// its first paragraph, paragraphs that each run straight into a table, a
// finding that MD058 fixes with a blank line inside the loose item.
func itemTablesDocument(tables int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n- outer\n\n")
	for idx := range tables {
		fmt.Fprintf(&doc, "  text %d\n  | a | b |\n  | --- | --- |\n\n", idx)
	}
	return []byte(doc.String())
}

// nestedTablesDocument returns a document with one list item that holds a
// loose list, whose items each run their text straight into a table, a
// finding that MD058 fixes with a blank line inside the nested item.
func nestedTablesDocument(items int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n- outer\n\n")
	for idx := range items {
		fmt.Fprintf(&doc, "  - text %d\n    | a | b |\n    | --- | --- |\n\n", idx)
	}
	return []byte(doc.String())
}

// itemFencesDocument returns a document with one list item that holds, after
// its first paragraph, fenced code blocks with text right before and right
// after them, two findings that MD031 fixes with blank lines inside the
// loose item.
func itemFencesDocument(fences int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n- outer\n\n")
	for idx := range fences {
		fmt.Fprintf(&doc, "  text %d\n  ```sh\n  echo %d\n  ```\n  after %d\n\n", idx, idx, idx)
	}
	return []byte(doc.String())
}

// itemHeadingsDocument returns a document with one list item that holds,
// after its first paragraph, headings with text right before and right after
// them, two findings that MD022 fixes with blank lines inside the loose
// item.
func itemHeadingsDocument(headings int) []byte {
	var doc strings.Builder
	doc.WriteString("# Title\n\n- outer\n\n")
	for idx := range headings {
		fmt.Fprintf(&doc, "  text %d\n  ## Heading %d\n  after %d\n\n", idx, idx, idx)
	}
	return []byte(doc.String())
}

// lintDuration lints content with every default rule through the engine,
// as the CLI does, and returns how long it took, failing the test when it
// takes longer than limit.
func lintDuration(t *testing.T, content []byte, limit time.Duration) time.Duration {
	t.Helper()
	registry := lint.NewRegistry()
	RegisterAll(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	cfg := config.NewConfig()

	type outcome struct {
		result *lint.FileResult
		err    error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		result, err := engine.LintFile(context.Background(), "doc.md", content, cfg)
		done <- outcome{result: result, err: err}
	}()
	select {
	case got := <-done:
		elapsed := time.Since(start)
		require.NoError(t, got.err)
		require.Empty(t, got.result.RuleErrors)
		fixable := 0
		for _, diag := range got.result.Diagnostics {
			if diag.HasFix() {
				fixable++
			}
		}
		require.Positive(t, fixable, "the document has fixable findings")
		t.Logf("%d bytes, %d findings, %d with fixes: %v", len(content), len(got.result.Diagnostics), fixable, elapsed)
		return elapsed
	case <-time.After(limit):
		t.Fatalf("linting a %d byte document took longer than %v", len(content), limit)
		return 0
	}
}

// TestLintTimeWithManyFixes guards the time the fix checks take on a long
// document with thousands of fixable findings. Checking each fix by parsing
// the whole document again made linting grow with the square of the
// document: this document took about 88 seconds that way.
func TestLintTimeWithManyFixes(t *testing.T) {
	if testing.Short() {
		t.Skip("long document")
	}
	// The race detector slows every rule down several times, so it gets
	// smaller documents and more time, which a slow machine running other
	// packages may need; a check that is quadratic still runs far past the
	// limit on them, as it took about 9 seconds on 800 sections without it.
	limit := 5 * time.Second
	sections, items := 2500, 2500
	if raceEnabled {
		limit = 30 * time.Second
		sections, items = 800, 800
	}
	t.Run("sections", func(t *testing.T) {
		lintDuration(t, trailingSectionsDocument(sections), limit)
	})
	t.Run("long list", func(t *testing.T) {
		lintDuration(t, longListDocument(items), limit)
	})
	t.Run("code list", func(t *testing.T) {
		lintDuration(t, codeListDocument(items), limit)
	})
	// Each pair of findings shares one fix, which the check must take once:
	// with both copies, the check of all fixes together failed and fell
	// back to checking them one by one, and 2,000 pairs took 30 seconds.
	t.Run("adjacent lists", func(t *testing.T) {
		lintDuration(t, adjacentListsDocument(items), limit)
	})
}

// TestLintTimeGrowsLinearly checks that linting a document eight times as
// long takes about eight times as long, not sixty-four times, as it would if
// each fix check parsed the whole document.
func TestLintTimeGrowsLinearly(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	const (
		limit = 5 * time.Second
		// Smaller documents lint in tens of milliseconds, too little to time
		// on a busy machine.
		small  = 500
		factor = 8
		// A linear check takes about eight times as long on the larger
		// document and a quadratic one sixty-four times; the bound leaves
		// room for a busy machine.
		maxRatio = 24
	)
	for _, doc := range []struct {
		name     string
		generate func(int) []byte
	}{
		{name: "sections", generate: trailingSectionsDocument},
		{name: "long list", generate: longListDocument},
		{name: "code list", generate: codeListDocument},
		{name: "heading code", generate: headingCodeDocument},
		{name: "thematic breaks", generate: func(n int) []byte { return []byte(leafRunDocument(n, "***")) }},
		{name: "separate lists", generate: func(n int) []byte { return []byte(separateListsDocument(n)) }},
	} {
		t.Run(doc.name, func(t *testing.T) {
			// The fastest of a few runs is the least disturbed by other work
			// on the machine.
			fastest := func(content []byte) time.Duration {
				best := limit
				for range 3 {
					best = min(best, lintDuration(t, content, limit))
				}
				return best
			}
			short := fastest(doc.generate(small))
			long := fastest(doc.generate(factor * small))
			ratio := float64(long) / float64(short)
			t.Logf("%s: %v for %d, %v for %d, ratio %.1f", doc.name, short, small, long, factor*small, ratio)
			require.Less(t, ratio, float64(maxRatio), "lint time grows faster than the document")
		})
	}
}

// TestLintTimeGrowsLinearlyInBlocks checks that linting a long list in a
// blockquote, a blockquote of many paragraphs, a long table and a long list
// or blockquote of link reference definitions takes about as long as
// linting an eighth of it eight times. Every fix of such a document shares
// the blockquote, the table or the list with the other fixes, so each fix is
// checked on its own, and a check that parsed the whole blockquote or table
// made eight times the document take about eight times as long: 2,000 items
// in a blockquote took 15 seconds to lint, and 8,000 took 200. A fix that
// deletes a definition made the check parse the whole document.
//
// A bound in seconds fails on a busy machine, so the test compares two
// measurements of the same amount of input, as
// TestManyClosingBracketsParseInLinearTime does: one lint of the long
// document against a batch of lints of a document an eighth as long. Both
// last about as long, so they meet the same disturbances; they run in turns,
// the fastest of each counts, and garbage collection is paused while they
// are timed. Like TestLintTimeGrowsLinearly, it is skipped under the race
// detector, with which CI runs the tests. There TestRegionParsesGrowInBlocks
// and TestDefinitionChecksGrowInBlocks, which count the bytes the checks
// parse instead of timing them, guard the same documents, and
// TestRegionParsesGrowInBlocks a table in a blockquote too.
func TestLintTimeGrowsLinearlyInBlocks(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	requireLintTimeLinear(t, 100, []namedDocument{
		{name: "list in a blockquote", generate: quoteListDocument},
		{name: "paragraphs in a blockquote", generate: quoteParagraphsDocument},
		{name: "table", generate: tableRowsDocument},
		{name: "definitions in a list", generate: definitionListDocument},
		{name: "definitions in a blockquote", generate: definitionQuoteDocument},
	})
}

// TestLintTimeGrowsLinearlyInListItems checks, as
// TestLintTimeGrowsLinearlyInBlocks does, that linting a document whose
// fixes all sit in one list item takes about as long as linting an eighth of
// it eight times: tables right after text, in the item or in a loose list
// nested in it, which MD058 fixes, and fences and headings right next to
// text, which MD031 and MD022 fix. A check that parsed the whole item for
// each fix took 4.4 seconds for 400 tables in the item, 5 seconds with the
// tables in the nested list, and 8 and 11 seconds for 400 fences and
// headings. There TestRegionParsesGrowInListItems guards the same documents
// by the bytes the checks parse.
func TestLintTimeGrowsLinearlyInListItems(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	requireLintTimeLinear(t, 50, []namedDocument{
		{name: "tables in a list item", generate: itemTablesDocument},
		{name: "tables in a nested list", generate: nestedTablesDocument},
		{name: "fences in a list item", generate: itemFencesDocument},
		{name: "headings in a list item", generate: itemHeadingsDocument},
	})
}

// namedDocument is a generated test document of a given size.
type namedDocument struct {
	name     string
	generate func(int) []byte
}

// requireLintTimeLinear checks for each document that one lint of it at
// eight times small takes less than three times as long as eight lints of it
// at small, each of which keeps a fix for at least small findings (see
// TestLintTimeGrowsLinearlyInBlocks).
func requireLintTimeLinear(t *testing.T, small int, docs []namedDocument) {
	t.Helper()
	const (
		factor = 8
		rounds = 3
		// A linear check gives a ratio near one, and a check that parses the
		// whole block for each fix a ratio near eight; the bound leaves room
		// for a busy machine.
		maxRatio = 3
		// The paused collector runs past this heap, so that a check that
		// parses the whole block for each fix fails without taking the
		// machine's memory.
		memoryLimit = 1 << 30
	)
	registry := lint.NewRegistry()
	RegisterAll(registry)
	engine := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
	cfg := config.NewConfig()
	// lintOnce lints content and returns how long it took and how many of
	// its findings keep a fix.
	lintOnce := func(t *testing.T, content []byte) (time.Duration, int) {
		t.Helper()
		start := time.Now()
		result, err := engine.LintFile(context.Background(), "doc.md", content, cfg)
		elapsed := time.Since(start)
		require.NoError(t, err)
		require.Empty(t, result.RuleErrors)
		fixable := 0
		for _, diag := range result.Diagnostics {
			if diag.HasFix() {
				fixable++
			}
		}
		return elapsed, fixable
	}
	for _, doc := range docs {
		t.Run(doc.name, func(t *testing.T) {
			short, long := doc.generate(small), doc.generate(factor*small)
			runtime.GC()
			defer debug.SetGCPercent(debug.SetGCPercent(-1))
			defer debug.SetMemoryLimit(debug.SetMemoryLimit(memoryLimit))

			// Warm up both documents before timing them. Every line of the
			// block, or every definition no link uses, keeps its fix, so
			// every fix check runs and passes.
			_, fixable := lintOnce(t, short)
			require.GreaterOrEqual(t, fixable, small)
			_, fixable = lintOnce(t, long)
			require.GreaterOrEqual(t, fixable, factor*small)

			// Collecting before each measurement, outside the timers, keeps
			// the paused collector from letting the heap grow for the whole
			// test.
			bestBatch, bestLong := time.Duration(1<<63-1), time.Duration(1<<63-1)
			for range rounds {
				runtime.GC()
				var batch time.Duration
				for range factor {
					elapsed, _ := lintOnce(t, short)
					batch += elapsed
				}
				bestBatch = min(bestBatch, batch)
				runtime.GC()
				elapsed, _ := lintOnce(t, long)
				bestLong = min(bestLong, elapsed)
			}
			ratio := float64(bestLong) / float64(bestBatch)
			t.Logf("%d lints of %d in %v, one lint of %d in %v, ratio %.2f",
				factor, small, bestBatch, factor*small, bestLong, ratio)
			require.Less(t, ratio, float64(maxRatio), "lint time grows faster than the document")
		})
	}
}

// TestOutlineAllocationFollowsTheText checks that writing the outline of a
// document allocates a few bytes for each byte of the document, not a buffer
// for each of its text nodes. Every rule that checks its fixes by the
// outline writes the outline of the whole document at least twice, and
// rendering each text node through a buffered writer of its own allocated
// 4 KB for each node, which garbage collection then had to clear.
func TestOutlineAllocationFollowsTheText(t *testing.T) {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range 500 {
		fmt.Fprintf(&doc, "Text %d with *emphasis*, `code` and a [link](https://example.com/%d).\n\n", idx, idx)
	}
	snapshot := parseForCheck(t, doc.String())
	// TotalAlloc counts every allocation, collected or not.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	outline := documentOutline(snapshot, fixAllowance{})
	runtime.ReadMemStats(&after)
	require.Contains(t, outline, "emphasis")
	perByte := float64(after.TotalAlloc-before.TotalAlloc) / float64(len(snapshot.Content))
	t.Logf("%d bytes, %.1f bytes allocated for each", len(snapshot.Content), perByte)
	// A paragraph holds five text nodes in about 70 bytes, so a buffer of
	// 4 KB for each node makes about 300 bytes for each byte.
	require.Less(t, perByte, 40.0, "the outline allocates more than its text needs")
}
