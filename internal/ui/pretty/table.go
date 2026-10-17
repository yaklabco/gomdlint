package pretty

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/clipperhouse/displaywidth"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// Table formatting constants.
const (
	fixableSymbol      = "+"
	tablePadding       = 2
	tableColumnCount   = 6 // SEVERITY, FILE, LOC, MESSAGE, RULE, FIXABLE
	perFileColumnCount = 5 // SEVERITY, LOC, MESSAGE, RULE, FIXABLE (no FILE column)
	fixableColumnWidth = 3 // width for fixable indicator column
	markerColumnWidth  = 1 // width of the severity marker column (see severityMarker)
	minFileWidth       = 20
	minLocWidth        = 10
	minMessageWidth    = 35
	minRuleWidth       = 8
	heavySeparator     = "="
	lightSeparator     = "-"
	defaultTermWidth   = 100
	truncationMarker   = "..." // Ends text cut to fit its column.

	// A table wider than the terminal narrows MESSAGE and FILE past their
	// minimum widths above, down to these floors, before it cuts rule
	// identifiers (see fitWidths).
	fitMessageWidth    = 20
	fitMinMessageWidth = 10
	fitMinFileWidth    = 10
)

// Column headers.
const (
	fileHeader    = "FILE"
	locHeader     = "LOC"
	messageHeader = "MESSAGE"
	ruleHeader    = "RULE"
)

// TableRow represents a single row in the diagnostic table.
type TableRow struct {
	File     string
	Location string
	Message  string
	Rule     string // The rule identifier, in the table's rule format.
	Severity config.Severity
	Fixable  bool
}

// TableFormatter formats diagnostics as a styled table. Its tables show the
// same text with and without color: color only adds escape sequences.
type TableFormatter struct {
	styles     *Styles
	termWidth  int
	ruleFormat config.RuleFormat
}

// NewTableFormatter creates a new table formatter whose RULE column names
// rules in ruleFormat.
func NewTableFormatter(styles *Styles, termWidth int, ruleFormat config.RuleFormat) *TableFormatter {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	return &TableFormatter{
		styles:     styles,
		termWidth:  termWidth,
		ruleFormat: ruleFormat,
	}
}

// FormatTable formats runner results as a styled table.
func (t *TableFormatter) FormatTable(result *runner.Result) string {
	if result == nil || len(result.Files) == 0 {
		return ""
	}

	// Collect all rows grouped by file
	fileGroups := t.collectRows(result)
	if len(fileGroups) == 0 {
		return ""
	}

	// Calculate column widths
	colWidths := t.fitWidths(fileGroups, true)

	var builder strings.Builder

	// Write header
	builder.WriteString(t.formatHeader(colWidths))
	builder.WriteString("\n")
	builder.WriteString(t.formatSeparator(colWidths, heavySeparator))
	builder.WriteString("\n")

	// Write rows grouped by file
	isFirstGroup := true
	for _, group := range fileGroups {
		if !isFirstGroup {
			builder.WriteString(t.formatSeparator(colWidths, lightSeparator))
			builder.WriteString("\n")
		}
		isFirstGroup = false

		for _, row := range group {
			builder.WriteString(t.formatRow(row, colWidths))
			builder.WriteString("\n")
		}
	}

	// Write footer separator
	builder.WriteString(t.formatSeparator(colWidths, heavySeparator))
	builder.WriteString("\n")

	// Write legend
	builder.WriteString(t.legend(fileGroups))
	builder.WriteString("\n")

	return builder.String()
}

// FormatFileTitle formats the path that heads a per-file table, shortened
// from its start to the terminal width.
func (t *TableFormatter) FormatFileTitle(path string) string {
	return t.styles.Bold.Render(TruncateStart(cellText(path), t.termWidth, truncationMarker))
}

// FormatFileTable formats a single file's diagnostics as a standalone table.
func (t *TableFormatter) FormatFileTable(file runner.FileOutcome) string {
	if file.Result == nil || file.Result.FileResult == nil {
		return ""
	}

	diagnostics := file.Result.Diagnostics
	if len(diagnostics) == 0 {
		return ""
	}

	// Collect rows for this file only
	rows := make([]TableRow, 0, len(diagnostics))
	for idx := range diagnostics {
		rows = append(rows, DiagnosticToTableRow(file.Path, &diagnostics[idx], t.ruleFormat))
	}

	// Calculate column widths for this file (without FILE column since it's shown in header)
	colWidths := t.fitWidths([][]TableRow{rows}, false)

	var builder strings.Builder

	// Write header (simplified for per-file view - no FILE column needed)
	builder.WriteString(t.formatPerFileHeader(colWidths))
	builder.WriteString("\n")
	builder.WriteString(t.formatPerFileSeparator(colWidths, heavySeparator))
	builder.WriteString("\n")

	// Write rows
	for _, row := range rows {
		builder.WriteString(t.formatPerFileRow(row, colWidths))
		builder.WriteString("\n")
	}

	// Write footer separator
	builder.WriteString(t.formatPerFileSeparator(colWidths, heavySeparator))
	builder.WriteString("\n")

	// Write summary for this file
	builder.WriteString(t.formatFileSummary(rows))
	builder.WriteString("\n")

	return builder.String()
}

// formatPerFileHeader formats the header for per-file tables.
func (t *TableFormatter) formatPerFileHeader(widths columnWidths) string {
	header := markerHeader + fitCell(locHeader, widths.loc) +
		"  " + fitCell(messageHeader, widths.message) +
		"  " + fitCell(ruleHeader, widths.rule) + "   "
	return t.styles.TableHeader.Render(header)
}

// formatPerFileSeparator formats a separator line for per-file tables.
func (t *TableFormatter) formatPerFileSeparator(widths columnWidths, char string) string {
	sep := strings.Repeat(char, separatorWidth(widths, false))
	return t.styles.TableSeparator.Render(sep)
}

// formatPerFileRow formats a single row in the per-file table.
func (t *TableFormatter) formatPerFileRow(row TableRow, widths columnWidths) string {
	fixable := " "
	if row.Fixable {
		fixable = t.styles.TableFixable.Render(fixableSymbol)
	}

	content := markerCell(row.Severity) + fitCell(row.Location, widths.loc) +
		"  " + fitCell(row.Message, widths.message) +
		"  " + fitCell(row.Rule, widths.rule) +
		"  " + fixable

	rowStyle := t.getRowStyle(row.Severity)
	return rowStyle.Render(content)
}

// markerHeader heads the severity marker column, which has no title.
const markerHeader = "    "

// markerCell returns the start of a row of severity: its marker, between
// the table's left margin and the padding before the next column. It is as
// wide as markerHeader.
func markerCell(severity config.Severity) string {
	return " " + severityMarker(severity) + "  "
}

// formatFileSummary formats a summary line for a single file.
func (t *TableFormatter) formatFileSummary(rows []TableRow) string {
	var errors, warnings, infos, fixable int

	for _, row := range rows {
		switch row.Severity {
		case config.SeverityError:
			errors++
		case config.SeverityWarning:
			warnings++
		case config.SeverityInfo:
			infos++
		}
		if row.Fixable {
			fixable++
		}
	}

	var parts []string
	if errors > 0 {
		parts = append(parts, t.styles.Error.Render(fmt.Sprintf("%d errors", errors)))
	}
	if warnings > 0 {
		parts = append(parts, t.styles.Warning.Render(fmt.Sprintf("%d warnings", warnings)))
	}
	if infos > 0 {
		parts = append(parts, t.styles.Info.Render(fmt.Sprintf("%d info", infos)))
	}
	if fixable > 0 {
		parts = append(parts, t.styles.TableFixable.Render(fmt.Sprintf("%d fixable", fixable)))
	}

	return " " + strings.Join(parts, " | ")
}

// collectRows collects diagnostic rows grouped by file.
func (t *TableFormatter) collectRows(result *runner.Result) [][]TableRow {
	var groups [][]TableRow

	for _, file := range result.Files {
		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}

		diagnostics := file.Result.Diagnostics
		if len(diagnostics) == 0 {
			continue
		}

		rows := make([]TableRow, 0, len(diagnostics))
		for idx := range diagnostics {
			rows = append(rows, DiagnosticToTableRow(file.Path, &diagnostics[idx], t.ruleFormat))
		}

		if len(rows) > 0 {
			groups = append(groups, rows)
		}
	}

	return groups
}

// columnWidths holds the widths of the table's columns, in terminal cells.
// Per-file tables have no FILE column.
type columnWidths struct {
	file    int
	loc     int
	message int
	rule    int
}

// fitWidths returns the column widths of a table of the rows in groups,
// with a FILE column if withFile. Each column is as wide as its widest cell
// and at least its minimum width. When that is wider than the terminal, the
// columns narrow in this order until the table fits: each column down to
// its widest cell, MESSAGE and FILE down to their minimum widths, then
// further down to their floors, and RULE last, so only a narrow terminal
// cuts rule identifiers. A terminal narrower than every floor leaves the
// table wider than itself.
func (t *TableFormatter) fitWidths(groups [][]TableRow, withFile bool) columnWidths {
	content := columnWidths{
		file:    displaywidth.String(fileHeader),
		loc:     displaywidth.String(locHeader),
		message: displaywidth.String(messageHeader),
		rule:    displaywidth.String(ruleHeader),
	}
	for _, group := range groups {
		for idx := range group {
			row := &group[idx]
			content.file = max(content.file, displaywidth.String(row.File))
			content.loc = max(content.loc, displaywidth.String(row.Location))
			content.message = max(content.message, displaywidth.String(row.Message))
			content.rule = max(content.rule, displaywidth.String(row.Rule))
		}
	}
	if !withFile {
		content.file = 0
	}

	widths := columnWidths{
		loc:     max(minLocWidth, content.loc),
		message: max(minMessageWidth, content.message),
		rule:    max(minRuleWidth, content.rule),
	}
	if withFile {
		widths.file = max(minFileWidth, content.file)
	}

	shrinkToFit(t.termWidth, func() int { return separatorWidth(widths, withFile) },
		shrinkStep{&widths.loc, content.loc},
		shrinkStep{&widths.file, content.file},
		shrinkStep{&widths.message, content.message},
		shrinkStep{&widths.rule, content.rule},
		shrinkStep{&widths.message, minMessageWidth},
		shrinkStep{&widths.file, minFileWidth},
		shrinkStep{&widths.message, fitMessageWidth},
		shrinkStep{&widths.file, fitMinFileWidth},
		shrinkStep{&widths.message, fitMinMessageWidth},
		shrinkStep{&widths.rule, minRuleWidth},
	)

	return widths
}

// shrinkStep narrows one column, at most down to floor.
type shrinkStep struct {
	width *int
	floor int
}

// shrinkToFit takes steps in order, each narrowing its column by as much
// as the table is wider than limit, until total, the width of the table,
// is at most limit or the steps run out.
func shrinkToFit(limit int, total func() int, steps ...shrinkStep) {
	for _, step := range steps {
		excess := total() - limit
		if excess <= 0 {
			return
		}
		if *step.width > step.floor {
			*step.width -= min(excess, *step.width-step.floor)
		}
	}
}

// separatorWidth returns the width of the separators of a table with
// columns of widths, its widest lines.
func separatorWidth(widths columnWidths, withFile bool) int {
	total := markerColumnWidth + widths.loc + widths.message + widths.rule + fixableColumnWidth
	if withFile {
		return total + widths.file + tablePadding*tableColumnCount
	}
	return total + tablePadding*perFileColumnCount
}

// fitCell cuts text to width terminal cells at its end and pads it with
// spaces to width.
func fitCell(text string, width int) string {
	text = TruncateEnd(text, width, truncationMarker)
	return text + fillTo(text, width)
}

// fitPathCell cuts a path to width terminal cells at its start, so the
// file name stays visible, and pads it with spaces to width.
func fitPathCell(path string, width int) string {
	path = TruncateStart(path, width, truncationMarker)
	return path + fillTo(path, width)
}

// formatHeader formats the table header row.
func (t *TableFormatter) formatHeader(widths columnWidths) string {
	header := markerHeader + fitCell(fileHeader, widths.file) +
		"  " + fitCell(locHeader, widths.loc) +
		"  " + fitCell(messageHeader, widths.message) +
		"  " + fitCell(ruleHeader, widths.rule) + "   "
	return t.styles.TableHeader.Render(header)
}

// formatSeparator formats a separator line.
func (t *TableFormatter) formatSeparator(widths columnWidths, char string) string {
	sep := strings.Repeat(char, separatorWidth(widths, true))
	return t.styles.TableSeparator.Render(sep)
}

// formatRow formats a single table row with severity-based styling.
func (t *TableFormatter) formatRow(row TableRow, widths columnWidths) string {
	// Build the row content
	fixable := " "
	if row.Fixable {
		fixable = t.styles.TableFixable.Render(fixableSymbol)
	}

	content := markerCell(row.Severity) + fitPathCell(row.File, widths.file) +
		"  " + fitCell(row.Location, widths.loc) +
		"  " + fitCell(row.Message, widths.message) +
		"  " + fitCell(row.Rule, widths.rule) +
		"  " + fixable

	// Apply row background based on severity
	rowStyle := t.getRowStyle(row.Severity)
	return rowStyle.Render(content)
}

// getRowStyle returns the appropriate style for a severity level.
func (t *TableFormatter) getRowStyle(severity config.Severity) lipgloss.Style {
	switch severity {
	case config.SeverityError:
		return t.styles.TableErrorRow
	case config.SeverityWarning:
		return t.styles.TableWarnRow
	case config.SeverityInfo:
		return t.styles.TableInfoRow
	default:
		return lipgloss.NewStyle()
	}
}

// FormatLegend formats the legend of the per-file tables of result, the
// legend that ends its combined table (see legend). It returns nothing for
// a result without findings.
func (t *TableFormatter) FormatLegend(result *runner.Result) string {
	if result == nil {
		return ""
	}
	legend := t.legend(t.collectRows(result))
	if legend == "" {
		return ""
	}
	return legend + "\n"
}

// legend formats the legend of a table of the rows in groups. It explains
// exactly the markers the rows show: the severity markers of the
// severities they have, in the colors of their rows, and the fixable
// marker if a row is fixable. It is the same text with and without color.
// It returns nothing when there are no rows.
func (t *TableFormatter) legend(groups [][]TableRow) string {
	severities := make(map[config.Severity]bool)
	fixable := false
	for _, group := range groups {
		for _, row := range group {
			severities[row.Severity] = true
			fixable = fixable || row.Fixable
		}
	}

	var entries []string
	for _, severity := range []config.Severity{config.SeverityError, config.SeverityWarning, config.SeverityInfo} {
		if severities[severity] {
			entries = append(entries, t.getRowStyle(severity).Render(severityMarker(severity))+
				t.styles.TableLegend.Render(" = "+string(severity)))
		}
	}
	if fixable {
		entries = append(entries, t.styles.TableFixable.Render(fixableSymbol)+t.styles.TableLegend.Render(" = fixable"))
	}
	if len(entries) == 0 {
		return ""
	}

	return " " + t.styles.TableLegend.Render("Legend: ") + strings.Join(entries, t.styles.TableLegend.Render(" | "))
}

// severityMarker returns the marker that leads the rows of severity: E, W
// or I. A severity that is none of those gets a blank marker, which the
// legend does not list.
func severityMarker(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "E"
	case config.SeverityWarning:
		return "W"
	case config.SeverityInfo:
		return "I"
	default:
		return " "
	}
}

// FormatTableSummary formats a summary line for table output.
func (t *TableFormatter) FormatTableSummary(stats runner.Stats, duration string) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%d %s checked", stats.FilesProcessed, filesNoun(stats.FilesProcessed)))

	parts = append(parts, t.styles.formatFailures(stats)...)

	if stats.DiagnosticsBySeverity["error"] > 0 {
		errCount := t.styles.Error.Render(fmt.Sprintf("%d errors", stats.DiagnosticsBySeverity["error"]))
		parts = append(parts, errCount)
	}

	if stats.DiagnosticsBySeverity["warning"] > 0 {
		warnCount := t.styles.Warning.Render(fmt.Sprintf("%d warnings", stats.DiagnosticsBySeverity["warning"]))
		parts = append(parts, warnCount)
	}

	if stats.DiagnosticsBySeverity["info"] > 0 {
		infoCount := t.styles.Info.Render(fmt.Sprintf("%d info", stats.DiagnosticsBySeverity["info"]))
		parts = append(parts, infoCount)
	}

	// Count fixable issues
	fixableCount := countFixable(stats)
	if fixableCount > 0 {
		fixable := t.styles.TableFixable.Render(fmt.Sprintf("%d fixable", fixableCount))
		parts = append(parts, fixable)
	}

	if duration != "" {
		parts = append(parts, t.styles.Dim.Render(duration))
	}

	return " " + strings.Join(parts, " | ")
}

// countFixable returns the count of fixable issues from stats.
// This is a placeholder - actual implementation would need to track this in Stats.
func countFixable(_ runner.Stats) int {
	// TODO: Add FixableCount to runner.Stats
	return 0
}

// cellText makes text printable on one line of a table: invalid UTF-8
// becomes the replacement character, and control characters, such as tabs
// and newlines, become spaces. Tabs and newlines would otherwise widen or
// break a row, and invalid bytes would make the output invalid UTF-8.
func cellText(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, strings.ToValidUTF8(text, string(unicode.ReplacementChar)))
}

// DiagnosticToTableRow converts a lint diagnostic to a table row, naming its
// rule in ruleFormat. Each cell holds one line of valid UTF-8 (see cellText).
// A diagnostic without a severity is a warning, as run statistics count it.
func DiagnosticToTableRow(path string, diag *lint.Diagnostic, ruleFormat config.RuleFormat) TableRow {
	severity := diag.Severity
	if severity == "" {
		severity = config.SeverityWarning
	}
	return TableRow{
		File:     cellText(path),
		Location: fmt.Sprintf("%d:%d", diag.StartLine, diag.StartColumn),
		Message:  cellText(diag.Message),
		Rule:     cellText(config.FormatRuleID(ruleFormat, diag.RuleID, diag.RuleName)),
		Severity: severity,
		Fixable:  len(diag.FixEdits) > 0,
	}
}
