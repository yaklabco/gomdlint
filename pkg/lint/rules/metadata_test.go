package rules

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
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
			name:       "consecutive headings allowed",
			input:      "# H1\n## H2\n\nText",
			linesAbove: 1,
			linesBelow: 1,
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

func TestProperNamesRule_DefaultDisabled(t *testing.T) {
	rule := NewProperNamesRule()
	if rule.DefaultEnabled() {
		t.Error("ProperNamesRule should be disabled by default")
	}
}
