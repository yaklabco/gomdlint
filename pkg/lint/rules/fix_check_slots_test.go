package rules

import (
	"math/rand/v2"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scanEditSlots returns the slots of editSlots for the lines first to last
// by scanning every block and gap, as editSlots does for blocks that are
// not ordered.
func scanEditSlots(lines int, blocks [][2]int, first, last int) []int {
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
				assert.Equal(t, slotSet(scanEditSlots(lines, blocks, first, last)),
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
