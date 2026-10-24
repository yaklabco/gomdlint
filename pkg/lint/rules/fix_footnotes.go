package rules

import (
	"bytes"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmextast "github.com/yuin/goldmark/extension/ast"
	goldparser "github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"
)

// footnoteBlockPriority is the priority that goldmark's footnote extension
// gives its block parser.
const footnoteBlockPriority = 999

// maxFootnoteNesting is the most footnote definitions that may start on one
// line before parseFootnotes gives up without parsing: goldmark takes time
// quadratic in their nesting, and no document needs more.
const maxFootnoteNesting = 16

// footnoteParser is the GFM parser that reads footnote definitions, built on
// first use (see footnoteMarkdown).
//
//nolint:gochecknoglobals // A goldmark parser is safe for concurrent parses and costly to build.
var (
	footnoteOnce   sync.Once
	footnoteParser goldmark.Markdown
)

// footnoteInPlace is goldmark's footnote block parser, but for closing a
// definition, which leaves it where it is defined. goldmark's parser moves
// it into a list beside the first definition it closed, which for nested
// definitions lies inside the one it moves, so that every later definition
// drops out of the document.
type footnoteInPlace struct {
	goldparser.BlockParser
}

// Close leaves the definition where it is.
func (footnoteInPlace) Close(gmast.Node, gmtext.Reader, goldparser.Context) {}

// footnoteMarkdown returns a GFM parser that also reads every footnote
// definition that goldmark reads, where it is defined, whether a reference
// matches its label or not. Only the fix check's HTML scan uses it: it
// finds the raw HTML of a footnote, which the signature's parsers may read
// as a link reference definition or as code, so that the check sees it (see
// htmlHiddenFromCheck). goldmark's footnote extension would drop a definition that no reference
// matches exactly, but GitHub folds the case of labels, so this parser
// leaves out the extension's step that matches them.
func footnoteMarkdown() goldmark.Markdown {
	footnoteOnce.Do(func() {
		footnoteParser = goldmark.New(
			goldmark.WithExtensions(gmext.GFM),
			goldmark.WithParserOptions(goldparser.WithBlockParsers(gmutil.Prioritized(
				footnoteInPlace{gmext.NewFootnoteBlockParser()}, footnoteBlockPriority,
			))),
		)
	})
	return footnoteParser
}

// parseFootnotes parses source with footnoteMarkdown and reports whether a
// footnote definition may start where the document it returns holds none
// that goldmark reads as cmark-gfm, which GitHub uses, does (see
// mayStartFootnotes). goldmark rejects some labels that cmark-gfm reads,
// such as one with a "[" in a code span, reads an escaped "]" in a label
// that ends it for cmark-gfm, and keeps the spaces after the label that
// cmark-gfm skips, which moves the columns of a block that starts there
// (see blockAfterLabel). Where goldmark does not read a definition as
// GitHub does, its raw HTML may look like code or text, so the fix check
// cannot see it. A line with more than maxFootnoteNesting definitions
// counts as unread without a parse, and the document it returns is then
// nil.
func parseFootnotes(source []byte) (gmast.Node, bool) {
	candidates := mayStartFootnotes(source)
	if deepestFootnoteNesting(source, candidates) > maxFootnoteNesting {
		return nil, true
	}
	doc := footnoteMarkdown().Parser().Parse(gmtext.NewReader(source))
	read := make(map[int]bool)
	//nolint:errcheck // The visitor never returns an error.
	_ = gmast.Walk(doc, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) {
		footnote, ok := node.(*gmextast.Footnote)
		if !ok || !entering || bytes.IndexByte(footnote.Ref, ']') >= 0 {
			return gmast.WalkContinue, nil
		}
		// goldmark puts a definition after the columns left of a tab that a
		// container takes in part, which are no "[".
		start := bytes.LastIndexByte(source[:min(node.Pos()+1, len(source))], '[')
		if start >= 0 && !blockAfterLabel(source, start) {
			read[start] = true
		}
		return gmast.WalkContinue, nil
	})
	for _, offset := range candidates {
		if !read[offset] {
			return doc, true
		}
	}
	return doc, false
}

// blockAfterLabel reports whether, on the line of start, a "]:" after start
// is followed by a tab, by four spaces or more, or by up to three spaces and
// a character that may start a block other than a paragraph. cmark-gfm
// skips the spaces and tabs before a definition's text, and goldmark does
// not, so a list item there takes other columns, and goldmark may read an
// indented code block where cmark-gfm reads text. A paragraph reads the same.
func blockAfterLabel(source []byte, start int) bool {
	line := source[start:]
	if end := bytes.IndexAny(line, "\r\n"); end >= 0 {
		line = line[:end]
	}
	for {
		idx := bytes.Index(line, []byte("]:"))
		if idx < 0 {
			return false
		}
		line = line[idx+2:]
		spaces := 0
		for spaces < len(line) && line[spaces] == ' ' {
			spaces++
		}
		switch {
		case spaces >= codeBlockIndent:
			return true
		case spaces < len(line) && (isASCIIDigit(line[spaces]) || strings.IndexByte(blockStarts, line[spaces]) >= 0):
			return true
		}
	}
}

// blockStarts holds the characters that may start a block other than a
// paragraph, and a tab, which may make an indented code block.
const blockStarts = "\t-+*_>#`~<|=[\\"

// deepestFootnoteNesting returns the most offsets of offsets, which are
// sorted, that one line holds.
func deepestFootnoteNesting(source []byte, offsets []int) int {
	deepest, count := 0, 0
	for idx, offset := range offsets {
		if idx == 0 || bytes.ContainsAny(source[offsets[idx-1]:offset], "\r\n") {
			count = 0
		}
		count++
		deepest = max(deepest, count)
	}
	return deepest
}

// mayStartFootnotes returns the offsets of every "[^" where a footnote
// definition may start in some renderer, by a loose test that finds more
// than any renderer reads: only whitespace, blockquote and list markers,
// and the "]:" that ends the label of another definition precede it on its
// line, and "]:" follows it before the next empty line. A line ends at a
// line feed or a carriage return.
func mayStartFootnotes(source []byte) []int {
	var offsets []int
	blankLF, blankCRLF, blankCR, colon := -1, -1, -1, -1
	for pos := 0; pos+1 < len(source); pos++ {
		if source[pos] != '[' || source[pos+1] != '^' || !afterContainerMarkers(source, pos) {
			continue
		}
		// Each search starts where the last ended, so the scan is linear.
		blankLF = nextIndexFrom(source, blankLF, pos, []byte("\n\n"))
		blankCRLF = nextIndexFrom(source, blankCRLF, pos, []byte("\r\n\r\n"))
		blankCR = nextIndexFrom(source, blankCR, pos, []byte("\r\r"))
		colon = nextIndexFrom(source, colon, pos, []byte("]:"))
		if colon < min(blankLF, blankCRLF, blankCR) {
			offsets = append(offsets, pos)
		}
	}
	return offsets
}

// nextIndexFrom returns the offset of the first sep in source at or after
// pos, or len(source) when there is none, given found, the offset it
// returned for an earlier pos, or -1.
func nextIndexFrom(source []byte, found, pos int, sep []byte) int {
	if found >= pos {
		return found
	}
	if idx := bytes.Index(source[pos:], sep); idx >= 0 {
		return pos + idx
	}
	return len(source)
}

// afterContainerMarkers reports whether only whitespace, the characters of
// blockquote and list markers, and a "]:" precede pos on its line, back to
// the line start or to that "]:".
func afterContainerMarkers(source []byte, pos int) bool {
	for idx := pos - 1; idx >= 0; idx-- {
		switch char := source[idx]; {
		case char == '\n' || char == '\r':
			return true
		case char == ':' && idx > 0 && source[idx-1] == ']':
			return true
		case strings.IndexByte(" \t>-+*.)", char) >= 0 || isASCIIDigit(char):
		default:
			return false
		}
	}
	return true
}
