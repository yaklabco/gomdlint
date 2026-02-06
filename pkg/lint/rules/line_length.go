package rules

import (
	"fmt"
	"strings"

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

	maxLength := ctx.OptionInt("max", defaultMaxLineLength)
	ignoreCodeBlocks := ctx.OptionBool("ignore_code_blocks", true)
	ignoreURLs := ctx.OptionBool("ignore_urls", true)

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		// Skip lines in code blocks if configured.
		if ignoreCodeBlocks && ctx.IsLineInCodeBlock(lineNum) {
			continue
		}

		length := lint.LineLength(ctx.File, lineNum)
		if length <= maxLength {
			continue
		}

		// Skip lines with URLs if configured.
		if ignoreURLs && lint.LineContainsURL(ctx.File, lineNum) {
			continue
		}

		pos := mdast.SourcePosition{
			StartLine:   lineNum,
			StartColumn: maxLength + 1,
			EndLine:     lineNum,
			EndColumn:   length,
		}

		diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
			fmt.Sprintf("Line length %d exceeds maximum %d", length, maxLength)).
			WithSeverity(config.SeverityWarning).
			WithSuggestion(fmt.Sprintf("Shorten the line to at most %d characters", maxLength))

		// Add autofix if possible.
		if fixer := r.buildWrapFix(ctx.File, lineNum, maxLength); fixer != nil {
			diagBuilder = diagBuilder.WithFix(fixer)
		}

		diags = append(diags, diagBuilder.Build())
	}

	return diags, nil
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
