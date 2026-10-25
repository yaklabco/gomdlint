package rules

import (
	"bytes"
	"unicode"
	"unicode/utf8"

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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD031")),
	}
}

// Apply checks that fenced code blocks are surrounded by blank lines.
func (r *BlanksAroundFencesRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	includeListItems := ctx.OptionBool("list_items", true)

	var diags []lint.Diagnostic
	inserted := blankLineInserts{}

	codeBlocks := ctx.CodeBlocks()
	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Only fenced code blocks have fences to surround. This also makes
		// sure the block has code block attributes.
		if !lint.IsFencedCodeBlock(cb) {
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
		if fenceOpenLine < 1 || fenceCloseLine > len(ctx.File.Lines) {
			continue
		}
		blankLine := lint.BlankLineText(ctx.File, cb)

		if fenceOpenLine > 1 && !isMarkdownlintBlankLine(lint.LineContent(ctx.File, fenceOpenLine-1)) {
			at := ctx.File.Lines[fenceOpenLine-1].StartOffset
			diagPos := mdast.SourcePosition{
				StartLine: fenceOpenLine, StartColumn: pos.StartColumn, EndLine: fenceOpenLine, EndColumn: pos.StartColumn,
			}
			diags = append(diags, r.blankLineDiagnostic(ctx, diagPos, "before", inserted.insert(at, blankLine)))
		}

		// Like markdownlint, a block whose last line is blank needs no blank after it.
		if fenceCloseLine < len(ctx.File.Lines) && !isMarkdownlintBlankLine(lint.LineContent(ctx.File, fenceCloseLine+1)) &&
			!isMarkdownlintBlankLine(lint.LineContent(ctx.File, fenceCloseLine)) {
			column := 1
			var builder *fix.EditBuilder
			// After an unclosed fence, an inserted line would become code.
			if cb.Block.CodeBlock.Closed {
				if start, _, ok := fenceRun(ctx.File, cb, fenceCloseLine); ok {
					column = start - ctx.File.Lines[fenceCloseLine-1].StartOffset + 1
				}
				builder = inserted.insert(ctx.File.Lines[fenceCloseLine].StartOffset, blankLine)
			}
			diagPos := mdast.SourcePosition{StartLine: fenceCloseLine, StartColumn: column, EndLine: fenceCloseLine, EndColumn: column}
			diags = append(diags, r.blankLineDiagnostic(ctx, diagPos, "after", builder))
		}
	}

	return verifyFixes(ctx, diags, fixAllowance{}), nil
}

func (r *BlanksAroundFencesRule) blankLineDiagnostic(
	ctx *lint.RuleContext, pos mdast.SourcePosition, side string, builder *fix.EditBuilder,
) lint.Diagnostic {
	diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, "Missing blank line "+side+" fenced code block").
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Add a blank line " + side + " the fenced code block")
	if builder != nil {
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
}

// blankLineInserts hands out insertion fixes, at most one per offset, so the
// blank line between two adjacent fences is inserted once.
type blankLineInserts map[int]bool

// insert returns a fix that inserts text at offset, or nil when an earlier
// fix already inserts there.
func (inserts blankLineInserts) insert(offset int, text string) *fix.EditBuilder {
	if inserts[offset] {
		return nil
	}
	inserts[offset] = true
	builder := fix.NewEditBuilder()
	builder.Insert(offset, text)
	return builder
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
		BaseRule: lint.NewBaseRuleFromMeta(catalogEntry("MD038")),
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
		diags = append(diags, r.checkCodeSpan(ctx, span)...)
	}
	return verifyFixes(ctx, diags, fixAllowance{codeSpanSpace: true}), nil
}

// checkCodeSpan reports the extra space at the start and at the end of a code
// span separately, each at the spaces its fix deletes, like markdownlint.
func (r *NoSpaceInCodeRule) checkCodeSpan(ctx *lint.RuleContext, span *mdast.Node) []lint.Diagnostic {
	inner := span.ContentRange()
	if inner.IsEmpty() {
		return nil
	}
	tokens := codeTextTokens(ctx.File, span, inner.StartOffset, inner.EndOffset)
	datas, paddings := resolveCodeText(tokens)
	if len(datas) == 0 {
		return nil
	}
	content := ctx.File.Content
	hasPadding := len(paddings) == 2

	// A space that separates a backtick in the content from the delimiter
	// is kept. It may be a multibyte space such as a no-break space, so the
	// kept character's whole encoding is left out of the count.
	startData := datas[0]
	startSpaces, startNext := leadingSpaceRun(content[startData.start:startData.end])
	startBacktick := startNext == '`'
	startCount := startSpaces
	if startBacktick && !hasPadding {
		_, kept := utf8.DecodeLastRune(content[startData.start : startData.start+startSpaces])
		startCount -= kept
	}

	endData := datas[len(datas)-1]
	endSpaces, endPrev := trailingSpaceRun(content[endData.start:endData.end])
	endBacktick := endPrev == '`'
	endCount := endSpaces
	if endBacktick && !hasPadding {
		_, kept := utf8.DecodeRune(content[endData.end-endSpaces : endData.end])
		endCount -= kept
	}

	removePadding := startCount > 0 && endCount > 0 && hasPadding && !startBacktick && !endBacktick

	var diags []lint.Diagnostic
	if startCount > 0 {
		from, to := startData.start, startData.start+startCount
		if removePadding {
			from = paddings[0].start
		}
		diags = append(diags, r.spaceDiagnostic(ctx, from, to, "Excessive leading space inside code span"))
	}
	if endCount > 0 {
		from, to := endData.end-endCount, endData.end
		if removePadding {
			to = paddings[1].end
		}
		diags = append(diags, r.spaceDiagnostic(ctx, from, to, "Excessive trailing space inside code span"))
	}
	return diags
}

// spaceDiagnostic reports the spaces in content[from:to] with a fix that
// deletes them, unless the range holds a line ending, which a fix must not join.
func (r *NoSpaceInCodeRule) spaceDiagnostic(ctx *lint.RuleContext, from, to int, msg string) lint.Diagnostic {
	startLine, startColumn := ctx.File.LineAt(from)
	endLine, endColumn := ctx.File.LineAt(to)
	pos := mdast.SourcePosition{StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn}
	diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
		WithSeverity(config.SeverityWarning).
		WithSuggestion("Remove extra spaces from inside the code span")
	if from < to && !bytes.ContainsAny(ctx.File.Content[from:to], "\r\n") {
		builder := fix.NewEditBuilder()
		builder.Delete(from, to)
		diagBuilder = diagBuilder.WithFix(builder)
	}
	return diagBuilder.Build()
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
