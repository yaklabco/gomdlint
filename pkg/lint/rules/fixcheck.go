package rules

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// Fix safety check.
//
// A fix is offered only when it changes the document the way its rule
// intends and in no other way. Rules offer their fixes to a fixChecker,
// which keeps those that are safe each on its own and all together.
//
// The document check parses the whole document with the fixes applied, with
// the document's flavor, and compares it with the document as it is: its
// front matter, and its blocks, summarized by kind, attributes (heading
// level and style, list type and tightness, code fence info, table
// alignment), nesting and rendered text, with inline markup, the
// destinations and titles links resolve to, decoded escapes and character
// references, and raw HTML byte for byte. The blocks must match, except for
// each fix's target block, which must become what its rule says.
//
// Heading anchors are not emulated. When the document holds anything that
// could be a fragment reference, a fix may not change what any heading's
// anchor is made of, nor add or remove a heading (see anchorKey).
//
// Each fix is first checked on its own. When its rule shows that it leaves
// link labels alone and the fix cannot reach the front matter, that check
// parses only the blocks around the fix (fixesKeepDocument), so that a long
// document with many fixes is not parsed whole for each of them.
// Other fixes are checked on their own against the whole document. Then the
// fixes that pass are checked together against the whole document, and
// fixes are dropped until those left pass together.

// fixIntent describes the change a fix is meant to make. The zero value
// allows no change: every block keeps its kind, attributes, nesting and
// text.
type fixIntent struct {
	// target is the leaf block (paragraph, heading, table cell, code block,
	// HTML block or thematic break) whose summary may change.
	target *mdast.Node
	// into lists the blocks that target becomes, in order. Empty keeps
	// target as it is.
	into []blockWant
	// local is true when the rule shows that the fix leaves link labels
	// alone, so that, unless it can reach the front matter, its effects
	// stay among the blocks around it, but for the headings' anchors.
	local bool
}

// withLocal returns intent with local set.
func (i fixIntent) withLocal(local bool) fixIntent {
	i.local = local
	return i
}

// blockWant is one block that a fix's target becomes.
type blockWant struct {
	// keep gives the block the target's kind and attributes; otherwise it
	// has kind and attrs.
	keep  bool
	kind  mdast.NodeKind
	attrs string
	// from is the part of the target's source whose rendered text the block
	// holds, without line breaks and whitespace at either end.
	from mdast.SourceRange
	// subs change what parts of that source render as.
	subs []textSub
}

// textSub replaces the rendering of a source range of the document with
// text. An empty range inserts text where it lies.
type textSub struct {
	rng  mdast.SourceRange
	text string
}

// changeText returns the intent of a fix that keeps target's kind and
// attributes and changes its text by subs.
func changeText(target *mdast.Node, subs ...textSub) fixIntent {
	return fixIntent{target: target, into: []blockWant{{keep: true, from: target.SourceRange(), subs: subs}}}
}

// becomes returns the intent of a fix that turns target into a block of kind
// with attrs and its text changed by subs.
func becomes(target *mdast.Node, kind mdast.NodeKind, attrs string, subs ...textSub) fixIntent {
	return fixIntent{target: target, into: []blockWant{{kind: kind, attrs: attrs, from: target.SourceRange(), subs: subs}}}
}

// headingAttrs returns the summary attributes of a heading.
func headingAttrs(level int, style HeadingStyle) string {
	return fmt.Sprintf("h%d %s", level, style)
}

// fixChecker collects the fixes that a rule offers and keeps those that are
// safe each on its own and all together.
type fixChecker struct {
	ctx     *lint.RuleContext
	pending []pendingFix
	// before is the outline of the document as it is, parsed when first
	// needed; ready reports whether that parse worked, and fragments whether
	// the document holds anything that could be a fragment reference.
	before    *docOutline
	ready     bool
	fragments bool
	// children caches the block children of containers for window searches.
	children blockChildren
}

// blockKey identifies a block by its kind and where it starts.
type blockKey struct {
	kind  mdast.NodeKind
	start int
}

// pendingFix is a fix that a fixChecker has yet to check.
type pendingFix struct {
	edits  []fix.TextEdit
	intent fixIntent
}

// newFixChecker returns a fixChecker for a rule's run.
func newFixChecker(ctx *lint.RuleContext) *fixChecker {
	return &fixChecker{ctx: ctx, children: blockChildren{}}
}

// offer records a fix and returns builder, so that a diagnostic carries its
// edits until filter checks them.
func (c *fixChecker) offer(builder *fix.EditBuilder, intent fixIntent) *fix.EditBuilder {
	if builder == nil || len(builder.Edits) == 0 {
		return nil
	}
	c.pending = append(c.pending, pendingFix{edits: slices.Clone(builder.Edits), intent: intent})
	return builder
}

// filter removes from diags the fixes that are not safe on their own or
// together with the others. When the run was cancelled, it removes them all.
func (c *fixChecker) filter(diags []lint.Diagnostic) []lint.Diagnostic {
	safe := make(map[string]bool, len(c.pending))
	if !c.ctx.Cancelled() {
		var alone []int
		for idx, pending := range c.pending {
			if c.safeAlone(pending) {
				alone = append(alone, idx)
			}
		}
		for _, idx := range c.safeTogether(alone) {
			safe[editsKey(c.pending[idx].edits)] = true
		}
	}
	for idx := range diags {
		if len(diags[idx].FixEdits) > 0 && !safe[editsKey(diags[idx].FixEdits)] {
			diags[idx].FixEdits = nil
		}
	}
	return diags
}

// safeAlone checks a fix on its own: among the blocks around it, with the
// document's heading anchors, when its rule shows it local and it cannot
// reach the front matter, and against the whole document otherwise.
func (c *fixChecker) safeAlone(pending pendingFix) bool {
	if !pending.intent.local || touchesFrontMatter(c.ctx.File, pending.edits) {
		return c.documentKept(pending.edits, []fixIntent{pending.intent})
	}
	return c.localKept(pending)
}

// localKept reports whether a local fix changes the blocks around it only as
// its intent says, and leaves the headings' anchors alone when the document
// could refer to them.
func (c *fixChecker) localKept(pending pendingFix) bool {
	window, ok := checkWindow(c.ctx, pending.edits, []fixIntent{pending.intent}, c.children)
	if !ok || !slices.EqualFunc(window.want, window.after, sameBlock) {
		return false
	}
	// The rest of the document is as it was, so its anchors stay the same
	// when the window's headings make the same anchor keys.
	return slices.Equal(anchorKeys(window.before), anchorKeys(window.after)) || !c.mayReferToAnchors()
}

// mayReferToAnchors reports whether the document holds anything that could
// be a fragment reference to a heading anchor.
func (c *fixChecker) mayReferToAnchors() bool {
	_, ok := c.outline()
	return !ok || c.fragments
}

// safeTogether returns the fixes among candidates that the document check
// accepts together, in document order: all of them when it accepts them
// all, and otherwise those it accepts as it adds halves of them, down to
// single fixes, to those it has accepted, so that a fix which is only safe
// without another is left out.
func (c *fixChecker) safeTogether(candidates []int) []int {
	order := slices.Clone(candidates)
	sort.SliceStable(order, func(a, b int) bool {
		return c.pending[order[a]].edits[0].StartOffset < c.pending[order[b]].edits[0].StartOffset
	})
	var accepted []int
	var add func(group []int)
	add = func(group []int) {
		if len(group) == 0 {
			return
		}
		trial := append(slices.Clone(accepted), group...)
		var edits []fix.TextEdit
		intents := make([]fixIntent, 0, len(trial))
		for _, idx := range trial {
			edits = append(edits, c.pending[idx].edits...)
			intents = append(intents, c.pending[idx].intent)
		}
		if c.documentKept(edits, intents) {
			accepted = trial
			return
		}
		if len(group) > 1 {
			half := len(group) / 2
			add(group[:half])
			add(group[half:])
		}
	}
	add(order)
	return accepted
}

// outline returns the outline of the document as it is, parsed again with
// the flavor the checks use. ok is false when that parse fails.
func (c *fixChecker) outline() (docOutline, bool) {
	if c.before == nil {
		c.before = &docOutline{}
		if doc, ok := reparse(c.ctx, c.ctx.File.Content); ok {
			*c.before = outlineOf(doc)
			c.ready = true
			c.fragments = c.before.fragments || definitionsMayReferToAnchors(c.ctx)
		}
	}
	return *c.before, c.ready
}

// documentKept reports whether applying edits changes the whole document
// only as the intents say: the same front matter, the same blocks except
// for the intents' targets, and the same heading anchors when the document
// could refer to them.
func (c *fixChecker) documentKept(edits []fix.TextEdit, intents []fixIntent) bool {
	before, ok := c.outline()
	if !ok || len(edits) == 0 {
		return false
	}
	content, ok := applyShiftedEdits(c.ctx.File.Content, edits, 0)
	if !ok {
		return false
	}
	doc, ok := reparse(c.ctx, content)
	if !ok {
		return false
	}
	after := outlineOf(doc)
	if after.frontMatter != before.frontMatter {
		return false
	}
	want, ok := expectedBlocks(before.blocks, intents)
	if !ok || !slices.EqualFunc(want, after.blocks, sameBlock) {
		return false
	}
	return !c.fragments || slices.Equal(anchorKeys(before.blocks), anchorKeys(after.blocks))
}

// editsKey identifies a fix by its edits.
func editsKey(edits []fix.TextEdit) string {
	var key strings.Builder
	for _, edit := range edits {
		fmt.Fprintf(&key, "%d:%d:%q;", edit.StartOffset, edit.EndOffset, edit.NewText)
	}
	return key.String()
}

// fixesKeepDocument reports whether applying edits changes the blocks around
// them only as the intents say. It parses only those blocks, so it is for
// fixes whose effects stay there: front matter, reference definitions and
// heading anchors elsewhere are out of its sight.
func fixesKeepDocument(ctx *lint.RuleContext, edits []fix.TextEdit, intents []fixIntent) bool {
	window, ok := checkWindow(ctx, edits, intents, nil)
	return ok && slices.EqualFunc(window.want, window.after, sameBlock)
}

// windowCheck holds the summaries of the blocks around fixes: before the
// fixes, as the fixes' intents say they should be, and after the fixes;
// start and end bound those blocks in the document.
type windowCheck struct {
	before, want, after []blockSummary
	start, end          int
}

// checkWindow parses the blocks around edits before and after applying them.
// ok is false when the edits do not apply or the intents cannot be met.
// cache, which may be nil, keeps the block children it looks up.
func checkWindow(
	ctx *lint.RuleContext,
	edits []fix.TextEdit,
	intents []fixIntent,
	cache blockChildren,
) (windowCheck, bool) {
	file := ctx.File
	if file == nil || file.Root == nil || len(edits) == 0 {
		return windowCheck{}, false
	}
	low, high := edits[0].StartOffset, edits[0].EndOffset
	for _, edit := range edits[1:] {
		low, high = min(low, edit.StartOffset), max(high, edit.EndOffset)
	}
	for _, intent := range intents {
		if intent.target != nil {
			rng := intent.target.SourceRange()
			low, high = min(low, rng.StartOffset), max(high, rng.EndOffset)
		}
	}
	if low < 0 || high > len(file.Content) {
		return windowCheck{}, false
	}

	// Parsed on their own, the blocks of a window that does not start the
	// document must not be taken for front matter, so a blank line goes
	// first.
	start, end, looseDepth := fixWindow(file, low, high, cache)
	prefix := ""
	if start > 0 {
		prefix = "\n"
	}
	shift := len(prefix) - start
	before := append([]byte(prefix), file.Content[start:end]...)
	after, ok := applyShiftedEdits(before, edits, shift)
	if !ok {
		return windowCheck{}, false
	}
	beforeDoc, ok := reparse(ctx, before)
	if !ok {
		return windowCheck{}, false
	}
	afterDoc, ok := reparse(ctx, after)
	if !ok {
		return windowCheck{}, false
	}
	window := windowCheck{
		before: summarizeBlocks(beforeDoc, -shift),
		after:  summarizeBlocks(afterDoc, 0),
		start:  start,
		end:    end,
	}
	if looseDepth >= 0 {
		loosenLists(window.before, looseDepth)
		loosenLists(window.after, looseDepth)
	}
	window.want, ok = expectedBlocks(window.before, intents)
	return window, ok
}

// touchesFrontMatter reports whether edits could change the document's front
// matter: they touch its first line, or that line could open front matter
// and a line they edit could close it, before or after the edit.
func touchesFrontMatter(file *mdast.FileSnapshot, edits []fix.TextEdit) bool {
	if len(file.Lines) == 0 {
		return false
	}
	for _, edit := range edits {
		if edit.StartOffset < file.Lines[0].EndOffset {
			return true
		}
	}
	opener := strings.TrimRight(strings.TrimPrefix(string(lint.RawLineContent(file, 1)), mdast.ByteOrderMark), " \t\r")
	if opener != "---" && opener != "+++" && opener != "{" {
		return false
	}
	closes := func(text string) bool {
		for _, line := range strings.Split(text, "\n") {
			switch strings.TrimRight(line, " \t\r") {
			case "---", "...", "+++", "}":
				return true
			}
		}
		return false
	}
	for _, edit := range edits {
		lineStart := lineStartOf(file, edit.StartOffset)
		lineEnd := lineEndOf(file, edit.EndOffset)
		if edit.EndOffset < len(file.Content) {
			lineEnd = lineEndOf(file, edit.EndOffset+1)
		}
		edited := string(file.Content[lineStart:edit.StartOffset]) + edit.NewText + string(file.Content[edit.EndOffset:lineEnd])
		if closes(string(file.Content[lineStart:lineEnd])) || closes(edited) {
			return true
		}
	}
	return false
}

// loosenLists marks the lists at depth as loose. A window that holds some of
// the items of a loose list need not show that the list is loose, and none of
// the fixes removes the blank lines that make it so.
func loosenLists(blocks []blockSummary, depth int) {
	for idx := range blocks {
		if blocks[idx].kind == mdast.NodeList && blocks[idx].depth == depth {
			blocks[idx].attrs = strings.Replace(blocks[idx].attrs, "tight=true", "tight=false", 1)
		}
	}
}

// leafBlockAt returns the innermost block of root whose source holds
// offset, or nil.
func leafBlockAt(root *mdast.Node, offset int) *mdast.Node {
	var found *mdast.Node
	for node := root; node != nil; {
		var next *mdast.Node
		for child := node.FirstChild; child != nil; child = child.Next {
			rng := child.SourceRange()
			if rng.StartOffset > offset {
				break // Children are in document order.
			}
			if !child.IsInline() && offset < rng.EndOffset {
				next = child
				break
			}
		}
		if next == nil {
			break
		}
		found, node = next, next
	}
	return found
}

// fixWindow returns the part of the document that fixesKeepDocument parses
// for a change from low to high. It holds the top-level blocks that the
// change touches, with one more block on either side, so that a change in
// how a block starts or ends shows, from the start of the first block's line
// to the end of the last block's line; it starts at the start of the
// document or ends at its end when the change reaches the first or last
// block.
//
// While the change lies inside one list or blockquote that is a top-level
// block or a block of a blockquote, and away from its first and last item or
// block, the window narrows to the items or blocks of that container in the
// same way: a long list is then not parsed whole for each fix in it. Lines
// keep their blockquote markers, so the window parses with the same nesting.
// looseDepth is the depth of the list whose items the window holds when that
// list is loose, and -1 otherwise.
func fixWindow(file *mdast.FileSnapshot, low, high int, cache blockChildren) (int, int, int) {
	type level struct {
		container   *mdast.Node
		depth       int
		children    []*mdast.Node
		first, last int
	}
	children, first, last := blockSlice(cache.of(file.Root), low, high)
	levels := []level{{container: file.Root, children: children, first: first, last: last}}
	for {
		current := levels[len(levels)-1]
		if current.container.Kind == mdast.NodeList || current.first != current.last ||
			current.first < 0 || current.first >= len(current.children) {
			break
		}
		child := current.children[current.first]
		if child.Kind != mdast.NodeList && child.Kind != mdast.NodeBlockquote {
			break
		}
		inner, innerFirst, innerLast := blockSlice(cache.of(child), low, high)
		levels = append(levels, level{
			container: child, depth: current.depth + 1, children: inner, first: innerFirst, last: innerLast,
		})
	}
	chosen := levels[0]
	for _, candidate := range levels[1:] {
		if min(candidate.first, candidate.last) >= 1 && max(candidate.first, candidate.last) <= len(candidate.children)-2 {
			chosen = candidate
		}
	}
	container, depth := chosen.container, chosen.depth
	children, first, last = chosen.children, chosen.first, chosen.last
	if len(children) == 0 {
		return 0, len(file.Content), -1
	}

	first, last = max(min(first, last)-1, 0), min(max(first, last)+1, len(children)-1)
	start := 0
	if first > 0 || container != file.Root {
		start = lineStartOf(file, children[first].SourceRange().StartOffset)
	}
	end := len(file.Content)
	if last < len(children)-1 || container != file.Root {
		end = lineEndOf(file, children[last].SourceRange().EndOffset)
	}
	looseDepth := -1
	if container.Kind == mdast.NodeList && !lint.IsTightList(container) {
		looseDepth = depth - 1
	}
	return min(start, lineStartOf(file, low)), max(end, lineEndOf(file, high)), looseDepth
}

// blockChildren caches the block children of containers. A nil cache keeps
// nothing.
type blockChildren map[*mdast.Node][]*mdast.Node

// of returns the block children of container.
func (cache blockChildren) of(container *mdast.Node) []*mdast.Node {
	if children, found := cache[container]; found {
		return children
	}
	var children []*mdast.Node
	for child := container.FirstChild; child != nil; child = child.Next {
		if !child.IsInline() {
			children = append(children, child)
		}
	}
	if cache != nil {
		cache[container] = children
	}
	return children
}

// blockSlice returns children, the block children of a container, with the
// indexes of the first one that ends at or after low and of the last one
// that starts at or before high. first is greater than last when the range
// lies between two children.
func blockSlice(children []*mdast.Node, low, high int) ([]*mdast.Node, int, int) {
	first := sort.Search(len(children), func(i int) bool { return children[i].SourceRange().EndOffset >= low })
	last := sort.Search(len(children), func(i int) bool { return children[i].SourceRange().StartOffset > high }) - 1
	return children, first, last
}

// lineStartOf returns the offset where the line holding offset starts.
func lineStartOf(file *mdast.FileSnapshot, offset int) int {
	if line := lineOf(file, offset); line != nil {
		return line.StartOffset
	}
	return min(max(offset, 0), len(file.Content))
}

// lineEndOf returns the offset after the line ending of the line holding
// offset, or after the line that ends just before it.
func lineEndOf(file *mdast.FileSnapshot, offset int) int {
	if offset > 0 {
		offset--
	}
	if line := lineOf(file, offset); line != nil {
		return line.EndOffset
	}
	return len(file.Content)
}

// applyShiftedEdits applies edits, whose offsets are shift bytes off from
// text's, to a copy of text.
func applyShiftedEdits(text []byte, edits []fix.TextEdit, shift int) ([]byte, bool) {
	shifted := make([]fix.TextEdit, 0, len(edits))
	for _, edit := range edits {
		edit.StartOffset += shift
		edit.EndOffset += shift
		shifted = append(shifted, edit)
	}
	prepared, err := fix.PrepareEdits(shifted, len(text))
	if err != nil {
		return nil, false
	}
	return fix.ApplyEdits(slices.Clone(text), prepared), true
}

// parseFlavor returns the Markdown flavor the document was linted with.
func parseFlavor(ctx *lint.RuleContext) string {
	if ctx.Config != nil && ctx.Config.Flavor != "" {
		return string(ctx.Config.Flavor)
	}
	return string(config.FlavorCommonMark)
}

// reparse parses text on its own with the document's flavor.
func reparse(ctx *lint.RuleContext, text []byte) (*mdast.FileSnapshot, bool) {
	parseCtx := ctx.Ctx
	if parseCtx == nil {
		parseCtx = context.Background()
	}
	snapshot, err := gmparser.New(parseFlavor(ctx)).Parse(parseCtx, "", text)
	if err != nil || snapshot == nil || snapshot.Root == nil {
		return nil, false
	}
	return snapshot, true
}

// blockSummary is what the checks compare of a block.
type blockSummary struct {
	depth int
	kind  mdast.NodeKind
	attrs string
	// start is where the block starts in the document.
	start int
	atoms []textAtom
	// plain is a heading's text content, from which its anchor is made.
	plain string
	// origin is the index, among the summaries of the document as it is,
	// of the block this one is or comes from, or -1 for a heading that a fix
	// makes of another kind of block.
	origin int
}

// text returns the block's rendered text. Like CommonMark, it leaves out
// whitespace and line breaks at either end of inline content.
func (b blockSummary) text() string {
	var out strings.Builder
	for _, atom := range b.atoms {
		out.WriteString(atom.text)
	}
	if hasInlineContent(b.kind) {
		return trimRendered(out.String())
	}
	return out.String()
}

// hasInlineContent reports whether blocks of kind hold inline content.
func hasInlineContent(kind mdast.NodeKind) bool {
	return kind == mdast.NodeParagraph || kind == mdast.NodeHeading || kind == mdast.NodeTableCell
}

// sameBlock reports whether two summaries match.
func sameBlock(a, b blockSummary) bool {
	return a.depth == b.depth && a.kind == b.kind && a.attrs == b.attrs && a.text() == b.text()
}

// textAtom is a piece of a block's rendered text and the source it comes
// from.
type textAtom struct {
	text string
	rng  mdast.SourceRange
	// linear is true when text is a copy of the source in rng, so that each
	// byte has its own offset.
	linear bool
}

// summarizeBlocks lists the blocks of doc in document order. offset is
// added to source offsets, to map them to the document a window comes from.
func summarizeBlocks(doc *mdast.FileSnapshot, offset int) []blockSummary {
	var blocks []blockSummary
	var visit func(parent *mdast.Node, depth int)
	visit = func(parent *mdast.Node, depth int) {
		for node := parent.FirstChild; node != nil; node = node.Next {
			if node.IsInline() {
				continue
			}
			summary := blockSummary{
				depth:  depth,
				kind:   node.Kind,
				attrs:  blockAttrs(doc, node),
				start:  node.SourceRange().StartOffset + offset,
				origin: len(blocks),
			}
			if node.Kind == mdast.NodeHeading {
				summary.plain = headingPlainText(node)
			}
			render := &atomRenderer{doc: doc, offset: offset}
			switch {
			case hasInlineContent(node.Kind):
				render.inlines(node)
			case node.Kind == mdast.NodeCodeBlock:
				render.raw(node.ContentRange())
			case node.Kind == mdast.NodeHTMLBlock:
				render.raw(node.SourceRange())
			}
			summary.atoms = render.atoms
			blocks = append(blocks, summary)
			visit(node, depth+1)
		}
	}
	visit(doc.Root, 0)
	return blocks
}

// blockAttrs returns the attributes of a block that its summary compares.
func blockAttrs(doc *mdast.FileSnapshot, node *mdast.Node) string {
	block := node.Block
	switch {
	case node.Kind == mdast.NodeHeading:
		return headingAttrs(lint.HeadingLevel(node), detectHeadingStyle(doc, node))
	case block == nil:
		return ""
	case node.Kind == mdast.NodeList && block.List != nil:
		list := block.List
		return fmt.Sprintf("ordered=%t bullet=%q start=%d delimiter=%q tight=%t",
			list.Ordered, list.BulletMarker, list.StartNumber, list.Delimiter, list.Tight)
	case node.Kind == mdast.NodeCodeBlock && block.CodeBlock != nil:
		return fmt.Sprintf("indented=%t info=%q", block.CodeBlock.Indented, block.CodeBlock.Info)
	case block.Table != nil:
		return fmt.Sprintf("%v %t %v", block.Table.Alignments, block.Table.Header, block.Table.Alignment)
	default:
		return ""
	}
}

// atomRenderer renders the inline content of a block as atoms.
type atomRenderer struct {
	doc    *mdast.FileSnapshot
	offset int
	atoms  []textAtom
}

// token adds markup text that rng's source stands for.
func (r *atomRenderer) token(text string, start, end int) {
	r.atoms = append(r.atoms, textAtom{
		text: text,
		rng:  mdast.SourceRange{StartOffset: start + r.offset, EndOffset: end + r.offset},
	})
}

// raw adds the source in rng as it is.
func (r *atomRenderer) raw(rng mdast.SourceRange) {
	if rng.StartOffset < 0 || rng.EndOffset > len(r.doc.Content) || rng.IsEmpty() {
		return
	}
	r.atoms = append(r.atoms, textAtom{
		text:   string(r.doc.Content[rng.StartOffset:rng.EndOffset]),
		rng:    mdast.SourceRange{StartOffset: rng.StartOffset + r.offset, EndOffset: rng.EndOffset + r.offset},
		linear: true,
	})
}

// inlines renders the inline children of parent.
func (r *atomRenderer) inlines(parent *mdast.Node) {
	for node := parent.FirstChild; node != nil; node = node.Next {
		full, content := node.SourceRange(), node.ContentRange()
		switch node.Kind {
		case mdast.NodeText:
			r.text(node)
		case mdast.NodeSoftBreak:
			r.token("\n", full.StartOffset, full.EndOffset)
		case mdast.NodeHardBreak:
			r.token("\x00br\x00", full.StartOffset, full.EndOffset)
		case mdast.NodeCodeSpan:
			r.token("\x00code\x00", full.StartOffset, content.StartOffset)
			r.literal(node.Inline, content)
			r.token("\x00/code\x00", content.EndOffset, full.EndOffset)
		case mdast.NodeHTMLInline:
			r.token("\x00html\x00", full.StartOffset, full.StartOffset)
			r.raw(full)
			r.token("\x00/html\x00", full.EndOffset, full.EndOffset)
		case mdast.NodeTaskCheckbox:
			checked := node.Inline != nil && node.Inline.Checked
			r.token(fmt.Sprintf("\x00task %t\x00", checked), full.StartOffset, full.EndOffset)
		default:
			target := ""
			if node.Inline != nil && node.Inline.Link != nil {
				target = node.Inline.Link.Destination + "\x01" + node.Inline.Link.Title
			}
			r.token(markupToken(node.Kind, target, false), full.StartOffset, content.StartOffset)
			r.inlines(node)
			r.token(markupToken(node.Kind, "", true), content.EndOffset, full.EndOffset)
		}
	}
}

// markupToken returns the summary token that stands for the opening
// delimiter of inline markup of kind, such as emphasis or a link, or with
// closing for its closing delimiter. target holds a link's destination and
// title.
func markupToken(kind mdast.NodeKind, target string, closing bool) string {
	name := strings.ToLower(kind.String())
	switch {
	case closing:
		return "\x00/" + name + "\x00"
	case target != "":
		return "\x00" + name + " " + target + "\x00"
	default:
		return "\x00" + name + "\x00"
	}
}

// literal adds an inline node's text, which comes from the source in rng.
func (r *atomRenderer) literal(inline *mdast.InlineAttrs, rng mdast.SourceRange) {
	if inline == nil {
		return
	}
	if rng.StartOffset >= 0 && rng.EndOffset <= len(r.doc.Content) &&
		bytes.Equal(r.doc.Content[rng.StartOffset:rng.EndOffset], inline.Text) {
		r.raw(rng)
		return
	}
	r.token(string(inline.Text), rng.StartOffset, rng.EndOffset)
}

// characterReferenceStart matches a character reference at the start of
// text.
var characterReferenceStart = regexp.MustCompile(`^` + characterReferencePattern.String())

// text adds a text node, with backslash escapes and character references
// decoded, as they render.
func (r *atomRenderer) text(node *mdast.Node) {
	start, ok := textNodeStart(r.doc, node)
	if !ok {
		r.literal(node.Inline, node.SourceRange())
		return
	}
	text := node.Inline.Text
	runStart := 0
	flush := func(end int) {
		if end > runStart {
			r.raw(mdast.SourceRange{StartOffset: start + runStart, EndOffset: start + end})
		}
	}
	for idx := 0; idx < len(text); {
		size := 0
		decoded := ""
		switch {
		case text[idx] == '\\' && idx+1 < len(text) && isASCIIPunctuation(text[idx+1]):
			size, decoded = 2, string(text[idx+1])
		case text[idx] == '&':
			if loc := characterReferenceStart.FindIndex(text[idx:]); loc != nil {
				reference := string(text[idx : idx+loc[1]])
				if unescaped := html.UnescapeString(reference); unescaped != reference {
					size, decoded = loc[1], unescaped
				}
			}
		}
		if size == 0 {
			idx++
			continue
		}
		flush(idx)
		r.token(decoded, start+idx, start+idx+size)
		idx += size
		runStart = idx
	}
	flush(len(text))
}

// expectedBlocks returns the summaries that fixes with intents should
// leave, given the summaries before them. ok is false when an intent's target
// is not a leaf block among them, when two intents change one block in ways
// that cannot be merged, or when a block an intent makes would split an atom.
func expectedBlocks(before []blockSummary, intents []fixIntent) ([]blockSummary, bool) {
	index := make(map[blockKey]int, len(before))
	for idx, block := range before {
		if _, seen := index[blockKey{block.kind, block.start}]; !seen {
			index[blockKey{block.kind, block.start}] = idx
		}
	}
	plans := make(map[int][]blockWant)
	for _, intent := range intents {
		if intent.target == nil {
			continue
		}
		idx, found := index[blockKey{intent.target.Kind, intent.target.SourceRange().StartOffset}]
		if !found || (idx+1 < len(before) && before[idx+1].depth > before[idx].depth) {
			return nil, false
		}
		into, ok := mergeWants(plans[idx], intent.into)
		if !ok {
			return nil, false
		}
		plans[idx] = into
	}

	want := make([]blockSummary, 0, len(before))
	for idx, target := range before {
		into := plans[idx]
		if len(into) == 0 {
			want = append(want, target)
			continue
		}
		for _, block := range into {
			atoms, ok := atomsWithin(target.atoms, block.from)
			if !ok {
				return nil, false
			}
			// A piece of a split paragraph that holds no text makes no block.
			if !block.keep && block.kind == mdast.NodeParagraph && trimRendered(substitute(atoms, block.subs)) == "" {
				continue
			}
			summary := blockSummary{
				depth:  target.depth,
				kind:   block.kind,
				attrs:  block.attrs,
				atoms:  []textAtom{{text: substitute(atoms, block.subs)}},
				origin: -1,
			}
			if block.keep {
				summary.kind, summary.attrs = target.kind, target.attrs
			}
			if summary.kind == mdast.NodeHeading && target.kind == mdast.NodeHeading {
				summary.origin = target.origin
			}
			want = append(want, summary)
		}
	}
	return want, true
}

// mergeWants combines what two fixes make of one block: either may keep it
// as it is, two text changes that keep its kind merge their subs, and two
// splits of it into consecutive pieces merge by refineSplits. ok is false for
// anything else.
func mergeWants(earlier, later []blockWant) ([]blockWant, bool) {
	switch {
	case len(earlier) == 0:
		return later, true
	case len(later) == 0:
		return earlier, true
	case len(earlier) == 1 && len(later) == 1 && earlier[0].keep && later[0].keep && earlier[0].from == later[0].from:
		merged := earlier[0]
		merged.subs = append(slices.Clone(earlier[0].subs), later[0].subs...)
		return []blockWant{merged}, true
	default:
		return refineSplits(earlier, later)
	}
}

// refineSplits merges two splits of a block into consecutive pieces of its
// source, such as a paragraph around a line that a fix makes a heading. The
// merged split cuts the source at the boundaries of both, and each piece
// becomes the block that either split makes of it other than a paragraph,
// or a paragraph. ok is false when either list is not such a split, or when
// both make something other than a paragraph of one piece.
func refineSplits(first, second []blockWant) ([]blockWant, bool) {
	if !isSplit(first) || !isSplit(second) {
		return nil, false
	}
	var cuts []int
	for _, block := range slices.Concat(first, second) {
		cuts = append(cuts, block.from.StartOffset, block.from.EndOffset)
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	covering := func(split []blockWant, piece mdast.SourceRange) *blockWant {
		for idx := range split {
			if split[idx].from.StartOffset <= piece.StartOffset && piece.EndOffset <= split[idx].from.EndOffset {
				return &split[idx]
			}
		}
		return nil
	}
	var merged []blockWant
	for idx := 0; idx+1 < len(cuts); idx++ {
		piece := mdast.SourceRange{StartOffset: cuts[idx], EndOffset: cuts[idx+1]}
		block := blockWant{kind: mdast.NodeParagraph, from: piece}
		for _, made := range []*blockWant{covering(first, piece), covering(second, piece)} {
			if made == nil || made.kind == mdast.NodeParagraph {
				continue
			}
			if block.kind != mdast.NodeParagraph || made.from != piece {
				return nil, false
			}
			block = *made
		}
		if last := len(merged) - 1; last >= 0 && block.kind == mdast.NodeParagraph && merged[last].kind == mdast.NodeParagraph {
			merged[last].from.EndOffset = piece.EndOffset
			continue
		}
		merged = append(merged, block)
	}
	return merged, true
}

// isSplit reports whether blocks split a block into consecutive pieces of
// its source, none of which keeps the block as it is.
func isSplit(blocks []blockWant) bool {
	for idx, block := range blocks {
		if block.keep || (idx > 0 && blocks[idx-1].from.EndOffset != block.from.StartOffset) {
			return false
		}
	}
	return len(blocks) > 0
}

// atomsWithin returns the atoms, or parts of linear atoms, whose source lies
// within rng. ok is false when an atom that is not linear crosses an end of
// rng.
func atomsWithin(atoms []textAtom, rng mdast.SourceRange) ([]textAtom, bool) {
	var within []textAtom
	for _, atom := range atoms {
		switch {
		case atom.rng.StartOffset >= rng.StartOffset && atom.rng.EndOffset <= rng.EndOffset:
			within = append(within, atom)
		case atom.rng.EndOffset <= rng.StartOffset || atom.rng.StartOffset >= rng.EndOffset:
		case atom.linear:
			start := max(atom.rng.StartOffset, rng.StartOffset)
			end := min(atom.rng.EndOffset, rng.EndOffset)
			within = append(within, textAtom{
				text:   atom.text[start-atom.rng.StartOffset : end-atom.rng.StartOffset],
				rng:    mdast.SourceRange{StartOffset: start, EndOffset: end},
				linear: true,
			})
		default:
			return nil, false
		}
	}
	return within, true
}

// substitute renders atoms with subs applied: the text of a sub replaces the
// atoms, or bytes of linear atoms, whose source it overlaps, and a sub that
// overlaps none is inserted where its range starts.
func substitute(atoms []textAtom, subs []textSub) string {
	overlapsAtom := make([]bool, len(subs))
	for idx, sub := range subs {
		overlapsAtom[idx] = slices.ContainsFunc(atoms, func(atom textAtom) bool {
			return rangesOverlap(sub.rng, atom.rng)
		})
	}
	var out strings.Builder
	used := make([]bool, len(subs))
	use := func(idx int) {
		if !used[idx] {
			out.WriteString(subs[idx].text)
			used[idx] = true
		}
	}
	insertBefore := func(offset int) {
		for idx, sub := range subs {
			if !used[idx] && !overlapsAtom[idx] && sub.rng.StartOffset <= offset {
				use(idx)
			}
		}
	}
	covering := func(rng mdast.SourceRange) int {
		for idx, sub := range subs {
			if rangesOverlap(sub.rng, rng) {
				return idx
			}
		}
		return -1
	}
	for _, atom := range atoms {
		if !atom.linear {
			if idx := covering(atom.rng); idx >= 0 {
				use(idx)
				continue
			}
			insertBefore(atom.rng.StartOffset)
			out.WriteString(atom.text)
			continue
		}
		for pos := range len(atom.text) {
			offset := atom.rng.StartOffset + pos
			if idx := covering(mdast.SourceRange{StartOffset: offset, EndOffset: offset + 1}); idx >= 0 {
				use(idx)
				continue
			}
			insertBefore(offset)
			out.WriteByte(atom.text[pos])
		}
	}
	insertBefore(math.MaxInt)
	return out.String()
}

// docOutline is what the document check compares of a whole document.
type docOutline struct {
	// frontMatter is the source of the front matter, if any.
	frontMatter string
	blocks      []blockSummary
	// fragments is true when the document holds anything that could be a
	// fragment reference: a link or image whose destination holds "#", or
	// any raw HTML at all, comments included.
	fragments bool
}

// outlineOf returns the outline of doc.
func outlineOf(doc *mdast.FileSnapshot) docOutline {
	outline := docOutline{blocks: summarizeBlocks(doc, 0)}
	if lines := doc.FrontMatterLines; lines > 0 && lines <= len(doc.Lines) {
		outline.frontMatter = string(doc.Content[:doc.Lines[lines-1].EndOffset])
	}
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(doc.Root, func(node *mdast.Node) error {
		rng := node.SourceRange()
		if rng.StartOffset < 0 || rng.EndOffset > len(doc.Content) {
			return nil
		}
		source := doc.Content[rng.StartOffset:rng.EndOffset]
		switch node.Kind {
		case mdast.NodeLink, mdast.NodeImage:
			if strings.Contains(lint.LinkDestination(node), "#") || mayHoldHash(source) {
				outline.fragments = true
			}
		case mdast.NodeHTMLBlock, mdast.NodeHTMLInline:
			// Raw HTML can run on past the Markdown block that holds it, as a
			// tag that a blank line cuts in two does, so any of it may refer
			// to an anchor, wherever a hash is.
			outline.fragments = true
		case mdast.NodeParagraph, mdast.NodeHeading, mdast.NodeTableCell:
			// So may raw HTML that the parser did not read as such, such as
			// an unterminated tag in text.
			if textHasRawHTML(doc.Content, node) {
				outline.fragments = true
			}
		}
		return nil
	})
	return outline
}

// textHasRawHTML reports whether the inline content of container holds, in
// its text, a "<" that could start raw HTML: one followed by a letter, "/",
// "!" or "?", outside code spans, autolinks and link destinations, and not
// escaped by a backslash.
func textHasRawHTML(content []byte, container *mdast.Node) bool {
	rng := container.SourceRange()
	if rng.StartOffset < 0 || rng.EndOffset > len(content) {
		return false
	}
	var parsed []mdast.SourceRange
	//nolint:errcheck // Walk visitor never returns error.
	_ = mdast.Walk(container, func(node *mdast.Node) error {
		full, inner := node.SourceRange(), node.ContentRange()
		switch {
		case node.Kind == mdast.NodeCodeSpan, node.Kind == mdast.NodeHTMLInline, isAutolink(node):
			parsed = append(parsed, full)
		case node.Kind == mdast.NodeLink || node.Kind == mdast.NodeImage:
			// The destination and title follow the link text.
			parsed = append(parsed, mdast.SourceRange{StartOffset: inner.EndOffset, EndOffset: full.EndOffset})
		}
		return nil
	})
	slices.SortFunc(parsed, func(a, b mdast.SourceRange) int { return a.StartOffset - b.StartOffset })
	text := content[:rng.EndOffset]
	for idx := rng.StartOffset; idx+1 < len(text); idx++ {
		if text[idx] != '<' || escapedAt(text, rng.StartOffset, idx) || insideSorted(parsed, idx) {
			continue
		}
		if next := text[idx+1]; isASCIILetter(next) || next == '/' || next == '!' || next == '?' {
			return true
		}
	}
	return false
}

// insideSorted reports whether offset lies in one of ranges, which are
// sorted by start and may nest.
func insideSorted(ranges []mdast.SourceRange, offset int) bool {
	for _, rng := range ranges {
		if rng.StartOffset > offset {
			return false
		}
		if offset < rng.EndOffset {
			return true
		}
	}
	return false
}

// mayHoldHash reports whether text holds "#", or a character reference that
// could stand for it, without decoding anything.
func mayHoldHash(text []byte) bool {
	return bytes.ContainsRune(text, '#') || bytes.Contains(text, []byte("&#")) || bytes.Contains(text, []byte("&num;"))
}

// definitionsMayReferToAnchors reports whether a reference definition of
// the document has a destination that could hold a fragment.
func definitionsMayReferToAnchors(ctx *lint.RuleContext) bool {
	for _, definition := range ctx.RefContext().AllDefinitions {
		if mayHoldHash([]byte(definition.Destination)) {
			return true
		}
	}
	return false
}

// anchorKeys returns the anchor keys of the headings among blocks, in order.
func anchorKeys(blocks []blockSummary) []string {
	var keys []string
	for _, block := range blocks {
		if block.kind == mdast.NodeHeading {
			keys = append(keys, anchorKey(block.plain))
		}
	}
	return keys
}

// anchorKey returns what a heading's anchor is made of: the letters, marks,
// numbers, connector punctuation, hyphens and spaces of its text, in order,
// with ASCII letters lowercased and other letters as they are. GitHub and
// others make an anchor by lowercasing those and turning spaces into
// hyphens, so headings whose keys are the same in order get the same
// anchors, duplicate numbering included, however other letters lowercase.
func anchorKey(plain string) string {
	var key strings.Builder
	for _, char := range plain {
		switch {
		case char >= 'A' && char <= 'Z':
			key.WriteRune(char + ('a' - 'A'))
		case char == ' ' || char == '-' || unicode.In(char, unicode.L, unicode.M, unicode.N, unicode.Pc):
			key.WriteRune(char)
		}
	}
	return key.String()
}

// headingPlainText returns a heading's text content, as GitHub reads it to
// make its anchor: text with escapes and character references decoded, code
// span content, and the text inside emphasis and links, without images and
// raw HTML.
func headingPlainText(heading *mdast.Node) string {
	var text strings.Builder
	var visit func(parent *mdast.Node)
	visit = func(parent *mdast.Node) {
		for node := parent.FirstChild; node != nil; node = node.Next {
			switch node.Kind {
			case mdast.NodeText:
				if node.Inline != nil {
					text.WriteString(decodeInline(node.Inline.Text))
				}
			case mdast.NodeCodeSpan:
				if node.Inline != nil {
					text.Write(node.Inline.Text)
				}
			case mdast.NodeSoftBreak, mdast.NodeHardBreak:
				text.WriteByte(' ')
			case mdast.NodeImage, mdast.NodeHTMLInline, mdast.NodeTaskCheckbox:
			default:
				visit(node)
			}
		}
	}
	visit(heading)
	return strings.TrimSpace(text.String())
}

// decodeInline returns raw inline text with backslash escapes and character
// references decoded.
func decodeInline(text []byte) string {
	var decoded strings.Builder
	for idx := 0; idx < len(text); {
		if text[idx] == '\\' && idx+1 < len(text) && isASCIIPunctuation(text[idx+1]) {
			decoded.WriteByte(text[idx+1])
			idx += 2
			continue
		}
		if text[idx] == '&' {
			if loc := characterReferenceStart.FindIndex(text[idx:]); loc != nil {
				decoded.WriteString(html.UnescapeString(string(text[idx : idx+loc[1]])))
				idx += loc[1]
				continue
			}
		}
		decoded.WriteByte(text[idx])
		idx++
	}
	return decoded.String()
}

// rangesOverlap reports whether two ranges share a byte.
func rangesOverlap(a, b mdast.SourceRange) bool {
	return a.StartOffset < b.EndOffset && b.StartOffset < a.EndOffset
}

// trimRendered removes whitespace and line breaks at both ends of rendered
// inline content.
func trimRendered(text string) string {
	const hardBreak = "\x00br\x00"
	for {
		trimmed := strings.Trim(text, " \t\n")
		trimmed = strings.TrimPrefix(strings.TrimSuffix(trimmed, hardBreak), hardBreak)
		if trimmed == text {
			return text
		}
		text = trimmed
	}
}
