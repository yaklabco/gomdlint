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

	shown := displayResult(result, r.opts.WorkingDir)

	var totalIssues int

	if r.opts.GroupByFile {
		totalIssues = r.reportGrouped(ctx, shown)
	} else {
		totalIssues = r.reportFlat(ctx, shown)
	}

	if r.opts.ShowSummary {
		fmt.Fprint(r.out, r.styles.FormatSummaryOneLine(shown.Stats))
	}

	return totalIssues, nil
}

// reportGrouped writes diagnostics grouped by file.
// The result must already be in display form (see displayResult).
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

		// The diagnostics of a file line up their rules and messages.
		layout := pretty.NewDiagnosticLayout(diagnostics, r.opts.RuleFormat)
		for _, diag := range diagnostics {
			// Get source line for context if enabled
			var contextLine string
			if r.opts.ShowContext {
				contextLine = sourceLine(file.Result.Snapshot, diag.StartLine)
			}

			fmt.Fprint(r.out, r.styles.FormatDiagnosticWithLayout(&diag, r.opts.ShowContext, contextLine, layout))
			total++
		}

		// Warn if fix loop was exhausted
		if file.Exhausted {
			fmt.Fprintf(r.out, "  %s\n",
				r.styles.Warning.Render(fmt.Sprintf("warning: fix loop exhausted after %d passes with %d edits remaining",
					file.FixPasses, file.RemainingEdits)))
		}

		// Blank line between files
		fmt.Fprintln(r.out)
	}

	return total
}

// reportFlat writes diagnostics without grouping.
// The result must already be in display form (see displayResult).
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

		// The diagnostics of a file line up their rules and messages.
		layout := pretty.NewDiagnosticLayout(file.Result.Diagnostics, r.opts.RuleFormat)
		for _, diag := range file.Result.Diagnostics {
			// Get source line for context if enabled
			var contextLine string
			if r.opts.ShowContext {
				contextLine = sourceLine(file.Result.Snapshot, diag.StartLine)
			}

			fmt.Fprint(r.out, r.styles.FormatDiagnosticWithLayout(&diag, r.opts.ShowContext, contextLine, layout))
			total++
		}

		// Warn if fix loop was exhausted
		if file.Exhausted {
			fmt.Fprintf(r.out, "%s: %s\n",
				r.styles.FilePath.Render(file.Path),
				r.styles.Warning.Render(fmt.Sprintf("warning: fix loop exhausted after %d passes with %d edits remaining",
					file.FixPasses, file.RemainingEdits)))
		}
	}

	return total
}
