package reporter

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"golang.org/x/term"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// defaultTermWidth is used when terminal width cannot be determined.
const defaultTermWidth = 100

// overallRuleWidth is the width of the rule above the overall summary of
// per-file tables, on a terminal at least that wide.
const overallRuleWidth = 80

// TableReporter formats results as a styled table with color-coded rows.
type TableReporter struct {
	opts      Options
	styles    *pretty.Styles
	formatter *pretty.TableFormatter
	out       io.Writer
	termWidth int
}

// NewTableReporter creates a new table reporter.
func NewTableReporter(opts Options) *TableReporter {
	styles := pretty.NewStylesFor(opts.Color, opts.Writer)

	// Try to get terminal width
	termWidth := getTerminalWidth(opts.Writer)

	return &TableReporter{
		opts:      opts,
		styles:    styles,
		formatter: pretty.NewTableFormatter(styles, termWidth, opts.RuleFormat),
		out:       opts.Writer,
		termWidth: termWidth,
	}
}

// Report implements Reporter.
func (r *TableReporter) Report(_ context.Context, result *runner.Result) (int, error) {
	if result == nil || len(result.Files) == 0 {
		if r.opts.ShowSummary {
			fmt.Fprintln(r.out, r.styles.Success.Render("No files to check."))
		}
		return 0, nil
	}

	// Count total issues
	totalIssues := countTotalIssues(result)

	if totalIssues == 0 {
		if r.opts.ShowSummary {
			fmt.Fprintln(r.out)
			if hasFailures(result.Stats) {
				// No findings, but the run was incomplete: no table, only
				// the summary that names the failures.
				fmt.Fprintln(r.out, r.formatter.FormatTableSummary(result.Stats, ""))
			} else {
				fmt.Fprintln(r.out, r.styles.Success.Render("All files passed!"))
				fmt.Fprintln(r.out, r.styles.Dim.Render(
					fmt.Sprintf("%d files checked", result.Stats.FilesProcessed),
				))
			}
		}
		return 0, nil
	}

	shown := displayResult(result, r.opts.WorkingDir)

	// Use per-file or combined output based on option
	if r.opts.PerFile {
		r.reportPerFile(shown)
	} else {
		r.reportCombined(shown)
	}

	return totalIssues, nil
}

// reportCombined outputs all files in a single table.
// The result must already be in display form (see displayResult).
func (r *TableReporter) reportCombined(result *runner.Result) {
	// Format and print the table
	table := r.formatter.FormatTable(result)
	fmt.Fprint(r.out, table)

	// Print summary
	if r.opts.ShowSummary {
		summary := r.formatter.FormatTableSummary(result.Stats, "")
		fmt.Fprintln(r.out, summary)
		fmt.Fprintln(r.out)

		// Add actionable hint for fixable issues
		if hasFixableIssues(result) {
			fmt.Fprintln(r.out, r.styles.Dim.Render("Run with --fix to auto-repair fixable issues"))
		}
	}
}

// reportPerFile outputs a separate table for each file with issues.
// The result must already be in display form (see displayResult).
func (r *TableReporter) reportPerFile(result *runner.Result) {
	filesWithIssues := 0

	for _, file := range result.Files {
		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}

		diagnostics := file.Result.Diagnostics
		if len(diagnostics) == 0 {
			continue
		}

		filesWithIssues++

		// Print file header
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, r.formatter.FormatFileTitle(file.Path))

		// Format and print this file's table
		table := r.formatter.FormatFileTable(file)
		fmt.Fprint(r.out, table)
	}

	// One legend explains the markers of every table, as the legend of a
	// combined table does.
	if legend := r.formatter.FormatLegend(result); legend != "" {
		fmt.Fprintln(r.out)
		fmt.Fprint(r.out, legend)
	}

	// Print overall summary
	if r.opts.ShowSummary && filesWithIssues > 0 {
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, r.styles.TableSeparator.Render(strings.Repeat("═", min(overallRuleWidth, r.termWidth))))
		fmt.Fprintln(r.out, r.styles.Bold.Render("Overall Summary"))
		summary := r.formatter.FormatTableSummary(result.Stats, "")
		fmt.Fprintln(r.out, summary)

		// Add actionable hint for fixable issues
		if hasFixableIssues(result) {
			fmt.Fprintln(r.out)
			fmt.Fprintln(r.out, r.styles.Dim.Render("Run with --fix to auto-repair fixable issues"))
		}
	}
}

// hasFailures reports whether the run could not lint every file
// completely: a file could not be linted, or a rule failed.
func hasFailures(stats runner.Stats) bool {
	return stats.FilesErrored > 0 || stats.RuleErrors > 0
}

// countTotalIssues counts all diagnostics in the result.
func countTotalIssues(result *runner.Result) int {
	var total int
	for _, file := range result.Files {
		if file.Result != nil && file.Result.FileResult != nil {
			total += len(file.Result.Diagnostics)
		}
	}
	return total
}

// hasFixableIssues checks if any diagnostics have fixes available.
func hasFixableIssues(result *runner.Result) bool {
	for _, file := range result.Files {
		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}
		for _, diag := range file.Result.Diagnostics {
			if len(diag.FixEdits) > 0 {
				return true
			}
		}
	}
	return false
}

// getTerminalWidth attempts to get the terminal width from the writer.
func getTerminalWidth(writer io.Writer) int {
	if f, ok := writer.(interface{ Fd() uintptr }); ok {
		descriptor := f.Fd()
		if descriptor > math.MaxInt {
			return defaultTermWidth
		}
		width, _, err := term.GetSize(int(descriptor))
		if err == nil && width > 0 {
			return width
		}
	}
	return defaultTermWidth
}
