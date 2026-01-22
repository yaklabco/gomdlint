// Package lint provides the rule engine, diagnostics, and registry for gomdlint.
package lint

import (
	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Diagnostic represents a single lint issue found in a file.
type Diagnostic struct {
	// RuleID is the identifier of the rule that produced this diagnostic.
	RuleID string

	// RuleName is the human-readable name of the rule (e.g., "no-trailing-spaces").
	RuleName string

	// Message is the human-readable description of the issue.
	Message string

	// Severity indicates the importance of the diagnostic.
	Severity config.Severity

	// FilePath is the path to the file containing the issue.
	FilePath string

	// StartLine is the 1-based line number where the issue starts.
	StartLine int

	// StartColumn is the 1-based column number where the issue starts.
	StartColumn int

	// EndLine is the 1-based line number where the issue ends.
	EndLine int

	// EndColumn is the 1-based column number where the issue ends.
	EndColumn int

	// Suggestion is an optional human-readable fix suggestion.
	Suggestion string

	// FixEdits contains the text edits to fix this issue (may be empty).
	FixEdits []fix.TextEdit
}

// HasFix returns true if this diagnostic has associated fix edits.
func (d *Diagnostic) HasFix() bool {
	return len(d.FixEdits) > 0
}

// SourcePosition returns the diagnostic position as a SourcePosition.
func (d *Diagnostic) SourcePosition() mdast.SourcePosition {
	return mdast.SourcePosition{
		StartLine:   d.StartLine,
		StartColumn: d.StartColumn,
		EndLine:     d.EndLine,
		EndColumn:   d.EndColumn,
	}
}

// Rule defines the interface that all lint rules must implement.
type Rule interface {
	// ID returns the unique identifier for this rule (e.g., "MD001").
	ID() string

	// Name returns the human-readable name of the rule.
	Name() string

	// Description returns a detailed description of what the rule checks.
	Description() string

	// DefaultEnabled returns whether the rule is enabled by default.
	DefaultEnabled() bool

	// DefaultSeverity returns the default severity for this rule.
	DefaultSeverity() config.Severity

	// Tags returns categorization tags for this rule (e.g., ["style", "heading"]).
	Tags() []string

	// CanFix returns whether this rule can auto-fix issues.
	CanFix() bool

	// Apply executes the rule against the given context and returns diagnostics.
	//
	// Rules must:
	//   - Return diagnostics for each violation found.
	//   - Use Builder to propose fix edits (if CanFix() is true).
	//   - Respect context cancellation.
	//   - Return error only for internal failures, not violations.
	Apply(ctx *RuleContext) ([]Diagnostic, error)
}
