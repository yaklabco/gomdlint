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
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// largeDocument returns a document of sections, each with a heading, a
// paragraph, lists, a quote, a code block, a table and a reference
// definition, and with a few violations of the heading, emphasis and proper
// name rules.
func largeDocument(sections int) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range sections {
		name := "GitHub"
		if idx%100 == 0 {
			name = "github _here_ and __there__"
		}
		fmt.Fprintf(&doc, "## Section %d\n\n", idx)
		fmt.Fprintf(&doc, "Some paragraph text with *emphasis*, **strong**, `code` and %s.\n\n", name)
		doc.WriteString("- item one\n- item two\n  - nested item\n- item three\n\n")
		doc.WriteString("1. first\n2. second\n\n")
		fmt.Fprintf(&doc, "> A quote with a [ref][r%d].\n\n", idx)
		fmt.Fprintf(&doc, "```bash\necho %d\n```\n\n", idx)
		doc.WriteString("| a | b |\n| - | - |\n| 1 | 2 |\n\n")
		fmt.Fprintf(&doc, "[r%d]: https://example.com/ref/%d\n\n", idx, idx)
		if idx%100 == 0 {
			fmt.Fprintf(&doc, "#Missing space %d\n\n### Trailing %d.\nText right below.\n\n", idx, idx)
		}
	}
	return doc.String()
}

// TestRulesOnLargeDocument verifies that the heading and emphasis rules run
// in time that grows about linearly with the document: on a document with
// thousands of headings and list items, a rule that scans every list item
// for every line would take minutes, far past the generous limit.
func TestRulesOnLargeDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("large document")
	}
	const limit = 30 * time.Second
	// The race detector slows every rule down several times, so it gets a
	// smaller document; a rule that is quadratic still runs far past the
	// limit on either.
	sections := 2000
	if raceEnabled {
		sections = 1000
	}
	input := []byte(largeDocument(sections))
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "large.md", input)
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM

	rules := []struct {
		rule    lint.Rule
		options map[string]any
	}{
		{rule: NewHeadingIncrementRule()},
		{rule: NewHeadingStyleRule()},
		{rule: NewNoMissingSpaceATXRule()},
		{rule: NewNoMultipleSpaceATXRule()},
		{rule: NewNoMissingSpaceClosedATXRule()},
		{rule: NewNoMultipleSpaceClosedATXRule()},
		{rule: NewHeadingBlankLinesRule()},
		{rule: NewHeadingStartLeftRule()},
		{rule: NewNoDuplicateHeadingRule()},
		{rule: NewSingleH1Rule()},
		{rule: NewNoTrailingPunctuationRule()},
		{rule: NewHRStyleRule()},
		{rule: NewNoEmphasisAsHeadingRule()},
		{rule: NewNoSpaceInEmphasisRule()},
		{rule: NewFirstLineHeadingRule()},
		{rule: NewRequiredHeadingsRule(), options: map[string]any{"headings": []any{"# Title", "*"}}},
		{rule: NewProperNamesRule(), options: map[string]any{"names": []any{"GitHub"}}},
		{rule: NewEmphasisStyleRule()},
		{rule: NewStrongStyleRule()},
	}
	for _, tt := range rules {
		var ruleCfg *config.RuleConfig
		if tt.options != nil {
			ruleCfg = &config.RuleConfig{Options: tt.options}
		}
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			_, err := tt.rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, ruleCfg))
			done <- err
		}()
		select {
		case err := <-done:
			require.NoError(t, err, tt.rule.ID())
			t.Logf("%s: %v", tt.rule.ID(), time.Since(start))
		case <-time.After(limit):
			t.Fatalf("%s took longer than %v on a %d byte document", tt.rule.ID(), limit, len(input))
		}
	}
}

// largeHTMLDocument returns a document of sections that each hold inline
// HTML in a paragraph, in a table cell and in a blockquote, where one tag
// spans two lines, and an HTML block in a blockquote that spans four lines.
func largeHTMLDocument(sections int) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range sections {
		fmt.Fprintf(&doc, "Press <kbd>Ctrl</kbd>+<kbd>C</kbd> to copy %d.<br> Version<sup>%d</sup>.\n\n", idx, idx)
		fmt.Fprintf(&doc, "| a | b |\n| - | - |\n| x<br>y | %d |\n\n", idx)
		fmt.Fprintf(&doc, "> Quote %d has <span\n> title=\"item\">a tag</span> here.\n\n", idx)
		fmt.Fprintf(&doc, "> <div\n> class=\"note\">\n> text %d\n> </div>\n\n", idx)
	}
	return doc.String()
}

// largeDocumentLimit is how long a rule may take on a large document. A rule
// that grows about linearly with the document takes a small fraction of it,
// and one that does work proportional to the document for each element runs
// far past it.
const largeDocumentLimit = 30 * time.Second

// parseLargeDocument parses input as GFM and returns it with a matching
// configuration.
func parseLargeDocument(t *testing.T, input string) (*mdast.FileSnapshot, *config.Config) {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "large.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	return file, cfg
}

// applyWithinLimit runs rule with its default options on file and fails the
// test when it takes longer than largeDocumentLimit. A rule that runs past
// the limit keeps running in the background until the test binary exits.
func applyWithinLimit(t *testing.T, rule lint.Rule, file *mdast.FileSnapshot, cfg *config.Config) []lint.Diagnostic {
	t.Helper()
	type outcome struct {
		diags []lint.Diagnostic
		err   error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		diags, err := rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
		done <- outcome{diags, err}
	}()
	select {
	case got := <-done:
		require.NoError(t, got.err, rule.ID())
		t.Logf("%s: %v", rule.ID(), time.Since(start))
		return got.diags
	case <-time.After(largeDocumentLimit):
		t.Fatalf("%s took longer than %v on a %d byte document", rule.ID(), largeDocumentLimit, len(file.Content))
		return nil
	}
}

// TestInlineHTMLRuleOnLargeDocument verifies that MD033 runs in time that
// grows about linearly with the document. Looking up the tables or copying
// the file for each HTML element would take minutes on a document with tens
// of thousands of elements, far past the generous limit.
func TestInlineHTMLRuleOnLargeDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("large document")
	}
	// The race detector slows the rule down several times, so it gets a
	// smaller document; a quadratic rule still runs far past the limit.
	sections := 8000
	if raceEnabled {
		sections = 4000
	}
	file, cfg := parseLargeDocument(t, largeHTMLDocument(sections))

	diags := applyWithinLimit(t, NewInlineHTMLRule(), file, cfg)

	// kbd twice, br, sup, br in the table, span, and div per section.
	require.Len(t, diags, 7*sections)
}

// largeQuotedImageDocument returns a document of blockquotes that each hold
// a paragraph with four img elements spanning two lines, two of them
// without alt text.
func largeQuotedImageDocument(sections int) string {
	var doc strings.Builder
	doc.WriteString("# Title\n\n")
	for idx := range sections {
		fmt.Fprintf(&doc, "> Item %d <img\n> src=\"1.png\" alt=\"one\"> and <img\n> src=\"2.png\"> and <img\n"+
			"> src=\"3.png\" alt=\"three\"> and <img\n> src=\"4.png\"> end.\n\n", idx)
	}
	return doc.String()
}

// TestImageAltTextRuleOnLargeDocument verifies that MD045 runs in time that
// grows about linearly with the document. Copying the file for each img
// element that spans lines in a blockquote would take minutes on a document
// with tens of thousands of them, far past the generous limit.
func TestImageAltTextRuleOnLargeDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("large document")
	}
	// Copying memory costs about as much with the race detector as without
	// it, so both use the same document.
	const sections = 15000
	file, cfg := parseLargeDocument(t, largeQuotedImageDocument(sections))

	diags := applyWithinLimit(t, NewImageAltTextRule(), file, cfg)

	require.Len(t, diags, 2*sections)
}
