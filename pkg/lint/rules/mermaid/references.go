package mermaid

import (
	"errors"
	"strings"

	mermaidlib "github.com/sammcj/go-mermaid"
	"github.com/sammcj/go-mermaid/validator"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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
	if ctx.Root == nil || ctx.File == nil {
		return nil, nil
	}

	strict := ctx.OptionBool("strict", false)
	var diags []lint.Diagnostic
	blocks := ExtractMermaidBlocks(ctx)

	for _, block := range blocks {
		if ctx.Cancelled() {
			return diags, errors.New("rule cancelled")
		}

		// Skip blocks that failed to parse (MM001 will report those)
		if block.ParseErr != nil || block.Diagram == nil {
			continue
		}

		validationErrors := mermaidlib.Validate(block.Diagram, strict)
		for _, err := range validationErrors {
			if !isUndefinedReferenceError(err) {
				continue
			}

			// Calculate the document line from the validation error's relative line
			// block.Node.SourcePosition().StartLine is the first content line of the code block
			blockPos := block.Node.SourcePosition()
			docLine := blockPos.StartLine + err.Line - 1

			pos := mdast.SourcePosition{
				StartLine:   docLine,
				StartColumn: err.Column,
				EndLine:     docLine,
				EndColumn:   err.Column,
			}

			msg := "Undefined reference: " + err.Message
			severity := mapMermaidSeverity(err.Severity)

			diag := lint.NewDiagnosticAt(r.ID(), ctx.File.Path, pos, msg).
				WithSeverity(severity).
				WithSuggestion("Define the referenced node, state, branch, or participant").
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
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

// mapMermaidSeverity converts go-mermaid severity to gomdlint severity.
func mapMermaidSeverity(s validator.Severity) config.Severity {
	switch s {
	case validator.SeverityError:
		return config.SeverityError
	case validator.SeverityWarning:
		return config.SeverityWarning
	case validator.SeverityInfo:
		return config.SeverityInfo
	default:
		return config.SeverityWarning
	}
}
