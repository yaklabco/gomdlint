package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// replaceOnce returns the edit that replaces the only occurrence of old in
// input with replacement.
func replaceOnce(t *testing.T, input, old, replacement string) fix.TextEdit {
	t.Helper()
	require.Equal(t, 1, strings.Count(input, old), "%q must occur once", old)
	start := strings.Index(input, old)
	return fix.TextEdit{StartOffset: start, EndOffset: start + len(old), NewText: replacement}
}

func parseForCheck(t *testing.T, input string) *mdast.FileSnapshot {
	t.Helper()
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	return snapshot
}

func TestFixCheckKeeps(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		old, new    string
		codeChanges bool
		want        bool
	}{
		{
			name:  "wrapping a paragraph line",
			input: "# Title\n\nSome words in a line here.\n\nNext paragraph.\n",
			old:   "in a ", new: "in\na ",
			want: true,
		},
		{
			name:  "removing a single trailing space",
			input: "Line one \nLine two\n",
			old:   "one \n", new: "one\n",
			want: true,
		},
		{
			name:  "removing a hard break",
			input: "Line one  \nLine two\n",
			old:   "one  \n", new: "one\n",
			want: false,
		},
		{
			name:  "changing the start of an ordered list",
			input: "3. three\n4. four\n",
			old:   "3.", new: "1.",
			want: false,
		},
		{
			name:  "renumbering a later item",
			input: "1. one\n3. two\n",
			old:   "3.", new: "2.",
			want: true,
		},
		{
			name:  "changing every bullet of a list",
			input: "* a\n* b\n",
			old:   "* a\n* b", new: "- a\n- b",
			want: true,
		},
		{
			name:  "splitting a list by changing one bullet",
			input: "* a\n* b\n",
			old:   "* b", new: "- b",
			want: false,
		},
		{
			name:  "making a tight list loose",
			input: "- a\n- b\n",
			old:   "- a\n", new: "- a\n\n",
			want: false,
		},
		{
			name:  "nesting an item",
			input: "- a\n- b\n",
			old:   "- b", new: "  - b",
			want: false,
		},
		{
			name:  "reindenting a nested item",
			input: "- a\n    - b\n",
			old:   "    - b", new: "  - b",
			want: true,
		},
		{
			name:  "changing code content",
			input: "```\ncode  here\n```\n",
			old:   "code  here", new: "code here",
			want: false,
		},
		{
			name:  "changing code whitespace where allowed",
			input: "```\ncode\there\n```\n",
			old:   "code\there", new: "code here",
			codeChanges: true,
			want:        true,
		},
		{
			name:  "changing code span spacing",
			input: "Some `a  b` code.\n",
			old:   "a  b", new: "a b",
			want: false,
		},
		{
			name:  "removing a task checkbox",
			input: "- [ ] task\n",
			old:   "[ ]", new: "[\n  ]",
			want: false,
		},
		{
			name:  "changing a link destination",
			input: "A [link](/a) here.\n",
			old:   "/a", new: "/b",
			want: false,
		},
		{
			name:  "turning a setext heading into a paragraph",
			input: "Title\n===\n\ntext\n",
			old:   "Title\n===", new: "Title\n\n===",
			want: false,
		},
		{
			// The blank line makes the second line a paragraph of its own.
			name:  "joining the two paragraphs of a list item",
			input: "intro\n\n- item\n\n  after\n\nlast\n",
			old:   "- item\n\n", new: "- item\n",
			want: false,
		},
		{
			name:  "deleting blank lines between blocks",
			input: "# Title\n\n\n\nText.\n",
			old:   "\n\n\n\n", new: "\n\n",
			want: true,
		},
		{
			name:  "adding a final newline",
			input: "Text.",
			old:   "Text.", new: "Text.\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := parseForCheck(t, tt.input)
			check := newFixCheck(snapshot, config.FlavorGFM, fixCheckOptions{codeWhitespace: tt.codeChanges})
			assert.Equal(t, tt.want, check.keeps([]fix.TextEdit{replaceOnce(t, tt.input, tt.old, tt.new)}))
		})
	}
}

// TestFixCheckRegion checks that a fix is judged by the blocks around it and
// that changes reaching past them are still seen.
func TestFixCheckRegion(t *testing.T) {
	input := "# One\n\npara one\n\n# Two\n\n- item\n\n  in item\n\npara three\n\n# Four\n"
	snapshot := parseForCheck(t, input)
	check := newFixCheck(snapshot, config.FlavorGFM, fixCheckOptions{})

	t.Run("edit inside a block", func(t *testing.T) {
		assert.True(t, check.keeps([]fix.TextEdit{replaceOnce(t, input, "para one", "para  one")}))
	})
	t.Run("edit between blocks", func(t *testing.T) {
		assert.True(t, check.keeps([]fix.TextEdit{replaceOnce(t, input, "\n\n# Two", "\n\n\n# Two")}))
	})
	t.Run("edit that pulls the next block into a list", func(t *testing.T) {
		assert.False(t, check.keeps([]fix.TextEdit{replaceOnce(t, input, "\npara three", "\n  para three")}))
	})
	t.Run("edit at the end of the file", func(t *testing.T) {
		end := len(input)
		assert.True(t, check.keeps([]fix.TextEdit{{StartOffset: end, EndOffset: end, NewText: "\n"}}))
	})
	t.Run("overlapping edits", func(t *testing.T) {
		assert.False(t, check.keeps([]fix.TextEdit{
			{StartOffset: 2, EndOffset: 5, NewText: "x"},
			{StartOffset: 4, EndOffset: 6, NewText: "y"},
		}))
	})
}

func TestKeepSafeFixes(t *testing.T) {
	input := "Line one  \nLine two \nLine three\n"
	snapshot := parseForCheck(t, input)
	diags := []lint.Diagnostic{
		{RuleID: "T1", FixEdits: []fix.TextEdit{replaceOnce(t, input, "one  \n", "one\n")}},
		{RuleID: "T2", FixEdits: []fix.TextEdit{replaceOnce(t, input, "two \n", "two\n")}},
		{RuleID: "T3"},
	}
	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
	kept := keepSafeFixes(ctx, diags, fixCheckOptions{})
	require.Len(t, kept, 3)
	assert.False(t, kept[0].HasFix(), "removing a hard break changes the rendering")
	assert.True(t, kept[1].HasFix())
	assert.False(t, kept[2].HasFix())
}
