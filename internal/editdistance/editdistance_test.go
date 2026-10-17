package editdistance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevenshtein(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		left, right string
		want        int
	}{
		{"", "", 0},
		{"same", "same", 0},
		{"abc", "", 3},
		{"flavor", "flavour", 1},
		{"no-trailling-spaces", "no-trailing-spaces", 1},
		{"ab", "ba", 2},
		{"kitten", "sitting", 3},
	} {
		assert.Equal(t, tt.want, Levenshtein(tt.left, tt.right), "%q to %q", tt.left, tt.right)
		assert.Equal(t, tt.want, Levenshtein(tt.right, tt.left), "%q to %q", tt.right, tt.left)
	}
}
