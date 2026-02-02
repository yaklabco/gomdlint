package reporter

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"golang.org/x/term"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// defaultTermWidth is used when terminal width cannot be determined.
const defaultTermWidth = 100

// TableReporter formats results as a styled table with color-coded rows.
type TableReporter struct {
	opts      Options
	styles    *pretty.Styles
	formatter *pretty.TableFormatter
	bw        *bufio.Writer
}

// NewTableReporter creates a new table reporter.
func NewTableReporter(opts Options) *TableReporter {
	colorEnabled := pretty.IsColorEnabled(opts.Color, opts.Writer)
	styles := pretty.NewStyles(colorEnabled)

	// Try to get terminal width
	termWidth := getTerminalWidth(opts.Writer)

	return &TableReporter{
		opts:      opts,
		styles:    styles,
		formatter: pretty.NewTableFormatter(styles, colorEnabled, termWidth),
		bw:        bufio.NewWriterSize(opts.Writer, bufWriterSize),
	}
}

// Report implements Reporter.
func (r *TableReporter) Report(_ context.Context, result *runner.Result) (_ int, err error) {
	defer func() {
		if flushErr := r.bw.Flush(); err == nil {
			err = flushErr
		}
	}()

	if result == nil || len(result.Files) == 0 {
		if r.opts.ShowSummary {
			fmt.Fprintln(r.bw, r.styles.Success.Render("No files to check."))
		}
		return 0, nil
	}

	// Count total issues
	totalIssues := countTotalIssues(result)

	if totalIssues == 0 {
		if r.opts.ShowSummary {
			fmt.Fprintln(r.bw)
			fmt.Fprintln(r.bw, r.styles.Success.Render("All files passed!"))
			fmt.Fprintln(r.bw, r.styles.Dim.Render(
				fmt.Sprintf("%d files checked", result.Stats.FilesProcessed),
			))
		}
		return 0, nil
	}

	// Use per-file or combined output based on option
	if r.opts.PerFile {
		r.reportPerFile(result)
	} else {
		r.reportCombined(result)
	}

	return totalIssues, nil
}

// reportCombined outputs all files in a single table.
func (r *TableReporter) reportCombined(result *runner.Result) {
	// Format and print the table
	table := r.formatter.FormatTable(result)
	fmt.Fprint(r.bw, table)

	// Print summary
	if r.opts.ShowSummary {
		summary := r.formatter.FormatTableSummary(result.Stats, "")
		fmt.Fprintln(r.bw, summary)
		fmt.Fprintln(r.bw)

		// Add actionable hint for fixable issues
		if hasFixableIssues(result) {
			fmt.Fprintln(r.bw, r.styles.Dim.Render("Run with --fix to auto-repair fixable issues"))
		}
	}
}

// reportPerFile outputs a separate table for each file with issues.
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
		fmt.Fprintln(r.bw)
		fmt.Fprintln(r.bw, r.styles.Bold.Render(file.Path))

		// Format and print this file's table
		table := r.formatter.FormatFileTable(file)
		fmt.Fprint(r.bw, table)
	}

	// Print overall summary
	if r.opts.ShowSummary && filesWithIssues > 0 {
		fmt.Fprintln(r.bw)
		fmt.Fprintln(r.bw, r.styles.TableSeparator.Render("════════════════════════════════════════════════════════════════════════════════"))
		fmt.Fprintln(r.bw, r.styles.Bold.Render("Overall Summary"))
		summary := r.formatter.FormatTableSummary(result.Stats, "")
		fmt.Fprintln(r.bw, summary)

		// Add actionable hint for fixable issues
		if hasFixableIssues(result) {
			fmt.Fprintln(r.bw)
			fmt.Fprintln(r.bw, r.styles.Dim.Render("Run with --fix to auto-repair fixable issues"))
		}
	}
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
		width, _, err := term.GetSize(int(f.Fd()))
		if err == nil && width > 0 {
			return width
		}
	}
	return defaultTermWidth
}
