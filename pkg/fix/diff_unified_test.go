package fix_test

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklabco/gomdlint/pkg/fix"
)

func TestGenerateDiff_ShowsAddedTrailingNewline(t *testing.T) {
	t.Parallel()

	diff := fix.GenerateDiff("doc.md", []byte("# Title\n\nSome text"), []byte("# Title\n\nSome text\n"))
	require.NotNil(t, diff, "adding only a final newline must produce a diff")

	want := "--- a/doc.md\n" +
		"+++ b/doc.md\n" +
		"@@ -1,3 +1,3 @@\n" +
		" # Title\n" +
		" \n" +
		"-Some text\n" +
		"\\ No newline at end of file\n" +
		"+Some text\n"
	assert.Equal(t, want, diff.String())
	assert.Equal(t, 1, diff.Additions)
	assert.Equal(t, 1, diff.Deletions)
}

func TestGenerateDiff_ShowsRemovedTrailingNewline(t *testing.T) {
	t.Parallel()

	diff := fix.GenerateDiff("doc.md", []byte("a\nb\n"), []byte("a\nb"))
	require.NotNil(t, diff)

	want := "--- a/doc.md\n" +
		"+++ b/doc.md\n" +
		"@@ -1,2 +1,2 @@\n" +
		" a\n" +
		"-b\n" +
		"+b\n" +
		"\\ No newline at end of file\n"
	assert.Equal(t, want, diff.String())
}

func TestGenerateDiff_MarksUnchangedLastLineWithoutNewline(t *testing.T) {
	t.Parallel()

	diff := fix.GenerateDiff("doc.md", []byte("a\nb\nc"), []byte("x\nb\nc"))
	require.NotNil(t, diff)

	want := "--- a/doc.md\n" +
		"+++ b/doc.md\n" +
		"@@ -1,3 +1,3 @@\n" +
		"-a\n" +
		"+x\n" +
		" b\n" +
		" c\n" +
		"\\ No newline at end of file\n"
	assert.Equal(t, want, diff.String())
}

func TestGenerateDiff_EmptySideHunkHeaders(t *testing.T) {
	t.Parallel()

	// A range with no lines names the line before it, as in GNU diff and git.
	added := fix.GenerateDiff("doc.md", nil, []byte("a\nb\n"))
	require.NotNil(t, added)
	assert.Contains(t, added.String(), "@@ -0,0 +1,2 @@\n")

	removed := fix.GenerateDiff("doc.md", []byte("a\nb\n"), nil)
	require.NotNil(t, removed)
	assert.Contains(t, removed.String(), "@@ -1,2 +0,0 @@\n")
}

func TestGenerateDiff_IsMinimalAndReconstructsModified(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(20261016, 7))
	for iteration := range 3000 {
		original := randomDocument(rng)
		modified := randomDocument(rng)

		diff := fix.GenerateDiff("doc.md", []byte(original), []byte(modified))
		if original == modified {
			require.Nil(t, diff, "iteration %d: identical content", iteration)
			continue
		}
		require.NotNil(t, diff, "iteration %d: %q -> %q", iteration, original, modified)

		rebuilt := applyDiff(t, []byte(original), diff)
		require.Equal(t, modified, string(rebuilt), "iteration %d: %q -> %q", iteration, original, modified)

		origLines := splitKeepEnds([]byte(original))
		modLines := splitKeepEnds([]byte(modified))
		minimal := len(origLines) + len(modLines) - 2*lcsLength(origLines, modLines)
		require.Equal(t, minimal, diff.Additions+diff.Deletions,
			"iteration %d: diff is not minimal for %q -> %q", iteration, original, modified)
	}
}

// TestGenerateDiff_UsesLinearMemory measures heap allocations, so it does not
// run in parallel with the other tests in this package.
func TestGenerateDiff_UsesLinearMemory(t *testing.T) {
	const lineCount = 4000

	var origBuilder, modBuilder strings.Builder
	for idx := range lineCount {
		fmt.Fprintf(&origBuilder, "line %d\n", idx)
		if idx%2 == 0 {
			fmt.Fprintf(&modBuilder, "changed %d\n", idx)
		} else {
			fmt.Fprintf(&modBuilder, "line %d\n", idx)
		}
	}
	original := []byte(origBuilder.String())
	modified := []byte(modBuilder.String())

	// TotalAlloc counts every allocation, collected or not, so no GC is needed.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	diff := fix.GenerateDiff("doc.md", original, modified)

	runtime.ReadMemStats(&after)

	require.NotNil(t, diff)
	assert.Equal(t, lineCount/2, diff.Additions)
	assert.Equal(t, lineCount/2, diff.Deletions)
	assert.Equal(t, string(modified), string(applyDiff(t, original, diff)))

	// A quadratic LCS table for 4000x4000 lines needs about 128 MB.
	const maxAllocated = 32 << 20
	allocated := after.TotalAlloc - before.TotalAlloc
	assert.Less(t, allocated, uint64(maxAllocated), "GenerateDiff allocated %d bytes", allocated)
}

func TestGenerateDiff_RewritingEveryLineIsExact(t *testing.T) {
	t.Parallel()

	const lineCount = 50000

	var origBuilder, modBuilder strings.Builder
	for idx := range lineCount {
		fmt.Fprintf(&origBuilder, "line %d   \n", idx)
		fmt.Fprintf(&modBuilder, "line %d\n", idx)
	}
	original := []byte(origBuilder.String())
	modified := []byte(modBuilder.String())

	diff := fix.GenerateDiff("doc.md", original, modified)

	require.NotNil(t, diff)
	assert.Equal(t, lineCount, diff.Additions)
	assert.Equal(t, lineCount, diff.Deletions)
	assert.Equal(t, string(modified), string(applyDiff(t, original, diff)))
}

func TestGenerateDiff_ManyScatteredFixesStayMinimal(t *testing.T) {
	t.Parallel()

	// Every fourth line of a large file changes: 10000 changed lines, more
	// than one bounded search covers, so the region must not be replaced
	// whole.
	const lineCount = 20000
	var origBuilder, modBuilder strings.Builder
	changed := 0
	for idx := range lineCount {
		fmt.Fprintf(&origBuilder, "line %d\n", idx)
		if idx%4 == 0 {
			fmt.Fprintf(&modBuilder, "fixed line %d\n", idx)
			changed++
		} else {
			fmt.Fprintf(&modBuilder, "line %d\n", idx)
		}
	}
	original := []byte(origBuilder.String())
	modified := []byte(modBuilder.String())

	diff := fix.GenerateDiff("doc.md", original, modified)

	require.NotNil(t, diff)
	assert.Equal(t, changed, diff.Additions)
	assert.Equal(t, changed, diff.Deletions)
	assert.Equal(t, string(modified), string(applyDiff(t, original, diff)))
}

func TestGenerateDiff_ManyRepeatedLineFixesStayMinimal(t *testing.T) {
	t.Parallel()

	// A fix that removes 10000 extra blank lines between paragraphs: the
	// removed lines also occur in the fixed file, so they stay in the search
	// and exceed one bounded search.
	const paragraphs = 10000
	var origBuilder, modBuilder strings.Builder
	for idx := range paragraphs {
		fmt.Fprintf(&origBuilder, "Paragraph %d.\n\n\n", idx)
		fmt.Fprintf(&modBuilder, "Paragraph %d.\n\n", idx)
	}
	original := []byte(origBuilder.String())
	modified := []byte(modBuilder.String())

	diff := fix.GenerateDiff("doc.md", original, modified)

	require.NotNil(t, diff)
	assert.Equal(t, 0, diff.Additions)
	assert.Equal(t, paragraphs, diff.Deletions)
	assert.Equal(t, string(modified), string(applyDiff(t, original, diff)))
}

// randomDocument returns a short document over a tiny alphabet, so random
// pairs share many lines, sometimes without a final newline.
func randomDocument(rng *rand.Rand) string {
	alphabet := []string{"a", "b", "c", ""}
	count := rng.IntN(9)
	lines := make([]string, count)
	for idx := range lines {
		lines[idx] = alphabet[rng.IntN(len(alphabet))]
	}
	doc := strings.Join(lines, "\n")
	if count > 0 && rng.IntN(3) > 0 {
		doc += "\n"
	}
	return doc
}

// splitKeepEnds splits content into lines that keep their trailing newline.
func splitKeepEnds(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(content), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lcsLength returns the length of the longest common subsequence of two line slices.
func lcsLength(left, right []string) int {
	prev := make([]int, len(right)+1)
	curr := make([]int, len(right)+1)
	for row := 1; row <= len(left); row++ {
		for col := 1; col <= len(right); col++ {
			if left[row-1] == right[col-1] {
				curr[col] = prev[col-1] + 1
			} else {
				curr[col] = max(prev[col], curr[col-1])
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(right)]
}

// applyDiff rebuilds the modified content from the original and the diff's
// hunks. It fails the test when a hunk's position, counts or context do not
// match the original.
func applyDiff(t *testing.T, original []byte, diff *fix.Diff) []byte {
	t.Helper()

	origLines := splitKeepEnds(original)
	var out []string
	next := 0
	for hunkIdx, hunk := range diff.Hunks {
		start := hunk.OriginalStart - 1
		require.GreaterOrEqual(t, start, next, "hunk %d overlaps the previous hunk", hunkIdx)
		out = append(out, origLines[next:start]...)
		require.Len(t, out, hunk.ModifiedStart-1, "hunk %d has a wrong modified start", hunkIdx)

		idx := start
		var origCount, modCount int
		for _, line := range hunk.Lines {
			text := line.Content
			if !line.NoNewline {
				text += "\n"
			}
			switch line.Kind {
			case fix.DiffLineContext:
				require.Less(t, idx, len(origLines), "hunk %d runs past the original", hunkIdx)
				require.Equal(t, origLines[idx], text, "hunk %d context mismatch", hunkIdx)
				out = append(out, text)
				idx++
				origCount++
				modCount++
			case fix.DiffLineRemove:
				require.Less(t, idx, len(origLines), "hunk %d runs past the original", hunkIdx)
				require.Equal(t, origLines[idx], text, "hunk %d removal mismatch", hunkIdx)
				idx++
				origCount++
			case fix.DiffLineAdd:
				out = append(out, text)
				modCount++
			}
		}
		require.Equal(t, hunk.OriginalCount, origCount, "hunk %d original count", hunkIdx)
		require.Equal(t, hunk.ModifiedCount, modCount, "hunk %d modified count", hunkIdx)
		next = idx
	}
	out = append(out, origLines[next:]...)
	return []byte(strings.Join(out, ""))
}
