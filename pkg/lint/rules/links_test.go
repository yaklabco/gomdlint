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

// TestLinkSpacesRule_LinksAfterTaskCheckboxes checks MD039 where a list
// item starts with brackets. The parser reads them as a task checkbox as
// goldmark does, at the start of the item's first block, a heading
// included, so "[ ](/url)" there is no link, as on main, though markdownlint,
// which reads no checkboxes, reports it. A link after a checkbox is checked.
func TestLinkSpacesRule_LinksAfterTaskCheckboxes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantColumns []int
	}{
		{name: "heading in a list item", input: "- ## [ ](/url).\n", wantColumns: []int{}},
		{name: "quoted heading in a list item", input: "> - ## [ ](/url).\n", wantColumns: []int{}},
		{name: "link right after the brackets", input: "- [ ](https://example.org)\n", wantColumns: []int{}},
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

// TestRulesDoNotSkipMath checks that MD011, MD033, MD045 and MD052 read
// math as main does: goldmark parses no math, so a math block or inline
// math is Markdown to them, where markdownlint 0.41.1, which parses math,
// skips it. In the last four rows, markdownlint reports the same.
func TestRulesDoNotSkipMath(t *testing.T) {
	const tail = "(a)[b] <i> ![](x) [missing][label]"
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  map[string][]position // By rule ID.
	}{
		{
			name:  "math block",
			input: "$$\n" + tail + "\n$$\n",
			want:  map[string][]position{"MD011": {{2, 1}}, "MD033": {{2, 8}}, "MD045": {{2, 12}}, "MD052": {{2, 19}}},
		},
		{
			name:  "math block with a blank line",
			input: "$$\nx\n\n" + tail + "\n$$\n",
			want:  map[string][]position{"MD011": {{4, 1}}, "MD033": {{4, 8}}, "MD045": {{4, 12}}, "MD052": {{4, 19}}},
		},
		{
			name:  "inline math",
			input: "Inline $" + tail + "$ math.\n",
			want:  map[string][]position{"MD011": {{1, 9}}, "MD033": {{1, 16}}, "MD045": {{1, 20}}, "MD052": {{1, 27}}},
		},
		{
			name:  "inline math between double dollar signs",
			input: "Display $$" + tail + "$$ math.\n",
			want:  map[string][]position{"MD011": {{1, 11}}, "MD033": {{1, 18}}, "MD045": {{1, 22}}, "MD052": {{1, 29}}},
		},
		{
			name:  "math in a blockquote",
			input: "> $$\n> " + tail + "\n> $$\n",
			want:  map[string][]position{"MD011": {{2, 3}}, "MD033": {{2, 10}}, "MD045": {{2, 14}}, "MD052": {{2, 21}}},
		},
		{
			name:  "math in a list item",
			input: "- $$\n  " + tail + "\n  $$\n",
			want:  map[string][]position{"MD011": {{2, 3}}, "MD033": {{2, 10}}, "MD045": {{2, 14}}, "MD052": {{2, 21}}},
		},
		{
			name:  "math in an HTML block",
			input: "<div>\n$$\n<i> <img src=\"a.png\">\n$$\n</div>\n",
			want:  map[string][]position{"MD033": {{1, 1}, {3, 1}, {3, 5}}, "MD045": {{3, 5}}},
		},
		{
			name:  "inline math in an HTML block",
			input: "<div>\n$<i>$ and $<img src=\"a.png\">$\n</div>\n",
			want:  map[string][]position{"MD033": {{1, 1}, {2, 2}, {2, 12}}, "MD045": {{2, 12}}},
		},
		{
			name:  "math in a directive",
			input: ":::note\n$$\nx\n:::\n\n" + tail + "\n",
			want:  map[string][]position{"MD011": {{6, 1}}, "MD033": {{6, 8}}, "MD045": {{6, 12}}, "MD052": {{6, 19}}},
		},
		{
			name:  "list item started in math",
			input: "$$\n- a\n$$\n  $$\n$$\n" + tail + "\n",
			want:  map[string][]position{"MD011": {{6, 1}}, "MD033": {{6, 8}}, "MD045": {{6, 12}}, "MD052": {{6, 19}}},
		},
		{
			name:  "math in an item after indented code",
			input: "    code\n\n3. a\n   $$\n$$\n" + tail + "\n",
			want:  map[string][]position{"MD011": {{6, 1}}, "MD033": {{6, 8}}, "MD045": {{6, 12}}, "MD052": {{6, 19}}},
		},
		{
			name:  "math fences around an HTML block",
			input: "$$\n- a\n$$\n  <div>\n$$\n(a)[b]\n$$\n",
			want:  map[string][]position{"MD011": {{6, 1}}, "MD033": {{4, 3}}},
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
				want := tt.want[rule.ID()]
				if want == nil {
					want = []position{}
				}
				assert.Equal(t, want, got)
			})
		}
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

// TestLinkDestinationStyleRule checks which links MDL001 compares and how
// each style option reports them. A link counts only when it could be
// written in the other kind: a relative link, which an absolute URL could
// replace, or a file: URL that names no other host, which a relative link
// could replace. A link to another host, a URI such as mailto:, an autolink
// and a link to a fragment have one form, so they never set the consistent
// style and are never reported.
func TestLinkDestinationStyleRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		style string
		wantN int
	}{
		// A link to another host cannot be relative, so it sets no style,
		// and neither the consistent nor the relative style reports it.
		{name: "links to other hosts under the relative style", input: "[a](http://example.com) and [b](https://example.org)", style: "relative"},
		{name: "link to another host before a relative link", input: "[a](http://example.com) and [b](relative.md)", style: "consistent"},
		{name: "link to another host after a relative link", input: "[a](relative.md) and [b](http://example.com)", style: "consistent"},
		{name: "link to another host under the relative style", input: "[a](relative.md) and [b](http://example.com)", style: "relative"},
		// Any relative link could be an absolute URL, so the absolute style
		// reports each one, an absolute path or a query alike.
		{name: "relative link under the absolute style", input: "[a](http://example.com) and [b](relative.md)", style: "absolute", wantN: 1},
		{name: "absolute path under the absolute style", input: "[a](https://example.com) and [b](/docs/guide.md)", style: "absolute", wantN: 1},
		{name: "query under the absolute style", input: "[a](?tab=readme)", style: "absolute", wantN: 1},
		{name: "all relative", input: "[a](a.md) and [b](b.md)", style: "relative"},
		// A file: URL that names no other host names a local file, which a
		// relative link could name too.
		{name: "file URL after a relative link", input: "[a](docs/a.md) and [b](file:///home/user/b.md)", style: "consistent", wantN: 1},
		{name: "relative link after a file URL", input: "[b](file:///home/user/b.md) and [a](docs/a.md)", style: "consistent", wantN: 1},
		{name: "file URL under the relative style", input: "[b](file:///home/user/b.md)", style: "relative", wantN: 1},
		{name: "file URL for localhost", input: "[b](file://localhost/home/user/b.md)", style: "relative", wantN: 1},
		{name: "file URL with one slash", input: "[b](file:/home/user/b.md)", style: "relative", wantN: 1},
		{name: "file scheme in capitals", input: "[b](FILE:///home/user/b.md)", style: "relative", wantN: 1},
		{name: "localhost in capitals", input: "[b](file://LOCALHOST/home/user/b.md)", style: "relative", wantN: 1},
		{name: "file URL with a drive", input: "[b](file:///C:/docs/b.md)", style: "relative", wantN: 1},
		{name: "file URL on another host", input: "[b](file://server/share/b.md)", style: "relative"},
		{name: "UNC file URL", input: "[b](file:////server/share/b.md)", style: "relative"},
		{name: "file URL under the absolute style", input: "[b](file:///home/user/b.md)", style: "absolute"},
		{name: "file URL in a definition", input: "[a](docs/a.md) and [b][ref]\n\n[ref]: file:///home/user/b.md", style: "consistent", wantN: 1},
		// A fragment link points into this document, so it sets no style
		// and is never reported.
		{name: "fragment after a file URL", input: "[a](file:///home/user/a.md) and [b](#anchor)", style: "consistent"},
		{name: "fragment under the absolute style", input: "[b](#anchor)", style: "absolute"},
		// An autolink shows its URI as its text, so it has no relative form.
		// Email autolinks, which the parser gives without their mailto:
		// scheme, are URIs too, though the destination reads as relative.
		{name: "email autolink after a file URL", input: "[b](file:///home/user/b.md) and <user@example.com>", style: "consistent"},
		{name: "email autolink before a file URL", input: "<user@example.com> and [b](file:///home/user/b.md)", style: "consistent"},
		{name: "file autolink after a relative link", input: "[a](a.md) and <file:///home/user/b.md>", style: "consistent"},
		{name: "file autolink under the relative style", input: "[a](a.md) and <file:///home/user/b.md>", style: "relative"},
		{name: "autolinks under the absolute style", input: "<https://example.com> and <user@example.com>", style: "absolute"},
		// A URI with another scheme names no file that a relative link could
		// name, whatever the link syntax.
		{name: "mailto link after a relative link", input: "[a](docs/a.md) and [mail us](mailto:team@example.com)", style: "consistent"},
		{
			name: "URIs under the relative style",
			input: "[mail us](mailto:team@example.com), [text us](sms:+15551234567), [book](urn:isbn:0451450523) and " +
				"[extension](vscode:extension/ms-python.python)",
			style: "relative",
		},
		{
			name: "URIs under the absolute style",
			input: "[mail us](mailto:team@example.com), [text us](sms:+15551234567), [book](urn:isbn:0451450523) and " +
				"[extension](vscode:extension/ms-python.python)",
			style: "absolute",
		},
		// localhost:8080 starts with the scheme "localhost", as browsers and
		// CommonMark autolinks read it.
		{name: "host and port read as a scheme", input: "[a](docs/a.md) and [dev](localhost:8080)", style: "relative"},
		{name: "host and port under the absolute style", input: "[dev](localhost:8080)", style: "absolute"},
		// A network-path reference names its own host, so it is absolute and
		// cannot be relative.
		{name: "protocol-relative link after a relative link", input: "[a](docs/a.md) and [cdn](//cdn.example.com/a.js)", style: "consistent"},
		{name: "protocol-relative link under the relative style", input: "[cdn](//cdn.example.com/a.js)", style: "relative"},
		{name: "protocol-relative link under the absolute style", input: "[cdn](//cdn.example.com/a.js)", style: "absolute"},
		// A single letter before the colon is a Windows drive, so the
		// destination is a relative path.
		{name: "drive letter under the absolute style", input: "[file](C:/docs/a.md)", style: "absolute", wantN: 1},
		{name: "backslash drive letter under the absolute style", input: "[file](C:\\docs\\a.md)", style: "absolute", wantN: 1},
		{name: "drive letter after a file URL", input: "[b](file:///home/user/b.md) and [file](C:/docs/a.md)", style: "consistent", wantN: 1},
		{name: "url in the query of a relative path", input: "[go](./a?u=https://b.example)", style: "absolute", wantN: 1},
		// The destination is classified as the renderer reads it, with
		// backslash escapes and character references resolved, in inline
		// links and in reference definitions alike.
		{name: "escaped colon", input: "[site](https\\://example.com)", style: "absolute"},
		{name: "named reference", input: "[text us](sms&colon;+15551234567)", style: "absolute"},
		{name: "numeric reference", input: "[text us](sms&#58;+15551234567)", style: "absolute"},
		{name: "escaped definition", input: "[site][ref]\n\n[ref]: https\\://example.com", style: "absolute"},
		{name: "escaped file URL", input: "[a](docs/a.md) and [b](file\\:///home/user/b.md)", style: "consistent", wantN: 1},
		// Percent-encoding stays as written, so an encoded colon starts no
		// scheme.
		{name: "percent-encoded colon", input: "[x](https%3A//example.com)", style: "absolute", wantN: 1},
		// A resolved "#" makes a fragment-only link, which is never reported.
		{name: "escaped fragment", input: "[top](\\#top)", style: "absolute"},
		{name: "numeric fragment", input: "[top](&#35;top)", style: "absolute"},
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
	images := NewImageAltTextRule().htmlImages(ctx)

	require.Len(t, images, 2)
	assert.Equal(t, "<img src=\"a.png\"\n  alt=\"x\">", string(images[0].tag))
	assert.Equal(t, "<img\n  src=\"b.png\">", string(images[1].tag))
}

// TestLinkDestinationStyleRule_GFMAutolinkLiterals checks that GFM autolink
// literals, which are autolinks, never count, so they break no style that a
// file: URL sets. The parser gives a bare email address without its mailto:
// scheme, so its destination reads as relative, but it renders as a mailto:
// link.
func TestLinkDestinationStyleRule_GFMAutolinkLiterals(t *testing.T) {
	for _, input := range []string{
		"See [docs](file:///home/user/docs.md) or mail user@example.com today.\n",
		"See [docs](file:///home/user/docs.md) or visit www.example.com today.\n",
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

// TestLinkDestinationStyleRule_RepositoryDocuments lints excerpts that keep,
// in order, a sample of the links of the repository's README and of
// docs/golden-test-methodology.md. Both link to other sites and to files of
// the repository. A link to another host cannot be relative, so it sets no
// consistent style and the relative style allows it. Only the absolute style
// reports anything: each relative link, which an absolute URL could replace.
func TestLinkDestinationStyleRule_RepositoryDocuments(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantAbsolute []int // Lines that the absolute style reports.
	}{
		{
			name: "README",
			input: "# gomdlint\n\n" +
				"Inspired by [markdownlint](https://github.com/DavidAnson/markdownlint).\n\n" +
				"See [bench/PERFORMANCE.md](bench/PERFORMANCE.md) for benchmarks.\n\n" +
				"Download from [GitHub Releases](https://github.com/yaklabco/gomdlint/releases).\n\n" +
				"See [Known Limitations](#known-limitations).\n\n" +
				"This project uses [stave](https://github.com/yaklabco/stave) for builds.\n\n" +
				"[CI_CD.md](CI_CD.md) describes the CI workflows.\n\n" +
				"See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design.\n\n" +
				"MIT License, see [LICENSE](LICENSE) for details.\n\n" +
				"## Known Limitations\n",
			wantAbsolute: []int{5, 13, 15, 17},
		},
		{
			name: "golden test methodology",
			input: "# Golden Tests\n\n" +
				"Agents load [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) skills.\n\n" +
				"The [gap analysis skill](../.claude/skills/golden-test-gap-analysis.md) audits coverage.\n\n" +
				"The [authoring skill](../.claude/skills/golden-test-authoring.md) writes inputs.\n\n" +
				"The [verification skill](../.claude/skills/golden-test-verification.md) checks them.\n\n" +
				"Five [sub-agents](https://docs.anthropic.com/en/docs/claude-code/sub-agents) run in parallel.\n\n" +
				"See [markdownlint](https://github.com/DavidAnson/markdownlint) and " +
				"[#11](https://github.com/yaklabco/gomdlint/issues/11).\n\n" +
				"A new [bug-fixing skill](../.claude/skills/golden-test-bug-fixing.md) guides fixes.\n",
			wantAbsolute: []int{5, 7, 9, 15},
		},
	}

	for _, tt := range tests {
		for _, style := range []string{"consistent", "relative", "absolute"} {
			t.Run(tt.name+"/"+style, func(t *testing.T) {
				diags := applyRuleWithOptions(t, NewLinkDestinationStyleRule(), tt.input, map[string]any{"style": style})

				want := []int{}
				if style == "absolute" {
					want = tt.wantAbsolute
				}
				assert.Equal(t, want, diagLines(diags))
			})
		}
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

// TestDestinationKind checks whether MDL001 compares a link with a
// destination, that is, whether the destination could be written in the
// other kind, and the kind it then reads the destination as. Every relative
// link could, and so could a file: URL that names no other host. A link to
// another host, a URI with another scheme, an empty destination and a
// fragment could not, so they have no kind.
func TestDestinationKind(t *testing.T) {
	type kind struct {
		style  LinkDestStyle
		counts bool
	}
	relative := kind{LinkDestRelative, true}
	localFile := kind{LinkDestAbsolute, true}
	oneForm := kind{}
	tests := map[string]kind{
		"docs/guide.md":                     relative,
		"./guide.md":                        relative,
		"../up.md":                          relative,
		"/docs/guide.md":                    relative,
		"a.md#section":                      relative,
		"?tab=readme":                       relative,
		"C:/docs/a.md":                      relative,
		"file":                              relative,
		"":                                  oneForm,
		"#section":                          oneForm,
		"https://example.com/a":             oneForm,
		"HTTP://EXAMPLE.COM":                oneForm,
		"//cdn.example.com/a.js":            oneForm,
		"mailto:team@example.com":           oneForm,
		"urn:isbn:0451450523":               oneForm,
		"localhost:8080":                    oneForm,
		"files:///home/user/a.md":           oneForm,
		"file:///home/user/a.md":            localFile,
		"file:/home/user/a.md":              localFile,
		"file:///C:/docs/a.md":              localFile,
		"file://localhost/home/user/a.md":   localFile,
		"FILE://LOCALHOST/HOME/A.MD":        localFile,
		"file://localhost":                  localFile,
		"file://localhost?x#y":              localFile,
		"file://server/share/a.md":          oneForm,
		"file://localhost.example.com/a.md": oneForm,
		"file://localhost:8080/a.md":        oneForm,
		"file://user@localhost/a.md":        oneForm,
		// An empty host followed by "//" starts a UNC path, which names the
		// server after it (RFC 8089, Appendix E.3.2).
		"file:////server/share/a.md":  oneForm,
		"file://///server/share/a.md": oneForm,
		// RFC 3986 reads the drive here as a host, as gomdlint does.
		"file://C:/docs/a.md": oneForm,
	}
	for dest, want := range tests {
		style, counts := destinationKind(dest)
		assert.Equal(t, want, kind{style, counts}, dest)
	}
}
