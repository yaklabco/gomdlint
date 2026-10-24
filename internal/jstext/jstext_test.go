package jstext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// nodeDecodings are byte strings with the UTF-16 length and text that Node's
// UTF-8 decoder, and so markdownlint, gives them: Buffer.toString("utf8").
var nodeDecodings = []struct {
	name    string
	text    string
	length  int
	decoded string
}{
	{name: "ascii", text: "abc", length: 3, decoded: "abc"},
	{name: "two-byte letter", text: "Ä", length: 1, decoded: "Ä"},
	{name: "three-byte letter", text: "中", length: 1, decoded: "中"},
	{name: "astral character", text: "\U0001F600", length: 2, decoded: "\U0001F600"},
	{name: "combining mark", text: "é", length: 2, decoded: "é"},
	{name: "replacement character", text: "�", length: 1, decoded: "�"},
	{name: "truncated three-byte character", text: "\xe2\x82x", length: 2, decoded: "�x"},
	{name: "truncated four-byte character", text: "\xf0\x9f\x98x", length: 2, decoded: "�x"},
	{name: "lone bytes", text: "\xff\xfex", length: 3, decoded: "��x"},
	{name: "encoded surrogate", text: "\xed\xa0\x80x", length: 4, decoded: "���x"},
	{name: "overlong encoding", text: "\xc0\xafx", length: 3, decoded: "��x"},
	{name: "overlong three-byte encoding", text: "\xe0\x80x", length: 3, decoded: "��x"},
	{name: "out of range four-byte lead", text: "\xf4\x90\x80x", length: 4, decoded: "���x"},
	{name: "lead byte at the end", text: "x\xe2", length: 2, decoded: "x�"},
	{name: "empty", text: "", length: 0, decoded: ""},
}

func TestLength(t *testing.T) {
	t.Parallel()

	for _, tt := range nodeDecodings {
		assert.Equal(t, tt.length, Length(tt.text), tt.name)
	}
}

func TestDecode(t *testing.T) {
	t.Parallel()

	for _, tt := range nodeDecodings {
		assert.Equal(t, tt.decoded, Decode(tt.text), tt.name)
		assert.Equal(t, Length(tt.text), Length(Decode(tt.text)), "%s: decoding keeps the length", tt.name)
	}
}

func TestNext(t *testing.T) {
	t.Parallel()

	for _, tt := range nodeDecodings {
		if tt.text == "" {
			continue
		}
		size, units := Next([]byte(tt.text))
		stringSize, stringUnits := NextInString(tt.text)
		assert.Equal(t, size, stringSize, tt.name)
		assert.Equal(t, units, stringUnits, tt.name)
	}

	size, units := NextInString("\xe2\x82x")
	assert.Equal(t, 2, size, "a truncated character is one sequence")
	assert.Equal(t, 1, units)

	size, units = NextInString("\U0001F600x")
	assert.Equal(t, 4, size)
	assert.Equal(t, 2, units)
}

func TestByteOffset(t *testing.T) {
	t.Parallel()

	text := "a\U0001F600Ä\xe2\x82x"
	tests := []struct {
		units int
		want  int
	}{
		{units: 0, want: 0},
		{units: 1, want: 1},
		{units: 2, want: 1}, // inside the emoji: the offset stays before it
		{units: 3, want: 5},
		{units: 4, want: 7},
		{units: 5, want: 9}, // the truncated character is one unit
		{units: 6, want: 10},
		{units: 99, want: 10},
		{units: -1, want: 0},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, ByteOffset(text, tt.units), "units %d", tt.units)
	}
}

func TestLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		limit  int
		offset int
		within int
	}{
		{name: "within the limit", text: "abc", limit: 3, offset: 3, within: 0},
		{name: "ascii past the limit", text: "abcd", limit: 3, offset: 3, within: 0},
		{name: "letter past the limit", text: "aaÄb", limit: 2, offset: 2, within: 0},
		{name: "emoji across the limit", text: "aa\U0001F600 b", limit: 3, offset: 2, within: 1},
		{name: "emoji after the limit", text: "aa\U0001F600 b", limit: 2, offset: 2, within: 0},
		{name: "truncated character before the limit", text: "\xe2\x82abc", limit: 3, offset: 4, within: 0},
	}

	for _, tt := range tests {
		offset, within := Limit(tt.text, tt.limit)
		assert.Equal(t, tt.offset, offset, "%s: byte offset", tt.name)
		assert.Equal(t, tt.within, within, "%s: units into the character", tt.name)
	}
}
