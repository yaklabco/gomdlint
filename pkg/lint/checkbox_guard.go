package lint

import (
	"bytes"
	"cmp"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Task list checkboxes and fixes.
//
// A task list checkbox changes how a list item renders, and the renderers
// disagree on where one is: micromark, which markdownlint uses, and
// GitHub's cmark-gfm differ on the line it may start on, on what may come
// before, between and after the brackets, and each has quirks of its own.
// Rather than emulate any of them, the engine withholds every fix that
// changes text a checkbox could be read from or that makes such text
// (checkboxGuard). The finding is reported without a fix.
//
// A candidate line is any line holding "[" and then one character and "]",
// or "[" at its end, wherever on the line; no reading of list markers,
// containers or footnotes decides it. Every checkbox either renderer reads
// is such a line, whatever prefix or container the renderer accepts before
// it. A candidate's region runs over its run, the lines around it up to a
// blank line (one of nothing but spaces and tabs), which holds the list item
// marker, the link reference definitions and the paragraph lines its
// reading can depend on, and over the line before the run and the line after
// it, so that no fix inserts or removes a line next to the run either. A fix
// is withheld when it changes a byte of a region, inserts text inside one,
// inserts text before a region that starts the content or after one that
// ends it, or leaves a region's first byte off the start of a line, and when
// it leaves a window of lines around its edits with more candidate lines than
// it had.
//
// Plain task items such as "- [ ] todo" take some fixes instead (see
// checkbox_plain.go).

// checkboxRegion is a source range around a candidate. Fixes may change
// its lines only as their kinds allow: not at all where it guards a
// candidate that is not plain, and as checkbox_plain.go tells on the
// lines of a plain candidate and the lines around it.
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
	// sorted and apart: the region of each candidate that is not plain, a
	// plain candidate's marker line and the line after it, and the frozen
	// lines above a plain candidate (see checkbox_plain.go).
	regions []checkboxRegion
	// kinds tells how each line of a region is guarded, plain holds the
	// parts of each plain candidate's marker line, and plainLines lists the
	// plain candidates' lines in order (see checkbox_plain.go).
	kinds      []lineKind
	plain      map[int]plainLine
	plainLines []int
	// interrupting tells, for each plain candidate's line, whether its item
	// may directly follow paragraph text in both renderers.
	interrupting map[int]bool
}

// newCheckboxGuard builds the guard for content, which goldmark parsed into
// the tree at root.
func newCheckboxGuard(content []byte, root *mdast.Node) *checkboxGuard {
	guard := &checkboxGuard{content: content, lines: lineStarts(content)}
	count := len(guard.lines)
	candidates := make([]bool, count)
	blank := make([]bool, count)
	for idx := range count {
		line := guard.line(idx)
		candidates[idx] = holdsCandidate(line)
		if idx == 0 {
			line = bytes.TrimPrefix(line, []byte(mdast.ByteOrderMark))
		}
		blank[idx] = len(bytes.Trim(line, " \t")) == 0
	}
	guard.classifyLines(candidates, root)

	// The region of a candidate that is not plain covers its run and the
	// lines on either side of it.
	var regions []checkboxRegion
	for idx := 0; idx < count; idx++ {
		if !candidates[idx] || guard.kinds[idx] == linePlain {
			continue
		}
		first, last := idx, idx
		for first > 0 && !blank[first-1] {
			first--
		}
		for last+1 < count && !blank[last+1] {
			last++
		}
		first, last = max(first-1, 0), min(last+1, count-1)
		regions = append(regions, checkboxRegion{start: guard.lines[first], end: guard.lineEnd(last)})
		idx = last
	}
	guard.guardRegions(regions)
	for _, line := range guard.plainLines {
		regions = append(regions, checkboxRegion{start: guard.lines[line], end: guard.lineEnd(min(line+1, count-1))})
	}
	regions = append(regions, guard.freezeAbove()...)
	guard.regions = mergeRegions(regions)
	return guard
}

// holdsCandidate reports whether a line, without its line ending, holds "["
// and then one character and "]", or ends with "[".
func holdsCandidate(line []byte) bool {
	for pos := bytes.IndexByte(line, '['); pos >= 0; {
		if pos+1 == len(line) {
			return true
		}
		_, size := utf8.DecodeRune(line[pos+1:])
		if closing := pos + 1 + size; closing < len(line) && line[closing] == ']' {
			return true
		}
		next := bytes.IndexByte(line[pos+1:], '[')
		if next < 0 {
			return false
		}
		pos += 1 + next
	}
	return false
}

// guardRegions marks every line of the given regions guarded, which takes
// them from the plain candidates.
func (g *checkboxGuard) guardRegions(regions []checkboxRegion) {
	for _, region := range regions {
		for line := g.lineOf(region.start); line < len(g.lines) && g.lines[line] < region.end; line++ {
			g.kinds[line] = lineGuarded
		}
	}
	plainLines := g.plainLines[:0]
	for _, line := range g.plainLines {
		if g.kinds[line] == linePlain {
			plainLines = append(plainLines, line)
			continue
		}
		delete(g.plain, line)
		delete(g.interrupting, line)
	}
	g.plainLines = plainLines
	for line, kind := range g.kinds {
		if kind == linePlainNext && (line == 0 || g.kinds[line-1] != linePlain) {
			g.kinds[line] = lineFree
		}
	}
}

// withholds reports whether a fix, edits in content offsets, changes or
// makes a checkbox candidate: an edit changes the text of a candidate's
// region or joins its first line to the line before, unless the change is
// one that a plain candidate allows (see checkbox_plain.go); the edits
// change what comes directly before a plain candidate in a way it does not
// allow; or a window of lines around the edits holds more candidate lines
// after them than before.
func (g *checkboxGuard) withholds(edits []fix.TextEdit) bool {
	edits = slices.Clone(edits)
	// An insertion sorts before an edit that starts where it does, so that
	// the edits apply in order.
	slices.SortStableFunc(edits, func(a, b fix.TextEdit) int {
		return cmp.Or(a.StartOffset-b.StartOffset, a.EndOffset-b.EndOffset)
	})
	carved := make(map[int][]fix.TextEdit)
	for _, edit := range edits {
		if !g.touches(edit) {
			continue
		}
		pieces, ok := splitAtLineEndings(g.content, edit)
		if !ok {
			return true
		}
		for _, piece := range pieces {
			if !g.touches(piece) {
				continue
			}
			line, ok := g.carvedOut(piece)
			if !ok {
				return true
			}
			carved[line] = append(carved[line], piece)
		}
	}
	for line, onLine := range carved {
		if !g.allowsOnLine(line, onLine) {
			return true
		}
	}
	if !g.keepsPlainBoundaries(edits) {
		return true
	}
	return g.makesCandidate(edits)
}

// touches reports whether edit changes a candidate region: it replaces a
// byte of one, inserts text inside one, inserts text before one that starts
// the content or after one that ends it, or leaves the region's first byte
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
		case start == end && ((start == 0 && region.start == 0) || (start == len(g.content) && region.end == len(g.content))):
			// No line comes in next to a region at either end of the
			// content, where no line before or after the region guards it.
			return true
		case end == region.start && !g.lineStartAfter(edit):
			return true
		}
	}
	return false
}

// lineStartAfter reports whether the text after edit starts a line once the
// edit is applied: the edit's text ends with a line ending, or it has none
// and a line ending, or the start of the content, comes before it.
func (g *checkboxGuard) lineStartAfter(edit fix.TextEdit) bool {
	if edit.NewText != "" {
		return isLineEnd(edit.NewText[len(edit.NewText)-1])
	}
	return edit.StartOffset == 0 || isLineEnd(g.content[edit.StartOffset-1])
}

// makesCandidate reports whether edits, sorted, leave the lines they touch,
// with the line after each, holding more candidate lines than before. An
// edit that changes no region keeps every candidate line it had, so more
// candidate lines means a new one.
func (g *checkboxGuard) makesCandidate(edits []fix.TextEdit) bool {
	type lineRange struct{ first, last int }
	var ranges []lineRange
	for _, edit := range edits {
		first := g.lineOf(edit.StartOffset)
		last := min(g.lineOf(max(edit.StartOffset, edit.EndOffset-1))+1, len(g.lines)-1)
		if count := len(ranges); count > 0 && first <= ranges[count-1].last+1 {
			ranges[count-1].last = max(ranges[count-1].last, last)
			continue
		}
		ranges = append(ranges, lineRange{first: first, last: last})
	}
	for _, lines := range ranges {
		before, after := g.windowTexts(g.lines[lines.first], g.lineEnd(lines.last), edits)
		if countCandidateLines(after) > countCandidateLines(before) {
			return true
		}
	}
	return false
}

// windowTexts returns content[from:to] before and after the edits that lie
// in it, which are sorted.
func (g *checkboxGuard) windowTexts(from, to int, edits []fix.TextEdit) ([]byte, []byte) {
	var after []byte
	pos := from
	first := sort.Search(len(edits), func(idx int) bool { return edits[idx].StartOffset >= from })
	for _, edit := range edits[first:] {
		// The edits do not overlap, so none after one that ends past to
		// lies in the window either.
		if edit.EndOffset > to {
			break
		}
		after = append(after, g.content[pos:edit.StartOffset]...)
		after = append(after, edit.NewText...)
		pos = edit.EndOffset
	}
	after = append(after, g.content[pos:to]...)
	return g.content[from:to], after
}

// countCandidateLines returns the number of candidate lines in text.
func countCandidateLines(text []byte) int {
	count := 0
	starts := lineStarts(text)
	for idx, start := range starts {
		end := len(text)
		if idx+1 < len(starts) {
			end = starts[idx+1]
		}
		if holdsCandidate(trimLineEnding(text[start:end])) {
			count++
		}
	}
	return count
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

// lineStarts returns the offset where each line of content starts. A line
// ends with a line feed, a carriage return and a line feed, or a carriage
// return alone, as CommonMark reads them. Empty content has one empty
// line; content that ends with a line ending has no line after it.
func lineStarts(content []byte) []int {
	starts := []int{0}
	for idx, char := range content {
		ends := char == '\n' || (char == '\r' && (idx+1 == len(content) || content[idx+1] != '\n'))
		if ends && idx+1 < len(content) {
			starts = append(starts, idx+1)
		}
	}
	return starts
}

// isLineEnd reports whether char ends a line: a line feed, or a carriage
// return, which fix.Validator never parts from a line feed after it.
func isLineEnd(char byte) bool {
	return char == '\n' || char == '\r'
}

// trimLineEnding returns line without its line ending.
func trimLineEnding(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}

// opensFootnote reports whether a line, without its line ending, starts a
// footnote definition: after blockquote markers, list item markers, spaces
// and tabs, it holds "[^" and then "]:".
func opensFootnote(line []byte, first bool) bool {
	pos := 0
	if first && bytes.HasPrefix(line, []byte(mdast.ByteOrderMark)) {
		pos = len(mdast.ByteOrderMark)
	}
	for pos < len(line) {
		switch line[pos] {
		case ' ', '\t', '>':
			pos++
			continue
		case '[':
			return pos+1 < len(line) && line[pos+1] == '^' && bytes.Contains(line[pos:], []byte("]:"))
		}
		end, _, ok := listMarkerEnd(line, pos)
		if !ok || (end < len(line) && line[end] != ' ' && line[end] != '\t') {
			return false
		}
		pos = end
	}
	return false
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
