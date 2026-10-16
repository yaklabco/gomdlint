package fix

// maxSnakeSearch bounds how many edit steps a single middle-snake search may
// take. Inputs that need more edits than about twice this number in one
// region are reported as a whole-region replacement instead of a minimal
// script, which keeps the running time near O((N+M) * maxSnakeSearch) for
// large rewrites. GNU diff uses the same lower bound for its cost limit.
const maxSnakeSearch = 4096

// lineDiff marks the lines that differ between two line sequences.
type lineDiff struct {
	a, b    []int
	removed []bool
	added   []bool
}

// diffLineIDs computes a shortest edit script between left and right with
// Myers' O(ND) algorithm, using the linear-space divide and conquer
// refinement. It returns, for each line of left, whether it is removed, and
// for each line of right, whether it is added. The unmarked lines form a
// longest common subsequence of the two inputs.
func diffLineIDs(left, right []int) ([]bool, []bool) {
	ld := &lineDiff{
		a:       left,
		b:       right,
		removed: make([]bool, len(left)),
		added:   make([]bool, len(right)),
	}
	ld.compare(0, len(left), 0, len(right))
	return ld.removed, ld.added
}

// compare marks the differences between a[aLo:aHi] and b[bLo:bHi].
func (ld *lineDiff) compare(aLo, aHi, bLo, bHi int) {
	for aLo < aHi && bLo < bHi && ld.a[aLo] == ld.b[bLo] {
		aLo++
		bLo++
	}
	for aLo < aHi && bLo < bHi && ld.a[aHi-1] == ld.b[bHi-1] {
		aHi--
		bHi--
	}

	switch {
	case aLo == aHi:
		markAll(ld.added[bLo:bHi])
	case bLo == bHi:
		markAll(ld.removed[aLo:aHi])
	default:
		splitA, splitB, found := middleSnake(ld.a[aLo:aHi], ld.b[bLo:bHi])
		noProgress := (splitA == 0 && splitB == 0) || (splitA == aHi-aLo && splitB == bHi-bLo)
		if !found || noProgress {
			// No common line was found within the search bound: replace the
			// whole region. This is always a correct script, if not minimal.
			markAll(ld.removed[aLo:aHi])
			markAll(ld.added[bLo:bHi])
			return
		}
		ld.compare(aLo, aLo+splitA, bLo, bLo+splitB)
		ld.compare(aLo+splitA, aHi, bLo+splitB, bHi)
	}
}

// middleSnake finds a point (x, y) on a shortest edit path from (0, 0) to
// (len(left), len(right)) by running the forward and reverse searches until
// they overlap. Both inputs must be non-empty and must not share a first or
// last element. The boolean result is false when the inputs share no
// element, or when the search exceeds maxSnakeSearch steps.
//
// This follows the bisection in Neil Fraser's diff-match-patch, a widely used
// formulation of Myers' "find the middle snake" procedure.
func middleSnake(left, right []int) (int, int, bool) {
	limit := min((len(left)+len(right)+1)/2, maxSnakeSearch)
	search := newSnakeSearch(left, right, limit)
	for depth := range limit {
		if x, y, found := search.stepForward(depth); found {
			return x, y, true
		}
		if x, y, found := search.stepReverse(depth); found {
			return x, y, true
		}
	}
	return 0, 0, false
}

// snakeSearch holds the state of one middle-snake search. forward[offset+k]
// is the furthest x reached on diagonal k (k = x - y) by the forward search,
// and reverse[offset+k] the same for the search that runs from the end of
// both inputs. Unreached diagonals hold -1.
type snakeSearch struct {
	left, right []int
	lenA, lenB  int
	delta       int
	// front is true when the total length is odd: then the forward path
	// detects the overlap, otherwise the reverse path does.
	front            bool
	offset           int
	forward, reverse []int
	// Offsets for the start and end of the k loops, which keep each search
	// inside the edit graph.
	k1start, k1end, k2start, k2end int
}

func newSnakeSearch(left, right []int, limit int) *snakeSearch {
	// Diagonals reached in fewer than limit steps satisfy |k| < limit, and
	// each step reads its neighbours, so the arrays cover |k| <= limit+1.
	length := 2*limit + 3
	search := &snakeSearch{
		left:    left,
		right:   right,
		lenA:    len(left),
		lenB:    len(right),
		delta:   len(left) - len(right),
		front:   (len(left)-len(right))%2 != 0,
		offset:  limit + 1,
		forward: make([]int, length),
		reverse: make([]int, length),
	}
	for idx := range length {
		search.forward[idx] = -1
		search.reverse[idx] = -1
	}
	search.forward[search.offset+1] = 0
	search.reverse[search.offset+1] = 0
	return search
}

// stepForward extends the forward search by one edit step.
func (s *snakeSearch) stepForward(depth int) (int, int, bool) {
	for k1 := -depth + s.k1start; k1 <= depth-s.k1end; k1 += 2 {
		x1 := s.furthest(s.forward, k1, depth)
		y1 := x1 - k1
		for x1 < s.lenA && y1 < s.lenB && s.left[x1] == s.right[y1] {
			x1++
			y1++
		}
		s.forward[s.offset+k1] = x1

		switch {
		case x1 > s.lenA:
			// Ran off the right of the graph.
			s.k1end += 2
		case y1 > s.lenB:
			// Ran off the bottom of the graph.
			s.k1start += 2
		case s.front:
			// Mirror the reverse x onto the forward coordinates.
			if x2, reached := s.reached(s.reverse, s.delta-k1); reached && x1 >= s.lenA-x2 {
				return x1, y1, true
			}
		}
	}
	return 0, 0, false
}

// stepReverse extends the reverse search by one edit step.
func (s *snakeSearch) stepReverse(depth int) (int, int, bool) {
	for k2 := -depth + s.k2start; k2 <= depth-s.k2end; k2 += 2 {
		x2 := s.furthest(s.reverse, k2, depth)
		y2 := x2 - k2
		for x2 < s.lenA && y2 < s.lenB && s.left[s.lenA-x2-1] == s.right[s.lenB-y2-1] {
			x2++
			y2++
		}
		s.reverse[s.offset+k2] = x2

		switch {
		case x2 > s.lenA:
			// Ran off the left of the graph.
			s.k2end += 2
		case y2 > s.lenB:
			// Ran off the top of the graph.
			s.k2start += 2
		case !s.front:
			// Mirror the reverse x onto the forward coordinates.
			k1 := s.delta - k2
			if x1, reached := s.reached(s.forward, k1); reached && x1 >= s.lenA-x2 {
				return x1, x1 - k1, true
			}
		}
	}
	return 0, 0, false
}

// furthest returns the x at which diagonal k starts at the given depth: one
// step down from diagonal k+1 or one step right from diagonal k-1, whichever
// reaches further.
func (s *snakeSearch) furthest(v []int, k, depth int) int {
	idx := s.offset + k
	if k == -depth || (k != depth && v[idx-1] < v[idx+1]) {
		return v[idx+1]
	}
	return v[idx-1] + 1
}

// reached returns the furthest x recorded on diagonal k, if any.
func (s *snakeSearch) reached(v []int, k int) (int, bool) {
	idx := s.offset + k
	if idx < 0 || idx >= len(v) || v[idx] == -1 {
		return 0, false
	}
	return v[idx], true
}

// markAll sets every element of flags to true.
func markAll(flags []bool) {
	for idx := range flags {
		flags[idx] = true
	}
}
