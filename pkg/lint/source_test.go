package lint_test

import (
	"context"
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
