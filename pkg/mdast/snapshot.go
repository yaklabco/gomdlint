// Package mdast provides the core Markdown AST representation for gomdlint.
// It defines a lossless, immutable view of Markdown files including:
// - FileSnapshot: the complete file representation
// - Token stream: every byte classified
// - AST nodes: structural representation referencing token spans
package mdast

// FileSnapshot is an immutable, lossless view of a Markdown file at a specific time.
// It holds the raw content, line metadata, token stream, and AST root.
type FileSnapshot struct {
	// Path is the file path (may be empty for in-memory content).
	Path string

	// Content is the full file bytes.
	Content []byte

	// Lines contains metadata for each line in the file.
	Lines []LineInfo

	// Tokens is the full token stream covering every byte.
	Tokens []Token

	// Root is the AST root node (Document).
	Root *Node
}

// LineInfo holds metadata for a single line in a file.
type LineInfo struct {
	// StartOffset is the byte index of the line start.
	StartOffset int

	// NewlineStart is the byte index where newline characters begin.
	// For lines without a trailing newline (e.g., last line), this equals EndOffset.
	NewlineStart int

	// EndOffset is the byte index just after the newline (or end of file).
	EndOffset int
}

// NewFileSnapshot creates a new FileSnapshot from content.
// It builds the line index but does not tokenize or parse (that requires a Parser).
func NewFileSnapshot(path string, content []byte) *FileSnapshot {
	return &FileSnapshot{
		Path:    path,
		Content: content,
		Lines:   BuildLines(content),
		Tokens:  nil,
		Root:    nil,
	}
}
