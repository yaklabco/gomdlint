package goldmark

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestManyClosingBracketsParseInLinearTime guards against a quadratic scan
// for the opening bracket of a link: every "]" used to walk all earlier
// inline nodes, which took over ten seconds for these inputs. They now
// parse in well under a second, so the bound holds on slow CI machines and
// under -race while still catching the quadratic scan.
func TestManyClosingBracketsParseInLinearTime(t *testing.T) {
	const repeats = 40000
	inputs := map[string]string{
		"code spans and brackets":        strings.Repeat("`a`]", repeats),
		"one opener before many closers": "[" + strings.Repeat("`a`]", repeats),
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			parser := New(FlavorGFM)
			start := time.Now()
			file, err := parser.Parse(context.Background(), "brackets.md", []byte(input))
			elapsed := time.Since(start)
			require.NoError(t, err)
			require.NotNil(t, file.Root)
			t.Logf("parsed %d bytes in %s", len(input), elapsed)
			require.Less(t, elapsed, 4*time.Second, "parsing took %s", elapsed)
		})
	}
}
