package goldmark

import (
	"sort"

	"github.com/yaklabco/gomdlint/pkg/mdast"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// tokenRegions returns the parts of the content whose token kind follows
// from the parsed document: front matter lines (TokText), code fences
// (TokCodeFence) and info strings (TokCodeFenceInfo), lines of code
// (TokText) and lines of HTML blocks (TokHTML). Guessing these line by line
// goes wrong whenever a line merely looks like a fence or a tag, and inside
// containers. The regions are sorted and do not overlap.
func (m *mapper) tokenRegions(gmDoc ast.Node, frontMatterLines int) []tokenRegion {
	var regions []tokenRegion
	for idx := 0; idx < frontMatterLines && idx < len(m.lines); idx++ {
		line := m.lines[idx]
		regions = appendRegion(regions, line.StartOffset, line.NewlineStart, mdast.TokText)
	}

	//nolint:errcheck // The visitor never returns an error.
	ast.Walk(gmDoc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch block := node.(type) {
		case *ast.FencedCodeBlock:
			regions = m.fencedCodeRegions(regions, block)
		case *ast.CodeBlock:
			regions = m.lineRegions(regions, block.Lines(), mdast.TokText, false)
		case *ast.HTMLBlock:
			regions = m.lineRegions(regions, block.Lines(), mdast.TokHTML, true)
			if block.HasClosure() {
				closure := text.NewSegments()
				closure.Append(block.ClosureLine)
				regions = m.lineRegions(regions, closure, mdast.TokHTML, true)
			}
		default:
			return ast.WalkContinue, nil
		}
		return ast.WalkSkipChildren, nil
	})

	sort.SliceStable(regions, func(i, j int) bool {
		return regions[i].start < regions[j].start
	})
	return regions
}

// fencedCodeRegions adds the opening fence, the info string, the lines of
// code and the closing fence of a fenced code block.
func (m *mapper) fencedCodeRegions(regions []tokenRegion, block *ast.FencedCodeBlock) []tokenRegion {
	extent := m.block(block)
	if extent == nil {
		return m.lineRegions(regions, block.Lines(), mdast.TokText, false)
	}
	opening := m.abs(extent.start)
	if opening >= len(m.content) {
		return regions
	}
	fenceChar := m.content[opening]
	if fenceChar != '`' && fenceChar != '~' {
		return regions
	}
	regions = appendRegion(regions, opening, opening+runLength(m.content, opening, fenceChar), mdast.TokCodeFence)
	if block.Info != nil {
		regions = appendRegion(regions, m.abs(block.Info.Segment.Start), m.abs(block.Info.Segment.Stop), mdast.TokCodeFenceInfo)
	}
	regions = m.lineRegions(regions, block.Lines(), mdast.TokText, false)
	if extent.closed {
		// The closing fence is the run of fence characters on the last line
		// the block owns, after any container prefix.
		line := m.lines[m.lineIndex(m.abs(extent.lastLine))]
		start := line.StartOffset
		for start < line.NewlineStart && (m.content[start] == ' ' || m.content[start] == '\t' || m.content[start] == '>') {
			start++
		}
		if start < line.NewlineStart && m.content[start] == fenceChar {
			regions = appendRegion(regions, start, start+runLength(m.content, start, fenceChar), mdast.TokCodeFence)
		}
	}
	return regions
}

// lineRegions adds one region per line segment, from the segment start (or
// its first non-blank byte when skipBlank is set) to the end of the line,
// excluding the line ending. Blank lines add nothing.
func (m *mapper) lineRegions(regions []tokenRegion, lines *text.Segments, kind mdast.TokenKind, skipBlank bool) []tokenRegion {
	for idx := range lines.Len() {
		segment := lines.At(idx)
		start := m.abs(segment.Start)
		end := min(m.abs(segment.Stop), m.lineEnd(start))
		if skipBlank {
			for start < end && (m.content[start] == ' ' || m.content[start] == '\t') {
				start++
			}
		}
		regions = appendRegion(regions, start, end, kind)
	}
	return regions
}

// appendRegion adds a region unless it is empty.
func appendRegion(regions []tokenRegion, start, end int, kind mdast.TokenKind) []tokenRegion {
	if end <= start {
		return regions
	}
	return append(regions, tokenRegion{start: start, end: end, kind: kind})
}
