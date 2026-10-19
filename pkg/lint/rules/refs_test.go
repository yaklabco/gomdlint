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
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
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

// TestLinkFragmentsRule_HTMLAnchors checks MD051 against markdownlint
// 0.41.1, which reads the tags of an HTML block from its lines parsed again
// as Markdown, as MD033 does, so a tag on any line of the block names an
// anchor and one in a code span names none. It leaves a block that starts
// with "<!--" and ends with "-->" unparsed, as a comment. An opening tag
// names the value of its first id attribute, quoted or not, or an "a" tag
// without one the value of its first name attribute.
func TestLinkFragmentsRule_HTMLAnchors(t *testing.T) {
	markdown := "# Title\n\n" +
		"<div>\n<a name=\"later\"></a>\n</div>\n\n" + // 3
		"<div id=\"first\">\n<span ID=\"second\">text</span>\n</div>\n\n" + // 7
		"Text <a\nid=\"spread\">over two lines</a>.\n\n" + // 11
		"<div name=\"div-name\"></div>\n\n" + // 14: name counts only on an a tag
		"<div>\n`<a id=\"in-code\"></a>`\n</div>\n\n" + // 16
		"<a name=\"name-first\" id=\"id-second\"></a>\n\n" + // 20: the id wins
		"<a data-id=\"data\"></a> <a id=unquoted></a> <A NAME='upper'></A>\n\n" + // 22
		"<a id=\"\" name=\"after-empty\"></a>\n\n" + // 24: an empty id still wins
		"> <div>\n> <a id=\"quoted\"></a>\n> </div>\n\n" + // 26
		"- <div>\n  <a id=\"listed\"></a>\n  </div>\n\n" + // 30
		"<!--\n<a id=\"in-comment\"></a>\n-->\n\n" + // 34
		"[a](#later) [b](#first) [c](#second) [d](#spread) [e](#div-name)\n" + // 38
		"[f](#in-code) [g](#name-first) [h](#id-second) [i](#data)\n" + // 39
		"[j](#unquoted) [k](#upper) [l](#after-empty) [m](#quoted)\n" + // 40
		"[n](#listed) [o](#in-comment)\n\n" + // 41
		"<!--\na\n--> <a id=\"comment-end\"></a> <!-- z -->\n\n" + // 43: one comment to markdownlint
		"  <!--\na\n--> <a id=\"indented-comment\"></a> <!-- z -->\n\n" + // 47: the indentation starts its text
		"[p](#comment-end) [q](#indented-comment)\n" // 51

	type position struct{ line, column int }
	diags := applyRuleWithOptions(t, NewLinkFragmentsRule(), markdown, nil)
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{38, 51}, {39, 1}, {39, 15}, {39, 48}, {40, 28}, {41, 14}, {51, 1}}, got)
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
// markdownlint 0.41.1, which parses no strikethrough, so code or HTML in a
// strikethrough is directly in the link text for it, and the link is
// skipped. Code or HTML in inline math is code or HTML too, as on main:
// goldmark parses no math, where markdownlint reads it as math and reports
// the links on lines 15 and 17.
func TestDescriptiveLinkTextRule_StrikethroughAndMath(t *testing.T) {
	markdown := "# Title\n\n" +
		"[~~`here`~~](x)\n\n" + // 3: the code is direct
		"[~~*`here`*~~](x)\n\n" + // 5: emphasis holds the code
		"[~here~](x)\n\n" + // 7
		"[~~here <b>x</b>~~](x)\n\n" + // 9
		"[*~~`here`~~*](x)\n\n" + // 11: emphasis holds the code
		"[~~`a`~~ here](x)\n\n" + // 13
		"[$<here>$](x)\n\n" + // 15: the tag is HTML
		"[$`here`$](x)\n\n" + // 17: the code is code
		"[here $<b>$](x)\n" // 19

	diags := applyRuleWithOptions(t, NewDescriptiveLinkTextRule(), markdown, nil)
	type position struct{ line, column int }
	got := make([]position, 0, len(diags))
	for _, diag := range diags {
		got = append(got, position{diag.StartLine, diag.StartColumn})
	}
	assert.Equal(t, []position{{5, 2}, {7, 2}, {11, 2}}, got)
}

// footnoteCase is an MD052 footnote case: its input, and the positions of
// its findings with shortcut_syntax (want) and without it (wantFull).
type footnoteCase struct {
	name     string
	input    string
	want     []footnotePosition
	wantFull []footnotePosition
}

// footnotePosition is the line and column of an MD052 finding.
type footnotePosition struct{ line, column int }

// checkFootnoteCases runs MD052 on each case, with shortcut_syntax and
// without it.
func checkFootnoteCases(t *testing.T, tests []footnoteCase) {
	t.Helper()
	positions := func(input string, options map[string]any) []footnotePosition {
		diags := applyRuleWithOptions(t, NewReferenceLinkImagesRule(), input, options)
		got := make([]footnotePosition, 0, len(diags))
		for _, diag := range diags {
			got = append(got, footnotePosition{diag.StartLine, diag.StartColumn})
		}
		return got
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = []footnotePosition{}
			}
			wantFull := tt.wantFull
			if wantFull == nil {
				wantFull = []footnotePosition{}
			}
			assert.Equal(t, want, positions(tt.input, map[string]any{"shortcut_syntax": true}), "with shortcut_syntax")
			assert.Equal(t, wantFull, positions(tt.input, nil), "without shortcut_syntax")
		})
	}
}

// TestReferenceLinkImagesRule_CertainFootnotes checks MD052 on GFM
// footnotes whose definition micromark is certain to read (see
// certainFootnoteLabels), where it reports what markdownlint 0.41.1
// reports: "[^label]" calls the footnote, and in "[text][^label]" the
// second pair calls it, so the first is a shortcut reference, and the
// pair is reported with shortcut_syntax where main reports it. Two
// definitions in a row count, as micromark reads each, and so does a
// definition right after front matter, at the start of micromark's
// document, and one on a line that a carriage return alone ends.
func TestReferenceLinkImagesRule_CertainFootnotes(t *testing.T) {
	checkFootnoteCases(t, []footnoteCase{
		{name: "definition before the call", input: "# T\n\n[^first]: Defined before.\n\nUse [^first] here.\n"},
		{name: "lines that end with carriage returns alone", input: "# T\r\rUse [^n] and [x][^n].\r\r[^n]: Note text.\r"},
		{name: "mixed line endings", input: "# T\r\n\rUse [^n] and [x][^n].\n\r[^n]: Note text.\r\n"},
		{name: "definition after the call", input: "# T\n\nUse [^first] here.\n\n[^first]: Defined after.\n"},
		{name: "labels in another case", input: "# T\n\nUse [^Second].\n\n[^second]: Two words.\n"},
		{name: "definition on the first line", input: "[^n]: Note text.\n\nUse [^n] and [x][^n].\n"},
		{name: "definition after a byte order mark", input: "\ufeff[^n]: Note text.\n\nUse [^n] and [x][^n].\n"},
		{name: "definition right after front matter", input: "---\ntitle: x\n---\n[^n]: Note text.\n\nUse [^n] and [x][^n].\n"},
		{name: "definitions in a row", input: "# T\n\nUse [^a], [^b] and [x][^c].\n\n[^a]: One note.\n[^b]: Two notes.\n[^c]: Three notes.\n"},
		{name: "definition after one that opens math", input: "# T\n\nUse [^a] and [x][^b].\n\n[^a]: $$\n[^b]: Two notes.\n"},
		{name: "definition after one that opens a fence", input: "# T\n\nUse [^a] and [x][^b].\n\n[^a]: ```\n[^b]: Two notes.\n"},
		{name: "definition before math", input: "# T\n\nUse [^n] and [x][^n].\n\n[^n]: Note text.\n\n$$\nx\n$$\n"},
		{name: "definition after a list", input: "# T\n\nUse [^n] and [x][^n].\n\n- item\n\n[^n]: Note text.\n"},
		{name: "definition after a closed fence in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n    ```\n\n[^n]: y\n"},
		{name: "definition after a fenced code block", input: "# T\n\nUse [^n] and [x][^n].\n\n```\n[^n]: in code\n\n```\n\n[^n]: Note text.\n"},
		{name: "definition after a one-line comment", input: "# T\n\nUse [^n] and [x][^n].\n\n<!-- note -->\n\n[^n]: Note text.\n"},
		{name: "definition after a div", input: "# T\n\nUse [^n] and [x][^n].\n\n<div>\n<b>x</b>\n</div>\n\n[^n]: Note text.\n"},
		{name: "definition after one-line CDATA", input: "# T\n\nUse [^n] and [x][^n].\n\n<![CDATA[x]]]]>\n\n[^n]: Note text.\n"},
		{name: "undefined footnotes", input: "# T\n\nMore [^missing] and [^] and [^ x] text.\n", want: []footnotePosition{{3, 6}, {3, 21}, {3, 29}}},
		{name: "label with a space is no footnote", input: "# T\n\nUse [^a b].\n\n[^a b]: Not a footnote.\n", want: []footnotePosition{{3, 5}, {5, 1}}},
		{name: "footnote call after link text", input: "# T\n\nUse [text][^first].\n\n[^first]: Note text.\n", want: []footnotePosition{{3, 5}}},
		{name: "footnote call that is also a link definition", input: "# T\n\nUse [^n] here.\n\n[^n]: /url\n"},
		{name: "byte order mark on a later line", input: "# T\n\nUse [^n] and [x][^n].\n\n\ufeff<!--\n\n[^n]: Note text.\n-->\n"},
		{name: "definition after a one-line pre block", input: "# T\n\nUse [^n] and [x][^n].\n\n<pre>x</pre>\n\n[^n]: Note text.\n"},
	})
}

// TestReferenceLinkImagesRule_FootnotesAroundBlocks checks MD052 on GFM
// footnote definitions in or after blocks that micromark may read
// otherwise than goldmark: code, HTML, math, directives, code fences that
// hold blank lines unless both parsers read them alike, and HTML blocks of
// kinds 1 to 5 that go on past their first line. The definitions do not
// count, and MD052 reports what main reports.
func TestReferenceLinkImagesRule_FootnotesAroundBlocks(t *testing.T) {
	checkFootnoteCases(t, []footnoteCase{
		{
			name:     "definition after a comment over several lines",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<!--\n\n[^n]: in a comment\n-->\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {10, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "fence in a nested footnote",
			input:    "Use [x][^n].\n\n[^a]: one\n    [^b]: two\n    ```\n    [^n]: three four\n    ```\n",
			want:     []footnotePosition{{1, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{
			name:     "fence in a nested footnote after a blank line",
			input:    "Use [x][^n].\n\n[^a]: one\n\n    [^b]: two\n    ```\n    [^n]: three four\n",
			want:     []footnotePosition{{1, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{name: "definition in code", input: "# T\n\nUse [^c].\n\n```\n[^c]: in code\n```\n", want: []footnotePosition{{3, 5}}},
		{name: "definition in HTML", input: "# T\n\nUse [^h].\n\n<div>\n[^h]: in html\n</div>\n", want: []footnotePosition{{3, 5}}},
		{
			name:     "definition in an HTML block after a blank line",
			input:    "# T\n\nUse [^h] and [x][^h].\n\n<!--\n\n[^h]: in a comment\n-->\n",
			want:     []footnotePosition{{3, 5}, {3, 14}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition in a fence after a blank line",
			input:    "# T\n\nUse [^c] and [x][^c].\n\n```\n\n[^c]: in code\n```\n",
			want:     []footnotePosition{{3, 5}, {3, 14}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{name: "definition in indented code", input: "# T\n\nUse [^n].\n\n    [^n]: indented code\n", want: []footnotePosition{{3, 5}}},
		{name: "definition in math", input: "# T\n\nUse [^n].\n\n$$\n[^n]: in math\n$$\n", want: []footnotePosition{{3, 5}, {6, 1}}},
		{name: "definition after closed math", input: "# T\n\nUse [^n].\n\n$$\nx\n$$\n\n[^n]: y\n"},
		{
			name:     "definition after closed math with a full reference",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n$$\nx\n$$\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after math in code",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n```\n$$\n```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a quoted fence",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n> $$\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition in a directive",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n:::note\n\n[^n]: Note text.\n:::\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a closed directive",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n:::note\nx\n:::\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "indented definition after a directive",
			input:    "Use [x][^n].\n\n:::note\n[^a]: one\n:::\n    [^n]: three four\n",
			want:     []footnotePosition{{1, 5}, {4, 1}, {6, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{
			name:     "definition that ends a list item before a fence",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n- item\n[^x]: y\n  ```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {6, 1}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition that ends a list item before a comment",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n- item\n[^x]: y\n  <!--\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {6, 1}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "lazy table row before a fence",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n- | a |\n  | - |\n| b |\n  ```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {10, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after CDATA that micromark does not close",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<![CDATA[\nx]]]>\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {8, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after one-line CDATA that micromark does not close",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<![CDATA[x]]]>\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a fence that a tag line holds for micromark",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<del\t>\n```\nx\n\n```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {11, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a fence that an end tag with a space holds for goldmark",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n</ div>\n```\nx\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after an empty item that micromark refuses",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n    code\n\n-\n  ```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {10, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after an empty item after a definition",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n[ref]: /url\n-\n  ```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a fence in a script block that micromark ends early",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<script/>\n\n```\n\n</script>\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {11, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a fence right after text",
			input:    "# T\n\nUse [^n] and [x][^n].\ntext\n```\nx\n```\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition in a declaration that goldmark does not read",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<!doctype\n\n[^n]: Note text.\n>\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition in a declaration after text",
			input:    "# T\n\nUse [^n] and [x][^n].\ntext\n<!x\n\n[^n]: Note text.\n>\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition in a declaration that goldmark reads",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<!DOCTYPE\n\n[^n]: Note text.\n>\n",
			want:     []footnotePosition{{3, 5}, {3, 14}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{name: "definition in indented code in a list item", input: "# T\n\nUse [^n].\n\n- item\n\n      [^n]: indented in item\n", want: []footnotePosition{{3, 5}}},
		{name: "indented code in a footnote", input: "# T\n\nUse [^n].\n\n[^o]:\n\n        [^n]: code\n", want: []footnotePosition{{3, 5}}},
		{name: "fenced code in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: note\n    ```\n    [^n]: code\n    ```\n", want: []footnotePosition{{3, 5}}},
		{name: "fenced code after an empty footnote line", input: "# T\n\nUse [^n].\n\n[^o]:\n    ```\n    [^n]: code\n    ```\n"},
		{name: "definition in math in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n    $$\n    [^n]: x\n    $$\n", want: []footnotePosition{{3, 5}, {7, 5}}},
		{
			name:  "definition in math in an item after a heading",
			input: "# T\n\nUse [^n].\n\n## H\n3. $$\n   [^n]: y\n   $$\n\nEnd.\n",
			want:  []footnotePosition{{3, 5}, {7, 4}},
		},
		{name: "definition after indented math", input: "# T\n\nUse [^n].\n\nIntro\n  $$\n[^n]: y\n  $$\n", want: []footnotePosition{{3, 5}, {7, 1}}},
		{name: "definition after indented math that starts a block", input: "# T\n\nUse [^n].\n\n  $$\n[^n]: y\n  $$\n", want: []footnotePosition{{3, 5}, {6, 1}}},
		{name: "definition after math that ends a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n$$\nx\n$$\n    [^n]: y\n", want: []footnotePosition{{3, 5}, {9, 5}}},
		{name: "definition after an HTML block in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n\n    <div>\n    [^n]: y\n", want: []footnotePosition{{3, 5}}},
		{
			name:  "definition after a fence in a nested footnote",
			input: "# T\n\nUse [^n].\n\n[^o]: a\n    [^p]: b\n        ```\n    [^n]: y\n",
			want:  []footnotePosition{{3, 5}, {8, 5}},
		},
		{name: "definition after HTML on a footnote's first line", input: "# T\n\nUse [^n].\n\n[^o]: <div> x\n    [^n]: y\n", want: []footnotePosition{{3, 5}, {6, 5}}},
		{
			name:  "definition in a list from 01 after indented code and a blank line",
			input: "# T\n\nUse [^n].\n\n    code\n\n01. a\n    [^n]: y\n",
			want:  []footnotePosition{{3, 5}, {8, 5}},
		},
		{
			name:  "definition in a sibling of a footnote with HTML",
			input: "# T\n\nUse [^n].\n\n[^o]: <div> x\n[^p]: b\n    [^n]: y\n",
			want:  []footnotePosition{{3, 5}, {7, 5}},
		},
		{name: "definition after a fence that a footnote's end closes", input: "# T\n\nUse [^n].\n\n[^o]: a\n    ```\n[^n]: y\n", want: []footnotePosition{{3, 5}, {7, 1}}},
		{
			name:     "references in math",
			input:    "# T\n\nInline $[a][b]$ and $[^n]$ math.\n\n$$\n[c][d] and [e]\n$$\n\n[f][g] outside.\n",
			want:     []footnotePosition{{3, 9}, {3, 22}, {6, 1}, {6, 12}, {9, 1}},
			wantFull: []footnotePosition{{3, 9}, {6, 1}, {9, 1}},
		},
		{
			name:     "definition after a script block that goldmark ends at a long s",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<script>\n</\u017fcript>\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {8, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{
			name:     "definition after a pre block over several lines",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n<pre>\nx\n</pre>\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {9, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
	})
}

// TestReferenceLinkImagesRule_FootnoteLabels checks which labels MD052
// reads as GFM footnote calls. Only a link, not an image, whose label is
// "^" and then ASCII letters, digits and hyphens calls a footnote, and
// only a certain definition whose label holds the same letters in any
// case. Images and other labels, which micromark or markdownlint may read
// otherwise than as written, get main's result, and so do calls that
// markdownlint keeps apart from a definition. "[text][^label]" is
// reported with shortcut_syntax exactly where main reports it, unless
// markdownlint certainly ignores "text".
func TestReferenceLinkImagesRule_FootnoteLabels(t *testing.T) {
	checkFootnoteCases(t, []footnoteCase{
		{name: "full reference to a footnote that is also a link definition", input: "# T\n\nUse [text][^n].\n\n[^n]: /url\n"},
		{name: "footnote call with a long s", input: "Use [^\u017f].\n\n[^s]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{name: "shortcut before a footnote call with a sharp s", input: "Use [\u00df][^n].\n\n[^n]: Note text.\n\n[ss]: /a\n", want: []footnotePosition{{1, 5}}},
		{name: "footnote call with an accented capital", input: "Use [^\u00c9].\n\n[^\u00e9]: Note text.\n", want: []footnotePosition{{1, 5}, {3, 1}}},
		{name: "footnote call in another ASCII case", input: "Use [^Note].\n\n[^note]: Note text.\n"},
		{
			name:  "shortcut before a footnote call with a definition in math",
			input: "Use [text][^n].\n\n[^n]: Note text.\n\n$$\n\n[text]: /a\n",
			want:  []footnotePosition{{1, 5}},
		},
		{name: "shortcut before a footnote call with a defined label", input: "Use [text][^n].\n\n[^n]: Note text.\n\n[text]: /a\n", want: []footnotePosition{{1, 5}}},
		{name: "shortcut before a footnote call that markdownlint ignores", input: "Use [X][^n].\n\n[^n]: Note text.\n"},
		{name: "image as the text before a footnote call", input: "Use [![badge](/b.svg)][^n].\n\n[^n]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{name: "link in the text before a footnote call", input: "Use [x [b](/c)][^n].\n\n[^n]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{name: "image after an ignored label before a footnote call", input: "Use [x![i](/a)][^n].\n\n[^n]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{name: "next line control as the text before a footnote call", input: "Use [\u0085][^n].\n\n[^n]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{
			name:     "image shortcut before a footnote call",
			input:    "Use ![alt][^n].\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{1, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{
			name:     "ignored image shortcut before a footnote call",
			input:    "Use ![x][^n].\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{1, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{name: "blank text before a footnote call", input: "Use [ ][^n].\n\n[^n]: Note text.\n"},
		{name: "empty text before a footnote call", input: "Use [][^n].\n\n[^n]: Note text.\n"},
		{name: "blank text before a footnote call in a list item", input: "- [ ][^n] note\n\n[^n]: Note text.\n"},
		{name: "escaped bracket in the label", input: "# T\n\nUse [^e\\]x].\n\n[^e\\]x]: Escaped bracket.\n", want: []footnotePosition{{3, 5}, {5, 1}}},
		{
			name:     "collapsed image with an escape in a footnote label",
			input:    "Use ![^a\\*b][].\n\n[^a\\*b]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {3, 1}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{
			name:     "collapsed image with emphasis in a footnote label",
			input:    "Use ![^*n*][].\n\n[^*n*]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {3, 1}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{name: "shortcut image of a footnote label", input: "Use ![^n].\n\n[^n]: Note text.\n", want: []footnotePosition{{1, 5}}},
		{name: "shortcut image with an escape in a footnote label", input: "Use ![^a\\*b].\n\n[^a\\*b]: Note text.\n", want: []footnotePosition{{1, 5}, {3, 1}}},
		{
			name:     "full reference with an escape in a footnote label",
			input:    "Use [x][^a\\*b].\n\n[^a\\*b]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {3, 1}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{
			name:     "footnote label with an entity",
			input:    "Use [^a&amp;b] and [x][^a&amp;b].\n\n[^a&amp;b]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {1, 20}, {3, 1}},
			wantFull: []footnotePosition{{1, 20}},
		},
		{
			name:     "footnote label with an underscore",
			input:    "Use [^my_note] and [x][^my_note].\n\n[^my_note]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {1, 20}, {3, 1}},
			wantFull: []footnotePosition{{1, 20}},
		},
		{name: "footnote label with a code span", input: "Use [^`n`].\n\n[^`n`]: Note text.\n", want: []footnotePosition{{1, 5}, {3, 1}}},
		{name: "footnote label with a hyphen", input: "Use [^my-note] and [x][^my-note].\n\n[^my-note]: Note text.\n"},
		{name: "footnote label of digits", input: "Use [^1] and [x][^1].\n\n[^1]: Note text.\n"},
		{
			name:     "escaped hyphen in a footnote label",
			input:    "Use [^a\\-b] and [x][^a\\-b].\n\n[^a-b]: Note text.\n",
			want:     []footnotePosition{{1, 5}, {1, 17}},
			wantFull: []footnotePosition{{1, 17}},
		},
		{
			name:     "collapsed image of a plain footnote label",
			input:    "Use ![^n][].\n\n[^n]: Note text.\n",
			want:     []footnotePosition{{1, 5}},
			wantFull: []footnotePosition{{1, 5}},
		},
		{name: "form feed as the text before a footnote call", input: "Use [\f][^n].\n\n[^n]: Note text.\n"},
	})
}

// TestReferenceLinkImagesRule_UncertainFootnotes checks MD052 on GFM
// footnote definitions that micromark is not certain to read where they
// stand: in a container or a footnote, indented, or after a line that is
// not blank. They do not count, so their calls are read as goldmark reads
// them, and MD052 reports what main reports. markdownlint counts most of
// them. A certain definition in the same document, such as "[^o]:" at
// column 0, still counts.
func TestReferenceLinkImagesRule_UncertainFootnotes(t *testing.T) {
	checkFootnoteCases(t, []footnoteCase{
		{name: "definition in a list item", input: "# T\n\nUse [^third].\n\n- item\n\n  [^third]: In a list.\n", want: []footnotePosition{{3, 5}, {7, 3}}},
		{name: "definition in a blockquote", input: "# T\n\nUse [^q].\n\n> [^q]: In a quote.\n", want: []footnotePosition{{3, 5}, {5, 3}}},
		{name: "definition that interrupts a paragraph", input: "# T\n\nUse [^n].\nSome text\n[^n]: Interrupts the paragraph.\n", want: []footnotePosition{{3, 5}, {5, 1}}},
		{
			name:     "definition after a heading",
			input:    "# T\n[^n]: Note text.\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{2, 1}, {4, 5}, {4, 14}},
			wantFull: []footnotePosition{{4, 14}},
		},
		{name: "indented definition", input: "# T\n\nUse [^i].\n\n   [^i]: Indented three.\n", want: []footnotePosition{{3, 5}, {5, 4}}},
		{name: "paragraph line indented four columns", input: "# T\n\nUse [^n].\n\ntext\n    [^n]: note text\n", want: []footnotePosition{{3, 5}, {6, 5}}},
		{name: "definition that interrupts a list item's paragraph", input: "# T\n\nUse [^n].\n\n- item\n   [^n]: in item\n", want: []footnotePosition{{3, 5}, {6, 4}}},
		{name: "definition on a lazy line", input: "# T\n\nUse [^n].\n\n> quote\n[^n]: lazy\n", want: []footnotePosition{{3, 5}, {6, 1}}},
		{
			name:     "definition after a quote marker line",
			input:    "# T\n\nUse [^n] and [x][^n].\n\n> quote\n>\n[^n]: Note text.\n",
			want:     []footnotePosition{{3, 5}, {3, 14}, {7, 1}},
			wantFull: []footnotePosition{{3, 14}},
		},
		{name: "definition in a quoted list item", input: "# T\n\nUse [^n].\n\n> - a\n>     [^n]: x\n", want: []footnotePosition{{3, 5}, {6, 7}}},
		{name: "quoted list item line indented four columns", input: "# T\n\nUse [^n].\n\n> - a\n>       [^n]: x\n", want: []footnotePosition{{3, 5}, {6, 9}}},
		{name: "tab past a list item's indentation", input: "# T\n\nUse [^n].\n\n- a\n\n\t[^n]: tab\n"},
		{name: "defined link text before a footnote that is also a link definition", input: "# T\n\nUse [text][^n].\n\n[text]: /a\n[^n]: /url\n"},
		{
			name:     "definition after a definition's continuation line",
			input:    "# T\n\nUse [^a], [^b] and [x][^b].\n\n[^a]: One note.\n    more\n[^b]: Two notes.\n",
			want:     []footnotePosition{{3, 11}, {3, 20}, {7, 1}},
			wantFull: []footnotePosition{{3, 20}},
		},
		{name: "definition in a footnote's content", input: "# T\n\nUse [^n].\n\n[^o]:\n    [^n]: inner\n", want: []footnotePosition{{3, 5}, {6, 5}}},
		{name: "definition after text in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: text\n    [^n]: inner\n"},
		{name: "definition after a blank line in a footnote", input: "# T\n\nUse [^n].\n\n[^o]:\n\n    [^n]: inner\n", want: []footnotePosition{{3, 5}}},
		{name: "definition two footnotes deep", input: "# T\n\nUse [^n].\n\n[^o]:\n    [^p]:\n        [^n]: inner\n"},
		{name: "seven columns into a footnote", input: "# T\n\nUse [^n].\n\n[^o]:\n       [^n]: seven\n", want: []footnotePosition{{3, 5}, {6, 8}}},
		{name: "lazy line in a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\nlazy\n    [^n]: inner\n", want: []footnotePosition{{3, 5}, {7, 5}}},
		{name: "footnote in a list item", input: "# T\n\nUse [^n].\n\n- [^o]:\n      [^n]: inner\n", want: []footnotePosition{{3, 5}, {5, 3}, {6, 7}}},
		{name: "line after a closed footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n\ntext\n    [^n]: not\n", want: []footnotePosition{{3, 5}, {8, 5}}},
		{name: "indented line in an item marked 01", input: "# T\n\nUse [^n].\n\nIntro\n01. $$\n    [^n]: y\n    $$\n\nEnd.\n", want: []footnotePosition{{3, 5}, {7, 5}}},
		{name: "definition in an item marked 01", input: "# T\n\nUse [^n].\n\nIntro\n01. a\n    [^n]: y\n\nEnd.\n", want: []footnotePosition{{3, 5}, {7, 5}}},
		{name: "definition after a heading that ends a footnote", input: "# T\n\nUse [^n].\n\n[^o]: a\n# H\n    [^n]: y\n", want: []footnotePosition{{3, 5}}},
		{name: "definition in an item marked 1", input: "# T\n\nUse [^n].\n\n1. [^n]: y\n"},
		{name: "definition in the second item of a list from 1", input: "# T\n\nUse [^n].\n\n1. a\n2. b\n   [^n]: y\n", want: []footnotePosition{{3, 5}, {7, 4}}},
		{name: "definition after a formula on one line", input: "# T\n\nUse [^n].\n\n- Equation:\n  $$E = mc^2$$\n  [^n]: y\n", want: []footnotePosition{{3, 5}, {7, 3}}},
		{name: "definition in a bullet item", input: "# T\n\nUse [^n].\n\n- [^n]: y\n"},
		{name: "indented line after text", input: "Intro\n    $$\n\n[a][b]\n", want: []footnotePosition{{4, 1}}, wantFull: []footnotePosition{{4, 1}}},
	})
}

// TestReferenceLinkImagesRule_FootnotesWithoutCertainDefinitions checks
// MD052 where markdownlint may take other lines than the parser for front
// matter, or the text that markdownlint gives micromark starts with a byte
// order mark. No definition counts there, and MD052 reports what main
// reports.
func TestReferenceLinkImagesRule_FootnotesWithoutCertainDefinitions(t *testing.T) {
	checkFootnoteCases(t, []footnoteCase{
		{
			name:     "front matter closed with dots",
			input:    "---\ntitle: x\n...\n\n[^n]: Note text.\n---\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{5, 1}, {8, 5}, {8, 14}},
			wantFull: []footnotePosition{{8, 14}},
		},
		{
			name:     "front matter opened with a no-break space",
			input:    "---\u00a0\ntitle: x\n---\n\n[^n]: Note text.\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{5, 1}, {7, 5}, {7, 14}},
			wantFull: []footnotePosition{{7, 14}},
		},
		{
			name:     "line separator in front matter",
			input:    "---\ntitle: x\u2028---\n---\n\n[^n]: Note text.\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{5, 1}, {7, 5}, {7, 14}},
			wantFull: []footnotePosition{{7, 14}},
		},
		{name: "two byte order marks", input: "\ufeff\ufeffIntro\n\n[^b]: note text.\n\nUse [^b].\n", want: []footnotePosition{{3, 1}, {5, 5}}},
		{
			name:     "byte order mark after front matter",
			input:    "---\ntitle: x\n---\n\ufeff<!--\n\n[^n]: Note text.\n-->\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{6, 1}, {9, 5}, {9, 14}},
			wantFull: []footnotePosition{{9, 14}},
		},
		{
			name:     "byte order mark after front matter and a no-break space line",
			input:    "---\ntitle: x\n---\n\u00a0\n\ufeff<!--\n\n[^n]: Note text.\n-->\n\nUse [^n] and [x][^n].\n",
			want:     []footnotePosition{{7, 1}, {10, 5}, {10, 14}},
			wantFull: []footnotePosition{{10, 14}},
		},
		{
			name:  "byte order mark after front matter before a fence",
			input: "---\ntitle: x\n---\n\ufeff```\n\n[^n]: Note text.\n```\n\nUse [^n] and [x][^n].\n",
			want:  []footnotePosition{{6, 1}},
		},
		{name: "two byte order marks before a fence", input: "\ufeff\ufeff```\n\n[^b]: note text.\n```\n\nUse [^b] and [x][^b].\n", want: []footnotePosition{{3, 1}}},
	})
}

// TestMarkdownlintFrontMatterLines checks the front matter that
// markdownlint 0.41.1's frontMatterRe takes, up to its closing line, which
// MD052 compares with the parser's before it counts a footnote definition.
func TestMarkdownlintFrontMatterLines(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   int
		wantOK bool
	}{
		{name: "no front matter", input: "# T\n", want: 0, wantOK: true},
		{name: "YAML", input: "---\na: 1\n---\nText\n", want: 3, wantOK: true},
		{name: "spaces and a tab after the markers", input: "--- \t\na: 1\n---  \n", want: 3, wantOK: true},
		{name: "YAML closed with dots only", input: "---\na: 1\n...\n", want: 0, wantOK: true},
		{name: "YAML closed with dots and then dashes", input: "---\na: 1\n...\n---\n", want: 4, wantOK: true},
		{name: "TOML closed with dots", input: "+++\na = 1\n...\n", want: 3, wantOK: true},
		{name: "JSON", input: "{\n\"a\": 1\n}\n", want: 3, wantOK: true},
		{name: "no-break space after the opening", input: "---\u00a0\na: 1\n---\n", want: 3, wantOK: true},
		{name: "text after the opening", input: "--- x\na: 1\n---\n", want: 0, wantOK: true},
		{name: "four dashes", input: "----\na\n---\n", want: 0, wantOK: true},
		{name: "unclosed", input: "---\na: 1\n", want: 0, wantOK: true},
		{name: "closing with a no-break space", input: "---\na: 1\n---\u00a0\n", want: 3, wantOK: true},
		{name: "closing without a final newline", input: "---\na: 1\n---", want: 3, wantOK: true},
		{name: "byte order mark", input: "\ufeff---\na: 1\n---\n", want: 3, wantOK: true},
		{name: "line separator", input: "---\na\u2028---\n---\n", want: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := markdownlintFrontMatterLines(parseForCheck(t, tt.input))
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMicromarkDropsByteOrderMark checks where the text that markdownlint
// gives micromark starts with a byte order mark: after the one markdownlint
// drops from the file, or after front matter and the white space lines
// that markdownlint's pattern takes with it.
func TestMicromarkDropsByteOrderMark(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "no byte order mark", input: "# T\n", want: false},
		{name: "one byte order mark", input: "\ufeff# T\n", want: false},
		{name: "two byte order marks", input: "\ufeff\ufeff# T\n", want: true},
		{name: "byte order mark before front matter", input: "\ufeff---\na: 1\n---\n# T\n", want: false},
		{name: "byte order mark right after front matter", input: "---\na: 1\n---\n\ufeff<!--\n", want: true},
		{name: "after a blank line after front matter", input: "---\na: 1\n---\n\n\ufeff<!--\n", want: true},
		{name: "after a no-break space line after front matter", input: "---\na: 1\n---\n\u00a0\n\ufeff<!--\n", want: true},
		{name: "after text after front matter", input: "---\na: 1\n---\ntext\n\ufeff<!--\n", want: false},
		{name: "after a line that holds only a byte order mark", input: "---\na: 1\n---\n\ufeff\n\ufeff<!--\n", want: true},
		// micromark keeps this one, as the pattern takes the line, but a
		// line that starts with a byte order mark is left unsure.
		{name: "line that holds only a byte order mark after front matter", input: "---\na: 1\n---\n\ufeff\n# T\n", want: true},
		{name: "line separator after front matter", input: "---\na: 1\n---\n\u2028\n# T\n", want: true},
		{name: "byte order mark on a later line", input: "# T\n\n\ufeff<!--\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, micromarkDropsByteOrderMark(parseForCheck(t, tt.input)))
		})
	}
}

// TestReferenceLinkImagesRule_BlankLabelsAndCheckboxes checks MD052 where
// task checkboxes or blank brackets decide what is a reference. The parser
// reads goldmark's checkboxes, as main did: after link reference definitions
// "[x]" is a shortcut reference, which markdownlint and main report when "x"
// is not ignored, and at the start of a list item's first block, a heading
// included, brackets holding one space, tab, form feed or carriage return,
// "x" or "X" are a checkbox. markdownlint never reports a label of nothing
// but ASCII white space, so MD052 does not either. Blank text before a
// label is read as main reads it.
func TestReferenceLinkImagesRule_BlankLabelsAndCheckboxes(t *testing.T) {
	type position struct{ line, column int }
	shortcut := map[string]any{"shortcut_syntax": true}
	xNotIgnored := map[string]any{"shortcut_syntax": true, "ignored_labels": []string{"skip"}}
	tests := []struct {
		name    string
		input   string
		options map[string]any
		want    []position
	}{
		{name: "checkbox after a definition", input: "- [c]: /url\n  [x] done\n", options: xNotIgnored, want: []position{{2, 3}}},
		{name: "checkbox after two definitions", input: "- [c]: /url\n  [d]: /other\n  [x] done\n", options: xNotIgnored, want: []position{{3, 3}}},
		{
			name:    "checkbox after a blank first line and a definition",
			input:   "-\n  [c]: /url\n  [x] done\n",
			options: xNotIgnored,
			want:    []position{{3, 3}},
		},
		// The parser reads a checkbox here, as main does, so "[x]" is no
		// reference. markdownlint, which reads no checkboxes, reports 1:3.
		{name: "checkbox", input: "- [x] done\n", options: xNotIgnored},
		{name: "form feed before an empty pair in a list item", input: "- [\f][] done\n"},
		{name: "form feed before an empty pair in a list item with shortcut_syntax", input: "- [\f][] done\n", options: shortcut},
		{name: "form feed before an empty pair", input: "Use [\f][] done\n"},
		{name: "form feed in a list item", input: "- [\f] done\n", options: shortcut},
		{name: "vertical tab before an empty pair", input: "Use [\v][] done.\n"},
		{name: "form feeds and spaces before an empty pair", input: "Use [ \f \f ][] done.\n"},
		{name: "form feed as a full reference's label", input: "Use [text][\f] done.\n", options: shortcut},
		{name: "form feed as an image's label", input: "Use ![alt][\f] done.\n"},
		// White space outside ASCII is a label, as on main, although
		// markdownlint trims it and reports nothing.
		{name: "no-break space before an empty pair", input: "Use [\xc2\xa0][] done.\n", want: []position{{1, 5}}},
		{name: "letter before an empty pair", input: "Use [y][] done.\n", want: []position{{1, 5}}},
		// The parser reads a checkbox from "[\f]" and "[x]" at the start of
		// an item, as main does, so the brackets after it are a shortcut.
		{name: "form feed before a label in a list item", input: "- [\f][y] done\n"},
		{name: "form feed before a label in a list item with shortcut_syntax", input: "- [\f][y] done\n", options: shortcut, want: []position{{1, 6}}},
		{name: "checkbox before a footnote label", input: "- [x][^n]\n\n[c]: /url\n[^n]: Note text.\n"},
		{
			name:    "checkbox before a footnote label with shortcut_syntax",
			input:   "- [x][^n]\n\n[c]: /url\n[^n]: Note text.\n",
			options: shortcut,
			want:    []position{{1, 6}, {4, 1}},
		},
		// Blank text before a label makes a full reference, as on main.
		{name: "form feed before a label", input: "Use [\f][y] done.\n", want: []position{{1, 5}}},
		{name: "form feed before a label with shortcut_syntax", input: "Use [\f][y] done.\n", options: shortcut, want: []position{{1, 5}}},
		{name: "space before a label in an image", input: "Use ![ ][y] done.\n", options: shortcut, want: []position{{1, 5}}},
		{name: "space before a label in a heading", input: "# T [ ][y] done\n", options: shortcut, want: []position{{1, 5}}},
		// At the start of a heading that starts a list item, the parser
		// reads a checkbox, as main does, and then the brackets after it.
		{name: "space before a label in a heading in a list item", input: "- ## [ ][y] done\n"},
		{name: "space before a label in a heading in a list item with shortcut_syntax", input: "- ## [ ][y] done\n", options: shortcut, want: []position{{1, 9}}},
		{name: "tab before a label in a quoted heading", input: "> - ## [\t][y] done\n", options: shortcut, want: []position{{1, 11}}},
		{name: "space before a label in a setext heading", input: "- [ ][y]\n  ---\n", options: shortcut, want: []position{{1, 6}}},
		{name: "space before a footnote label in a heading", input: "- ## [ ][^n] done\n", options: shortcut, want: []position{{1, 9}}},
		// Two spaces make no checkbox, so the brackets make a full reference.
		{name: "two spaces before a label in a heading", input: "- ## [  ][y] done\n", options: shortcut, want: []position{{1, 6}}},
		{name: "space before a collapsed reference in a heading", input: "- ## [ ][y][] done\n", want: []position{{1, 9}}},
		{name: "space before a full reference in a heading", input: "- ## [ ][y][z] done\n", options: shortcut, want: []position{{1, 9}}},
		{name: "space before a full reference to x in a heading", input: "- ## [ ][x][y] done\n", want: []position{{1, 9}}},
		{name: "space before a full reference with an ignored label in a heading", input: "- ## [ ][y][x] done\n", options: shortcut},
		{
			name:    "space before a full reference and a shortcut in a heading",
			input:   "- ## [ ][y][z][w] done\n",
			options: shortcut,
			want:    []position{{1, 9}, {1, 15}},
		},
		{name: "x before a full reference in a heading", input: "- ## [x][y][z] done\n", options: shortcut, want: []position{{1, 9}}},
		{name: "x before a label in a heading", input: "- ## [x][y] done\n"},
		{name: "x before a label in a heading with shortcut_syntax", input: "- ## [x][y] done\n", options: shortcut, want: []position{{1, 9}}},
		{name: "space before a label over two lines in a setext heading", input: "- [ ][a\n  b]\n  ---\n", options: shortcut, want: []position{{1, 6}}},
		{
			name:    "space before a label over two lines in a quoted setext heading",
			input:   "> - [ ][a\n>   b]\n>   ---\n",
			options: shortcut,
			want:    []position{{1, 8}},
		},
		{name: "space before a label in a heading after a blank first line", input: "-\n  ## [ ][y] done\n", options: shortcut, want: []position{{2, 9}}},
		// After a definition, goldmark reads no checkbox.
		{name: "space before a label in a heading after a definition", input: "- [c]: /url\n  ## [ ][y] done\n", options: shortcut, want: []position{{2, 6}}},
		{name: "x before a label in a heading after a blank first line and a definition", input: "-\n  [c]: /url\n  ## [x][y] done\n", want: []position{{3, 6}}},
		{
			name:    "x before a label in a heading after a blank first line and a definition with shortcut_syntax",
			input:   "-\n  [c]: /url\n  ## [x][y] done\n",
			options: shortcut,
			want:    []position{{3, 6}},
		},
		{
			name:    "x before a label in a setext heading after a blank first line and a definition",
			input:   "-\n  [c]: /url\n  [x][y] done\n  ---\n",
			options: shortcut,
			want:    []position{{3, 3}},
		},
		{
			name:    "space before a label in a heading after a blank first line and a definition",
			input:   "-\n  [c]: /url\n  ## [ ][y] done\n",
			options: shortcut,
			want:    []position{{3, 6}},
		},
		// Nor does anything before the brackets.
		{name: "space before a label in emphasis in a heading", input: "- ## *[ ][y]* done\n", options: shortcut, want: []position{{1, 7}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewReferenceLinkImagesRule(), tt.input, tt.options)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			want := tt.want
			if want == nil {
				want = []position{}
			}
			assert.Equal(t, want, got)
		})
	}
}

// TestRulesReadNoCheckboxAfterDefinitions checks the link rules on "[x]"
// after link reference definitions that start a list item. The parser reads
// no checkbox there, as goldmark and GitHub's cmark-gfm read none, so "[x]"
// is a reference: it uses the definition of "x", which MD053 then keeps,
// and its link is empty for MD042 and a shortcut for MD054, as markdownlint
// and main report.
func TestRulesReadNoCheckboxAfterDefinitions(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name    string
		rule    lint.Rule
		input   string
		options map[string]any
		want    []position
	}{
		{name: "MD053 definition of a spaced label", rule: NewLinkImageRefDefsRule(), input: "- [c]: /url\n  [x] done [c]\n\n[ x ]: /x\n"},
		{name: "MD053 definition", rule: NewLinkImageRefDefsRule(), input: "- [c]: /url\n  [x] done [c]\n\n[x]: /x\n"},
		{name: "MD042 empty fragment", rule: NewEmptyLinkRule(), input: "- [c]: /url\n  [x] done\n\n[x]: #\n", want: []position{{2, 3}}},
		{
			name:    "MD054 shortcut",
			rule:    NewLinkImageStyleRule(),
			input:   "- [c]: /url\n  [x] done [c]\n\n[x]: /x\n",
			options: map[string]any{"shortcut": false},
			want:    []position{{2, 3}, {2, 12}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, tt.rule, tt.input, tt.options)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			want := tt.want
			if want == nil {
				want = []position{}
			}
			assert.Equal(t, want, got)
		})
	}
}

// TestRulesReadCheckboxesInHeadings checks the link rules where goldmark,
// as on main, reads a checkbox at the start of a heading that starts a list
// item, and where it reads none: under CommonMark, MD052 reads "[x][y]" as
// a full reference; "[X]" is a checkbox and no link, though "[x]: #" would
// define it; and so is "[\r]", before "(/url)" or a label.
func TestRulesReadCheckboxesInHeadings(t *testing.T) {
	type position struct{ line, column int }
	tests := []struct {
		name    string
		rule    lint.Rule
		flavor  config.Flavor
		input   string
		options map[string]any
		want    []position
	}{
		{name: "MD052 under CommonMark", rule: NewReferenceLinkImagesRule(), flavor: config.FlavorCommonMark, input: "- ## [x][y] done\n", want: []position{{1, 6}}},
		{
			name:    "MD052 under CommonMark with shortcut_syntax",
			rule:    NewReferenceLinkImagesRule(),
			flavor:  config.FlavorCommonMark,
			input:   "- ## [x][y] done\n",
			options: map[string]any{"shortcut_syntax": true},
			want:    []position{{1, 6}},
		},
		{
			name:    "MD052 with shortcut_syntax",
			rule:    NewReferenceLinkImagesRule(),
			input:   "- ## [x][y] done\n",
			options: map[string]any{"shortcut_syntax": true},
			want:    []position{{1, 9}},
		},
		{name: "MD042 on a checkbox", rule: NewEmptyLinkRule(), input: "- # [X]\n\n[x]: #\n"},
		{name: "MD054 on a checkbox", rule: NewLinkImageStyleRule(), input: "- # [X]\n\n[x]: #\n", options: map[string]any{"shortcut": false}},
		{name: "MD039 on a checkbox before a destination", rule: NewLinkSpacesRule(), input: "- # [\r](/url)\n"},
		{name: "MD039 on a checkbox before a destination and a period", rule: NewLinkSpacesRule(), input: "- ## [ ](/url).\n"},
		{name: "MD054 on a checkbox before a label", rule: NewLinkImageStyleRule(), input: "- # [\r][y]\n\n[y]: /url\n", options: map[string]any{"full": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor := tt.flavor
			if flavor == "" {
				flavor = config.FlavorGFM
			}
			file, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)
			cfg := config.NewConfig()
			cfg.Flavor = flavor
			ruleCfg := &config.RuleConfig{Options: engineOptions(tt.rule.ID(), tt.options)}
			diags, err := tt.rule.Apply(lint.NewRuleContext(context.Background(), file, cfg, ruleCfg))
			require.NoError(t, err)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			want := tt.want
			if want == nil {
				want = []position{}
			}
			assert.Equal(t, want, got)
		})
	}
}

// TestLinkImageStyleRule_BlankLabels checks that MD054 reports no reference
// whose label is nothing but ASCII white space, as markdownlint reports
// none, and that the parser's checkboxes hide the brackets they hide on
// main.
func TestLinkImageStyleRule_BlankLabels(t *testing.T) {
	type position struct{ line, column int }
	noCollapsed := map[string]any{"collapsed": false}
	noShortcut := map[string]any{"shortcut": false}
	tests := []struct {
		name    string
		input   string
		options map[string]any
		want    []position
	}{
		{name: "collapsed form feed in a list item", input: "- [\f][] done\n\n[\f]: /url\n", options: noCollapsed},
		{name: "shortcut form feed in a list item", input: "- [\f] done\n\n[\f]: /url\n", options: noShortcut},
		{name: "collapsed form feed", input: "Use [\f][] done.\n\n[\f]: /url\n", options: noCollapsed},
		{name: "shortcut form feed", input: "Use [\f] done.\n\n[\f]: /url\n", options: noShortcut},
		{name: "collapsed form feed in a heading", input: "- ## [\f][] done\n\n[\f]: /url\n", options: noCollapsed},
		{name: "collapsed letter", input: "Use [y][] done.\n\n[y]: /url\n", options: noCollapsed, want: []position{{1, 5}}},
		{name: "checkbox before an empty pair", input: "- [x][] done\n\n[x]: /url\n", options: noCollapsed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewLinkImageStyleRule(), tt.input, tt.options)
			got := make([]position, 0, len(diags))
			for _, diag := range diags {
				got = append(got, position{diag.StartLine, diag.StartColumn})
			}
			want := tt.want
			if want == nil {
				want = []position{}
			}
			assert.Equal(t, want, got)
		})
	}
}

// TestIgnoresShortcut checks when markdownlint certainly ignores the
// shortcut reference before a footnote call: only for a label of ASCII
// letters, digits and spaces, compared as normalizeReference gives it with
// ignored_labels as written.
func TestIgnoresShortcut(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		ignored []string
		want    bool
	}{
		{name: "default label", text: "x", ignored: []string{"x"}, want: true},
		{name: "label in upper case", text: "X", ignored: []string{"x"}, want: true},
		{name: "ignored label in upper case", text: "x", ignored: []string{"X"}, want: false},
		{name: "white space collapsed", text: " x  y ", ignored: []string{"x y"}, want: true},
		{name: "emphasis", text: "*x*", ignored: []string{"*x*"}, want: false},
		{name: "escape", text: "a\\*b", ignored: []string{"a\\*b"}, want: false},
		{name: "letter outside ASCII", text: "é", ignored: []string{"é"}, want: false},
		{name: "blank", text: " ", ignored: []string{"x"}, want: true},
		{name: "tab", text: "\t", ignored: nil, want: true},
		{name: "empty", text: "", ignored: nil, want: true},
		{name: "form feed", text: "\f", ignored: nil, want: true},
		{name: "vertical tab and space", text: "\v ", ignored: []string{"x"}, want: true},
		{name: "no-break space", text: "\u00a0", ignored: nil, want: false},
		{name: "label not ignored", text: "y", ignored: []string{"x"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ignoresShortcut(tt.text, tt.ignored))
		})
	}
}

// TestPlainFootnoteCall checks which usages MD052 may read as a footnote
// call: a link, never an image, whose label is "^" and then nothing but
// ASCII letters, digits and hyphens, which every parser reads as written.
func TestPlainFootnoteCall(t *testing.T) {
	tests := []struct {
		name  string
		usage refs.ReferenceUsage
		want  bool
	}{
		{name: "digits", usage: refs.ReferenceUsage{Label: "^1"}, want: true},
		{name: "letters in any case", usage: refs.ReferenceUsage{Label: "^Note"}, want: true},
		{name: "hyphen", usage: refs.ReferenceUsage{Label: "^my-note"}, want: true},
		{name: "full reference", usage: refs.ReferenceUsage{Style: refs.StyleFull, Label: "^n"}, want: true},
		{name: "image", usage: refs.ReferenceUsage{IsImage: true, Label: "^n"}, want: false},
		{name: "no caret", usage: refs.ReferenceUsage{Label: "note"}, want: false},
		{name: "caret alone", usage: refs.ReferenceUsage{Label: "^"}, want: false},
		{name: "underscore", usage: refs.ReferenceUsage{Label: "^my_note"}, want: false},
		{name: "escape", usage: refs.ReferenceUsage{Label: "^a\\*b"}, want: false},
		{name: "emphasis", usage: refs.ReferenceUsage{Label: "^*n*"}, want: false},
		{name: "entity", usage: refs.ReferenceUsage{Label: "^a&amp;b"}, want: false},
		{name: "code span", usage: refs.ReferenceUsage{Label: "^`n`"}, want: false},
		{name: "HTML", usage: refs.ReferenceUsage{Label: "^<b>n</b>"}, want: false},
		{name: "space", usage: refs.ReferenceUsage{Label: "^a b"}, want: false},
		{name: "letter outside ASCII", usage: refs.ReferenceUsage{Label: "^é"}, want: false},
		{name: "second caret", usage: refs.ReferenceUsage{Label: "^^n"}, want: false},
		{name: "comma", usage: refs.ReferenceUsage{Label: "^a,b"}, want: false},
		{name: "dot", usage: refs.ReferenceUsage{Label: "^a.b"}, want: false},
		{name: "slash", usage: refs.ReferenceUsage{Label: "^a/b"}, want: false},
		{name: "colon", usage: refs.ReferenceUsage{Label: "^a:b"}, want: false},
		{name: "at sign", usage: refs.ReferenceUsage{Label: "^a@b"}, want: false},
		{name: "opening bracket", usage: refs.ReferenceUsage{Label: "^a[b"}, want: false},
		{name: "opening brace", usage: refs.ReferenceUsage{Label: "^a{b"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, plainFootnoteCall(&tt.usage))
		})
	}
}

// TestLabelKey checks the keys under which MD052 matches footnote labels:
// white space collapsed and trimmed as markdownlint's normalizeReference
// does, lowercase only for a label that is all ASCII, and every other byte
// kept, so labels with a character outside ASCII match only when they are
// the same bytes.
func TestLabelKey(t *testing.T) {
	tests := []struct {
		name  string
		label string
		want  string
	}{
		{name: "ASCII letters in any case", label: "^Note", want: "^note"},
		{name: "white space collapsed and trimmed", label: " a \t\n b ", want: "a b"},
		{name: "JavaScript white space", label: "a\u00a0\u2028\ufeffb\u3000", want: "a b"},
		{name: "accented capital kept", label: "^É", want: "^É"},
		{name: "long s kept", label: "^\u017f", want: "^\u017f"},
		{name: "sharp s kept", label: "ß", want: "ß"},
		{name: "ASCII next to a letter outside ASCII kept", label: "^Aé", want: "^Aé"},
		{name: "invalid bytes kept", label: "a\xffb", want: "a\xffb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, labelKey(tt.label))
		})
	}
	assert.NotEqual(t, labelKey("\xfe"), labelKey("\xff"), "invalid bytes stay apart")
}

// TestFootnoteDefinitionLabel checks the labels that micromark reads in a
// footnote definition, whose length it counts in UTF-16 code units, up to
// 999.
func TestFootnoteDefinitionLabel(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		want   string
		wantOK bool
	}{
		{name: "label", text: "[^note]: text", want: "note", wantOK: true},
		{name: "escaped bracket", text: "[^a\\]b]: text", want: "a\\]b", wantOK: true},
		{name: "empty label", text: "[^]: text"},
		{name: "space in the label", text: "[^a b]: text"},
		{name: "no colon", text: "[^note] text"},
		{name: "999 code units", text: "[^" + strings.Repeat("\U0001F600", 499) + "a]: x", want: strings.Repeat("\U0001F600", 499) + "a", wantOK: true},
		{name: "1000 code units", text: "[^" + strings.Repeat("\U0001F600", 500) + "]: x"},
		{name: "999 code units with an escape", text: "[^" + strings.Repeat("a", 997) + "\\]]: x", want: strings.Repeat("a", 997) + "\\]", wantOK: true},
		{name: "1000 code units with an escape", text: "[^" + strings.Repeat("a", 998) + "\\]]: x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := footnoteDefinitionLabel([]byte(tt.text))
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestOpensRawHTMLBlock checks which lines open an HTML block of kinds 1 to
// 5 that does not end on the line, as micromark reads them.
func TestOpensRawHTMLBlock(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "<!--", want: true},
		{line: "<!-- note -->", want: false},
		{line: "<!-->", want: false},
		{line: "<?php", want: true},
		{line: "<?>", want: false},
		{line: "<![CDATA[ x", want: true},
		{line: "<![CDATA[ x ]]>", want: false},
		{line: "<![CDATA[x]]]>", want: true},
		{line: "<![CDATA[x]]]]>", want: false},
		{line: "<![CDATA[]]>", want: false},
		{line: "<![CDATA[>", want: true},
		{line: "<![CDATA[x]>", want: true},
		{line: "<!doctype", want: true},
		{line: "<!DOCTYPE html>", want: false},
		{line: "<script>", want: true},
		{line: "<SCRIPT src=a.js>", want: true},
		{line: "<textarea", want: true},
		{line: "<pre\tclass=x>", want: true},
		{line: "<script>a</b></script>", want: false},
		{line: "<script></</script>", want: false},
		{line: "<script>x</script", want: true},
		{line: "<script></\u017fcript>", want: true},
		{line: "<pre>x</PRE>", want: false},
		{line: "<pre>x</pre >", want: true},
		{line: "<pre>x</prescription>", want: true},
		{line: "<style>x</script>", want: false},
		{line: "<prescription>", want: false},
		{line: "<script/>", want: false},
		{line: "<scripts>", want: false},
		{line: "<div>", want: false},
		{line: "text <!--", want: false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, opensRawHTMLBlock([]byte(tt.line)), "%q", tt.line)
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
