package rules

import (
	"context"
	"testing"

	"github.com/jamesainslie/gomdlint/pkg/config"
	"github.com/jamesainslie/gomdlint/pkg/lint"
	"github.com/jamesainslie/gomdlint/pkg/parser/goldmark"
)

func TestTableColumnCountRule(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		flavor config.Flavor
		wantN  int
	}{
		{
			name: "consistent columns",
			input: `| A | B | C |
| --- | --- | --- |
| 1 | 2 | 3 |
| 4 | 5 | 6 |`,
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name: "inconsistent data row",
			input: `| A | B | C |
| --- | --- | --- |
| 1 | 2 |
| 4 | 5 | 6 |`,
			flavor: config.FlavorGFM,
			wantN:  1,
		},
		{
			name: "inconsistent header",
			input: `| A | B |
| --- | --- | --- |
| 1 | 2 | 3 |`,
			flavor: config.FlavorGFM,
			wantN:  1,
		},
		{
			name: "skipped for commonmark",
			input: `| A | B | C |
| --- | --- | --- |
| 1 | 2 |`,
			flavor: config.FlavorCommonMark,
			wantN:  0,
		},
		{
			name:   "no table",
			input:  "Just some text.",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewTableColumnCountRule()
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

func TestTableAlignmentRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		flavor  config.Flavor
		wantN   int
		wantFix bool
	}{
		{
			name: "proper dashes",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			flavor:  config.FlavorGFM,
			wantN:   0,
			wantFix: false,
		},
		{
			name: "too few dashes",
			input: `| A | B |
| - | -- |
| 1 | 2 |`,
			flavor:  config.FlavorGFM,
			wantN:   1,
			wantFix: true,
		},
		{
			name: "with alignment markers",
			input: `| A | B |
| :--- | ---: |
| 1 | 2 |`,
			flavor:  config.FlavorGFM,
			wantN:   0,
			wantFix: false,
		},
		{
			name: "skipped for commonmark",
			input: `| A | B |
| - | - |`,
			flavor:  config.FlavorCommonMark,
			wantN:   0,
			wantFix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewTableAlignmentRule()
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

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}

func TestTableBlankLinesRule(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		flavor  config.Flavor
		wantN   int
		wantFix bool
	}{
		{
			name: "proper blank lines",
			input: `Text before.

| A | B |
| --- | --- |
| 1 | 2 |

Text after.`,
			flavor:  config.FlavorGFM,
			wantN:   0,
			wantFix: false,
		},
		{
			name: "missing blank before",
			input: `Text before.
| A | B |
| --- | --- |
| 1 | 2 |

Text after.`,
			flavor:  config.FlavorGFM,
			wantN:   1,
			wantFix: true,
		},
		{
			name: "missing blank after",
			input: `Text before.

| A | B |
| --- | --- |
| 1 | 2 |
Text after.`,
			flavor:  config.FlavorGFM,
			wantN:   1,
			wantFix: true,
		},
		{
			name: "missing both",
			input: `Text before.
| A | B |
| --- | --- |
| 1 | 2 |
Text after.`,
			flavor:  config.FlavorGFM,
			wantN:   2,
			wantFix: true,
		},
		{
			name: "table at start of file",
			input: `| A | B |
| --- | --- |
| 1 | 2 |

Text after.`,
			flavor:  config.FlavorGFM,
			wantN:   0,
			wantFix: false,
		},
		{
			name: "table at end of file",
			input: `Text before.

| A | B |
| --- | --- |
| 1 | 2 |`,
			flavor:  config.FlavorGFM,
			wantN:   0,
			wantFix: false,
		},
		{
			name: "skipped for commonmark",
			input: `Text before.
| A | B |
| --- | --- |
Text after.`,
			flavor:  config.FlavorCommonMark,
			wantN:   0,
			wantFix: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewTableBlankLinesRule()
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

			if tt.wantFix && len(diags) > 0 && len(diags[0].FixEdits) == 0 {
				t.Error("expected fix edits, got none")
			}
		})
	}
}

func TestTableHelpers(t *testing.T) {
	t.Run("isTableDelimiterRow", func(t *testing.T) {
		tests := []struct {
			input string
			want  bool
		}{
			{"| --- | --- |", true},
			{"| :--- | ---: |", true},
			{"| :---: |", true},
			{"|---|---|", true},
			{"| - | - |", true},
			{"Not a table", false},
			{"| text | text |", false},
			{"", false},
		}

		for _, tt := range tests {
			got := isTableDelimiterRow([]byte(tt.input))
			if got != tt.want {
				t.Errorf("isTableDelimiterRow(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	})

	t.Run("countTableColumns", func(t *testing.T) {
		tests := []struct {
			input string
			want  int
		}{
			{"| A | B | C |", 3},
			{"| A | B |", 2},
			{"|A|B|C|D|", 4},
			{"| Single |", 1},
		}

		for _, tt := range tests {
			got := countTableColumns([]byte(tt.input))
			if got != tt.want {
				t.Errorf("countTableColumns(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	})
}

func TestTablePipeStyleRule(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		style  string
		flavor config.Flavor
		wantN  int
	}{
		{
			name: "consistent leading and trailing",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			style:  "consistent",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name: "inconsistent pipes",
			input: `| A | B |
| --- | ---
  1 | 2 |`,
			style:  "consistent",
			flavor: config.FlavorGFM,
			wantN:  2,
		},
		{
			name: "leading_and_trailing enforced",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			style:  "leading_and_trailing",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name: "leading_only required but has trailing",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			style:  "leading_only",
			flavor: config.FlavorGFM,
			wantN:  3,
		},
		{
			name:   "skipped for commonmark",
			input:  "| A | B |\n| --- | --- |\n  1 | 2 |",
			style:  "consistent",
			flavor: config.FlavorCommonMark,
			wantN:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewTablePipeStyleRule()
			cfg := config.NewConfig()
			cfg.Flavor = tt.flavor
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

func TestTableColumnStyleRule(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		style  string
		flavor config.Flavor
		wantN  int
	}{
		{
			name: "any style allowed",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			style:  "any",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name: "compact style",
			input: `| A | B |
| --- | --- |
| 1 | 2 |`,
			style:  "compact",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name:   "tight style required",
			input:  "|A|B|\n|---|---|\n|1|2|",
			style:  "tight",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name:   "skipped for commonmark",
			input:  "| A | B |\n| --- | --- |",
			style:  "aligned",
			flavor: config.FlavorCommonMark,
			wantN:  0,
		},
		{
			name:   "any style (default)",
			input:  "| A | B |\n| --- | --- |",
			style:  "any",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(tt.flavor))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			rule := NewTableColumnStyleRule()
			cfg := config.NewConfig()
			cfg.Flavor = tt.flavor
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
