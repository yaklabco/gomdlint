package mermaid

import (
	"errors"
	"strings"

	mermaidlib "github.com/sammcj/go-mermaid"
	"github.com/sammcj/go-mermaid/ast"
	"github.com/sammcj/go-mermaid/validator"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// MermaidBlock holds a parsed mermaid diagram with its source location.
type MermaidBlock struct {
	Node     *mdast.Node // Original code block node
	Source   string      // Raw mermaid source text
	Diagram  ast.Diagram // Parsed AST from go-mermaid (nil if parse failed)
	ParseErr error       // Non-nil if parsing failed
}

// ExtractMermaidBlocks extracts and parses all mermaid code blocks from context.
func ExtractMermaidBlocks(ctx *lint.RuleContext) []*MermaidBlock {
	if ctx.Root == nil || ctx.File == nil {
		return nil
	}

	codeBlocks := ctx.CodeBlocks()
	var blocks []*MermaidBlock

	for _, cb := range codeBlocks {
		info := strings.ToLower(strings.TrimSpace(lint.CodeBlockInfo(cb)))
		if info != "mermaid" {
			continue
		}

		source := extractCodeBlockContent(ctx.File, cb)
		diagram, parseErr := mermaidlib.Parse(source)

		blocks = append(blocks, &MermaidBlock{
			Node:     cb,
			Source:   source,
			Diagram:  diagram,
			ParseErr: parseErr,
		})
	}

	return blocks
}

// extractCodeBlockContent extracts the text content from a code block node.
// For fenced code blocks, pos.StartLine points to the first content line
// (not the opening fence), and pos.EndLine includes the closing fence.
func extractCodeBlockContent(file *mdast.FileSnapshot, cb *mdast.Node) string {
	pos := cb.SourcePosition()
	if !pos.IsValid() {
		return ""
	}

	// StartLine is already the first content line.
	// EndLine includes the closing fence, so we skip it.
	startLine := pos.StartLine
	endLine := pos.EndLine - 1

	if startLine > endLine || startLine < 1 || endLine > len(file.Lines) {
		return ""
	}

	startOffset := file.Lines[startLine-1].StartOffset
	endOffset := file.Lines[endLine-1].NewlineStart

	if endOffset > len(file.Content) {
		endOffset = len(file.Content)
	}

	return string(file.Content[startOffset:endOffset])
}

// ValidationErrorFilter determines if a validation error should be reported.
type ValidationErrorFilter func(validator.ValidationError) bool

// ValidationDiagnosticBuilder builds a diagnostic from a validation error.
type ValidationDiagnosticBuilder struct {
	RuleID      string
	MessageFunc func(validator.ValidationError) string
	Suggestion  string
	ErrorFilter ValidationErrorFilter
}

// CollectValidationDiagnostics processes mermaid blocks and collects filtered diagnostics.
// This is the common implementation used by MM002 and MM003.
func CollectValidationDiagnostics(
	ctx *lint.RuleContext,
	builder ValidationDiagnosticBuilder,
) ([]lint.Diagnostic, error) {
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
			if !builder.ErrorFilter(err) {
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

			msg := builder.MessageFunc(err)
			severity := mapMermaidSeverity(err.Severity)

			diag := lint.NewDiagnosticAt(builder.RuleID, ctx.File.Path, pos, msg).
				WithSeverity(severity).
				WithSuggestion(builder.Suggestion).
				Build()
			diags = append(diags, diag)
		}
	}

	return diags, nil
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
