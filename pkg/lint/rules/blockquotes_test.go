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

func TestNoMultipleSpaceBlockquoteRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "correct single space",
			input:     "> This is a blockquote\n",
			wantDiags: 0,
		},
		{
			name:      "multiple spaces after >",
			input:     ">  This is a blockquote\n",
			wantDiags: 1,
			wantFix:   "> This is a blockquote\n",
		},
		{
			name:      "three spaces after >",
			input:     ">   This is a blockquote\n",
			wantDiags: 1,
			wantFix:   "> This is a blockquote\n",
		},
		{
			name:      "multiple lines with issues",
			input:     ">  Line one\n>   Line two\n",
			wantDiags: 2,
			wantFix:   "> Line one\n> Line two\n",
		},
		{
			name:      "nested blockquote",
			input:     ">> Nested\n",
			wantDiags: 0,
		},
		{
			name:      "no space after > is ok",
			input:     ">No space\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			// The indentation belongs to the list item, not to the quote.
			name:      "list item continuation in a blockquote",
			input:     "> - item\n>\n>   second paragraph\n",
			wantDiags: 0,
		},
		{
			name:      "nested list in a blockquote",
			input:     "> - parent\n>   - child\n",
			wantDiags: 0,
		},
		{
			name:      "extra space in a nested blockquote",
			input:     "> >  x\n",
			wantDiags: 1,
			wantFix:   "> > x\n",
		},
		{
			name:      "extra space before a nested blockquote marker",
			input:     ">  > x\n",
			wantDiags: 1,
			wantFix:   "> > x\n",
		},
		{
			name:      "indented blockquote",
			input:     "  >  x\n",
			wantDiags: 1,
			wantFix:   "  > x\n",
		},
		{
			name:      "extra space before a list marker",
			input:     ">  - item\n",
			wantDiags: 1,
			wantFix:   "> - item\n",
		},
		{
			name:      "list items can be excluded",
			input:     ">  - item\n",
			wantDiags: 0,
			config:    map[string]any{"list_items": false},
		},
		{
			// The indentation of the table rows is the content indentation
			// of the list item, not extra space after the blockquote marker.
			name:      "table in a quoted list item",
			input:     "> - | a |\n>   | - |\n",
			wantDiags: 0,
		},
		{
			// An item holding a table is not moved, because its rows depend
			// on its content column.
			name:      "indented quoted list item holding a table",
			input:     ">  - | a |\n>    | - |\n",
			wantDiags: 1,
			wantFix:   ">  - | a |\n>    | - |\n",
		},
		{
			name:      "lazy line in a quoted list",
			input:     "> - item\n>  lazy text\n",
			wantDiags: 1,
			wantFix:   "> - item\n> lazy text\n",
		},
		{
			name:      "lazy line in a quoted list is a list line",
			input:     "> - item\n>  lazy text\n",
			wantDiags: 0,
			config:    map[string]any{"list_items": false},
		},
		{
			name:      "indented code in a blockquote",
			input:     ">     code\n",
			wantDiags: 0,
		},
		{
			name:      "fenced code content in a blockquote",
			input:     "> ```\n>  code\n> ```\n",
			wantDiags: 0,
		},
		{
			// Moving a fence changes how much indentation its content keeps.
			name:      "indented fence is reported without a fix",
			input:     ">  ```\n>  code\n>  ```\n",
			wantDiags: 2,
			wantFix:   ">  ```\n>  code\n>  ```\n",
		},
		{
			name:      "quote line holding only spaces",
			input:     "> a\n>   \n> b\n",
			wantDiags: 0,
		},
		{
			// The tab keeps "- second item" paragraph text; removing it would
			// start a new list item.
			name:      "no fix when removing the space would start a block",
			input:     "> -  first item\n> \t- second item\n",
			wantDiags: 1,
			wantFix:   "> -  first item\n> \t- second item\n",
		},
		{
			name:      "extra space before a heading",
			input:     ">  # Heading\n",
			wantDiags: 1,
			wantFix:   "> # Heading\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoMultipleSpaceBlockquoteRule()
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

				// The fixed text keeps its structure and leaves nothing to fix.
				assert.Equal(t, listShape(t, tt.input), listShape(t, string(fixed)))
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				diags2, err := rule.Apply(lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg))
				require.NoError(t, err)
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

func TestNoBlanksBlockquoteRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
	}{
		{
			name:      "single blockquote",
			input:     "> This is a blockquote\n",
			wantDiags: 0,
		},
		{
			name:      "continuous blockquote",
			input:     "> Line one\n> Line two\n",
			wantDiags: 0,
		},
		{
			name:      "blockquote with internal blank",
			input:     "> Line one\n>\n> Line two\n",
			wantDiags: 0,
		},
		{
			name:      "separated blockquotes",
			input:     "> First blockquote\n\n> Second blockquote\n",
			wantDiags: 1,
		},
		{
			name:      "blockquotes with text between",
			input:     "> First blockquote\n\nSome text\n\n> Second blockquote\n",
			wantDiags: 0,
		},
		{
			// markdownlint reports each blank line.
			name:      "multiple blank lines between blockquotes",
			input:     "> First\n\n\n> Second\n",
			wantDiags: 2,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "short file",
			input:     "> Single\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewNoBlanksBlockquoteRule()
			cfg := config.NewConfig()
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, nil)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)
		})
	}
}

func TestNoMultipleSpaceBlockquoteRule_Metadata(t *testing.T) {
	rule := NewNoMultipleSpaceBlockquoteRule()

	assert.Equal(t, "MD027", rule.ID())
	assert.Equal(t, "no-multiple-space-blockquote", rule.Name())
	assert.Contains(t, rule.Tags(), "blockquote")
	assert.Contains(t, rule.Tags(), "whitespace")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}

// TestNoBlanksBlockquoteRule_MarkdownlintParity checks MD028 against
// markdownlint 0.41.1: each blank line between two blockquotes that are
// next to each other in the same document, list or list item is reported,
// wherever the blockquotes start on their lines. A blockquote inside
// another one is not, because the outer one's ">" is on every line between.
func TestNoBlanksBlockquoteRule_MarkdownlintParity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines []int
	}{
		{name: "two blank lines", input: "> a\n\n\n> b\n", wantLines: []int{2, 3}},
		{name: "three blockquotes", input: "> a\n\n> b\n\n> c\n", wantLines: []int{2, 4}},
		{name: "indented blockquotes", input: "  > a\n\n  > b\n", wantLines: []int{2}},
		{name: "differently indented blockquotes", input: " > a\n\n   > b\n", wantLines: []int{2}},
		{name: "blockquotes in a list item", input: "- item\n\n  > c\n\n  > d\n", wantLines: []int{4}},
		{name: "blockquotes right after the item text", input: "- item\n  > c\n\n  > d\n", wantLines: []int{3}},
		{name: "blockquote that starts an item", input: "- > a\n\n  > b\n", wantLines: []int{2}},
		{name: "blank line with indentation", input: "- > a\n  \n  > b\n", wantLines: []int{2}},
		{name: "blockquotes in an ordered list item", input: "1. x\n\n   > a\n\n   > b\n", wantLines: []int{4}},
		{name: "blockquotes in a nested list item", input: "- - > a\n\n    > b\n", wantLines: []int{2}},
		{name: "whitespace-only line", input: "> a\n   \n> b\n", wantLines: []int{2}},
		{name: "outer blockquotes holding inner ones", input: "> > a\n\n> > b\n", wantLines: []int{2}},
		{name: "inner blockquotes", input: "> > a\n>\n> > b\n", wantLines: []int{}},
		{name: "blockquotes in a list in a blockquote", input: "> - > a\n>\n>   > b\n", wantLines: []int{}},
		{name: "blockquotes in different list items", input: "- > a\n\n- > b\n", wantLines: []int{}},
		{name: "blockquote and a list", input: "> a\n\n- > b\n", wantLines: []int{}},
		{name: "definition between", input: "> a\n\n[x]: /u\n\n> b\n", wantLines: []int{}},
		{name: "comment between", input: "> a\n\n<!-- c -->\n\n> b\n", wantLines: []int{}},
		{name: "indented code after", input: "> a\n\n    > b\n", wantLines: []int{}},
		{name: "quote markers in an HTML block", input: "<div>\n> x\n\n> y\n</div>\n", wantLines: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := applyRuleWithOptions(t, NewNoBlanksBlockquoteRule(), tt.input, nil)
			assert.Equal(t, tt.wantLines, diagLines(diags))
		})
	}
}

func TestNoBlanksBlockquoteRule_Metadata(t *testing.T) {
	rule := NewNoBlanksBlockquoteRule()

	assert.Equal(t, "MD028", rule.ID())
	assert.Equal(t, "no-blanks-blockquote", rule.Name())
	assert.Contains(t, rule.Tags(), "blockquote")
	assert.False(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
}
