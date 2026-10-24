package mdast

import (
	"bytes"
	"sort"
)

// ByteOrderMark is the UTF-8 encoding of the byte order mark U+FEFF. A file
// that starts with it keeps it in its content and its first line, but the
// mark is not part of the Markdown text, so parsers skip it.
const ByteOrderMark = "\xEF\xBB\xBF"

// BuildLines constructs line metadata from file content.
//
// A line ends at a line feed (LF, "\n"), a carriage return and a line feed
// (CRLF, "\r\n"), or a carriage return that no line feed follows (CR, "\r"),
// the line endings of CommonMark and markdownlint.
func BuildLines(content []byte) []LineInfo {
	if len(content) == 0 {
		return []LineInfo{}
	}

	var lines []LineInfo
	lineStart := 0

	for idx, char := range content {
		newlineStart := idx
		switch {
		case char == '\n' && idx > lineStart && content[idx-1] == '\r':
			newlineStart = idx - 1 // CRLF.
		case char == '\n':
		case char == '\r' && (idx+1 == len(content) || content[idx+1] != '\n'):
		default:
			continue
		}

		lines = append(lines, LineInfo{
			StartOffset:  lineStart,
			NewlineStart: newlineStart,
			EndOffset:    idx + 1,
		})
		lineStart = idx + 1
	}

	// Handle last line (may not have trailing newline).
	if lineStart <= len(content) {
		lines = append(lines, LineInfo{
			StartOffset:  lineStart,
			NewlineStart: len(content),
			EndOffset:    len(content),
		})
	}

	return lines
}

// HasBareCarriageReturn reports whether content holds a carriage return that
// no line feed follows, a line ending of its own.
func HasBareCarriageReturn(content []byte) bool {
	for {
		idx := bytes.IndexByte(content, '\r')
		if idx < 0 {
			return false
		}
		if idx+1 == len(content) || content[idx+1] != '\n' {
			return true
		}
		content = content[idx+2:]
	}
}

// LineFeedContent returns content with every carriage return that no line
// feed follows written as a line feed, so that every line ends with LF or
// CRLF. Each such carriage return is one byte, as the line feed is, so an
// offset into the result is the same offset into content, and the result has
// the same lines. It is content itself when content holds no such carriage
// return, and a copy otherwise.
//
// goldmark ends lines only at line feeds and reads a carriage return alone
// as a space, so the parser gives goldmark this content. Code that parses
// document text with goldmark directly, or reads lines by their line feeds,
// must read this content too.
func LineFeedContent(content []byte) []byte {
	if !HasBareCarriageReturn(content) {
		return content
	}
	out := bytes.Clone(content)
	WriteLineFeeds(out)
	return out
}

// WriteLineFeeds writes every carriage return in content that no line feed
// follows as a line feed, in place.
func WriteLineFeeds(content []byte) {
	for idx, char := range content {
		if char == '\r' && (idx+1 == len(content) || content[idx+1] != '\n') {
			content[idx] = '\n'
		}
	}
}

// LineFeedText is LineFeedContent for a string: text itself when it holds no
// carriage return that no line feed follows.
func LineFeedText(text string) string {
	for idx := range len(text) {
		if text[idx] == '\r' && (idx+1 == len(text) || text[idx+1] != '\n') {
			out := []byte(text)
			WriteLineFeeds(out[idx:])
			return string(out)
		}
	}
	return text
}

// LineCount returns the number of lines in the file.
func (f *FileSnapshot) LineCount() int {
	return len(f.Lines)
}

// LineAt converts a byte offset to 1-based line and column numbers.
// Column counts bytes, not runes.
// Returns (0, 0) if the offset is out of range.
func (f *FileSnapshot) LineAt(offset int) (int, int) {
	if offset < 0 || len(f.Lines) == 0 {
		return 0, 0
	}

	// Handle offset at or past end of content.
	if offset >= len(f.Content) {
		lastLine := f.Lines[len(f.Lines)-1]
		// Return position at end of last line.
		return len(f.Lines), offset - lastLine.StartOffset + 1
	}

	// Binary search to find the line containing the offset.
	lineIdx := sort.Search(len(f.Lines), func(i int) bool {
		return f.Lines[i].EndOffset > offset
	})

	if lineIdx >= len(f.Lines) {
		lineIdx = len(f.Lines) - 1
	}

	lineInfo := f.Lines[lineIdx]

	// Verify offset is within this line.
	if offset < lineInfo.StartOffset {
		return 0, 0
	}

	// 1-based line and column.
	return lineIdx + 1, offset - lineInfo.StartOffset + 1
}

// Offset converts 1-based line and column numbers to a byte offset.
// Returns (offset, true) on success, or (0, false) if out of range.
func (f *FileSnapshot) Offset(line, col int) (int, bool) {
	// Validate line number.
	if line < 1 || line > len(f.Lines) {
		return 0, false
	}

	lineInfo := f.Lines[line-1]

	// Validate column number.
	// Column 1 is the first byte of the line.
	if col < 1 {
		return 0, false
	}

	offset := lineInfo.StartOffset + col - 1

	// Allow column to point to end of line (for cursor positioning).
	if offset > lineInfo.EndOffset {
		return 0, false
	}

	return offset, true
}

// LineContent returns the content of a 1-based line number, excluding the newline.
// Returns nil if the line number is out of range.
func (f *FileSnapshot) LineContent(line int) []byte {
	if line < 1 || line > len(f.Lines) {
		return nil
	}

	lineInfo := f.Lines[line-1]
	return f.Content[lineInfo.StartOffset:lineInfo.NewlineStart]
}
