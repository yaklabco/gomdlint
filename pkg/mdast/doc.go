// Package mdast provides the core Markdown AST representation for gomdlint.
//
// It defines a lossless, immutable view of a Markdown file:
//   - FileSnapshot: the file content, its lines, token stream and AST.
//   - Token stream: a classification of every byte of the content.
//   - Nodes: the document structure, each with exact source spans.
//
// The Parser interface lives in the lint package (lint.Parser), the
// consumer of parsers. Parser implementations implement lint.Parser.
//
// # Source spans
//
// Every node produced by the parser carries two byte ranges into
// FileSnapshot.Content. Offsets are byte offsets, and ranges are half-open:
// StartOffset is included and EndOffset is not. Columns derived from them
// (SourcePosition, ContentPosition) are 1-based byte columns; the end
// position is exclusive.
//
// The full span (Node.SourceRange, Node.Text, Node.SourcePosition) is the
// exact source of the construct, including its syntax markers. The content
// span (Node.ContentRange, Node.ContentText, Node.ContentPosition) is the
// part inside the markers. When a node has no markers the two are equal.
//
// Spans are always well formed: 0 <= StartOffset <= EndOffset <=
// len(Content), the content span lies inside the full span, a child's full
// span lies inside its parent's full span, and siblings appear in source
// order without overlapping. A node with no source of its own has an empty
// span where it sits.
//
// Block spans start at the block's first marker (or first non-blank byte
// when it has none), after any container prefix such as "> " or list
// indentation, and end at the end of the block's last line, excluding the
// line ending. They never end on a blank line, except that fenced code owns
// the blank lines of its content. Lines inside a container keep their
// container prefix in the middle of a multi-line span.
//
// Per node kind:
//
//   - Document: the whole content. The content span equals it.
//   - Heading (ATX): from the first '#' to the end of the line, including
//     any closing sequence. Content: the heading text without the opening
//     and closing sequences; empty when there is no text.
//   - Heading (setext): from the first text line through the underline.
//     Content: the text lines, without trailing whitespace.
//   - Paragraph: from the first non-blank byte to the end of its last line.
//     Content: the same range without trailing whitespace on the last line.
//   - List: from the first item's marker to the end of the last item. Blank
//     lines after the last item are not included.
//   - ListItem: from the marker to the end of the item's last line.
//     Content: from the start of the first child to the end of the last
//     child, or empty after the marker when the item has no children.
//   - Blockquote: from the first '>' to the end of its last line, including
//     lines that hold only '>' and lazy continuation lines. Content: from the
//     first to the last child, or empty at the end when it has none.
//   - CodeBlock (fenced): from the opening fence to the closing fence, or to
//     the last content line when the block is unclosed (see
//     CodeBlockAttrs.Closed). Content: the content lines, from the first
//     content byte after the fence indentation to the end of the last
//     content line; empty, at the end of the opening fence line, when there
//     are no content lines. FenceChar and FenceLength describe the opening
//     fence.
//   - CodeBlock (indented): from the start of the indentation of the first
//     line to the end of the last non-blank line. Content: from the first
//     code byte after the four columns of indentation.
//   - ThematicBreak: from the first marker character to the end of the line.
//   - HTMLBlock: from '<' to the end of the last line of the block,
//     including a closing line such as "-->".
//   - Text: the literal text.
//   - SoftBreak and HardBreak: the text that ends the line, followed by the
//     line ending, so they end at the start of the next line.
//   - Emphasis and Strong: including their delimiters. Content: the text
//     between the delimiters.
//   - CodeSpan: including the backtick runs. Content: everything between
//     the runs, including any padding space.
//   - Link and Image: from '[' (or "![") through the destination or the
//     reference label, so "[text](url)", "[text][ref]", "[ref][]" and "[ref]"
//     are covered whole. Content: the link text or image description between
//     the brackets.
//   - Link (autolink): "<https://example.com>" including the angle
//     brackets; content is the address. An extended (GFM) autolink covers
//     the address only.
//   - HTMLInline: the raw HTML, such as "<b>" or "</b>".
//
// Positions on a line are byte columns. Converting them to character
// columns is the job of the reporting layer.
package mdast
