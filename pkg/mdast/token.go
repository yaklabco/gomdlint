package mdast

//go:generate stringer -type=TokenKind -trimprefix=Tok

// TokenKind classifies the type of a token in the Markdown source.
type TokenKind uint16

// Token kinds cover every byte in the source, classifying Markdown syntax elements.
const (
	TokText TokenKind = iota
	TokWhitespace
	TokNewline

	TokHeadingMarker    // '#', '##', etc.
	TokSetextUnderline  // '====' / '----'
	TokListBullet       // '-', '+', '*'
	TokListNumber       // '1.', '2)', etc.
	TokBlockquoteMarker // '>'
	TokCodeFence        // ``` or ~~~ fence line
	TokCodeFenceInfo    // info string portion
	TokEmphasisMarker   // '*', '_', '**', '__'
	TokLinkOpen         // '['
	TokLinkClose        // ']'
	TokParenOpen        // '('
	TokParenClose       // ')'
	TokImageMarker      // '!'
	TokBacktick         // inline code backtick sequences
	TokEscapedChar      // '\' + char
	TokHTML             // raw HTML block/inline
	TokThematicBreak    // '---', '***'

	TokOther
)

// Token represents a classified span of bytes in the Markdown source.
// Tokens are contiguous and non-overlapping, covering [0, len(Content)).
type Token struct {
	// Kind classifies what this token represents.
	Kind TokenKind

	// StartOffset is the byte index where this token begins (inclusive).
	StartOffset int

	// EndOffset is the byte index where this token ends (exclusive).
	EndOffset int

	// Meta holds optional parser-specific metadata (e.g., parsed list index, tag name).
	// Must be treated as opaque by generic logic.
	Meta any
}

// Text returns the source text of this token from the given content.
func (t Token) Text(content []byte) []byte {
	if t.StartOffset < 0 || t.EndOffset > len(content) || t.StartOffset > t.EndOffset {
		return nil
	}
	return content[t.StartOffset:t.EndOffset]
}

// Len returns the length of this token in bytes.
func (t Token) Len() int {
	return t.EndOffset - t.StartOffset
}

// IsEmpty returns true if this token has zero length.
func (t Token) IsEmpty() bool {
	return t.StartOffset == t.EndOffset
}

// ValidateTokens checks that a token slice is valid:
// - Tokens are contiguous and non-overlapping.
// - Tokens cover the full content range [0, contentLen).
// Returns true if valid, false otherwise.
func ValidateTokens(tokens []Token, contentLen int) bool {
	if len(tokens) == 0 {
		return contentLen == 0
	}

	// First token must start at 0.
	if tokens[0].StartOffset != 0 {
		return false
	}

	// Last token must end at contentLen.
	if tokens[len(tokens)-1].EndOffset != contentLen {
		return false
	}

	// Check contiguity.
	for i := 1; i < len(tokens); i++ {
		if tokens[i].StartOffset != tokens[i-1].EndOffset {
			return false
		}
	}

	return true
}
