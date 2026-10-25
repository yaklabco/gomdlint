package rules

import (
	"context"
	"slices"

	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// itemWindowMin is the size in bytes of a window from which localKept first
// tries the window of the list item that holds a fix. A smaller window parses
// quickly, and the item window needs the region layout, a parse of the whole
// document.
const itemWindowMin = 4096

// regionLayout returns the region layout of the document as the checks read
// it, built on first use, or nil when region checks cannot be used for it.
func (c *fixChecker) regionLayout() *docLayout {
	if !c.layoutBuilt {
		c.layoutBuilt = true
		c.layout = newDocLayoutFlags(recordingMarkdown(parseFlavor(c.ctx)), c.content, true)
		c.parser = gmparser.New(parseFlavor(c.ctx))
	}
	return c.layout
}

// itemWindow checks edits with intents as checkWindow does, but on the
// groups of the list item that holds them, parsed after the item's opener
// with the document's definitions, instead of on the top-level blocks around
// them. It decides only when the item region of the edits holds (see Item
// regions in fix_regions.go), with the definitions it had: then the
// document's other blocks parse as before but for the tightness of the
// item's list, which a blank line in the item can change, and which
// itemWindow checks; ok is false when the list changes tightness. A fix that
// changes a list's tightness changes the document, and no intent allows it.
// decided is false when the item window cannot stand for the document, and
// checkWindow decides.
func (c *fixChecker) itemWindow(edits []fix.TextEdit, intents []fixIntent) (windowCheck, bool, bool) {
	layout := c.regionLayout()
	if layout == nil {
		return windowCheck{}, false, false
	}
	prepared, err := fix.PrepareEdits(checkedEdits(edits), len(c.content))
	if err != nil || len(prepared) == 0 {
		return windowCheck{}, false, false
	}
	placed, early, ok := layout.placeEdits(prepared)
	if !ok || early {
		return windowCheck{}, false, false
	}
	// The region holds the lines the edits change and the intents' targets.
	cover := placedEdit{firstLine: placed[0].firstLine, lastLine: placed[0].lastLine}
	for _, edit := range placed[1:] {
		cover.firstLine, cover.lastLine = min(cover.firstLine, edit.firstLine), max(cover.lastLine, edit.lastLine)
	}
	for _, intent := range intents {
		if intent.target == nil {
			continue
		}
		rng := intent.target.SourceRange()
		start, end := rng.StartOffset-layout.base, max(rng.EndOffset-1, rng.StartOffset)-layout.base
		if start < 0 || end > len(layout.source) {
			return windowCheck{}, false, false
		}
		cover.firstLine = min(cover.firstLine, lineStartIn(layout.source, start))
		cover.lastLine = max(cover.lastLine, lineStartIn(layout.source, end))
	}
	plain := make([]fix.TextEdit, len(placed))
	for idx, edit := range placed {
		plain[idx] = edit.edit
	}
	reg := layout.fitted(layout.regionFor(cover))
	var parse regionParse
	for {
		if reg.kind != itemRegion {
			return windowCheck{}, false, false
		}
		if from, to := layout.editSpan(reg); len(regionEdits(from, to, placed)) != len(placed) {
			return windowCheck{}, false, false
		}
		parse = layout.parseRegion(reg, plain)
		if parse.undecided {
			return windowCheck{}, false, false
		}
		if parse.holds {
			break
		}
		grown := layout.grow(reg)
		if grown == reg {
			return windowCheck{}, false, false
		}
		reg = grown
	}
	if !parse.refsSame {
		return windowCheck{}, false, false
	}
	item := layout.items[reg.at]
	tight := layout.listsTightAfter([]region{reg}, func(int) bool { return layout.loosensList(reg, parse) })
	if tight[item.list] != layout.lists[item.list].tightWith(0, false) {
		return windowCheck{}, true, false
	}
	from, to := layout.span(reg)
	opener := layout.itemOpener(reg.at, reg.first)
	before := slices.Concat([]byte{'\n'}, opener.text, layout.source[from:to], layout.source[to:nextLineStart(layout.source, to)])
	beforeNodes, beforeDoc, ok := c.windowNodes(layout, before, reg.at, 1+len(opener.text)+to-from, opener.skip)
	if !ok {
		return windowCheck{}, false, false
	}
	afterNodes, afterDoc, ok := c.windowNodes(layout, slices.Concat([]byte{'\n'}, parse.source), reg.at, 1+parse.end, parse.skip)
	if !ok {
		return windowCheck{}, false, false
	}
	window := windowCheck{
		before: summarizeNodes(beforeDoc, beforeNodes, layout.base+from-1-len(opener.text)),
		after:  summarizeNodes(afterDoc, afterNodes, 0),
		start:  layout.base + from,
		end:    layout.base + to,
	}
	window.want, ok = expectedBlocks(window.before, intents)
	if !ok {
		return windowCheck{}, false, false
	}
	return window, true, true
}

// windowNodes parses the text of an item window with the document's parser
// and definitions, and returns the blocks of the item at index at that start
// before end, after the skip blocks of the window's opener, and the parse.
// ok is false when the parse is not of the shape of the item's chain.
func (c *fixChecker) windowNodes(
	layout *docLayout, text []byte, at, end, skip int,
) ([]*mdast.Node, *mdast.FileSnapshot, bool) {
	parseCtx := c.ctx.Ctx
	if parseCtx == nil {
		parseCtx = context.Background()
	}
	snapshot, err := c.parser.ParseWithReferences(parseCtx, "", text, layout.refIndex)
	if err != nil || snapshot == nil || snapshot.Root == nil {
		return nil, nil, false
	}
	container, ok := layout.mdastContainer(snapshot.Root, itemScope, at, end)
	if !ok {
		return nil, nil, false
	}
	nodes, ok := blocksBefore(container, end)
	if !ok || len(nodes) < skip {
		return nil, nil, false
	}
	return nodes[skip:], snapshot, true
}
