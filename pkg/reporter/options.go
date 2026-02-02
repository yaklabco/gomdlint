package reporter

import (
	"io"
	"os"

	"github.com/yaklabco/gomdlint/pkg/config"
)

// bufWriterSize is the buffer size for buffered output writers (64 KiB).
const bufWriterSize = 64 * 1024

// Options configures reporter behavior.
type Options struct {
	// Writer is the destination for output (typically os.Stdout).
	Writer io.Writer

	// ErrorWriter is the destination for errors (typically os.Stderr).
	ErrorWriter io.Writer

	// Format specifies the output format.
	Format Format

	// Color controls colorized output.
	// Values: "auto" (default), "always", "never"
	Color string

	// ShowContext includes source line context in diagnostics.
	ShowContext bool

	// ShowSummary displays aggregate statistics after results.
	ShowSummary bool

	// GroupByFile groups diagnostics by file (default: true for text format).
	GroupByFile bool

	// Compact uses compact/minified output where applicable.
	Compact bool

	// PerFile outputs a separate report for each file (table format only).
	PerFile bool

	// RuleFormat controls how rule identifiers appear in output.
	RuleFormat config.RuleFormat

	// SummaryOrder controls the order of tables in summary output.
	SummaryOrder config.SummaryOrder

	// WorkingDir is the directory to make paths relative to.
	// If empty, paths are kept as-is (typically absolute).
	WorkingDir string
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Writer:       os.Stdout,
		ErrorWriter:  os.Stderr,
		Format:       FormatText,
		Color:        "auto",
		ShowContext:  true,
		ShowSummary:  true,
		GroupByFile:  true,
		Compact:      false,
		RuleFormat:   config.RuleFormatName,
		SummaryOrder: config.SummaryOrderRules,
	}
}
