package rules

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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
	// Adjacent headings are only exempt in the relaxed (non-markdownlint) mode.
	diags := applyRuleWithOptions(t, NewHeadingBlankLinesRule(), input, map[string]any{"markdownlint_compatible": false})
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

func TestCodeFenceStyleFixReplacesWholeClosingRun(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty block with a longer closer",
			input: "# Title\n\n~~~\n~~~~~\n\nText\n",
			want:  "# Title\n\n```\n`````\n\nText\n",
		},
		{
			name:  "longer opener and closer with info",
			input: "# Title\n\n~~~~~ js\nx\n~~~~~~~~\n\nText\n",
			want:  "# Title\n\n````` js\nx\n````````\n\nText\n",
		},
		{
			name:  "quoted block",
			input: "# Title\n\n> ~~~\n> x\n> ~~~~\n\nText\n",
			want:  "# Title\n\n> ```\n> x\n> ````\n\nText\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewCodeFenceStyleRule(), tt.input, map[string]any{"style": "backtick"})
			require.Len(t, diags, 1)
			fixed := applyAllFixEdits(t, tt.input, diags)
			require.Equal(t, tt.want, fixed)
			require.Empty(t, applyRuleWithOptions(t, NewCodeFenceStyleRule(), fixed, map[string]any{"style": "backtick"}))
		})
	}
}

func TestCodeFenceStyleWithholdsUnsafeFixes(t *testing.T) {
	for name, input := range map[string]string{
		"backtick in info string":           "# Title\n\n~~~foo`bar\n~~~\n\nText\n",
		"backtick fence inside tilde fence": "# Title\n\n~~~\n```\n~~~\n\nText\n",
		"quoted backtick fence inside":      "# Title\n\n> ~~~\n>   ````\n> ~~~\n\nText\n",
	} {
		t.Run(name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewCodeFenceStyleRule(), input, map[string]any{"style": "backtick"})
			require.Len(t, diags, 1, "the style mismatch is still reported")
			require.False(t, diags[0].HasFix(), "converting would break the document")
		})
	}
}

func TestCommandsShowOutputReadsTheContentSpan(t *testing.T) {
	tests := []struct {
		name  string
		input string
		line  int
		want  string
	}{
		{
			name:  "fence beside a list marker",
			input: "# Title\n\n- ```sh\n  $ ls\n  ```\n",
			line:  4,
			want:  "# Title\n\n- ```sh\n  ls\n  ```\n",
		},
		{
			name:  "top-level fence",
			input: "# Title\n\n```sh\n$ ls\n$ pwd\n```\n",
			line:  4,
			want:  "# Title\n\n```sh\nls\npwd\n```\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyCompatibilityRule(t, NewCommandsShowOutputRule(), tt.input)
			require.Len(t, diags, 1)
			require.Equal(t, tt.line, diags[0].StartLine)
			require.Equal(t, tt.want, applyAllFixEdits(t, tt.input, diags))
		})
	}
	require.Empty(t, applyCompatibilityRule(t, NewCommandsShowOutputRule(), "# Title\n\n```sh\n```\n"), "an empty block has no commands")
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

func TestMultilineSetextHeadingFragment(t *testing.T) {
	// GitHub and markdownlint join the lines of a heading without a
	// separator when they build its fragment.
	input := "# Title\n\nFirst\nsecond line\n-----------\n\n[link](#firstsecond-line)\n"
	require.Empty(t, applyCompatibilityRule(t, NewLinkFragmentsRule(), input))
}

func TestEmphasisAsHeadingKeepsWordsAcrossLines(t *testing.T) {
	// Like markdownlint, MD036 does not report multi-line emphasis, so the
	// paragraph is never rewritten and no words are lost.
	input := "# Title\n\n**Bold\ntext**\n\nMore.\n"
	diags := applyCompatibilityRule(t, NewNoEmphasisAsHeadingRule(), input)
	require.Empty(t, diags)
	require.Equal(t, input, applyAllFixEdits(t, input, diags))
}

func TestStrikethroughIsNotEmphasisAsHeading(t *testing.T) {
	input := "# Title\n\n~~Struck paragraph~~\n\nMore.\n"
	require.Empty(t, applyCompatibilityRule(t, NewNoEmphasisAsHeadingRule(), input))
}

func TestMultilineCollapsedReferenceUsesDefinition(t *testing.T) {
	input := "# Title\n\nSee [multi\nline][].\n\n[multi line]: https://example.com\n"
	require.Empty(t, applyCompatibilityRule(t, NewLinkImageRefDefsRule(), input))
}

func TestReferenceLabelsUseTheirSourceText(t *testing.T) {
	// A collapsed or shortcut reference is matched by its raw label, not by
	// the rendered text of the link.
	input := "# Title\n\nSee [**foo**][], [bar baz][] and [*qux*].\n\n" +
		"[**foo**]: https://example.com/foo\n[bar baz]: https://example.com/bar\n[*qux*]: https://example.com/qux\n"
	require.Empty(t, applyCompatibilityRule(t, NewReferenceLinkImagesRule(), input))
	require.Empty(t, applyCompatibilityRule(t, NewLinkImageRefDefsRule(), input))
}

func TestQuotedMultilineReferencesUseTheirDefinitions(t *testing.T) {
	for _, input := range []string{
		"# Title\n\n> See [a\n> b][].\n\n[a b]: https://example.com\n",
		"# Title\n\n> See [text][a\n> b].\n\n[a b]: https://example.com\n",
	} {
		t.Run(input, func(t *testing.T) {
			require.Empty(t, applyCompatibilityRule(t, NewReferenceLinkImagesRule(), input))
			require.Empty(t, applyCompatibilityRule(t, NewLinkImageRefDefsRule(), input))
		})
	}
}

func TestOrderedListFixKeepsParenthesisDelimiter(t *testing.T) {
	input := "# Title\n\n1) one\n2) two\n4) three\n"
	diags := applyCompatibilityRule(t, NewOrderedListIncrementRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, "Use 3) instead", diags[0].Suggestion)
	require.Equal(t, "# Title\n\n1) one\n2) two\n3) three\n", applyAllFixEdits(t, input, diags))
}

func TestBlanksBetweenBlockquotesAroundQuotedCode(t *testing.T) {
	// The fence lines of quoted code are blockquote lines; only the code
	// itself is skipped.
	input := "# Title\n\n> Some quoted text.\n\n> ```bash\n> echo hi\n> ```\n\n> More quoted text.\n"
	diags := applyCompatibilityRule(t, NewNoBlanksBlockquoteRule(), input)
	lines := make([]int, 0, len(diags))
	for _, diag := range diags {
		lines = append(lines, diag.StartLine)
	}
	require.Equal(t, []int{4, 8}, lines)
}

func TestTrailingWhitespaceIgnoreCodeBlocksChecksFences(t *testing.T) {
	input := "# Title\n\n```sh   \necho hi   \n```   \n"
	diags := applyRuleWithOptions(t, NewTrailingWhitespaceRule(), input, map[string]any{"ignore_code_blocks": true})
	lines := make([]int, 0, len(diags))
	for _, diag := range diags {
		lines = append(lines, diag.StartLine)
	}
	require.Equal(t, []int{3, 5}, lines, "fence lines are checked; code content is ignored")
}

// TestRulesIgnoreByteOrderMark checks that a byte order mark before the
// first line changes neither the diagnostics nor the fixes of rules that
// read a construct from the start of its line. goldmark parses the content
// after the mark, so these line scanners must skip it too.
func TestRulesIgnoreByteOrderMark(t *testing.T) {
	closingATX := map[string]any{"style": "atx", "require_closing_atx": true}
	tests := []struct {
		name    string
		rule    lint.Rule
		options map[string]any
		input   string
		count   int
	}{
		{"MD052 definition on the first line", NewReferenceLinkImagesRule(), nil, "[foo]: /u\n\n[x][foo]\n", 0},
		{"MD053 unused definition on the first line", NewLinkImageRefDefsRule(), nil, "[foo]: /u\n\nText\n", 1},
		{"MD003 closed heading on the first line", NewHeadingStyleRule(), closingATX, "# Title #\n\n## Sub\n", 1},
		{"MD003 open heading on the first line", NewHeadingStyleRule(), closingATX, "# Title\n\n## Sub ##\n", 1},
		{"MD029 ordered list on the first line", NewOrderedListIncrementRule(), nil, "1. one\n3. two\n", 1},
		{"MD029 misnumbered item on the first line", NewOrderedListIncrementRule(), map[string]any{"style": "one"}, "2. one\n1. two\n", 1},
		{"MD007 list on the first line", NewULIndentRule(), map[string]any{"start_indented": true}, "- a\n  - b\n", 2},
		{"MD005 indented list on the first line", NewListIndentRule(), nil, "  - a\n  - b\n", 0},
		{
			"MD060 table on the first line", NewTableColumnStyleRule(),
			map[string]any{"markdownlint_compatible": true, "style": "compact"},
			"| a | b |\n| - | - |\n| 1 | 2 |\n", 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := applyRuleWithOptions(t, tt.rule, tt.input, tt.options)
			require.Len(t, plain, tt.count, "without a byte order mark")
			marked := applyRuleWithOptions(t, tt.rule, mdast.ByteOrderMark+tt.input, tt.options)
			require.Equal(t, diagnosticLines(plain), diagnosticLines(marked))
			require.Equal(t, mdast.ByteOrderMark+applyAllFixEdits(t, tt.input, plain), applyAllFixEdits(t, mdast.ByteOrderMark+tt.input, marked))
		})
	}
}

// diagnosticLines describes diagnostics by line and message.
func diagnosticLines(diags []lint.Diagnostic) []string {
	described := make([]string, 0, len(diags))
	for _, diag := range diags {
		described = append(described, fmt.Sprintf("%d: %s", diag.StartLine, diag.Message))
	}
	return described
}
