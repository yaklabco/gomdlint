package rules

import (
	"bytes"
	"fmt"
	"strings"

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

// defaultBreakSpaces is markdownlint's default br_spaces: two trailing spaces
// written to produce a hard line break are allowed.
const defaultBreakSpaces = 2

// Apply checks for trailing whitespace on each line.
//
// Defaults follow markdownlint: two trailing spaces (br_spaces) are allowed,
// code blocks are skipped (ignore_code_blocks), and with strict the allowance
// only applies where the spaces actually produce a hard line break.
func (r *TrailingWhitespaceRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	ignoreCodeBlocks := ctx.OptionBool("ignore_code_blocks", true)
	var codeLines map[int]bool
	if ignoreCodeBlocks {
		// Fence lines are not code, so their trailing whitespace is checked.
		codeLines = lint.CodeContentLines(ctx.CodeBlocks())
	}
	listItemEmptyLines := ctx.OptionBool("list_item_empty_lines", false)
	strict := ctx.OptionBool("strict", false)
	breakSpaces := ctx.OptionInt("br_spaces", defaultBreakSpaces)
	if breakSpaces < 2 {
		// markdownlint: values below 2 disallow trailing spaces entirely.
		breakSpaces = 0
	}
	breakLines := hardBreakLines(ctx)

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip lines in code blocks if configured.
		if codeLines[lineNum] {
			continue
		}
		if listItemEmptyLines && isListBlankLine(ctx, lineNum) {
			continue
		}

		if !lint.HasTrailingWhitespace(ctx.File, lineNum) {
			continue
		}

		start, end := lint.TrailingWhitespaceRange(ctx.File, lineNum)
		if start < 0 || end <= start {
			continue
		}
		canBreak := breakLines[lineNum]
		if end-start == breakSpaces && (!strict || canBreak) {
			continue
		}

		line := ctx.File.Lines[lineNum-1]
		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: start - line.StartOffset + 1,
			EndLine:     lineNum,
			EndColumn:   end - line.StartOffset + 1,
		}

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Trailing whitespace").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Remove trailing whitespace")
		if builder := trailingWhitespaceFix(ctx.File.Content[start:end], start, end, canBreak, breakSpaces); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

// trailingWhitespaceFix builds the fix for trailing whitespace without changing
// the rendered output: whitespace that forms a hard line break (two or more
// spaces before the line ending inside a paragraph) is normalized to the
// configured break spaces instead of being deleted. When trailing-space breaks
// are disallowed (breakSpaces is 0) such a break is reported without a fix.
func trailingWhitespaceFix(trailing []byte, start, end int, canBreak bool, breakSpaces int) *fix.EditBuilder {
	builder := fix.NewEditBuilder()
	hardBreak := canBreak && bytes.HasSuffix(trailing, []byte("  "))
	switch {
	case !hardBreak:
		builder.Delete(start, end)
	case breakSpaces >= 2:
		builder.ReplaceRange(start, end, strings.Repeat(" ", breakSpaces))
	default:
		return nil
	}
	return builder
}

// hardBreakLines returns the lines whose trailing spaces would produce a hard
// line break: every paragraph line except the last, outside code blocks and
// outside code spans that continue onto the next line.
func hardBreakLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, paragraph := range paragraphLikeNodes(ctx) {
		pos := paragraph.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		for lineNum := pos.StartLine; lineNum < pos.EndLine; lineNum++ {
			lines[lineNum] = true
		}
	}
	for _, span := range ctx.CodeSpans() {
		pos := span.SourcePosition()
		for lineNum := pos.StartLine; lineNum < pos.EndLine; lineNum++ {
			delete(lines, lineNum)
		}
	}
	for lineNum := range lines {
		if ctx.IsLineInCodeBlock(lineNum) {
			delete(lines, lineNum)
		}
	}
	return lines
}

// paragraphLikeNodes returns the paragraphs of the document, including the
// text blocks of tight list items, which the parser currently maps to raw nodes.
func paragraphLikeNodes(ctx *lint.RuleContext) []*mdast.Node {
	nodes := append([]*mdast.Node(nil), ctx.Paragraphs()...)
	for _, item := range ctx.ListItems() {
		for child := item.FirstChild; child != nil; child = child.Next {
			if isParagraphLike(child) && child.Kind != mdast.NodeParagraph {
				nodes = append(nodes, child)
			}
		}
	}
	return nodes
}

// isParagraphLike reports whether a node is a paragraph or the text block of a
// tight list item.
func isParagraphLike(node *mdast.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == mdast.NodeParagraph {
		return true
	}
	return node.Kind == mdast.NodeRaw && node.Ext == nil &&
		node.Parent != nil && node.Parent.Kind == mdast.NodeListItem
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
