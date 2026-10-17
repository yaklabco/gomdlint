package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
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
			// A header and delimiter row with different cell counts is not a GFM table.
			name: "inconsistent header",
			input: `| A | B |
| --- | --- | --- |
| 1 | 2 | 3 |`,
			flavor: config.FlavorGFM,
			wantN:  0,
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
			// GFM absorbs text directly after a table as a row, so the table
			// ends at that row and a blank line follows it.
			name: "text after the table is a row",
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
			name: "missing blank after",
			input: `Text before.

| A | B |
| --- | --- |
| 1 | 2 |
# Heading`,
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
# Heading`,
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

func TestTableRowCells(t *testing.T) {
	tests := []struct {
		input             string
		leading, trailing bool
		cells             int
	}{
		{"| A | B | C |", true, true, 3},
		{"|A|B|C|D|", true, true, 4},
		{"| Single |", true, true, 1},
		{"A | B", false, false, 2},
		{"| A | B", true, false, 2},
		{"Absorbed text", false, false, 1},
		{"| `a|b` | x\\|y |", true, true, 3},
		{"| a \\|", true, false, 1},
	}
	for _, tt := range tests {
		row := parseTableRowText([]byte(tt.input), 0)
		if row.leading != tt.leading || row.trailing != tt.trailing || row.cellCount() != tt.cells {
			t.Errorf("%q: leading %v trailing %v cells %d, want %v %v %d",
				tt.input, row.leading, row.trailing, row.cellCount(), tt.leading, tt.trailing, tt.cells)
		}
	}
}

// tableRulesSample covers absorbed rows, tables in blockquotes and list
// items, text that is not a table, and a pipe inside a code span.
const tableRulesSample = "# Tables\n\n" +
	"| a | b |\n| - | - |\n| 1 | 2 |\nAbsorbed into the table as a row.\n\n" +
	"> | q | r |\n> |---|---|\n> | x | y | extra |\n> | z |\n> # Quoted heading\n\n" +
	"- item\n\n  | c | d |\n  | - | - |\n  | 3 | 4\n\n" +
	"| a | b | c |\n| - | - |\n\n" +
	"- | -\n\n" +
	"Para before\n| e | f |\n| --- | --- |\n| `a|b` | x\\|y |\nPara after\n\n" +
	"| left | right\n| ---- | -----\n| 5    | 6\n"

// applyTableRule runs a table rule on GFM input with options.
func applyTableRule(t *testing.T, rule lint.Rule, input string, options map[string]any) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(string(config.FlavorGFM)).Parse(context.Background(), "test.md", []byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	var ruleCfg *config.RuleConfig
	if options != nil {
		ruleCfg = &config.RuleConfig{Options: options}
	}
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg))
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	return diags
}

func diagPositions(diags []lint.Diagnostic) [][2]int {
	positions := make([][2]int, 0, len(diags))
	for _, diag := range diags {
		positions = append(positions, [2]int{diag.StartLine, diag.StartColumn})
	}
	return positions
}

func applyDiagFixes(t *testing.T, input string, diags []lint.Diagnostic) string {
	t.Helper()
	var edits []fix.TextEdit
	for _, diag := range diags {
		edits = append(edits, diag.FixEdits...)
	}
	prepared, err := fix.PrepareEdits(edits, len(input))
	if err != nil {
		t.Fatalf("PrepareEdits error: %v", err)
	}
	return string(fix.ApplyEdits([]byte(input), prepared))
}

// TestTableRulesFollowParsedTables checks the positions markdownlint reports
// for the sample: rows the parser absorbed into a table are table rows, and
// tables in blockquotes and list items are checked.
func TestTableRulesFollowParsedTables(t *testing.T) {
	assert.Equal(t, [][2]int{{6, 1}, {6, 33}, {18, 9}, {29, 1}, {29, 10}, {31, 14}, {32, 14}, {33, 10}},
		diagPositions(applyTableRule(t, NewTablePipeStyleRule(), tableRulesSample, nil)), "MD055")
	assert.Equal(t, [][2]int{{6, 33}, {10, 11}, {11, 7}, {28, 9}, {29, 10}},
		diagPositions(applyTableRule(t, NewTableColumnCountRule(), tableRulesSample, nil)), "MD056")

	blank := applyTableRule(t, NewTableBlankLinesRule(), tableRulesSample, nil)
	assert.Equal(t, [][2]int{{11, 3}, {26, 1}}, diagPositions(blank), "MD058")
	fixed := applyDiagFixes(t, tableRulesSample, blank)
	assert.Contains(t, fixed, "> | z |\n>\n> # Quoted heading\n")
	assert.Contains(t, fixed, "Para before\n\n| e | f |\n")

	alignment := applyTableRule(t, NewTableAlignmentRule(), tableRulesSample, nil)
	assert.Equal(t, [][2]int{{4, 1}, {17, 3}}, diagPositions(alignment), "MDL003")
	fixed = applyDiagFixes(t, tableRulesSample, alignment)
	assert.Contains(t, fixed, "| a | b |\n| --- | --- |\n")
	assert.Contains(t, fixed, "  | c | d |\n  | --- | --- |\n")
	assert.Contains(t, fixed, "- | -\n", "a list item is not a delimiter row")
}

// TestTableAlignmentRule_FixKeepsStyle lengthens only the dash runs that are
// too short and keeps alignment colons, padding, and pipes as written.
func TestTableAlignmentRule_FixKeepsStyle(t *testing.T) {
	tests := []struct{ name, input, fixed string }{
		{"compliant cell untouched", "| A | B |\n| :- | ---- |\n", "| A | B |\n| :--- | ---- |\n"},
		{"tight row is not padded", "|A|B|\n|-|---|\n", "|A|B|\n|---|---|\n"},
		{"no outer pipes", "A | B\n-: | :-:\n", "A | B\n---: | :---:\n"},
		{"inside blockquote", "> | A |\n> | - |\n", "> | A |\n> | --- |\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyTableRule(t, NewTableAlignmentRule(), tt.input, nil)
			if len(diags) != 1 {
				t.Fatalf("got %d diagnostics, want 1", len(diags))
			}
			assert.Equal(t, tt.fixed, applyDiagFixes(t, tt.input, diags))
		})
	}
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

// TestTableColumnStyleRule_DefaultMarkdownlintCompatible checks that with no
// options MD060 behaves like markdownlint's style "any": each table must
// follow one of the aligned, compact, or tight styles.
func TestTableColumnStyleRule_DefaultMarkdownlintCompatible(t *testing.T) {
	apply := func(input string) []lint.Diagnostic {
		t.Helper()
		snapshot, err := goldmark.New(string(config.FlavorGFM)).Parse(context.Background(), "test.md", []byte(input))
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		cfg := config.NewConfig()
		cfg.Flavor = config.FlavorGFM
		diags, err := NewTableColumnStyleRule().Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}
		return diags
	}
	if diags := apply("| A | B |\n| --- | --- |\n| x | y |\n"); len(diags) != 0 {
		t.Errorf("compact table: got %d diagnostics, want 0", len(diags))
	}
	if diags := apply("| A  | B |\n| --- | --- |\n|x|y|\n"); len(diags) == 0 {
		t.Error("table mixing styles: got 0 diagnostics, want at least 1")
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
