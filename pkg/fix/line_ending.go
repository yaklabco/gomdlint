package fix

import (
	"bytes"
	"strings"
)

// Line endings the fix engine writes.
const (
	// LineEndingLF is a Unix line ending.
	LineEndingLF = "\n"

	// LineEndingCRLF is a Windows line ending.
	LineEndingCRLF = "\r\n"
)

// DetectLineEnding returns the dominant line ending of content:
// LineEndingCRLF when more lines end in "\r\n" than in a bare "\n", and
// LineEndingLF when fewer do. A tie goes to the ending of the first line.
// Content without a newline is treated as LF. A carriage return that is not
// followed by a newline is not a line ending.
func DetectLineEnding(content []byte) string {
	newlines := bytes.Count(content, []byte(LineEndingLF))
	if newlines == 0 {
		return LineEndingLF
	}

	crlf := bytes.Count(content, []byte(LineEndingCRLF))
	bare := newlines - crlf
	switch {
	case crlf > bare:
		return LineEndingCRLF
	case crlf < bare:
		return LineEndingLF
	}

	first := bytes.IndexByte(content, '\n')
	if first > 0 && content[first-1] == '\r' {
		return LineEndingCRLF
	}
	return LineEndingLF
}

// ConvertNewlines returns text with every "\n" that is not already part of a
// "\r\n" pair written as lineEnding. Rules build fix text with "\n"; the
// engine converts it so fixes keep a CRLF file's line endings. With
// LineEndingLF the text is returned unchanged.
func ConvertNewlines(text, lineEnding string) string {
	if lineEnding != LineEndingCRLF || !strings.Contains(text, LineEndingLF) {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text) + strings.Count(text, LineEndingLF))
	for idx := range len(text) {
		if text[idx] == '\n' && (idx == 0 || text[idx-1] != '\r') {
			builder.WriteByte('\r')
		}
		builder.WriteByte(text[idx])
	}
	return builder.String()
}
