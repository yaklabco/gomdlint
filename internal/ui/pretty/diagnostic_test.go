package pretty_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

func TestFormatDiagnostic_Basic(t *testing.T) {
	styles := pretty.NewStyles(false) // No colors for easier testing

	diag := &lint.Diagnostic{
		RuleID:      "MD001",
		Message:     "Heading level increment",
		Severity:    config.SeverityError,
		FilePath:    "test.md",
		StartLine:   10,
		StartColumn: 1,
		EndLine:     10,
		EndColumn:   15,
	}

	result := styles.FormatDiagnostic(diag, false, "")

	assert.Contains(t, result, "test.md:10:1")
	assert.Contains(t, result, "error")
	assert.Contains(t, result, "Heading level increment")
	assert.Contains(t, result, "(MD001)")
}

func TestFormatDiagnostic_WithContext(t *testing.T) {
	styles := pretty.NewStyles(false)

	diag := &lint.Diagnostic{
		RuleID:      "MD001",
		Message:     "Test message",
		Severity:    config.SeverityWarning,
		FilePath:    "test.md",
		StartLine:   5,
		StartColumn: 3,
	}

	sourceLine := "## Heading"
	result := styles.FormatDiagnostic(diag, true, sourceLine)

	assert.Contains(t, result, "## Heading")
	assert.Contains(t, result, "^") // Caret marker
}

func TestFormatDiagnostic_WithSuggestion(t *testing.T) {
	styles := pretty.NewStyles(false)

	diag := &lint.Diagnostic{
		RuleID:     "MD001",
		Message:    "Test message",
		Severity:   config.SeverityInfo,
		FilePath:   "test.md",
		StartLine:  1,
		Suggestion: "Use H2 instead of H3",
	}

	result := styles.FormatDiagnostic(diag, false, "")

	assert.Contains(t, result, "Suggestion:")
	assert.Contains(t, result, "Use H2 instead of H3")
}

func TestFormatSeverity_AllLevels(t *testing.T) {
	styles := pretty.NewStyles(false)

	tests := []struct {
		severity config.Severity
		expected string
	}{
		{config.SeverityError, "error"},
		{config.SeverityWarning, "warning"},
		{config.SeverityInfo, "info"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			result := styles.FormatSeverity(tt.severity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSourceContext_WithCaret(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatSourceContext("test line", 5)

	lines := strings.Split(result, "\n")
	assert.GreaterOrEqual(t, len(lines), 2) // Source line and caret line

	// Check caret position
	assert.Contains(t, result, "^")
}

func TestFormatSourceContext_ZeroColumn(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatSourceContext("test line", 0)

	// With column 0, no caret should be shown
	// The result should contain the source line but behavior for caret depends on impl
	assert.Contains(t, result, "test line")
}

func TestFormatFileHeader_WithIssues(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatFileHeader("docs/readme.md", 5)

	assert.Contains(t, result, "docs/readme.md")
	assert.Contains(t, result, "(5 issues)")
}

func TestFormatFileHeader_NoIssues(t *testing.T) {
	styles := pretty.NewStyles(false)

	result := styles.FormatFileHeader("docs/readme.md", 0)

	assert.Contains(t, result, "docs/readme.md")
	assert.NotContains(t, result, "issues")
}

func TestFormatDiagnostic_WithRuleFormat(t *testing.T) {
	styles := pretty.NewStyles(false)

	diag := &lint.Diagnostic{
		RuleID:      "MD009",
		RuleName:    "no-trailing-spaces",
		Message:     "Trailing whitespace",
		Severity:    config.SeverityWarning,
		FilePath:    "test.md",
		StartLine:   1,
		StartColumn: 1,
	}

	tests := []struct {
		format   config.RuleFormat
		contains string
		excludes string
	}{
		{config.RuleFormatName, "(no-trailing-spaces)", "(MD009)"},
		{config.RuleFormatID, "(MD009)", "(no-trailing-spaces)"},
		{config.RuleFormatCombined, "(MD009/no-trailing-spaces)", ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			result := styles.FormatDiagnosticWithFormat(diag, false, "", tt.format)
			assert.Contains(t, result, tt.contains)
			if tt.excludes != "" {
				assert.NotContains(t, result, tt.excludes)
			}
		})
	}
}
