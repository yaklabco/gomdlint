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
