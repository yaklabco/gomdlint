package rules

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	gmast "github.com/yuin/goldmark/ast"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

// anchorHistory records the slugs and anchors the signature writer gives
// headings and every change to its anchor counts, so that the counts before
// any heading can be looked up.
type anchorHistory struct {
	slugs, anchors []string
	// writes holds, for each key, its values in order, each with the index
	// of the heading that set it.
	writes map[string][]anchorWrite
	// keys holds, for each heading, the keys it set.
	keys [][]string
}

type anchorWrite struct {
	heading, value int
}

// write records that the heading being written set key to value.
func (h *anchorHistory) write(key string, value int) {
	if h.writes == nil {
		h.writes = make(map[string][]anchorWrite)
	}
	heading := len(h.slugs)
	h.writes[key] = append(h.writes[key], anchorWrite{heading: heading, value: value})
	for len(h.keys) <= heading {
		h.keys = append(h.keys, nil)
	}
	h.keys[heading] = append(h.keys[heading], key)
}

// lookup returns the count of key before the given heading.
func (h *anchorHistory) lookup(key string, before int) (int, bool) {
	writes := h.writes[key]
	idx := sort.Search(len(writes), func(i int) bool { return writes[i].heading >= before })
	if idx == 0 {
		return 0, false
	}
	return writes[idx-1].value, true
}

// replay returns the anchors that headings with the given slugs get when
// they follow the first `before` headings of the document, as
// signatureWriter.anchor gives them, and the counts they set.
func (h *anchorHistory) replay(slugs []string, before int) ([]string, map[string]int) {
	counts := make(map[string]int)
	get := func(key string) (int, bool) {
		if value, found := counts[key]; found {
			return value, true
		}
		return h.lookup(key, before)
	}
	anchors := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		unique := slug
		for {
			if _, taken := get(unique); !taken {
				break
			}
			value, _ := get(slug)
			counts[slug] = value + 1
			unique = slug + "-" + strconv.Itoa(value+1)
		}
		counts[unique] = 0
		anchors = append(anchors, unique)
	}
	return anchors, counts
}

// sameCounts reports whether replacing headings [from, to) of the document
// with headings that set counts leaves the anchor counts as the document
// has them when it reaches heading to, after the replaced headings.
func (h *anchorHistory) sameCounts(counts map[string]int, from, to int) bool {
	same := func(key string) bool {
		want, wantFound := h.lookup(key, to)
		got, gotFound := counts[key]
		if !gotFound {
			got, gotFound = h.lookup(key, from)
		}
		return wantFound == gotFound && want == got
	}
	for key := range counts {
		if !same(key) {
			return false
		}
	}
	for heading := from; heading < to && heading < len(h.keys); heading++ {
		for _, key := range h.keys[heading] {
			if !same(key) {
				return false
			}
		}
	}
	return true
}

// regionModel is the original document as one parser reads it, with its
// render signature, as fixCheck.signatures writes it, cut into the parts
// that regions replace.
type regionModel struct {
	*docLayout
	sig string
	// groupSig and groupHeading hold, for each group and for the end, where
	// its blocks start in sig and the index of its first heading.
	groupSig     []int
	groupHeading []int
	listSigs     []listSig
	history      anchorHistory
	// outcomes caches what parsing a region with its edits found, and
	// cached counts the bytes it holds of its own (see cacheBudget).
	outcomes map[string]*regionOutcome
	cached   int
}

// cacheBudget returns how many bytes of their own the cached outcomes may
// hold. An outcome whose region keeps its part shares the text of the
// signature, but in a document that is one big block every check covers
// that block, and each outcome that changes it would otherwise keep a copy
// the size of the document.
func (m *regionModel) cacheBudget() int {
	return 8*len(m.sig) + 1<<20
}

// listSig is where the signature of a top-level list has its parts.
type listSig struct {
	ordered bool
	start   int
	tight   bool
	// tokenStart and tokenEnd bound the token that opens the list.
	tokenStart, tokenEnd int
	// itemSig and itemHeading hold, for each item and for the end, where its
	// signature starts and the index of its first heading.
	itemSig     []int
	itemHeading []int
}

// buildRegionModel builds the region model of content for a recording
// parser. It returns nil when region checks cannot be used: when the layout
// cannot be built, or the recording parse does not give baseline, the
// signature that the plain parse of the whole-document check gave.
func buildRegionModel(markdown goldmark.Markdown, content []byte, baseline string) *regionModel {
	layout := newDocLayout(markdown, content)
	if layout == nil {
		return nil
	}
	model := &regionModel{
		docLayout: layout,
		listSigs:  make([]listSig, len(layout.lists)),
		outcomes:  make(map[string]*regionOutcome),
	}
	var out strings.Builder
	fmt.Fprintf(&out, "FM%q", content[layout.base:layout.base+layout.frontMatter])
	writer := signatureWriter{source: layout.source, out: &out, anchors: make(map[string]int), log: &model.history}
	for group := range len(layout.bounds) - 1 {
		model.groupSig = append(model.groupSig, out.Len())
		model.groupHeading = append(model.groupHeading, len(model.history.slugs))
		for _, node := range layout.nodes[layout.groupNodes[group]:layout.groupNodes[group+1]] {
			list, isList := node.(*gmast.List)
			if index, found := layout.listIndex[node]; isList && found {
				model.listSigs[index] = writeListSignature(&writer, list)
				continue
			}
			writer.block(node)
		}
	}
	model.groupSig = append(model.groupSig, out.Len())
	model.groupHeading = append(model.groupHeading, len(model.history.slugs))
	model.sig = out.String()
	if model.sig != baseline {
		return nil
	}
	return model
}

// writeListSignature writes the signature of a top-level list, as
// signatureWriter.block does, and returns where its parts are.
func writeListSignature(writer *signatureWriter, list *gmast.List) listSig {
	sig := listSig{ordered: list.IsOrdered(), start: list.Start, tight: list.IsTight}
	sig.tokenStart = writer.out.Len()
	writer.out.WriteString(listToken(list.IsOrdered(), list.Start, list.IsTight))
	sig.tokenEnd = writer.out.Len()
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		sig.itemSig = append(sig.itemSig, writer.out.Len())
		sig.itemHeading = append(sig.itemHeading, len(writer.log.slugs))
		writer.block(item)
	}
	sig.itemSig = append(sig.itemSig, writer.out.Len())
	sig.itemHeading = append(sig.itemHeading, len(writer.log.slugs))
	writer.out.WriteString(")")
	return sig
}

// listToken is the signature token that opens a list.
func listToken(ordered bool, start int, tight bool) string {
	kind := "UL"
	if ordered {
		kind = fmt.Sprintf("OL%d", start)
	}
	return fmt.Sprintf("%s%t(", kind, tight)
}

// regionOutcome is what parsing a region with its edits found.
type regionOutcome struct {
	regionParse
	// start and end bound the region's part of the original signature, and
	// text is the signature of the region after the edits; same reports
	// whether text is that part.
	start, end int
	text       string
	same       bool
	// countsSame reports whether the anchor counts after the region are as
	// before.
	countsSame bool
	// loosens reports whether a list region makes its list loose.
	loosens bool
}

// sigSpan returns where a region's part of the signature starts and ends,
// and the indexes of its first heading and of the first heading after it.
func (m *regionModel) sigSpan(reg region) (int, int, int, int) {
	if reg.isList() {
		sig := m.listSigs[reg.list]
		return sig.itemSig[reg.first-1], sig.itemSig[reg.last+2], sig.itemHeading[reg.first-1], sig.itemHeading[reg.last+2]
	}
	return m.groupSig[reg.first], m.groupSig[reg.last+1], m.groupHeading[reg.first], m.groupHeading[reg.last+1]
}

// evaluate parses a region with the edits that start in it, sorted, or
// returns the cached outcome.
func (m *regionModel) evaluate(reg region, edits []placedEdit) *regionOutcome {
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
	outcome := &regionOutcome{regionParse: m.parseRegion(reg, plain)}
	start, end, from, to := m.sigSpan(reg)
	outcome.start, outcome.end = start, end
	if outcome.holds && !outcome.undecided {
		if reg.isList() {
			outcome.loosens = m.lists[reg.list].loosens(reg, outcome.tight)
		}
		m.finish(outcome, from, to)
	}
	// The cache keeps what the checks compare, not the parse, and stops
	// growing at its budget; an outcome left out is found again if needed.
	outcome.source, outcome.blocks = nil, nil
	size := key.Len()
	if !outcome.same {
		size += len(outcome.text)
	}
	if m.cached+size <= m.cacheBudget() {
		m.outcomes[key.String()] = outcome
		m.cached += size
	}
	return outcome
}

// finish writes the signature of a region's blocks, with the anchors their
// headings get after the first `from` headings of the document, in place of
// headings [from, to).
func (m *regionModel) finish(outcome *regionOutcome, from, to int) {
	slugs := headingSlugs(outcome.blocks, outcome.source)
	anchors := m.history.anchors[from:to]
	outcome.countsSame = true
	if !slices.Equal(slugs, m.history.slugs[from:to]) {
		var counts map[string]int
		anchors, counts = m.history.replay(slugs, from)
		outcome.countsSame = m.history.sameCounts(counts, from, to)
	}
	text, ok := regionSignature(outcome.blocks, outcome.source, anchors)
	part := m.sig[outcome.start:outcome.end]
	outcome.same = text == part
	outcome.text = text
	if outcome.same {
		// The part shares the signature's memory; the copy can go.
		outcome.text = part
	}
	outcome.undecided = !ok
}

// headingSlugs returns the slugs of the headings of blocks, in the order
// the signature writer gives them anchors.
func headingSlugs(blocks []gmast.Node, source []byte) []string {
	var slugs []string
	for _, block := range blocks {
		//nolint:errcheck // The visitor never returns an error.
		_ = gmast.Walk(block, func(node gmast.Node, entering bool) (gmast.WalkStatus, error) {
			if heading, ok := node.(*gmast.Heading); ok && entering {
				slugs = append(slugs, githubSlug(headingText(heading, source)))
				return gmast.WalkSkipChildren, nil
			}
			return gmast.WalkContinue, nil
		})
	}
	return slugs
}

// regionSignature writes the signature of blocks with the given anchors for
// their headings, in order. ok is false when the blocks hold a different
// number of headings.
func regionSignature(blocks []gmast.Node, source []byte, anchors []string) (string, bool) {
	var out strings.Builder
	writer := signatureWriter{source: source, out: &out, assigned: anchors, useAssigned: true}
	for _, block := range blocks {
		writer.block(block)
	}
	return out.String(), !writer.missing && len(writer.assigned) == 0
}

// checkEdits reports whether applying edits, prepared and in content
// offsets, leaves the render signature of the document unchanged. decided is
// false when region checks cannot tell, so that the whole document must be
// parsed.
func (m *regionModel) checkEdits(edits []fix.TextEdit) (bool, bool) {
	if len(edits) == 0 {
		return true, true
	}
	placed, early, ok := m.placeEdits(edits)
	if !ok || (early && !m.firstLineKeeps(placed, nil)) {
		return false, false
	}
	regions, outcomes, ok := m.decompose(placed)
	if !ok {
		return false, false
	}
	for _, outcome := range outcomes {
		if !outcome.refsSame {
			return false, false
		}
	}
	return m.compare(regions, outcomes)
}

// decompose returns the regions that placed edits, sorted, need, merged and
// grown until each holds, with their outcomes. ok is false when a region
// cannot be checked on its own.
func (m *regionModel) decompose(placed []placedEdit) ([]region, []*regionOutcome, bool) {
	return decomposeRegions(m.docLayout, placed, m.evaluate)
}

// replacement replaces a range of the original signature with text.
type replacement struct {
	start, end int
	text       string
	// unknownAfter reports that the signature after the replacement is not
	// known, because the anchor counts after the region changed.
	unknownAfter bool
}

// compare decides from the outcomes of regions, which all hold and add the
// same definitions, whether the signature of the edited document equals the
// original's.
func (m *regionModel) compare(regions []region, outcomes []*regionOutcome) (bool, bool) {
	reps := make([]replacement, 0, len(outcomes))
	listsSeen := make(map[int]bool)
	for idx, outcome := range outcomes {
		reps = append(reps, replacement{
			start: outcome.start, end: outcome.end, text: outcome.text, unknownAfter: !outcome.countsSame,
		})
		reg := regions[idx]
		if !reg.isList() || listsSeen[reg.list] {
			continue
		}
		listsSeen[reg.list] = true
		sig := m.listSigs[reg.list]
		if tight := m.tightAfter(reg.list, regions, outcomes); tight != sig.tight {
			reps = append(reps, replacement{
				start: sig.tokenStart, end: sig.tokenEnd, text: listToken(sig.ordered, sig.start, tight),
			})
		}
	}
	return decideReplacements(m.sig, reps)
}

// decideReplacements reports whether orig with reps replaced, which do not
// overlap, equals orig. decided is false when a replacement leaves the rest
// unknown and the part up to its end does not already differ: the anchors
// after a region whose anchor counts changed are not known.
func decideReplacements(orig string, reps []replacement) (bool, bool) {
	slices.SortFunc(reps, func(a, b replacement) int { return a.start - b.start })
	for idx, rep := range reps {
		if rep.unknownAfter {
			if replacedDiffers(orig, reps[:idx+1], false) {
				return true, false
			}
			return false, false
		}
	}
	return true, sameAfterReplacements(orig, reps)
}

// tightAfter reports whether a list is tight after edits, given the
// outcomes of the regions.
func (m *regionModel) tightAfter(list int, regions []region, outcomes []*regionOutcome) bool {
	return m.listTightAfter(list, regions, func(idx int) bool { return outcomes[idx].loosens })
}

// sameAfterReplacements reports whether orig with the ranges of reps, which
// are sorted and do not overlap, replaced by their text equals orig.
func sameAfterReplacements(orig string, reps []replacement) bool {
	delta := 0
	same := true
	for _, rep := range reps {
		delta += len(rep.text) - (rep.end - rep.start)
		same = same && rep.text == orig[rep.start:rep.end]
	}
	if same {
		return true
	}
	if delta != 0 {
		return false
	}
	return !replacedDiffers(orig, reps, true)
}

// replacedDiffers reports whether orig with the ranges of reps, which are
// sorted and do not overlap, replaced by their text differs from orig: up to
// the end of the last replacement, or with whole anywhere. A piece of orig
// that lands at its own offset is not compared.
func replacedDiffers(orig string, reps []replacement, whole bool) bool {
	pos, cursor := 0, 0
	differs := func(text string, origin int) bool {
		at := pos
		pos += len(text)
		switch {
		case origin == at:
			return false
		case at >= len(orig):
			return len(text) > 0
		}
		end := min(at+len(text), len(orig))
		return orig[at:end] != text[:end-at] || end-at < len(text)
	}
	for _, rep := range reps {
		if differs(orig[cursor:rep.start], cursor) || differs(rep.text, -1) {
			return true
		}
		cursor = rep.end
	}
	return whole && (differs(orig[cursor:], cursor) || pos != len(orig))
}

// regionState is a set of edits whose regions each keep their part of the
// signature: every region holds, adds the definitions it added before,
// leaves the anchor counts as they were and has the signature of the
// original's part, and no list changes tightness. So the set leaves the
// signature unchanged. When a fix is added, the regions it needs are merged
// with the set's regions they meet; the set's other regions keep their
// parts, so the signature after the fix is the original's with the parts of
// the merged regions and the tokens of lists that change tightness
// replaced, and tryAdd compares exactly that. Adding a fix then costs only
// the regions it meets, where checking the set and the fix as a whole costs
// all of them.
type regionState struct {
	// edits holds the set's edits in source offsets, sorted, as the fix
	// engine accepts them: no two overlap, and insertions at one offset,
	// which a state built from accepted edits may hold, keep their order.
	// An edit added later never has the range of one already held.
	edits []placedEdit
	// regions holds the set's regions, sorted by span, no two of which meet,
	// with their outcomes.
	regions  []region
	outcomes []*regionOutcome
	// inside counts, for each list, the items of the original inside its
	// list regions that make it loose, and loosening its list regions that
	// make it loose.
	inside, loosening []int
}

// newRegionState returns the state of the edits, prepared and in content
// offsets, or nil when some region of theirs does not keep its part of the
// signature or region checks cannot tell.
func (m *regionModel) newRegionState(edits []fix.TextEdit) *regionState {
	state := &regionState{inside: make([]int, len(m.lists)), loosening: make([]int, len(m.lists))}
	placed, early, ok := m.placeEdits(edits)
	if !ok || (early && !m.firstLineKeeps(placed, nil)) {
		return nil
	}
	regions, outcomes, ok := m.decompose(placed)
	if !ok {
		return nil
	}
	withRegions := make(map[int]bool)
	for idx, outcome := range outcomes {
		if !outcome.refsSame || !outcome.same || !outcome.countsSame {
			return nil
		}
		if reg := regions[idx]; reg.isList() {
			withRegions[reg.list] = true
			state.inside[reg.list] += m.lists[reg.list].looseIn(reg)
			if outcome.loosens {
				state.loosening[reg.list]++
			}
		}
	}
	for list := range withRegions {
		if m.tightWith(list, state.inside[list], state.loosening[list]) != m.listSigs[list].tight {
			return nil
		}
	}
	state.edits, state.regions, state.outcomes = placed, regions, outcomes
	return state
}

// tightWith reports whether a list that may have list regions is tight when
// its list regions hold the given number of its loose items and the given
// number of them make it loose. Without list regions, that is the list's
// tightness in the original, as newLayoutList checks.
func (m *regionModel) tightWith(list, inside, loosening int) bool {
	model := m.lists[list]
	return loosening == 0 && model.loose[len(model.loose)-1]-inside == 0
}

// clashes reports whether an edit, in source offsets, overlaps an edit of
// the state or has the same range, so that the fix engine might not apply
// both as they are.
func (s *regionState) clashes(edit fix.TextEdit) bool {
	// The edits do not overlap, so their ends grow with their starts.
	idx := sort.Search(len(s.edits), func(i int) bool { return s.edits[i].edit.EndOffset >= edit.StartOffset })
	for ; idx < len(s.edits) && s.edits[idx].edit.StartOffset <= edit.EndOffset; idx++ {
		other := s.edits[idx].edit
		if (edit.StartOffset < other.EndOffset && other.StartOffset < edit.EndOffset) ||
			(edit.StartOffset == other.StartOffset && edit.EndOffset == other.EndOffset) {
			return true
		}
	}
	return false
}

// meeting returns the indexes of the state's regions that meet reg.
func (s *regionState) meeting(layout *docLayout, reg region) []int {
	start, end := layout.span(reg)
	idx := sort.Search(len(s.regions), func(i int) bool {
		_, otherEnd := layout.span(s.regions[i])
		return otherEnd > start
	})
	var found []int
	for ; idx < len(s.regions); idx++ {
		if otherStart, _ := layout.span(s.regions[idx]); otherStart >= end {
			break
		}
		if layout.meets(reg, s.regions[idx]) {
			found = append(found, idx)
		}
	}
	return found
}

// tryAdd reports whether adding edits, prepared and in content offsets, to
// the state's edits leaves the signature unchanged. decided is false when the
// state cannot tell. When the edits keep the signature, commit adds them to
// the state.
func (m *regionModel) tryAdd(state *regionState, edits []fix.TextEdit) (bool, bool, func()) {
	placed, early, ok := m.placeEdits(edits)
	if !ok {
		return false, false, nil
	}
	for _, edit := range placed {
		if state.clashes(edit.edit) {
			return false, false, nil
		}
	}
	// An edit that joins the first line with the next makes the edits there
	// part of the first line, so the edits are cleared together.
	if (early || m.startsEarly(state.edits)) && !m.firstLineKeeps(state.edits, placed) {
		return false, false, nil
	}
	regions, outcomes, absorbed, ok := m.settle(state, placed)
	if !ok {
		return false, false, nil
	}
	// Definitions reach links before them as well as after.
	for _, outcome := range outcomes {
		if !outcome.refsSame {
			return false, false, nil
		}
	}
	type change struct{ inside, loosening int }
	changes := make(map[int]change)
	for _, idx := range absorbed {
		if reg := state.regions[idx]; reg.isList() {
			c := changes[reg.list]
			c.inside -= m.lists[reg.list].looseIn(reg)
			if state.outcomes[idx].loosens {
				c.loosening--
			}
			changes[reg.list] = c
		}
	}
	for idx, reg := range regions {
		if reg.isList() {
			c := changes[reg.list]
			c.inside += m.lists[reg.list].looseIn(reg)
			if outcomes[idx].loosens {
				c.loosening++
			}
			changes[reg.list] = c
		}
	}
	// The state's other regions keep their parts, so the signature after the
	// edits is the original's with the parts of the new regions replaced, and
	// the token of each list whose tightness changes.
	reps := make([]replacement, 0, len(outcomes))
	clean := true
	for _, outcome := range outcomes {
		reps = append(reps, replacement{
			start: outcome.start, end: outcome.end, text: outcome.text, unknownAfter: !outcome.countsSame,
		})
		clean = clean && outcome.same
	}
	for list, c := range changes {
		sig := m.listSigs[list]
		if tight := m.tightWith(list, state.inside[list]+c.inside, state.loosening[list]+c.loosening); tight != sig.tight {
			reps = append(reps, replacement{
				start: sig.tokenStart, end: sig.tokenEnd, text: listToken(sig.ordered, sig.start, tight),
			})
			clean = false
		}
	}
	if decided, unchanged := decideReplacements(m.sig, reps); !decided || !unchanged {
		return decided, unchanged, nil
	}
	// Regions that keep the signature only together, by parts that offset
	// each other, cannot join the state; the whole check decides them.
	if !clean {
		return false, false, nil
	}
	commit := func() {
		for idx := len(absorbed) - 1; idx >= 0; idx-- {
			state.regions = slices.Delete(state.regions, absorbed[idx], absorbed[idx]+1)
			state.outcomes = slices.Delete(state.outcomes, absorbed[idx], absorbed[idx]+1)
		}
		for idx, reg := range regions {
			start, _ := m.span(reg)
			pos := sort.Search(len(state.regions), func(i int) bool {
				other, _ := m.span(state.regions[i])
				return other > start
			})
			state.regions = slices.Insert(state.regions, pos, reg)
			state.outcomes = slices.Insert(state.outcomes, pos, outcomes[idx])
		}
		for list, c := range changes {
			state.inside[list] += c.inside
			state.loosening[list] += c.loosening
		}
		for _, edit := range placed {
			pos := sort.Search(len(state.edits), func(i int) bool { return compareEdits(state.edits[i], edit) > 0 })
			state.edits = slices.Insert(state.edits, pos, edit)
		}
	}
	return true, true, commit
}

// settle returns the regions that placed edits, sorted, need together with
// the state's edits, merged with the state's regions they meet and grown
// until each holds, with their outcomes and the indexes of the state's
// regions they take in, in order. ok is false when a region cannot be
// checked on its own.
func (m *regionModel) settle(state *regionState, placed []placedEdit) ([]region, []*regionOutcome, []int, bool) {
	fresh := make([]region, len(placed))
	for idx, edit := range placed {
		fresh[idx] = m.regionFor(edit)
	}
	regions := m.mergeRegions(fresh)
	absorbed := make(map[int]bool)
	for {
		var more []region
		for _, reg := range regions {
			for _, idx := range state.meeting(m.docLayout, reg) {
				if !absorbed[idx] {
					absorbed[idx] = true
					more = append(more, state.regions[idx])
				}
			}
		}
		if len(more) > 0 {
			regions = m.mergeRegions(slices.Concat(regions, more))
			continue
		}
		outcomes := make([]*regionOutcome, len(regions))
		grew := false
		for idx, reg := range regions {
			from, to := m.editSpan(reg)
			// Stable, so that insertions at one offset keep their order.
			edits := slices.Concat(regionEdits(from, to, state.edits), regionEdits(from, to, placed))
			slices.SortStableFunc(edits, compareEdits)
			outcome := m.evaluate(reg, edits)
			if outcome.undecided {
				return nil, nil, nil, false
			}
			if !outcome.holds {
				grown := m.grow(reg)
				if grown == reg {
					return nil, nil, nil, false
				}
				regions[idx] = grown
				grew = true
				break
			}
			outcomes[idx] = outcome
		}
		if grew {
			regions = m.mergeRegions(regions)
			continue
		}
		taken := slices.Sorted(maps.Keys(absorbed))
		return regions, outcomes, taken, true
	}
}
