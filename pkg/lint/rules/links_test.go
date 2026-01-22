package rules

import (
	"context"
	"testing"

	"github.com/yaklabco/gomdlint/pkg/config"
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
			name:  "both spaces",
			input: "[ text ](url)",
			wantN: 1,
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
			name:  "empty text",
			input: "[](url)",
			wantN: 1,
		},
		{
			name:  "both empty",
			input: "[]()",
			wantN: 1,
		},
		{
			name:  "whitespace only text",
			input: "[   ](url)",
			wantN: 1,
		},
		{
			name:  "multiple empty links",
			input: "[text]() and [](url)",
			wantN: 2,
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
