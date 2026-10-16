package fix_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestDetectLineEnding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "empty", content: "", want: fix.LineEndingLF},
		{name: "no line breaks", content: "# Title", want: fix.LineEndingLF},
		{name: "lf", content: "a\nb\n", want: fix.LineEndingLF},
		{name: "crlf", content: "a\r\nb\r\n", want: fix.LineEndingCRLF},
		{name: "mostly crlf", content: "a\r\nb\r\nc\n", want: fix.LineEndingCRLF},
		{name: "mostly lf", content: "a\nb\nc\r\n", want: fix.LineEndingLF},
		{name: "tie goes to first crlf", content: "a\r\nb\n", want: fix.LineEndingCRLF},
		{name: "tie goes to first lf", content: "a\nb\r\n", want: fix.LineEndingLF},
		{name: "lone carriage returns are not line endings", content: "a\rb\rc\n", want: fix.LineEndingLF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, fix.DetectLineEnding([]byte(tt.content)))
		})
	}
}

func TestConvertNewlines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		text   string
		ending string
		want   string
	}{
		{name: "lf keeps text", text: "a\nb\n", ending: fix.LineEndingLF, want: "a\nb\n"},
		{name: "lf keeps existing crlf", text: "a\r\nb\n", ending: fix.LineEndingLF, want: "a\r\nb\n"},
		{name: "crlf converts bare newlines", text: "\n", ending: fix.LineEndingCRLF, want: "\r\n"},
		{name: "crlf converts every newline", text: "a\n\nb\n", ending: fix.LineEndingCRLF, want: "a\r\n\r\nb\r\n"},
		{name: "crlf leaves existing pairs", text: "a\r\nb\n", ending: fix.LineEndingCRLF, want: "a\r\nb\r\n"},
		{name: "crlf without newlines", text: "abc", ending: fix.LineEndingCRLF, want: "abc"},
		{name: "empty", text: "", ending: fix.LineEndingCRLF, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, fix.ConvertNewlines(tt.text, tt.ending))
		})
	}
}
