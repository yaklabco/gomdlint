package rules

import (
	"bytes"
	"cmp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	goldparser "github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// Region checks.
//
// The fix check compares the render signature of the whole document before
// and after a fix (see fixCheck). Parsing the whole document for every fix
// made a long document with many findings take time that grows with the
// square of its length. A region check decides as the whole-document check
// does, while it parses only the part of the document that a fix can
// change. The argument rests on how goldmark 1.7 parses.
//
// Groups. goldmark reads blocks line by line. When no block is open, it skips
// blank lines and opens the next block (the outer loop of
// parser.parseBlocks, the only caller of the reader's SkipBlankLines). From
// there on, the parse is a function of the rest of the source: no open
// block, container offset or blank-line statistic carries over. The context
// flags of the list parsers are reset when a list or item opens, before they
// are read; the setext and fence flags live only while their block is open;
// the first block opened there is marked as following a blank line whatever
// came before; and link reference definitions only matter to inline parsing,
// which starts once all blocks are closed. So the document falls into
// groups, each starting at a line where the parser opened a block with no
// block open, and the blocks of a group parse the same on their own as in
// the document, given the document's definitions. A group runs from its
// first line to the first line of the next, blank lines included.
// docLayout records the groups of the original by wrapping the reader.
//
// Group regions. A group region is a run of whole groups that holds every
// line an edit changes; an edit that changes the line ending of a line
// changes the next line too. The lines before the region are the same after
// the edits, so the parser reaches the region with no block open, as before.
// The region, edited, is parsed on its own, followed by a sentinel line. When
// the parser starts a group at the sentinel, the region left no block open,
// so the lines after it, unchanged, parse as before. When it does not, the
// region grows into the next groups, up to the end of the document. Regions
// that hold, taken in document order, make up the edited document: the
// original's blocks outside them and each region's blocks.
//
// List regions. A long list is one group. Inside a top-level list, the items
// that edits change are parsed with the item before them and the item after
// them, whose lines are unchanged. The item before opens as in the document,
// and from its marker line on, the list and item parsers read each line as in
// the document: they decide from the line, the last item's content offset
// and emptiness, the list's marker, the blank-line statistics of the
// previous line and context flags that each item resets when it opens. The
// region holds when its parse is one list whose first item opens on the
// first line and whose last item opens on the marker line of the item after
// the edited ones: then that item, and the lines after it, parse as in the
// document. A list is tight when no item but the first follows a blank line
// and no item has a block but its first after a blank line
// (listParser.Close). The region's list decides that for its items but its
// first; that item's blank line and the items outside the regions come from
// the original, so the tightness of the edited list follows. docLayout
// checks that this rule gives goldmark's answer for every list of the
// original and uses no list regions for a list where it does not.
//
// Signatures. The signature of the document is its front matter and the
// signatures of its top-level blocks in order, and that of a list is a token
// with its tightness and the signatures of its items. So the signature of
// the edited document is the original's with each region's part replaced by
// the signature of the region's blocks or items, and the token of a list
// that changes tightness replaced, provided that: the edits leave the front
// matter alone (they start after it, see frontMatterLimit); every region
// adds the same link reference definitions in the same order, so that every
// link, before or after it, resolves as before (regions are parsed with the
// original's definitions); and the anchor counts after each region are as
// before, so that later headings keep their anchors (a region's headings get
// their anchors from the counts before it, see anchorHistory). Where
// definitions change, or anchor counts change and the signature up to that
// region does not already differ, region checks give up and the whole
// document is parsed, as before. sameAfterReplacements compares the replaced
// signature with the original exactly, without writing it out.
//
// The argument depends on goldmark's parser, not on what the signature
// compares, so docLayout can serve any fix check; regionModel adds the
// render signature that fixCheck compares. TestRegionChecksDecideAsWholeDocument
// checks the region decisions against the whole-document check.

// regionSentinel is the line parsed after a group region to find out whether
// the region leaves a block open.
const regionSentinel = "x"

// regionMarkdowns returns the recording goldmark instances for the parsers
// of a fixCheck, in the same order: GFM, and CommonMark unless the flavor is
// GFM. They are shared: a parse keeps all its state in its context.
func regionMarkdowns(flavor config.Flavor) []goldmark.Markdown {
	recordingOnce.Do(func() {
		recordingGFM = goldmark.New(goldmark.WithExtensions(gmext.GFM))
		recordingGFM.Parser().AddOptions(recordingOption{})
		recordingCommonMark = goldmark.New()
		recordingCommonMark.Parser().AddOptions(recordingOption{})
	})
	markdowns := []goldmark.Markdown{recordingGFM}
	if flavor != config.FlavorGFM {
		markdowns = append(markdowns, recordingCommonMark)
	}
	return markdowns
}

//nolint:gochecknoglobals // goldmark instances built once; each parse keeps its state in its own context.
var (
	recordingOnce       sync.Once
	recordingGFM        goldmark.Markdown
	recordingCommonMark goldmark.Markdown
)

// parseRecord collects what a recording parse observes.
type parseRecord struct {
	// doc is the document node, known once a block opens in it.
	doc gmast.Node
	// groups holds where the parser started each group.
	groups []groupStart
	// opens holds an offset on the line where each block opened.
	opens map[gmast.Node]int
	// refs holds the link reference definitions in the order the parser
	// added them, each with the offset of the paragraph it came from.
	refs []recordedRef
	// paragraph is the offset of the paragraph being transformed, or -1.
	paragraph int
}

// groupStart is the first line of a group and the last top-level block of
// the document when the group started, or nil. The block is recorded rather
// than counted, because goldmark miscounts children: inserting a node after
// the last child, as a table built from the last paragraph is, counts it
// twice.
type groupStart struct {
	offset int
	last   gmast.Node
}

// groupBlocks returns, for each group start, the number of the top-level
// blocks of nodes before it. ok is false when a block that was last at a
// group start is no longer among nodes, which no parser does once the
// blocks are closed.
func groupBlocks(nodes []gmast.Node, starts []groupStart) ([]int, bool) {
	index := make(map[gmast.Node]int, len(nodes))
	for idx, node := range nodes {
		index[node] = idx
	}
	counts := make([]int, len(starts))
	for idx, start := range starts {
		if start.last == nil {
			continue
		}
		pos, found := index[start.last]
		if !found {
			return nil, false
		}
		counts[idx] = pos + 1
	}
	return counts, true
}

// recordedRef is a link reference definition and the offset of the first
// line of the paragraph it came from, or -1 when that is not known.
type recordedRef struct {
	offset int
	ref    goldparser.Reference
}

// sameRef reports whether two definitions resolve links the same way.
func sameRef(a, b goldparser.Reference) bool {
	return gmutil.ToLinkReference(a.Label()) == gmutil.ToLinkReference(b.Label()) &&
		bytes.Equal(a.Destination(), b.Destination()) && bytes.Equal(a.Title(), b.Title())
}

// recordingContext records the link reference definitions a parse adds.
type recordingContext struct {
	goldparser.Context
	rec *parseRecord
}

func (c *recordingContext) AddReference(ref goldparser.Reference) {
	c.rec.refs = append(c.rec.refs, recordedRef{offset: c.rec.paragraph, ref: ref})
	c.Context.AddReference(ref)
}

// groupReader records where the parser starts each group: the parser skips
// blank lines only when no block is open, and then opens a block at the
// line it stops at.
type groupReader struct {
	gmtext.Reader
	rec *parseRecord
}

func (r *groupReader) SkipBlankLines() (gmtext.Segment, int, bool) {
	segment, lines, ok := r.Reader.SkipBlankLines()
	if ok {
		var last gmast.Node
		if r.rec.doc != nil {
			last = r.rec.doc.LastChild()
		}
		r.rec.groups = append(r.rec.groups, groupStart{offset: segment.Start, last: last})
	}
	return segment, lines, ok
}

// recordingOption wraps the block parsers and paragraph transformers of a
// goldmark parser so that they record where blocks open and where link
// reference definitions come from. The wrappers change nothing a parse
// produces and record nothing without a recordingContext. It must be added
// after the extensions.
type recordingOption struct{}

func (recordingOption) SetParserOption(cfg *goldparser.Config) {
	for idx := range cfg.BlockParsers {
		if parser, ok := cfg.BlockParsers[idx].Value.(goldparser.BlockParser); ok {
			cfg.BlockParsers[idx].Value = &recordingBlockParser{BlockParser: parser}
		}
	}
	for idx := range cfg.ParagraphTransformers {
		if transformer, ok := cfg.ParagraphTransformers[idx].Value.(goldparser.ParagraphTransformer); ok {
			cfg.ParagraphTransformers[idx].Value = &recordingParagraphTransformer{ParagraphTransformer: transformer}
		}
	}
}

// recordingBlockParser records the line where each block opens.
type recordingBlockParser struct {
	goldparser.BlockParser
}

func (p *recordingBlockParser) SetOption(name goldparser.OptionName, value any) {
	forwardParserOption(p.BlockParser, name, value)
}

func (p *recordingBlockParser) Open(
	parent gmast.Node, reader gmtext.Reader, pc goldparser.Context,
) (gmast.Node, goldparser.State) {
	_, segment := reader.PeekLine()
	node, state := p.BlockParser.Open(parent, reader, pc)
	if rc, ok := pc.(*recordingContext); ok && node != nil {
		rc.rec.opens[node] = segment.Start
		if parent.Kind() == gmast.KindDocument {
			rc.rec.doc = parent
		}
	}
	return node, state
}

// recordingParagraphTransformer records the paragraph that link reference
// definitions come from.
type recordingParagraphTransformer struct {
	goldparser.ParagraphTransformer
}

func (t *recordingParagraphTransformer) SetOption(name goldparser.OptionName, value any) {
	forwardParserOption(t.ParagraphTransformer, name, value)
}

func (t *recordingParagraphTransformer) Transform(node *gmast.Paragraph, reader gmtext.Reader, pc goldparser.Context) {
	rc, ok := pc.(*recordingContext)
	if !ok || node.Lines().Len() == 0 {
		t.ParagraphTransformer.Transform(node, reader, pc)
		return
	}
	outer := rc.rec.paragraph
	rc.rec.paragraph = node.Lines().At(0).Start
	t.ParagraphTransformer.Transform(node, reader, pc)
	rc.rec.paragraph = outer
}

// forwardParserOption passes a parser option to a wrapped parser that takes
// options.
func forwardParserOption(inner any, name goldparser.OptionName, value any) {
	if setter, ok := inner.(goldparser.SetOptioner); ok {
		setter.SetOption(name, value)
	}
}

// recordedParse is a document parsed with recording.
type recordedParse struct {
	doc gmast.Node
	rec *parseRecord
}

// parseRecorded parses source with a recording goldmark instance. The parse
// starts out with the given link reference definitions, so that links
// resolve as in the document a region comes from; the record holds only the
// definitions that source adds.
func parseRecorded(markdown goldmark.Markdown, source []byte, preset []recordedRef) *recordedParse {
	inner := goldparser.NewContext()
	for _, ref := range preset {
		inner.AddReference(ref.ref)
	}
	rec := &parseRecord{opens: make(map[gmast.Node]int), paragraph: -1}
	reader := &groupReader{Reader: gmtext.NewReader(source), rec: rec}
	doc := markdown.Parser().Parse(reader, goldparser.WithContext(&recordingContext{Context: inner, rec: rec}))
	return &recordedParse{doc: doc, rec: rec}
}

// children returns the children of a node.
func children(node gmast.Node) []gmast.Node {
	var nodes []gmast.Node
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		nodes = append(nodes, child)
	}
	return nodes
}

// docLayout is what region checks know of the original document from how one
// parser reads it, whatever a check compares: where its groups and the items
// of its top-level lists start, its link reference definitions, and where
// edits may start without reaching the front matter. It selects, merges,
// grows and parses the regions that edits need.
type docLayout struct {
	markdown goldmark.Markdown
	// source is the text the parser reads: the content without a byte order
	// mark, with front matter blanked. base is the length of the byte order
	// mark, so that a content offset is a source offset plus base, and
	// frontMatter the length of the front matter in source.
	source      []byte
	base        int
	frontMatter int
	// frontLimit is the content offset before which an edit could change
	// the front matter (see frontMatterLimit). firstLineFree reports that
	// the document has no front matter and its first line opens none, so
	// that an edit before frontLimit is safe when the edited first line
	// opens none either (see firstLineKeeps).
	frontLimit    int
	firstLineFree bool
	// nodes holds the top-level blocks of the original.
	nodes []gmast.Node
	// bounds holds the source offset where each group starts, 0 for the
	// first, and the length of the source; groupNodes holds the index in
	// nodes of the first block of each group, and the number of nodes.
	bounds     []int
	groupNodes []int
	// lists holds the top-level lists with items, in document order, and
	// listIndex the index in lists of each list node. searchable holds the
	// lists that may have list regions.
	lists      []*layoutList
	listIndex  map[gmast.Node]int
	searchable []*layoutList
	refs       []recordedRef
}

// layoutList is a top-level list of the original document.
type layoutList struct {
	index, group int
	// regions reports whether list regions may be used for this list.
	regions bool
	// lines holds the source offset of the marker line of each item.
	lines []int
	// blank reports whether each item follows a blank line, and loose
	// counts the items before each that make the list loose (itemLoose).
	blank []bool
	loose []int
}

// newDocLayout parses the original content with a recording parser. It
// returns nil when region checks cannot be used for it: when the parse has
// no groups, or the groups or the link reference definitions cannot be
// placed.
func newDocLayout(markdown goldmark.Markdown, content []byte) *docLayout {
	base := 0
	if bytes.HasPrefix(content, []byte(mdast.ByteOrderMark)) {
		base = len(mdast.ByteOrderMark)
	}
	frontMatter := frontMatterLength(content[base:])
	source := bytes.Clone(content[base:])
	for idx := range frontMatter {
		if source[idx] != '\n' && source[idx] != '\r' {
			source[idx] = ' '
		}
	}
	parsed := parseRecorded(markdown, source, nil)
	starts := parsed.rec.groups
	nodes := children(parsed.doc)
	counts, ok := groupBlocks(nodes, starts)
	if len(starts) == 0 || !ok || !slices.IsSorted(counts) {
		return nil
	}
	// Definitions are placed by the paragraph they come from; paragraphs
	// close in document order.
	for idx, ref := range parsed.rec.refs {
		if ref.offset < 0 || (idx > 0 && ref.offset < parsed.rec.refs[idx-1].offset) {
			return nil
		}
	}
	firstLine := source
	if end := bytes.IndexByte(source, '\n'); end >= 0 {
		firstLine = source[:end]
	}
	layout := &docLayout{
		markdown:      markdown,
		source:        source,
		base:          base,
		frontMatter:   frontMatter,
		frontLimit:    frontMatterLimit(content, base, frontMatter),
		firstLineFree: frontMatter == 0 && !opensFrontMatter(firstLine),
		nodes:         nodes,
		groupNodes:    append(counts, len(nodes)),
		listIndex:     make(map[gmast.Node]int),
		refs:          parsed.rec.refs,
	}
	layout.bounds = append(layout.bounds, 0)
	for _, start := range starts[1:] {
		if start.offset <= layout.bounds[len(layout.bounds)-1] {
			return nil
		}
		layout.bounds = append(layout.bounds, start.offset)
	}
	layout.bounds = append(layout.bounds, len(source))
	group := 0
	for idx, node := range nodes {
		for group+1 < len(starts) && counts[group+1] <= idx {
			group++
		}
		if list, isList := node.(*gmast.List); isList && list.FirstChild() != nil {
			model := newLayoutList(list, parsed.rec, source, len(layout.lists), group)
			layout.listIndex[node] = model.index
			layout.lists = append(layout.lists, model)
			if model.regions {
				layout.searchable = append(layout.searchable, model)
			}
		}
	}
	return layout
}

// newLayoutList records the items of a top-level list.
func newLayoutList(list *gmast.List, rec *parseRecord, source []byte, index, group int) *layoutList {
	model := &layoutList{index: index, group: group, regions: true, loose: []int{0}}
	loose := 0
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		offset, found := rec.opens[item]
		line := lineStartIn(source, offset)
		if !found || (len(model.lines) > 0 && line <= model.lines[len(model.lines)-1]) {
			model.regions = false
		}
		model.lines = append(model.lines, line)
		model.blank = append(model.blank, item.HasBlankPreviousLines())
		if itemLoose(item, item == list.FirstChild()) {
			loose++
		}
		model.loose = append(model.loose, loose)
	}
	// List regions rely on goldmark's rule for tight lists, so a list for
	// which the rule does not give goldmark's answer gets none.
	if (loose == 0) != list.IsTight {
		model.regions = false
	}
	return model
}

// itemLoose reports whether a list item makes its list loose: it follows a
// blank line and is not the first item, or one of its blocks but the first
// follows a blank line. This is how goldmark's listParser.Close decides.
func itemLoose(item gmast.Node, first bool) bool {
	if !first && item.HasBlankPreviousLines() {
		return true
	}
	if item.FirstChild() == nil || item.FirstChild() == item.LastChild() {
		return false
	}
	for child := item.FirstChild().NextSibling(); child != nil; child = child.NextSibling() {
		if child.HasBlankPreviousLines() {
			return true
		}
	}
	return false
}

// lineStartIn returns the offset of the start of the line holding offset.
func lineStartIn(source []byte, offset int) int {
	offset = min(max(offset, 0), len(source))
	return bytes.LastIndexByte(source[:offset], '\n') + 1
}

// frontMatterLimit returns the content offset from which an edit can neither
// change nor make front matter. Front matter is recognized from the first
// line: with front matter, an edit after its closing line cannot change it;
// without, an edit after the first line cannot make some, unless the first
// line opens front matter that no line closes yet, which any later line
// could, so that no offset is safe.
func frontMatterLimit(content []byte, base, frontMatter int) int {
	if frontMatter > 0 {
		return base + frontMatter
	}
	end := bytes.IndexByte(content[base:], '\n')
	if end < 0 {
		return len(content) + 1
	}
	switch strings.TrimRight(string(content[base:base+end]), " \t\r") {
	case yamlFrontMatterFence, tomlFrontMatterFence, "{":
		return len(content) + 1
	}
	return base + end + 1
}

// opensFrontMatter reports whether a first line, without its line ending,
// opens front matter, as frontMatterLength reads it.
func opensFrontMatter(line []byte) bool {
	switch string(bytes.TrimRight(line, " \t\r")) {
	case yamlFrontMatterFence, tomlFrontMatterFence, "{":
		return true
	}
	return false
}

// firstLineKeeps reports whether the edits that edits and more hold between
// them, in source offsets and each sorted, leave a document whose first line
// is free (see docLayout.firstLineFree) without front matter and as it was
// at its start: the edited first line opens no front matter, and the edited
// document starts with a byte order mark exactly when it did. Front matter
// is recognized from the first line, so the document then has none, as
// before, and its parse starts where it did.
func (l *docLayout) firstLineKeeps(edits, more []placedEdit) bool {
	var line []byte
	pos := 0
	for {
		fromEdits := len(edits) > 0 && (len(more) == 0 || compareEdits(edits[0], more[0]) <= 0)
		var next *placedEdit
		switch {
		case fromEdits:
			next = &edits[0]
		case len(more) > 0:
			next = &more[0]
		}
		switch {
		case next != nil && next.edit.StartOffset == pos:
			line = append(line, next.edit.NewText...)
			pos = next.edit.EndOffset
			if fromEdits {
				edits = edits[1:]
			} else {
				more = more[1:]
			}
		case pos < len(l.source):
			line = append(line, l.source[pos])
			pos++
		default:
			return l.firstLineOK(line)
		}
		if end := bytes.IndexByte(line, '\n'); end >= 0 {
			return l.firstLineOK(line[:end])
		}
	}
}

// firstLineOK reports whether an edited first line, read from the start of
// the source, opens no front matter and adds no byte order mark.
func (l *docLayout) firstLineOK(line []byte) bool {
	if l.base == 0 && bytes.HasPrefix(line, []byte(mdast.ByteOrderMark)) {
		return false
	}
	return !opensFrontMatter(line)
}

// region is a part of the document that a region check parses on its own:
// the groups first to last, or, when list is not negative, the items first
// to last of that top-level list, which are parsed with the item before and
// the item after them.
type region struct {
	list        int
	first, last int
}

func (reg region) isList() bool { return reg.list >= 0 }

// placedEdit is an edit in source offsets with the start offsets of the
// first and the last line it changes. An edit that changes the line ending
// of its last line also changes the next line, which it may join.
type placedEdit struct {
	edit                fix.TextEdit
	firstLine, lastLine int
}

// compareEdits orders edits by start, then by end, as fix.SortEdits does.
func compareEdits(a, b placedEdit) int {
	if c := cmp.Compare(a.edit.StartOffset, b.edit.StartOffset); c != 0 {
		return c
	}
	return cmp.Compare(a.edit.EndOffset, b.edit.EndOffset)
}

// placeEdits returns edits, prepared and in content offsets, in source
// offsets and placed on their lines. early reports whether an edit starts
// before frontLimit, so that firstLineKeeps must clear the edits; ok is
// false when an edit could reach the front matter or the byte order mark
// otherwise.
func (l *docLayout) placeEdits(edits []fix.TextEdit) ([]placedEdit, bool, bool) {
	placed := make([]placedEdit, 0, len(edits))
	early := false
	for _, edit := range edits {
		if edit.StartOffset < l.frontLimit {
			if !l.firstLineFree || edit.StartOffset < l.base {
				return nil, false, false
			}
			early = true
		}
		edit.StartOffset -= l.base
		edit.EndOffset -= l.base
		placed = append(placed, l.placeEdit(edit))
	}
	return placed, early, true
}

// startsEarly reports whether the first of placed edits, sorted, starts
// before frontLimit.
func (l *docLayout) startsEarly(placed []placedEdit) bool {
	return len(placed) > 0 && placed[0].edit.StartOffset < l.frontLimit-l.base
}

// placeEdit places an edit, in source offsets, on its lines.
func (l *docLayout) placeEdit(edit fix.TextEdit) placedEdit {
	last := edit.StartOffset
	if edit.EndOffset > edit.StartOffset {
		last = edit.EndOffset - 1
		if l.source[last] == '\n' {
			last = edit.EndOffset
		}
	}
	return placedEdit{
		edit:      edit,
		firstLine: lineStartIn(l.source, edit.StartOffset),
		lastLine:  lineStartIn(l.source, last),
	}
}

// groupOf returns the group that holds the line starting at offset.
func (l *docLayout) groupOf(offset int) int {
	return sort.SearchInts(l.bounds[1:len(l.bounds)-1], offset+1)
}

// itemOf returns the item of a list that holds the line starting at offset.
func (l *layoutList) itemOf(offset int) int {
	return sort.SearchInts(l.lines, offset+1) - 1
}

// listOf returns the list, among those that may have list regions, in group
// whose items hold the lines from first to last, with the items that hold
// them, or false. The lines from a list's first marker line to its last
// item's marker line are the list's; where the last item ends is not
// recorded.
func (l *docLayout) listOf(group, first, last int) (int, int, int, bool) {
	idx := sort.Search(len(l.searchable), func(i int) bool { return l.searchable[i].lines[0] > first }) - 1
	if idx < 0 {
		return 0, 0, 0, false
	}
	list := l.searchable[idx]
	if list.group != group || last >= list.lines[len(list.lines)-1] {
		return 0, 0, 0, false
	}
	return list.index, list.itemOf(first), list.itemOf(last), true
}

// regionFor returns the region that a placed edit needs at least.
func (l *docLayout) regionFor(placed placedEdit) region {
	first, last := l.groupOf(placed.firstLine), l.groupOf(placed.lastLine)
	if first == last {
		if list, a, b, ok := l.listOf(first, placed.firstLine, placed.lastLine); ok {
			return region{list: list, first: a, last: b}
		}
	}
	return region{list: -1, first: first, last: last}
}

// listRegionFits reports whether a list region has an item before it and an
// item after the item after it, so that it starts and ends at marker lines.
func (l *docLayout) listRegionFits(reg region) bool {
	list := l.lists[reg.list]
	return list.regions && reg.first >= 1 && reg.last+2 <= len(list.lines)-1
}

// span returns the source offsets that a region parses.
func (l *docLayout) span(reg region) (int, int) {
	if reg.isList() {
		list := l.lists[reg.list]
		return list.lines[reg.first-1], list.lines[reg.last+2]
	}
	return l.bounds[reg.first], l.bounds[reg.last+1]
}

// editSpan returns the source offsets between which the edits of a region
// start: in its groups, or in the items it edits. An edit at the end of the
// source belongs to the last group.
func (l *docLayout) editSpan(reg region) (int, int) {
	if reg.isList() {
		list := l.lists[reg.list]
		return list.lines[reg.first], list.lines[reg.last+1]
	}
	to := l.bounds[reg.last+1]
	if reg.last == len(l.bounds)-2 {
		to++
	}
	return l.bounds[reg.first], to
}

// regionEdits returns the placed edits, sorted by start, that start between
// from and to. Each edit starts in one region, which holds all the lines it
// changes.
func regionEdits(from, to int, placed []placedEdit) []placedEdit {
	first := sort.Search(len(placed), func(i int) bool { return placed[i].edit.StartOffset >= from })
	last := sort.Search(len(placed), func(i int) bool { return placed[i].edit.StartOffset >= to })
	return placed[first:last]
}

// meets reports whether two regions must be parsed as one: group regions
// that share a group, list regions of a list whose items with their
// neighbours overlap, beyond the item one shares with the other as its
// neighbour, and a group region and a list region in one of its groups.
// Regions that do not meet are parsed apart, and their spans do not
// overlap.
func (l *docLayout) meets(one, other region) bool {
	switch {
	case !one.isList() && !other.isList():
		return one.first <= other.last && other.first <= one.last
	case one.isList() && other.isList():
		return one.list == other.list && one.first <= other.last+2 && other.first <= one.last+2
	case one.isList():
		group := l.lists[one.list].group
		return other.first <= group && group <= other.last
	default:
		group := l.lists[other.list].group
		return one.first <= group && group <= one.last
	}
}

// mergeRegions returns regions merged so that no two meet, sorted by span.
// A list region without an item before it or two after it becomes the group
// region of its list, and a list region that a group region covers is
// dropped.
func (l *docLayout) mergeRegions(regions []region) []region {
	for {
		var groups, lists []region
		for _, reg := range regions {
			switch {
			case !reg.isList():
				groups = append(groups, reg)
			case l.listRegionFits(reg):
				lists = append(lists, reg)
			default:
				group := l.lists[reg.list].group
				groups = append(groups, region{list: -1, first: group, last: group})
			}
		}
		slices.SortFunc(groups, func(a, b region) int { return a.first - b.first })
		var merged []region
		for _, reg := range groups {
			if n := len(merged); n > 0 && reg.first <= merged[n-1].last {
				merged[n-1].last = max(merged[n-1].last, reg.last)
				continue
			}
			merged = append(merged, reg)
		}
		covered := func(group int) bool {
			idx := sort.Search(len(merged), func(i int) bool { return merged[i].last >= group })
			return idx < len(merged) && merged[idx].first <= group
		}
		slices.SortFunc(lists, func(a, b region) int {
			if a.list != b.list {
				return a.list - b.list
			}
			return a.first - b.first
		})
		changed := false
		var kept []region
		for _, reg := range lists {
			if covered(l.lists[reg.list].group) {
				continue
			}
			if n := len(kept); n > 0 && kept[n-1].list == reg.list && reg.first <= kept[n-1].last+2 {
				kept[n-1].last = max(kept[n-1].last, reg.last)
				changed = changed || !l.listRegionFits(kept[n-1])
				continue
			}
			kept = append(kept, reg)
		}
		regions = slices.Concat(merged, kept)
		if !changed {
			break
		}
	}
	slices.SortFunc(regions, func(a, b region) int {
		start, _ := l.span(a)
		other, _ := l.span(b)
		return start - other
	})
	return regions
}

// grow returns a region that holds reg and more of what follows it: twice as
// many groups or items, or the group region of a list region that would
// reach its list's last items. A region that ends the document stays.
func (l *docLayout) grow(reg region) region {
	size := reg.last - reg.first + 1
	if !reg.isList() {
		reg.last = min(reg.last+size, len(l.bounds)-2)
		return reg
	}
	reg.last += size
	if !l.listRegionFits(reg) {
		group := l.lists[reg.list].group
		return region{list: -1, first: group, last: group}
	}
	return reg
}

// regionParse is a region parsed with its edits.
type regionParse struct {
	// holds reports whether the region parsed as part of the document: a
	// group region left no block open, the items of a list region opened as
	// in the document. undecided reports that the region cannot be checked
	// on its own.
	holds, undecided bool
	// source is the text parsed, and blocks the region's top-level blocks,
	// or the items of a list region.
	source []byte
	blocks []gmast.Node
	// refsSame reports whether the region adds the link reference
	// definitions it added before, in the same order.
	refsSame bool
	// tight reports whether the list of a list region is tight, as
	// goldmark decides for its items but its first.
	tight bool
}

// parseRegion parses a region with its edits, which are in source offsets
// and sorted.
func (l *docLayout) parseRegion(reg region, edits []fix.TextEdit) regionParse {
	if reg.isList() {
		return l.parseListRegion(reg, edits)
	}
	return l.parseGroupRegion(reg, edits)
}

// parseGroupRegion parses a group region. Unless it ends the document, the
// sentinel line follows it, and the region holds when the parser starts a
// group at the sentinel.
func (l *docLayout) parseGroupRegion(reg region, edits []fix.TextEdit) regionParse {
	from, to := l.span(reg)
	text := applyWithin(l.source, from, to, edits)
	final := reg.last == len(l.bounds)-2
	length := len(text)
	if !final {
		if length == 0 || text[length-1] != '\n' {
			return regionParse{undecided: true}
		}
		text = append(text, regionSentinel...)
	}
	parsed := parseRecorded(l.markdown, text, l.refs)
	blocks := children(parsed.doc)
	if !final {
		groups := parsed.rec.groups
		if len(groups) == 0 || groups[len(groups)-1].offset != length {
			return regionParse{}
		}
		counts, ok := groupBlocks(blocks, groups[len(groups)-1:])
		if !ok {
			return regionParse{undecided: true}
		}
		blocks = blocks[:counts[0]]
	}
	return regionParse{holds: true, source: text, blocks: blocks, refsSame: l.sameRefs(from, to, parsed.rec.refs)}
}

// parseListRegion parses the items of a list region with the item before and
// the item after them. The region holds when the parse is one list whose
// first item opens on the first line and whose last item opens on the
// marker line of the item after the region.
func (l *docLayout) parseListRegion(reg region, edits []fix.TextEdit) regionParse {
	list := l.lists[reg.list]
	from, to := l.span(reg)
	text := applyWithin(l.source, from, to, edits)
	parsed := parseRecorded(l.markdown, text, l.refs)
	blocks := children(parsed.doc)
	if len(blocks) != 1 {
		return regionParse{}
	}
	parsedList, ok := blocks[0].(*gmast.List)
	if !ok || parsedList.FirstChild() == nil || parsedList.FirstChild() == parsedList.LastChild() {
		return regionParse{}
	}
	shift := 0
	for _, edit := range edits {
		shift += len(edit.NewText) - (edit.EndOffset - edit.StartOffset)
	}
	firstOpen, firstFound := parsed.rec.opens[parsedList.FirstChild()]
	lastOpen, lastFound := parsed.rec.opens[parsedList.LastChild()]
	if !firstFound || !lastFound || lineStartIn(text, firstOpen) != 0 ||
		lineStartIn(text, lastOpen) != list.lines[reg.last+1]-from+shift {
		return regionParse{}
	}
	return regionParse{
		holds:    true,
		source:   text,
		blocks:   children(parsedList),
		refsSame: l.sameRefs(from, to, parsed.rec.refs),
		tight:    parsedList.IsTight,
	}
}

// applyWithin returns source[from:to] with edits applied; the edits lie in
// that range and are sorted.
func applyWithin(source []byte, from, to int, edits []fix.TextEdit) []byte {
	shifted := make([]fix.TextEdit, len(edits))
	for idx, edit := range edits {
		edit.StartOffset -= from
		edit.EndOffset -= from
		shifted[idx] = edit
	}
	return bytes.Clone(fix.ApplyEdits(source[from:to], shifted))
}

// sameRefs reports whether refs are the link reference definitions of the
// document that come from paragraphs between from and to, in order.
func (l *docLayout) sameRefs(from, to int, refs []recordedRef) bool {
	first := sort.Search(len(l.refs), func(i int) bool { return l.refs[i].offset >= from })
	last := sort.Search(len(l.refs), func(i int) bool { return l.refs[i].offset >= to })
	return slices.EqualFunc(l.refs[first:last], refs, func(a, b recordedRef) bool { return sameRef(a.ref, b.ref) })
}

// looseIn returns the number of items of the original, among the items of a
// list region with their neighbours, that make the list loose.
func (l *layoutList) looseIn(reg region) int {
	return l.loose[reg.last+2] - l.loose[reg.first-1]
}

// loosens reports whether a list region whose list parsed as tight or not
// makes its list loose: its list is loose, or its first item, which is not
// the list's first, follows a blank line.
func (l *layoutList) loosens(reg region, tight bool) bool {
	return !tight || (reg.first-1 >= 1 && l.blank[reg.first-1])
}
