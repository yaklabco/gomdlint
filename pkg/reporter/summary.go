package reporter

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jamesainslie/gomdlint/internal/ui/pretty"
	"github.com/jamesainslie/gomdlint/pkg/analysis"
	"github.com/jamesainslie/gomdlint/pkg/config"
)

// Table layout constants for summary output.
// Both tables use the same width for visual consistency.
const (
	tableWidth         = 90 // Width of table separators (same for both tables).
	ruleColWidth       = 30 // Width of the rule name column.
	fileColWidth       = 60 // Width of the file path column (wider for relative paths).
	numColWidth        = 7  // Width of numeric columns.
	warnColWidth       = 8  // Width of warnings column.
	fixableColWidth    = 8  // Width of fixable column.
	maxRuleNameLength  = 28 // Maximum characters for rule name before truncation.
	maxFilePathLength  = 58 // Maximum characters for file path before truncation.
	totalPartsCapacity = 2  // Expected number of parts in total summary line.
)

// padRight pads a string to the given width with spaces on the right.
// This must be called BEFORE applying ANSI styles.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// padLeft pads a string to the given width with spaces on the left.
// This must be called BEFORE applying ANSI styles.
func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// SummaryRenderer formats results as aggregated summary tables.
type SummaryRenderer struct {
	opts   Options
	styles *pretty.Styles
	out    io.Writer
}

// NewSummaryRenderer creates a new summary renderer.
func NewSummaryRenderer(opts Options) *SummaryRenderer {
	colorEnabled := pretty.IsColorEnabled(opts.Color, opts.Writer)
	return &SummaryRenderer{
		opts:   opts,
		styles: pretty.NewStyles(colorEnabled),
		out:    opts.Writer,
	}
}

// Render implements Renderer.
func (r *SummaryRenderer) Render(_ context.Context, report *analysis.Report) error {
	if report.Totals.Issues == 0 {
		fmt.Fprintln(r.out, r.styles.Success.Render("No issues found"))
		return nil
	}

	// Determine order
	if r.opts.SummaryOrder == config.SummaryOrderFiles {
		r.renderFileTable(report.ByFile)
		fmt.Fprintln(r.out)
		r.renderRuleTable(report.ByRule)
	} else {
		r.renderRuleTable(report.ByRule)
		fmt.Fprintln(r.out)
		r.renderFileTable(report.ByFile)
	}

	fmt.Fprintln(r.out)
	r.renderTotals(report.Totals)

	return nil
}

func (r *SummaryRenderer) renderRuleTable(rules []analysis.RuleAnalysis) {
	if len(rules) == 0 {
		return
	}

	fmt.Fprintln(r.out, r.styles.Bold.Render("Rules Summary"))
	fmt.Fprintln(r.out, r.styles.TableSeparator.Render(strings.Repeat("─", tableWidth)))

	// Header - pad first, then style
	fmt.Fprintf(r.out, "%s %s %s %s %s\n",
		r.styles.TableHeader.Render(padRight("Rule", ruleColWidth)),
		r.styles.TableHeader.Render(padLeft("Count", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Errors", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Warnings", warnColWidth)),
		r.styles.TableHeader.Render(padLeft("Fixable", fixableColWidth)),
	)
	fmt.Fprintln(r.out, r.styles.TableSeparator.Render(strings.Repeat("─", tableWidth)))

	// Rows
	for _, rule := range rules {
		ruleName := rule.RuleName
		if ruleName == "" {
			ruleName = rule.RuleID
		}
		if len(ruleName) > maxRuleNameLength {
			ruleName = ruleName[:maxRuleNameLength] + "…"
		}

		// Pad first, then style
		paddedName := padRight(ruleName, ruleColWidth)
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

func (r *SummaryRenderer) renderFileTable(files []analysis.FileAnalysis) {
	if len(files) == 0 {
		return
	}

	fmt.Fprintln(r.out, r.styles.Bold.Render("Files Summary"))
	fmt.Fprintln(r.out, r.styles.TableSeparator.Render(strings.Repeat("─", tableWidth)))

	// Header - pad first, then style
	fmt.Fprintf(r.out, "%s %s %s %s\n",
		r.styles.TableHeader.Render(padRight("File", fileColWidth)),
		r.styles.TableHeader.Render(padLeft("Count", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Errors", numColWidth)),
		r.styles.TableHeader.Render(padLeft("Warnings", warnColWidth)),
	)
	fmt.Fprintln(r.out, r.styles.TableSeparator.Render(strings.Repeat("─", tableWidth)))

	// Rows
	for _, file := range files {
		path := file.Path
		if len(path) > maxFilePathLength {
			path = "…" + path[len(path)-(maxFilePathLength-1):]
		}

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
	issueWord := "issues"
	if totals.Issues == 1 {
		issueWord = "issue"
	}
	parts = append(parts, fmt.Sprintf("%d %s", totals.Issues, issueWord))

	// Severity breakdown
	var severityParts []string
	if totals.Errors > 0 {
		severityParts = append(severityParts, r.styles.Error.Render(fmt.Sprintf("%d errors", totals.Errors)))
	}
	if totals.Warnings > 0 {
		severityParts = append(severityParts, r.styles.Warning.Render(fmt.Sprintf("%d warnings", totals.Warnings)))
	}
	if len(severityParts) > 0 {
		parts[0] = fmt.Sprintf("%d %s (%s)", totals.Issues, issueWord, strings.Join(severityParts, ", "))
	}

	// Files with issues
	fileWord := "files"
	if totals.FilesWithIssues == 1 {
		fileWord = "file"
	}
	parts = append(parts, fmt.Sprintf("in %d %s", totals.FilesWithIssues, fileWord))

	fmt.Fprintln(r.out, r.styles.Bold.Render("Total: ")+strings.Join(parts, " "))
}
