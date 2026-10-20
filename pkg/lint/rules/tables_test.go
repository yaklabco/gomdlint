package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
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

// TestTableBlankLinesRule_ReportsWarnings checks that MD058 builds its
// findings as warnings, the severity that every rule resolves to without
// configuration.
func TestTableBlankLinesRule_ReportsWarnings(t *testing.T) {
	diags := applyRuleWithOptions(t, NewTableBlankLinesRule(), "Text\n| a | b |\n| - | - |\n# Heading\n", nil)

	require.Len(t, diags, 2)
	for _, diag := range diags {
		assert.Equal(t, config.SeverityWarning, diag.Severity, "line %d", diag.StartLine)
	}
}

// TestTableBlankLinesRule_FixKeepsListsTight verifies that MD058 reports a
// table in a tight list without a fix when the blank line it needs would
// separate two items of the list or two blocks of an item, which makes the
// list loose. It keeps the fix where the blank line falls outside every
// list, inside a blockquote within the item, or inside a loose list, which
// stays loose. Only the innermost list that would take the blank line
// between two of its items or blocks decides. The fix check withholds a fix
// that would move the table out of a loose list.
func TestTableBlankLinesRule_FixKeepsListsTight(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Output after the fixes; empty means unchanged.
	}{
		{
			name:      "table after the text of an item",
			input:     "- item\n  | a | b |\n  | --- | --- |\n  | 1 | 2 |\n- next\n",
			wantLines: []int{2, 4},
		},
		{
			name:      "table after the text of an ordered item",
			input:     "1. item\n   | a | b |\n   | --- | --- |\n2. next\n",
			wantLines: []int{2, 3},
		},
		{
			name:      "table that starts a later item",
			input:     "- first\n- | a | b |\n  | --- | --- |\n- next\n",
			wantLines: []int{2, 3},
		},
		{
			name:      "table in a nested list",
			input:     "- outer\n  - item\n    | a | b |\n    | --- | --- |\n  - next\n",
			wantLines: []int{3, 4},
		},
		{
			name:      "list in a blockquote",
			input:     "> - item\n>   | a | b |\n>   | --- | --- |\n> - next\n",
			wantLines: []int{2, 3},
		},
		{
			name:      "table after a heading in an item",
			input:     "- # Title\n  | a | b |\n  | --- | --- |\n- next\n",
			wantLines: []int{2, 3},
		},
		{
			name:      "table after a fenced code block in an item",
			input:     "- item\n  ```sh\n  code\n  ```\n  | a | b |\n  | --- | --- |\n- next\n",
			wantLines: []int{5, 6},
		},
		{
			// A loose list stays loose, so the blank line changes nothing.
			name:      "table after text in a loose list",
			input:     "- item\n\n  text\n  | a | b |\n  | --- | --- |\n\n- next\n",
			wantLines: []int{4},
			wantFixed: "- item\n\n  text\n\n  | a | b |\n  | --- | --- |\n\n- next\n",
		},
		{
			name:      "table that ends an item of a loose list",
			input:     "- item\n\n  | a | b |\n  | --- | --- |\n- next\n",
			wantLines: []int{4},
			wantFixed: "- item\n\n  | a | b |\n  | --- | --- |\n\n- next\n",
		},
		{
			name:      "table that starts a later item of a loose list",
			input:     "- first\n\n- second\n- | a | b |\n  | --- | --- |\n",
			wantLines: []int{4},
			wantFixed: "- first\n\n- second\n\n- | a | b |\n  | --- | --- |\n",
		},
		{
			name:      "loose list in a blockquote",
			input:     "> - item\n>\n>   text\n>   | a | b |\n>   | --- | --- |\n>\n> - next\n",
			wantLines: []int{4},
			wantFixed: "> - item\n>\n>   text\n>\n>   | a | b |\n>   | --- | --- |\n>\n> - next\n",
		},
		{
			// Both blank lines fall inside the tight inner list.
			name:      "tight list in a loose list",
			input:     "- outer\n\n  - inner\n    | a | b |\n    | --- | --- |\n  - next\n\n- outer2\n",
			wantLines: []int{4, 5},
		},
		{
			// Both blank lines fall inside the loose inner list, between
			// blocks of its item and between its items. The tight outer list
			// holds them inside one block of its item, so it stays tight.
			name:      "loose list in a tight list",
			input:     "- outer\n  - inner\n\n    text\n    | a | b |\n    | --- | --- |\n  - next\n- outer2\n",
			wantLines: []int{5, 6},
			wantFixed: "- outer\n  - inner\n\n    text\n\n    | a | b |\n    | --- | --- |\n\n  - next\n- outer2\n",
		},
		{
			// The header row continues the item's paragraph lazily, without
			// the item's indent. After a blank line it would start a table
			// outside the list, so the fix check withholds the fix.
			name:      "lazy table rows in a loose list",
			input:     "- item\n\n  text\n| a | b |\n| --- | --- |\n\n- next\n",
			wantLines: []int{4},
		},
		{
			// An item that starts with an empty line ends at a second blank
			// line, which would move the table out of the list, so the fix
			// check withholds the fix.
			name:      "table after the empty first line of an item in a loose list",
			input:     "- first\n\n-\n  | a | b |\n  | --- | --- |\n",
			wantLines: []int{4},
		},
		{
			// CommonMark takes the blank line into the unclosed fence, so the
			// list is tight, while goldmark gives the line to the next item
			// and reads the list as loose. The fix check reads it as goldmark
			// does, so only the guard keeps this list tight.
			name:      "tight list with an unclosed fence",
			input:     "1. Run:\n   ```sh\n   make\n\n2. Next\n   text\n   | a | b |\n   | --- | --- |\n3. Done\n",
			wantLines: []int{7, 8},
		},
		{
			// A blank line ends the blockquote first, so the list is loose.
			name:      "unclosed fence in a blockquote of a loose list",
			input:     "- a\n  > ```sh\n  > code\n\n- item\n  text\n  | x | y |\n  | --- | --- |\n- c\n",
			wantLines: []int{7, 8},
			wantFixed: "- a\n  > ```sh\n  > code\n\n- item\n  text\n\n  | x | y |\n  | --- | --- |\n\n- c\n",
		},
		{
			// The fix check reads an edit in a long top-level list region by
			// region, and the region checks cannot see a blank line before a
			// table, so only the guard keeps this list tight.
			name:      "long tight list",
			input:     "- a\n- item\n  | x | y |\n  | --- | --- |\n- b\n- c\n",
			wantLines: []int{3, 4},
		},
		{
			name:      "long loose list",
			input:     "- a\n\n- item\n  text\n  | x | y |\n  | --- | --- |\n- b\n- c\n- d\n",
			wantLines: []int{5, 6},
			wantFixed: "- a\n\n- item\n  text\n\n  | x | y |\n  | --- | --- |\n\n- b\n- c\n- d\n",
		},
		{
			name:      "lazy table rows in a long loose list",
			input:     "- a\n\n- item\n  text\n| x | y |\n| --- | --- |\n\n- b\n- c\n- d\n",
			wantLines: []int{5},
		},
		{
			// The inserted lines hold the blockquote marker, so they stay in
			// the blockquote and the item keeps its blocks.
			name:      "table in a blockquote in an item",
			input:     "- item\n  > | a | b |\n  > | --- | --- |\n- next\n",
			wantLines: []int{2, 3},
			wantFixed: "- item\n  >\n  > | a | b |\n  > | --- | --- |\n  >\n- next\n",
		},
		{
			// The blank line after the table falls after the list.
			name:      "table that ends the last item before a heading",
			input:     "- first\n- item\n  | a | b |\n  | --- | --- |\n# Heading\n",
			wantLines: []int{3, 4},
			wantFixed: "- first\n- item\n  | a | b |\n  | --- | --- |\n\n# Heading\n",
		},
		{
			// The blank line before the table falls before the list.
			name:      "table that starts the first item after a paragraph",
			input:     "Text\n- | a | b |\n  | --- | --- |\n",
			wantLines: []int{2},
			wantFixed: "Text\n\n- | a | b |\n  | --- | --- |\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewTableBlankLinesRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

// TestBlankLineLoosensList checks the guard that keeps MD058's fix out of
// tight lists. The fix check cannot always see a list loosened by a blank
// line before a table, so the guard has to. A blank line before or after a
// table loosens the innermost list that would take it between two of its
// items or two blocks of an item, when that list is tight, and no list in
// any other case. A list that goldmark reads as loose counts as tight when
// an item holds an unclosed fence or an HTML block outside a blockquote.
func TestBlankLineLoosensList(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		before, after bool // Whether a blank line before or after the table loosens a list.
	}{
		{name: "tight item", input: "- item\n  | a | b |\n  | --- | --- |\n- next\n", before: true, after: true},
		{name: "loose item", input: "- item\n\n  text\n  | a | b |\n  | --- | --- |\n- next\n"},
		{
			name:   "tight list in a loose list",
			input:  "- outer\n\n  - inner\n    | a | b |\n    | --- | --- |\n  - next\n\n- outer2\n",
			before: true, after: true,
		},
		{name: "loose list in a tight list", input: "- outer\n  - inner\n\n    text\n    | a | b |\n    | --- | --- |\n  - next\n- outer2\n"},
		{name: "blockquote in a tight item", input: "- item\n  > | a | b |\n  > | --- | --- |\n- next\n"},
		{name: "first item after a paragraph", input: "Text\n- | a | b |\n  | --- | --- |\n- next\n", after: true},
		{name: "last item before a heading", input: "- first\n- item\n  | a | b |\n  | --- | --- |\n# Heading\n", before: true},
		// CommonMark takes a blank line into an unclosed fence or HTML block
		// that ends an item, so these lists are tight, but goldmark reads
		// the blank line as one before the next item or block.
		{
			name:   "unclosed fence before the item",
			input:  "1. Run:\n   ```sh\n   make\n\n2. Next\n   text\n   | a | b |\n   | --- | --- |\n3. Done\n",
			before: true, after: true,
		},
		{
			name:   "unclosed comment before the item",
			input:  "- a\n  <!--\n\n- item\n  text\n  | a | b |\n  | --- | --- |\n- c\n",
			before: true, after: true,
		},
		{
			name:   "unclosed fence in a nested item",
			input:  "- outer\n  - inner\n    ```sh\n    code\n\n  text\n  | a | b |\n  | --- | --- |\n- next\n",
			before: true, after: true,
		},
		// A blank line ends a blockquote first, so a fence in it takes in
		// no blank line, and the list is loose.
		{name: "unclosed fence in a blockquote", input: "- a\n  > ```sh\n  > code\n\n- item\n  text\n  | x | y |\n  | --- | --- |\n- c\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)
			tables := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil).Tables()
			require.Len(t, tables, 1)
			pos := tables[0].SourcePosition()

			tightness := listTightness{}
			assert.Equal(t, tt.before, blankLineLoosensList(tables[0], pos.StartLine, tightness), "a blank line before the table")
			assert.Equal(t, tt.after, blankLineLoosensList(tables[0], pos.EndLine+1, tightness), "a blank line after the table")
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

// applyTableRule runs a table rule with its default options on GFM input.
func applyTableRule(t *testing.T, rule lint.Rule, input string) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(string(config.FlavorGFM)).Parse(context.Background(), "test.md", []byte(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	cfg := config.NewConfig()
	cfg.Flavor = config.FlavorGFM
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, cfg, nil))
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
		diagPositions(applyTableRule(t, NewTablePipeStyleRule(), tableRulesSample)), "MD055")
	assert.Equal(t, [][2]int{{6, 33}, {10, 11}, {11, 7}, {28, 9}, {29, 10}},
		diagPositions(applyTableRule(t, NewTableColumnCountRule(), tableRulesSample)), "MD056")

	blank := applyTableRule(t, NewTableBlankLinesRule(), tableRulesSample)
	assert.Equal(t, [][2]int{{11, 3}, {26, 1}}, diagPositions(blank), "MD058")
	fixed := applyDiagFixes(t, tableRulesSample, blank)
	assert.Contains(t, fixed, "> | z |\n>\n> # Quoted heading\n")
	assert.Contains(t, fixed, "Para before\n\n| e | f |\n")

	alignment := applyTableRule(t, NewTableAlignmentRule(), tableRulesSample)
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
			diags := applyTableRule(t, NewTableAlignmentRule(), tt.input)
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
		{
			name:   "unknown style expects both pipes like markdownlint",
			input:  "| A | B |\n| --- | --- |\n| 1 | 2 |",
			style:  "typo",
			flavor: config.FlavorGFM,
			wantN:  0,
		},
		{
			name:   "unknown style reports missing pipes",
			input:  "A | B\n--- | ---\n1 | 2",
			style:  "typo",
			flavor: config.FlavorGFM,
			wantN:  6,
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

// TestTableRulesIgnoreByteOrderMark checks that a byte order mark before a
// table on the first line changes neither the diagnostics nor the fixes of
// the table rules: the parser skips the mark, so rows start after it.
func TestTableRulesIgnoreByteOrderMark(t *testing.T) {
	tests := []struct {
		name  string
		rule  lint.Rule
		input string
		count int
	}{
		{"MD055 pipes on every row", NewTablePipeStyleRule(), "| a | b |\n| - | - |\n| 1 | 2 |\n", 0},
		{"MD055 header without pipes", NewTablePipeStyleRule(), "a | b\n| - | - |\n| 1 | 2 |\n", 4},
		{"MD055 header without a trailing pipe", NewTablePipeStyleRule(), "| a | b\n| - | - |\n| 1 | 2 |\n", 2},
		{"MD056 matching cells", NewTableColumnCountRule(), "| a | b |\n| - | - |\n| 1 | 2 |\n", 0},
		{"MD056 extra cell", NewTableColumnCountRule(), "| a | b |\n| - | - |\n| 1 | 2 | 3 |\n", 1},
		{"MDL003 short delimiter", NewTableAlignmentRule(), "| a | b |\n| - | - |\n| 1 | 2 |\n", 1},
		{"MD058 heading after the table", NewTableBlankLinesRule(), "| a | b |\n| - | - |\n# Next\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := applyRuleWithOptions(t, tt.rule, tt.input, nil)
			require.Len(t, plain, tt.count, "without a byte order mark")
			marked := applyRuleWithOptions(t, tt.rule, mdast.ByteOrderMark+tt.input, nil)
			require.Equal(t, diagnosticLines(plain), diagnosticLines(marked))
			require.Equal(t, mdast.ByteOrderMark+applyAllFixEdits(t, tt.input, plain),
				applyAllFixEdits(t, mdast.ByteOrderMark+tt.input, marked))
		})
	}
}
