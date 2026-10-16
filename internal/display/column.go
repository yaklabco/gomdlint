// Package display converts the byte-based positions and absolute paths that
// lint diagnostics carry into the forms reporters show to people and tools.
package display

import (
	"bytes"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Unit names the unit in which a reported column counts the text before it
// on its line.
type Unit int

const (
	// CodePoints counts Unicode code points. These are the character columns
	// that markdownlint and most terminal tools report.
	CodePoints Unit = iota + 1

	// UTF16CodeUnits counts UTF-16 code units, so a character outside the
	// Basic Multilingual Plane occupies two columns. SARIF logs declare it as
	// columnKind utf16CodeUnits.
	UTF16CodeUnits
)

// utf8BOM is the UTF-8 encoding of a byte order mark. Editors hide it, and
// SARIF forbids it from affecting columns, so it never counts as a column.
const utf8BOM = "\xef\xbb\xbf"

// utf16SurrogateThreshold is the first code point that UTF-16 encodes as a
// surrogate pair.
const utf16SurrogateThreshold = 0x10000

// BOMLength returns the length in bytes of the byte order mark at the start
// of content, or 0 when content has none.
func BOMLength(content []byte) int {
	if bytes.HasPrefix(content, []byte(utf8BOM)) {
		return len(utf8BOM)
	}
	return 0
}

// Column converts pos, a 1-based line and byte column of snapshot, into a
// 1-based column counted in unit.
//
// A character whose encoding starts before the byte column counts in full, so
// a column that points inside a multibyte character lands just after it. Bytes
// past the end of the line text (its line terminator) count one column each. A
// byte order mark at the start of the file is not counted.
//
// The column is returned unchanged when it cannot be converted: a column below
// 1 (unknown), a nil snapshot, or a line outside the snapshot.
func Column(snapshot *mdast.FileSnapshot, pos mdast.Position, unit Unit) int {
	if snapshot == nil || pos.Column < 1 || pos.Line < 1 || pos.Line > len(snapshot.Lines) {
		return pos.Column
	}

	text := snapshot.LineContent(pos.Line)
	offset := pos.Column - 1
	if pos.Line == 1 {
		if skip := BOMLength(text); skip > 0 {
			text = text[skip:]
			offset = max(offset-skip, 0)
		}
	}

	return unitsBefore(text, offset, unit) + 1
}

// LineText returns the text of a 1-based line of snapshot without its line
// terminator and, for the first line, without a byte order mark. Columns
// produced by Column index into this text. It returns nil for a nil snapshot
// or a line outside it.
func LineText(snapshot *mdast.FileSnapshot, line int) []byte {
	if snapshot == nil {
		return nil
	}

	text := snapshot.LineContent(line)
	if line == 1 {
		text = text[BOMLength(text):]
	}

	return text
}

// unitsBefore counts the units of text that precede byte offset. Offsets past
// the end of text add one unit per byte.
func unitsBefore(text []byte, offset int, unit Unit) int {
	count := 0
	pos := 0

	for pos < len(text) && pos < offset {
		char, size := utf8.DecodeRune(text[pos:])
		count += runeUnits(char, unit)
		pos += size
	}

	if offset > len(text) {
		count += offset - len(text)
	}

	return count
}

// runeUnits returns how many columns char occupies in unit. Invalid bytes
// decode to utf8.RuneError and occupy one column.
func runeUnits(char rune, unit Unit) int {
	if unit == UTF16CodeUnits && char >= utf16SurrogateThreshold {
		return 2
	}

	return 1
}
