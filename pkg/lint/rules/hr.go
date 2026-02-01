package rules

import (
	"bytes"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// styleConsistent is the configuration value for consistent style detection.
const styleConsistent = "consistent"

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
// The tokenizer correctly emits TokThematicBreak tokens with accurate byte offsets.
func (r *HRStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := ctx.OptionString("style", styleConsistent)

	var diags []lint.Diagnostic
	var expectedStyle string

	if configStyle != styleConsistent {
		expectedStyle = configStyle
	}

	for _, tok := range ctx.File.Tokens {
		if tok.Kind != mdast.TokThematicBreak {
			continue
		}

		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		line, col := ctx.File.LineAt(tok.StartOffset)
		if line == 0 {
			continue
		}

		// Skip HRs inside code blocks.
		if ctx.IsLineInCodeBlock(line) {
			continue
		}

		hrStyle := string(bytes.TrimSpace(tok.Text(ctx.File.Content)))

		// Set expected style from first HR if consistent mode.
		if expectedStyle == "" {
			expectedStyle = hrStyle
			continue
		}

		// Check for style mismatch.
		if hrStyle != expectedStyle {
			lineInfo := ctx.File.Lines[line-1]

			pos := mdast.SourcePosition{
				StartLine:   line,
				StartColumn: col,
				EndLine:     line,
				EndColumn:   col + len(hrStyle),
			}

			// Build fix.
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(lineInfo.StartOffset, lineInfo.NewlineStart, expectedStyle)

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos,
				fmt.Sprintf("Horizontal rule style %q does not match expected %q", hrStyle, expectedStyle)).
				WithSeverity(config.SeverityWarning).
				WithSuggestion(fmt.Sprintf("Use %q for all horizontal rules", expectedStyle)).
				WithFix(builder).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}
