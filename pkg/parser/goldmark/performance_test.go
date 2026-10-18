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
// inline nodes, which took over ten seconds for 40,000 closers. It compares
// the parse time of an input eight times as long with that of a short one,
// because a fixed bound in seconds fails on a busy CI machine. A linear
// parse takes about eight times as long on the longer input and a quadratic
// one about sixty-four times; the bound leaves room for a busy machine.
func TestManyClosingBracketsParseInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	const (
		small    = 5000
		factor   = 8
		maxRatio = 24
	)
	inputs := map[string]func(int) string{
		"code spans and brackets": func(n int) string {
			return strings.Repeat("`a`]", n)
		},
		"one opener before many closers": func(n int) string {
			return "[" + strings.Repeat("`a`]", n)
		},
	}
	for name, generate := range inputs {
		t.Run(name, func(t *testing.T) {
			// The fastest of a few runs is the least disturbed by other work
			// on the machine.
			fastest := func(input string) time.Duration {
				best := time.Duration(1<<63 - 1)
				for range 3 {
					best = min(best, parseDuration(t, input))
				}
				return best
			}
			short := fastest(generate(small))
			long := fastest(generate(factor * small))
			ratio := float64(long) / float64(short)
			t.Logf("%v for %d closers, %v for %d, ratio %.1f", short, small, long, factor*small, ratio)
			require.Less(t, ratio, float64(maxRatio), "parse time grows faster than the input")
		})
	}
}

// parseDuration parses input with the GFM parser and returns how long the
// parse took.
func parseDuration(t *testing.T, input string) time.Duration {
	t.Helper()
	parser := New(FlavorGFM)
	start := time.Now()
	file, err := parser.Parse(context.Background(), "brackets.md", []byte(input))
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.NotNil(t, file.Root)
	return elapsed
}
