package rules

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestListAndWhitespaceRulesIgnoreByteOrderMark checks that a byte order mark
// before the first line changes neither the findings of the list,
// whitespace and blockquote rules nor their fixes, which must keep the mark.
func TestListAndWhitespaceRulesIgnoreByteOrderMark(t *testing.T) {
	tests := []struct {
		name    string
		rule    lint.Rule
		options map[string]any
		input   string
		count   int
	}{
		{"MD004 bullets on the first line", NewUnorderedListStyleRule(), map[string]any{"style": "dash"}, "* a\n* b\n", 2},
		{"MD005 ordered list on the first line", NewListIndentRule(), nil, "1. a\n 2. b\n", 1},
		{"MD007 indented list on the first line", NewULIndentRule(), nil, "  - a\n  - b\n", 2},
		{"MD009 trailing space on the first line", NewTrailingWhitespaceRule(), nil, "text \nmore\n", 1},
		{"MD010 tab on the first line", NewHardTabsRule(), nil, "\tcode\n", 1},
		{"MD012 blank lines at the start", NewMultipleBlankLinesRule(), nil, "\n\n\ntext\n", 1},
		{"MD013 long first line", NewMaxLineLengthRule(), map[string]any{"line_length": 20}, "aaaa bbbb cccc dddd eeee ffff\n", 1},
		{"MD027 quote space on the first line", NewNoMultipleSpaceBlockquoteRule(), nil, ">  quoted\n", 1},
		{"MD027 quote space before a list item on the first line", NewNoMultipleSpaceBlockquoteRule(), nil, ">  - item\n", 1},
		{"MD027 nested quote space on the first line", NewNoMultipleSpaceBlockquoteRule(), nil, "> >  nested\n", 1},
		{"MD028 blank line in a quote that starts the file", NewNoBlanksBlockquoteRule(), nil, "> a\n\n> b\n", 1},
		{"MD029 list starting above one on the first line", NewOrderedListIncrementRule(), nil, "3. a\n4. b\n", 2},
		{"MD030 marker space on the first line", NewListMarkerSpaceRule(), nil, "-  item\n", 1},
		{"MD030 ordered marker space on the first line", NewListMarkerSpaceRule(), nil, "1.  item\n2.  next\n", 2},
		{"MD009 hard break on the first line", NewTrailingWhitespaceRule(), nil, "text   \nmore\n", 1},
		{"MD032 list on the first line", NewBlanksAroundListsRule(), nil, "- a\n# h\n", 1},
		{"MD047 missing final newline", NewFinalNewlineRule(), nil, "text", 1},
		{"MD047 empty file", NewFinalNewlineRule(), nil, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plain := applyRuleWithOptions(t, tt.rule, tt.input, tt.options)
			require.Len(t, plain, tt.count, "without a byte order mark")
			marked := applyRuleWithOptions(t, tt.rule, mdast.ByteOrderMark+tt.input, tt.options)
			require.Equal(t, diagnosticLines(plain), diagnosticLines(marked))
			require.Len(t, filterFixableDiags(marked), len(filterFixableDiags(plain)), "fixable diagnostics")
			require.Equal(t, mdast.ByteOrderMark+applyAllFixEdits(t, tt.input, plain),
				applyAllFixEdits(t, mdast.ByteOrderMark+tt.input, marked))
		})
	}
}
