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

// SourcePosition returns the line/column range for this node.
// Returns an invalid position if the node has no associated file.
func (n *Node) SourcePosition() SourcePosition {
	if n.File == nil {
		return SourcePosition{}
	}

	sourceRange := n.SourceRange()
	if sourceRange.IsEmpty() && sourceRange.StartOffset == 0 {
		// Check if this is a truly empty range or just uninitialized.
		if n.FirstToken < 0 {
			return SourcePosition{}
		}
	}

	startLine, startCol := n.File.LineAt(sourceRange.StartOffset)
	endLine, endCol := n.File.LineAt(sourceRange.EndOffset)

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
	if n.File == nil {
		return nil
	}

	r := n.SourceRange()
	if r.StartOffset < 0 || r.EndOffset > len(n.File.Content) {
		return nil
	}

	return n.File.Content[r.StartOffset:r.EndOffset]
}
