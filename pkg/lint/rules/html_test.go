package rules

import (
	"context"
	"testing"

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
			wantN:   2, // Opening and closing tags.
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
			wantN:   2, // span opening and closing.
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

func TestInlineHTMLRule_FlavorDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		flavor config.Flavor
		wantN  int
	}{
		{
			name:   "commonmark no defaults",
			input:  "Text<br>here.",
			flavor: config.FlavorCommonMark,
			wantN:  1,
		},
		{
			name:   "gfm br allowed",
			input:  "Text<br>here.",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name:   "gfm sup allowed",
			input:  "Text<sup>a</sup>here.",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name:   "gfm div not allowed",
			input:  "<div>content</div>",
			flavor: config.FlavorGFM,
			wantN:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewInlineHTMLRule()
			cfg := config.NewConfig()
			cfg.Flavor = tt.flavor

			ctx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)
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

func TestInlineHTMLRule_DefaultDisabled(t *testing.T) {
	rule := NewInlineHTMLRule()
	if rule.DefaultEnabled() {
		t.Error("InlineHTMLRule should be disabled by default")
	}
}
