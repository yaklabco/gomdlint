package pretty

import (
	"fmt"
	"strings"

	"github.com/clipperhouse/displaywidth"

	"github.com/yaklabco/gomdlint/internal/jstext"
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// DiagnosticLayout controls the main line of formatted diagnostics: the form
// of the rule identifier, and the display widths that the location,
// severity and rule columns are padded to. Diagnostics formatted with one
// layout line up their rule identifiers and messages. The zero value names
// rules by name and pads nothing.
type DiagnosticLayout struct {
	// RuleFormat is the form of the rule identifier.
	RuleFormat config.RuleFormat

	// LocationWidth, SeverityWidth and RuleWidth are the widths, in terminal
	// cells, that the location, severity and rule columns are padded to.
	// Wider text is never cut.
	LocationWidth int
	SeverityWidth int
	RuleWidth     int
}

// NewDiagnosticLayout returns the layout that lines up diagnostics, naming
// their rules in ruleFormat: each column is as wide as its widest entry.
func NewDiagnosticLayout(diagnostics []lint.Diagnostic, ruleFormat config.RuleFormat) DiagnosticLayout {
	layout := DiagnosticLayout{RuleFormat: ruleFormat}
	for idx := range diagnostics {
		diag := &diagnostics[idx]
		layout.LocationWidth = max(layout.LocationWidth, displaywidth.String(diag.FilePath+diagnosticPosition(diag)))
		layout.SeverityWidth = max(layout.SeverityWidth, displaywidth.String(string(diag.Severity)))
		layout.RuleWidth = max(layout.RuleWidth,
			displaywidth.String(config.FormatRuleID(ruleFormat, diag.RuleID, diag.RuleName)))
	}
	return layout
}

// FormatDiagnostic formats a single diagnostic for terminal output, naming
// its rule by name and padding no column.
func (s *Styles) FormatDiagnostic(diag *lint.Diagnostic, showContext bool, sourceLine string) string {
	return s.FormatDiagnosticWithLayout(diag, showContext, sourceLine, DiagnosticLayout{RuleFormat: config.RuleFormatName})
}

// FormatDiagnosticWithLayout formats a diagnostic for terminal output. Its
// main line holds the location, which terminals and editors open as
// path:line:col, then the severity, the rule identifier and the message,
// with the first three padded to the widths of layout. The source line with
// a caret under the column and the suggestion follow, when there are any.
func (s *Styles) FormatDiagnosticWithLayout(diag *lint.Diagnostic, showContext bool, sourceLine string, layout DiagnosticLayout) string {
	var builder strings.Builder

	position := diagnosticPosition(diag)
	severity := string(diag.Severity) // The text FormatSeverity styles.
	rule := config.FormatRuleID(layout.RuleFormat, diag.RuleID, diag.RuleName)

	// Columns are padded by the width of their plain text, outside the
	// styles, whose escape sequences take no cells.
	line := "  " + s.FilePath.Render(diag.FilePath) + position + fillTo(diag.FilePath+position, layout.LocationWidth) +
		"  " + s.FormatSeverity(diag.Severity) + fillTo(severity, layout.SeverityWidth) +
		"  " + s.RuleID.Render(rule)
	if diag.Message != "" {
		line += fillTo(rule, layout.RuleWidth) + "  " + s.Message.Render(diag.Message)
	}
	builder.WriteString(line + "\n")

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

// diagnosticPosition returns the ":line:col" that follows the path in the
// location of diag.
func diagnosticPosition(diag *lint.Diagnostic) string {
	return fmt.Sprintf(":%d:%d", diag.StartLine, diag.StartColumn)
}

// fillTo returns the spaces that widen text to width terminal cells.
func fillTo(text string, width int) string {
	return strings.Repeat(" ", max(width-displaywidth.String(text), 0))
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
// column, a 1-based column counted in UTF-16 code units as reported columns
// are. The line is printed as markdownlint decodes it, with each ill-formed
// UTF-8 sequence as U+FFFD, and with its tabs expanded. The caret is padded
// by the display width of the text before the character that holds the
// column, so it lines up after wide characters, characters outside the Basic
// Multilingual Plane, ill-formed UTF-8 and tabs, and marks a character such
// as an emoji when the column falls in its middle.
func (s *Styles) FormatSourceContext(line string, column int) string {
	var builder strings.Builder

	// Indent to align with diagnostic output
	const indent = "        "

	// Decode as markdownlint does, so ill-formed UTF-8 prints, measures and
	// counts as the U+FFFD characters the column counted.
	line = jstext.Decode(line)
	split := jstext.ByteOffset(line, column-1)
	before, caretOffset := expandTabs(line[:split], 0)
	after, _ := expandTabs(line[split:], caretOffset)

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
		header += s.Dim.Render(" (" + Count(issueCount, "issue", "issues") + ")")
	}
	return header
}
