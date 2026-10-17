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

// TestFixLayers groups fixes that can depend on each other. An edit that
// stays on one line of one leaf block can only change that block, so it is
// grouped with fixes in the same block; any other edit can change what a
// top-level block shares, such as a list's tightness, so it is grouped with
// fixes in the same top-level block.
func TestFixLayers(t *testing.T) {
	sixItems := "- one\n- two\n- three\n- four\n- five\n- six\n"
	nested := "- a\n  sec\n  three\n- b\n- c\n"
	tests := []struct {
		name  string
		input string
		diags []lint.Diagnostic
		want  [][]int
	}{
		{
			name:  "edits on lines of different items stay apart",
			input: sixItems,
			diags: []lint.Diagnostic{edit(9, 10, "W"), edit(23, 25, "U")},
		},
		{
			name:  "edits in one item depend on each other",
			input: "- one two\n",
			diags: []lint.Diagnostic{edit(3, 4, "N"), edit(7, 8, "W")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "line breaks far apart in one list depend on each other",
			input: sixItems,
			diags: []lint.Diagnostic{edit(6, 6, "\n"), edit(34, 34, "\n"), edit(15, 18, "x")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "an edit that empties its line is not local",
			input: nested,
			diags: []lint.Diagnostic{edit(6, 9, ""), edit(22, 22, "\n")},
			want:  [][]int{{0}, {1}},
		},
		{
			name:  "an edit that leaves its line with text is local",
			input: nested,
			diags: []lint.Diagnostic{edit(6, 8, ""), edit(22, 22, "\n")},
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
