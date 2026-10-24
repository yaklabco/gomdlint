// Package jstext measures text as markdownlint does, as a JavaScript string:
// in UTF-16 code units, after decoding UTF-8 as Node does. A character
// outside the Basic Multilingual Plane, such as an emoji, is two code units
// and every other character one. Text that is not valid UTF-8 decodes as in
// the WHATWG UTF-8 decoder that Node uses: each maximal ill-formed sequence
// becomes one U+FFFD, one code unit. Reporters, rules and the text caret all
// measure with this package, so their columns and lengths agree.
package jstext

import (
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Next returns the length in bytes of the character at the start of text,
// which must not be empty, and the UTF-16 code units it occupies once
// decoded: a well-formed character keeps its own code units, and a maximal
// ill-formed sequence becomes one U+FFFD, one code unit.
func Next(text []byte) (int, int) {
	char, size := utf8.DecodeRune(text)
	if char != utf8.RuneError || size > 1 {
		return size, utf16.RuneLen(char)
	}

	return maximalSubpart(text), 1
}

// NextInString is Next for a string.
func NextInString(text string) (int, int) {
	char, size := utf8.DecodeRuneInString(text)
	if char != utf8.RuneError || size > 1 {
		return size, utf16.RuneLen(char)
	}

	return maximalSubpart(text), 1
}

// Length returns the UTF-16 code units of text, the length markdownlint
// gives it.
func Length(text string) int {
	length := 0
	for offset := 0; offset < len(text); {
		size, units := NextInString(text[offset:])
		offset += size
		length += units
	}

	return length
}

// Decode returns text as Node decodes it: valid UTF-8 unchanged, and each
// maximal ill-formed sequence replaced by U+FFFD. The result has the same
// UTF-16 code units as text.
func Decode(text string) string {
	if utf8.ValidString(text) {
		return text
	}

	var out strings.Builder
	out.Grow(len(text))
	for offset := 0; offset < len(text); {
		char, size := utf8.DecodeRuneInString(text[offset:])
		if char == utf8.RuneError && size <= 1 {
			size = maximalSubpart(text[offset:])
			out.WriteRune(utf8.RuneError)
		} else {
			out.WriteString(text[offset : offset+size])
		}
		offset += size
	}

	return out.String()
}

// ByteOffset returns the byte offset in text of the character that holds
// UTF-16 code unit units, counted from 0: the offset after the characters
// that end at or before that unit. A count that ends inside a character
// outside the Basic Multilingual Plane stays before that character, and a
// count past the end of text returns its length.
func ByteOffset(text string, units int) int {
	offset, counted := 0, 0
	for offset < len(text) {
		size, charUnits := NextInString(text[offset:])
		if counted+charUnits > units {
			break
		}
		offset += size
		counted += charUnits
	}

	return offset
}

// Limit returns where text first goes past limit UTF-16 code units: the byte
// offset of the character that holds code unit limit, counted from 0, and
// how many code units of that character come before it. The second result is
// 1 when the limit falls in the middle of a surrogate pair, such as an
// emoji, and 0 otherwise. It returns the length of text and 0 when text is
// not longer than limit.
func Limit(text string, limit int) (int, int) {
	length := 0
	for offset := 0; offset < len(text); {
		size, units := NextInString(text[offset:])
		if length+units > limit {
			return offset, limit - length
		}
		offset += size
		length += units
	}

	return len(text), 0
}

// maximalSubpart returns the length of the ill-formed sequence at the start
// of text that a UTF-8 decoder replaces with one U+FFFD: the lead byte and
// the continuation bytes after it that still form the start of a well-formed
// character, at least one byte. Unicode calls this the maximal subpart.
func maximalSubpart[T ~string | ~[]byte](text T) int {
	idx := slices.IndexFunc(utf8Leads[:], func(lead utf8Lead) bool {
		return text[0] >= lead.first && text[0] <= lead.last
	})
	if idx < 0 {
		return 1
	}

	lead := utf8Leads[idx]
	low, high := lead.low, lead.high
	size := 1
	for size <= lead.continuations && size < len(text) {
		next := text[size]
		if next < low || next > high {
			break
		}
		size++
		low, high = utf8.RuneSelf, continuationMax
	}

	return size
}

// continuationMax is the largest UTF-8 continuation byte. The smallest is
// utf8.RuneSelf.
const continuationMax = 0xBF

// utf8Lead describes the well-formed UTF-8 sequences whose lead byte lies in
// [first, last]: the number of continuation bytes after the lead, and the
// range [low, high] of the first of them. Every later continuation byte lies
// in [utf8.RuneSelf, continuationMax].
type utf8Lead struct {
	first, last   byte
	continuations int
	low, high     byte
}

// utf8Leads are the lead bytes of well-formed UTF-8, after Table 3-7 of the
// Unicode Standard. Other bytes never start a well-formed sequence.
//
//nolint:gochecknoglobals,mnd // Read-only lookup table of byte values.
var utf8Leads = [...]utf8Lead{
	{first: 0xC2, last: 0xDF, continuations: 1, low: 0x80, high: 0xBF},
	{first: 0xE0, last: 0xE0, continuations: 2, low: 0xA0, high: 0xBF},
	{first: 0xE1, last: 0xEC, continuations: 2, low: 0x80, high: 0xBF},
	{first: 0xED, last: 0xED, continuations: 2, low: 0x80, high: 0x9F},
	{first: 0xEE, last: 0xEF, continuations: 2, low: 0x80, high: 0xBF},
	{first: 0xF0, last: 0xF0, continuations: 3, low: 0x90, high: 0xBF},
	{first: 0xF1, last: 0xF3, continuations: 3, low: 0x80, high: 0xBF},
	{first: 0xF4, last: 0xF4, continuations: 3, low: 0x80, high: 0x8F},
}
