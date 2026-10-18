package rules

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gmast "github.com/yuin/goldmark/ast"
	gmextast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// checkboxDocuments are documents whose task checkboxes micromark reads
// differently from goldmark's own task list extension, with the number of
// checkboxes micromark 4 with micromark-extension-gfm reads in each.
var checkboxDocuments = []struct {
	input string
	boxes int
}{
	{"- [\t] task\n", 1},
	{"-  [\t] task\n", 0},
	{"1. [\t] task\n", 0},
	{"> - [\t] task\n", 0},
	{"- [\f] done\n", 0},
	{"- [x]foo\n", 0},
	{"- ## [ ](/url).\n", 0},
	{"- [\n  ] task\n", 1},
	{"- [\n   ] task\n", 0},
	{"> - [\n> ] task\n", 1},
	{"- [c]: /url\n  [x] done\n", 1},
	{"- [c]: /url\n\n  [x] done\n", 0},
	{"- [x] a\n- [ ] b\n", 2},
}

// TestFixChecksReadCheckboxesAsTheParser checks that every goldmark instance
// the fix checks build for GFM reads the checkboxes the parser reads, and
// that both read what micromark reads. A fix check that read checkboxes
// differently could accept a fix that turns text into a checkbox, as MD030
// narrowing "-  [\t] task" did.
func TestFixChecksReadCheckboxesAsTheParser(t *testing.T) {
	for _, tt := range checkboxDocuments {
		t.Run(tt.input, func(t *testing.T) {
			source := []byte(tt.input)
			file, err := gmparser.New(gmparser.FlavorGFM).Parse(context.Background(), "test.md", source)
			require.NoError(t, err)
			assert.Len(t, mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox), tt.boxes, "parser")

			check := newFixCheck(file, config.FlavorGFM)
			require.NotEmpty(t, check.markdowns)
			assert.Len(t, goldmarkCheckboxes(check.markdowns[0].Parser().Parse(gmtext.NewReader(source))), tt.boxes,
				"render signature check")
			assert.Len(t,
				goldmarkCheckboxes(recordingMarkdown(gmparser.FlavorGFM).Parser().Parse(gmtext.NewReader(source))),
				tt.boxes, "region check")
		})
	}
}

// TestFixKeepsCheckboxes runs --fix with every default rule over a corpus
// of task list documents: the cases of earlier review rounds, fixes that
// could make a checkbox, and checkboxes that micromark and GitHub's
// cmark-gfm read differently. The engine withholds every fix that changes
// or makes a possible checkbox (lint's checkbox guard), so only fixes away
// from them apply. micromark is the checkbox count of micromark with
// micromark-extension-gfm, and cmark that of cmark-gfm 0.29.0.gfm.0; both
// were counted on the input and on the expected output, and are the same.
func TestFixKeepsCheckboxes(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		options          map[string]map[string]any
		want             string // Output of --fix; empty means unchanged.
		micromark, cmark int
	}{
		{name: "md030 zero, tab four wide", input: "-  [\t] task\n", options: map[string]map[string]any{"MD030": {"ul_single": 0}}, micromark: 0, cmark: 0},
		{name: "tab one wide", input: "- [\t] task\n", micromark: 1, cmark: 0},
		{name: "tab four wide", input: "-  [\t] task\n", micromark: 0, cmark: 0},
		{name: "md007 indent four", input: "- a\n  - [\t] x\n", options: map[string]map[string]any{"MD007": {"indent": 4}}, micromark: 0, cmark: 0},
		{name: "md030 ordered", input: "1.  [\t] task\n", micromark: 0, cmark: 0},
		{name: "multiline", input: "- [\n  ] task\n", micromark: 1, cmark: 0},
		{name: "multiline extra space", input: "- [\n   ] task\n", micromark: 0, cmark: 0},
		{name: "after definition", input: "-\n  [c]: /url\n  [x] done\n", micromark: 1, cmark: 0},
		{name: "after definition used", input: "- [c]: /url\n  [x] done [c]\n", micromark: 1, cmark: 0},
		{name: "trailing spaces", input: "- [ ]   \n  task\n", micromark: 1, cmark: 1},
		{name: "tab after checkbox", input: "- [x]\ttask\n", micromark: 1, cmark: 1},
		{name: "md030 plain", input: "-  [x] task\n", micromark: 1, cmark: 1},
		{name: "one trailing space", input: "- [x] \n  task\n", micromark: 1, cmark: 1},
		{name: "quoted multiline md027", input: ">  - [\n>    ] task\n", micromark: 1, cmark: 0},
		{name: "quoted multiline", input: "> - [\n>   ] task\n", micromark: 1, cmark: 0},
		{name: "ordered tab two wide", input: "1.   [\t] task\n", micromark: 0, cmark: 0},
		{name: "space after an empty marker", input: "- \n  [x] task\n", micromark: 0, cmark: 0},
		{name: "space after the brackets", input: "- [x] \n", micromark: 0, cmark: 1},
		{name: "md039 link text", input: "- [ x](u)\n", micromark: 0, cmark: 0},
		{name: "md011 reversed link", input: "- (x)[y]\n", micromark: 0, cmark: 0},
		{name: "md004 marker style", input: "* [x] a\n- [ ] b\n", micromark: 2, cmark: 2},
		{name: "md029 numbering", input: "1. [x] a\n3. [ ] b\n", options: map[string]map[string]any{"MD029": {"style": "ordered"}}, micromark: 2, cmark: 2},
		{name: "md007 nested", input: "- a\n    - [x] b\n", micromark: 1, cmark: 1},
		{name: "md027 quote spacing", input: ">  - [x] a\n", micromark: 1, cmark: 0},
		{name: "md010 tab after text", input: "- [x] a\tb\n", micromark: 1, cmark: 1},
		{name: "md009 text after", input: "- [x] task   \n", micromark: 1, cmark: 1},
		{name: "md032 list after paragraph", input: "Text\n- [x] a\n- [ ] b\nMore\n", want: "Text\n\n- [x] a\n- [ ] b\nMore\n", micromark: 2, cmark: 2},
		{name: "md012 blank lines in item", input: "- [x] a\n\n\n  b\n", want: "- [x] a\n\n  b\n", micromark: 1, cmark: 1},
		{name: "md047 final newline", input: "- [x] a", micromark: 1, cmark: 1},
		{name: "blank first line", input: "-\n  [x] done\n", micromark: 1, cmark: 0},
		{name: "blank first line, trailing tab", input: "-\t\n  [x] done\n", micromark: 0, cmark: 0},
		{name: "definitions then checkbox", input: "- [cc]: /url\n  [dd]: /other\n  [x] done\n", micromark: 1, cmark: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := fixWithDefaultRules(t, tt.input, tt.options)
			want := tt.want
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, got)
			assert.Equal(t, checkboxLines(tt.input), checkboxLines(got),
				"the lines a checkbox could be read from stay as they are")
		})
	}
}

// TestFixKeepsCheckboxFindings checks that a finding whose fix the checkbox
// guard withholds is still reported, without a fix: MD009 on the two
// documents of round 2's review, where removing the space changed
// micromark's and cmark-gfm's checkboxes.
func TestFixKeepsCheckboxFindings(t *testing.T) {
	for _, input := range []string{"- \n  [x] task\n", "- [x] \n"} {
		t.Run(input, func(t *testing.T) {
			got, diags := fixWithDefaultRules(t, input, nil)
			assert.Equal(t, input, got)
			var found bool
			for _, diag := range diags {
				if diag.RuleID == "MD009" {
					found = true
					assert.Empty(t, diag.FixEdits, "MD009 keeps its finding without a fix")
				}
			}
			assert.True(t, found, "MD009 reports the trailing space")
		})
	}
}

// checkboxLinePattern matches a line whose text, after blockquote markers,
// list item markers, spaces and tabs, starts with "[", and a list item's
// marker line that holds nothing else, after which an item's first
// paragraph starts on the next line.
var checkboxLinePattern = regexp.MustCompile(
	`^[ \t>]*(?:(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]+|$))*\[|^[ \t>]*(?:(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]+|$))+$`)

// checkboxLines returns the lines of text that checkboxLinePattern
// matches, in order, with their line endings.
func checkboxLines(text string) []string {
	var lines []string
	for _, line := range strings.SplitAfter(text, "\n") {
		if checkboxLinePattern.MatchString(strings.TrimRight(line, "\r\n")) {
			lines = append(lines, line)
		}
	}
	return lines
}

// goldmarkCheckboxes returns the task checkboxes of a goldmark tree.
func goldmarkCheckboxes(root gmast.Node) []*gmextast.TaskCheckBox {
	var boxes []*gmextast.TaskCheckBox
	_ = gmast.Walk(root, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) { //nolint:errcheck // The walker never returns an error.
		if box, ok := node.(*gmextast.TaskCheckBox); ok && entering {
			boxes = append(boxes, box)
		}
		return gmast.WalkContinue, nil
	})
	return boxes
}
