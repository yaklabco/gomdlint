package lint_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	_ "github.com/yaklabco/gomdlint/pkg/lint/rules" // registers the rules in lint.DefaultRegistry
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// These tests fix documents with the product's rules. Each pair of rules
// proposes fixes that are safe for the document as it is but change its
// structure when applied together. The engine applies one rule's fixes to
// each top-level block per pass, so the second rule derives and checks its
// fix again on the content the first rule fixed.

// productRules returns the rules of lint.DefaultRegistry with the given IDs.
func productRules(t *testing.T, ids ...string) []lint.Rule {
	t.Helper()

	rules := make([]lint.Rule, 0, len(ids))
	for _, id := range ids {
		rule, ok := lint.DefaultRegistry.GetByID(id)
		require.True(t, ok, "rule %s is registered", id)
		rules = append(rules, rule)
	}
	return rules
}

// fixWithRules fixes content through the pipeline with only the given rules
// registered and enabled.
func fixWithRules(t *testing.T, content string, cfg *config.Config, rules ...lint.Rule) *lint.PipelineResult {
	t.Helper()

	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
		cfg.EnableRules = append(cfg.EnableRules, rule.ID())
	}
	cfg.Fix = true
	result, err := lint.NewPipeline(lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)).ProcessContent(
		context.Background(), "test.md", []byte(content), cfg, lint.PipelineOptions{Fix: true})
	require.NoError(t, err)
	require.Empty(t, result.RuleErrors)
	return result
}

func fixedContent(result *lint.PipelineResult, original string) string {
	if !result.Modified {
		return original
	}
	return string(result.ModifiedContent)
}

// outline describes the block structure of content: each top-level node by
// kind, with list items nested as their first line of text.
func outline(t *testing.T, content string) string {
	t.Helper()

	var parts []string
	for node := parseMarkdown(t, content).Root.FirstChild; node != nil; node = node.Next {
		parts = append(parts, outlineNode(node))
	}
	return strings.Join(parts, " | ")
}

func outlineNode(node *mdast.Node) string {
	switch node.Kind {
	case mdast.NodeList:
		var items []string
		for item := node.FirstChild; item != nil; item = item.Next {
			items = append(items, outlineNode(item))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case mdast.NodeListItem:
		var parts []string
		for child := node.FirstChild; child != nil; child = child.Next {
			if child.Kind == mdast.NodeList {
				parts = append(parts, outlineNode(child))
			} else {
				parts = append(parts, strings.TrimSpace(string(child.ContentText())))
			}
		}
		return strings.Join(parts, " ")
	case mdast.NodeCodeBlock:
		return "code(" + strings.TrimSpace(string(node.ContentText())) + ")"
	default:
		return node.Kind.String() + "(" + strings.TrimSpace(string(node.ContentText())) + ")"
	}
}

func TestPipeline_PromptAndFenceFixesKeepACodeBlockClosed(t *testing.T) {
	t.Parallel()

	// MD014 removes the "$ " prompts and MD048 turns the tilde fences into
	// backticks, as the first code block uses. Applied together, "$ ```"
	// becomes "```", which closes the block and turns "Done." into code.
	const content = "# Commands\n\n```sh\necho hi\n```\n\n~~~sh\n$ ```\n$ ls\n~~~\n\nDone.\n"
	rules := productRules(t, "MD014", "MD048")

	first := lintOnce(t, content, rules...)
	require.NotEmpty(t, first.Edits)
	assert.NotEmpty(t, first.DeferredEdits, "MD048's fix waits for MD014's in the same code block")
	for _, deferred := range first.DeferredEdits {
		assert.Equal(t, "MD048", deferred.RuleID)
	}

	result := fixWithRules(t, content, config.NewConfig(), rules...)
	fixed := fixedContent(result, content)

	assert.Equal(t, outline(t, "# Commands\n\n```sh\necho hi\n```\n\n~~~sh\n```\nls\n~~~\n\nDone.\n"), outline(t, fixed),
		"the code block keeps its lines and \"Done.\" stays a paragraph:\n%s", fixed)
	assert.NotContains(t, fixed, "$ ", "the prompts are removed")
	assert.False(t, result.Exhausted)
	assert.False(t, result.Oscillated)
}

func TestPipeline_IndentAndMarkerSpaceFixesKeepListNesting(t *testing.T) {
	t.Parallel()

	// MD007 with start_indented moves " *   three" right to two spaces,
	// and MD030 narrows "*   two" to "* two", whose content then starts at
	// column 3. Each fix keeps the list as it is, but together they make
	// "three" a child of "two".
	const content = "# Title\n\n*   one\n*   two\n    *   one\n *   three\n"
	cfg := config.NewConfig()
	cfg.Rules["MD007"] = config.RuleConfig{Options: map[string]any{"start_indented": true, "start_indent": 2}}

	result := fixWithRules(t, content, cfg, productRules(t, "MD007", "MD030")...)
	fixed := fixedContent(result, content)

	assert.Equal(t, outline(t, content), outline(t, fixed), "the list keeps its nesting:\n%s", fixed)
	assert.NotEqual(t, content, fixed, "the fixes that keep the nesting are applied")
	assert.False(t, result.Exhausted)
	assert.False(t, result.Oscillated)
}

func TestPipeline_IndentAndMarkerSpaceFixesKeepListsApartAcrossABlankLine(t *testing.T) {
	t.Parallel()

	// MD007 with start_indent 3 moves the bullet from two spaces to three,
	// and MD030 narrows "1.   two" to "1. two", whose content then starts
	// at column 4. Each fix keeps two lists, but together they make the
	// bullet a child of the ordered item: after a blank line, a line
	// indented as far as an item's content continues that item.
	const content = "# Title\n\n1.   two\n\n  * three\n"
	cfg := config.NewConfig()
	cfg.Rules["MD007"] = config.RuleConfig{Options: map[string]any{"start_indented": true, "start_indent": 3}}

	result := fixWithRules(t, content, cfg, productRules(t, "MD007", "MD030")...)
	fixed := fixedContent(result, content)

	assert.Equal(t, outline(t, content), outline(t, fixed), "the lists stay apart:\n%s", fixed)
	assert.NotEqual(t, content, fixed, "the fixes that keep the lists apart are applied")
	assert.False(t, result.Exhausted)
	assert.False(t, result.Oscillated)
}

// cancellingFixes are AA001's two fixes on "a  b \n": deleting the second
// space and inserting a space after the first. Each fix changes the text,
// but together they change nothing.
var cancellingFixes = [][]fix.TextEdit{
	{{StartOffset: 1, EndOffset: 2}},
	{{StartOffset: 2, EndOffset: 2, NewText: " "}},
}

func TestEngine_FixesOfOneRuleThatCancelOutDoNotHoldBackAnotherRule(t *testing.T) {
	t.Parallel()

	const content = "a  b \n"
	result := lintOnce(t, content, fixedRule("AA001", cancellingFixes...), productRules(t, "MD009")[0])

	assert.Equal(t, "a  b\n", string(fix.ApplyEdits([]byte(content), result.Edits)), "MD009 removes the trailing space")
	assert.Equal(t, 1, result.AcceptedFixes)
	assert.Empty(t, result.DeferredEdits)
}

func TestPipeline_FixesOfOneRuleThatCancelOutDoNotStarveAnotherRule(t *testing.T) {
	t.Parallel()

	const content = "a  b \n"
	result := fixWithRules(t, content, config.NewConfig(), fixedRule("AA001", cancellingFixes...), productRules(t, "MD009")[0])

	assert.Equal(t, "a  b\n", fixedContent(result, content))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied)
	assert.True(t, result.Oscillated, "the fixes that cancel out make no progress")
}

func TestPipeline_FixesOfOneRuleThatCancelOutAcrossTwoBlocksDoNotStarveAnotherRule(t *testing.T) {
	t.Parallel()

	// AA001 deletes the line ending of "x" and inserts it again at the
	// start of "y ". Both fixes touch both blocks, and together they change
	// nothing.
	const content = "x\n\ny \n"
	aa001 := fixedRule("AA001",
		[]fix.TextEdit{{StartOffset: 1, EndOffset: 2}},
		[]fix.TextEdit{{StartOffset: 3, EndOffset: 3, NewText: "\n"}})

	result := fixWithRules(t, content, config.NewConfig(), aa001, productRules(t, "MD009")[0])

	assert.Equal(t, "x\n\ny\n", fixedContent(result, content))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied)
}

func TestEngine_FixesThatCancelOutInOneBlockFreeItWhileTheRulesOtherFixesApply(t *testing.T) {
	t.Parallel()

	// AA001's fixes in the first block cancel out; its fix in the second
	// block changes "d".
	const content = "a  b \n\ncd\n"
	aa001 := fixedRule("AA001", append(slices.Clone(cancellingFixes),
		[]fix.TextEdit{{StartOffset: 8, EndOffset: 9, NewText: "D"}})...)

	result := lintOnce(t, content, aa001, productRules(t, "MD009")[0])

	assert.Equal(t, "a  b\n\ncD\n", string(fix.ApplyEdits([]byte(content), result.Edits)))
	assert.Equal(t, 2, result.AcceptedFixes, "MD009's fix and AA001's fix in the second block")
	assert.Empty(t, result.DeferredEdits)
}

// putBackLineEnding is AA001 on "# H\ntext\n": it deletes the line ending
// after the heading and inserts one again at the start of the next line.
// Each fix changes the text, but together they change nothing.
func putBackLineEnding() lint.Rule {
	return fixedRule("AA001",
		[]fix.TextEdit{{StartOffset: 3, EndOffset: 4}},
		[]fix.TextEdit{{StartOffset: 4, EndOffset: 4, NewText: "\n"}})
}

func TestEngine_AnotherRulesBlankLineIsNotAbsorbedByFixesThatCancelOut(t *testing.T) {
	t.Parallel()

	// MD022 inserts a blank line at the boundary where AA001 inserts its
	// line ending.
	const content = "# H\ntext\n"
	result := lintOnce(t, content, putBackLineEnding(), productRules(t, "MD022")[0])

	assert.Equal(t, "# H\n\ntext\n", string(fix.ApplyEdits([]byte(content), result.Edits)), "MD022 inserts its blank line")
	assert.Equal(t, 1, result.AcceptedFixes)
	assert.Empty(t, result.DeferredEdits)
}

func TestPipeline_AnotherRulesBlankLineIsNotAbsorbedByFixesThatCancelOut(t *testing.T) {
	t.Parallel()

	const content = "# H\ntext\n"
	result := fixWithRules(t, content, config.NewConfig(), putBackLineEnding(), productRules(t, "MD022")[0])

	assert.Equal(t, "# H\n\ntext\n", fixedContent(result, content))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied)
}

func TestPipeline_TwoRulesThatWantOneBlankLineInsertOne(t *testing.T) {
	t.Parallel()

	// MD022 wants a blank line after the heading and MD032 one before the
	// list, at the same boundary. Applied together, the two insertions
	// would leave two blank lines. MD032's waits for MD022's, and once the
	// blank line is there MD032 no longer reports.
	const content = "# H\n- item\n"
	result := fixWithRules(t, content, config.NewConfig(), productRules(t, "MD022", "MD032")...)

	assert.Equal(t, "# H\n\n- item\n", fixedContent(result, content))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied, "MD032's finding is gone, not fixed")
	assert.Empty(t, result.Diagnostics)
}

func TestPipeline_FixesBothMarkersOfAnEmphasisOnce(t *testing.T) {
	t.Parallel()

	// MD049 reports each marker of "_b_", and both findings carry the fix
	// that replaces both markers.
	const content = "# H\n\n*a* and _b_ here.\n"
	result := fixWithRules(t, content, config.NewConfig(), productRules(t, "MD049")...)

	assert.Equal(t, "# H\n\n*a* and *b* here.\n", fixedContent(result, content))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 2, result.TotalFixesApplied, "the fix fixes both findings")
	assert.Empty(t, result.Diagnostics)
}

func TestPipeline_FixesABlockWithManyRulesWithinThePassLimit(t *testing.T) {
	t.Parallel()

	// One top-level block, no blank lines, with findings of many rules:
	// each pass applies the fixes of one rule to it.
	const content = "# Title  \nSome text with trailing spaces   \n* item one\n*   item two with\ttab\n" +
		"+ item three\nVisit https://example.com today\nSome _emphasis_ and __strong__ and *more* and **more**\n" +
		"## Heading with punctuation:\n#Missing space heading\n>  quote with two spaces\n~~~\ncode\n~~~\n" +
		"```\nmore\n```\n1. one\n1. two\n3. three\n# Another top heading #\n" +
		"Text (reversed)[https://example.com] here.\n### Skipped level\n"

	result := fixWithRules(t, content, config.NewConfig(), lint.DefaultRegistry.Rules()...)

	assert.Greater(t, result.FixPasses, lint.DefaultMaxFixPasses, "the block takes a pass per rule")
	assert.False(t, result.Exhausted, "every fix is applied within the pass limit")
	assert.False(t, result.Oscillated)
	assert.Zero(t, result.FixableCount(), "no fixable finding remains")
}

// lintOnce lints content once in fix mode with only the given rules
// registered and enabled.
func lintOnce(t *testing.T, content string, rules ...lint.Rule) *lint.FileResult {
	t.Helper()

	cfg := fixConfig()
	for _, rule := range rules {
		cfg.EnableRules = append(cfg.EnableRules, rule.ID())
	}
	result, err := markdownEngine(rules...).LintFile(context.Background(), "test.md", []byte(content), cfg)
	require.NoError(t, err)
	return result
}
