package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestSelectEdits_SelectsAgainWhenTheSelectionChangesNothing(t *testing.T) {
	t.Parallel()

	// Hand-made blocks split "aaaa" between TEST001's fix and TEST002's, so
	// both are selected. Each changes the text, but together they delete an
	// "a" and insert one further on, which changes nothing. TEST003 waits
	// for TEST002's block.
	content := []byte("aaaa\n")
	blocks := &topLevelBlocks{starts: []int{0, 2}, ends: []int{2, 5}, leads: []int{0, 2}}
	fixes := []pendingFix{
		{ruleID: "TEST001", diagnostics: 1, edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1}}},
		{ruleID: "TEST002", diagnostics: 1, edits: []fix.TextEdit{{StartOffset: 3, EndOffset: 3, NewText: "a"}}},
		{ruleID: "TEST003", diagnostics: 1, edits: []fix.TextEdit{{StartOffset: 3, EndOffset: 4, NewText: "b"}}},
	}

	var result FileResult
	selectEdits(&result, fixes, content, blocks, false)

	assert.Equal(t, []fix.TextEdit{{StartOffset: 3, EndOffset: 4, NewText: "b"}}, result.Edits,
		"the rules whose fixes changed nothing together sit out, and TEST003 gets the block")
	assert.Equal(t, 1, result.AcceptedFixes)
	assert.Empty(t, result.DeferredEdits)

	var alone FileResult
	selectEdits(&alone, fixes[:2], content, blocks, false)

	assert.Len(t, alone.Edits, 2, "with no fix waiting, the pipeline sees that the fixes make no progress")
	assert.Empty(t, alone.DeferredEdits)
}

func TestChangesContent(t *testing.T) {
	t.Parallel()

	const content = "abc def\n"
	tests := []struct {
		name  string
		edits []fix.TextEdit
		want  bool
	}{
		{"an edit that puts back its text", []fix.TextEdit{{StartOffset: 0, EndOffset: 3, NewText: "abc"}}, false},
		{"an empty insertion", []fix.TextEdit{{StartOffset: 4, EndOffset: 4}}, false},
		{"an edit of the final line ending that puts it back", []fix.TextEdit{{StartOffset: 7, EndOffset: 8, NewText: "\n"}}, false},
		{
			name:  "a deletion and an insertion that move a space back",
			edits: []fix.TextEdit{{StartOffset: 3, EndOffset: 4}, {StartOffset: 4, EndOffset: 4, NewText: " "}},
			want:  false,
		},
		{
			name:  "an insertion and a deletion that put the text back",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 0, NewText: "ab"}, {StartOffset: 0, EndOffset: 2}},
			want:  false,
		},
		{
			name:  "a deletion and an insertion that put two characters back",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 2}, {StartOffset: 2, EndOffset: 2, NewText: "ab"}},
			want:  false,
		},
		{
			name:  "two replacements that swap characters",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "b"}, {StartOffset: 1, EndOffset: 2, NewText: "a"}},
			want:  true,
		},
		{"a replacement of the same length", []fix.TextEdit{{StartOffset: 4, EndOffset: 7, NewText: "DEF"}}, true},
		{"a replacement that differs at its end", []fix.TextEdit{{StartOffset: 0, EndOffset: 3, NewText: "abd"}}, true},
		{
			name:  "a change before an edit that puts back its text",
			edits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1, NewText: "z"}, {StartOffset: 2, EndOffset: 3, NewText: "c"}},
			want:  true,
		},
		{"an insertion", []fix.TextEdit{{StartOffset: 7, EndOffset: 7, NewText: "x"}}, true},
		{"a deletion", []fix.TextEdit{{StartOffset: 3, EndOffset: 4}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			changed := string(fix.ApplyEdits([]byte(content), tt.edits)) != content
			require.Equal(t, tt.want, changed, "the case agrees with applying the edits")
			assert.Equal(t, tt.want, changesContent([]byte(content), tt.edits))
		})
	}
}

func TestResolveApart_SkipsEveryFixWhenNoAcceptedFixHoldsTheJoin(t *testing.T) {
	t.Parallel()

	// The accepted edits join the carriage return to the line feed, but no
	// fix has an edit there, as fix.ResolveFixes never leaves it. Rather
	// than write the joined line endings, every fix waits.
	content := []byte("a\rxy\nb")
	groups := [][]fix.TextEdit{{{StartOffset: 6, EndOffset: 7, NewText: "c"}}}
	accepted := []fix.TextEdit{{StartOffset: 2, EndOffset: 4}, {StartOffset: 6, EndOffset: 7, NewText: "c"}}

	gotAccepted, gotSkipped := resolveApart(groups, content, accepted, nil)

	assert.Empty(t, gotAccepted)
	assert.Equal(t, []int{0}, gotSkipped)
}
