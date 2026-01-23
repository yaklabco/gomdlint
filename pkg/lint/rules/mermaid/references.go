package mermaid

import (
	"strings"

	"github.com/sammcj/go-mermaid/validator"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// UndefinedReferenceRule validates that all references in mermaid diagrams are defined.
type UndefinedReferenceRule struct {
	lint.BaseRule
}

// NewUndefinedReferenceRule creates a new MM002 rule.
func NewUndefinedReferenceRule() *UndefinedReferenceRule {
	return &UndefinedReferenceRule{
		BaseRule: lint.NewBaseRule(
			"MM002",
			"mermaid-undefined-reference",
			"All referenced nodes/participants must be defined",
			[]string{"mermaid"},
			false, // Cannot auto-fix
		),
	}
}

// DefaultSeverity returns warning - undefined references may be intentional.
func (r *UndefinedReferenceRule) DefaultSeverity() config.Severity {
	return config.SeverityWarning
}

// Apply checks for undefined references in mermaid diagrams.
func (r *UndefinedReferenceRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return CollectValidationDiagnostics(ctx, ValidationDiagnosticBuilder{
		RuleID:      r.ID(),
		MessageFunc: func(err validator.ValidationError) string { return "Undefined reference: " + err.Message },
		Suggestion:  "Define the referenced node, state, branch, or participant",
		ErrorFilter: isUndefinedReferenceError,
	})
}

// isUndefinedReferenceError checks if the validation error is about an undefined reference.
func isUndefinedReferenceError(err validator.ValidationError) bool {
	msg := strings.ToLower(err.Message)

	// Match patterns from go-mermaid validators:
	// - "undefined node '%s' in link" (flowchart NoUndefinedNodes)
	// - "transition references undefined state %q" (state ValidStateReferences)
	// - "checkout references undefined branch %q" (gitgraph ValidBranchReferences)
	// - "merge references undefined branch %q" (gitgraph ValidBranchReferences)
	// - "note references undefined participant '%s'" (sequence ValidNotePositions)
	// - "link references undefined node %q" (sankey SankeyValidNodeReferencesRule)
	// - various C4 relationship undefined references
	return strings.Contains(msg, "undefined") ||
		strings.Contains(msg, "not defined") ||
		strings.Contains(msg, "unknown node") ||
		strings.Contains(msg, "unknown state") ||
		strings.Contains(msg, "unknown branch") ||
		strings.Contains(msg, "unknown participant")
}
