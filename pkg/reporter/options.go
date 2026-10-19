package reporter

import (
	"io"
	"os"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

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

	// RuleFormat controls how text, table and summary output name rules: by
	// name, the default, by ID, or both. JSON and SARIF output always carry
	// both the ID and the name, and diff output names no rules.
	RuleFormat config.RuleFormat

	// SummaryOrder controls the order of tables in summary output.
	SummaryOrder config.SummaryOrder

	// WorkingDir is the directory paths are shown relative to. Paths of files
	// inside it are printed relative to it and other paths stay absolute; if
	// WorkingDir is empty, paths are kept as-is. SARIF output instead makes
	// artifact URIs relative to the root of the git work tree that contains
	// WorkingDir, or to WorkingDir itself outside one.
	WorkingDir string

	// ToolVersion is the gomdlint version recorded in machine-readable output
	// (SARIF tool.driver.version). When it is empty or "dev", the main module
	// version from the binary's build information is used, if it has one.
	ToolVersion string

	// Registry supplies rule metadata (descriptions, tags and default
	// severities) to formats that describe rules, such as SARIF. Rule
	// descriptions come from the rules' catalog entries where the entries
	// have them. When nil, lint.DefaultRegistry is used.
	Registry RuleLookup
}

// RuleLookup finds a rule and its catalog entry by the rule's ID.
// *lint.Registry implements it.
type RuleLookup interface {
	// GetByID returns the rule with the given ID.
	GetByID(id string) (lint.Rule, bool)

	// Meta returns the catalog entry of the rule with the given ID. It
	// reports false for a rule without an entry.
	Meta(id string) (lint.RuleMeta, bool)
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
