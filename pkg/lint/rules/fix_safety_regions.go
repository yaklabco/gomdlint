package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yuin/goldmark"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint/refs"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// Region checks for the outline that fixVerifier compares.
//
// The outline of a document (documentOutline) is its front matter item, the
// Document item with the outlines of the top-level blocks in order, and the
// anchors item. A block's outline depends only on the block, the links it
// resolves and the check's allowances, and a list's is a token with its
// tightness and the outlines of its items. So, on the argument of the region
// core (see fix_regions.go), the outline after edits is the original's with
// each region's part outlined anew and the token of each list whose
// tightness changes replaced, and with the anchors of the headings
// recomputed, provided the front matter stays and every region adds the
// definitions it added before. Whether a region holds comes from the
// recording parse of docLayout; its outline comes from a parse with the
// document's parser, of the region after a blank line, with the document's
// definitions preset. The blank line keeps the region's first line from
// being read as front matter, and a byte order mark that an edit puts at
// the region's start from being taken for the document's. An item's outline
// does not depend on whether its list is tight, though goldmark turns the
// paragraphs of a tight list into text blocks, because the parser maps both
// to paragraphs. When anything cannot be shown, the whole document is
// parsed, as before.

// slugOverhead is what the cache counts for each slug it holds besides the
// slug's bytes: the string header.
const slugOverhead = 16

// outlineModel is the outline of the original document, as fixVerifier
// compares it, cut into the parts that regions replace.
type outlineModel struct {
	*docLayout
	verifier *fixVerifier
	// body is the outline up to the anchors item. groups holds where the
	// blocks of each group start in body, counting the headings with an
	// anchor. listParts, quoteParts and tableParts hold where the parts of
	// each list, blockquote and table of the layout are.
	body       string
	groups     regionParts
	listParts  []listOutline
	quoteParts []regionParts
	tableParts []regionParts
	// slugs holds the slug of each heading with an anchor, in order, history
	// the anchors GitHub gives them with every count it keeps (see
	// recordAnchors), and anchors the text of the anchors item: the anchors
	// joined as documentOutline joins them.
	slugs   []string
	history anchorHistory
	anchors string
	// outcomes caches what parsing a region with its edits found, and
	// cached counts the bytes it holds of its own.
	outcomes map[string]*outlineOutcome
	cached   int
}

// listOutline is where the outline of a list that the layout knows has its
// parts.
type listOutline struct {
	tight bool
	// tokenStart and tokenEnd bound the item that opens the list, and
	// tokens holds that item for a tight list and for a loose one. items
	// holds where the outline of each item starts.
	tokenStart, tokenEnd int
	tokens               map[bool]string
	items                regionParts
}

// outlinePlaces finds the lists, blockquotes and tables of the layout by the
// line where their outlines start: a list's first marker line, a
// blockquote's first line and a table's header line.
type outlinePlaces struct {
	lists, quotes, tables map[int]int
}

// outlineOutcome is what parsing a region with its edits found.
type outlineOutcome struct {
	regionParse
	// start and end bound the region's part of body, and text is the
	// outline of the region after the edits; same reports whether text is
	// that part.
	start, end int
	text       string
	same       bool
	// slugs holds the slugs of the region's headings with anchors after the
	// edits, when they are not those of the original.
	slugs     []string
	slugsSame bool
	// loosens reports whether a list region makes its list loose.
	loosens bool
}

// recordingMarkdown returns the recording goldmark instance that reads as a
// parser of the given flavor does: GFM, or CommonMark for any other flavor,
// as gmparser.New takes them.
func recordingMarkdown(flavor string) goldmark.Markdown {
	gfm, commonMark := recordingInstances()
	if flavor == gmparser.FlavorGFM {
		return gfm
	}
	return commonMark
}

// headingSlug returns the slug GitHub derives from a heading's text, and
// false for a heading whose text gives it no anchor (see refs.HeadingAnchors,
// which numbers repeated slugs; a single heading gets its slug).
func headingSlug(heading *mdast.Node) (string, bool) {
	anchors := refs.HeadingAnchors(heading)
	if len(anchors) == 0 {
		return "", false
	}
	return anchors[0], true
}

// nodeSlugs returns the slugs of the headings with anchors among nodes and
// their descendants, in document order.
func nodeSlugs(nodes []*mdast.Node) []string {
	var slugs []string
	for _, node := range nodes {
		//nolint:errcheck // The visitor never returns an error.
		_ = mdast.Walk(node, func(child *mdast.Node) error {
			if child.Kind == mdast.NodeHeading {
				if slug, ok := headingSlug(child); ok {
					slugs = append(slugs, slug)
				}
			}
			return nil
		})
	}
	return slugs
}

// outlineItem returns the opening of an outline item, as writeOutlineItem
// writes it.
func outlineItem(name, attrs string) string {
	var out strings.Builder
	writeOutlineItem(&out, name, attrs)
	return out.String()
}

// listTokens returns the item that opens the outline of a list, as
// writeOutline writes it, for the list tight and loose.
func listTokens(file *mdast.FileSnapshot, list *mdast.Node, allow fixAllowance) (map[bool]string, bool) {
	if list.Block == nil || list.Block.List == nil {
		return nil, false
	}
	tokens := make(map[bool]string, 2)
	for _, tight := range []bool{true, false} {
		block := *list.Block
		attrs := *block.List
		attrs.Tight = tight
		block.List = &attrs
		node := *list
		node.Block = &block
		tokens[tight] = outlineItem(node.Kind.String(), blockOutlineAttrs(file, &node, allow))
	}
	return tokens, true
}

// buildOutlineModel cuts the outline of original, the verifier's parse of
// the document, into the parts that regions replace. It returns nil when
// region checks cannot be used: when the layout cannot be built, a top-level
// block of the parse cannot be placed in a group, or the outline written in
// parts is not the verifier's. A list, blockquote or table whose parts are
// not the layout's takes no regions of its own: its regions become those of
// its container.
func buildOutlineModel(verifier *fixVerifier, original *mdast.FileSnapshot) *outlineModel {
	layout := newDocLayout(recordingMarkdown(verifier.parser.Flavor()), verifier.file.Content)
	if layout == nil {
		return nil
	}
	model := &outlineModel{
		docLayout:  layout,
		verifier:   verifier,
		listParts:  make([]listOutline, len(layout.lists)),
		quoteParts: make([]regionParts, len(layout.quotes)),
		tableParts: make([]regionParts, len(layout.tables)),
		outcomes:   make(map[string]*outlineOutcome),
	}
	places := outlinePlaces{
		lists:  make(map[int]int, len(layout.lists)),
		quotes: make(map[int]int, len(layout.quotes)),
		tables: make(map[int]int, len(layout.tables)),
	}
	for idx, list := range layout.lists {
		places.lists[list.lines[0]] = idx
	}
	for idx, quote := range layout.quotes {
		places.quotes[quote.line] = idx
	}
	for idx, table := range layout.tables {
		places.tables[table.head] = idx
	}
	groups, ok := model.placeBlocks(original.Root, len(layout.bounds)-1, layout.groupOf)
	if !ok {
		return nil
	}

	var out strings.Builder
	frontMatter := ""
	if lines := original.FrontMatterLines; lines > 0 && lines <= len(original.Lines) {
		frontMatter = string(original.Content[:original.Lines[lines-1].EndOffset])
	}
	writeOutlineItem(&out, "frontmatter", frontMatter)
	writeOutlineItem(&out, original.Root.Kind.String(), blockOutlineAttrs(original, original.Root, verifier.allow))
	for _, nodes := range groups {
		model.groups.add(out.Len(), len(model.slugs))
		for _, node := range nodes {
			model.writeBlock(&out, original, node, places, true)
		}
	}
	model.groups.add(out.Len(), len(model.slugs))
	out.WriteString(outlineClose)
	// A list, blockquote or table without parts takes no regions of its own.
	for idx, list := range layout.lists {
		if list.regions && model.listParts[idx].items.starts == nil {
			list.regions = false
		}
	}
	for idx, quote := range layout.quotes {
		if model.quoteParts[idx].starts == nil {
			quote.regions = false
		}
	}
	for idx, table := range layout.tables {
		if model.tableParts[idx].starts == nil {
			table.regions = false
		}
	}
	model.body = out.String()
	model.history = recordAnchors(model.slugs)
	model.anchors = strings.Join(model.history.anchors, "\x00")
	if model.body+outlineItem("anchors", model.anchors) != verifier.original {
		return nil
	}
	return model
}

// placeBlocks returns the blocks of parent in the groups that groupOf finds
// for their first lines, of which there are count. ok is false when a block
// has no span of its own, which cannot be placed in a group, or the groups
// of the blocks are not in order.
func (m *outlineModel) placeBlocks(parent *mdast.Node, count int, groupOf func(int) int) ([][]*mdast.Node, bool) {
	groups := make([][]*mdast.Node, count)
	lastGroup := 0
	for child := parent.FirstChild; child != nil; child = child.Next {
		rng := child.SourceRange()
		offset := rng.StartOffset - m.base
		if rng.IsEmpty() || offset < 0 || offset > len(m.source) {
			return nil, false
		}
		group := groupOf(lineStartIn(m.source, offset))
		if group < lastGroup || group >= count {
			return nil, false
		}
		lastGroup = group
		groups[group] = append(groups[group], child)
	}
	return groups, true
}

// writeBlock writes the outline of a top-level block of the original, with
// top, or of a block of a top-level blockquote, and records the slugs of its
// headings, and where the parts of a list, blockquote or table that the
// layout knows are. A list, blockquote or table whose parts are not the
// layout's is written without them.
func (m *outlineModel) writeBlock(
	out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, places outlinePlaces, top bool,
) {
	line := lineStartIn(m.source, node.SourceRange().StartOffset-m.base)
	switch node.Kind {
	case mdast.NodeList:
		// Only a list that list regions may split needs its parts.
		if index, known := places.lists[line]; known && m.lists[index].regions && m.writeList(out, file, node, index) {
			return
		}
	case mdast.NodeBlockquote:
		// A blockquote in a top-level one can start on the same line.
		if index, known := places.quotes[line]; known && top && m.writeQuote(out, file, node, index, places) {
			return
		}
	case mdast.NodeTable:
		if index, known := places.tables[line]; known && m.writeTable(out, file, node, index) {
			return
		}
	}
	writeOutline(out, file, node, m.verifier.allow)
	m.slugs = append(m.slugs, nodeSlugs([]*mdast.Node{node})...)
}

// writeList writes the outline of a list that list regions may split, with
// its parts. It writes nothing and returns false when the list's items are
// not the layout's.
func (m *outlineModel) writeList(out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, index int) bool {
	layoutList := m.lists[index]
	tokens, ok := listTokens(file, node, m.verifier.allow)
	if !ok {
		return false
	}
	item := 0
	for child := node.FirstChild; child != nil; child = child.Next {
		if item >= len(layoutList.lines) ||
			lineStartIn(m.source, child.SourceRange().StartOffset-m.base) != layoutList.lines[item] {
			return false
		}
		item++
	}
	if item != len(layoutList.lines) {
		return false
	}
	list := listOutline{tight: node.Block.List.Tight, tokens: tokens, tokenStart: out.Len()}
	out.WriteString(tokens[list.tight])
	list.tokenEnd = out.Len()
	for child := node.FirstChild; child != nil; child = child.Next {
		list.items.add(out.Len(), len(m.slugs))
		writeOutline(out, file, child, m.verifier.allow)
		m.slugs = append(m.slugs, nodeSlugs([]*mdast.Node{child})...)
	}
	list.items.add(out.Len(), len(m.slugs))
	out.WriteString(outlineClose)
	m.listParts[index] = list
	return true
}

// writeQuote writes the outline of a top-level blockquote, as writeOutline
// does, with its parts and those of the lists and tables in it. It writes
// nothing and returns false when the blockquote's blocks cannot be placed
// in its groups.
func (m *outlineModel) writeQuote(
	out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, index int, places outlinePlaces,
) bool {
	quote := m.quotes[index]
	groups, ok := m.placeBlocks(node, len(quote.bounds), quote.groupOf)
	if !ok {
		return false
	}
	var parts regionParts
	writeOutlineItem(out, node.Kind.String(), blockOutlineAttrs(file, node, m.verifier.allow))
	for _, nodes := range groups {
		parts.add(out.Len(), len(m.slugs))
		for _, child := range nodes {
			m.writeBlock(out, file, child, places, false)
		}
	}
	parts.add(out.Len(), len(m.slugs))
	out.WriteString(outlineClose)
	m.quoteParts[index] = parts
	return true
}

// writeTable writes the outline of a table, as writeOutline does, with its
// parts. It writes nothing and returns false when the table's rows are not
// the layout's.
func (m *outlineModel) writeTable(out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, index int) bool {
	table := m.tables[index]
	row := 0
	for child := node.FirstChild; child != nil; child = child.Next {
		if isHeaderRow(child) {
			continue
		}
		if row >= len(table.rows)-1 ||
			lineStartIn(m.source, child.SourceRange().StartOffset-m.base) != table.rows[row] {
			return false
		}
		row++
	}
	if row != len(table.rows)-1 {
		return false
	}
	// A table holds no headings.
	var parts regionParts
	writeOutlineItem(out, node.Kind.String(), blockOutlineAttrs(file, node, m.verifier.allow))
	for child := node.FirstChild; child != nil; child = child.Next {
		if !isHeaderRow(child) {
			parts.add(out.Len(), len(m.slugs))
		}
		writeOutline(out, file, child, m.verifier.allow)
	}
	parts.add(out.Len(), len(m.slugs))
	out.WriteString(outlineClose)
	m.tableParts[index] = parts
	return true
}

// outSpan returns where a region's part of body starts and ends, and the
// indexes of its first heading with an anchor and of the first after it.
func (m *outlineModel) outSpan(reg region) (int, int, int, int) {
	switch reg.kind {
	case quoteRegion:
		return m.quoteParts[reg.at].partOf(reg)
	case listRegion:
		return m.listParts[reg.at].items.partOf(reg)
	case tableRegion:
		return m.tableParts[reg.at].partOf(reg)
	default:
		return m.groups.partOf(reg)
	}
}

// cacheBudget returns how many bytes of their own the cached outcomes may
// hold (see regionModel.cacheBudget).
func (m *outlineModel) cacheBudget() int {
	return 8*len(m.body) + 1<<20
}

// evaluate parses a region with the edits that start in it, sorted, or
// returns the cached outcome.
func (m *outlineModel) evaluate(reg region, edits []placedEdit) *outlineOutcome {
	var key strings.Builder
	regionKey(&key, reg)
	plain := make([]fix.TextEdit, len(edits))
	for idx, edit := range edits {
		plain[idx] = edit.edit
		fmt.Fprintf(&key, "%d:%d:%q;", edit.edit.StartOffset, edit.edit.EndOffset, edit.edit.NewText)
	}
	if outcome, found := m.outcomes[key.String()]; found {
		return outcome
	}
	outcome := &outlineOutcome{regionParse: m.parseRegion(reg, plain)}
	start, end, from, to := m.outSpan(reg)
	outcome.start, outcome.end = start, end
	if outcome.holds && !outcome.undecided {
		if reg.isList() {
			outcome.loosens = m.docLayout.lists[reg.at].loosens(reg, outcome.tight)
		}
		m.outline(outcome, reg, from, to)
	}
	outcome.source, outcome.blocks = nil, nil
	size := key.Len()
	if !outcome.same {
		size += len(outcome.text)
	}
	for _, slug := range outcome.slugs {
		size += len(slug) + slugOverhead
	}
	if m.cached+size <= m.cacheBudget() {
		m.outcomes[key.String()] = outcome
		m.cached += size
	}
	return outcome
}

// outline parses a region that holds with the document's parser, writes the
// outline of its blocks, the blocks of its blockquote, its items or its body
// rows, and finds whether the slugs of their headings are those of headings
// [from, to) of the original.
func (m *outlineModel) outline(outcome *outlineOutcome, reg region, from, to int) {
	text := outcome.source
	// The blank line before the region shifts its offsets by one, and the
	// region's blocks start before the sentinel.
	end := 1 + len(text) - len(m.sentinel(reg))
	verifier := m.verifier
	m.work.parsed += 1 + len(text)
	snapshot, err := verifier.parser.ParseWithReferences(verifier.ctx, verifier.file.Path,
		append([]byte{'\n'}, text...), m.refIndex)
	if err != nil {
		outcome.undecided = true
		return
	}
	nodes, ok := m.regionNodes(reg, snapshot.Root, end)
	if !ok {
		outcome.undecided = true
		return
	}
	var out strings.Builder
	for _, node := range nodes {
		writeOutline(&out, snapshot, node, verifier.allow)
	}
	part := m.body[outcome.start:outcome.end]
	outcome.same = out.String() == part
	outcome.text = part
	if !outcome.same {
		outcome.text = out.String()
	}
	slugs := nodeSlugs(nodes)
	outcome.slugsSame = slices.Equal(slugs, m.slugs[from:to])
	if !outcome.slugsSame {
		outcome.slugs = slugs
	}
}

// regionNodes returns the nodes of the parse of a region, from its root, whose
// outlines make up the region's part, for a parse whose region's blocks
// start before end: the top-level blocks, the blocks of the one blockquote
// of a quote region, the items of the one list of a list region, or the
// body rows of the one table of a table region, in one blockquote for a list
// or table in a blockquote. ok is false when the parse is not of that shape,
// or a block has no span of its own.
func (m *outlineModel) regionNodes(reg region, root *mdast.Node, end int) ([]*mdast.Node, bool) {
	nodes, ok := blocksBefore(root, end)
	inQuote := reg.kind == quoteRegion ||
		(reg.kind == listRegion && m.lists[reg.at].quote >= 0) ||
		(reg.kind == tableRegion && m.tables[reg.at].quote >= 0)
	if ok && inQuote {
		if len(nodes) != 1 || nodes[0].Kind != mdast.NodeBlockquote {
			return nil, false
		}
		nodes, ok = blocksBefore(nodes[0], end)
	}
	if !ok {
		return nil, false
	}
	switch reg.kind {
	case listRegion:
		if len(nodes) != 1 || nodes[0].Kind != mdast.NodeList {
			return nil, false
		}
		var items []*mdast.Node
		for item := nodes[0].FirstChild; item != nil; item = item.Next {
			items = append(items, item)
		}
		return items, true
	case tableRegion:
		if len(nodes) != 1 || nodes[0].Kind != mdast.NodeTable {
			return nil, false
		}
		// The rows before the sentinel's, but the header.
		rows, found := blocksBefore(nodes[0], end)
		if !found || len(rows) == 0 || !isHeaderRow(rows[0]) {
			return nil, false
		}
		return rows[1:], true
	default:
		return nodes, true
	}
}

// isHeaderRow reports whether a node is the header row of a table.
func isHeaderRow(row *mdast.Node) bool {
	return row.Kind == mdast.NodeTableRow && row.Block != nil && row.Block.Table != nil && row.Block.Table.Header
}

// blocksBefore returns the blocks of parent that start before end. ok is
// false when one of them has no span of its own, which cannot be placed
// before or after the sentinel.
func blocksBefore(parent *mdast.Node, end int) ([]*mdast.Node, bool) {
	var nodes []*mdast.Node
	for child := parent.FirstChild; child != nil; child = child.Next {
		rng := child.SourceRange()
		if rng.IsEmpty() {
			return nil, false
		}
		if rng.StartOffset < end {
			nodes = append(nodes, child)
		}
	}
	return nodes, true
}

// checkEdits reports whether applying edits, prepared and in content
// offsets, leaves the outline unchanged. decided is false when region checks
// cannot tell, so that the whole document must be parsed.
func (m *outlineModel) checkEdits(edits []fix.TextEdit) (bool, bool) {
	if len(edits) == 0 {
		return true, true
	}
	placed, early, ok := m.placeEdits(edits)
	if !ok || (early && !m.firstLineKeeps(placed, nil)) {
		return false, false
	}
	regions, outcomes, ok := decomposeRegions(m.docLayout, placed, m.evaluate)
	if !ok {
		return false, false
	}
	// Definitions reach links before them as well as after.
	for _, outcome := range outcomes {
		if !outcome.refsSame {
			return false, false
		}
	}
	reps := make([]replacement, 0, len(outcomes))
	for _, outcome := range outcomes {
		reps = append(reps, replacement{start: outcome.start, end: outcome.end, text: outcome.text})
	}
	for index, tight := range m.listsTightAfter(regions, func(at int) bool { return outcomes[at].loosens }) {
		if list := m.listParts[index]; tight != list.tight {
			reps = append(reps, replacement{start: list.tokenStart, end: list.tokenEnd, text: list.tokens[tight]})
		}
	}
	if _, unchanged := decideReplacements(m.body, reps); !unchanged {
		return true, false
	}
	return true, m.anchorsKept(regions, outcomes)
}

// anchorsKept reports whether the anchors of the edited document join to
// the text of the original's anchors item: each region that changes the
// slugs of its headings replaces them, and the anchor walk works out the
// anchors that change (see anchorHistory.keeps).
func (m *outlineModel) anchorsKept(regions []region, outcomes []*outlineOutcome) bool {
	var changes []anchorChange
	for idx, outcome := range outcomes {
		if !outcome.slugsSame {
			_, _, from, to := m.outSpan(regions[idx])
			changes = append(changes, anchorChange{from: from, to: to, slugs: outcome.slugs})
		}
	}
	return m.history.keeps(changes)
}
