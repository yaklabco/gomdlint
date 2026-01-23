package mermaid

import (
	"strings"

	"github.com/sammcj/go-mermaid/validator"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// DuplicateIDRule validates that all identifiers in mermaid diagrams are unique.
type DuplicateIDRule struct {
	lint.BaseRule
}

// NewDuplicateIDRule creates a new MM003 rule.
func NewDuplicateIDRule() *DuplicateIDRule {
	return &DuplicateIDRule{
		BaseRule: lint.NewBaseRule(
			"MM003",
			"mermaid-duplicate-id",
			"Diagram identifiers must be unique",
			[]string{"mermaid"},
			false, // Cannot auto-fix
		),
	}
}

// DefaultSeverity returns warning - duplicate identifiers indicate a problem.
func (r *DuplicateIDRule) DefaultSeverity() config.Severity {
	return config.SeverityWarning
}

// Apply checks for duplicate identifiers in mermaid diagrams.
func (r *DuplicateIDRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return CollectValidationDiagnostics(ctx, ValidationDiagnosticBuilder{
		RuleID:      r.ID(),
		MessageFunc: func(err validator.ValidationError) string { return "Duplicate identifier: " + err.Message },
		Suggestion:  "Remove or rename the duplicate identifier",
		ErrorFilter: isDuplicateError,
	})
}

// isDuplicateError checks if the validation error is about a duplicate identifier.
func isDuplicateError(err validator.ValidationError) bool {
	msg := strings.ToLower(err.Message)

	// Match patterns from go-mermaid validators:
	// - "duplicate node ID '%s' (first defined at line %d)" (flowchart NoDuplicateNodeIDs)
	// - "duplicate participant ID '%s', first defined at line %d" (sequence NoDuplicateParticipants)
	// - "duplicate state ID %q (first defined at line %d)" (state NoDuplicateStates)
	// - "duplicate class name %q (first defined at line %d)" (class NoDuplicateClasses)
	// - "duplicate branch %q (first defined at line %d)" (gitgraph NoDuplicateBranchNamesRule)
	// - "duplicate %s %q (first defined at line %d)" (utilities DuplicateChecker)
	return strings.Contains(msg, "duplicate")
}
