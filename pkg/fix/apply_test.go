package fix_test

import (
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/fix"
)

func TestApplyEdits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		edits   []fix.TextEdit
		want    string
	}{
		{
			name:    "empty edits returns original",
			content: "hello world",
			edits:   nil,
			want:    "hello world",
		},
		{
			name:    "single replacement",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "hi"},
			},
			want: "hi world",
		},
		{
			name:    "single insertion",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 5, NewText: " beautiful"},
			},
			want: "hello beautiful world",
		},
		{
			name:    "single deletion",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 11, NewText: ""},
			},
			want: "hello",
		},
		{
			name:    "multiple non-overlapping edits",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "hi"},
				{StartOffset: 6, EndOffset: 11, NewText: "there"},
			},
			want: "hi there",
		},
		{
			name:    "adjacent edits",
			content: "abcdef",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 2, NewText: "XX"},
				{StartOffset: 2, EndOffset: 4, NewText: "YY"},
				{StartOffset: 4, EndOffset: 6, NewText: "ZZ"},
			},
			want: "XXYYZZ",
		},
		{
			name:    "replace entire content",
			content: "hello",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "world"},
			},
			want: "world",
		},
		{
			name:    "insert at start",
			content: "world",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 0, NewText: "hello "},
			},
			want: "hello world",
		},
		{
			name:    "insert at end",
			content: "hello",
			edits: []fix.TextEdit{
				{StartOffset: 5, EndOffset: 5, NewText: " world"},
			},
			want: "hello world",
		},
		{
			name:    "empty content with insertion",
			content: "",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 0, NewText: "hello"},
			},
			want: "hello",
		},
		{
			name:    "delete all content",
			content: "hello",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: ""},
			},
			want: "",
		},
		{
			name:    "multiple insertions",
			content: "ac",
			edits: []fix.TextEdit{
				{StartOffset: 1, EndOffset: 1, NewText: "b"},
			},
			want: "abc",
		},
		{
			name:    "grow content",
			content: "ab",
			edits: []fix.TextEdit{
				{StartOffset: 1, EndOffset: 1, NewText: "xxx"},
			},
			want: "axxxb",
		},
		{
			name:    "shrink content",
			content: "axxxb",
			edits: []fix.TextEdit{
				{StartOffset: 1, EndOffset: 4, NewText: ""},
			},
			want: "ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := fix.ApplyEdits([]byte(tt.content), tt.edits)

			if string(result) != tt.want {
				t.Errorf("ApplyEdits() = %q, want %q", string(result), tt.want)
			}
		})
	}
}

func TestApplyEdits_PreservesUnmodifiedContent(t *testing.T) {
	t.Parallel()

	content := []byte("hello world")
	original := make([]byte, len(content))
	copy(original, content)

	edits := []fix.TextEdit{
		{StartOffset: 0, EndOffset: 5, NewText: "hi"},
	}

	_ = fix.ApplyEdits(content, edits)

	// Original content should be unchanged.
	if string(content) != string(original) {
		t.Error("ApplyEdits modified original content")
	}
}
