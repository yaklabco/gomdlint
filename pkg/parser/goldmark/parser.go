// Package goldmark provides a Parser implementation using the goldmark library.
package goldmark

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Flavor identifies the Markdown flavor supported by the parser.
const (
	FlavorCommonMark = "commonmark"
	FlavorGFM        = "gfm"
)

// Parser implements lint.Parser using goldmark.
type Parser struct {
	flavor string
	md     goldmark.Markdown

	// spans locates the span recorder of a parse in the parser context.
	spans spanKey
}

// New creates a new goldmark-based parser for the given flavor.
// Supported flavors are "commonmark" and "gfm".
// Invalid flavors default to "commonmark".
func New(flavor string) *Parser {
	f := flavorOrDefault(flavor)
	spans := spanKey{key: spanContextKey}
	return &Parser{
		flavor: f,
		md:     newGoldmarkInstance(f, spans),
		spans:  spans,
	}
}

// Flavor returns the configured Markdown flavor.
func (p *Parser) Flavor() string {
	return p.flavor
}

// Parse converts raw Markdown bytes into a fully-populated FileSnapshot.
//
// The method:
//  1. Checks for context cancellation.
//  2. Builds a FileSnapshot shell with path, content, and lines.
//  3. Parses content with goldmark, recording source positions.
//  4. Builds the mdast.Node tree from goldmark AST, with source spans.
//  5. Tokenizes the content, following the parsed code and HTML blocks.
//  6. Assigns token ranges to nodes from their source spans.
//  7. Sets File back-references throughout the tree.
//  8. Validates the token stream.
//
// Returns nil and an error if parsing fails or context is cancelled.
func (p *Parser) Parse(ctx context.Context, path string, content []byte) (*FileSnapshot, error) {
	return p.parse(ctx, path, content, nil)
}

// ParseWithReferences parses content as Parse does, as a part of a larger
// document: links resolve first through refs, the link reference
// definitions of that document keyed by normalized label (as goldmark's
// util.ToLinkReference makes them), and then through the definitions
// content holds. The definitions are shared, not copied, so a parse costs
// nothing per definition. A byte order mark and front matter are still
// recognized at the start of content, so a part that does not start the
// document should start with a blank line.
func (p *Parser) ParseWithReferences(
	ctx context.Context, path string, content []byte, refs map[string]parser.Reference,
) (*FileSnapshot, error) {
	return p.parse(ctx, path, content, refs)
}

// presetContext is a goldmark parse context whose links resolve through a
// set of definitions before those the parse adds. goldmark resolves links
// only through Reference.
type presetContext struct {
	parser.Context
	refs map[string]parser.Reference
}

// Reference returns the definition a normalized label resolves to, a preset
// one first.
//
//nolint:ireturn // goldmark's parser.Context interface fixes this signature.
func (c *presetContext) Reference(label string) (parser.Reference, bool) {
	if ref, found := c.refs[label]; found {
		return ref, true
	}
	return c.Context.Reference(label)
}

// parse is Parse with an optional set of preset definitions.
func (p *Parser) parse(
	ctx context.Context, path string, content []byte, refs map[string]parser.Reference,
) (*FileSnapshot, error) {
	// Check for early cancellation.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("parse cancelled: %w", err)
	}

	// Create the snapshot shell.
	snapshot := &FileSnapshot{
		Path:    path,
		Content: copyContent(content),
		Lines:   mdast.BuildLines(content),
	}

	// A byte order mark is not Markdown. goldmark parses the content after
	// it, and spans keep counting from the start of the file.
	base := bomLength(snapshot.Content)

	// Parse with goldmark.
	snapshot.FrontMatterLines = frontMatterLines(snapshot, base)
	parseContent := snapshot.Content
	if snapshot.FrontMatterLines > 0 {
		parseContent = copyContent(snapshot.Content)
		end := snapshot.Lines[snapshot.FrontMatterLines-1].EndOffset
		for idx := range end {
			if parseContent[idx] != '\n' && parseContent[idx] != '\r' {
				parseContent[idx] = ' '
			}
		}
	}
	source := parseContent[base:]
	reader := text.NewReader(source)
	recorder := newSpanRecorder()
	pc := parser.NewContext()
	pc.Set(p.spans.key, recorder)
	if refs != nil {
		pc = &presetContext{Context: pc, refs: refs}
	}
	gmDoc := p.md.Parser().Parse(reader, parser.WithContext(pc))

	// Check for cancellation after parsing.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("parse cancelled: %w", err)
	}

	// Build mdast.Node tree from goldmark AST.
	mapper := newSourceMapper(snapshot.Content, source, base, snapshot.Lines, recorder)
	snapshot.Root = mapper.mapDocument(gmDoc)

	// Tokenize the content. Code, HTML blocks and front matter follow the
	// parsed document; the rest is tokenized line by line.
	snapshot.Tokens = tokenize(snapshot.Content, mapper.tokenRegions(gmDoc, snapshot.FrontMatterLines))

	// Derive token ranges from the source spans.
	assignTokenRanges(snapshot.Root, snapshot.Tokens)

	// Set File back-references.
	mdast.SetFile(snapshot.Root, snapshot)

	// Validate tokens.
	if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
		return nil, errors.New("invalid token stream: tokens do not cover content")
	}

	return snapshot, nil
}

// utf8BOM is the UTF-8 encoding of the byte order mark U+FEFF.
const utf8BOM = mdast.ByteOrderMark

// bomLength returns the length of a leading UTF-8 byte order mark.
func bomLength(content []byte) int {
	if bytes.HasPrefix(content, []byte(utf8BOM)) {
		return len(utf8BOM)
	}
	return 0
}

// frontMatterLines recognizes a complete leading YAML or TOML metadata block.
// skip is the number of leading bytes to ignore, such as a byte order mark.
func frontMatterLines(file *FileSnapshot, skip int) int {
	if len(file.Lines) < 2 {
		return 0
	}
	lineText := func(idx int) []byte {
		line := file.Lines[idx]
		start := line.StartOffset
		if idx == 0 {
			start = min(start+skip, line.NewlineStart)
		}
		return bytes.TrimRight(file.Content[start:line.NewlineStart], " \t\r")
	}
	opening := lineText(0)
	closingMarker := opening
	switch string(opening) {
	case "---", "+++":
	case "{":
		closingMarker = []byte("}")
	default:
		return 0
	}
	for idx := 1; idx < len(file.Lines); idx++ {
		closing := lineText(idx)
		if bytes.Equal(closing, closingMarker) || (!bytes.Equal(opening, []byte("{")) && bytes.Equal(closing, []byte("..."))) {
			return idx + 1
		}
	}
	return 0
}

// FileSnapshot is a type alias for mdast.FileSnapshot for convenience.
type FileSnapshot = mdast.FileSnapshot

// flavorOrDefault returns the flavor if valid, otherwise defaults to CommonMark.
func flavorOrDefault(flavor string) string {
	switch flavor {
	case FlavorCommonMark, FlavorGFM:
		return flavor
	default:
		return FlavorCommonMark
	}
}

// newGoldmarkInstance creates a configured goldmark.Markdown instance.
func newGoldmarkInstance(flavor string, spans spanKey) goldmark.Markdown {
	var opts []goldmark.Option

	// Configure extensions based on flavor.
	switch flavor {
	case FlavorGFM:
		opts = append(opts, goldmark.WithExtensions(gfmExtensions...))
	case FlavorCommonMark:
		// No extensions for pure CommonMark.
	}

	md := goldmark.New(opts...)
	// Span recording wraps every parser, so it must see the parsers the
	// extensions added. goldmark applies extensions inside New.
	md.Parser().AddOptions(sourceSpans{spanKey: spans})
	return md
}

// copyContent creates a copy of the content slice to ensure immutability.
func copyContent(content []byte) []byte {
	if content == nil {
		return nil
	}
	cp := make([]byte, len(content))
	copy(cp, content)
	return cp
}
