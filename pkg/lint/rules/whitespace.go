package rules

import (
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

	var codeBlockLines map[int]bool
	if ignoreCodeBlocks {
		codeBlockLines = buildCodeBlockLineSet(ctx.Root)
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Skip lines in code blocks if configured.
		if ignoreCodeBlocks && codeBlockLines[lineNum] {
			continue
		}

		if !lint.HasTrailingWhitespace(ctx.File, lineNum) {
			continue
		}

		start, end := lint.TrailingWhitespaceRange(ctx.File, lineNum)
		if start < 0 || end <= start {
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

// Apply checks that the file ends with exactly one newline.
func (r *FinalNewlineRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Content) == 0 {
		return nil, nil
	}

	content := ctx.File.Content
	contentLen := len(content)

	// Check if file ends with a newline.
	if content[contentLen-1] != '\n' {
		// Missing final newline.
		builder := fix.NewEditBuilder()
		builder.Insert(contentLen, "\n")

		lastLine := len(ctx.File.Lines)
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

	// Check for excessive trailing blank lines.
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

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		if lint.IsBlankLine(ctx.File, lineNum) {
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

// buildCodeBlockLineSet returns a set of line numbers that are inside code blocks.
func buildCodeBlockLineSet(root *mdast.Node) map[int]bool {
	lineSet := make(map[int]bool)
	if root == nil {
		return lineSet
	}

	codeBlocks := lint.CodeBlocks(root)
	for _, codeBlock := range codeBlocks {
		pos := codeBlock.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		for line := pos.StartLine; line <= pos.EndLine; line++ {
			lineSet[line] = true
		}
	}

	return lineSet
}
