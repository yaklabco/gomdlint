package rules

import (
	"bytes"
	"cmp"
	"slices"
	"sort"

	gmast "github.com/yuin/goldmark/ast"

	"github.com/yaklabco/gomdlint/pkg/fix"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// Item regions, and the lists and tables in items: see the top comment of
// fix_regions.go for why they decide as the whole document does.

// openerSteps is how many groups before its first an item region may start
// at instead, when no opener reads the item as the document does where the
// region starts (see fitItemRegion).
const openerSteps = 3

// layoutItem is an item of a list of the layout.
type layoutItem struct {
	// index is the item's index in items, group the document group that
	// holds it, list the index of its list in lists and number its place in
	// the list.
	index, group int
	list, number int
	// line is the source offset of the item's marker line, content where its
	// content starts on that line, and offset the content offset goldmark
	// gives the item, which decides how the lines after it continue it.
	line, content, offset int
	// prefix opens the item, with the lists and items around it, on one line
	// up to where the item's content starts (see Item regions). It is nil
	// when it cannot be built: then nothing in the item takes regions of its
	// own.
	prefix []byte
	// regions reports whether item regions may be used for this item: it
	// has a prefix, the parse placed two groups or more in it, in order, the
	// first on its marker line, its first block lies in the first group, and
	// it makes its list loose exactly when its own flag or a block it counts
	// in flagged does.
	regions bool
	// bounds, afterClosed, blank, nodes and groupNodes hold the item's
	// groups as those of a layoutQuote hold a blockquote's; without regions,
	// bounds holds the item's marker line alone, as one group.
	bounds      []int
	afterClosed []bool
	blank       []bool
	nodes       []gmast.Node
	groupNodes  []int
	// loose reports whether the item makes its list loose
	// (gmparser.ListItemLoose), own whether it does by following a blank
	// line, and flagged counts, before each group and after the last, the
	// blocks after the item's first block that follow a blank line, each of
	// which makes the list loose.
	loose, own bool
	flagged    []int
	// openers holds the opener of an item region that starts at each group,
	// found on first use (see itemOpener).
	openers []itemOpener
}

// itemOpener is the text that a region of an item's groups starting at one
// group is parsed after, and the number of blocks it puts in the item. text
// is nil when no opener reads the item as the document does there.
type itemOpener struct {
	checked bool
	text    []byte
	skip    int
}

// addItems records the items of a list of the layout and, in the items that
// have a prefix, the lists and tables they hold. parentPrefix is the prefix
// of the item that holds the list, and nil for a list of the document or of
// a top-level blockquote.
func (l *docLayout) addItems(list *gmast.List, model *layoutList, rec *parseRecord, parentPrefix []byte) {
	var nodes []*gmast.ListItem
	for node := list.FirstChild(); node != nil; node = node.NextSibling() {
		if listItem, ok := node.(*gmast.ListItem); ok {
			nodes = append(nodes, listItem)
		}
	}
	// The items' places come from the list's marker lines, one for each item.
	if len(nodes) != len(model.lines) {
		model.regions = false
	}
	for number, node := range nodes {
		item := newLayoutItem(node, rec, l.source, model, number, len(l.items), parentPrefix)
		l.itemIndex[node] = item.index
		l.items = append(l.items, item)
		model.items = append(model.items, item.index)
	}
	for _, at := range model.items {
		item := l.items[at]
		if item.prefix == nil {
			continue
		}
		group := 0
		for idx, child := range item.nodes {
			for group+1 < len(item.bounds) && item.groupNodes[group+1] <= idx {
				group++
			}
			l.addBlock(child, rec, model.group, layoutScope{kind: itemScope, at: item.index, group: group}, item.prefix)
		}
	}
}

// newLayoutItem records the groups of the item at number in a list of the
// layout, whose items before it have been recorded.
func newLayoutItem(
	node *gmast.ListItem, rec *parseRecord, source []byte, list *layoutList, number, index int, parentPrefix []byte,
) *layoutItem {
	item := &layoutItem{
		index: index, group: list.group, list: list.index, number: number, offset: node.Offset,
		nodes: children(node), own: node.PreviousSibling() != nil && node.HasBlankPreviousLines(),
	}
	if number < len(list.lines) {
		item.line = list.lines[number]
		item.loose = list.loose[number+1]-list.loose[number] == 1
	}
	item.bounds = []int{item.line}
	item.afterClosed, item.blank = []bool{false}, []bool{false}
	item.groupNodes = []int{0, len(item.nodes)}
	if !list.regions || number >= len(list.lines) || (list.parent.kind == itemScope && parentPrefix == nil) {
		return item
	}
	item.content, item.prefix = itemPrefix(node, rec, source, item.line, list.parent.kind == itemScope, parentPrefix)
	if item.prefix == nil {
		return item
	}
	starts := rec.items[node]
	counts, ok := groupBlocks(item.nodes, starts)
	regions := ok && len(starts) >= 2 && starts[0].offset == item.line && counts[0] == 0 && slices.IsSorted(counts)
	for idx := 1; regions && idx < len(starts); idx++ {
		regions = starts[idx].offset > starts[idx-1].offset
	}
	if !regions {
		return item
	}
	counts = append(counts, len(item.nodes))
	// The item's first block must lie in its first group, so that every
	// block of a region comes after it; then the flags of the blocks after
	// it decide, as ListItemLoose reads them.
	end := slices.Index(item.nodes, gmparser.ItemFirstBlockEnd(node, source))
	if end < 0 || end >= counts[1] {
		return item
	}
	before := make([]int, len(item.nodes)+1)
	for idx, child := range item.nodes {
		before[idx+1] = before[idx]
		if idx > end && child.HasBlankPreviousLines() {
			before[idx+1]++
		}
	}
	flagged := make([]int, len(counts))
	for group, count := range counts {
		flagged[group] = before[count]
	}
	if item.loose != (item.own || flagged[len(flagged)-1] > 0) {
		return item
	}
	item.regions = true
	item.bounds = item.bounds[:0]
	item.afterClosed, item.blank = item.afterClosed[:0], item.blank[:0]
	for _, start := range starts {
		item.bounds = append(item.bounds, start.offset)
		item.afterClosed = append(item.afterClosed, start.afterClosed)
		item.blank = append(item.blank, start.opened.HasBlankPreviousLines())
	}
	item.groupNodes = counts
	item.flagged = flagged
	return item
}

// itemPrefix returns where an item's content starts and its prefix: the
// bytes of its marker line up to there, after the bytes that the containers
// around its list take off the line, and after the prefix of the item that
// holds the list, for a nested list. The prefix is nil when the content does
// not start on the marker line, or the bytes up to it hold a tab, whose
// width depends on its column.
func itemPrefix(
	node gmast.Node, rec *parseRecord, source []byte, line int, nested bool, parentPrefix []byte,
) (int, []byte) {
	first := node.FirstChild()
	if first == nil {
		return 0, nil
	}
	content, found := rec.opens[first]
	// goldmark turns the paragraphs of a list it reads as tight into text
	// blocks when the list closes, so the item's first block is not the one
	// that opened; a paragraph's first line starts where its block opened,
	// at the item's content.
	if textBlock, isText := first.(*gmast.TextBlock); !found && isText && textBlock.Lines().Len() > 0 {
		content, found = textBlock.Lines().At(0).Start, true
	}
	if !found || content < line || lineStartIn(source, content) != line ||
		bytes.ContainsAny(source[line:content], "\t\r\n") {
		return 0, nil
	}
	from := line
	if nested {
		open, opened := rec.opens[node]
		if !opened || open < line || open > content {
			return 0, nil
		}
		from = open
	}
	return content, slices.Concat(parentPrefix, source[from:content])
}

// groupOf returns the group of the item that holds the line starting at
// offset; a line before the first group's is counted in it.
func (it *layoutItem) groupOf(offset int) int {
	return sort.SearchInts(it.bounds[1:], offset+1)
}

// itemRegionFits reports whether an item region lies in groups that item
// regions may take: after the item's first group, which holds its marker
// line, and before its last, so that it ends at the start of a group.
func (l *docLayout) itemRegionFits(reg region) bool {
	item := l.items[reg.at]
	return item.regions && reg.first >= 1 && reg.last+1 <= len(item.bounds)-1
}

// fitItemRegion returns an item region that fits, starting at its first
// group or at most openerSteps groups before it, at the first of them where
// an opener reads the item as the document does (see itemOpener). ok is
// false when there is none.
func (l *docLayout) fitItemRegion(reg region) (region, bool) {
	if !l.itemRegionFits(reg) {
		return reg, false
	}
	for first := reg.first; first >= max(1, reg.first-openerSteps); first-- {
		if l.itemOpener(reg.at, first).text != nil {
			reg.first = first
			return reg, true
		}
	}
	return reg, false
}

// itemOpener returns the opener of an item region that starts at the
// item's group at index group (see Item regions), worked out on first use.
// It is the item's prefix followed by lines that leave the item as the
// document has it before the group's first line, ending with the line before
// it in the document: the rest of the marker line when that is the line
// before; else, when a block that closes alone is open or the line before is
// blank, an empty ATX heading and that line; else, after a block that the
// line before closes, a fence or an HTML block that the line closes, opened,
// and that line (see closingOpeners). The opener holds when its parse,
// followed by the group's first line, opens the item's chain as the document
// does and starts a group of the item on that line as the document starts
// it; of several, the first that holds is taken.
func (l *docLayout) itemOpener(at, group int) *itemOpener {
	item := l.items[at]
	if item.openers == nil {
		item.openers = make([]itemOpener, len(item.bounds))
	}
	opener := &item.openers[group]
	if opener.checked {
		return opener
	}
	opener.checked = true
	start := item.bounds[group]
	prevStart := lineStartIn(l.source, start-1)
	prev := l.source[prevStart:start]
	var candidates [][]byte
	switch {
	case prevStart == item.line:
		candidates = append(candidates, slices.Concat(item.prefix, l.source[item.content:start]))
	case item.afterClosed[group] || item.blank[group]:
		candidates = append(candidates, slices.Concat(item.prefix, []byte("#\n"), prev))
	default:
		for _, open := range closingOpeners(prev) {
			candidates = append(candidates, slices.Concat(item.prefix, open, []byte("\n"), prev))
		}
	}
	line := l.source[start:nextLineStart(l.source, start)]
	for _, text := range candidates {
		if skip, ok := l.openerHolds(at, group, text, line); ok {
			opener.text, opener.skip = text, skip
			break
		}
	}
	return opener
}

// openerHolds reports whether text opens the item at index at as the
// document has it before the first line of its group at index group, which
// is line: parsed with line after it, it opens the item's chain as the
// document does, and starts a group of the item at line as the document
// starts it there. skip is the number of the item's blocks before that
// group.
func (l *docLayout) openerHolds(at, group int, text, line []byte) (int, bool) {
	item := l.items[at]
	check := slices.Concat(text, line)
	l.work.parsed += len(check)
	parsed := parseRecorded(l.markdown, check, l.refIndex, false, l.tableFlags)
	node, ok := l.parsedContainer(parsed.doc, itemScope, at)
	if !ok {
		return 0, false
	}
	for _, opened := range parsed.rec.items[node] {
		if opened.offset != len(text) {
			continue
		}
		if opened.afterClosed != item.afterClosed[group] || opened.opened.HasBlankPreviousLines() != item.blank[group] {
			return 0, false
		}
		counts, found := groupBlocks(children(node), []groupStart{opened})
		if !found {
			return 0, false
		}
		return counts[0], true
	}
	return 0, false
}

// htmlClosers pairs the end conditions of the HTML blocks that goldmark
// closes on a later line than their first with a line that opens such a
// block and does not close it.
//
//nolint:gochecknoglobals // a fixed table, read only.
var htmlClosers = []struct{ closes, opens string }{
	{"-->", "<!--"},
	{"?>", "<?"},
	{"]]>", "<![CDATA["},
	{"</pre>", "<pre>"},
	{"</script>", "<script>"},
	{"</style>", "<style>"},
	{"</textarea>", "<textarea>"},
	{">", "<!A"},
}

// closingOpeners returns the lines that open a block that line closes on
// a later line than its first: a fence of the characters of a line that
// closes a fence, or the opening of each HTML block whose end condition the
// line holds. The opener's parse decides which of them, if any, stands for
// the document.
func closingOpeners(line []byte) [][]byte {
	if fence := closingFence(line); fence != nil {
		return [][]byte{fence}
	}
	lower := bytes.ToLower(line)
	var opens [][]byte
	for _, closer := range htmlClosers {
		if bytes.Contains(lower, []byte(closer.closes)) {
			opens = append(opens, []byte(closer.opens))
		}
	}
	return opens
}

// closingFence returns the fence characters of a line that closes a fence, and
// nil for a line that does not look like one: the line, without the
// blockquote markers and spaces before it and the spaces and line ending
// after it, is three or more backticks or tildes.
func closingFence(line []byte) []byte {
	run := bytes.TrimRight(bytes.TrimLeft(line, " >"), " \r\n")
	if len(run) < 3 || (run[0] != '`' && run[0] != '~') || bytes.Count(run, run[:1]) != len(run) {
		return nil
	}
	return bytes.Clone(run)
}

// parsedContainer returns the node that stands, in the parse of a region's
// text, for the document, the top-level blockquote at index at, or the item
// at index at, by kind: the document, its one block, a blockquote, or the
// one item of the one list that is the one block of the node that stands
// for the list's container. The list must have the marker and kind of the
// item's list and the item its content offset, which decide, with the
// line, how goldmark continues them on each line. ok is false when the
// parse is not of that shape.
func (l *docLayout) parsedContainer(doc gmast.Node, kind scopeKind, at int) (gmast.Node, bool) {
	switch kind {
	case docScope:
		return doc, true
	case quoteScope:
		quote, ok := soleQuote(children(doc))
		if !ok {
			return nil, false
		}
		return quote, true
	}
	item := l.items[at]
	list := l.lists[item.list]
	parent, ok := l.parsedContainer(doc, list.parent.kind, list.parent.at)
	if !ok || parent.FirstChild() == nil || parent.FirstChild() != parent.LastChild() {
		return nil, false
	}
	parsedList, ok := parent.FirstChild().(*gmast.List)
	if !ok || parsedList.Marker != list.marker || parsedList.IsOrdered() != list.ordered ||
		parsedList.FirstChild() == nil || parsedList.FirstChild() != parsedList.LastChild() {
		return nil, false
	}
	parsedItem, ok := parsedList.FirstChild().(*gmast.ListItem)
	if !ok || parsedItem.Offset != item.offset {
		return nil, false
	}
	return parsedItem, true
}

// scopeOpener returns the text that the region of a list or table in scope
// is parsed after: for a list or table in an item, the item's prefix and an
// empty ATX heading, which opens the item and the lists and items around it
// with a block that closes on the next line whatever it holds (see Item
// regions), and nothing otherwise.
func (l *docLayout) scopeOpener(scope layoutScope) []byte {
	if scope.kind != itemScope {
		return nil
	}
	return slices.Concat(l.items[scope.at].prefix, []byte("#\n"))
}

// openerBlocks returns the number of blocks that scopeOpener puts in the
// container of a list or table in scope: its heading in an item.
func openerBlocks(scope layoutScope) int {
	if scope.kind == itemScope {
		return 1
	}
	return 0
}

// scopeBlocks returns the blocks, in the parse of the region of a list or
// table in scope, that stand for the blocks of the scope's container: those
// of the document or of its one blockquote, or those after the opener's
// heading in the one item of the item's chain. ok is false when the parse is
// not of that shape.
func (l *docLayout) scopeBlocks(doc gmast.Node, scope layoutScope) ([]gmast.Node, bool) {
	node, ok := l.parsedContainer(doc, scope.kind, scope.at)
	if !ok {
		return nil, false
	}
	blocks := children(node)
	if scope.kind != itemScope {
		return blocks, true
	}
	if len(blocks) == 0 {
		return nil, false
	}
	if _, isHeading := blocks[0].(*gmast.Heading); !isHeading {
		return nil, false
	}
	return blocks[1:], true
}

// parseItemRegion parses an item region after its opener (see itemOpener),
// followed by the first line of the item's group after the region. The
// region holds when the parse opens the item's chain as the document does
// and starts a group of the item at that line as the original's next group
// starts: after a closed block or not, with the first block opened there
// marked as following a blank line or not as the original's. A region whose
// edits take its last line ending away does not hold.
func (l *docLayout) parseItemRegion(reg region, edits []fix.TextEdit) regionParse {
	item := l.items[reg.at]
	opener := l.itemOpener(reg.at, reg.first)
	if opener.text == nil {
		return regionParse{}
	}
	from, to := l.span(reg)
	body := applyWithin(l.source, from, to, edits)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		return regionParse{}
	}
	end := len(opener.text) + len(body)
	text := slices.Concat(opener.text, body, l.source[to:nextLineStart(l.source, to)])
	l.work.parsed += len(text)
	parsed := parseRecorded(l.markdown, text, l.refIndex, false, l.tableFlags)
	node, ok := l.parsedContainer(parsed.doc, itemScope, reg.at)
	if !ok {
		return regionParse{}
	}
	starts := parsed.rec.items[node]
	if len(starts) == 0 {
		return regionParse{}
	}
	next := starts[len(starts)-1]
	if next.offset != end || next.afterClosed != item.afterClosed[reg.last+1] ||
		next.opened.HasBlankPreviousLines() != item.blank[reg.last+1] {
		return regionParse{}
	}
	blocks := children(node)
	counts, ok := groupBlocks(blocks, []groupStart{next})
	if !ok || counts[0] < opener.skip {
		return regionParse{}
	}
	blocks = blocks[opener.skip:counts[0]]
	// The line after the region can start a paragraph of definitions, which
	// the document adds in the group after the region; the opener adds none.
	refs := parsed.rec.refs[:0]
	for _, ref := range parsed.rec.refs {
		if ref.offset < len(opener.text) {
			return regionParse{}
		}
		if ref.offset < end {
			refs = append(refs, ref)
		}
	}
	parsed.rec.refs = refs
	held := l.heldParse(from, to, text, end, blocks, parsed.rec)
	held.skip = opener.skip
	for _, block := range blocks {
		if block.HasBlankPreviousLines() {
			held.blankBlock = true
			break
		}
	}
	return held
}

// loosensList reports what a region that holds, parsed as parse, tells of
// the tightness of its list: for a list region, whether it makes its list
// loose; for an item region, whether it holds a block that follows a blank
// line (see itemTally).
func (l *docLayout) loosensList(reg region, parse regionParse) bool {
	switch reg.kind {
	case listRegion:
		return l.lists[reg.at].loosens(reg, parse.tight)
	case itemRegion:
		return parse.blankBlock
	default:
		return false
	}
}

// itemTally is what the item regions of one item hold: how many there are,
// the blocks of the original in them that make the item's list loose, and
// how many hold a block that follows a blank line.
type itemTally struct {
	regions, inside, blank int
}

// plus returns the tally of the regions of two tallies.
func (t itemTally) plus(other itemTally) itemTally {
	return itemTally{regions: t.regions + other.regions, inside: t.inside + other.inside, blank: t.blank + other.blank}
}

// minus returns the tally without the regions of other.
func (t itemTally) minus(other itemTally) itemTally {
	return itemTally{regions: t.regions - other.regions, inside: t.inside - other.inside, blank: t.blank - other.blank}
}

// tallyOf returns the tally of one item region of the item, which holds a
// block that follows a blank line when blank is set.
func (it *layoutItem) tallyOf(reg region, blank bool) itemTally {
	tally := itemTally{regions: 1, inside: it.flagged[reg.last+1] - it.flagged[reg.first]}
	if blank {
		tally.blank = 1
	}
	return tally
}

// contribution returns how the item counts for its list's tightness when
// its item regions hold tally: whether the item makes the list loose in the
// original and after the edits, as 1 or 0, or nothing without regions. It
// makes the list loose after the edits when it follows a blank line and is
// not the list's first item, a block of the original outside its regions
// after its first block follows a blank line, or a region holds such a
// block.
func (it *layoutItem) contribution(tally itemTally) (int, int) {
	if tally.regions == 0 {
		return 0, 0
	}
	inside, loosening := 0, 0
	if it.loose {
		inside = 1
	}
	if it.own || it.flagged[len(it.flagged)-1]-tally.inside > 0 || tally.blank > 0 {
		loosening = 1
	}
	return inside, loosening
}

// layoutUnit is a part of a list, table, blockquote or item that a region
// can take as its first or last: an item of a list but the last, a body row
// of a table, or a group of a blockquote or item but the last. Where the
// last item or group ends is not recorded. Units nest, as what holds them
// does; parent is the index of the innermost unit that holds this one, or
// -1.
type layoutUnit struct {
	start, end int
	kind       regionKind
	at, index  int
	parent     int
}

// unitIndex returns the units of the lists, tables, blockquotes and items
// with regions, sorted by start, a unit before those it holds, built on
// first use.
func (l *docLayout) unitIndex() []layoutUnit {
	if l.units != nil {
		return l.units
	}
	units := []layoutUnit{}
	add := func(kind regionKind, at int, bounds []int) {
		for idx := 0; idx+1 < len(bounds); idx++ {
			units = append(units, layoutUnit{start: bounds[idx], end: bounds[idx+1], kind: kind, at: at, index: idx, parent: -1})
		}
	}
	for _, list := range l.lists {
		if list.regions {
			add(listRegion, list.index, list.lines)
		}
	}
	for _, table := range l.tables {
		if table.regions {
			add(tableRegion, table.index, table.rows)
		}
	}
	for _, quote := range l.quotes {
		if quote.regions {
			add(quoteRegion, quote.index, quote.bounds)
		}
	}
	for _, item := range l.items {
		if item.regions {
			add(itemRegion, item.index, item.bounds)
		}
	}
	slices.SortFunc(units, func(a, b layoutUnit) int {
		return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(b.end, a.end),
			cmp.Compare(a.kind, b.kind), cmp.Compare(a.at, b.at))
	})
	var open []int
	for idx := range units {
		unit := &units[idx]
		for len(open) > 0 && !units[open[len(open)-1]].holds(unit.start, unit.end) {
			open = open[:len(open)-1]
		}
		if len(open) > 0 {
			unit.parent = open[len(open)-1]
		}
		open = append(open, idx)
	}
	l.units = units
	return units
}

// holds reports whether a unit holds the source offsets from start to end.
func (u layoutUnit) holds(start, end int) bool {
	return u.start <= start && end <= u.end
}

// innermostUnit returns the index of the innermost unit that holds the line
// starting at offset, or -1.
func innermostUnit(units []layoutUnit, offset int) int {
	at := sort.Search(len(units), func(i int) bool { return units[i].start > offset }) - 1
	for at >= 0 && !units[at].holds(offset, offset+1) {
		at = units[at].parent
	}
	return at
}

// unitUsable reports whether regions may still use a unit's list, table,
// blockquote or item, which a check may turn off once the layout is built.
func (l *docLayout) unitUsable(unit layoutUnit) bool {
	switch unit.kind {
	case listRegion:
		return l.lists[unit.at].regions
	case tableRegion:
		return l.tables[unit.at].regions
	case quoteRegion:
		return l.quotes[unit.at].regions
	default:
		return l.items[unit.at].regions
	}
}

// unitHolding returns the unit of the list, table, blockquote or item at
// index at, by kind, that holds the line starting at offset. ok is false
// when no unit of it does.
func (l *docLayout) unitHolding(kind regionKind, at, offset int) (int, bool) {
	switch kind {
	case listRegion:
		list := l.lists[at]
		item := list.itemOf(offset)
		return item, item >= 0 && item < len(list.lines)-1
	case tableRegion:
		table := l.tables[at]
		row := table.rowOf(offset)
		return row, row >= 0 && row < len(table.rows)-1
	case quoteRegion:
		quote := l.quotes[at]
		return quote.groupOf(offset), offset >= quote.bounds[0] && offset < quote.bounds[len(quote.bounds)-1]
	default:
		item := l.items[at]
		return item.groupOf(offset), offset >= item.bounds[0] && offset < item.bounds[len(item.bounds)-1]
	}
}
