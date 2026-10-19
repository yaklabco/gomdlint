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
	gmextast "github.com/yuin/goldmark/extension/ast"
	goldparser "github.com/yuin/goldmark/parser"
	gmtext "github.com/yuin/goldmark/text"
	gmutil "github.com/yuin/goldmark/util"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// Region checks.
//
// The fix check compares the render signature of the whole document before
// and after a fix (see fixCheck). Parsing the whole document for every fix
// made a long document with many findings take time that grows with the
// square of its length. A region check decides as the whole-document check
// does, while it parses only the part of the document that a fix can
// change. The argument rests on how goldmark 1.8 parses.
//
// Groups. goldmark reads blocks line by line. When no block is open, it skips
// blank lines and opens the next block (the outer loop of
// parser.parseBlocks, the only caller of the reader's SkipBlankLines). From
// there on, the parse is a function of the rest of the source: no open
// block, container offset or blank-line statistic carries over. The context
// flags of the list parsers live within a line or are reset when a list or
// item opens, before they are read; the setext and fence flags live only
// while their block is open; the blocks opened there are marked as following
// a blank line whatever came before (the outer loop passes true to
// openBlocks); and link reference definitions become nodes, made from the
// lines of their paragraph when it closes, and otherwise only matter to
// inline parsing, which starts once all blocks are closed.
//
// The parse starts afresh in one more place. A thematic break, an ATX heading
// and an HTML block that goldmark closes without taking the next line (one
// whose closing condition holds on its first line) close on the next line
// whatever it holds: their Continue returns Close without reading it. When
// such a block is the only open block and the next line opens a top-level
// block, goldmark opens it from its inner loop, with that block still on the
// stack, and closes the old block after. That block counts as no block for
// every decision openBlocks and the block parsers make on the line: it is
// not a paragraph (no interruption rules, no setext heading) and not a list.
// No context flag is read: fence data lives only while a fence is open, the
// setext paragraph is set by a setext heading's Open and read only by its
// Close, and the list item flag is set and read on the line an item ends.
// And its Close changes nothing in the tree. Two things differ from a fresh
// start. The blocks opened on the line are marked as not following a blank
// line, since the line before is not blank, where a fresh start marks them
// as following one. Nothing a check compares reads that flag: list tightness
// comes from gmparser.ListTight, which never reads the flags of a list's
// first item or of an item's first block, and an item's first block takes in
// every node made from the paragraph the item starts with that keeps the
// paragraph's flag (its first definition, its rest, a setext heading made
// from it) and a table between them (goldmark's own IsTight can read them,
// but it only decides whether a list's paragraphs are text blocks, which
// signatures and outlines write alike).
// And the blank-line statistics keep the old block's line, but the next
// lookup is of this line, which is not blank either way. A setext heading
// does not start a group so: its Close can add its underline to the
// paragraph opened after it. Nor does an indented code block: whether it
// closes depends on the indentation of the next line, which the sentinel
// line of a group region does not stand for.
//
// So the document falls into groups, each starting at a line where the
// parser opened a block with no block open, or a top-level block with only
// such a closed block open, and the blocks of a group parse the same on
// their own as in the document, given the document's definitions. A group
// runs from its first line to the first line of the next, blank lines
// included. docLayout records the groups of the original by wrapping the
// reader and the block parsers.
//
// Group regions. A group region is a run of whole groups that holds every
// line an edit changes; an edit that changes the line ending of a line
// changes the next line too. The lines before the region are the same after
// the edits, so the parser reaches the region as it did: with no block open,
// or with only a closed block open that closes on the region's first line
// whatever it holds. Either way the region parses as it does on its own, but
// for the blank-line flags of the blocks opened on its first line (see
// Groups). The
// region, edited, is parsed on its own, followed by a sentinel line. When the
// parser starts a group at the sentinel, the region left no block open, or
// only a closed block that closes on the next line whatever it holds, so
// the lines after it, unchanged, parse as before; the group must start as
// the original's next group did, after a closed block or not, so that the
// blank-line flags of its first blocks are as before too. When it does not
// start so, the region grows into the next groups, up to the end of the
// document. Regions that hold, taken in document order, make up the edited
// document: the original's blocks outside them and each region's blocks.
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
// document. A list is tight, as CommonMark reads it (gmparser.ListTight),
// when no item makes it loose by following a blank line, unless it is the
// first item, or by having a block after its first block that follows one
// (gmparser.ListItemLoose). The only blank-line flags that depend on the
// line before the region are those of the blocks opened on the marker line
// of the item before (the item, its first block, and containers opened on
// that line with their first items and blocks) and of the nodes goldmark
// makes from a paragraph opened there (its definitions, the rest of it, or
// a setext heading made from it). ListItemLoose reads none of them: an
// item's own flag only when it is not its list's first item, and blocks
// only after an item's first block, which takes in what goldmark makes from
// a paragraph opened there (see gmparser.ListItemLoose). So the region's
// list decides for its items as
// the document does, but for whether the item before follows a blank line,
// which comes from the original, as do the items outside the regions, and
// the tightness of the edited list follows. goldmark's own IsTight is not
// used: it reads the rest of a paragraph that starts with link reference
// definitions as a block of its own, with the flag of the item's first line.
//
// Quote regions. A long blockquote at the top level is one group too.
// goldmark reads each line of a blockquote by its marker first: the
// blockquote's Continue takes up to three spaces, the marker and one space
// or tab off the line, or closes the blockquote, as a function of the line
// alone, and the blockquote holds no other state. So the blocks of a
// top-level blockquote fall into groups of their own, as the document's
// blocks do: each starts at a line where the parser opened a block in the
// blockquote while the blockquote was the last open block, or while its
// only open block was a closed block (see Groups), and from there on the
// parse of the blockquote is a function of the rest of the source.
// docLayout records these groups, whether each starts after a closed
// block, and the blank-line flag goldmark gave the first block opened at
// each. A quote region is a run of the groups of a top-level blockquote but
// its last. It starts where the document continues the blockquote, or opens
// it at the start of a group of the document: where the blockquote opens
// while other blocks are open, such as a list, an edit of its first line
// can make the line one of theirs. The region, edited, is parsed on its
// own, followed by a sentinel line that holds the blockquote marker. It
// holds when the parse is one blockquote that opens a block on the region's
// first line and starts a group at the sentinel as the original's next
// group starts: after a closed block or not, with the blank-line flag of
// that group's first block in the original. The parse opens the blockquote
// on the first line, where the document continues it or opens it with no
// other block open, and then reads every line of the region as the edited
// document does, but for two things. The blocks opened on the first line
// get other blank-line flags, which nothing a check compares reads: each is
// the first block of the block it opens in, so a block of a blockquote, a
// list, the first item of a list or the first block of an item, and
// ListTight reads none of these (see Groups). And the blank-line statistics
// of the first line differ: the document records the line as not blank at
// each level, since it holds the marker and a block opens on it, and a
// parse that starts on it records nothing, which the next line reads as not
// blank too. At the sentinel, the blockquote is open with no block open in
// it, or with only a closed block, as in the original, and the blocks
// opened on the next line get the original's flags, so the lines after the
// region parse as in the original. A region that ends the blockquote cannot
// show where the blockquote ends, so the group region of the blockquote's
// group stands for it.
//
// Lists in quotes. A list that is a block of a top-level blockquote has
// list regions too. Each line of the region is read by the blockquote's
// marker first, as in the document, and the item before the edited ones,
// whose lines are unchanged, opens in a blockquote that the region parse
// opens on that item's marker line, so the argument of list regions holds
// for the list's items. The region holds when the parse is one blockquote
// whose one block is a list that holds as a list region does. A list region
// that does not fit becomes the quote region of the list's group in the
// blockquote, or, when that does not fit either, the group region of the
// blockquote.
//
// Table regions. GFM makes a table of the lines of a paragraph from a
// header row and a delimiter row on, when the paragraph closes, and reads
// each body row from its line alone, given the delimiter row's alignments.
// While the paragraph is open, a line continues it, or interrupts it with
// another block, as a function of the line and of the open paragraph and
// its parent: no other state is read. So a run of the body rows of a table
// that is a top-level block, or a block of a top-level blockquote, is
// parsed after the table's header and delimiter rows, whose lines are
// unchanged, and followed by the sentinel line, or for a table in a
// blockquote by the quote sentinel, which continues the paragraph exactly
// when the paragraph is still open. The region holds when the parse is one
// table, in one blockquote for a table in a blockquote, whose last row is
// the sentinel's: the paragraph is open before the region's first row, as
// in the original, and after its last, so the next line continues or
// interrupts it as in the original, and the table keeps the original's
// other rows. A table region needs no rows around it, and can hold its
// table's first and last rows. One that does not hold becomes the region of
// the table's group, in its blockquote or in the document. CommonMark has
// no tables: it reads the rows as the lines of one paragraph.
//
// Signatures. The signature of the document is its front matter and the
// signatures of its top-level blocks in order; that of a list is a token
// with its tightness and the signatures of its items, and that of a
// blockquote or a table a token and the signatures of its blocks or rows.
// So the signature of the edited document is the original's with each
// region's part replaced by the signature of the region's blocks, items or
// rows, and the token of a list that changes tightness replaced, provided
// that: the edits leave the front matter alone (they start after it, see
// frontMatterLimit); every region adds the same link reference definitions
// in the same order, so that every link, before or after it, resolves as
// before (regions are parsed with the original's definitions); and the
// anchor counts after each region are as before, so that later headings
// keep their anchors (a region's headings get their anchors from the
// counts before it, see anchorHistory). Where definitions change, region
// checks give up and the whole document is parsed, as before. Where anchor
// counts change, the render signature's checks give up too, unless the
// signature up to that region already differs; the outline, whose anchors
// stand together in one item, has the anchors that change worked out
// instead (see anchorHistory.keeps). sameAfterReplacements compares the
// replaced signature with the original exactly, without writing it out.
//
// The argument depends on goldmark's parser, not on what the signature
// compares, so docLayout serves both fix checks that use it: regionModel
// adds the render signature that fixCheck compares, and outlineModel (see
// fix_safety_regions.go) the outline that fixVerifier compares.
// TestRegionChecksDecideAsWholeDocument and
// TestOutlineRegionsDecideAsWholeDocument check the region decisions against
// the whole-document checks, and must pass again after a goldmark upgrade.
//
// Cost. A region is at least a group of the document or of a top-level
// blockquote, the items of a list around the edited ones, or the rows of a
// table with its header, so a check costs a parse of the blocks around its
// edits. Groups still run long where blocks follow one another with no
// blank line and none of the closed blocks above between them: paragraphs
// that a list or an HTML block interrupts, setext headings or indented code
// blocks followed directly by text, and everything inside one container
// other than a top-level blockquote. So a document whose fixes sit in one
// big block, other than a top-level blockquote or a list or table at the
// top level or in a top-level blockquote, still costs a parse of that block
// per check: a long paragraph, a table under CommonMark, or a list, table or
// blockquote in a list item or in another blockquote. Beside its parses, a
// check's work is linear in its regions and edits and in the headings whose
// anchors change: edits are placed each from the one before it, a region
// that grows takes in only the regions it meets, one pass gives the
// tightness of every edited list, a slug's number is found from an index of
// the numbered keys, and the outline's anchors are taken only where counts
// differ (see fix_anchors.go).

// regionSentinel is the line parsed after a group region to find out whether
// the region leaves a block open, and after a table region to find out
// whether the table's paragraph is still open; quoteSentinel is the line
// parsed after a quote region, or a table region in a blockquote, to the
// same ends in the blockquote.
const (
	regionSentinel = "x"
	quoteSentinel  = ">x"
)

// regionMarkdowns returns the recording goldmark instances for the parsers
// of a fixCheck, in the same order: GFM, and CommonMark unless the flavor is
// GFM. They are shared: a parse keeps all its state in its context.
func regionMarkdowns(flavor config.Flavor) []goldmark.Markdown {
	gfm, commonMark := recordingInstances()
	markdowns := []goldmark.Markdown{gfm}
	if flavor != config.FlavorGFM {
		markdowns = append(markdowns, commonMark)
	}
	return markdowns
}

// recordingInstances returns the recording goldmark instances for GFM and
// CommonMark, built on first use.
func recordingInstances() (goldmark.Markdown, goldmark.Markdown) {
	recordingOnce.Do(func() {
		recordingGFM = goldmark.New(goldmark.WithExtensions(gmext.GFM))
		recordingGFM.Parser().AddOptions(recordingOption{})
		recordingCommonMark = goldmark.New()
		recordingCommonMark.Parser().AddOptions(recordingOption{})
	})
	return recordingGFM, recordingCommonMark
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
	// setext holds the headings that a setext heading parser opened.
	setext map[gmast.Node]bool
	// quotes holds where the parser started each group of each top-level
	// blockquote (see Quote regions).
	quotes map[gmast.Node][]groupStart
	// tables holds, for each table, the source offsets of the lines of its
	// header row, its delimiter row and its body rows; it is nil when the
	// parse does not record tables (see parseRecorded).
	tables map[gmast.Node][]int
}

// startsQuoteGroup reports whether a block that opens in parent while blocks
// are open starts a group of a top-level blockquote (see Quote regions):
// parent is such a blockquote, and it is the last open block, or its only
// open block is one that closes alone, in which case afterClosed is true.
func (r *parseRecord) startsQuoteGroup(parent gmast.Node, blocks []goldparser.Block) (bool, bool) {
	if parent.Kind() != gmast.KindBlockquote || parent.Parent() == nil ||
		parent.Parent().Kind() != gmast.KindDocument {
		return false, false
	}
	count := len(blocks)
	switch {
	case count >= 1 && blocks[count-1].Node == parent:
		return true, false
	case count >= 2 && blocks[count-2].Node == parent && r.closesAlone(blocks[count-1:]):
		return true, true
	}
	return false, false
}

// closesAlone reports whether blocks, the blocks open when a top-level block
// opens, are one thematic break, ATX heading or HTML block, which closes on
// the line that block opens on whatever the line holds (see Groups). An HTML
// block that is still open then is one goldmark closes without taking the
// line, or no block would open on it.
func (r *parseRecord) closesAlone(blocks []goldparser.Block) bool {
	if len(blocks) != 1 {
		return false
	}
	switch node := blocks[0].Node.(type) {
	case *gmast.ThematicBreak, *gmast.HTMLBlock:
		return true
	case *gmast.Heading:
		return !r.setext[node]
	}
	return false
}

// groupStart is the first line of a group and the last top-level block of
// the document when the group started, or nil. The block is recorded rather
// than counted, because goldmark miscounts children: inserting a node after
// the last child, as a table built from the last paragraph is, counts it
// twice.
type groupStart struct {
	offset int
	last   gmast.Node
	// afterClosed reports a start after a closed block (see Groups), where
	// the blocks opened follow no blank line, rather than one with no block
	// open.
	afterClosed bool
	// opened is the block opened at a start of a group of a blockquote,
	// whose blank-line flag goldmark sets once the block has opened.
	opened gmast.Node
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

// recordingContext records the link reference definitions a parse adds. Links
// resolve through preset first, the definitions of the document a region
// comes from, by normalized label, and then through those the parse adds.
type recordingContext struct {
	goldparser.Context
	rec    *parseRecord
	preset map[string]goldparser.Reference
}

func (c *recordingContext) AddReference(ref goldparser.Reference) {
	c.rec.refs = append(c.rec.refs, recordedRef{offset: c.rec.paragraph, ref: ref})
	c.Context.AddReference(ref)
}

// Reference returns the definition a normalized label resolves to. goldmark
// resolves links only through it.
//
//nolint:ireturn // goldmark's parser.Context interface fixes this signature.
func (c *recordingContext) Reference(label string) (goldparser.Reference, bool) {
	if ref, found := c.preset[label]; found {
		return ref, true
	}
	return c.Context.Reference(label)
}

// referenceIndex returns definitions by normalized label, the first of each
// label winning, as goldmark's parse context keeps them.
func referenceIndex(refs []recordedRef) map[string]goldparser.Reference {
	index := make(map[string]goldparser.Reference, len(refs))
	for _, ref := range refs {
		key := gmutil.ToLinkReference(ref.ref.Label())
		if _, taken := index[key]; !taken {
			index[key] = ref.ref
		}
	}
	return index
}

// groupReader records where the parser starts each group with no block open:
// the parser skips blank lines only when no block is open, and then opens a
// block at the line it stops at.
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

// recordingBlockParser records the line where each block opens, where a
// group starts with a top-level block that opens while only a closed block
// is open (see Groups), and where the groups of a top-level blockquote start
// (see Quote regions).
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
	rc, recording := pc.(*recordingContext)
	// The open blocks are read before the block opens, as openBlocks has
	// them when it decides; the old block closes after.
	var startsGroup, startsQuote, quoteAfterClosed bool
	if recording {
		startsGroup = parent.Kind() == gmast.KindDocument && rc.rec.closesAlone(pc.OpenedBlocks())
		startsQuote, quoteAfterClosed = rc.rec.startsQuoteGroup(parent, pc.OpenedBlocks())
	}
	node, state := p.BlockParser.Open(parent, reader, pc)
	if !recording || node == nil {
		return node, state
	}
	rc.rec.opens[node] = segment.Start
	if state&goldparser.RequireParagraph != 0 {
		rc.rec.setext[node] = true
	}
	if parent.Kind() == gmast.KindDocument {
		rc.rec.doc = parent
		if startsGroup {
			rc.rec.groups = append(rc.rec.groups, groupStart{
				offset:      lineStartIn(reader.Source(), segment.Start),
				last:        parent.LastChild(),
				afterClosed: true,
			})
		}
	}
	if startsQuote {
		rc.rec.quotes[parent] = append(rc.rec.quotes[parent], groupStart{
			offset:      lineStartIn(reader.Source(), segment.Start),
			last:        parent.LastChild(),
			afterClosed: quoteAfterClosed,
			opened:      node,
		})
	}
	return node, state
}

// recordingParagraphTransformer records the paragraph that link reference
// definitions come from, and the lines of the paragraph a table is made of.
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
	if rc.rec.tables == nil {
		t.ParagraphTransformer.Transform(node, reader, pc)
		rc.rec.paragraph = outer
		return
	}
	lines := make([]int, node.Lines().Len())
	for idx := range lines {
		lines[idx] = lineStartIn(reader.Source(), node.Lines().At(idx).Start)
	}
	parent, prev := node.Parent(), node.PreviousSibling()
	t.ParagraphTransformer.Transform(node, reader, pc)
	rc.rec.paragraph = outer
	// GFM places a table after the paragraph it keeps the lines before the
	// header row of, or in its place.
	var next gmast.Node
	remaining := 0
	switch {
	case node.Parent() != nil:
		next = node.NextSibling()
		remaining = node.Lines().Len()
	case prev != nil:
		next = prev.NextSibling()
	case parent != nil:
		next = parent.FirstChild()
	}
	if table, isTable := next.(*gmextast.Table); isTable && remaining < len(lines) {
		if _, seen := rc.rec.tables[table]; !seen {
			rc.rec.tables[table] = lines[remaining:]
		}
	}
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

// parseRecorded parses source with a recording goldmark instance. Links
// resolve through preset first, the definitions of the document a region
// comes from (see referenceIndex), so that they resolve as in that document;
// the record holds only the definitions that source adds. The index is
// shared, not copied, so a parse costs nothing per definition. With tables,
// the record holds the lines of the paragraphs that tables are made of,
// which only the layout reads.
func parseRecorded(
	markdown goldmark.Markdown, source []byte, preset map[string]goldparser.Reference, tables bool,
) *recordedParse {
	rec := &parseRecord{
		opens:     make(map[gmast.Node]int),
		paragraph: -1,
		setext:    make(map[gmast.Node]bool),
		quotes:    make(map[gmast.Node][]groupStart),
	}
	if tables {
		rec.tables = make(map[gmast.Node][]int)
	}
	reader := &groupReader{Reader: gmtext.NewReader(source), rec: rec}
	pc := &recordingContext{Context: goldparser.NewContext(), rec: rec, preset: preset}
	doc := markdown.Parser().Parse(reader, goldparser.WithContext(pc))
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
// parser reads it, whatever a check compares: where its groups, the groups
// of its top-level blockquotes, the items of its lists and the rows of its
// tables start, its link reference definitions, and where edits may start
// without reaching the front matter. It selects, merges, grows and parses
// the regions that edits need.
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
	// first, and the length of the source; afterClosed reports for each
	// group whether it starts after a closed block; groupNodes holds the
	// index in nodes of the first block of each group, and the number of
	// nodes.
	bounds      []int
	afterClosed []bool
	groupNodes  []int
	// lists holds the lists with items that are top-level blocks or blocks
	// of a top-level blockquote, in document order, and listIndex the index
	// in lists of each list node. searchable holds the lists that may have
	// list regions.
	lists      []*layoutList
	listIndex  map[gmast.Node]int
	searchable []*layoutList
	// quotes holds the top-level blockquotes, in document order, and
	// quoteIndex the index in quotes of each blockquote node.
	// searchableQuotes holds those that may have quote regions.
	quotes           []*layoutQuote
	quoteIndex       map[gmast.Node]int
	searchableQuotes []*layoutQuote
	// tables holds the tables with body rows that are top-level blocks or
	// blocks of a top-level blockquote, in document order, and tableIndex
	// the index in tables of each table node.
	tables     []*layoutTable
	tableIndex map[gmast.Node]int
	// refs holds the link reference definitions in the order the parser
	// added them, and refIndex the definition each label resolves to.
	refs     []recordedRef
	refIndex map[string]goldparser.Reference
	// work counts what region checks have done, which tests bound.
	work regionWork
}

// regionWork counts the work of region checks: the bytes their parses read,
// the regions their merges took, the regions their list tightness checks
// visited, the bytes of edited first lines they read, the bytes they read to
// place edits on their lines, and the regions of each kind they parsed.
type regionWork struct {
	parsed, merged, tightVisits, firstLine, placing int
	kinds                                           [tableRegion + 1]int
}

// layoutList is a list of the original document that is a top-level block,
// or a block of a top-level blockquote.
type layoutList struct {
	// group is the document group that holds the list; quote is the index
	// in quotes of the blockquote the list is a block of, or -1, and inner
	// the group of that blockquote that holds the list.
	index, group int
	quote, inner int
	// regions reports whether list regions may be used for this list.
	regions bool
	// lines holds the source offset of the marker line of each item.
	lines []int
	// blank reports whether each item follows a blank line, and loose
	// counts the items before each that make the list loose
	// (gmparser.ListItemLoose).
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
	parsed := parseRecorded(markdown, source, nil, true)
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
		quoteIndex:    make(map[gmast.Node]int),
		tableIndex:    make(map[gmast.Node]int),
		refs:          parsed.rec.refs,
		refIndex:      referenceIndex(parsed.rec.refs),
	}
	layout.bounds = append(layout.bounds, 0)
	layout.afterClosed = append(layout.afterClosed, false)
	for _, start := range starts[1:] {
		if start.offset <= layout.bounds[len(layout.bounds)-1] {
			return nil
		}
		layout.bounds = append(layout.bounds, start.offset)
		layout.afterClosed = append(layout.afterClosed, start.afterClosed)
	}
	layout.bounds = append(layout.bounds, len(source))
	group := 0
	for idx, node := range nodes {
		for group+1 < len(starts) && counts[group+1] <= idx {
			group++
		}
		layout.addBlock(node, parsed.rec, group, -1, 0)
		// A block that opens on the line its group starts on opens with no
		// block open, or only a closed block (see Groups).
		if quote, isQuote := layout.quoteIndex[node]; isQuote {
			layout.quotes[quote].opensGroup = layout.quotes[quote].line == starts[group].offset
		}
	}
	return layout
}

// addBlock records a block of the original that region checks may enter: a
// list with items or a table with body rows, and, at the top level, a
// blockquote with its blocks. The block lies in the document's group at
// index group; it is a top-level block when quote is -1, and otherwise a
// block of the top-level blockquote at index quote, in its group inner.
func (l *docLayout) addBlock(node gmast.Node, rec *parseRecord, group, quote, inner int) {
	switch block := node.(type) {
	case *gmast.List:
		if block.FirstChild() == nil {
			return
		}
		model := newLayoutList(block, rec, l.source, len(l.lists), group)
		model.quote, model.inner = quote, inner
		l.listIndex[node] = model.index
		l.lists = append(l.lists, model)
		if model.regions {
			l.searchable = append(l.searchable, model)
		}
	case *gmextast.Table:
		model := newLayoutTable(block, rec, l.source, len(l.tables), group)
		if model == nil {
			return
		}
		model.quote, model.inner = quote, inner
		l.tableIndex[node] = model.index
		l.tables = append(l.tables, model)
	case *gmast.Blockquote:
		if quote >= 0 {
			return
		}
		model := newLayoutQuote(block, rec, l.source, len(l.quotes), group)
		l.quoteIndex[node] = model.index
		l.quotes = append(l.quotes, model)
		if model.regions {
			l.searchableQuotes = append(l.searchableQuotes, model)
		}
		childGroup := 0
		for idx, child := range model.nodes {
			for childGroup+1 < len(model.bounds) && model.groupNodes[childGroup+1] <= idx {
				childGroup++
			}
			l.addBlock(child, rec, group, model.index, childGroup)
		}
	}
}

// layoutQuote is a top-level blockquote of the original document.
type layoutQuote struct {
	// line is the source offset of the blockquote's first line.
	index, group, line int
	// regions reports whether quote regions may be used for this
	// blockquote: the parse placed two groups or more in it, in order.
	// opensGroup reports whether the blockquote's first line starts its
	// document group, so that it opens with no other block open, or only a
	// closed block.
	regions, opensGroup bool
	// bounds holds the source offset where each group of the blockquote
	// starts (see Quote regions), the lines up to the last holding all
	// groups but the last; without regions, it holds the blockquote's first
	// line alone, as one group. afterClosed reports for each group whether
	// it starts after a closed block, and blank whether goldmark marked the
	// first block opened at its start as following a blank line.
	bounds      []int
	afterClosed []bool
	blank       []bool
	// nodes holds the blocks of the blockquote, and groupNodes the index in
	// nodes of the first block of each group, and the number of nodes.
	nodes      []gmast.Node
	groupNodes []int
}

// newLayoutQuote records the groups of a top-level blockquote.
func newLayoutQuote(quote *gmast.Blockquote, rec *parseRecord, source []byte, index, group int) *layoutQuote {
	model := &layoutQuote{index: index, group: group, line: lineStartIn(source, rec.opens[quote]), nodes: children(quote)}
	starts := rec.quotes[quote]
	counts, ok := groupBlocks(model.nodes, starts)
	model.regions = ok && len(starts) >= 2 && counts[0] == 0 && slices.IsSorted(counts)
	for idx := 1; model.regions && idx < len(starts); idx++ {
		model.regions = starts[idx].offset > starts[idx-1].offset
	}
	if !model.regions {
		model.bounds = []int{model.line}
		model.afterClosed, model.blank = []bool{false}, []bool{false}
		model.groupNodes = []int{0, len(model.nodes)}
		return model
	}
	for _, start := range starts {
		model.bounds = append(model.bounds, start.offset)
		model.afterClosed = append(model.afterClosed, start.afterClosed)
		model.blank = append(model.blank, start.opened.HasBlankPreviousLines())
	}
	counts = append(counts, len(model.nodes))
	model.groupNodes = counts
	return model
}

// groupOf returns the group of the blockquote that holds the line starting
// at offset; a line before the first group's is counted in it.
func (q *layoutQuote) groupOf(offset int) int {
	return sort.SearchInts(q.bounds[1:], offset+1)
}

// layoutTable is a table with body rows of the original document that is a
// top-level block, or a block of a top-level blockquote.
type layoutTable struct {
	// group, quote and inner place the table as they place a layoutList.
	index, group int
	quote, inner int
	// regions reports whether table regions may be used for this table.
	regions bool
	// head is the source offset of the line of the header row, which the
	// line of the delimiter row follows. rows holds the source offset of the
	// line of each body row, the first following the delimiter row's, and
	// where the last row's line ends.
	head int
	rows []int
}

// newLayoutTable records the rows of a table. It returns nil when table
// regions cannot be used for it: when it has no body rows, or the lines of
// the paragraph it was made of do not follow one another, one for each row
// after the header and the delimiter.
func newLayoutTable(table *gmextast.Table, rec *parseRecord, source []byte, index, group int) *layoutTable {
	lines, found := rec.tables[table]
	if !found || len(lines) < 3 {
		return nil
	}
	var body []gmast.Node
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		if _, isRow := row.(*gmextast.TableRow); isRow {
			body = append(body, row)
		}
	}
	if len(body) != len(lines)-2 {
		return nil
	}
	for idx := 1; idx < len(lines); idx++ {
		if lines[idx] != nextLineStart(source, lines[idx-1]) {
			return nil
		}
	}
	for idx, row := range body {
		if lineStartIn(source, row.Pos()) != lines[idx+2] {
			return nil
		}
	}
	rows := append(slices.Clone(lines[2:]), nextLineStart(source, lines[len(lines)-1]))
	return &layoutTable{index: index, group: group, regions: true, head: lines[0], rows: rows}
}

// rowOf returns the body row of the table whose line starts at offset.
func (t *layoutTable) rowOf(offset int) int {
	return sort.SearchInts(t.rows, offset+1) - 1
}

// nextLineStart returns the offset of the start of the line after the one
// that holds offset, or the length of the source.
func nextLineStart(source []byte, offset int) int {
	if end := bytes.IndexByte(source[offset:], '\n'); end >= 0 {
		return offset + end + 1
	}
	return len(source)
}

// newLayoutList records the items of a list.
func newLayoutList(list *gmast.List, rec *parseRecord, source []byte, index, group int) *layoutList {
	model := &layoutList{index: index, group: group, quote: -1, regions: true, loose: []int{0}}
	loose := 0
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		offset, found := rec.opens[item]
		line := lineStartIn(source, offset)
		if !found || (len(model.lines) > 0 && line <= model.lines[len(model.lines)-1]) {
			model.regions = false
		}
		model.lines = append(model.lines, line)
		model.blank = append(model.blank, item.HasBlankPreviousLines())
		if gmparser.ListItemLoose(item, source) {
			loose++
		}
		model.loose = append(model.loose, loose)
	}
	return model
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
		// Each piece is the text of an edit or the source up to the next
		// edit, so each byte is looked at once.
		var piece []byte
		switch {
		case next != nil && next.edit.StartOffset == pos:
			piece = []byte(next.edit.NewText)
			pos = next.edit.EndOffset
			if fromEdits {
				edits = edits[1:]
			} else {
				more = more[1:]
			}
		case pos < len(l.source):
			end := len(l.source)
			if next != nil {
				end = min(end, next.edit.StartOffset)
			}
			piece = l.source[pos:end]
			pos = end
		default:
			return l.firstLineOK(line)
		}
		if end := bytes.IndexByte(piece, '\n'); end >= 0 {
			l.work.firstLine += end + 1
			return l.firstLineOK(append(line, piece[:end]...))
		}
		l.work.firstLine += len(piece)
		line = append(line, piece...)
	}
}

// firstLineEnd returns where the first line of the source with edits, in
// source offsets and sorted, ends, as firstLineKeeps reads it: the offset of
// its line ending, or of the edit whose text holds it, or the length of the
// source. firstLineKeeps reads no edit that starts after it, so with edits
// after it as well it reads the line it reads with these alone.
func (l *docLayout) firstLineEnd(edits []placedEdit) int {
	pos := 0
	for {
		switch {
		case len(edits) > 0 && edits[0].edit.StartOffset == pos:
			l.work.firstLine += len(edits[0].edit.NewText)
			if strings.Contains(edits[0].edit.NewText, "\n") {
				return pos
			}
			pos = edits[0].edit.EndOffset
			edits = edits[1:]
		case pos < len(l.source):
			end := len(l.source)
			if len(edits) > 0 {
				end = min(end, edits[0].edit.StartOffset)
			}
			if newline := bytes.IndexByte(l.source[pos:end], '\n'); newline >= 0 {
				l.work.firstLine += newline + 1
				return pos + newline
			}
			l.work.firstLine += end - pos
			pos = end
		default:
			return len(l.source)
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

// regionParts is where the parts of a signature or an outline that regions
// replace start: for each group of the document or of a blockquote, each
// item of a list or each body row of a table, and for the end, the offset
// where its part starts and the index of its first heading.
type regionParts struct {
	starts, headings []int
}

// add records a part that starts at offset, with heading as its first
// heading.
func (p *regionParts) add(offset, heading int) {
	p.starts = append(p.starts, offset)
	p.headings = append(p.headings, heading)
}

// partOf returns where the part of a region starts and ends among parts, and
// the indexes of its first heading and of the first heading after it. The
// part of a list region runs from the item before its items to the item
// after them.
func (p regionParts) partOf(reg region) (int, int, int, int) {
	first, end := reg.first, reg.last+1
	if reg.isList() {
		first, end = reg.first-1, reg.last+2
	}
	return p.starts[first], p.starts[end], p.headings[first], p.headings[end]
}

// regionKind tells what part of the document a region parses.
type regionKind uint8

const (
	// groupRegion is a run of the document's groups.
	groupRegion regionKind = iota
	// quoteRegion is a run of the groups of a top-level blockquote.
	quoteRegion
	// listRegion is a run of the items of a list, parsed with the item
	// before and the item after them.
	listRegion
	// tableRegion is a run of the body rows of a table, parsed after the
	// table's header and delimiter rows.
	tableRegion
)

// region is a part of the document that a region check parses on its own:
// the groups first to last of the document, or of the blockquote at index
// at of quotes, the items first to last of the list at index at of lists,
// or the body rows first to last of the table at index at of tables. The
// zero region is the document's first group.
type region struct {
	kind        regionKind
	at          int
	first, last int
}

func (reg region) isList() bool { return reg.kind == listRegion }

// groupRegionOf returns the region of the document's group at index group.
func groupRegionOf(group int) region {
	return region{first: group, last: group}
}

// docGroup returns the document group that holds a region, the first one
// for a group region.
func (l *docLayout) docGroup(reg region) int {
	switch reg.kind {
	case quoteRegion:
		return l.quotes[reg.at].group
	case listRegion:
		return l.lists[reg.at].group
	case tableRegion:
		return l.tables[reg.at].group
	default:
		return reg.first
	}
}

// quoteGroup returns the blockquote and its group that hold a list or table
// region, or -1 when the list or table is a top-level block, as for a group
// or quote region, which no blockquote holds.
func (l *docLayout) quoteGroup(reg region) (int, int) {
	switch reg.kind {
	case listRegion:
		list := l.lists[reg.at]
		return list.quote, list.inner
	case tableRegion:
		table := l.tables[reg.at]
		return table.quote, table.inner
	default:
		return -1, 0
	}
}

// containerRegion returns the region that stands for a list or table region
// that does not fit or hold: the quote region of the group of the
// blockquote that holds the list or table, when that fits, and otherwise
// the region of its document group.
func (l *docLayout) containerRegion(reg region) region {
	if quote, inner := l.quoteGroup(reg); quote >= 0 {
		if inQuote := (region{kind: quoteRegion, at: quote, first: inner, last: inner}); l.quoteRegionFits(inQuote) {
			return inQuote
		}
	}
	return groupRegionOf(l.docGroup(reg))
}

// fitted returns reg when it fits, and otherwise the region that stands for
// it: a quote region that ends its blockquote becomes the region of the
// blockquote's document group, and a list region without an item before it
// or two after it, or a region of a table that takes none, the region of
// its container (see containerRegion).
func (l *docLayout) fitted(reg region) region {
	switch {
	case reg.kind == quoteRegion && !l.quoteRegionFits(reg):
		return groupRegionOf(l.quotes[reg.at].group)
	case reg.kind == listRegion && !l.listRegionFits(reg),
		reg.kind == tableRegion && !l.tables[reg.at].regions:
		return l.containerRegion(reg)
	}
	return reg
}

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

// placeEdits returns edits, prepared, so sorted, and in content offsets, in
// source offsets and placed on their lines. early reports whether an edit
// starts before frontLimit, so that firstLineKeeps must clear the edits; ok
// is false when an edit could reach the front matter or the byte order mark
// otherwise.
func (l *docLayout) placeEdits(edits []fix.TextEdit) ([]placedEdit, bool, bool) {
	placed := make([]placedEdit, 0, len(edits))
	early := false
	// The edits are sorted, so each is placed from the one before it: its
	// line starts at that one's line, or after the last line ending between
	// them, and many edits on one long line read it once.
	after, afterLine := 0, 0
	for _, edit := range edits {
		if edit.StartOffset < l.frontLimit {
			if !l.firstLineFree || edit.StartOffset < l.base {
				return nil, false, false
			}
			early = true
		}
		edit.StartOffset -= l.base
		edit.EndOffset -= l.base
		next := l.placeAfter(edit, after, afterLine)
		placed = append(placed, next)
		after, afterLine = edit.StartOffset, next.firstLine
	}
	return placed, early, true
}

// placeEdit places an edit, in source offsets, on its lines.
func (l *docLayout) placeEdit(edit fix.TextEdit) placedEdit {
	return l.placeAfter(edit, 0, 0)
}

// placeAfter places an edit, in source offsets, on its lines, given that
// the line holding the offset after, at or before the edit's start, starts
// at afterLine. It reads back from the edit's start to after or to the line
// ending before the start, whichever comes first, and the edit's range.
func (l *docLayout) placeAfter(edit fix.TextEdit, after, afterLine int) placedEdit {
	last := edit.StartOffset
	if edit.EndOffset > edit.StartOffset {
		last = edit.EndOffset - 1
		if l.source[last] == '\n' {
			last = edit.EndOffset
		}
	}
	firstLine := l.lineStartFrom(edit.StartOffset, after, afterLine)
	return placedEdit{edit: edit, firstLine: firstLine, lastLine: l.lineStartFrom(last, edit.StartOffset, firstLine)}
}

// lineStartFrom returns the offset of the start of the line holding offset,
// given that the line holding from, at or before offset, starts at fromLine.
func (l *docLayout) lineStartFrom(offset, from, fromLine int) int {
	offset = min(offset, len(l.source))
	newline := bytes.LastIndexByte(l.source[from:offset], '\n')
	if newline < 0 {
		l.work.placing += offset - from
		return fromLine
	}
	l.work.placing += offset - from - newline
	return from + newline + 1
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
// recorded. The lists are apart and in document order, those in blockquotes
// among them.
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

// tableOf returns the table in group whose body rows hold the lines from
// first to last, with the rows that hold them, or false.
func (l *docLayout) tableOf(group, first, last int) (int, int, int, bool) {
	idx := sort.Search(len(l.tables), func(i int) bool { return l.tables[i].rows[0] > first }) - 1
	if idx < 0 {
		return 0, 0, 0, false
	}
	table := l.tables[idx]
	if table.group != group || last >= table.rows[len(table.rows)-1] {
		return 0, 0, 0, false
	}
	return table.index, table.rowOf(first), table.rowOf(last), true
}

// quoteOf returns the blockquote, among those that may have quote regions,
// in group whose groups but the last hold the lines from first to last, with
// the groups that hold them, or false. Where the last group ends is not
// recorded.
func (l *docLayout) quoteOf(group, first, last int) (int, int, int, bool) {
	idx := sort.Search(len(l.searchableQuotes), func(i int) bool { return l.searchableQuotes[i].bounds[0] > first }) - 1
	if idx < 0 {
		return 0, 0, 0, false
	}
	quote := l.searchableQuotes[idx]
	if quote.group != group || last >= quote.bounds[len(quote.bounds)-1] {
		return 0, 0, 0, false
	}
	return quote.index, quote.groupOf(first), quote.groupOf(last), true
}

// regionFor returns the region that a placed edit needs at least: the rows
// of a table or the items of a list that hold its lines, or else the groups
// of a top-level blockquote, or else the document's groups.
func (l *docLayout) regionFor(placed placedEdit) region {
	first, last := l.groupOf(placed.firstLine), l.groupOf(placed.lastLine)
	if first != last {
		return region{first: first, last: last}
	}
	if list, a, b, ok := l.listOf(first, placed.firstLine, placed.lastLine); ok {
		return region{kind: listRegion, at: list, first: a, last: b}
	}
	if table, a, b, ok := l.tableOf(first, placed.firstLine, placed.lastLine); ok {
		return region{kind: tableRegion, at: table, first: a, last: b}
	}
	if quote, a, b, ok := l.quoteOf(first, placed.firstLine, placed.lastLine); ok {
		return region{kind: quoteRegion, at: quote, first: a, last: b}
	}
	return groupRegionOf(first)
}

// listRegionFits reports whether a list region has an item before it and an
// item after the item after it, so that it starts and ends at marker lines.
func (l *docLayout) listRegionFits(reg region) bool {
	list := l.lists[reg.at]
	return list.regions && reg.first >= 1 && reg.last+2 <= len(list.lines)-1
}

// quoteRegionFits reports whether a quote region has a group of its
// blockquote after it, so that it ends at the start of a group, and starts
// where the document continues its blockquote or opens it with no block
// open: on the line that opens the blockquote after other blocks, such as
// an open list, an edit can make the line one of those blocks.
func (l *docLayout) quoteRegionFits(reg region) bool {
	quote := l.quotes[reg.at]
	return quote.regions && reg.last+1 <= len(quote.bounds)-1 &&
		(quote.bounds[reg.first] != quote.line || quote.opensGroup)
}

// span returns the source offsets that a region parses. A table region
// parses the lines of its table's header and delimiter rows before them.
func (l *docLayout) span(reg region) (int, int) {
	switch reg.kind {
	case quoteRegion:
		quote := l.quotes[reg.at]
		return quote.bounds[reg.first], quote.bounds[reg.last+1]
	case listRegion:
		list := l.lists[reg.at]
		return list.lines[reg.first-1], list.lines[reg.last+2]
	case tableRegion:
		table := l.tables[reg.at]
		return table.rows[reg.first], table.rows[reg.last+1]
	default:
		return l.bounds[reg.first], l.bounds[reg.last+1]
	}
}

// editSpan returns the source offsets between which the edits of a region
// start: in its groups or rows, or in the items it edits. An edit at the end
// of the source belongs to the last group, or to a last row that ends the
// source without a line ending, whose line it is on.
func (l *docLayout) editSpan(reg region) (int, int) {
	switch reg.kind {
	case groupRegion:
		to := l.bounds[reg.last+1]
		if reg.last == len(l.bounds)-2 {
			to++
		}
		return l.bounds[reg.first], to
	case listRegion:
		list := l.lists[reg.at]
		return list.lines[reg.first], list.lines[reg.last+1]
	case tableRegion:
		from, to := l.span(reg)
		if to == len(l.source) {
			to++
		}
		return from, to
	default:
		return l.span(reg)
	}
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
// that share a group; quote regions of one blockquote that share a group;
// list regions of one list that share an item, counting the item before and
// the item after each (the spans of two list regions may only touch, where
// the item after one is the item before the other); table regions of one
// table that share a row; and a region and a list, table or quote region
// inside one of its groups. Regions that do not meet are parsed apart, and
// their spans do not overlap.
func (l *docLayout) meets(one, other region) bool {
	if one.kind > other.kind {
		one, other = other, one
	}
	switch {
	case one.kind == groupRegion && other.kind == groupRegion:
		return one.first <= other.last && other.first <= one.last
	case one.kind == groupRegion:
		group := l.docGroup(other)
		return one.first <= group && group <= one.last
	case one.kind == quoteRegion && other.kind == quoteRegion:
		return one.at == other.at && one.first <= other.last && other.first <= one.last
	case one.kind == quoteRegion:
		quote, inner := l.quoteGroup(other)
		return quote == one.at && one.first <= inner && inner <= one.last
	case one.kind != other.kind:
		// A list and a table are apart.
		return false
	case one.kind == listRegion:
		return one.at == other.at && one.first <= other.last+2 && other.first <= one.last+2
	default:
		return one.at == other.at && one.first <= other.last && other.first <= one.last
	}
}

// mergeRegions returns regions merged so that no two meet, sorted by span.
// A region that does not fit becomes the region that stands for it (see
// fitted); a quote region that a group region covers is dropped, and so is
// a list or table region that a group region or a quote region covers. Two
// regions of one kind that fit merge into one that fits, so merging needs
// one pass: group regions first, then quote regions, then the rest.
func (l *docLayout) mergeRegions(regions []region) []region {
	l.work.merged += len(regions)
	var groups, quotes, inner []region
	for _, reg := range regions {
		reg = l.fitted(reg)
		switch reg.kind {
		case groupRegion:
			groups = append(groups, reg)
		case quoteRegion:
			quotes = append(quotes, reg)
		default:
			inner = append(inner, reg)
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
	byPlace := func(a, b region) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.at, b.at), cmp.Compare(a.first, b.first))
	}
	slices.SortFunc(quotes, byPlace)
	var keptQuotes []region
	for _, reg := range quotes {
		if covered(l.quotes[reg.at].group) {
			continue
		}
		if n := len(keptQuotes); n > 0 && keptQuotes[n-1].at == reg.at && reg.first <= keptQuotes[n-1].last {
			keptQuotes[n-1].last = max(keptQuotes[n-1].last, reg.last)
			continue
		}
		keptQuotes = append(keptQuotes, reg)
	}
	inQuote := func(quote, group int) bool {
		idx := sort.Search(len(keptQuotes), func(i int) bool {
			return cmp.Or(cmp.Compare(keptQuotes[i].at, quote), cmp.Compare(keptQuotes[i].last, group)) >= 0
		})
		return idx < len(keptQuotes) && keptQuotes[idx].at == quote && keptQuotes[idx].first <= group
	}
	slices.SortFunc(inner, byPlace)
	var kept []region
	for _, reg := range inner {
		if covered(l.docGroup(reg)) {
			continue
		}
		if quote, group := l.quoteGroup(reg); quote >= 0 && inQuote(quote, group) {
			continue
		}
		reach := 0
		if reg.kind == listRegion {
			reach = 2
		}
		if n := len(kept); n > 0 && kept[n-1].kind == reg.kind && kept[n-1].at == reg.at &&
			reg.first <= kept[n-1].last+reach {
			kept[n-1].last = max(kept[n-1].last, reg.last)
			continue
		}
		kept = append(kept, reg)
	}
	regions = slices.Concat(merged, keptQuotes, kept)
	slices.SortFunc(regions, func(a, b region) int {
		start, _ := l.span(a)
		other, _ := l.span(b)
		return start - other
	})
	return regions
}

// grow returns a region that holds reg and more of what follows it: twice as
// many groups or items, or the region that stands for a quote or list region
// that would reach the last group of its blockquote or the last items of its
// list (see fitted). A table region becomes the region of its container: its
// rows parse as in the document whatever rows come with them, so more rows
// would not make it hold. A region that ends the document stays.
func (l *docLayout) grow(reg region) region {
	size := reg.last - reg.first + 1
	switch reg.kind {
	case groupRegion:
		reg.last = min(reg.last+size, len(l.bounds)-2)
		return reg
	case tableRegion:
		return l.containerRegion(reg)
	}
	reg.last += size
	return l.fitted(reg)
}

// regionParse is a region parsed with its edits.
type regionParse struct {
	// holds reports whether the region parsed as part of the document: a
	// group region left no block open, a quote region left no block of its
	// blockquote open, the items of a list region opened as in the
	// document, the paragraph of a table region's table stayed open (see
	// parseRegion). undecided reports that the region cannot be checked on
	// its own.
	holds, undecided bool
	// source is the text parsed, and blocks the region's top-level blocks,
	// the blocks of a quote region's blockquote, the items of a list region
	// or the body rows of a table region.
	source []byte
	blocks []gmast.Node
	// refsSame reports whether the region adds the link reference
	// definitions it added before, in the same order.
	refsSame bool
	// tight reports whether the list of a list region is tight, as
	// CommonMark reads it (gmparser.ListTight), which decides for its items
	// but whether its first follows a blank line.
	tight bool
}

// verdict reports whether a parsed region holds and whether it cannot be
// checked on its own.
func (p regionParse) verdict() (bool, bool) {
	return p.holds, p.undecided
}

// regionVerdict is what decomposeRegions needs of the outcome of a region
// with its edits, whatever a check compares.
type regionVerdict interface {
	verdict() (holds, undecided bool)
}

// decomposeRegions returns the regions that placed edits, sorted, need,
// merged and grown until each holds, with their outcomes as evaluate finds
// them for the edits that start in each. ok is false when a region cannot be
// checked on its own or cannot grow to hold.
//
// Regions are evaluated in order. One that does not hold grows, takes in the
// regions it then meets on either side (see absorbGrown) and is evaluated
// again, so a growth costs the regions it takes in, not all the regions.
func decomposeRegions[O regionVerdict](
	layout *docLayout, placed []placedEdit, evaluate func(region, []placedEdit) O,
) ([]region, []O, bool) {
	initial := make([]region, len(placed))
	for idx, edit := range placed {
		initial[idx] = layout.regionFor(edit)
	}
	// pending[next:] holds the regions still to evaluate, sorted by span;
	// done holds those that hold, with their outcomes, all before them.
	pending := layout.mergeRegions(initial)
	next := 0
	done := make([]region, 0, len(pending))
	outcomes := make([]O, 0, len(pending))
	for next < len(pending) {
		reg := pending[next]
		from, to := layout.editSpan(reg)
		outcome := evaluate(reg, regionEdits(from, to, placed))
		holds, undecided := outcome.verdict()
		switch {
		case undecided:
			return nil, nil, false
		case holds:
			done = append(done, reg)
			outcomes = append(outcomes, outcome)
			next++
			continue
		}
		grown := layout.grow(reg)
		if grown == reg {
			return nil, nil, false
		}
		lo, hi, merged := layout.absorbGrown(done, pending, next, grown)
		done, outcomes = done[:lo], outcomes[:lo]
		// Every region done or taken in has used a slot of pending before
		// hi, so the merged regions fit there, to be evaluated next.
		next = hi - len(merged)
		copy(pending[next:hi], merged)
	}
	return done, outcomes, true
}

// absorbGrown merges grown, the region at pending[next] after it grew, with
// the regions it meets, as mergeRegions merges them: the regions done[lo:]
// that hold before it and pending[next+1:hi] after it, all sorted by span
// with no two meeting. It returns lo, hi and the merged regions, sorted by
// span. Regions meet exactly where their spans overlap: group regions that
// share a group, quote regions of one blockquote that share a group, list
// regions of one list where first <= other.last+2, table regions of one
// table that share a row, and a list, table or quote region and a region of
// a group that holds it, whose span holds the inner region's. So the regions
// taken in are those whose spans overlap the grown region's; regions whose
// spans only touch it stay. One pass finds them all: the spans of the
// regions are disjoint and sorted, so the region after the last one taken in
// starts at or after the end of every span in the window, and the merged
// region, whose span covers theirs, ends there. grown must be a region that
// grow returns: a list or quote region that fits, or a group region.
func (l *docLayout) absorbGrown(done, pending []region, next int, grown region) (int, int, []region) {
	start, end := l.span(grown)
	lo := len(done)
	for lo > 0 {
		if _, prevEnd := l.span(done[lo-1]); prevEnd <= start {
			break
		}
		lo--
	}
	window := append(slices.Clone(done[lo:]), grown)
	hi := next + 1
	for hi < len(pending) {
		if nextStart, _ := l.span(pending[hi]); nextStart >= end {
			break
		}
		window = append(window, pending[hi])
		hi++
	}
	return lo, hi, l.mergeRegions(window)
}

// listsTightAfter reports, for each list that a list region of regions
// edits, whether it is tight after the edits: none of its items outside its
// list regions makes it loose, and none of its list regions does, as loosens
// reports for the region at an index of regions. One pass over the regions
// serves every list.
func (l *docLayout) listsTightAfter(regions []region, loosens func(int) bool) map[int]bool {
	type count struct {
		inside   int
		loosened bool
	}
	counts := make(map[int]count)
	for idx, reg := range regions {
		l.work.tightVisits++
		if !reg.isList() {
			continue
		}
		c := counts[reg.at]
		c.inside += l.lists[reg.at].looseIn(reg)
		c.loosened = c.loosened || loosens(idx)
		counts[reg.at] = c
	}
	tight := make(map[int]bool, len(counts))
	for list, c := range counts {
		tight[list] = l.lists[list].tightWith(c.inside, c.loosened)
	}
	return tight
}

// tightWith reports whether the list is tight when its list regions hold the
// given number of its loose items, and whether one of them makes it loose.
// Without list regions, that is the list's tightness in the original.
func (l *layoutList) tightWith(inside int, loosened bool) bool {
	return !loosened && l.loose[len(l.loose)-1]-inside == 0
}

// sentinel returns the line that a region's parse adds after it: the
// sentinel of a group region that does not end the document or of a table
// region, or the quote sentinel of a quote region or of a table region in a
// blockquote. A list region needs none.
func (l *docLayout) sentinel(reg region) string {
	switch reg.kind {
	case groupRegion:
		if reg.last == len(l.bounds)-2 {
			return ""
		}
		return regionSentinel
	case quoteRegion:
		return quoteSentinel
	case tableRegion:
		if l.tables[reg.at].quote >= 0 {
			return quoteSentinel
		}
		return regionSentinel
	default:
		return ""
	}
}

// parseRegion parses a region with its edits, which are in source offsets
// and sorted.
func (l *docLayout) parseRegion(reg region, edits []fix.TextEdit) regionParse {
	l.work.kinds[reg.kind]++
	switch reg.kind {
	case quoteRegion:
		return l.parseQuoteRegion(reg, edits)
	case listRegion:
		return l.parseListRegion(reg, edits)
	case tableRegion:
		return l.parseTableRegion(reg, edits)
	default:
		return l.parseGroupRegion(reg, edits)
	}
}

// parseGroupRegion parses a group region. Unless it ends the document, the
// sentinel line follows it, and the region holds when the parser starts a
// group at the sentinel of the kind the original's next group starts with.
func (l *docLayout) parseGroupRegion(reg region, edits []fix.TextEdit) regionParse {
	from, to := l.span(reg)
	text := applyWithin(l.source, from, to, edits)
	final := reg.last == len(l.bounds)-2
	length := len(text)
	if !final {
		if length == 0 || text[length-1] != '\n' {
			return regionParse{undecided: true}
		}
		text = append(text, l.sentinel(reg)...)
	}
	l.work.parsed += len(text)
	parsed := parseRecorded(l.markdown, text, l.refIndex, false)
	blocks := children(parsed.doc)
	if !final {
		groups := parsed.rec.groups
		if len(groups) == 0 {
			return regionParse{}
		}
		last := groups[len(groups)-1]
		if last.offset != length || last.afterClosed != l.afterClosed[reg.last+1] {
			return regionParse{}
		}
		counts, ok := groupBlocks(blocks, []groupStart{last})
		if !ok {
			return regionParse{undecided: true}
		}
		blocks = blocks[:counts[0]]
	}
	return regionParse{holds: true, source: text, blocks: blocks, refsSame: l.sameRefs(from, to, parsed.rec.refs)}
}

// parseQuoteRegion parses a quote region. The quote sentinel line follows it,
// and the region holds when the parse is one blockquote that opens a block
// on the region's first line and starts a group at the sentinel as the
// original's next group starts: after a closed block or not, with the first
// block opened there marked as following a blank line or not as the
// original's (see Quote regions). A region whose edits take its last line
// ending away does not hold.
func (l *docLayout) parseQuoteRegion(reg region, edits []fix.TextEdit) regionParse {
	quote := l.quotes[reg.at]
	from, to := l.span(reg)
	text := applyWithin(l.source, from, to, edits)
	length := len(text)
	if length == 0 || text[length-1] != '\n' {
		return regionParse{}
	}
	text = append(text, l.sentinel(reg)...)
	l.work.parsed += len(text)
	parsed := parseRecorded(l.markdown, text, l.refIndex, false)
	quoteNode, ok := soleQuote(children(parsed.doc))
	if !ok {
		return regionParse{}
	}
	starts := parsed.rec.quotes[quoteNode]
	if len(starts) < 2 || starts[0].offset != 0 {
		return regionParse{}
	}
	next := starts[len(starts)-1]
	if next.offset != length || next.afterClosed != quote.afterClosed[reg.last+1] ||
		next.opened.HasBlankPreviousLines() != quote.blank[reg.last+1] {
		return regionParse{}
	}
	blocks := children(quoteNode)
	counts, ok := groupBlocks(blocks, []groupStart{next})
	if !ok {
		return regionParse{undecided: true}
	}
	return regionParse{
		holds: true, source: text, blocks: blocks[:counts[0]], refsSame: l.sameRefs(from, to, parsed.rec.refs),
	}
}

// soleQuote returns the blockquote that is the one block of blocks.
func soleQuote(blocks []gmast.Node) (*gmast.Blockquote, bool) {
	if len(blocks) != 1 {
		return nil, false
	}
	quote, ok := blocks[0].(*gmast.Blockquote)
	return quote, ok
}

// parseListRegion parses the items of a list region with the item before and
// the item after them. The region holds when the parse is one list, in one
// blockquote for a list in a blockquote, whose first item opens on the first
// line and whose last item opens on the marker line of the item after the
// region.
func (l *docLayout) parseListRegion(reg region, edits []fix.TextEdit) regionParse {
	list := l.lists[reg.at]
	from, to := l.span(reg)
	text := applyWithin(l.source, from, to, edits)
	l.work.parsed += len(text)
	parsed := parseRecorded(l.markdown, text, l.refIndex, false)
	blocks := children(parsed.doc)
	if list.quote >= 0 {
		quoteNode, ok := soleQuote(blocks)
		if !ok {
			return regionParse{}
		}
		blocks = children(quoteNode)
	}
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
		tight:    gmparser.ListTight(parsedList, text),
	}
}

// parseTableRegion parses the body rows of a table region after the lines
// of the table's header and delimiter rows, followed by the sentinel line.
// The region holds when the parse is one table, in one blockquote for a
// table in a blockquote, whose last row is the sentinel's (see Table
// regions). A region whose edits take its last line ending away does not
// hold.
func (l *docLayout) parseTableRegion(reg region, edits []fix.TextEdit) regionParse {
	table := l.tables[reg.at]
	from, to := l.span(reg)
	text := slices.Concat(l.source[table.head:table.rows[0]], applyWithin(l.source, from, to, edits))
	length := len(text)
	if text[length-1] != '\n' {
		return regionParse{}
	}
	text = append(text, l.sentinel(reg)...)
	l.work.parsed += len(text)
	parsed := parseRecorded(l.markdown, text, l.refIndex, false)
	blocks := children(parsed.doc)
	if table.quote >= 0 {
		quoteNode, ok := soleQuote(blocks)
		if !ok {
			return regionParse{}
		}
		blocks = children(quoteNode)
	}
	if len(blocks) != 1 {
		return regionParse{}
	}
	parsedTable, ok := blocks[0].(*gmextast.Table)
	if !ok {
		return regionParse{}
	}
	rows := children(parsedTable)
	if len(rows) < 2 {
		return regionParse{}
	}
	if _, isHeader := rows[0].(*gmextast.TableHeader); !isHeader || lineStartIn(text, rows[len(rows)-1].Pos()) != length {
		return regionParse{}
	}
	return regionParse{
		holds: true, source: text, blocks: rows[1 : len(rows)-1], refsSame: l.sameRefs(from, to, parsed.rec.refs),
	}
}

// applyWithin returns a copy of source[from:to] with edits applied; the
// edits lie in that range and are sorted. The copy matters: fix.ApplyEdits
// returns its input when there are no edits, and a group region appends the
// sentinel to the text.
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
