package fix_test

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestValidator_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		edits   []fix.TextEdit
		wantErr string
	}{
		{
			name:    "valid edits come back sorted",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 6, EndOffset: 11, NewText: "there"},
				{StartOffset: 0, EndOffset: 5, NewText: "hi"},
			},
		},
		{
			name:    "end past content",
			content: "hello",
			edits:   []fix.TextEdit{{StartOffset: 3, EndOffset: 9}},
			wantErr: "exceeds content length",
		},
		{
			name:    "negative start",
			content: "hello",
			edits:   []fix.TextEdit{{StartOffset: -1, EndOffset: 2}},
			wantErr: "negative",
		},
		{
			name:    "edits of one fix overlap",
			content: "hello world",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 5, NewText: "a"},
				{StartOffset: 3, EndOffset: 7, NewText: "b"},
			},
			wantErr: "overlapping",
		},
		{
			name:    "insertion inside a CRLF pair",
			content: "a\r\nb",
			edits:   []fix.TextEdit{{StartOffset: 2, EndOffset: 2, NewText: "\n"}},
			wantErr: "line ending",
		},
		{
			name:    "deletion ending inside a CRLF pair",
			content: "a  \r\nb",
			edits:   []fix.TextEdit{{StartOffset: 1, EndOffset: 4}},
			wantErr: "line ending",
		},
		{
			name:    "edit inside a multi-byte character",
			content: "héllo",
			edits:   []fix.TextEdit{{StartOffset: 2, EndOffset: 3}},
			wantErr: "UTF-8",
		},
		{
			name:    "boundaries are not checked in content that is not UTF-8",
			content: "a\x80b",
			edits:   []fix.TextEdit{{StartOffset: 1, EndOffset: 2}},
		},
		{
			name:    "edits at both ends of the content",
			content: "a\r\n",
			edits: []fix.TextEdit{
				{StartOffset: 0, EndOffset: 0, NewText: "x"},
				{StartOffset: 3, EndOffset: 3, NewText: "y"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := slices.Clone(tt.edits)
			got, err := fix.NewValidator([]byte(tt.content)).Validate(tt.edits)
			assert.Equal(t, input, tt.edits, "Validate must not modify its input")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, slices.IsSortedFunc(got, compareEdits), "edits must be sorted: %v", got)
			assert.ElementsMatch(t, tt.edits, got)
		})
	}
}

func TestResolveFixes(t *testing.T) {
	t.Parallel()

	edit := func(start, end int, text string) fix.TextEdit {
		return fix.TextEdit{StartOffset: start, EndOffset: end, NewText: text}
	}

	tests := []struct {
		name         string
		fixes        [][]fix.TextEdit
		wantAccepted []fix.TextEdit
		wantSkipped  []int
	}{
		{
			name: "empty",
		},
		{
			name: "independent fixes are all accepted in position order",
			fixes: [][]fix.TextEdit{
				{edit(10, 12, "b")},
				{edit(0, 2, "a")},
			},
			wantAccepted: []fix.TextEdit{edit(0, 2, "a"), edit(10, 12, "b")},
		},
		{
			name: "the earlier fix wins a conflict",
			fixes: [][]fix.TextEdit{
				{edit(5, 15, "late")},
				{edit(0, 10, "early")},
			},
			wantAccepted: []fix.TextEdit{edit(0, 10, "early")},
			wantSkipped:  []int{0},
		},
		{
			name: "overlapping deletions merge",
			fixes: [][]fix.TextEdit{
				{edit(0, 5, "")},
				{edit(3, 8, "")},
			},
			wantAccepted: []fix.TextEdit{edit(0, 8, "")},
		},
		{
			name: "a fix is never half applied",
			fixes: [][]fix.TextEdit{
				// Rewrites an opening and a closing marker together.
				{edit(2, 3, "_"), edit(8, 9, "_")},
				// Conflicts with the opening marker only.
				{edit(1, 4, "xyz")},
			},
			wantAccepted: []fix.TextEdit{edit(1, 4, "xyz")},
			wantSkipped:  []int{0},
		},
		{
			name: "insertions at one offset keep fix order",
			fixes: [][]fix.TextEdit{
				{edit(100, 100, "x")},
				{edit(10, 10, "a"), edit(100, 100, "y")},
			},
			wantAccepted: []fix.TextEdit{edit(10, 10, "a"), edit(100, 100, "x"), edit(100, 100, "y")},
		},
		{
			name: "a merged deletion never swallows an accepted insertion",
			fixes: [][]fix.TextEdit{
				{edit(0, 5, "")},
				{edit(5, 5, "X")},
				{edit(3, 8, "")},
			},
			wantAccepted: []fix.TextEdit{edit(0, 8, "")},
			wantSkipped:  []int{1},
		},
		{
			name: "an insertion at the edge of a replacement is not a conflict",
			fixes: [][]fix.TextEdit{
				{edit(3, 6, "abc")},
				{edit(6, 6, "!")},
				{edit(3, 3, "<")},
			},
			wantAccepted: []fix.TextEdit{edit(3, 3, "<"), edit(3, 6, "abc"), edit(6, 6, "!")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accepted, skipped := fix.ResolveFixes(tt.fixes)
			assert.Equal(t, tt.wantAccepted, accepted)
			assert.Equal(t, tt.wantSkipped, skipped)
		})
	}
}

func TestResolveFixes_Properties(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(16, 10))
	texts := []string{"", "", "x", "yy", "\n"}

	for iteration := range 5000 {
		contentLen := rng.IntN(30)
		content := []byte(strings.Repeat("a", contentLen))
		validator := fix.NewValidator(content)

		var fixes [][]fix.TextEdit
		for range rng.IntN(8) {
			var edits []fix.TextEdit
			for range 1 + rng.IntN(3) {
				start := rng.IntN(contentLen + 1)
				end := min(contentLen, start+rng.IntN(6))
				edits = append(edits, fix.TextEdit{StartOffset: start, EndOffset: end, NewText: texts[rng.IntN(len(texts))]})
			}
			if valid, err := validator.Validate(edits); err == nil {
				fixes = append(fixes, valid)
			}
		}

		accepted, skipped := fix.ResolveFixes(fixes)

		// The result can be applied: sorted, in range, and conflict free.
		require.True(t, slices.IsSortedFunc(accepted, compareEdits), "iteration %d: unsorted %v", iteration, accepted)
		require.NoError(t, fix.ValidateEdits(accepted, contentLen), "iteration %d", iteration)
		for idx := 1; idx < len(accepted); idx++ {
			for prev := range idx {
				require.False(t, conflict(accepted[prev], accepted[idx]),
					"iteration %d: %v conflicts with %v", iteration, accepted[prev], accepted[idx])
			}
		}

		// Every accepted fix is applied whole; nothing else is applied.
		isSkipped := make(map[int]bool, len(skipped))
		for _, idx := range skipped {
			isSkipped[idx] = true
		}
		deleted := make([]bool, contentLen)
		var kept []fix.TextEdit
		for fixIdx, edits := range fixes {
			if isSkipped[fixIdx] {
				continue
			}
			for _, edit := range edits {
				if edit.NewText != "" {
					require.Contains(t, accepted, edit, "iteration %d: fix %d is half applied", iteration, fixIdx)
					kept = append(kept, edit)
					continue
				}
				require.True(t, coveredByDeletion(accepted, edit), "iteration %d: deletion %v of fix %d is missing", iteration, edit, fixIdx)
				for pos := edit.StartOffset; pos < edit.EndOffset; pos++ {
					deleted[pos] = true
				}
			}
		}
		for _, edit := range accepted {
			if edit.NewText != "" {
				require.Contains(t, kept, edit, "iteration %d: %v comes from a skipped fix", iteration, edit)
				continue
			}
			for pos := edit.StartOffset; pos < edit.EndOffset; pos++ {
				require.True(t, deleted[pos], "iteration %d: byte %d deleted by no accepted fix", iteration, pos)
			}
		}

		// With one edit per fix the result matches MergeAndFilterConflicts.
		if allSingle(fixes) {
			var flat []fix.TextEdit
			for _, edits := range fixes {
				flat = append(flat, edits...)
			}
			want, _, _, err := fix.PrepareEditsFiltered(flat, contentLen)
			require.NoError(t, err)
			require.Equal(t, want, accepted, "iteration %d: fixes %v", iteration, fixes)
		}
	}
}

func compareEdits(a, b fix.TextEdit) int {
	if a.StartOffset != b.StartOffset {
		return a.StartOffset - b.StartOffset
	}
	return a.EndOffset - b.EndOffset
}

// conflict reports whether two edits cannot both be applied: their ranges
// overlap, or one inserts strictly inside the other.
func conflict(a, b fix.TextEdit) bool {
	return a.StartOffset < b.EndOffset && b.StartOffset < a.EndOffset
}

func coveredByDeletion(accepted []fix.TextEdit, edit fix.TextEdit) bool {
	for _, candidate := range accepted {
		if candidate.NewText == "" && candidate.StartOffset <= edit.StartOffset && edit.EndOffset <= candidate.EndOffset {
			return true
		}
	}
	return false
}

func allSingle(fixes [][]fix.TextEdit) bool {
	for _, edits := range fixes {
		if len(edits) != 1 {
			return false
		}
	}
	return true
}
