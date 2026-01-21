package reporter

import (
	"context"

	"github.com/jamesainslie/gomdlint/pkg/analysis"
)

// Renderer formats an analysis.Report for output.
// Renderers are stateless and only handle presentation logic.
type Renderer interface {
	// Render writes the formatted report to the configured output.
	Render(ctx context.Context, report *analysis.Report) error
}
