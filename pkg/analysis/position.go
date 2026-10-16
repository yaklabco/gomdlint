package analysis

import (
	"bytes"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// ColumnUnit names the unit in which a reported column counts the text before
// it on its line. Lint diagnostics carry byte columns internally; reporters
// convert them to one of these units at the output boundary.
type ColumnUnit string

const (
	// ColumnCharacters counts Unicode code points. These are the character
	// columns that markdownlint and most terminal tools report.
	ColumnCharacters ColumnUnit = "unicodeCodePoints"

	// ColumnUTF16 counts UTF-16 code units, so a character outside the Basic
	// Multilingual Plane occupies two columns. SARIF uses this unit unless a
	// log declares otherwise.
	ColumnUTF16 ColumnUnit = "utf16CodeUnits"
)

// utf8BOM is the UTF-8 encoding of a byte order mark. Editors hide it, and
// SARIF forbids it from affecting columns, so it never counts as a column.
const utf8BOM = "\xef\xbb\xbf"

// utf16SurrogateThreshold is the first code point that UTF-16 encodes as a
// surrogate pair.
const utf16SurrogateThreshold = 0x10000

// ConvertColumn converts a 1-based byte column on a 1-based line of snapshot
// into a 1-based column counted in unit.
//
// A character whose encoding starts before the byte column counts in full, so
// a column that points inside a multibyte character lands just after it. Bytes
// past the end of the line text (its line terminator) count one column each. A
// byte order mark at the start of the file is not counted.
//
// The column is returned unchanged when it cannot be converted: a column below
// 1 (unknown), a nil snapshot, or a line outside the snapshot.
func ConvertColumn(snapshot *mdast.FileSnapshot, line, column int, unit ColumnUnit) int {
	if snapshot == nil || column < 1 || line < 1 || line > len(snapshot.Lines) {
		return column
	}

	text := snapshot.LineContent(line)
	offset := column - 1
	if line == 1 && bytes.HasPrefix(text, []byte(utf8BOM)) {
		text = text[len(utf8BOM):]
		offset = max(offset-len(utf8BOM), 0)
	}

	return unitsBefore(text, offset, unit) + 1
}

// LineText returns the text of a 1-based line of snapshot without its line
// terminator and, for the first line, without a byte order mark. Column
// numbers produced by ConvertColumn index into this text. It returns nil for a
// nil snapshot or a line outside it.
func LineText(snapshot *mdast.FileSnapshot, line int) []byte {
	if snapshot == nil {
		return nil
	}

	text := snapshot.LineContent(line)
	if line == 1 {
		text = bytes.TrimPrefix(text, []byte(utf8BOM))
	}

	return text
}

// unitsBefore counts the units of text that precede byte offset. Offsets past
// the end of text add one unit per byte.
func unitsBefore(text []byte, offset int, unit ColumnUnit) int {
	count := 0
	pos := 0

	for pos < len(text) && pos < offset {
		r, size := utf8.DecodeRune(text[pos:])
		count += runeUnits(r, unit)
		pos += size
	}

	if offset > len(text) {
		count += offset - len(text)
	}

	return count
}

// runeUnits returns how many columns r occupies in unit. Invalid bytes decode
// to utf8.RuneError and occupy one column.
func runeUnits(r rune, unit ColumnUnit) int {
	if unit == ColumnUTF16 && r >= utf16SurrogateThreshold {
		return 2
	}

	return 1
}
