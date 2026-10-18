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
			// Reported; the fix would add a heading anchor, which raw HTML
			// might refer to.
			name:      "heading after a one-line comment",
			input:     "<!-- c -->\n#Heading\n",
			wantLines: []int{2},
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
			// ("Learn C \# #"). A fix that kept "C\#" whole would still change
			// the heading text, and the escaped hash is a hash outside the
			// heading markers, so no fix is offered.
			name:      "escaped hash before a closing hash",
			input:     "## Learn C\\##\n",
			wantLines: []int{1},
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
		{
			name:      "MD019 empty heading",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "##  \n",
			wantLines: []int{1},
			wantFixed: "## \n",
		},
		{
			name:      "MD021 empty closed heading",
			rule:      NewNoMultipleSpaceClosedATXRule(),
			input:     "##  ##\n",
			wantLines: []int{1},
			wantFixed: "## ##\n",
		},
		{name: "MD019 skips an empty closed heading", rule: NewNoMultipleSpaceATXRule(), input: "##  ##\n", wantLines: []int{}},
		{
			name:      "MD019 after a byte order mark",
			rule:      NewNoMultipleSpaceATXRule(),
			input:     "\uFEFF##  Heading\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF## Heading\n",
		},
		{
			name:      "MD021 after a byte order mark",
			rule:      NewNoMultipleSpaceClosedATXRule(),
			input:     "\uFEFF#  Closed  #\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF# Closed #\n",
		},
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
		{
			name:      "indented quoted heading after a byte order mark",
			input:     "\uFEFF>  # Quoted\n",
			wantLines: []int{1},
			wantFixed: "\uFEFF> # Quoted\n",
		},
		{
			// The list item claims two of the tab's four columns; deleting
			// the tab would move the heading out of the list.
			name:      "heading after a tab the list item claims in part",
			input:     "- item\n\n\t## Heading\n",
			wantLines: []int{3},
			wantFixed: "- item\n\n  ## Heading\n",
		},
		{
			name:      "quoted heading after a tab the blockquote claims in part",
			input:     ">\t # Heading\n",
			wantLines: []int{1},
			wantFixed: "> # Heading\n",
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
		{
			// "***" would be a thematic break and end the heading.
			name:      "setext line that would become a thematic break",
			input:     "Heading\n***.\n---\n",
			wantLines: []int{2},
		},
		{
			// "---" would become the underline of "Text" alone.
			name:      "setext line that would become an underline",
			input:     "Text\n---.\n===\n",
			wantLines: []int{2},
		},
		{
			// "# Foo #" would read the hash as a closing sequence.
			name:      "deletion that would expose a closing sequence",
			input:     "# Foo #.\n",
			wantLines: []int{1},
		},
		{
			name:      "period after a character reference",
			input:     "# Foo &copy;.\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Foo &copy;\n",
		},
		{
			// The anchor would change from "foo-" to "foo", breaking the link.
			name:      "anchor of a fragment link",
			input:     "# Foo !\n\n[jump](#foo-)\n",
			wantLines: []int{1},
		},
		{
			// A fragment written with a character reference still points to
			// "foo-", so the anchor must stay.
			name:      "fragment link written with a character reference",
			input:     "# Foo !\n\n[jump](#fo&#111;-)\n",
			wantLines: []int{1},
		},
		{
			name:      "raw HTML link written with a character reference",
			input:     "# Foo !\n\n<a href=\"&#35;foo-\">jump</a>\n",
			wantLines: []int{1},
		},
		{
			// GitHub lowercases U+0130 to "i" and U+0307, unlike Go.
			name:      "anchor of a dotted capital I",
			input:     "# \u0130 !\n\n[jump](#i%CC%87-)\n",
			wantLines: []int{1},
		},
		{
			// GitHub lowercases a final capital sigma to a final sigma.
			name:      "anchor that ends in a capital sigma",
			input:     "# \u039f\u0394\u039f\u03a3 !\n\n[jump](#\u03bf\u03b4\u03bf\u03c2-)\n",
			wantLines: []int{1},
		},
		{
			name:      "anchor change in a document without fragment links",
			input:     "# Foo !\n\n[jump](https://example.com/)\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Foo\n\n[jump](https://example.com/)\n",
		},
		{
			// The link holds a hash, so no fix may change a heading's text,
			// whatever its anchor.
			name:      "heading text in a document with a fragment link",
			input:     "# Foo.\n\n[jump](#foo)\n",
			wantLines: []int{1},
		},
		{
			// Deleting only the "!" would leave the backslash showing.
			name:      "escaped punctuation",
			input:     "# Text\\!\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Text\n",
		},
		{
			name:      "punctuation after an escaped backslash",
			input:     "# Text\\\\!\n",
			wantLines: []int{1},
			wantFix:   true,
			wantFixed: "# Text\\\\\n",
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

// TestNoTrailingPunctuationRule_KeepsHeadingContent verifies that MD026
// reports a heading of nothing but punctuation, as markdownlint does, but
// withholds the fix whenever deleting the punctuation would leave the heading
// showing nothing: no text other than whitespace, no code span and no image.
func TestNoTrailingPunctuationRule_KeepsHeadingContent(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFixed string // Output after the fixes; empty means unchanged.
	}{
		{name: "period alone", input: "## .\n"},
		{name: "ellipsis alone", input: "## ...\n"},
		{name: "exclamation marks alone", input: "## !!\n"},
		{name: "full-width period alone", input: "# \u3002\n"},
		{name: "escaped period alone", input: "## \\.\n"},
		{name: "period alone in a closed heading", input: "## . ##\n"},
		{name: "period alone after extra spaces", input: "##   .\n"},
		{name: "period after a no-break space", input: "## \u00a0.\n"},
		{name: "period after a no-break space reference", input: "## &nbsp;.\n"},
		{name: "period after a link without text", input: "## [](https://example.com).\n"},
		{name: "period alone in a heading in a blockquote", input: "> ## .\n"},
		{name: "period alone in a heading in a list item", input: "- ## .\n"},
		{name: "ellipsis alone in a setext heading", input: "...\n---\n"},
		{name: "period after a code span", input: "## `x`.\n", wantFixed: "## `x`\n"},
		{name: "period after an image", input: "## ![logo](logo.png).\n", wantFixed: "## ![logo](logo.png)\n"},
		{name: "period after an image without alt text", input: "## ![](logo.png).\n", wantFixed: "## ![](logo.png)\n"},
		{name: "period after emphasis", input: "## *x*.\n", wantFixed: "## *x*\n"},
		{name: "period after link text", input: "## [docs](https://example.com) .\n", wantFixed: "## [docs](https://example.com)\n"},
		{name: "period after a character reference", input: "## &copy;.\n", wantFixed: "## &copy;\n"},
		// The run is both periods, so the fix deletes the backslash too.
		{name: "escaped period and period alone", input: "## \\..\n"},
		{name: "escaped period and period after text", input: "## a\\..\n", wantFixed: "## a\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewNoTrailingPunctuationRule(), tt.input, nil)
			require.Len(t, diags, 1)
			want := tt.wantFixed
			if want == "" {
				want = tt.input
			}
			assert.Equal(t, tt.wantFixed != "", diags[0].HasFix())
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

// TestNoTrailingPunctuationRule_FragmentsInRawHTML verifies that MD026 keeps
// a heading anchor that a raw HTML tag could refer to, however the tag is
// written, in both flavors, and still fixes headings when raw HTML holds a
// hash only in element text or a comment.
func TestNoTrailingPunctuationRule_FragmentsInRawHTML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantFix bool
	}{
		{name: "unquoted value with a character reference after an equals sign", input: "# Foo !\n\n<div>\n<a href=?q=&#35;foo->jump</a>\n</div>\n"},
		{name: "unquoted value with a hash after an equals sign", input: "# Foo !\n\n<div>\n<a href=?q==#foo->jump</a>\n</div>\n"},
		{name: "unterminated tag in an HTML block", input: "# Foo !\n\n<div>\n<a href=#foo-\n"},
		{name: "unterminated tag in a paragraph", input: "# Foo !\n\nSee <a href=#foo-\n"},
		{name: "tag spanning lines in an HTML block", input: "# Foo !\n\n<div>\n<a\nhref=\"#foo-\">jump</a>\n</div>\n"},
		{name: "tag spanning lines in a paragraph", input: "# Foo !\n\nSee <a\nhref=\"#foo-\">jump</a>.\n"},
		{name: "quoted greater-than sign before the hash", input: "# Foo !\n\n<div>\n<a title=\">\" href=\"#foo-\">jump</a>\n</div>\n"},
		{name: "comment opener in a code span before a tag", input: "# Foo !\n\n`<!--` <a href=\"#foo-\">jump</a>\n"},
		{name: "escaped comment opener before a tag", input: "# Foo !\n\n\\<!-- <a href=\"#foo-\">jump</a>\n"},
		{name: "tag in a code span", input: "# Foo !\n\n`<a href=\"#foo-\">`\n", wantFix: true},
		{name: "escaped tag", input: "# Foo !\n\n\\<a href=\"#foo-\">jump\n"},
		// Raw HTML is not interpreted: a hash anywhere in it counts, in
		// element text and comments too.
		{name: "hash in element text", input: "# Foo !\n\n<div>\n#hashtag text\n</div>\n"},
		{name: "hash in a comment", input: "# Foo !\n\n<!-- #note -->\n"},
		{name: "hash in an inline comment", input: "# Foo !\n\nText <!-- #note --> here.\n"},
		// Inside style, script and textarea, "<!--" opens no comment, so the
		// link after the closing tag is live.
		{name: "comment opener inside style", input: "# Foo !\n\n<style>\n<!-- </style><a href=\"#foo-\">jump</a> -->\n"},
		{name: "comment opener inside script", input: "# Foo !\n\n<script>\n<!-- </script><a href=\"#foo-\">jump</a> -->\n"},
		{name: "comment opener inside textarea", input: "# Foo !\n\n<textarea>\n<!-- </textarea><a href=\"#foo-\">jump</a> -->\n"},
		{name: "comment opener inside an inline textarea", input: "# Foo !\n\nA <textarea><!-- </textarea><a href=\"#foo-\">jump</a> --> b\n"},
		// Any raw HTML at all may refer to anchors: HTML can run on across a
		// blank line that ends the HTML block.
		{name: "raw HTML without a hash", input: "# Foo !\n\n<div>\n<!-- note -->\ntext\n</div>\n"},
		{name: "tag that a blank line cuts in two", input: "# Foo !\n\n<div>\n<a title=\"\n\nx\" href=#foo- ><b>jump</b></a>\n</div>\n"},
		{name: "inline tag without a hash", input: "# Foo !\n\nSome <b>bold</b> text.\n"},
		{name: "autolink", input: "# Foo !\n\nSee <https://example.com>.\n", wantFix: true},
		{name: "angle-bracket link destination", input: "# Foo !\n\nSee [it](<https://example.com>).\n", wantFix: true},
		{name: "escaped tag without a hash", input: "# Foo !\n\nSee \\<b> here.\n", wantFix: true},
		{name: "tag in a code span without a hash", input: "# Foo !\n\nSee `<b>` here.\n", wantFix: true},
		// Browsers end a comment at "--!>", and read "<!-->" and "<!--->" as
		// empty comments, so the links after them are live.
		{name: "comment that browsers end at --!>", input: "# Foo !\n\n<!-- comment --!><a href=\"#foo-\">jump</a>\n"},
		{name: "empty comment <!-->", input: "# Foo !\n\n<!--> <a href=\"#foo-\">jump</a>\n"},
		{name: "empty comment <!--->", input: "# Foo !\n\n<!---> <a href=\"#foo-\">jump</a>\n"},
		{name: "comment holding --", input: "# Foo !\n\n<!-- a -- b --> <a href=\"#foo-\">jump</a>\n"},
		{name: "unterminated comment before a link", input: "# Foo !\n\n<!-- open\n<a href=\"#foo-\">jump</a>\n"},
		{name: "empty inline comment before a link", input: "# Foo !\n\nText <!--> <a href=\"#foo-\">jump</a>\n"},
	}
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, tt := range tests {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				file, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoTrailingPunctuationRule().Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, 1)
				assert.Equal(t, tt.wantFix, diags[0].HasFix())
			})
		}
	}
}

// TestNoTrailingPunctuationRule_PossibleAnchorReferences verifies that MD026
// changes no heading text when the document holds a hash outside code and
// heading markers, or any raw HTML, in both flavors, and keeps its fix
// otherwise.
func TestNoTrailingPunctuationRule_PossibleAnchorReferences(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantFix bool
	}{
		{name: "heading text after inline HTML", input: "# <b></b> !\n\n[jump](#-)\n"},
		{name: "heading that holds only an image", input: "# ![logo](logo.png) !\n\n[jump](#-)\n"},
		{name: "multi-line reference definition", input: "# Foo !\n\n[unused]:\n  #foo-\n"},
		{name: "multi-line reference definition before text", input: "# Foo !\n\n[unused]:\n#foo-\ntext\n"},
		{name: "link destination at the start of a line", input: "# Foo !\n\n[jump](\n#foo-)\n"},
		{name: "reference definition in a blockquote", input: "# Foo !\n\n> [ref]: #foo-\n"},
		{name: "reference definition in a list", input: "# Foo !\n\n- [ref]: #foo-\n"},
		{name: "hash in prose", input: "# Foo !\n\nI like C#.\n"},
		{name: "no hash outside headings and no raw HTML", input: "# Foo !\n\nPlain text.\n", wantFix: true},
		{name: "closing hash sequence of another heading", input: "# Foo !\n\n## Bar ##\n\nText.\n", wantFix: true},
		// MD018 and MD020 may make these hashes heading markers.
		{name: "hashes around a paragraph line", input: "# Foo !\n\n#Bar#\n", wantFix: true},
		{name: "hashes that end another heading", input: "# Foo !\n\n## Bar##\n", wantFix: true},
		{name: "hash in a code block", input: "# Foo !\n\n```c#\nx #y\n```\n", wantFix: true},
		{name: "hash in a code span", input: "# Foo !\n\nSee `#x` here.\n", wantFix: true},
	}
	for _, flavor := range []config.Flavor{config.FlavorCommonMark, config.FlavorGFM} {
		for _, tt := range tests {
			t.Run(string(flavor)+"/"+tt.name, func(t *testing.T) {
				file, err := goldmark.New(string(flavor)).Parse(context.Background(), "test.md", []byte(tt.input))
				require.NoError(t, err)
				cfg := config.NewConfig()
				cfg.Flavor = flavor
				diags, err := NewNoTrailingPunctuationRule().Apply(lint.NewRuleContext(context.Background(), file, cfg, nil))
				require.NoError(t, err)
				require.Len(t, diags, 1)
				assert.Equal(t, tt.wantFix, diags[0].HasFix())
			})
		}
	}
}
