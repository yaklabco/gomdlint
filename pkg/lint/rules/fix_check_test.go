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
	want     bool
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
			check := newFixCheck(snapshot, flavor)
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
			// The tabs of a Makefile recipe are significant.
			name:  "replacing a tab in code",
			input: "```makefile\nall:\n\tgo build\n```\n",
			old:   "\tgo", new: " go",
			want: false,
		},
		{
			name:  "replacing a tab in a code span",
			input: "Some `a\tb` code.\n",
			old:   "a\tb", new: "a  b",
			want: false,
		},
		{
			name:  "replacing the tab that indents indented code",
			input: "Para\n\n\tcode\n",
			old:   "\tcode", new: "    code",
			want: true,
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
			// HTML other than br, wbr, hr, img and comments can change how
			// the text before it shows too, so no fix is kept anywhere.
			name:  "wrapping a paragraph without HTML before one with HTML",
			input: "alpha beta\n\nBefore <b>bold</b> after.\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// Whether the bold element was closed is not decided.
			name:  "wrapping a paragraph without HTML after one with HTML",
			input: "Before <b>bold</b> after.\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
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
			name:  "wrapping in a paragraph after an unclosed span",
			input: "Open <span>here\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
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
			want: false,
		},
		{
			name:  "wrapping after a script that holds a tag",
			input: "<script>\nvar x = '<div style=\"a\">';\n</script>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
	})
}

// TestFixCheckKeepsNoFixWithOtherHTML checks that a document with raw HTML
// other than whitespace, well-formed comments and br, wbr, hr and img tags
// keeps no fix, wherever the fix is, while one with only those keeps its
// fixes.
func TestFixCheckKeepsNoFixWithOtherHTML(t *testing.T) {
	runFixCheckCases(t, []fixCheckCase{
		{
			// Nothing after the div closes it.
			name:  "wrapping Markdown inside a styled div left open",
			input: "<div style=\"white-space: pre\">\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// "</textareax>" does not end the textarea.
			name:  "wrapping in a textarea after a lookalike end tag",
			input: "Open <textarea>here </textareax>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// The end tag in the attribute value does not end the pre.
			name:  "wrapping after a pre whose title holds its end tag",
			input: "<pre title=\"</pre>\">\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// Browsers end the comment at "--!>", so the pre is an element
			// for them.
			name:  "wrapping after a tag in a malformed comment",
			input: "<!-- a --!> <pre> -->\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping before an element opens",
			input: "alpha beta\n\n<div style=\"white-space: pre\">\n\ntext\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after a comment without tags",
			input: "<!-- markdownlint-disable-next-line MD013 -->\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// Void elements have no content to keep whitespace in.
			name:  "wrapping after a line break element",
			input: "Line<br>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			name:  "wrapping after an image element",
			input: "<img src=\"a.png\" alt=\"x\">\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			name:  "wrapping in a paragraph with a line break element",
			input: "Line<br/> alpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// A blank line between blocks leaves the structure as it was,
			// but the element may show it.
			name:  "inserting a blank line between paragraphs after an open div",
			input: "<div style=\"white-space: pre\">\n\nalpha\n\nbeta\n",
			old:   "alpha\n\nbeta", new: "alpha\n\n\nbeta",
			want: false,
		},
		{
			name:  "changing a container prefix after an open div",
			input: "<div style=\"white-space: pre\">\n\n>  quoted\n",
			old:   ">  quoted", new: "> quoted",
			want: false,
		},
		{
			// The line ending would join the value of the textarea.
			name:  "adding the final line ending after an open textarea",
			input: "<textarea>alpha",
			old:   "alpha", new: "alpha\n",
			want: false,
		},
		{
			name:  "wrapping after a byte order mark with a line break element",
			input: "\uFEFFalpha beta\n\n<br>\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// A style sheet can make the text before it keep its tabs.
			name:  "replacing a tab before a style element",
			input: "alpha\tbeta\n\n<style>p { white-space: pre }</style>\n",
			old:   "alpha\tbeta", new: "alpha beta",
			want: false,
		},
		{
			name:  "wrapping before a style element",
			input: "alpha beta\n\n<style>p { white-space: pre }</style>\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping before a style sheet link",
			input: "alpha beta\n\n<link rel=stylesheet href=\"a.css\">\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after a style sheet link",
			input: "<link rel=stylesheet href=\"a.css\">\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			// Browsers fold only ASCII letters, so the Kelvin sign makes
			// an unknown element, not a link.
			name:  "wrapping after a tag named with a Kelvin sign",
			input: "<hr><lin\u212a>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after a comment holding two hyphens",
			input: "<!-- a -- b -->\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after the end tag of a line break",
			input: "</br>\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after an image whose quoted value spans lines",
			input: "Text <img alt=\"a\nb\"> here\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping with only line breaks, images and comments",
			input: "<br>\n\n<img src=\"x.png\" alt=\"a b\">\n\n<!-- markdownlint-disable MD013 -->\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
		{
			// An event handler can set styles on the whole document.
			name:  "wrapping before an image with an error handler",
			input: "alpha beta\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "replacing a tab before an image with an error handler",
			input: "alpha\tbeta\n\n<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">\n",
			old:   "alpha\tbeta", new: "alpha beta",
			want: false,
		},
		{
			name:  "wrapping before an image with an error handler in capitals",
			input: "alpha beta\n\n<img src=x ONERROR=\"document.body.style.whiteSpace='pre'\">\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after an image with a style",
			input: "<img src=\"x.png\" style=\"display: block\">\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping after a line break with a class",
			input: "Text<br class=\"x\"> more\n\nalpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: false,
		},
		{
			name:  "wrapping with the presentational attributes of images and rules",
			input: "<img SRC=\"x.png\" Alt=\"a b\" width=10 height=\"20\" loading=lazy>\n\n<hr align=center noshade>\n\nText<br clear=all> alpha beta\n",
			old:   "alpha beta", new: "alpha\nbeta",
			want: true,
		},
	})
}

// TestKeepSafeFixesKeepsNoFixWithOtherHTML checks that a document with raw
// HTML other than whitespace, well-formed comments and br, wbr, hr and img
// tags keeps no fix at all, before or after the HTML, while a document with
// only those keeps its fixes.
func TestKeepSafeFixesKeepsNoFixWithOtherHTML(t *testing.T) {
	tests := []struct {
		html string
		keep bool
	}{
		{html: "<div style=\"white-space: pre\">", keep: false},
		{html: "<link rel=stylesheet href=\"a.css\">", keep: false},
		{html: "<br>\n<img src=\"x.png\" alt=\"a b\">\n<!-- markdownlint-disable MD013 -->", keep: true},
	}
	for _, tt := range tests {
		input := "alpha \nbeta\n\n" + tt.html + "\n\ngamma \ndelta"
		diags := []lint.Diagnostic{
			{RuleID: "T1", FixEdits: []fix.TextEdit{replaceOnce(t, input, "alpha \n", "alpha\n")}},
			{RuleID: "T1", FixEdits: []fix.TextEdit{replaceOnce(t, input, "gamma \n", "gamma\n")}},
			{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: len(input), EndOffset: len(input), NewText: "\n"}}},
		}
		snapshot := parseForCheck(t, input)
		ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)

		kept := keepSafeFixes(ctx, diags)
		require.Len(t, kept, len(diags))
		for idx, diag := range kept {
			assert.Equal(t, tt.keep, diag.HasFix(), "%q fix %d", tt.html, idx)
		}
	}
}

// TestBenignHTML checks which raw HTML can only show what the render
// signature compares.
func TestBenignHTML(t *testing.T) {
	tests := map[string]bool{
		"":                                true,
		" \t\n":                           true,
		"<br>":                            true,
		"<BR/>":                           true,
		"<br />":                          true,
		"<Wbr>":                           true,
		"<hr>":                            true,
		"<img src=\"x.png\" alt=\"a b\">": true,
		"<img src='x.png' alt=x>":         true,
		"<img\nsrc=\"x.png\"\n/>":         true,
		"<img SRC=\"x.png\" Alt=b>":       true,
		"<br clear=all>":                  true,
		"<hr align=center width=50% size=2 noshade>": true,
		"<img src=\"x.png\" alt=\"a b\" title='t' width=10 height=20 align=left border=0 loading=lazy decoding=async srcset=\"x.png 1x\" sizes=100vw>": true,
		"<img data-x_y.z:w=\"1\" hidden>": false,
		"<img src=\"data:image/png,broken\" onerror=\"document.body.style.whiteSpace='pre'\">": false,
		"<img src=x ONERROR=y>":                 false,
		"<img src=x onload=y>":                  false,
		"<img style=\"display: block\">":        false,
		"<img id=x>":                            false,
		"<img data-x=1>":                        false,
		"<img src=a src=b>":                     false,
		"<img src=a SRC=b>":                     false,
		"<br class=x>":                          false,
		"<br style=x>":                          false,
		"<wbr clear=all>":                       false,
		"<hr class=x>":                          false,
		"<img xml:lang=en>":                     false,
		"<!-- markdownlint-disable MD013 -->":   true,
		"<!---->":                               true,
		"<!--\nmulti\nline\n-->":                true,
		"<br>\n<!-- c -->\n<img src=x>\n":       true,
		"<link rel=stylesheet>":                 false,
		"<style>p { white-space: pre }</style>": false,
		"</br>":                                 false,
		"<b>":                                   false,
		"<div>":                                 false,
		"<textarea/>":                           false,
		"<lin\u212a>":                           false,
		"<hr><lin\u212a>":                       false,
		"<img alt=\"a\nb\">":                    false,
		"<img alt=a\"b>":                        false,
		"<img alt=a`b>":                         false,
		"<img =x>":                              false,
		"<img src=x":                            false,
		"<imgx>":                                false,
		"<br\v>":                                false,
		"<br> text":                             false,
		"<!-- a -- b -->":                       false,
		"<!-->":                                 false,
		"<!--->":                                false,
		"<!--->-->":                             false,
		"<!-- a --!> <pre> -->":                 false,
		"<!-- open":                             false,
		"<?php echo 1; ?>":                      false,
		"<!DOCTYPE html>":                       false,
		"<![CDATA[x]]>":                         false,
	}
	for raw, want := range tests {
		assert.Equal(t, want, benignHTML([]byte(raw)), "%q", raw)
	}
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
			check := newFixCheck(snapshot, config.FlavorGFM)
			assert.Equal(t, tt.want, check.keeps([]fix.TextEdit{replaceOnce(t, tt.input, tt.old, tt.new)}))
		})
	}

	t.Run("overlapping edits", func(t *testing.T) {
		snapshot := parseForCheck(t, "# One\n\ntext\n")
		check := newFixCheck(snapshot, config.FlavorGFM)
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
	check := newFixCheck(snapshot, config.FlavorCommonMark)
	require.True(t, check.keeps(diags[0].FixEdits), "the first fix is safe on its own")
	require.True(t, check.keeps(diags[1].FixEdits), "the second fix is safe on its own")

	kept := keepSafeFixes(ctx, diags)
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
	kept := keepSafeFixes(ctx, diags)
	require.Len(t, kept, 3)
	assert.False(t, kept[0].HasFix(), "removing a hard break changes the rendering")
	assert.True(t, kept[1].HasFix())
	assert.False(t, kept[2].HasFix())
}

// TestKeepSafeFixesDropsFixesThatChangeNothing checks that a fix whose edits
// leave the content as it was is removed while its diagnostic stays: an
// empty deletion, like the one MD012 offered at the end of a file, a
// replacement by the same text, and edits that undo each other.
func TestKeepSafeFixesDropsFixesThatChangeNothing(t *testing.T) {
	input := "Text\n\n"
	end := len(input)
	diags := []lint.Diagnostic{
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: end, EndOffset: end}}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{replaceOnce(t, input, "Text", "Text")}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: 0, EndOffset: 1}, {StartOffset: 1, EndOffset: 1, NewText: "T"}}},
		{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: end - 1, EndOffset: end}}},
	}
	snapshot := parseForCheck(t, input)
	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)

	kept := keepSafeFixes(ctx, diags)
	require.Len(t, kept, len(diags))
	assert.False(t, kept[0].HasFix(), "an empty deletion changes nothing")
	assert.False(t, kept[1].HasFix(), "replacing text by the same text changes nothing")
	assert.False(t, kept[2].HasFix(), "edits that undo each other change nothing")
	assert.True(t, kept[3].HasFix(), "removing the extra line ending changes the content")
}

// TestKeepSafeFixesDropsFixesThatChangeNothingInBatches checks the same for a
// rule with many fixes, where a fix that changes nothing would otherwise pass
// with the batch of fixes it is checked in, or on its own.
func TestKeepSafeFixesDropsFixesThatChangeNothingInBatches(t *testing.T) {
	var input strings.Builder
	input.WriteString("# Title\n\n")
	var diags []lint.Diagnostic
	// Each harmless fix removes the single space before a soft break in a
	// paragraph of its own.
	const harmless = 18
	for idx := range harmless {
		input.WriteString("line" + strconv.Itoa(idx))
		start := input.Len()
		input.WriteString(" \nmore\n\n")
		diags = append(diags, lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: start, EndOffset: start + 1}}})
	}
	end := input.Len()
	diags = append(diags,
		lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{replaceOnce(t, input.String(), "Title", "Title")}},
		lint.Diagnostic{RuleID: "T1", FixEdits: []fix.TextEdit{{StartOffset: end, EndOffset: end}}},
	)
	require.Greater(t, len(diags), individualFixChecks, "the batched path is taken")
	snapshot := parseForCheck(t, input.String())
	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)

	kept := keepSafeFixes(ctx, diags)
	for idx := range harmless {
		assert.True(t, kept[idx].HasFix(), "harmless fix %d", idx)
	}
	assert.False(t, kept[harmless].HasFix(), "replacing the heading text by the same text changes nothing")
	assert.False(t, kept[harmless+1].HasFix(), "an empty deletion at the end of the file changes nothing")
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

	kept := keepSafeFixes(ctx, diags)
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
	check := newFixCheck(snapshot, config.FlavorGFM)
	require.False(t, check.keeps(diags[0].FixEdits), "the opening fence alone breaks the document")
	require.False(t, check.keeps(diags[1].FixEdits), "the closing fence alone breaks the document")
	require.True(t, check.keepsTogether(diags, []int{0, 1}), "both fences together keep the document")

	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
	kept := keepSafeFixes(ctx, diags)
	assert.False(t, kept[0].HasFix(), "the opening fence fix breaks the document alone")
	assert.False(t, kept[1].HasFix(), "the closing fence fix breaks the document alone")
	for idx := 2; idx < len(kept); idx++ {
		assert.True(t, kept[idx].HasFix(), "harmless fix %d", idx)
	}
}
