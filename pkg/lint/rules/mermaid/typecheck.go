package mermaid

import (
	"strings"

	"github.com/sammcj/go-mermaid/validator"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TypeCheckRule validates type modifiers and relationships in mermaid diagrams.
type TypeCheckRule struct {
	lint.BaseRule
}

// NewTypeCheckRule creates a new MM005 rule.
func NewTypeCheckRule() *TypeCheckRule {
	return &TypeCheckRule{
		BaseRule: lint.NewBaseRule(
			"MM005",
			"mermaid-type-check",
			"Type modifiers and relationships must be valid",
			[]string{"mermaid"},
			false, // Cannot auto-fix
		),
	}
}

// DefaultSeverity returns warning - invalid types indicate a problem.
func (r *TypeCheckRule) DefaultSeverity() config.Severity {
	return config.SeverityWarning
}

// Apply checks for type validation errors in mermaid diagrams.
func (r *TypeCheckRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	return CollectValidationDiagnostics(ctx, ValidationDiagnosticBuilder{
		RuleID:      r.ID(),
		MessageFunc: func(err validator.ValidationError) string { return "Invalid type: " + err.Message },
		Suggestion:  "Use valid type modifiers and relationship types",
		ErrorFilter: isTypeCheckError,
	})
}

// isTypeCheckError checks if the validation error is about type validation.
// This catches errors not already handled by MM002 (undefined references),
// MM003 (duplicates), and MM004 (directions).
func isTypeCheckError(err validator.ValidationError) bool {
	msg := strings.ToLower(err.Message)

	// Exclude errors handled by other rules
	if isUndefinedReferenceError(err) {
		return false
	}
	if isDuplicateError(err) {
		return false
	}

	// Match type-check patterns from go-mermaid validators:
	// - "invalid visibility modifier %q (must be +, -, #, or ~)" (class ValidMemberVisibility)
	// - "invalid relationship type %q" (class ValidRelationshipType)
	// - "invalid message arrow '%s'" (sequence ValidMessageArrows)
	// - "invalid direction '%s'" (flowchart ValidDirection) - handled by MM004 but caught at parse time
	return strings.Contains(msg, "invalid visibility") ||
		strings.Contains(msg, "invalid relationship type") ||
		strings.Contains(msg, "invalid message arrow") ||
		strings.Contains(msg, "invalid arrow") ||
		strings.Contains(msg, "invalid type") ||
		strings.Contains(msg, "invalid modifier")
}
