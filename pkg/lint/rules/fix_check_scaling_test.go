package rules

import (
	"context"
	"fmt"
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
