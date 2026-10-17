package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// fixSafetyContext parses input with flavor and returns a rule context whose
// configuration names the same flavor.
func fixSafetyContext(t *testing.T, flavor config.Flavor, input string) *lint.RuleContext {
	t.Helper()
	snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	return lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
}

// edit returns a diagnostic whose fix replaces input[start:end] with text.
func edit(start, end int, text string) lint.Diagnostic {
	return lint.Diagnostic{RuleID: "TEST", FixEdits: []fix.TextEdit{{StartOffset: start, EndOffset: end, NewText: text}}}
}

func TestVerifyFixes(t *testing.T) {
	tests := []struct {
		name   string
		flavor config.Flavor
		input  string
		allow  fixAllowance
		diags  []lint.Diagnostic
		kept   []bool
	}{
		{
			name:  "blank line between paragraphs keeps the structure",
			input: "One\n\nTwo\n",
			diags: []lint.Diagnostic{edit(5, 5, "\n")},
			kept:  []bool{true},
		},
		{
			name:  "blank line in a tight list makes it loose",
			input: "- a\n- b\n",
			diags: []lint.Diagnostic{edit(4, 4, "\n")},
			kept:  []bool{false},
		},
		{
			name:  "only the fix that splits a paragraph is withheld",
			input: "One\ntwo\n\nThree\nfour\n",
			diags: []lint.Diagnostic{edit(9, 9, "\n"), edit(4, 4, "\n"), edit(15, 15, "\n")},
			kept:  []bool{true, false, false},
		},
		{
			name:  "changing a word changes the text",
			input: "Hello world\n",
			diags: []lint.Diagnostic{edit(6, 11, "there")},
			kept:  []bool{false},
		},
		{
			name:  "info string may change when allowed",
			input: "```\ncode\n```\n",
			allow: fixAllowance{codeInfo: true},
			diags: []lint.Diagnostic{edit(3, 3, "go")},
			kept:  []bool{true},
		},
		{
			name:  "info string may not change otherwise",
			input: "```\ncode\n```\n",
			diags: []lint.Diagnostic{edit(3, 3, "go")},
			kept:  []bool{false},
		},
		{
			name:  "text may become an autolink to the same address",
			input: "See https://example.com now.\n",
			allow: fixAllowance{autolinks: true},
			diags: []lint.Diagnostic{edit(4, 23, "<https://example.com>")},
			kept:  []bool{true},
		},
		{
			name:  "brackets that end the autolink early are withheld",
			input: "See https://example.com/a>b now.\n",
			allow: fixAllowance{autolinks: true},
			diags: []lint.Diagnostic{edit(4, 27, "<https://example.com/a>b>")},
			kept:  []bool{false},
		},
		{
			name:   "table delimiter dashes do not change the table",
			flavor: config.FlavorGFM,
			input:  "| a | b |\n| - | :- |\n| 1 | 2 |\n",
			diags:  []lint.Diagnostic{edit(12, 13, "---"), edit(16, 18, ":---")},
			kept:   []bool{true, true},
		},
		{
			name:   "table alignment may not change",
			flavor: config.FlavorGFM,
			input:  "| a | b |\n| - | - |\n| 1 | 2 |\n",
			diags:  []lint.Diagnostic{edit(16, 17, "--:")},
			kept:   []bool{false},
		},
		{
			name:  "a reference label may not change, even to a label with the same destination",
			input: "[` foo`][]\n\n[` foo`]: /url\n[`foo`]: /url\n",
			allow: fixAllowance{codeSpanSpace: true},
			diags: []lint.Diagnostic{edit(2, 3, "")},
			kept:  []bool{false},
		},
		{
			name:  "removing a definition a link uses is withheld",
			input: "[a][]\n\n[a]: /x\n",
			diags: []lint.Diagnostic{edit(7, 15, "")},
			kept:  []bool{false},
		},
		{
			name:  "removing an unused definition keeps every link",
			input: "[a][]\n\n[a]: /x\n[b]: /y\n",
			diags: []lint.Diagnostic{edit(15, 23, "")},
			kept:  []bool{true},
		},
		{
			name:  "an autolink must show the text it replaces, entities decoded",
			input: "See https://example.com/a&copy;b now.\n",
			allow: fixAllowance{autolinks: true},
			diags: []lint.Diagnostic{edit(4, 32, "<https://example.com/a&copy;b>")},
			kept:  []bool{false},
		},
		{
			name:  "a blank line taken in by an unclosed fence is code",
			input: "- a\n  ~~~`\n- ~~~ bash\n  x\n  ~~~\n\n- c\n",
			diags: []lint.Diagnostic{edit(11, 11, "\n")},
			kept:  []bool{false},
		},
		{
			name:  "fixes that are only safe together are withheld",
			input: "*a*\n",
			diags: []lint.Diagnostic{edit(0, 1, "_"), edit(2, 3, "_")},
			kept:  []bool{false, false},
		},
		{
			name:  "fixes that depend on each other are withheld among many",
			input: "*a*\n\nb\n\nc\n\nd\n\ne\n\nf\n\ng\n\nh\n",
			diags: []lint.Diagnostic{
				edit(0, 1, "_"), edit(2, 3, "_"),
				edit(5, 5, "\n"), edit(8, 8, "\n"), edit(11, 11, "\n"), edit(14, 14, "\n"),
				edit(17, 17, "\n"), edit(20, 20, "\n"), edit(23, 23, "\n"),
			},
			kept: []bool{false, false, true, true, true, true, true, true, true},
		},
		{
			name:  "fixes in one list share its tightness",
			input: "- a\n\n- b\n- c\n- d\n- e\n- f\n",
			diags: []lint.Diagnostic{edit(4, 5, ""), edit(21, 21, "\n")},
			kept:  []bool{false, true},
		},
		{
			name:  "a definition moved between paragraphs of one list is withheld",
			input: "- [x]X /url\n\n  [x]X /url\n\n  [x]: /url\n",
			diags: []lint.Diagnostic{edit(5, 6, ":"), edit(31, 32, "X")},
			kept:  []bool{false, false},
		},
		{
			name:  "front matter may not change",
			input: "---\ntitle: x\n---\n\nText\n",
			diags: []lint.Diagnostic{edit(11, 12, "y")},
			kept:  []bool{false},
		},
		{
			name:  "a heading anchor may not change",
			input: "# ` x`\n\n[link](#-x)\n",
			allow: fixAllowance{codeSpanSpace: true},
			diags: []lint.Diagnostic{edit(3, 4, "")},
			kept:  []bool{false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := tt.flavor
			if flavor == "" {
				flavor = config.FlavorCommonMark
			}
			diags := verifyFixes(fixSafetyContext(t, flavor, tt.input), tt.diags, tt.allow)
			require.Len(t, diags, len(tt.kept))
			for idx, kept := range tt.kept {
				assert.Equal(t, kept, diags[idx].HasFix(), "diagnostic %d", idx)
			}
		})
	}
}

func TestStripColumns(t *testing.T) {
	tests := []struct {
		line    string
		columns int
		want    string
	}{
		{"    code", 4, "code"},
		{"      code", 4, "  code"},
		{"  code", 4, "code"},
		{"\tcode", 4, "code"},
		{"\tcode", 2, "  code"},
		{"   ", 4, ""},
		{"code", 0, "code"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, stripColumns(tt.line, tt.columns), "stripColumns(%q, %d)", tt.line, tt.columns)
	}
}

// TestFixLayers groups fixes that can depend on each other. An edit is
// local when it cannot change block syntax: it stays on one line of a
// paragraph, heading, code block, or table that does not start with "[",
// after the first word of the line, adds and removes no line breaks, pipes,
// or backslashes, and leaves the line with text. A local edit depends on the
// other fixes in its leaf block; any other edit depends on every fix in the
// top-level blocks it touches, local ones included.
func TestFixLayers(t *testing.T) {
	multi := "- one more\n- two more\n- three more\n- four more\n- five more\n- six more\n"
	tests := []struct {
		name  string
		input string
		diags []lint.Diagnostic
		want  [][]int
	}{
		{
			name:  "local edits in different items stay apart",
			input: multi,
			diags: []lint.Diagnostic{edit(7, 8, "O"), edit(31, 32, "O")},
		},
		{
			name:  "local edits on one line depend on each other",
			input: "- one two three\n",
			diags: []lint.Diagnostic{edit(7, 8, "W"), edit(11, 12, "H")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "a line break depends on every edit in its list",
			input: multi,
			diags: []lint.Diagnostic{edit(11, 11, "\n"), edit(59, 59, "\n"), edit(31, 32, "O")},
			want:  [][]int{{0}, {1}, {2}},
		},
		{
			name:  "an edit in the first word of a line is not local",
			input: multi,
			diags: []lint.Diagnostic{edit(3, 4, "N"), edit(31, 32, "O")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "an edit that adds a pipe is not local",
			input: multi,
			diags: []lint.Diagnostic{edit(7, 7, "|"), edit(31, 32, "O")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "an edit that removes a backslash is not local",
			input: "- one \\*more\n- two more\n",
			diags: []lint.Diagnostic{edit(6, 7, ""), edit(20, 21, "O")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "an edit in a paragraph that starts with a bracket is not local",
			input: "- [a] b c d\n- e f g\n",
			diags: []lint.Diagnostic{edit(8, 9, "C"), edit(16, 17, "F")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "the same edits without the bracket stay apart",
			input: "- a] b c d\n- e f g\n",
			diags: []lint.Diagnostic{edit(7, 8, "C"), edit(15, 16, "F")},
		},
		{
			name:  "an edit that empties its line is not local",
			input: "- a\n  sec\n  three\n- b\n- c\n",
			diags: []lint.Diagnostic{edit(6, 9, ""), edit(22, 22, "\n")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "edits in different top-level blocks stay apart",
			input: "One two three\nfour\n\nFive six seven\n",
			diags: []lint.Diagnostic{edit(5, 6, "W\n"), edit(26, 27, "I")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := fixSafetyContext(t, config.FlavorCommonMark, tt.input)
			fixable := make([]int, len(tt.diags))
			for idx := range fixable {
				fixable[idx] = idx
			}
			assert.Equal(t, tt.want, fixLayers(ctx.File, ctx.Root, tt.diags, fixable))
		})
	}
}
