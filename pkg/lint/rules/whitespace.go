package rules

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

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
// only applies to paragraph lines, where the spaces can produce a hard line
// break. The fix reads the line breaks the parser found, so it keeps every
// hard break and leaves alone whitespace that renderers read differently.
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
	lineEnds := trailingLineEnds(ctx)

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
		if builder := trailingWhitespaceFix(ctx.File.Content[start:end], start, end, lineEnds[lineNum], breakSpaces); builder != nil {
			diagBuilder = diagBuilder.WithFix(builder)
		}
		diags = append(diags, diagBuilder.Build())
	}

	return keepSafeFixes(ctx, diags), nil
}

// trailingLineEnd says how the parser reads the line ending after trailing
// whitespace, which decides whether the whitespace is rendered.
type trailingLineEnd int

const (
	// lineEndPlain is a line ending that ends a block or lies outside inline
	// content, where trailing whitespace is never rendered.
	lineEndPlain trailingLineEnd = iota
	// lineEndSoftBreak is a soft line break inside inline content.
	lineEndSoftBreak
	// lineEndHardBreak is a hard line break that the trailing whitespace
	// makes.
	lineEndHardBreak
	// lineEndCode is a line ending inside a code span, where it becomes a
	// space and the trailing whitespace is part of the code text.
	lineEndCode
)

// trailingLineEnds classifies the line ending of every line that ends inside
// inline content: lines ending in a soft or hard break, including breaks in
// headings and in containers, and lines ending inside a code span.
func trailingLineEnds(ctx *lint.RuleContext) map[int]trailingLineEnd {
	ends := make(map[int]trailingLineEnd)
	_ = mdast.Walk(ctx.Root, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
		switch node.Kind {
		case mdast.NodeHardBreak:
			ends[node.SourcePosition().StartLine] = lineEndHardBreak
		case mdast.NodeSoftBreak:
			ends[node.SourcePosition().StartLine] = lineEndSoftBreak
		default:
		}
		return nil
	})
	for _, span := range ctx.CodeSpans() {
		pos := span.SourcePosition()
		for lineNum := pos.StartLine; pos.IsValid() && lineNum < pos.EndLine; lineNum++ {
			ends[lineNum] = lineEndCode
		}
	}
	return ends
}

// trailingWhitespaceFix builds the fix for trailing whitespace without changing
// the rendered output. Whitespace that makes a hard line break is normalized
// to the configured break spaces, which every renderer reads as a hard break;
// it is reported without a fix when trailing-space breaks are disallowed
// (breakSpaces is 0) or already normalized. Before a soft break only a single
// space is removed: renderers such as pandoc read tabs and wider whitespace as
// a hard break where goldmark does not. Whitespace inside a code span is code
// text and is never fixed.
func trailingWhitespaceFix(trailing []byte, start, end int, lineEnd trailingLineEnd, breakSpaces int) *fix.EditBuilder {
	builder := fix.NewEditBuilder()
	switch lineEnd {
	case lineEndHardBreak:
		normalized := strings.Repeat(" ", breakSpaces)
		if breakSpaces < 2 || string(trailing) == normalized {
			return nil
		}
		builder.ReplaceRange(start, end, normalized)
	case lineEndSoftBreak:
		if string(trailing) != " " {
			return nil
		}
		builder.Delete(start, end)
	case lineEndCode:
		return nil
	case lineEndPlain:
		builder.Delete(start, end)
	}
	return builder
}

// hardBreakLines returns the lines where strict mode allows break spaces,
// as markdownlint decides them: every paragraph line except the last,
// outside code blocks and outside code spans that continue onto the next
// line.
func hardBreakLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, paragraph := range ctx.Paragraphs() {
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

	// A byte order mark is not part of the text, so a file holding only one
	// is empty.
	last := lint.RawLineContent(ctx.File, lastLine)
	last = last[min(len(last), lint.LineTextOffset(ctx.File, lastLine)):]
	if !isMarkdownlintBlankLine(last) {
		// Missing final newline. Like markdownlint, the finding points at
		// the last character, which the newline would follow.
		builder := fix.NewEditBuilder()
		builder.Insert(contentLen, "\n")

		end := lint.LineLength(ctx.File, lastLine) + 1
		_, size := utf8.DecodeLastRune(last)
		pos := mdast.SourcePosition{
			StartLine:   lastLine,
			StartColumn: end - size,
			EndLine:     lastLine,
			EndColumn:   end,
		}

		diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "File should end with a newline").
			WithSeverity(config.SeverityWarning).
			WithSuggestion("Add a newline at end of file").
			WithFix(builder).
			Build()
		return keepSafeFixes(ctx, []lint.Diagnostic{diag}), nil
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
		return keepSafeFixes(ctx, []lint.Diagnostic{diag}), nil
	}

	return nil, nil
}

// isBlankText reports whether the text of a line, after a byte order mark on
// the first line, is empty or whitespace.
func isBlankText(file *mdast.FileSnapshot, lineNum int) bool {
	return len(bytes.TrimSpace(lint.LineText(file, lineNum))) == 0
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

	// markdownlint reads its maximum as Number(maximum || 1), so 0 counts as
	// 1. A negative maximum counts as 1 too. With no blank line allowed, the
	// fix would remove the final line ending that MD047 puts back.
	maxConsecutive := ctx.OptionInt("max_consecutive", 1)
	if maxConsecutive < 1 {
		maxConsecutive = 1
	}

	var diags []lint.Diagnostic
	count := 0

	for lineNum := firstLineAfterFrontMatter(ctx.File); lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Like markdownlint, blank lines inside code blocks are content and
		// end any run in progress, and each blank line past the maximum is a
		// finding of its own.
		if ctx.IsLineInCodeBlock(lineNum) || !isBlankText(ctx.File, lineNum) {
			count = 0
			continue
		}
		count++
		if count > maxConsecutive {
			diags = append(diags, r.createDiagnostic(ctx, lineNum, count, maxConsecutive))
		}
	}

	return keepSafeFixes(ctx, diags), nil
}

// firstLineAfterFrontMatter returns the first line markdownlint checks for
// blank lines: the line after any front matter and the blank lines right
// after it, which markdownlint's front matter pattern ("^---\s*" and the
// like) takes into the front matter.
func firstLineAfterFrontMatter(file *mdast.FileSnapshot) int {
	line := file.FrontMatterLines + 1
	if file.FrontMatterLines == 0 {
		return line
	}
	for line <= len(file.Lines) && isBlankText(file, line) {
		line++
	}
	return line
}

// createDiagnostic reports a blank line past the maximum, the count-th in a
// row. Its fix deletes the line ending before the line and the line's own
// whitespace, so the fixes of a run's extra lines touch without overlapping
// and together remove them all, even the empty line after a final newline.
// Where that would leave a carriage return that ends a line right before
// the line feed that ends this one, which would make the two one line
// ending, the fix deletes this line and its line ending instead. The engine
// withholds that fix too where it would still join two line endings, as
// when the next line is empty and ends with a line feed.
func (r *MultipleBlankLinesRule) createDiagnostic(
	ctx *lint.RuleContext,
	lineNum, count, maxConsecutive int,
) lint.Diagnostic {
	content, lines := ctx.File.Content, ctx.File.Lines
	start, end := lines[lineNum-2].NewlineStart, lines[lineNum-1].NewlineStart
	if start > 0 && content[start-1] == '\r' && end < len(content) && content[end] == '\n' {
		start, end = lines[lineNum-1].StartOffset, lines[lineNum-1].EndOffset
	}
	builder := fix.NewEditBuilder()
	builder.Delete(start, end)

	pos := mdast.SourcePosition{
		StartLine:   lineNum,
		StartColumn: 1,
		EndLine:     lineNum,
		EndColumn:   1,
	}

	return lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
		fmt.Sprintf("Multiple consecutive blank lines (found %d, max %d)", count, maxConsecutive)).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove the blank line").
		WithFix(builder).
		Build()
}
