package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestInlineHTMLRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		allowed []any
		wantN   int
	}{
		{
			name:    "no html",
			input:   "Just plain text.",
			allowed: nil,
			wantN:   0,
		},
		{
			name:    "html block not allowed",
			input:   "<div>content</div>",
			allowed: nil,
			wantN:   1,
		},
		{
			name:    "inline html not allowed",
			input:   "Text with <span>inline</span> html.",
			allowed: nil,
			wantN:   1, // Report the opening tag once.
		},
		{
			name:    "allowed element",
			input:   "Line break<br>here.",
			allowed: []any{"br"},
			wantN:   0,
		},
		{
			name:    "mixed allowed and not allowed",
			input:   "Text<br>with<span>mixed</span>.",
			allowed: []any{"br"},
			wantN:   1, // Report the disallowed opening span once.
		},
		{
			name:    "self closing tag allowed",
			input:   "Text<br/>here.",
			allowed: []any{"br"},
			wantN:   0,
		},
		{
			name:    "case insensitive",
			input:   "Text<BR>here.",
			allowed: []any{"br"},
			wantN:   0,
		},
		{
			name:    "multiple allowed elements",
			input:   "Text<sup>a</sup> and <sub>b</sub>.",
			allowed: []any{"sup", "sub"},
			wantN:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewInlineHTMLRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.allowed != nil {
				ruleCfg = &config.RuleConfig{
					Options: map[string]any{
						"allowed_elements": tt.allowed,
					},
				}
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

// TestInlineHTMLRule_DefaultEnabled checks that MD033 runs by default, as in
// markdownlint.
func TestInlineHTMLRule_DefaultEnabled(t *testing.T) {
	assert.True(t, NewInlineHTMLRule().DefaultEnabled())
}

// applyHTMLRule runs MD033 on input parsed with the given flavor and options.
func applyHTMLRule(t *testing.T, flavor config.Flavor, input string, options map[string]any) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	cfg := config.NewConfig()
	cfg.Flavor = flavor
	var ruleCfg *config.RuleConfig
	if options != nil {
		ruleCfg = &config.RuleConfig{Options: options}
	}
	diags, err := NewInlineHTMLRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg))
	require.NoError(t, err)
	return diags
}

type htmlDiagWant struct {
	line, column int
	element      string
}

func requireHTMLDiags(t *testing.T, diags []lint.Diagnostic, want []htmlDiagWant) {
	t.Helper()
	require.Len(t, diags, len(want))
	for idx, expected := range want {
		assert.Equal(t, expected.line, diags[idx].StartLine, "diagnostic %d line", idx)
		assert.Equal(t, expected.column, diags[idx].StartColumn, "diagnostic %d column", idx)
		assert.Equal(t, "HTML element '"+expected.element+"' is not allowed", diags[idx].Message,
			"diagnostic %d message", idx)
	}
}

// TestInlineHTMLRule_EachElement reports every disallowed opening tag, including
// tags after the first one in an HTML block (markdownlint re-parses HTML blocks).
func TestInlineHTMLRule_EachElement(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		allowed []any
		want    []htmlDiagWant
	}{
		{
			name:    "disallowed tag after allowed tags in one block",
			input:   "<details>\n<summary>More</summary>\n<script>alert(1)</script>\n</details>\n",
			allowed: []any{"details", "summary"},
			want:    []htmlDiagWant{{3, 1, "script"}},
		},
		{
			name:  "every opening tag in a block",
			input: "<div><span>x</span></div>\n",
			want:  []htmlDiagWant{{1, 1, "div"}, {1, 6, "span"}},
		},
		{
			name:    "comment content is not markup",
			input:   "<div>\n<!-- <b>hidden</b> -->\n</div>\n",
			allowed: []any{"div"},
		},
		{
			name:  "declaration is not an element",
			input: "<!DOCTYPE html>\n",
		},
		{
			name:  "CDATA content is not markup",
			input: "<![CDATA[\n<b>not a tag</b>\n]]>\n",
		},
		{
			name:  "processing instruction reported like markdownlint",
			input: "<?xml version=\"1.0\"?>\n",
			want:  []htmlDiagWant{{1, 1, "?xml"}},
		},
		{
			name:  "multi-line opening tag",
			input: "<div\n  class=\"note\">\ntext\n</div>\n",
			want:  []htmlDiagWant{{1, 1, "div"}},
		},
		{
			name:  "block inside blockquote",
			input: "> <div>\n> <span>x</span>\n> </div>\n",
			want:  []htmlDiagWant{{1, 3, "div"}, {2, 3, "span"}},
		},
		{
			name:  "quoted tag spanning lines keeps its attributes",
			input: "> <div\n> title=\"<b>\">\n> x\n> </div>\n",
			want:  []htmlDiagWant{{1, 3, "div"}},
		},
		{
			name:  "quoted inline tag spanning lines keeps its attributes",
			input: "> Quote has <span\n> title=\"<b>\">x</span> text.\n",
			want:  []htmlDiagWant{{1, 13, "span"}},
		},
		{
			name:  "quoted inline tags spanning lines in one paragraph",
			input: "> a <span\n> title=\"<b>\">x</span> and <em\n> class=\"<i>\">z</em>\n",
			want:  []htmlDiagWant{{1, 5, "span"}, {2, 28, "em"}},
		},
		{
			name:  "inline element position",
			input: "Text with <span>inline</span> html.\n",
			want:  []htmlDiagWant{{1, 11, "span"}},
		},
		{
			name:  "invalid tag syntax is text",
			input: "<div>a < b and c<d</div>\n",
			want:  []htmlDiagWant{{1, 1, "div"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var options map[string]any
			if tt.allowed != nil {
				options = map[string]any{"allowed_elements": tt.allowed}
			}
			requireHTMLDiags(t, applyHTMLRule(t, config.FlavorCommonMark, tt.input, options), tt.want)
		})
	}
}

// TestInlineHTMLRule_DefaultAllowsNothing checks the markdownlint default
// allowed_elements of [] for every flavor.
func TestInlineHTMLRule_DefaultAllowsNothing(t *testing.T) {
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			requireHTMLDiags(t, applyHTMLRule(t, flavor, "Text<br>here.\n", nil), []htmlDiagWant{{1, 5, "br"}})
			requireHTMLDiags(t, applyHTMLRule(t, flavor, "Text<sup>a</sup>here.\n", nil), []htmlDiagWant{{1, 5, "sup"}})
		})
	}
}

// TestInlineHTMLRule_TableAllowedElements uses table_allowed_elements inside tables
// and falls back to allowed_elements when it is not set.
func TestInlineHTMLRule_TableAllowedElements(t *testing.T) {
	input := "Para<br>text.\n\n| A | B |\n| --- | --- |\n| x<br>y | z |\n"
	requireHTMLDiags(t, applyHTMLRule(t, config.FlavorGFM, input, map[string]any{
		"allowed_elements": []any{"br"},
	}), nil)
	requireHTMLDiags(t, applyHTMLRule(t, config.FlavorGFM, input, map[string]any{
		"allowed_elements":       []any{"br"},
		"table_allowed_elements": []any{},
	}), []htmlDiagWant{{5, 4, "br"}})
	requireHTMLDiags(t, applyHTMLRule(t, config.FlavorGFM, input, map[string]any{
		"table_allowed_elements": []any{"br"},
	}), []htmlDiagWant{{1, 5, "br"}})
}

// TestHTMLSourcesMaskOneNodeAtATime checks that htmlSources masks the
// quoted continuation lines of the node it is given and of no other node:
// the shared copy is restored after each call, and a node that needs no
// masking reads the file itself.
func TestHTMLSourcesMaskOneNodeAtATime(t *testing.T) {
	input := "> a <b\n> c=\"d\">e</b> and <i\n> j=\"k\">l</i>\n"
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	inlines := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil).HTMLInlines()
	require.Len(t, inlines, 4, "<b ...>, </b>, <i ...> and </i>")
	line2, line3 := snapshot.Lines[1].StartOffset, snapshot.Lines[2].StartOffset
	sources := htmlSources{file: snapshot}

	sources.with(inlines[0], func(source []byte) {
		assert.Equal(t, "  ", string(source[line2:line2+2]), "<b continues on line 2")
		assert.Equal(t, "> ", string(source[line3:line3+2]), "only <i continues on line 3")
	})
	sources.with(inlines[2], func(source []byte) {
		assert.Equal(t, "> ", string(source[line2:line2+2]), "line 2 is restored")
		assert.Equal(t, "  ", string(source[line3:line3+2]), "<i continues on line 3")
	})
	sources.with(inlines[1], func(source []byte) {
		assert.Equal(t, input, string(source), "</b> needs no masking")
	})
	assert.Equal(t, input, string(snapshot.Content), "the file is never masked")
}

// TestTableLineRanges checks the table line lookup that MD033 builds once
// per file: unsorted and overlapping ranges are joined, and a line is in a
// table from the table's first line to its last.
func TestTableLineRanges(t *testing.T) {
	ranges := tableLineRanges{{20, 22}, {3, 5}, {4, 9}, {30, 30}, {6, 7}}.merged()
	assert.Equal(t, tableLineRanges{{3, 9}, {20, 22}, {30, 30}}, ranges)

	for line, want := range map[int]bool{
		1: false, 2: false, 3: true, 7: true, 9: true, 10: false,
		19: false, 20: true, 22: true, 23: false, 29: false, 30: true, 31: false,
	} {
		assert.Equal(t, want, ranges.contains(line), "line %d", line)
	}
	assert.False(t, tableLineRanges(nil).merged().contains(1))
}

// TestInlineHTMLRule_TableAllowedElementsSeveralTables tells the lines of
// each table apart from the text before, between and after the tables,
// including a table inside a blockquote.
func TestInlineHTMLRule_TableAllowedElementsSeveralTables(t *testing.T) {
	input := "Para<br>one.\n\n" +
		"| A | B |\n| --- | --- |\n| x<br>y | z |\n\n" +
		"Between<br>tables.\n\n" +
		"> | C | D |\n> | --- | --- |\n> | p<br>q | r |\n\n" +
		"After<br>all.\n"
	requireHTMLDiags(t, applyHTMLRule(t, config.FlavorGFM, input, map[string]any{
		"allowed_elements":       []any{"br"},
		"table_allowed_elements": []any{},
	}), []htmlDiagWant{{5, 4, "br"}, {11, 6, "br"}})
	requireHTMLDiags(t, applyHTMLRule(t, config.FlavorGFM, input, map[string]any{
		"table_allowed_elements": []any{"br"},
	}), []htmlDiagWant{{1, 5, "br"}, {7, 8, "br"}, {13, 6, "br"}})
}
