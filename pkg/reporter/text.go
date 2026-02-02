package reporter

import (
	"bufio"
	"context"
	"fmt"

	"github.com/yaklabco/gomdlint/internal/ui/pretty"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// TextReporter formats results as styled terminal output.
type TextReporter struct {
	opts   Options
	styles *pretty.Styles
	bw     *bufio.Writer
}

// NewTextReporter creates a new text reporter.
func NewTextReporter(opts Options) *TextReporter {
	colorEnabled := pretty.IsColorEnabled(opts.Color, opts.Writer)
	return &TextReporter{
		opts:   opts,
		styles: pretty.NewStyles(colorEnabled),
		bw:     bufio.NewWriterSize(opts.Writer, bufWriterSize),
	}
}

// Report implements Reporter.
func (r *TextReporter) Report(ctx context.Context, result *runner.Result) (_ int, err error) {
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

	var totalIssues int

	if r.opts.GroupByFile {
		totalIssues = r.reportGrouped(ctx, result)
	} else {
		totalIssues = r.reportFlat(ctx, result)
	}

	if r.opts.ShowSummary {
		fmt.Fprint(r.bw, r.styles.FormatSummaryOneLine(result.Stats))
	}

	return totalIssues, nil
}

// reportGrouped writes diagnostics grouped by file.
func (r *TextReporter) reportGrouped(_ context.Context, result *runner.Result) int {
	var total int

	for _, file := range result.Files {
		// Handle file errors
		if file.Error != nil {
			fmt.Fprintf(r.bw, "%s: %s\n",
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
		fmt.Fprintln(r.bw, r.styles.FormatFileHeader(file.Path, len(diagnostics)))

		for _, diag := range diagnostics {
			// Get source line for context if enabled
			var sourceLine string
			if r.opts.ShowContext && file.Result.Snapshot != nil {
				sourceLine = getSourceLine(file.Result.Snapshot, diag.StartLine)
			}

			fmt.Fprint(r.bw, r.styles.FormatDiagnosticWithFormat(&diag, r.opts.ShowContext, sourceLine, r.opts.RuleFormat))
			total++
		}

		// Blank line between files
		fmt.Fprintln(r.bw)
	}

	return total
}

// reportFlat writes diagnostics without grouping.
func (r *TextReporter) reportFlat(_ context.Context, result *runner.Result) int {
	var total int

	for _, file := range result.Files {
		// Handle file errors
		if file.Error != nil {
			fmt.Fprintf(r.bw, "%s: %s\n",
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
				sourceLine = getSourceLine(file.Result.Snapshot, diag.StartLine)
			}

			fmt.Fprint(r.bw, r.styles.FormatDiagnosticWithFormat(&diag, r.opts.ShowContext, sourceLine, r.opts.RuleFormat))
			total++
		}
	}

	return total
}

// getSourceLine extracts a specific line from a file snapshot using its pre-computed
// line index. This is O(1) per call, unlike the previous splitLines approach which
// re-parsed the entire file content for every diagnostic.
func getSourceLine(snapshot *mdast.FileSnapshot, lineNum int) string {
	if snapshot == nil {
		return ""
	}
	content := snapshot.LineContent(lineNum)
	if content == nil {
		return ""
	}
	return string(content)
}
