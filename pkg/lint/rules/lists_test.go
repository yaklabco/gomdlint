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

func TestUnorderedListStyleRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "consistent dash style",
			input:     "- Item 1\n- Item 2\n- Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "consistent plus style",
			input:     "+ Item 1\n+ Item 2\n",
			wantDiags: 0,
			config:    map[string]any{"style": "plus"},
		},
		{
			name:      "consistent asterisk style",
			input:     "* Item 1\n* Item 2\n",
			wantDiags: 0,
			config:    map[string]any{"style": "asterisk"},
		},
		{
			name:      "mixed styles default to the first style",
			input:     "- Item 1\n\nText.\n\n* Item 2\n\nMore.\n\n+ Item 3\n",
			wantDiags: 2,
			wantFix:   "- Item 1\n\nText.\n\n- Item 2\n\nMore.\n\n- Item 3\n",
		},
		{
			name:      "default style is consistent",
			input:     "* Item 1\n* Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "default style follows the first list",
			input:     "+ Item 1\n\nText.\n\n- Item 2\n",
			wantDiags: 1,
			wantFix:   "+ Item 1\n\nText.\n\n+ Item 2\n",
		},
		{
			// The bullet change is what keeps the two lists apart, so fixing
			// it would merge them into one list.
			name:      "no fix when the change would merge adjacent lists",
			input:     "+ Item 1\n\n- Item 2\n",
			wantDiags: 1,
			wantFix:   "+ Item 1\n\n- Item 2\n",
		},
		{
			name:      "explicit dash style",
			input:     "* Item 1\n",
			wantDiags: 1,
			wantFix:   "- Item 1\n",
			config:    map[string]any{"style": "dash"},
		},
		{
			name:      "unknown style falls back to dash",
			input:     "* Item 1\n",
			wantDiags: 1,
			wantFix:   "- Item 1\n",
			config:    map[string]any{"style": "bogus"},
		},
		{
			name:      "sublist style accepts alternating markers",
			input:     "* Item 1\n  + Item 2\n    - Item 3\n  + Item 4\n* Item 5\n",
			wantDiags: 0,
			config:    map[string]any{"style": "sublist"},
		},
		{
			name:      "sublist style requires a marker that differs from the parent",
			input:     "* Item 1\n  * Item 2\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n  - Item 2\n",
			config:    map[string]any{"style": "sublist"},
		},
		{
			name:      "sublist style keeps each level consistent",
			input:     "* Item 1\n  + Item 2\n\nText.\n\n- Item 3\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n  + Item 2\n\nText.\n\n* Item 3\n",
			config:    map[string]any{"style": "sublist"},
		},
		{
			name:      "sublist style counts ordered parents",
			input:     "1. Item 1\n   * Item 2\n",
			wantDiags: 0,
			config:    map[string]any{"style": "sublist"},
		},
		{
			name:      "fix replaces the marker inside a blockquote",
			input:     "> * Item 1\n>\n> Text.\n>\n> - Item 2\n",
			wantDiags: 1,
			wantFix:   "> * Item 1\n>\n> Text.\n>\n> * Item 2\n",
		},
		{
			name:      "fix replaces the nested marker on a shared line",
			input:     "- Item\n\nText.\n\n- * Nested\n",
			wantDiags: 1,
			wantFix:   "- Item\n\nText.\n\n- - Nested\n",
		},
		{
			name:      "consistent style from first",
			input:     "* Item 1\n\nText.\n\n- Item 2\n",
			wantDiags: 1,
			wantFix:   "* Item 1\n\nText.\n\n* Item 2\n",
			config:    map[string]any{"style": "consistent"},
		},
		{
			name:      "ordered list ignored",
			input:     "1. Item 1\n2. Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "no lists",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "nested list same style",
			input:     "- Item 1\n  - Nested 1\n  - Nested 2\n- Item 2\n",
			wantDiags: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewUnorderedListStyleRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application.
			if tt.wantFix != "" && tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))

				// Verify idempotency: no fixable diagnostics remain.
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

// TestUnorderedListStyleRuleKeepsThematicBreaksOut checks that the fix never
// rewrites a line of nested empty items into a thematic break.
func TestUnorderedListStyleRuleKeepsThematicBreaksOut(t *testing.T) {
	for _, input := range []string{"* + *\n", "- + -\n"} {
		t.Run(input, func(t *testing.T) {
			snapshot, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, config.NewConfig(), nil)
			diags, err := NewUnorderedListStyleRule().Apply(ruleCtx)
			require.NoError(t, err)
			require.Len(t, diags, 1, "the middle bullet differs from the first list")
			assert.Empty(t, filterFixableDiags(diags))
		})
	}
}

func TestOrderedListIncrementRule(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDiags int
		wantFix   string
		config    map[string]any
	}{
		{
			name:      "sequential numbering",
			input:     "1. Item 1\n2. Item 2\n3. Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "all ones with ordered style",
			input:     "1. Item 1\n1. Item 2\n1. Item 3\n",
			wantDiags: 2,
			wantFix:   "1. Item 1\n2. Item 2\n3. Item 3\n",
			config:    map[string]any{"style": "ordered"},
		},
		{
			name:      "default accepts all ones",
			input:     "1. Item 1\n1. Item 2\n1. Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "default accepts zero-based numbering",
			input:     "0. Item 1\n1. Item 2\n2. Item 3\n",
			wantDiags: 0,
		},
		{
			name:      "wrong sequence",
			input:     "1. Item 1\n3. Item 2\n5. Item 3\n",
			wantDiags: 2,
			wantFix:   "1. Item 1\n2. Item 2\n3. Item 3\n",
		},
		{
			// markdownlint expects an incrementing list to start at 0 or 1,
			// but the first number is the start the list renders with, so
			// no item of the list is renumbered.
			name:      "list starting above one is reported without a fix",
			input:     "5. Item 1\n6. Item 2\n7. Item 3\n",
			wantDiags: 3,
			wantFix:   "5. Item 1\n6. Item 2\n7. Item 3\n",
		},
		{
			name:      "list starting above one in a blockquote is reported without a fix",
			input:     "> 3. Step three\n> 4. Step four\n",
			wantDiags: 2,
			wantFix:   "> 3. Step three\n> 4. Step four\n",
		},
		{
			name:      "single item numbered above one is reported without a fix",
			input:     "2. Item\n",
			wantDiags: 1,
			wantFix:   "2. Item\n",
		},
		{
			name:      "ones style reports a list starting at zero without a fix",
			input:     "0. Item\n0. Item\n",
			wantDiags: 2,
			wantFix:   "0. Item\n0. Item\n",
			config:    map[string]any{"style": "one"},
		},
		{
			name:      "zero-based list is renumbered from zero",
			input:     "0. Item\n2. Item\n",
			wantDiags: 1,
			wantFix:   "0. Item\n1. Item\n",
		},
		{
			name:      "incrementing list with a repeated number",
			input:     "1. Item\n2. Item\n1. Item\n4. Item\n",
			wantDiags: 1,
			wantFix:   "1. Item\n2. Item\n3. Item\n4. Item\n",
		},
		{
			name:      "ones list with a stray number",
			input:     "1. Item\n1. Item\n3. Item\n1. Item\n",
			wantDiags: 1,
			wantFix:   "1. Item\n1. Item\n1. Item\n1. Item\n",
		},
		{
			name:      "zero style",
			input:     "0. Item\n1. Item\n",
			wantDiags: 1,
			wantFix:   "0. Item\n0. Item\n",
			config:    map[string]any{"style": "zero"},
		},
		{
			name:      "zero style reports a list starting at one without a fix",
			input:     "1. Item\n",
			wantDiags: 1,
			wantFix:   "1. Item\n",
			config:    map[string]any{"style": "zero"},
		},
		{
			name:      "numbers inside a blockquote",
			input:     "> 1. one\n> 2. two\n",
			wantDiags: 0,
		},
		{
			name:      "wrong number inside a blockquote",
			input:     "> 1. one\n> 3. two\n",
			wantDiags: 1,
			wantFix:   "> 1. one\n> 2. two\n",
		},
		{
			name:      "ordered list nested on a bullet line",
			input:     "- 1. alpha\n  2. beta\n",
			wantDiags: 0,
		},
		{
			name:      "wrong number in a list nested on a bullet line",
			input:     "- 1. alpha\n  3. beta\n",
			wantDiags: 1,
			wantFix:   "- 1. alpha\n  2. beta\n",
		},
		{
			name:      "fix keeps the parenthesis delimiter",
			input:     "1) Item\n1) Item\n3) Item\n",
			wantDiags: 1,
			wantFix:   "1) Item\n1) Item\n1) Item\n",
		},
		{
			name:      "fix keeps right-aligned numbers aligned",
			input:     " 1. Item\n 2. Item\n 9. Item\n10. Item\n",
			wantDiags: 2,
			wantFix:   " 1. Item\n 2. Item\n 3. Item\n 4. Item\n",
		},
		{
			name:      "right-aligned list starting above one is reported without a fix",
			input:     " 9. Item\n10. Item\n11. Item\n",
			wantDiags: 3,
			wantFix:   " 9. Item\n10. Item\n11. Item\n",
		},
		{
			name:      "fix keeps zero-padded numbers padded",
			input:     "01. Item\n02. Item\n04. Item\n10. Item\n",
			wantDiags: 2,
			wantFix:   "01. Item\n02. Item\n03. Item\n04. Item\n",
		},
		{
			name:      "zero-padded list starting above one is reported without a fix",
			input:     "08. Item\n09. Item\n10. Item\n",
			wantDiags: 3,
			wantFix:   "08. Item\n09. Item\n10. Item\n",
		},
		{
			name:      "wider number is fixed when nothing depends on the content column",
			input:     "1. a\n2. b\n3. c\n4. d\n5. e\n6. f\n7. g\n8. h\n9. i\n9. j\n",
			wantDiags: 1,
			wantFix:   "1. a\n2. b\n3. c\n4. d\n5. e\n6. f\n7. g\n8. h\n9. i\n10. j\n",
		},
		{
			// The first number is the start of the list, so a first item is
			// neither renumbered nor padded.
			name:      "first item is neither renumbered nor padded",
			input:     "- first item\n 10. second item\n",
			wantDiags: 1,
			wantFix:   "- first item\n 10. second item\n",
		},
		{
			// A space inserted right after ">" becomes the blockquote's
			// optional space instead of moving the marker.
			name:      "no padding right after a blockquote marker",
			input:     "> 1. a\n> 9. b\n>10. c\n",
			wantDiags: 2,
			wantFix:   "> 1. a\n> 2. b\n>10. c\n",
		},
		{
			// Only a list starting at 1 can interrupt a paragraph, so "0." would
			// turn the list into paragraph text.
			name:      "no fix that stops a list from interrupting a paragraph",
			input:     "Para\n1. item\n",
			wantDiags: 1,
			wantFix:   "Para\n1. item\n",
			config:    map[string]any{"style": "zero"},
		},
		{
			// Widening "9." to "10." moves the content column past the
			// nested list, which would then leave the item.
			name:      "wider number is not fixed when it would move nested content",
			input:     "1. a\n2. b\n3. c\n4. d\n5. e\n6. f\n7. g\n8. h\n9. i\n9. j\n   - nested\n",
			wantDiags: 1,
			wantFix:   "1. a\n2. b\n3. c\n4. d\n5. e\n6. f\n7. g\n8. h\n9. i\n9. j\n   - nested\n",
		},
		{
			name:      "unordered list ignored",
			input:     "- Item 1\n- Item 2\n",
			wantDiags: 0,
		},
		{
			name:      "no lists",
			input:     "Just text\n",
			wantDiags: 0,
		},
		{
			name:      "empty file",
			input:     "",
			wantDiags: 0,
		},
		{
			name:      "allow renumbering false",
			input:     "1. Item 1\n1. Item 2\n",
			wantDiags: 1,
			wantFix:   "1. Item 1\n1. Item 2\n", // No fix when renumbering disabled.
			config:    map[string]any{"allow_renumbering": false, "style": "ordered"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := goldmark.New(string(config.FlavorCommonMark))
			snapshot, err := parser.Parse(context.Background(), "test.md", []byte(tt.input))
			require.NoError(t, err)

			rule := NewOrderedListIncrementRule()
			cfg := config.NewConfig()
			var ruleCfg *config.RuleConfig
			if tt.config != nil {
				ruleCfg = &config.RuleConfig{Options: tt.config}
			}
			ruleCtx := lint.NewRuleContext(context.Background(), snapshot, cfg, ruleCfg)

			diags, err := rule.Apply(ruleCtx)
			require.NoError(t, err)
			assert.Len(t, diags, tt.wantDiags)

			// Verify fix application.
			if tt.wantFix != "" && tt.wantDiags > 0 {
				var allEdits []fix.TextEdit
				for _, d := range diags {
					allEdits = append(allEdits, d.FixEdits...)
				}
				prepared, err := fix.PrepareEdits(allEdits, len(tt.input))
				require.NoError(t, err)
				fixed := fix.ApplyEdits([]byte(tt.input), prepared)
				assert.Equal(t, tt.wantFix, string(fixed))

				// Verify idempotency: no fixable diagnostics remain.
				snapshot2, err := parser.Parse(context.Background(), "test.md", fixed)
				require.NoError(t, err)
				ruleCtx2 := lint.NewRuleContext(context.Background(), snapshot2, cfg, ruleCfg)
				diags2, err := rule.Apply(ruleCtx2)
				require.NoError(t, err)
				assert.Empty(t, filterFixableDiags(diags2), "fix should be idempotent")
			}
		})
	}
}

func TestUnorderedListStyleRule_Metadata(t *testing.T) {
	rule := NewUnorderedListStyleRule()

	assert.Equal(t, "MD004", rule.ID())
	assert.Equal(t, "unordered-list-style", rule.Name())
	assert.Contains(t, rule.Tags(), "lists")
	assert.Contains(t, rule.Tags(), "style")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}

func TestOrderedListIncrementRule_Metadata(t *testing.T) {
	rule := NewOrderedListIncrementRule()

	assert.Equal(t, "MD029", rule.ID())
	assert.Equal(t, "ol-prefix", rule.Name())
	assert.Contains(t, rule.Tags(), "ol")
	assert.True(t, rule.CanFix())
	assert.True(t, rule.DefaultEnabled())
	assert.Equal(t, config.SeverityWarning, rule.DefaultSeverity())
}
