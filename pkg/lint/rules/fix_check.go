package rules

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"
	gmext "github.com/yuin/goldmark/extension"
	gmextast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/yaklabco/gomdlint/pkg/config"
	"github.com/yaklabco/gomdlint/pkg/fix"
	"github.com/yaklabco/gomdlint/pkg/lint"
	"github.com/yaklabco/gomdlint/pkg/mdast"
	gmparser "github.com/yaklabco/gomdlint/pkg/parser/goldmark"
)

// individualFixChecks is the number of fixes a rule may report in a file up
// to which each fix is checked on its own before they are checked together.
// Past it, only the fixes that share a top-level block with another fix are
// checked on their own; the others are checked in batches that are split
// until each batch keeps the document unchanged, which keeps the cost of a
// file with many fixes close to one parse per batch.
const individualFixChecks = 16

// keepSafeFixes removes the fix from every diagnostic whose fix would change
// what a renderer produces, applied on its own or together with the other
// fixes the rule keeps. It is the last step of every fixing rule in this file
// set, so that a gap in a rule's own safety checks cannot corrupt a document.
// A fix that changes no byte at all, such as an empty deletion, is no fix and
// is removed first.
//
// A document whose raw HTML is not benign (see benignHTML) keeps no fix at
// all. A style sheet, or an element left open, can change how a browser
// shows whitespace, tabs and line breaks anywhere in the document, before
// the HTML as well as after it, which no signature models: a final line
// ending, for one, joins the value of a textarea left open. Whitespace,
// well-formed comments such as markdownlint directives, and br, wbr, hr and
// img tags leave the fixes to the signature. Raw HTML that the signature's
// parsers cannot see, in GFM footnotes and in TOML or JSON front matter,
// may not be benign, so a document that may hold it keeps no fix either
// (see htmlHiddenFromCheck).
//
// The engine may apply only some of the fixes, after a directive, a conflict
// or the last pass, so every fix must be safe on its own. When a rule reports
// few fixes, each is checked on its own. With many, each fix that shares a
// top-level block with another fix, or lies between blocks next to one, is
// checked on its own, because the two could depend on each other. A fix
// alone in its block is checked in batches, halved until a batch passes or
// holds one fix: nothing a fix in another block changes can put back what it
// changed in its block. Then the kept fixes are checked together, applied the
// way the fix engine applies them; if they change the document together,
// they are kept one at a time in document order, each only when the fixes
// kept before it and it still leave the document unchanged.
//
// Findings that share a fix carry the same edits, such as the blank line
// that MD032 reports after one list and before the next. The fix is checked
// once, with the first of them, and kept or withheld for all: checked
// together, its copies would conflict, and every check would fall back to
// taking the fixes one at a time.
func keepSafeFixes(ctx *lint.RuleContext, diags []lint.Diagnostic) []lint.Diagnostic {
	if !slices.ContainsFunc(diags, func(diag lint.Diagnostic) bool { return len(diag.FixEdits) > 0 }) {
		return diags
	}
	content := checkedContent(ctx.File.Content)
	var candidates []int
	sharers := make(map[int][]int)
	first := make(map[string]int)
	for idx := range diags {
		switch {
		case len(diags[idx].FixEdits) == 0:
		case !changesContent(content, checkedEdits(diags[idx].FixEdits)):
			diags[idx].FixEdits = nil
		default:
			key := editsKey(diags[idx].FixEdits)
			if owner, ok := first[key]; ok {
				sharers[owner] = append(sharers[owner], idx)
				continue
			}
			first[key] = idx
			candidates = append(candidates, idx)
		}
	}
	if len(candidates) > 0 {
		keepCandidateFixes(ctx, content, diags, candidates)
	}
	for owner, others := range sharers {
		if diags[owner].HasFix() {
			continue
		}
		for _, idx := range others {
			diags[idx].FixEdits = nil
		}
	}
	return diags
}

// keepCandidateFixes withholds the fix of each diagnostic in candidates,
// indexes into diags of fixes that differ from each other, that keepSafeFixes
// does not keep. content is the file's content as the checks read it (see
// checkedContent).
func keepCandidateFixes(ctx *lint.RuleContext, content []byte, diags []lint.Diagnostic, candidates []int) {
	flavor := config.FlavorCommonMark
	if ctx.Config != nil && ctx.Config.Flavor != "" {
		flavor = ctx.Config.Flavor
	}
	check := newFixCheckOf(ctx.File, content, flavor)
	if !check.htmlBenign() {
		for _, idx := range candidates {
			diags[idx].FixEdits = nil
		}
		return
	}

	shared := check.sharedFixes(diags, candidates)
	var kept, isolated []int
	for _, idx := range candidates {
		switch {
		case len(candidates) > individualFixChecks && !shared[idx]:
			isolated = append(isolated, idx)
		case check.keeps(diags[idx].FixEdits):
			kept = append(kept, idx)
		}
	}
	if len(isolated) > 0 {
		kept = append(kept, check.keptInBatches(diags, isolated)...)
		slices.Sort(kept)
	}
	if len(kept) > 1 && !check.keepsTogether(diags, kept) {
		kept, _ = check.keptInOrder(diags, kept)
	}

	keep := make(map[int]bool, len(kept))
	for _, idx := range kept {
		keep[idx] = true
	}
	for _, idx := range candidates {
		if !keep[idx] {
			diags[idx].FixEdits = nil
		}
	}
}

// changesContent reports whether applying the edits of one fix changes the
// content. Edits that cannot be applied together count as a change, which
// the fix check then rejects.
func changesContent(content []byte, edits []fix.TextEdit) bool {
	prepared, err := fix.PrepareEdits(edits, len(content))
	if err != nil {
		return true
	}
	// Sorted edits that do not overlap end in order, so the last one ends
	// the span they change.
	start, end := prepared[0].StartOffset, prepared[len(prepared)-1].EndOffset
	changed := make([]byte, 0, end-start)
	cursor := start
	for _, edit := range prepared {
		changed = append(changed, content[cursor:edit.StartOffset]...)
		changed = append(changed, edit.NewText...)
		cursor = edit.EndOffset
	}
	return !bytes.Equal(changed, content[start:end])
}

// sharedFixes returns the fixes whose edits share a top-level block of the
// original document with the edits of another fix, or lie between blocks
// next to such edits or next to a block another fix changes. Fixes that do
// not are alone in their block.
func (c *fixCheck) sharedFixes(diags []lint.Diagnostic, candidates []int) map[int]bool {
	shared := make(map[int]bool)
	blocks, ok := topLevelBlockLines(c.file)
	if !ok {
		for _, idx := range candidates {
			shared[idx] = true
		}
		return shared
	}
	ordered := blocksOrdered(blocks, len(c.file.Lines))
	owners := make(map[int][]int)
	for _, idx := range candidates {
		slots := make(map[int]bool)
		for _, edit := range diags[idx].FixEdits {
			for _, slot := range editSlots(c.file, blocks, ordered, edit) {
				slots[slot] = true
			}
		}
		for slot := range slots {
			owners[slot] = append(owners[slot], idx)
		}
	}
	for _, fixes := range owners {
		if len(fixes) > 1 {
			for _, idx := range fixes {
				shared[idx] = true
			}
		}
	}
	return shared
}

// topLevelBlockLines returns the first and last line of each top-level block,
// in document order, or false when a block has no position.
func topLevelBlockLines(file *mdast.FileSnapshot) ([][2]int, bool) {
	if file == nil || file.Root == nil {
		return nil, false
	}
	var blocks [][2]int
	for block := file.Root.FirstChild; block != nil; block = block.Next {
		pos := block.SourcePosition()
		if !pos.IsValid() {
			return nil, false
		}
		blocks = append(blocks, [2]int{pos.StartLine, nodeLastLine(block)})
	}
	return blocks, true
}

// blocksOrdered reports whether the line spans of top-level blocks lie
// within the file's lines, in order and apart: each starts no later than it
// ends and ends before the next starts. Then the blocks and the gaps between
// them that a range of lines touches can be found by binary search.
func blocksOrdered(blocks [][2]int, lines int) bool {
	previousEnd := 0
	for _, block := range blocks {
		if block[0] <= previousEnd || block[0] > block[1] || block[1] > lines {
			return false
		}
		previousEnd = block[1]
	}
	return true
}

// orderedEditSlots returns the slots of editSlots for the lines first to
// last, given ordered blocks in a file of the given number of lines. The
// ends of the blocks and of the gaps grow with their index, and so do their
// starts, so the blocks and gaps that the lines touch are consecutive.
func orderedEditSlots(lines int, blocks [][2]int, first, last int) []int {
	var slots []int
	for idx := sort.Search(len(blocks), func(i int) bool { return blocks[i][1] >= first }); idx < len(blocks) &&
		blocks[idx][0] <= last; idx++ {
		slots = append(slots, 2*idx+1)
	}
	gapFirst := func(idx int) int {
		if idx == 0 {
			return 1
		}
		return blocks[idx-1][1] + 1
	}
	gapLast := func(idx int) int {
		if idx == len(blocks) {
			return lines
		}
		return blocks[idx][0] - 1
	}
	for idx := sort.Search(len(blocks)+1, func(i int) bool { return gapLast(i) >= first }); idx <= len(blocks) &&
		gapFirst(idx) <= last; idx++ {
		if gapFirst(idx) > gapLast(idx) {
			continue
		}
		slots = append(slots, 2*idx)
		if idx > 0 {
			slots = append(slots, 2*idx-1)
		}
		if idx < len(blocks) {
			slots = append(slots, 2*idx+1)
		}
	}
	return slots
}

// editSlots returns the places an edit touches: slot 2i+1 for the top-level
// block i, and slot 2i for the lines between blocks i-1 and i, together with
// the slots of those two blocks, since an edit there can join or split them.
// An insertion at the start of a line touches the line before it too. An edit
// whose lines cannot be found touches slot -1, which every such edit shares.
// With ordered blocks (see blocksOrdered), the blocks and gaps are found by
// binary search, which finds the slots that scanning them all finds.
func editSlots(file *mdast.FileSnapshot, blocks [][2]int, ordered bool, edit fix.TextEdit) []int {
	first, _ := file.LineAt(edit.StartOffset)
	last, _ := file.LineAt(max(edit.StartOffset, edit.EndOffset-1))
	if first < 1 || last < 1 {
		return []int{-1}
	}
	if edit.StartOffset == edit.EndOffset && first > 1 && file.Lines[first-1].StartOffset == edit.StartOffset {
		first--
	}
	if ordered {
		return orderedEditSlots(len(file.Lines), blocks, first, last)
	}
	return scannedEditSlots(len(file.Lines), blocks, first, last)
}

// scannedEditSlots returns the slots of editSlots for the lines first to
// last by scanning every block and gap of a file of the given number of
// lines.
func scannedEditSlots(lines int, blocks [][2]int, first, last int) []int {
	var slots []int
	for idx, block := range blocks {
		if block[1] >= first && block[0] <= last {
			slots = append(slots, 2*idx+1)
		}
	}
	for idx := 0; idx <= len(blocks); idx++ {
		gapFirst, gapLast := 1, lines
		if idx > 0 {
			gapFirst = blocks[idx-1][1] + 1
		}
		if idx < len(blocks) {
			gapLast = blocks[idx][0] - 1
		}
		if gapFirst > gapLast || gapLast < first || gapFirst > last {
			continue
		}
		slots = append(slots, 2*idx)
		if idx > 0 {
			slots = append(slots, 2*idx-1)
		}
		if idx < len(blocks) {
			slots = append(slots, 2*idx+1)
		}
	}
	return slots
}

// keptInOrder returns the fixes of kept, in order, that keep the document
// unchanged together with the fixes kept before them: each is kept only when
// the fixes kept before it and it still leave the document unchanged.
//
// Checking each step as a whole costs all the regions of the fixes kept so
// far, which makes the loop grow with the square of their number. So the
// fixes kept so far are held in a regionState for each parser, and a step
// that the states decide costs only the regions the new fix meets (see
// regionState). A step they cannot decide checks the fixes as a whole, and
// the states are then built again for the fixes kept, or dropped when no
// state holds them. It also returns the number of steps the states decided.
func (c *fixCheck) keptInOrder(diags []lint.Diagnostic, kept []int) ([]int, int) {
	if !c.htmlBenign() {
		return nil, 0
	}
	var together []int
	statesDecided := 0
	states := c.regionStates(nil)
	for _, idx := range kept {
		decided, keep, commits := c.tryAdd(states, diags[idx].FixEdits)
		if decided {
			statesDecided++
		} else {
			keep = c.keepsTogether(diags, append(slices.Clone(together), idx))
		}
		if !keep {
			continue
		}
		together = append(together, idx)
		if decided {
			for _, commit := range commits {
				commit()
			}
			continue
		}
		var edits []fix.TextEdit
		for _, other := range together {
			edits = append(edits, diags[other].FixEdits...)
		}
		accepted, _, _, err := fix.PrepareEditsFiltered(checkedEdits(edits), len(c.content))
		states = nil
		if err == nil {
			states = c.regionStates(accepted)
		}
	}
	return together, statesDecided
}

// regionStates returns the region state of the edits, prepared, for each
// parser, or nil when some parser has none.
func (c *fixCheck) regionStates(prepared []fix.TextEdit) []*regionState {
	c.ensureModels()
	states := make([]*regionState, len(c.models))
	for idx, model := range c.models {
		if model == nil {
			return nil
		}
		if states[idx] = model.newRegionState(prepared); states[idx] == nil {
			return nil
		}
	}
	return states
}

// tryAdd reports whether adding the edits of a fix to the edits of states
// leaves the signatures unchanged for every parser. decided is false when
// the states cannot tell; when the fix keeps the signatures, commits add it
// to the states.
func (c *fixCheck) tryAdd(states []*regionState, edits []fix.TextEdit) (bool, bool, []func()) {
	if states == nil {
		return false, false, nil
	}
	prepared, err := fix.PrepareEdits(checkedEdits(edits), len(c.content))
	if err != nil {
		return false, false, nil
	}
	commits := make([]func(), 0, len(states))
	for idx, model := range c.models {
		decided, unchanged, commit := model.tryAdd(states[idx], prepared)
		switch {
		case !decided:
			return false, false, nil
		case !unchanged:
			return true, false, nil
		}
		commits = append(commits, commit)
	}
	return true, true, commits
}

// keptInBatches returns the fixes of a batch of diagnostics that keep the
// document unchanged: the whole batch when it passes together, and
// otherwise the fixes kept from each half, down to single fixes.
func (c *fixCheck) keptInBatches(diags []lint.Diagnostic, batch []int) []int {
	if c.keepsTogether(diags, batch) {
		return batch
	}
	if len(batch) == 1 {
		return nil
	}
	half := len(batch) / 2
	// The halves share their array with the batch, so the result is a copy.
	return slices.Concat(c.keptInBatches(diags, batch[:half]), c.keptInBatches(diags, batch[half:]))
}

// fixCheck verifies candidate fixes by applying them to a copy of the whole
// file, parsing it again with goldmark, and comparing render signatures (see
// fixCheck.signatures) before and after. The file is parsed in the
// configured flavor and, when that is CommonMark, in GFM too, because most
// Markdown is also read on GitHub.
//
// The check compares the complete document, so it sees the effects a fix
// has far from the text it changes: a link reference definition that links
// elsewhere use, the anchor of a heading that a fragment link points to,
// front matter that starts or ends. It parses only the regions of the
// document that the fix can change, and compares their signatures within the
// document's (see regionModel), which decides as comparing the whole
// document's signatures would; where that cannot be shown, it parses the
// whole document.
type fixCheck struct {
	file *mdast.FileSnapshot
	// content is the file's content as the checks read it (see
	// checkedContent).
	content   []byte
	flavor    config.Flavor
	markdowns []goldmark.Markdown
	// baseline holds the render signatures of the original content, and
	// benign whether all its raw HTML is benign. Both are set on first use.
	baseline []string
	benign   bool
	// hidden reports whether raw HTML may hide in footnotes or front matter
	// (see htmlHiddenFromCheck), and hiddenChecked whether it is set.
	hidden        bool
	hiddenChecked bool
	// models holds the region model of the original for each parser, nil
	// where region checks cannot be used; modeled reports whether they are
	// built.
	models  []*regionModel
	modeled bool
	// wholeParsed counts the bytes that whole-document checks parsed,
	// which tests bound.
	wholeParsed int
}

func newFixCheck(file *mdast.FileSnapshot, flavor config.Flavor) *fixCheck {
	return newFixCheckOf(file, checkedContent(file.Content), flavor)
}

// newFixCheckOf returns a fixCheck for file, whose content as the checks read
// it is content (see checkedContent).
func newFixCheckOf(file *mdast.FileSnapshot, content []byte, flavor config.Flavor) *fixCheck {
	markdowns := []goldmark.Markdown{goldmark.New(goldmark.WithExtensions(gmext.GFM))}
	if flavor != config.FlavorGFM {
		markdowns = append(markdowns, goldmark.New())
	}
	return &fixCheck{file: file, content: content, flavor: flavor, markdowns: markdowns}
}

// keeps reports whether applying edits leaves the render signatures of the
// document unchanged, in a document whose raw HTML is benign. The edits must
// not overlap.
func (c *fixCheck) keeps(edits []fix.TextEdit) bool {
	if len(edits) == 0 || !c.htmlBenign() {
		return false
	}
	prepared, err := fix.PrepareEdits(checkedEdits(edits), len(c.content))
	if err != nil {
		return false
	}
	return c.unchangedBy(prepared)
}

// unchangedBy reports whether applying edits, prepared, leaves the render
// signatures of the document unchanged: by region checks when they can
// tell, and otherwise by parsing the whole document.
func (c *fixCheck) unchangedBy(prepared []fix.TextEdit) bool {
	if decided, unchanged := c.regionsUnchanged(prepared); decided {
		return unchanged
	}
	return c.unchanged(fix.ApplyEdits(c.content, prepared))
}

// regionsUnchanged reports whether applying edits, prepared, leaves the
// render signature unchanged for every parser, as region checks find.
// decided is false when region checks cannot tell for some parser.
func (c *fixCheck) regionsUnchanged(prepared []fix.TextEdit) (bool, bool) {
	c.ensureModels()
	for _, model := range c.models {
		if model == nil {
			return false, false
		}
	}
	for _, model := range c.models {
		decided, unchanged := model.checkEdits(prepared)
		if !decided {
			return false, false
		}
		if !unchanged {
			return true, false
		}
	}
	return true, true
}

// ensureModels builds the region model of the original for each parser.
func (c *fixCheck) ensureModels() {
	if c.modeled {
		return
	}
	c.modeled = true
	c.ensureBaseline()
	for idx, markdown := range regionMarkdowns(c.flavor) {
		c.models = append(c.models, buildRegionModel(markdown, c.content, c.baseline[idx]))
	}
}

// htmlBenign reports whether all raw HTML of the original content is
// benign, so that its fixes can be judged by the render signature. Raw HTML
// that may hide in footnotes or front matter counts as not benign (see
// htmlHiddenFromCheck), which is found before the content is parsed.
func (c *fixCheck) htmlBenign() bool {
	if !c.hiddenChecked {
		c.hiddenChecked = true
		c.hidden = htmlHiddenFromCheck(c.content)
	}
	if c.hidden {
		return false
	}
	c.ensureBaseline()
	return c.benign
}

// keepsTogether reports whether the fixes of the given diagnostics, applied
// together the way the fix engine applies them, leave the render signatures
// of the document unchanged.
func (c *fixCheck) keepsTogether(diags []lint.Diagnostic, indexes []int) bool {
	var edits []fix.TextEdit
	for _, idx := range indexes {
		edits = append(edits, diags[idx].FixEdits...)
	}
	accepted, _, _, err := fix.PrepareEditsFiltered(checkedEdits(edits), len(c.content))
	if err != nil || !c.htmlBenign() {
		return false
	}
	return c.unchangedBy(accepted)
}

// ensureBaseline parses the original content once, for its render
// signatures and whether its raw HTML is benign.
func (c *fixCheck) ensureBaseline() {
	if c.baseline == nil {
		c.baseline, c.benign = c.signatures(c.content)
	}
}

// unchanged reports whether changed renders like the original content.
func (c *fixCheck) unchanged(changed []byte) bool {
	c.ensureBaseline()
	c.wholeParsed += len(changed) * len(c.markdowns)
	candidate, _ := c.signatures(changed)
	for idx := range candidate {
		if candidate[idx] != c.baseline[idx] {
			return false
		}
	}
	return true
}

// signatures returns the render signature of content for each parser: its
// front matter, kept as written, and the signature of the Markdown after it
// (see goldmarkSignature). A byte order mark is left out, as the parser
// leaves it out, and the front matter lines are blanked before parsing. It
// also reports whether every raw HTML region is benign for every parser.
func (c *fixCheck) signatures(content []byte) ([]string, bool) {
	content = bytes.TrimPrefix(content, []byte(mdast.ByteOrderMark))
	frontMatter := frontMatterLength(content)
	source := bytes.Clone(content)
	for idx := range frontMatter {
		if source[idx] != '\n' && source[idx] != '\r' {
			source[idx] = ' '
		}
	}
	signatures := make([]string, len(c.markdowns))
	benign := true
	for idx, markdown := range c.markdowns {
		doc := markdown.Parser().Parse(gmtext.NewReader(source))
		signatures[idx] = fmt.Sprintf("FM%q", content[:frontMatter]) + goldmarkSignature(doc, source)
		benign = benign && htmlRegionsBenign(doc, source)
	}
	return signatures, benign
}

// The lines that open and close YAML and TOML front matter.
const (
	yamlFrontMatterFence = "---"
	tomlFrontMatterFence = "+++"
)

// frontMatterLength returns the number of bytes of a complete leading YAML
// or TOML metadata block, the way the parser recognizes one: a first line of
// "---", "+++" or "{", up to a closing "---" or "..." ("+++" or "}" for the
// other two), with trailing spaces allowed on both. It returns 0 when the
// content has no front matter.
func frontMatterLength(content []byte) int {
	lines := bytes.SplitAfter(content, []byte("\n"))
	if len(lines) < 2 {
		return 0
	}
	trimmed := func(line []byte) string {
		return string(bytes.TrimRight(line, " \t\r\n"))
	}
	opening := trimmed(lines[0])
	closing := opening
	switch opening {
	case yamlFrontMatterFence, tomlFrontMatterFence:
	case "{":
		closing = "}"
	default:
		return 0
	}
	length := len(lines[0])
	for _, line := range lines[1:] {
		length += len(line)
		text := trimmed(line)
		if text == closing || (opening != "{" && text == "...") {
			return length
		}
	}
	return 0
}

// goldmarkSignature describes what a renderer produces for a document parsed
// by goldmark: block kinds and nesting, heading levels and the anchors GitHub
// gives headings, list kinds, start numbers and tightness, code block info
// strings and content, table cell alignment, and inline content with its
// hard breaks, emphasis, links and images with their resolved destinations
// and titles, code spans and task checkboxes. Raw HTML, code, link titles
// and image descriptions are kept as written: code is opaque, because its
// whitespace can be significant, as the tabs of a Makefile are. Runs of the
// whitespace HTML collapses (spaces, tabs, line endings and form feeds)
// collapse to one space in other text.
//
// The signature does not model how HTML and CSS show whitespace, so the fix
// check keeps no fix in a document whose raw HTML is not benign (see
// benignHTML).
func goldmarkSignature(doc gmast.Node, source []byte) string {
	var out strings.Builder
	sig := signatureWriter{source: source, out: &out, anchors: make(map[string]int)}
	sig.blocks(doc)
	return out.String()
}

// signatureWriter builds a render signature. Inline text is buffered so
// that whitespace runs spanning text nodes and soft breaks collapse to one
// space.
type signatureWriter struct {
	source  []byte
	out     *strings.Builder
	text    []byte
	anchors map[string]int
	// log, when set, records the slug and anchor of each heading and every
	// change to the anchor counts (see anchorHistory).
	log *anchorHistory
	// With useAssigned, headings take their anchors from assigned in order,
	// as a region check works them out; missing records a heading for which
	// none was left.
	useAssigned bool
	assigned    []string
	missing     bool
}

// blocks writes the children of a container.
func (s *signatureWriter) blocks(parent gmast.Node) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		s.block(node)
	}
}

// htmlRegionsBenign reports whether every raw HTML region of a document,
// each HTML block and each inline HTML node, is benign.
func htmlRegionsBenign(doc gmast.Node, source []byte) bool {
	benign := true
	_ = gmast.Walk(doc, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) { //nolint:errcheck // Visitor never returns an error.
		if !entering {
			return gmast.WalkContinue, nil
		}
		var raw []byte
		switch node := node.(type) {
		case *gmast.HTMLBlock:
			for idx := range node.Lines().Len() {
				segment := node.Lines().At(idx)
				raw = append(raw, segment.Value(source)...)
			}
			if node.HasClosure() {
				raw = append(raw, node.ClosureLine.Value(source)...)
			}
		case *gmast.RawHTML:
			for idx := range node.Segments.Len() {
				segment := node.Segments.At(idx)
				raw = append(raw, segment.Value(source)...)
			}
		default:
			return gmast.WalkContinue, nil
		}
		if !benignHTML(raw) {
			benign = false
			return gmast.WalkStop, nil
		}
		return gmast.WalkSkipChildren, nil
	})
	return benign
}

// htmlHiddenFromCheck reports whether content may hold raw HTML that is not
// benign in GFM footnotes or front matter, where the signature's parsers
// read none. Such a document keeps no fix, like one whose raw HTML is not
// benign (see keepSafeFixes). It looks only at content with a "<" that may
// start raw HTML (see hasTagStart), and finds two cases:
//
//   - TOML and JSON front matter. The parsers blank front matter, but GitHub
//     renders "+++" and "{" front matter as Markdown, where raw HTML may open
//     an element that holds the rest of the page, and a block the front
//     matter leaves open, such as a paragraph or a list item, may take in
//     raw HTML after it, such as an indented line that the parsers read as
//     code. GitHub shows YAML front matter as a table, so it is left to the
//     check.
//   - A "[^" anywhere, which may start a GFM footnote. The parsers do not
//     read footnotes: they may read "[^1]: <!--" as a link reference
//     definition and an indented footnote paragraph as code, and see no raw
//     HTML there. cmark-gfm, which GitHub uses, writes footnotes after the
//     document, where a comment or an element left open in one holds the
//     footnotes after it. goldmark and cmark-gfm also disagree on which
//     labels make a definition, where a definition ends, and how the blocks
//     in it read, so no second parse can show what GitHub sees. A "[^" that
//     starts no footnote, such as one in a code span or an undefined
//     reference, costs the fixes all the same.
func htmlHiddenFromCheck(content []byte) bool {
	content = bytes.TrimPrefix(content, []byte(mdast.ByteOrderMark))
	if !hasTagStart(content) {
		return false
	}
	if (bytes.HasPrefix(content, []byte(tomlFrontMatterFence)) || bytes.HasPrefix(content, []byte("{"))) &&
		frontMatterLength(content) > 0 {
		return true
	}
	return bytes.Contains(content, []byte("[^"))
}

// hasTagStart reports whether text holds a "<" that may start raw HTML for
// a Markdown renderer: "<!" or "<?", which may start a comment, a
// declaration, CDATA or a processing instruction, or "<" or "</" and a tag
// name, ASCII letters, digits and hyphens after a letter, that whitespace,
// "/", ">" or the end of text follows, as in a start or end tag. Any other
// "<" is text for every renderer, as in "1 < 2" or "{{< x >}}", or starts
// an autolink, such as "<https://example.com>" or "<user@example.com>",
// whose name a ":" or an "@" ends.
func hasTagStart(text []byte) bool {
	for pos := 0; pos < len(text); {
		idx := bytes.IndexByte(text[pos:], '<')
		if idx < 0 {
			return false
		}
		pos += idx + 1
		if pos < len(text) && (text[pos] == '!' || text[pos] == '?') {
			return true
		}
		if pos < len(text) && text[pos] == '/' {
			pos++
		}
		if pos == len(text) || !isASCIILetter(text[pos]) {
			continue
		}
		for pos < len(text) && (isASCIILetter(text[pos]) || isASCIIDigit(text[pos]) || text[pos] == '-') {
			pos++
		}
		if pos == len(text) || strings.IndexByte(" \t\n\r\f\v/>", text[pos]) >= 0 {
			return true
		}
	}
	return false
}

// benignHTML reports whether raw HTML can show nothing that the render
// signature does not compare: it is a run of whitespace, well-formed
// comments, and start tags of br, wbr, hr and img, named in any ASCII case,
// with only the attributes benignAttribute allows, each once, written as
// CommonMark defines them with quoted values on one line. Any other HTML,
// including any other tag or attribute, an end tag, a name with other than
// ASCII letters, a malformed comment, a processing instruction, a
// declaration or CDATA, may bring a style sheet, run a script or open an
// element, which changes how a browser shows the text before or after it.
func benignHTML(raw []byte) bool {
	for pos := 0; pos < len(raw); {
		end := -1
		switch {
		case isHTMLSpace(raw[pos]):
			end = pos + 1
		case bytes.HasPrefix(raw[pos:], []byte("<!--")):
			end = htmlCommentEnd(raw, pos)
		case raw[pos] == '<':
			end = benignTagEnd(raw, pos)
		}
		if end < 0 {
			return false
		}
		pos = end
	}
	return true
}

// isHTMLSpace reports whether a byte is whitespace in an HTML tag.
func isHTMLSpace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\r' || char == '\f'
}

// htmlCommentEnd returns the offset after the well-formed comment that starts
// at pos, or -1. Its text runs to the first "-->", does not start with ">"
// or "->", and holds no "--".
func htmlCommentEnd(raw []byte, pos int) int {
	start := pos + len("<!--")
	length := bytes.Index(raw[start:], []byte("-->"))
	if length < 0 {
		return -1
	}
	text := raw[start : start+length]
	if bytes.HasPrefix(text, []byte(">")) || bytes.HasPrefix(text, []byte("->")) || bytes.Contains(text, []byte("--")) {
		return -1
	}
	return start + length + len("-->")
}

// benignTagEnd returns the offset after the start tag of br, wbr, hr or img
// that starts at pos, or -1. The name is matched in ASCII case only, so a
// letter that folds to an ASCII one, such as the Kelvin sign, does not
// match, as it does not for browsers.
func benignTagEnd(raw []byte, pos int) int {
	idx := pos + 1
	for idx < len(raw) && (isASCIILetter(raw[idx]) || isASCIIDigit(raw[idx]) || raw[idx] == '-') {
		idx++
	}
	tag := string(bytes.ToLower(raw[pos+1 : idx]))
	switch tag {
	case "br", "wbr", "hr", "img":
	default:
		return -1
	}
	var seen []string
	for {
		spaces := idx
		for idx < len(raw) && isHTMLSpace(raw[idx]) {
			idx++
		}
		switch {
		case idx == len(raw):
			return -1
		case raw[idx] == '>':
			return idx + 1
		case raw[idx] == '/':
			if idx+1 < len(raw) && raw[idx+1] == '>' {
				return idx + 2
			}
			return -1
		case idx == spaces || !isAttributeNameStart(raw[idx]):
			// Attributes are separated by whitespace.
			return -1
		}
		nameEnd := idx + 1
		for nameEnd < len(raw) && isAttributeNameChar(raw[nameEnd]) {
			nameEnd++
		}
		attribute := string(bytes.ToLower(raw[idx:nameEnd]))
		if !benignAttribute(tag, attribute) || slices.Contains(seen, attribute) {
			return -1
		}
		seen = append(seen, attribute)
		if idx = htmlAttributeValueEnd(raw, nameEnd); idx < 0 {
			return -1
		}
	}
}

// benignAttribute reports whether a benign tag may carry an attribute,
// named in lowercase: an img its source, text, size, alignment, border and
// loading hints, a br clear, and an hr its alignment, size and shading. Any
// other attribute, such as an event handler, a style, a class or an id, can
// change how the text around the tag shows.
func benignAttribute(tag, attribute string) bool {
	switch tag {
	case "img":
		switch attribute {
		case "src", "alt", "title", "width", "height", "align", "border", "loading", "decoding", "srcset", "sizes":
			return true
		}
	case "br":
		return attribute == "clear"
	case "hr":
		switch attribute {
		case "align", "width", "size", "noshade":
			return true
		}
	}
	return false
}

// htmlAttributeValueEnd returns the offset after the value of the attribute
// whose name ends at pos, or pos when it has none. A value follows "=" and
// is either quoted, with no line ending inside, or unquoted, without
// whitespace, quotes, "=", "<", ">" or "`". It returns -1 when the value is
// malformed.
func htmlAttributeValueEnd(raw []byte, pos int) int {
	value := pos
	for value < len(raw) && isHTMLSpace(raw[value]) {
		value++
	}
	if value == len(raw) || raw[value] != '=' {
		return pos
	}
	value++
	for value < len(raw) && isHTMLSpace(raw[value]) {
		value++
	}
	if value == len(raw) {
		return -1
	}
	if quote := raw[value]; quote == '"' || quote == '\'' {
		length := bytes.IndexByte(raw[value+1:], quote)
		if length < 0 || bytes.ContainsAny(raw[value+1:value+1+length], "\r\n") {
			return -1
		}
		return value + 1 + length + 1
	}
	end := value
	for end < len(raw) && !isHTMLSpace(raw[end]) && !strings.ContainsRune("\"'=<>`", rune(raw[end])) {
		end++
	}
	if end == value {
		return -1
	}
	return end
}

func (s *signatureWriter) block(node gmast.Node) {
	switch node := node.(type) {
	case *gmast.Paragraph, *gmast.TextBlock:
		s.out.WriteString("P(")
		s.inlineContent(node)
	case *gmast.Heading:
		fmt.Fprintf(s.out, "H%d#%s(", node.Level, s.anchor(headingText(node, s.source)))
		s.inlineContent(node)
	case *gmast.List:
		s.out.WriteString(listToken(node.IsOrdered(), node.Start, gmparser.ListTight(node, s.source)))
		s.blocks(node)
	case *gmast.FencedCodeBlock:
		info := ""
		if node.Info != nil {
			info = string(node.Info.Segment.Value(s.source))
		}
		fmt.Fprintf(s.out, "C%q(%q", info, linesText(node.Lines(), s.source))
	case *gmast.CodeBlock:
		fmt.Fprintf(s.out, "C(%q", linesText(node.Lines(), s.source))
	case *gmast.HTMLBlock:
		raw := linesText(node.Lines(), s.source)
		if node.HasClosure() {
			raw += "\n" + strings.TrimRight(string(node.ClosureLine.Value(s.source)), "\r\n")
		}
		fmt.Fprintf(s.out, "B(%q", raw)
	case *gmextast.TableCell:
		fmt.Fprintf(s.out, "D%d(", node.Alignment)
		s.inlineContent(node)
	default:
		s.out.WriteString(node.Kind().String() + "(")
		s.blocks(node)
	}
	s.out.WriteString(")")
}

// anchor returns the anchor GitHub gives a heading with the given text: the
// slug of the text, with "-1", "-2" and so on added to repeated slugs, the
// way github-slugger counts them.
func (s *signatureWriter) anchor(text string) string {
	if s.useAssigned {
		if len(s.assigned) == 0 {
			s.missing = true
			return ""
		}
		unique := s.assigned[0]
		s.assigned = s.assigned[1:]
		return unique
	}
	slug := githubSlug(text)
	unique := slug
	for {
		if _, taken := s.anchors[unique]; !taken {
			break
		}
		s.anchors[slug]++
		if s.log != nil {
			s.log.write(slug, s.anchors[slug])
		}
		unique = slug + "-" + strconv.Itoa(s.anchors[slug])
	}
	s.anchors[unique] = 0
	if s.log != nil {
		s.log.write(unique, 0)
		s.log.slugs = append(s.log.slugs, slug)
		s.log.anchors = append(s.log.anchors, unique)
	}
	return unique
}

// githubSlug returns the github-slugger slug of heading text: lowercased,
// with every character other than letters, marks, numbers, connector
// punctuation, spaces and hyphens removed, and each space turned into a
// hyphen.
func githubSlug(text string) string {
	var slug strings.Builder
	for _, char := range strings.ToLower(text) {
		switch {
		case char == ' ':
			slug.WriteByte('-')
		case char == '-' || unicode.IsLetter(char) || unicode.IsMark(char) || unicode.IsNumber(char) ||
			unicode.Is(unicode.Pc, char):
			slug.WriteRune(char)
		}
	}
	return slug.String()
}

// headingText returns the text content of a heading as a renderer produces
// it, the text that GitHub derives the anchor from: the text and code of
// its inline content without markup, with a line break inside the heading
// read as a line ending.
func headingText(heading gmast.Node, source []byte) string {
	var text strings.Builder
	var walk func(node gmast.Node)
	walk = func(parent gmast.Node) {
		for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
			switch node := node.(type) {
			case *gmast.Text:
				text.Write(node.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					text.WriteByte('\n')
				}
			case *gmast.String:
				text.Write(node.Value)
			case *gmast.RawHTML, *gmast.Image:
			default:
				walk(node)
			}
		}
	}
	walk(heading)
	return text.String()
}

// linesText joins the text of block lines, including the spaces that stand
// for the part of a tab that the indentation did not use. The line ending
// of the last line is left out, because a block that ends the file has none
// until a final newline is added.
func linesText(lines *gmtext.Segments, source []byte) string {
	var text strings.Builder
	for idx := range lines.Len() {
		segment := lines.At(idx)
		text.WriteString(strings.Repeat(" ", segment.Padding))
		text.Write(segment.Value(source))
	}
	return strings.TrimSuffix(strings.TrimSuffix(text.String(), "\n"), "\r")
}

// inlineContent writes the inline content of a block, trimmed.
func (s *signatureWriter) inlineContent(parent gmast.Node) {
	outer := s.out
	var inner strings.Builder
	s.out = &inner
	s.inlines(parent)
	s.flush()
	s.out = outer
	s.out.WriteString(strings.Trim(inner.String(), " "))
}

func (s *signatureWriter) inlines(parent gmast.Node) {
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		s.inline(node)
	}
}

func (s *signatureWriter) inline(node gmast.Node) {
	switch node := node.(type) {
	case *gmast.Text:
		value := node.Value(s.source)
		switch {
		case node.HardLineBreak():
			// Renderers drop the spaces before a line break; goldmark
			// leaves some of them in the text with GFM extensions.
			s.text = bytes.TrimRight(append(s.text, value...), " \t")
			s.token("\x00br")
		case node.SoftLineBreak():
			s.text = append(append(s.text, value...), ' ')
		default:
			s.text = append(s.text, value...)
		}
	case *gmast.String:
		s.text = append(s.text, node.Value...)
	case *gmast.CodeSpan:
		var code strings.Builder
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if text, ok := child.(*gmast.Text); ok {
				code.Write(text.Value(s.source))
				if text.SoftLineBreak() || text.HardLineBreak() {
					code.WriteByte(' ')
				}
			}
		}
		content := strings.NewReplacer("\r\n", " ", "\n", " ").Replace(code.String())
		s.token(fmt.Sprintf("\x00code%q", content))
	case *gmast.RawHTML:
		var raw strings.Builder
		for idx := range node.Segments.Len() {
			segment := node.Segments.At(idx)
			raw.Write(segment.Value(s.source))
		}
		s.token(fmt.Sprintf("\x00html%q", raw.String()))
	case *gmast.AutoLink:
		s.token(fmt.Sprintf("\x00autolink%q", node.URL(s.source)))
	case *gmast.Link:
		s.token(fmt.Sprintf("\x00link%q%q(", node.Destination, node.Title))
		s.inlines(node)
		s.token(")")
	case *gmast.Image:
		s.token(fmt.Sprintf("\x00image%q%q%q", node.Destination, node.Title, exactText(node, s.source)))
	case *gmast.Emphasis:
		s.token(fmt.Sprintf("\x00em%d(", node.Level))
		s.inlines(node)
		s.token(")")
	case *gmextast.TaskCheckBox:
		s.token(fmt.Sprintf("\x00task%t", node.IsChecked))
	default:
		s.token("\x00" + node.Kind().String() + "(")
		s.inlines(node)
		s.token(")")
	}
}

// exactText returns the source text of the inline content of a node, such
// as the description of an image, which renderers write into an attribute
// as it is, with each line break kept.
func exactText(parent gmast.Node, source []byte) string {
	var text strings.Builder
	var walk func(node gmast.Node)
	walk = func(parent gmast.Node) {
		for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
			switch node := node.(type) {
			case *gmast.Text:
				text.Write(node.Value(source))
				if node.SoftLineBreak() || node.HardLineBreak() {
					text.WriteByte('\n')
				}
			case *gmast.String:
				text.Write(node.Value)
			default:
				fmt.Fprintf(&text, "\x00%s(", node.Kind())
				walk(node)
				text.WriteString(")")
			}
		}
	}
	walk(parent)
	return text.String()
}

// token writes buffered text, collapsed, and then a token.
func (s *signatureWriter) token(token string) {
	s.flush()
	s.out.WriteString(token)
}

// flush writes buffered text with every run of HTML whitespace collapsed to
// one space.
func (s *signatureWriter) flush() {
	if len(s.text) == 0 {
		return
	}
	space := false
	for _, char := range s.text {
		if isHTMLWhitespace(char) {
			space = true
			continue
		}
		if space {
			s.out.WriteByte(' ')
			space = false
		}
		s.out.WriteByte(char)
	}
	if space {
		s.out.WriteByte(' ')
	}
	s.text = s.text[:0]
}

// isHTMLWhitespace reports whether a byte is whitespace that HTML collapses:
// a space, tab, line feed, form feed or carriage return. A no-break space is
// not.
func isHTMLWhitespace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\f' || char == '\r'
}
