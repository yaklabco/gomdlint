package lint

import (
	"bytes"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Task list checkboxes and fixes.
//
// A task list checkbox changes how a list item renders, and the renderers
// disagree on where one is. micromark, which markdownlint uses, reads one
// after a blank first line and after the link reference definitions that
// start an item, and GitHub's cmark-gfm only right after the item's
// marker; they also disagree on what may come between the brackets and
// after them. Rather than emulate either, the engine withholds every fix
// that changes text a checkbox could be read from or that makes such text
// (checkboxGuard). The finding is reported without a fix.
//
// A checkbox candidate is a lexical superset of the checkboxes both read.
// A line is a marker line when, after any blockquote markers, spaces and
// tabs, it holds one or more list item markers, a bullet or one to nine
// digits with "." or ")", each followed by a space, a tab or the end of
// the line. The marker line's chunk is the rest of the line after the
// markers and the lines after it, up to a blank line (one of nothing but
// ">", spaces and tabs) or the next marker line. A candidate is a chunk
// line whose text, after blockquote markers, spaces and tabs, starts with
// "[" and then either a line ending, or one character and "]".
//
// It is a superset because both renderers read a checkbox only at the
// start of the paragraph that is a list item's first content, and only
// with one character, or in micromark a line ending, between the
// brackets. That paragraph starts on the rest of the item's marker line,
// on a line after it when the marker line holds nothing else, or, in
// micromark, on the line after the link reference definitions that start
// the item's content. None of these is ever parted from the marker line by
// a blank line or another item's marker, and the item's later lines carry
// only blockquote markers and indentation before their text. cmark-gfm
// reads a checkbox only on the marker line, after the marker and spaces.
//
// A candidate's region runs from the start of its marker line, whose
// blockquote markers and indentation decide how wide a tab between the
// brackets is, through the end of the line holding the "]", line ending
// included, and the line after it: micromark reads a checkbox before only
// spaces and tabs only when more of the paragraph follows, and none in a
// paragraph that the next line makes a setext heading or a table. When a
// line ending follows the "[", the "]" can start the next line, so the
// region takes the two lines after the "[".

// checkboxRegion is the source range of one or more checkbox candidates.
type checkboxRegion struct {
	start, end int
}

// checkboxGuard tells which fixes of a document change or make a checkbox
// candidate. Its tables cover the document as linted.
type checkboxGuard struct {
	content []byte
	// lines holds the start of each line; a line runs to the next start.
	lines []int
	// regions holds the merged regions of the document's candidates,
	// sorted and apart.
	regions []checkboxRegion
	// chunkMarker holds, for each line, the marker line whose chunk holds
	// it, or -1. runEnd holds, for each line, the last line of the run of
	// lines from it that are neither blank nor marker lines, or the line
	// before it when it is one.
	chunkMarker, runEnd []int
}

// newCheckboxGuard builds the guard for content.
func newCheckboxGuard(content []byte) *checkboxGuard {
	guard := &checkboxGuard{content: content, lines: lineStarts(content)}
	count := len(guard.lines)
	shapes := make([]lineShape, count)
	for idx := range count {
		shapes[idx] = readLineShape(guard.line(idx), idx == 0)
	}
	guard.chunkMarker = make([]int, count)
	marker := -1
	for idx, shape := range shapes {
		switch {
		case shape.marker:
			marker = idx
		case shape.blank:
			marker = -1
		}
		guard.chunkMarker[idx] = marker
	}
	guard.runEnd = make([]int, count)
	for idx := count - 1; idx >= 0; idx-- {
		switch {
		case shapes[idx].blank || shapes[idx].marker:
			guard.runEnd[idx] = idx - 1
		case idx+1 < count:
			guard.runEnd[idx] = max(guard.runEnd[idx+1], idx)
		default:
			guard.runEnd[idx] = idx
		}
	}
	guard.regions = mergeRegions(candidateRegions(content, guard.lines, shapes))
	return guard
}

// withholds reports whether a fix, edits in content offsets, changes or
// makes a checkbox candidate: an edit changes the text of a candidate's
// region or joins its first line to the line before, or a window of lines
// around the edits holds a candidate after them that it did not hold
// before.
func (g *checkboxGuard) withholds(edits []fix.TextEdit) bool {
	for _, edit := range edits {
		if g.touches(edit) {
			return true
		}
	}
	edits = slices.Clone(edits)
	slices.SortFunc(edits, func(a, b fix.TextEdit) int { return a.StartOffset - b.StartOffset })
	for _, window := range g.windows(edits) {
		if g.makesCandidate(window, edits) {
			return true
		}
	}
	return false
}

// touches reports whether edit changes a candidate region: it replaces a
// byte of one, inserts text inside one, or leaves the region's first byte
// off the start of a line.
func (g *checkboxGuard) touches(edit fix.TextEdit) bool {
	start, end := edit.StartOffset, edit.EndOffset
	idx := sort.Search(len(g.regions), func(idx int) bool { return g.regions[idx].end >= start })
	for ; idx < len(g.regions) && g.regions[idx].start <= end; idx++ {
		region := g.regions[idx]
		switch {
		case start < end && start < region.end && end > region.start:
			return true
		case start == end && region.start < start && start < region.end:
			return true
		case g.appendsToUnterminatedEnd(edit, region):
			return true
		case end == region.start && !g.lineStartAfter(edit):
			return true
		}
	}
	return false
}

// appendsToUnterminatedEnd reports whether edit inserts text at the end of
// content that ends inside region, with no line ending: the text goes on
// the region's last line.
func (g *checkboxGuard) appendsToUnterminatedEnd(edit fix.TextEdit, region checkboxRegion) bool {
	if edit.StartOffset != edit.EndOffset || edit.StartOffset != len(g.content) {
		return false
	}
	return region.end == len(g.content) && !bytes.HasSuffix(g.content, []byte("\n"))
}

// lineStartAfter reports whether the text after edit starts a line once the
// edit is applied: the edit's text ends with a line ending, or it has none
// and a line ending, or the start of the content, comes before it.
func (g *checkboxGuard) lineStartAfter(edit fix.TextEdit) bool {
	if edit.NewText != "" {
		return edit.NewText[len(edit.NewText)-1] == '\n'
	}
	return edit.StartOffset == 0 || g.content[edit.StartOffset-1] == '\n'
}

// lineWindow is a range of lines, first to last: from an edit's first
// line, with a head through the line after its last line, and on through
// the lines after that which a chunk there would take in, and one more.
type lineWindow struct {
	first, head, last int
}

// windows returns the line windows of edits, merged and sorted.
func (g *checkboxGuard) windows(edits []fix.TextEdit) []lineWindow {
	windows := make([]lineWindow, 0, len(edits))
	for _, edit := range edits {
		first := g.lineOf(edit.StartOffset)
		head := min(g.lineOf(max(edit.StartOffset, edit.EndOffset-1))+1, len(g.lines)-1)
		through := max(head, g.runEnd[head])
		windows = append(windows, lineWindow{first: first, head: head, last: min(through+1, len(g.lines)-1)})
	}
	slices.SortFunc(windows, func(a, b lineWindow) int { return a.first - b.first })
	merged := windows[:0]
	for _, window := range windows {
		if count := len(merged); count > 0 && window.first <= merged[count-1].last+1 {
			merged[count-1].head = max(merged[count-1].head, window.head)
			merged[count-1].last = max(merged[count-1].last, window.last)
			continue
		}
		merged = append(merged, window)
	}
	return merged
}

// makesCandidate reports whether the edits that lie in window leave it
// with more candidates than it had. An edit that changes no region keeps
// every candidate it had, so more candidates means a new one. The lines
// before the window are the same before and after, and so is whether its
// first line continues a chunk, so they hold the same candidates. The
// lines after the head are the same too, and hold the same candidates
// unless the edits change whether the line after the head continues a
// chunk; only then is the rest of the window read.
func (g *checkboxGuard) makesCandidate(window lineWindow, edits []fix.TextEdit) bool {
	inChunk := window.first > 0 && g.chunkMarker[window.first-1] >= 0
	from, first := g.lines[window.first], window.first == 0
	headBefore, headAfter := g.windowTexts(from, g.lineEnd(window.head), edits)
	countBefore, chunkBefore := scanCandidates(headBefore, inChunk, first)
	countAfter, chunkAfter := scanCandidates(headAfter, inChunk, first)
	if countAfter != countBefore || chunkAfter == chunkBefore {
		return countAfter > countBefore
	}
	before, after := g.windowTexts(from, g.lineEnd(window.last), edits)
	countBefore, _ = scanCandidates(before, inChunk, first)
	countAfter, _ = scanCandidates(after, inChunk, first)
	return countAfter > countBefore
}

// windowTexts returns content[from:to] before and after the edits that lie
// in it, which are sorted.
func (g *checkboxGuard) windowTexts(from, to int, edits []fix.TextEdit) ([]byte, []byte) {
	var after []byte
	pos := from
	for _, edit := range edits {
		if edit.StartOffset < from || edit.EndOffset > to {
			continue
		}
		after = append(after, g.content[pos:edit.StartOffset]...)
		after = append(after, edit.NewText...)
		pos = edit.EndOffset
	}
	after = append(after, g.content[pos:to]...)
	return g.content[from:to], after
}

// scanCandidates returns the number of candidates in text, and whether a
// line after it would continue a chunk. inChunk tells whether the first
// line of text continues a chunk, and first whether it starts the content.
func scanCandidates(text []byte, inChunk, first bool) (int, bool) {
	count := 0
	starts := lineStarts(text)
	for idx, start := range starts {
		end := len(text)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}
		line := trimLineEnding(text[start:end])
		shape := readLineShape(line, first && idx == 0)
		switch {
		case shape.marker:
			inChunk = true
		case shape.blank:
			inChunk = false
		}
		if _, ok := candidateEnd(line[shape.rest:], 0); ok && inChunk {
			count++
		}
	}
	return count, inChunk
}

// line returns the text of a line without its line ending.
func (g *checkboxGuard) line(idx int) []byte {
	return trimLineEnding(g.content[g.lines[idx]:g.lineEnd(idx)])
}

// lineEnd returns the offset after a line's line ending.
func (g *checkboxGuard) lineEnd(idx int) int {
	if idx+1 < len(g.lines) {
		return g.lines[idx+1]
	}
	return len(g.content)
}

// lineOf returns the line that holds offset; the end of the content is on
// the last line.
func (g *checkboxGuard) lineOf(offset int) int {
	return max(sort.SearchInts(g.lines, offset+1)-1, 0)
}

// lineStarts returns the offset where each line of content starts. Empty
// content has one empty line; content that ends with a line ending has no
// line after it.
func lineStarts(content []byte) []int {
	starts := []int{0}
	for idx, char := range content {
		if char == '\n' && idx+1 < len(content) {
			starts = append(starts, idx+1)
		}
	}
	return starts
}

// trimLineEnding returns line without its line ending.
func trimLineEnding(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}

// lineShape is what the lexical scan reads from one line.
type lineShape struct {
	marker bool // The line holds a list item marker after its prefix.
	blank  bool // The line holds nothing but ">", spaces and tabs.
	rest   int  // Offset of the text after the prefix and markers.
}

// maxOrderedMarkerDigits is the most digits an ordered list marker has.
const maxOrderedMarkerDigits = 9

// readLineShape reads the blockquote markers, spaces, tabs and list item
// markers that start a line, without its line ending. A byte order mark
// starts the first line.
func readLineShape(line []byte, first bool) lineShape {
	pos := 0
	if first && bytes.HasPrefix(line, []byte(mdast.ByteOrderMark)) {
		pos = len(mdast.ByteOrderMark)
	}
	var shape lineShape
	spaceAfter := func(idx int) bool {
		return idx >= len(line) || line[idx] == ' ' || line[idx] == '\t'
	}
	for pos < len(line) {
		switch char := line[pos]; {
		case char == ' ' || char == '\t' || char == '>':
			pos++
			continue
		case (char == '-' || char == '+' || char == '*') && spaceAfter(pos+1):
			shape.marker = true
			pos++
			continue
		case char >= '0' && char <= '9':
			end := pos
			for end < len(line) && end-pos < maxOrderedMarkerDigits && line[end] >= '0' && line[end] <= '9' {
				end++
			}
			if end < len(line) && (line[end] == '.' || line[end] == ')') && spaceAfter(end+1) {
				shape.marker = true
				pos = end + 1
				continue
			}
		}
		break
	}
	shape.rest = pos
	shape.blank = !shape.marker && pos == len(line)
	return shape
}

// candidateRegions returns the region of each candidate of content, whose
// lines start at lines and have the given shapes, in order.
func candidateRegions(content []byte, lines []int, shapes []lineShape) []checkboxRegion {
	lineEnd := func(idx int) int {
		if idx+1 < len(lines) {
			return lines[idx+1]
		}
		return len(content)
	}
	var regions []checkboxRegion
	marker := -1
	for idx, shape := range shapes {
		switch {
		case shape.marker:
			marker = idx
		case shape.blank:
			marker = -1
		}
		if marker < 0 {
			continue
		}
		text := trimLineEnding(content[lines[idx]:lineEnd(idx)])[shape.rest:]
		lastLine, ok := candidateEnd(text, idx)
		if !ok {
			continue
		}
		regions = append(regions, checkboxRegion{start: lines[marker], end: lineEnd(min(lastLine, len(lines)-1))})
	}
	return regions
}

// candidateEnd reports whether text, a chunk line's text after its prefix
// on line idx, is a candidate, and returns the last line of its region.
func candidateEnd(text []byte, idx int) (int, bool) {
	if len(text) == 0 || text[0] != '[' {
		return 0, false
	}
	if len(text) == 1 {
		// A line ending follows the "[": the "]" can start the next line.
		return idx + 2, true
	}
	_, size := utf8.DecodeRune(text[1:])
	if closing := 1 + size; closing >= len(text) || text[closing] != ']' {
		return 0, false
	}
	return idx + 1, true
}

// mergeRegions merges sorted regions that overlap or touch.
func mergeRegions(regions []checkboxRegion) []checkboxRegion {
	slices.SortFunc(regions, func(a, b checkboxRegion) int { return a.start - b.start })
	merged := regions[:0]
	for _, region := range regions {
		if count := len(merged); count > 0 && region.start <= merged[count-1].end {
			merged[count-1].end = max(merged[count-1].end, region.end)
			continue
		}
		merged = append(merged, region)
	}
	return merged
}
