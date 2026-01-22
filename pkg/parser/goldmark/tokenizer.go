package goldmark

import (
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// tokenizer performs a single-pass tokenization of Markdown content.
// It produces a contiguous, non-overlapping token stream covering [0, len(content)).
type tokenizer struct {
	content []byte
	tokens  []mdast.Token
	pos     int
}

// Tokenize performs a single-pass tokenization of the given content.
// Returns a slice of tokens that are contiguous, non-overlapping, and cover [0, len(content)).
func Tokenize(content []byte) []mdast.Token {
	if len(content) == 0 {
		return nil
	}

	const initialCapacityDivisor = 4 // reasonable initial capacity estimate
	tok := &tokenizer{
		content: content,
		tokens:  make([]mdast.Token, 0, len(content)/initialCapacityDivisor),
		pos:     0,
	}

	tok.tokenize()

	return tok.tokens
}

// tokenize performs the main tokenization loop.
func (t *tokenizer) tokenize() {
	for t.pos < len(t.content) {
		t.tokenizeLine()
	}
}

// tokenizeLine tokenizes a single line, handling line-start constructs first.
func (t *tokenizer) tokenizeLine() {
	// Handle leading whitespace (indentation).
	t.consumeIndentation()

	// Check for block-level constructs at line start.
	if t.pos < len(t.content) {
		switch t.content[t.pos] {
		case '#':
			if t.tryHeadingMarker() {
				t.tokenizeInlineContent()
				return
			}
		case '>':
			t.emitBlockquoteMarker()
			t.tokenizeInlineContent()
			return
		case '-', '+', '*':
			if t.tryListBulletOrThematicBreak() {
				return
			}
		case '_':
			// Underscores can be thematic breaks (not list bullets).
			if t.isThematicBreak('_') {
				t.consumeThematicBreak('_')
				return
			}
		case '~', '`':
			if t.tryCodeFence() {
				return
			}
		case '=':
			if t.trySetextUnderline('=') {
				return
			}
		case '<':
			if t.tryHTMLBlock() {
				return
			}
		}

		// Check for ordered list.
		if t.pos < len(t.content) && isDigit(t.content[t.pos]) {
			if t.tryOrderedListMarker() {
				t.tokenizeInlineContent()
				return
			}
		}

		// Check for setext underline with dash (after ruling out list bullet).
		if t.pos < len(t.content) && t.content[t.pos] == '-' {
			if t.trySetextUnderline('-') {
				return
			}
		}
	}

	// Continue with inline content (regardless of whether we consumed leading content).
	t.tokenizeInlineContent()
}

// consumeIndentation consumes leading whitespace and emits as TokWhitespace.
func (t *tokenizer) consumeIndentation() {
	start := t.pos
	for t.pos < len(t.content) && (t.content[t.pos] == ' ' || t.content[t.pos] == '\t') {
		t.pos++
	}
	if t.pos > start {
		t.emit(mdast.TokWhitespace, start, t.pos)
	}
}

// tryHeadingMarker attempts to parse an ATX heading marker (# through ######).
func (t *tokenizer) tryHeadingMarker() bool {
	start := t.pos
	count := 0

	for t.pos < len(t.content) && t.content[t.pos] == '#' && count < 7 {
		t.pos++
		count++
	}

	// Must be 1-6 # characters followed by space, tab, or end of line.
	if count >= 1 && count <= 6 {
		if t.pos >= len(t.content) || t.content[t.pos] == ' ' || t.content[t.pos] == '\t' || t.content[t.pos] == '\n' || t.content[t.pos] == '\r' {
			t.emit(mdast.TokHeadingMarker, start, t.pos)
			// Consume trailing space after heading marker.
			if t.pos < len(t.content) && (t.content[t.pos] == ' ' || t.content[t.pos] == '\t') {
				wsStart := t.pos
				t.pos++
				t.emit(mdast.TokWhitespace, wsStart, t.pos)
			}
			return true
		}
	}

	// Not a valid heading marker, reset.
	t.pos = start
	return false
}

// emitBlockquoteMarker emits a blockquote marker (>).
func (t *tokenizer) emitBlockquoteMarker() {
	start := t.pos
	t.pos++ // consume '>'
	t.emit(mdast.TokBlockquoteMarker, start, t.pos)

	// Consume optional space after >.
	if t.pos < len(t.content) && t.content[t.pos] == ' ' {
		wsStart := t.pos
		t.pos++
		t.emit(mdast.TokWhitespace, wsStart, t.pos)
	}
}

// tryListBulletOrThematicBreak handles -, +, * which can be list bullets or thematic breaks.
func (t *tokenizer) tryListBulletOrThematicBreak() bool {
	start := t.pos
	marker := t.content[t.pos]

	// Check for thematic break: at least 3 of the same character with optional spaces.
	if t.isThematicBreak(marker) {
		t.consumeThematicBreak(marker)
		return true
	}

	// It's a list bullet if followed by space/tab.
	t.pos++
	if t.pos < len(t.content) && (t.content[t.pos] == ' ' || t.content[t.pos] == '\t') {
		t.emit(mdast.TokListBullet, start, t.pos)
		// Consume the space.
		wsStart := t.pos
		t.pos++
		t.emit(mdast.TokWhitespace, wsStart, t.pos)
		t.tokenizeInlineContent()
		return true
	}

	// Not a list bullet, reset and let inline handle it.
	t.pos = start
	return false
}

// isThematicBreak checks if the line is a thematic break starting with the given marker.
func (t *tokenizer) isThematicBreak(marker byte) bool {
	count := 0
	pos := t.pos

	for pos < len(t.content) && t.content[pos] != '\n' && t.content[pos] != '\r' {
		ch := t.content[pos]
		if ch == marker {
			count++
		} else if ch != ' ' && ch != '\t' {
			return false
		}
		pos++
	}

	return count >= 3
}

// consumeThematicBreak consumes a thematic break line.
func (t *tokenizer) consumeThematicBreak(_ byte) {
	start := t.pos

	for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		t.pos++
	}

	t.emit(mdast.TokThematicBreak, start, t.pos)
	t.consumeNewline()
}

// tryCodeFence attempts to parse a code fence (``` or ~~~).
func (t *tokenizer) tryCodeFence() bool {
	start := t.pos
	fenceChar := t.content[t.pos]
	count := 0

	for t.pos < len(t.content) && t.content[t.pos] == fenceChar {
		t.pos++
		count++
	}

	// Must have at least 3 fence characters.
	if count < 3 {
		t.pos = start
		return false
	}

	fenceEnd := t.pos
	t.emit(mdast.TokCodeFence, start, fenceEnd)

	// Parse info string (rest of line).
	if t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		infoStart := t.pos
		for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
			t.pos++
		}
		if t.pos > infoStart {
			t.emit(mdast.TokCodeFenceInfo, infoStart, t.pos)
		}
	}

	t.consumeNewline()

	// Consume code block content until closing fence.
	t.consumeCodeBlockContent(fenceChar, count)

	return true
}

// consumeCodeBlockContent consumes the content of a fenced code block.
func (t *tokenizer) consumeCodeBlockContent(fenceChar byte, fenceLen int) {
	for t.pos < len(t.content) {
		lineStart := t.pos

		// Check for closing fence.
		// Skip leading whitespace (up to 3 spaces).
		spaces := 0
		for t.pos < len(t.content) && t.content[t.pos] == ' ' && spaces < 3 {
			t.pos++
			spaces++
		}

		if t.pos < len(t.content) && t.content[t.pos] == fenceChar {
			fenceStart := t.pos
			count := 0
			for t.pos < len(t.content) && t.content[t.pos] == fenceChar {
				t.pos++
				count++
			}

			// Check if this is a valid closing fence.
			if count >= fenceLen {
				// Check rest of line is whitespace only.
				restStart := t.pos
				validClosing := true
				for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
					if t.content[t.pos] != ' ' && t.content[t.pos] != '\t' {
						// Not a valid closing fence.
						validClosing = false
						break
					}
					t.pos++
				}

				if validClosing {
					// Valid closing fence.
					if lineStart < fenceStart {
						t.emit(mdast.TokWhitespace, lineStart, fenceStart)
					}
					t.emit(mdast.TokCodeFence, fenceStart, restStart)
					if restStart < t.pos {
						t.emit(mdast.TokWhitespace, restStart, t.pos)
					}
					t.consumeNewline()
					return
				}

				// Not a valid closing fence, reset and treat as content.
				t.pos = lineStart
			} else {
				// Not enough fence chars, reset and treat as content.
				t.pos = lineStart
			}
		} else {
			t.pos = lineStart
		}

		t.consumeCodeLine()
	}
}

// consumeCodeLine consumes a line of code block content.
func (t *tokenizer) consumeCodeLine() {
	start := t.pos
	for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		t.pos++
	}
	if t.pos > start {
		t.emit(mdast.TokText, start, t.pos)
	}
	t.consumeNewline()
}

// trySetextUnderline attempts to parse a setext-style heading underline.
func (t *tokenizer) trySetextUnderline(char byte) bool {
	start := t.pos
	count := 0

	for t.pos < len(t.content) && t.content[t.pos] == char {
		t.pos++
		count++
	}

	// Must have at least one character and rest of line must be whitespace.
	if count < 1 {
		t.pos = start
		return false
	}

	// Check rest of line.
	for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		if t.content[t.pos] != ' ' && t.content[t.pos] != '\t' {
			t.pos = start
			return false
		}
		t.pos++
	}

	t.emit(mdast.TokSetextUnderline, start, t.pos)
	t.consumeNewline()
	return true
}

// tryOrderedListMarker attempts to parse an ordered list marker (1., 2), etc.).
func (t *tokenizer) tryOrderedListMarker() bool {
	start := t.pos

	// Consume digits.
	for t.pos < len(t.content) && isDigit(t.content[t.pos]) {
		t.pos++
	}

	// Must have at least one digit.
	if t.pos == start {
		return false
	}

	// Must be followed by . or ) and then space/tab.
	if t.pos >= len(t.content) {
		t.pos = start
		return false
	}

	delimiter := t.content[t.pos]
	if delimiter != '.' && delimiter != ')' {
		t.pos = start
		return false
	}
	t.pos++

	// Must be followed by space/tab.
	if t.pos >= len(t.content) || (t.content[t.pos] != ' ' && t.content[t.pos] != '\t') {
		t.pos = start
		return false
	}

	t.emit(mdast.TokListNumber, start, t.pos)

	// Consume the space.
	wsStart := t.pos
	t.pos++
	t.emit(mdast.TokWhitespace, wsStart, t.pos)

	return true
}

// tryHTMLBlock attempts to parse an HTML block.
func (t *tokenizer) tryHTMLBlock() bool {
	if t.pos >= len(t.content) || t.content[t.pos] != '<' {
		return false
	}

	start := t.pos

	// Simple heuristic: if it starts with <, consume until end of line.
	// This is a simplified approach; full HTML block detection is complex.
	for t.pos < len(t.content) && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		t.pos++
	}

	t.emit(mdast.TokHTML, start, t.pos)
	t.consumeNewline()
	return true
}

// tokenizeInlineContent tokenizes inline content until end of line.
func (t *tokenizer) tokenizeInlineContent() {
	for t.pos < len(t.content) {
		ch := t.content[t.pos]

		if ch == '\n' || ch == '\r' {
			t.consumeNewline()
			return
		}

		switch ch {
		case '\\':
			t.consumeEscapedChar()
		case '`':
			t.consumeBackticks()
		case '*', '_':
			t.consumeEmphasisMarker()
		case '[':
			t.emitSingle(mdast.TokLinkOpen)
		case ']':
			t.emitSingle(mdast.TokLinkClose)
		case '(':
			t.emitSingle(mdast.TokParenOpen)
		case ')':
			t.emitSingle(mdast.TokParenClose)
		case '!':
			t.emitSingle(mdast.TokImageMarker)
		case '<':
			t.consumeInlineHTML()
		case ' ', '\t':
			t.consumeInlineWhitespace()
		default:
			t.consumeText()
		}
	}
}

// consumeEscapedChar consumes a backslash escape sequence.
func (t *tokenizer) consumeEscapedChar() {
	start := t.pos
	t.pos++ // consume '\'

	if t.pos < len(t.content) && isPunctuation(t.content[t.pos]) {
		t.pos++ // consume escaped char
		t.emit(mdast.TokEscapedChar, start, t.pos)
	} else {
		// Not a valid escape, emit as text.
		t.emit(mdast.TokText, start, t.pos)
	}
}

// consumeBackticks consumes a run of backticks for inline code.
func (t *tokenizer) consumeBackticks() {
	start := t.pos

	for t.pos < len(t.content) && t.content[t.pos] == '`' {
		t.pos++
	}

	t.emit(mdast.TokBacktick, start, t.pos)
}

// consumeEmphasisMarker consumes a run of emphasis markers (* or _).
func (t *tokenizer) consumeEmphasisMarker() {
	start := t.pos
	marker := t.content[t.pos]

	for t.pos < len(t.content) && t.content[t.pos] == marker {
		t.pos++
	}

	t.emit(mdast.TokEmphasisMarker, start, t.pos)
}

// consumeInlineHTML consumes inline HTML.
func (t *tokenizer) consumeInlineHTML() {
	start := t.pos
	t.pos++ // consume '<'

	// Look for closing >.
	for t.pos < len(t.content) && t.content[t.pos] != '>' && t.content[t.pos] != '\n' && t.content[t.pos] != '\r' {
		t.pos++
	}

	if t.pos < len(t.content) && t.content[t.pos] == '>' {
		t.pos++ // consume '>'
		t.emit(mdast.TokHTML, start, t.pos)
	} else {
		// Not valid HTML, emit as text.
		t.emit(mdast.TokText, start, t.pos)
	}
}

// consumeInlineWhitespace consumes inline whitespace.
func (t *tokenizer) consumeInlineWhitespace() {
	start := t.pos

	for t.pos < len(t.content) && (t.content[t.pos] == ' ' || t.content[t.pos] == '\t') {
		t.pos++
	}

	t.emit(mdast.TokWhitespace, start, t.pos)
}

// consumeText consumes regular text content.
func (t *tokenizer) consumeText() {
	start := t.pos

	for t.pos < len(t.content) {
		ch := t.content[t.pos]
		// Stop at special characters or newlines.
		if ch == '\\' || ch == '`' || ch == '*' || ch == '_' || ch == '[' || ch == ']' ||
			ch == '(' || ch == ')' || ch == '!' || ch == '<' || ch == ' ' || ch == '\t' ||
			ch == '\n' || ch == '\r' {
			break
		}
		t.pos++
	}

	if t.pos > start {
		t.emit(mdast.TokText, start, t.pos)
	}
}

// consumeNewline consumes a newline (LF or CRLF).
func (t *tokenizer) consumeNewline() {
	if t.pos >= len(t.content) {
		return
	}

	start := t.pos

	switch t.content[t.pos] {
	case '\r':
		t.pos++
		if t.pos < len(t.content) && t.content[t.pos] == '\n' {
			t.pos++
		}
	case '\n':
		t.pos++
	default:
		return
	}

	t.emit(mdast.TokNewline, start, t.pos)
}

// emit adds a token to the token list.
func (t *tokenizer) emit(kind mdast.TokenKind, start, end int) {
	t.tokens = append(t.tokens, mdast.Token{
		Kind:        kind,
		StartOffset: start,
		EndOffset:   end,
	})
}

// emitSingle emits a single-character token and advances position.
func (t *tokenizer) emitSingle(kind mdast.TokenKind) {
	t.emit(kind, t.pos, t.pos+1)
	t.pos++
}

// isDigit returns true if the byte is an ASCII digit.
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// isPunctuation returns true if the byte is ASCII punctuation (escapable).
func isPunctuation(b byte) bool {
	switch b {
	case '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
		':', ';', '<', '=', '>', '?', '@', '[', '\\', ']', '^', '_', '`', '{', '|', '}', '~':
		return true
	default:
		return false
	}
}
