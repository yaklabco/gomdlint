package rules

import (
	"bytes"
	"context"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	goldparser "github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// htmlBlockParser parses the lines of an HTML block the way markdownlint
// 0.41 reads them for its HTML rules (MD033 and MD045): as Markdown in
// which no line starts an HTML block or indented code. The block's tags
// are then inline HTML, while a tag in a code span or a fenced code block
// is code, and one after a backslash or as a link destination is text.
//
//nolint:gochecknoglobals // A parser is safe for concurrent parses and costly to build.
var htmlBlockParser = gmparser.NewHTMLBlockContent()

// htmlBlockMarkdown is goldmark configured as htmlBlockParser is, without
// its source spans, for the lines of a block that hold no table: there a
// plain parse finds the same raw HTML at less cost.
//
//nolint:gochecknoglobals // A goldmark parser is safe for concurrent parses and costly to build.
var htmlBlockMarkdown = newHTMLBlockMarkdown()

// newHTMLBlockMarkdown builds goldmark's default parser without its HTML
// block and indented code parsers, with GFM tables and autolink literals.
func newHTMLBlockMarkdown() goldmark.Markdown {
	skip := map[goldparser.BlockParser]bool{
		goldparser.NewHTMLBlockParser(): true,
		goldparser.NewCodeBlockParser(): true,
	}
	var blocks []util.PrioritizedValue
	for _, block := range goldparser.DefaultBlockParsers() {
		if parser, ok := block.Value.(goldparser.BlockParser); ok && skip[parser] {
			continue
		}
		blocks = append(blocks, block)
	}
	parser := goldparser.NewParser(
		goldparser.WithBlockParsers(blocks...),
		goldparser.WithInlineParsers(goldparser.DefaultInlineParsers()...),
		goldparser.WithParagraphTransformers(goldparser.DefaultParagraphTransformers()...),
	)
	return goldmark.New(goldmark.WithParser(parser), goldmark.WithExtensions(gmext.Table, gmext.Linkify))
}

// blockHTML is a raw HTML region that markdownlint finds in an HTML block,
// and whether it lies in a table, where MD033 applies
// table_allowed_elements.
type blockHTML struct {
	span    mdast.SourceRange
	inTable bool
}

// htmlBlockInlineHTML returns the raw HTML that markdownlint finds in an
// HTML block: the block's whole lines are parsed as Markdown (see
// parsedBlockHTML). A line's container prefix is part of what is parsed,
// as in markdownlint, so a block in a blockquote is read as a blockquote.
func htmlBlockInlineHTML(file *mdast.FileSnapshot, block *mdast.Node) []blockHTML {
	span := block.SourceRange()
	if file == nil || span.StartOffset < 0 || span.EndOffset <= span.StartOffset || span.EndOffset > len(file.Content) {
		return nil
	}
	firstLine, _ := file.LineAt(span.StartOffset)
	lastLine, _ := file.LineAt(span.EndOffset - 1)
	if firstLine < 1 || lastLine < firstLine || lastLine > len(file.Lines) {
		return nil
	}
	start := file.Lines[firstLine-1].StartOffset
	source := file.Content[start:file.Lines[lastLine-1].NewlineStart]
	if bytes.IndexByte(source, '|') < 0 {
		return plainBlockHTML(source, start)
	}
	return parsedBlockHTML(file.Path, source, start)
}

// parsedBlockHTML returns the raw HTML in the lines of a block, source,
// that start at offset start in the file: the lines are parsed with
// htmlBlockParser, and each inline HTML node there is raw HTML, in a table
// or not.
func parsedBlockHTML(path string, source []byte, start int) []blockHTML {
	parsed, err := htmlBlockParser.Parse(context.Background(), path, source)
	if err != nil {
		return nil
	}
	lines := lint.NewRuleContext(context.Background(), parsed, nil, nil)
	var tags []blockHTML
	for _, node := range lines.HTMLInlines() {
		raw := node.SourceRange()
		if raw.IsEmpty() {
			continue
		}
		tags = append(tags, blockHTML{
			span:    mdast.SourceRange{StartOffset: start + raw.StartOffset, EndOffset: start + raw.EndOffset},
			inTable: inTable(node),
		})
	}
	return tags
}

// plainBlockHTML returns the raw HTML in the lines of a block, source,
// that start at offset start in the file and hold no pipe, so no table:
// each raw HTML node of the plain parse runs from the start of its first
// segment to the end of its last.
func plainBlockHTML(source []byte, start int) []blockHTML {
	var tags []blockHTML
	document := htmlBlockMarkdown.Parser().Parse(gmtext.NewReader(source))
	_ = gmast.Walk(document, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) { //nolint:errcheck // The walker never returns an error.
		raw, ok := node.(*gmast.RawHTML)
		if !ok || !entering || raw.Segments.Len() == 0 {
			return gmast.WalkContinue, nil
		}
		first, last := raw.Segments.At(0), raw.Segments.At(raw.Segments.Len()-1)
		tags = append(tags, blockHTML{span: mdast.SourceRange{StartOffset: start + first.Start, EndOffset: start + last.Stop}})
		return gmast.WalkSkipChildren, nil
	})
	return tags
}

// inTable reports whether a node lies in a table.
func inTable(node *mdast.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeTable {
			return true
		}
	}
	return false
}
