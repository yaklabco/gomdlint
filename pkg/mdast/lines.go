package mdast

import "sort"

// BuildLines constructs line metadata from file content.
// It handles both LF (\n) and CRLF (\r\n) line endings.
func BuildLines(content []byte) []LineInfo {
	if len(content) == 0 {
		return []LineInfo{}
	}

	var lines []LineInfo
	lineStart := 0

	for idx, char := range content {
		if char == '\n' {
			// Check for CRLF.
			newlineStart := idx
			if idx > 0 && content[idx-1] == '\r' {
				newlineStart = idx - 1
			}

			lines = append(lines, LineInfo{
				StartOffset:  lineStart,
				NewlineStart: newlineStart,
				EndOffset:    idx + 1,
			})
			lineStart = idx + 1
		}
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
