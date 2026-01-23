package mermaid

import (
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
)

// SyntaxRule validates that mermaid diagrams have valid syntax.
type SyntaxRule struct {
	lint.BaseRule
}

// NewSyntaxRule creates a new MM001 mermaid-syntax rule.
func NewSyntaxRule() *SyntaxRule {
	return &SyntaxRule{
		BaseRule: lint.NewBaseRule(
			"MM001",
			"mermaid-syntax",
			"Mermaid diagram syntax must be valid",
			[]string{"mermaid"},
			false, // Cannot auto-fix
		),
	}
}

// DefaultSeverity returns error - syntax errors are serious.
func (r *SyntaxRule) DefaultSeverity() config.Severity {
	return config.SeverityError
}

// Apply checks all mermaid blocks for parse errors.
func (r *SyntaxRule) Apply(ctx *lint.RuleContext) ([]lint.Diagnostic, error) {
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	var diags []lint.Diagnostic
	blocks := ExtractMermaidBlocks(ctx)

	for _, block := range blocks {
		if ctx.Cancelled() {
			return diags, fmt.Errorf("rule cancelled: %w", ctx.Ctx.Err())
		}

		if block.ParseErr != nil {
			msg := fmt.Sprintf("Invalid mermaid syntax: %v", block.ParseErr)
			diag := lint.NewDiagnostic(r.ID(), block.Node, msg).
				WithSeverity(config.SeverityError).
				WithSuggestion("Check mermaid diagram syntax").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
}
