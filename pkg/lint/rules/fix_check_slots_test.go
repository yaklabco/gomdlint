package rules

import (
	"math/rand/v2"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slotSet returns slots sorted and without repeats.
func slotSet(slots []int) []int {
	sorted := slices.Clone(slots)
	sort.Ints(sorted)
	return slices.Compact(sorted)
}

// TestOrderedEditSlotsMatchesScan checks that binary search over ordered
// blocks finds the slots that scanning every block and gap finds.
func TestOrderedEditSlotsMatchesScan(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for range 3000 {
		lines := 1 + rng.IntN(30)
		var blocks [][2]int
		for line := 1 + rng.IntN(3); line <= lines; {
			end := min(line+rng.IntN(4), lines)
			blocks = append(blocks, [2]int{line, end})
			line = end + 1 + rng.IntN(3)
		}
		require.True(t, blocksOrdered(blocks, lines), "%v in %d lines", blocks, lines)
		for first := 1; first <= lines; first++ {
			for last := first; last <= lines; last++ {
				require.Equal(t, slotSet(scannedEditSlots(lines, blocks, first, last)),
					slotSet(orderedEditSlots(lines, blocks, first, last)),
					"blocks %v in %d lines, lines %d to %d", blocks, lines, first, last)
			}
		}
	}
}

func TestBlocksOrdered(t *testing.T) {
	assert.True(t, blocksOrdered(nil, 3))
	assert.True(t, blocksOrdered([][2]int{{1, 2}, {4, 4}}, 4))
	assert.False(t, blocksOrdered([][2]int{{1, 2}, {2, 3}}, 4), "blocks that share a line")
	assert.False(t, blocksOrdered([][2]int{{3, 2}}, 4), "a block that ends before it starts")
	assert.False(t, blocksOrdered([][2]int{{1, 5}}, 4), "a block past the last line")
	assert.False(t, blocksOrdered([][2]int{{0, 1}}, 4), "a block before the first line")
}
