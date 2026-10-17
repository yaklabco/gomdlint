// Package editdistance measures how far apart two strings are, for
// suggesting a known key in place of a mistyped one.
package editdistance

// Levenshtein returns the Levenshtein distance between two strings: the
// fewest single-byte insertions, deletions and substitutions that turn one
// into the other.
func Levenshtein(left, right string) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)
	for col := range previous {
		previous[col] = col
	}
	for row := 1; row <= len(left); row++ {
		current[0] = row
		for col := 1; col <= len(right); col++ {
			cost := 1
			if left[row-1] == right[col-1] {
				cost = 0
			}
			current[col] = min(previous[col]+1, current[col-1]+1, previous[col-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(right)]
}
