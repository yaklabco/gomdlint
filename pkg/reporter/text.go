package reporter

import (
	"context"
	"fmt"
	"io"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TextReporter formats results as styled terminal output.
type TextReporter struct {
	opts   Options
	styles *pretty.Styles
	out    io.Writer
}

// NewTextReporter creates a new text reporter.
func NewTextReporter(opts Options) *TextReporter {
	colorEnabled := pretty.IsColorEnabled(opts.Color, opts.Writer)
	return &TextReporter{
		opts:   opts,
		styles: pretty.NewStyles(colorEnabled),
		out:    opts.Writer,
	}
}

// Report implements Reporter.
func (r *TextReporter) Report(ctx context.Context, result *runner.Result) (int, error) {
	if result == nil || len(result.Files) == 0 {
		if r.opts.ShowSummary {
			fmt.Fprintln(r.out, r.styles.Success.Render("No files to check."))
		}
		return 0, nil
	}

	var totalIssues int

	if r.opts.GroupByFile {
		totalIssues = r.reportGrouped(ctx, result)
	} else {
		totalIssues = r.reportFlat(ctx, result)
	}

	if r.opts.ShowSummary {
		fmt.Fprint(r.out, r.styles.FormatSummaryOneLine(result.Stats))
	}

	return totalIssues, nil
}

// reportGrouped writes diagnostics grouped by file.
func (r *TextReporter) reportGrouped(_ context.Context, result *runner.Result) int {
	var total int

	for _, file := range result.Files {
		// Handle file errors
		if file.Error != nil {
			fmt.Fprintf(r.out, "%s: %s\n",
				r.styles.FilePath.Render(file.Path),
				r.styles.Error.Render(fmt.Sprintf("error: %v", file.Error)),
			)
			continue
		}

		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}

		diagnostics := file.Result.Diagnostics
		if len(diagnostics) == 0 {
			continue
		}

		// File header
		fmt.Fprintln(r.out, r.styles.FormatFileHeader(file.Path, len(diagnostics)))

		for _, diag := range diagnostics {
			// Get source line for context if enabled
			var sourceLine string
			if r.opts.ShowContext && file.Result.Snapshot != nil {
				sourceLine = r.getSourceLine(file.Result.Snapshot.Content, diag.StartLine)
			}

			fmt.Fprint(r.out, r.styles.FormatDiagnosticWithFormat(&diag, r.opts.ShowContext, sourceLine, r.opts.RuleFormat))
			total++
		}

		// Blank line between files
		fmt.Fprintln(r.out)
	}

	return total
}

// reportFlat writes diagnostics without grouping.
func (r *TextReporter) reportFlat(_ context.Context, result *runner.Result) int {
	var total int

	for _, file := range result.Files {
		// Handle file errors
		if file.Error != nil {
			fmt.Fprintf(r.out, "%s: %s\n",
				r.styles.FilePath.Render(file.Path),
				r.styles.Error.Render(fmt.Sprintf("error: %v", file.Error)),
			)
			continue
		}

		if file.Result == nil || file.Result.FileResult == nil {
			continue
		}

		for _, diag := range file.Result.Diagnostics {
			// Get source line for context if enabled
			var sourceLine string
			if r.opts.ShowContext && file.Result.Snapshot != nil {
				sourceLine = r.getSourceLine(file.Result.Snapshot.Content, diag.StartLine)
			}

			fmt.Fprint(r.out, r.styles.FormatDiagnosticWithFormat(&diag, r.opts.ShowContext, sourceLine, r.opts.RuleFormat))
			total++
		}
	}

	return total
}

// getSourceLine extracts a specific line from content (1-based line number).
func (r *TextReporter) getSourceLine(content []byte, lineNum int) string {
	if lineNum < 1 || len(content) == 0 {
		return ""
	}

	lines := splitLines(content)
	if lineNum > len(lines) {
		return ""
	}

	return lines[lineNum-1]
}

// splitLines splits content into lines.
func splitLines(content []byte) []string {
	var lines []string
	var line []byte

	for _, b := range content {
		if b == '\n' {
			lines = append(lines, string(line))
			line = nil
		} else {
			line = append(line, b)
		}
	}

	// Handle last line without newline
	if len(line) > 0 {
		lines = append(lines, string(line))
	}

	return lines
}
