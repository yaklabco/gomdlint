package lint

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestJSStringNumber covers what JavaScript's Number reads from a string,
// which markdownlint 0.41.1's rules read their numeric options with. Each
// expected number is what Number gives in Node 22; NaN, and the 0 that it
// gives for empty text or whitespace, count as no number.
func TestJSStringNumber(t *testing.T) {
	t.Parallel()

	numbers := map[string]float64{
		"10": 10, " 10 ": 10, "+10": 10, "-10": -10, "1.": 1, ".5": 0.5, "1.5": 1.5, "1e3": 1000,
		"1E-2": 0.01, "00012": 12, "0012.5": 12.5, "\u00a010\u00a0": 10, "\ufeff10": 10,
		"\u2028 10\u2029": 10, "\t\n\v\f\r10": 10, "0x1F": 31, " 0X1f ": 31, "0o17": 15, "0O17": 15,
		"0b101": 5, "0B11": 3, "-0": math.Copysign(0, -1), "0.0": 0, "1e-999": 0,
		"Infinity": math.Inf(1), "+Infinity": math.Inf(1), "-Infinity": math.Inf(-1), "1e999": math.Inf(1),
	}
	for text, want := range numbers {
		got, ok := jsStringNumber(text)
		assert.True(t, ok, "%q", text)
		// Each value is exact, so their shortest forms compare.
		assert.Equal(t, strconv.FormatFloat(want, 'g', -1, 64), strconv.FormatFloat(got, 'g', -1, 64), "%q", text)
	}

	for _, text := range []string{
		"", "  ", "1e", "e5", ".", "+", "infinity", "1_0", "\u200b10", "\u0085 10", "+0x1F", "-0x1F",
		"0x", "0o8", "0b102", "0x1p3", "1.5.2", "10px", "NaN",
	} {
		_, ok := jsStringNumber(text)
		assert.False(t, ok, "%q", text)
	}
}
