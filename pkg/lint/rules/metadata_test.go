package rules

import (
	"context"
	"reflect"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestFirstLineHeadingRule(t *testing.T) {
	tests := []struct {
		name  string
		input string
		level int
		wantN int
	}{
		{
			name:  "starts with h1",
			input: "# Title\n\nContent",
			level: 1,
			wantN: 0,
		},
		{
			name:  "starts with h2 expecting h1",
			input: "## Title\n\nContent",
			level: 1,
			wantN: 1,
		},
		{
			name:  "starts with h2 expecting h2",
			input: "## Title\n\nContent",
			level: 2,
			wantN: 0,
		},
		{
			name:  "starts with paragraph",
			input: "Some text\n\n# Title",
			level: 1,
			wantN: 1,
		},
		{
			name:  "empty file",
			input: "",
			level: 1,
			wantN: 0,
		},
		{
			name:  "blank lines then h1",
			input: "\n\n# Title",
			level: 1,
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

			rule := NewFirstLineHeadingRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"level": tt.level,
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

func TestFirstLineHeadingRule_FrontMatter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		wantN   int
	}{
		{
			name: "front matter with title",
			input: `---
title: My Title
---

Some content`,
			pattern: "^title:",
			wantN:   0,
		},
		{
			name: "front matter without title",
			input: `---
author: John
---

Some content`,
			pattern: "^title:",
			wantN:   1,
		},
		{
			name: "front matter with h1 after",
			input: `---
author: John
---

# Title

Content`,
			pattern: "^title:",
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

			rule := NewFirstLineHeadingRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"front_matter_title": tt.pattern,
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

// TestFirstLineHeadingRule_FrontMatterTitleDefaults verifies markdownlint's
// case-insensitive default front_matter_title handling.
func TestFirstLineHeadingRule_FrontMatterTitleDefaults(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options map[string]any
		wantN   int
	}{
		{name: "lowercase title", input: "---\ntitle: Doc\n---\n\nText.\n", wantN: 0},
		{name: "capitalised title", input: "---\nTitle: Doc\n---\n\nText.\n", wantN: 0},
		{name: "no title", input: "---\nauthor: Someone\n---\n\nText.\n", wantN: 1},
		{
			name:    "empty pattern disables front matter titles",
			input:   "---\ntitle: Doc\n---\n\nText.\n",
			options: map[string]any{"front_matter_title": ""},
			wantN:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewFirstLineHeadingRule(), tt.input, tt.options)
			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

// TestFirstLineHeadingRule_HTMLHeading verifies that, like markdownlint, an
// HTML heading of the required level may start the document.
func TestFirstLineHeadingRule_HTMLHeading(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		options map[string]any
		wantN   int
	}{
		{name: "centered h1", input: "<h1 align=\"center\">Logo</h1>\n\nText.\n", wantN: 0},
		{name: "h2 when h1 is required", input: "<h2>Sub</h2>\n\nText.\n", wantN: 1},
		{name: "h2 when h2 is required", input: "<h2>Sub</h2>\n\nText.\n", options: map[string]any{"level": 2}, wantN: 0},
		{name: "other HTML", input: "<p align=\"center\">Logo</p>\n\n# Title\n", wantN: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewFirstLineHeadingRule(), tt.input, tt.options)
			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
			}
		})
	}
}

func TestFirstLineHeadingRule_DefaultDisabled(t *testing.T) {
	rule := NewFirstLineHeadingRule()
	if rule.DefaultEnabled() {
		t.Error("FirstLineHeadingRule should be disabled by default")
	}
}

func TestHeadingBlankLinesRule(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		linesAbove int
		linesBelow int
		relaxed    bool // Sets markdownlint_compatible: false.
		wantN      int
	}{
		{
			name:       "proper blank lines",
			input:      "Text\n\n# Heading\n\nMore text",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      0,
		},
		{
			name:       "missing blank above",
			input:      "Text\n# Heading\n\nMore text",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      1,
		},
		{
			name:       "missing blank below",
			input:      "Text\n\n# Heading\nMore text",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      1,
		},
		{
			name:       "missing both",
			input:      "Text\n# Heading\nMore text",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      2,
		},
		{
			name:       "heading at start",
			input:      "# Heading\n\nText",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      0,
		},
		{
			name:       "heading at end",
			input:      "Text\n\n# Heading",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      0,
		},
		{
			// markdownlint, and so the default, wants blank lines between
			// adjacent headings too.
			name:       "consecutive headings flagged by default",
			input:      "# H1\n## H2\n\nText",
			linesAbove: 1,
			linesBelow: 1,
			wantN:      2,
		},
		{
			name:       "consecutive headings allowed when relaxed",
			input:      "# H1\n## H2\n\nText",
			linesAbove: 1,
			linesBelow: 1,
			relaxed:    true,
			wantN:      0, // Headings directly adjacent to other headings are OK.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewHeadingBlankLinesRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"lines_above": tt.linesAbove,
					"lines_below": tt.linesBelow,
				},
			}
			if tt.relaxed {
				ruleCfg.Options["markdownlint_compatible"] = false
			}

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ctx)
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}

			if len(diags) != tt.wantN {
				t.Errorf("got %d diagnostics, want %d", len(diags), tt.wantN)
				for _, d := range diags {
					t.Logf("  - %s at line %d", d.Message, d.StartLine)
				}
			}
		})
	}
}

// TestHeadingBlankLinesRule_Containers verifies that MD022 counts blank lines
// within blockquotes and list items, and that its fix keeps the container.
func TestHeadingBlankLinesRule_Containers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "blank blockquote line counts as blank", input: "> ## Quoted\n>\n> Para\n", wantLines: []int{}},
		{
			name:      "below inside a blockquote",
			input:     "> ## Quoted\n> Para\n",
			wantLines: []int{1},
			wantFixed: "> ## Quoted\n>\n> Para\n",
		},
		{
			name:      "above inside a blockquote",
			input:     "> Para\n> ## Quoted\n>\n> More\n",
			wantLines: []int{2},
			wantFixed: "> Para\n>\n> ## Quoted\n>\n> More\n",
		},
		{
			name:      "nested blockquote",
			input:     "> Para\n> > ## Nested\n",
			wantLines: []int{2},
			wantFixed: "> Para\n> >\n> > ## Nested\n",
		},
		{
			name:      "setext heading in a blockquote",
			input:     "> Title\n> -----\n> Para\n",
			wantLines: []int{1},
			wantFixed: "> Title\n> -----\n>\n> Para\n",
		},
		{
			name:      "list item",
			input:     "- item\n\n  ## Heading\n  Text\n",
			wantLines: []int{3},
			wantFixed: "- item\n\n  ## Heading\n\n  Text\n",
		},
		{
			name:      "lazy line after a quoted heading",
			input:     "> ## Quoted\nlazy text\n",
			wantLines: []int{1},
			wantFixed: "> ## Quoted\n>\nlazy text\n",
		},
		{name: "heading at the end of the file", input: "Text\n\n# Heading\n", wantLines: []int{}},
		{name: "heading at the end without a newline", input: "Text\n\n# Heading", wantLines: []int{}},
		{
			// The underline belongs to the heading: count from below it and
			// never insert between the text and the underline.
			name:      "setext heading followed by text",
			input:     "# Title\n\nSetext heading\n--------------\nParagraph.\n",
			wantLines: []int{3},
			wantFixed: "# Title\n\nSetext heading\n--------------\n\nParagraph.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHeadingBlankLinesRule(), tt.input, nil)
			if got := diagLines(diags); !reflect.DeepEqual(got, tt.wantLines) {
				t.Errorf("diagnostic lines = %v, want %v", got, tt.wantLines)
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			if fixed := applyAllEdits(t, tt.input, diags); fixed != want {
				t.Errorf("fixed = %q, want %q", fixed, want)
			}
		})
	}
}

// TestHeadingBlankLinesRule_DefaultAdjacentHeadingsEngine verifies that, by
// default, adjacent headings are reported and the engine's fix inserts a
// single blank line into the gap they share.
func TestHeadingBlankLinesRule_DefaultAdjacentHeadingsEngine(t *testing.T) {
	registry := lint.NewRegistry()
	registry.Register(NewHeadingBlankLinesRule())
	cfg := config.NewConfig()
	cfg.Fix = true
	input := "# Title\n## Section\n\nText.\n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).
		LintFile(context.Background(), "test.md", []byte(input), cfg)
	if err != nil {
		t.Fatalf("LintFile: %v", err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(result.Diagnostics))
	}
	if fixed := string(fix.ApplyEdits([]byte(input), result.Edits)); fixed != "# Title\n\n## Section\n\nText.\n" {
		t.Errorf("fixed = %q", fixed)
	}
}

func TestRequiredHeadingsRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		headings  []any
		matchCase bool
		wantN     int
	}{
		{
			name:     "exact match",
			input:    "# Title\n## Overview\n### Details",
			headings: []any{"# Title", "## Overview", "### Details"},
			wantN:    0,
		},
		{
			name:     "wrong heading",
			input:    "# Title\n## Summary\n### Details",
			headings: []any{"# Title", "## Overview", "### Details"},
			wantN:    1,
		},
		{
			name:     "missing heading",
			input:    "# Title\n## Overview",
			headings: []any{"# Title", "## Overview", "### Details"},
			wantN:    1,
		},
		{
			name:     "wildcard zero or more",
			input:    "# Title\n## Extra\n## More\n## Final",
			headings: []any{"# Title", "*", "## Final"},
			wantN:    0,
		},
		{
			name:     "wildcard one or more",
			input:    "# Title\n## Required\n## Final",
			headings: []any{"# Title", "+", "## Final"},
			wantN:    0,
		},
		{
			name:     "optional single heading",
			input:    "# Project Name\n## Description\n## Examples",
			headings: []any{"?", "## Description", "## Examples"},
			wantN:    0,
		},
		{
			name:      "case sensitive match",
			input:     "# title\n## Overview",
			headings:  []any{"# Title", "## Overview"},
			matchCase: true,
			wantN:     1,
		},
		{
			name:      "case insensitive match",
			input:     "# title\n## Overview",
			headings:  []any{"# Title", "## Overview"},
			matchCase: false,
			wantN:     0,
		},
		{
			name:     "no config no check",
			input:    "# Anything",
			headings: nil,
			wantN:    0,
		},
		{
			name:     "empty config no check",
			input:    "# Anything",
			headings: []any{},
			wantN:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewRequiredHeadingsRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"headings":   tt.headings,
					"match_case": tt.matchCase,
				},
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

func TestRequiredHeadingsRule_DefaultDisabled(t *testing.T) {
	rule := NewRequiredHeadingsRule()
	if rule.DefaultEnabled() {
		t.Error("RequiredHeadingsRule should be disabled by default")
	}
}

func TestProperNamesRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		names   []any
		wantN   int
		wantFix bool
	}{
		{
			name:    "correct capitalization",
			input:   "Use JavaScript for frontend.",
			names:   []any{"JavaScript"},
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "incorrect capitalization",
			input:   "Use javascript for frontend.",
			names:   []any{"JavaScript"},
			wantN:   1,
			wantFix: true,
		},
		{
			name:    "multiple incorrect",
			input:   "Use javascript and github.",
			names:   []any{"JavaScript", "GitHub"},
			wantN:   2,
			wantFix: true,
		},
		{
			name:    "mixed case variants",
			input:   "Visit GitHub for more info about GitHub.",
			names:   []any{"GitHub"},
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "no config no check",
			input:   "Use javascript.",
			names:   nil,
			wantN:   0,
			wantFix: false,
		},
		{
			name:    "partial word not matched",
			input:   "JavaScripting is not a word.",
			names:   []any{"JavaScript"},
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

			rule := NewProperNamesRule()
			cfg := config.NewConfig()
			ruleCfg := &config.RuleConfig{
				Options: map[string]any{
					"names": tt.names,
				},
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

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}

// TestProperNamesRule_MarkdownlintParity verifies MD044 scans the same text
// as markdownlint 0.41: prose, code (code_blocks) and HTML (html_elements),
// but never link destinations, URLs, e-mail addresses or fence info strings,
// and that a longer configured name shields the names inside it.
func TestProperNamesRule_MarkdownlintParity(t *testing.T) {
	github := []any{"GitHub"}
	javascript := []any{"JavaScript"}
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{
			name:      "link destination and bare URL",
			input:     "See [docs](https://github.com/org/repo) and https://github.com/org/repo here.\n",
			options:   map[string]any{"names": github},
			wantLines: []int{},
		},
		{name: "autolink", input: "<https://github.com/x>\n", options: map[string]any{"names": github}, wantLines: []int{}},
		{name: "www address", input: "See www.github.com now.\n", options: map[string]any{"names": github}, wantLines: []int{}},
		{name: "e-mail address", input: "Mail me@github.com now.\n", options: map[string]any{"names": github}, wantLines: []int{}},
		{
			name:      "name after a URL",
			input:     "Visit https://github.com/x and github too.\n",
			options:   map[string]any{"names": github},
			wantLines: []int{1},
			wantFixed: "Visit https://github.com/x and GitHub too.\n",
		},
		{
			name:      "fence info string is not checked",
			input:     "```javascript\nconst javascript = 1;\n```\n",
			options:   map[string]any{"names": javascript},
			wantLines: []int{2},
			wantFixed: "```javascript\nconst JavaScript = 1;\n```\n",
		},
		{
			name:      "code spans skipped without code_blocks",
			input:     "Use `javascript` now.\n",
			options:   map[string]any{"names": javascript, "code_blocks": false},
			wantLines: []int{},
		},
		{
			name:      "URL in a code span",
			input:     "Run `curl https://github.com/x`\n",
			options:   map[string]any{"names": github},
			wantLines: []int{},
		},
		{
			name:      "longer name shields the shorter one",
			input:     "Visit github.com today, and github too.\n",
			options:   map[string]any{"names": []any{"GitHub", "github.com"}},
			wantLines: []int{1},
			wantFixed: "Visit github.com today, and GitHub too.\n",
		},
		{
			name:      "inline HTML skipped without html_elements",
			input:     "An <img src=\"javascript.png\"> image.\n",
			options:   map[string]any{"names": javascript, "html_elements": false},
			wantLines: []int{},
		},
		{
			name:      "text in an HTML block is checked without html_elements",
			input:     "<div>\njavascript text\n</div>\n",
			options:   map[string]any{"names": javascript, "html_elements": false},
			wantLines: []int{2},
			wantFixed: "<div>\nJavaScript text\n</div>\n",
		},
		{
			name:      "HTML attributes are checked with html_elements",
			input:     "<a href=\"https://github.com/x\">github</a>\n",
			options:   map[string]any{"names": github},
			wantLines: []int{1, 1},
			wantFixed: "<a href=\"https://GitHub.com/x\">GitHub</a>\n",
		},
		{
			name:      "HTML attributes are skipped without html_elements",
			input:     "<a href=\"https://github.com/x\">github</a>\n",
			options:   map[string]any{"names": github, "html_elements": false},
			wantLines: []int{1},
			wantFixed: "<a href=\"https://github.com/x\">GitHub</a>\n",
		},
		{
			name:      "HTML comment skipped without html_elements",
			input:     "<!-- github note -->\n",
			options:   map[string]any{"names": github, "html_elements": false},
			wantLines: []int{},
		},
		{
			name:      "HTML comment checked with html_elements",
			input:     "<!-- github note -->\n",
			options:   map[string]any{"names": github},
			wantLines: []int{1},
			wantFixed: "<!-- GitHub note -->\n",
		},
		{
			name:      "image description",
			input:     "![github logo](https://github.com/logo.png)\n",
			options:   map[string]any{"names": github},
			wantLines: []int{1},
			wantFixed: "![GitHub logo](https://github.com/logo.png)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewProperNamesRule(), tt.input, tt.options)
			if got := diagLines(diags); !reflect.DeepEqual(got, tt.wantLines) {
				t.Errorf("diagnostic lines = %v, want %v", got, tt.wantLines)
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			if fixed := applyAllEdits(t, tt.input, diags); fixed != want {
				t.Errorf("fixed = %q, want %q", fixed, want)
			}
		})
	}
}

func TestProperNamesRule_DefaultDisabled(t *testing.T) {
	rule := NewProperNamesRule()
	if rule.DefaultEnabled() {
		t.Error("ProperNamesRule should be disabled by default")
	}
}
