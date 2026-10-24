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

func TestHeadingIncrementRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantMsgs  []string
	}{
		{
			name:      "valid increments",
			input:     "# H1\n\n## H2\n\n### H3\n",
			wantDiags: 0,
		},
		{
			name:      "skip level H1 to H3",
			input:     "# H1\n\n### H3\n",
			wantDiags: 1,
			wantMsgs:  []string{"jumped from H1 to H3"},
		},
		{
			name:      "skip level H2 to H4",
			input:     "## H2\n\n#### H4\n",
			wantDiags: 1,
			wantMsgs:  []string{"jumped from H2 to H4"},
		},
		{
			name:      "multiple skips",
			input:     "# H1\n\n### H3\n\n##### H5\n",
			wantDiags: 2,
			wantMsgs:  []string{"jumped from H1 to H3", "jumped from H3 to H5"},
		},
		{
			name:      "decreasing levels allowed",
			input:     "# H1\n\n## H2\n\n# H1 again\n",
			wantDiags: 0,
		},
		{
			name:      "first heading can be any level",
			input:     "### H3\n\n#### H4\n",
			wantDiags: 0,
		},
		{
			name:      "single heading",
			input:     "## H2\n",
			wantDiags: 0,
		},
		{
			name:      "no headings",
			input:     "Just some text\n",
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

			rule := NewHeadingIncrementRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			for i, msg := range tt.wantMsgs {
				if i < len(diags) {
					assert.Contains(t, diags[i].Message, msg)
				}
			}
		})
	}
}

func TestSingleH1Rule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "single H1",
			input:     "# Title\n\n## Section\n",
			wantDiags: 0,
		},
		{
			name:      "multiple H1s",
			input:     "# Title\n\n# Another Title\n",
			wantDiags: 1,
		},
		{
			name:      "three H1s",
			input:     "# One\n\n# Two\n\n# Three\n",
			wantDiags: 2,
		},
		{
			name:      "no H1 allowed by default",
			input:     "## Section\n\n### Subsection\n",
			wantDiags: 0,
		},
		{
			name:      "no H1 not allowed",
			input:     "## Section\n\n### Subsection\n",
			wantDiags: 1,
			config:    map[string]any{"allow_no_h1": false},
		},
		{
			name:      "no headings",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "no headings not allowed",
			input:     "Just text\n",
			wantDiags: 1,
			config:    map[string]any{"allow_no_h1": false},
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

			rule := NewSingleH1Rule()
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

func TestHeadingStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "all ATX style",
			input:     "# H1\n\n## H2\n\n### H3\n",
			wantDiags: 0,
		},
		{
			name:      "ATX with closing hashes consistent",
			input:     "# H1 #\n\n## H2 ##\n",
			wantDiags: 0,
		},
		{
			// The default style is markdownlint's consistent, where ATX and
			// closed ATX are different styles.
			name:      "mixed ATX styles default",
			input:     "# H1\n\n## H2 ##\n",
			wantDiags: 1,
			wantFix:   "# H1\n\n## H2\n",
		},
		{
			name:      "require closing ATX",
			input:     "# H1\n\n## H2\n",
			wantDiags: 2,
			wantFix:   "# H1 #\n\n## H2 ##\n",
			config:    map[string]any{"require_closing_atx": true},
		},
		{
			name:      "consistent style from first",
			input:     "## H2\n\n### H3\n",
			wantDiags: 0,
			config:    map[string]any{"style": "consistent"},
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "no headings",
			input:     "Just text\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewHeadingStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application if expected.
			if tt.wantFix != "" && tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				if len(allEdits) > 0 {
					prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
					require.NoError(t, err)
					fixed := fix.ApplyEdits([]byte(tt.input), prepared)
					assert.Equal(t, tt.wantFix, string(fixed))
				}
			}
		})
	}
}

// applyAllEdits applies the fixes of diags to input, a fix that several
// diagnostics share once.
func applyAllEdits(t *testing.T, input string, diags []lint.Diagnostic) string {
	t.Helper()
	var edits []fix.TextEdit
	// Findings that share a fix carry the same edits, which the engine
	// applies once.
	shared := make(map[string]bool)
	for _, diag := range diags {
		if key := editsKey(diag.FixEdits); !shared[key] {
			shared[key] = true
			edits = append(edits, diag.FixEdits...)
		}
	}
	prepared, err := fix.PrepareEdits(edits, len(input))
	require.NoError(t, err)
	return string(fix.ApplyEdits([]byte(input), prepared))
}

// diagLines returns the start line of each diagnostic.
func diagLines(diags []lint.Diagnostic) []int {
	lines := make([]int, 0, len(diags))
	for _, diag := range diags {
		lines = append(lines, diag.StartLine)
	}
	return lines
}

// TestHeadingIncrementRule_FrontMatterTitle verifies the markdownlint default
// front_matter_title pattern treats a front matter title as the H1.
func TestHeadingIncrementRule_FrontMatterTitle(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantLines []int
	}{
		{
			name:      "default pattern treats title as H1",
			input:     "---\ntitle: Test\n---\n\n### Deep\n",
			wantLines: []int{5},
		},
		{
			name:      "default pattern ignores case",
			input:     "---\nTitle: Test\n---\n\n### Deep\n",
			wantLines: []int{5},
		},
		{
			name:      "quoted JSON title",
			input:     "{\n\"title\": \"Test\"\n}\n\n### Deep\n",
			wantLines: []int{5},
		},
		{
			name:      "title then H2 is fine",
			input:     "---\ntitle: Test\n---\n\n## Section\n",
			wantLines: []int{},
		},
		{
			name:      "no title key",
			input:     "---\nauthor: Someone\n---\n\n### Deep\n",
			wantLines: []int{},
		},
		{
			name:      "empty pattern disables",
			input:     "---\ntitle: Test\n---\n\n### Deep\n",
			options:   map[string]any{"front_matter_title": ""},
			wantLines: []int{},
		},
		{
			name:      "false disables",
			input:     "---\ntitle: Test\n---\n\n### Deep\n",
			options:   map[string]any{"front_matter_title": false},
			wantLines: []int{},
		},
		{
			name:      "custom pattern",
			input:     "---\nname: Test\n---\n\n### Deep\n",
			options:   map[string]any{"front_matter_title": `^\s*name\s*:`},
			wantLines: []int{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHeadingIncrementRule(), tt.input, tt.options)
			assert.Equal(t, tt.wantLines, diagLines(diags))
		})
	}
}

// TestSingleH1Rule_MarkdownlintDefaults verifies markdownlint's MD025
// semantics: only a title (first content H1 or front matter title) makes
// later H1 headings violations.
func TestSingleH1Rule_MarkdownlintDefaults(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantLines []int
		wantMsgs  []string
	}{
		{
			name:      "front matter title counts as the first H1",
			input:     "---\ntitle: Test\n---\n\n# First H1\n\nText.\n\n# Second H1\n",
			wantLines: []int{5, 9},
			wantMsgs:  []string{"this is H1 #2", "this is H1 #3"},
		},
		{
			name:      "front matter title with a single H1",
			input:     "---\nTitle: Test\n---\n\n# Only H1\n",
			wantLines: []int{5},
		},
		{
			name:      "front matter title disabled",
			input:     "---\ntitle: Test\n---\n\n# First\n\n# Second\n",
			options:   map[string]any{"front_matter_title": ""},
			wantLines: []int{7},
		},
		{
			name:      "front matter without title",
			input:     "---\nauthor: Someone\n---\n\n# First\n\n# Second\n",
			wantLines: []int{7},
		},
		{
			name:      "preamble before the first H1 means no title",
			input:     "Intro.\n\n# A\n\n# B\n",
			wantLines: []int{},
		},
		{
			name:      "leading comment does not count as content",
			input:     "<!-- comment -->\n# A\n\n# B\n",
			wantLines: []int{4},
		},
		{
			// goldmark leaves definitions out of the tree, but markdownlint
			// counts one as content, so the first H1 is not a title.
			name:      "leading definition means no title",
			input:     "[a]: /url\n# A\n\n# B\n",
			wantLines: []int{},
		},
		{
			name:      "definition after the first H1",
			input:     "# A\n\n[a]: /url\n\n# B\n",
			wantLines: []int{5},
		},
		{
			name:      "title_only false flags every later H1",
			input:     "Intro.\n\n# A\n\n# B\n",
			options:   map[string]any{"title_only": false},
			wantLines: []int{5},
		},
		{
			name:      "front matter title satisfies allow_no_h1",
			input:     "---\ntitle: Test\n---\n\n## Section\n",
			options:   map[string]any{"allow_no_h1": false},
			wantLines: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewSingleH1Rule(), tt.input, tt.options)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			for i, msg := range tt.wantMsgs {
				if i < len(diags) {
					assert.Contains(t, diags[i].Message, msg)
				}
			}
		})
	}
}

// TestHeadingStyleRule_MarkdownlintStyles verifies MD003 against markdownlint
// 0.41 for every style value, including the default (consistent).
func TestHeadingStyleRule_MarkdownlintStyles(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		options   map[string]any
		wantLines []int
		wantMsgs  []string
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{
			name:      "default consistent flags closed after open",
			input:     "# H1\n\n## H2 ##\n",
			wantLines: []int{3},
			wantMsgs:  []string{"'atx_closed' does not match expected style 'atx'"},
			wantFixed: "# H1\n\n## H2\n",
		},
		{
			name:      "default consistent flags open after closed",
			input:     "# H1 #\n\n## H2\n",
			wantLines: []int{3},
			wantFixed: "# H1 #\n\n## H2 ##\n",
		},
		{
			name:      "atx_closed flags plain ATX",
			input:     "## Plain ATX\n",
			options:   map[string]any{"style": "atx_closed"},
			wantLines: []int{1},
			wantFixed: "## Plain ATX ##\n",
		},
		{
			name:      "atx flags closed ATX",
			input:     "## Closed ##\n",
			options:   map[string]any{"style": "atx"},
			wantLines: []int{1},
			wantFixed: "## Closed\n",
		},
		{
			name:      "atx removes trailing whitespace after closing sequence",
			input:     "## Closed ##  \n",
			options:   map[string]any{"style": "atx"},
			wantLines: []int{1},
			wantFixed: "## Closed\n",
		},
		{
			name:      "setext_with_atx flags ATX H1 without a fix",
			input:     "# H1\n\n### H3\n",
			options:   map[string]any{"style": "setext_with_atx"},
			wantLines: []int{1},
			wantMsgs:  []string{"expected style 'setext'"},
		},
		{
			name:      "setext_with_atx accepts setext H1 and H2 with ATX H3",
			input:     "Title\n=====\n\nSub\n---\n\n### H3\n",
			options:   map[string]any{"style": "setext_with_atx"},
			wantLines: []int{},
		},
		{
			name:      "setext_with_atx expects open ATX for H3",
			input:     "### H3 ###\n",
			options:   map[string]any{"style": "setext_with_atx"},
			wantLines: []int{1},
			wantFixed: "### H3\n",
		},
		{
			name:      "setext_with_atx_closed expects closed ATX for H3",
			input:     "### H3\n",
			options:   map[string]any{"style": "setext_with_atx_closed"},
			wantLines: []int{1},
			wantFixed: "### H3 ###\n",
		},
		{
			name:      "escaped trailing hash is not a closing sequence",
			input:     "# Learn C\\#\n\n## Plain\n",
			wantLines: []int{},
		},
		{
			name:      "closed heading in blockquote sets the style",
			input:     "> ## Quoted ##\n\n## Plain\n",
			wantLines: []int{3},
			wantFixed: "> ## Quoted ##\n\n## Plain ##\n",
		},
		{
			name:      "fix keeps the blockquote prefix",
			input:     "> ## Quoted ##\n",
			options:   map[string]any{"style": "atx"},
			wantLines: []int{1},
			wantFixed: "> ## Quoted\n",
		},
		{
			name:      "setext first heading sets the style",
			input:     "Title\n=====\n\n## Section\n",
			wantLines: []int{4},
			wantMsgs:  []string{"'atx' does not match expected style 'setext'"},
		},
		{
			name:      "setext heading starting with a hash",
			input:     "#hashtag\n===\n\n# ATX\n",
			wantLines: []int{4},
		},
		{
			name:      "setext heading in a list item",
			input:     "- Title\n  ===\n\n- # ATX\n",
			wantLines: []int{4},
		},
		{
			name:      "setext style flags ATX without a fix",
			input:     "Title\n=====\n\n# ATX\n",
			options:   map[string]any{"style": "setext"},
			wantLines: []int{4},
		},
		{
			// Removing the closing "#" would make the literal "#" before it
			// the closing sequence, dropping it from the heading text.
			name:      "no fix when removing the closing sequence exposes another",
			input:     "# Text # #\n",
			options:   map[string]any{"style": "atx"},
			wantLines: []int{1},
		},
		{
			name:      "empty heading sets the style",
			input:     "#\n\n## Closed ##\n",
			wantLines: []int{3},
			wantFixed: "#\n\n## Closed\n",
		},
		{
			name:      "require_closing_atx with the default style",
			input:     "# H1\n\n## H2\n",
			options:   map[string]any{"require_closing_atx": true},
			wantLines: []int{1, 3},
			wantFixed: "# H1 #\n\n## H2 ##\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHeadingStyleRule(), tt.input, tt.options)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			for i, msg := range tt.wantMsgs {
				if i < len(diags) {
					assert.Contains(t, diags[i].Message, msg)
				}
			}
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			fixed := applyAllEdits(t, tt.input, diags)
			assert.Equal(t, want, fixed)

			// Fixes converge: nothing fixable remains after one pass.
			for _, diag := range applyRuleWithOptions(t, NewHeadingStyleRule(), fixed, tt.options) {
				assert.False(t, diag.HasFix(), "fixable diagnostic remains: %s", diag.Message)
			}
		})
	}
}

func TestHeadingIncrementRule_Metadata(t *testing.T) {
	rule := NewHeadingIncrementRule()

	assert.Equal(t, "MD001", rule.ID())
	assert.Equal(t, "heading-increment", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestSingleH1Rule_Metadata(t *testing.T) {
	rule := NewSingleH1Rule()

	assert.Equal(t, "MD025", rule.ID())
	assert.Equal(t, "single-h1", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestHeadingStyleRule_Metadata(t *testing.T) {
	rule := NewHeadingStyleRule()

	assert.Equal(t, "MD003", rule.ID())
	assert.Equal(t, "heading-style", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "style")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestDetectHeadingStyle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  HeadingStyle
	}{
		{
			name:  "ATX style",
			input: "# Heading\n",
			want:  StyleATX,
		},
		{
			name:  "ATX closed style",
			input: "# Heading #\n",
			want:  StyleATXClosed,
		},
		{
			name:  "ATX level 2",
			input: "## Heading\n",
			want:  StyleATX,
		},
		{
			name:  "ATX level 2 closed",
			input: "## Heading ##\n",
			want:  StyleATXClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			headings := lint.Headings(snapshot.Root)
			require.Len(t, headings, 1)

			got := detectHeadingStyle(snapshot, headings[0])
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAllSameChar(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		c    byte
		want bool
	}{
		{"all equals", []byte("==="), '=', true},
		{"all dashes", []byte("---"), '-', true},
		{"mixed", []byte("=-="), '=', false},
		{"empty", []byte(""), '=', false},
		{"single", []byte("="), '=', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allSameChar(tt.b, tt.c)
			assert.Equal(t, tt.want, got)
		})
	}
}
