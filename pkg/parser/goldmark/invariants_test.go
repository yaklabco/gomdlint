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
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// invariantInputs are documents that stress the span contract beyond the
// golden fixtures: containers, lazy lines, breaks, GFM extensions and
// degenerate blocks.
var invariantInputs = map[string]string{
	"containers": "> - item one\n>   continued\n> - item two\n>\n>   para\n\n- > quote in item\n  > more\n",
	"lazy":       "> a\nb\n\n- c\nd\n",
	"breaks":     "one  \ntwo\\\nthree\nfour\r\nfive\n",
	"setext":     "First\nsecond line\n-----\n\nOne\n===\n",
	"fences":     "```\n```\n\n~~~~ js\nx\n~~~~~\n\n- ```\n  code\n\n- next\n\n> ```\n> unclosed\n",
	"html":       "<div>\nx\n</div>\n\n<!-- a\nb -->\nPara\n  <p>indented</p>\n",
	"empty":      "#\n\n-\n- \n\n>\n\n***\n",
	"links":      "[a](b) [c][d] [d][] [d] ![e][d] <https://x.y> [multi\nline][]\n\n[d]: https://x.y\n[multi line]: /m\n",
	"defs":       "- [a]: https://example.com\n- b\n\n[x]: https://x.y\nText after\n",
	"inline":     "*a* **b** ***c*** _d_ `e` `` f`g `` <b>h</b> \\*i\\*\n",
	"residuals": "*foo**\n\n**foo***\n\n*foo****\n\n__foo___\n\n_foo____\n\na *b** c\n\n" +
		"*foo**bar\n\n**foo*\n\n***foo**\n\n*foo **bar***\n\n[*x**](u) and ~~y~~~ and *a**b*c**\n",
	"gfm":    "| a | b |\n| - | - |\n| 1 |\n\n- [ ] task\n- [x] done\n\n~~gone~~ www.example.com and https://x.y/z.\n",
	"tables": "Lead line\n| a | b |\n|---|---|\n| `x\\|y` | 2 |\n\n> | q |\n> |---|\n> | r |\n",
	"bom":    "\xEF\xBB\xBF# Title\n\nText\n",
	"tabs":   "-\tone\n\n\tcode\n>\tquote\n",
	"items":  "> -\t> # H\n>     > Text\n\n> 1.\t> q\n\n- -\t# H\n\n1. >\t- x\n\n-\t\tcode\n\n- [a]: /u\n  text\n\n- [b]: /v\n",
	"crlf":   "# Title\r\n\r\n- a\r\n- b\r\n\r\n```\r\nx\r\n```\r\n",
}

func TestSpanInvariants(t *testing.T) {
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
	require.Greater(t, len(inputs), len(invariantInputs), "no testdata inputs found under %s", root)

	for name, content := range inputs {
		for _, flavor := range []string{FlavorCommonMark, FlavorGFM} {
			t.Run(flavor+"/"+name, func(t *testing.T) {
				file, parseErr := New(flavor).Parse(context.Background(), "invariants.md", content)
				require.NoError(t, parseErr)
				checkNodeSpans(t, file, file.Root)
				checkTextSegmentsAgree(t, flavor, file)
			})
		}
	}
}

// checkNodeSpans verifies the span contract for node and its descendants.
func checkNodeSpans(t *testing.T, file *mdast.FileSnapshot, node *mdast.Node) {
	t.Helper()
	require.NotNil(t, node.SourceSpan, "%s has no source span", node.Kind)
	full := node.SourceRange()
	content := node.ContentRange()
	where := func() string {
		return node.Kind.String() + " " + strings.TrimSpace(string(file.Content[max(0, full.StartOffset):min(len(file.Content), full.EndOffset)]))
	}
	require.LessOrEqual(t, 0, full.StartOffset, where())
	require.LessOrEqual(t, full.StartOffset, full.EndOffset, "inverted span: %s", where())
	require.LessOrEqual(t, full.EndOffset, len(file.Content), where())
	require.LessOrEqual(t, full.StartOffset, content.StartOffset, "content starts before span: %s", where())
	require.LessOrEqual(t, content.StartOffset, content.EndOffset, "inverted content: %s", where())
	require.LessOrEqual(t, content.EndOffset, full.EndOffset, "content ends after span: %s", where())
	if len(file.Tokens) > 0 {
		require.LessOrEqual(t, 0, node.FirstToken, where())
		require.LessOrEqual(t, node.FirstToken, node.LastToken, "inverted token range: %s", where())
		tokens := file.Tokens
		require.LessOrEqual(t, tokens[node.FirstToken].StartOffset, full.StartOffset, "first token after span: %s", where())
		require.LessOrEqual(t, full.EndOffset, max(tokens[node.LastToken].EndOffset, full.StartOffset), "last token before span end: %s", where())
	} else {
		require.Equal(t, -1, node.FirstToken, "token range without tokens: %s", where())
	}

	checkMarkers(t, file, node)

	prevEnd := content.StartOffset
	for child := node.FirstChild; child != nil; child = child.Next {
		require.NotNil(t, child.SourceSpan, "%s child of %s has no span", child.Kind, where())
		span := child.SourceRange()
		require.LessOrEqual(t, content.StartOffset, span.StartOffset, "%s starts before the content of %s", child.Kind, where())
		require.LessOrEqual(t, span.EndOffset, content.EndOffset, "%s ends after the content of %s", child.Kind, where())
		require.LessOrEqual(t, prevEnd, span.StartOffset, "%s overlaps its previous sibling in %s", child.Kind, where())
		prevEnd = span.EndOffset
		checkNodeSpans(t, file, child)
	}
}

// checkMarkers verifies that block and inline spans start with their syntax
// markers.
func checkMarkers(t *testing.T, file *mdast.FileSnapshot, node *mdast.Node) {
	t.Helper()
	src := node.Text()
	if len(src) == 0 {
		return
	}
	switch node.Kind {
	case mdast.NodeHeading:
		if src[0] != '#' {
			lines := bytes.Split(src, []byte("\n"))
			underline := bytes.TrimLeft(bytes.TrimSpace(lines[len(lines)-1]), "> ")
			require.Greater(t, len(lines), 1, "setext heading without underline: %q", src)
			require.True(t, len(bytes.Trim(underline, "=")) == 0 || len(bytes.Trim(underline, "-")) == 0,
				"setext heading does not end with its underline: %q", src)
		}
	case mdast.NodeListItem:
		require.True(t, strings.IndexByte("-+*", src[0]) >= 0 || (src[0] >= '0' && src[0] <= '9'),
			"list item does not start with its marker: %q", src)
	case mdast.NodeBlockquote:
		require.Equal(t, byte('>'), src[0], "blockquote does not start with '>': %q", src)
	case mdast.NodeCodeBlock:
		checkCodeBlockMarkers(t, node, src)
	case mdast.NodeThematicBreak:
		marks := bytes.Map(func(r rune) rune {
			if r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, src)
		require.GreaterOrEqual(t, len(marks), 3, "thematic break too short: %q", src)
		require.Empty(t, bytes.Trim(marks, string(marks[0])), "mixed thematic break: %q", src)
	case mdast.NodeHTMLBlock, mdast.NodeHTMLInline:
		require.Equal(t, byte('<'), src[0], "HTML does not start with '<': %q", src)
	case mdast.NodeEmphasis, mdast.NodeStrong:
		delimiters := "*_"
		require.Contains(t, delimiters, string(src[0]), "emphasis does not start with a delimiter: %q", src)
		require.Contains(t, delimiters, string(src[len(src)-1]), "emphasis does not end with a delimiter: %q", src)
	case mdast.NodeStrikethrough:
		require.Equal(t, byte('~'), src[0], "strikethrough does not start with '~': %q", src)
		require.Equal(t, byte('~'), src[len(src)-1], "strikethrough does not end with '~': %q", src)
	case mdast.NodeCodeSpan:
		require.Equal(t, byte('`'), src[0], "code span: %q", src)
		require.Equal(t, byte('`'), src[len(src)-1], "code span: %q", src)
	case mdast.NodeImage:
		require.True(t, bytes.HasPrefix(src, []byte("![")), "image: %q", src)
	case mdast.NodeLink:
		if node.Inline.Link.ReferenceStyle != mdast.RefStyleAutolink {
			require.Equal(t, byte('['), src[0], "link: %q", src)
		}
	case mdast.NodeSoftBreak, mdast.NodeHardBreak:
		require.Equal(t, byte('\n'), src[len(src)-1], "break does not end with a line ending: %q", src)
	}
	_ = file
}

func checkCodeBlockMarkers(t *testing.T, node *mdast.Node, src []byte) {
	t.Helper()
	attrs := node.Block.CodeBlock
	if attrs.Indented {
		require.Contains(t, " \t", string(src[0]), "indented code does not start with its indentation: %q", src)
		return
	}
	fence := bytes.Repeat([]byte{attrs.FenceChar}, attrs.FenceLength)
	require.True(t, bytes.HasPrefix(src, fence), "fenced code does not start with its fence %q: %q", fence, src)
	if attrs.Closed {
		lines := bytes.Split(src, []byte("\n"))
		closing := bytes.TrimLeft(bytes.TrimRight(lines[len(lines)-1], " \t\r"), " \t>")
		require.GreaterOrEqual(t, len(closing), attrs.FenceLength, "closing fence too short: %q", src)
		require.Empty(t, bytes.Trim(closing, string(attrs.FenceChar)), "closed code does not end with its fence: %q", src)
	}
}

// checkTextSegmentsAgree parses the content again with plain goldmark and
// checks that every text segment goldmark produced is the span of a Text
// node, so spans agree with goldmark's own positions.
func checkTextSegmentsAgree(t *testing.T, flavor string, file *mdast.FileSnapshot) {
	t.Helper()
	spans := make(map[mdast.SourceRange]bool)
	//nolint:errcheck // The visitor never returns an error.
	mdast.Walk(file.Root, func(node *mdast.Node) error {
		if node.Kind == mdast.NodeText {
			spans[node.SourceRange()] = true
		}
		return nil
	})

	base := bomLength(file.Content)
	source := blankedSource(file)[base:]
	md := newGoldmarkInstance(flavor, spanKey{key: spanContextKey})
	doc := md.Parser().Parse(text.NewReader(source))
	//nolint:errcheck // The visitor never returns an error.
	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, code := node.(*ast.CodeSpan); code {
			return ast.WalkSkipChildren, nil
		}
		textNode, ok := node.(*ast.Text)
		if !ok || textNode.Segment.IsEmpty() {
			return ast.WalkContinue, nil
		}
		want := mdast.SourceRange{StartOffset: textNode.Segment.Start + base, EndOffset: textNode.Segment.Stop + base}
		require.True(t, spans[want] || movedResidual(file.Content, spans, want),
			"no Text node spans goldmark segment %q at %v", source[textNode.Segment.Start:textNode.Segment.Stop], want)
		return ast.WalkContinue, nil
	})
}

// movedResidual reports whether the goldmark segment want is the leftover
// part of a closing delimiter run that the mapper moved to the end of its
// run: goldmark leaves such segments at the start of the run.
func movedResidual(content []byte, spans map[mdast.SourceRange]bool, want mdast.SourceRange) bool {
	if want.IsEmpty() || strings.IndexByte("*_~", content[want.StartOffset]) < 0 {
		return false
	}
	char := content[want.StartOffset]
	runEnd := want.StartOffset
	for runEnd < len(content) && content[runEnd] == char {
		runEnd++
	}
	if runEnd < want.EndOffset {
		return false
	}
	for start := want.StartOffset + 1; start+want.Len() <= runEnd; start++ {
		if spans[mdast.SourceRange{StartOffset: start, EndOffset: start + want.Len()}] {
			return true
		}
	}
	return false
}

// blankedSource returns the content goldmark parses: front matter blanked.
func blankedSource(file *mdast.FileSnapshot) []byte {
	source := bytes.Clone(file.Content)
	if file.FrontMatterLines > 0 {
		for idx := range file.Lines[file.FrontMatterLines-1].EndOffset {
			if source[idx] != '\n' && source[idx] != '\r' {
				source[idx] = ' '
			}
		}
	}
	return source
}
