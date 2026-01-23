package mermaid

import (
	"fmt"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// InvalidDirectionRule validates that flowchart directions are valid.
type InvalidDirectionRule struct {
	lint.BaseRule
}

// NewInvalidDirectionRule creates a new MM004 rule.
func NewInvalidDirectionRule() *InvalidDirectionRule {
	return &InvalidDirectionRule{
		BaseRule: lint.NewBaseRule(
			"MM004",
			"mermaid-invalid-direction",
			"Flowchart direction must be valid (TB, TD, BT, RL, LR)",
			[]string{"mermaid"},
			false, // Cannot auto-fix
		),
	}
}

// DefaultSeverity returns warning - invalid direction prevents correct rendering.
func (r *InvalidDirectionRule) DefaultSeverity() config.Severity {
	return config.SeverityWarning
}

// Apply checks for invalid flowchart directions in mermaid diagrams.
// The go-mermaid library reports invalid directions as parse errors (not validation errors)
// because the parser requires a valid direction to construct the AST.
func (r *InvalidDirectionRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	blocks := ExtractMermaidBlocks(ctx)

	for _, block := range blocks {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		// Check if parse error is a direction-related error
		if block.ParseErr != nil && IsDirectionParseError(block.ParseErr) {
			msg := "Invalid flowchart direction: must be one of TB, TD, BT, RL, or LR"
			diag := lint.NewDiagnostic(r.ID(), block.Node, msg).
				WithSeverity(config.SeverityError).
				WithSuggestion("Use a valid direction: TB (top-bottom), TD (top-down), BT (bottom-top), RL (right-left), or LR (left-right)").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}

// IsDirectionParseError checks if the parse error is about an invalid direction.
// The go-mermaid parser produces errors like
// "invalid diagram header: expected 'flowchart' or 'graph' followed by direction".
func IsDirectionParseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "followed by direction")
}
