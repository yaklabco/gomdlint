package fix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireCommonSubsequence checks that the lines left unmarked on both sides
// are the same sequence, which makes the marks a valid edit script.
func requireCommonSubsequence(t *testing.T, left, right []int, removed, added []bool) {
	t.Helper()

	var keptLeft, keptRight []int
	for idx, id := range left {
		if !removed[idx] {
			keptLeft = append(keptLeft, id)
		}
	}
	for idx, id := range right {
		if !added[idx] {
			keptRight = append(keptRight, id)
		}
	}
	require.Equal(t, keptLeft, keptRight)
}

func TestDiffWithBudget_BoundsWorkOnPathologicalInput(t *testing.T) {
	t.Parallel()

	// Distinct lines present on both sides in reverse order: nothing can be
	// discarded up front, and every search runs to its step bound.
	const count = 20000
	left := make([]int, count)
	right := make([]int, count)
	for idx := range count {
		left[idx] = idx
		right[idx] = count - 1 - idx
	}

	const budget = 1 << 20
	removed, added, remaining := diffWithBudget(left, right, budget)

	requireCommonSubsequence(t, left, right, removed, added)
	// One search may overshoot by at most a single step and its snake.
	assert.Greater(t, remaining, -(len(left) + len(right) + 1), "work must stay within the budget")
}

func TestDiffWithBudget_DiscardsLinesWithoutMatch(t *testing.T) {
	t.Parallel()

	// No line of left occurs in right: the script is found without any search.
	left := []int{0, 1, 2, 3}
	right := []int{4, 5, 6}

	removed, added, remaining := diffWithBudget(left, right, 10)

	assert.Equal(t, []bool{true, true, true, true}, removed)
	assert.Equal(t, []bool{true, true, true}, added)
	assert.Equal(t, 10, remaining, "unmatched lines need no search")
}
