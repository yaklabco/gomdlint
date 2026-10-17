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
			name:  "image with whitespace alt",
			input: "![   ](image.png)",
			wantN: 1,
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
		{name: "other elements ignored", input: "<div class=\"x\">text</div>\n"},
		{name: "quoted HTML block img spanning lines", input: "> <img src=\"a.png\"\n> alt=\"x\">\n"},
		{name: "quoted inline img spanning lines", input: "> Text <img src=\"a.png\"\n> alt=\"x\"> more.\n"},
		{name: "quoted img without alt spanning lines", input: "> <img src=\"a.png\"\n> title=\"x\">\n", want: []position{{1, 3}}},
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

func TestLinkDestinationStyleRule_DefaultDisabled(t *testing.T) {
	rule := NewLinkDestinationStyleRule()
	if rule.DefaultEnabled() {
		t.Error("LinkDestinationStyleRule should be disabled by default")
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
