package goldmark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

func TestFrontmatterPreservesSource(t *testing.T) {
	input := []byte("---\ntitle: Example\n  # Metadata comment\n---\n# Document\n\nText.\n")
	file, err := New(FlavorGFM).Parse(context.Background(), "test.md", input)
	require.NoError(t, err)
	require.Equal(t, input, file.Content)
	require.True(t, mdast.ValidateTokens(file.Tokens, len(input)))
	headings := mdast.FindByKind(file.Root, mdast.NodeHeading)
	require.Len(t, headings, 1)
	require.Equal(t, 5, headings[0].SourcePosition().StartLine)
	require.Equal(t, "Document", string(headings[0].FirstChild.Inline.Text))
}

func TestEmptyAndNestedLinkPositions(t *testing.T) {
	input := "# Document\n\nBefore []() and []() and [**label**](target).\n"
	file, err := New(FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
	require.NoError(t, err)
	links := mdast.FindByKind(file.Root, mdast.NodeLink)
	require.Len(t, links, 3)
	for idx, col := range []int{8, 17, 26} {
		pos := links[idx].SourcePosition()
		require.Equal(t, 3, pos.StartLine)
		require.Equal(t, col, pos.StartColumn)
	}
}

func TestFrontmatterFormatsAndBoundaries(t *testing.T) {
	for _, metadata := range []string{
		"--- \ntitle: Example\n---\n",
		"+++\ntitle = 'Example'\n...\n",
		"{\n\"title\": \"Example\"\n}\n",
		"---\r\ntitle: Example\r\n---\r\n",
	} {
		t.Run(metadata, func(t *testing.T) {
			file, err := New(FlavorGFM).Parse(context.Background(), "test.md", []byte(metadata+"# Document\n"))
			require.NoError(t, err)
			require.Equal(t, mdast.NodeHeading, file.Root.FirstChild.Kind)
			require.Equal(t, 4, file.Root.FirstChild.SourcePosition().StartLine)
		})
	}
	file, err := New(FlavorGFM).Parse(context.Background(), "test.md", []byte("---\n# Document\n"))
	require.NoError(t, err)
	require.Equal(t, mdast.NodeThematicBreak, file.Root.FirstChild.Kind, "unterminated metadata stays Markdown")
}

func TestLinkRangesWithImagesAndReferences(t *testing.T) {
	for _, input := range []string{
		"[![alt](image.png)](target)",
		"[**bold** and `code`](target)",
		"[](target)",
		"[label][ref]\n\n[ref]: target",
		"[label\ncontinued](target)",
	} {
		t.Run(input, func(t *testing.T) {
			file, err := New(FlavorGFM).Parse(context.Background(), "test.md", []byte(input))
			require.NoError(t, err)
			links := mdast.FindByKind(file.Root, mdast.NodeLink)
			require.Len(t, links, 1)
			require.Equal(t, 1, links[0].SourcePosition().StartColumn)
			require.Equal(t, 0, links[0].SourceRange().StartOffset)
			require.Greater(t, links[0].SourceRange().EndOffset, 1)
		})
	}
}

func TestInlineHTMLPositionsWithinSharedToken(t *testing.T) {
	file, err := New(FlavorGFM).Parse(context.Background(), "test.md", []byte("<sup>text</sup>\n"))
	require.NoError(t, err)
	nodes := mdast.FindByKind(file.Root, mdast.NodeHTMLInline)
	require.Len(t, nodes, 2)
	require.Equal(t, "<sup>", string(nodes[0].Text()))
	require.Equal(t, "</sup>", string(nodes[1].Text()))
	require.Equal(t, 10, nodes[1].SourcePosition().StartColumn)
}
