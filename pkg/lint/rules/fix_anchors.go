package rules

import (
	"cmp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Anchors of edited documents.
//
// GitHub gives a heading the slug of its text as its anchor, or, when that
// is taken, the slug followed by "-1", "-2" and so on: the first number,
// after the last one the slug was given, whose key is free (see
// signatureWriter.anchor and refs.AnchorMap). The writer keeps a count for
// every key: a key gets the count 0 when a heading takes it as its anchor,
// and a slug's count goes up by one for each number it tries. So a key's
// counts are 0, 1, 2 and so on, the first set by the heading that takes it,
// and a key is taken once it has a count.
//
// anchorHistory records the counts of the original document. A region check
// that changes the slugs of a region's headings works out their anchors from
// the counts before the region (replay), and whether the counts after it are
// as before (sameCounts). Finding a number passes every taken number on the
// way; replay passes a run of numbers that headings before the region took
// in one step, with an index of the numbered keys (numberedKeys), so that a
// heading costs a step for each number taken inside the region, not for
// each number taken before it. When the counts after a region change, the
// anchors of later headings can change, and anchorWalk finds which: only
// headings that set or pass a key whose count changed can, and the walk
// takes only those.

// anchorHistory records the slugs and anchors that headings get and every
// change to the anchor counts, so that the counts before any heading can be
// looked up.
type anchorHistory struct {
	slugs, anchors []string
	// writes holds, for each key, its counts in order, each with the index
	// of the heading that set it.
	writes map[string][]anchorWrite
	// keys holds, for each heading, the keys it set: its slug, when it
	// counted it, and its anchor.
	keys [][]string
	// numbered indexes the numbered keys by slug; it is built on first use.
	numbered map[string]*numberedKeys
	// lookups counts the counts and numbers looked up, for tests that bound
	// it.
	lookups int
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
	// A heading counts its slug once for each number it tries.
	if keys := h.keys[heading]; len(keys) == 0 || keys[len(keys)-1] != key {
		h.keys[heading] = append(keys, key)
	}
}

// recordAnchors returns the history of giving headings with the given slugs,
// in order, the anchors GitHub gives them, numbering repeated slugs as
// signatureWriter.anchor and refs.AnchorMap do.
func recordAnchors(slugs []string) anchorHistory {
	var history anchorHistory
	counts := make(map[string]int, len(slugs))
	for _, slug := range slugs {
		unique := slug
		for {
			if _, taken := counts[unique]; !taken {
				break
			}
			counts[slug]++
			history.write(slug, counts[slug])
			unique = numberedKey(slug, counts[slug])
		}
		counts[unique] = 0
		history.write(unique, 0)
		history.slugs = append(history.slugs, slug)
		history.anchors = append(history.anchors, unique)
	}
	return history
}

// numberedKey returns the key of a slug with a number.
func numberedKey(slug string, number int) string {
	return slug + "-" + strconv.Itoa(number)
}

// splitNumbered returns the slug and number of a key that numberedKey gives
// for a number from 1 on, or false.
func splitNumbered(key string) (string, int, bool) {
	dash := strings.LastIndexByte(key, '-')
	if dash < 0 {
		return "", 0, false
	}
	digits := key[dash+1:]
	number, err := strconv.Atoi(digits)
	if err != nil || number < 1 || strconv.Itoa(number) != digits {
		return "", 0, false
	}
	return key[:dash], number, true
}

// lookup returns the count of key before the given heading.
func (h *anchorHistory) lookup(key string, before int) (int, bool) {
	h.lookups++
	writes := h.writes[key]
	idx := sort.Search(len(writes), func(i int) bool { return writes[i].heading >= before })
	if idx == 0 {
		return 0, false
	}
	return writes[idx-1].value, true
}

// numberedKeys holds a history's keys slug-n for one slug: the numbers n,
// sorted, with the heading that took each key, so that the first number from
// some n on whose key is free before a heading is found without trying each
// taken number.
type numberedKeys struct {
	numbers []int
	// runEnd holds, for each position, the last position of the run of
	// consecutive numbers that holds it.
	runEnd []int
	// latest holds, at level j and position i, the last heading to take the
	// key of a number at positions i to i+2^j-1.
	latest [][]int
}

// index returns the numbered keys of the history by slug, building them on
// first use.
func (h *anchorHistory) index() map[string]*numberedKeys {
	if h.numbered != nil {
		return h.numbered
	}
	type entry struct{ number, heading int }
	bySlug := make(map[string][]entry)
	for key, writes := range h.writes {
		if slug, number, ok := splitNumbered(key); ok {
			bySlug[slug] = append(bySlug[slug], entry{number: number, heading: writes[0].heading})
		}
	}
	h.numbered = make(map[string]*numberedKeys, len(bySlug))
	for slug, entries := range bySlug {
		slices.SortFunc(entries, func(a, b entry) int { return cmp.Compare(a.number, b.number) })
		keys := &numberedKeys{numbers: make([]int, len(entries)), runEnd: make([]int, len(entries))}
		taken := make([]int, len(entries))
		for idx, entry := range entries {
			keys.numbers[idx], taken[idx] = entry.number, entry.heading
		}
		for idx := len(entries) - 1; idx >= 0; idx-- {
			keys.runEnd[idx] = idx
			if idx+1 < len(entries) && entries[idx+1].number == entries[idx].number+1 {
				keys.runEnd[idx] = keys.runEnd[idx+1]
			}
		}
		keys.latest = [][]int{taken}
		for width := 2; width <= len(entries); width *= 2 {
			below := keys.latest[len(keys.latest)-1]
			level := make([]int, len(entries)-width+1)
			for idx := range level {
				level[idx] = max(below[idx], below[idx+width/2])
			}
			keys.latest = append(keys.latest, level)
		}
		h.numbered[slug] = keys
	}
	return h.numbered
}

// freeBefore returns the first number from `from` on whose key, slug with
// the number, no heading before the given one took.
func (h *anchorHistory) freeBefore(slug string, from, before int) int {
	h.lookups++
	keys := h.index()[slug]
	if keys == nil {
		return from
	}
	pos := sort.SearchInts(keys.numbers, from)
	if pos == len(keys.numbers) || keys.numbers[pos] != from {
		return from
	}
	// The numbers from pos to the end of its run follow one another; pass
	// those taken before the heading, the longest stretch of them whose
	// latest taker comes before it.
	end, at := keys.runEnd[pos], pos
	for level := len(keys.latest) - 1; level >= 0; level-- {
		if width := 1 << level; at+width-1 <= end && keys.latest[level][at] < before {
			at += width
		}
	}
	return from + at - pos
}

// replay returns the anchors that headings with the given slugs get when
// they follow the first `before` headings of the document, as
// signatureWriter.anchor gives them, and the counts they set.
func (h *anchorHistory) replay(slugs []string, before int) ([]string, map[string]int) {
	counts := make(map[string]int)
	anchors := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		unique := slug
		value, taken := counts[slug]
		if !taken {
			value, taken = h.lookup(slug, before)
		}
		if taken {
			// Numbers taken before the region are passed a run at a time;
			// each number taken in the region is passed once, as the slug's
			// count then goes past it.
			number := value + 1
			for {
				number = h.freeBefore(slug, number, before)
				if _, local := counts[numberedKey(slug, number)]; !local {
					break
				}
				number++
			}
			counts[slug] = number
			unique = numberedKey(slug, number)
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

// anchorChange replaces headings [from, to) of a document with headings
// with the given slugs.
type anchorChange struct {
	from, to int
	slugs    []string
}

// keeps reports whether the document with changes, sorted and apart, gives
// its headings the document's own anchors, in order, as the anchors item of
// the outline compares them: joined with a separator that no anchor holds.
//
// Lists of different lengths join to different texts, but for no anchors
// and one empty anchor. Otherwise the anchors at each place of the list must
// be the same. The walk takes the changes in order, the headings each
// replaces, and the headings between them that can get another anchor. A
// heading between changes keeps its own anchor unless it sets or passes a
// key whose count the changes changed; where the changes before it changed
// the number of headings, its own anchor stands at another place of the
// list, and differs from the anchor there, since a document's anchors
// differ from one another.
func (h *anchorHistory) keeps(changes []anchorChange) bool {
	total := len(h.anchors)
	for _, change := range changes {
		total += len(change.slugs) - (change.to - change.from)
	}
	if total != len(h.anchors) {
		if max(total, len(h.anchors)) > 1 {
			return false
		}
		var edited []string
		next := 0
		for _, change := range changes {
			edited = append(append(edited, h.slugs[next:change.from]...), change.slugs...)
			next = change.to
		}
		edited = append(edited, h.slugs[next:]...)
		h.lookups += len(edited)
		return strings.Join(recordAnchors(edited).anchors, "\x00") == strings.Join(h.anchors, "\x00")
	}
	walk := newAnchorWalk(h)
	shift := 0
	for _, change := range changes {
		if !walk.keepsUntil(change.from, shift) {
			return false
		}
		for idx, anchor := range walk.change(change) {
			if anchor != h.anchors[change.from+shift+idx] {
				return false
			}
		}
		shift += len(change.slugs) - (change.to - change.from)
	}
	return walk.keepsUntil(len(h.anchors), shift)
}

// anchorWalk works out the anchors of the document with changes, heading by
// heading in order, from the document's counts and the counts that differ.
//
// A heading of the document outside the changes gets another anchor only
// if it reads a key whose count differs: its slug, or a key it passes or
// takes. The keys a heading sets are those it reads, but for the keys it
// passes, which have counts already. So a key whose count differs next
// matters at the next heading that sets it, and a key that the document has
// and the edited document does not also at the heading whose count of the
// slug it numbers reaches its number, the heading that passes it; those
// headings are the key's events. Between events the counts that differ stay
// as they are. A heading that passes, in the document, a key that the
// edited document lacks gets another anchor: every number up to a slug's
// count has its key, so the edited document's count of that slug is below
// the key's number, and the heading takes that key, a number before it, or
// the slug itself. A heading the walk takes and that keeps its anchor sets
// what the document's heading sets, so the key whose event it was no longer
// differs. So each heading the walk takes gets another anchor or makes a
// key agree.
type anchorWalk struct {
	h *anchorHistory
	// pos is the next heading of the document to take: the counts of the
	// edited document are the document's before pos, but for diff.
	pos int
	// diff holds each key whose count in the edited document differs from
	// the document's before pos, with present false for a key that the
	// document has and the edited document does not.
	diff map[string]walkCount
	// version counts the changes to each key's event, so that events left
	// behind are passed over.
	version map[string]int
	// events holds the events, the first heading first.
	events minHeap[walkEvent]
	// freed holds, for each slug, the numbers whose keys the document has
	// and the edited document does not, smallest first. A number below the
	// slug's count, and one whose key the edited document has since taken,
	// is dropped when it comes up.
	freed map[string]*minHeap[int]
}

// walkCount is the count of a key, or the absence of one.
type walkCount struct {
	value   int
	present bool
}

func newAnchorWalk(history *anchorHistory) *anchorWalk {
	return &anchorWalk{
		h:       history,
		diff:    make(map[string]walkCount),
		version: make(map[string]int),
		events:  minHeap[walkEvent]{less: func(a, b walkEvent) bool { return a.heading < b.heading }},
		freed:   make(map[string]*minHeap[int]),
	}
}

// edited returns the count of key in the edited document before pos.
func (w *anchorWalk) edited(key string) walkCount {
	if count, found := w.diff[key]; found {
		return count
	}
	value, present := w.h.lookup(key, w.pos)
	return walkCount{value: value, present: present}
}

// editedWith returns the count of key in the edited document before pos and
// the headings of a change already taken, whose counts local holds.
func (w *anchorWalk) editedWith(key string, local map[string]int) walkCount {
	if value, found := local[key]; found {
		return walkCount{value: value, present: true}
	}
	return w.edited(key)
}

// anchor returns the anchor that a heading with slug gets in the edited
// document at pos, after the headings of a change whose counts local holds,
// and the slug's count after it, with whether it counted the slug.
func (w *anchorWalk) anchor(slug string, local map[string]int) (string, int, bool) {
	count := w.editedWith(slug, local)
	if !count.present {
		return slug, 0, false
	}
	number := count.value + 1
	for {
		// The keys between number and free are taken in the document before
		// pos; the edited document has them too, but for freed ones.
		free := w.h.freeBefore(slug, number, w.pos)
		if freed, found := w.firstFreed(slug, number, free); found {
			free = freed
		}
		key := numberedKey(slug, free)
		if !w.editedWith(key, local).present {
			return key, free, true
		}
		number = free + 1
	}
}

// firstFreed returns the smallest number from `from` and below `below` whose
// key, slug with the number, the document has and the edited document does
// not, or false. Numbers below from are dropped: the slug's count is at
// least from-1, and only goes up.
func (w *anchorWalk) firstFreed(slug string, from, below int) (int, bool) {
	numbers := w.freed[slug]
	for numbers != nil && len(numbers.items) > 0 {
		number := numbers.items[0]
		count, found := w.diff[numberedKey(slug, number)]
		if number < from || !found || count.present {
			numbers.drop()
			continue
		}
		return number, number < below
	}
	return 0, false
}

// take works out the anchor of heading at of the document in the edited
// document, where no heading from pos up to it gets another anchor, and
// moves past it.
func (w *anchorWalk) take(at int) string {
	w.pos = at
	due := w.popEvents(at + 1)
	slug := w.h.slugs[at]
	anchor, number, counted := w.anchor(slug, nil)
	keys := slices.Concat([]string{slug, anchor}, w.h.keys[at])
	counts := make([]walkCount, len(keys))
	for idx, key := range keys {
		switch {
		case key == anchor:
			counts[idx] = walkCount{present: true}
		case key == slug && counted:
			counts[idx] = walkCount{value: number, present: true}
		default:
			counts[idx] = w.edited(key)
		}
	}
	w.pos = at + 1
	w.settle(keys, counts, due)
	return anchor
}

// change works out the anchors of a change's headings in the edited
// document, where no heading from pos up to the change gets another anchor,
// and moves past the headings it replaces.
func (w *anchorWalk) change(change anchorChange) []string {
	w.pos = change.from
	local := make(map[string]int)
	anchors := make([]string, 0, len(change.slugs))
	keys := make([]string, 0, 2*len(change.slugs))
	for _, slug := range change.slugs {
		anchor, number, counted := w.anchor(slug, local)
		if counted {
			local[slug] = number
			keys = append(keys, slug)
		}
		local[anchor] = 0
		keys = append(keys, anchor)
		anchors = append(anchors, anchor)
	}
	for heading := change.from; heading < change.to; heading++ {
		keys = append(keys, w.h.keys[heading]...)
	}
	counts := make([]walkCount, len(keys))
	for idx, key := range keys {
		counts[idx] = w.editedWith(key, local)
	}
	w.pos = change.to
	// Events at the headings replaced are moot.
	w.settle(keys, counts, w.popEvents(change.to))
	return anchors
}

// settle records the counts that keys have in the edited document before
// pos and gives each key that differs, and each key whose event was due,
// its next event.
func (w *anchorWalk) settle(keys []string, counts []walkCount, due []string) {
	for idx, key := range keys {
		value, present := w.h.lookup(key, w.pos)
		if counts[idx] == (walkCount{value: value, present: present}) {
			delete(w.diff, key)
		} else {
			w.diff[key] = counts[idx]
			if slug, number, ok := splitNumbered(key); ok && !counts[idx].present {
				numbers := w.freed[slug]
				if numbers == nil {
					numbers = &minHeap[int]{less: func(a, b int) bool { return a < b }}
					w.freed[slug] = numbers
				}
				numbers.push(number)
			}
		}
		w.schedule(key)
	}
	for _, key := range due {
		w.schedule(key)
	}
}

// schedule gives key, when its count differs, its next event from pos on.
func (w *anchorWalk) schedule(key string) {
	w.version[key]++
	count, found := w.diff[key]
	if !found {
		return
	}
	// The next heading that sets the key.
	writes := w.h.writes[key]
	idx := sort.Search(len(writes), func(i int) bool { return writes[i].heading >= w.pos })
	at, found := 0, idx < len(writes)
	if found {
		at = writes[idx].heading
	}
	// The heading that passes a key the edited document does not have: the
	// one whose count of the slug reaches its number, as the slug's counts
	// are 0, 1, 2 and so on.
	if slug, number, ok := splitNumbered(key); ok && !count.present {
		if counts := w.h.writes[slug]; number < len(counts) && counts[number].heading >= w.pos &&
			(!found || counts[number].heading < at) {
			at, found = counts[number].heading, true
		}
	}
	if found {
		w.events.push(walkEvent{heading: at, key: key, version: w.version[key]})
	}
}

// popEvents removes the events before a heading and returns the keys of
// those still current.
func (w *anchorWalk) popEvents(before int) []string {
	var due []string
	for len(w.events.items) > 0 && w.events.items[0].heading < before {
		event := w.events.items[0]
		w.events.drop()
		if event.version == w.version[event.key] {
			due = append(due, event.key)
		}
	}
	return due
}

// next returns the first heading from pos on and before until that the
// current events name, or until.
func (w *anchorWalk) next(until int) int {
	for len(w.events.items) > 0 {
		event := w.events.items[0]
		if event.version != w.version[event.key] {
			w.events.drop()
			continue
		}
		return min(event.heading, until)
	}
	return until
}

// keepsUntil takes the headings from pos up to until that can get another
// anchor, where the changes before them changed the number of headings by
// shift, and reports whether each gets the anchor at its place in the
// document's list.
func (w *anchorWalk) keepsUntil(until, shift int) bool {
	for w.pos < until {
		next := w.next(until)
		if shift != 0 && next != w.pos {
			// The heading at pos keeps its own anchor, at another place.
			return false
		}
		if next == until {
			w.pos = until
			break
		}
		if w.take(next) != w.h.anchors[next+shift] {
			return false
		}
	}
	return true
}

// walkEvent names a heading at which a key whose count differs matters.
type walkEvent struct {
	heading int
	key     string
	version int
}

// minHeap is a binary heap whose first item is the least, as less orders
// the items.
type minHeap[T any] struct {
	items []T
	less  func(a, b T) bool
}

// push adds an item.
func (h *minHeap[T]) push(item T) {
	h.items = append(h.items, item)
	for at := len(h.items) - 1; at > 0; {
		parent := (at - 1) / 2
		if !h.less(h.items[at], h.items[parent]) {
			break
		}
		h.items[at], h.items[parent] = h.items[parent], h.items[at]
		at = parent
	}
}

// drop removes the first item, items[0], of a heap that has one.
func (h *minHeap[T]) drop() {
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	var zero T
	h.items[last] = zero
	h.items = h.items[:last]
	for at := 0; ; {
		least := at
		if left := 2*at + 1; left < last && h.less(h.items[left], h.items[least]) {
			least = left
		}
		if right := 2*at + 2; right < last && h.less(h.items[right], h.items[least]) {
			least = right
		}
		if least == at {
			return
		}
		h.items[at], h.items[least] = h.items[least], h.items[at]
		at = least
	}
}
