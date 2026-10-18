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
	// body is the outline up to the anchors item. groupOut and groupHead
	// hold, for each group and for the end, where its blocks start in body
	// and the index of its first heading with an anchor.
	body      string
	groupOut  []int
	groupHead []int
	listParts []listOutline
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

// listOutline is where the outline of a top-level list has its parts.
type listOutline struct {
	tight bool
	// tokenStart and tokenEnd bound the item that opens the list, and
	// tokens holds that item for a tight list and for a loose one.
	tokenStart, tokenEnd int
	tokens               map[bool]string
	// itemOut and itemHead hold, for each item and for the end, where its
	// outline starts and the index of its first heading with an anchor.
	itemOut  []int
	itemHead []int
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
// block of the parse cannot be placed in a group, a list's items are not
// those the layout found, or the outline written in parts is not the
// verifier's.
func buildOutlineModel(verifier *fixVerifier, original *mdast.FileSnapshot) *outlineModel {
	layout := newDocLayout(recordingMarkdown(verifier.parser.Flavor()), verifier.file.Content)
	if layout == nil {
		return nil
	}
	model := &outlineModel{
		docLayout: layout,
		verifier:  verifier,
		listParts: make([]listOutline, len(layout.lists)),
		outcomes:  make(map[string]*outlineOutcome),
	}
	groups := make([][]*mdast.Node, len(layout.bounds)-1)
	listAt := make(map[int]int, len(layout.lists))
	for idx, list := range layout.lists {
		listAt[list.lines[0]] = idx
	}
	lastGroup := 0
	for child := original.Root.FirstChild; child != nil; child = child.Next {
		// A block without a span of its own cannot be placed in a group.
		rng := child.SourceRange()
		offset := rng.StartOffset - layout.base
		if rng.IsEmpty() || offset < 0 || offset > len(layout.source) {
			return nil
		}
		group := layout.groupOf(lineStartIn(layout.source, offset))
		if group < lastGroup {
			return nil
		}
		lastGroup = group
		groups[group] = append(groups[group], child)
	}

	var out strings.Builder
	frontMatter := ""
	if lines := original.FrontMatterLines; lines > 0 && lines <= len(original.Lines) {
		frontMatter = string(original.Content[:original.Lines[lines-1].EndOffset])
	}
	writeOutlineItem(&out, "frontmatter", frontMatter)
	writeOutlineItem(&out, original.Root.Kind.String(), blockOutlineAttrs(original, original.Root, verifier.allow))
	for _, nodes := range groups {
		model.groupOut = append(model.groupOut, out.Len())
		model.groupHead = append(model.groupHead, len(model.slugs))
		for _, node := range nodes {
			if !model.writeTopLevel(&out, original, node, listAt) {
				return nil
			}
		}
	}
	model.groupOut = append(model.groupOut, out.Len())
	model.groupHead = append(model.groupHead, len(model.slugs))
	out.WriteString(outlineClose)
	// Every list that list regions may split needs its parts.
	for idx, list := range layout.lists {
		if list.regions && model.listParts[idx].itemOut == nil {
			return nil
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

// writeTopLevel writes the outline of a top-level block of the original and
// records the slugs of its headings, and where the parts of a list that the
// layout knows are. ok is false when such a list's items are not the
// layout's.
func (m *outlineModel) writeTopLevel(
	out *strings.Builder, file *mdast.FileSnapshot, node *mdast.Node, listAt map[int]int,
) bool {
	index, known := -1, false
	if node.Kind == mdast.NodeList {
		index, known = listAt[lineStartIn(m.source, node.SourceRange().StartOffset-m.base)]
	}
	// Only a list that list regions may split needs its parts.
	if !known || !m.lists[index].regions {
		writeOutline(out, file, node, m.verifier.allow)
		m.slugs = append(m.slugs, nodeSlugs([]*mdast.Node{node})...)
		return true
	}
	layoutList := m.lists[index]
	tokens, ok := listTokens(file, node, m.verifier.allow)
	if !ok {
		return false
	}
	list := listOutline{tight: node.Block.List.Tight, tokens: tokens, tokenStart: out.Len()}
	out.WriteString(tokens[list.tight])
	list.tokenEnd = out.Len()
	item := 0
	for child := node.FirstChild; child != nil; child = child.Next {
		if item >= len(layoutList.lines) ||
			lineStartIn(m.source, child.SourceRange().StartOffset-m.base) != layoutList.lines[item] {
			return false
		}
		list.itemOut = append(list.itemOut, out.Len())
		list.itemHead = append(list.itemHead, len(m.slugs))
		writeOutline(out, file, child, m.verifier.allow)
		m.slugs = append(m.slugs, nodeSlugs([]*mdast.Node{child})...)
		item++
	}
	if item != len(layoutList.lines) {
		return false
	}
	list.itemOut = append(list.itemOut, out.Len())
	list.itemHead = append(list.itemHead, len(m.slugs))
	out.WriteString(outlineClose)
	m.listParts[index] = list
	return true
}

// outSpan returns where a region's part of body starts and ends, and the
// indexes of its first heading with an anchor and of the first after it.
func (m *outlineModel) outSpan(reg region) (int, int, int, int) {
	if reg.isList() {
		list := m.listParts[reg.list]
		return list.itemOut[reg.first-1], list.itemOut[reg.last+2], list.itemHead[reg.first-1], list.itemHead[reg.last+2]
	}
	return m.groupOut[reg.first], m.groupOut[reg.last+1], m.groupHead[reg.first], m.groupHead[reg.last+1]
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
	fmt.Fprintf(&key, "%d:%d:%d|", reg.list, reg.first, reg.last)
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
			outcome.loosens = m.docLayout.lists[reg.list].loosens(reg, outcome.tight)
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
// outline of its blocks, or items, and finds whether the slugs of their
// headings are those of headings [from, to) of the original.
func (m *outlineModel) outline(outcome *outlineOutcome, reg region, from, to int) {
	text := outcome.source
	end := len(text)
	if !reg.isList() && reg.last != len(m.bounds)-2 {
		end -= len(regionSentinel)
	}
	verifier := m.verifier
	m.work.parsed += 1 + len(text)
	snapshot, err := verifier.parser.ParseWithReferences(verifier.ctx, verifier.file.Path,
		append([]byte{'\n'}, text...), m.refIndex)
	if err != nil {
		outcome.undecided = true
		return
	}
	// The blank line before the region shifts its offsets by one. A block
	// without a span of its own cannot be placed before or after the
	// sentinel.
	var nodes []*mdast.Node
	for child := snapshot.Root.FirstChild; child != nil; child = child.Next {
		rng := child.SourceRange()
		if rng.IsEmpty() {
			outcome.undecided = true
			return
		}
		if rng.StartOffset < 1+end {
			nodes = append(nodes, child)
		}
	}
	if reg.isList() {
		if len(nodes) != 1 || nodes[0].Kind != mdast.NodeList {
			outcome.undecided = true
			return
		}
		var items []*mdast.Node
		for item := nodes[0].FirstChild; item != nil; item = item.Next {
			items = append(items, item)
		}
		nodes = items
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
// the text of the original's anchors item.
//
// Regions are taken in order while the anchor counts before each are the
// original's. A region that keeps its headings' slugs keeps their anchors
// and the counts. Otherwise its anchors are replayed from the counts before
// it (see anchorHistory). When there are as many as before and they differ,
// the anchors at those places of the list differ, so the lists do, and so
// does their text, since no anchor holds the separator. When they are the
// same and leave the counts as they were, the next region is taken.
// Anything else, a region that adds or removes headings or changes the
// counts, has the anchors of the whole edited document worked out.
func (m *outlineModel) anchorsKept(regions []region, outcomes []*outlineOutcome) bool {
	for idx, outcome := range outcomes {
		if outcome.slugsSame {
			continue
		}
		_, _, from, to := m.outSpan(regions[idx])
		anchors, counts := m.history.replay(outcome.slugs, from)
		switch {
		case len(anchors) != to-from:
			return m.anchorsRebuilt(regions, outcomes)
		case !slices.Equal(anchors, m.history.anchors[from:to]):
			return false
		case !m.history.sameCounts(counts, from, to):
			return m.anchorsRebuilt(regions, outcomes)
		}
	}
	return true
}

// anchorsRebuilt reports whether the anchors that the slugs of the edited
// document give, read in order, join to the text of the original's anchors
// item.
func (m *outlineModel) anchorsRebuilt(regions []region, outcomes []*outlineOutcome) bool {
	var slugs []string
	next := 0
	for idx, reg := range regions {
		_, _, from, to := m.outSpan(reg)
		slugs = append(slugs, m.slugs[next:from]...)
		if outcomes[idx].slugsSame {
			slugs = append(slugs, m.slugs[from:to]...)
		} else {
			slugs = append(slugs, outcomes[idx].slugs...)
		}
		next = to
	}
	slugs = append(slugs, m.slugs[next:]...)
	return strings.Join(recordAnchors(slugs).anchors, "\x00") == m.anchors
}
