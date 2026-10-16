package rules

import (
	"bytes"
	"unicode"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// BlanksAroundFencesRule checks that fenced code blocks are surrounded by blank lines.
type BlanksAroundFencesRule struct {
	lint.BaseRule
}

// NewBlanksAroundFencesRule creates a new blanks-around-fences rule.
func NewBlanksAroundFencesRule() *BlanksAroundFencesRule {
	return &BlanksAroundFencesRule{
		BaseRule: lint.NewBaseRule(
			"MD031",
			"blanks-around-fences",
			"Fenced code blocks should be surrounded by blank lines",
			[]string{"blank_lines", "code"},
			true,
		),
	}
}

// Apply checks that fenced code blocks are surrounded by blank lines.
func (r *BlanksAroundFencesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	includeListItems := ctx.OptionBool("list_items", true)

	var diags []lint.Diagnostic

	codeBlocks := ctx.CodeBlocks()
	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip indented code blocks.
		if lint.IsIndentedCodeBlock(cb) {
			continue
		}

		pos := cb.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		// Skip if in list item and list_items is false.
		if !includeListItems && r.isInListItem(cb) {
			continue
		}

		// The full span of a fenced code block runs from the opening fence
		// to the closing fence (or the last content line when unclosed).
		fenceOpenLine := pos.StartLine
		fenceCloseLine := pos.EndLine

		// Validate fence lines exist.
		if fenceOpenLine < 1 || fenceCloseLine > len(ctx.File.Lines) {
			continue
		}

		// Check for blank line before the opening fence.
		// Need blank on fenceOpenLine - 1 (the line before the fence).
		if fenceOpenLine > 1 && !lint.IsBlankLine(ctx.File, fenceOpenLine-1) {
			fenceLine := ctx.File.Lines[fenceOpenLine-1]

			builder := fix.NewEditBuilder()
			builder.Insert(fenceLine.StartOffset, "\n")

			diagPos := mdast.SourcePosition{
				StartLine:   fenceOpenLine,
				StartColumn: 1,
				EndLine:     fenceOpenLine,
				EndColumn:   1,
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
				"Missing blank line before fenced code block").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Add a blank line before the fenced code block").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}

		// Check for blank line after the closing fence.
		// Need blank on fenceCloseLine + 1 (the line after the fence).
		if fenceCloseLine < len(ctx.File.Lines) && !lint.IsBlankLine(ctx.File, fenceCloseLine+1) {
			fenceLine := ctx.File.Lines[fenceCloseLine-1]

			builder := fix.NewEditBuilder()
			builder.Insert(fenceLine.EndOffset, "\n")

			diagPos := mdast.SourcePosition{
				StartLine:   fenceCloseLine,
				StartColumn: 1,
				EndLine:     fenceCloseLine,
				EndColumn:   1,
			}

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos,
				"Missing blank line after fenced code block").
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Add a blank line after the fenced code block").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func (r *BlanksAroundFencesRule) isInListItem(node *mdast.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeListItem {
			return true
		}
	}
	return false
}

// NoSpaceInCodeRule checks for spaces inside code span elements.
type NoSpaceInCodeRule struct {
	lint.BaseRule
}

// NewNoSpaceInCodeRule creates a new no-space-in-code rule.
func NewNoSpaceInCodeRule() *NoSpaceInCodeRule {
	return &NoSpaceInCodeRule{
		BaseRule: lint.NewBaseRule(
			"MD038",
			"no-space-in-code",
			"Spaces inside code span elements",
			[]string{"code", "whitespace"},
			true,
		),
	}
}

// Apply reports code spans with spaces beside the backticks, following
// markdownlint: one space of padding on both sides is part of the syntax and
// allowed, a space that separates a backtick in the content from the
// delimiter is allowed, and content made only of spaces is allowed.
func (r *NoSpaceInCodeRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	for _, span := range ctx.CodeSpans() {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}
		if diag, ok := r.checkCodeSpan(ctx, span); ok {
			diags = append(diags, diag)
		}
	}
	return diags, nil
}

func (r *NoSpaceInCodeRule) checkCodeSpan(ctx *lint.RuleContext, span *mdast.Node) (lint.Diagnostic, bool) {
	inner := span.ContentRange()
	if inner.IsEmpty() {
		return lint.Diagnostic{}, false
	}
	tokens := codeTextTokens(ctx.File, span, inner.StartOffset, inner.EndOffset)
	datas, paddings := resolveCodeText(tokens)
	if len(datas) == 0 {
		return lint.Diagnostic{}, false
	}
	content := ctx.File.Content
	hasPadding := len(paddings) == 2

	startData := datas[0]
	startSpaces, startNext := leadingSpaceRun(content[startData.start:startData.end])
	startBacktick := startNext == '`'
	startCount := startSpaces
	if startBacktick && !hasPadding {
		startCount--
	}

	endData := datas[len(datas)-1]
	endSpaces, endPrev := trailingSpaceRun(content[endData.start:endData.end])
	endBacktick := endPrev == '`'
	endCount := endSpaces
	if endBacktick && !hasPadding {
		endCount--
	}

	if startCount <= 0 && endCount <= 0 {
		return lint.Diagnostic{}, false
	}
	removePadding := startCount > 0 && endCount > 0 && hasPadding && !startBacktick && !endBacktick

	builder := fix.NewEditBuilder()
	if startCount > 0 {
		from, to := startData.start, startData.start+startCount
		if removePadding {
			from = paddings[0].start
		}
		deleteSameLine(builder, content, from, to)
	}
	if endCount > 0 {
		from, to := endData.end-endCount, endData.end
		if removePadding {
			to = paddings[1].end
		}
		deleteSameLine(builder, content, from, to)
	}

	var msg string
	switch {
	case startCount > 0 && endCount > 0:
		msg = "Excessive spaces inside code span"
	case startCount > 0:
		msg = "Excessive leading space inside code span"
	default:
		msg = "Excessive trailing space inside code span"
	}
	diagBuilder := lint.NewDiagnostic(r.ID(), span, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove extra spaces from inside the code span")
	if len(builder.Edits) > 0 {
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build(), true
}

// deleteSameLine deletes content[from:to] unless the range holds a line
// ending, which a fix must not join.
func deleteSameLine(builder *fix.EditBuilder, content []byte, from, to int) {
	if from < to && !bytes.ContainsAny(content[from:to], "\r\n") {
		builder.Delete(from, to)
	}
}

// codeTextKind classifies a piece of code span content the way micromark does.
type codeTextKind int

const (
	codeTextData codeTextKind = iota
	codeTextSpace
	codeTextLineEnding
	codeTextPadding
)

// codeTextToken is a piece of code span content with absolute offsets.
type codeTextToken struct {
	kind       codeTextKind
	start, end int
}

// codeTextTokens splits the content of a code span into micromark's code text
// tokens: one token per space, one per line ending, and runs of other
// characters. Container prefixes and the indentation that paragraphs strip
// from continuation lines are not content.
func codeTextTokens(file *mdast.FileSnapshot, span *mdast.Node, start, end int) []codeTextToken {
	content := file.Content
	var tokens []codeTextToken
	for pos := start; pos < end; {
		switch ch := content[pos]; ch {
		case '\r', '\n':
			lineEnd := pos + 1
			if ch == '\r' && lineEnd < end && content[lineEnd] == '\n' {
				lineEnd++
			}
			tokens = append(tokens, codeTextToken{kind: codeTextLineEnding, start: pos, end: lineEnd})
			pos = lineEnd
			if pos < end {
				lineNum, _ := file.LineAt(pos)
				pos += len(lint.ContainerPrefix(file, span, lineNum))
				for pos < end && (content[pos] == ' ' || content[pos] == '\t') {
					pos++
				}
			}
		case ' ':
			tokens = append(tokens, codeTextToken{kind: codeTextSpace, start: pos, end: pos + 1})
			pos++
		default:
			dataEnd := pos
			for dataEnd < end && content[dataEnd] != ' ' && content[dataEnd] != '\r' && content[dataEnd] != '\n' {
				dataEnd++
			}
			tokens = append(tokens, codeTextToken{kind: codeTextData, start: pos, end: dataEnd})
			pos = dataEnd
		}
	}
	return tokens
}

// resolveCodeText applies micromark's resolveCodeText: when the content starts
// and ends with a space or line ending and has data in between, those two
// become padding; then runs of spaces and data merge into data tokens.
func resolveCodeText(tokens []codeTextToken) ([]codeTextToken, []codeTextToken) {
	if len(tokens) == 0 {
		return nil, nil
	}
	head, tail := 0, len(tokens)-1
	var paddings []codeTextToken
	if head < tail && tokens[head].kind != codeTextData && tokens[tail].kind != codeTextData {
		for idx := head + 1; idx < tail; idx++ {
			if tokens[idx].kind == codeTextData {
				paddings = []codeTextToken{tokens[head], tokens[tail]}
				head++
				tail--
				break
			}
		}
	}

	var datas []codeTextToken
	for idx := head; idx <= tail; idx++ {
		if tokens[idx].kind == codeTextLineEnding {
			continue
		}
		merged := tokens[idx]
		for idx+1 <= tail && tokens[idx+1].kind != codeTextLineEnding {
			idx++
			merged.end = tokens[idx].end
		}
		merged.kind = codeTextData
		datas = append(datas, merged)
	}
	return datas, paddings
}

// leadingSpaceRun returns the length of the whitespace that starts text when
// a non-space character follows it, and that character.
func leadingSpaceRun(text []byte) (int, byte) {
	trimmed := bytes.TrimLeftFunc(text, unicode.IsSpace)
	if len(trimmed) == 0 || len(trimmed) == len(text) {
		return 0, 0
	}
	return len(text) - len(trimmed), trimmed[0]
}

// trailingSpaceRun returns the length of the whitespace that ends text when a
// non-space character precedes it, and that character.
func trailingSpaceRun(text []byte) (int, byte) {
	trimmed := bytes.TrimRightFunc(text, unicode.IsSpace)
	if len(trimmed) == 0 || len(trimmed) == len(text) {
		return 0, 0
	}
	return len(text) - len(trimmed), trimmed[len(trimmed)-1]
}
