// Package goldmark provides a Parser implementation using the goldmark library.
package goldmark

import (
	"context"
	"errors"
	"fmt"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
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
}

// New creates a new goldmark-based parser for the given flavor.
// Supported flavors are "commonmark" and "gfm".
// Invalid flavors default to "commonmark".
func New(flavor string) *Parser {
	f := flavorOrDefault(flavor)
	return &Parser{
		flavor: f,
		md:     newGoldmarkInstance(f),
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
//  3. Parses content with goldmark.
//  4. Builds the mdast.Node tree from goldmark AST.
//  5. Tokenizes the content.
//  6. Assigns token ranges to nodes.
//  7. Sets File back-references throughout the tree.
//  8. Validates the token stream.
//
// Returns nil and an error if parsing fails or context is cancelled.
func (p *Parser) Parse(ctx context.Context, path string, content []byte) (*FileSnapshot, error) {
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

	// Parse with goldmark.
	reader := text.NewReader(snapshot.Content)
	gmDoc := p.md.Parser().Parse(reader, parser.WithContext(parser.NewContext()))

	// Check for cancellation after parsing.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("parse cancelled: %w", err)
	}

	// Build mdast.Node tree from goldmark AST.
	mapper := newMapper(snapshot.Content)
	snapshot.Root = mapper.mapDocument(gmDoc)

	// Tokenize content.
	snapshot.Tokens = Tokenize(snapshot.Content)

	// Assign token ranges to nodes.
	assigner := NewTokenRangeAssigner(snapshot.Tokens, snapshot.Content)
	assigner.AssignRanges(snapshot.Root, gmDoc)

	// Set File back-references.
	mdast.SetFile(snapshot.Root, snapshot)

	// Validate tokens.
	if !mdast.ValidateTokens(snapshot.Tokens, len(snapshot.Content)) {
		return nil, errors.New("invalid token stream: tokens do not cover content")
	}

	return snapshot, nil
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
//
//nolint:ireturn // goldmark.Markdown is an external interface type
func newGoldmarkInstance(flavor string) goldmark.Markdown {
	var opts []goldmark.Option

	// Configure extensions based on flavor.
	switch flavor {
	case FlavorGFM:
		opts = append(opts,
			goldmark.WithExtensions(
				extension.GFM,
			),
		)
	case FlavorCommonMark:
		// No extensions for pure CommonMark.
	}

	return goldmark.New(opts...)
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
