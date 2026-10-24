package lint_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// These tests use the real parser: which fixes may share a pass depends on
// the file's top-level blocks.

func markdownEngine(rules ...lint.Rule) *lint.Engine {
	registry := lint.NewRegistry()
	for _, rule := range rules {
		registry.Register(rule)
	}
	return lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry)
}

func lintMarkdown(t *testing.T, content string, rules ...lint.Rule) *lint.FileResult {
	t.Helper()

	result, err := markdownEngine(rules...).LintFile(context.Background(), "test.md", []byte(content), fixConfig())
	require.NoError(t, err)
	return result
}

func fixMarkdown(t *testing.T, content string, opts lint.PipelineOptions, rules ...lint.Rule) *lint.PipelineResult {
	t.Helper()

	opts.Fix = true
	result, err := lint.NewPipeline(markdownEngine(rules...)).ProcessContent(
		context.Background(), "test.md", []byte(content), fixConfig(), opts)
	require.NoError(t, err)
	return result
}

func parseMarkdown(t *testing.T, content string) *mdast.FileSnapshot {
	t.Helper()

	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(content))
	require.NoError(t, err)
	return snapshot
}

func topLevelKinds(t *testing.T, content string) []mdast.NodeKind {
	t.Helper()

	var kinds []mdast.NodeKind
	for child := parseMarkdown(t, content).Root.FirstChild; child != nil; child = child.Next {
		kinds = append(kinds, child.Kind)
	}
	return kinds
}

// edit returns an edit that replaces the first occurrence of from in
// content with to.
func edit(t *testing.T, content, from, to string) fix.TextEdit {
	t.Helper()

	idx := strings.Index(content, from)
	require.GreaterOrEqual(t, idx, 0, "%q not in %q", from, content)
	return fix.TextEdit{StartOffset: idx, EndOffset: idx + len(from), NewText: to}
}

// fixedRule proposes the same fixes, one per diagnostic, on every pass
// whose content still holds the text each fix replaces.
func fixedRule(id string, fixes ...[]fix.TextEdit) *funcRule {
	return newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
		var diags []lint.Diagnostic
		for _, edits := range fixes {
			diags = append(diags, fixDiag(id, edits...))
		}
		return diags, nil
	})
}

// replaceAllRule replaces every occurrence of from with to, as one fix per
// occurrence, for as long as the content holds from.
func replaceAllRule(id, from, to string) *funcRule {
	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		var diags []lint.Diagnostic
		content := ctx.File.Content
		for offset := 0; ; {
			idx := bytes.Index(content[offset:], []byte(from))
			if idx < 0 {
				return diags, nil
			}
			start := offset + idx
			diags = append(diags, fixDiag(id, fix.TextEdit{StartOffset: start, EndOffset: start + len(from), NewText: to}))
			offset = start + len(from)
		}
	})
}

// fencedBlock is a fenced code block found by line: the indexes of its
// fence lines and its fence.
type fencedBlock struct {
	open, close int
	char        byte
	length      int
}

// fenceRun returns the fence character and run length at the start of line,
// or 0 when the line does not start with at least three backticks or tildes.
func fenceRun(line string) (byte, int) {
	if line == "" || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	run := len(line) - len(strings.TrimLeft(line, line[:1]))
	if run < 3 {
		return 0, 0
	}
	return line[0], run
}

// fencedBlocks finds the closed fenced code blocks of content's lines.
func fencedBlocks(lines []string) []fencedBlock {
	var blocks []fencedBlock
	for idx := 0; idx < len(lines); idx++ {
		char, length := fenceRun(lines[idx])
		if char == 0 {
			continue
		}
		for end := idx + 1; end < len(lines); end++ {
			if closeChar, closeLength := fenceRun(lines[end]); closeChar == char && closeLength >= length &&
				strings.Trim(lines[end], string(char)) == "" {
				blocks = append(blocks, fencedBlock{open: idx, close: end, char: char, length: length})
				idx = end
				break
			}
		}
	}
	return blocks
}

func fileLines(file *mdast.FileSnapshot) []string {
	lines := make([]string, len(file.Lines))
	for idx, line := range file.Lines {
		lines[idx] = string(file.Content[line.StartOffset:line.NewlineStart])
	}
	return lines
}

// promptRule removes the "$ " shell prompt from the lines of fenced code
// blocks, one fix per line. Each fix is safe on its own.
func promptRule(id string) *funcRule {
	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		lines := fileLines(ctx.File)
		var diags []lint.Diagnostic
		for _, block := range fencedBlocks(lines) {
			for idx := block.open + 1; idx < block.close; idx++ {
				if strings.HasPrefix(lines[idx], "$ ") {
					start := ctx.File.Lines[idx].StartOffset
					diags = append(diags, fixDiag(id, fix.TextEdit{StartOffset: start, EndOffset: start + 2}))
				}
			}
		}
		return diags, nil
	})
}

// backtickFenceRule turns tilde fences into backtick fences, longer than
// any backtick run that starts a line of the block, so the new fence is
// safe for the content as it is.
func backtickFenceRule(id string) *funcRule {
	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		lines := fileLines(ctx.File)
		var diags []lint.Diagnostic
		for _, block := range fencedBlocks(lines) {
			if block.char != '~' {
				continue
			}
			length := 3
			for idx := block.open + 1; idx < block.close; idx++ {
				if char, run := fenceRun(lines[idx]); char == '`' && run >= length {
					length = run + 1
				}
			}
			fence := strings.Repeat("`", length)
			var edits []fix.TextEdit
			for _, idx := range []int{block.open, block.close} {
				start := ctx.File.Lines[idx].StartOffset
				_, run := fenceRun(lines[idx])
				edits = append(edits, fix.TextEdit{StartOffset: start, EndOffset: start + run, NewText: fence})
			}
			diags = append(diags, fixDiag(id, edits...))
		}
		return diags, nil
	})
}

func TestEngine_DefersOtherRulesFixesInTheSameTopLevelBlock(t *testing.T) {
	t.Parallel()

	const content = "# Commands\n\n~~~sh\n$ ```\n$ ls\n~~~\n\nDone.\n"

	result := lintMarkdown(t, content, promptRule("TEST001"), backtickFenceRule("TEST002"))

	assert.Equal(t, []fix.TextEdit{
		{StartOffset: 18, EndOffset: 20},
		{StartOffset: 24, EndOffset: 26},
	}, result.Edits, "only the first rule's fixes touch the code block in this pass")
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST002", Edit: fix.TextEdit{StartOffset: 12, EndOffset: 15, NewText: "```"}},
		{RuleID: "TEST002", Edit: fix.TextEdit{StartOffset: 29, EndOffset: 32, NewText: "```"}},
	}, result.DeferredEdits)
	assert.Empty(t, result.SkippedEdits, "a deferred fix is not a conflict")
	assert.False(t, result.EditConflicts)
	assert.Empty(t, result.RuleErrors)
}

func TestPipeline_FixesFromTwoRulesInOneCodeBlockDoNotCorruptIt(t *testing.T) {
	t.Parallel()

	// Together, the first pass's fixes would turn "$ ```" into a closing
	// fence and make the rest of the file code.
	const content = "# Commands\n\n~~~sh\n$ ```\n$ ls\n~~~\n\nDone.\n"

	result := fixMarkdown(t, content, lint.PipelineOptions{}, promptRule("TEST001"), backtickFenceRule("TEST002"))

	const want = "# Commands\n\n````sh\n```\nls\n````\n\nDone.\n"
	assert.Equal(t, want, string(result.ModifiedContent),
		"the deferred fence fix is derived again from the content without prompts")
	assert.Equal(t, 2, result.FixPasses)
	assert.False(t, result.Exhausted)
	assert.Equal(t, []mdast.NodeKind{mdast.NodeHeading, mdast.NodeCodeBlock, mdast.NodeParagraph},
		topLevelKinds(t, string(result.ModifiedContent)))
}

func TestEngine_AppliesFixesOfDifferentRulesInDifferentBlocksInOnePass(t *testing.T) {
	t.Parallel()

	const content = "# Title\n\nfirst paragraph x\n\nsecond paragraph y\n"
	rules := []lint.Rule{
		fixedRule("TEST001", []fix.TextEdit{edit(t, content, "x", "X")}),
		fixedRule("TEST002", []fix.TextEdit{edit(t, content, "y", "Y")}),
	}

	result := lintMarkdown(t, content, rules...)
	assert.Equal(t, []fix.TextEdit{edit(t, content, "x", "X"), edit(t, content, "y", "Y")}, result.Edits)
	assert.Empty(t, result.DeferredEdits)

	fixed := fixMarkdown(t, content, lint.PipelineOptions{},
		replaceAllRule("TEST001", "x", "X"), replaceAllRule("TEST002", "y", "Y"))
	assert.Equal(t, "# Title\n\nfirst paragraph X\n\nsecond paragraph Y\n", string(fixed.ModifiedContent))
	assert.Equal(t, 1, fixed.FixPasses)
}

// characterRule deletes every occurrence of char unless that would make
// the file's first list loose: like a rule that checks its own fixes, it
// re-parses the content with its fix applied.
func characterRule(t *testing.T, id string, char byte) *funcRule {
	t.Helper()

	return newFuncRule(id, func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		content := ctx.File.Content
		var edits []fix.TextEdit
		for idx := range content {
			if content[idx] == char {
				edits = append(edits, fix.TextEdit{StartOffset: idx, EndOffset: idx + 1})
			}
		}
		if len(edits) == 0 {
			return nil, nil
		}
		diag := fixDiag(id, edits...)
		if !firstListIsTight(t, string(fix.ApplyEdits(content, edits))) {
			diag.FixEdits = nil
		}
		return []lint.Diagnostic{diag}, nil
	})
}

func firstListIsTight(t *testing.T, content string) bool {
	t.Helper()

	lists := mdast.FindByKind(parseMarkdown(t, content).Root, mdast.NodeList)
	return len(lists) > 0 && lint.IsTightList(lists[0])
}

func TestPipeline_FixesFromTwoRulesNeverCombineIntoALooseList(t *testing.T) {
	t.Parallel()

	// Deleting "x" or "y" keeps the list tight; deleting both leaves a
	// blank line between the items.
	const content = "- a\n  xy\n- b\n"
	require.True(t, firstListIsTight(t, content))

	first := lintMarkdown(t, content, characterRule(t, "TEST001", 'x'), characterRule(t, "TEST002", 'y'))
	assert.Equal(t, []fix.TextEdit{edit(t, content, "x", "")}, first.Edits)
	assert.Equal(t, []lint.SkippedEdit{{RuleID: "TEST002", Edit: edit(t, content, "y", "")}}, first.DeferredEdits)

	result := fixMarkdown(t, content, lint.PipelineOptions{}, characterRule(t, "TEST001", 'x'), characterRule(t, "TEST002", 'y'))
	assert.Equal(t, "- a\n  y\n- b\n", string(result.ModifiedContent),
		"the second rule sees the first fix and keeps the list tight")
	assert.True(t, firstListIsTight(t, string(result.ModifiedContent)))
	require.Len(t, result.Diagnostics, 1, "the finding the second rule cannot fix is still reported")
	assert.Equal(t, "TEST002", result.Diagnostics[0].RuleID)
}

func TestEngine_DefersAFixBetweenBlocksAgainstBothNeighbours(t *testing.T) {
	t.Parallel()

	// TEST001 removes the second blank line after the heading, which
	// touches the heading and the first paragraph.
	const content = "# A\n\n\nfirst x\n\nsecond y\n"
	gap := fix.TextEdit{StartOffset: 5, EndOffset: 6}
	result := lintMarkdown(t, content,
		fixedRule("TEST001", []fix.TextEdit{gap}),
		fixedRule("TEST002", []fix.TextEdit{edit(t, content, "x", "X")}),
		fixedRule("TEST003", []fix.TextEdit{edit(t, content, "y", "Y")}),
	)

	assert.Equal(t, []fix.TextEdit{gap, edit(t, content, "y", "Y")}, result.Edits)
	assert.Equal(t, []lint.SkippedEdit{{RuleID: "TEST002", Edit: edit(t, content, "x", "X")}}, result.DeferredEdits)
}

func TestEngine_DefersARulesFixesTogetherAcrossTheBlocksTheyConnect(t *testing.T) {
	t.Parallel()

	const content = "para one\n\npara two\n\npara three\n"
	one, two, three := edit(t, content, "one", "1"), edit(t, content, "two", "2"), edit(t, content, "three", "3")
	join := fix.TextEdit{StartOffset: two.StartOffset - 1, EndOffset: two.StartOffset - 1, NewText: "!"}

	result := lintMarkdown(t, content,
		fixedRule("TEST001", []fix.TextEdit{two}),
		// TEST002's second fix touches the first two paragraphs, so its
		// fixes there wait together; its fix in the third paragraph is
		// independent of them.
		fixedRule("TEST002", []fix.TextEdit{one}, []fix.TextEdit{{StartOffset: 0, EndOffset: 4, NewText: "Para"}, join},
			[]fix.TextEdit{three}),
	)

	assert.Equal(t, []fix.TextEdit{two, three}, result.Edits)
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST002", Edit: one},
		{RuleID: "TEST002", Edit: fix.TextEdit{StartOffset: 0, EndOffset: 4, NewText: "Para"}},
		{RuleID: "TEST002", Edit: join},
	}, result.DeferredEdits)
}

// In a file whose lines end with a carriage return alone, the engine writes
// a fix's line feeds as carriage returns, and whole lines inserted next to a
// disabled line still leave it as it was.
func TestEngine_KeepsLinesInsertedNextToADisabledLineWithCarriageReturns(t *testing.T) {
	t.Parallel()

	const content = "ab <!-- markdownlint-disable-line TEST001 -->\rcd\r"
	lineEnd := strings.Index(content, "\r")
	tests := []struct {
		name string
		edit fix.TextEdit
		want string
	}{
		{
			name: "line inserted at the end of the disabled line",
			edit: fix.TextEdit{StartOffset: lineEnd, EndOffset: lineEnd, NewText: "\n"},
			want: "ab <!-- markdownlint-disable-line TEST001 -->\r\rcd\r",
		},
		{
			name: "line inserted at the start of the disabled line",
			edit: fix.TextEdit{StartOffset: 0, EndOffset: 0, NewText: "x\n"},
			want: "x\rab <!-- markdownlint-disable-line TEST001 -->\rcd\r",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
				diag := fixDiag("TEST001", tt.edit)
				diag.StartLine = 2
				return []lint.Diagnostic{diag}, nil
			})

			result := lintMarkdown(t, content, rule)

			require.Len(t, result.Diagnostics, 1)
			require.Empty(t, result.RuleErrors, "the fix is valid")
			assert.True(t, result.Diagnostics[0].HasFix(), "the inserted line leaves the disabled line as it was")
			assert.Equal(t, tt.want, string(fix.ApplyEdits([]byte(content), result.Edits)))
		})
	}
}

func TestEngine_WithholdsFixThatChangesALineWhereTheRuleIsDisabled(t *testing.T) {
	t.Parallel()

	const content = "ab <!-- markdownlint-disable-line TEST001 -->\ncd\n"
	lineEnd := strings.Index(content, "\n")
	tests := []struct {
		name  string
		edit  fix.TextEdit
		keeps bool
	}{
		{"replacement on the disabled line", edit(t, content, "b", "x"), false},
		{"insertion inside the disabled line", fix.TextEdit{StartOffset: 1, EndOffset: 1, NewText: "x"}, false},
		{"deletion of the disabled line's newline", fix.TextEdit{StartOffset: lineEnd, EndOffset: lineEnd + 1}, false},
		{"replacement across both lines", fix.TextEdit{StartOffset: lineEnd - 1, EndOffset: lineEnd + 2, NewText: "x"}, false},
		{"line inserted at the end of the disabled line", fix.TextEdit{StartOffset: lineEnd, EndOffset: lineEnd, NewText: "\n"}, true},
		{"text inserted at the end of the disabled line", fix.TextEdit{StartOffset: lineEnd, EndOffset: lineEnd, NewText: "x"}, false},
		{"line inserted at the start of the disabled line", fix.TextEdit{StartOffset: 0, EndOffset: 0, NewText: "x\n"}, true},
		{"text inserted at the start of the disabled line", fix.TextEdit{StartOffset: 0, EndOffset: 0, NewText: "x"}, false},
		{"line inserted at the start of the enabled line", fix.TextEdit{StartOffset: lineEnd + 1, EndOffset: lineEnd + 1, NewText: "\n"}, true},
		{"text inserted at the start of the enabled line", fix.TextEdit{StartOffset: lineEnd + 1, EndOffset: lineEnd + 1, NewText: "x"}, true},
		{"replacement on the enabled line", edit(t, content, "cd", "x"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rule := newFuncRule("TEST001", func(*lint.RuleContext) ([]lint.Diagnostic, error) {
				diag := fixDiag("TEST001", tt.edit)
				diag.StartLine = 2
				return []lint.Diagnostic{diag}, nil
			})

			result := lintMarkdown(t, content, rule)

			require.Len(t, result.Diagnostics, 1, "the finding stays either way")
			assert.Equal(t, tt.keeps, result.Diagnostics[0].HasFix())
			if tt.keeps {
				assert.Equal(t, []fix.TextEdit{tt.edit}, result.Edits)
			} else {
				assert.Empty(t, result.Edits)
			}
		})
	}
}

func TestEngine_TreatsFrontMatterAsItsOwnBlock(t *testing.T) {
	t.Parallel()

	const content = "---\ntitle: x\n---\n# Heading\n"
	// Findings inside front matter are not reported, so both rules report
	// on the heading's line.
	onHeading := func(id string, edits ...fix.TextEdit) *funcRule {
		return newFuncRule(id, func(*lint.RuleContext) ([]lint.Diagnostic, error) {
			diag := fixDiag(id, edits...)
			diag.StartLine = 4
			return []lint.Diagnostic{diag}, nil
		})
	}

	result := lintMarkdown(t, content,
		onHeading("TEST001", edit(t, content, "x", "y")),
		onHeading("TEST002", edit(t, content, "Heading", "Title")),
	)

	assert.Equal(t, []fix.TextEdit{edit(t, content, "x", "y"), edit(t, content, "Heading", "Title")}, result.Edits)
	assert.Empty(t, result.DeferredEdits)
}

func TestPipeline_ReportsDeferredFixesAsRemainingWhenThePassLimitIsHit(t *testing.T) {
	t.Parallel()

	// TEST001 needs one pass per "x"; TEST002 waits for the paragraph.
	result := fixMarkdown(t, "xxx z\n", lint.PipelineOptions{MaxFixPasses: 2},
		replaceFirstRule("TEST001", "x", "y"), replaceAllRule("TEST002", "z", "Z"))

	assert.Equal(t, "yyx z\n", string(result.ModifiedContent))
	assert.True(t, result.Exhausted)
	assert.Equal(t, 2, result.RemainingEdits, "the fix still due and the deferred fix both remain")
	assert.Len(t, result.DeferredEdits, 1)
	assert.Empty(t, result.SkippedEdits)
	assert.Equal(t, 2, result.TotalFixesApplied, "only the two applied fixes count")
}

func TestPipeline_AppliesDeferredFixesOnceTheBlockIsFree(t *testing.T) {
	t.Parallel()

	result := fixMarkdown(t, "xxx z\n", lint.PipelineOptions{},
		replaceFirstRule("TEST001", "x", "y"), replaceAllRule("TEST002", "z", "Z"))

	assert.Equal(t, "yyy Z\n", string(result.ModifiedContent))
	assert.Equal(t, 4, result.FixPasses)
	assert.False(t, result.Exhausted)
	assert.Zero(t, result.RemainingEdits)
}

func TestPipeline_GivesEveryRuleOfABlockAPassWithinTheDefaultLimit(t *testing.T) {
	t.Parallel()

	// Each rule fixes its own word of one paragraph, so the block takes a
	// pass per rule: more passes than DefaultMaxFixPasses.
	count := lint.DefaultMaxFixPasses + 2
	words := make([]string, count)
	rules := make([]lint.Rule, count)
	for idx := range count {
		words[idx] = fmt.Sprintf("w%02d", idx)
		rules[idx] = replaceAllRule(fmt.Sprintf("TEST%03d", idx+1), words[idx], strings.ToUpper(words[idx]))
	}
	content := strings.Join(words, " ") + "\n"

	result := fixMarkdown(t, content, lint.PipelineOptions{}, rules...)

	assert.Equal(t, strings.ToUpper(content), string(result.ModifiedContent))
	assert.Equal(t, count, result.FixPasses)
	assert.False(t, result.Exhausted)
	assert.Zero(t, result.RemainingEdits)
	assert.Equal(t, count, result.TotalFixesApplied)
}

func TestPipeline_KeepsAnExplicitPassLimit(t *testing.T) {
	t.Parallel()

	result := fixMarkdown(t, "a b c\n", lint.PipelineOptions{MaxFixPasses: 2},
		replaceAllRule("TEST001", "a", "A"), replaceAllRule("TEST002", "b", "B"), replaceAllRule("TEST003", "c", "C"))

	assert.Equal(t, "A B c\n", string(result.ModifiedContent))
	assert.True(t, result.Exhausted)
	assert.Equal(t, 1, result.RemainingEdits)
}

func TestEngine_AFixThatChangesNothingDoesNotHoldBackAnotherRule(t *testing.T) {
	t.Parallel()

	const content = "para x\n"
	result := lintMarkdown(t, content,
		fixedRule("TEST001", []fix.TextEdit{edit(t, content, "para", "para")}),
		fixedRule("TEST002", []fix.TextEdit{edit(t, content, "x", "y")}),
	)

	assert.Equal(t, []fix.TextEdit{edit(t, content, "x", "y")}, result.Edits)
	assert.Equal(t, 1, result.AcceptedFixes, "the fix that changes nothing is not offered")
	assert.Empty(t, result.DeferredEdits)
}

func TestEngine_OffersFixesThatChangeNothingWhenNothingElseApplies(t *testing.T) {
	t.Parallel()

	const content = "para x\n"
	noop := edit(t, content, "para", "para")
	result := lintMarkdown(t, content, fixedRule("TEST001", []fix.TextEdit{noop}))

	assert.Equal(t, []fix.TextEdit{noop}, result.Edits, "the pipeline sees that the fix makes no progress")
	assert.Equal(t, 1, result.AcceptedFixes)
	assert.Empty(t, result.DeferredEdits)
}

func TestPipeline_AFixThatChangesNothingDoesNotStarveAnotherRule(t *testing.T) {
	t.Parallel()

	// Like a blank-line rule that proposes an empty edit at the end of the
	// file on every pass, next to a rule that removes trailing blank lines.
	emptyAtEnd := newFuncRule("TEST001", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		end := len(ctx.File.Content)
		return []lint.Diagnostic{fixDiag("TEST001", fix.TextEdit{StartOffset: end, EndOffset: end})}, nil
	})
	trailing := newFuncRule("TEST002", func(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
		content := ctx.File.Content
		trimmed := len(bytes.TrimRight(content, "\n")) + 1
		if trimmed >= len(content) {
			return nil, nil
		}
		return []lint.Diagnostic{fixDiag("TEST002", fix.TextEdit{StartOffset: trimmed, EndOffset: len(content)})}, nil
	})

	result := fixMarkdown(t, "# Title\n\ntext\n\n\n", lint.PipelineOptions{}, emptyAtEnd, trailing)

	assert.Equal(t, "# Title\n\ntext\n", string(result.ModifiedContent))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied, "the fix that changes nothing is never applied")
	assert.True(t, result.Oscillated, "the remaining fix makes no progress")
	assert.Equal(t, 1, result.RemainingEdits)
}

// cancelAndConflict is a rule whose first two fixes on "a  b\n" cancel out
// and whose third fix conflicts with the first.
func cancelAndConflict() *funcRule {
	return fixedRule("TEST001",
		[]fix.TextEdit{{StartOffset: 1, EndOffset: 2}},
		[]fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: " "}},
		[]fix.TextEdit{{StartOffset: 1, EndOffset: 3, NewText: "z"}})
}

func TestEngine_OfferedFixesThatChangeNothingKeepTheirConflicts(t *testing.T) {
	t.Parallel()

	const content = "a  b\n"
	result := lintMarkdown(t, content, cancelAndConflict())

	assert.Equal(t, content, string(fix.ApplyEdits([]byte(content), result.Edits)), "the offered fixes change nothing")
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 1, EndOffset: 3, NewText: "z"}},
	}, result.SkippedEdits)
	assert.True(t, result.EditConflicts)
}

func TestPipeline_ReportsTheConflictsOfFixesThatMakeNoProgress(t *testing.T) {
	t.Parallel()

	result := fixMarkdown(t, "a  b\n", lint.PipelineOptions{}, cancelAndConflict())

	assert.True(t, result.Oscillated)
	assert.Equal(t, 2, result.RemainingEdits)
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 1, EndOffset: 3, NewText: "z"}},
	}, result.SkippedEdits)
	assert.True(t, result.EditConflicts)
}

// blankLineConflictInIdleGroup is a rule whose fixes on "> >\n> > # A\n"
// delete the first line, insert it again twice and insert ">\n" at the same
// boundary. The duplicate insertions merge and cancel the deletion, and
// ">\n" is for a different container, so it conflicts with them.
func blankLineConflictInIdleGroup() *funcRule {
	return fixedRule("TEST001",
		[]fix.TextEdit{{StartOffset: 0, EndOffset: 4}},
		[]fix.TextEdit{{StartOffset: 4, EndOffset: 4, NewText: "> >\n"}},
		[]fix.TextEdit{{StartOffset: 4, EndOffset: 4, NewText: "> >\n"}},
		[]fix.TextEdit{{StartOffset: 4, EndOffset: 4, NewText: ">\n"}})
}

func TestEngine_OfferedFixesThatChangeNothingKeepTheirBlankLineConflicts(t *testing.T) {
	t.Parallel()

	const content = "> >\n> > # A\n"
	result := lintMarkdown(t, content, blankLineConflictInIdleGroup())

	assert.Equal(t, content, string(fix.ApplyEdits([]byte(content), result.Edits)), "the offered fixes change nothing")
	assert.Equal(t, 3, result.AcceptedFixes, "the deletion and the merged insertion of two findings")
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 4, EndOffset: 4, NewText: ">\n"}},
	}, result.SkippedEdits, "the insertion for another container is never offered")
	assert.True(t, result.EditConflicts)
}

func TestPipeline_ReportsTheBlankLineConflictsOfFixesThatMakeNoProgress(t *testing.T) {
	t.Parallel()

	result := fixMarkdown(t, "> >\n> > # A\n", lint.PipelineOptions{}, blankLineConflictInIdleGroup())

	assert.True(t, result.Oscillated)
	assert.Equal(t, 2, result.RemainingEdits)
	assert.Equal(t, []lint.SkippedEdit{
		{RuleID: "TEST001", Edit: fix.TextEdit{StartOffset: 4, EndOffset: 4, NewText: ">\n"}},
	}, result.SkippedEdits)
	assert.True(t, result.EditConflicts)
}

// putBack deletes the first character of the content and inserts it again
// right after itself: each edit changes the text, but together they change
// nothing.
var putBack = []fix.TextEdit{{StartOffset: 0, EndOffset: 1}, {StartOffset: 1, EndOffset: 1, NewText: "x"}}

func TestEngine_AFixWhoseEditsTogetherChangeNothingDoesNotHoldBackAnotherRule(t *testing.T) {
	t.Parallel()

	const content = "x y\n"
	result := lintMarkdown(t, content,
		fixedRule("TEST001", putBack),
		fixedRule("TEST002", []fix.TextEdit{edit(t, content, "y", "Y")}),
	)

	assert.Equal(t, []fix.TextEdit{edit(t, content, "y", "Y")}, result.Edits)
	assert.Equal(t, 1, result.AcceptedFixes)
	assert.Empty(t, result.DeferredEdits)
}

func TestPipeline_AFixWhoseEditsTogetherChangeNothingDoesNotStarveAnotherRule(t *testing.T) {
	t.Parallel()

	result := fixMarkdown(t, "x y\n", lint.PipelineOptions{},
		fixedRule("TEST001", putBack), replaceAllRule("TEST002", "y", "Y"))

	assert.Equal(t, "x Y\n", string(result.ModifiedContent))
	assert.Equal(t, 1, result.FixPasses)
	assert.Equal(t, 1, result.TotalFixesApplied)
	assert.True(t, result.Oscillated, "the remaining fix makes no progress")
}
