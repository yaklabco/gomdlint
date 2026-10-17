package rules

import (
	"context"
	"strconv"
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
	return parseForCheckWith(t, config.FlavorGFM, input)
}

func parseForCheckWith(t *testing.T, flavor config.Flavor, input string) *mdast.FileSnapshot {
	t.Helper()
	snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	return snapshot
}

// fixCheckCase is one candidate edit and whether the check keeps it.
type fixCheckCase struct {
	name     string
	flavor   config.Flavor
	input    string
	old, new string
	// codeTabSpaces is the number of spaces a tab in code may become.
	codeTabSpaces int
	want          bool
}

// runFixCheckCases checks each edit on its own against its input.
func runFixCheckCases(t *testing.T, tests []fixCheckCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := tt.flavor
			if flavor == "" {
				flavor = config.FlavorGFM
			}
			snapshot := parseForCheckWith(t, flavor, tt.input)
			check := newFixCheck(snapshot, flavor, fixCheckOptions{codeTabSpaces: tt.codeTabSpaces})
			assert.Equal(t, tt.want, check.keeps([]fix.TextEdit{replaceOnce(t, tt.input, tt.old, tt.new)}))
		})
	}
}

func TestFixCheckKeeps(t *testing.T) {
	runFixCheckCases(t, []fixCheckCase{
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
			name:  "replacing a tab in code where allowed",
			input: "```\ncode\there\n```\n",
			old:   "code\there", new: "code here",
			codeTabSpaces: 1,
			want:          true,
		},
		{
			name:  "replacing a tab in a code span where allowed",
			input: "Some `a\tb` code.\n",
			old:   "a\tb", new: "a  b",
			codeTabSpaces: 2,
			want:          true,
		},
		{
			name:  "changing other code whitespace where tabs may change",
			input: "```\ncode  here\n```\n",
			old:   "code  here", new: "code here",
			codeTabSpaces: 1,
			want:          false,
		},
		{
			name:  "spacing in a link title",
			input: "A [l](/u \"a  b\") here.\n",
			old:   "a  b", new: "a b",
			want: false,
		},
		{
			name:  "spacing in image alt text",
			input: "An ![a  b](x.png) image.\n",
			old:   "a  b", new: "a b",
			want: false,
		},
		{
			name:  "replacing a non-breaking space",
			input: "a\u00a0b\n",
			old:   "\u00a0", new: " ",
			want: false,
		},
		{
			// The definition is far from the link that uses it.
			name:  "changing a definition used outside its blocks",
			input: "[x][r]\n\n# A\n\n# B\n\n[r]: </a\tb>\n",
			old:   "\t", new: " ",
			want: false,
		},
		{
			name:  "removing a trailing space after a definition",
			input: "[x][r]\n\n# A\n\n# B\n\n[r]: /a \n",
			old:   "/a \n", new: "/a\n",
			want: true,
		},
		{
			// GFM renderers read the checkbox even when the file is linted
			// as CommonMark.
			name:   "breaking a task checkbox under CommonMark",
			flavor: config.FlavorCommonMark,
			input:  "- [ ] task\n",
			old:    "[ ]", new: "[\n  ]",
			want: false,
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
	})
}

// TestFixCheckKeepsRawHTML covers raw HTML, which renderers pass through as
// written, and the text of the elements it opens.
func TestFixCheckKeepsRawHTML(t *testing.T) {
	runFixCheckCases(t, []fixCheckCase{
		{
			name:  "trailing spaces in a pre HTML block",
			input: "<pre>\nalpha   \nbeta\n</pre>\n",
			old:   "alpha   \n", new: "alpha\n",
			want: false,
		},
		{
			name:  "tab in an HTML block",
			input: "<div>\n\tx\n</div>\n",
			old:   "\tx", new: " x",
			want: false,
		},
		{
			name:  "spacing in an inline HTML attribute",
			input: "A <span title=\"a  b\">x</span> here.\n",
			old:   "a  b", new: "a b",
			want: false,
		},
		{
			name:  "trailing space inside an inline HTML tag",
			input: "A <a href=\"x\"  \ntitle=\"y\">z</a> here.\n",
			old:   "\"x\"  \n", new: "\"x\"\n",
			want: false,
		},
		{
			// A textarea shows its text with the line break.
			name:  "wrapping inside an inline textarea",
			input: "Before <textarea>alpha beta gamma</textarea> after.\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:  "wrapping inside an inline pre element",
			input: "Before <pre>alpha beta</pre> after.\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping inside a span with a style",
			input: "Before <span style=\"white-space: pre\">alpha beta</span> after.\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// CSS can change how any element treats whitespace.
			name:  "wrapping inside nested inline elements",
			input: "Before <b><i>alpha beta</i></b> after.\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// The element runs to the end of the paragraph.
			name:  "wrapping after an unclosed inline element",
			input: "Before <span>alpha beta gamma\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			// A paragraph with raw HTML must stay byte for byte the same.
			name:  "wrapping elsewhere in a paragraph with inline HTML",
			input: "Before <b>bold</b> alpha beta after.\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping a paragraph without HTML next to one with HTML",
			input: "Before <b>bold</b> after.\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// The tags in the textarea are text, which no tag matching
			// can see; the paragraph is compared byte for byte.
			name:  "wrapping a textarea that holds literal tags",
			input: "Before <b><textarea>alpha </b> beta gamma</textarea></b> after.\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			// HTML does not self-close a textarea.
			name:  "wrapping after a self-closed textarea",
			input: "Before <textarea/>alpha beta gamma</textarea> after.\n",
			old:   "beta gamma", new: "beta\ngamma",
			want: false,
		},
		{
			name:  "changing a heading with inline HTML",
			input: "# Title <b>x</b>  with spaces\n",
			old:   "  with", new: " with",
			want: false,
		},
		{
			name:  "changing a table cell with inline HTML",
			input: "| a <b>x</b>  y | b |\n| - | - |\n| 1 | 2 |\n",
			old:   "  y", new: " y",
			want: false,
		},
		{
			// The end of the first paragraph closes the span.
			name:  "wrapping in a paragraph after an unclosed span",
			input: "Open <span>here\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// A textarea takes all the HTML that follows it as its text.
			name:  "wrapping in a paragraph after an unclosed textarea",
			input: "Open <textarea>here\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping Markdown inside a styled HTML block",
			input: "<div style=\"white-space: pre\">\n\nalpha beta\n\n</div>\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// The HTML blocks may open and close an element around it.
			name:  "wrapping Markdown inside a details block",
			input: "<details>\n\nalpha beta\n\n</details>\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping Markdown closed by inline HTML",
			input: "<div style=\"white-space: pre\">\n\nalpha beta\n\nend </div>\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping Markdown after a closed HTML block",
			input: "<div>x</div>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// The div inside the script is script text, not an element.
			name:  "wrapping after a script that holds a tag",
			input: "<script>\nvar x = '<div style=\"a\">';\n</script>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
	})
}

// TestFixCheckSurvivesUnbalancedHTML runs every fixing rule of this file set
// over raw HTML whose tags do not match, which must not crash the fix check.
func TestFixCheckSurvivesUnbalancedHTML(t *testing.T) {
	inputs := []string{
		"<div>\n\nhello </div>",
		"</div></div>\n\ntext  \nmore\n",
		"<b>\n\n</b></b> text\n",
		"> <span>\n>\n> </span></span> x\n",
		"Before <b><textarea>alpha </b> beta</textarea></b> after.\n",
		"Before <textarea/>alpha beta</textarea> after.\n",
	}
	rules := []lint.Rule{
		NewUnorderedListStyleRule(), NewListIndentRule(), NewULIndentRule(), NewTrailingWhitespaceRule(),
		NewHardTabsRule(), NewMultipleBlankLinesRule(), NewMaxLineLengthRule(), NewNoMultipleSpaceBlockquoteRule(),
		NewOrderedListIncrementRule(), NewListMarkerSpaceRule(), NewBlanksAroundListsRule(), NewFinalNewlineRule(),
	}
	for _, input := range inputs {
		snapshot := parseForCheck(t, input)
		for _, rule := range rules {
			assert.NotPanics(t, func() {
				_, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
				assert.NoError(t, err)
			}, "%s on %q", rule.ID(), input)
		}
	}
}

// TestFixCheckWholeDocument checks the effects a fix can have far from the
// text it changes.
func TestFixCheckWholeDocument(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		old, new string
		want     bool
	}{
		{
			name:  "edit that pulls the next block into a list",
			input: "# One\n\n- item\n\n  in item\n\npara three\n",
			old:   "\npara three", new: "\n  para three",
			want: false,
		},
		{
			// GitHub turns each space into a hyphen, so the anchor of the
			// heading and the link to it would change.
			name:  "changing the anchor of a heading",
			input: "# Foo  Bar\n\n# A\n\n# B\n\nSee [it](#foo--bar).\n",
			old:   "Foo  Bar", new: "Foo Bar",
			want: false,
		},
		{
			// A tab is dropped from the anchor, a space becomes a hyphen.
			name:  "replacing a tab in a heading",
			input: "# Foo\tBar\n\ntext\n",
			old:   "Foo\tBar", new: "Foo Bar",
			want: false,
		},
		{
			// The second heading takes the anchor of the first, which
			// shifts the numbered anchors of the duplicates.
			name:  "changing which heading gets a duplicate anchor",
			input: "# a  b\n\n# a b\n\n# a b\n\n[x](#a-b-1)\n",
			old:   "# a  b", new: "# a b",
			want: false,
		},
		{
			name:  "turning a thematic break into front matter",
			input: "***\n\ntitle: abc\n\n---\n",
			old:   "***", new: "---",
			want: false,
		},
		{
			name:  "edit at the end of the file",
			input: "# One\n\ntext",
			old:   "text", new: "text\n",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := parseForCheck(t, tt.input)
			check := newFixCheck(snapshot, config.FlavorGFM, fixCheckOptions{})
			assert.Equal(t, tt.want, check.keeps([]fix.TextEdit{replaceOnce(t, tt.input, tt.old, tt.new)}))
		})
	}

	t.Run("overlapping edits", func(t *testing.T) {
		snapshot := parseForCheck(t, "# One\n\ntext\n")
		check := newFixCheck(snapshot, config.FlavorGFM, fixCheckOptions{})
		assert.False(t, check.keeps([]fix.TextEdit{
			{StartOffset: 2, EndOffset: 5, NewText: "x"},
			{StartOffset: 4, EndOffset: 6, NewText: "y"},
		}))
	})
}

// TestKeepSafeFixesChecksTheirUnion checks that fixes that are safe on their
// own but not together are not all kept.
func TestKeepSafeFixesChecksTheirUnion(t *testing.T) {
	// Either thematic break may become "---", but both together make the
	// lines between them front matter.
	input := "***\n\ntitle: abc\n\n***\n"
	snapshot := parseForCheck(t, input)
	diags := []lint.Diagnostic{
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 3, NewText: "---"}}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: len(input) - 4, EndOffset: len(input) - 1, NewText: "---"}}},
	}
	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
	check := newFixCheck(snapshot, config.FlavorCommonMark, fixCheckOptions{})
	require.True(t, check.keeps(diags[0].FixEdits), "the first fix is safe on its own")
	require.True(t, check.keeps(diags[1].FixEdits), "the second fix is safe on its own")

	kept := keepSafeFixes(ctx, diags, fixCheckOptions{})
	assert.True(t, kept[0].HasFix())
	assert.False(t, kept[1].HasFix(), "the second fix would make front matter together with the first")
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

// TestKeepSafeFixesInBatches checks the batched path for a rule with many
// fixes, each alone in its paragraph: removing the spaces of a hard break is
// dropped, removing a single space before a soft break is kept.
func TestKeepSafeFixesInBatches(t *testing.T) {
	var input strings.Builder
	var diags []lint.Diagnostic
	var wantFix []bool
	const pairs = 12
	for idx := range pairs {
		for _, line := range []string{"soft" + strconv.Itoa(idx) + " ", "hard" + strconv.Itoa(idx) + "  "} {
			start := input.Len() + len(strings.TrimRight(line, " "))
			input.WriteString(line + "\nnext\n\n")
			diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: start, EndOffset: start + len(line) - len(strings.TrimRight(line, " "))}}})
			wantFix = append(wantFix, strings.HasPrefix(line, "soft"))
		}
	}
	require.Greater(t, len(diags), individualFixChecks, "the batched path is taken")
	snapshot := parseForCheck(t, input.String())
	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)

	kept := keepSafeFixes(ctx, diags, fixCheckOptions{})
	for idx, diag := range kept {
		assert.Equal(t, wantFix[idx], diag.HasFix(), "diagnostic %d", idx)
	}
}

// TestKeepSafeFixesChecksDependentFixesAlone checks that a batch that passes
// as a whole cannot carry a fix that breaks the document on its own: the
// engine may apply only some fixes, after a directive, a conflict or the
// last pass. Changing only the opening or only the closing fence of a code
// block leaves it unclosed, so it swallows the rest of the document, but
// changing both keeps it.
func TestKeepSafeFixesChecksDependentFixesAlone(t *testing.T) {
	var input strings.Builder
	input.WriteString("# Title\n\n")
	opening := input.Len()
	input.WriteString("```\ncode\n")
	closing := input.Len()
	input.WriteString("```\n")
	diags := []lint.Diagnostic{
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: opening, EndOffset: opening + 3, NewText: "~~~"}}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: closing, EndOffset: closing + 3, NewText: "~~~"}}},
	}
	// Each harmless fix removes the single space before a soft break in a
	// paragraph of its own.
	const harmless = 18
	for idx := range harmless {
		input.WriteString("\nline" + strconv.Itoa(idx))
		start := input.Len()
		input.WriteString(" \nmore\n")
		diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: start, EndOffset: start + 1}}})
	}
	require.Greater(t, len(diags), individualFixChecks, "the batched path is taken")

	snapshot := parseForCheck(t, input.String())
	check := newFixCheck(snapshot, config.FlavorGFM, fixCheckOptions{})
	require.False(t, check.keeps(diags[0].FixEdits), "the opening fence alone breaks the document")
	require.False(t, check.keeps(diags[1].FixEdits), "the closing fence alone breaks the document")
	require.True(t, check.keepsTogether(diags, []int{0, 1}), "both fences together keep the document")

	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
	kept := keepSafeFixes(ctx, diags, fixCheckOptions{})
	assert.False(t, kept[0].HasFix(), "the opening fence fix breaks the document alone")
	assert.False(t, kept[1].HasFix(), "the closing fence fix breaks the document alone")
	for idx := 2; idx < len(kept); idx++ {
		assert.True(t, kept[idx].HasFix(), "harmless fix %d", idx)
	}
}
