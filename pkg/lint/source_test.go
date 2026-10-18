package lint_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/lint/rules"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

func parseSource(t *testing.T, input string) *mdast.FileSnapshot {
	t.Helper()
	file, err := goldmark.New(goldmark.FlavorGFM).Parse(context.Background(), "source.md", []byte(input))
	require.NoError(t, err)
	return file
}

func TestCodeContentLines(t *testing.T) {
	t.Parallel()

	input := "# T\n" + // 1
		"\n" + // 2
		"```js  \n" + // 3: opening fence
		"code\n" + // 4
		"\n" + // 5: blank content line
		"```  \n" + // 6: closing fence
		"\n" + // 7
		"    indented\n" + // 8
		"\n" + // 9
		"> ~~~\n" + // 10: opening fence in a blockquote
		"> ~~~\n" + // 11: closing fence, no content
		"\n" + // 12
		"~~~\n" + // 13: unclosed fence
		"unclosed\n" // 14

	file := parseSource(t, input)
	lines := lint.CodeContentLines(lint.CodeBlocks(file.Root))

	assert.Equal(t, map[int]bool{4: true, 5: true, 8: true, 14: true}, lines)
}

// containerFixture exercises blockquotes, list items and their nesting.
const containerFixture = "> ## Quoted\n" + // 1
	">\n" + // 2
	"> Para\n" + // 3
	"\n" + // 4
	"- item\n" + // 5
	"\n" + // 6
	"  ## Nested\n" + // 7
	"\n" + // 8
	"> - quoted item\n" + // 9
	">\n" + // 10
	">   ```\n" + // 11
	">   code\n" + // 12
	">   ```\n" + // 13
	"\n" + // 14
	"> lazy\n" + // 15
	"continuation\n" // 16

func TestContainerPrefix(t *testing.T) {
	t.Parallel()

	file := parseSource(t, containerFixture)
	headings := lint.Headings(file.Root)
	require.Len(t, headings, 2)
	quoted, nested := headings[0], headings[1]
	code := lint.CodeBlocks(file.Root)[0]
	lazy := lint.Paragraphs(file.Root)[len(lint.Paragraphs(file.Root))-1]

	tests := []struct {
		name string
		node *mdast.Node
		line int
		want string
	}{
		{"heading line in blockquote", quoted, 1, "> "},
		{"marker-only line in blockquote", quoted, 2, ">"},
		{"text line in blockquote", quoted, 3, "> "},
		{"line outside the blockquote", quoted, 4, ""},
		{"continuation line in list item", nested, 7, "  "},
		{"blank line in list item", nested, 6, ""},
		{"first line of the list item", nested, 5, "- "},
		{"first line of a quoted list item", code, 9, "> - "},
		{"blank line in quoted list item", code, 10, ">"},
		{"code line in quoted list item", code, 12, ">   "},
		{"lazy continuation line", lazy, 16, ""},
		{"out of range line", quoted, 99, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(lint.ContainerPrefix(file, tt.node, tt.line)))
		})
	}
}

// TestContinuesContainers checks which lines continue a container and the
// containers around it explicitly, as a construct that allows no lazy line,
// such as a math block, needs.
func TestContinuesContainers(t *testing.T) {
	t.Parallel()

	file := parseSource(t, "> a\nlazy\n> b\n>\n\n- c\n\n  d\ne\n> - f\n>   g\n> h\n")
	quotes := mdast.FindByKind(file.Root, mdast.NodeBlockquote)
	items := mdast.FindByKind(file.Root, mdast.NodeListItem)
	require.Len(t, quotes, 2)
	require.Len(t, items, 2)
	quote, item, quotedItem := quotes[0], items[0], items[1]

	tests := []struct {
		name      string
		container *mdast.Node
		line      int
		want      bool
	}{
		{"first line of a blockquote", quote, 1, true},
		{"lazy line", quote, 2, false},
		{"quoted line", quote, 3, true},
		{"marker-only line", quote, 4, true},
		{"blank line after a blockquote", quote, 5, false},
		{"first line of a list item", item, 6, true},
		{"blank line in a list item", item, 7, true},
		{"indented line", item, 8, true},
		{"unindented line", item, 9, false},
		{"first line of a quoted list item", quotedItem, 10, true},
		{"indented quoted line", quotedItem, 11, true},
		{"quoted line lazy for the list item", quotedItem, 12, false},
		{"out of range line", quote, 99, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, lint.ContinuesContainers(file, tt.container, tt.line))
		})
	}
	assert.False(t, lint.ContinuesContainers(file, nil, 1))
}

// TestIndentAfterContainers checks the space a line holds after the
// prefixes of its containers, which decides whether a container such as a
// footnote definition can start there.
func TestIndentAfterContainers(t *testing.T) {
	t.Parallel()

	file := parseSource(t, "  top\n>    quoted\n- a\n\n\ttab\n     five\nlazy\n")
	quote := mdast.FindByKind(file.Root, mdast.NodeBlockquote)[0]
	item := mdast.FindByKind(file.Root, mdast.NodeListItem)[0]

	tests := []struct {
		name       string
		container  *mdast.Node
		line       int
		wantIndent int
		wantText   string
		wantOK     bool
	}{
		{"no container", nil, 1, 2, "top", true},
		{"after a quote marker and its space", quote, 2, 3, "quoted", true},
		{"tab partly claimed by the list item", item, 5, 2, "tab", true},
		{"spaces past the list item's indentation", item, 6, 3, "five", true},
		{"lazy line", item, 7, 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			indent, start, ok := lint.IndentAfterContainers(file, tt.container, tt.line)
			assert.Equal(t, tt.wantOK, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.wantIndent, indent)
			assert.Equal(t, tt.wantText, string(lint.RawLineContent(file, tt.line)[start:]))
		})
	}
}

func TestIsBlankLineInContainer(t *testing.T) {
	t.Parallel()

	file := parseSource(t, containerFixture)
	headings := lint.Headings(file.Root)
	quoted, nested := headings[0], headings[1]
	code := lint.CodeBlocks(file.Root)[0]

	assert.False(t, lint.IsBlankLineInContainer(file, quoted, 1))
	assert.True(t, lint.IsBlankLineInContainer(file, quoted, 2), "a lone '>' is blank inside a blockquote")
	assert.False(t, lint.IsBlankLineInContainer(file, quoted, 3))
	assert.True(t, lint.IsBlankLineInContainer(file, quoted, 4))
	assert.True(t, lint.IsBlankLineInContainer(file, nested, 6))
	assert.False(t, lint.IsBlankLineInContainer(file, nested, 7))
	assert.True(t, lint.IsBlankLineInContainer(file, code, 10))
	assert.False(t, lint.IsBlankLineInContainer(file, code, 11))
	assert.False(t, lint.IsBlankLineInContainer(file, nested, 2), "'>' is not blank outside a blockquote")
}

func TestBlankLineTextAndContinuationPrefix(t *testing.T) {
	t.Parallel()

	file := parseSource(t, containerFixture+"\n> > deep\n")
	headings := lint.Headings(file.Root)
	quoted, nested := headings[0], headings[1]
	code := lint.CodeBlocks(file.Root)[0]
	deep := lint.Paragraphs(file.Root)[len(lint.Paragraphs(file.Root))-1]

	assert.Equal(t, ">\n", lint.BlankLineText(file, quoted))
	assert.Equal(t, "> ", lint.ContinuationPrefix(file, quoted))
	assert.Equal(t, "\n", lint.BlankLineText(file, nested))
	assert.Equal(t, "  ", lint.ContinuationPrefix(file, nested))
	assert.Equal(t, ">\n", lint.BlankLineText(file, code))
	assert.Equal(t, ">   ", lint.ContinuationPrefix(file, code))
	assert.Equal(t, "> >\n", lint.BlankLineText(file, deep))
	assert.Equal(t, "> > ", lint.ContinuationPrefix(file, deep))
}

func TestCountBlankLinesInContainer(t *testing.T) {
	t.Parallel()

	input := "> Para\n>\n> ## Heading\n>\n>\n> More\n"
	file := parseSource(t, input)
	heading := lint.Headings(file.Root)[0]

	assert.Equal(t, 1, lint.CountBlankLinesBeforeInContainer(file, heading, 3))
	assert.Equal(t, 2, lint.CountBlankLinesAfterInContainer(file, heading, 3))
}

func TestBlankLineTextKeepsBlockquoteIntact(t *testing.T) {
	t.Parallel()

	input := "> ## Heading\n> Para\n"
	file := parseSource(t, input)
	heading := lint.Headings(file.Root)[0]
	line := file.Lines[heading.SourcePosition().EndLine-1]

	fixed := input[:line.EndOffset] + lint.BlankLineText(file, heading) + input[line.EndOffset:]
	assert.Equal(t, "> ## Heading\n>\n> Para\n", fixed)

	refile := parseSource(t, fixed)
	require.Len(t, mdast.FindByKind(refile.Root, mdast.NodeBlockquote), 1, "the blockquote must not split")
}

// insertBlankLineAfter inserts BlankLineText(node) after node's last line and
// returns the result.
func insertBlankLineAfter(file *mdast.FileSnapshot, node *mdast.Node) string {
	line := file.Lines[node.SourcePosition().EndLine-1]
	content := string(file.Content)
	return content[:line.EndOffset] + lint.BlankLineText(file, node) + content[line.EndOffset:]
}

func TestContainerPrefixWithTabs(t *testing.T) {
	t.Parallel()

	// After ">", a tab counts one column as the optional space; its other
	// columns belong to the list item inside, so the inner '>' is a marker.
	input := ">\t- > # Heading\n>\t  > Text\n"
	file := parseSource(t, input)
	heading := lint.Headings(file.Root)[0]
	require.Len(t, mdast.FindByKind(file.Root, mdast.NodeBlockquote), 2)

	assert.Equal(t, ">     > ", lint.ContinuationPrefix(file, heading))
	assert.Equal(t, ">     >\n", lint.BlankLineText(file, heading))
	assert.Equal(t, ">\t- > ", string(lint.ContainerPrefix(file, heading, 1)))
	assert.False(t, lint.IsBlankLineInContainer(file, heading, 2))

	fixed := insertBlankLineAfter(file, heading)
	refile := parseSource(t, fixed)
	assert.Len(t, mdast.FindByKind(refile.Root, mdast.NodeBlockquote), 2, "the inserted line must stay inside the inner blockquote: %q", fixed)
	assert.True(t, lint.IsBlankLineInContainer(refile, lint.Headings(refile.Root)[0], 2))
}

func TestContainerPrefixAfterByteOrderMark(t *testing.T) {
	t.Parallel()

	const bom = "\xEF\xBB\xBF"
	input := bom + "> # H\n> Text\n"
	file := parseSource(t, input)
	heading := lint.Headings(file.Root)[0]

	assert.Equal(t, bom+"> ", string(lint.ContainerPrefix(file, heading, 1)), "the prefix keeps byte offsets")
	assert.Equal(t, "> ", lint.ContinuationPrefix(file, heading), "inserted text has no BOM")
	assert.Equal(t, ">\n", lint.BlankLineText(file, heading))

	fixed := insertBlankLineAfter(file, heading)
	assert.Equal(t, bom+"> # H\n>\n> Text\n", fixed)
	refile := parseSource(t, fixed)
	assert.Len(t, mdast.FindByKind(refile.Root, mdast.NodeBlockquote), 1, "the blockquote must not split")
}

// TestContainerPrefixAfterTabbedMarkers covers tabs after list and
// blockquote markers. goldmark measures the tab after a list marker from
// where the item starts, not from the start of the line, so the item's
// content can start fewer columns after the marker than its content
// indentation says.
func TestContainerPrefixAfterTabbedMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		quotes       int
		firstPrefix  string
		secondPrefix string
		continuation string
	}{
		{
			name:         "tab after a bullet in a blockquote",
			input:        "> -\t> # H\n>     > Text\n",
			quotes:       2,
			firstPrefix:  "> -\t> ",
			secondPrefix: ">     > ",
			continuation: ">     > ",
		},
		{
			name:         "tab after an ordered marker in a blockquote",
			input:        "> 1.\t> # H\n>     > Text\n",
			quotes:       2,
			firstPrefix:  "> 1.\t> ",
			secondPrefix: ">     > ",
			continuation: ">     > ",
		},
		{
			name:         "tab after a parenthesis marker in a blockquote",
			input:        "> 1)\t> # H\n>     > Text\n",
			quotes:       2,
			firstPrefix:  "> 1)\t> ",
			secondPrefix: ">     > ",
			continuation: ">     > ",
		},
		{
			name:         "tab after a bullet right after a blockquote marker",
			input:        ">-\t# H\n>     Text\n",
			quotes:       1,
			firstPrefix:  ">-\t",
			secondPrefix: ">     ",
			continuation: ">     ",
		},
		{
			name:         "bullet right after a blockquote marker",
			input:        ">- # H\n>   Text\n",
			quotes:       1,
			firstPrefix:  ">- ",
			secondPrefix: ">   ",
			continuation: ">   ",
		},
		{
			name:         "tab after a nested bullet",
			input:        "- -\t# H\n      Text\n",
			firstPrefix:  "- -\t",
			secondPrefix: "      ",
			continuation: "      ",
		},
		{
			name:         "tab after a blockquote marker in a list item",
			input:        "- >\t# H\n  >\tText\n",
			quotes:       1,
			firstPrefix:  "- >\t",
			secondPrefix: "  >\t",
			continuation: "  > ",
		},
		{
			name:         "tab partly claimed after a blockquote marker in an ordered item",
			input:        "1. >\t# H\n   >\tText\n",
			quotes:       1,
			firstPrefix:  "1. >",
			secondPrefix: "   >",
			continuation: "   > ",
		},
		{
			name:         "tab after a blockquote marker before a nested item",
			input:        "1. >\t- # H\n   >      Text\n",
			quotes:       1,
			firstPrefix:  "1. >\t- ",
			secondPrefix: "   >      ",
			continuation: "   >      ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file := parseSource(t, tt.input)
			heading := lint.Headings(file.Root)[0]
			require.Len(t, mdast.FindByKind(file.Root, mdast.NodeBlockquote), tt.quotes)
			require.Len(t, lint.Paragraphs(file.Root), 1)
			requireSameContainers(t, heading, lint.Paragraphs(file.Root)[0], len(containerChain(heading)), tt.input)

			assert.Equal(t, tt.firstPrefix, string(lint.ContainerPrefix(file, heading, 1)))
			assert.Equal(t, tt.secondPrefix, string(lint.ContainerPrefix(file, heading, 2)))
			assert.Equal(t, tt.continuation, lint.ContinuationPrefix(file, heading))
			assert.Equal(t, strings.TrimRight(tt.continuation, " ")+"\n", lint.BlankLineText(file, heading))
			assert.False(t, lint.IsBlankLineInContainer(file, heading, 1))
			assert.False(t, lint.IsBlankLineInContainer(file, heading, 2))

			fixed := insertBlankLineAfter(file, heading)
			refile := assertSameContainers(t, fixed, len(containerChain(heading)))
			assert.Len(t, mdast.FindByKind(refile.Root, mdast.NodeBlockquote), tt.quotes, "the inserted line must not split a blockquote: %q", fixed)
			assert.True(t, lint.IsBlankLineInContainer(refile, lint.Headings(refile.Root)[0], 2))
		})
	}
}

// TestContainerHelpersAcrossMarkerCombinations checks the container helpers
// against the parser for every nesting of up to three containers drawn from
// blockquote and list item openers with spaces and tabs after their
// markers, with and without a byte order mark.
func TestContainerHelpersAcrossMarkerCombinations(t *testing.T) {
	t.Parallel()

	openers := []string{
		"> ", ">\t", ">", " >\t",
		"- ", "-\t", "-   ", "- \t", "* ",
		"1. ", "1.\t", "10)\t", "1.  ",
	}
	var combos [][]string
	var build func(prefix []string)
	build = func(prefix []string) {
		if len(prefix) > 0 {
			combos = append(combos, append([]string(nil), prefix...))
		}
		if len(prefix) == 3 {
			return
		}
		for _, opener := range openers {
			build(append(prefix, opener))
		}
	}
	build(nil)

	for _, combo := range combos {
		for _, bom := range []string{"", mdast.ByteOrderMark} {
			input := bom + strings.Join(combo, "") + "# H\n"
			// Only a tab can keep a combination from nesting as written, by
			// turning the heading into indented code.
			if !checkContainerHelpers(t, input, len(combo)) {
				require.Contains(t, input, "\t", "%q does not nest as written", input)
			}
		}
	}
}

// checkContainerHelpers checks the container helpers on a heading that sits
// behind depth containers opened on its own line. It reports false, without
// checking anything, when the parser does not nest the heading that deep.
func checkContainerHelpers(t *testing.T, first string, depth int) bool {
	t.Helper()

	file := parseSource(t, first)
	headings := lint.Headings(file.Root)
	if len(headings) != 1 || len(containerChain(headings[0])) != depth {
		return false
	}
	heading := headings[0]
	quotes := 0
	for _, container := range containerChain(heading) {
		if container.Kind == mdast.NodeBlockquote {
			quotes++
		}
	}

	// The prefix holds every container marker and ends where the heading
	// starts, give or take indentation.
	prefix := string(lint.ContainerPrefix(file, heading, 1))
	start := heading.SourceRange().StartOffset
	require.LessOrEqual(t, len(prefix), start, "the prefix of %q runs into the heading: %q", first, prefix)
	require.Empty(t, strings.Trim(first[len(prefix):start], " \t"), "the prefix of %q stops before a marker: %q", first, prefix)
	require.Equal(t, quotes, strings.Count(prefix, ">"), "the prefix of %q misses a blockquote marker: %q", first, prefix)
	require.False(t, lint.IsBlankLineInContainer(file, heading, 1), "%q", first)

	// A line written with the continuation prefix continues every container,
	// and so does an inserted blank line.
	continuation := lint.ContinuationPrefix(file, heading)
	assertSameContainers(t, first+continuation+"Text\n", depth)
	inserted := first + lint.BlankLineText(file, heading) + continuation + "Text\n"
	refile := assertSameContainers(t, inserted, depth)
	require.True(t, lint.IsBlankLineInContainer(refile, lint.Headings(refile.Root)[0], 2), "%q", inserted)
	return true
}

// TestContainerHelpersOnContinuationLines checks the container helpers
// against the parser on a heading that continues the containers of the
// paragraph before it, for continuation prefixes built from blockquote
// markers, spaces and tabs.
func TestContainerHelpersOnContinuationLines(t *testing.T) {
	t.Parallel()

	openers := []string{"> ", ">\t", ">", "- ", "-\t", "1. ", "1.\t", "10)\t"}
	pieces := []string{">", "> ", ">\t", " ", "  ", "\t"}
	var firsts []string
	for _, outer := range openers {
		firsts = append(firsts, outer)
		for _, inner := range openers {
			firsts = append(firsts, outer+inner)
		}
	}
	// Every sequence of up to three pieces, each written once.
	seen := map[string]bool{"": true}
	continuations := []string{""}
	frontier := []string{""}
	for range 3 {
		var next []string
		for _, prefix := range frontier {
			for _, piece := range pieces {
				if !seen[prefix+piece] {
					seen[prefix+piece] = true
					next = append(next, prefix+piece)
				}
			}
		}
		continuations = append(continuations, next...)
		frontier = next
	}

	// Every opening line must be continued by at least one of the prefixes.
	for _, first := range firsts {
		checked := 0
		for _, continuation := range continuations {
			if checkContinuationLine(t, first+"P\n"+continuation+"# H\n") {
				checked++
			}
		}
		require.Positive(t, checked, "no prefix continues %q", first)
	}
}

// checkContinuationLine checks the container helpers on the heading on line
// 2 of input. It reports false, without checking anything, unless the
// heading sits in the same containers as the paragraph on line 1.
func checkContinuationLine(t *testing.T, input string) bool {
	t.Helper()

	file := parseSource(t, input)
	headings := lint.Headings(file.Root)
	paragraphs := lint.Paragraphs(file.Root)
	if len(headings) != 1 || len(paragraphs) != 1 {
		return false
	}
	heading := headings[0]
	chain := containerChain(heading)
	if len(chain) == 0 || heading.SourcePosition().StartLine != 2 {
		return false
	}
	if paragraphChain := containerChain(paragraphs[0]); len(paragraphChain) != len(chain) || paragraphChain[0] != chain[0] {
		return false
	}
	quotes := 0
	for _, container := range chain {
		if container.Kind == mdast.NodeBlockquote {
			quotes++
		}
	}

	line := string(lint.RawLineContent(file, 2))
	prefix := string(lint.ContainerPrefix(file, heading, 2))
	start := heading.SourceRange().StartOffset - file.Lines[1].StartOffset
	require.LessOrEqual(t, len(prefix), start, "the prefix of %q runs into the heading: %q", input, prefix)
	require.Empty(t, strings.Trim(line[len(prefix):start], " \t"), "the prefix of %q stops before a marker: %q", input, prefix)
	require.Equal(t, quotes, strings.Count(prefix, ">"), "the prefix of %q misses a blockquote marker: %q", input, prefix)
	require.False(t, lint.IsBlankLineInContainer(file, heading, 2), "%q", input)

	continuation := lint.ContinuationPrefix(file, heading)
	for _, extended := range []string{
		input + continuation + "Text\n",
		input + lint.BlankLineText(file, heading) + continuation + "Text\n",
	} {
		refile := parseSource(t, extended)
		reheadings := lint.Headings(refile.Root)
		reparagraphs := lint.Paragraphs(refile.Root)
		require.Len(t, reheadings, 1, "%q", extended)
		require.Len(t, reparagraphs, 2, "%q", extended)
		requireSameContainers(t, reheadings[0], reparagraphs[1], len(chain), extended)
	}
	return true
}

// assertSameContainers parses input, which must hold one heading and one
// paragraph, and requires both to sit in the same depth containers.
func assertSameContainers(t *testing.T, input string, depth int) *mdast.FileSnapshot {
	t.Helper()

	file := parseSource(t, input)
	headings := lint.Headings(file.Root)
	paragraphs := lint.Paragraphs(file.Root)
	require.Len(t, headings, 1, "%q", input)
	require.Len(t, paragraphs, 1, "%q", input)
	requireSameContainers(t, headings[0], paragraphs[0], depth, input)
	return file
}

// requireSameContainers requires a heading and a paragraph to sit in the
// same depth blockquotes and list items.
func requireSameContainers(t *testing.T, heading, paragraph *mdast.Node, depth int, input string) {
	t.Helper()

	headingChain, paragraphChain := containerChain(heading), containerChain(paragraph)
	require.Len(t, headingChain, depth, "the heading changed containers: %q", input)
	require.Len(t, paragraphChain, depth, "the text left a container: %q", input)
	if depth > 0 {
		require.Same(t, headingChain[0], paragraphChain[0], "the text left a container: %q", input)
	}
}

// containerChain returns the blockquotes and list items enclosing node,
// innermost first.
func containerChain(node *mdast.Node) []*mdast.Node {
	var chain []*mdast.Node
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind == mdast.NodeBlockquote || parent.Kind == mdast.NodeListItem {
			chain = append(chain, parent)
		}
	}
	return chain
}

func TestTableHelpersFindTableNodes(t *testing.T) {
	t.Parallel()

	file := parseSource(t, "# T\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n> | q |\n> |---|\n")
	tables := lint.Tables(file.Root)
	require.Len(t, tables, 2)
	assert.True(t, lint.IsTableNode(tables[0]))
	assert.False(t, lint.IsTableNode(tables[0].FirstChild), "a row is not a table")
	assert.True(t, lint.IsLineInTable(file, file.Root, 4))
	assert.True(t, lint.IsLineInTable(file, file.Root, 8))
	assert.False(t, lint.IsLineInTable(file, file.Root, 1))

	ctx := lint.NewRuleContext(context.Background(), file, config.NewConfig(), nil)
	assert.Equal(t, tables, ctx.Tables())
}

func TestMultilineHTMLCommentSpansItsClosingLine(t *testing.T) {
	t.Parallel()

	file := parseSource(t, "<!-- a\nb -->\nText\n")
	blocks := lint.HTMLBlocks(file.Root)
	require.Len(t, blocks, 1)
	assert.True(t, lint.IsHTMLComment(blocks[0]))
	assert.True(t, lint.IsCommentLine(file, 1))
	assert.True(t, lint.IsCommentLine(file, 2))
	assert.False(t, lint.IsCommentLine(file, 3))
}

func TestEngineMultilineDirectiveComment(t *testing.T) {
	t.Parallel()

	registry := lint.NewRegistry()
	registry.Register(rules.NewTrailingWhitespaceRule())
	input := "<!-- markdownlint-disable\n     MD009 -->\nbad  \n"
	result, err := lint.NewEngine(goldmark.New(goldmark.FlavorGFM), registry).LintFile(
		context.Background(), "test.md", []byte(input), config.NewConfig())
	require.NoError(t, err)
	assert.Empty(t, result.Diagnostics)
}
