package lint

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Plain task list items.
//
// Task lists such as "- [ ] todo" are common, and the list rules fix them.
// The guard lets through the fixes of one form of candidate whose
// checkbox both renderers read before and after them, a plain candidate:
//
//   - the brackets hold a space, "x" or "X", and the "]" is followed on its
//     line by spaces or tabs and then text;
//   - the "[" is on its item's marker line, after a single list marker and
//     one tab or one to four spaces;
//   - goldmark reads the line as the first line of a list item whose first
//     content is the paragraph that starts at the "[";
//   - no footnote definition starts on or above the line.
//
// On a plain candidate's marker line a fix may change the list marker for
// another of its kind, a bullet for a bullet or a number and delimiter for
// a number and delimiter (MD004, MD029); the space between the marker and
// the "[" to one to four spaces (MD030, MD010); the blockquote markers'
// spaces and the indentation before the marker (MD005, MD007, MD010,
// MD027); and the whitespace after the line's text (MD009). It must leave
// the bytes from the "[" through the first character of text after the
// "]", and the rest of the text, as they are, and add or remove no line
// ending. On the line after, a fix may change only the trailing whitespace,
// or make the changes above when that line is a plain candidate itself.
//
// Why both renderers still read the checkbox. micromark reads one where a
// list item's first content is a paragraph starting with "[", a space or
// "x" or "X", "]", whitespace and more text. cmark-gfm reads one where the
// marker line holds, after the marker and spacing, "[ ]", "[x]" or "[X]"
// and a space or tab (its 0.29.0.gfm.0 build also wants nothing but spaces
// before the marker, which the allowed changes keep or keep failing, since
// the blockquote markers stay). After an allowed change the marker is still
// a list marker, followed by one tab, which is one to four columns wherever
// it falls, or by one to four spaces, which CommonMark and both renderers
// take whole as the marker's spacing, so the item's content still starts at
// the "["; the indentation can only move the item within the list
// structure, which the fix checks compare; the brackets, their content, the
// whitespace and text after them are the same bytes; and with no line
// ending added or removed, the paragraph still starts on the marker line.
// The next line stays a paragraph line that can turn the paragraph into a
// heading or a table no more than before: only its trailing whitespace
// changes, which no setext underline or table delimiter row depends on, or
// it stays a plain candidate line, which holds "[" and text and so is
// neither.
//
// What comes before a plain candidate (keepsBefore). No fix may insert or
// remove a line directly before or after it, nor turn the line before it
// from blank to not blank or back, with two exceptions. Blank lines directly
// before it may go while one stays, since one blank line between blocks
// reads as many do. And one blank line may come in directly before a
// candidate whose list may start after paragraph text in both renderers: a
// bullet list, or an ordered list whose first marker is "1." or "1)", when
// no blockquote starts on the candidate's line (no footnote definition comes
// before a plain candidate). CommonMark lets exactly those lists interrupt a
// paragraph, and both renderers agree within limits that the condition
// keeps: micromark wants the single digit 1 where goldmark and cmark-gfm
// take any start number 1, such as "01.", and micromark reads a list in a
// blockquote that starts on the item's line as interrupting the paragraph
// above the quote, so the exception leaves such items out. The item then
// starts the same list after paragraph text as after a blank line, and its
// checkbox reads the same, so the blank line MD032 inserts before a task
// list keeps its fix. Before a candidate whose list may not start after
// paragraph text, after a line that is not blank, whether micromark reads
// the list depends on what the lines above leave open, so those lines, up to
// the nearest blank line, and that blank line, are frozen (freezeAbove):
// they may change only in their trailing whitespace, unless they are plain
// candidates or the lines after them, and no line may come in among them.

// lineKind tells how the guard treats a line of a candidate's region.
type lineKind uint8

const (
	// lineFree holds no candidate.
	lineFree lineKind = iota
	// linePlain is a plain candidate's marker line.
	linePlain
	// linePlainNext is the line after a plain candidate's marker line.
	linePlainNext
	// lineFrozen is above a frozen candidate (see freezeAbove).
	lineFrozen
	// lineGuarded is in the region of a candidate that is not plain.
	lineGuarded
)

// plainLine is the parts of a plain candidate's marker line.
type plainLine struct {
	// prefix holds the blockquote markers and indentation before the
	// marker, with a byte order mark on the first line.
	prefix  []byte
	ordered bool
	// spacing holds the spaces or tab between the marker and the "[".
	spacing []byte
	// box runs from the "[" through the first character of text after the
	// "]", which starts boxAt in the line; text runs from there through the
	// line's last non-whitespace character.
	box, text []byte
	boxAt     int
}

// readPlainLine reads the parts of a line, without its line ending, that
// holds a single list marker followed by spacing and "[ ]", "[x]" or
// "[X]", spaces or tabs and text. first tells whether the line starts the
// content.
func readPlainLine(line []byte, first bool) (plainLine, bool) {
	pos := 0
	if first && bytes.HasPrefix(line, []byte(mdast.ByteOrderMark)) {
		pos = len(mdast.ByteOrderMark)
	}
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t' || line[pos] == '>') {
		pos++
	}
	var parts plainLine
	parts.prefix = line[:pos]
	markerEnd, ordered, ok := listMarkerEnd(line, pos)
	if !ok {
		return plainLine{}, false
	}
	parts.ordered = ordered
	pos = markerEnd
	start := pos
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	parts.spacing = line[start:pos]
	if len(parts.spacing) == 0 || pos+3 > len(line) || line[pos] != '[' || line[pos+2] != ']' ||
		(line[pos+1] != ' ' && line[pos+1] != 'x' && line[pos+1] != 'X') {
		return plainLine{}, false
	}
	parts.boxAt = pos
	pos += 3
	spaceStart := pos
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		pos++
	}
	if pos == spaceStart || pos == len(line) {
		return plainLine{}, false
	}
	_, size := utf8.DecodeRune(line[pos:])
	pos += size
	parts.box = line[parts.boxAt:pos]
	parts.text = bytes.TrimRight(line[pos:], " \t")
	return parts, true
}

// maxOrderedMarkerDigits is the most digits an ordered list marker has.
const maxOrderedMarkerDigits = 9

// listMarkerEnd returns the end of a list item marker at line[pos], a
// bullet or one to nine digits with "." or ")", and whether it is ordered.
func listMarkerEnd(line []byte, pos int) (int, bool, bool) {
	if pos >= len(line) {
		return 0, false, false
	}
	switch line[pos] {
	case '-', '+', '*':
		return pos + 1, false, true
	}
	end := pos
	for end < len(line) && end-pos < maxOrderedMarkerDigits && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	if end == pos || end >= len(line) || (line[end] != '.' && line[end] != ')') {
		return 0, false, false
	}
	return end + 1, true, true
}

// plainSpacing reports whether spacing, between a marker and its item's
// content, is one tab or one to four spaces, which both renderers take
// whole as the marker's spacing wherever the marker stands.
func plainSpacing(spacing []byte) bool {
	return string(spacing) == "\t" || plainSpaces(spacing)
}

// plainSpaces reports whether spacing is one to four spaces.
func plainSpaces(spacing []byte) bool {
	const maxSpaces = 4
	return len(spacing) >= 1 && len(spacing) <= maxSpaces && len(bytes.Trim(spacing, " ")) == 0
}

// allows reports whether after, the new text of a plain candidate's marker
// line, keeps its checkbox: the same blockquote markers before a marker of
// the same kind, the same spacing or one to four spaces, and the same box
// and text, with any trailing whitespace.
func (before plainLine) allows(after []byte, first bool) bool {
	parts, ok := readPlainLine(after, first)
	if !ok || parts.ordered != before.ordered {
		return false
	}
	if !bytes.Equal(withoutSpace(parts.prefix), withoutSpace(before.prefix)) {
		return false
	}
	if !bytes.Equal(parts.spacing, before.spacing) && !plainSpaces(parts.spacing) {
		return false
	}
	return bytes.Equal(parts.box, before.box) && bytes.Equal(parts.text, before.text)
}

// withoutSpace returns text without its spaces and tabs.
func withoutSpace(text []byte) []byte {
	return bytes.Map(func(char rune) rune {
		if char == ' ' || char == '\t' {
			return -1
		}
		return char
	}, text)
}

// plainItem is a list item goldmark reads whose first content is a
// paragraph that starts on the item's first line, at the item's content.
// interrupts tells whether it may directly follow paragraph text in both
// renderers: its list is a bullet list, or an ordered list whose first
// marker is "1." or "1)", and no blockquote starts on its line.
type plainItem struct {
	interrupts bool
}

// plainItems returns the items goldmark reads as plain in source, by the
// offset where their content starts.
func plainItems(source []byte, root *mdast.Node) map[int]plainItem {
	items := make(map[int]plainItem)
	if root == nil {
		return items
	}
	for _, item := range mdast.FindByKind(root, mdast.NodeListItem) {
		first := item.FirstChild
		if first == nil || first.Kind != mdast.NodeParagraph {
			continue
		}
		content := item.ContentRange()
		if first.SourceRange().StartOffset != content.StartOffset ||
			first.SourcePosition().StartLine != item.SourcePosition().StartLine {
			continue
		}
		interrupts := !opensBlockquote(source, item)
		if list := item.Parent; interrupts && list != nil && list.Block != nil && list.Block.List != nil &&
			list.Block.List.Ordered {
			interrupts = firstMarkerIsOne(source, list)
		}
		items[content.StartOffset] = plainItem{interrupts: interrupts}
	}
	return items
}

// opensBlockquote reports whether a blockquote that holds item starts on
// the item's line: no line ending comes between the two starts.
func opensBlockquote(source []byte, item *mdast.Node) bool {
	at := item.SourceRange().StartOffset
	for parent := item.Parent; parent != nil; parent = parent.Parent {
		if parent.Kind != mdast.NodeBlockquote {
			continue
		}
		start := parent.SourceRange().StartOffset
		if start <= at && at <= len(source) && !bytes.ContainsAny(source[start:at], "\r\n") {
			return true
		}
	}
	return false
}

// firstMarkerIsOne reports whether the first item marker of an ordered list
// in source is "1." or "1)". goldmark and cmark-gfm let an ordered list
// start after paragraph text when it starts at 1, but micromark wants the
// single digit 1, so "01." does not.
func firstMarkerIsOne(source []byte, list *mdast.Node) bool {
	first := list.FirstChild
	if first == nil || list.Block == nil || list.Block.List == nil || list.Block.List.StartNumber != 1 {
		return false
	}
	at := first.SourceRange().StartOffset
	return at+1 < len(source) && source[at] == '1' && (source[at+1] == '.' || source[at+1] == ')')
}

// splitAtLineEndings splits edit into one edit for each line it changes,
// when its new text keeps the line endings of the text it replaces, the
// same ones in the same order, as when MD007 moves an item and the item
// nested in it in one edit. ok is false when the edit adds, removes or
// changes a line ending.
func splitAtLineEndings(content []byte, edit fix.TextEdit) ([]fix.TextEdit, bool) {
	oldText := content[edit.StartOffset:edit.EndOffset]
	newText := []byte(edit.NewText)
	oldLines, newLines := splitLines(oldText), splitLines(newText)
	if len(oldLines) != len(newLines) {
		return nil, false
	}
	pieces := make([]fix.TextEdit, 0, len(oldLines))
	pos := edit.StartOffset
	for idx := range oldLines {
		oldLine, newLine := oldLines[idx], newLines[idx]
		if !bytes.Equal(lineEnding(oldLine), lineEnding(newLine)) {
			return nil, false
		}
		oldBody, newBody := trimLineEnding(oldLine), trimLineEnding(newLine)
		pieces = append(pieces, fix.TextEdit{StartOffset: pos, EndOffset: pos + len(oldBody), NewText: string(newBody)})
		pos += len(oldLine)
	}
	return pieces, true
}

// splitLines splits text after each line ending, keeping it; text that
// ends with a line ending has an empty last part.
func splitLines(text []byte) [][]byte {
	var parts [][]byte
	start := 0
	for idx := range text {
		switch {
		case text[idx] == '\n':
			parts = append(parts, text[start:idx+1])
			start = idx + 1
		case text[idx] == '\r' && (idx+1 == len(text) || text[idx+1] != '\n'):
			parts = append(parts, text[start:idx+1])
			start = idx + 1
		}
	}
	return append(parts, text[start:])
}

// lineEnding returns the line ending that ends line, if any.
func lineEnding(line []byte) []byte {
	return line[len(trimLineEnding(line)):]
}

// carvedOut returns the line that holds edit when the edit stays inside
// the text of a plain candidate's marker line, the line after it or a
// frozen line, before the line ending, and adds no line ending.
func (g *checkboxGuard) carvedOut(edit fix.TextEdit) (int, bool) {
	if strings.ContainsAny(edit.NewText, "\r\n") {
		return 0, false
	}
	line := g.lineOf(edit.StartOffset)
	if edit.EndOffset > g.lines[line]+len(g.line(line)) {
		return 0, false
	}
	switch g.kinds[line] {
	case linePlain, linePlainNext, lineFrozen:
		return line, true
	default:
		return 0, false
	}
}

// allowsOnLine reports whether edits, sorted and all on one line of a
// plain candidate's region, keep its checkbox.
func (g *checkboxGuard) allowsOnLine(line int, edits []fix.TextEdit) bool {
	start := g.lines[line]
	before := g.line(line)
	var after []byte
	pos := 0
	for _, edit := range edits {
		after = append(after, before[pos:edit.StartOffset-start]...)
		after = append(after, edit.NewText...)
		pos = edit.EndOffset - start
	}
	after = append(after, before[pos:]...)
	if g.kinds[line] == linePlain {
		return g.plain[line].allows(after, line == 0)
	}
	return bytes.Equal(bytes.TrimRight(after, " \t"), bytes.TrimRight(before, " \t"))
}

// classifyLines finds the plain candidates among the candidate lines and
// marks each one's line and the line after it. A candidate that is not
// plain guards its region later, which takes those lines back.
func (g *checkboxGuard) classifyLines(candidates []bool, root *mdast.Node) {
	g.kinds = make([]lineKind, len(g.lines))
	g.plain = make(map[int]plainLine)
	g.interrupting = make(map[int]bool)
	footnote := len(g.lines)
	for idx := range g.lines {
		if opensFootnote(g.line(idx), idx == 0) {
			footnote = idx
			break
		}
	}
	items := plainItems(g.content, root)
	for idx, candidate := range candidates {
		if !candidate || idx >= footnote {
			continue
		}
		parts, ok := readPlainLine(g.line(idx), idx == 0)
		if !ok || !plainSpacing(parts.spacing) {
			continue
		}
		item, ok := items[g.lines[idx]+parts.boxAt]
		if !ok {
			continue
		}
		g.plain[idx] = parts
		g.plainLines = append(g.plainLines, idx)
		g.kinds[idx] = linePlain
		g.interrupting[idx] = item.interrupts
	}
	for _, line := range g.plainLines {
		if next := line + 1; next < len(g.lines) && g.kinds[next] == lineFree {
			g.kinds[next] = linePlainNext
		}
	}
}

// keepsPlainBoundaries reports whether edits, sorted, keep what comes
// directly before each plain candidate as it allows. A change inside the
// text of a plain candidate's line, the line after it or a frozen line is
// allowsOnLine's to judge; keepsBefore judges the rest, for each candidate
// that one of them comes directly before.
func (g *checkboxGuard) keepsPlainBoundaries(edits []fix.TextEdit) bool {
	if len(g.plainLines) == 0 {
		return true
	}
	// An edit that keeps its line endings counts line by line, so that the
	// part of it on the line before a candidate is judged as a change to
	// that line. One that adds or removes line endings across a candidate's
	// start changes the candidate's region, which withholds it.
	var pieces []fix.TextEdit
	for _, edit := range edits {
		split, ok := splitAtLineEndings(g.content, edit)
		if !ok {
			split = []fix.TextEdit{edit}
		}
		for _, piece := range split {
			if _, carved := g.carvedOut(piece); !carved {
				pieces = append(pieces, piece)
			}
		}
	}
	checked := make(map[int]bool)
	for _, piece := range pieces {
		line := g.lineAfterChange(piece)
		if _, plain := g.plain[line]; !plain || checked[line] {
			continue
		}
		checked[line] = true
		if !g.keepsBefore(line, pieces) {
			return false
		}
	}
	return true
}

// lineAfterChange returns the line that piece comes directly before once
// applied: the line after the last line whose text or line ending it
// changes, or the line at whose start it inserts whole lines.
func (g *checkboxGuard) lineAfterChange(piece fix.TextEdit) int {
	if piece.StartOffset == piece.EndOffset {
		at := g.lineOf(piece.StartOffset)
		if g.lines[at] == piece.StartOffset && g.lineStartAfter(piece) {
			return at
		}
		return at + 1
	}
	last := g.lineOf(piece.EndOffset - 1)
	if next := last + 1; next < len(g.lines) && g.lines[next] == piece.EndOffset && !g.lineStartAfter(piece) {
		// The piece takes the line ending before line next, which joins
		// that line to the one above.
		return next + 1
	}
	return last + 1
}

// keepsBefore reports whether pieces, sorted, keep what comes directly
// before plain candidate line. It compares the lines from the line before
// the candidate up, as far up as the pieces change them, before and after
// the pieces. Those may insert or remove no line there, nor turn the line
// before the candidate from blank to not blank or back, with two
// exceptions: blank lines directly before the candidate may go while one
// stays, and one blank line may come in before a candidate whose item may
// follow paragraph text in both renderers (g.interrupting). Before a frozen
// candidate, whose lines above take only the changes allowsOnLine allows,
// nothing else may change.
//
// The guard judges each fix alone. Two fixes that each removed one of two
// blank lines before a candidate would remove both, but MD012, the rule
// that removes blank lines, leaves one of them.
func (g *checkboxGuard) keepsBefore(line int, pieces []fix.TextEdit) bool {
	if g.frozen(line) {
		return false
	}
	start := g.lines[line]
	from := start
	if line > 0 {
		from = g.lines[line-1]
	}
	// Take in each line above whose text or line ending a piece changes.
	// The pieces do not overlap, so once the nearest piece above the window
	// leaves the line above alone, every piece does. Pieces that the window
	// took in as it grew are skipped.
	for idx := sort.Search(len(pieces), func(idx int) bool { return pieces[idx].StartOffset >= from }) - 1; idx >= 0 && from > 0; idx-- {
		if pieces[idx].StartOffset >= from {
			continue
		}
		if !g.changesLine(pieces[idx], g.lineOf(from-1)) {
			break
		}
		from = g.lines[g.lineOf(pieces[idx].StartOffset)]
	}
	before, after := g.windowTexts(from, start, pieces)
	if len(after) > 0 && !isLineEnd(after[len(after)-1]) {
		return false
	}
	blankBefore := line == 0 || g.holdsNoText(line-1)
	blankAfter := true
	switch body := trimLineEnding(after); {
	case len(after) > 0:
		lastStart := bytes.LastIndexAny(body, "\r\n") + 1
		blankAfter = noText(body[lastStart:], from == 0 && lastStart == 0)
	case from > 0:
		// Every line of the window went, and no piece changes the line
		// above it, which now comes directly before the candidate.
		blankAfter = g.holdsNoText(g.lineOf(from - 1))
	}
	inserted := countLineEndings(after) - countLineEndings(before)
	switch {
	case blankBefore && blankAfter:
		return inserted <= 0 || (inserted == 1 && g.interrupting[line])
	case !blankBefore && !blankAfter:
		return inserted == 0
	case !blankBefore && blankAfter:
		return inserted == 1 && g.interrupting[line]
	default:
		return false
	}
}

// changesLine reports whether piece changes the text or the line ending of
// line idx: it overlaps them, puts text before the line's text, or takes
// the line ending before the line, which joins it to the one above.
func (g *checkboxGuard) changesLine(piece fix.TextEdit, idx int) bool {
	start, end := g.lines[idx], g.lineEnd(idx)
	if piece.StartOffset < end && piece.EndOffset > start {
		return true
	}
	return piece.EndOffset == start && !g.lineStartAfter(piece)
}

// frozen reports whether plain candidate line has frozen lines above it:
// its item may not follow paragraph text in both renderers, and the line
// before it holds text.
func (g *checkboxGuard) frozen(line int) bool {
	return line > 0 && !g.interrupting[line] && !g.holdsNoText(line-1)
}

// freezeAbove marks frozen the lines above each frozen candidate, from the
// nearest line above it that holds no text, or the first line, through the
// line before it, and returns their regions. micromark reads such a
// candidate's checkbox or not by what those lines open and close
// ("#Title\n> 3. [x] task" holds none, "# Title\n> 3. [x] task" one), so
// they may change only in their trailing whitespace, unless they are plain
// candidates or the lines after them, and no line may come in among them.
func (g *checkboxGuard) freezeAbove() []checkboxRegion {
	var regions []checkboxRegion
	top, next, marked := 0, 0, -1
	for _, line := range g.plainLines {
		if !g.frozen(line) {
			continue
		}
		// top becomes the first of the lines above that hold text.
		for ; next < line; next++ {
			if g.holdsNoText(next) {
				top = next + 1
			}
		}
		first := max(top-1, 0)
		for idx := max(first, marked+1); idx < line; idx++ {
			if g.kinds[idx] == lineFree {
				g.kinds[idx] = lineFrozen
			}
		}
		marked = line - 1
		regions = append(regions, checkboxRegion{start: g.lines[first], end: g.lines[line]})
	}
	return regions
}

// holdsNoText reports whether line idx holds no text (see noText).
func (g *checkboxGuard) holdsNoText(idx int) bool {
	return noText(g.line(idx), idx == 0)
}

// noText reports whether a line, without its line ending, holds no text:
// nothing but blockquote markers, each after at most three spaces, and
// spaces and tabs. first tells whether the line starts the content, where
// a byte order mark is no text either.
func noText(line []byte, first bool) bool {
	if first {
		line = bytes.TrimPrefix(line, []byte(mdast.ByteOrderMark))
	}
	pos := 0
	for {
		spaces := 0
		for pos+spaces < len(line) && spaces < 3 && line[pos+spaces] == ' ' {
			spaces++
		}
		if pos+spaces >= len(line) || line[pos+spaces] != '>' {
			break
		}
		pos += spaces + 1
	}
	return len(bytes.Trim(line[pos:], " \t")) == 0
}

// countLineEndings returns the number of line endings in text: line feeds,
// and carriage returns that no line feed follows.
func countLineEndings(text []byte) int {
	count := 0
	for idx, char := range text {
		if char == '\n' || (char == '\r' && (idx+1 == len(text) || text[idx+1] != '\n')) {
			count++
		}
	}
	return count
}
