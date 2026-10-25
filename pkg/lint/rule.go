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

	// StartColumnUTF16Offset is how many UTF-16 code units into the character
	// at StartColumn the issue starts. Columns count bytes inside the linter
	// and cannot point into a character, while markdownlint, counting UTF-16
	// code units, can start a finding in the middle of a character outside
	// the Basic Multilingual Plane, such as the second half of an emoji's
	// surrogate pair. A rule that matches such a column points StartColumn at
	// the character and sets this to 1. Reporters add it to the column they
	// report. It is 0 otherwise.
	StartColumnUTF16Offset int

	// EndLine is the 1-based line number where the issue ends.
	EndLine int

	// EndColumn is the 1-based column number where the issue ends.
	EndColumn int

	// Suggestion is an optional human-readable fix suggestion.
	Suggestion string

	// FixEdits contains the text edits to fix this issue (may be empty).
	FixEdits []fix.TextEdit

	// FixGroup, when not 0, puts the fix of this diagnostic in a group with
	// the fixes of the rule's other diagnostics in the file that have the
	// same FixGroup. A rule groups fixes that are only safe together, such
	// as the bullets of one list, where a different bullet starts a new
	// list. The engine checks and applies a group as one fix, which fixes
	// each of its diagnostics and makes an edit that several of them carry
	// once. Or it withholds the fix of every diagnostic in it, and clears
	// their FixGroup: when one of them is suppressed, has no valid fix or
	// overlaps another, or when the engine withholds the one fix. Each
	// diagnostic keeps its own edits, so reports show what each of them
	// changes, with the group, which a consumer applies together. Where the
	// engine writes the one fix in other line endings, it rewrites the
	// members' edits to match, so together they make that fix, or, where
	// they cannot, gives each member the whole fix, which a consumer
	// applies once. Insertions of several members at one offset are made in
	// the order the rule reports the members.
	FixGroup int
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

	// Tags returns the rule's own tags (e.g., ["headings", "style"]). Inline
	// directives and migrated markdownlint configs resolve tags through the
	// rule's catalog entry instead (RuleMeta.Tags), which carries
	// markdownlint's tags. Directives fall back to these tags only for a
	// word that names no registered rule and that no catalog entry uses as
	// an identifier or a tag.
	Tags() []string

	// CanFix returns whether this rule can auto-fix issues.
	CanFix() bool

	// Apply executes the rule against the given context and returns diagnostics.
	//
	// Rules must:
	//   - Return diagnostics for each violation found.
	//   - Attach fix edits to Diagnostic.FixEdits (if CanFix() is true). The
	//     edits of one diagnostic are applied together or not at all, and
	//     "\n" in their text is written with the file's line ending. Fixes
	//     that are only safe together share a Diagnostic.FixGroup.
	//   - Respect context cancellation.
	//   - Return error only for internal failures, not violations.
	Apply(ctx *RuleContext) ([]Diagnostic, error)
}
