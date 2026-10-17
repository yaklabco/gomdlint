package rules

import (
	"context"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestBlanksAroundFencesRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "missing blank before",
			input:     "Some text\n```\ncode\n```\n",
			wantDiags: 1,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no code block",
			input:     "Just text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewBlanksAroundFencesRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoSpaceInCodeRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "no spaces",
			input:     "`code`\n",
			wantDiags: 0,
		},
		{
			name:      "single space padding allowed",
			input:     "` code `\n",
			wantDiags: 0,
		},
		{
			name:      "excessive leading spaces",
			input:     "`  code`\n",
			wantDiags: 1,
		},
		{
			name:      "excessive trailing spaces",
			input:     "`code  `\n",
			wantDiags: 1,
		},
		{
			// markdownlint reports each side separately.
			name:      "excessive both sides",
			input:     "`  code  `\n",
			wantDiags: 2,
		},
		{
			name:      "backtick content with padding",
			input:     "`` `code` ``\n",
			wantDiags: 0,
		},
		{
			name:      "spaces only allowed",
			input:     "`   `\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInCodeRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestBlanksAroundFencesRule_Metadata(t *testing.T) {
	rule := NewBlanksAroundFencesRule()

	assert.Equal(t, "MD031", rule.ID())
	assert.Equal(t, "blanks-around-fences", rule.Name())
	assert.Contains(t, rule.Tags(), "code")
	assert.True(t, rule.CanFix())
}

func TestNoSpaceInCodeRule_Metadata(t *testing.T) {
	rule := NewNoSpaceInCodeRule()

	assert.Equal(t, "MD038", rule.ID())
	assert.Equal(t, "no-space-in-code", rule.Name())
	assert.Contains(t, rule.Tags(), "code")
	assert.True(t, rule.CanFix())
}

func TestBlanksAroundFencesRule_Fix(t *testing.T) {
	// The BlanksAroundFencesRule checks for blank lines around fenced code blocks.
	// The parser's SourcePosition for code blocks points to the CODE CONTENT,
	// so the rule adjusts to find the actual fence lines (one line before/after content).
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input with blank lines",
			input:     "Some text\n\n```\ncode\n```\n\nMore text\n",
			wantDiags: 0,
			wantFix:   "Some text\n\n```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "missing blank before code block",
			input:     "Some text\n```\ncode\n```\n",
			wantDiags: 1,
			wantFix:   "Some text\n\n```\ncode\n```\n",
		},
		{
			name:      "missing blank after code block",
			input:     "```\ncode\n```\nMore text\n",
			wantDiags: 1,
			wantFix:   "```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "missing both blank lines",
			input:     "Some text\n```\ncode\n```\nMore text\n",
			wantDiags: 2,
			wantFix:   "Some text\n\n```\ncode\n```\n\nMore text\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "code block at start of file",
			input:     "```\ncode\n```\n\nText\n",
			wantDiags: 0,
			wantFix:   "```\ncode\n```\n\nText\n",
		},
		{
			name:      "code block at end of file",
			input:     "Text\n\n```\ncode\n```\n",
			wantDiags: 0,
			wantFix:   "Text\n\n```\ncode\n```\n",
		},
		{
			name:      "multiple code blocks missing blanks",
			input:     "Text\n```\ncode1\n```\nText\n```\ncode2\n```\nEnd\n",
			wantDiags: 4,
			wantFix:   "Text\n\n```\ncode1\n```\n\nText\n\n```\ncode2\n```\n\nEnd\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewBlanksAroundFencesRule()
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

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}

// TestBlanksAroundFencesRule_Containers follows markdownlint MD031: a line
// holding only blockquote markers, whitespace, or HTML comments is blank, and
// the fix inserts a blank line that keeps the enclosing blockquotes whole.
func TestBlanksAroundFencesRule_Containers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		lines []int
		fixed string
	}{
		{
			name:  "blockquote",
			input: "> Text\n> ```\n> code\n> ```\n> More\n",
			lines: []int{2, 4},
			fixed: "> Text\n>\n> ```\n> code\n> ```\n>\n> More\n",
		},
		{
			name:  "blockquote marker lines are blank",
			input: "> ## Quoted\n>\n> ```\n> code\n> ```\n>\n> Para\n",
		},
		{
			name:  "list item",
			input: "- item\n  ```\n  code\n  ```\n- next\n",
			lines: []int{2, 4},
			fixed: "- item\n\n  ```\n  code\n  ```\n\n- next\n",
		},
		{
			name:  "comment line is blank",
			input: "<!-- comment -->\n```\nafter comment\n```\n",
		},
		{
			name:  "nested blockquotes",
			input: "> > text\n> > ```\n> > x\n> > ```\n> > tail\n",
			lines: []int{2, 4},
			fixed: "> > text\n> >\n> > ```\n> > x\n> > ```\n> >\n> > tail\n",
		},
		{
			name:  "empty fenced block",
			input: "Para before\n```\n```\nPara after\n",
			lines: []int{2, 3},
			fixed: "Para before\n\n```\n```\n\nPara after\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)
			diags, err := NewBlanksAroundFencesRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			require.NoError(t, err)
			require.Len(t, diags, len(tt.lines), "diagnostics: %+v", diags)
			var edits []fix.TextEdit
			for idx, line := range tt.lines {
				assert.Equal(t, line, diags[idx].StartLine, "diagnostic %d line", idx)
				edits = append(edits, diags[idx].FixEdits...)
			}
			if len(tt.lines) == 0 {
				return
			}
			prepared, err := fix.PrepareEdits(edits, len(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.fixed, string(fix.ApplyEdits([]byte(tt.input), prepared)))
		})
	}
}

// TestNoSpaceInCodeRule_ReportsEachSide reports leading and trailing spaces
// separately at the spaces that the fix deletes, like markdownlint.
func TestNoSpaceInCodeRule_ReportsEachSide(t *testing.T) {
	snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte("A `  code  ` b\n"))
	require.NoError(t, err)
	diags, err := NewNoSpaceInCodeRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
	require.NoError(t, err)
	require.Len(t, diags, 2)
	assert.Equal(t, [2]int{1, 4}, [2]int{diags[0].StartLine, diags[0].StartColumn})
	assert.Equal(t, "Excessive leading space inside code span", diags[0].Message)
	assert.Equal(t, [2]int{1, 10}, [2]int{diags[1].StartLine, diags[1].StartColumn})
	assert.Equal(t, "Excessive trailing space inside code span", diags[1].Message)
}

// TestNoSpaceInCodeRule_MarkdownlintSemantics follows markdownlint MD038: a
// single space of padding is allowed only on both sides, a space that
// separates a backtick in the content from the delimiter is allowed, and
// escaped backticks are not code.
func TestNoSpaceInCodeRule_MarkdownlintSemantics(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
		fixed string
	}{
		{"extra space after backtick content", "A `` `x`  `` b\n", 1, "A `` `x` `` b\n"},
		{"escaped backtick is not code", "A \\`  not code  ` b\n", 0, ""},
		{"single leading space", "A ` x` b\n", 1, "A `x` b\n"},
		{"single trailing space", "A `y ` b\n", 1, "A `y` b\n"},
		{"backtick inside padded content", "A ``  b`c  `` b\n", 2, "A ``b`c`` b\n"},
		{"space separating a backtick", "A `` `a `` b\n", 0, ""},
		{"symmetric padding", "A ` code ` b\n", 0, ""},
		{"only spaces", "A `   ` b\n", 0, ""},
		{"trailing space on a later line", "A `foo\nbar ` b\n", 1, "A `foo\nbar` b\n"},
		{"blockquote", "> A `  quoted` b\n", 1, "> A `quoted` b\n"},
		{"no-break space kept before a backtick", "A `` \u00a0`foo`` b\n", 1, "A ``\u00a0`foo`` b\n"},
		{"no-break space kept after a backtick", "A ``foo`\u00a0 `` b\n", 1, "A ``foo`\u00a0`` b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot, err := goldmark.New(string(config.FlavorCommonMark)).Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)
			diags, err := NewNoSpaceInCodeRule().Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil))
			require.NoError(t, err)
			require.Len(t, diags, tt.want, "diagnostics: %+v", diags)
			if tt.want == 0 {
				return
			}
			var edits []fix.TextEdit
			for _, diag := range diags {
				edits = append(edits, diag.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(edits, len(tt.input))
			require.NoError(t, err)
			fixed := string(fix.ApplyEdits([]byte(tt.input), prepared))
			assert.Equal(t, tt.fixed, fixed)
			assert.True(t, utf8.ValidString(fixed), "the fix must delete whole characters")
		})
	}
}

func TestNoSpaceInCodeRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "clean input - no spaces",
			input:     "`code`\n",
			wantDiags: 0,
			wantFix:   "`code`\n",
		},
		{
			name:      "single space padding allowed",
			input:     "` code `\n",
			wantDiags: 0,
			wantFix:   "` code `\n",
		},
		{
			name:      "excessive leading spaces",
			input:     "`  code`\n",
			wantDiags: 1,
			wantFix:   "`code`\n",
		},
		{
			name:      "excessive trailing spaces",
			input:     "`code  `\n",
			wantDiags: 1,
			wantFix:   "`code`\n",
		},
		{
			name:      "excessive both sides",
			input:     "`  code  `\n",
			wantDiags: 2,
			wantFix:   "`code`\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "backtick content with padding allowed",
			input:     "`` `code` ``\n",
			wantDiags: 0,
			wantFix:   "`` `code` ``\n",
		},
		{
			name:      "spaces only allowed",
			input:     "`   `\n",
			wantDiags: 0,
			wantFix:   "`   `\n",
		},
		{
			name:      "multiple code spans with violations",
			input:     "Use `  cmd1  ` and `  cmd2  ` here.\n",
			wantDiags: 4,
			wantFix:   "Use `cmd1` and `cmd2` here.\n",
		},
		{
			name:      "mixed valid and invalid spans",
			input:     "Valid `code` and invalid `  spaces  ` here.\n",
			wantDiags: 2,
			wantFix:   "Valid `code` and invalid `spaces` here.\n",
		},
		{
			name:      "double backticks with excessive spaces",
			input:     "``  code  ``\n",
			wantDiags: 2,
			wantFix:   "``code``\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInCodeRule()
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

			// Verify idempotency - re-running on fixed content should produce no diagnostics
			if tt.wantDiags > 0 {
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, diags2, "fix should be idempotent")
			}
		})
	}
}
