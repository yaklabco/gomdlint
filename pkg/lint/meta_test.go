package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGomdlintOnlyRule(t *testing.T) {
	for id, want := range map[string]bool{
		"MDL001": true, "MDL003": true,
		"MD009": false, "MD060": false,
		"CUSTOM001": false, "": false,
	} {
		assert.Equal(t, want, IsGomdlintOnlyRule(id), "rule ID %q", id)
	}
}
