package rules

import (
	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmparser "github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// htmlBlockMarkdown parses the lines of an HTML block the way markdownlint
// 0.41 reads them for its HTML rules (MD033, MD045 and MD051): as Markdown
// in which no line starts an HTML block or indented code. The block's tags
// are then inline HTML, while a tag in a code span or a fenced code block is
// code, and one after a backslash or as a link destination is text.
//
//nolint:gochecknoglobals // A goldmark parser is safe for concurrent parses and costly to build.
var htmlBlockMarkdown = newHTMLBlockMarkdown()

// newHTMLBlockMarkdown builds goldmark's default parser without its HTML
// block and indented code parsers, with GFM tables and autolink literals.
func newHTMLBlockMarkdown() goldmark.Markdown {
	skip := map[gmparser.BlockParser]bool{
		gmparser.NewHTMLBlockParser(): true,
		gmparser.NewCodeBlockParser(): true,
	}
	var blocks []util.PrioritizedValue
	for _, block := range gmparser.DefaultBlockParsers() {
		if parser, ok := block.Value.(gmparser.BlockParser); ok && skip[parser] {
			continue
		}
		blocks = append(blocks, block)
	}
	parser := gmparser.NewParser(
		gmparser.WithBlockParsers(blocks...),
		gmparser.WithInlineParsers(gmparser.DefaultInlineParsers()...),
		gmparser.WithParagraphTransformers(gmparser.DefaultParagraphTransformers()...),
	)
	return goldmark.New(goldmark.WithParser(parser), goldmark.WithExtensions(gmext.Table, gmext.Linkify))
}

// htmlBlockInlineHTML returns the source ranges of the raw HTML that
// markdownlint finds in an HTML block: the block's whole lines are parsed
// with htmlBlockMarkdown, and each raw HTML node there runs from the start
// of its first segment to the end of its last. A line's container prefix is
// part of what is parsed, as in markdownlint, so a block in a blockquote is
// read as a blockquote.
func htmlBlockInlineHTML(file *mdast.FileSnapshot, block *mdast.Node) []mdast.SourceRange {
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

	var ranges []mdast.SourceRange
	document := htmlBlockMarkdown.Parser().Parse(gmtext.NewReader(source))
	_ = gmast.Walk(document, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) { //nolint:errcheck // The walker never returns an error.
		raw, ok := node.(*gmast.RawHTML)
		if !ok || !entering || raw.Segments.Len() == 0 {
			return gmast.WalkContinue, nil
		}
		first, last := raw.Segments.At(0), raw.Segments.At(raw.Segments.Len()-1)
		ranges = append(ranges, mdast.SourceRange{StartOffset: start + first.Start, EndOffset: start + last.Stop})
		return gmast.WalkSkipChildren, nil
	})
	return ranges
}
