package rules

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/lint"
)

// TestLinkRulesMatchMarkdownlintAtCheckboxes holds the documents of the
// differential test against markdownlint 0.41.1 where list items and
// headings in list items start with brackets that goldmark's own task list
// extension reads as a checkbox and GFM does not, with each finding of the
// link rules that markdownlint gives (with MD052's shortcut_syntax and
// MD054 disallowing every reference style). markdownlint reads no
// checkboxes, so each pair of brackets there is text or a link to it.
func TestLinkRulesMatchMarkdownlintAtCheckboxes(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "# T\n\n- ## [ ](/url).\n", want: []string{"MD039 3:7", "MD039 3:7"}},
		{input: "# T\n\n- [x](https://example.com)\n", want: []string{}},
		{input: "# T\n\n- [ ](https://example.org)\n", want: []string{"MD039 3:4", "MD039 3:4"}},
		{input: "# T\n\n- # [X]\n\n[x]: #\n", want: []string{}},
		{input: "# T\n\n- # [x]\n\n[x]: #\n", want: []string{"MD042 3:5", "MD054 3:5"}},
		{input: "# T\n\n- ## [x][y] done\n", want: []string{"MD052 3:6"}},
		{input: "# T\n\n- [ ][y] done\n", want: []string{"MD052 3:6"}},
		{input: "# T\n\n> - ## [X][y] done\n\n[Y]: #\n", want: []string{"MD042 3:8", "MD054 3:8"}},
		{input: "# T\n\n- ## [ ](/url).\r\n", want: []string{"MD039 3:7", "MD039 3:7"}},
		{input: "# T\r\r- [x][Y]\r\r[y]: #\r", want: []string{}},
		{input: "# T\n\n- [x]\ttext [ y ](#)\n", want: []string{"MD039 3:13", "MD039 3:15", "MD042 3:12"}},
		{input: "# T\n\n- [ ](/u)\n  ---\n", want: []string{"MD039 3:4", "MD039 3:4"}},
		{input: "# T\n\n* [\t][]\n\n[\f]: #\n", want: []string{"MD053 5:1"}},
		{input: "# T\n\n- [x][^n] done\n\n[^n]: Note.\n", want: []string{}},
		{input: "# T\n\n- [ ] [docs](/u) and [Docs][]\n\n[docs]: /docs\n", want: []string{}},
		{input: "# T\n\n10) ### [x][y][z].\n\n[z]: #\n", want: []string{"MD042 3:12", "MD054 3:12"}},
		{input: "# T\n\n- [x]https://example.com\n", want: []string{"MD034 3:6"}},
		{input: "# T\n\n- [x] https://example.com\n", want: []string{"MD034 3:7"}},
		{input: "# T\n\n+ ## [X](#)\n", want: []string{"MD042 3:6"}},
		{input: "# T\n\n- [  ][Y  Z]\n\n[y z]: #\n", want: []string{"MD039 3:4", "MD039 3:4"}},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.want, checkboxLinkFindings(t, tt.input))
		})
	}
}

// TestLinkRulesKnownGapsAtCheckboxes pins the findings of the link rules
// where they still differ from markdownlint 0.41.1 at brackets that are no
// longer checkboxes, so that a fix shows up here. In "- [x][ ](#)" with "x"
// defined, goldmark reads "[x][ ]" as a collapsed reference and the "(#)"
// after it as text, where micromark reads "[x]" as text and "[ ](#)" as a
// link, which markdownlint reports under MD039 and MD042 (and MD051 and
// MD054 where they apply). Before checkboxes were read only where GFM
// reads them, "[x]" was a checkbox and "[ ](#)" a link to goldmark too.
// MD053 reports "[x]: /x" unused there, as it did before. In "- [x][^n]",
// "[x]" is no longer a checkbox either, so MD052 reports the footnote call
// "[x][^n]" when the footnote definition comes right after a link reference
// definition, which it does not count as a footnote; before, it reported
// the shortcut "[^n]" there with shortcut_syntax set, and nothing without.
func TestLinkRulesKnownGapsAtCheckboxes(t *testing.T) {
	tests := []struct {
		input, markdownlint string
		want                []string
	}{
		{
			input:        "# T\n\n- [x][ ](#)\n\n[x]: /x\n",
			markdownlint: "MD039 3:7, MD039 3:7, MD042 3:6",
			want:         []string{"MD053 5:1"},
		},
		{
			input:        "# T\n\n- ## [x][ ](#).\n\n[x]: /x\n",
			markdownlint: "MD039 3:10, MD039 3:10, MD042 3:9",
			want:         []string{"MD053 5:1"},
		},
		{
			input:        "# T\n\n- [x][^n]\n\n[c]: /url\n[^n]: Note text.\n",
			markdownlint: "MD053 5:1",
			want:         []string{"MD052 3:3", "MD052 6:1", "MD053 5:1"},
		},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.want, checkboxLinkFindings(t, tt.input), "markdownlint: %s", tt.markdownlint)
		})
	}
}

// checkboxLinkFindings returns the findings of the link rules for input,
// sorted, as "RULE line:column", with MD052's shortcut_syntax and MD054
// disallowing every reference style.
func checkboxLinkFindings(t *testing.T, input string) []string {
	t.Helper()
	rules := []struct {
		rule    lint.Rule
		options map[string]any
	}{
		{rule: NewNoBareURLsRule()},
		{rule: NewLinkSpacesRule()},
		{rule: NewEmptyLinkRule()},
		{rule: NewReferenceLinkImagesRule(), options: map[string]any{"shortcut_syntax": true}},
		{rule: NewLinkImageRefDefsRule()},
		{rule: NewLinkImageStyleRule(), options: map[string]any{"full": false, "collapsed": false, "shortcut": false}},
	}
	got := []string{}
	for _, rule := range rules {
		for _, diag := range applyRuleWithOptions(t, rule.rule, input, rule.options) {
			got = append(got, fmt.Sprintf("%s %d:%d", diag.RuleID, diag.StartLine, diag.StartColumn))
		}
	}
	slices.Sort(got)
	return got
}
