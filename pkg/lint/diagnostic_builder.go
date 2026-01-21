package lint

import (
	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/fix"
	"github.com/jamesainslie/gomdlint/pkg/mdast"
)

// DiagnosticBuilder helps construct Diagnostic values.
type DiagnosticBuilder struct {
	diag Diagnostic
}

// NewDiagnostic starts building a diagnostic for the given rule and node.
func NewDiagnostic(ruleID string, node *mdast.Node, message string) *DiagnosticBuilder {
	var filePath string
	var pos mdast.SourcePosition

	if node != nil {
		pos = node.SourcePosition()
		if node.File != nil {
			filePath = node.File.Path
		}
	}

	return &DiagnosticBuilder{
		diag: Diagnostic{
			RuleID:      ruleID,
			Message:     message,
			FilePath:    filePath,
			StartLine:   pos.StartLine,
			StartColumn: pos.StartColumn,
			EndLine:     pos.EndLine,
			EndColumn:   pos.EndColumn,
		},
	}
}

// NewDiagnosticAt starts building a diagnostic at a specific position.
func NewDiagnosticAt(
	ruleID string,
	filePath string,
	pos mdast.SourcePosition,
	message string,
) *DiagnosticBuilder {
	return &DiagnosticBuilder{
		diag: Diagnostic{
			RuleID:      ruleID,
			Message:     message,
			FilePath:    filePath,
			StartLine:   pos.StartLine,
			StartColumn: pos.StartColumn,
			EndLine:     pos.EndLine,
			EndColumn:   pos.EndColumn,
		},
	}
}

// NewDiagnosticAtWithRegistry creates a DiagnosticBuilder with rule name lookup.
func NewDiagnosticAtWithRegistry(
	ruleID string,
	filePath string,
	pos mdast.SourcePosition,
	message string,
	reg *Registry,
) *DiagnosticBuilder {
	ruleName := ""
	if reg != nil {
		if rule, ok := reg.GetByID(ruleID); ok {
			ruleName = rule.Name()
		}
	}
	return &DiagnosticBuilder{
		diag: Diagnostic{
			RuleID:      ruleID,
			RuleName:    ruleName,
			FilePath:    filePath,
			Message:     message,
			StartLine:   pos.StartLine,
			StartColumn: pos.StartColumn,
			EndLine:     pos.EndLine,
			EndColumn:   pos.EndColumn,
		},
	}
}

// WithSeverity sets the severity.
func (b *DiagnosticBuilder) WithSeverity(s config.Severity) *DiagnosticBuilder {
	b.diag.Severity = s
	return b
}

// WithSuggestion sets a human-readable fix suggestion.
func (b *DiagnosticBuilder) WithSuggestion(s string) *DiagnosticBuilder {
	b.diag.Suggestion = s
	return b
}

// WithFix adds fix edits from an EditBuilder.
func (b *DiagnosticBuilder) WithFix(builder *fix.EditBuilder) *DiagnosticBuilder {
	if builder != nil {
		b.diag.FixEdits = append(b.diag.FixEdits, builder.Edits...)
	}
	return b
}

// WithEdit adds a single fix edit.
func (b *DiagnosticBuilder) WithEdit(edit fix.TextEdit) *DiagnosticBuilder {
	b.diag.FixEdits = append(b.diag.FixEdits, edit)
	return b
}

// Build returns the constructed Diagnostic.
func (b *DiagnosticBuilder) Build() Diagnostic {
	return b.diag
}
