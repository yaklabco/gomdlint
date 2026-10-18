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
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestReversedLinkRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantN   int
		wantFix bool
	}{
		{
			name:    "valid link",
			input:   "[text](url)",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "reversed link",
			input:   "(text)[url]",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "multiple reversed",
			input:   "(a)[b] and (c)[d]",
			wantN:   2,
			wantFix: true,
		},
		{
			name:    "reversed with spaces",
			input:   "(some text)[http://example.com]",
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "valid link with title",
			input:   `[text](url "title")`,
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "empty file",
			input:   "",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "normal parentheses not link",
			input:   "This is (just parentheses) not a link.",
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "code block should be skipped",
			input:   "```\n(text)[url]\n```",
			wantN:   0,
			wantFix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewReversedLinkRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}

func TestLinkSpacesRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantN int
	}{
		{
			name:  "valid link no spaces",
			input: "[text](url)",
			wantN: 0,
		},
		{
			name:  "leading space",
			input: "[ text](url)",
			wantN: 1,
		},
		{
			name:  "trailing space",
			input: "[text ](url)",
			wantN: 1,
		},
		{
			// markdownlint reports each side separately.
			name:  "both spaces",
			input: "[ text ](url)",
			wantN: 2,
		},
		{
			name:  "multiple links with issues",
			input: "[ a](b) and [c ](d)",
			wantN: 2,
		},
		{
			name:  "empty link text",
			input: "[](url)",
			wantN: 0, // Empty text handled by MD042.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewLinkSpacesRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

// TestReversedLinkRule_SkipsMath checks MD011 against markdownlint 0.41.1,
// which parses math with micromark-extension-math: a block fenced by two or
// more dollar signs, which may hold blank lines and runs to its closing
// fence, and inline math between equal runs of dollar signs, are skipped.
func TestReversedLinkRule_SkipsMath(t *testing.T) {
	markdown := "# Title\n\n" +
		"Reversed (link)[http://a] outside math.\n\n" + // 3
		"$$\n(a)[b]\n\n(c)[d]\n$$\n\n" + // 5-9
		"Inline $(e)[f]$ math and $$(g)[h]$$ display.\n\n" + // 11
		"Money $5 and (i)[j] then $6 again.\n\n" + // 13
		"Escaped \\$ (k)[l] dollar and $m$.\n\n" + // 15
		"Code `$` then (n)[o] and $p$.\n\n" + // 17
		"$$$\n(q)[r]\n$$\n(s)[t]\n$$$\n\n" + // 19-23
		"> $$\n> (u)[v]\n> $$\n\n" + // 25-27
		"- $$\n  (w)[x]\n  $$\n\n" + // 29-31
		"Text\n$$ meta\n(y)[z]\n$$\n\n" + // 33-36
		"Unclosed $ (aa)[bb] here.\n\n" + // 38
		"$$\n(cc)[dd]\n" // 40-41

	diags := applyRuleWithOptions(t, NewReversedLinkRule(), markdown, nil)
	type position struct{ line, column int }
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{3, 10}, {15, 12}, {17, 15}, {38, 12}}, got)
}

// TestReversedLinkRule_MathAndContainers checks MD011 against markdownlint
// 0.41.1 on math next to lazy lines and fences in blockquotes and list
// items. markdownlint ends a math block at the first line that does not
// continue its blockquotes and list items explicitly. gomdlint reads only
// top-level math from a fence at column 0, which a lazy line cannot end,
// and closes it at a fence after at most three spaces. Rows where
// markdownlint skips math in a container and gomdlint reports, as main
// does, come last. Some of the other rows agree because inline math
// between two "$$" runs of one paragraph covers the same text.
func TestReversedLinkRule_MathAndContainers(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
	}{
		{name: "lazy line in a blockquote", input: "> $$\n(a)[b]\n", want: []position{{2, 1}}},
		{name: "lazy line in a list item", input: "- $$\n(a)[b]\n", want: []position{{2, 1}}},
		{name: "list item ended by a blank line", input: "- $$\n\n(a)[b]\n", want: []position{{3, 1}}},
		{name: "lazy fence opens math of its own", input: "> $$\n> a\n$$\n(e)[f]\n", want: []position{}},
		// A fence on a lazy line opens math outside the containers it does
		// not continue, where goldmark reads a lazy paragraph line.
		{name: "lazy fence after a quoted paragraph", input: "> a\n$$\n(b)[c]\n", want: []position{}},
		{name: "lazy fence after a list item", input: "- a\n$$\n(b)[c]\n", want: []position{}},
		{name: "fence in the list item", input: "> - a\n>   $$\n> (b)[c]\n", want: []position{{3, 3}}},
		// A closing fence follows the opening block's own container prefix
		// and at most three spaces. No container starts inside math, so a
		// fence after other markers, or after four columns of space, is
		// math.
		{name: "quoted fence in top-level math", input: "$$\n> $$\n(a)[b]\n$$\n", want: []position{}},
		{name: "nested quote fence", input: "> $$\n> > $$\n> (a)[b]\n> $$\n", want: []position{}},
		{name: "list marker fence in an item", input: "- $$\n  - $$\n  (a)[b]\n  $$\n", want: []position{}},
		{name: "list marker fence at the top", input: "$$\n- $$\n(a)[b]\n$$\n", want: []position{}},
		{name: "fence indented four spaces", input: "$$\n    $$\n(a)[b]\n$$\n", want: []position{}},
		{name: "fence after a tab", input: "$$\n\t$$\n(a)[b]\n$$\n", want: []position{}},
		{name: "fence indented three spaces", input: "$$\n   $$\n(a)[b]\n", want: []position{{3, 1}}},
		{name: "quoted fence indented three spaces", input: "> $$\n>    $$\n> (a)[b]\n", want: []position{{3, 3}}},
		{name: "item fence indented three spaces", input: "- $$\n     $$\n  (a)[b]\n", want: []position{{3, 3}}},
		// Indented or contained math, which markdownlint reads and gomdlint
		// does not: gomdlint reads a math block only from a fence at column 0,
		// so MD011 reports here, as main does.
		{name: "lazy line after math", input: "> $$\n> (a)[b]\n(c)[d]\n", want: []position{{2, 3}, {3, 1}}},
		{name: "blank quoted line", input: "> $$\n>\n> (a)[b]\n", want: []position{{3, 3}}},
		{name: "blank line in a list item", input: "- $$\n\n  (a)[b]\n", want: []position{{3, 3}}},
		{name: "quoted line lazy for the list item", input: "> - $$\n>   (a)[b]\n> (c)[d]\n", want: []position{{2, 5}, {3, 3}}},
		{name: "ordered item indentation", input: "1. $$\n   (a)[b]\n  (c)[d]\n", want: []position{{2, 4}, {3, 3}}},
		{name: "quote markers without spaces", input: ">  $$\n> (a)[b]\n>(c)[d]\n", want: []position{{2, 3}, {3, 2}}},
		{name: "fence lazy for the list item", input: "> - a\n> $$\n> (b)[c]\n", want: []position{{3, 3}}},
		{name: "blockquote in a list item", input: "- > $$\n  > (a)[b]\n  (c)[d]\n", want: []position{{2, 5}, {3, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewReversedLinkRule(), tt.input, nil)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLinkSpacesRule_ReportsEachSide reports leading and trailing spaces
// separately at the spaces the fix deletes, like markdownlint.
func TestLinkSpacesRule_ReportsEachSide(t *testing.T) {
	snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte("Here is [ a link ](url).\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	diags, err := NewLinkSpacesRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(diags))
	}
	want := []struct {
		column  int
		message string
	}{{10, "Link text has leading spaces"}, {17, "Link text has trailing spaces"}}
	for idx, exp := range want {
		if diags[idx].StartColumn != exp.column || diags[idx].Message != exp.message {
			t.Errorf("diagnostic %d = %d %q, want %d %q", idx, diags[idx].StartColumn, diags[idx].Message, exp.column, exp.message)
		}
	}
}

// TestLinkSpacesRule_LinksInTaskListItems checks that MD039 sees the links
// GFM reads at the start of a list item where goldmark alone would read a
// task checkbox: in a heading, or right before "(". markdownlint 0.41.1
// reports each side of the whitespace at the same column.
func TestLinkSpacesRule_LinksInTaskListItems(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantColumns []int
	}{
		{name: "heading in a list item", input: "- ## [ ](/url).\n", wantColumns: []int{7, 7}},
		{name: "quoted heading in a list item", input: "> - ## [ ](/url).\n", wantColumns: []int{9, 9}},
		{name: "link right after the brackets", input: "- [ ](https://example.org)\n", wantColumns: []int{4, 4}},
		{name: "checkbox before parenthesized text", input: "- [ ] (https://example.org)\n", wantColumns: []int{}},
		{name: "checkbox before a link", input: "- [ ] [ text ](/url)\n", wantColumns: []int{8, 13}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewLinkSpacesRule(), tt.input, nil)
			columns := make([]int, 0, len(diags))
			for _, diag := range diags {
				require.Equal(t, 1, diag.StartLine)
				columns = append(columns, diag.StartColumn)
			}
			require.Equal(t, tt.wantColumns, columns)
		})
	}
}

// TestLinkSpacesRule_Fix trims the whitespace inside link text like markdownlint:
// only spaces and tabs on the same line as the bracket are removed.
func TestLinkSpacesRule_Fix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"both sides", "[ text ](url)\n", "[text](url)\n"},
		{"leading", "[ text](url)\n", "[text](url)\n"},
		{"trailing tab", "[text\t](url)\n", "[text](url)\n"},
		{"only whitespace", "[   ](url)\n", "[](url)\n"},
		{"code span inside", "[ a `b` ](url)\n", "[a `b`](url)\n"},
		{"bracket inside code span", "[ a `]` ](url)\n", "[a `]`](url)\n"},
		{"image inside link", "[ ![img](x.png) ](url)\n", "[![img](x.png)](url)\n"},
		{"full reference", "[ text ][ref]\n\n[ref]: https://example.com\n", "[text][ref]\n\n[ref]: https://example.com\n"},
		{"collapsed reference", "[ ref ][]\n\n[ref]: https://example.com\n", "[ref][]\n\n[ref]: https://example.com\n"},
		{"blockquote", "> [ text ](url)\n", "> [text](url)\n"},
		{"line ending is not trimmed", "[\ntext ](url)\n", "[\ntext](url)\n"},
		{"title is not link text", "[text](url \"title \")\n", "[text](url \"title \")\n"},
		{"backslash would escape the bracket", "[foo\\ ](url)\n", "[foo\\ ](url)\n"},
		{"escaped backslash before the space", "[foo\\\\ ](url)\n", "[foo\\\\](url)\n"},
		{"leading side still fixed", "[ foo\\ ](url)\n", "[foo\\ ](url)\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			diags, err := NewLinkSpacesRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}
			var edits []fix.TextEdit
			for _, diag := range diags {
				edits = append(edits, diag.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(edits, len(tt.input))
			if err != nil {
				t.Fatalf("PrepareEdits error: %v", err)
			}
			if got := string(fix.ApplyEdits([]byte(tt.input), prepared)); got != tt.want {
				t.Errorf("fixed = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyLinkRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantN int
	}{
		{
			name:  "valid link",
			input: "[text](url)",
			wantN: 0,
		},
		{
			name:  "empty destination",
			input: "[text]()",
			wantN: 1,
		},
		{
			// markdownlint MD042 checks destinations only; empty text is not an empty link.
			name:  "empty text",
			input: "[](url)",
			wantN: 0,
		},
		{
			name:  "both empty",
			input: "[]()",
			wantN: 1,
		},
		{
			name:  "whitespace only text",
			input: "[   ](url)",
			wantN: 0,
		},
		{
			name:  "multiple empty links",
			input: "[text]() and [](url) and [other](<>)",
			wantN: 2,
		},
		{
			name:  "empty fragment",
			input: "[text](#)",
			wantN: 1,
		},
		{
			name:  "non-empty fragment",
			input: "[text](#section)",
			wantN: 0,
		},
		{
			name:  "reference to empty fragment definition",
			input: "[text][ref] and [ref][]\n\n[ref]: #\n",
			wantN: 2,
		},
		{
			name:  "reference to valid definition",
			input: "[text][ref]\n\n[ref]: https://example.com\n",
			wantN: 0,
		},
		{
			name:  "reference to empty destination definition",
			input: "[text][empty] and [empty][]\n\n[empty]: <>\n",
			wantN: 0,
		},
		{
			name:  "image with empty destination is not a link",
			input: "![alt]()",
			wantN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewEmptyLinkRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

func TestImageAltTextRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantN int
	}{
		{
			name:  "image with alt",
			input: "![alt text](image.png)",
			wantN: 0,
		},
		{
			name:  "image without alt",
			input: "![](image.png)",
			wantN: 1,
		},
		{
			// markdownlint counts whitespace between the brackets as alt
			// text.
			name:  "image with whitespace alt",
			input: "![   ](image.png)",
			wantN: 0,
		},
		{
			name:  "multiple images mixed",
			input: "![good](a.png) and ![](b.png)",
			wantN: 1,
		},
		{
			name:  "link not image",
			input: "[text](url)",
			wantN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewImageAltTextRule()
			ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

// TestImageAltTextRule_EmptyLabel checks MD045 against markdownlint 0.41.1,
// which reports a Markdown image only when nothing at all is written between
// its brackets: whitespace, a line ending or raw HTML there is alt text.
func TestImageAltTextRule_EmptyLabel(t *testing.T) {
	markdown := "# Title\n\n" +
		"![ ](a.png) and ![](b.png) and ![  ](c.png) and ![alt](d.png)\n\n" + // 3
		"![<b></b>](e.png) and ![*](f.png) and ![&nbsp;](g.png)\n\n" + // 5
		"![][ref] and ![ ][ref] and ![ref] and ![ref][]\n\n" + // 7
		"![\n](h.png)\n\n" + // 9
		"[![](i.png)](https://example.com)\n\n" + // 12
		"> ![](j.png) quoted\n\n" + // 14
		"[ref]: /image.png\n"

	type position struct{ line, column int }
	diags := applyRuleWithOptions(t, NewImageAltTextRule(), markdown, nil)
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{3, 17}, {7, 1}, {12, 2}, {14, 3}}, got)
}

// TestImageAltTextRule_Math checks MD045 against markdownlint 0.41.1, which
// parses math, so an image in math, Markdown or HTML, in an HTML block's
// lines or elsewhere, is math.
func TestImageAltTextRule_Math(t *testing.T) {
	markdown := "# Title\n\n" +
		"<div>\n$<img>$\n</div>\n\n" + // 3-5
		"<div>\n$$\n<img>\n$$\n</div>\n\n" + // 7-11
		"$<img src=\"a.png\">$ and $![](a.png)$\n\n" + // 13
		"![](b.png) outside.\n\n" + // 15
		"$$\n> $$\n\n![](c.png) <img src=\"d.png\">\n$$\n\n" + // 17-21: the quoted fence is math
		"Intro\n    $$\n\n![](e.png)\n" // 23-26: the indented line is text

	type position struct{ line, column int }
	diags := applyRuleWithOptions(t, NewImageAltTextRule(), markdown, nil)
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{15, 1}, {26, 1}}, got)
}

// TestReversedLinkRule_MathBlocksStartAtColumnZero checks MD011 against
// markdownlint 0.41.1 on lines that look like a math fence. gomdlint reads
// a math block only when its opening fence starts its line at column 0,
// outside code and HTML blocks, which micromark reads as top-level math
// too, so the block holds exactly the lines up to its closing fence. A
// fence anywhere else opens no math in gomdlint: where markdownlint reads
// no math either, both report; where markdownlint reads indented or
// contained math, gomdlint reports there as main does. From the first fence
// where micromark may read the blocks before it differently from goldmark,
// gomdlint reads no more math blocks, and reports there as main does.
func TestReversedLinkRule_MathBlocksStartAtColumnZero(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
	}{
		// A fence at column 0 opens math, as in markdownlint.
		{name: "fence after a blank line", input: "Intro\n\n$$\n\n(a)[b]\n", want: []position{}},
		{name: "fence right after text", input: "Intro\n$$\n\n(a)[b]\n", want: []position{}},
		{name: "fence after an item after math", input: "$$\nx\n$$\n3. a\n$$\n(a)[b]\n", want: []position{}},
		{name: "fence after an indented line", input: "Intro\n    $$\n$$\n(a)[b]\n", want: []position{}},
		{name: "fence in a table", input: "| a |\n| - |\n$$\n(a)[b]\n$$\n", want: []position{}},
		{name: "fence after a byte order mark", input: "\ufeff$$\n(a)[b]\n$$\n(c)[d]\n", want: []position{{4, 1}}},
		{name: "fence with CRLF", input: "$$\r\n(a)[b]\r\n$$\r\n(c)[d]\r\n", want: []position{{4, 1}}},
		{name: "closing fence after three spaces", input: "$$\n(a)[b]\n   $$\n(c)[d]\n", want: []position{{4, 1}}},
		{name: "text after a closing fence", input: "$$\n(a)[b]\n$$ x\n(c)[d]\n$$\n(e)[f]\n", want: []position{{6, 1}}},
		{name: "fence in code", input: "```\n$$\n```\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "fence in an HTML block", input: "<div>\n$$\n</div>\n\n(a)[b]\n", want: []position{{5, 1}}},
		{name: "comment right before math", input: "<!-- prettier-ignore -->\n$$\n(a)[b]\n$$\n\n(c)[d]\n", want: []position{{6, 1}}},
		{name: "closed code block right before math", input: "```\nx\n```\n$$\n(a)[b]\n$$\n\n(c)[d]\n", want: []position{{8, 1}}},
		{name: "list item right before math", input: "1. Step\n$$\n(a)[b]\n$$\n\n(c)[d]\n", want: []position{{6, 1}}},
		{name: "inline HTML right before math", input: "<b>Note:</b> the formula\n$$\n(a)[b]\n$$\n\n(c)[d]\n", want: []position{{6, 1}}},
		{name: "math after a closed directive", input: ":::note\ntext\n:::\n\n$$\nx\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{9, 1}}},
		{name: "item fence and a column-0 fence after it", input: "- a\n  $$\n  x\n$$\n(a)[b]\n$$\n(c)[d]\n", want: []position{{7, 1}}},
		{name: "fence in an item numbered 1 and a column-0 fence", input: "1. a\n   $$\n   x\n$$\n(a)[b]\n$$\n(c)[d]\n", want: []position{{7, 1}}},
		{name: "fence in a list from 3 after a blank line", input: "Intro\n\n3. a\n   $$\n   x\n$$\n(a)[b]\n$$\n(c)[d]\n", want: []position{{9, 1}}},
		{name: "math in a fenced div", input: "::: {.callout-note}\n$$\nf(x)[y]\n$$\n:::\n\n$$\n(a)[b]\n$$\n(c)[d]\n", want: []position{{10, 1}}},
		{name: "math in a Pandoc div", input: "::: note\n$$\nf(x)[y]\n$$\n:::\n", want: []position{}},
		// No math, and markdownlint reads none here either: the line is
		// paragraph text, or the math markdownlint reads ends before the
		// reversed link.
		{name: "fence indented four spaces after text", input: "Intro\n    $$\n\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "fence after a tab after text", input: "Intro\n\t$$\n\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "ordered marker 3 after text", input: "Intro\n3. $$\n\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "ordered marker 01 after text", input: "Intro\n01. $$\n    (a)[b]\n", want: []position{{3, 5}}},
		{name: "quoted fence indented four spaces", input: "> Intro\n>     $$\n> (a)[b]\n", want: []position{{3, 3}}},
		{name: "ordered marker 3 in a quote after text", input: "Intro\n> 3. $$\n>    (a)[b]\n", want: []position{{3, 6}}},
		{name: "item fence indented four columns", input: "- a\n      $$\n  (a)[b]\n", want: []position{{3, 3}}},
		{name: "ordered marker 3 after indented code", input: "    code\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "ordered marker 3 in an item after text", input: "Intro\n- 3. $$\n    (a)[b]\n", want: []position{{3, 5}}},
		{name: "math in an item after a heading ends there", input: "# T\n3. $$\n   x\n   $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "math in a second list ends there", input: "1. a\n2) $$\n   x\n   $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "math in an item after a quote ends there", input: "> Intro\n3. $$\n   x\n   $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "item after math", input: "$$\nx\n$$\n3. a\n   $$\n   y\n(a)[b]\n", want: []position{{7, 1}}},
		{name: "item in a table", input: "| a |\n| - |\n3. a\n   $$\n   y\n(a)[b]\n", want: []position{{6, 1}}},
		{name: "item after math with a lazy line", input: "$$\nx\n$$\n3. a\nb\n   $$\n   y\n(a)[b]\n", want: []position{{8, 1}}},
		{name: "math closed in an item after math", input: "$$\nx\n$$\n3. a\n   $$\n   y\n   $$\n   (a)[b]\n", want: []position{{8, 4}}},
		{name: "blank line in an item after math", input: "$$\nx\n$$\n3. a\n\n   $$\n   y\n(a)[b]\n", want: []position{{8, 1}}},
		{name: "later line of an item marked 01", input: "Intro\n01. a\n    $$\n    (a)[b]\n", want: []position{{4, 5}}},
		{name: "item marked 3 after an item marked 01", input: "Intro\n01. a\n3. $$\n   (a)[b]\n", want: []position{{4, 4}}},
		{name: "item marked 02 after an item marked 01", input: "Intro\n01. a\n02. $$\n    (a)[b]\n", want: []position{{4, 5}}},
		{name: "item marked 4 after an item marked 3 after code", input: "    code\n3. a\n4. $$\n   (a)[b]\n", want: []position{{4, 4}}},
		{name: "item after a refused item that ends in indented code", input: "Intro\n01. a\n\n        code\n\n3. $$\n   (a)[b]\n", want: []position{{7, 4}}},
		{name: "later line of an item marked 10 after code", input: "    code\n10. a\n    $$\n    (a)[b]\n", want: []position{{4, 5}}},
		{name: "fence in an item marked 01 and its closing", input: "Intro\n01. $$\n    x\n    $$\n    (a)[b]\n", want: []position{{5, 5}}},
		{name: "ordered marker 3 after code and a blank line", input: "    code\n\n3. $$\n   (a)[b]\n", want: []position{{4, 4}}},
		{name: "quoted ordered marker 3 after code and a blank line", input: ">     code\n>\n> 3. $$\n>    (a)[b]\n", want: []position{{4, 6}}},
		{name: "ordered marker 3 after a definition", input: "[x]: /url\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "ordered marker 3 in a nested quote after text", input: "> Intro\n>\t> 3. $$\n>\t>    (a)[b]\n", want: []position{{3, 8}}},
		{name: "fence indented four spaces after text with CRLF", input: "Intro\r\n    $$\r\n\r\n(a)[b]\r\n", want: []position{{4, 1}}},
		{name: "ordered marker 3 after code with CRLF", input: "    code\r\n3. $$\r\n   (a)[b]\r\n", want: []position{{3, 4}}},
		{name: "ordered marker 3 after code after a byte order mark", input: "\ufeff    code\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "spaced ordered marker 3 after code", input: "    code\n 3. $$\n    (a)[b]\n", want: []position{{3, 5}}},
		{name: "fence after code in an item in a table", input: "| a |\n| - |\n3. a\n   ```\n   ```\n   $$\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{7, 1}}},
		{name: "indented fence after math holding a list marker", input: "$$\n- a\n$$\n    $$\n\n  (a)[b] <i> ![](x) [missing][label]\n", want: []position{{6, 3}}},
		// Where micromark may read the blocks before a fence differently from
		// goldmark, gomdlint reads no more math blocks, and reports as main
		// does: markdownlint's results here. Main reports (a)[b] on line 6 of
		// the first two too, which markdownlint reads as math, and gomdlint
		// reads no front matter past the dots, where markdownlint does. With a
		// carriage return alone, markdownlint numbers the lines differently.
		{name: "code fence opened in math", input: "$$\n```\n$$\n$$\n```\n(a)[b]\n$$\n(c)[d]\n", want: []position{{6, 1}, {8, 1}}},
		{name: "HTML block opened in math", input: "$$\n<div>\n$$\n$$\n</div>\n(a)[b]\n$$\n(c)[d]\n", want: []position{{6, 1}, {8, 1}}},
		{name: "HTML block opened in math and micromark's after it", input: "$$\n<div>\n$$\n<del>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{7, 1}}},
		{name: "comment after math that opened an HTML block", input: "$$\n<div>\n$$\n<!--\n$$\n\n-->\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{9, 1}}},
		{name: "tilde fence after math that opened a code block", input: "$$\n```\n$$\n~~~\n$$\n```\n<div>\n~~~\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{10, 1}}},
		{name: "closing fence that micromark reads as opening", input: "$$\n```\n$$\n```\n$$\n<div>\n```\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{9, 1}}},
		{name: "HTML block after math", input: "$$\nx\n$$\n<del>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{7, 1}}},
		{name: "HTML block after a table row", input: "| a |\n| - |\n<del>\n$$\n(a)[b]\n\n(c)[d]\n", want: []position{{5, 1}, {7, 1}}},
		{name: "tag that goldmark does not read", input: "<del\t>\n$$\n\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "HTML block in an item that holds math", input: "- a\n$$\nx\n$$\n  <div>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{8, 1}}},
		{name: "HTML block in an item opened in math", input: "$$\n- a\n$$\n  <div>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{7, 1}}},
		{name: "text after an HTML block in an item that holds math", input: "- a\n$$\nx\n$$\n  <div>\nfoo\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{9, 1}}},
		{name: "math in a directive", input: ":::note\n$$\nx\n:::\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{6, 1}}},
		{name: "closed math in a directive", input: ":::note\n$$\nx\n$$\n:::\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{7, 1}}},
		{name: "front matter closed with dots", input: "---\na: b\n...\n$$\n(a)[b]\n---\n(c)[d]\n", want: []position{{5, 1}, {7, 1}}},
		{name: "carriage return alone after a fence", input: "$$\nx\n$$\r(a)[b]\n\n(c)[d]\n", want: []position{{3, 4}, {5, 1}}},
		{name: "prices around math", input: "Costs $5\n$$\nx\n$$\nSee (a)[b] <i> ![](x) [missing][label] for $6.\n", want: []position{{5, 5}}},
		{name: "prices around math in an item", input: "- Costs $5\n  $$\n  x\n  $$\n  See (a)[b] <i> ![](x) [missing][label] for $6.\n", want: []position{{5, 7}}},
		{name: "indented fence after text", input: "Intro\n  $$\nx\n$$\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{5, 1}}},
		{name: "indented fence on the first line", input: " $$\nx\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{5, 1}}},
		{name: "indented fence on a lazy line of a quote", input: "> a\n  $$\nx\n$$\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{5, 1}}},
		{name: "indented fence on a lazy line of an item", input: "- a\n $$\nx\n$$\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{5, 1}}},
		{name: "indented fence in an item that micromark does not read", input: "    code\n3. a\n   $$\n   x\n$$\n(a)[b] <i> ![](x) [missing][label]\n", want: []position{{6, 1}}},
		{name: "code block in an item that micromark does not read", input: "    code\n3. ```\n   $$\n   x\n   ```\n$$\n(a)[b]\n", want: []position{{7, 1}}},
		{name: "front matter opened with a no-break space", input: "---\u00a0\na: b\n$$\n---\n(a)[b]\n", want: []position{{5, 1}}},
		{name: "two byte order marks", input: "\ufeff\ufeff$$\nx\n$$\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "prices around a footnote definition", input: "Costs $5\n[^1]: See (a)[b] <i> ![](x) [missing][label] for $6.\n\nUse [^1].\n", want: []position{{2, 11}}},
		{name: "prices around a container directive", input: "Costs $5\n:::note\nSee (a)[b] <i> ![](x) [missing][label] for $6.\n:::\n", want: []position{{3, 5}}},
		{name: "prices around a leaf directive", input: "Costs $5\n::note\nSee (a)[b] <i> ![](x) [missing][label] for $6.\n", want: []position{{3, 5}}},
		{name: "inline math after a tag that goldmark does not read", input: "<del\t>\n$x (a)[b] y$\n", want: []position{{2, 4}}},
		{name: "inline math in a table after a tag line", input: "| a |\n| - |\n<del>\n$x (a)[b] y$\n", want: []position{{4, 4}}},
		// Indented or contained math, which markdownlint reads and gomdlint
		// does not: gomdlint reads a math block only from a fence at column 0,
		// so MD011 reports here, as main does.
		{name: "fence indented three spaces after a blank line", input: "Intro\n\n   $$\n\n(a)[b]\n", want: []position{{5, 1}}},
		{name: "fence indented three spaces after text", input: "Intro\n   $$\n\n(a)[b]\n", want: []position{{4, 1}}},
		{name: "quoted fence after quoted text", input: "> Intro\n> $$\n> (a)[b]\n", want: []position{{3, 3}}},
		{name: "quoted fence after a tab", input: "> Intro\n>\t$$\n> (a)[b]\n", want: []position{{3, 3}}},
		{name: "item fence indented three columns", input: "- a\n     $$\n  (a)[b]\n", want: []position{{3, 3}}},
		{name: "ordered marker 1 after text", input: "Intro\n1. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "second item of an ordered list", input: "1. a\n2. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "ordered marker 01 after a blank line", input: "Intro\n\n01. $$\n    (a)[b]\n", want: []position{{4, 5}}},
		{name: "bullet in a quote after text", input: "Intro\n> - $$\n>   (a)[b]\n", want: []position{{3, 5}}},
		{name: "ordered marker 3 after a heading", input: "# T\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "ordered marker 3 after a thematic break", input: "Para\n\n***\n3. $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "ordered marker 3 after a table", input: "| a |\n| - |\n3. $$\n   (a)[b]\n", want: []position{{4, 4}}},
		{name: "ordered marker 3 in an empty item", input: "-\n  3. $$\n     (a)[b]\n", want: []position{{3, 6}}},
		{name: "ordered marker 3 after math", input: "$$\nx\n$$\n3. $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "ordered marker 3 after a quote", input: "> Intro\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "ordered marker 3 after a bullet item", input: "- a\n3. $$\n   (a)[b]\n", want: []position{{3, 4}}},
		{name: "tab after the marker of a quoted item after math", input: "> $$\n> x\n> $$\n>3.\t$$\n>    (a)[b]\n", want: []position{{5, 6}}},
		{name: "list nested in an item after math", input: "$$\nx\n$$\n3. a\n   - $$\n     (a)[b]\n", want: []position{{6, 6}}},
		{name: "item marked 1 nested in an item after math", input: "$$\nx\n$$\n3. a\n   1. $$\n      (a)[b]\n", want: []position{{6, 7}}},
		{name: "quote in an item after math", input: "$$\nx\n$$\n3. a\n   > $$\n   > (a)[b]\n", want: []position{{6, 6}}},
		{name: "list nested in an item in a table", input: "| a |\n| - |\n3. a\n   - $$\n     (a)[b]\n", want: []position{{5, 6}}},
		{name: "next item in a table", input: "| a |\n| - |\n3. a\n4. $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "next item after math", input: "$$\nx\n$$\n3. a\n4. $$\n   (a)[b]\n", want: []position{{6, 4}}},
		{name: "item after a blank line after a refused item", input: "    code\n3. a\n\n4. $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "item marked 1 after a refused item", input: "    code\n3. a\n1. $$\n   (a)[b]\n", want: []position{{4, 4}}},
		{name: "fence interrupting text after a refused item", input: "    code\n3. $$\n   x\n   $$\n   (a)[b]\n", want: []position{{5, 4}}},
		{name: "spaced bullet after text", input: "Intro\n - $$\n   (a)[b]\n", want: []position{{3, 4}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewReversedLinkRule(), tt.input, nil)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRulesReportWhereGomdlintReadsNoMath checks the four rules that skip
// math against markdownlint 0.41.1 where gomdlint reads no math: after a
// fence in a list item, after an indented fence, and after fences where
// micromark may read the blocks before them differently from goldmark.
// main reports the same, and also <div> on line 2 of the HTML block opened
// in math, which markdownlint reads as math.
func TestRulesReportWhereGomdlintReadsNoMath(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  map[string][]position // By rule ID.
	}{
		{
			name:  "fence after code in an item in a table",
			input: "| a |\n| - |\n3. a\n   ```\n   ```\n   $$\n(a)[b] <i> ![](x) [missing][label]\n",
			want:  map[string][]position{"MD011": {{7, 1}}, "MD033": {{7, 8}}, "MD045": {{7, 12}}, "MD052": {{7, 19}}},
		},
		{
			name:  "indented fence after math holding a list marker",
			input: "$$\n- a\n$$\n    $$\n\n  (a)[b] <i> ![](x) [missing][label]\n",
			want:  map[string][]position{"MD011": {{6, 3}}, "MD033": {{6, 10}}, "MD045": {{6, 14}}, "MD052": {{6, 21}}},
		},
		{
			name:  "HTML block opened in math and micromark's after it",
			input: "$$\n<div>\n$$\n<del>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n",
			want: map[string][]position{
				"MD011": {{7, 1}}, "MD033": {{2, 1}, {4, 1}, {7, 8}}, "MD045": {{7, 12}}, "MD052": {{7, 19}},
			},
		},
		{
			name:  "HTML block after math",
			input: "$$\nx\n$$\n<del>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n",
			want:  map[string][]position{"MD011": {{7, 1}}, "MD033": {{4, 1}, {7, 8}}, "MD045": {{7, 12}}, "MD052": {{7, 19}}},
		},
		{
			name:  "HTML block in an item that holds math",
			input: "- a\n$$\nx\n$$\n  <div>\n$$\n\n(a)[b] <i> ![](x) [missing][label]\n",
			want:  map[string][]position{"MD011": {{8, 1}}, "MD033": {{5, 3}, {8, 8}}, "MD045": {{8, 12}}, "MD052": {{8, 19}}},
		},
		{
			name:  "math in a directive",
			input: ":::note\n$$\nx\n:::\n\n(a)[b] <i> ![](x) [missing][label]\n",
			want:  map[string][]position{"MD011": {{6, 1}}, "MD033": {{6, 8}}, "MD045": {{6, 12}}, "MD052": {{6, 19}}},
		},
		{
			name:  "prices around math",
			input: "Costs $5\n$$\nx\n$$\nSee (a)[b] <i> ![](x) [missing][label] for $6.\n",
			want:  map[string][]position{"MD011": {{5, 5}}, "MD033": {{5, 12}}, "MD045": {{5, 16}}, "MD052": {{5, 23}}},
		},
	}
	rules := []lint.Rule{NewReversedLinkRule(), NewInlineHTMLRule(), NewImageAltTextRule(), NewReferenceLinkImagesRule()}
	for _, tt := range tests {
		for _, rule := range rules {
			t.Run(tt.name+" "+rule.ID(), func(t *testing.T) {
				diags := applyRuleWithOptions(t, rule, tt.input, nil)
				got := make([]position, 0, len(diags))
				for _, diag := range diags {
					got = append(got, position{diag.StartLine, diag.StartColumn})
				}
				assert.Equal(t, tt.want[rule.ID()], got)
			})
		}
	}
}

// TestHTMLBlockStart checks which lines could start an HTML block, by the
// kinds CommonMark defines, for the math guards: inline HTML starts none.
func TestHTMLBlockStart(t *testing.T) {
	tests := []struct {
		line string
		want htmlKind
	}{
		{line: "<!-- note", want: htmlHoldsBlankLines},
		{line: "<?php echo 1; ?>", want: htmlHoldsBlankLines},
		{line: "<!DOCTYPE html>", want: htmlHoldsBlankLines},
		{line: "<![CDATA[ x", want: htmlHoldsBlankLines},
		{line: "<script>", want: htmlHoldsBlankLines},
		{line: "<PRE class=x>", want: htmlHoldsBlankLines},
		{line: "<textarea", want: htmlHoldsBlankLines},
		{line: "<div>", want: htmlEndsAtBlankLine},
		{line: "</div>", want: htmlEndsAtBlankLine},
		{line: "<p/>", want: htmlEndsAtBlankLine},
		{line: "<param>", want: htmlEndsAtBlankLine},
		{line: "<table class=x>text", want: htmlEndsAtBlankLine},
		{line: "<del>", want: htmlEndsAtBlankLine},
		{line: "</del>  ", want: htmlEndsAtBlankLine},
		{line: "<img src=\"a.png\">", want: htmlEndsAtBlankLine},
		{line: "<b>Note:</b> text", want: htmlNone},
		{line: "<https://example.com> text", want: htmlNone},
		{line: "<", want: htmlNone},
		{line: "text <div>", want: htmlNone},
	}
	for _, tt := range tests {
		got, _ := htmlBlockStart([]byte(tt.line))
		assert.Equal(t, tt.want, got, "%q", tt.line)
	}
}

// TestImageAltTextRule_HTMLImages checks HTML img elements like markdownlint:
// an alt attribute (even an empty one) or aria-hidden="true" satisfies the rule.
func TestImageAltTextRule_HTMLImages(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
	}{
		{name: "inline img without alt", input: "Text <img src=\"a.png\"> here.\n", want: []position{{1, 6}}},
		{name: "inline img with alt", input: "Text <img src=\"a.png\" alt=\"A cat\"> here.\n"},
		{name: "empty alt marks a decorative image", input: "Text <img src=\"a.png\" alt=\"\"> here.\n"},
		{name: "aria-hidden image", input: "Text <img src=\"a.png\" aria-hidden=\"true\"> here.\n"},
		{name: "aria-hidden false still needs alt", input: "Text <img src=\"a.png\" aria-hidden=\"false\">.\n", want: []position{{1, 6}}},
		{name: "uppercase element name", input: "Text <IMG SRC=\"a.png\"> here.\n", want: []position{{1, 6}}},
		{name: "img inside HTML block", input: "<p>\n  <img src=\"a.png\">\n</p>\n", want: []position{{2, 3}}},
		{name: "img inside HTML comment", input: "<!-- <img src=\"a.png\"> -->\n"},
		{
			// markdownlint reads an HTML block's lines as Markdown, so an
			// img in a code span there is code.
			name:  "img in a code span in an HTML block",
			input: "<div>\n`<img src=\"a.png\">` code\n<img src=\"b.png\">\n</div>\n",
			want:  []position{{3, 1}},
		},
		{name: "other elements ignored", input: "<div class=\"x\">text</div>\n"},
		{name: "quoted HTML block img spanning lines", input: "> <img src=\"a.png\"\n> alt=\"x\">\n"},
		{name: "quoted inline img spanning lines", input: "> Text <img src=\"a.png\"\n> alt=\"x\"> more.\n"},
		{name: "quoted img without alt spanning lines", input: "> <img src=\"a.png\"\n> title=\"x\">\n", want: []position{{1, 3}}},
		{
			name:  "quoted imgs spanning lines in one paragraph",
			input: "> a <img src=\"a.png\"\n> alt=\"x\"> b <img src=\"b.png\"\n> title=\"y\"> c\n",
			want:  []position{{2, 14}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			diags, err := NewImageAltTextRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}
			if len(diags) != len(tt.want) {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(diags), len(tt.want), diags)
			}
			for idx, want := range tt.want {
				if diags[idx].StartLine != want.line || diags[idx].StartColumn != want.column {
					t.Errorf("diagnostic %d at %d:%d, want %d:%d", idx, diags[idx].StartLine, diags[idx].StartColumn, want.line, want.column)
				}
			}
		})
	}
}

func TestLinkDestinationStyleRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		style string
		wantN int
	}{
		{
			name:  "consistent style absolute first",
			input: "[a](http://example.com) and [b](https://example.org)",
			style: "consistent",
			wantN: 0,
		},
		{
			name:  "consistent style mixed",
			input: "[a](http://example.com) and [b](relative.md)",
			style: "consistent",
			wantN: 1,
		},
		{
			name:  "relative style enforced",
			input: "[a](relative.md) and [b](http://example.com)",
			style: "relative",
			wantN: 1,
		},
		{
			name:  "absolute style enforced",
			input: "[a](http://example.com) and [b](relative.md)",
			style: "absolute",
			wantN: 1,
		},
		{
			name:  "fragment only skipped",
			input: "[a](http://example.com) and [b](#anchor)",
			style: "consistent",
			wantN: 0,
		},
		{
			name:  "all relative",
			input: "[a](a.md) and [b](b.md)",
			style: "relative",
			wantN: 0,
		},
		{
			// An email autolink is a mailto: URI, which is absolute.
			name:  "email autolink is absolute",
			input: "[a](https://example.com) and <user@example.com>",
			style: "consistent",
			wantN: 0,
		},
		{
			name:  "email autolink sets the absolute style",
			input: "<user@example.com> and [b](docs/guide.md)",
			style: "consistent",
			wantN: 1,
		},
		{
			name:  "autolink under relative style",
			input: "[a](a.md) and <https://example.com>",
			style: "relative",
			wantN: 1,
		},
		// A destination with a URI scheme is absolute whatever the link
		// syntax, so an autolink and an inline link to it agree, in either
		// order.
		{name: "sms autolink then inline link", input: "<sms:+15551234567> and [text us](sms:+15551234567)", style: "consistent"},
		{name: "sms inline link then autolink", input: "[text us](sms:+15551234567) and <sms:+15551234567>", style: "consistent"},
		{name: "urn autolink then inline link", input: "<urn:isbn:0451450523> and [book](urn:isbn:0451450523)", style: "consistent"},
		{name: "urn inline link then autolink", input: "[book](urn:isbn:0451450523) and <urn:isbn:0451450523>", style: "consistent"},
		{
			name:  "vscode autolink then inline link",
			input: "<vscode:extension/ms-python.python> and [extension](vscode:extension/ms-python.python)",
			style: "consistent",
		},
		{
			name:  "vscode inline link then autolink",
			input: "[extension](vscode:extension/ms-python.python) and <vscode:extension/ms-python.python>",
			style: "consistent",
		},
		{name: "mailto autolink then inline link", input: "<mailto:team@example.com> and [mail us](mailto:team@example.com)", style: "consistent"},
		{name: "mailto inline link then autolink", input: "[mail us](mailto:team@example.com) and <mailto:team@example.com>", style: "consistent"},
		// A single letter before the colon is a Windows drive, so the
		// destination is a relative path.
		{name: "drive letter after an autolink", input: "<https://example.com> and [file](C:/docs/a.md)", style: "consistent", wantN: 1},
		{name: "drive letter before an autolink", input: "[file](C:/docs/a.md) and <https://example.com>", style: "consistent", wantN: 1},
		{name: "backslash drive letter after an autolink", input: "<https://example.com> and [file](C:\\docs\\a.md)", style: "consistent", wantN: 1},
		{name: "relative path after an autolink", input: "<https://example.com> and [guide](docs/guide.md)", style: "consistent", wantN: 1},
		{name: "relative path before an autolink", input: "[guide](docs/guide.md) and <https://example.com>", style: "consistent", wantN: 1},
		{name: "url in the query of a relative path", input: "<https://example.com> and [go](./a?u=https://b.example)", style: "consistent", wantN: 1},
		// A network-path reference names its own host, so it is absolute.
		{name: "protocol-relative after an autolink", input: "<https://example.com> and [cdn](//cdn.example.com/a.js)", style: "consistent"},
		{name: "protocol-relative before an autolink", input: "[cdn](//cdn.example.com/a.js) and <https://example.com>", style: "consistent"},
		// localhost:8080 starts with the scheme "localhost", as browsers and
		// CommonMark autolinks read it.
		{name: "host and port read as a scheme", input: "<https://example.com> and [dev](localhost:8080)", style: "consistent"},
		// The destination is classified as the renderer reads it, with
		// backslash escapes and character references resolved, in inline
		// links and in reference definitions alike.
		{name: "escaped colon after an autolink", input: "<https://example.com> and [site](https\\://example.com)", style: "consistent"},
		{name: "escaped colon before an autolink", input: "[site](https\\://example.com) and <https://example.com>", style: "consistent"},
		{name: "named reference after an autolink", input: "<sms:+15551234567> and [text us](sms&colon;+15551234567)", style: "consistent"},
		{name: "named reference before an autolink", input: "[text us](sms&colon;+15551234567) and <sms:+15551234567>", style: "consistent"},
		{name: "numeric reference after an autolink", input: "<sms:+15551234567> and [text us](sms&#58;+15551234567)", style: "consistent"},
		{name: "numeric reference before an autolink", input: "[text us](sms&#58;+15551234567) and <sms:+15551234567>", style: "consistent"},
		{
			name:  "escaped definition after an autolink",
			input: "<https://example.com> and [site][ref]\n\n[ref]: https\\://example.com",
			style: "consistent",
		},
		{
			name:  "escaped definition before an autolink",
			input: "[site][ref] and <https://example.com>\n\n[ref]: https\\://example.com",
			style: "consistent",
		},
		// Percent-encoding stays as written, so an encoded colon starts no
		// scheme.
		{name: "percent-encoded colon after an autolink", input: "<https://example.com> and [x](https%3A//example.com)", style: "consistent", wantN: 1},
		{name: "percent-encoded colon before an autolink", input: "[x](https%3A//example.com) and <https://example.com>", style: "consistent", wantN: 1},
		// A resolved "#" makes a fragment-only link, which sets no style.
		{name: "escaped fragment before an autolink", input: "[top](\\#top) and <https://example.com>", style: "consistent"},
		{name: "numeric fragment before an autolink", input: "[top](&#35;top) and <https://example.com>", style: "consistent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewLinkDestinationStyleRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"style": tt.style,
				},
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

// TestImageAltTextRule_HTMLImagesKeepMaskedTags checks that MD045 keeps the
// tag of an img element that spans lines in a blockquote with the quote
// markers of its later lines replaced by spaces, as the parser read it. The
// rule reads the masked tag from a copy of the file that is restored after
// each element, so it must keep its own copy of the tag.
func TestImageAltTextRule_HTMLImagesKeepMaskedTags(t *testing.T) {
	input := "> a <img src=\"a.png\"\n> alt=\"x\"> b <img\n> src=\"b.png\">\n"
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)

	ctx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
	images := NewImageAltTextRule().htmlImages(ctx, &lazyMath{ctx: ctx})

	require.Len(t, images, 2)
	assert.Equal(t, "<img src=\"a.png\"\n  alt=\"x\">", string(images[0].tag))
	assert.Equal(t, "<img\n  src=\"b.png\">", string(images[1].tag))
}

// TestLinkDestinationStyleRule_GFMAutolinkLiterals checks that GFM autolink
// literals count as absolute links. The parser gives a bare email address
// without its mailto: scheme, but it renders as a mailto: link.
func TestLinkDestinationStyleRule_GFMAutolinkLiterals(t *testing.T) {
	for _, input := range []string{
		"See [docs](https://example.com) or mail user@example.com today.\n",
		"See [docs](https://example.com) or visit www.example.com today.\n",
	} {
		t.Run(input, func(t *testing.T) {
			snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			require.Len(t, lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil).Links(), 2,
				"the literal is parsed as a link")

			diags, err := NewLinkDestinationStyleRule().Apply(
				lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			require.NoError(t, err)
			assert.Empty(t, diags)
		})
	}
}

// TestLinkDestinationStyleRule_DefaultEnabled checks that MDL001, a gomdlint
// rule that markdownlint does not have, runs by default like every
// markdownlint rule.
func TestLinkDestinationStyleRule_DefaultEnabled(t *testing.T) {
	rule := NewLinkDestinationStyleRule()
	if !rule.DefaultEnabled() {
		t.Error("LinkDestinationStyleRule should be enabled by default")
	}
}

// TestReversedLinkRule_MarkdownlintMatches finds reversed links the way
// markdownlint does: the text holds no parentheses, the opening parenthesis
// is not escaped, neither part ends with a backslash, the destination is not
// a footnote label, no "(" follows, and code spans are not searched. In
// "(a)[b](c)[d]" the "(c)" is also the destination of the link "[b](c)", so
// the fix, which would break that link, is withheld.
func TestReversedLinkRule_MarkdownlintMatches(t *testing.T) {
	type position struct {
		line, column int
		fixable      bool
	}
	input := "(text)[url]\n\n" +
		"(a)[b] and (c)[d]\n\n" +
		"[t](bad dest (text)[https://example.com]\n\n" +
		"\\(text)[url]\n\n" +
		"(text)[url](x)\n\n" +
		"(text\\)[url]\n\n" +
		"(text)[^1]\n\n" +
		"`(text)[url]` and `x (text)`[url]\n\n" +
		"(a (b) c)[url]\n\n" +
		"x(text)[url]\n\n" +
		"(a)[b](c)[d]\n\n" +
		"(x\\)[y](a)[b]\n"
	want := []position{{1, 1, true}, {3, 1, true}, {3, 12, true}, {5, 14, true}, {19, 2, true}, {21, 7, false}, {23, 8, false}}
	snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	diags, err := NewReversedLinkRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
	require.NoError(t, err)
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn, diag.HasFix()})
	}
	assert.Equal(t, want, got)
}

// TestLinkSpacesRule_FixKeepsAnchors withholds the fix for spaces in link
// text inside a heading, since trimming them changes the heading's anchor.
func TestLinkSpacesRule_FixKeepsAnchors(t *testing.T) {
	input := "# [ x](url)\n\n[link](#-x)\n"
	snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	diags, err := NewLinkSpacesRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
	require.NoError(t, err)
	require.Len(t, diags, 1)
	assert.False(t, diags[0].HasFix())
}

// TestIsAbsoluteDestination checks how MDL001 classifies a destination. It
// is absolute when it starts with a URI scheme as CommonMark defines one for
// autolinks, 2 to 32 characters long, or with "//". A single letter before
// the colon is a Windows drive, so those destinations stay relative.
func TestIsAbsoluteDestination(t *testing.T) {
	tests := map[string]bool{
		"https://example.com/a":             true,
		"HTTPS://EXAMPLE.COM":               true,
		"sms:+15551234567":                  true,
		"urn:isbn:0451450523":               true,
		"vscode:extension/ms-python.python": true,
		"mailto:team@example.com":           true,
		"tel:+15551234567":                  true,
		"localhost:8080":                    true,
		"ab:x":                              true,
		"a+b.c-d:x":                         true,
		strings.Repeat("a", 32) + ":x":      true,
		"//cdn.example.com/a.js":            true,
		"C:/docs/a.md":                      false,
		"C:\\docs\\a.md":                    false,
		"c:x":                               false,
		strings.Repeat("a", 33) + ":x":      false,
		"1ab:x":                             false,
		"a_b:x":                             false,
		":x":                                false,
		"docs/guide.md":                     false,
		"./a?u=https://b.example":           false,
		"/root/path.md":                     false,
		"../up.md":                          false,
		"a.md#section":                      false,
		"user@example.com":                  false,
		"/":                                 false,
	}
	for dest, want := range tests {
		assert.Equal(t, want, isAbsoluteDestination(dest), dest)
	}
}
