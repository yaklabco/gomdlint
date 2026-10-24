package rules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMayStartFootnotes checks the loose test for where a footnote
// definition may start: a "[^" that only whitespace, container markers and
// the "]:" of another definition precede on its line, with "]:" after it
// before the next blank line.
func TestMayStartFootnotes(t *testing.T) {
	tests := []struct {
		input   string
		offsets []int
	}{
		{input: "[^1]: Note.\n", offsets: []int{0}},
		{input: "Text.\n\n   [^a`[`]: Note.\n", offsets: []int{10}},
		{input: "> - 1. [^a]: Note.\n", offsets: []int{7}},
		{input: "1) [^a]: Note.\n", offsets: []int{3}},
		{input: "\t[^a]: Note.\n", offsets: []int{1}},
		{input: "[^a\nb]: Note.\n", offsets: []int{0}},
		{input: "[^x]: [^a`[`]: Note.\n", offsets: []int{0, 6}},
		{input: "[^x]: - [^y]: Note.\n", offsets: []int{0, 8}},
		{input: "[^1]: Note.\r\n[^2]: More.\r\n", offsets: []int{0, 13}},
		{input: "Text.\r[^1]: Note.\r", offsets: []int{6}},
		{input: "[^1] starts.\r\n\r\n[^1]: Note.\r\n", offsets: []int{16}},
		{input: "[^1] starts.\r\r[^1]: Note.\r", offsets: []int{14}},
		{input: "[^1] starts this line.\n"},
		{input: "[^a\n\nb]: Note.\n"},
		{input: "Text [^a]: mid line.\n"},
		// No renderer reads a definition here, but the test may find more.
		{input: "-[^a]: Note.\n", offsets: []int{1}},
		{input: "x[^a]: Note.\n"},
		{input: "12x [^a]: Note.\n"},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.offsets, mayStartFootnotes([]byte(tt.input)))
		})
	}
}

// TestParseFootnotesGivesUpOnDeepNesting checks that a line with many
// nested footnote definitions, which goldmark parses in quadratic time,
// counts as unread without the footnote parse.
func TestParseFootnotesGivesUpOnDeepNesting(t *testing.T) {
	source := []byte(strings.Repeat("[^a]: ", maxFootnoteNesting+1) + "x <b>\n")
	doc, unread := parseFootnotes(source)
	assert.Nil(t, doc)
	assert.True(t, unread)
	doc, _ = parseFootnotes([]byte(strings.Repeat("[^a]: ", maxFootnoteNesting) + "x\n"))
	assert.NotNil(t, doc, "the footnote parse runs up to the limit")
}
