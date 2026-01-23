// Package reporter provides diagnostic and diff reporting functionality.
package reporter

import (
	"context"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/analysis"
	"github.com/yaklabco/gomdlint/pkg/runner"
)

// Compile-time interface check for reporterFacade.
var _ Reporter = (*reporterFacade)(nil)

// Reporter formats and writes lint results.
type Reporter interface {
	// Report writes formatted output for the given result.
	// It returns the number of issues reported and any write errors.
	Report(ctx context.Context, result *runner.Result) (int, error)
}

// reporterFacade bridges the Reporter interface to Renderer implementations.
type reporterFacade struct {
	renderer     Renderer
	analysisOpts analysis.Options
}

// Report implements Reporter by analyzing the result and rendering it.
func (f *reporterFacade) Report(ctx context.Context, result *runner.Result) (int, error) {
	report := analysis.Analyze(result, f.analysisOpts)
	if err := f.renderer.Render(ctx, report); err != nil {
		return 0, fmt.Errorf("render: %w", err)
	}
	return report.Totals.Issues, nil
}

// newRendererFacade creates a facade wrapping a Renderer.
// Used internally as reporters are migrated.
func newRendererFacade(renderer Renderer, opts Options) *reporterFacade {
	return &reporterFacade{
		renderer: renderer,
		analysisOpts: analysis.Options{
			IncludeDiagnostics: true,
			IncludeByFile:      true,
			IncludeByRule:      true,
			SortBy:             analysis.SortByCount,
			SortDesc:           true,
			RuleFormat:         opts.RuleFormat,
			WorkingDir:         opts.WorkingDir,
		},
	}
}

// New creates a Reporter for the specified options.
//

func New(opts Options) (Reporter, error) {
	// Default writer to stdout if not specified
	if opts.Writer == nil {
		opts.Writer = DefaultOptions().Writer
	}

	// Validate and handle format
	format := opts.Format
	if format == "" {
		format = FormatText
	}
	if !format.IsValid() {
		return nil, fmt.Errorf("unsupported format: %s", format)
	}

	switch format {
	case FormatJSON:
		return NewJSONReporter(opts), nil
	case FormatSARIF:
		return NewSARIFReporter(opts), nil
	case FormatDiff:
		return NewDiffReporter(opts), nil
	case FormatTable:
		return NewTableReporter(opts), nil
	case FormatText:
		return NewTextReporter(opts), nil
	case FormatSummary:
		return newRendererFacade(NewSummaryRenderer(opts), opts), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
