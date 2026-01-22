package pretty

import (
	"fmt"
	"strings"

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
	builder.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
		location,
		severity,
		s.Message.Render(diag.Message),
		ruleDisplay,
	))

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

// FormatSourceContext formats the source line with a caret marker.
func (s *Styles) FormatSourceContext(line string, column int) string {
	var builder strings.Builder

	// Indent to align with diagnostic output
	const indent = "        "

	// Source line
	builder.WriteString(indent + s.SourceLine.Render(line) + "\n")

	// Caret marker
	if column > 0 {
		padding := indent + strings.Repeat(" ", column-1)
		builder.WriteString(padding + s.Caret.Render("^") + "\n")
	}

	return builder.String()
}

// FormatFileHeader formats a file header for grouped output.
func (s *Styles) FormatFileHeader(path string, issueCount int) string {
	header := s.FilePath.Render(path)
	if issueCount > 0 {
		header += s.Dim.Render(fmt.Sprintf(" (%d issues)", issueCount))
	}
	return header
}
