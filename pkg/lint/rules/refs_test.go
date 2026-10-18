package rules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// testHelper parses markdown and runs a rule.
func testHelper(t *testing.T, rule lint.Rule, markdown string) []lint.Diagnostic {
	t.Helper()

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", []byte(markdown))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := &config.Config{}
	ruleCtx := lint.NewRuleContext(context.Background(), file, cfg, nil)

	diags, err := rule.Apply(ruleCtx)
	if err != nil {
		t.Fatalf("Rule.Apply failed: %v", err)
	}

	return diags
}

// testHelperWithOptions parses markdown and runs a rule with custom options,
// as the engine hands them to the rule (see engineOptions).
func testHelperWithOptions(t *testing.T, rule lint.Rule, markdown string, options map[string]interface{}) []lint.Diagnostic {
	t.Helper()

	parser := goldmark.New("gfm")
	file, err := parser.Parse(context.Background(), "test.md", []byte(markdown))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	cfg := &config.Config{}
	ruleCfg := &config.RuleConfig{Options: engineOptions(rule.ID(), options)}
	ruleCtx := lint.NewRuleContext(context.Background(), file, cfg, ruleCfg)

	diags, err := rule.Apply(ruleCtx)
	if err != nil {
		t.Fatalf("Rule.Apply failed: %v", err)
	}

	return diags
}

func TestLinkFragmentsRule(t *testing.T) {
	rule := NewLinkFragmentsRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "valid fragment",
			markdown: `# Hello World

[link](#hello-world)
`,
			want: 0,
		},
		{
			name: "invalid fragment",
			markdown: `# Hello World

[link](#nonexistent)
`,
			want: 1,
		},
		{
			name: "multiple headings",
			markdown: `# First Heading

## Second Heading

[first](#first-heading)
[second](#second-heading)
[invalid](#bad-fragment)
`,
			want: 1,
		},
		{
			name: "special top fragment",
			markdown: `# Heading

[back to top](#top)
`,
			want: 0,
		},
		{
			name: "github line reference",
			markdown: `# Heading

[see line 20](#L20)
[see lines 19-21](#L19-L21)
`,
			want: 0,
		},
		{
			name: "no fragment links",
			markdown: `# Heading

[external](https://example.com)
[relative](page.md)
`,
			want: 0,
		},
		{
			name: "empty document",
			markdown: `[link](#somewhere)
`,
			want: 1,
		},
		{
			name: "duplicate headings generate unique anchors",
			markdown: `# Hello

## Hello

[first](#hello)
[second](#hello-1)
[invalid](#hello-2)
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkFragmentsRule_IgnorePattern(t *testing.T) {
	rule := NewLinkFragmentsRule()

	markdown := `# Heading

[fig](#figure-1)
[tbl](#table-2)
`

	// Without ignore pattern - should report 2 issues
	diags := testHelper(t, rule, markdown)
	if len(diags) != 2 {
		t.Errorf("without pattern: got %d diagnostics, want 2", len(diags))
	}

	// With ignore pattern - should report 0 issues
	diags = testHelperWithOptions(t, rule, markdown, map[string]interface{}{
		"ignored_pattern": "^(figure|table)-\\d+$",
	})
	if len(diags) != 0 {
		t.Errorf("with pattern: got %d diagnostics, want 0", len(diags))
	}
}

func TestReferenceLinkImagesRule(t *testing.T) {
	rule := NewReferenceLinkImagesRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "defined reference",
			markdown: `[link][example]

[example]: https://example.com
`,
			want: 0,
		},
		{
			// Note: goldmark doesn't create Link nodes for undefined references,
			// so we can only test that defined references work correctly.
			name: "inline link no issue",
			markdown: `[link](https://example.com)
`,
			want: 0,
		},
		{
			name: "autolink no issue",
			markdown: `<https://example.com>
`,
			want: 0,
		},
		{
			name: "ignored label x",
			markdown: `- [x] Task item
`,
			want: 0, // [x] is in default ignored list
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkImageRefDefsRule(t *testing.T) {
	rule := NewLinkImageRefDefsRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name: "used definition",
			markdown: `[link][example]

[example]: https://example.com
`,
			want: 0,
		},
		{
			name: "unused definition",
			markdown: `Some text without links.

[unused]: https://example.com
`,
			want: 1,
		},
		{
			name: "duplicate definition",
			markdown: `[link][example]

[example]: https://first.com
[example]: https://second.com
`,
			want: 1, // second is duplicate
		},
		{
			name: "ignored definition",
			markdown: `Some text.

[//]: # "This is a comment"
`,
			want: 0, // // is in default ignored list
		},
		{
			name: "mixed used and unused",
			markdown: `[link][used]

[used]: https://used.com
[unused]: https://unused.com
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestLinkImageStyleRule(t *testing.T) {
	rule := NewLinkImageStyleRule()

	tests := []struct {
		name     string
		markdown string
		options  map[string]interface{}
		want     int
	}{
		{
			name:     "all styles allowed by default",
			markdown: `[inline](url) and <https://autolink.com>`,
			options:  nil,
			want:     0,
		},
		{
			name:     "inline disallowed",
			markdown: `[inline](url)`,
			options:  map[string]interface{}{"inline": false},
			want:     1,
		},
		{
			name:     "autolink disallowed",
			markdown: `<https://example.com>`,
			options:  map[string]interface{}{"autolink": false},
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags []lint.Diagnostic
			if tt.options != nil {
				diags = testHelperWithOptions(t, rule, tt.markdown, tt.options)
			} else {
				diags = testHelper(t, rule, tt.markdown)
			}
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

// TestLinkImageStyleRule_URLInline checks url_inline: false the way
// markdownlint 0.41 decides it. A link, not an image, without a title,
// whose text is its destination, is reported when autolinks are allowed and
// the destination could be written as a CommonMark autolink: a scheme of 2
// to 32 characters in any case, then no spaces, control characters, "<" or
// ">".
func TestLinkImageStyleRule_URLInline(t *testing.T) {
	urlInline := map[string]any{"url_inline": false}
	tests := []struct {
		name     string
		markdown string
		options  map[string]any
		want     int
	}{
		{name: "https", markdown: "[https://example.com](https://example.com)\n", want: 1},
		{name: "mailto", markdown: "[mailto:team@example.com](mailto:team@example.com)\n", want: 1},
		{name: "uppercase mailto", markdown: "[MAILTO:team@example.com](MAILTO:team@example.com)\n", want: 1},
		{name: "sms", markdown: "[sms:+15551234567](sms:+15551234567)\n", want: 1},
		{name: "urn", markdown: "[urn:isbn:0451450523](urn:isbn:0451450523)\n", want: 1},
		{name: "vscode", markdown: "[vscode:extension/ms-python.python](vscode:extension/ms-python.python)\n", want: 1},
		// Like markdownlint, the label is compared as written, so formatting
		// or an escape in it makes it differ from the destination, and an
		// autolink would lose them.
		{name: "bold label", markdown: "[**sms:+15551234567**](sms:+15551234567)\n"},
		{name: "emphasis label", markdown: "[*https://example.com*](https://example.com)\n"},
		{name: "underscore strong label", markdown: "[__https://example.com__](https://example.com)\n"},
		{name: "code span label", markdown: "[`https://example.com`](https://example.com)\n"},
		{name: "escaped label", markdown: "[sms\\:+15551234567](sms:+15551234567)\n"},
		{name: "same escape in label and destination", markdown: "[https://example.com/a\\_b](https://example.com/a\\_b)\n", want: 1},
		{name: "destination in angle brackets", markdown: "[https://example.com](<https://example.com>)\n", want: 1},
		{name: "protocol-relative is not an autolink", markdown: "[//cdn.example.com/a.js](//cdn.example.com/a.js)\n"},
		{name: "relative path", markdown: "[docs/a.md](docs/a.md)\n"},
		{name: "space in the destination", markdown: "[https://example.com/a b](<https://example.com/a b>)\n"},
		{name: "image", markdown: "![https://example.com/a.png](https://example.com/a.png)\n"},
		{name: "title", markdown: "[https://example.com](https://example.com \"Title\")\n"},
		{
			name:     "autolinks not allowed",
			markdown: "[https://example.com](https://example.com)\n",
			options:  map[string]any{"url_inline": false, "autolink": false},
		},
		// Two deliberate differences from markdownlint, which asks whether a
		// browser could parse the destination as a URL. A one-letter scheme
		// is a Windows drive and cannot start an autolink, and http:// with
		// no host is a valid autolink.
		{name: "one-letter scheme", markdown: "[c:x](c:x)\n"},
		{name: "scheme without a host", markdown: "[http://](http://)\n", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.options
			if options == nil {
				options = urlInline
			}
			diags := testHelperWithOptions(t, NewLinkImageStyleRule(), tt.markdown, options)
			assert.Len(t, diags, tt.want, "diagnostics: %+v", diags)
		})
	}
}

// TestReferenceLinkImagesRule_Undefined reports full and collapsed references
// whose label has no definition. The parser leaves such references as text.
func TestReferenceLinkImagesRule_Undefined(t *testing.T) {
	rule := NewReferenceLinkImagesRule()
	type want struct {
		line, column int
		message      string
	}
	tests := []struct {
		name     string
		markdown string
		options  map[string]any
		want     []want
	}{
		{name: "full reference", markdown: "Text [missing][nope] end.\n",
			want: []want{{1, 6, "Reference link uses undefined label [nope]"}}},
		{name: "collapsed reference", markdown: "[gone][]\n",
			want: []want{{1, 1, "Reference link uses undefined label [gone]"}}},
		{name: "image reference", markdown: "![img][nada]\n",
			want: []want{{1, 1, "Reference image uses undefined label [nada]"}}},
		{name: "reference in link text", markdown: "[see [missing][nope]](https://example.com)\n",
			want: []want{{1, 6, "Reference link uses undefined label [nope]"}}},
		{name: "reference in heading", markdown: "## Heading [h][hh]\n",
			want: []want{{1, 12, "Reference link uses undefined label [hh]"}}},
		{name: "defined reference", markdown: "Text [defined][ok] here.\n\n[ok]: https://example.com\n"},
		{name: "code span is not a reference", markdown: "`[missing][nope]`\n"},
		{name: "escaped brackets", markdown: "\\[missing\\]\\[nope\\]\n"},
		{name: "shortcut ignored by default", markdown: "Use [nope] here.\n"},
		{name: "shortcut checked when enabled", markdown: "Use [nope] here.\n",
			options: map[string]any{"shortcut_syntax": true},
			want:    []want{{1, 5, "Reference link uses undefined label [nope]"}}},
		{name: "ignored label", markdown: "- [x] done\n", options: map[string]any{"shortcut_syntax": true}},
		{name: "definition inside blockquote", markdown: "> [docs]: https://example.com\n\nSee [docs][] and [Docs][docs].\n"},
		{name: "unicode case folding", markdown: "[Straße][]\n\n[STRASSE]: https://example.com\n"},
		{name: "multi-line label", markdown: "See [multi\nline][]\n\n[multi line]: https://example.com\n"},
		{name: "reference inside strikethrough", markdown: "Old ~~[text][gone]~~ note.\n",
			want: []want{{1, 7, "Reference link uses undefined label [gone]"}}},
		{name: "brackets inside an autolink", markdown: "<https://example.com/[a][b]>\n"},
		{name: "multi-line code span keeps its padding", markdown: "[a ` x\ny `][]\n\n[a `x y`]: /url\n",
			want: []want{{1, 1, "Reference link uses undefined label [a ` x y `]"}}},
		{name: "hard break backslash stays in the label", markdown: "[a\\\nb][]\n\n[a b]: /url\n",
			want: []want{{1, 1, "Reference link uses undefined label [a\\ b]"}}},
		{name: "quoted label continues without the prefix", markdown: "> See [a\n> b][] here.\n",
			want: []want{{1, 7, "Reference link uses undefined label [a b]"}}},
		{name: "label of 400 wide characters", markdown: "See [text][" + strings.Repeat("\u4e2d", 400) + "] here.\n",
			want: []want{{1, 5, "Reference link uses undefined label [" + strings.Repeat("\u4e2d", 400) + "]"}}},
		{name: "label longer than 999 characters", markdown: "See [text][" + strings.Repeat("a", 1000) + "] here.\n"},
		{name: "escaped bracket in a code span of a full label", markdown: "See [text][`a\\]b`] here.\n",
			want: []want{{1, 5, "Reference link uses undefined label [`a\\]b`]"}}},
		{name: "escaped bracket in a code span of a collapsed label", markdown: "See [`a\\]b`][] here.\n",
			want: []want{{1, 5, "Reference link uses undefined label [`a\\]b`]"}}},
		{name: "defined label with an escaped bracket in a code span", markdown: "See [text][`a\\]b`] here.\n\n[`a\\]b`]: /url\n"},
		{name: "code span in a collapsed label", markdown: "See [a `code`][] here.\n",
			want: []want{{1, 5, "Reference link uses undefined label [a `code`]"}}},
		{name: "strikethrough in a full label", markdown: "See [text][~~missing~~] here.\n\n[missing]: /exists\n",
			want: []want{{1, 5, "Reference link uses undefined label [~~missing~~]"}}},
		{name: "brackets inside a bare URL", markdown: "See https://example.com/[a][b] now.\n"},
		{name: "shortcut after a failed inline link", markdown: "See [t](bad dest now.\n",
			options: map[string]any{"shortcut_syntax": true},
			want:    []want{{1, 5, "Reference link uses undefined label [t]"}}},
		{name: "failed inline link is not a reference by default", markdown: "See [t](bad dest now.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags []lint.Diagnostic
			if tt.options != nil {
				diags = testHelperWithOptions(t, rule, tt.markdown, tt.options)
			} else {
				diags = testHelper(t, rule, tt.markdown)
			}
			if len(diags) != len(tt.want) {
				t.Fatalf("got %d diagnostics, want %d: %+v", len(diags), len(tt.want), diags)
			}
			for idx, expected := range tt.want {
				got := diags[idx]
				if got.StartLine != expected.line || got.StartColumn != expected.column || got.Message != expected.message {
					t.Errorf("diagnostic %d = %d:%d %q, want %d:%d %q", idx,
						got.StartLine, got.StartColumn, got.Message, expected.line, expected.column, expected.message)
				}
			}
		})
	}
}

// TestLinkImageRefDefsRule_LabelPerLink attributes each reference to its own
// label, even with several references on one line, and folds case like the parser.
func TestLinkImageRefDefsRule_LabelPerLink(t *testing.T) {
	rule := NewLinkImageRefDefsRule()
	for _, markdown := range []string{
		"See [one][alpha] and [two][beta].\n\n[alpha]: https://a.example\n[beta]: https://b.example\n",
		"[Straße][]\n\n[STRASSE]: https://example.com\n",
		"See [alpha][] and [beta].\n\n[alpha]: https://a.example\n[beta]: https://b.example\n",
		"Undefined [text][used] still uses nothing, but [ref][kept] uses kept.\n\n[kept]: https://example.com\n",
		"> See [multi\n> line][] here.\n\n[multi line]: https://example.com\n",
		"> [a\n> b][]\n\n[a b]: https://example.com\n",
		"Use [`code` label][] here.\n\n[`code` label]: https://example.com\n",
		"> See [*docs\n> page*][] now.\n\n[*docs page*]: https://example.com\n",
		"> See [*docs\n> page*] now.\n\n[*docs page*]: https://example.com\n",
		"Use [a\\]b][] here.\n\n[a\\]b]: https://example.com\n",
	} {
		if diags := testHelper(t, rule, markdown); len(diags) != 0 {
			t.Errorf("%q: got %d diagnostics, want 0: %+v", markdown, len(diags), diags)
		}
	}
	diags := testHelper(t, rule, "See [one][alpha].\n\n[alpha]: https://a.example\n[beta]: https://b.example\n")
	if len(diags) != 1 || diags[0].Message != "Unused reference definition [beta]" {
		t.Errorf("got %+v, want one unused [beta]", diags)
	}
	// Labels are read from the source across lines, so code span padding and a
	// hard break backslash keep the definition without them unused.
	for _, markdown := range []string{
		"[a ` x\ny `][]\n\n[a `x y`]: /url\n",
		"[a\\\nb][]\n\n[a b]: /url\n",
	} {
		if got := testHelper(t, rule, markdown); len(got) != 1 {
			t.Errorf("%q: got %+v, want one unused definition", markdown, got)
		}
	}
	// The length limit on labels counts characters, not bytes.
	wide := strings.Repeat("\u4e2d", 400)
	if got := testHelper(t, rule, "Text.\n\n["+wide+"]: /url\n"); len(got) != 1 {
		t.Errorf("unused definition with a wide label: got %+v, want one", got)
	}
	if got := testHelper(t, rule, "See [text]["+wide+"].\n\n["+wide+"]: /url\n"); len(got) != 0 {
		t.Errorf("used definition with a wide label: got %+v, want none", got)
	}
	// The strikethrough markers are part of the label, so the definition of
	// a label without them is not used.
	diags = testHelper(t, rule, "See [text][~~missing~~] here.\n\n[missing]: /exists\n")
	if len(diags) != 1 || diags[0].Message != "Unused reference definition [missing]" {
		t.Errorf("got %+v, want one unused [missing]", diags)
	}
}

// TestLinkImageRefDefsRule_ParsedDefinitions finds definitions where the
// parser does (inside blockquotes and list items, spread over several lines,
// never inside a paragraph) and fixes unused or duplicate ones by deleting
// their lines when that cannot change the document's structure.
func TestLinkImageRefDefsRule_ParsedDefinitions(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []string
		fixed    string
	}{
		{
			name:     "blockquote definitions",
			markdown: "Use [quoted][].\n\n> [quoted]: https://a.example\n> [spare]: https://b.example\n",
			want:     []string{"Unused reference definition [spare]"},
			fixed:    "Use [quoted][].\n\n> [quoted]: https://a.example\n",
		},
		{
			name:     "list item definition is reported without a fix",
			markdown: "- [item]: https://a.example\n",
			want:     []string{"Unused reference definition [item]"},
			fixed:    "- [item]: https://a.example\n",
		},
		{
			name:     "multi-line definition in use",
			markdown: "Use [multi][].\n\n[multi]:\n  https://a.example\n  \"Title\"\n",
		},
		{
			name:     "unused multi-line definition",
			markdown: "Text.\n\n[multi]:\n  https://a.example\n  \"Title\"\n",
			want:     []string{"Unused reference definition [multi]"},
			fixed:    "Text.\n\n",
		},
		{
			name:     "paragraph line is not a definition",
			markdown: "Some text\n[notadef]: https://a.example\n",
		},
		{
			name:     "unused and duplicate definitions",
			markdown: "Use [used][].\n\n[used]: https://a.example\n[unused]: https://b.example\n[used]: https://c.example\n",
			want:     []string{"Unused reference definition [unused]", "Duplicate reference definition [used]"},
			fixed:    "Use [used][].\n\n[used]: https://a.example\n",
		},
		{
			name:     "definition after front matter",
			markdown: "---\ntitle: x\n---\n\n[spare]: https://a.example\n",
			want:     []string{"Unused reference definition [spare]"},
			fixed:    "---\ntitle: x\n---\n\n",
		},
		{
			name:     "definitions after a list are removed",
			markdown: "- a\n\n[one]: https://a.example\n[two]: https://b.example\n",
			want:     []string{"Unused reference definition [one]", "Unused reference definition [two]"},
			fixed:    "- a\n\n",
		},
		{
			name:     "definition followed by another definition is removed",
			markdown: "[a]: https://a.example\n[b]: https://b.example\n\nUse [b].\n",
			want:     []string{"Unused reference definition [a]"},
			fixed:    "[b]: https://b.example\n\nUse [b].\n",
		},
		{
			name:     "definition between two lists keeps its line",
			markdown: "- a\n\n[unused]: https://example.com\n- b\n",
			want:     []string{"Unused reference definition [unused]"},
			fixed:    "- a\n\n[unused]: https://example.com\n- b\n",
		},
		{
			name:     "definition before an indented definition after a list keeps its line",
			markdown: "- a\n\n[one]: https://a.example\n  [two]: https://b.example\n",
			want:     []string{"Unused reference definition [one]", "Unused reference definition [two]"},
			fixed:    "- a\n\n[one]: https://a.example\n",
		},
		{
			name:     "definition between indented code blocks keeps its line",
			markdown: "    code1\n\n[unused]: https://example.com\n\n    code2\n",
			want:     []string{"Unused reference definition [unused]"},
			fixed:    "    code1\n\n[unused]: https://example.com\n\n    code2\n",
		},
		{
			name:     "definition followed by paragraph text keeps its line",
			markdown: "Text.\n\n[unused]: https://example.com\n    indented\n",
			want:     []string{"Unused reference definition [unused]"},
			fixed:    "Text.\n\n[unused]: https://example.com\n    indented\n",
		},
		{
			name:     "quoted definition followed by a lazy line keeps its line",
			markdown: "> [unused]: https://example.com\nlazy\n",
			want:     []string{"Unused reference definition [unused]"},
			fixed:    "> [unused]: https://example.com\nlazy\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, NewLinkImageRefDefsRule(), tt.markdown)
			require.Len(t, diags, len(tt.want), "diagnostics: %+v", diags)
			var edits []fix.TextEdit
			for idx, message := range tt.want {
				assert.Equal(t, message, diags[idx].Message)
				edits = append(edits, diags[idx].FixEdits...)
			}
			if len(tt.want) == 0 {
				return
			}
			prepared, err := fix.PrepareEdits(edits, len(tt.markdown))
			require.NoError(t, err)
			assert.Equal(t, tt.fixed, string(fix.ApplyEdits([]byte(tt.markdown), prepared)))
		})
	}
}

// TestReferenceRulesIgnoreUnresolvedText checks that text the parser did not
// turn into a link is not treated as a link by style rules.
func TestReferenceRulesIgnoreUnresolvedText(t *testing.T) {
	markdown := "[missing][nope] and [here][]\n"
	if diags := testHelperWithOptions(t, NewLinkImageStyleRule(), markdown, map[string]any{
		"full": false, "collapsed": false,
	}); len(diags) != 0 {
		t.Errorf("MD054: got %d diagnostics, want 0: %+v", len(diags), diags)
	}
	if diags := testHelper(t, NewDescriptiveLinkTextRule(), markdown); len(diags) != 0 {
		t.Errorf("MD059: got %d diagnostics, want 0: %+v", len(diags), diags)
	}
}

// TestLinkFragmentsRule_GitHubSlugs checks fragments against the anchors GitHub
// generates (github-slugger) and accepts percent-encoded fragments.
func TestLinkFragmentsRule_GitHubSlugs(t *testing.T) {
	rule := NewLinkFragmentsRule()
	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{"spaced hyphen keeps every hyphen", "## A - B\n\n[x](#a---b)\n", 0},
		{"collapsed hyphens are not the github slug", "## A - B\n\n[x](#a-b)\n", 1},
		{"code span with leading hyphens", "## `--flag` option\n\n[x](#--flag-option)\n", 0},
		{"combining marks are kept", "## हिन्दी\n\n[x](#हिन्दी)\n", 0},
		{"percent-encoded fragment", "## Café Menu\n\n[x](#caf%C3%A9-menu)\n", 0},
		{"unencoded fragment", "## Café Menu\n\n[x](#café-menu)\n", 0},
		{"suffix collision keeps counting", "# Foo\n\n# Foo\n\n# Foo 1\n\n[x](#foo-1-1)\n", 0},
		{"missing duplicate suffix", "# Foo\n\n# Foo\n\n[x](#foo-2)\n", 1},
		{"trailing emoji leaves a hyphen", "## Hello World 🎉\n\n[x](#hello-world-)\n", 0},
		{"capital I with dot lowercases to i and a dot", "# \u0130\n\n[x](#i%CC%87)\n", 0},
		{"capital I with dot is not plain i", "# \u0130\n\n[x](#i)\n", 1},
		{"final sigma", "# \u039f\u03a3\n\n[x](#\u03bf\u03c2)\n", 0},
		{"non-final sigma form at the end of a word", "# \u039f\u03a3\n\n[x](#\u03bf\u03c3)\n", 1},
		{"spaces before a closing sequence are not heading text", "## Foo Bar  ##\n\n[x](#foo-bar)\n", 0},
		{"a tab before a closing sequence is not heading text", "## Foo Bar\t##\n\n[x](#foo-bar)\n", 0},
		{"closed heading in a blockquote", "> # Foo Bar   #\n\n[x](#foo-bar)\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diags := testHelper(t, rule, tt.markdown); len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d: %+v", len(diags), tt.want, diags)
			}
		})
	}

	diags := testHelperWithOptions(t, rule, "## Café Menu\n\n[x](#CAF%C3%A9-Menu)\n", map[string]any{"ignore_case": true})
	if len(diags) != 0 {
		t.Errorf("ignore_case with encoded fragment: got %d diagnostics, want 0", len(diags))
	}
}

// TestLinkFragmentsRule_CustomAnchorsAndTop checks MD051 against
// markdownlint 0.41.1: "{#name}" in a heading's text adds the anchor name
// when the name is lowercase words joined by single hyphens or underscores,
// and "#top" matches only in lowercase unless case is ignored.
func TestLinkFragmentsRule_CustomAnchorsAndTop(t *testing.T) {
	markdown := "# Title\n\n" +
		"## Custom {#custom}\n\n" + // 3
		"## Upper {#Upper}\n\n" + // 5
		"## Two {#one} {#two}\n\n" + // 7
		"## Doubled {#a--b}\n\n" + // 9
		"Setext {#setext-anchor}\n-----------------------\n\n" + // 11
		"## Code `{#in-code}`\n\n" + // 14
		"## Under {#under_score}\n\n" + // 16
		"[a](#custom) [b](#custom-custom) [c](#Upper) [d](#upper) [e](#one) [f](#two)\n" + // 18
		"[g](#a--b) [h](#setext-anchor) [i](#in-code) [j](#under_score) [k](#TOP)\n" + // 19
		"[l](#top) [m](#Top) [n](#L12) [o](#Custom)\n" // 20

	type position struct{ line, column int }
	positions := func(options map[string]any) []position {
		diags := applyRuleWithOptions(t, NewLinkFragmentsRule(), markdown, options)
		got := make([]position, 0, len(diags))
		for _, diag := range diags {
			got = append(got, position{diag.StartLine, diag.StartColumn})
		}
		return got
	}

	assert.Equal(t, []position{{18, 34}, {18, 46}, {19, 1}, {19, 64}, {20, 11}, {20, 31}}, positions(nil))
	assert.Equal(t, []position{{18, 34}, {18, 46}, {19, 1}}, positions(map[string]any{"ignore_case": true}))
}

func TestDescriptiveLinkTextRule(t *testing.T) {
	rule := NewDescriptiveLinkTextRule()

	tests := []struct {
		name     string
		markdown string
		want     int
	}{
		{
			name:     "descriptive text",
			markdown: `[Visit the documentation](url)`,
			want:     0,
		},
		{
			name:     "click here",
			markdown: `[click here](url)`,
			want:     1,
		},
		{
			name:     "here",
			markdown: `[here](url)`,
			want:     1,
		},
		{
			// markdownlint's default prohibited_texts are "click here", "here", "link", and "more".
			name:     "read more",
			markdown: `[Read More](url)`,
			want:     0,
		},
		{
			name:     "case insensitive",
			markdown: `[Click Here](url)`,
			want:     1,
		},
		{
			name:     "link text",
			markdown: `[link](url)`,
			want:     1,
		},
		{
			name:     "images not checked",
			markdown: `![click here](image.png)`,
			want:     0, // images have alt text, different concern
		},
		{
			name:     "multiple generic links",
			markdown: `[here](url1) and [click](url2) and [more](url3)`,
			want:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, rule, tt.markdown)
			if len(diags) != tt.want {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.want)
				for _, d := range diags {
					t.Logf("  - %s", d.Message)
				}
			}
		})
	}
}

func TestDescriptiveLinkTextRule_CustomPatterns(t *testing.T) {
	rule := NewDescriptiveLinkTextRule()

	markdown := `[info](url) and [details](url)`

	// Default patterns - should not flag "info" or "details"
	diags := testHelper(t, rule, markdown)
	if len(diags) != 0 {
		t.Errorf("default patterns: got %d diagnostics, want 0", len(diags))
	}

	// Custom patterns including "info" and "details"
	diags = testHelperWithOptions(t, rule, markdown, map[string]interface{}{
		"patterns": []interface{}{"info", "details"},
	})
	if len(diags) != 2 {
		t.Errorf("custom patterns: got %d diagnostics, want 2", len(diags))
	}
}

// TestDescriptiveLinkTextRule_MarkdownlintDefaults checks the markdownlint
// defaults: punctuation is ignored when comparing link text, and the
// markdownlint option name prohibited_texts is accepted.
func TestDescriptiveLinkTextRule_MarkdownlintDefaults(t *testing.T) {
	rule := NewDescriptiveLinkTextRule()
	if diags := testHelper(t, rule, "[Click here!](url)\n"); len(diags) != 1 {
		t.Errorf("punctuation: got %d diagnostics, want 1", len(diags))
	}
	diags := testHelperWithOptions(t, rule, "[info](url) and [here](url)\n", map[string]interface{}{
		"prohibited_texts": []interface{}{"info"},
	})
	if len(diags) != 1 || diags[0].Message != "Link text 'info' is not descriptive" {
		t.Errorf("prohibited_texts: got %+v, want one diagnostic for 'info'", diags)
	}
}

// TestDescriptiveLinkTextRule_MarkdownlintPositions checks MD059 against
// markdownlint 0.41.1 on the same document: the link text is compared as
// written, a link is skipped only when its text holds code or HTML directly,
// autolinks are never checked, and a finding points at the first character
// of the link text, one column after the bracket.
func TestDescriptiveLinkTextRule_MarkdownlintPositions(t *testing.T) {
	markdown := "# Title\n\n" +
		"[*`here`*](x)\n\n" + // 3: code inside emphasis is not direct
		"[`here`](x)\n\n" + // 5: code directly in the text
		"[**here**](x)\n\n" + // 7
		"[here](x)\n\n" + // 9
		"![here](y) and [more](z)\n\n" + // 11
		"[here <b>x</b>](x)\n\n" + // 13: HTML directly in the text
		"[![here](a.png)](x)\n\n" + // 15: the image's source is part of the text
		"[h&#101;re](x)\n\n" + // 17
		"> [here](x)\n\n" + // 19
		"[click\nhere](x)\n\n" + // 21: text over two lines
		"[here][ref] and [here][] and [here]\n\n" + // 24
		"[~~here~~](x)\n\n" + // 26
		"| a | b |\n| - | - |\n| [link](x) | y |\n\n" + // 30
		"Some <https://example.com> text.\n\n" + // 32
		"[here\\!](x) and [\\here](x) and [&nbsp;here](x)\n\n" + // 34
		"[ref]: /u\n[here]: /v\n"

	type position struct{ line, column int }
	positions := func(options map[string]any) []position {
		diags := applyRuleWithOptions(t, NewDescriptiveLinkTextRule(), markdown, options)
		got := make([]position, 0, len(diags))
		for _, diag := range diags {
			got = append(got, position{diag.StartLine, diag.StartColumn})
		}
		return got
	}

	assert.Equal(t, []position{
		{3, 2}, {7, 2}, {9, 2}, {11, 17}, {19, 4}, {21, 2}, {24, 2}, {24, 18}, {24, 31},
		{26, 2}, {30, 4}, {34, 2}, {34, 18},
	}, positions(nil))
	assert.Equal(t, []position{
		{3, 2}, {7, 2}, {9, 2}, {19, 4}, {24, 2}, {24, 18}, {24, 31}, {26, 2}, {34, 2}, {34, 18},
	}, positions(map[string]any{"prohibited_texts": []any{"https example com", "here"}}))
}

// TestDescriptiveLinkTextRule_StrikethroughAndMath checks MD059 against
// markdownlint 0.41.1, which parses no strikethrough and parses math. Code
// or HTML in a strikethrough is directly in the link text for it, so the
// link is skipped, and code or HTML in math is math, so it is not.
func TestDescriptiveLinkTextRule_StrikethroughAndMath(t *testing.T) {
	markdown := "# Title\n\n" +
		"[~~`here`~~](x)\n\n" + // 3: the code is direct
		"[~~*`here`*~~](x)\n\n" + // 5: emphasis holds the code
		"[~here~](x)\n\n" + // 7
		"[~~here <b>x</b>~~](x)\n\n" + // 9
		"[*~~`here`~~*](x)\n\n" + // 11: emphasis holds the code
		"[~~`a`~~ here](x)\n\n" + // 13
		"[$<here>$](x)\n\n" + // 15: the tag is math
		"[$`here`$](x)\n\n" + // 17: the code is math
		"[here $<b>$](x)\n" // 19

	diags := applyRuleWithOptions(t, NewDescriptiveLinkTextRule(), markdown, nil)
	type position struct{ line, column int }
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{5, 2}, {7, 2}, {11, 2}, {15, 2}, {17, 2}}, got)
}

// TestReferenceLinkImagesRule_Footnotes checks MD052 against markdownlint
// 0.41.1, which reads GFM footnotes: "[^label]" calls a footnote when a line
// starts with "[^label]:", in any container and before or after the call,
// so it is not an undefined reference. goldmark reads no footnotes, and
// takes such a line for a paragraph unless it is also a link reference
// definition.
func TestReferenceLinkImagesRule_Footnotes(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name     string
		input    string
		want     []position // With shortcut_syntax.
		wantFull []position // Without it.
	}{
		{name: "definition before the call", input: "# T\n\n[^first]: Defined before.\n\nUse [^first] here.\n"},
		{name: "definition after the call", input: "# T\n\nUse [^first] here.\n\n[^first]: Defined after.\n"},
		{name: "labels in another case", input: "# T\n\nUse [^Second].\n\n[^second]: Two words.\n"},
		{name: "definition in a list item", input: "# T\n\nUse [^third].\n\n- item\n\n  [^third]: In a list.\n"},
		{name: "definition in a blockquote", input: "# T\n\nUse [^q].\n\n> [^q]: In a quote.\n"},
		{name: "definition that interrupts a paragraph", input: "# T\n\nUse [^n].\nSome text\n[^n]: Interrupts the paragraph.\n"},
		{name: "escaped bracket in the label", input: "# T\n\nUse [^e\\]x].\n\n[^e\\]x]: Escaped bracket.\n"},
		{name: "indented definition", input: "# T\n\nUse [^i].\n\n   [^i]: Indented three.\n"},
		{
			name:  "undefined footnotes",
			input: "# T\n\nMore [^missing] and [^] and [^ x] text.\n",
			want:  []position{{3, 6}, {3, 21}, {3, 29}},
		},
		{
			name:  "label with a space is no footnote",
			input: "# T\n\nUse [^a b].\n\n[^a b]: Not a footnote.\n",
			want:  []position{{3, 5}, {5, 1}},
		},
		{name: "definition in code", input: "# T\n\nUse [^c].\n\n```\n[^c]: in code\n```\n", want: []position{{3, 5}}},
		{name: "definition in HTML", input: "# T\n\nUse [^h].\n\n<div>\n[^h]: in html\n</div>\n", want: []position{{3, 5}}},
		{
			// The second bracket pair calls the footnote, so the first is
			// a shortcut reference of its own.
			name:  "footnote call after link text",
			input: "# T\n\nUse [text][^first].\n\n[^first]: Note text.\n",
			want:  []position{{3, 5}},
		},
		// A definition starts after the prefixes of the containers its line
		// continues and at most three columns of space, as a container does.
		{
			name:  "paragraph line indented four columns",
			input: "# T\n\nUse [^n].\n\ntext\n    [^n]: note text\n",
			want:  []position{{3, 5}, {6, 5}},
		},
		{name: "definition in indented code", input: "# T\n\nUse [^n].\n\n    [^n]: indented code\n", want: []position{{3, 5}}},
		{name: "definition in math", input: "# T\n\nUse [^n].\n\n$$\n[^n]: in math\n$$\n", want: []position{{3, 5}}},
		{
			name:  "definition in indented code in a list item",
			input: "# T\n\nUse [^n].\n\n- item\n\n      [^n]: indented in item\n",
			want:  []position{{3, 5}},
		},
		{name: "definition that interrupts a list item's paragraph", input: "# T\n\nUse [^n].\n\n- item\n   [^n]: in item\n"},
		{name: "definition on a lazy line", input: "# T\n\nUse [^n].\n\n> quote\n[^n]: lazy\n"},
		{name: "definition in a quoted list item", input: "# T\n\nUse [^n].\n\n> - a\n>     [^n]: x\n"},
		{
			name:  "quoted list item line indented four columns",
			input: "# T\n\nUse [^n].\n\n> - a\n>       [^n]: x\n",
			want:  []position{{3, 5}, {6, 9}},
		},
		{name: "tab past a list item's indentation", input: "# T\n\nUse [^n].\n\n- a\n\n\t[^n]: tab\n"},
		// A footnote definition is no link reference definition, so a
		// label that names both calls the footnote.
		{
			name:  "full reference to a footnote that is also a link definition",
			input: "# T\n\nUse [text][^n].\n\n[^n]: /url\n",
			want:  []position{{3, 5}},
		},
		{name: "footnote call that is also a link definition", input: "# T\n\nUse [^n] here.\n\n[^n]: /url\n"},
		{
			name:  "defined link text before a footnote that is also a link definition",
			input: "# T\n\nUse [text][^n].\n\n[text]: /a\n[^n]: /url\n",
		},
		// A footnote definition is a container whose content lines are
		// indented four columns, so a definition can start four to seven
		// columns in, and code in its content holds none.
		{name: "definition in a footnote's content", input: "# T\n\nUse [^n].\n\n[^o]:\n    [^n]: inner\n"},
		{name: "definition after text in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: text\n    [^n]: inner\n"},
		{name: "definition after a blank line in a footnote", input: "# T\n\nUse [^n].\n\n[^o]:\n\n    [^n]: inner\n"},
		{name: "definition two footnotes deep", input: "# T\n\nUse [^n].\n\n[^o]:\n    [^p]:\n        [^n]: inner\n"},
		{name: "seven columns into a footnote", input: "# T\n\nUse [^n].\n\n[^o]:\n       [^n]: seven\n"},
		{name: "lazy line in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\nlazy\n    [^n]: inner\n"},
		{name: "footnote in a list item", input: "# T\n\nUse [^n].\n\n- [^o]:\n      [^n]: inner\n"},
		{
			name:  "indented code in a footnote",
			input: "# T\n\nUse [^n].\n\n[^o]:\n\n        [^n]: code\n",
			want:  []position{{3, 5}},
		},
		{
			name:  "fenced code in a footnote",
			input: "# T\n\nUse [^n].\n\n[^o]: note\n    ```\n    [^n]: code\n    ```\n",
			want:  []position{{3, 5}},
		},
		{
			// A known gap, on the side of reporting nothing: markdownlint
			// reports 3:5, as the definition line is code in the footnote's
			// content, but goldmark reads the fence line as the destination
			// of "[^o]:" and "[^n]: code" as a link reference definition, so
			// the reference resolves.
			name:  "fenced code after an empty footnote line",
			input: "# T\n\nUse [^n].\n\n[^o]:\n    ```\n    [^n]: code\n    ```\n",
		},
		{
			name:  "line after a closed footnote",
			input: "# T\n\nUse [^n].\n\n[^o]: a\n\ntext\n    [^n]: not\n",
			want:  []position{{3, 5}, {8, 5}},
		},
		{
			// markdownlint reads math, which holds no references.
			name:     "references in math",
			input:    "# T\n\nInline $[a][b]$ and $[^n]$ math.\n\n$$\n[c][d] and [e]\n$$\n\n[f][g] outside.\n",
			want:     []position{{9, 1}},
			wantFull: []position{{9, 1}},
		},
	}

	positions := func(input string, options map[string]any) []position {
		diags := applyRuleWithOptions(t, NewReferenceLinkImagesRule(), input, options)
		got := make([]position, 0, len(diags))
		for _, diag := range diags {
			got = append(got, position{diag.StartLine, diag.StartColumn})
		}
		return got
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = []position{}
			}
			wantFull := tt.wantFull
			if wantFull == nil {
				wantFull = []position{}
			}
			assert.Equal(t, want, positions(tt.input, map[string]any{"shortcut_syntax": true}), "with shortcut_syntax")
			assert.Equal(t, wantFull, positions(tt.input, nil), "without shortcut_syntax")
		})
	}
}

func TestRuleMetadata(t *testing.T) {
	tests := []struct {
		rule         lint.Rule
		expectedID   string
		expectedName string
	}{
		{NewLinkFragmentsRule(), "MD051", "link-fragments"},
		{NewReferenceLinkImagesRule(), "MD052", "reference-links-images"},
		{NewLinkImageRefDefsRule(), "MD053", "link-image-reference-definitions"},
		{NewLinkImageStyleRule(), "MD054", "link-image-style"},
		{NewDescriptiveLinkTextRule(), "MD059", "descriptive-link-text"},
	}

	for _, tt := range tests {
		t.Run(tt.expectedID, func(t *testing.T) {
			if tt.rule.ID() != tt.expectedID {
				t.Errorf("ID() = %q, want %q", tt.rule.ID(), tt.expectedID)
			}
			if tt.rule.Name() != tt.expectedName {
				t.Errorf("Name() = %q, want %q", tt.rule.Name(), tt.expectedName)
			}
		})
	}
}

// TestReferenceRulesDefaultEnabled checks that the link and reference rules
// run by default, as in markdownlint. MD054 allows every style by default,
// so it reports nothing until an option disallows one.
func TestReferenceRulesDefaultEnabled(t *testing.T) {
	for _, rule := range []lint.Rule{
		NewLinkFragmentsRule(),
		NewReferenceLinkImagesRule(),
		NewLinkImageRefDefsRule(),
		NewLinkImageStyleRule(),
		NewDescriptiveLinkTextRule(),
	} {
		if !rule.DefaultEnabled() {
			t.Errorf("%s.DefaultEnabled() = false, want true", rule.ID())
		}
	}
}

// TestLinkImageRefDefsRule_ByteOrderMark checks that a byte order mark before
// definitions on the first lines changes neither MD053's diagnostics nor its
// fixes: the mark is not part of the first definition's column.
func TestLinkImageRefDefsRule_ByteOrderMark(t *testing.T) {
	for _, input := range []string{
		"[a]: https://a.example\n[b]: https://b.example\n",
		"> [a]:\n>   https://a.example\n> [b]: https://b.example\n\nText\n",
	} {
		t.Run(input, func(t *testing.T) {
			plain := applyRuleWithOptions(t, NewLinkImageRefDefsRule(), input, nil)
			require.Len(t, plain, 2)
			marked := applyRuleWithOptions(t, NewLinkImageRefDefsRule(), mdast.ByteOrderMark+input, nil)
			require.Equal(t, diagnosticLines(plain), diagnosticLines(marked))
			require.Equal(t, mdast.ByteOrderMark+applyAllFixEdits(t, input, plain),
				applyAllFixEdits(t, mdast.ByteOrderMark+input, marked))
		})
	}
}

// TestReferenceLabelLengthLimit checks the CommonMark limit of 999
// characters inside a label's brackets, counted in characters, for full and
// collapsed references and for definitions.
func TestReferenceLabelLengthLimit(t *testing.T) {
	for _, char := range []string{"a", "\u4e2d"} {
		for _, tt := range []struct {
			length int
			label  bool
		}{{998, true}, {999, true}, {1000, false}} {
			label := strings.Repeat(char, tt.length)
			name := fmt.Sprintf("%d of %q", tt.length, char)
			want := 0
			if tt.label {
				want = 1
			}
			t.Run(name, func(t *testing.T) {
				full := testHelper(t, NewReferenceLinkImagesRule(), "See [text]["+label+"] here.\n")
				assert.Len(t, full, want, "full reference")
				collapsed := testHelper(t, NewReferenceLinkImagesRule(), "See ["+label+"][] here.\n")
				assert.Len(t, collapsed, want, "collapsed reference")
				definition := testHelper(t, NewLinkImageRefDefsRule(), "Text.\n\n["+label+"]: /url\n")
				assert.Len(t, definition, want, "unused definition")
			})
		}
	}
}

// TestLinkImageRefDefsRule_AfterOverlongLabel checks what follows a
// definition whose label is over the limit, at 1,001 characters. CommonMark
// leaves its paragraph as text, so a definition later in that paragraph is
// text too. A blockquote or list item that follows starts a new block whose
// definitions count, and so does a paragraph after a blank line. Every case
// matches markdownlint 0.41.1.
func TestLinkImageRefDefsRule_AfterOverlongLabel(t *testing.T) {
	long := strings.Repeat("a", 997)
	unused := func(line int) []string {
		return []string{fmt.Sprintf("%d: Unused reference definition [unused]", line)}
	}
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"blockquote after", "[" + long + "\n   b]: /long\n> [unused]: /url\n", unused(3)},
		{"list item after", "[" + long + "\n   b]: /long\n- [unused]: /url\n", unused(3)},
		{"next list item", "- [" + long + "\n     b]: /long\n- [unused]: /url\n", unused(3)},
		{"next nested list item", "- a\n  - [" + long + "\n       b]: /long\n  - [unused]: /url\n", unused(4)},
		{
			"duplicate in a blockquote after",
			"See [x].\n\n[x]: /a\n[" + long + "\n   b]: /long\n> [x]: /dup\n",
			[]string{"6: Duplicate reference definition [x]"},
		},
		{"blockquote paragraph after a blank line", "> [" + long + "\n>    b]: /long\n>\n> [unused]: /url\n", unused(4)},
		{"same paragraph", "[" + long + "\n   b]: /long\n[unused]: /url\n", []string{}},
		{"same blockquote paragraph", "> [" + long + "\n>    b]: /long\n> [unused]: /url\n", []string{}},
		{"same list item paragraph", "- [" + long + "\n     b]: /long\n  [unused]: /url\n", []string{}},
		{"duplicate in the same paragraph", "See [x].\n\n[x]: /a\n[" + long + "\n   b]: /long\n[x]: /dup\n", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := testHelper(t, NewLinkImageRefDefsRule(), tt.input)
			assert.Equal(t, tt.want, diagnosticLines(diags))
		})
	}
}

// TestLinkImageRefDefsRule_MarkerOnlyItem checks a definition after the
// first line of a list item that holds only its marker: inside the item,
// whose content starts on the next line, or after it when the item is empty.
func TestLinkImageRefDefsRule_MarkerOnlyItem(t *testing.T) {
	for _, input := range []string{
		"-\n  [unused]: /url\n",
		"1.\n   [unused]: /url\n",
		"> -\n>   [unused]: /url\n",
		"-\n[unused]: /url\n",
		"1)\n[unused]: /url\n",
	} {
		t.Run(input, func(t *testing.T) {
			diags := testHelper(t, NewLinkImageRefDefsRule(), input)
			assert.Equal(t, []string{"2: Unused reference definition [unused]"}, diagnosticLines(diags))
		})
	}
}

// TestMultiLineLabelLengthLimit checks the limit on a label that spans two
// lines, written as a code span like "` aaa\n `". The limit counts what
// markdownlint counts: every character between the brackets except line
// endings and the container prefix of the second line, so indentation after
// the prefix counts. A full or collapsed reference reports an undefined
// label only when the label fits, and text is a definition, and so can be
// unused, only when its label fits.
func TestMultiLineLabelLengthLimit(t *testing.T) {
	containers := []struct {
		name         string
		first, later string // the container prefix of the label's first and second lines
	}{
		{"top level", "", ""},
		{"blockquote", "> ", "> "},
		{"list item", "- ", "  "},
	}
	for _, container := range containers {
		for _, indent := range []string{"", " ", "   "} {
			for _, tt := range []struct {
				length int
				label  bool
			}{{998, true}, {999, true}, {1000, false}, {1001, false}} {
				// Besides the line ending and the prefix, the label holds a
				// backtick, a space, the code, the indentation, and a backtick.
				code := strings.Repeat("a", tt.length-3-len(indent))
				label := "` " + code + "\n" + container.later + indent + "`"
				want := 0
				if tt.label {
					want = 1
				}
				name := fmt.Sprintf("%s indented %d with %d characters", container.name, len(indent), tt.length)
				t.Run(name, func(t *testing.T) {
					full := testHelper(t, NewReferenceLinkImagesRule(), container.first+"See [text]["+label+"] here.\n")
					assert.Len(t, full, want, "full reference")
					collapsed := testHelper(t, NewReferenceLinkImagesRule(), container.first+"See ["+label+"][] here.\n")
					assert.Len(t, collapsed, want, "collapsed reference")
					blank := strings.TrimRight(container.later, " ")
					definition := testHelper(t, NewLinkImageRefDefsRule(),
						container.first+"Text.\n"+blank+"\n"+container.later+"["+label+"]: /url\n")
					assert.Len(t, definition, want, "unused definition")
				})
			}
		}
	}
}
