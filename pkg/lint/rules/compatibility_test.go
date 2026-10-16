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

func applyCompatibilityRule(t *testing.T, rule lint.Rule, input string) []lint.Diagnostic {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
	require.NoError(t, err)
	return diags
}

func TestEmptyLinkCodeLabel(t *testing.T) {
	for _, input := range []string{
		"[`docs/runbook.md`](docs/runbook.md)",
		"[**bold** and `code`](target)",
		"[![diagram](diagram.png)](target)",
		"[label\n](target)",
	} {
		t.Run(input, func(t *testing.T) {
			require.Empty(t, applyCompatibilityRule(t, NewEmptyLinkRule(), input))
		})
	}
	require.Len(t, applyCompatibilityRule(t, NewEmptyLinkRule(), "[`label`]()"), 1)
}

func TestEmphasisIndependentSpans(t *testing.T) {
	for _, input := range []string{
		"**GOROOT** and **GOBIN**",
		"*one* and *two*",
		"__one__ and __two__",
		"`** spaced **`",
	} {
		t.Run(input, func(t *testing.T) {
			require.Empty(t, applyCompatibilityRule(t, NewNoSpaceInEmphasisRule(), input))
		})
	}
	require.Len(t, applyCompatibilityRule(t, NewNoSpaceInEmphasisRule(), "** spaced **"), 1)
}

func TestFrontmatterIsNotMarkdown(t *testing.T) {
	input := "---\ntitle: Example\n  # Metadata\n#Broken\n---\n\n# Document\n\nText.\n"
	for _, rule := range []lint.Rule{
		NewHeadingBlankLinesRule(), NewHeadingStartLeftRule(), NewNoMissingSpaceATXRule(),
	} {
		t.Run(rule.ID(), func(t *testing.T) {
			require.Empty(t, applyCompatibilityRule(t, rule, input))
		})
	}
	t.Run("MD025", func(t *testing.T) {
		// The metadata title counts as the first H1 (markdownlint front_matter_title),
		// so only the real H1 is reported; the metadata lines are not headings.
		diags := applyCompatibilityRule(t, NewSingleH1Rule(), input)
		require.Len(t, diags, 1)
		require.Equal(t, 7, diags[0].StartLine)
	})
}

func TestCommentsAreNotHTMLViolations(t *testing.T) {
	require.Empty(t, applyCompatibilityRule(t, NewInlineHTMLRule(), "<!-- markdownlint-disable MD025 -->\n"))
	require.Empty(t, applyCompatibilityRule(t, NewInlineHTMLRule(), "text <!-- ordinary comment --> text\n"))
	require.Len(t, applyCompatibilityRule(t, NewInlineHTMLRule(), "<div>text</div>\n"), 1)
}

func TestHeadingAfterLeadingComment(t *testing.T) {
	input := "<!-- markdownlint-disable MD025 -->\n# Document\n\nText.\n"
	require.Empty(t, applyCompatibilityRule(t, NewHeadingBlankLinesRule(), input))
	require.Empty(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), input))
}

func TestFirstHeadingFrontmatterTitle(t *testing.T) {
	input := "---\ntitle: Document\n---\n\nText without a heading.\n"
	require.Empty(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), input))
	require.Empty(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), "+++\ntitle = 'Document'\n+++\n\nText.\n"))
	require.Empty(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), "{\n\"title\": \"Document\"\n}\n\nText.\n"))
	require.Len(t, applyCompatibilityRule(t, NewFirstLineHeadingRule(), "#Heading\n\n[]()\n"), 1)
	input = "<!-- comment -->\n\nText without a heading.\n"
	diags := applyCompatibilityRule(t, NewFirstLineHeadingRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, 3, diags[0].StartLine)
}

func TestListIndentedContinuation(t *testing.T) {
	input := "# Document\n\n- first\n- wrapped item\n  continues here\n  and here.\n\nAfter.\n"
	require.Empty(t, applyCompatibilityRule(t, NewBlanksAroundListsRule(), input))
	require.Len(t, applyCompatibilityRule(t, NewBlanksAroundListsRule(), "- item\n# Heading\n"), 1)
}

func TestListLazyContinuationIsContent(t *testing.T) {
	require.Empty(t, applyCompatibilityRule(t, NewBlanksAroundListsRule(), "- item\nwrapped paragraph\n\nAfter.\n"))
	require.Len(t, applyCompatibilityRule(t, NewBlanksAroundListsRule(), "- item\n# Heading\n"), 1)
}

func TestTableEscapedPipes(t *testing.T) {
	for _, value := range []string{`a\|b`, "`a\\|b`", `a\\\|b`} {
		input := "| Name | Value |\n| --- | --- |\n| key | " + value + " |\n"
		require.Empty(t, applyCompatibilityRule(t, NewTableColumnCountRule(), input))
	}
	require.Len(t, applyCompatibilityRule(t, NewTableColumnCountRule(), "| A | B |\n| --- | --- |\n| a | b | extra |\n"), 1)
}

func TestEmphasisInListIsNotHeading(t *testing.T) {
	input := "# Document\n\n- **Phase one**\n  - details\n\n- **Phase two**\n  - details\n"
	require.Empty(t, applyCompatibilityRule(t, NewNoEmphasisAsHeadingRule(), input))
	require.Len(t, applyCompatibilityRule(t, NewNoEmphasisAsHeadingRule(), "**Section**\n"), 1)
}

func TestReversedLinkInsideCode(t *testing.T) {
	require.Empty(t, applyCompatibilityRule(t, NewReversedLinkRule(), "Use `SHA256(name)[:8]` here.\n"))
	require.Len(t, applyCompatibilityRule(t, NewReversedLinkRule(), "(label)[target]\n"), 1)
}

func TestEmphasisDoesNotCrossTableCells(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| * | * |\n"
	require.Empty(t, applyCompatibilityRule(t, NewNoSpaceInEmphasisRule(), input))
	require.Len(t, applyCompatibilityRule(t, NewNoSpaceInEmphasisRule(), "| A | B |\n| --- | --- |\n| ** spaced ** | text |\n"), 1)
}

func TestBareURLReferenceDefinitions(t *testing.T) {
	require.Empty(t, applyCompatibilityRule(t, NewNoBareURLsRule(), "[release]: https://example.test/release\n"))
	require.Len(t, applyCompatibilityRule(t, NewNoBareURLsRule(), "Visit https://example.test/release\n"), 1)
}

func TestHashInWrappedListText(t *testing.T) {
	input := "- Changes for #123 and\n  #456 are ready.\n"
	require.Empty(t, applyCompatibilityRule(t, NewNoMissingSpaceATXRule(), input))
	require.Len(t, applyCompatibilityRule(t, NewNoMissingSpaceATXRule(), "#Heading\n"), 1)
	require.Len(t, applyCompatibilityRule(t, NewNoMissingSpaceATXRule(), "- item\n\n  #Heading\n"), 1)
}

func TestTableAdjacentDirectiveComments(t *testing.T) {
	input := "# Document\n\n<!-- markdownlint-disable MD013 -->\n| A | B |\n| --- | --- |\n| a | b |\n<!-- markdownlint-enable MD013 -->\n\nAfter.\n"
	require.Empty(t, applyCompatibilityRule(t, NewTableBlankLinesRule(), input))
	require.Len(t, applyCompatibilityRule(t, NewTableBlankLinesRule(), "Text.\n| A | B |\n| --- | --- |\n| a | b |\n"), 1)
}

func TestTableRulesIgnoreFencedExamples(t *testing.T) {
	input := "```markdown\n| A | B |\n| - | - |\n| a | b | extra |\n```\n"
	for _, rule := range []lint.Rule{NewTablePipeStyleRule(), NewTableColumnCountRule(), NewTableAlignmentRule(), NewTableBlankLinesRule(), NewTableColumnStyleRule()} {
		t.Run(rule.ID(), func(t *testing.T) {
			require.Empty(t, applyCompatibilityRule(t, rule, input))
		})
	}
}

func TestNestedBlockquoteListIndent(t *testing.T) {
	require.Empty(t, applyCompatibilityRule(t, NewNoMultipleSpaceBlockquoteRule(), "> - parent\n>   - child\n"))
	require.Len(t, applyCompatibilityRule(t, NewNoMultipleSpaceBlockquoteRule(), ">  text\n"), 1)
}

func TestBlankLineFixPreservesMetadata(t *testing.T) {
	input := "---\ntitle: Document\n---\n\n# Document\n\nText.\n"
	require.Empty(t, applyCompatibilityRule(t, NewMultipleBlankLinesRule(), input))
}

func TestListBlankLineFixFollowsContinuation(t *testing.T) {
	input := "- item\n  continuation\n# Heading\n"
	diags := applyCompatibilityRule(t, NewBlanksAroundListsRule(), input)
	require.Len(t, diags, 1)
	require.Equal(t, "- item\n  continuation\n\n# Heading\n", string(fix.ApplyEdits([]byte(input), diags[0].FixEdits)))
}

func TestAdjacentListFixInsertsOneBlank(t *testing.T) {
	input := "- first\n1. second\n"
	diags := applyCompatibilityRule(t, NewBlanksAroundListsRule(), input)
	var edits []fix.TextEdit
	for _, diag := range diags {
		edits = append(edits, diag.FixEdits...)
	}
	prepared, err := fix.PrepareEdits(edits, len(input))
	require.NoError(t, err)
	require.Equal(t, "- first\n\n1. second\n", string(fix.ApplyEdits([]byte(input), prepared)))
}
