package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// These tests cover rules whose behaviour depends on the node span contract:
// full spans include block markers (setext underlines, code fences, list
// markers) and every block has a position.

func applyRuleWithOptions(t *testing.T, rule lint.Rule, input string, options map[string]any) []lint.Diagnostic {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	ruleCfg := &config.RuleConfig{Options: options}
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, ruleCfg))
	require.NoError(t, err)
	return diags
}

func applyAllFixEdits(t *testing.T, input string, diags []lint.Diagnostic) string {
	t.Helper()
	var edits []fix.TextEdit
	for _, diag := range diags {
		edits = append(edits, diag.FixEdits...)
	}
	prepared, err := fix.PrepareEdits(edits, len(input))
	require.NoError(t, err)
	return string(fix.ApplyEdits([]byte(input), prepared))
}

func TestSetextHeadingBlankLinesFixKeepsUnderline(t *testing.T) {
	input := "# Title\n\nSetext heading\n--------------\nParagraph.\n"
	diags := applyCompatibilityRule(t, NewHeadingBlankLinesRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, 3, diags[0].StartLine)
	require.Equal(t, "# Title\n\nSetext heading\n--------------\n\nParagraph.\n", applyAllFixEdits(t, input, diags))
}

func TestHeadingAfterSetextHeadingIsAdjacentHeading(t *testing.T) {
	input := "Setext heading\n==============\n## Next\n\nText.\n"
	diags := applyCompatibilityRule(t, NewHeadingBlankLinesRule(), input)
	for _, diag := range diags {
		require.NotContains(t, diag.Message, "above", "the underline ends the previous heading")
	}
}

func TestHeadingStyleDetectsSetext(t *testing.T) {
	input := "# ATX heading\n\nSetext heading\n==============\n"
	diags := applyRuleWithOptions(t, NewHeadingStyleRule(), input, map[string]any{"style": "atx"})
	require.Len(t, diags, 1)
	require.Equal(t, 3, diags[0].StartLine)
	require.Contains(t, diags[0].Message, "setext")
}

func TestCodeBlockLanguageReportsAtOpeningFence(t *testing.T) {
	input := "# Title\n\n- item\n\n  ```\n  echo hi\n  ```\n\n```\n```\n"
	diags := applyCompatibilityRule(t, NewCodeBlockLanguageRule(), input)
	require.Len(t, diags, 2)
	require.Equal(t, 5, diags[0].StartLine)
	require.Equal(t, 3, diags[0].StartColumn)
	require.Equal(t, 9, diags[1].StartLine, "an empty fenced code block has a position")
	require.Equal(t, 1, diags[1].StartColumn)
}

func TestCodeFenceStyleFixRewritesBothFences(t *testing.T) {
	input := "# Title\n\n```\na\n```\n\n~~~\nb\n~~~\n"
	diags := applyCompatibilityRule(t, NewCodeFenceStyleRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, 7, diags[0].StartLine)
	require.Equal(t, "# Title\n\n```\na\n```\n\n```\nb\n```\n", applyAllFixEdits(t, input, diags))
}

func TestFirstLineHeadingSeesLeadingThematicBreak(t *testing.T) {
	input := "***\n\n# Title\n"
	diags := applyCompatibilityRule(t, NewFirstLineHeadingRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, 1, diags[0].StartLine)
}

func TestByteOrderMarkBeforeFrontMatterAndHeading(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	input := bom + "---\ntitle: Example\n---\n# Document\n\nText.\n"
	require.Empty(t, applyCompatibilityRule(t, NewHeadingBlankLinesRule(), input))
	require.Empty(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), bom+"# Document\n\nText.\n"))
}

func TestOrderedListFixKeepsParenthesisDelimiter(t *testing.T) {
	input := "# Title\n\n1) one\n1) two\n3) three\n"
	diags := applyCompatibilityRule(t, NewOrderedListIncrementRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, "Use 2) instead", diags[0].Suggestion)
	require.Equal(t, "# Title\n\n1) one\n2) two\n3) three\n", applyAllFixEdits(t, input, diags))
}

func TestTrailingWhitespaceIgnoreCodeBlocksChecksFences(t *testing.T) {
	input := "# Title\n\n```sh  \necho hi  \n```  \n"
	diags := applyRuleWithOptions(t, NewTrailingWhitespaceRule(), input, map[string]any{"ignore_code_blocks": true})
	lines := make([]int, 0, len(diags))
	for _, diag := range diags {
		lines = append(lines, diag.StartLine)
	}
	require.Equal(t, []int{3, 5}, lines, "fence lines are checked; code content is ignored")
}
