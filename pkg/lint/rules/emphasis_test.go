package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func TestNoEmphasisAsHeadingRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "normal emphasis in text",
			input:     "Some *emphasized* text here.\n",
			wantDiags: 0,
		},
		{
			name:      "normal heading",
			input:     "# Heading\n\nSome text.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "emphasis ending with punctuation",
			input:     "**Question?**\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoEmphasisAsHeadingRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

// applyGFMRule parses input as GFM and applies rule with options.
func applyGFMRule(t *testing.T, rule lint.Rule, input string, options map[string]any) []lint.Diagnostic {
	t.Helper()
	snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	var ruleCfg *config.RuleConfig
	if options != nil {
		ruleCfg = &config.RuleConfig{Options: options}
	}
	diags, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), ruleCfg))
	require.NoError(t, err)
	return diags
}

// TestNoEmphasisAsHeadingRule_MarkdownlintParity verifies MD036 matches
// markdownlint 0.41 detection and that its fix (a gomdlint extension) is
// built from the source line and withheld whenever it could change meaning.
func TestNoEmphasisAsHeadingRule_MarkdownlintParity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFix   bool
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "inline code and link are not plain text", input: "**Use `foo` with [docs](https://x.y) now**\n", wantLines: []int{}},
		{
			name:      "plain bold paragraph",
			input:     "**Section Title**\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "## Section Title\n",
		},
		{name: "italic paragraph is reported without a fix", input: "_Another section_\n", wantLines: []int{1}},
		{name: "blockquote paragraph", input: "> **Quoted Title**\n", wantLines: []int{}},
		{name: "list item paragraph", input: "- **Listed Title**\n", wantLines: []int{}},
		{name: "multi-line emphasis", input: "**Two\nlines**\n", wantLines: []int{}},
		{name: "strikethrough", input: "~~Struck paragraph~~\n", wantLines: []int{}},
		{name: "character reference", input: "**Price &amp; Value**\n", wantLines: []int{}},
		{name: "backslash escape", input: "**Price \\* Value**\n", wantLines: []int{}},
		{name: "nested emphasis", input: "***Both***\n", wantLines: []int{}},
		{name: "full-width question mark", input: "**Title？**\n", wantLines: []int{}},
		{
			name:      "trailing comment is ignored but blocks the fix",
			input:     "**Title** <!-- note -->\n",
			wantLines: []int{1},
		},
		{
			name:      "indented paragraph",
			input:     "   **Indented**\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "## Indented\n",
		},
		{
			// "## Item #" would read the "#" as a closing sequence.
			name:      "trailing hash blocks the fix",
			input:     "**Item #**\n",
			wantLines: []int{1},
		},
		{
			name:      "level follows the previous heading",
			input:     "# Title\n\n## Part\n\n__Detail__\n",
			wantLines: []int{5},
			wantFix:   true,
			wantFixed: "# Title\n\n## Part\n\n### Detail\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyGFMRule(t, NewNoEmphasisAsHeadingRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			for _, diag := range diags {
				assert.Equal(t, tt.wantFix, diag.HasFix())
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

// TestNoSpaceInEmphasisRule_DelimiterRuns verifies MD037 pairs unmatched
// delimiter runs like markdownlint 0.41 and reports one diagnostic per pair.
func TestNoSpaceInEmphasisRule_DelimiterRuns(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{
			// markdownlint reports this; gomdlint treats markers around
			// content without letters as arithmetic.
			name:      "arithmetic",
			input:     "See 2 * 3 * 4\n",
			wantLines: []int{},
		},
		{
			name:      "space only after the opening marker",
			input:     "One-sided ** bold** here\n",
			wantLines: []int{1},
			wantFixed: "One-sided **bold** here\n",
		},
		{
			name:      "space only before the closing marker",
			input:     "And **bold ** too\n",
			wantLines: []int{1},
			wantFixed: "And **bold** too\n",
		},
		{
			name:      "pair across lines",
			input:     "Some ** text\nmore ** here\n",
			wantLines: []int{1},
			wantFixed: "Some **text\nmore** here\n",
		},
		{name: "escaped markers", input: "Some \\* text \\* here\n", wantLines: []int{}},
		{name: "single marker", input: "x * y\n", wantLines: []int{}},
		{name: "runs longer than three", input: "**** x ****\n", wantLines: []int{}},
		{name: "intraword underscores", input: "snake_case_name and other_var_x\n", wantLines: []int{}},
		{name: "markers inside strong text", input: "**bold * x * here**\n", wantLines: []int{}},
		{name: "markers inside link text", input: "[a * x * b](https://example.com)\n", wantLines: []int{}},
		{
			name:      "letters between markers",
			input:     "a * b * c\n",
			wantLines: []int{1},
			wantFixed: "a *b* c\n",
		},
		{
			name:      "heading",
			input:     "# Title * x *\n",
			wantLines: []int{1},
			wantFixed: "# Title *x*\n",
		},
		{
			name:      "tight list item",
			input:     "- a * x * b\n",
			wantLines: []int{1},
			wantFixed: "- a *x* b\n",
		},
		{
			name:      "blockquote",
			input:     "> a * x * b\n",
			wantLines: []int{1},
			wantFixed: "> a *x* b\n",
		},
		{
			name:      "after inline HTML",
			input:     "a <b>x</b> * y * z\n",
			wantLines: []int{1},
			wantFixed: "a <b>x</b> *y* z\n",
		},
		{
			name:      "table cell",
			input:     "| A | B |\n| - | - |\n| a * x * b | c |\n",
			wantLines: []int{3},
			wantFixed: "| A | B |\n| - | - |\n| a *x* b | c |\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyGFMRule(t, NewNoSpaceInEmphasisRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

func TestNoSpaceInEmphasisRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "correct emphasis",
			input:     "Some *emphasized* text.\n",
			wantDiags: 0,
		},
		{
			name:      "spaces in emphasis",
			input:     "Some * spaced * text.\n",
			wantDiags: 1,
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

			rule := NewNoSpaceInEmphasisRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestEmphasisStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single emphasis",
			input:     "Some *text* here.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no emphasis",
			input:     "Plain text.\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewEmphasisStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestStrongStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single strong",
			input:     "Some **text** here.\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no strong",
			input:     "Plain text.\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewStrongStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoEmphasisAsHeadingRule_Metadata(t *testing.T) {
	rule := NewNoEmphasisAsHeadingRule()

	assert.Equal(t, "MD036", rule.ID())
	assert.Equal(t, "no-emphasis-as-heading", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix()) // Now auto-fixable for bold-only paragraphs
}

func TestNoSpaceInEmphasisRule_Metadata(t *testing.T) {
	rule := NewNoSpaceInEmphasisRule()

	assert.Equal(t, "MD037", rule.ID())
	assert.Equal(t, "no-space-in-emphasis", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestEmphasisStyleRule_Metadata(t *testing.T) {
	rule := NewEmphasisStyleRule()

	assert.Equal(t, "MD049", rule.ID())
	assert.Equal(t, "emphasis-style", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestStrongStyleRule_Metadata(t *testing.T) {
	rule := NewStrongStyleRule()

	assert.Equal(t, "MD050", rule.ID())
	assert.Equal(t, "strong-style", rule.Name())
	assert.Contains(t, rule.Tags(), "emphasis")
	assert.True(t, rule.CanFix())
}

func TestNoSpaceInEmphasisRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			// Matched emphasis on the same line does not pair with the
			// unmatched markers.
			name:      "valid emphasis before spaced markers",
			input:     "Some *valid* and * invalid * here.\n",
			wantDiags: 1,
			wantFix:   "Some *valid* and *invalid* here.\n",
		},
		{
			name:      "clean input - no spaces in emphasis",
			input:     "Some *text* here.\n",
			wantDiags: 0,
			wantFix:   "Some *text* here.\n",
		},
		{
			name:      "single violation - spaces in asterisk emphasis",
			input:     "Some * text * here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* here.\n",
		},
		{
			name:      "strong emphasis with spaces",
			input:     "Some ** text ** here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** here.\n",
		},
		{
			name:      "underscore emphasis with spaces",
			input:     "Some _ text _ here.\n",
			wantDiags: 1,
			wantFix:   "Some _text_ here.\n",
		},
		{
			name:      "strong underscore emphasis with spaces",
			input:     "Some __ text __ here.\n",
			wantDiags: 1,
			wantFix:   "Some __text__ here.\n",
		},
		{
			name:      "multiple lines with violations",
			input:     "First * line * text.\n\nSecond * para * here.\n",
			wantDiags: 2,
			wantFix:   "First *line* text.\n\nSecond *para* here.\n",
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			// "* " at the start of a line is a list item, not emphasis;
			// rewriting it to "*spaced*" would turn the list into a paragraph.
			name:      "list marker at start of line",
			input:     "* spaced * text.\n",
			wantDiags: 0,
			wantFix:   "* spaced * text.\n",
		},
		{
			name:      "violation at end of line",
			input:     "text * spaced *\n",
			wantDiags: 1,
			wantFix:   "text *spaced*\n",
		},
		{
			name:      "no text content",
			input:     "Just plain text.\n",
			wantDiags: 0,
			wantFix:   "Just plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoSpaceInEmphasisRule()
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
			snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
			require.NoError(t, err)
			ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, nil)
			diags2, err := rule.Apply(ruleCtx2)
			require.NoError(t, err)
			assert.Empty(t, diags2, "fix should be idempotent")
		})
	}
}

func TestEmphasisStyleRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - consistent asterisk style",
			input:     "Some *text* and *more* here.\n",
			wantDiags: 0,
			wantFix:   "Some *text* and *more* here.\n",
		},
		{
			name:      "clean input - consistent underscore style",
			input:     "Some _text_ and _more_ here.\n",
			wantDiags: 0,
			wantFix:   "Some _text_ and _more_ here.\n",
		},
		{
			name:      "style mismatch - consistent mode",
			input:     "Some *text* and _more_ here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* and _more_ here.\n",
		},
		{
			name:      "explicit style config - asterisk required",
			input:     "Some _text_ here.\n",
			wantDiags: 1,
			wantFix:   "Some _text_ here.\n",
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "explicit style config - underscore required",
			input:     "Some *text* here.\n",
			wantDiags: 1,
			wantFix:   "Some *text* here.\n",
			config:    map[string]any{"style": "underscore"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "no emphasis",
			input:     "Plain text.\n",
			wantDiags: 0,
			wantFix:   "Plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewEmphasisStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes (will be empty since buildStyleFix returns nil)
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))
		})
	}
}

func TestStrongStyleRule_Fix(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "clean input - consistent asterisk style",
			input:     "Some **text** and **more** here.\n",
			wantDiags: 0,
			wantFix:   "Some **text** and **more** here.\n",
		},
		{
			name:      "clean input - consistent underscore style",
			input:     "Some __text__ and __more__ here.\n",
			wantDiags: 0,
			wantFix:   "Some __text__ and __more__ here.\n",
		},
		{
			name:      "style mismatch - consistent mode",
			input:     "Some **text** and __more__ here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** and __more__ here.\n",
		},
		{
			name:      "explicit style config - asterisk required",
			input:     "Some __text__ here.\n",
			wantDiags: 1,
			wantFix:   "Some __text__ here.\n",
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "explicit style config - underscore required",
			input:     "Some **text** here.\n",
			wantDiags: 1,
			wantFix:   "Some **text** here.\n",
			config:    map[string]any{"style": "underscore"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
			wantFix:   "",
		},
		{
			name:      "no strong",
			input:     "Plain text.\n",
			wantDiags: 0,
			wantFix:   "Plain text.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewStrongStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)
			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Collect and apply fixes (will be empty since buildStyleFix returns nil)
			var allEdits []fix.TextEdit
			for _, d := range diags {
				allEdits = append(allEdits, d.FixEdits...)
			}
			prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
			require.NoError(t, err)
			fixed := fix.ApplyEdits([]byte(tt.input), prepared)
			assert.Equal(t, tt.wantFix, string(fixed))
		})
	}
}
