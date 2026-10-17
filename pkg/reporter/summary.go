package reporter

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/clipperhouse/displaywidth"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/analysis"
	"github.com/yaklabco/gomdlint/pkg/config"
)

// Table layout constants for summary output.
// Both tables use the same width for visual consistency.
const (
	tableWidth         = 90 // Width of table separators, unless the rules table is wider.
	fileColWidth       = 60 // Width of the file path column (wider for relative paths).
	numColWidth        = 7  // Width of numeric columns.
	warnColWidth       = 8  // Width of warnings column.
	fixableColWidth    = 8  // Width of fixable column.
	maxFilePathLength  = 58 // Maximum display width for file path before truncation.
	totalPartsCapacity = 2  // Expected number of parts in total summary line.
)

// ruleHeader heads the rule column of the rules table. The column is as wide
// as the header or the longest rule identifier, so no identifier is cut.
const ruleHeader = "Rule"

// ruleCountsWidth is the width of the rules table after its rule column: the
// count, errors, warnings and fixable columns, each after a space.
const ruleCountsWidth = 1 + numColWidth + 1 + numColWidth + 1 + warnColWidth + 1 + fixableColWidth

// ellipsis marks text shortened to fit a column.
const ellipsis = "…"

// padRight pads a string with spaces on the right to the given display width.
// This must be called BEFORE applying ANSI styles.
func padRight(s string, width int) string {
	shown := displaywidth.String(s)
	if shown >= width {
		return s
	}
	return s + strings.Repeat(" ", width-shown)
}

// padLeft pads a string with spaces on the left to the given display width.
// This must be called BEFORE applying ANSI styles.
func padLeft(s string, width int) string {
	shown := displaywidth.String(s)
	if shown >= width {
		return s
	}
	return strings.Repeat(" ", width-shown) + s
}

// SummaryRenderer formats results as aggregated summary tables.
type SummaryRenderer struct {
	opts   Options
	styles *pretty.Styles
	out    io.Writer
}

// NewSummaryRenderer creates a new summary renderer.
func NewSummaryRenderer(opts Options) *SummaryRenderer {
	return &SummaryRenderer{
		opts:   opts,
		styles: pretty.NewStylesFor(opts.Color, opts.Writer),
		out:    opts.Writer,
	}
}

// Render implements Renderer.
func (r *SummaryRenderer) Render(_ context.Context, report *analysis.Report) error {
	if report.Totals.Issues == 0 {
		failures := r.styles.FormatFailures(report.Totals.FilesErrored, report.Totals.RuleErrors)
		if len(failures) > 0 {
			// The findings of an incomplete run are no success.
			fmt.Fprintln(r.out, strings.Join(failures, ", ")+r.styles.Dim.Render(" (no issues found)"))
			return nil
		}
		fmt.Fprintln(r.out, r.styles.Success.Render("No issues found"))
		return nil
	}

	// Both tables share one separator width: tableWidth, or the width of the
	// rules table when its rule column needs more.
	column := r.newRuleColumn(report.ByRule)
	separator := r.styles.TableSeparator.Render(strings.Repeat("─", max(tableWidth, column.width+ruleCountsWidth)))

	// Determine order
	if r.opts.SummaryOrder == config.SummaryOrderFiles {
		r.renderFileTable(report.ByFile, separator)
		fmt.Fprintln(r.out)
		r.renderRuleTable(report.ByRule, column, separator)
	} else {
		r.renderRuleTable(report.ByRule, column, separator)
		fmt.Fprintln(r.out)
		r.renderFileTable(report.ByFile, separator)
	}

	fmt.Fprintln(r.out)
	r.renderTotals(report.Totals)

	return nil
}

// ruleColumn is the rule column of the rules table: the identifier of each
// rule, and the column width, which fits the header and every identifier.
type ruleColumn struct {
	identifiers []string
	width       int
}

// newRuleColumn names rules the way --rule-format asks.
func (r *SummaryRenderer) newRuleColumn(rules []analysis.RuleAnalysis) ruleColumn {
	column := ruleColumn{identifiers: make([]string, len(rules)), width: displaywidth.String(ruleHeader)}
	for idx := range rules {
		column.identifiers[idx] = config.FormatRuleID(r.opts.RuleFormat, rules[idx].RuleID, rules[idx].RuleName)
		column.width = max(column.width, displaywidth.String(column.identifiers[idx]))
	}
	return column
}

func (r *SummaryRenderer) renderRuleTable(rules []analysis.RuleAnalysis, column ruleColumn, separator string) {
	if len(rules) == 0 {
		return
	}

	fmt.Fprintln(r.out, r.styles.Bold.Render("Rules Summary"))
	fmt.Fprintln(r.out, separator)

	// Header - pad first, then style
	fmt.Fprintf(r.out, "%s %s %s %s %s\n",
		r.styles.TableHeader.Render(padRight(ruleHeader, column.width)),
		r.styles.TableHeader.Render(padLeft("Count", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Errors", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Warnings", warnColWidth)),
		r.styles.TableHeader.Render(padLeft("Fixable", fixableColWidth)),
	)
	fmt.Fprintln(r.out, separator)

	// Rows
	for idx, rule := range rules {
		// Pad first, then style
		paddedName := padRight(column.identifiers[idx], column.width)
		var styledName string
		switch {
		case rule.Errors > 0:
			styledName = r.styles.TableErrorRow.Render(paddedName)
		case rule.Warnings > 0:
			styledName = r.styles.TableWarnRow.Render(paddedName)
		default:
			styledName = paddedName
		}

		fixable := padLeft("", fixableColWidth)
		if rule.Fixable {
			fixable = r.styles.Success.Render(padLeft("✓", fixableColWidth))
		}

		fmt.Fprintf(r.out, "%s %s %s %s %s\n",
			styledName,
			padLeft(strconv.Itoa(rule.Issues), numColWidth),
			padLeft(strconv.Itoa(rule.Errors), numColWidth),
			padLeft(strconv.Itoa(rule.Warnings), warnColWidth),
			fixable,
		)
	}
}

func (r *SummaryRenderer) renderFileTable(files []analysis.FileAnalysis, separator string) {
	if len(files) == 0 {
		return
	}

	fmt.Fprintln(r.out, r.styles.Bold.Render("Files Summary"))
	fmt.Fprintln(r.out, separator)

	// Header - pad first, then style
	fmt.Fprintf(r.out, "%s %s %s %s\n",
		r.styles.TableHeader.Render(padRight("File", fileColWidth)),
		r.styles.TableHeader.Render(padLeft("Count", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Errors", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Warnings", warnColWidth)),
	)
	fmt.Fprintln(r.out, separator)

	// Rows
	for _, file := range files {
		path := pretty.TruncateStart(file.Path, maxFilePathLength, ellipsis)

		// Pad first, then style
		paddedPath := padRight(path, fileColWidth)
		var styledPath string
		switch {
		case file.Errors > 0:
			styledPath = r.styles.TableErrorRow.Render(paddedPath)
		case file.Warnings > 0:
			styledPath = r.styles.TableWarnRow.Render(paddedPath)
		default:
			styledPath = paddedPath
		}

		fmt.Fprintf(r.out, "%s %s %s %s\n",
			styledPath,
			padLeft(strconv.Itoa(file.Issues), numColWidth),
			padLeft(strconv.Itoa(file.Errors), numColWidth),
			padLeft(strconv.Itoa(file.Warnings), warnColWidth),
		)
	}
}

func (r *SummaryRenderer) renderTotals(totals analysis.Totals) {
	parts := make([]string, 0, totalPartsCapacity)

	// Total issues
	issues := pretty.Count(totals.Issues, "issue", "issues")
	parts = append(parts, issues)

	// Severity breakdown
	var severityParts []string
	if totals.Errors > 0 {
		severityParts = append(severityParts, r.styles.Error.Render(pretty.SeverityCount(totals.Errors, config.SeverityError)))
	}
	if totals.Warnings > 0 {
		severityParts = append(severityParts, r.styles.Warning.Render(pretty.SeverityCount(totals.Warnings, config.SeverityWarning)))
	}
	if totals.Infos > 0 {
		severityParts = append(severityParts, r.styles.Info.Render(pretty.SeverityCount(totals.Infos, config.SeverityInfo)))
	}
	if len(severityParts) > 0 {
		parts[0] = issues + " (" + strings.Join(severityParts, ", ") + ")"
	}

	// Files with issues
	parts = append(parts, "in "+pretty.Count(totals.FilesWithIssues, "file", "files"))

	line := strings.Join(parts, " ")
	if failures := r.styles.FormatFailures(totals.FilesErrored, totals.RuleErrors); len(failures) > 0 {
		line += ", " + strings.Join(failures, ", ")
	}
	fmt.Fprintln(r.out, r.styles.Bold.Render("Total: ")+line)
}
