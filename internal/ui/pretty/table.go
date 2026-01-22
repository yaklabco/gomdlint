package pretty

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// Table formatting constants.
const (
	fixableSymbol      = "+"
	tablePadding       = 2
	tableColumnCount   = 5 // FILE, LOC, MESSAGE, RULE, FIXABLE
	perFileColumnCount = 4 // LOC, MESSAGE, RULE, FIXABLE (no FILE column)
	fixableColumnWidth = 3 // width for fixable indicator column
	minFileWidth       = 20
	minLocWidth        = 10
	minMessageWidth    = 35
	minRuleWidth       = 8
	heavySeparator     = "="
	lightSeparator     = "-"
	defaultTermWidth   = 100
)

// TableRow represents a single row in the diagnostic table.
type TableRow struct {
	File     string
	Location string
	Message  string
	RuleID   string
	Severity config.Severity
	Fixable  bool
}

// TableFormatter formats diagnostics as a styled table.
type TableFormatter struct {
	styles       *Styles
	colorEnabled bool
	termWidth    int
}

// NewTableFormatter creates a new table formatter.
func NewTableFormatter(styles *Styles, colorEnabled bool, termWidth int) *TableFormatter {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	return &TableFormatter{
		styles:       styles,
		colorEnabled: colorEnabled,
		termWidth:    termWidth,
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
	colWidths := t.calculateColumnWidths(fileGroups)

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
	builder.WriteString(t.formatLegend())
	builder.WriteString("\n")

	return builder.String()
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
	for _, diag := range diagnostics {
		rows = append(rows, TableRow{
			File:     file.Path,
			Location: fmt.Sprintf("%d:%d", diag.StartLine, diag.StartColumn),
			Message:  diag.Message,
			RuleID:   diag.RuleID,
			Severity: diag.Severity,
			Fixable:  len(diag.FixEdits) > 0,
		})
	}

	// Calculate column widths for this file (without FILE column since it's shown in header)
	colWidths := t.calculateColumnWidthsForRows(rows)

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

// calculateColumnWidthsForRows calculates widths for per-file table (no FILE column).
func (t *TableFormatter) calculateColumnWidthsForRows(rows []TableRow) perFileColumnWidths {
	widths := perFileColumnWidths{
		loc:     minLocWidth,
		message: minMessageWidth,
		rule:    minRuleWidth,
	}

	for _, row := range rows {
		if len(row.Location) > widths.loc {
			widths.loc = len(row.Location)
		}
		if len(row.Message) > widths.message {
			widths.message = len(row.Message)
		}
		if len(row.RuleID) > widths.rule {
			widths.rule = len(row.RuleID)
		}
	}

	// Constrain to terminal width (allowing more space for message without FILE column)
	totalWidth := widths.loc + widths.message + widths.rule + (tablePadding * perFileColumnCount) + fixableColumnWidth
	if totalWidth > t.termWidth {
		excess := totalWidth - t.termWidth
		widths.message = max(minMessageWidth, widths.message-excess)
	}

	return widths
}

type perFileColumnWidths struct {
	loc     int
	message int
	rule    int
}

// formatPerFileHeader formats the header for per-file tables.
func (t *TableFormatter) formatPerFileHeader(widths perFileColumnWidths) string {
	header := fmt.Sprintf(" %-*s  %-*s  %-*s   ",
		widths.loc, "LOC",
		widths.message, "MESSAGE",
		widths.rule, "RULE",
	)
	return t.styles.TableHeader.Render(header)
}

// formatPerFileSeparator formats a separator line for per-file tables.
func (t *TableFormatter) formatPerFileSeparator(widths perFileColumnWidths, char string) string {
	totalWidth := widths.loc + widths.message + widths.rule + (tablePadding * perFileColumnCount) + fixableColumnWidth
	sep := strings.Repeat(char, totalWidth)
	return t.styles.TableSeparator.Render(sep)
}

// formatPerFileRow formats a single row in the per-file table.
func (t *TableFormatter) formatPerFileRow(row TableRow, widths perFileColumnWidths) string {
	loc := truncateString(row.Location, widths.loc)
	message := truncateString(row.Message, widths.message)
	ruleID := truncateString(row.RuleID, widths.rule)

	fixable := " "
	if row.Fixable {
		fixable = t.styles.TableFixable.Render(fixableSymbol)
	}

	content := fmt.Sprintf(" %-*s  %-*s  %-*s  %s",
		widths.loc, loc,
		widths.message, message,
		widths.rule, ruleID,
		fixable,
	)

	rowStyle := t.getRowStyle(row.Severity)
	return rowStyle.Render(content)
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
		for _, diag := range diagnostics {
			rows = append(rows, TableRow{
				File:     file.Path,
				Location: fmt.Sprintf("%d:%d", diag.StartLine, diag.StartColumn),
				Message:  diag.Message,
				RuleID:   diag.RuleID,
				Severity: diag.Severity,
				Fixable:  len(diag.FixEdits) > 0,
			})
		}

		if len(rows) > 0 {
			groups = append(groups, rows)
		}
	}

	return groups
}

// calculateColumnWidths determines optimal column widths based on content.
func (t *TableFormatter) calculateColumnWidths(groups [][]TableRow) columnWidths {
	widths := columnWidths{
		file:    minFileWidth,
		loc:     minLocWidth,
		message: minMessageWidth,
		rule:    minRuleWidth,
	}

	// Scan all rows to find max widths
	for _, group := range groups {
		for _, row := range group {
			if len(row.File) > widths.file {
				widths.file = len(row.File)
			}
			if len(row.Location) > widths.loc {
				widths.loc = len(row.Location)
			}
			if len(row.Message) > widths.message {
				widths.message = len(row.Message)
			}
			if len(row.RuleID) > widths.rule {
				widths.rule = len(row.RuleID)
			}
		}
	}

	// Constrain to terminal width
	totalWidth := t.calculateTotalWidth(widths)
	if totalWidth > t.termWidth {
		// Reduce message width first
		excess := totalWidth - t.termWidth
		widths.message = max(minMessageWidth, widths.message-excess)

		// If still too wide, reduce file width
		totalWidth = t.calculateTotalWidth(widths)
		if totalWidth > t.termWidth {
			excess = totalWidth - t.termWidth
			widths.file = max(minFileWidth, widths.file-excess)
		}
	}

	return widths
}

type columnWidths struct {
	file    int
	loc     int
	message int
	rule    int
}

// formatHeader formats the table header row.
func (t *TableFormatter) formatHeader(widths columnWidths) string {
	header := fmt.Sprintf(" %-*s  %-*s  %-*s  %-*s   ",
		widths.file, "FILE",
		widths.loc, "LOC",
		widths.message, "MESSAGE",
		widths.rule, "RULE",
	)
	return t.styles.TableHeader.Render(header)
}

// calculateTotalWidth calculates the total table width from column widths.
func (t *TableFormatter) calculateTotalWidth(widths columnWidths) int {
	return widths.file + widths.loc + widths.message + widths.rule +
		(tablePadding * tableColumnCount) + fixableColumnWidth
}

// formatSeparator formats a separator line.
func (t *TableFormatter) formatSeparator(widths columnWidths, char string) string {
	totalWidth := t.calculateTotalWidth(widths)
	sep := strings.Repeat(char, totalWidth)
	return t.styles.TableSeparator.Render(sep)
}

// formatRow formats a single table row with severity-based styling.
func (t *TableFormatter) formatRow(row TableRow, widths columnWidths) string {
	// Truncate fields if necessary - use special truncation for file paths
	file := truncateFilePath(row.File, widths.file)
	loc := truncateString(row.Location, widths.loc)
	message := truncateString(row.Message, widths.message)
	ruleID := truncateString(row.RuleID, widths.rule)

	// Build the row content
	fixable := " "
	if row.Fixable {
		fixable = t.styles.TableFixable.Render(fixableSymbol)
	}

	content := fmt.Sprintf(" %-*s  %-*s  %-*s  %-*s  %s",
		widths.file, file,
		widths.loc, loc,
		widths.message, message,
		widths.rule, ruleID,
		fixable,
	)

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

// formatLegend formats the legend explaining the table symbols and colors.
func (t *TableFormatter) formatLegend() string {
	if !t.colorEnabled {
		return t.styles.TableLegend.Render(
			fmt.Sprintf(" Legend: E = error | W = warning | %s = fixable", fixableSymbol),
		)
	}

	errorSample := t.styles.TableErrorRow.Render(" error ")
	warnSample := t.styles.TableWarnRow.Render(" warning ")
	fixableSample := t.styles.TableFixable.Render(fixableSymbol)

	return t.styles.TableLegend.Render(
		fmt.Sprintf(" Legend: %s = error  %s = warning  %s = fixable",
			errorSample, warnSample, fixableSample),
	)
}

// FormatTableSummary formats a summary line for table output.
func (t *TableFormatter) FormatTableSummary(stats runner.Stats, duration string) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%d files checked", stats.FilesProcessed))

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

// truncateString truncates a string to maxLen, adding "..." if truncated.
func truncateString(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	if maxLen <= 3 {
		return str[:maxLen]
	}
	return str[:maxLen-3] + "..."
}

// truncateFilePath truncates a file path, preserving the end (filename) rather than beginning.
func truncateFilePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	if maxLen <= 3 {
		return path[len(path)-maxLen:]
	}
	return "..." + path[len(path)-maxLen+3:]
}

// DiagnosticToTableRow converts a lint diagnostic to a table row.
func DiagnosticToTableRow(path string, diag *lint.Diagnostic) TableRow {
	return TableRow{
		File:     path,
		Location: fmt.Sprintf("%d:%d", diag.StartLine, diag.StartColumn),
		Message:  diag.Message,
		RuleID:   diag.RuleID,
		Severity: diag.Severity,
		Fixable:  len(diag.FixEdits) > 0,
	}
}
