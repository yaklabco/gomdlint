package rules

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
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

	codeBlocks := lint.CodeBlocks(ctx.Root)
	for _, cb := range codeBlocks {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
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

		// IMPORTANT: goldmark SourcePosition for fenced code blocks:
		// pos.StartLine = first content line
		// pos.EndLine = closing fence line
		// So: opening fence is at pos.StartLine - 1
		fenceOpenLine := pos.StartLine - 1
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

// codeSpanPattern matches inline code spans with their content.
var codeSpanPattern = regexp.MustCompile("`+[^`]+`+")

// Apply checks for spaces inside code span elements.
func (r *NoSpaceInCodeRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic

	for lineNum := 1; lineNum <= len(ctx.File.Lines); lineNum++ {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Skip lines in code blocks.
		if lint.IsLineInCodeBlock(ctx.File, ctx.Root, lineNum) {
			continue
		}

		lineContent := lint.LineContent(ctx.File, lineNum)
		matches := codeSpanPattern.FindAllIndex(lineContent, -1)

		for _, match := range matches {
			start, end := match[0], match[1]
			codeSpan := string(lineContent[start:end])

			// Extract content between backticks.
			content := extractCodeSpanContent(codeSpan)
			if content == "" {
				continue
			}

			// Check for leading/trailing spaces.
			trimmed := strings.Trim(content, " ")
			hasLeading := len(content) > 0 && content[0] == ' '
			hasTrailing := len(content) > 0 && content[len(content)-1] == ' '

			// Allow single space padding if content contains backticks.
			if strings.Contains(trimmed, "`") {
				// Single space on each side is allowed for backtick-containing content.
				if len(content) >= 2 && content[0] == ' ' && content[len(content)-1] == ' ' {
					innerContent := content[1 : len(content)-1]
					if !strings.HasPrefix(innerContent, " ") && !strings.HasSuffix(innerContent, " ") {
						continue
					}
				}
			}

			// Only spaces content is allowed.
			if len(strings.TrimSpace(content)) == 0 {
				continue
			}

			// Check for excessive spaces.
			leadingSpaces := len(content) - len(strings.TrimLeft(content, " "))
			trailingSpaces := len(content) - len(strings.TrimRight(content, " "))

			if leadingSpaces <= 1 && trailingSpaces <= 1 {
				// Single space padding is allowed.
				continue
			}

			if !hasLeading && !hasTrailing {
				continue
			}

			line := ctx.File.Lines[lineNum-1]
			diagPos := mdast.SourcePosition{
				StartLine:   lineNum,
				StartColumn: start + 1,
				EndLine:     lineNum,
				EndColumn:   end + 1,
			}

			var msg string
			switch {
			case hasLeading && hasTrailing && (leadingSpaces > 1 || trailingSpaces > 1):
				msg = "Excessive spaces inside code span"
			case hasLeading && leadingSpaces > 1:
				msg = "Excessive leading space inside code span"
			case hasTrailing && trailingSpaces > 1:
				msg = "Excessive trailing space inside code span"
			default:
				continue
			}

			// Build fix.
			builder := fix.NewEditBuilder()
			backtickCount := countLeadingBackticks(codeSpan)
			backticks := strings.Repeat("`", backtickCount)
			fixedContent := backticks + strings.TrimSpace(content) + backticks
			builder.ReplaceRange(line.StartOffset+start, line.StartOffset+end, fixedContent)

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, diagPos, msg).
				WithSeverity(config.SeverityWarning).
				WithSuggestion("Remove extra spaces from inside the code span").
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

func extractCodeSpanContent(span string) string {
	// Count leading backticks.
	backtickCount := countLeadingBackticks(span)
	if backtickCount == 0 {
		return ""
	}

	// Remove leading and trailing backticks.
	content := span[backtickCount:]
	if len(content) < backtickCount {
		return ""
	}
	content = content[:len(content)-backtickCount]

	return content
}

func countLeadingBackticks(s string) int {
	count := 0
	for _, ch := range s {
		if ch != '`' {
			break
		}
		count++
	}
	return count
}
