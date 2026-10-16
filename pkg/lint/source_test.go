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
