package goldmark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// parseTokens parses content with the GFM parser and returns its tokens.
func parseTokens(t *testing.T, content string) ([]mdast.Token, []byte) {
	t.Helper()
	file, err := New(FlavorGFM).Parse(context.Background(), "tokens.md", []byte(content))
	require.NoError(t, err)
	require.True(t, mdast.ValidateTokens(file.Tokens, len(file.Content)), "tokens must cover the content")
	return file.Tokens, file.Content
}

// tokenTexts returns the text of every token of kind.
func tokenTexts(tokens []mdast.Token, content []byte, kind mdast.TokenKind) []string {
	var texts []string
	for _, tok := range tokens {
		if tok.Kind == kind {
			texts = append(texts, string(tok.Text(content)))
		}
	}
	return texts
}

func TestTokensFollowParsedCodeBlocks(t *testing.T) {
	t.Run("inline code at the start of a line is not a fence", func(t *testing.T) {
		tokens, content := parseTokens(t, "```foo``` bar\n\n---\n\nPara with _emph_\n")
		require.Empty(t, tokenTexts(tokens, content, mdast.TokCodeFence))
		require.Equal(t, []string{"---"}, tokenTexts(tokens, content, mdast.TokThematicBreak))
		require.Equal(t, []string{"_", "_"}, tokenTexts(tokens, content, mdast.TokEmphasisMarker))
	})
	t.Run("fence in a list item closes", func(t *testing.T) {
		tokens, content := parseTokens(t, "- item\n\n    ```sh\n    code\n    ```\n\nPara *one*\n")
		require.Equal(t, []string{"```", "```"}, tokenTexts(tokens, content, mdast.TokCodeFence))
		require.Equal(t, []string{"sh"}, tokenTexts(tokens, content, mdast.TokCodeFenceInfo))
		require.Equal(t, []string{"*", "*"}, tokenTexts(tokens, content, mdast.TokEmphasisMarker))
	})
	t.Run("code content is text", func(t *testing.T) {
		tokens, content := parseTokens(t, "~~~\n# not a heading\n---\n~~~\n\n***\n")
		require.Equal(t, []string{"~~~", "~~~"}, tokenTexts(tokens, content, mdast.TokCodeFence))
		require.Empty(t, tokenTexts(tokens, content, mdast.TokHeadingMarker))
		require.Equal(t, []string{"***"}, tokenTexts(tokens, content, mdast.TokThematicBreak))
		require.Contains(t, tokenTexts(tokens, content, mdast.TokText), "# not a heading")
	})
	t.Run("fence in a blockquote", func(t *testing.T) {
		tokens, content := parseTokens(t, "> ```js\n> x = 1\n> ```\n")
		require.Equal(t, []string{"```", "```"}, tokenTexts(tokens, content, mdast.TokCodeFence))
		require.Equal(t, []string{"js"}, tokenTexts(tokens, content, mdast.TokCodeFenceInfo))
		require.Equal(t, []string{">", ">", ">"}, tokenTexts(tokens, content, mdast.TokBlockquoteMarker))
		require.Contains(t, tokenTexts(tokens, content, mdast.TokText), "x = 1")
	})
	t.Run("indented code", func(t *testing.T) {
		tokens, content := parseTokens(t, "Para\n\n    # not a heading\n")
		require.Empty(t, tokenTexts(tokens, content, mdast.TokHeadingMarker))
		require.Contains(t, tokenTexts(tokens, content, mdast.TokText), "# not a heading")
	})
}

func TestTokensFollowParsedHTML(t *testing.T) {
	t.Run("inline HTML at the start of a paragraph", func(t *testing.T) {
		tokens, content := parseTokens(t, "<kbd>Ctrl</kbd> then _two_ here.\n")
		require.Equal(t, []string{"<kbd>", "</kbd>"}, tokenTexts(tokens, content, mdast.TokHTML))
		require.Equal(t, []string{"_", "_"}, tokenTexts(tokens, content, mdast.TokEmphasisMarker))
	})
	t.Run("HTML block", func(t *testing.T) {
		tokens, content := parseTokens(t, "<div>\n# not a heading\n</div>\n\n# Heading\n")
		require.Equal(t, []string{"<div>", "# not a heading", "</div>"}, tokenTexts(tokens, content, mdast.TokHTML))
		require.Equal(t, []string{"#"}, tokenTexts(tokens, content, mdast.TokHeadingMarker))
	})
	t.Run("multi-line comment with closing line", func(t *testing.T) {
		tokens, content := parseTokens(t, "<!-- a\n---\n-->\n")
		require.Equal(t, []string{"<!-- a", "---", "-->"}, tokenTexts(tokens, content, mdast.TokHTML))
		require.Empty(t, tokenTexts(tokens, content, mdast.TokThematicBreak))
	})
}

func TestFrontMatterIsNotTokenizedAsMarkdown(t *testing.T) {
	tokens, content := parseTokens(t, "---\ntitle: x\n---\n# Doc\n\n***\n")
	require.Equal(t, []string{"***"}, tokenTexts(tokens, content, mdast.TokThematicBreak))
	require.Equal(t, []string{"#"}, tokenTexts(tokens, content, mdast.TokHeadingMarker))
}
