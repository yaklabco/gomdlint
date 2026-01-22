package analysis

import "github.com/yaklabco/gomdlint/pkg/config"

// SortField specifies how to sort analysis results.
type SortField string

const (
	// SortByCount sorts by issue count (descending by default).
	SortByCount SortField = "count"
	// SortByAlpha sorts alphabetically.
	SortByAlpha SortField = "alpha"
	// SortBySeverity sorts by severity (errors first).
	SortBySeverity SortField = "severity"
)

// IsValid returns true if the sort field is valid.
func (s SortField) IsValid() bool {
	switch s {
	case SortByCount, SortByAlpha, SortBySeverity:
		return true
	default:
		return false
	}
}

// Options configures the Analyze function.
type Options struct {
	// IncludeDiagnostics includes the flat diagnostics list.
	IncludeDiagnostics bool

	// IncludeByFile includes the per-file analysis.
	IncludeByFile bool

	// IncludeByRule includes the per-rule analysis.
	IncludeByRule bool

	// SortBy specifies how to sort ByFile and ByRule.
	SortBy SortField

	// SortDesc sorts in descending order (highest first).
	SortDesc bool

	// RuleFormat controls how rule identifiers appear.
	RuleFormat config.RuleFormat

	// WorkingDir is the directory to make paths relative to.
	// If empty, paths are kept as-is (typically absolute).
	WorkingDir string
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		IncludeDiagnostics: true,
		IncludeByFile:      true,
		IncludeByRule:      true,
		SortBy:             SortByCount,
		SortDesc:           true,
		RuleFormat:         config.RuleFormatName,
	}
}
