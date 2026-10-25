package fix_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestCombineEdits(t *testing.T) {
	t.Parallel()

	const content = "- a\n  * b\n  * c\n"
	tests := []struct {
		name  string
		edits []fix.TextEdit
		want  fix.TextEdit
		ok    bool
	}{
		{name: "no edits"},
		{
			name:  "one edit",
			edits: []fix.TextEdit{{StartOffset: 6, EndOffset: 7, NewText: "-"}},
			want:  fix.TextEdit{StartOffset: 6, EndOffset: 7, NewText: "-"},
			ok:    true,
		},
		{
			// The text between the edits is written back unchanged.
			name: "edits out of order",
			edits: []fix.TextEdit{
				{StartOffset: 12, EndOffset: 13, NewText: "-"},
				{StartOffset: 6, EndOffset: 7, NewText: "-"},
			},
			want: fix.TextEdit{StartOffset: 6, EndOffset: 13, NewText: "- b\n  -"},
			ok:   true,
		},
		{
			name: "insertion and deletion",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 0, NewText: "  "},
				{StartOffset: 4, EndOffset: 6},
			},
			want: fix.TextEdit{StartOffset: 0, EndOffset: 6, NewText: "  - a\n"},
			ok:   true,
		},
		{
			name: "edits that touch",
			edits: []fix.TextEdit{
				{StartOffset: 6, EndOffset: 7, NewText: "-"},
				{StartOffset: 7, EndOffset: 8, NewText: "  "},
			},
			want: fix.TextEdit{StartOffset: 6, EndOffset: 8, NewText: "-  "},
			ok:   true,
		},
		{
			// An insertion at the start of a replacement goes before it, as
			// the validator orders them.
			name: "insertion at the start of a replacement",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 1, NewText: "A"},
				{StartOffset: 0, EndOffset: 0, NewText: "B"},
			},
			want: fix.TextEdit{StartOffset: 0, EndOffset: 1, NewText: "BA"},
			ok:   true,
		},
		{
			name: "overlapping edits",
			edits: []fix.TextEdit{
				{StartOffset: 4, EndOffset: 7, NewText: "x"},
				{StartOffset: 6, EndOffset: 8, NewText: "y"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := fix.CombineEdits([]byte(content), tt.edits)

			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestCombineEditsKeepsItsInput checks that combining sorts a copy, not the
// caller's edits.
func TestCombineEditsKeepsItsInput(t *testing.T) {
	t.Parallel()

	edits := []fix.TextEdit{{StartOffset: 12, EndOffset: 13, NewText: "-"}, {StartOffset: 6, EndOffset: 7, NewText: "-"}}
	before := append([]fix.TextEdit(nil), edits...)

	_, ok := fix.CombineEdits([]byte("- a\n  * b\n  * c\n"), edits)

	assert.True(t, ok)
	assert.Equal(t, before, edits)
}
