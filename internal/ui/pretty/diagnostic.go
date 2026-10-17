package pretty

import (
	"fmt"
	"strings"

	"github.com/clipperhouse/displaywidth"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// FormatDiagnostic formats a single diagnostic for terminal output.
// Uses ID format for backwards compatibility.
func (s *Styles) FormatDiagnostic(diag *lint.Diagnostic, showContext bool, sourceLine string) string {
	return s.FormatDiagnosticWithFormat(diag, showContext, sourceLine, config.RuleFormatID)
}

// FormatDiagnosticWithFormat formats a diagnostic with configurable rule identifier format.
func (s *Styles) FormatDiagnosticWithFormat(diag *lint.Diagnostic, showContext bool, sourceLine string, ruleFormat config.RuleFormat) string {
	var builder strings.Builder

	// Location: path:line:col
	location := fmt.Sprintf("%s:%d:%d",
		s.FilePath.Render(diag.FilePath),
		diag.StartLine,
		diag.StartColumn,
	)

	// Severity with prefix
	severity := s.FormatSeverity(diag.Severity)

	// Rule identifier formatted according to config
	ruleIdentifier := config.FormatRuleID(ruleFormat, diag.RuleID, diag.RuleName)
	ruleDisplay := s.RuleID.Render("(" + ruleIdentifier + ")")

	// Main line: location  severity  message  (rule-id)
	builder.WriteString("  " + location + "  " + severity + "  " + s.Message.Render(diag.Message) + "  " + ruleDisplay + "\n")

	// Source context
	if showContext && sourceLine != "" {
		builder.WriteString(s.FormatSourceContext(sourceLine, diag.StartColumn))
	}

	// Suggestion
	if diag.Suggestion != "" {
		builder.WriteString("    " + s.Dim.Render("Suggestion:") + " " +
			s.Suggestion.Render(diag.Suggestion) + "\n")
	}

	return builder.String()
}

// FormatSeverity returns a styled severity string.
func (s *Styles) FormatSeverity(sev config.Severity) string {
	switch sev {
	case config.SeverityError:
		return s.Error.Render("error")
	case config.SeverityWarning:
		return s.Warning.Render("warning")
	case config.SeverityInfo:
		return s.Info.Render("info")
	default:
		return string(sev)
	}
}

// contextTabWidth is the tab stop used to print source lines. CommonMark
// uses the same stop for indentation.
const contextTabWidth = 4

// FormatSourceContext formats the source line with a caret marker under
// column, a 1-based character column. The line is printed with its tabs
// expanded, and the caret is padded by the display width of the text
// before the column, so it lines up after wide characters and tabs.
func (s *Styles) FormatSourceContext(line string, column int) string {
	var builder strings.Builder

	// Indent to align with diagnostic output
	const indent = "        "

	runes := []rune(line)
	split := min(max(column-1, 0), len(runes))
	before, caretOffset := expandTabs(string(runes[:split]), 0)
	after, _ := expandTabs(string(runes[split:]), caretOffset)

	// Source line
	builder.WriteString(indent + s.SourceLine.Render(before+after) + "\n")

	// Caret marker
	if column > 0 {
		padding := indent + strings.Repeat(" ", caretOffset)
		builder.WriteString(padding + s.Caret.Render("^") + "\n")
	}

	return builder.String()
}

// expandTabs replaces the tabs in text with spaces up to the next tab stop,
// counting display cells from start. It returns the expanded text and the
// display cell after it.
func expandTabs(text string, start int) (string, int) {
	var out strings.Builder
	cell := start
	for {
		tab := strings.IndexByte(text, '\t')
		chunk := text
		if tab >= 0 {
			chunk = text[:tab]
		}
		out.WriteString(chunk)
		cell += displaywidth.String(chunk)
		if tab < 0 {
			return out.String(), cell
		}
		spaces := contextTabWidth - cell%contextTabWidth
		out.WriteString(strings.Repeat(" ", spaces))
		cell += spaces
		text = text[tab+1:]
	}
}

// FormatFileHeader formats a file header for grouped output.
func (s *Styles) FormatFileHeader(path string, issueCount int) string {
	header := s.FilePath.Render(path)
	if issueCount > 0 {
		header += s.Dim.Render(fmt.Sprintf(" (%d issues)", issueCount))
	}
	return header
}
