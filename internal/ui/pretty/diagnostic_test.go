package pretty_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/clipperhouse/displaywidth"
	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestFormatDiagnostic_Basic covers the order of a diagnostic line: the
// location, which editors open as path:line:col, then the severity, the
// rule and the message.
func TestFormatDiagnostic_Basic(t *testing.T) {
	styles := pretty.NewStyles(false) // No colors for easier testing

	diag := &lint.Diagnostic{
		RuleID:      "MD001",
		RuleName:    "heading-increment",
		Message:     "Heading level increment",
		Severity:    config.SeverityError,
		FilePath:    "test.md",
		StartLine:   10,
		StartColumn: 1,
		EndLine:     10,
		EndColumn:   15,
	}

	result := styles.FormatDiagnostic(diag, false, "")

	assert.Equal(t, "  test.md:10:1  error  heading-increment  Heading level increment\n", result,
		"rules are named by name, without the ID, by default")
}

// TestFormatDiagnostic_FallsBackToRuleID covers a diagnostic whose rule has
// no name, which is named by its ID instead.
func TestFormatDiagnostic_FallsBackToRuleID(t *testing.T) {
	styles := pretty.NewStyles(false)

	diag := &lint.Diagnostic{
		RuleID:      "MD001",
		Message:     "Heading level increment",
		Severity:    config.SeverityError,
		FilePath:    "test.md",
		StartLine:   10,
		StartColumn: 1,
	}

	assert.Equal(t, "  test.md:10:1  error  MD001  Heading level increment\n", styles.FormatDiagnostic(diag, false, ""))
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

// TestFormatSourceContext_CaretAlignment checks that the caret sits under
// the character at the 1-based column, counted in UTF-16 code units, even
// after wide characters, characters outside the Basic Multilingual Plane and
// tabs, by comparing display cells of the printed lines.
func TestFormatSourceContext_CaretAlignment(t *testing.T) {
	styles := pretty.NewStyles(false)

	tests := []struct {
		name   string
		line   string
		column int
		want   string // the printed source line, after the indent
		under  string // the text from the caret's cell onward
	}{
		{name: "ascii", line: "# Title  ", column: 8, want: "# Title  ", under: "  "},
		{name: "wide emoji", line: "\U0001F600 emoji   ", column: 9, want: "\U0001F600 emoji   ", under: "   "},
		{name: "emoji then tab", line: "\U0001F600\U0001F600 tab\there", column: 9, want: "\U0001F600\U0001F600 tab    here", under: "    here"},
		{name: "column inside an emoji", line: "\U0001F600x", column: 2, want: "\U0001F600x", under: "x"},
		{name: "combining mark", line: "e\u0301 combining   ", column: 13, want: "e\u0301 combining   ", under: "   "},
		{name: "bmp letters", line: "ÄÖÜ trail   ", column: 10, want: "ÄÖÜ trail   ", under: "   "},
		{name: "cjk", line: "中文 trailing ", column: 12, want: "中文 trailing ", under: " "},
		{name: "tab", line: "\tcode", column: 2, want: "    code", under: "code"},
		{name: "text then tab", line: "a\tb", column: 3, want: "a   b", under: "b"},
		{name: "end of line", line: "abc", column: 4, want: "abc", under: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := styles.FormatSourceContext(tt.line, tt.column)
			lines := strings.Split(strings.TrimSuffix(result, "\n"), "\n")
			if !assert.Len(t, lines, 2) {
				return
			}

			const indent = "        "
			printed := strings.TrimPrefix(lines[0], indent)
			assert.Equal(t, tt.want, printed)

			caretCell := displaywidth.String(lines[1]) - 1
			prefixCells := caretCell - len(indent)
			// The cells before the caret must hold exactly the printed
			// text before the target character.
			assert.Equal(t, tt.under, cellsFrom(printed, prefixCells))
		})
	}
}

// cellsFrom returns the part of text that starts at display cell offset.
func cellsFrom(text string, offset int) string {
	width := 0
	for idx, char := range text {
		if width == offset {
			return text[idx:]
		}
		width += displaywidth.Rune(char)
	}
	return ""
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

func TestFormatDiagnosticWithLayout_RuleFormat(t *testing.T) {
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
		format config.RuleFormat
		want   string
	}{
		{config.RuleFormatName, "  test.md:1:1  warning  no-trailing-spaces  Trailing whitespace\n"},
		{config.RuleFormatID, "  test.md:1:1  warning  MD009  Trailing whitespace\n"},
		{config.RuleFormatCombined, "  test.md:1:1  warning  MD009/no-trailing-spaces  Trailing whitespace\n"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			layout := pretty.DiagnosticLayout{RuleFormat: tt.format}
			assert.Equal(t, tt.want, styles.FormatDiagnosticWithLayout(diag, false, "", layout))
		})
	}
}

// layoutDiagnostics are the diagnostics of one file, with locations,
// severities and rule names of different display widths. The path holds
// wide characters, which take two cells each.
func layoutDiagnostics() []lint.Diagnostic {
	return []lint.Diagnostic{
		{
			RuleID: "MD009", RuleName: "no-trailing-spaces", Message: "Trailing whitespace",
			Severity: config.SeverityWarning, FilePath: "docs/文档.md", StartLine: 3, StartColumn: 11,
		},
		{
			RuleID: "MD001", RuleName: "heading-increment", Message: "Heading level jumped from H1 to H3",
			Severity: config.SeverityError, FilePath: "docs/文档.md", StartLine: 12, StartColumn: 1,
		},
		{
			RuleID: "MD053", RuleName: "link-image-reference-definitions", Message: "Unused reference definition [x]",
			Severity: config.SeverityWarning, FilePath: "docs/文档.md", StartLine: 40, StartColumn: 81,
		},
	}
}

// TestNewDiagnosticLayout_LinesUpColumns covers the diagnostics of a file:
// their location, severity and rule columns are padded to the widest entry,
// measured in display cells, so rule names and messages start in the same
// column on every line.
func TestNewDiagnosticLayout_LinesUpColumns(t *testing.T) {
	styles := pretty.NewStyles(false)
	diagnostics := layoutDiagnostics()

	layout := pretty.NewDiagnosticLayout(diagnostics, config.RuleFormatName)

	assert.Equal(t, pretty.DiagnosticLayout{
		RuleFormat:    config.RuleFormatName,
		LocationWidth: displaywidth.String("docs/文档.md:40:81"),
		SeverityWidth: len("warning"),
		RuleWidth:     len("link-image-reference-definitions"),
	}, layout)

	spaces := func(count int) string { return strings.Repeat(" ", count) }
	want := []string{
		"  docs/文档.md:3:11   warning  no-trailing-spaces" + spaces(16) + "Trailing whitespace\n",
		"  docs/文档.md:12:1   error    heading-increment" + spaces(17) + "Heading level jumped from H1 to H3\n",
		"  docs/文档.md:40:81  warning  link-image-reference-definitions  Unused reference definition [x]\n",
	}
	for idx := range diagnostics {
		got := styles.FormatDiagnosticWithLayout(&diagnostics[idx], false, "", layout)
		assert.Equal(t, want[idx], got)

		// The message starts after the indent, the three padded columns and
		// their separators.
		before, _, found := strings.Cut(got, diagnostics[idx].Message)
		if assert.True(t, found) {
			assert.Equal(t, 2+layout.LocationWidth+2+layout.SeverityWidth+2+layout.RuleWidth+2, displaywidth.String(before))
		}
	}
}

// TestNewDiagnosticLayout_RuleFormat covers the rule column, which is as wide
// as the widest identifier in the chosen format.
func TestNewDiagnosticLayout_RuleFormat(t *testing.T) {
	diagnostics := layoutDiagnostics()

	assert.Equal(t, len("MD053"), pretty.NewDiagnosticLayout(diagnostics, config.RuleFormatID).RuleWidth)
	assert.Equal(t, len("MD053/link-image-reference-definitions"),
		pretty.NewDiagnosticLayout(diagnostics, config.RuleFormatCombined).RuleWidth)
	assert.Equal(t, pretty.DiagnosticLayout{RuleFormat: config.RuleFormatID}, pretty.NewDiagnosticLayout(nil, config.RuleFormatID))
}

// ansiSequence matches the SGR escape sequences terminals use for styling.
var ansiSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// TestFormatDiagnosticWithLayout_PadsOutsideStyles covers styled output: the
// escape sequences of colors take no cells, so the columns must be padded
// by the width of the plain text and line up once the styling is removed.
func TestFormatDiagnosticWithLayout_PadsOutsideStyles(t *testing.T) {
	// A style that wraps its text in escape sequences, as colors do on a
	// terminal, whatever the color profile of the test process.
	escape := lipgloss.NewStyle().Transform(func(text string) string { return "\x1b[1;31m" + text + "\x1b[0m" })
	colored := &pretty.Styles{FilePath: escape, Error: escape, Warning: escape, RuleID: escape, Message: escape}
	plain := pretty.NewStyles(false)

	diagnostics := layoutDiagnostics()
	layout := pretty.NewDiagnosticLayout(diagnostics, config.RuleFormatName)
	for idx := range diagnostics {
		got := colored.FormatDiagnosticWithLayout(&diagnostics[idx], false, "", layout)
		assert.Contains(t, got, "\x1b[", "the styles apply")
		assert.Equal(t, plain.FormatDiagnosticWithLayout(&diagnostics[idx], false, "", layout),
			ansiSequence.ReplaceAllString(got, ""))
	}
}

// TestFormatDiagnosticWithLayout_EmptyMessage covers a diagnostic without a
// message, whose line ends at the rule identifier rather than in padding.
func TestFormatDiagnosticWithLayout_EmptyMessage(t *testing.T) {
	styles := pretty.NewStyles(false)
	diag := &lint.Diagnostic{
		RuleID: "MD009", RuleName: "no-trailing-spaces",
		Severity: config.SeverityWarning, FilePath: "a.md", StartLine: 1, StartColumn: 2,
	}
	layout := pretty.DiagnosticLayout{RuleFormat: config.RuleFormatName, RuleWidth: 32}

	assert.Equal(t, "  a.md:1:2  warning  no-trailing-spaces\n", styles.FormatDiagnosticWithLayout(diag, false, "", layout))
}
