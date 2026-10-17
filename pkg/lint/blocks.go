package lint

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Fixes of different rules can each be safe for the file as it is and still
// corrupt it together. One rule removes "$ " prompts in a code block while
// another changes its fences from tildes to backticks, and the prompt line
// "$ ```" becomes a closing fence. So in each pass the engine applies the
// fixes of only one rule to each top-level block (the front matter, or a
// child of the document root). The other rules' fixes in that block are
// deferred: the next pass lints the fixed content, and each rule derives
// and checks its fixes again. Fixes in different top-level blocks share a
// pass.

// topLevelBlocks holds the byte ranges of a file's top-level blocks, in
// document order. A block covers whole lines, from the start of its first
// line to the end of its last line ending.
type topLevelBlocks struct {
	starts []int
	ends   []int

	// leads[i] is the end of the lead of block i's first line: its indent
	// and the list marker after it, if the line starts with one.
	leads []int
}

// newTopLevelBlocks finds the top-level blocks of a parsed file. The front
// matter is a block of its own. After it, a block is a run of non-blank
// lines joined with any further runs that the full span of a top-level node
// covers, such as the runs of a code block with blank lines in it or of a
// list whose items are separated by blank lines. A full span includes the
// node's syntax markers, such as code fences and the closing line of an
// HTML block, and the spans of all its descendants (see package mdast).
// Treating two nodes as one block only defers more fixes, while splitting a
// node would let two rules' fixes into it in one pass.
//
// Fixes in two blocks can still change, together, how the second block
// relates to the first, through what continues across blank lines. A blank
// line ends a paragraph, a blockquote, a table and HTML blocks of types 6
// and 7. Fenced code and HTML blocks of types 1 to 5 run on across blank
// lines, but their full spans already join those lines. That leaves lists.
// After a blank line a list goes on with a sibling item, which starts at
// most three columns in with the same kind of marker, or with more content
// of an item, which must be indented at least as far as that content. A
// sibling item is in the list's full span already, and more content needs
// an indented line. So a run whose first line starts with a space or a tab
// joins the run before it: a fix in either may decide whether that line
// continues an item of the other, as when one rule narrows the space after
// an item's marker while another indents the next list. An edit that starts
// in the lead of a block's first line (see touchedBy) touches the block
// before it too, since it may indent the line or change its marker, and so
// make it continue an item or the list before it.
//
// It takes time linear in the number of lines and top-level nodes.
func newTopLevelBlocks(file *mdast.FileSnapshot) *topLevelBlocks {
	lines := file.Lines
	frontMatter := min(max(file.FrontMatterLines, 0), len(lines))

	// run[i] numbers the run of non-blank lines that line i belongs to, or
	// is -1 for a blank line or a front matter line. The byte order mark
	// that can start the first line is not text: the parser skips it.
	run := make([]int, len(lines))
	runs := 0
	for idx := range lines {
		switch {
		case idx < frontMatter || isBlankText(LineText(file, idx+1)):
			run[idx] = -1
		case idx > frontMatter && run[idx-1] >= 0:
			run[idx] = run[idx-1]
		default:
			run[idx] = runs
			runs++
		}
	}

	// joined[r] is true when run r and run r+1 belong to one block. A run
	// whose first line is indented joins the run before it. A node spans
	// consecutive lines, so it joins consecutive runs. Top-level nodes do not
	// overlap, so each line is visited about once.
	joined := make([]bool, runs)
	for idx := frontMatter + 1; idx < len(lines); idx++ {
		if current := run[idx]; current > 0 && run[idx-1] < 0 && startsIndented(LineText(file, idx+1)) {
			joined[current-1] = true
		}
	}
	if file.Root != nil {
		for node := file.Root.FirstChild; node != nil; node = node.Next {
			first, last, ok := nodeLines(file, node)
			if !ok {
				continue
			}
			prev := -1
			for idx := first; idx <= last; idx++ {
				current := run[idx]
				if current < 0 {
					continue
				}
				if prev >= 0 {
					for joinedRun := prev; joinedRun < current; joinedRun++ {
						joined[joinedRun] = true
					}
				}
				prev = current
			}
		}
	}

	blocks := &topLevelBlocks{}
	if frontMatter > 0 {
		blocks.add(0, lines[frontMatter-1].EndOffset, 0)
	}
	first := -1
	for idx := frontMatter; idx < len(lines); idx++ {
		if run[idx] < 0 {
			continue
		}
		if first < 0 {
			first = idx
		}
		lastOfRun := idx+1 == len(lines) || run[idx+1] != run[idx]
		if lastOfRun && !joined[run[idx]] {
			lead := lines[first].StartOffset + LineTextOffset(file, first+1) + leadLength(LineText(file, first+1))
			blocks.add(lines[first].StartOffset, lines[idx].EndOffset, lead)
			first = -1
		}
	}
	return blocks
}

func (b *topLevelBlocks) add(start, end, lead int) {
	b.starts = append(b.starts, start)
	b.ends = append(b.ends, end)
	b.leads = append(b.leads, lead)
}

// startsIndented reports whether a line starts with a space or a tab.
func startsIndented(text []byte) bool {
	return len(text) > 0 && isBlank(text[0])
}

// leadLength returns the length of a line's lead: its leading spaces and
// tabs, and the list marker after them if the line starts with one: "-",
// "+" or "*", or up to nine digits followed by "." or ")".
func leadLength(text []byte) int {
	length := 0
	for length < len(text) && isBlank(text[length]) {
		length++
	}
	if length < len(text) && strings.IndexByte("-+*", text[length]) >= 0 {
		return length + 1
	}
	digits := 0
	for digits < 9 && length+digits < len(text) && text[length+digits] >= '0' && text[length+digits] <= '9' {
		digits++
	}
	if end := length + digits; digits > 0 && end < len(text) && (text[end] == '.' || text[end] == ')') {
		return end + 1
	}
	return length
}

// isBlankText reports whether text holds only spaces and tabs.
func isBlankText(text []byte) bool {
	for _, char := range text {
		if !isBlank(char) {
			return false
		}
	}
	return true
}

// nodeLines returns the indexes of the first and last line of node's full
// span. It reports false when the node has no source range.
func nodeLines(file *mdast.FileSnapshot, node *mdast.Node) (int, int, bool) {
	if node.SourceSpan == nil && (node.FirstToken < 0 || node.LastToken < 0) {
		return 0, 0, false
	}
	span := node.SourceRange()
	startLine, _ := file.LineAt(span.StartOffset)
	endLine, _ := file.LineAt(max(span.EndOffset-1, span.StartOffset))
	if startLine == 0 || endLine == 0 {
		return 0, 0, false
	}
	// LineAt counts lines from 1.
	return startLine - 1, endLine - 1, true
}

// touchedBy returns the blocks that a fix's edits touch, in ascending
// order. An edit touches every block it overlaps. An edit in the space
// between two blocks, or an insertion at either edge of that space, touches
// the blocks on both sides, as does an edit that reaches the end of a block
// (its last line ending, which separates it from what follows). An edit
// that starts in the lead of a block's first line, its indent and any list
// marker after it, or right at the end of the lead, touches the block
// before it too (see newTopLevelBlocks). Without blocks, every edit touches
// block 0.
func (b *topLevelBlocks) touchedBy(edits []fix.TextEdit) []int {
	if len(b.starts) == 0 {
		return []int{0}
	}
	var touched []int
	for _, edit := range edits {
		touched = b.appendTouched(touched, edit)
	}
	slices.Sort(touched)
	return slices.Compact(touched)
}

// appendTouched appends the blocks that edit touches.
func (b *topLevelBlocks) appendTouched(touched []int, edit fix.TextEdit) []int {
	start, end := edit.StartOffset, edit.EndOffset
	insertion := start == end
	count := len(b.starts)

	// A block that ends before the edit starts cannot be touched: the
	// space after it is handled with the block that follows it.
	for idx := sort.SearchInts(b.ends, start); idx < count; idx++ {
		blockStart, blockEnd := b.starts[idx], b.ends[idx]

		// The space before this block: from the end of the previous block,
		// or from the start of the file.
		gapStart := 0
		if idx > 0 {
			gapStart = b.ends[idx-1]
		}
		inGap := start < blockStart && end > gapStart
		if insertion {
			inGap = gapStart <= start && start <= blockStart
		}
		if inGap {
			touched = append(touched, idx)
			if idx > 0 {
				touched = append(touched, idx-1)
			}
		}
		if blockStart > end {
			break
		}

		// An edit in the lead of the first line can indent the line or
		// change its marker.
		if idx > 0 && blockStart <= start && start <= b.leads[idx] {
			touched = append(touched, idx-1)
		}

		overlaps := start < blockEnd && end > blockStart
		if insertion {
			overlaps = blockStart <= start && start <= blockEnd
		}
		if overlaps {
			touched = append(touched, idx)
			if end >= blockEnd && idx+1 < count {
				touched = append(touched, idx+1)
			}
		}
	}

	// The space after the last block.
	if last := count - 1; end > b.ends[last] || (insertion && start >= b.ends[last]) {
		touched = append(touched, last)
	}
	return touched
}

// selectFixes chooses the fixes to apply in this pass and returns them, in
// input order, with the deferred fixes, also in input order. Rules are
// served in ID order. A rule's fixes are taken when no earlier rule took a
// block they touch. They are taken or deferred together for all the
// blocks they connect, since the rule checked them as a set.
func selectFixes(fixes []pendingFix, blocks *topLevelBlocks) ([]pendingFix, []pendingFix) {
	touched := make([][]int, len(fixes))
	order := make([]int, len(fixes))
	for idx, pending := range fixes {
		touched[idx] = blocks.touchedBy(pending.edits)
		order[idx] = idx
	}
	slices.SortStableFunc(order, func(first, second int) int {
		return cmp.Compare(fixes[first].ruleID, fixes[second].ruleID)
	})

	owner := make(map[int]string)
	take := make([]bool, len(fixes))
	for low := 0; low < len(order); {
		high := low
		for high < len(order) && fixes[order[high]].ruleID == fixes[order[low]].ruleID {
			high++
		}
		takeRuleFixes(fixes[order[low]].ruleID, order[low:high], touched, owner, take)
		low = high
	}

	var selected, deferred []pendingFix
	for idx, pending := range fixes {
		if take[idx] {
			selected = append(selected, pending)
		} else {
			deferred = append(deferred, pending)
		}
	}
	return selected, deferred
}

// takeRuleFixes decides on one rule's fixes, the indexes in members. Fixes
// that touch a common block form one group with all the blocks they touch.
// A group is taken, and its blocks given to the rule, when no other rule
// owns any of those blocks.
func takeRuleFixes(ruleID string, members []int, touched [][]int, owner map[int]string, take []bool) {
	parent := make(map[int]int)
	find := func(block int) int {
		for parent[block] != block {
			parent[block] = parent[parent[block]]
			block = parent[block]
		}
		return block
	}
	for _, member := range members {
		for _, block := range touched[member] {
			if _, ok := parent[block]; !ok {
				parent[block] = block
			}
		}
		for _, block := range touched[member][1:] {
			parent[find(block)] = find(touched[member][0])
		}
	}

	free := make(map[int]bool)
	for block := range parent {
		root := find(block)
		if _, seen := free[root]; !seen {
			free[root] = true
		}
		if other, owned := owner[block]; owned && other != ruleID {
			free[root] = false
		}
	}
	for block := range parent {
		if free[find(block)] {
			owner[block] = ruleID
		}
	}
	for _, member := range members {
		take[member] = free[find(touched[member][0])]
	}
}
