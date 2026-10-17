package refs

import (
	"bytes"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/lint/mdscan"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Definitions are found where the parser put them. The parser removes link
// reference definitions from the paragraphs they start and records no node
// for them, so every non-blank line that no leaf block covers holds part of a
// definition. Those lines are parsed with the CommonMark definition grammar.

// definitionLine is one line of a run of definition lines, with the part
// before the definition text (the container prefix) left out.
type definitionLine struct {
	number int
	start  int  // offset where the line's definition text starts
	end    int  // offset of the line ending
	marker bool // the prefix holds a list item marker
}

// collectDefinitions records every link reference definition in the file.
func (c *collector) collectDefinitions() {
	file := c.ctx.File
	if file == nil || len(file.Content) == 0 {
		return
	}
	covered := c.leafBlockLines()
	var run []definitionLine
	for number := file.FrontMatterLines + 1; number <= len(file.Lines); number++ {
		line, blank := definitionLineAt(file, number)
		if covered[number] || blank {
			c.parseDefinitionRun(run)
			run = run[:0]
			continue
		}
		run = append(run, line)
	}
	c.parseDefinitionRun(run)
}

// leafBlockLines returns the lines covered by blocks that hold their own
// content: paragraphs, headings, code, HTML, thematic breaks, and tables.
func (c *collector) leafBlockLines() map[int]bool {
	lines := make(map[int]bool)
	_ = mdast.Walk(c.root, func(node *mdast.Node) error { //nolint:errcheck // visitor never returns error
		switch node.Kind {
		case mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeCodeBlock, mdast.NodeHTMLBlock,
			mdast.NodeThematicBreak, mdast.NodeTable:
			pos := node.SourcePosition()
			for line := pos.StartLine; line <= pos.EndLine; line++ {
				lines[line] = true
			}
		}
		return nil
	})
	return lines
}

// definitionLineAt locates the definition text of a line: after blockquote
// markers, indentation, and list item markers. It reports whether nothing
// remains, which makes the line blank.
func definitionLineAt(file *mdast.FileSnapshot, number int) (definitionLine, bool) {
	info := file.Lines[number-1]
	content := file.Content
	pos := info.StartOffset
	if number == 1 && bytes.HasPrefix(content, []byte(mdast.ByteOrderMark)) {
		// The parser skips a byte order mark, so a definition can follow one.
		pos += len(mdast.ByteOrderMark)
	}
	marker := false
	for pos < info.NewlineStart {
		switch ch := content[pos]; {
		case ch == ' ' || ch == '\t' || ch == '>':
			pos++
			continue
		case ch == '-' || ch == '+' || ch == '*':
			if pos+1 < info.NewlineStart && (content[pos+1] == ' ' || content[pos+1] == '\t') {
				pos += 2
				marker = true
				continue
			}
		case ch >= '0' && ch <= '9':
			if next := orderedMarkerEnd(content, pos, info.NewlineStart); next > pos {
				pos = next
				marker = true
				continue
			}
		}
		break
	}
	line := definitionLine{number: number, start: pos, end: info.NewlineStart, marker: marker}
	return line, pos >= info.NewlineStart
}

// orderedMarkerEnd returns the offset after an ordered list marker and the
// whitespace that follows it, or pos when none starts there.
func orderedMarkerEnd(content []byte, pos, limit int) int {
	end := pos
	for end < limit && end-pos < 9 && content[end] >= '0' && content[end] <= '9' {
		end++
	}
	if end == pos || end+1 >= limit || (content[end] != '.' && content[end] != ')') ||
		(content[end+1] != ' ' && content[end+1] != '\t') {
		return pos
	}
	return end + 2
}

// parseDefinitionRun parses consecutive definitions from a run of lines.
// Each definition starts at the start of a line and ends at a line ending.
func (c *collector) parseDefinitionRun(run []definitionLine) {
	text, offsets := joinDefinitionLines(c.ctx.File.Content, run)
	for pos := 0; pos < len(text); {
		parsed, ok := parseDefinition(text, pos)
		if !ok {
			return
		}
		first := lineIndexAt(offsets, pos)
		last := lineIndexAt(offsets, parsed.end-1)
		c.addDefinition(parsed, run[first:last+1])
		pos = parsed.end
	}
}

// joinDefinitionLines concatenates the definition text of each line with
// "\n" and returns, for each line, the text offset where it starts.
func joinDefinitionLines(content []byte, run []definitionLine) (string, []int) {
	var text strings.Builder
	offsets := make([]int, 0, len(run))
	for _, line := range run {
		offsets = append(offsets, text.Len())
		text.Write(content[line.start:line.end])
		_ = text.WriteByte('\n') // strings.Builder.WriteByte never fails
	}
	return text.String(), offsets
}

// lineIndexAt returns the index of the line holding text offset pos.
func lineIndexAt(offsets []int, pos int) int {
	idx := 0
	for idx+1 < len(offsets) && offsets[idx+1] <= pos {
		idx++
	}
	return idx
}

// addDefinition records one parsed definition spanning lines.
func (c *collector) addDefinition(parsed parsedDefinition, lines []definitionLine) {
	file := c.ctx.File
	first, last := lines[0], lines[len(lines)-1]
	normalized := NormalizeLabel(parsed.label)
	startLine, startColumn := file.LineAt(first.start)
	endLine, endColumn := file.LineAt(last.end)
	def := &ReferenceDefinition{
		Label:           parsed.label,
		NormalizedLabel: normalized,
		Destination:     parsed.destination,
		Title:           parsed.title,
		LineNumber:      first.number,
		Position: mdast.SourcePosition{
			StartLine: startLine, StartColumn: startColumn, EndLine: endLine, EndColumn: endColumn,
		},
		Lines:     [2]int{first.number, last.number},
		Removable: allNoMarker(lines),
	}
	if _, exists := c.ctx.Definitions[normalized]; exists {
		def.IsDuplicate = true
	} else {
		c.ctx.Definitions[normalized] = def
	}
	c.ctx.AllDefinitions = append(c.ctx.AllDefinitions, def)
}

func allNoMarker(lines []definitionLine) bool {
	for _, line := range lines {
		if line.marker {
			return false
		}
	}
	return true
}

// parsedDefinition is a link reference definition read from definition text.
type parsedDefinition struct {
	label, destination, title string
	end                       int // offset after the definition's last line ending
}

// parseDefinition parses one link reference definition starting at
// text[pos]: a label, a colon, a destination, and an optional title, each
// separated by whitespace that may include one line ending, followed only by
// spaces or tabs on its last line.
func parseDefinition(text string, pos int) (parsedDefinition, bool) {
	labelEnd, ok := scanDefinitionLabel(text, pos)
	if !ok || labelEnd+1 >= len(text) || text[labelEnd+1] != ':' {
		return parsedDefinition{}, false
	}
	parsed := parsedDefinition{label: text[pos+1 : labelEnd]}
	idx := skipDefinitionSpace(text, labelEnd+2)
	destEnd, ok := scanDefinitionDestination(text, idx)
	if !ok {
		return parsedDefinition{}, false
	}
	parsed.destination = text[idx:destEnd]
	if strings.HasPrefix(parsed.destination, "<") {
		// A pointy destination is written inside one pair of angle brackets.
		parsed.destination = strings.TrimSuffix(parsed.destination[1:], ">")
	}

	// The title is optional; without a valid one the destination must end the line.
	if titleStart := skipDefinitionSpace(text, destEnd); titleStart > destEnd && titleStart < len(text) {
		if titleEnd, found := scanDefinitionTitle(text, titleStart); found {
			if lineEnd, clean := restOfLineBlank(text, titleEnd); clean {
				parsed.title = text[titleStart+1 : titleEnd-1]
				parsed.end = lineEnd
				return parsed, true
			}
		}
	}
	lineEnd, clean := restOfLineBlank(text, destEnd)
	if !clean {
		return parsedDefinition{}, false
	}
	parsed.end = lineEnd
	return parsed, true
}

// scanDefinitionLabel returns the offset of the ']' closing the label opened
// at text[pos]: no unescaped brackets, at most 999 characters, not blank.
func scanDefinitionLabel(text string, pos int) (int, bool) {
	if pos >= len(text) || text[pos] != '[' {
		return 0, false
	}
	blank := true
	length := 0
	for idx := pos + 1; idx < len(text); idx++ {
		switch ch := text[idx]; ch {
		case '\\':
			blank = false
			if idx+1 < len(text) && mdscan.IsASCIIPunct(text[idx+1]) {
				idx++
				length++
			}
		case '[':
			return 0, false
		case ']':
			return idx, !blank
		case ' ', '\t', '\n':
		default:
			blank = false
		}
		// Only the characters inside the brackets count toward the limit,
		// in characters rather than bytes.
		if length += runeStarts(text[idx]); length > maxLabelLength {
			return 0, false
		}
	}
	return 0, false
}

// skipDefinitionSpace skips spaces, tabs, and at most one line ending.
func skipDefinitionSpace(text string, idx int) int {
	newline := false
	for idx < len(text) {
		switch text[idx] {
		case ' ', '\t':
		case '\n':
			if newline {
				return idx
			}
			newline = true
		default:
			return idx
		}
		idx++
	}
	return idx
}

// scanDefinitionDestination returns the end of a destination: "<...>" on one
// line, or a run of non-space characters with balanced parentheses.
func scanDefinitionDestination(text string, idx int) (int, bool) {
	if idx >= len(text) {
		return idx, false
	}
	if text[idx] == '<' {
		for end := idx + 1; end < len(text); end++ {
			switch text[end] {
			case '\\':
				end++
			case '\n', '<':
				return idx, false
			case '>':
				return end + 1, true
			}
		}
		return idx, false
	}
	depth := 0
	end := idx
	for end < len(text) && text[end] > ' ' && text[end] != 0x7f {
		switch text[end] {
		case '\\':
			if end+1 < len(text) && mdscan.IsASCIIPunct(text[end+1]) {
				end++
			}
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return end, end > idx
			}
			depth--
		}
		end++
	}
	return end, end > idx && depth == 0
}

// scanDefinitionTitle returns the offset after a title in double quotes,
// single quotes, or parentheses. A title cannot contain a blank line.
func scanDefinitionTitle(text string, idx int) (int, bool) {
	var closing byte
	switch text[idx] {
	case '"', '\'':
		closing = text[idx]
	case '(':
		closing = ')'
	default:
		return idx, false
	}
	for end := idx + 1; end < len(text); end++ {
		switch ch := text[end]; {
		case ch == '\\':
			end++
		case ch == closing:
			return end + 1, true
		case ch == '(' && closing == ')':
			return idx, false
		case ch == '\n' && end+1 < len(text) && text[end+1] == '\n':
			return idx, false
		}
	}
	return idx, false
}

// restOfLineBlank reports whether only spaces or tabs follow text[idx] on
// its line, and returns the offset after that line's ending.
func restOfLineBlank(text string, idx int) (int, bool) {
	for idx < len(text) && (text[idx] == ' ' || text[idx] == '\t') {
		idx++
	}
	if idx < len(text) && text[idx] != '\n' {
		return idx, false
	}
	return min(idx+1, len(text)), true
}
