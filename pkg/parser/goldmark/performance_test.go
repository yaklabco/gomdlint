package goldmark

import (
	"context"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestManyClosingBracketsParseInLinearTime guards against a quadratic scan
// for the opening bracket of a link: every "]" used to walk all earlier
// inline nodes, which took over ten seconds for 40,000 closers.
//
// A fixed bound in seconds fails on a busy CI machine, so the test compares
// two measurements of the same amount of input instead: one parse of a long
// input against a batch of parses of an input an eighth as long. A linear
// parse takes about as long either way and a quadratic one about eight
// times as long for the long input. Both measurements last about as long,
// so they meet the same disturbances; they run in turns, the fastest of
// each counts, and garbage collection is paused while they are timed,
// because marking the long parse's growing tree would otherwise weigh on it
// alone.
func TestManyClosingBracketsParseInLinearTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	const (
		small  = 5000
		factor = 8
		rounds = 5
		// A linear parse gives a ratio near one and the quadratic scan near
		// eight; the bound leaves room for a busy machine.
		maxRatio = 3
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
			short, long := generate(small), generate(factor*small)
			runtime.GC()
			defer debug.SetGCPercent(debug.SetGCPercent(-1))

			// Warm up both inputs before timing them.
			parseDuration(t, short)
			parseDuration(t, long)

			// Collecting before each measurement, outside the timers, keeps
			// the paused collector from letting the heap grow for the whole
			// test.
			bestBatch, bestLong := time.Duration(1<<63-1), time.Duration(1<<63-1)
			for range rounds {
				runtime.GC()
				var batch time.Duration
				for range factor {
					batch += parseDuration(t, short)
				}
				bestBatch = min(bestBatch, batch)
				runtime.GC()
				bestLong = min(bestLong, parseDuration(t, long))
			}
			ratio := float64(bestLong) / float64(bestBatch)
			t.Logf("%d parses of %d closers in %v, one parse of %d in %v, ratio %.2f",
				factor, small, bestBatch, factor*small, bestLong, ratio)
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
