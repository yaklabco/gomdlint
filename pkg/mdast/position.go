package mdast

// SourceRange represents a byte range in the source content.
type SourceRange struct {
	// StartOffset is the byte index where the range begins (inclusive).
	StartOffset int

	// EndOffset is the byte index where the range ends (exclusive).
	EndOffset int
}

// Len returns the length of the range in bytes.
func (r SourceRange) Len() int {
	return r.EndOffset - r.StartOffset
}

// IsEmpty returns true if the range has zero length.
func (r SourceRange) IsEmpty() bool {
	return r.StartOffset == r.EndOffset
}

// Contains returns true if the given offset is within this range.
func (r SourceRange) Contains(offset int) bool {
	return offset >= r.StartOffset && offset < r.EndOffset
}

// Position represents a 1-based line and column in a file.
type Position struct {
	Line   int
	Column int
}

// IsValid returns true if this position has valid (positive) values.
func (p Position) IsValid() bool {
	return p.Line > 0 && p.Column > 0
}

// SourcePosition represents a range in terms of line/column positions.
type SourcePosition struct {
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// Start returns the start position.
func (sp SourcePosition) Start() Position {
	return Position{Line: sp.StartLine, Column: sp.StartColumn}
}

// End returns the end position.
func (sp SourcePosition) End() Position {
	return Position{Line: sp.EndLine, Column: sp.EndColumn}
}

// IsValid returns true if both start and end positions are valid.
func (sp SourcePosition) IsValid() bool {
	return sp.StartLine > 0 && sp.StartColumn > 0 &&
		sp.EndLine > 0 && sp.EndColumn > 0
}

// IsSingleLine returns true if start and end are on the same line.
func (sp SourcePosition) IsSingleLine() bool {
	return sp.StartLine == sp.EndLine
}

// SourceRange returns the byte range for this node.
// Returns an empty range if the node has no associated file or tokens.
func (n *Node) SourceRange() SourceRange {
	if n.SourceSpan != nil {
		return *n.SourceSpan
	}
	if n.File == nil || n.FirstToken < 0 || n.LastToken < 0 {
		return SourceRange{}
	}

	tokens := n.File.Tokens
	if n.FirstToken >= len(tokens) || n.LastToken >= len(tokens) {
		return SourceRange{}
	}

	start := tokens[n.FirstToken].StartOffset
	end := tokens[n.LastToken].EndOffset

	return SourceRange{StartOffset: start, EndOffset: end}
}

// ContentRange returns the byte range of the node's content: the part of
// the full span inside the node's syntax markers. It equals SourceRange
// when the node has no separate content span.
func (n *Node) ContentRange() SourceRange {
	if n.ContentSpan != nil {
		return *n.ContentSpan
	}
	return n.SourceRange()
}

// SourcePosition returns the line/column range for this node.
// Returns an invalid position if the node has no associated file.
func (n *Node) SourcePosition() SourcePosition {
	if n.File == nil {
		return SourcePosition{}
	}

	sourceRange := n.SourceRange()
	if n.SourceSpan == nil && sourceRange.IsEmpty() && sourceRange.StartOffset == 0 {
		// Check if this is a truly empty range or just uninitialized.
		if n.FirstToken < 0 {
			return SourcePosition{}
		}
	}

	return n.File.positionOf(sourceRange)
}

// ContentPosition returns the line/column range of the node's content span.
// Returns an invalid position if the node has no associated file.
func (n *Node) ContentPosition() SourcePosition {
	if n.ContentSpan == nil {
		return n.SourcePosition()
	}
	if n.File == nil {
		return SourcePosition{}
	}
	return n.File.positionOf(*n.ContentSpan)
}

// positionOf converts a byte range into 1-based line and column positions.
// The end position is exclusive, like the range itself.
func (f *FileSnapshot) positionOf(r SourceRange) SourcePosition {
	startLine, startCol := f.LineAt(r.StartOffset)
	endLine, endCol := f.LineAt(r.EndOffset)

	return SourcePosition{
		StartLine:   startLine,
		StartColumn: startCol,
		EndLine:     endLine,
		EndColumn:   endCol,
	}
}

// Text returns the source text for this node.
// Returns nil if the node has no associated file.
func (n *Node) Text() []byte {
	return n.sourceText(n.SourceRange())
}

// ContentText returns the source text of the node's content span.
// Returns nil if the node has no associated file.
func (n *Node) ContentText() []byte {
	return n.sourceText(n.ContentRange())
}

func (n *Node) sourceText(rng SourceRange) []byte {
	if n.File == nil {
		return nil
	}
	if rng.StartOffset < 0 || rng.EndOffset > len(n.File.Content) || rng.StartOffset > rng.EndOffset {
		return nil
	}

	return n.File.Content[rng.StartOffset:rng.EndOffset]
}
