package rules

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/mdast"
)

// scanInside reports whether offset lies in one of ranges, which are sorted
// by start and may nest, by scanning the ranges from the first: the
// definition that sortedCover answers in linear time.
func scanInside(ranges []mdast.SourceRange, offset int) bool {
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

// TestSortedCoverMatchesScan checks sortedCover against scanning the ranges
// for every offset, on random ranges that nest, touch and repeat.
func TestSortedCoverMatchesScan(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		var ranges []mdast.SourceRange
		for range rng.IntN(8) {
			start := rng.IntN(40)
			ranges = append(ranges, mdast.SourceRange{StartOffset: start, EndOffset: start + rng.IntN(10)})
		}
		slices.SortFunc(ranges, func(a, b mdast.SourceRange) int { return a.StartOffset - b.StartOffset })
		cover := sortedCover{ranges: ranges, end: math.MinInt}
		for offset := range 50 {
			require.Equal(t, scanInside(ranges, offset), cover.inside(offset), "ranges %v offset %d", ranges, offset)
		}
	}
}
