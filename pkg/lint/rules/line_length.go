package rules

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// MaxLineLengthRule checks that lines do not exceed a maximum length.
type MaxLineLengthRule struct {
	lint.BaseRule
}

// NewMaxLineLengthRule creates a new max line length rule.
func NewMaxLineLengthRule() *MaxLineLengthRule {
	return &MaxLineLengthRule{
		BaseRule: lint.NewBaseRule(
			"MD013",
			"line-length",
			"Line length should not exceed the configured maximum",
			[]string{"line_length"},
			true, // Auto-fixable via line wrapping.
		),
	}
}

// defaultMaxLineLength is the default maximum line length.
const defaultMaxLineLength = 120

// Apply checks that no line exceeds the maximum length.
func (r *MaxLineLengthRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil || len(ctx.File.Lines) == 0 {
		return nil, nil
	}

	defaultLength := ctx.OptionInt("max", defaultMaxLineLength)
	ignoreCodeBlocks := ctx.OptionBool("ignore_code_blocks", true)
	ignoreURLs := ctx.OptionBool("ignore_urls", true)
	compatible := ctx.OptionBool("markdownlint_compatible", false)
	linkOnlyLines := make(map[int]bool)
	definitionLines := make(map[int]bool)
	codeLines := make(map[int]bool)
	if compatible {
		codeLines = markdownlintCodeLines(ctx)
		linkOnlyLines = markdownlintLinkOnlyLines(ctx)
		for _, definition := range ctx.RefContext().AllDefinitions {
			definitionLines[definition.LineNumber] = true
		}
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip lines in code blocks if configured.
		inCode := ctx.IsLineInCodeBlock(lineNum) || codeLines[lineNum]
		if ignoreCodeBlocks && inCode {
			continue
		}
		if ctx.OptionBool("ignore_tables", false) && lint.IsLineInTable(ctx.File, ctx.Root, lineNum) {
			continue
		}
		maxLength := defaultLength
		if inCode {
			maxLength = ctx.OptionInt("code_block_max", maxLength)
		} else if isHeading(string(lint.LineContent(ctx.File, lineNum))) {
			if ctx.OptionBool("ignore_headings", false) {
				continue
			}
			maxLength = ctx.OptionInt("heading_max", maxLength)
		}

		length := lint.LineLength(ctx.File, lineNum)
		endColumn := length
		byteLimit := maxLength
		measured := length
		if compatible {
			if definitionLines[lineNum] {
				continue
			}
			line := string(lint.LineContent(ctx.File, lineNum))
			length = markdownlintTextLength(line)
			byteLimit = markdownlintByteLimit(line, maxLength)
			value, exempt := markdownlintMeasuredLength(ctx, line, linkOnlyLines[lineNum])
			if exempt {
				continue
			}
			measured = value
		}
		if maxLength <= 0 || measured <= maxLength {
			continue
		}

		// Skip lines with URLs if configured.
		if ignoreURLs && lint.LineContainsURL(ctx.File, lineNum) {
			continue
		}

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: byteLimit + 1,
			EndLine:     lineNum,
			EndColumn:   endColumn,
		}

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Line length %d exceeds maximum %d", length, maxLength)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Shorten the line to at most %d characters", maxLength))

		// Add autofix if possible.
		if !compatible || !inCode {
			if fixer := r.buildWrapFix(ctx.File, lineNum, byteLimit); fixer != nil {
				diagBuilder = diagBuilder.WithFix(fixer)
			}
		}

		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
}

func markdownlintMeasuredLength(ctx *lint.RuleContext, line string, linkOnly bool) (int, bool) {
	if ctx.OptionBool("strict", false) {
		return markdownlintTextLength(line), false
	}
	if linkOnly {
		return 0, true
	}
	if ctx.OptionBool("stern", false) {
		return markdownlintTextLength(line), isUnwrappableMarkdownlintLine(line)
	}
	prefix := strings.TrimRightFunc(line, func(char rune) bool { return !unicode.IsSpace(char) })
	return markdownlintTextLength(prefix) + 1, false
}

func markdownlintTextLength(text string) int {
	length := 0
	for _, char := range text {
		length += utf16.RuneLen(char)
	}
	return length
}

func markdownlintByteLimit(text string, limit int) int {
	length := 0
	for offset, char := range text {
		length += utf16.RuneLen(char)
		if length > limit {
			return offset
		}
	}
	return len(text)
}

func markdownlintCodeLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, block := range ctx.CodeBlocks() {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			continue
		}
		for line := pos.StartLine; line <= pos.EndLine; line++ {
			lines[line] = true
		}
		if lint.IsFencedCodeBlock(block) {
			fence := strings.Repeat(string(lint.CodeFenceChar(block)), lint.CodeFenceLength(block))
			for _, line := range []int{pos.StartLine - 1, pos.EndLine + 1} {
				if strings.HasPrefix(strings.TrimSpace(string(lint.LineContent(ctx.File, line))), fence) {
					lines[line] = true
				}
			}
		}
	}
	return lines
}

func markdownlintLinkOnlyLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, link := range ctx.Links() {
		pos := link.SourcePosition()
		for line := pos.StartLine; line <= pos.EndLine; line++ {
			lines[line] = true
		}
	}
	for _, paragraph := range ctx.Paragraphs() {
		_ = mdast.Walk(paragraph, func(node *mdast.Node) error { //nolint:errcheck // Visitor never returns an error.
			if node.Inline == nil || len(node.Inline.Text) == 0 || node.Kind == mdast.NodeCodeSpan {
				return nil
			}
			for parent := node.Parent; parent != nil && parent != paragraph; parent = parent.Parent {
				if parent.Kind == mdast.NodeLink || parent.Kind == mdast.NodeImage {
					return nil
				}
			}
			if strings.TrimSpace(string(node.Inline.Text)) != "" {
				delete(lines, node.SourcePosition().StartLine)
			}
			return nil
		})
	}
	return lines
}

func isUnwrappableMarkdownlintLine(line string) bool {
	line = strings.TrimLeft(line, "#> \t")
	return !strings.ContainsFunc(line, unicode.IsSpace)
}

// buildWrapFix creates a fix to wrap a long line at word boundary.
func (r *MaxLineLengthRule) buildWrapFix(
	file *mdast.FileSnapshot,
	lineNum int,
	maxLen int,
) *fix.EditBuilder {
	if lineNum < 1 || lineNum > len(file.Lines) {
		return nil
	}

	lineInfo := file.Lines[lineNum-1]
	content := string(file.Content[lineInfo.StartOffset:lineInfo.NewlineStart])

	// Skip headings.
	if isHeading(content) {
		return nil
	}

	// Skip table lines.
	if isTableLine(content) {
		return nil
	}

	// Get prefix for continuation line.
	prefix, contentStart := linePrefix(content)

	// Find wrap point (last space before maxLen).
	wrapPoint := findWrapPoint(content, maxLen)
	if wrapPoint <= contentStart {
		return nil // Can't wrap - no suitable break point.
	}

	// Create the wrapped version.
	firstPart := content[:wrapPoint]
	secondPart := strings.TrimLeft(content[wrapPoint:], " ")
	newContent := firstPart + "\n" + prefix + secondPart

	builder := fix.NewEditBuilder()
	builder.ReplaceRange(lineInfo.StartOffset, lineInfo.NewlineStart, newContent)
	return builder
}

// linePrefix extracts the prefix for continuation lines.
// Returns the prefix string and the start position of actual content.
func linePrefix(line string) (string, int) {
	pos := 0
	lineLen := len(line)
	var prefixBuilder strings.Builder

	// Skip leading whitespace.
	for pos < lineLen && (line[pos] == ' ' || line[pos] == '\t') {
		_ = prefixBuilder.WriteByte(line[pos]) // strings.Builder.WriteByte never fails
		pos++
	}
	leadingSpace := prefixBuilder.String()
	prefixBuilder.Reset()
	prefixBuilder.WriteString(leadingSpace)

	// Check for blockquote prefix.
	if pos < lineLen && line[pos] == '>' {
		_ = prefixBuilder.WriteByte('>') // strings.Builder.WriteByte never fails
		pos++
		// Skip space after >.
		if pos < lineLen && line[pos] == ' ' {
			_ = prefixBuilder.WriteByte(' ') // strings.Builder.WriteByte never fails
			pos++
		}
		// Recursively check for nested structures.
		nestedPrefix, nestedStart := linePrefix(line[pos:])
		prefixBuilder.WriteString(nestedPrefix)
		return prefixBuilder.String(), pos + nestedStart
	}

	// Check for list markers (-, *, +, or number.).
	listStart := pos
	if pos < lineLen && (line[pos] == '-' || line[pos] == '*' || line[pos] == '+') {
		pos++
		if pos < lineLen && line[pos] == ' ' {
			// List item: continuation uses spaces to align.
			markerLen := pos - listStart + 1
			prefixBuilder.WriteString(strings.Repeat(" ", markerLen))
			pos++
			return prefixBuilder.String(), pos
		}
		pos = listStart // Not a list marker, reset.
	}

	// Check for numbered list (1. or 1)).
	if pos < lineLen && line[pos] >= '0' && line[pos] <= '9' {
		for pos < lineLen && line[pos] >= '0' && line[pos] <= '9' {
			pos++
		}
		if pos < lineLen && (line[pos] == '.' || line[pos] == ')') {
			pos++
			if pos < lineLen && line[pos] == ' ' {
				markerLen := pos - listStart + 1
				prefixBuilder.WriteString(strings.Repeat(" ", markerLen))
				pos++
				return prefixBuilder.String(), pos
			}
		}
		// Not a numbered list, fall through to return leading space.
	}

	// Plain paragraph - no special prefix needed for continuation.
	return leadingSpace, len(leadingSpace)
}

// findWrapPoint finds the last space before maxLen.
func findWrapPoint(line string, maxLen int) int {
	if len(line) <= maxLen {
		return -1
	}

	// Find last space before or at maxLen.
	lastSpace := -1
	for i := 0; i < len(line) && i <= maxLen; i++ {
		if line[i] == ' ' {
			lastSpace = i
		}
	}
	return lastSpace
}

// isHeading checks if a line is a heading.
func isHeading(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return len(trimmed) > 0 && trimmed[0] == '#'
}

// isTableLine checks if a line is part of a table.
func isTableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return len(trimmed) > 0 && trimmed[0] == '|'
}
