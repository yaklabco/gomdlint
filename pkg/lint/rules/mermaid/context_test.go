package mermaid_test

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules/mermaid"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestExtractMermaidBlocks(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```mermaid\nflowchart TD\n    A --> B\n```\n\n```go\nfunc main() {}\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}

	ctx := &lint.RuleContext{File: file, Root: file.Root}
	blocks := mermaid.ExtractMermaidBlocks(ctx)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 mermaid block, got %d", len(blocks))
	}

	if blocks[0].ParseErr != nil {
		t.Errorf("expected successful parse, got error: %v", blocks[0].ParseErr)
	}

	if blocks[0].Diagram == nil {
		t.Error("expected non-nil Diagram")
	}
}

func TestExtractMermaidBlocks_InvalidSyntax(t *testing.T) {
	t.Parallel()

	md := "```mermaid\ninvalid stuff here\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}

	ctx := &lint.RuleContext{File: file, Root: file.Root}
	blocks := mermaid.ExtractMermaidBlocks(ctx)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 mermaid block, got %d", len(blocks))
	}

	if blocks[0].ParseErr == nil {
		t.Error("expected parse error for invalid mermaid")
	}
}

func TestExtractMermaidBlocks_NoMermaidBlocks(t *testing.T) {
	t.Parallel()

	md := "# Test\n\n```go\nfunc main() {}\n```\n"

	parser := goldmark.New(goldmark.FlavorGFM)
	file, err := parser.Parse(context.Background(), "test.md", []byte(md))
	if err != nil {
		t.Fatal(err)
	}

	ctx := &lint.RuleContext{File: file, Root: file.Root}
	blocks := mermaid.ExtractMermaidBlocks(ctx)

	if len(blocks) != 0 {
		t.Fatalf("expected 0 mermaid blocks, got %d", len(blocks))
	}
}
