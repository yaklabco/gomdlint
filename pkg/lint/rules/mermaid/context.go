package mermaid

import (
	"strings"

	mermaidlib "github.com/sammcj/go-mermaid"
	"github.com/sammcj/go-mermaid/ast"

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
