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
// blockquote, a blockquote of many paragraphs and a long table takes about
// as long as linting an eighth of it eight times. Every fix of such a
// document shares the blockquote or the table with the other fixes, so each
// fix is checked on its own, and a check that parsed the whole blockquote or
// table made eight times the document take about eight times as long: 2,000
// items in a blockquote took 15 seconds to lint, and 8,000 took 200.
//
// A bound in seconds fails on a busy machine, so the test compares two
// measurements of the same amount of input, as
// TestManyClosingBracketsParseInLinearTime does: one lint of the long
// document against a batch of lints of a document an eighth as long. Both
// last about as long, so they meet the same disturbances; they run in turns,
// the fastest of each counts, and garbage collection is paused while they
// are timed. Like TestLintTimeGrowsLinearly, it is skipped under the race
// detector, with which CI runs the tests; TestRegionParsesGrowInBlocks,
// which counts the bytes the region checks parse instead of timing them,
// guards the same documents there, and a table in a blockquote too.
func TestLintTimeGrowsLinearlyInBlocks(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	const (
		small  = 100
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
	for _, doc := range []struct {
		name     string
		generate func(int) []byte
	}{
		{name: "list in a blockquote", generate: quoteListDocument},
		{name: "paragraphs in a blockquote", generate: quoteParagraphsDocument},
		{name: "table", generate: tableRowsDocument},
	} {
		t.Run(doc.name, func(t *testing.T) {
			short, long := doc.generate(small), doc.generate(factor*small)
			runtime.GC()
			defer debug.SetGCPercent(debug.SetGCPercent(-1))
			defer debug.SetMemoryLimit(debug.SetMemoryLimit(memoryLimit))

			// Warm up both documents before timing them. Every line of the
			// block keeps its fix, so every fix check runs and passes.
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
