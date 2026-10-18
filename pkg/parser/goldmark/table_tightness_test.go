package goldmark

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestTableKeepsParagraphBlankLine checks the blank-line flag of each table
// that GFM makes from a paragraph, parsed as Parse parses: a table that takes
// the paragraph's first line has the paragraph's flag, so it follows a blank
// line exactly when the paragraph did; a table made from later lines of a
// paragraph, or from the lines after link reference definitions at the start
// of one, follows those lines directly. Tables are listed in document order.
func TestTableKeepsParagraphBlankLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		blank []bool
	}{
		{"table after a blank line", "Text\n\n| a | b |\n| --- | --- |\n", []bool{true}},
		{"table right after text", "Text\n| a | b |\n| --- | --- |\n", []bool{false}},
		{"table after a blank line in an item", "- item\n\n  | a | b |\n  | --- | --- |\n- next\n", []bool{true}},
		{"table right after the text of an item", "- item\n  | a | b |\n  | --- | --- |\n- next\n", []bool{false}},
		{"table after a blank quoted line", "> text\n>\n> | a | b |\n> | --- | --- |\n", []bool{true}},
		{"table right after a definition", "Intro\n\n[x]: /u\n| a | b |\n| --- | --- |\n", []bool{false}},
		{"table right after a definition in an item", "Intro\n\n- [x]: /u\n  | a | b |\n  | --- | --- |\n- next\n", []bool{false}},
		{"table right after two definitions", "Intro\n\n[x]: /u\n[y]: /v\n| a | b |\n| --- | --- |\n", []bool{false}},
		{"table after a definition and a blank line", "[x]: /u\n\n| a | b |\n| --- | --- |\n", []bool{true}},
		{"table before a setext underline", "Intro\n\n| b | c |\n| - | - |\n===\n", []bool{true}},
		{"table between text and a setext underline", "Intro\n\na\n| b | c |\n| - | - |\n===\n", []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(FlavorGFM)
			pc := parser.NewContext()
			pc.Set(p.spans.key, newSpanRecorder())
			doc := p.md.Parser().Parse(text.NewReader([]byte(tt.input)), parser.WithContext(pc))
			blank := []bool{}
			err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
				if table, ok := node.(*east.Table); ok && entering {
					blank = append(blank, table.HasBlankPreviousLines())
				}
				return ast.WalkContinue, nil
			})
			require.NoError(t, err)
			require.Equal(t, tt.blank, blank)
		})
	}
}

// TestListTightnessAroundTablesFollowsCommonMark checks the tightness of
// lists whose items hold a table, under both flavors, against CommonMark: a
// blank line before a table that is a later block of an item makes the list
// loose, and a table directly after another block does not. GFM makes the
// table from a paragraph, so the blank line before the table is the
// paragraph's. Under CommonMark the table's lines stay a paragraph, which
// reads the same. Lists are listed in document order.
func TestListTightnessAroundTablesFollowsCommonMark(t *testing.T) {
	tests := []struct {
		name  string
		input string
		tight []bool
	}{
		{"table after a blank line in an item", "- item\n\n  | a | b |\n  | --- | --- |\n- next\n", []bool{false}},
		{"table after a blank line in a nested item", "- a\n  - item\n\n    | a | b |\n    | --- | --- |\n  - c\n", []bool{true, false}},
		{"table after a blank line in a quoted list", "> - item\n>\n>   | a | b |\n>   | --- | --- |\n> - next\n", []bool{false}},
		{"table after a heading and a blank line", "- # T\n\n  | a | b |\n  | --- | --- |\n- next\n", []bool{false}},
		{"table after a fence and a blank line", "- item\n  ```\n  code\n  ```\n\n  | a | b |\n  | --- | --- |\n- next\n", []bool{false}},
		{"table right after the text of an item", "- item\n  | a | b |\n  | --- | --- |\n- next\n", []bool{true}},
		{"table right after a definition after a blank line", "Intro\n\n- [x]: /u\n  | a | b |\n  | --- | --- |\n- next\n", []bool{true}},
		{"table right after a quote's blank line", "- item\n  > quote\n  >\n  | a | b |\n  | --- | --- |\n- next\n", []bool{true}},
	}
	for _, tt := range tests {
		for _, flavor := range []string{FlavorCommonMark, FlavorGFM} {
			t.Run(tt.name+"/"+flavor, func(t *testing.T) {
				file := parseForSpans(t, flavor, tt.input)
				lists := mdast.FindByKind(file.Root, mdast.NodeList)
				tight := make([]bool, 0, len(lists))
				for _, list := range lists {
					require.NotNil(t, list.Block)
					require.NotNil(t, list.Block.List)
					tight = append(tight, list.Block.List.Tight)
				}
				require.Equal(t, tt.tight, tight)
			})
		}
	}
}
