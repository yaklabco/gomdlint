package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAlignLineEndings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		written string
		offsets []int
		want    []int
	}{
		{
			name:    "line feed written as carriage return and line feed",
			raw:     "a\nb",
			written: "a\r\nb",
			offsets: []int{0, 1, 2, 3},
			want:    []int{0, 1, 3, 4},
		},
		{
			name:    "line endings written apart",
			raw:     "a\r\n\nb",
			written: "a\n\rb",
			offsets: []int{1, 3, 4},
			want:    []int{1, 2, 3},
		},
		{
			// Two offsets between "\r" and "\n", as an empty piece gives,
			// keep their place when the line ending stays.
			name:    "offsets inside a line ending that stays",
			raw:     "a\r\nb",
			written: "a\r\nb",
			offsets: []int{2, 2},
			want:    []int{2, 2},
		},
		{name: "offset inside a line ending that changes", raw: "a\r\nb", written: "a\nb", offsets: []int{2}},
		{name: "written ends early", raw: "ab\r\n", written: "ab", offsets: []int{0, 2}},
		{name: "text differs", raw: "ab", written: "ac", offsets: []int{0}},
		{name: "line ending where raw has text", raw: "ab", written: "a\n", offsets: []int{0}},
		{name: "written runs on", raw: "a", written: "ab", offsets: []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := alignLineEndings(tt.raw, tt.written, tt.offsets)

			assert.Equal(t, tt.want != nil, ok)
			if tt.want != nil {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestChangedStretch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		written string
		want    string
		start   int
		end     int
		changed bool
	}{
		{name: "same", content: "a\nb", written: "a\nb"},
		// The edit takes the whole "\r\n" rather than insert a "\r" before "\n".
		{name: "line feed widened", content: "a\nb", written: "a\r\nb", start: 11, end: 12, want: "\r\n", changed: true},
		// Trimming the "\n" both share would split the "\r\n" of content.
		{name: "carriage return dropped", content: "a\r\nb", written: "a\nb", start: 11, end: 13, want: "\n", changed: true},
		{name: "two line endings", content: "\n\n", written: "\r\r", start: 10, end: 12, want: "\r\r", changed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, changed := changedStretch([]byte(tt.content), tt.written, 10)

			assert.Equal(t, tt.changed, changed)
			if tt.changed {
				assert.Equal(t, tt.start, got.StartOffset)
				assert.Equal(t, tt.end, got.EndOffset)
				assert.Equal(t, tt.want, got.NewText)
			}
		})
	}
}
