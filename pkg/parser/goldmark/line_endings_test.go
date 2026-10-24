package goldmark

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// TestBareCarriageReturnsEndLines checks that a carriage return that no line
// feed follows ends a line for the parser, as it does for CommonMark and
// markdownlint, while the snapshot keeps the original bytes.
func TestBareCarriageReturnsEndLines(t *testing.T) {
	input := "# Title\rText\r\r- a\r- b\r\n\r```\rcode\r```\r"
	for _, flavor := range []string{FlavorCommonMark, FlavorGFM} {
		t.Run(flavor, func(t *testing.T) {
			file, err := New(flavor).Parse(context.Background(), "cr.md", []byte(input))
			require.NoError(t, err)
			require.Equal(t, input, string(file.Content))
			require.Len(t, file.Lines, 10)

			var kinds []mdast.NodeKind
			for child := file.Root.FirstChild; child != nil; child = child.Next {
				kinds = append(kinds, child.Kind)
			}
			require.Equal(t, []mdast.NodeKind{mdast.NodeHeading, mdast.NodeParagraph, mdast.NodeList, mdast.NodeCodeBlock}, kinds)

			heading := file.Root.FirstChild
			require.Equal(t, "# Title", string(heading.Text()))
			require.Equal(t, "Title", string(heading.ContentText()))
			require.Equal(t, mdast.SourcePosition{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 8}, heading.SourcePosition())

			paragraph := heading.Next
			require.Equal(t, "Text", string(paragraph.Text()))
			require.Equal(t, 2, paragraph.SourcePosition().StartLine)

			list := paragraph.Next
			require.Equal(t, 2, list.ChildCount())
			require.Equal(t, mdast.SourcePosition{StartLine: 4, StartColumn: 1, EndLine: 5, EndColumn: 4}, list.SourcePosition())

			code := list.Next
			require.True(t, code.Block.CodeBlock.Closed)
			require.Equal(t, "code", string(code.ContentText()))
			require.Equal(t, 7, code.SourcePosition().StartLine)
			require.Equal(t, 9, code.SourcePosition().EndLine)
		})
	}
}

// TestBareCarriageReturnsParseLikeLineFeeds checks that a document whose
// lines end with carriage returns alone, or with a mix of carriage returns
// and line feeds, parses to the tree, tokens and lines of the same document
// with line feeds. The documents have the same length, so every offset
// agrees.
func TestBareCarriageReturnsParseLikeLineFeeds(t *testing.T) {
	inputs := make(map[string][]byte)
	for name, input := range invariantInputs {
		inputs[name] = []byte(input)
	}
	root := filepath.Join("..", "..", "lint", "rules", "testdata")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".input.md") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		inputs[path] = content
		return nil
	})
	require.NoError(t, err)

	for name, lf := range inputs {
		if bytes.IndexByte(lf, '\r') >= 0 {
			continue
		}
		variants := map[string][]byte{"cr": carriageReturnsFor(lf, func(int) bool { return true }), "mixed": carriageReturnsFor(lf, func(idx int) bool { return idx%2 == 1 })}
		for _, flavor := range []string{FlavorCommonMark, FlavorGFM} {
			want, wantErr := New(flavor).Parse(context.Background(), "lf.md", lf)
			require.NoError(t, wantErr, name)
			for variant, content := range variants {
				t.Run(flavor+"/"+variant+"/"+name, func(t *testing.T) {
					got, gotErr := New(flavor).Parse(context.Background(), "lf.md", content)
					require.NoError(t, gotErr)
					require.Equal(t, want.Lines, got.Lines)
					require.Equal(t, want.FrontMatterLines, got.FrontMatterLines)
					require.Equal(t, want.Tokens, got.Tokens)
					requireSameTree(t, want.Root, got.Root, "document")
					checkNodeSpans(t, got, got.Root)
				})
			}
		}
	}
}

// carriageReturnsFor returns content with the line feeds whose index among
// the line feeds convert reports true written as carriage returns. A line
// feed that another line feed follows stays when that one does, so that no
// carriage return and line feed pair forms.
func carriageReturnsFor(content []byte, convert func(idx int) bool) []byte {
	out := bytes.Clone(content)
	lineFeed := 0
	for idx, char := range out {
		if char != '\n' {
			continue
		}
		if convert(lineFeed) && (idx+1 == len(out) || out[idx+1] != '\n' || convert(lineFeed+1)) {
			out[idx] = '\r'
		}
		lineFeed++
	}
	return out
}

// requireSameTree checks that two trees hold the same nodes with the same
// spans, token ranges and attributes. A reference label keeps the source's
// line endings, so labels compare with carriage returns alone read as line
// feeds.
func requireSameTree(t *testing.T, want, got *mdast.Node, path string) {
	t.Helper()
	path += "/" + want.Kind.String()
	require.Equal(t, want.Kind, got.Kind, path)
	require.Equal(t, want.SourceSpan, got.SourceSpan, path)
	require.Equal(t, want.ContentSpan, got.ContentSpan, path)
	require.Equal(t, want.FirstToken, got.FirstToken, path)
	require.Equal(t, want.LastToken, got.LastToken, path)
	require.Equal(t, want.Block, got.Block, path)
	gotInline := got.Inline
	if gotInline != nil && gotInline.Link != nil {
		link := *gotInline.Link
		link.ReferenceLabel = mdast.LineFeedText(link.ReferenceLabel)
		inline := *gotInline
		inline.Link = &link
		gotInline = &inline
	}
	require.Equal(t, want.Inline, gotInline, path)
	require.Equal(t, want.ChildCount(), got.ChildCount(), path)
	for wantChild, gotChild := want.FirstChild, got.FirstChild; wantChild != nil; wantChild, gotChild = wantChild.Next, gotChild.Next {
		requireSameTree(t, wantChild, gotChild, path)
	}
}
