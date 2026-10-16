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

// styleConsistent is the configuration value for consistent style detection.
const styleConsistent = "consistent"

// minThematicBreakMarkers is the number of marker characters a thematic break needs.
const minThematicBreakMarkers = 3

// HRStyleRule checks for consistent horizontal rule style.
type HRStyleRule struct {
	lint.BaseRule
}

// NewHRStyleRule creates a new hr-style rule.
func NewHRStyleRule() *HRStyleRule {
	return &HRStyleRule{
		BaseRule: lint.NewBaseRule(
			"MD035",
			"hr-style",
			"Horizontal rule style",
			[]string{"hr"},
			true,
		),
	}
}

// Apply checks for consistent horizontal rule style.
//
// This uses the token stream instead of AST node positions because goldmark
// does not assign source positions to ThematicBreak AST nodes (Lines.Len() == 0).
// Tokens that are not thematic breaks are skipped: front matter delimiters,
// setext heading underlines, and lines inside code and HTML blocks.
//
// The fix rewrites only the marker characters, so indentation (for example
// inside a list item) and trailing whitespace are kept. It is withheld when
// the expected style is not a thematic break, or when it is a run of dashes
// directly under a non-blank line, which could make that line a setext heading.
func (r *HRStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := strings.TrimSpace(ctx.OptionString("style", styleConsistent))

	var diags []lint.Diagnostic
	var expectedStyle string

	if configStyle != styleConsistent {
		expectedStyle = configStyle
	}

	underlines := setextUnderlineLines(ctx)
	htmlLines := htmlBlockLines(ctx)

	for _, tok := range ctx.File.Tokens {
		if tok.Kind != mdast.TokThematicBreak {
			continue
		}

		if ctx.Cancelled() {
			return diags, ctx.Ctx.Err()
		}

		line, col := ctx.File.LineAt(tok.StartOffset)
		if line == 0 || line <= ctx.File.FrontMatterLines || underlines[line] || htmlLines[line] {
			continue
		}

		// Skip HRs inside code blocks.
		if ctx.IsLineInCodeBlock(line) {
			continue
		}

		marker := bytes.TrimRight(tok.Text(ctx.File.Content), " \t")
		hrStyle := string(marker)

		// Set expected style from first HR if consistent mode.
		if expectedStyle == "" {
			expectedStyle = hrStyle
			continue
		}

		// Check for style mismatch.
		if hrStyle != expectedStyle {
			pos := mdast.SourcePosition{
				StartLine:   line,
				StartColumn: col,
				EndLine:     line,
				EndColumn:   col + len(hrStyle),
			}

			diagBuilder := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Horizontal rule style %q does not match expected %q", hrStyle, expectedStyle)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all horizontal rules", expectedStyle))

			if hrFixAllowed(ctx.File, line, expectedStyle) {
				builder := fix.NewEditBuilder()
				builder.ReplaceRange(tok.StartOffset, tok.StartOffset+len(marker), expectedStyle)
				diagBuilder = diagBuilder.WithFix(builder)
			}

			diags = append(diags, diagBuilder.Build())
		}
	}

	return diags, nil
}

// setextUnderlineLines returns the lines holding setext heading underlines.
func setextUnderlineLines(ctx *lint.RuleContext) map[int]bool {
	lines := make(map[int]bool)
	for _, heading := range ctx.Headings() {
		if detectHeadingStyle(ctx.File, heading) != StyleSetext {
			continue
		}
		content, ok := headingContentRange(ctx.File, heading)
		if !ok {
			continue
		}
		lastLine, _ := ctx.File.LineAt(content.EndOffset - 1)
		lines[lastLine+1] = true
	}
	return lines
}

// hrFixAllowed reports whether a thematic break on line can be rewritten to
// style without changing the document structure.
func hrFixAllowed(file *mdast.FileSnapshot, line int, style string) bool {
	if !isThematicBreakStyle(style) {
		return false
	}
	// A run of dashes under a paragraph line is a setext heading underline.
	dashesOnly := strings.Trim(style, "-") == ""
	return !dashesOnly || line == 1 || lint.IsBlankLine(file, line-1)
}

// isThematicBreakStyle reports whether style is a valid thematic break: three
// or more of the same marker (-, * or _), optionally separated by spaces or tabs.
func isThematicBreakStyle(style string) bool {
	markers := strings.NewReplacer(" ", "", "\t", "").Replace(style)
	if len(markers) < minThematicBreakMarkers {
		return false
	}
	marker := markers[0]
	if marker != '-' && marker != '*' && marker != '_' {
		return false
	}
	return strings.Count(markers, string(marker)) == len(markers)
}
