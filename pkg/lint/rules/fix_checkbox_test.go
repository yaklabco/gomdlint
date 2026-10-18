package rules

import (
	"context"
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

// TestCheckboxForm checks the forms of a checkbox that the render signature
// keeps because micromark reads a checkbox there and GitHub's cmark-gfm
// reads text. cmark-gfm 0.29.0.gfm.0 reads a checkbox in each document
// whose form is empty.
func TestCheckboxForm(t *testing.T) {
	tests := []struct {
		input string
		form  string
	}{
		{"- [ ] task\n", ""},
		{"- [X] task\n", ""},
		{"-\t[x] foo\n", ""},
		{"1.  [x] wide\n", ""},
		{"10) [ ] ten\n", ""},
		{"- [x] \n  task\n", ""},
		{"- [\t] task\n", "tab"},
		{"- [\n  ] task\n", "eol"},
		{"- [\r\n  ] task\r\n", "eol"},
		{"- [x]\n  task\n", "bare"},
		{"- [\n  ]\n  task\n", "eolbare"},
		{"-\n  [x] done\n", "late"},
		{"- [c]: /url\n  [x] done\n", "late"},
		{"> -\n>   [x] done\n", "late"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			source := []byte(tt.input)
			markdown := newFixCheck(&mdast.FileSnapshot{Content: source}, config.FlavorGFM).markdowns[0]
			boxes := goldmarkCheckboxes(markdown.Parser().Parse(gmtext.NewReader(source)))
			require.Len(t, boxes, 1)
			open := boxes[0].Parent().Lines().At(0).Start
			require.Equal(t, byte('['), source[open], "the paragraph starts with the checkbox")
			assert.Equal(t, tt.form, checkboxForm(boxes[0], source))
		})
	}
}

// TestFixKeepsCheckboxes runs --fix with every default rule on documents
// where a fix could turn text into a task checkbox or a checkbox into text,
// and checks that the output reads as micromark reads the input. Each
// comment gives the checkboxes micromark with micromark-extension-gfm reads
// in the input and in the document the withheld fix would make.
func TestFixKeepsCheckboxes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options map[string]map[string]any
		want    string
		ruleIDs []string // Rules whose findings stay without a fix.
	}{
		{
			// MD030 narrowing the space would make the tab one column
			// wide: 0 checkboxes, then 1. MD010 replacing the tab with a
			// space would too.
			name:    "marker space before a tab between brackets",
			input:   "-  [\t] task\n",
			options: map[string]map[string]any{"MD030": {"ul_single": 0}},
			want:    "-  [\t] task\n",
			ruleIDs: []string{"MD010", "MD030"},
		},
		{
			// MD007 indenting the item moves the tab two columns: 0, then 1.
			name:    "nested item with a tab between brackets",
			input:   "- a\n  - [\t] x\n",
			options: map[string]map[string]any{"MD007": {"indent": 4}},
			want:    "- a\n  - [\t] x\n",
			ruleIDs: []string{"MD007", "MD010"},
		},
		{
			// MD030 keeps the tab four columns wide: 0, then 0. MD010
			// replacing it with a space would make a checkbox: 0, then 1.
			name:    "ordered item with a tab between brackets",
			input:   "1.  [\t] task\n",
			want:    "1. [\t] task\n",
			ruleIDs: []string{"MD010"},
		},
		{
			// Replacing the tab with a space keeps micromark's checkbox, 1
			// and 1, but GitHub's cmark-gfm reads "[\t]" as text, so the
			// fix would add a checkbox there.
			name:    "tab between brackets",
			input:   "- [\t] task\n",
			want:    "- [\t] task\n",
			ruleIDs: []string{"MD010"},
		},
		{
			// MD027 removing the space moves the bracket off the item's
			// content column: 1, then 0.
			name:    "quoted line ending between brackets",
			input:   ">  - [\n>    ] task\n",
			want:    ">  - [\n>    ] task\n",
			ruleIDs: []string{"MD027"},
		},
		{
			// The spaces after the checkbox are a hard line break, which
			// MD009 keeps at two spaces: 1 checkbox and a break, then the
			// same.
			name:  "hard break after a checkbox",
			input: "- [ ]   \n  task\n",
			want:  "- [ ]  \n  task\n",
		},
		{
			// cmark-gfm reads "- [x]" with nothing after it on its line as
			// text, and micromark reads a checkbox, 1 and 1.
			name:    "one space after a checkbox",
			input:   "- [x] \n  task\n",
			want:    "- [x] \n  task\n",
			ruleIDs: []string{"MD009"},
		},
		{
			// Both read a checkbox before and after: 1, then 1.
			name:  "tab after a checkbox",
			input: "- [x]\ttask\n",
			want:  "- [x] task\n",
		},
		{
			name:  "marker space before a checkbox",
			input: "-  [x] task\n",
			want:  "- [x] task\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := fixWithDefaultRules(t, tt.input, tt.options)
			assert.Equal(t, tt.want, got)
			for _, ruleID := range tt.ruleIDs {
				var found bool
				for _, diag := range diags {
					if diag.RuleID == ruleID {
						found = true
						assert.Empty(t, diag.FixEdits, "%s keeps its finding without a fix", ruleID)
					}
				}
				assert.True(t, found, "%s reports the finding", ruleID)
			}
		})
	}
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
