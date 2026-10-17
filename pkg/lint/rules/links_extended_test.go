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

func TestNoBareURLsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "autolink URL",
			input:     "<https://example.com>\n",
			wantDiags: 0,
		},
		{
			name:      "bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
		},
		{
			name:      "URL in code span",
			input:     "`https://example.com`\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no URLs",
			input:     "Just some text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

// TestNoBareURLsRule_AutolinkLiterals follows markdownlint MD034, which reports
// GFM autolink literals: trailing punctuation and unbalanced parentheses are
// not part of a URL, and URLs in link text, code, or inline HTML content are
// not bare.
func TestNoBareURLsRule_AutolinkLiterals(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name  string
		input string
		want  []position
		fixed string
	}{
		{"trailing period", "Visit https://example.com/path.\n", []position{{1, 7}}, "Visit <https://example.com/path>.\n"},
		{"balanced parentheses", "See https://en.wikipedia.org/wiki/Foo_(bar) now\n", []position{{1, 5}},
			"See <https://en.wikipedia.org/wiki/Foo_(bar)> now\n"},
		{"inside parentheses", "Docs (see https://example.com/a) here\n", []position{{1, 11}},
			"Docs (see <https://example.com/a>) here\n"},
		{"unbalanced parenthesis before punctuation", "Docs (see https://example.com/a). here\n", []position{{1, 11}},
			"Docs (see <https://example.com/a>). here\n"},
		{"email with trailing period", "Mail user@example.com.\n", []position{{1, 6}}, "Mail <user@example.com>.\n"},
		{"entity after url", "Go to https://example.com/q?a=1&amp; now\n", []position{{1, 7}},
			"Go to <https://example.com/q?a=1>&amp; now\n"},
		{"inside emphasis", "Read *https://example.com* now\n", []position{{1, 7}}, "Read *<https://example.com>* now\n"},
		{"inside strikethrough", "Old ~~see https://example.com~~ now\n", []position{{1, 11}}, "Old ~~see <https://example.com>~~ now\n"},
		{"blockquote", "> Visit https://example.com\n", []position{{1, 9}}, "> Visit <https://example.com>\n"},
		{"multibyte text before url", "中文中文 https://example.com\n", []position{{1, 14}}, "中文中文 <https://example.com>\n"},
		{"url in link text", "[https://example.com](https://example.com)\n", nil, ""},
		{"url in brackets", "[https://example.com]\n", nil, ""},
		{"url after unclosed bracket", "[see https://example.com and more\n", nil, ""},
		{"url after closed bracket", "[note] https://example.com\n", []position{{1, 8}}, "[note] <https://example.com>\n"},
		{"url in code span", "Use `https://example.com` here\n", nil, ""},
		{"url in inline html content", "Use <span>https://example.com</span> here\n", nil, ""},
		{"url in html attribute", "Use <a href=\"https://example.com\">x</a> here\n", nil, ""},
		{"url in html block", "<div>\nhttps://example.com\n</div>\n", nil, ""},
		{"letter before protocol", "xhttps://example.com\n", nil, ""},
		{"autolink", "<https://example.com>\n", nil, ""},
		{"reference definition", "[ref]: https://example.com\n\n[ref]\n", nil, ""},
	}

	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, tt := range tests {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, len(tt.want), "diagnostics: %+v", diags)
				var edits []fix.TextEdit
				for idx, want := range tt.want {
					assert.Equal(t, want.line, diags[idx].StartLine, "diagnostic %d line", idx)
					assert.Equal(t, want.column, diags[idx].StartColumn, "diagnostic %d column", idx)
					edits = append(edits, diags[idx].FixEdits...)
				}
				if len(tt.want) == 0 {
					return
				}
				prepared, err := fix.PrepareEdits(edits, len(tt.input))
				require.NoError(t, err)
				assert.Equal(t, tt.fixed, string(fix.ApplyEdits([]byte(tt.input), prepared)))
			})
		}
	}
}

// TestNoBareURLsRule_WWWLiterals reports GFM "www." autolink literals like
// markdownlint, without a fix: wrapping one in angle brackets does not make
// an autolink (it has no scheme) and would show the brackets.
func TestNoBareURLsRule_WWWLiterals(t *testing.T) {
	input := "Visit www.example.com/path. today\n\n" +
		"See (www.example.com) and *www.example.org* and xwww.example.net here\n\n" +
		"[www.example.com/shortcut]\n"
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 3, "diagnostics: %+v", diags)
			for idx, want := range [][2]int{{1, 7}, {3, 6}, {3, 28}} {
				assert.Equal(t, want[0], diags[idx].StartLine, "diagnostic %d line", idx)
				assert.Equal(t, want[1], diags[idx].StartColumn, "diagnostic %d column", idx)
				assert.False(t, diags[idx].HasFix(), "diagnostic %d must not have a fix", idx)
			}
			assert.Equal(t, 1+len("www.example.com/path"), diags[0].EndColumn-diags[0].StartColumn+1)
		})
	}
}

func TestNoBareURLsRule_Metadata(t *testing.T) {
	rule := NewNoBareURLsRule()

	assert.Equal(t, "MD034", rule.ID())
	assert.Equal(t, "no-bare-urls", rule.Name())
	assert.Contains(t, rule.Tags(), "links")
	assert.True(t, rule.CanFix())
}

func TestNoBareURLsRule_EmailEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		// Test various email patterns that might cause issues
		{
			name:      "email at start of line",
			input:     "user@example.com is my email\n",
			wantDiags: 1,
			wantFix:   "<user@example.com> is my email\n",
		},
		{
			name:      "email at end of line",
			input:     "Contact user@example.com\n",
			wantDiags: 1,
			wantFix:   "Contact <user@example.com>\n",
		},
		{
			name:      "email alone on line",
			input:     "user@example.com\n",
			wantDiags: 1,
			wantFix:   "<user@example.com>\n",
		},
		{
			name:      "multiple emails on same line",
			input:     "Contact alice@test.com or bob@test.com\n",
			wantDiags: 2,
			wantFix:   "Contact <alice@test.com> or <bob@test.com>\n",
		},
		{
			name:      "email with complex domain",
			input:     "Send to user@sub.domain.example.com please\n",
			wantDiags: 1,
			wantFix:   "Send to <user@sub.domain.example.com> please\n",
		},
		{
			name:      "email with plus addressing",
			input:     "Send to user+tag@example.com please\n",
			wantDiags: 1,
			wantFix:   "Send to <user+tag@example.com> please\n",
		},
		{
			name:      "already wrapped email - should not match",
			input:     "Contact <user@example.com> for help\n",
			wantDiags: 0,
			wantFix:   "Contact <user@example.com> for help\n",
		},
		{
			name:      "email next to URL on same line",
			input:     "Visit https://example.com or email admin@example.com\n",
			wantDiags: 2,
			wantFix:   "Visit <https://example.com> or email <admin@example.com>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// CRITICAL: Verify idempotency — this is what would catch the infinite loop
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent — no re-detection after wrapping")
		})
	}
}

func TestNoBareURLsRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - already wrapped URL",
			input:     "Visit <https://example.com> for info\n",
			wantDiags: 0,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "single bare URL",
			input:     "Visit https://example.com for info\n",
			wantDiags: 1,
			wantFix:   "Visit <https://example.com> for info\n",
		},
		{
			name:      "multiple bare URLs same line",
			input:     "Check https://example.com and https://test.org today\n",
			wantDiags: 2,
			wantFix:   "Check <https://example.com> and <https://test.org> today\n",
		},
		{
			name:      "multiple bare URLs different lines",
			input:     "First https://example.com here\nSecond https://test.org there\n",
			wantDiags: 2,
			wantFix:   "First <https://example.com> here\nSecond <https://test.org> there\n",
		},
		{
			name:      "bare email",
			input:     "Contact a@b.co for help\n",
			wantDiags: 1,
			wantFix:   "Contact <a@b.co> for help\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "URL in code span - should be skipped",
			input:     "Use `https://example.com` in code\n",
			wantDiags: 0,
			wantFix:   "Use `https://example.com` in code\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBareURLsRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))

			// Verify idempotency (always)
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}

// TestNoBareURLsRule_WithholdsUnsafeFixes reports literals that angle brackets
// would not turn into the same autolink, without a fix: a URL holding '<',
// '>', or a backslash, and an email address whose domain an autolink does not
// accept.
func TestNoBareURLsRule_WithholdsUnsafeFixes(t *testing.T) {
	for _, input := range []string{
		"See https://example.com/a>b now.\n",
		"See \\[https://example.com\\] now.\n",
		"Mail foo@a_b.com now.\n",
	} {
		for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
			t.Run(string(flavor)+" "+input, func(t *testing.T) {
				snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, 1, "the literal is still reported: %+v", diags)
				assert.False(t, diags[0].HasFix(), "angle brackets would not make the same autolink")
			})
		}
	}
}

// TestNoBareURLsRule_ByteOrderMark treats the start of a file that begins
// with a byte order mark as the start of its text, as the parser does, so a
// "www." literal there is still reported.
func TestNoBareURLsRule_ByteOrderMark(t *testing.T) {
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			input := "\xEF\xBB\xBFwww.example.com and https://example.com\n"
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 2, "diagnostics: %+v", diags)
			assert.Equal(t, 4, diags[0].StartColumn, "the www literal starts after the mark")
		})
	}
}

// TestNoBareURLsRule_EntityInURL withholds the fix when the URL holds an
// entity reference: as text the entity is decoded ("a©b"), but an
// autolink shows and links to it as written. GitHub decodes it in a GFM
// literal too, so the fix is withheld under both flavors.
func TestNoBareURLsRule_EntityInURL(t *testing.T) {
	input := "See https://example.com/a&copy;b now.\n"
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		t.Run(string(flavor), func(t *testing.T) {
			snapshot, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			diags, err := NewNoBareURLsRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
			require.NoError(t, err)
			require.Len(t, diags, 1)
			assert.False(t, diags[0].HasFix(), "angle brackets would change the shown text and the destination")
		})
	}
}
