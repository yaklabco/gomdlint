package rules

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gmextast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// checkboxDocuments are documents at the edges of where micromark,
// cmark-gfm and goldmark read task checkboxes, with the number of
// checkboxes the parser reads in each (see gmparser.GFM).
var checkboxDocuments = []struct {
	input string
	boxes int
}{
	{"- [\t] task\n", 1},
	{"-  [\t] task\n", 1},
	{"1. [\t] task\n", 1},
	{"> - [\t] task\n", 1},
	{"- [\f] done\n", 0},
	{"- [x]foo\n", 0},
	{"- ## [ ](/url).\n", 0},
	{"- [x](https://example.com)\n", 0},
	{"- [x] \n", 0},
	{"- [x] foo\n  ---\n", 0},
	{"- [\n  ] task\n", 0},
	{"- [\n   ] task\n", 0},
	{"> - [\n> ] task\n", 0},
	{"- [c]: /url\n  [x] done\n", 0},
	{"- [c]: /url\n\n  [x] done\n", 0},
	{"- [x] a\n- [ ] b\n", 2},
}

// TestFixChecksReadCheckboxesAsTheParser checks that every goldmark instance
// the fix checks build for GFM reads the checkboxes the parser reads (see
// checkboxDocuments). A fix check that read checkboxes differently could
// accept a fix that turns text into a checkbox. Fixes near any possible
// checkbox are withheld by lint's checkbox guard, which reads bytes, not
// this parse.
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

// checkboxCorpus is a corpus of task list documents: the cases of earlier
// review rounds, fixes that could make a checkbox, checkboxes that
// micromark and GitHub's cmark-gfm read differently, and plain task lists
// that each list rule fixes. want is the output of --fix with every default
// rule and options, and empty when the document stays as it is. micromark
// is the checkbox count of micromark with micromark-extension-gfm, and
// cmark that of cmark-gfm 0.29.0.gfm.0; both were counted on the input and
// on the expected output, and are the same.
var checkboxCorpus = []struct {
	name             string
	input            string
	options          map[string]map[string]any
	want             string
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
	{name: "md030 plain", input: "-  [x] task\n", want: "- [x] task\n", micromark: 1, cmark: 1},
	{name: "one trailing space", input: "- [x] \n  task\n", micromark: 1, cmark: 1},
	{name: "quoted multiline md027", input: ">  - [\n>    ] task\n", micromark: 1, cmark: 0},
	{name: "quoted multiline", input: "> - [\n>   ] task\n", micromark: 1, cmark: 0},
	{name: "ordered tab two wide", input: "1.   [\t] task\n", micromark: 0, cmark: 0},
	{name: "space after an empty marker", input: "- \n  [x] task\n", micromark: 0, cmark: 0},
	{name: "space after the brackets", input: "- [x] \n", micromark: 0, cmark: 1},
	{name: "md039 link text", input: "- [ x](u)\n", micromark: 0, cmark: 0},
	{name: "md011 reversed link", input: "- (x)[y]\n", micromark: 0, cmark: 0},
	{name: "md004 marker style", input: "* [x] a\n- [ ] b\n", micromark: 2, cmark: 2},
	{
		name:      "md029 numbering",
		input:     "1. [x] a\n3. [ ] b\n",
		options:   map[string]map[string]any{"MD029": {"style": "ordered"}},
		want:      "1. [x] a\n2. [ ] b\n",
		micromark: 2, cmark: 2,
	},
	{name: "md007 nested", input: "- a\n    - [x] b\n", want: "- a\n  - [x] b\n", micromark: 1, cmark: 1},
	{name: "md027 quote spacing", input: ">  - [x] a\n", want: "> - [x] a\n", micromark: 1, cmark: 0},
	{name: "md010 tab after text", input: "- [x] a\tb\n", micromark: 1, cmark: 1},
	{name: "md009 text after", input: "- [x] task   \n", want: "- [x] task\n", micromark: 1, cmark: 1},
	{name: "md032 list after paragraph", input: "Text\n- [x] a\n- [ ] b\nMore\n", want: "Text\n\n- [x] a\n- [ ] b\nMore\n", micromark: 2, cmark: 2},
	{name: "md012 blank lines in item", input: "- [x] a\n\n\n  b\n", micromark: 1, cmark: 1},
	{name: "md047 final newline", input: "- [x] a", micromark: 1, cmark: 1},
	{name: "blank first line", input: "-\n  [x] done\n", micromark: 1, cmark: 0},
	{name: "blank first line, trailing tab", input: "-\t\n  [x] done\n", micromark: 0, cmark: 0},
	{name: "definitions then checkbox", input: "- [cc]: /url\n  [dd]: /other\n  [x] done\n", micromark: 1, cmark: 0},
	{name: "footnote list, trailing spaces", input: "[^a]: - [x] task  \n\nUse [^a].\n", micromark: 1, cmark: 0},
	{name: "footnote list, tab after brackets", input: "[^a]: - [ ]\ttask\n\nUse [^a].\n", micromark: 1, cmark: 0},
	{name: "quoted footnote list", input: "> [^a]: - [x] task \n\nUse [^a].\n", micromark: 1, cmark: 0},
	{name: "footnote list in an item", input: "- a\n  [^b]: - [x] c \n\nUse [^b].\n", micromark: 1, cmark: 0},
	{name: "footnote list, space after an empty marker", input: "[^a]: - \n    [x] task\n\nUse [^a].\n", micromark: 0, cmark: 0},
	{name: "plain md004", input: "* [ ] a\n- [x] b\n* [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain md004 nested", input: "* [ ] a\n  - [x] b\n  - [ ] c\n", want: "* [ ] a\n  * [x] b\n  * [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain md005", input: "- [ ] a\n - [x] b\n- [ ] c\n", want: "- [ ] a\n- [x] b\n- [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain md007", input: "- [ ] a\n    - [x] b\n", want: "- [ ] a\n  - [x] b\n", micromark: 2, cmark: 2},
	{name: "plain md009", input: "- [ ] a   \n- [x] b\n", want: "- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "plain md010 indentation", input: "- [ ] a\n\t- [x] b\n", want: "- [ ] a\n  - [x] b\n", micromark: 2, cmark: 2},
	{name: "plain md010 after the marker", input: "-\t[ ] a\n-\t[x] b\n", want: "- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{
		name:      "plain md029",
		input:     "1. [ ] a\n3. [x] b\n",
		options:   map[string]map[string]any{"MD029": {"style": "ordered"}},
		want:      "1. [ ] a\n2. [x] b\n",
		micromark: 2, cmark: 2,
	},
	{
		name: "plain md029 nine to ten",
		input: "1. [ ] item 1\n" +
			"2. [ ] item 2\n" +
			"3. [ ] item 3\n" +
			"4. [ ] item 4\n" +
			"5. [ ] item 5\n" +
			"6. [ ] item 6\n" +
			"7. [ ] item 7\n" +
			"8. [ ] item 8\n" +
			"9. [ ] item 9\n" +
			"9. [x] item ten\n",
		options: map[string]map[string]any{"MD029": {"style": "ordered"}},
		want: "1. [ ] item 1\n" +
			"2. [ ] item 2\n" +
			"3. [ ] item 3\n" +
			"4. [ ] item 4\n" +
			"5. [ ] item 5\n" +
			"6. [ ] item 6\n" +
			"7. [ ] item 7\n" +
			"8. [ ] item 8\n" +
			"9. [ ] item 9\n" +
			"10. [x] item ten\n",
		micromark: 10, cmark: 10,
	},
	{name: "plain md030", input: "-  [ ] a\n-  [x] b\n", want: "- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "plain md030 ordered", input: "1.  [ ] a\n2.  [x] b\n", want: "1. [ ] a\n2. [x] b\n", micromark: 2, cmark: 2},
	{name: "plain md027", input: ">  - [ ] a\n>  - [x] b\n", want: "> - [ ] a\n> - [x] b\n", micromark: 2, cmark: 0},
	{name: "plain nested", input: "- [ ] a\n  - [x] b\n    - [ ] c   \n", want: "- [ ] a\n  - [x] b\n    - [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain nested, wrong indent", input: "- [ ] a\n   - [x] b\n      - [ ] c\n", want: "- [ ] a\n  - [x] b\n    - [ ] c\n", micromark: 3, cmark: 3},
	{
		name:      "plain ordered nine to ten, wide",
		input:     "9.  [ ] a\n9.  [x] b\n",
		options:   map[string]map[string]any{"MD029": {"style": "ordered"}},
		want:      "9. [ ] a\n9. [x] b\n",
		micromark: 2, cmark: 2,
	},
	{name: "plain in a blockquote", input: "> - [ ] a   \n> - [x] b\n", want: "> - [ ] a\n> - [x] b\n", micromark: 2, cmark: 0},
	{name: "plain in a nested blockquote", input: "> > - [ ] a\n> >   - [x] b   \n", want: "> > - [ ] a\n> >   - [x] b\n", micromark: 2, cmark: 0},
	{name: "plain with tabs", input: "-\t[ ] a\t\n\t-\t[x] b\n", want: "- [ ] a\n  - [x] b\n", micromark: 2, cmark: 2},
	{name: "plain with trailing spaces", input: "- [ ] a \n- [x] b  \n- [ ] c   \n", want: "- [ ] a\n- [x] b  \n- [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain with text after", input: "- [ ] a\n  more text   \n- [x] b\n", want: "- [ ] a\n  more text\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "plain then underline-like line", input: "- [ ] a\n      ---\n", micromark: 1, cmark: 1},
	{name: "plain after a paragraph", input: "Intro\n- [ ] a\n- [x] b\n", want: "Intro\n\n- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "plain loose list", input: "- [ ] a\n\n- [x] b  \n\n- [ ] c\n", micromark: 3, cmark: 3},
	{name: "plain checked uppercase", input: "* [X] a\n- [X] b\n", micromark: 2, cmark: 2},
	{name: "plain with a link", input: "-  [ ] see [docs](https://example.com)\n", want: "- [ ] see [docs](https://example.com)\n", micromark: 1, cmark: 1},
	{name: "plain mixed with non-plain", input: "- [ ] a   \n- [\t] b   \n- [x] c   \n", micromark: 3, cmark: 2},
	{name: "plain after a footnote", input: "[^n]: note\n\n- [ ] a   \n\nUse [^n].\n", micromark: 1, cmark: 1},
	{name: "plain before a footnote", input: "- [ ] a   \n\nUse [^n].\n\n[^n]: note\n", want: "- [ ] a\n\nUse [^n].\n\n[^n]: note\n", micromark: 1, cmark: 1},
	{name: "escaped footnote label", input: "[^a\\]:b]: - [\t] task\n\nUse [^a\\]:b].\n", micromark: 0, cmark: 0},
	{name: "quoted ordered item after text", input: "Intro\n> 3. [x] task\n", micromark: 0, cmark: 0},
	{name: "cmark marker with an exclamation mark", input: "-\n  1! [x] \n", micromark: 0, cmark: 1},
	{name: "cmark ten digit marker", input: "-\n  1234567890. [x] \n", micromark: 0, cmark: 1},
	{name: "cmark ten digit marker first", input: "1234567890. [x] a  \n", micromark: 0, cmark: 0},
	{name: "escaped label with trailing spaces", input: "[^a\\]]: - [ ] task  \n\nUse [^a\\]].\n", micromark: 1, cmark: 0},
	{name: "md032 bullet after text", input: "Intro\n- [ ] a\n- [x] b\n", want: "Intro\n\n- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "md032 ordered from 1 after text", input: "Intro\n1. [ ] a\n2. [x] b\n", want: "Intro\n\n1. [ ] a\n2. [x] b\n", micromark: 2, cmark: 2},
	{name: "md032 ordered from 1 with a parenthesis", input: "Intro\n1) [ ] a\n2) [x] b\n", want: "Intro\n\n1) [ ] a\n2) [x] b\n", micromark: 2, cmark: 2},
	{name: "md032 in an open blockquote", input: "> Intro\n> - [ ] a\n> - [x] b\n", want: "> Intro\n>\n> - [ ] a\n> - [x] b\n", micromark: 2, cmark: 0},
	{name: "md032 blockquote starting on the item", input: "Intro\n> - [ ] a\n> - [x] b\n", micromark: 2, cmark: 0},
	{name: "md032 after a heading", input: "# Title\n- [ ] a\n- [x] b\n", want: "# Title\n\n- [ ] a\n- [x] b\n", micromark: 2, cmark: 2},
	{name: "md032 ordered from 3 after text", input: "Intro\n3. [ ] a\n4. [x] b\n", micromark: 0, cmark: 0},
	{name: "md032 quoted ordered from 3", input: "Intro\n> 3. [ ] a\n> 4. [x] b\n", micromark: 0, cmark: 0},
	{name: "md012 blank lines before a list", input: "Intro\n\n\n- [ ] a\n", want: "Intro\n\n- [ ] a\n", micromark: 1, cmark: 1},
	{name: "md032 zero-padded first item", input: "Intro\n01. [ ] a\n", micromark: 0, cmark: 1},
	{name: "md032 zero-padded list", input: "Intro\n01. a\n2. [x] b\n", micromark: 0, cmark: 1},
	{name: "md018 before a quoted ordered item", input: "#Title\n> 3. [x] task\n", micromark: 0, cmark: 0},
	{name: "md018 two lines above a quoted ordered item", input: "#Title\nmore\n> 3. [x] task\n", micromark: 0, cmark: 0},
	{name: "md018 before a bullet item", input: "#Title\n- [x] task\n", want: "# Title\n\n- [x] task\n", micromark: 1, cmark: 1},
	{name: "md026 before an ordered item from 3", input: "# Title:\n3. [ ] a\n", micromark: 1, cmark: 1},
	{name: "md009 above a quoted ordered item", input: "Intro  \n> 3. [x] task\n", micromark: 0, cmark: 0},
	{name: "md012 above an ordered item from 3", input: "# T\n\n\n3. [ ] a\n", want: "# T\n\n3. [ ] a\n", micromark: 1, cmark: 1},
	{name: "md047 after a list that is not plain", input: "- [\t] a", micromark: 1, cmark: 0},
}

// TestFixKeepsCheckboxes runs --fix with every default rule over the
// checkbox corpus. The engine withholds every fix that changes or makes a
// possible checkbox (lint's checkbox guard), except the marker, spacing,
// indentation and trailing whitespace changes it lets through on plain task
// items.
func TestFixKeepsCheckboxes(t *testing.T) {
	for _, tt := range checkboxCorpus {
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

// gfmCheckboxLinePattern matches the first line of a paragraph, as goldmark
// holds it, that GFM starts with a task checkbox: the brackets followed by
// a line ending, or by spaces and tabs and more text. goldmark trims the
// white space that ends a paragraph from its last line.
var gfmCheckboxLinePattern = regexp.MustCompile(`^\[[ \txX]\](?:[\r\n]|[ \t]+[^ \t])`)

// TestParserReadsCheckboxesAsGoldmark checks that the parser reads the task
// checkboxes that goldmark's own GFM extension reads, at the same offsets
// and checked alike, in every document of the checkbox corpus and in its
// expected outputs, except those GFM does not read: in a heading, or with
// the brackets not followed as GFM requires (gfmCheckboxLinePattern). The
// parser hands goldmark's checkbox parser only the brackets GFM reads, so
// this pins its configuration and the mapping of checkbox nodes.
func TestParserReadsCheckboxesAsGoldmark(t *testing.T) {
	type place struct {
		offset  int
		checked bool
	}
	inputs := make([]string, 0, 2*len(checkboxCorpus)+len(checkboxDocuments))
	for _, doc := range checkboxDocuments {
		inputs = append(inputs, doc.input)
	}
	for _, doc := range checkboxCorpus {
		inputs = append(inputs, doc.input)
		if doc.want != "" {
			inputs = append(inputs, doc.want)
		}
	}
	reference := goldmark.New(goldmark.WithExtensions(extension.GFM))
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			source := []byte(input)
			want := []place{}
			for _, box := range goldmarkCheckboxes(reference.Parser().Parse(gmtext.NewReader(source))) {
				block := box.Parent()
				if kind := block.Kind(); kind != gmast.KindParagraph && kind != gmast.KindTextBlock {
					continue
				}
				first := block.Lines().At(0)
				if !gfmCheckboxLinePattern.Match(first.Value(source)) {
					continue
				}
				want = append(want, place{offset: first.Start, checked: box.IsChecked})
			}
			file, err := gmparser.New(gmparser.FlavorGFM).Parse(context.Background(), "test.md", source)
			require.NoError(t, err)
			got := []place{}
			for _, box := range mdast.FindByKind(file.Root, mdast.NodeTaskCheckbox) {
				got = append(got, place{offset: box.SourceRange().StartOffset, checked: box.Inline.Checked})
			}
			assert.Equal(t, want, got)
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

// checkboxLinePattern reads a line whose text, after blockquote markers
// and indentation, holds list item markers and then starts with "[", or
// holds nothing but a list item marker, after which an item's first
// paragraph starts on the next line.
var checkboxLinePattern = regexp.MustCompile(
	`^([ \t>]*)((?:(?:[-+*]|[0-9]{1,9}[.)])(?:[ \t]+|$))*)(\[.*)?$`)

// orderedMarkerPattern and bulletMarkerPattern match list item markers, so
// that checkboxLines can name their kinds.
var (
	orderedMarkerPattern = regexp.MustCompile(`[0-9]+[.)]`)
	bulletMarkerPattern  = regexp.MustCompile(`[-+*]`)
)

// checkboxLines returns a key for each line of text that a checkbox could be
// read from, in order: its blockquote markers, the kinds of its list item
// markers, and its text from the "[" on without trailing whitespace. A fix
// that the checkbox guard lets through may change the indentation, the
// markers for others of their kind, the space after them and the trailing
// whitespace, which the keys leave out.
func checkboxLines(text string) []string {
	var keys []string
	for _, line := range strings.SplitAfter(text, "\n") {
		match := checkboxLinePattern.FindStringSubmatch(strings.TrimRight(line, " \t\r\n"))
		if match == nil || (match[3] == "" && match[2] == "") {
			continue
		}
		kinds := orderedMarkerPattern.ReplaceAllString(match[2], "1.")
		kinds = bulletMarkerPattern.ReplaceAllString(kinds, "-")
		keys = append(keys, strings.Repeat(">", strings.Count(match[1], ">"))+"|"+strings.Join(strings.Fields(kinds), " ")+"|"+match[3])
	}
	return keys
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
