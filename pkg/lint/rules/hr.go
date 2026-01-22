package rules

import (
	"bytes"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
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
func (r *HRStyleRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	configStyle := ctx.OptionString("style", styleConsistent)

	hrs := ctx.ThematicBreaks()
	if len(hrs) == 0 {
		return nil, nil
	}

	var diags []lint.Diagnostic
	var expectedStyle string

	if configStyle != styleConsistent {
		expectedStyle = configStyle
	}

	for _, hr := range hrs {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		pos := hr.SourcePosition()
		if !pos.IsValid() {
			continue
		}

		lineContent := lint.LineContent(ctx.File, pos.StartLine)
		hrStyle := string(bytes.TrimSpace(lineContent))

		// Set expected style from first HR if consistent mode.
		if expectedStyle == "" {
			expectedStyle = hrStyle
			continue
		}

		// Check for style mismatch.
		if hrStyle != expectedStyle {
			line := ctx.File.Lines[pos.StartLine-1]

			// Build fix.
			builder := fix.NewEditBuilder()
			builder.ReplaceRange(line.StartOffset, line.NewlineStart, expectedStyle)

			diag := lint.NewDiagnostic(r.ID(), hr,
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
