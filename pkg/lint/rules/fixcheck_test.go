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

// fixCheckContext parses input as GFM and returns a rule context for it.
func fixCheckContext(t *testing.T, input string) *lint.RuleContext {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	return lint.NewRuleContext(context.Background(), file, cfg, nil)
}

// fixKeepsDocument reports whether applying edits changes the document only
// as intent says.
func fixKeepsDocument(ctx *lint.RuleContext, edits []fix.TextEdit, intent fixIntent) bool {
	return fixesKeepDocument(ctx, edits, []fixIntent{intent})
}

// firstNode returns the first node of kind in document order, or nil.
func firstNode(root *mdast.Node, kind mdast.NodeKind) *mdast.Node {
	var found *mdast.Node
	//nolint:errcheck // The visitor never returns an error.
	_ = mdast.Walk(root, func(node *mdast.Node) error {
		if found == nil && node.Kind == kind {
			found = node
		}
		return nil
	})
	return found
}

// span returns the range of the first occurrence of part in input, moved by
// skip bytes and len bytes long.
func span(t *testing.T, input, part string, skip, length int) mdast.SourceRange {
	t.Helper()
	idx := strings.Index(input, part)
	require.GreaterOrEqual(t, idx, 0, "%q not in %q", part, input)
	return mdast.SourceRange{StartOffset: idx + skip, EndOffset: idx + skip + length}
}

func TestFixKeepsDocument(t *testing.T) {
	type fixCase struct {
		name   string
		input  string
		edit   func(t *testing.T, input string) fix.TextEdit
		intent func(t *testing.T, ctx *lint.RuleContext) fixIntent
		want   bool
	}
	replace := func(part string, skip, length int, text string) func(*testing.T, string) fix.TextEdit {
		return func(t *testing.T, input string) fix.TextEdit {
			t.Helper()
			rng := span(t, input, part, skip, length)
			return fix.TextEdit{StartOffset: rng.StartOffset, EndOffset: rng.EndOffset, NewText: text}
		}
	}
	noChange := func(*testing.T, *lint.RuleContext) fixIntent { return fixIntent{} }
	headingLoses := func(part string, skip, length int) func(*testing.T, *lint.RuleContext) fixIntent {
		return func(t *testing.T, ctx *lint.RuleContext) fixIntent {
			t.Helper()
			heading := firstNode(ctx.Root, mdast.NodeHeading)
			require.NotNil(t, heading)
			return changeText(heading, textSub{rng: span(t, string(ctx.File.Content), part, skip, length)})
		}
	}

	tests := []fixCase{
		{
			name:   "whitespace inside the heading markers",
			input:  "#  Heading\n",
			edit:   replace("  ", 0, 2, " "),
			intent: noChange,
			want:   true,
		},
		{
			name:   "blank line that loosens a tight list",
			input:  "- a\n- b\n",
			edit:   replace("- b", 0, 0, "\n"),
			intent: noChange,
			want:   false,
		},
		{
			name:   "blank line in a loose list",
			input:  "- a\n\n- b\n- c\n",
			edit:   replace("- c", 0, 0, "\n"),
			intent: noChange,
			want:   true,
		},
		{
			// The window holds only some items of the list, but the list was
			// loose already.
			name:   "blank line in the middle of a long loose list",
			input:  "- a\n\n- b\n- c\n- d\n- e\n",
			edit:   replace("- d", 0, 0, "\n"),
			intent: noChange,
			want:   true,
		},
		{
			name:   "blank line in the middle of a long tight list",
			input:  "- a\n- b\n- c\n- d\n- e\n",
			edit:   replace("- d", 0, 0, "\n"),
			intent: noChange,
			want:   false,
		},
		{
			name:   "blockquote marker that adds a block to the previous item",
			input:  "- First\n\n- Text\n- > # Heading\n",
			edit:   replace("- > #", 0, 0, "  >\n"),
			intent: noChange,
			want:   false,
		},
		{
			name:   "tab deletion that moves a heading out of its list item",
			input:  "- item\n\n\t## Heading\n",
			edit:   replace("\t", 0, 1, ""),
			intent: noChange,
			want:   false,
		},
		{
			name:   "text change that the intent allows",
			input:  "# Heading.\n",
			edit:   replace(".", 0, 1, ""),
			intent: headingLoses(".", 0, 1),
			want:   true,
		},
		{
			name:   "text change without an intent",
			input:  "# Heading.\n",
			edit:   replace(".", 0, 1, ""),
			intent: noChange,
			want:   false,
		},
		{
			name:   "deletion that leaves a backslash showing",
			input:  "# Text\\!\n",
			edit:   replace("!", 0, 1, ""),
			intent: headingLoses("!", 0, 1),
			want:   false,
		},
		{
			name:   "deletion of escaped punctuation with its backslash",
			input:  "# Text\\!\n",
			edit:   replace("\\!", 0, 2, ""),
			intent: headingLoses("\\!", 0, 2),
			want:   true,
		},
		{
			// Parsed on its own, a window that starts with "---" must not
			// read as front matter, which would hide the setext heading the
			// edit makes.
			name:   "window that starts with a thematic break",
			input:  "Intro\n\n---\n# Heading\n---\n",
			edit:   replace("# Heading", 0, 2, ""),
			intent: noChange,
			want:   false,
		},
		{
			name:   "front matter at the start of the document",
			input:  "---\ntitle: x\n---\n# Heading.\n",
			edit:   replace(".", 0, 1, ""),
			intent: headingLoses(".", 0, 1),
			want:   true,
		},
		{
			name:   "byte order mark",
			input:  "\uFEFF# Heading.\n",
			edit:   replace(".", 0, 1, ""),
			intent: headingLoses(".", 0, 1),
			want:   true,
		},
		{
			name:  "strong emphasis made from spaced markers",
			input: "Some ** bold** here\n",
			edit:  replace("** bold", 2, 1, ""),
			intent: func(t *testing.T, ctx *lint.RuleContext) fixIntent {
				t.Helper()
				input := string(ctx.File.Content)
				return changeText(firstNode(ctx.Root, mdast.NodeParagraph),
					textSub{rng: span(t, input, "** bold", 0, 2), text: emphasisTokens(2, false)},
					textSub{rng: span(t, input, "** bold", 2, 1)},
					textSub{rng: span(t, input, "** here", 0, 2), text: emphasisTokens(2, true)})
			},
			want: true,
		},
		{
			name:  "paragraph split by a heading",
			input: "Text\n#Heading\nMore\n",
			edit:  replace("#Heading", 1, 0, " "),
			intent: func(t *testing.T, ctx *lint.RuleContext) fixIntent {
				t.Helper()
				hashes := span(t, string(ctx.File.Content), "#Heading", 0, 1)
				intent, ok := lineHeadingIntent(ctx, ctx.File.Lines[1], hashes.StartOffset,
					headingAttrs(1, StyleATX), false, hashes)
				require.True(t, ok)
				return intent
			},
			want: true,
		},
		{
			name:  "paragraph split into a heading of another level",
			input: "Text\n#Heading\nMore\n",
			edit:  replace("#Heading", 1, 0, " "),
			intent: func(t *testing.T, ctx *lint.RuleContext) fixIntent {
				t.Helper()
				hashes := span(t, string(ctx.File.Content), "#Heading", 0, 1)
				intent, ok := lineHeadingIntent(ctx, ctx.File.Lines[1], hashes.StartOffset,
					headingAttrs(2, StyleATX), false, hashes)
				require.True(t, ok)
				return intent
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := fixCheckContext(t, tt.input)
			edit := tt.edit(t, tt.input)
			assert.Equal(t, tt.want, fixKeepsDocument(ctx, []fix.TextEdit{edit}, tt.intent(t, ctx)))
		})
	}
}

func TestFixWindow(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		part       string
		want       string
		looseDepth int
	}{
		{
			name:       "top-level blocks",
			input:      "# One\n\nTwo\n\n## Three\n\nFour\n\nFive\n",
			part:       "Three",
			want:       "Two\n\n## Three\n\nFour\n",
			looseDepth: -1,
		},
		{
			name:       "items of a long list",
			input:      "Intro\n\n- a\n- b\n- c\n- d\n- e\n",
			part:       "c",
			want:       "- b\n- c\n- d\n",
			looseDepth: -1,
		},
		{
			name:       "items of a loose list in a blockquote",
			input:      "> - a\n>\n> - b\n>\n> - c\n>\n> - d\n",
			part:       "c",
			want:       "> - b\n>\n> - c\n>\n> - d\n",
			looseDepth: 1,
		},
		{
			name:       "first item of a list",
			input:      "Intro\n\n- a\n- b\n- c\n\nEnd\n",
			part:       "a",
			want:       "Intro\n\n- a\n- b\n- c\n\nEnd\n",
			looseDepth: -1,
		},
		{
			// Items of a list inside a list item are not reconstructed from
			// their indentation alone, so the window stays at the top level.
			name:       "items of a nested list",
			input:      "- x\n  - a\n  - b\n  - c\n  - d\n",
			part:       "b",
			want:       "- x\n  - a\n  - b\n  - c\n  - d\n",
			looseDepth: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := fixCheckContext(t, tt.input)
			edit := span(t, tt.input, tt.part, 0, len(tt.part))
			start, end, looseDepth := fixWindow(ctx.File, edit.StartOffset, edit.EndOffset)
			assert.Equal(t, tt.want, tt.input[start:end])
			assert.Equal(t, tt.looseDepth, looseDepth)
		})
	}
}

// TestFixChecker verifies that a rule keeps the fixes that pass the check
// when another of its fixes fails, and that the fixes it keeps apply.
func TestFixChecker(t *testing.T) {
	input := "# A\nText\n\n- a\n- # B\n\n# C\nMore\n"
	ctx := fixCheckContext(t, input)
	diags, err := NewHeadingBlankLinesRule().Apply(ctx)
	require.NoError(t, err)

	fixable := map[int]bool{}
	for _, diag := range diags {
		fixable[diag.StartLine] = diag.HasFix()
	}
	// The blank line above "# B" would make the list loose.
	assert.Equal(t, map[int]bool{1: true, 5: false, 7: true}, fixable)
	assert.Equal(t, "# A\n\nText\n\n- a\n- # B\n\n# C\n\nMore\n", applyAllEdits(t, input, diags))
}
