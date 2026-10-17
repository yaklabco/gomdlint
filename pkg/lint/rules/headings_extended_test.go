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

func TestNoMissingSpaceATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct ATX heading",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "missing space level 1",
			input:     "#Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "missing space level 2",
			input:     "##Heading\n",
			wantDiags: 1,
			wantFix:   "## Heading\n",
		},
		{
			name:      "missing space level 3",
			input:     "###Heading\n",
			wantDiags: 1,
			wantFix:   "### Heading\n",
		},
		{
			name:      "multiple headings missing space",
			input:     "#Heading 1\n\n##Heading 2\n",
			wantDiags: 2,
			wantFix:   "# Heading 1\n\n## Heading 2\n",
		},
		{
			name:      "correct heading with text",
			input:     "# Heading with text\n",
			wantDiags: 0,
		},
		{
			name:      "not a heading - hash in text",
			input:     "This is #not a heading\n",
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

			rule := NewNoMissingSpaceATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

// TestNoMissingSpaceATXRule_MarkdownlintParity verifies MD018 matches
// markdownlint 0.41: the hash must start the line, code and HTML blocks are
// skipped, and closed headings are left to MD020.
func TestNoMissingSpaceATXRule_MarkdownlintParity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "closed heading is left to MD020", input: "#Heading#\n", wantLines: []int{}},
		{name: "closing hash with trailing space", input: "#Heading #  \n", wantLines: []int{}},
		{name: "inside an HTML comment", input: "<!--\n#notaheading\n-->\n", wantLines: []int{}},
		{name: "on the closing line of an HTML comment", input: "<!--\nx\n#also -->\n", wantLines: []int{}},
		{name: "inside an HTML block", input: "<div>\n#inside\n</div>\n", wantLines: []int{}},
		{name: "inside a fenced code block", input: "```\n#comment\n```\n", wantLines: []int{}},
		{name: "indented line", input: "  #Indented\n", wantLines: []int{}},
		{name: "list item continuation", input: "- item\n\n  #Heading\n", wantLines: []int{}},
		{name: "blockquote", input: "> #Quoted\n", wantLines: []int{}},
		{name: "keycap emoji", input: "#\uFE0F\u20E3 keycap\n", wantLines: []int{}},
		{name: "seven hashes are never a heading", input: "#######seven\n", wantLines: []int{}},
		{
			name:      "heading after a one-line comment",
			input:     "<!-- c -->\n#Heading\n",
			wantLines: []int{2},
			wantFixed: "<!-- c -->\n# Heading\n",
		},
		{
			name:      "paragraph continuation line",
			input:     "Para\n#tag\n",
			wantLines: []int{2},
			wantFixed: "Para\n# tag\n",
		},
		{
			// markdownlint strips a byte order mark before checking line 1.
			name:      "after a byte order mark",
			input:     "\uFEFF#Heading\n\nText.\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF# Heading\n\nText.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewNoMissingSpaceATXRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

func TestNoMultipleSpaceATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct ATX heading",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "two spaces",
			input:     "#  Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "three spaces",
			input:     "#   Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "level 2 multiple spaces",
			input:     "##  Heading\n",
			wantDiags: 1,
			wantFix:   "## Heading\n",
		},
		{
			name:      "multiple headings with extra spaces",
			input:     "#  Heading 1\n\n##   Heading 2\n",
			wantDiags: 2,
			wantFix:   "# Heading 1\n\n## Heading 2\n",
		},
		{
			name:      "no space - different rule",
			input:     "#Heading\n",
			wantDiags: 0, // MD018 handles this.
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

			rule := NewNoMultipleSpaceATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMissingSpaceClosedATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct closed ATX heading",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "missing space both sides",
			input:     "#Heading#\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "missing space after open",
			input:     "#Heading #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "missing space before close",
			input:     "# Heading#\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "level 2 missing space",
			input:     "##Heading##\n",
			wantDiags: 1,
			wantFix:   "## Heading ##\n",
		},
		{
			name:      "not closed ATX - regular ATX",
			input:     "# Heading\n",
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

			rule := NewNoMissingSpaceClosedATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

func TestNoMultipleSpaceClosedATXRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "correct closed ATX heading",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "multiple spaces both sides",
			input:     "#  Heading  #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "multiple spaces after open",
			input:     "#  Heading #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "multiple spaces before close",
			input:     "# Heading  #\n",
			wantDiags: 1,
			wantFix:   "# Heading #\n",
		},
		{
			name:      "level 2 multiple spaces",
			input:     "##  Heading  ##\n",
			wantDiags: 1,
			wantFix:   "## Heading ##\n",
		},
		{
			name:      "single space - ok",
			input:     "# Heading #\n",
			wantDiags: 0,
		},
		{
			name:      "not closed ATX",
			input:     "#  Heading\n",
			wantDiags: 0, // MD019 handles non-closed.
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

			rule := NewNoMultipleSpaceClosedATXRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

// TestNoMissingSpaceClosedATXRule_MarkdownlintParity verifies MD020 matches
// markdownlint 0.41, including escaped trailing hashes.
func TestNoMissingSpaceClosedATXRule_MarkdownlintParity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "escaped trailing hash is content", input: "## Learn C\\#\n", wantLines: []int{}},
		{
			// markdownlint inserts the space before the escaped hash
			// ("Learn C \# #"); keeping "C\#" intact preserves the text.
			name:      "escaped hash before a closing hash",
			input:     "## Learn C\\##\n",
			wantLines: []int{1},
			wantFixed: "## Learn C\\# #\n",
		},
		{name: "hash inside closed heading content", input: "## C# ##\n", wantLines: []int{}},
		{name: "inside an HTML comment", input: "<!--\n#x#\n-->\n", wantLines: []int{}},
		{name: "inside a fenced code block", input: "```\n#x#\n```\n", wantLines: []int{}},
		{name: "indented line", input: "  #Heading#\n", wantLines: []int{}},
		{name: "blockquote", input: "> #Heading#\n", wantLines: []int{}},
		{
			name:      "missing space before closing hash with trailing whitespace",
			input:     "# Heading#   \n",
			wantLines: []int{1},
			wantFixed: "# Heading #\n",
		},
		{
			name:      "missing space after opening hash",
			input:     "#Heading #\n",
			wantLines: []int{1},
			wantFixed: "# Heading #\n",
		},
		{
			name:      "after a byte order mark",
			input:     "\uFEFF#Closed#\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF# Closed #\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewNoMissingSpaceClosedATXRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

// TestMultipleSpaceATXRules_MarkdownlintParity verifies MD019 and MD021
// match markdownlint 0.41: both check parsed headings (so containers are
// included and HTML is not), and MD019 leaves closed headings to MD021.
func TestMultipleSpaceATXRules_MarkdownlintParity(t *testing.T) {
	tests := []struct {
		name      string
		rule      lint.Rule
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "MD019 skips closed headings", rule: NewNoMultipleSpaceATXRule(), input: "##  Spaced heading  ##\n", wantLines: []int{}},
		{
			name:      "MD021 reports closed headings",
			rule:      NewNoMultipleSpaceClosedATXRule(),
			input:     "##  Spaced heading  ##\n",
			wantLines: []int{1},
			wantFixed: "## Spaced heading ##\n",
		},
		{
			name:      "MD019 in a blockquote",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "> ##  Quoted\n",
			wantLines: []int{1},
			wantFixed: "> ## Quoted\n",
		},
		{
			name:      "MD019 in a list item",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "- ##  Listed\n",
			wantLines: []int{1},
			wantFixed: "- ## Listed\n",
		},
		{
			name:      "MD021 in a blockquote",
			rule:      NewNoMultipleSpaceClosedATXRule(),
			input:     "> #  Quoted  #\n",
			wantLines: []int{1},
			wantFixed: "> # Quoted #\n",
		},
		{name: "MD019 skips HTML comments", rule: NewNoMultipleSpaceATXRule(), input: "<!--\n##  x\n-->\n", wantLines: []int{}},
		{name: "MD021 skips HTML comments", rule: NewNoMultipleSpaceClosedATXRule(), input: "<!--\n##  x  ##\n-->\n", wantLines: []int{}},
		{
			name:      "MD019 keeps an escaped trailing hash",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "##  Learn C\\#\n",
			wantLines: []int{1},
			wantFixed: "## Learn C\\#\n",
		},
		{
			name:      "MD019 with tabs",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "#\t\tTabbed\n",
			wantLines: []int{1},
			wantFixed: "# Tabbed\n",
		},
		{
			name:      "MD021 only right side",
			rule:      NewNoMultipleSpaceClosedATXRule(),
			input:     "# Right   #\n",
			wantLines: []int{1},
			wantFixed: "# Right #\n",
		},
		{name: "MD021 skips escaped hash headings", rule: NewNoMultipleSpaceClosedATXRule(), input: "##  Learn C\\#\n", wantLines: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, tt.rule, tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

func TestNoMissingSpaceATXRule_Metadata(t *testing.T) {
	rule := NewNoMissingSpaceATXRule()

	assert.Equal(t, "MD018", rule.ID())
	assert.Equal(t, "no-missing-space-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx")
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "spaces")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMultipleSpaceATXRule_Metadata(t *testing.T) {
	rule := NewNoMultipleSpaceATXRule()

	assert.Equal(t, "MD019", rule.ID())
	assert.Equal(t, "no-multiple-space-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMissingSpaceClosedATXRule_Metadata(t *testing.T) {
	rule := NewNoMissingSpaceClosedATXRule()

	assert.Equal(t, "MD020", rule.ID())
	assert.Equal(t, "no-missing-space-closed-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx_closed")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoMultipleSpaceClosedATXRule_Metadata(t *testing.T) {
	rule := NewNoMultipleSpaceClosedATXRule()

	assert.Equal(t, "MD021", rule.ID())
	assert.Equal(t, "no-multiple-space-closed-atx", rule.Name())
	assert.Contains(t, rule.Tags(), "atx_closed")
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestHeadingStartLeftRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
	}{
		{
			name:      "heading at start",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "indented heading one space",
			input:     " # Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "indented heading two spaces",
			input:     "  # Heading\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "indented heading with tab",
			input:     "\t# Heading\n",
			wantDiags: 0, // Tab creates a code block in CommonMark, so no heading to detect.
		},
		{
			name:      "multiple indented headings",
			input:     " # H1\n\n  ## H2\n",
			wantDiags: 2,
			wantFix:   "# H1\n\n## H2\n",
		},
		{
			name:      "code block with hash",
			input:     "    # Not a heading\n",
			wantDiags: 0, // Code block.
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

			rule := NewHeadingStartLeftRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

// TestHeadingStartLeftRule_Containers verifies that MD023 measures heading
// indentation from the content column of the heading's containers, like
// markdownlint 0.41.
func TestHeadingStartLeftRule_Containers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "heading at the list item content column", input: "- item\n\n  ## Heading\n", wantLines: []int{}},
		{
			name:      "heading indented past the list item content column",
			input:     "- item\n\n   ## Heading\n",
			wantLines: []int{3},
			wantFixed: "- item\n\n  ## Heading\n",
		},
		{
			name:      "ordered list item",
			input:     "1. item\n\n    ## Heading\n",
			wantLines: []int{3},
			wantFixed: "1. item\n\n   ## Heading\n",
		},
		{name: "heading on the list item line", input: "-   ## Heading\n", wantLines: []int{}},
		{name: "heading in a blockquote", input: "> # Heading\n", wantLines: []int{}},
		{
			name:      "heading indented inside a blockquote",
			input:     ">  # Heading\n",
			wantLines: []int{1},
			wantFixed: "> # Heading\n",
		},
		{
			name:      "indented setext heading",
			input:     "  Title\n  =====\n",
			wantLines: []int{1},
			wantFixed: "Title\n  =====\n",
		},
		{
			name:      "indented heading after a byte order mark",
			input:     "\uFEFF  # Indented\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF# Indented\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewHeadingStartLeftRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, want, applyAllEdits(t, tt.input, diags))
		})
	}
}

func TestNoDuplicateHeadingRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		config    map[string]any
	}{
		{
			name:      "unique headings",
			input:     "# One\n\n## Two\n\n### Three\n",
			wantDiags: 0,
		},
		{
			name:      "duplicate headings",
			input:     "# Same\n\n## Same\n",
			wantDiags: 1,
		},
		{
			name:      "multiple duplicates",
			input:     "# Same\n\n# Same\n\n# Same\n",
			wantDiags: 2,
		},
		{
			name:      "siblings only - different parents allowed",
			input:     "# Version 1\n\n## Features\n\n# Version 2\n\n## Features\n",
			wantDiags: 0,
			config:    map[string]any{"siblings_only": true},
		},
		{
			name:      "siblings only - same parent not allowed",
			input:     "# Title\n\n## Same\n\n## Same\n",
			wantDiags: 1,
			config:    map[string]any{"siblings_only": true},
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

			rule := NewNoDuplicateHeadingRule()
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

func TestNoTrailingPunctuationRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "no trailing punctuation",
			input:     "# Heading\n",
			wantDiags: 0,
		},
		{
			name:      "trailing period",
			input:     "# Heading.\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing comma",
			input:     "# Heading,\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing exclamation",
			input:     "# Heading!\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing colon",
			input:     "# Heading:\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "trailing semicolon",
			input:     "# Heading;\n",
			wantDiags: 1,
			wantFix:   "# Heading\n",
		},
		{
			name:      "question mark allowed by default",
			input:     "# FAQ?\n",
			wantDiags: 0,
		},
		{
			name:      "custom punctuation",
			input:     "# Heading!\n",
			wantDiags: 0,
			config:    map[string]any{"punctuation": "."},
		},
		{
			name:      "empty punctuation disables rule",
			input:     "# Heading.\n",
			wantDiags: 0,
			config:    map[string]any{"punctuation": ""},
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

			rule := NewNoTrailingPunctuationRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			if tt.wantDiags > 0 && tt.wantFix != "" {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))
			}
		})
	}
}

// TestNoTrailingPunctuationRule_RawText verifies MD026 checks the raw
// trailing characters of the heading content like markdownlint 0.41, and
// that its fix deletes only the trailing punctuation.
func TestNoTrailingPunctuationRule_RawText(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
		wantFix   bool
		wantFixed string // Expected output after fixes; empty means unchanged.
	}{
		{name: "code span ends the heading", input: "## Run `make.`\n", wantLines: []int{}},
		{name: "link ends the heading", input: "## See [docs.](https://x.y)\n", wantLines: []int{}},
		{name: "emphasis ends the heading", input: "## Use *this.*\n", wantLines: []int{}},
		{
			name:      "multi-line setext heading",
			input:     "Heading one\nline two.\n-----\n",
			wantLines: []int{2},
			wantFix:   true,
			wantFixed: "Heading one\nline two\n-----\n",
		},
		{
			name:      "run of punctuation",
			input:     "# Heading!!!\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Heading\n",
		},
		{
			name:      "whitespace before punctuation",
			input:     "# Heading .\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Heading\n",
		},
		{name: "HTML entity", input: "# Copyright &copy;\n", wantLines: []int{}},
		{name: "numeric HTML entity", input: "# Copyright &#169;\n", wantLines: []int{}},
		{name: "GitHub emoji code", input: "# Smile :smile:\n", wantLines: []int{}},
		{
			name:      "full-width punctuation by default",
			input:     "# Full width\u3002\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Full width\n",
		},
		{name: "question mark allowed", input: "# FAQ?\n", wantLines: []int{}},
		{
			name:      "closed heading",
			input:     "## Closed. ##\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "## Closed ##\n",
		},
		{
			name:      "heading in a blockquote",
			input:     "> ## Quoted.\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "> ## Quoted\n",
		},
		{
			// Deleting the only character would leave "===" as a paragraph.
			name:      "setext heading of only punctuation is not fixed",
			input:     ".\n===\n",
			wantLines: []int{1},
		},
		{
			// Deleting the line's only character would split the heading.
			name:      "setext line of only punctuation is not fixed",
			input:     "Heading\n.\n---\n",
			wantLines: []int{2},
		},
		{
			// "- \n  ---" would turn the heading into a horizontal rule.
			name:      "setext heading of only punctuation in a list item is not fixed",
			input:     "- .\n  ---\n",
			wantLines: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewNoTrailingPunctuationRule(), tt.input, nil)
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

func TestHeadingStartLeftRule_Metadata(t *testing.T) {
	rule := NewHeadingStartLeftRule()

	assert.Equal(t, "MD023", rule.ID())
	assert.Equal(t, "heading-start-left", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.Contains(t, rule.Tags(), "spaces")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoDuplicateHeadingRule_Metadata(t *testing.T) {
	rule := NewNoDuplicateHeadingRule()

	assert.Equal(t, "MD024", rule.ID())
	assert.Equal(t, "no-duplicate-heading", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

func TestNoTrailingPunctuationRule_Metadata(t *testing.T) {
	rule := NewNoTrailingPunctuationRule()

	assert.Equal(t, "MD026", rule.ID())
	assert.Equal(t, "no-trailing-punctuation", rule.Name())
	assert.Contains(t, rule.Tags(), "headings")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}
