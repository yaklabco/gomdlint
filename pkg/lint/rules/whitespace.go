package rules

import (
	"bytes"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TrailingWhitespaceRule checks for trailing whitespace on lines.
type TrailingWhitespaceRule struct {
	lint.BaseRule
}

// NewTrailingWhitespaceRule creates a new trailing whitespace rule.
func NewTrailingWhitespaceRule() *TrailingWhitespaceRule {
	return &TrailingWhitespaceRule{
		BaseRule: lint.NewBaseRule(
			"MD009",
			"no-trailing-spaces",
			"Lines should not have trailing spaces",
			[]string{"whitespace"},
			true,
		),
	}
}

// Apply checks for trailing whitespace on each line.
func (r *TrailingWhitespaceRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	ignoreCodeBlocks := ctx.OptionBool("ignore_code_blocks", false)
	var codeLines map[int]bool
	if ignoreCodeBlocks {
		// Fence lines are not code, so their trailing whitespace is checked.
		codeLines = lint.CodeContentLines(ctx.CodeBlocks())
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip lines in code blocks if configured.
		if codeLines[lineNum] {
			continue
		}
		if ctx.OptionBool("list_item_empty_lines", false) && isListBlankLine(ctx, lineNum) {
			continue
		}

		if !lint.HasTrailingWhitespace(ctx.File, lineNum) {
			continue
		}

		start, end := lint.TrailingWhitespaceRange(ctx.File, lineNum)
		if start < 0 || end <= start {
			continue
		}
		breakSpaces := ctx.OptionInt("br_spaces", 0)
		if breakSpaces >= 2 && end-start == breakSpaces && (!ctx.OptionBool("strict", false) || isHardBreakLine(ctx, lineNum)) {
			continue
		}

		// Build the fix edit.
		builder := fix.NewEditBuilder()
		builder.Delete(start, end)

		// Calculate the column where trailing whitespace starts.
		line := ctx.File.Lines[lineNum-1]
		col := start - line.StartOffset + 1

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: col,
			EndLine:     lineNum,
			EndColumn:   end - line.StartOffset + 1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Trailing whitespace").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove trailing whitespace").
			WithFix(builder).
			Build()
		diags = append(diags, diag)
	}

	return diags, nil
}

func isHardBreakLine(ctx *lint.RuleContext, lineNum int) bool {
	if ctx.IsLineInCodeBlock(lineNum) {
		return false
	}
	for _, span := range ctx.CodeSpans() {
		pos := span.SourcePosition()
		if pos.StartLine <= lineNum && lineNum < pos.EndLine {
			return false
		}
	}
	for _, paragraph := range ctx.Paragraphs() {
		pos := paragraph.SourcePosition()
		if pos.StartLine <= lineNum && lineNum < pos.EndLine {
			return true
		}
	}
	return false
}

func isListBlankLine(ctx *lint.RuleContext, lineNum int) bool {
	if !lint.IsBlankLine(ctx.File, lineNum) {
		return false
	}
	for _, item := range ctx.ListItems() {
		pos := item.SourcePosition()
		if pos.StartLine <= lineNum && lineNum < pos.EndLine {
			return true
		}
	}
	return false
}

// FinalNewlineRule ensures files end with a single newline.
type FinalNewlineRule struct {
	lint.BaseRule
}

// NewFinalNewlineRule creates a new final newline rule.
func NewFinalNewlineRule() *FinalNewlineRule {
	return &FinalNewlineRule{
		BaseRule: lint.NewBaseRule(
			"MD047",
			"single-trailing-newline",
			"Files should end with a single newline character",
			[]string{"blank_lines"},
			true,
		),
	}
}

// Apply checks that the file ends with a newline.
//
// Like markdownlint, the rule only requires the last line to be blank, where a
// line holding only whitespace, blockquote markers, or HTML comments counts as
// blank. Extra blank lines at the end of the file are reported by MD012, unless
// the gomdlint-specific max_trailing_blank_lines option is set.
func (r *FinalNewlineRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Content) == 0 {
		return nil, nil
	}

	contentLen := len(ctx.File.Content)
	lastLine := len(ctx.File.Lines)

	if !isMarkdownlintBlankLine(lint.RawLineContent(ctx.File, lastLine)) {
		// Missing final newline.
		builder := fix.NewEditBuilder()
		builder.Insert(contentLen, "\n")

		pos := mdast.SourcePosition{
			StartLine:   lastLine,
			StartColumn: lint.LineLength(ctx.File, lastLine) + 1,
			EndLine:     lastLine,
			EndColumn:   lint.LineLength(ctx.File, lastLine) + 1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "File should end with a newline").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add a newline at end of file").
			WithFix(builder).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	// The trailing blank line limit is a gomdlint extension that markdownlint
	// does not have, so it only applies when configured explicitly.
	if ctx.Option("max_trailing_blank_lines", nil) == nil {
		return nil, nil
	}
	maxTrailingBlankLines := ctx.OptionInt("max_trailing_blank_lines", 1)

	// Count trailing blank lines (excluding the final newline on the last non-blank line).
	trailingBlankCount := 0
	for lineNum := len(ctx.File.Lines); lineNum >= 1; lineNum-- {
		if !lint.IsBlankLine(ctx.File, lineNum) {
			break
		}
		trailingBlankCount++
	}

	if trailingBlankCount > maxTrailingBlankLines {
		// Calculate the range to remove.
		excessCount := trailingBlankCount - maxTrailingBlankLines
		firstExcessLine := len(ctx.File.Lines) - trailingBlankCount + 1
		lastExcessLine := firstExcessLine + excessCount - 1

		startOffset := ctx.File.Lines[firstExcessLine-1].StartOffset
		endOffset := ctx.File.Lines[lastExcessLine-1].EndOffset

		builder := fix.NewEditBuilder()
		builder.Delete(startOffset, endOffset)

		pos := mdast.SourcePosition{
			StartLine:   firstExcessLine,
			StartColumn: 1,
			EndLine:     lastExcessLine,
			EndColumn:   1,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Too many trailing blank lines (found %d, max %d)", trailingBlankCount, maxTrailingBlankLines)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Remove %d trailing blank line(s)", excessCount)).
			WithFix(builder).
			Build()
		return []lint.Diagnostic{diag}, nil
	}

	return nil, nil
}

// isMarkdownlintBlankLine reports whether a line is blank the way markdownlint's
// isBlankLine helper defines it: empty, whitespace only, or holding nothing but
// blockquote markers and HTML comments (including unterminated comment parts).
func isMarkdownlintBlankLine(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return true
	}
	rest := bytes.ReplaceAll(removeHTMLCommentText(line), []byte(">"), nil)
	return len(bytes.TrimSpace(rest)) == 0
}

// removeHTMLCommentText strips HTML comment text from a single line, treating a
// leading unmatched "-->" or a trailing unmatched "<!--" as part of a comment.
func removeHTMLCommentText(line []byte) []byte {
	const (
		startComment = "<!--"
		endComment   = "-->"
	)
	text := append([]byte(nil), line...)
	for {
		start := bytes.Index(text, []byte(startComment))
		end := bytes.Index(text, []byte(endComment))
		switch {
		case end != -1 && (start == -1 || end < start):
			text = text[end+len(endComment):]
		case start != -1 && end != -1:
			text = append(text[:start], text[end+len(endComment):]...)
		case start != -1:
			text = text[:start]
		default:
			return text
		}
	}
}

// MultipleBlankLinesRule checks for consecutive blank lines.
type MultipleBlankLinesRule struct {
	lint.BaseRule
}

// NewMultipleBlankLinesRule creates a new multiple blank lines rule.
func NewMultipleBlankLinesRule() *MultipleBlankLinesRule {
	return &MultipleBlankLinesRule{
		BaseRule: lint.NewBaseRule(
			"MD012",
			"no-multiple-blank-lines",
			"Multiple consecutive blank lines should be collapsed",
			[]string{"whitespace", "layout"},
			true,
		),
	}
}

// Apply checks for sequences of blank lines exceeding the maximum.
func (r *MultipleBlankLinesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Lines) == 0 {
		return nil, nil
	}

	maxConsecutive := ctx.OptionInt("max_consecutive", 1)
	if maxConsecutive < 0 {
		maxConsecutive = 1
	}

	var diags []lint.Diagnostic
	streakStart := 0
	streakCount := 0

	for lineNum := ctx.File.FrontMatterLines + 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Like markdownlint, blank lines inside code blocks are content and
		// end any streak in progress.
		if !ctx.IsLineInCodeBlock(lineNum) && lint.IsBlankLine(ctx.File, lineNum) {
			if streakCount == 0 {
				streakStart = lineNum
			}
			streakCount++
		} else {
			if streakCount > maxConsecutive {
				diag := r.createDiagnostic(ctx, streakStart, streakCount, maxConsecutive)
				diags = append(diags, diag)
			}
			streakCount = 0
		}
	}

	// Handle trailing blank lines streak (but don't double-report with MD011).
	// We still report if there's a streak in the middle that ends at EOF.
	if streakCount > maxConsecutive {
		diag := r.createDiagnostic(ctx, streakStart, streakCount, maxConsecutive)
		diags = append(diags, diag)
	}

	return diags, nil
}

func (r *MultipleBlankLinesRule) createDiagnostic(
	ctx *lint.RuleContext,
	streakStart, streakCount, maxConsecutive int,
) lint.Diagnostic {
	excessCount := streakCount - maxConsecutive
	firstExcessLine := streakStart + maxConsecutive
	lastExcessLine := streakStart + streakCount - 1

	startOffset := ctx.File.Lines[firstExcessLine-1].StartOffset
	endOffset := ctx.File.Lines[lastExcessLine-1].EndOffset
	if last := ctx.File.Lines[lastExcessLine-1]; last.EndOffset == last.NewlineStart && firstExcessLine > 1 {
		// The final line has no line ending of its own (it may be the empty
		// line after a trailing newline), so deleting it means deleting the
		// line ending in front of the excess lines instead.
		startOffset = ctx.File.Lines[firstExcessLine-2].NewlineStart
	}

	builder := fix.NewEditBuilder()
	builder.Delete(startOffset, endOffset)

	pos := mdast.SourcePosition{
		StartLine:   firstExcessLine,
		StartColumn: 1,
		EndLine:     lastExcessLine,
		EndColumn:   1,
	}

	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
		fmt.Sprintf("Multiple consecutive blank lines (found %d, max %d)", streakCount, maxConsecutive)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion(fmt.Sprintf("Remove %d blank line(s)", excessCount)).
		WithFix(builder).
		Build()
}
